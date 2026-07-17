package engine

import "math"

const (
	TileWidth  = 48
	TileHeight = 32
)

// Camera2D provides pan/zoom with top-left origin (Y-down).
type Camera2D struct {
	X, Y   float64 // world position (top-left corner of viewport)
	Zoom   float64
	ViewW  int // viewport pixels
	ViewH  int
}

// NewCamera creates a camera with default settings.
func NewCamera(viewW, viewH int) *Camera2D {
	return &Camera2D{
		Zoom:  1.0,
		ViewW: viewW,
		ViewH: viewH,
	}
}

// ScreenToWorld converts screen pixel to world coordinate.
func (c *Camera2D) ScreenToWorld(sx, sy float64) (wx, wy float64) {
	return c.X + sx/c.Zoom, c.Y + sy/c.Zoom
}

// WorldToTile converts world coordinate to tile index.
func (c *Camera2D) WorldToTile(wx, wy float64) (tx, ty int) {
	tx = int(math.Floor(wx / TileWidth))
	ty = int(math.Floor(wy / TileHeight))
	return
}

// TileToWorld converts tile index to world coordinate (top-left of tile).
func (c *Camera2D) TileToWorld(tx, ty int) (wx, wy float64) {
	return float64(tx * TileWidth), float64(ty * TileHeight)
}

// ViewportTiles returns the visible tile range [startX, endX) x [startY, endY).
func (c *Camera2D) ViewportTiles(marginX, marginY int) (startX, startY, endX, endY int) {
	wx0, wy0 := c.ScreenToWorld(0, 0)
	wx1, wy1 := c.ScreenToWorld(float64(c.ViewW), float64(c.ViewH))

	sx, sy := c.WorldToTile(wx0, wy0)
	ex, ey := c.WorldToTile(wx1, wy1)

	startX = sx - marginX
	startY = sy - marginY
	endX = ex + marginX + 1
	endY = ey + marginY + 1
	return
}

// Pan moves the camera by (dx, dy) screen pixels.
func (c *Camera2D) Pan(dx, dy float64) {
	c.X -= dx / c.Zoom
	c.Y -= dy / c.Zoom
}

// ZoomAt changes zoom centered on screen position (sx, sy).
func (c *Camera2D) ZoomAt(factor float64, sx, sy float64) {
	wx, wy := c.ScreenToWorld(sx, sy)
	c.Zoom *= factor
	c.Zoom = math.Max(0.1, math.Min(10.0, c.Zoom))
	// Keep (wx, wy) at same screen position
	c.X = wx - sx/c.Zoom
	c.Y = wy - sy/c.Zoom
}

// SetViewport updates the viewport dimensions.
func (c *Camera2D) SetViewport(w, h int) {
	c.ViewW = w
	c.ViewH = h
}

// CenterOn centers the camera on a world position.
func (c *Camera2D) CenterOn(wx, wy float64) {
	c.X = wx - float64(c.ViewW)/(2*c.Zoom)
	c.Y = wy - float64(c.ViewH)/(2*c.Zoom)
}

// ClampToBounds keeps the viewport within map bounds (with overscroll margin).
func (c *Camera2D) ClampToBounds(mapW, mapH int) {
	worldW := float64(mapW) * TileWidth
	worldH := float64(mapH) * TileHeight
	viewW := float64(c.ViewW) / c.Zoom
	viewH := float64(c.ViewH) / c.Zoom
	marginX := viewW * 0.5
	marginY := viewH * 0.5

	if c.X < -marginX {
		c.X = -marginX
	}
	if c.Y < -marginY {
		c.Y = -marginY
	}
	if c.X+viewW > worldW+marginX {
		c.X = worldW + marginX - viewW
	}
	if c.Y+viewH > worldH+marginY {
		c.Y = worldH + marginY - viewH
	}
}
