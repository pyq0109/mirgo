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

// Window represents a GLFW window with OpenGL context.
type Window struct {
	window *glfw.Window
	width  int
	height int
	title  string
}

// NewWindow creates a new GLFW window with OpenGL 3.3 Core Profile.
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
	gl.ClearColor(0.1, 0.1, 0.1, 1.0)

	return &Window{
		window: window,
		width:  width,
		height: height,
		title:  title,
	}, nil
}

// Run starts the main loop with the given update and render functions.
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

// ShouldClose returns true if the window should close.
func (w *Window) ShouldClose() bool {
	return w.window.ShouldClose()
}

// Destroy terminates GLFW.
func (w *Window) Destroy() {
	w.window.Destroy()
	glfw.Terminate()
}

// GetSize returns the window size.
func (w *Window) GetSize() (int, int) {
	return w.window.GetSize()
}

// GetFramebufferSize returns the framebuffer size.
func (w *Window) GetFramebufferSize() (int, int) {
	return w.window.GetFramebufferSize()
}

// GetCursorPos returns the cursor position.
func (w *Window) GetCursorPos() (float64, float64) {
	return w.window.GetCursorPos()
}

// GetKey returns the key state.
func (w *Window) GetKey(key glfw.Key) glfw.Action {
	return w.window.GetKey(key)
}

// GetMouseButton returns the mouse button state.
func (w *Window) GetMouseButton(button glfw.MouseButton) glfw.Action {
	return w.window.GetMouseButton(button)
}

// SetKeyCallback sets the key callback.
func (w *Window) SetKeyCallback(cb func(window *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)) {
	w.window.SetKeyCallback(cb)
}

// SetMouseButtonCallback sets the mouse button callback.
func (w *Window) SetMouseButtonCallback(cb func(window *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)) {
	w.window.SetMouseButtonCallback(cb)
}

// SetCursorPosCallback sets the cursor position callback.
func (w *Window) SetCursorPosCallback(cb func(window *glfw.Window, xpos float64, ypos float64)) {
	w.window.SetCursorPosCallback(cb)
}

// SetScrollCallback sets the scroll callback.
func (w *Window) SetScrollCallback(cb func(window *glfw.Window, xoff float64, yoff float64)) {
	w.window.SetScrollCallback(cb)
}

// SetFramebufferSizeCallback sets the framebuffer size callback.
func (w *Window) SetFramebufferSizeCallback(cb func(window *glfw.Window, width int, height int)) {
	w.window.SetFramebufferSizeCallback(cb)
}

// GetWindow returns the underlying GLFW window (for ImGui integration).
func (w *Window) GetWindow() *glfw.Window {
	return w.window
}
