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
	mlog "github.com/pyq0109/mirgo/internal/log"
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
	mlog.Logf(mlog.LevelInfo, "Main", "启动参数: datadir=%s", dataDir)

	// Verify directory exists.
	info, err := os.Stat(dataDir)
	if err != nil || !info.IsDir() {
		mlog.Logf(mlog.LevelError, "Main", "目录无效: %s, err=%v", dataDir, err)
		log.Fatalf("Not a valid directory: %s", dataDir)
	}
	mlog.Logf(mlog.LevelInfo, "Main", "数据目录: %s", dataDir)

	// Init GLFW.
	if err := glfw.Init(); err != nil {
		mlog.Logf(mlog.LevelError, "Main", "GLFW 初始化失败: %v", err)
		log.Fatal(err)
	}
	defer glfw.Terminate()
	mlog.Logf(mlog.LevelDebug, "Main", "GLFW 初始化成功")

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(windowW, windowH, "WIL Viewer - "+filepath.Base(dataDir), nil, nil)
	if err != nil {
		mlog.Logf(mlog.LevelError, "Main", "窗口创建失败: %v", err)
		log.Fatal(err)
	}
	window.MakeContextCurrent()
	glfw.SwapInterval(1)
	mlog.Logf(mlog.LevelDebug, "Main", "窗口创建成功: %dx%d", windowW, windowH)

	// Init OpenGL.
	if err := gl.Init(); err != nil {
		mlog.Logf(mlog.LevelError, "Main", "OpenGL 初始化失败: %v", err)
		log.Fatal(err)
	}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.ClearColor(0.1, 0.1, 0.1, 1.0)
	mlog.Logf(mlog.LevelDebug, "Main", "OpenGL 初始化成功")

	// Init ImGui.
	ui.Init(window)
	defer ui.Shutdown()
	mlog.Logf(mlog.LevelDebug, "Main", "ImGui 初始化成功")

	// Create renderer.
	glState, err := renderer.NewGLState()
	if err != nil {
		mlog.Logf(mlog.LevelError, "Main", "GLState 创建失败: %v", err)
		log.Fatal(err)
	}

	// Create WIL renderer (no file loaded yet).
	wilRenderer := renderer.NewWILRenderer(nil, glState)
	mlog.Logf(mlog.LevelDebug, "Main", "WIL 渲染器创建成功")

	// Create UI state.
	uiState := &ui.UIState{
		DataDir:    dataDir,
		WILFile:    nil,
		Renderer:   wilRenderer,
		CurrentIdx: 0,
		Mode:       "browse",
		AnimAction: "stand",
		AnimSpeed:  1.0,
	}

	// Pan state.
	panning := false
	var lastPanX, lastPanY float64

	// Set GLFW callbacks: scroll for zoom.
	ui.SetGLFWCallbacks(window, func(w *glfw.Window, xoff, yoff float64) {
		io := ui.IO()
		if io.WantCaptureMouse() {
			return
		}
		oldZoom := wilRenderer.Zoom
		if yoff > 0 {
			wilRenderer.Zoom *= 1.1
		} else if yoff < 0 {
			wilRenderer.Zoom /= 1.1
		}
		if wilRenderer.Zoom < 0.1 {
			wilRenderer.Zoom = 0.1
		}
		if wilRenderer.Zoom > 20 {
			wilRenderer.Zoom = 20
		}
		mlog.Logf(mlog.LevelTrace, "Zoom", "缩放: %.2f -> %.2f", oldZoom, wilRenderer.Zoom)
	})

	// Window resize callback.
	window.SetFramebufferSizeCallback(func(w *glfw.Window, width, height int) {
		mlog.Logf(mlog.LevelDebug, "Main", "窗口大小变更: %dx%d", width, height)
	})

	mlog.Logf(mlog.LevelInfo, "Main", "WIL 查看器启动完成")
	mlog.Logf(mlog.LevelInfo, "Main", "操作: ESC=退出, 左右箭头=切换图像, 滚轮=缩放, 中键拖拽=平移")

	// Main loop.
	for !window.ShouldClose() {
		glfw.PollEvents()

		glfwWi, glfwHi := window.GetSize()
		glfwW := int32(glfwWi)
		glfwH := int32(glfwHi)
		io := ui.IO()

		// Keyboard input (only if ImGui doesn't want it).
		if !io.WantCaptureKeyboard() {
			if window.GetKey(glfw.KeyEscape) == glfw.Press {
				mlog.Logf(mlog.LevelInfo, "Main", "用户按下 ESC，退出")
				window.SetShouldClose(true)
			}

			// Navigate images with arrow keys.
			if uiState.WILFile != nil {
				if window.GetKey(glfw.KeyRight) == glfw.Press {
					if uiState.CurrentIdx < uiState.WILFile.Count-1 {
						uiState.CurrentIdx++
						mlog.Logf(mlog.LevelTrace, "Nav", "右箭头: idx=%d", uiState.CurrentIdx)
					}
				}
				if window.GetKey(glfw.KeyLeft) == glfw.Press {
					if uiState.CurrentIdx > 0 {
						uiState.CurrentIdx--
						mlog.Logf(mlog.LevelTrace, "Nav", "左箭头: idx=%d", uiState.CurrentIdx)
					}
				}
			}
		}

		// Middle button drag for panning (only if ImGui doesn't want mouse).
		if !io.WantCaptureMouse() {
			if window.GetMouseButton(glfw.MouseButtonMiddle) == glfw.Press {
				cx, cy := window.GetCursorPos()
				if !panning {
					panning = true
					lastPanX, lastPanY = cx, cy
					mlog.Logf(mlog.LevelTrace, "Pan", "平移开始: (%.0f, %.0f)", cx, cy)
				} else {
					dx := cx - lastPanX
					dy := cy - lastPanY
					wilRenderer.OffsetX -= dx / wilRenderer.Zoom
					wilRenderer.OffsetY -= dy / wilRenderer.Zoom
					lastPanX, lastPanY = cx, cy
				}
			} else {
				if panning {
					mlog.Logf(mlog.LevelTrace, "Pan", "平移结束: offset=(%.1f, %.1f)", wilRenderer.OffsetX, wilRenderer.OffsetY)
				}
				panning = false
			}
		}

		// Calculate viewport for the center area (between left and right panels).
		leftW := ui.LeftPanelWidth()
		rightW := ui.RightPanelWidth()
		menuH := int32(ui.FrameHeight())
		vpX := int32(leftW)
		vpY := int32(0)
		vpW := glfwW - int32(leftW) - int32(rightW)
		vpH := glfwH - menuH
		if vpW < 1 {
			vpW = 1
		}
		if vpH < 1 {
			vpH = 1
		}

		// Clear full window.
		gl.Viewport(0, 0, glfwW, glfwH)
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		// Render current image in center viewport.
		if uiState.WILFile != nil {
			wilRenderer.Render(uiState.CurrentIdx, vpX, vpY, vpW, vpH)
		}

		// ImGui frame.
		ui.BeginFrame()

		shouldClose := false
		ui.RenderMenuBar(&shouldClose)
		if shouldClose {
			mlog.Logf(mlog.LevelInfo, "Main", "用户点击菜单退出")
			window.SetShouldClose(true)
		}

		ui.RenderLeftPanel(uiState, glfwW, glfwH, float32(menuH))
		ui.RenderMainPanel(uiState, glfwW, glfwH, float32(menuH))

		ui.EndFrame()

		window.SwapBuffers()
	}

	// Cleanup GL resources before exit.
	mlog.Logf(mlog.LevelInfo, "Main", "正在清理资源...")
	wilRenderer.Destroy()
	glState.Destroy()
	mlog.Logf(mlog.LevelInfo, "Main", "退出完成")
}
