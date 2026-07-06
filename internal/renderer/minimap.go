package renderer

import (
	"image"
	"image/color"

	"github.com/pyq0109/mirgo/internal/mapformat"
)

const (
	minimapSize = 200
)

// GenerateMinimap creates a 200x200 collision texture.
func GenerateMinimap(m *mapformat.MapData) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, minimapSize, minimapSize))

	walkable := color.RGBA{R: 34, G: 85, B: 34, A: 255}
	blocked := color.RGBA{R: 60, G: 60, B: 60, A: 255}

	scaleX := float64(m.Width) / float64(minimapSize)
	scaleY := float64(m.Height) / float64(minimapSize)

	for my := 0; my < minimapSize; my++ {
		for mx := 0; mx < minimapSize; mx++ {
			tileX := int(float64(mx) * scaleX)
			tileY := int(float64(my) * scaleY)
			if tileX >= m.Width {
				tileX = m.Width - 1
			}
			if tileY >= m.Height {
				tileY = m.Height - 1
			}

			if m.IsCollision(tileX, tileY) {
				img.SetRGBA(mx, my, blocked)
			} else {
				img.SetRGBA(mx, my, walkable)
			}
		}
	}

	return img
}

// DrawMinimapViewport draws the viewport rectangle on the minimap.
func DrawMinimapViewport(minimap *image.RGBA, cam *Camera2D, mapW, mapH int) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 200}

	// Map camera viewport to minimap coordinates
	worldW := float64(mapW) * TileWidth
	worldH := float64(mapH) * TileHeight

	x0 := int(cam.X / worldW * minimapSize)
	y0 := int(cam.Y / worldH * minimapSize)
	viewW := float64(cam.ViewW) / cam.Zoom
	viewH := float64(cam.ViewH) / cam.Zoom
	x1 := int((cam.X + viewW) / worldW * minimapSize)
	y1 := int((cam.Y + viewH) / worldH * minimapSize)

	x0 = clamp(x0, 0, minimapSize-1)
	y0 = clamp(y0, 0, minimapSize-1)
	x1 = clamp(x1, 0, minimapSize-1)
	y1 = clamp(y1, 0, minimapSize-1)

	// Draw rectangle outline
	for x := x0; x <= x1; x++ {
		minimap.SetRGBA(x, y0, white)
		minimap.SetRGBA(x, y1, white)
	}
	for y := y0; y <= y1; y++ {
		minimap.SetRGBA(x0, y, white)
		minimap.SetRGBA(x1, y, white)
	}
}
