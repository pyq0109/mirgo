package engine

import (
	"image"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// GLState holds OpenGL resources for rendering.
type GLState struct {
	TextureShader *TextureShader
	ColorShader   *ColorShader
	VAO           uint32
	VBO           uint32
	WhiteTex      uint32
}

// NewGLState initializes OpenGL resources.
func NewGLState() (*GLState, error) {
	texShader, err := NewTextureShader()
	if err != nil {
		return nil, err
	}
	colorShader, err := NewColorShader()
	if err != nil {
		return nil, err
	}

	// Unit quad VBO: pos(2) + uv(2) per vertex, 6 vertices
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

	// White 1x1 texture
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

// UploadTexture uploads an *image.RGBA to an OpenGL texture.
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

// DeleteTexture deletes an OpenGL texture.
func (s *GLState) DeleteTexture(id uint32) {
	if id != 0 {
		gl.DeleteTextures(1, &id)
	}
}

// DrawQuad draws a textured quad at (x, y) with size (w, h).
func (s *GLState) DrawQuad(texID uint32, x, y, w, h float32, proj [16]float32) {
	gl.UseProgram(s.TextureShader.ID)
	gl.BindVertexArray(s.VAO)

	gl.UniformMatrix4fv(s.TextureShader.ProjLoc, 1, false, &proj[0])

	// Model matrix: translate(x,y) scale(w,h)
	model := [16]float32{
		w, 0, 0, 0,
		0, h, 0, 0,
		0, 0, 1, 0,
		x, y, 0, 1,
	}
	gl.UniformMatrix4fv(s.TextureShader.ModelLoc, 1, false, &model[0])

	gl.Uniform1i(s.TextureShader.UseTexLoc, 1)
	gl.Uniform4f(s.TextureShader.ColorLoc, 1, 1, 1, 1)

	gl.ActiveTexture(gl.TEXTURE0)
	if texID != 0 {
		gl.BindTexture(gl.TEXTURE_2D, texID)
	} else {
		gl.BindTexture(gl.TEXTURE_2D, s.WhiteTex)
	}

	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// DrawQuadColor draws a colored quad (no texture).
func (s *GLState) DrawQuadColor(x, y, w, h float32, r, g, b, a float32, proj [16]float32) {
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

	gl.Uniform1i(s.TextureShader.UseTexLoc, 0)
	gl.Uniform4f(s.TextureShader.ColorLoc, r, g, b, a)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, s.WhiteTex)

	gl.DrawArrays(gl.TRIANGLES, 0, 6)
}

// OrthoProj computes an orthographic projection matrix (Y-down) from width/height.
func OrthoProj(width, height float32) [16]float32 {
	return [16]float32{
		2 / width, 0, 0, 0,
		0, -2 / height, 0, 0,
		0, 0, -1, 0,
		-1, 1, 0, 1,
	}
}

// OrthoProj4 computes an orthographic projection matrix from left/right/bottom/top.
// This matches the mapviewer's OrthoProj function.
func OrthoProj4(left, right, bottom, top float32) [16]float32 {
	return [16]float32{
		2 / (right - left), 0, 0, 0,
		0, 2 / (top - bottom), 0, 0,
		0, 0, -1, 0,
		-(right + left) / (right - left), -(top + bottom) / (top - bottom), 0, 1,
	}
}

// Destroy frees all GL resources held by the GLState.
func (s *GLState) Destroy() {
	gl.DeleteTextures(1, &s.WhiteTex)
	gl.DeleteBuffers(1, &s.VBO)
	gl.DeleteVertexArrays(1, &s.VAO)
	gl.DeleteProgram(s.TextureShader.ID)
	gl.DeleteProgram(s.ColorShader.ID)
}
