package renderer

import (
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// GLState 持有渲染所需的 OpenGL 资源。
type GLState struct {
	Shader   *ShaderProgram
	VAO      uint32
	VBO      uint32
	WhiteTex uint32
}

// NewGLState 初始化 OpenGL 资源。
func NewGLState() (*GLState, error) {
	shader, err := NewShaderProgram()
	if err != nil {
		return nil, err
	}

	// 单位四边形 VBO: 每顶点 pos(2) + uv(2)，共 6 个顶点
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
		Shader:   shader,
		VAO:      vao,
		VBO:      vbo,
		WhiteTex: whiteTex,
	}, nil
}

// DrawQuad 在 (x, y) 处绘制大小为 (w, h) 的纹理四边形。
func (s *GLState) DrawQuad(x, y, w, h float32, texture uint32, flipV bool, proj [16]float32) {
	gl.UseProgram(s.Shader.ID)
	gl.BindVertexArray(s.VAO)

	gl.UniformMatrix4fv(s.Shader.ProjLoc, 1, false, &proj[0])

	// 模型矩阵: translate(x,y) scale(w,h)
	model := [16]float32{
		w, 0, 0, 0,
		0, h, 0, 0,
		0, 0, 1, 0,
		x, y, 0, 1,
	}
	gl.UniformMatrix4fv(s.Shader.ModelLoc, 1, false, &model[0])

	gl.Uniform1i(s.Shader.UseTexLoc, 1)
	gl.Uniform4f(s.Shader.ColorLoc, 1, 1, 1, 1)

	flipVInt := int32(0)
	if flipV {
		flipVInt = 1
	}
	gl.Uniform1i(s.Shader.FlipVLoc, flipVInt)

	gl.ActiveTexture(gl.TEXTURE0)
	if texture != 0 {
		gl.BindTexture(gl.TEXTURE_2D, texture)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, s.WhiteTex)
	}

	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadColor 绘制纯色四边形（无纹理）。
func (s *GLState) DrawQuadColor(x, y, w, h float32, r, g, b, a float32, proj [16]float32) {
	gl.UseProgram(s.Shader.ID)
	gl.BindVertexArray(s.VAO)

	gl.UniformMatrix4fv(s.Shader.ProjLoc, 1, false, &proj[0])

	model := [16]float32{
		w, 0, 0, 0,
		0, h, 0, 0,
		0, 0, 1, 0,
		x, y, 0, 1,
	}
	gl.UniformMatrix4fv(s.Shader.ModelLoc, 1, false, &model[0])

	gl.Uniform1i(s.Shader.UseTexLoc, 0)
	gl.Uniform4f(s.Shader.ColorLoc, r, g, b, a)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, s.WhiteTex)

	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// OrthoProj 计算正交投影矩阵（Y 轴向下）。
func OrthoProj(left, right, bottom, top float32) [16]float32 {
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
	gl.DeleteProgram(s.Shader.ID)
}
