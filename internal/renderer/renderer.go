package renderer

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	cullMargin     = 3  // back/middle layer viewport margin
	frontCullMargin = 20 // front layer viewport margin (tall objects)
)

// Renderer composites map layers into an *image.RGBA.
type Renderer struct {
	Tiles   *wil.File // Tiles.wil (back layer)
	SmTiles *wil.File // SmTiles.wil (middle layer)
	Objects *wil.File // Objects.wil (front layer)

	collisionImg *image.RGBA
	animCounter  int
	dst          *image.RGBA // cached render buffer
	dstW, dstH   int
}

// New creates a Renderer.
func New(tiles, smTiles, objects *wil.File, mapW, mapH int) *Renderer {
	ci := image.NewRGBA(image.Rect(0, 0, TileWidth, TileHeight))
	for y := 0; y < TileHeight; y++ {
		for x := 0; x < TileWidth; x++ {
			ci.SetRGBA(x, y, color.RGBA{R: 255, A: 80})
		}
	}

	return &Renderer{
		Tiles:        tiles,
		SmTiles:      smTiles,
		Objects:      objects,
		collisionImg: ci,
	}
}

// Render draws the visible portion of the map.
func (r *Renderer) Render(m *mapformat.MapData, cam *Camera2D, showBack, showMid, showFront, showCollision, showGrid bool) *image.RGBA {
	// Reuse dst buffer if size matches
	if r.dst == nil || r.dstW != cam.ViewW || r.dstH != cam.ViewH {
		r.dst = image.NewRGBA(image.Rect(0, 0, cam.ViewW, cam.ViewH))
		r.dstW = cam.ViewW
		r.dstH = cam.ViewH
	}
	dst := r.dst
	// Clear to black
	for i := range dst.Pix {
		dst.Pix[i] = 0
	}

	// Back/middle layer cull range
	startX, startY, endX, endY := cam.ViewportTiles(cullMargin, cullMargin)
	startX = clamp(startX, 0, m.Width-1)
	startY = clamp(startY, 0, m.Height-1)
	endX = clamp(endX, 0, m.Width-1)
	endY = clamp(endY, 0, m.Height-1)

	// Front layer cull range (wider for tall objects)
	fStartX, fStartY, fEndX, fEndY := cam.ViewportTiles(frontCullMargin, frontCullMargin)
	fStartX = clamp(fStartX, 0, m.Width-1)
	fStartY = clamp(fStartY, 0, m.Height-1)
	fEndX = clamp(fEndX, 0, m.Width-1)
	fEndY = clamp(fEndY, 0, m.Height-1)

	// 1. Back layer: even x,y only
	if showBack && r.Tiles != nil {
		// Align to even boundaries
		bStartX := startX
		bStartY := startY
		bEndX := endX
		bEndY := endY
		if bStartX%2 == 1 {
			bStartX--
		}
		if bStartY%2 == 1 {
			bStartY--
		}
		if bEndX%2 == 1 {
			bEndX++
		}
		if bEndY%2 == 1 {
			bEndY++
		}
		bStartX = clamp(bStartX, 0, m.Width-1)
		bStartY = clamp(bStartY, 0, m.Height-1)
		bEndX = clamp(bEndX, 0, m.Width-1)
		bEndY = clamp(bEndY, 0, m.Height-1)

		for y := bStartY; y <= bEndY; y += 2 {
			for x := bStartX; x <= bEndX; x += 2 {
				cell := m.At(x, y)
				idx := int(cell.BkImg&0x7FFF) - 1
				if idx < 0 || idx >= len(r.Tiles.Images) {
					continue
				}
				img := r.Tiles.Images[idx]
				if img == nil || img.RGBA == nil {
					continue
				}
				sx, sy := cam.worldToScreen(float64(x*TileWidth), float64(y*TileHeight))
				dstRect := image.Rect(int(sx), int(sy), int(sx)+img.Width, int(sy)+img.Height)
				draw.Draw(dst, dstRect, img.RGBA, image.Point{}, draw.Over)
			}
		}
	}

	// 2. Middle layer
	if showMid && r.SmTiles != nil {
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				cell := m.At(x, y)
				idx := int(cell.MidImg&0x7FFF) - 1
				if idx < 0 || idx >= len(r.SmTiles.Images) {
					continue
				}
				img := r.SmTiles.Images[idx]
				if img == nil || img.RGBA == nil {
					continue
				}
				sx, sy := cam.worldToScreen(float64(x*TileWidth), float64(y*TileHeight))
				dstRect := image.Rect(int(sx), int(sy), int(sx)+img.Width, int(sy)+img.Height)
				draw.Draw(dst, dstRect, img.RGBA, image.Point{}, draw.Over)
			}
		}
	}

	// 3. Front layer (uses wider cull range)
	if showFront && r.Objects != nil {
		// Normal objects
		for y := fStartY; y <= fEndY; y++ {
			for x := fStartX; x <= fEndX; x++ {
				cell := m.At(x, y)
				r.drawFrontCell(dst, cell, x, y, cam, false)
			}
		}
		// Blend objects
		for y := fStartY; y <= fEndY; y++ {
			for x := fStartX; x <= fEndX; x++ {
				cell := m.At(x, y)
				if cell.AniFrame&0x80 != 0 {
					r.drawFrontCell(dst, cell, x, y, cam, true)
				}
			}
		}
		r.animCounter++
	}

	// 4. Collision overlay
	if showCollision {
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				if m.IsCollision(x, y) {
					sx, sy := cam.worldToScreen(float64(x*TileWidth), float64(y*TileHeight))
					dstRect := image.Rect(int(sx), int(sy), int(sx)+TileWidth, int(sy)+TileHeight)
					draw.Draw(dst, dstRect, r.collisionImg, image.Point{}, draw.Over)
				}
			}
		}
	}

	// 5. Grid
	if showGrid {
		gridColor := color.RGBA{R: 255, G: 255, B: 255, A: 40}
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				sx, sy := cam.worldToScreen(float64(x*TileWidth), float64(y*TileHeight))
				ix, iy := int(sx), int(sy)
				for px := ix; px < ix+TileWidth && px < cam.ViewW; px++ {
					if px >= 0 {
						setPixelSafe(dst, px, iy, gridColor)
					}
				}
				for py := iy; py < iy+TileHeight && py < cam.ViewH; py++ {
					if py >= 0 {
						setPixelSafe(dst, ix, py, gridColor)
					}
				}
			}
		}
	}

	return dst
}

