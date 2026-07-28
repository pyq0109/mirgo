package engine

import (
	"image"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// GLState 保存用于渲染的 OpenGL 资源。
type GLState struct {
	TextureShader *TextureShader
	ColorShader   *ColorShader
	VAO           uint32
	VBO           uint32
	WhiteTex      uint32

	// 当前视口（帧缓冲像素，由 SetViewport 更新）。
	ViewX, ViewY, ViewW, ViewH int32

	scissorStack [][4]int32
}

// NewGLState 初始化 OpenGL 资源。
func NewGLState() (*GLState, error) {
	texShader, err := NewTextureShader()
	if err != nil {
		return nil, err
	}
	colorShader, err := NewColorShader()
	if err != nil {
		return nil, err
	}

	// 单位四边形 VBO：每个顶点 pos(2) + uv(2)，共 6 个顶点
	vertices := []float32{
		0, 0, 0, 0,
		1, 0, 1, 0,
		1, 1, 1, 1,
		0, 0, 0, 0,
		1, 1, 1, 1,
		0, 1, 0, 1,
	}

	var vao, vbo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	gl.BindVertexArray(vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, unsafe.Pointer(&vertices[0]), gl.STATIC_DRAW)

	// a_pos
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 4*4, 0)
	// a_uv
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, 4*4, 2*4)

	gl.BindVertexArray(0)

	// 白色 1x1 纹理
	var whiteTex uint32
	gl.GenTextures(1, &whiteTex)
	gl.BindTexture(gl.TEXTURE_2D, whiteTex)
	white := []byte{255, 255, 255, 255}
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&white[0]))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)

	return &GLState{
		TextureShader: texShader,
		ColorShader:   colorShader,
		VAO:           vao,
		VBO:           vbo,
		WhiteTex:      whiteTex,
	}, nil
}

// UploadTexture 将 *image.RGBA 上传为 OpenGL 纹理。
func (s *GLState) UploadTexture(img *image.RGBA) uint32 {
	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA,
		int32(img.Bounds().Dx()), int32(img.Bounds().Dy()),
		0, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&img.Pix[0]))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	return tex
}

// DeleteTexture 删除一个 OpenGL 纹理。
func (s *GLState) DeleteTexture(id uint32) {
	if id != 0 {
		gl.DeleteTextures(1, &id)
	}
}

// SetViewport 设置 GL 视口并记录下来（用于裁剪计算）。
func (s *GLState) SetViewport(x, y, w, h int32) {
	s.ViewX, s.ViewY, s.ViewW, s.ViewH = x, y, w, h
	gl.Viewport(x, y, w, h)
}

// PushScissor 启用一个裁剪矩形。坐标相对于当前视口自上而下。
// 在帧缓冲被缩放（HiDPI）时仍按逻辑 800x600 绘制 UI 的调用方，
// 必须自行把逻辑像素换算为视口像素：pix = logical * ViewW / 800。
func (s *GLState) PushScissor(x, y, w, h int32) {
	s.scissorStack = append(s.scissorStack, [4]int32{x, y, w, h})
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(s.ViewX+x, s.ViewY+s.ViewH-y-h, w, h)
}

