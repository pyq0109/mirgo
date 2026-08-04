package main

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// grayPass 实现 Delphi ceGrayScale 死亡灰度后处理（cliUtil.pas:271-291）。
// 世界层先渲染到离屏 FBO，再用片段着色器对每通道取 g=(R+G+B)/3 灰度输出。
// 底栏 UI 在灰度之后绘制，保持彩色（PlayScn.pas:1397-1408）。
type grayPass struct {
	w, h    int
	fbo     uint32
	tex     uint32
	prog    uint32
	vao     uint32
	vbo     uint32
	projLoc int32
	texLoc  int32
}

const grayVertexShader = `#version 330 core
layout(location=0) in vec2 a_pos;
layout(location=1) in vec2 a_uv;
uniform mat4 u_proj;
out vec2 v_uv;
void main() {
    gl_Position = u_proj * vec4(a_pos, 0.0, 1.0);
    v_uv = a_uv;
}
` + "\x00"

const grayFragmentShader = `#version 330 core
in vec2 v_uv;
uniform sampler2D u_tex;
out vec4 frag_color;
void main() {
    vec4 c = texture(u_tex, v_uv);
    // Delphi ceGrayScale：每通道取三通道均值（cliUtil.pas:271-291）。
    float g = (c.r + c.g + c.b) / 3.0;
    frag_color = vec4(g, g, g, c.a);
}
` + "\x00"

func compileGrayShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	csource, free := gl.Strs(source)
	defer free()
	gl.ShaderSource(shader, 1, csource, nil)
	gl.CompileShader(shader)
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)
		info := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(info))
		return 0, fmt.Errorf("shader compile: %s", info)
	}
	return shader, nil
}

// newGrayPass 创建 w×h（逻辑像素）的离屏灰度管线。
func newGrayPass(w, h int) (*grayPass, error) {
	gp := &grayPass{w: w, h: h}

	// 离屏颜色纹理。
	gl.GenTextures(1, &gp.tex)
	gl.BindTexture(gl.TEXTURE_2D, gp.tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	// FBO。
	gl.GenFramebuffers(1, &gp.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, gp.fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, gp.tex, 0)
	if gl.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
		return nil, fmt.Errorf("grayscale framebuffer incomplete")
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)

	// 着色器程序。
	vs, err := compileGrayShader(grayVertexShader, gl.VERTEX_SHADER)
	if err != nil {
		return nil, err
	}
	fs, err := compileGrayShader(grayFragmentShader, gl.FRAGMENT_SHADER)
	if err != nil {
		return nil, err
	}
	gp.prog = gl.CreateProgram()
	gl.AttachShader(gp.prog, vs)
	gl.AttachShader(gp.prog, fs)
	gl.LinkProgram(gp.prog)
	var status int32
	gl.GetProgramiv(gp.prog, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(gp.prog, gl.INFO_LOG_LENGTH, &logLen)
		info := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(gp.prog, logLen, nil, gl.Str(info))
		return nil, fmt.Errorf("grayscale program link: %s", info)
	}
	gl.DeleteShader(vs)
	gl.DeleteShader(fs)
	gp.projLoc = gl.GetUniformLocation(gp.prog, gl.Str("u_proj\x00"))
	gp.texLoc = gl.GetUniformLocation(gp.prog, gl.Str("u_tex\x00"))

	// 全屏四边形（逻辑坐标 0,0–w,h）；UV 垂直翻转以抵消 FBO 左下原点。
	quad := []float32{
		// x, y, u, v
		0, 0, 0, 1,
		float32(w), 0, 1, 1,
		float32(w), float32(h), 1, 0,
		0, 0, 0, 1,
		float32(w), float32(h), 1, 0,
		0, float32(h), 0, 0,
	}
	gl.GenVertexArrays(1, &gp.vao)
	gl.BindVertexArray(gp.vao)
	gl.GenBuffers(1, &gp.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, gp.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(quad)*4, gl.Ptr(quad), gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)

	return gp, nil
}

// bind 切换到离屏 FBO 并设置其视口。
func (gp *grayPass) bind() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, gp.fbo)
	gl.Viewport(0, 0, int32(gp.w), int32(gp.h))
}

// unbind 恢复默认帧缓冲。
func (gp *grayPass) unbind() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// draw 将离屏内容灰度化输出到当前帧缓冲。uiProj 为逻辑坐标正交投影。
func (gp *grayPass) draw(uiProj [16]float32) {
	gl.UseProgram(gp.prog)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, gp.tex)
	gl.Uniform1i(gp.texLoc, 0)
	gl.UniformMatrix4fv(gp.projLoc, 1, false, &uiProj[0])
	gl.BindVertexArray(gp.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)
	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	gl.UseProgram(0)
}

// dispose 释放 GL 资源。
func (gp *grayPass) dispose() {
	if gp.vbo != 0 {
		gl.DeleteBuffers(1, &gp.vbo)
	}
	if gp.vao != 0 {
		gl.DeleteVertexArrays(1, &gp.vao)
	}
	if gp.prog != 0 {
		gl.DeleteProgram(gp.prog)
	}
	if gp.fbo != 0 {
		gl.DeleteFramebuffers(1, &gp.fbo)
	}
	if gp.tex != 0 {
		gl.DeleteTextures(1, &gp.tex)
	}
	*gp = grayPass{}
}
