package ui

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/glfw/v3.4/glfw"

	ig "github.com/AllenDang/cimgui-go/imgui"
	igglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	igopengl3 "github.com/AllenDang/cimgui-go/impl/opengl3"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/renderer"
)

const (
	rightPanelWidth = 380
	minimapSize     = 200
)

// UIState holds the shared state between the UI and the main loop.
type UIState struct {
	Map            *mapformat.MapData
	Renderer       *renderer.GLRenderer
	Cam            *renderer.Camera2D
	ShowBackground *bool
	ShowMiddle     *bool
	ShowForeground *bool
	ShowGrid       *bool
	ShowCollision  *bool
	MinimapTex     uint32
}

// toImGuiWindow converts a go-gl/glfw Window to the cimgui-go GLFWwindow type.
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow {
	return igglfw.NewGLFWwindowFromC(unsafe.Pointer(w.Handle()))
}

// Init initializes ImGui with the given GLFW window.
func Init(window *glfw.Window) {
	ig.CreateContext()
	ig.StyleColorsDark()

	imWin := toImGuiWindow(window)
	igglfw.InitForOpenGL(imWin, true)
	igopengl3.InitV("#version 330")
}

// Shutdown shuts down ImGui backends and destroys the context.
func Shutdown() {
	igopengl3.Shutdown()
	igglfw.Shutdown()
	ig.DestroyContext()
}

// ScrollHandler is a callback for scroll events (after ImGui processing).
type ScrollHandler func(window *glfw.Window, xoff, yoff float64)

// SetGLFWCallbacks sets up GLFW callbacks that forward to ImGui.
// scrollHandler is called after ImGui processes the scroll event.
func SetGLFWCallbacks(window *glfw.Window, scrollHandler ScrollHandler) {
	imWin := toImGuiWindow(window)

	window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		igglfw.MouseButtonCallback(imWin, int32(button), int32(action), int32(mods))
	})

	window.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		igglfw.CursorPosCallback(imWin, xpos, ypos)
	})

	window.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		igglfw.ScrollCallback(imWin, xoff, yoff)
		if scrollHandler != nil {
			scrollHandler(w, xoff, yoff)
		}
	})

	window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		igglfw.KeyCallback(imWin, int32(key), int32(scancode), int32(action), int32(mods))
	})
}

// BeginFrame starts a new ImGui frame.
func BeginFrame() {
	igopengl3.NewFrame()
	igglfw.NewFrame()
	ig.NewFrame()
}

// EndFrame finalizes and renders the ImGui draw data.
func EndFrame() {
	ig.Render()
	igopengl3.RenderDrawData(ig.CurrentDrawData())
}

// IO returns the current ImGui IO.
func IO() *ig.IO {
	return ig.CurrentIO()
}

// RenderMenuBar renders the top menu bar (File -> Exit).
func RenderMenuBar(shouldClose *bool) {
	if !ig.BeginMainMenuBar() {
		return
	}
	if ig.BeginMenu("File") {
		if ig.MenuItemBool("Exit") {
			*shouldClose = true
		}
		ig.EndMenu()
	}
	ig.EndMainMenuBar()
}

// FrameHeight returns the current ImGui frame height (menu bar height).
func FrameHeight() float32 {
	return ig.FrameHeight()
}

