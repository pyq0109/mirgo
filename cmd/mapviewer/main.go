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
		fmt.Fprintln(os.Stderr, "Usage: mapviewer <mapfile> [datadir]")
		os.Exit(1)
	}

	mapPath := os.Args[1]
	dataDir := "asset/client/Data"
	if len(os.Args) >= 3 {
		dataDir = os.Args[2]
	}

	// Parse map.
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		log.Fatalf("Failed to parse map: %v", err)
	}
	fmt.Printf("Map: %dx%d, title: %s\n", m.Width, m.Height, string(m.Header.Title[:m.Header.TitleLen]))

	// Load WIL files.
	fmt.Println("Loading Tiles.wil ...")
	tiles, err := wil.Load(filepath.Join(dataDir, "Tiles.wil"))
	if err != nil {
		log.Fatalf("Failed to load Tiles.wil: %v", err)
	}
	fmt.Println("Loading SmTiles.wil ...")
	smTiles, err := wil.Load(filepath.Join(dataDir, "SmTiles.wil"))
	if err != nil {
		log.Fatalf("Failed to load SmTiles.wil: %v", err)
	}
	fmt.Println("Loading Objects.wil ...")
	objects, err := wil.Load(filepath.Join(dataDir, "Objects.wil"))
	if err != nil {
		log.Fatalf("Failed to load Objects.wil: %v", err)
	}
	fmt.Printf("Tiles: %d, SmTiles: %d, Objects: %d\n", tiles.Count, smTiles.Count, objects.Count)

	// Init GLFW.
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

	rightW := float64(ui.RightPanelWidth())
	cam := renderer.NewCamera(int(float64(windowW)-rightW), windowH)
	cam.CenterOnContent(float64(m.Width)*renderer.TileWidth, float64(m.Height)*renderer.TileHeight)

	ren := renderer.NewGLRenderer(tiles, smTiles, objects, dataDir, glState)
	minimap := renderer.NewMinimap(m)

	// State.
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

	// Drag state.
	dragging := false
	var lastX, lastY float64

	// Tile lock state.
	lockedTileX, lockedTileY := -1, -1
	tileLocked := false
	leftPressed := false

	// Grid toggle debounce.
	gPressed := false

	// Set GLFW callbacks (forward to ImGui, plus zoom handler).
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

	// Window resize callback.
	window.SetFramebufferSizeCallback(func(w *glfw.Window, width, height int) {
		cam.SetViewport(int(float64(width)-rightW), height)
	})

	fmt.Println("Map viewer started.")
	fmt.Println("Controls: middle-drag=pan, scroll=zoom, WASD=navigate, G=grid, left-click=lock tile, ESC=quit.")

	// Main loop.
	for !window.ShouldClose() {
		glfw.PollEvents()

		glfwW, glfwH := window.GetSize()
		io := ui.IO()

		// Keyboard input (only if ImGui doesn't want it).
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

		// Mouse input (only if ImGui doesn't want it).
		mouseTileX, mouseTileY := -1, -1
		if !io.WantCaptureMouse() {
			cx, cy := window.GetCursorPos()

			// Hover tile.
			wx, wy := cam.ScreenToWorld(cx, cy)
			tx, ty := cam.WorldToTile(wx, wy)
			if tx >= 0 && tx < m.Width && ty >= 0 && ty < m.Height {
				mouseTileX, mouseTileY = tx, ty
			}

			// Middle button drag.
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

			// Left button: tile lock/unlock (with press detection).
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

		// Update renderer highlight state.
		ren.HighlightX, ren.HighlightY = mouseTileX, mouseTileY
		if tileLocked {
			ren.LockedX, ren.LockedY = lockedTileX, lockedTileY
		} else {
			ren.LockedX, ren.LockedY = -1, -1
		}

		// Render map (custom OpenGL).
		mapVpW := int32(float64(glfwW) - rightW)
		if mapVpW < 1 {
			mapVpW = 1
		}
		gl.Viewport(0, 0, mapVpW, int32(glfwH))
		gl.ClearColor(0.1, 0.1, 0.1, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		ren.Render(m, cam, showBack, showMid, showFront, showCollision, showGrid)

		// Render minimap to FBO.
		minimap.Render(cam, m.Width, m.Height, glState)
		uiState.MinimapTex = minimap.FBOTex

		// ImGui frame.
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
}
