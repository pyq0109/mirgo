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
	windowW = 1600
	windowH = 1000
)

func init() {
	runtime.LockOSThread()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: wilviewer <资源目录>")
		os.Exit(1)
	}

	dataDir := os.Args[1]
	mlog.Logf(mlog.LevelInfo, "Main", "启动参数: datadir=%s", dataDir)

	// 验证目录是否存在
	info, err := os.Stat(dataDir)
	if err != nil || !info.IsDir() {
		mlog.Logf(mlog.LevelError, "Main", "目录无效: %s, err=%v", dataDir, err)
		log.Fatalf("目录无效: %s", dataDir)
	}
	mlog.Logf(mlog.LevelInfo, "Main", "数据目录: %s", dataDir)

	// 初始化 GLFW
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

	// 初始化 OpenGL
	if err := gl.Init(); err != nil {
		mlog.Logf(mlog.LevelError, "Main", "OpenGL 初始化失败: %v", err)
		log.Fatal(err)
	}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.ClearColor(0.1, 0.1, 0.1, 1.0)
	mlog.Logf(mlog.LevelDebug, "Main", "OpenGL 初始化成功")

	// 初始化 ImGui
	ui.Init(window)
	defer ui.Shutdown()
	mlog.Logf(mlog.LevelDebug, "Main", "ImGui 初始化成功")

	// 创建渲染器
	glState, err := renderer.NewGLState()
	if err != nil {
		mlog.Logf(mlog.LevelError, "Main", "GLState 创建失败: %v", err)
		log.Fatal(err)
	}
	defer glState.Destroy()

	// 创建 WIL 渲染器（尚未加载文件）
	wilRenderer := renderer.NewWILRenderer(nil, glState)
	defer wilRenderer.Destroy()
	mlog.Logf(mlog.LevelDebug, "Main", "WIL 渲染器创建成功")

	// 创建 UI 状态
	uiState := &ui.UIState{
		DataDir:      dataDir,
		WILFile:      nil,
		Renderer:     wilRenderer,
		CurrentIdx:   0,
		GridScrollTo: -1,
		Mode:         "browse",
		AnimAction:   "stand",
		AnimSpeed:    1.0,
	}

	// 设置 GLFW 回调：滚轮用于网格滚动，方向键用于切换图像
	ui.SetGLFWCallbacks(window, nil, func(w *glfw.Window, key glfw.Key, action glfw.Action) {
		if action != glfw.Press {
			return
		}
		if uiState.WILFile == nil {
			return
		}
		switch key {
		case glfw.KeyRight:
			if uiState.CurrentIdx < uiState.WILFile.Count-1 {
				uiState.CurrentIdx++
				uiState.GridScrollTo = uiState.CurrentIdx
				mlog.Logf(mlog.LevelTrace, "Nav", "右箭头: idx=%d", uiState.CurrentIdx)
			}
		case glfw.KeyLeft:
			if uiState.CurrentIdx > 0 {
				uiState.CurrentIdx--
				uiState.GridScrollTo = uiState.CurrentIdx
				mlog.Logf(mlog.LevelTrace, "Nav", "左箭头: idx=%d", uiState.CurrentIdx)
			}
		}
	})

	// 窗口大小变化回调
	window.SetFramebufferSizeCallback(func(w *glfw.Window, width, height int) {
		mlog.Logf(mlog.LevelDebug, "Main", "窗口大小变更: %dx%d", width, height)
	})

	mlog.Logf(mlog.LevelInfo, "Main", "WIL 查看器启动完成")
	mlog.Logf(mlog.LevelInfo, "Main", "操作: ESC=退出, 左右箭头=切换图像")

	// 主循环
	for !window.ShouldClose() {
		glfw.PollEvents()

		glfwWi, glfwHi := window.GetSize()
		glfwW := int32(glfwWi)
		glfwH := int32(glfwHi)
		io := ui.IO()

		// 键盘输入（仅在 ImGui 不拦截时处理）
		if !io.WantCaptureKeyboard() {
			if window.GetKey(glfw.KeyEscape) == glfw.Press {
				mlog.Logf(mlog.LevelInfo, "Main", "用户按下 ESC，退出")
				window.SetShouldClose(true)
			}
		}

		// 清除整个窗口
		gl.Viewport(0, 0, glfwW, glfwH)
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		// ImGui 帧
		ui.BeginFrame()

		ui.RenderLeftPanel(uiState, glfwW, glfwH)
		ui.RenderGridPanel(uiState, glfwW, glfwH)
		ui.RenderInfoPanel(uiState, glfwW, glfwH)
		ui.RenderPreviewPanel(uiState, glfwW, glfwH)

		ui.EndFrame()

		window.SwapBuffers()
	}

	mlog.Logf(mlog.LevelInfo, "Main", "退出完成")
}
