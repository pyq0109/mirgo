package main

import (
	"image"
	"image/color"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"

	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

const minimapSize = 200

// Minimap 持有小地图 FBO 和碰撞纹理。
type Minimap struct {
	Texture uint32 // 碰撞纹理
	FBO     uint32
	FBOTex  uint32
}

// NewMinimap 创建带碰撞纹理的小地图。
func NewMinimap(m *mapformat.MapData) *Minimap {
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

	tex := UploadTexture(img)
	mlog.Logf(mlog.LevelInfo, "minimap", "已创建: tex=%d, 地图=%dx%d, 缩放=(%.4f, %.4f)", tex, m.Width, m.Height, scaleX, scaleY)

	// 采样碰撞图四角用于调试
	tl := img.RGBAAt(0, 0)
	tr := img.RGBAAt(minimapSize-1, 0)
	bl := img.RGBAAt(0, minimapSize-1)
	br := img.RGBAAt(minimapSize-1, minimapSize-1)
	mlog.Logf(mlog.LevelDebug, "minimap", "碰撞图四角: TL=(%d,%d,%d,%d) TR=(%d,%d,%d,%d) BL=(%d,%d,%d,%d) BR=(%d,%d,%d,%d)",
		tl.R, tl.G, tl.B, tl.A, tr.R, tr.G, tr.B, tr.A, bl.R, bl.G, bl.B, bl.A, br.R, br.G, br.B, br.A)

	// 创建小地图渲染用 FBO
	var fbo, fboTex uint32
	gl.GenFramebuffers(1, &fbo)
	gl.GenTextures(1, &fboTex)
	gl.BindTexture(gl.TEXTURE_2D, fboTex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, minimapSize, minimapSize, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, fboTex, 0)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	mlog.Logf(mlog.LevelInfo, "minimap", "FBO 已创建: fbo=%d, fboTex=%d", fbo, fboTex)

	return &Minimap{
		Texture: tex,
		FBO:     fbo,
		FBOTex:  fboTex,
	}
}

// Render 将小地图绘制到 FBO 并叠加视口矩形。
// 对应 C++ MapRenderer::RenderMinimap。
func (mm *Minimap) Render(cam *Camera2D, mapW, mapH int, glState *GLState) {
	mlog.Logf(mlog.LevelTrace, "minimap", "渲染: cam=(%.1f, %.1f) zoom=%.2f 地图=%dx%d", cam.X, cam.Y, cam.Zoom, mapW, mapH)

	// 保存 GL 状态（C++ 923-925 行）
	var lastFBO int32
	var lastVP [4]int32
	gl.GetIntegerv(gl.FRAMEBUFFER_BINDING, &lastFBO)
	gl.GetIntegerv(gl.VIEWPORT, &lastVP[0])

	gl.BindFramebuffer(gl.FRAMEBUFFER, mm.FBO)
	gl.Viewport(0, 0, minimapSize, minimapSize)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// 用 Y 轴朝上的正交投影绘制碰撞纹理（对应 C++ glm::ortho(0,1,0,1)）
	proj := OrthoProj(0, minimapSize, 0, minimapSize)
	glState.DrawQuad(0, 0, minimapSize, minimapSize, mm.Texture, false, proj)

	// 回读 FBO 四角像素验证方向
	var pixels [4]uint8
	// FBO 左下角（OpenGL 像素行 0, 列 0）
	gl.ReadPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboBL := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// FBO 左上角（OpenGL 像素行 199, 列 0）
	gl.ReadPixels(0, minimapSize-1, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboTL := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// FBO 右下角
	gl.ReadPixels(minimapSize-1, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboBR := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// FBO 右上角
	gl.ReadPixels(minimapSize-1, minimapSize-1, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboTR := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	mlog.Logf(mlog.LevelTrace, "minimap", "FBO 四角: BL=(%d,%d,%d,%d) TL=(%d,%d,%d,%d) BR=(%d,%d,%d,%d) TR=(%d,%d,%d,%d)",
		fboBL[0], fboBL[1], fboBL[2], fboBL[3], fboTL[0], fboTL[1], fboTL[2], fboTL[3],
		fboBR[0], fboBR[1], fboBR[2], fboBR[3], fboTR[0], fboTR[1], fboTR[2], fboTR[3])

	// 绘制视口矩形
	worldW := float32(mapW) * TileWidth
	worldH := float32(mapH) * TileHeight
	x0 := float32(cam.X) / worldW * minimapSize
	y0 := float32(cam.Y) / worldH * minimapSize
	viewW := float32(float64(cam.ViewW) / cam.Zoom)
	viewH := float32(float64(cam.ViewH) / cam.Zoom)
	x1 := (float32(cam.X) + viewW) / worldW * minimapSize
	y1 := (float32(cam.Y) + viewH) / worldH * minimapSize
	mlog.Logf(mlog.LevelTrace, "minimap", "视口原始值: (%.2f, %.2f) - (%.2f, %.2f)", x0, y0, x1, y1)

	// 裁剪到边界
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > minimapSize {
		x1 = minimapSize
	}
	if y1 > minimapSize {
		y1 = minimapSize
	}
	mlog.Logf(mlog.LevelTrace, "minimap", "视口裁剪后: (%.1f, %.1f) - (%.1f, %.1f)", x0, y0, x1, y1)

	// 用网格着色器 + 网格 VAO/VBO 绘制白色矩形边框
	gl.UseProgram(glState.GridShader.ID)
	gl.UniformMatrix4fv(glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(glState.GridShader.ColorLoc, 1, 1, 1, 0.8)
	gl.BindVertexArray(glState.GridVAO)

	lines := []float32{
		x0, y0, x1, y0, // 上
		x1, y0, x1, y1, // 右
		x1, y1, x0, y1, // 下
		x0, y1, x0, y0, // 左
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, glState.GridVBO)
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(lines)*4, unsafe.Pointer(&lines[0]))
	gl.DrawArrays(gl.LINES, 0, 4*2)

	gl.BindVertexArray(0)

	// 恢复 GL 状态（C++ 974-975 行）
	gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(lastFBO))
	gl.Viewport(lastVP[0], lastVP[1], lastVP[2], lastVP[3])
}

// Destroy 释放小地图持有的所有 GL 资源。
func (mm *Minimap) Destroy() {
	gl.DeleteTextures(1, &mm.Texture)
	gl.DeleteTextures(1, &mm.FBOTex)
	gl.DeleteFramebuffers(1, &mm.FBO)
}
