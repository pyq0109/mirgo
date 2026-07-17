package engine

import (
	"image"
	"os"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func TestFontLoading(t *testing.T) {
	// Try to find a font.
	var fontPath string
	for _, p := range fontSearchPaths {
		if _, err := os.Stat(p); err == nil {
			fontPath = p
			break
		}
	}
	if fontPath == "" {
		t.Skip("No system fonts found")
	}

	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Try single-font parse (TTF), then collection parse (TTC).
	f, err := opentype.Parse(fontData)
	if err != nil {
		col, colErr := opentype.ParseCollection(fontData)
		if colErr != nil {
			t.Fatalf("Parse: single=%v, collection=%v", err, colErr)
		}
		f, err = col.Font(0)
		if err != nil {
			t.Fatalf("Font(0): %v", err)
		}
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    16,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		t.Fatalf("NewFace: %v", err)
	}

	metrics := face.Metrics()
	t.Logf("Font: %s", fontPath)
	t.Logf("Ascent: %d, Descent: %d, Height: %d",
		metrics.Ascent.Ceil(), metrics.Descent.Ceil(),
		(metrics.Ascent + metrics.Descent).Ceil())

	// Test rasterizing a Chinese character.
	testCases := []struct {
		name string
		text string
	}{
		{"ASCII", "Hello"},
		{"Chinese", "账号密码"},
		{"Mixed", "test测试"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, ch := range tc.text {
				advance, ok := face.GlyphAdvance(ch)
				if !ok {
					t.Logf("GlyphAdvance(%c): not found", ch)
					continue
				}

				bounds, _, ok := face.GlyphBounds(ch)
				if !ok {
					t.Logf("GlyphBounds(%c): not found", ch)
					continue
				}

				minX := bounds.Min.X.Floor()
				maxX := bounds.Max.X.Ceil()
				minY := bounds.Min.Y.Floor()
				maxY := bounds.Max.Y.Ceil()
				gw := maxX - minX
				gh := maxY - minY

				if gw > 0 && gh > 0 {
					img := image.NewRGBA(image.Rect(0, 0, gw, gh))
					d := &font.Drawer{
						Dst: img,
						Src: image.White,
						Face: face,
						Dot: fixed.P(-minX, -minY),
					}
					d.DrawString(string(ch))

					// Count non-transparent pixels.
					pixels := 0
					for y := 0; y < gh; y++ {
						for x := 0; x < gw; x++ {
							if img.RGBAAt(x, y).A > 0 {
								pixels++
							}
						}
					}
					t.Logf("  '%c': advance=%d, size=%dx%d, pixels=%d",
						ch, advance.Ceil(), gw, gh, pixels)
				}
			}
		})
	}
}
