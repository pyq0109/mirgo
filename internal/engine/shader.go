// Package engine provides shared OpenGL rendering infrastructure for the MIR2 client.
package engine

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

const vertexShader = `#version 330 core
layout(location=0) in vec2 a_pos;
layout(location=1) in vec2 a_uv;
uniform mat4 u_proj;
uniform mat4 u_model;
out vec2 v_uv;
void main() {
    gl_Position = u_proj * u_model * vec4(a_pos, 0.0, 1.0);
    v_uv = a_uv;
}
` + "\x00"

const fragmentShader = `#version 330 core
in vec2 v_uv;
uniform sampler2D u_tex;
uniform vec4 u_color;
uniform bool u_use_tex;
out vec4 frag_color;
void main() {
    if (u_use_tex) {
        frag_color = texture(u_tex, v_uv);
        if (frag_color.a < 0.01) discard;
    } else {
        frag_color = u_color;
    }
}
` + "\x00"

const colorVertexShader = `#version 330 core
layout(location=0) in vec2 a_pos;
uniform mat4 u_proj;
uniform mat4 u_model;
void main() {
    gl_Position = u_proj * u_model * vec4(a_pos, 0.0, 1.0);
}
` + "\x00"

const colorFragmentShader = `#version 330 core
uniform vec4 u_color;
out vec4 frag_color;
void main() {
    frag_color = u_color;
}
` + "\x00"

func compileShader(source string, shaderType uint32) (uint32, error) {
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
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(log))
		return 0, fmt.Errorf("shader compile: %s", log)
	}
	return shader, nil
}

func linkProgram(shaders ...uint32) (uint32, error) {
	program := gl.CreateProgram()
	for _, s := range shaders {
		gl.AttachShader(program, s)
	}
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(program, logLen, nil, gl.Str(log))
		return 0, fmt.Errorf("program link: %s", log)
	}
	for _, s := range shaders {
		gl.DeleteShader(s)
	}
	return program, nil
}

// TextureShader holds the texture shader program and uniform locations.
type TextureShader struct {
	ID       uint32
	ProjLoc  int32
	ModelLoc int32
	TexLoc   int32
	ColorLoc int32
	UseTexLoc int32
}

// NewTextureShader compiles and links the texture shader.
func NewTextureShader() (*TextureShader, error) {
	vs, err := compileShader(vertexShader, gl.VERTEX_SHADER)
	if err != nil {
		return nil, err
	}
	fs, err := compileShader(fragmentShader, gl.FRAGMENT_SHADER)
	if err != nil {
		return nil, err
	}
	prog, err := linkProgram(vs, fs)
	if err != nil {
		return nil, err
	}
	return &TextureShader{
		ID:       prog,
		ProjLoc:  gl.GetUniformLocation(prog, gl.Str("u_proj\x00")),
		ModelLoc: gl.GetUniformLocation(prog, gl.Str("u_model\x00")),
		TexLoc:   gl.GetUniformLocation(prog, gl.Str("u_tex\x00")),
		ColorLoc: gl.GetUniformLocation(prog, gl.Str("u_color\x00")),
		UseTexLoc: gl.GetUniformLocation(prog, gl.Str("u_use_tex\x00")),
	}, nil
}

// ColorShader holds the color shader program and uniform locations.
type ColorShader struct {
	ID       uint32
	ProjLoc  int32
	ModelLoc int32
	ColorLoc int32
}

// NewColorShader compiles and links the color shader.
func NewColorShader() (*ColorShader, error) {
	vs, err := compileShader(colorVertexShader, gl.VERTEX_SHADER)
	if err != nil {
		return nil, err
	}
	fs, err := compileShader(colorFragmentShader, gl.FRAGMENT_SHADER)
	if err != nil {
		return nil, err
	}
	prog, err := linkProgram(vs, fs)
	if err != nil {
		return nil, err
	}
	return &ColorShader{
		ID:       prog,
		ProjLoc:  gl.GetUniformLocation(prog, gl.Str("u_proj\x00")),
		ModelLoc: gl.GetUniformLocation(prog, gl.Str("u_model\x00")),
		ColorLoc: gl.GetUniformLocation(prog, gl.Str("u_color\x00")),
	}, nil
}
