package engine

import (
	"fmt"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
)

func init() {
	runtime.LockOSThread()
}

// Window 表示带 OpenGL 上下文的 GLFW 窗口。
type Window struct {
	window *glfw.Window
	width  int
	height int
	title  string
}

// NewWindow 创建一个使用 OpenGL 3.3 Core Profile 的 GLFW 窗口。
func NewWindow(width, height int, title string) (*Window, error) {
	if err := glfw.Init(); err != nil {
		return nil, fmt.Errorf("glfw init: %w", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Resizable, glfw.True)

	window, err := glfw.CreateWindow(width, height, title, nil, nil)
	if err != nil {
		glfw.Terminate()
		return nil, fmt.Errorf("create window: %w", err)
	}

	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	if err := gl.Init(); err != nil {
		glfw.Terminate()
		return nil, fmt.Errorf("gl init: %w", err)
	}

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.ClearColor(0, 0, 0, 1)

	return &Window{
		window: window,
		width:  width,
		height: height,
		title:  title,
	}, nil
}

// Run 用给定的 update 和 render 函数启动主循环。
func (w *Window) Run(updateFn func(dt float64), renderFn func()) {
	lastTime := glfw.GetTime()

	for !w.window.ShouldClose() {
		currentTime := glfw.GetTime()
		dt := currentTime - lastTime
		lastTime = currentTime

		glfw.PollEvents()

		if updateFn != nil {
			updateFn(dt)
		}

		gl.Clear(gl.COLOR_BUFFER_BIT)

		if renderFn != nil {
			renderFn()
		}

		w.window.SwapBuffers()
	}
}

// ShouldClose 在窗口应关闭时返回 true。
func (w *Window) ShouldClose() bool {
	return w.window.ShouldClose()
}

// Destroy 终止 GLFW。
func (w *Window) Destroy() {
	w.window.Destroy()
	glfw.Terminate()
}

// GetSize 返回窗口大小。
func (w *Window) GetSize() (int, int) {
	return w.window.GetSize()
}

// GetFramebufferSize 返回帧缓冲区大小。
func (w *Window) GetFramebufferSize() (int, int) {
	return w.window.GetFramebufferSize()
}

// GetCursorPos 返回光标位置。
func (w *Window) GetCursorPos() (float64, float64) {
	return w.window.GetCursorPos()
}

// GetKey 返回按键状态。
func (w *Window) GetKey(key glfw.Key) glfw.Action {
	return w.window.GetKey(key)
}

// GetMouseButton 返回鼠标按键状态。
func (w *Window) GetMouseButton(button glfw.MouseButton) glfw.Action {
	return w.window.GetMouseButton(button)
}

// SetKeyCallback 设置按键回调。
func (w *Window) SetKeyCallback(cb func(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)) {
	w.window.SetKeyCallback(cb)
}

// SetMouseButtonCallback 设置鼠标按键回调。
func (w *Window) SetMouseButtonCallback(cb func(window *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)) {
	w.window.SetMouseButtonCallback(cb)
}

// SetCursorPosCallback 设置光标位置回调。
func (w *Window) SetCursorPosCallback(cb func(window *glfw.Window, xpos float64, ypos float64)) {
	w.window.SetCursorPosCallback(cb)
}

// SetScrollCallback 设置滚轮回调。
func (w *Window) SetScrollCallback(cb func(window *glfw.Window, xoff float64, yoff float64)) {
	w.window.SetScrollCallback(cb)
}

// SetFramebufferSizeCallback 设置帧缓冲区大小回调。
func (w *Window) SetFramebufferSizeCallback(cb func(window *glfw.Window, width int, height int)) {
	w.window.SetFramebufferSizeCallback(cb)
}

// SetCharCallback 设置字符输入回调。
func (w *Window) SetCharCallback(cb func(window *glfw.Window, char rune)) {
	w.window.SetCharCallback(cb)
}

// GetWindow 返回底层的 GLFW 窗口（用于 ImGui 集成）。
func (w *Window) GetWindow() *glfw.Window {
	return w.window
}

// SetResizable 启用或禁用窗口缩放。
func (w *Window) SetResizable(v bool) {
	if v {
		w.window.SetAttrib(glfw.Resizable, glfw.True)
	} else {
		w.window.SetAttrib(glfw.Resizable, glfw.False)
	}
}
