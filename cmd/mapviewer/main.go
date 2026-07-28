package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/wil"

	"github.com/pyq0109/mirgo/cmd/mapviewer/renderer"
	"github.com/pyq0109/mirgo/cmd/mapviewer/ui"
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
		fmt.Fprintln(os.Stderr, "用法: mapviewer <地图文件> [资源目录]")
		os.Exit(1)
	}

	mapPath := os.Args[1]
	dataDir := "asset/client/Data"
	if len(os.Args) >= 3 {
		dataDir = os.Args[2]
	}

	// 解析地图文件
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		log.Fatalf("解析地图失败: %v", err)
	}
	fmt.Printf("地图: %dx%d, 标题: %s\n", m.Width, m.Height, string(m.Header.Title[:m.Header.TitleLen]))

	// 加载 WIL 资源文件
	fmt.Println("加载 Tiles.wil ...")
	tiles, err := wil.Load(filepath.Join(dataDir, "Tiles.wil"))
	if err != nil {
		log.Fatalf("加载 Tiles.wil 失败: %v", err)
	}
	fmt.Println("加载 SmTiles.wil ...")
	smTiles, err := wil.Load(filepath.Join(dataDir, "SmTiles.wil"))
	if err != nil {
		log.Fatalf("加载 SmTiles.wil 失败: %v", err)
	}
	fmt.Println("加载 Objects.wil ...")
	objects, err := wil.Load(filepath.Join(dataDir, "Objects.wil"))
	if err != nil {
		log.Fatalf("加载 Objects.wil 失败: %v", err)
	}
	fmt.Printf("Tiles: %d, SmTiles: %d, Objects: %d\n", tiles.Count, smTiles.Count, objects.Count)

	// 初始化 GLFW
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	window, err := glfw.CreateWindow(windowW, windowH, "Map Viewer - "+filepath.Base(mapPath), nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	window.MakeContextCurrent()
	glfw.SwapInterval(1)

	// 初始化 OpenGL
	if err := gl.Init(); err != nil {
		log.Fatal(err)
	}
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.ClearColor(0.1, 0.1, 0.1, 1.0)

	// 初始化 ImGui
	ui.Init(window)
	defer ui.Shutdown()

	// 创建渲染器
	glState, err := renderer.NewGLState()
	if err != nil {
		log.Fatal(err)
	}

	rightW := float64(ui.RightPanelWidth())
	cam := renderer.NewCamera(int(float64(windowW)-rightW), windowH)
	cam.CenterOnContent(float64(m.Width)*renderer.TileWidth, float64(m.Height)*renderer.TileHeight)

	ren := renderer.NewGLRenderer(tiles, smTiles, objects, dataDir, glState)
	minimap := renderer.NewMinimap(m)

	// 显示状态
	showBack := true
	showMid := true
	showFront := true
	showCollision := false
	showGrid := false

	uiState := &ui.UIState{
		Map:            m,
		Renderer:       ren,
		Cam:            cam,
		ShowBackground: &showBack,
		ShowMiddle:     &showMid,
		ShowForeground: &showFront,
		ShowGrid:       &showGrid,
		ShowCollision:  &showCollision,
	}

	// 拖拽状态
	dragging := false
	var lastX, lastY float64

	// 格子锁定状态
	lockedTileX, lockedTileY := -1, -1
	tileLocked := false
	leftPressed := false

	// 网格开关防抖
	gPressed := false

	// 设置 GLFW 回调（转发给 ImGui，加上缩放处理）
	ui.SetGLFWCallbacks(window, func(w *glfw.Window, xoff, yoff float64) {
		io := ui.IO()
		if io.WantCaptureMouse() {
			return
		}
		factor := 1.0
		if yoff > 0 {
			factor = 1.1
		} else if yoff < 0 {
			factor = 0.9
		}
		cx, cy := w.GetCursorPos()
		cam.ZoomAt(factor, cx, cy)
		cam.ClampToBounds(m.Width, m.Height)
	})

	// 窗口大小变化回调
	window.SetFramebufferSizeCallback(func(w *glfw.Window, width, height int) {
		cam.SetViewport(int(float64(width)-rightW), height)
	})

	fmt.Println("地图查看器已启动。")
	fmt.Println("操作: 中键拖拽=平移, 滚轮=缩放, WASD=移动, G=网格, 左键=锁定格子, ESC=退出。")

	// 主循环
	for !window.ShouldClose() {
		glfw.PollEvents()

		glfwW, glfwH := window.GetSize()
		io := ui.IO()

		// 键盘输入（仅在 ImGui 不拦截时处理）
		if !io.WantCaptureKeyboard() {
			speed := 8.0 / cam.Zoom
			moved := false

			if window.GetKey(glfw.KeyW) == glfw.Press || window.GetKey(glfw.KeyUp) == glfw.Press {
				cam.Pan(0, -speed)
				moved = true
			}
			if window.GetKey(glfw.KeyS) == glfw.Press || window.GetKey(glfw.KeyDown) == glfw.Press {
				cam.Pan(0, speed)
				moved = true
			}
			if window.GetKey(glfw.KeyA) == glfw.Press || window.GetKey(glfw.KeyLeft) == glfw.Press {
				cam.Pan(speed, 0)
				moved = true
			}
			if window.GetKey(glfw.KeyD) == glfw.Press || window.GetKey(glfw.KeyRight) == glfw.Press {
				cam.Pan(-speed, 0)
				moved = true
			}

			if window.GetKey(glfw.KeyG) == glfw.Press {
				if !gPressed {
					showGrid = !showGrid
					gPressed = true
				}
			} else {
				gPressed = false
			}

			if window.GetKey(glfw.KeyEscape) == glfw.Press {
				window.SetShouldClose(true)
			}

			if moved {
				cam.ClampToBounds(m.Width, m.Height)
			}
		}

		// 鼠标输入（仅在 ImGui 不拦截时处理）
		mouseTileX, mouseTileY := -1, -1
		if !io.WantCaptureMouse() {
			cx, cy := window.GetCursorPos()

			// 悬停格子
			wx, wy := cam.ScreenToWorld(cx, cy)
			tx, ty := cam.WorldToTile(wx, wy)
			if tx >= 0 && tx < m.Width && ty >= 0 && ty < m.Height {
				mouseTileX, mouseTileY = tx, ty
			}

			// 中键拖拽
			if window.GetMouseButton(glfw.MouseButtonMiddle) == glfw.Press {
				if !dragging {
					dragging = true
					lastX, lastY = cx, cy
				} else {
					dx := cx - lastX
					dy := cy - lastY
					cam.Pan(dx, dy)
					cam.ClampToBounds(m.Width, m.Height)
					lastX, lastY = cx, cy
				}
			} else {
				dragging = false
			}

			// 左键：锁定/解锁格子（带按下检测）
			if window.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press {
				if !leftPressed {
					leftPressed = true
					if tx >= 0 && tx < m.Width && ty >= 0 && ty < m.Height {
						if tileLocked && lockedTileX == tx && lockedTileY == ty {
							tileLocked = false
							lockedTileX, lockedTileY = -1, -1
						} else {
							tileLocked = true
							lockedTileX, lockedTileY = tx, ty
						}
					}
				}
			} else {
				leftPressed = false
			}
		}

		// 更新渲染器高亮状态
		ren.HighlightX, ren.HighlightY = mouseTileX, mouseTileY
		if tileLocked {
			ren.LockedX, ren.LockedY = lockedTileX, lockedTileY
		} else {
			ren.LockedX, ren.LockedY = -1, -1
		}

		// 渲染地图（自定义 OpenGL）
		mapVpW := int32(float64(glfwW) - rightW)
		if mapVpW < 1 {
			mapVpW = 1
		}
		gl.Viewport(0, 0, mapVpW, int32(glfwH))
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		ren.Render(m, cam, showBack, showMid, showFront, showCollision, showGrid)

		// 渲染小地图到 FBO
		minimap.Render(cam, m.Width, m.Height, glState)
		uiState.MinimapTex = minimap.FBOTex

		// ImGui 帧
		ui.BeginFrame()

		menuH := ui.FrameHeight()
		shouldClose := false
		ui.RenderMenuBar(&shouldClose)
		if shouldClose {
			window.SetShouldClose(true)
		}

		ui.RenderRightPanel(uiState, int32(glfwW), int32(glfwH), menuH, mouseTileX, mouseTileY, lockedTileX, lockedTileY, tileLocked)
		ui.RenderMinimapWindow(uiState)

		ui.EndFrame()

		window.SwapBuffers()
	}

	// 退出前清理 GL 资源
	ren.Destroy()
	minimap.Destroy()
	glState.Destroy()
}
