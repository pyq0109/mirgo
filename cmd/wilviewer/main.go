package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"

	"github.com/pyq0109/mirgo/cmd/wilviewer/renderer"
	"github.com/pyq0109/mirgo/cmd/wilviewer/ui"
)

const (
	windowW = 1280
	windowH = 800
)

func init() {
	runtime.LockOSThread()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wilviewer <datadir>")
		os.Exit(1)
	}

	dataDir := os.Args[1]

	// Verify directory exists.
	info, err := os.Stat(dataDir)
	if err != nil || !info.IsDir() {
		log.Fatalf("Not a valid directory: %s", dataDir)
	}
	fmt.Printf("Data directory: %s\n", dataDir)

	// Init GLFW.
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(windowW, windowH, "WIL Viewer - "+filepath.Base(dataDir), nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	// Init OpenGL.
	if err := gl.Init(); err != nil {
		log.Fatal(err)
	}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.ClearColor(0.1, 0.1, 0.1, 1.0)

	// Init ImGui.
	ui.Init(window)
	defer ui.Shutdown()

	// Create renderer.
	glState, err := renderer.NewGLState()
	if err != nil {
		log.Fatal(err)
	}

	// Create WIL renderer (no file loaded yet).
	wilRenderer := renderer.NewWILRenderer(nil, glState)

	// Create UI state.
	uiState := &ui.UIState{
		DataDir:    dataDir,
		WILFile:    nil,
		Renderer:   wilRenderer,
		CurrentIdx: 0,
		Mode:       "browse",
	}

	// Set GLFW callbacks.
	ui.SetGLFWCallbacks(window, nil)

	fmt.Println("WIL viewer started.")
	fmt.Println("Controls: ESC=quit, Arrow keys=navigate images")

	// Main loop.
	for !window.ShouldClose() {
		glfw.PollEvents()

		glfwW, glfwH := window.GetSize()
		io := ui.IO()

		// Keyboard input (only if ImGui doesn't want it).
		if !io.WantCaptureKeyboard() {
			if window.GetKey(glfw.KeyEscape) == glfw.Press {
				window.SetShouldClose(true)
			}

			// Navigate images with arrow keys.
			if uiState.WILFile != nil {
				if window.GetKey(glfw.KeyRight) == glfw.Press {
					if uiState.CurrentIdx < uiState.WILFile.Count-1 {
						uiState.CurrentIdx++
					}
				}
				if window.GetKey(glfw.KeyLeft) == glfw.Press {
					if uiState.CurrentIdx > 0 {
						uiState.CurrentIdx--
					}
				}
			}
		}

		// Render WIL image.
		gl.Viewport(0, 0, int32(glfwW), int32(glfwH))
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		// Render current image.
		if uiState.WILFile != nil {
			wilRenderer.Render(uiState.CurrentIdx)
		}

		// ImGui frame.
		ui.BeginFrame()

		menuH := ui.FrameHeight()
		shouldClose := false
		ui.RenderMenuBar(&shouldClose)
		if shouldClose {
			window.SetShouldClose(true)
		}

		ui.RenderLeftPanel(uiState, int32(glfwW), int32(glfwH), menuH)
		ui.RenderMainPanel(uiState, int32(glfwW), int32(glfwH), menuH)

		ui.EndFrame()

		window.SwapBuffers()
	}

	// Cleanup GL resources before exit.
	wilRenderer.Destroy()
	glState.Destroy()
}