func (r *Renderer) drawFrontCell(dst *image.RGBA, cell *mapformat.Cell, x, y int, cam *Camera2D, blendOnly bool) {
	idx := int(cell.FrImg&0x7FFF) - 1
	if idx < 0 || idx >= len(r.Objects.Images) {
		return
	}
	isBlend := cell.AniFrame&0x80 != 0
	if blendOnly != isBlend {
		return
	}

	// Animation frame
	ani := int(cell.AniFrame & 0x7F)
	if ani > 0 {
		tick := int(cell.AniTick)
		if tick < 1 {
			tick = 1
		}
		cycleLen := ani + ani*tick
		if cycleLen > 0 {
			frame := (r.animCounter % cycleLen) / (1 + tick)
			idx += frame
		}
	}

	if idx < 0 || idx >= len(r.Objects.Images) {
		return
	}

	img := r.Objects.Images[idx]
	if img == nil || img.RGBA == nil {
		return
	}

	var sx, sy float64
	if isBlend {
		// Blend: hotspot positioning (matches Delphi: DrawBlend(surface, n+ax-2, m+ay-68, ...))
		sx, sy = cam.worldToScreen(
			float64(x*TileWidth)+float64(img.HotX)-2,
			float64(y*TileHeight)+float64(img.HotY)-68,
		)
	} else {
		// Normal: bottom-aligned (draw_y = cell_y - height + kTileHeight)
		sx, sy = cam.worldToScreen(
			float64(x*TileWidth),
			float64(y*TileHeight)-float64(img.Height)+float64(TileHeight),
		)
	}

	dstRect := image.Rect(int(sx), int(sy), int(sx)+img.Width, int(sy)+img.Height)
	draw.Draw(dst, dstRect, img.RGBA, image.Point{}, draw.Over)
}

// worldToScreen converts world coords to screen coords.
func (c *Camera2D) worldToScreen(wx, wy float64) (sx, sy float64) {
	return (wx - c.X) * c.Zoom, (wy - c.Y) * c.Zoom
}

func setPixelSafe(img *image.RGBA, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return
	}
	off := y*img.Stride + x*4
	img.Pix[off+0] = c.R
	img.Pix[off+1] = c.G
	img.Pix[off+2] = c.B
	img.Pix[off+3] = c.A
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
