package main

import (
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

const minimapSize = 200

// Minimap 渲染碰撞概览小地图。
type Minimap struct {
	gl           *engine.GLState
	mapData      *mapformat.MapData
	fbo          uint32
	fboTex       uint32
	collisionTex uint32
}

// NewMinimap 创建新的小地图。
func NewMinimap(glState *engine.GLState, mapData *mapformat.MapData) *Minimap {
	mm := &Minimap{
		gl:      glState,
		mapData: mapData,
	}
	mm.createCollisionTexture()
	mm.createFBO()
	return mm
}

func (mm *Minimap) createCollisionTexture() {
	data := make([]byte, minimapSize*minimapSize*4)

	for y := 0; y < minimapSize; y++ {
		for x := 0; x < minimapSize; x++ {
			tx := int(float64(x) / minimapSize * float64(mm.mapData.Width))
			ty := int(float64(y) / minimapSize * float64(mm.mapData.Height))

			info := mm.mapData.InfoAt(tx, ty)
			idx := (y*minimapSize + x) * 4

			if info != nil && !info.Collision {
				data[idx] = 34
				data[idx+1] = 85
				data[idx+2] = 34
				data[idx+3] = 255
			} else {
				data[idx] = 60
				data[idx+1] = 60
				data[idx+2] = 60
				data[idx+3] = 255
			}
		}
	}

	gl.GenTextures(1, &mm.collisionTex)
	gl.BindTexture(gl.TEXTURE_2D, mm.collisionTex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		minimapSize, minimapSize, 0,
		gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
}

func (mm *Minimap) createFBO() {
	gl.GenFramebuffers(1, &mm.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, mm.fbo)

	gl.GenTextures(1, &mm.fboTex)
	gl.BindTexture(gl.TEXTURE_2D, mm.fboTex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		minimapSize, minimapSize, 0,
		gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)

	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, mm.fboTex, 0)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// Render 根据当前相机位置更新小地图 FBO。
func (mm *Minimap) Render(cam *engine.Camera2D, mapW, mapH int) {
	var oldFBO int32
	gl.GetIntegerv(gl.FRAMEBUFFER_BINDING, &oldFBO)
	var oldViewport [4]int32
	gl.GetIntegerv(gl.VIEWPORT, &oldViewport[0])

	gl.BindFramebuffer(gl.FRAMEBUFFER, mm.fbo)
	gl.Viewport(0, 0, minimapSize, minimapSize)

	proj := engine.OrthoProj(minimapSize, minimapSize)
	mm.gl.DrawQuad(mm.collisionTex, 0, 0, minimapSize, minimapSize, proj)

	worldW := float32(mapW) * engine.TileWidth
	worldH := float32(mapH) * engine.TileHeight

	x0 := float32(cam.X) / worldW * minimapSize
	y0 := float32(cam.Y) / worldH * minimapSize
	viewW := float32(cam.ViewW) / float32(cam.Zoom) / worldW * minimapSize
	viewH := float32(cam.ViewH) / float32(cam.Zoom) / worldH * minimapSize

	mm.gl.DrawQuadColor(x0, y0, viewW, 1, 1, 1, 1, 1, proj)
	mm.gl.DrawQuadColor(x0, y0+viewH, viewW, 1, 1, 1, 1, 1, proj)
	mm.gl.DrawQuadColor(x0, y0, 1, viewH, 1, 1, 1, 1, proj)
	mm.gl.DrawQuadColor(x0+viewW, y0, 1, viewH, 1, 1, 1, 1, proj)

	gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(oldFBO))
	gl.Viewport(oldViewport[0], oldViewport[1], oldViewport[2], oldViewport[3])
}

// DrawActorDots 在小地图（屏幕右上 120×120 区域）上叠加周边角色雷达：
// 以自身为中心，±50 格范围内的角色按类型着色绘制为 2×2 点。
// 自身白色、玩家绿色、怪物红色、NPC 黄色。proj 为 UI 正交投影。
func (mm *Minimap) DrawActorDots(gl *engine.GLState, actors []*Actor, selfX, selfY int, proj [16]float32) {
	const radarRange = 50
	const radarSize = 120.0
	scale := float32(radarSize / (radarRange * 2)) // 120/100 = 1.2
	originX := float32(ScreenWidth) - radarSize
	for _, a := range actors {
		if a == nil {
			continue
		}
		dx := a.CurrX - selfX
		dy := a.CurrY - selfY
		if dx < -radarRange || dx > radarRange || dy < -radarRange || dy > radarRange {
			continue
		}
		dotX := originX + float32(dx+radarRange)*scale
		dotY := float32(dy+radarRange) * scale
		var r, g, b float32
		switch {
		case a.IsSelf:
			r, g, b = 1, 1, 1
		case a.Type == ActorMonster:
			r, g, b = 1, 0, 0
		case a.Type == ActorNPC:
			r, g, b = 1, 1, 0
		default:
			r, g, b = 0, 1, 0
		}
		gl.DrawQuadColor(dotX, dotY, 2, 2, r, g, b, 1, proj)
	}
}

// GetTexture 返回 FBO 纹理。
func (mm *Minimap) GetTexture() uint32 {
	return mm.fboTex
}

// Destroy 释放所有资源。
func (mm *Minimap) Destroy() {
	gl.DeleteTextures(1, &mm.collisionTex)
	gl.DeleteTextures(1, &mm.fboTex)
	gl.DeleteFramebuffers(1, &mm.fbo)
}
