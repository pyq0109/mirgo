package engine

import (
	"fmt"
	"image"
	"os"
	"sync"

	"github.com/go-gl/gl/v3.3-core/gl"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// glyphEntry is a cached glyph texture.
type glyphEntry struct {
	tex     uint32 // GL texture ID
	w, h    int    // glyph pixel size
	advance int    // horizontal advance in pixels
	bearingX int   // left-side bearing
	bearingY int   // top bearing (from baseline to top of glyph)
}

// TextRenderer renders text using a TTF font with glyph texture caching.
type TextRenderer struct {
	gl       *GLState
	face     font.Face
	ascent   int // pixels from baseline to top of line
	cache    map[rune]*glyphEntry
	cacheMu  sync.RWMutex
	size     float64
}

// fontSearchPaths lists common Windows Chinese font paths.
var fontSearchPaths = []string{
	`C:\Windows\Fonts\msyh.ttc`,  // Microsoft YaHei
	`C:\Windows\Fonts\msyhbd.ttc`, // Microsoft YaHei Bold
	`C:\Windows\Fonts\simsun.ttc`, // SimSun
	`C:\Windows\Fonts\simhei.ttf`, // SimHei
	`C:\Windows\Fonts\arial.ttf`,  // Arial (English fallback)
}

// NewTextRenderer creates a TextRenderer. If fontPath is empty, it tries common Windows fonts.
func NewTextRenderer(glState *GLState, fontPath string, size float64) (*TextRenderer, error) {
	if size <= 0 {
		size = 16
	}

	resolvedPath := fontPath
	if resolvedPath == "" {
		for _, p := range fontSearchPaths {
			if _, err := os.Stat(p); err == nil {
				resolvedPath = p
				break
			}
		}
	}
	if resolvedPath == "" {
		return nil, fmt.Errorf("no font found, specify a TTF/TTC path")
	}

	fontData, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("read font %s: %w", resolvedPath, err)
	}

	// Try single-font parse first (TTF), then collection parse (TTC).
	f, parseErr := opentype.Parse(fontData)
	if parseErr != nil {
		col, colErr := opentype.ParseCollection(fontData)
		if colErr != nil {
			return nil, fmt.Errorf("parse font: single=%v, collection=%v", parseErr, colErr)
		}
		if col.NumFonts() == 0 {
			return nil, fmt.Errorf("font collection is empty")
		}
		f, err = col.Font(0)
		if err != nil {
			return nil, fmt.Errorf("get font from collection: %w", err)
		}
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("create face: %w", err)
	}

	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()

	return &TextRenderer{
		gl:     glState,
		face:   face,
		ascent: ascent,
		cache:  make(map[rune]*glyphEntry),
		size:   size,
	}, nil
}

// getGlyph returns the cached glyph for a rune, rasterizing it on cache miss.
func (tr *TextRenderer) getGlyph(ch rune) *glyphEntry {
	tr.cacheMu.RLock()
	if g, ok := tr.cache[ch]; ok {
		tr.cacheMu.RUnlock()
		return g
	}
	tr.cacheMu.RUnlock()

	// Rasterize the glyph.
	advance, ok := tr.face.GlyphAdvance(ch)
	if !ok {
		// Glyph not in font — return a space-width entry with no texture.
		spaceAdv, _ := tr.face.GlyphAdvance(' ')
		return &glyphEntry{advance: spaceAdv.Ceil()}
	}

	bounds, _, ok := tr.face.GlyphBounds(ch)
	if !ok {
		spaceAdv, _ := tr.face.GlyphAdvance(' ')
		return &glyphEntry{advance: spaceAdv.Ceil()}
	}

	minX := bounds.Min.X.Floor()
	maxX := bounds.Max.X.Ceil()
	minY := bounds.Min.Y.Floor()
	maxY := bounds.Max.Y.Ceil()

	gw := maxX - minX
	gh := maxY - minY

	if gw <= 0 || gh <= 0 {
		return &glyphEntry{advance: advance.Ceil()}
	}

	// Create an RGBA image and draw the glyph.
	img := image.NewRGBA(image.Rect(0, 0, gw, gh))
	d := &font.Drawer{
		Dst: img,
		Src: image.White,
		Face: tr.face,
		Dot: fixed.P(-minX, -minY),
	}
	d.DrawString(string(ch))

	// Upload to GL.
	tex := tr.gl.UploadTexture(img)

	g := &glyphEntry{
		tex:      tex,
		w:        gw,
		h:        gh,
		advance:  advance.Ceil(),
		bearingX: minX,
		bearingY: -minY, // distance from top of image to baseline
	}

	tr.cacheMu.Lock()
	tr.cache[ch] = g
	tr.cacheMu.Unlock()

	return g
}

// DrawText renders text at (x, y) with the given RGBA color.
// (x, y) is the top-left of the text baseline area.
func (tr *TextRenderer) DrawText(text string, x, y float32, r, g, b, a float32, proj [16]float32) {
	cursorX := x
	for _, ch := range text {
		glyph := tr.getGlyph(ch)
		if glyph.tex != 0 {
			// Position: cursorX + bearingX, y + ascent - bearingY
			dx := cursorX + float32(glyph.bearingX)
			dy := y + float32(tr.ascent-glyph.bearingY)
			tr.gl.DrawQuadTint(glyph.tex, dx, dy, float32(glyph.w), float32(glyph.h), r, g, b, a, proj)
		}
		cursorX += float32(glyph.advance)
	}
}

// MeasureText returns the pixel width of the text.
func (tr *TextRenderer) MeasureText(text string) int {
	width := 0
	for _, ch := range text {
		glyph := tr.getGlyph(ch)
		width += glyph.advance
	}
	return width
}

// MeasureChar returns the pixel width of a single rune.
func (tr *TextRenderer) MeasureChar(ch rune) int {
	return tr.getGlyph(ch).advance
}

// Ascent returns the ascent in pixels.
func (tr *TextRenderer) Ascent() int {
	return tr.ascent
}

// LineHeight returns the full line height (ascent + descent).
func (tr *TextRenderer) LineHeight() int {
	metrics := tr.face.Metrics()
	return (metrics.Ascent + metrics.Descent).Ceil()
}

// Destroy frees all cached GL textures.
func (tr *TextRenderer) Destroy() {
	tr.cacheMu.Lock()
	for _, g := range tr.cache {
		if g.tex != 0 {
			gl.DeleteTextures(1, &g.tex)
		}
	}
	tr.cache = make(map[rune]*glyphEntry)
	tr.cacheMu.Unlock()
}