// PopScissor 恢复上一个裁剪矩形，或禁用裁剪。
func (s *GLState) PopScissor() {
	if len(s.scissorStack) == 0 {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	s.scissorStack = s.scissorStack[:len(s.scissorStack)-1]
	if len(s.scissorStack) == 0 {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	r := s.scissorStack[len(s.scissorStack)-1]
	gl.Scissor(s.ViewX+r[0], s.ViewY+s.ViewH-r[1]-r[3], r[2], r[3])
}

func (s *GLState) bindTexture(texID uint32) {
	gl.ActiveTexture(gl.TEXTURE0)
	if texID != 0 {
		gl.BindTexture(gl.TEXTURE_2D, texID)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, s.WhiteTex)
	}
}

func (s *GLState) setModel(x, y, w, h float32, proj [16]float32) {
	gl.UseProgram(s.TextureShader.ID)
	gl.BindVertexArray(s.VAO)
	gl.UniformMatrix4fv(s.TextureShader.ProjLoc, 1, false, &proj[0])
	model := [16]float32{
		w, 0, 0, 0,
		0, h, 0, 0,
		0, 0, 1, 0,
		x, y, 0, 1,
	}
	gl.UniformMatrix4fv(s.TextureShader.ModelLoc, 1, false, &model[0])
}

// DrawQuad 在 (x, y) 处绘制尺寸为 (w, h) 的带纹理四边形。
func (s *GLState) DrawQuad(texID uint32, x, y, w, h float32, proj [16]float32) {
	s.setModel(x, y, w, h, proj)
	gl.Uniform2f(s.TextureShader.UVScaleLoc, 1, 1)
	gl.Uniform2f(s.TextureShader.UVOffLoc, 0, 0)
	gl.Uniform1i(s.TextureShader.UseTexLoc, 1)
	gl.Uniform4f(s.TextureShader.ColorLoc, 1, 1, 1, 1)
	s.bindTexture(texID)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadTint 绘制带颜色着色的带纹理四边形（frag_color = texture * color）。
func (s *GLState) DrawQuadTint(texID uint32, x, y, w, h float32, r, g, b, a float32, proj [16]float32) {
	s.setModel(x, y, w, h, proj)
	gl.Uniform2f(s.TextureShader.UVScaleLoc, 1, 1)
	gl.Uniform2f(s.TextureShader.UVOffLoc, 0, 0)
	gl.Uniform1i(s.TextureShader.UseTexLoc, 1)
	gl.Uniform4f(s.TextureShader.ColorLoc, r, g, b, a)
	s.bindTexture(texID)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadSub 将一张 texW×texH 纹理中的子矩形（sx, sy, sw, sh，单位为纹理像素）
// 绘制到目标四边形 (x, y, w, h)，并带着色。
// 用于裁剪的进度条（HP/MP 球、经验/负重）和部分精灵。
func (s *GLState) DrawQuadSub(texID uint32, texW, texH float32, sx, sy, sw, sh, x, y, w, h float32, r, g, b, a float32, proj [16]float32) {
	if texW <= 0 || texH <= 0 {
		return
	}
	s.setModel(x, y, w, h, proj)
	gl.Uniform2f(s.TextureShader.UVScaleLoc, sw/texW, sh/texH)
	gl.Uniform2f(s.TextureShader.UVOffLoc, sx/texW, sy/texH)
	gl.Uniform1i(s.TextureShader.UseTexLoc, 1)
	gl.Uniform4f(s.TextureShader.ColorLoc, r, g, b, a)
	s.bindTexture(texID)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadColor 绘制一个纯色四边形（无纹理）。
func (s *GLState) DrawQuadColor(x, y, w, h float32, r, g, b, a float32, proj [16]float32) {
	s.setModel(x, y, w, h, proj)
	gl.Uniform2f(s.TextureShader.UVScaleLoc, 1, 1)
	gl.Uniform2f(s.TextureShader.UVOffLoc, 0, 0)
	gl.Uniform1i(s.TextureShader.UseTexLoc, 0)
	gl.Uniform4f(s.TextureShader.ColorLoc, r, g, b, a)
	s.bindTexture(s.WhiteTex)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadAdditive 以叠加混合（src+dst）绘制带纹理四边形，
// 对应 Delphi 用于选中发光效果的 DrawBlend(...,1)。
func (s *GLState) DrawQuadAdditive(texID uint32, x, y, w, h float32, proj [16]float32) {
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	s.setModel(x, y, w, h, proj)
	gl.Uniform2f(s.TextureShader.UVScaleLoc, 1, 1)
	gl.Uniform2f(s.TextureShader.UVOffLoc, 0, 0)
	gl.Uniform1i(s.TextureShader.UseTexLoc, 1)
	gl.Uniform4f(s.TextureShader.ColorLoc, 1, 1, 1, 1)
	s.bindTexture(texID)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
}

// OrthoProj 根据宽高计算正交投影矩阵（Y 轴向下）。
func OrthoProj(width, height float32) [16]float32 {
	return [16]float32{
		2 / width, 0, 0, 0,
		0, -2 / height, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	}
}

// OrthoProj4 根据 left/right/bottom/top 计算正交投影矩阵。
// 与 mapviewer 的 OrthoProj 函数一致。
func OrthoProj4(left, right, bottom, top float32) [16]float32 {
	return [16]float32{
		2 / (right - left), 0, 0, 0,
		0, 2 / (top - bottom), 0, 0,
		0, 0, -1, 0,
		-(right + left) / (right - left), -(top + bottom) / (top - bottom), 0, 1,
	}
}

// Destroy 释放 GLState 持有的所有 GL 资源。
func (s *GLState) Destroy() {
	gl.DeleteTextures(1, &s.WhiteTex)
	gl.DeleteBuffers(1, &s.VBO)
	gl.DeleteVertexArrays(1, &s.VAO)
	gl.DeleteProgram(s.TextureShader.ID)
	gl.DeleteProgram(s.ColorShader.ID)
}
