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

// TextScale 是全局文字光栅化缩放倍率。切换分辨率时调用 SetTextScale，
// 所有已创建的 TextRenderer 会立即重新缩放，之后新建的也自动应用。
var TextScale float64 = 1

var allRenderers []*TextRenderer

// SetTextScale 更新全局缩放倍率并立即应用到所有已注册的 TextRenderer。
func SetTextScale(s float64) {
	if s <= 0 {
		s = 1
	}
	TextScale = s
	for _, tr := range allRenderers {
		tr.SetScale(s)
	}
}

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
	gl         *GLState
	face       font.Face   // 逻辑尺寸 face（度量/布局）
	renderFace font.Face   // 高分辨率 face（光栅化），nil 时 == face
	ascent     int         // 从基线到行顶的逻辑像素数
	scale      float64     // 光栅化倍率（1.0 = 不缩放）
	cache      map[rune]*glyphEntry
	cacheMu    sync.RWMutex
	size       float64
	fontData   []byte // 保存以便 WithSize 复用
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
		size = 9
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

	tr := &TextRenderer{
		gl:       glState,
		face:     face,
		ascent:   ascent,
		scale:    1,
		cache:    make(map[rune]*glyphEntry),
		size:     size,
		fontData: fontData,
	}
	allRenderers = append(allRenderers, tr)
	if TextScale != 1 {
		tr.SetScale(TextScale)
	}
	return tr, nil
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

	child := &TextRenderer{
		gl:       tr.gl,
		face:     face,
		ascent:   face.Metrics().Ascent.Ceil(),
		scale:    1,
		cache:    make(map[rune]*glyphEntry),
		size:     size,
		fontData: tr.fontData,
	}
	allRenderers = append(allRenderers, child)
	if TextScale != 1 {
		child.SetScale(TextScale)
	}
	return child, nil
}

// SetScale 设置光栅化倍率。scale > 1 时以更高 DPI 光栅化字形使文字清晰，
// 但保持逻辑度量不变（布局/定位不受影响）。切换分辨率后调用。
func (tr *TextRenderer) SetScale(s float64) {
	if s <= 0 {
		s = 1
	}
	if s == tr.scale {
		return
	}
	tr.scale = s
	if s == 1 {
		tr.renderFace = nil
	} else {
		f, parseErr := opentype.Parse(tr.fontData)
		if parseErr != nil {
			col, _ := opentype.ParseCollection(tr.fontData)
			f, _ = col.Font(0)
		}
		if f != nil {
			face, err := opentype.NewFace(f, &opentype.FaceOptions{
				Size:    tr.size,
				DPI:     96 * s,
				Hinting: font.HintingFull,
			})
			if err == nil {
				tr.renderFace = face
			}
		}
	}
	tr.clearCache()
}

func (tr *TextRenderer) clearCache() {
	tr.cacheMu.Lock()
	for _, g := range tr.cache {
		if g.tex != 0 {
			gl.DeleteTextures(1, &g.tex)
		}
	}
	tr.cache = make(map[rune]*glyphEntry)
	tr.cacheMu.Unlock()
}

// getGlyph 返回某个 rune 缓存的字形，缓存未命中时进行光栅化。
func (tr *TextRenderer) getGlyph(ch rune) *glyphEntry {
	tr.cacheMu.RLock()
	if g, ok := tr.cache[ch]; ok {
		tr.cacheMu.RUnlock()
		return g
	}
	tr.cacheMu.RUnlock()

	// 逻辑度量（用于布局/quad 尺寸）。
	advance, ok := tr.face.GlyphAdvance(ch)
	if !ok {
		spaceAdv, _ := tr.face.GlyphAdvance(' ')
		return &glyphEntry{advance: spaceAdv.Ceil()}
	}

	bounds, _, ok := tr.face.GlyphBounds(ch)
	if !ok {
		spaceAdv, _ := tr.face.GlyphAdvance(' ')
		return &glyphEntry{advance: spaceAdv.Ceil()}
	}

	logMinX := bounds.Min.X.Floor()
	logMaxX := bounds.Max.X.Ceil()
	logMinY := bounds.Min.Y.Floor()
	logMaxY := bounds.Max.Y.Ceil()

	gw := logMaxX - logMinX
	gh := logMaxY - logMinY

	if gw <= 0 || gh <= 0 {
		return &glyphEntry{advance: advance.Ceil()}
	}

	// 光栅化：scale > 1 时用 renderFace 生成高分辨率位图。
	rf := tr.renderFace
	if rf == nil {
		rf = tr.face
	}
	var img *image.RGBA
	if rf == tr.face {
		img = image.NewRGBA(image.Rect(0, 0, gw, gh))
		d := &font.Drawer{Dst: img, Src: image.White, Face: rf, Dot: fixed.P(-logMinX, -logMinY)}
		d.DrawString(string(ch))
	} else {
		// 高分辨率光栅化。
		rBounds, _, _ := rf.GlyphBounds(ch)
		rMinX := rBounds.Min.X.Floor()
		rMaxX := rBounds.Max.X.Ceil()
		rMinY := rBounds.Min.Y.Floor()
		rMaxY := rBounds.Max.Y.Ceil()
		rw := rMaxX - rMinX
		rh := rMaxY - rMinY
		if rw <= 0 || rh <= 0 {
			return &glyphEntry{advance: advance.Ceil()}
		}
		img = image.NewRGBA(image.Rect(0, 0, rw, rh))
		d := &font.Drawer{Dst: img, Src: image.White, Face: rf, Dot: fixed.P(-rMinX, -rMinY)}
		d.DrawString(string(ch))
	}

	tex := tr.gl.UploadTexture(img)

	g := &glyphEntry{
		tex:      tex,
		w:        gw,
		h:        gh,
		advance:  advance.Ceil(),
		bearingX: logMinX,
		bearingY: -logMinY,
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

// Destroy 释放所有缓存的 GL 纹理并从全局注册表移除。
func (tr *TextRenderer) Destroy() {
	tr.cacheMu.Lock()
	for _, g := range tr.cache {
		if g.tex != 0 {
			gl.DeleteTextures(1, &g.tex)
		}
	}
	tr.cache = make(map[rune]*glyphEntry)
	tr.cacheMu.Unlock()
	for i, r := range allRenderers {
		if r == tr {
			allRenderers = append(allRenderers[:i], allRenderers[i+1:]...)
			break
		}
	}
}


