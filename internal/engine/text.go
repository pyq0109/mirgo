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

// glyphEntry 是缓存的字形纹理。
type glyphEntry struct {
	tex     uint32 // GL 纹理 ID
	w, h    int    // 字形像素尺寸
	advance int    // 水平步进（像素）
	bearingX int   // 左侧 bearing
	bearingY int   // 顶部 bearing（从基线到字形顶部）
}

// TextRenderer 使用 TTF 字体渲染文本，并缓存字形纹理。
type TextRenderer struct {
	gl       *GLState
	face     font.Face
	ascent   int // 从基线到行顶的像素数
	cache    map[rune]*glyphEntry
	cacheMu  sync.RWMutex
	size     float64
	fontData []byte // 保存以便 WithSize 复用
}

// fontSearchPaths 列出常见系统的中文字体路径。
var fontSearchPaths = []string{
	// Windows
	`C:\Windows\Fonts\msyh.ttc`,  // 微软雅黑
	`C:\Windows\Fonts\msyhbd.ttc`, // 微软雅黑 Bold
	`C:\Windows\Fonts\simsun.ttc`, // 宋体
	`C:\Windows\Fonts\simhei.ttf`, // 黑体
	`C:\Windows\Fonts\arial.ttf`,  // Arial（英文兜底）
	// Linux
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	// macOS
	"/System/Library/Fonts/PingFang.ttc",
	"/System/Library/Fonts/STHeiti Light.ttc",
}

// NewTextRenderer 创建一个 TextRenderer。若 fontPath 为空，则尝试常见的 Windows 字体。
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

	// 先尝试单字体解析（TTF），再尝试字体集合解析（TTC）。
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
		gl:       glState,
		face:     face,
		ascent:   ascent,
		cache:    make(map[rune]*glyphEntry),
		size:     size,
		fontData: fontData,
	}, nil
}

