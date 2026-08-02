package main

import "math"

const (
	TileWidth  = 48
	TileHeight = 32
)

// Camera2D 提供平移/缩放功能，左上角为原点（Y 轴向下）。
type Camera2D struct {
	X, Y   float64 // 世界坐标（视口左上角）
	Zoom   float64
	ViewW  int // 视口像素尺寸
	ViewH  int
}

// NewCamera 创建默认设置的相机。
func NewCamera(viewW, viewH int) *Camera2D {
	return &Camera2D{
		Zoom:  1.0,
		ViewW: viewW,
		ViewH: viewH,
	}
}

// ScreenToWorld 将屏幕像素坐标转换为世界坐标。
func (c *Camera2D) ScreenToWorld(sx, sy float64) (wx, wy float64) {
	return c.X + sx/c.Zoom, c.Y + sy/c.Zoom
}

// WorldToTile 将世界坐标转换为格子索引。
func (c *Camera2D) WorldToTile(wx, wy float64) (tx, ty int) {
	tx = int(math.Floor(wx / TileWidth))
	ty = int(math.Floor(wy / TileHeight))
	return
}

// ViewportTiles 返回可见格子范围 [startX, endX) x [startY, endY)。
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

// Pan 按 (dx, dy) 屏幕像素移动相机。
func (c *Camera2D) Pan(dx, dy float64) {
	c.X -= dx / c.Zoom
	c.Y -= dy / c.Zoom
}

// ZoomAt 以屏幕位置 (sx, sy) 为中心缩放。
func (c *Camera2D) ZoomAt(factor float64, sx, sy float64) {
	wx, wy := c.ScreenToWorld(sx, sy)
	c.Zoom *= factor
	c.Zoom = math.Max(0.1, math.Min(10.0, c.Zoom))
	// 保持 (wx, wy) 在屏幕上的位置不变
	c.X = wx - sx/c.Zoom
	c.Y = wy - sy/c.Zoom
}

// SetViewport 更新视口尺寸。
func (c *Camera2D) SetViewport(w, h int) {
	c.ViewW = w
	c.ViewH = h
}

// CenterOnContent 在缩放 1.0 下将相机居中到地图内容。
func (c *Camera2D) CenterOnContent(contentW, contentH float64) {
	if contentW <= 0 || contentH <= 0 {
		return
	}
	c.Zoom = 1.0
	c.X = (contentW - float64(c.ViewW)/c.Zoom) / 2.0
	c.Y = (contentH - float64(c.ViewH)/c.Zoom) / 2.0
}

// FitToContent 缩放以适应视口并居中。
func (c *Camera2D) FitToContent(contentW, contentH float64) {
	if contentW <= 0 || contentH <= 0 {
		return
	}
	scaleX := float64(c.ViewW) / contentW
	scaleY := float64(c.ViewH) / contentH
	c.Zoom = math.Min(scaleX, scaleY)
	c.X = (contentW - float64(c.ViewW)/c.Zoom) / 2.0
	c.Y = (contentH - float64(c.ViewH)/c.Zoom) / 2.0
}

// ClampToBounds 将视口限制在地图边界内（允许 50% 过滚边距）。
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