// RenderRightPanel renders the map info / tile info / layer toggles panel.
// Matches C++ RenderRightPanel.
func RenderRightPanel(state *UIState, glfwW, glfwH int32, menuH float32, mouseTileX, mouseTileY, lockedTileX, lockedTileY int, tileLocked bool) {
	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), menuH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(rightPanelWidth, float32(glfwH)-menuH), ig.CondAlways)

	ig.BeginV("Map Info", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.Map == nil {
		ig.Text("Open a .map file via command line:")
		ig.Text("  mapviewer <mapfile> [datadir]")
		ig.Separator()
		ig.Text("Controls:")
		ig.BulletText("Middle mouse: Pan")
		ig.BulletText("WASD/Arrows: Navigate")
		ig.BulletText("Scroll: Zoom to cursor")
		ig.BulletText("G: Toggle grid")
		ig.BulletText("Left click: Lock tile")
		ig.End()
		return
	}

	m := state.Map
	hdr := &m.Header
	title := string(hdr.Title[:hdr.TitleLen])

	ig.Text(fmt.Sprintf("Title: %s", title))
	ig.Text("Format: standard (12 bytes/cell)")
	ig.Separator()
	ig.Text(fmt.Sprintf("Size: %d x %d", hdr.Width, hdr.Height))
	ig.Text(fmt.Sprintf("Tiles: %d", int(hdr.Width)*int(hdr.Height)))
	ig.Separator()
	ig.Text("Tile Info")

	// Use locked tile if active, otherwise use mouse hover.
	var tileX, tileY int
	if tileLocked {
		tileX, tileY = lockedTileX, lockedTileY
		ig.TextColored(ig.NewVec4(1, 0.5, 0.5, 1), "[Locked] Click to unlock")
	} else {
		tileX, tileY = mouseTileX, mouseTileY
	}

	tileValid := tileX >= 0 && tileX < m.Width && tileY >= 0 && tileY < m.Height

	if tileValid {
		ig.Text(fmt.Sprintf("Pos: (%d, %d)", tileX, tileY))
	} else {
		ig.Text("Pos: (-, -)")
	}
	ig.Separator()

	// Back layer.
	ig.TextColored(ig.NewVec4(0.5, 1, 0.5, 1), "Back Layer")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("  lib: %d, image: %d", info.BackLib, info.BackImage))
		if info.Collision {
			ig.Text("  collision: Yes")
		} else {
			ig.Text("  collision: No")
		}
	} else {
		ig.Text("  lib: -, image: -")
		ig.Text("  collision: -")
	}

	// Middle layer.
	ig.TextColored(ig.NewVec4(1, 1, 0.5, 1), "Middle Layer")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("  lib: %d, image: %d", info.MiddleLib, info.MiddleImage))
	} else {
		ig.Text("  lib: -, image: -")
	}

	// Front layer.
	ig.TextColored(ig.NewVec4(0.5, 0.8, 1, 1), "Front Layer")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("  lib: %d, image: %d", info.FrontLib, info.FrontImage))
	} else {
		ig.Text("  lib: -, image: -")
	}

	// Animation info.
	ig.TextColored(ig.NewVec4(1, 0.6, 0.3, 1), "Animation")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		if info.FrontLib >= 0 {
			areaName := "Objects.wil"
			if info.FrontArea > 0 {
				areaName = fmt.Sprintf("Objects%d.wil", info.FrontArea+1)
			}
			ig.Text(fmt.Sprintf("  Area: %d (%s)", info.FrontArea, areaName))

			isBlend := info.FrontAniFrame&0x80 != 0
			aniFrames := info.FrontAniFrame & 0x7F
			blendStr := "N"
			if isBlend {
				blendStr = "Y"
			}
			ig.Text(fmt.Sprintf("  AniFrame: 0x%02X (blend=%s, frames=%d)", info.FrontAniFrame, blendStr, aniFrames))
			ig.Text(fmt.Sprintf("  AniTick: %d", info.FrontAniTick))

			doorOpen := info.FrontDoorOffset&0x80 != 0
			hasDoor := info.FrontDoorIndex&0x7F != 0
			doorOpenStr := "N"
			if doorOpen {
				doorOpenStr = "Y"
			}
			hasDoorStr := "N"
			if hasDoor {
				hasDoorStr = "Y"
			}
			ig.Text(fmt.Sprintf("  DoorOffset: 0x%02X (open=%s)", info.FrontDoorOffset, doorOpenStr))
			ig.Text(fmt.Sprintf("  DoorIndex: 0x%02X (has_door=%s)", info.FrontDoorIndex, hasDoorStr))
		} else {
			ig.TextDisabled("  No front object")
		}
	} else {
		ig.TextDisabled("  No front object")
	}

	ig.Separator()
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("Door: %d", info.Door))
		ig.Text(fmt.Sprintf("Light: %d", info.Light))
	} else {
		ig.Text("Door: -")
		ig.Text("Light: -")
	}

	ig.Separator()
	ig.Text(fmt.Sprintf("Zoom: %.0f%%", state.Cam.Zoom*100))
	ig.Text(fmt.Sprintf("Camera: (%.0f, %.0f)", state.Cam.X, state.Cam.Y))

	ig.Separator()
	ig.Text("Layer Visibility")
	ig.Checkbox("Back Layer", state.ShowBackground)
	ig.Checkbox("Middle Layer", state.ShowMiddle)
	ig.Checkbox("Front Layer", state.ShowForeground)
	ig.Checkbox("Collision", state.ShowCollision)

	ig.Separator()
	ig.Text("Controls")
	ig.BulletText("Middle mouse: Pan")
	ig.BulletText("WASD/Arrows: Navigate")
	ig.BulletText("Scroll: Zoom to cursor")

	ig.End()
}