// WithSize 用相同字体、不同字号创建一个新的 TextRenderer。
func (tr *TextRenderer) WithSize(size float64) (*TextRenderer, error) {
	f, parseErr := opentype.Parse(tr.fontData)
	if parseErr != nil {
		col, colErr := opentype.ParseCollection(tr.fontData)
		if colErr != nil {
			return nil, fmt.Errorf("parse font: single=%v, collection=%v", parseErr, colErr)
		}
		f, colErr = col.Font(0)
		if colErr != nil {
			return nil, fmt.Errorf("get font from collection: %w", colErr)
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

	return &TextRenderer{
		gl:       tr.gl,
		face:     face,
		ascent:   face.Metrics().Ascent.Ceil(),
		cache:    make(map[rune]*glyphEntry),
		size:     size,
		fontData: tr.fontData,
	}, nil
}

// getGlyph 返回某个 rune 缓存的字形，缓存未命中时进行光栅化。
func (tr *TextRenderer) getGlyph(ch rune) *glyphEntry {
	tr.cacheMu.RLock()
	if g, ok := tr.cache[ch]; ok {
		tr.cacheMu.RUnlock()
		return g
	}
	tr.cacheMu.RUnlock()

	// 光栅化字形。
	advance, ok := tr.face.GlyphAdvance(ch)
	if !ok {
		// 字体中无此字形——返回一个空格宽度、无纹理的条目。
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

	// 创建一张 RGBA 图像并绘制字形。
	img := image.NewRGBA(image.Rect(0, 0, gw, gh))
	d := &font.Drawer{
		Dst: img,
		Src: image.White,
		Face: tr.face,
		Dot: fixed.P(-minX, -minY),
	}
	d.DrawString(string(ch))

	// 上传到 GL。
	tex := tr.gl.UploadTexture(img)

	g := &glyphEntry{
		tex:      tex,
		w:        gw,
		h:        gh,
		advance:  advance.Ceil(),
		bearingX: minX,
		bearingY: -minY, // 从图像顶部到基线的距离
	}

	tr.cacheMu.Lock()
	tr.cache[ch] = g
	tr.cacheMu.Unlock()

	return g
}

// DrawText 用给定的 RGBA 颜色在 (x, y) 处渲染文本。
// (x, y) 是文本基线区域的左上角。
func (tr *TextRenderer) DrawText(text string, x, y float32, r, g, b, a float32, proj [16]float32) {
	cursorX := x
	for _, ch := range text {
		glyph := tr.getGlyph(ch)
		if glyph.tex != 0 {
			// 位置：cursorX + bearingX，y + ascent - bearingY
			dx := cursorX + float32(glyph.bearingX)
			dy := y + float32(tr.ascent-glyph.bearingY)
			tr.gl.DrawQuadTint(glyph.tex, dx, dy, float32(glyph.w), float32(glyph.h), r, g, b, a, proj)
		}
		cursorX += float32(glyph.advance)
	}
}

// DrawTextOutline 在主色后面渲染 1px 描边（颜色为 or, og, ob, oa）的文本。
// 对应 Delphi 用于 NPC 对话和提示的 BoldTextOut。
func (tr *TextRenderer) DrawTextOutline(text string, x, y float32, r, g, b, a float32, or, og, ob, oa float32, proj [16]float32) {
	tr.DrawText(text, x-1, y, or, og, ob, oa, proj)
	tr.DrawText(text, x+1, y, or, og, ob, oa, proj)
	tr.DrawText(text, x, y-1, or, og, ob, oa, proj)
	tr.DrawText(text, x, y+1, or, og, ob, oa, proj)
	tr.DrawText(text, x, y, r, g, b, a, proj)
}

// DrawTextBold 通过水平偏移 1px 绘制两次来渲染伪粗体文本。
// 近似 Delphi 的 fsBold 样式。
func (tr *TextRenderer) DrawTextBold(text string, x, y float32, r, g, b, a float32, proj [16]float32) {
	tr.DrawText(text, x, y, r, g, b, a, proj)
	tr.DrawText(text, x+1, y, r, g, b, a, proj)
}

// DrawTextBoldOutline 渲染带 1px 描边的伪粗体文本。
// 对应 Delphi 中带 fsBold 样式的 BoldTextOut（FState.pas:2274-2279）。
func (tr *TextRenderer) DrawTextBoldOutline(text string, x, y float32, r, g, b, a float32, or, og, ob, oa float32, proj [16]float32) {
	tr.DrawText(text, x-1, y, or, og, ob, oa, proj)
	tr.DrawText(text, x+1, y, or, og, ob, oa, proj)
	tr.DrawText(text, x+2, y, or, og, ob, oa, proj)
	tr.DrawText(text, x, y-1, or, og, ob, oa, proj)
	tr.DrawText(text, x+1, y-1, or, og, ob, oa, proj)
	tr.DrawText(text, x, y+1, or, og, ob, oa, proj)
	tr.DrawText(text, x+1, y+1, or, og, ob, oa, proj)
	tr.DrawText(text, x, y, r, g, b, a, proj)
	tr.DrawText(text, x+1, y, r, g, b, a, proj)
}

// MeasureText 返回文本的像素宽度。
func (tr *TextRenderer) MeasureText(text string) int {
	width := 0
	for _, ch := range text {
		glyph := tr.getGlyph(ch)
		width += glyph.advance
	}
	return width
}

// MeasureChar 返回单个 rune 的像素宽度。
func (tr *TextRenderer) MeasureChar(ch rune) int {
	return tr.getGlyph(ch).advance
}

// Ascent 返回 ascent（像素）。
func (tr *TextRenderer) Ascent() int {
	return tr.ascent
}

// LineHeight 返回完整行高（ascent + descent）。
func (tr *TextRenderer) LineHeight() int {
	metrics := tr.face.Metrics()
	return (metrics.Ascent + metrics.Descent).Ceil()
}

// Destroy 释放所有缓存的 GL 纹理。
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


