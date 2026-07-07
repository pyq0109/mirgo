package renderer

import (
	"image"
	"image/color"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"

	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

const minimapSize = 200

// Minimap holds the minimap FBO and collision texture.
type Minimap struct {
	Texture uint32 // collision texture
	FBO     uint32
	FBOTex  uint32
}

// NewMinimap creates a minimap with collision texture.
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
	mlog.Logf(mlog.LevelInfo, "minimap", "created: tex=%d, map=%dx%d, scale=(%.4f, %.4f)", tex, m.Width, m.Height, scaleX, scaleY)

	// Sample collision image corners for debugging.
	tl := img.RGBAAt(0, 0)
	tr := img.RGBAAt(minimapSize-1, 0)
	bl := img.RGBAAt(0, minimapSize-1)
	br := img.RGBAAt(minimapSize-1, minimapSize-1)
	mlog.Logf(mlog.LevelDebug, "minimap", "collision image corners: TL=(%d,%d,%d,%d) TR=(%d,%d,%d,%d) BL=(%d,%d,%d,%d) BR=(%d,%d,%d,%d)",
		tl.R, tl.G, tl.B, tl.A, tr.R, tr.G, tr.B, tr.A, bl.R, bl.G, bl.B, bl.A, br.R, br.G, br.B, br.A)

	// Create FBO for minimap rendering.
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
	mlog.Logf(mlog.LevelInfo, "minimap", "FBO created: fbo=%d, fboTex=%d", fbo, fboTex)

	return &Minimap{
		Texture: tex,
		FBO:     fbo,
		FBOTex:  fboTex,
	}
}

// Render draws the minimap to its FBO with viewport rectangle.
// Matches C++ MapRenderer::RenderMinimap.
func (mm *Minimap) Render(cam *Camera2D, mapW, mapH int, glState *GLState) {
	mlog.Logf(mlog.LevelTrace, "minimap", "render: cam=(%.1f, %.1f) zoom=%.2f map=%dx%d", cam.X, cam.Y, cam.Zoom, mapW, mapH)

	// Save GL state (C++ lines 923-925).
	var lastFBO int32
	var lastVP [4]int32
	gl.GetIntegerv(gl.FRAMEBUFFER_BINDING, &lastFBO)
	gl.GetIntegerv(gl.VIEWPORT, &lastVP[0])

	gl.BindFramebuffer(gl.FRAMEBUFFER, mm.FBO)
	gl.Viewport(0, 0, minimapSize, minimapSize)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// Draw collision texture using Y-up ortho (matching C++ glm::ortho(0,1,0,1)).
	proj := OrthoProj(0, minimapSize, 0, minimapSize)
	glState.DrawQuad(0, 0, minimapSize, minimapSize, mm.Texture, false, proj)

	// Read back FBO corners to verify orientation.
	var pixels [4]uint8
	// Bottom-left of FBO (OpenGL pixel row 0, col 0).
	gl.ReadPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboBL := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// Top-left of FBO (OpenGL pixel row 199, col 0).
	gl.ReadPixels(0, minimapSize-1, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboTL := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// Bottom-right of FBO.
	gl.ReadPixels(minimapSize-1, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboBR := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	// Top-right of FBO.
	gl.ReadPixels(minimapSize-1, minimapSize-1, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&pixels[0]))
	fboTR := [4]uint8{pixels[0], pixels[1], pixels[2], pixels[3]}
	mlog.Logf(mlog.LevelTrace, "minimap", "FBO corners: BL=(%d,%d,%d,%d) TL=(%d,%d,%d,%d) BR=(%d,%d,%d,%d) TR=(%d,%d,%d,%d)",
		fboBL[0], fboBL[1], fboBL[2], fboBL[3], fboTL[0], fboTL[1], fboTL[2], fboTL[3],
		fboBR[0], fboBR[1], fboBR[2], fboBR[3], fboTR[0], fboTR[1], fboTR[2], fboTR[3])

	// Draw viewport rectangle.
	worldW := float32(mapW) * TileWidth
	worldH := float32(mapH) * TileHeight
	x0 := float32(cam.X) / worldW * minimapSize
	y0 := float32(cam.Y) / worldH * minimapSize
	viewW := float32(float64(cam.ViewW) / cam.Zoom)
	viewH := float32(float64(cam.ViewH) / cam.Zoom)
	x1 := (float32(cam.X) + viewW) / worldW * minimapSize
	y1 := (float32(cam.Y) + viewH) / worldH * minimapSize
	mlog.Logf(mlog.LevelTrace, "minimap", "viewport raw: (%.2f, %.2f) - (%.2f, %.2f)", x0, y0, x1, y1)

	// Clamp.
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
	mlog.Logf(mlog.LevelTrace, "minimap", "viewport clamped: (%.1f, %.1f) - (%.1f, %.1f)", x0, y0, x1, y1)

	// Draw white rectangle outline using grid shader + grid VAO/VBO.
	gl.UseProgram(glState.GridShader.ID)
	gl.UniformMatrix4fv(glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(glState.GridShader.ColorLoc, 1, 1, 1, 0.8)
	gl.BindVertexArray(glState.GridVAO)

	lines := []float32{
		x0, y0, x1, y0, // top
		x1, y0, x1, y1, // right
		x1, y1, x0, y1, // bottom
		x0, y1, x0, y0, // left
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, glState.GridVBO)
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(lines)*4, unsafe.Pointer(&lines[0]))
	gl.DrawArrays(gl.LINES, 0, 4*2)

	gl.BindVertexArray(0)

	// Restore GL state (C++ lines 974-975).
	gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(lastFBO))
	gl.Viewport(lastVP[0], lastVP[1], lastVP[2], lastVP[3])
}

// Destroy frees all GL resources held by the minimap.
func (mm *Minimap) Destroy() {
	gl.DeleteTextures(1, &mm.Texture)
	gl.DeleteTextures(1, &mm.FBOTex)
	gl.DeleteFramebuffers(1, &mm.FBO)
}