// RenderMinimapWindow renders the minimap in a separate ImGui window.
// Matches C++ minimap window with click-to-navigate.
func RenderMinimapWindow(state *UIState) {
	if state.Map == nil || state.MinimapTex == 0 {
		return
	}
	m := state.Map
	if m.Width <= 128 && m.Height <= 128 {
		return
	}

	ig.SetNextWindowSizeV(ig.NewVec2(220, 240), ig.CondFirstUseEver)
	ig.BeginV("Minimap", nil, ig.WindowFlagsNoScrollbar)

	imgMin := ig.CursorScreenPos()
	texRef := ig.NewTextureRefTextureID(ig.TextureID(state.MinimapTex))
	ig.ImageWithBgV(*texRef, ig.NewVec2(minimapSize, minimapSize), ig.NewVec2(0, 0), ig.NewVec2(1, 1), ig.NewVec4(0, 0, 0, 0), ig.NewVec4(1, 1, 1, 1))

	// InvisibleButton to capture mouse events on the minimap image.
	ig.SetCursorScreenPos(imgMin)
	ig.InvisibleButtonV("##minimap_btn", ig.NewVec2(minimapSize, minimapSize), ig.ButtonFlagsNone)

	mapW := float32(m.Width * renderer.TileWidth)
	mapH := float32(m.Height * renderer.TileHeight)
	viewW := float32(float64(state.Cam.ViewW) / state.Cam.Zoom)
	viewH := float32(float64(state.Cam.ViewH) / state.Cam.Zoom)

	mousePos := ig.CurrentIO().MousePos()
	mmMx := mousePos.X - imgMin.X
	mmMy := mousePos.Y - imgMin.Y

	minimapToWorld := func(px, py float32) {
		worldX := (px / minimapSize) * mapW
		worldY := (py / minimapSize) * mapH
		camX := float64(worldX - viewW/2)
		camY := float64(worldY - viewH/2)
		if camX < 0 {
			camX = 0
		}
		if camY < 0 {
			camY = 0
		}
		if camX+float64(viewW) > float64(mapW) {
			camX = float64(mapW) - float64(viewW)
		}
		if camY+float64(viewH) > float64(mapH) {
			camY = float64(mapH) - float64(viewH)
		}
		state.Cam.X = camX
		state.Cam.Y = camY
	}

	if ig.IsItemActivated() {
		// Click outside viewport rect = jump, inside = start drag.
		vx := float32(state.Cam.X) / mapW * minimapSize
		vy := float32(state.Cam.Y) / mapH * minimapSize
		vw := viewW / mapW * minimapSize
		vh := viewH / mapH * minimapSize
		inRect := mmMx >= vx && mmMx <= vx+vw && mmMy >= vy && mmMy <= vy+vh
		if !inRect {
			minimapToWorld(mmMx, mmMy)
		}
	}

	if ig.IsItemActive() {
		minimapToWorld(mmMx, mmMy)
	}

	ig.End()
}

// RightPanelWidth returns the width of the right panel for viewport calculations.
func RightPanelWidth() int {
	return rightPanelWidth
}
