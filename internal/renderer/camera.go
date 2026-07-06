package renderer

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

// ViewportTiles returns the visible tile range [startX, endX) x [startY, endY).
func (c *Camera2D) ViewportTiles(marginX, marginY int) (startX, startY, endX, endY int) {
	wx0, wy0 := c.ScreenToWorld(0, 0)
	wx1, wy1 := c.ScreenToWorld(float64(c.ViewW), float64(c.ViewH))

	sx, sy := c.WorldToTile(wx0, wy0)
	ex, ey := c.WorldToTile(wx1, wy1)

	startX = sx - marginX
	startY = sy - marginY
	endX = ex + marginX
	endY = ey + marginY
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

// ClampToBounds keeps the viewport within map bounds.
func (c *Camera2D) ClampToBounds(mapW, mapH int) {
	worldW := float64(mapW) * TileWidth
	worldH := float64(mapH) * TileHeight
	viewW := float64(c.ViewW) / c.Zoom
	viewH := float64(c.ViewH) / c.Zoom

	if c.X < 0 {
		c.X = 0
	}
	if c.Y < 0 {
		c.Y = 0
	}
	if c.X+viewW > worldW {
		c.X = worldW - viewW
	}
	if c.Y+viewH > worldH {
		c.Y = worldH - viewH
	}
	if c.X < 0 {
		c.X = 0
	}
	if c.Y < 0 {
		c.Y = 0
	}
}
