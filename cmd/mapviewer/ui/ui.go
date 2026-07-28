package ui

import (
	"fmt"
	"unsafe"

	"github.com/go-gl/glfw/v3.4/glfw"

	ig "github.com/AllenDang/cimgui-go/imgui"
	igglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	igopengl3 "github.com/AllenDang/cimgui-go/impl/opengl3"

	"github.com/pyq0109/mirgo/cmd/mapviewer/renderer"
	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

const (
	rightPanelWidth = 380
	minimapSize     = 200
)

// UIState 持有 UI 和主循环之间的共享状态。
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

// toImGuiWindow 将 go-gl/glfw Window 转换为 cimgui-go 的 GLFWwindow 类型。
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow {
	return igglfw.NewGLFWwindowFromC(unsafe.Pointer(w.Handle()))
}

// Init 初始化 ImGui 并绑定 GLFW 窗口。
func Init(window *glfw.Window) {
	ig.CreateContext()
	ig.StyleColorsDark()

	imWin := toImGuiWindow(window)
	igglfw.InitForOpenGL(imWin, true)
	igopengl3.InitV("#version 330")
}

// Shutdown 关闭 ImGui 后端并销毁上下文。
func Shutdown() {
	igopengl3.Shutdown()
	igglfw.Shutdown()
	ig.DestroyContext()
}

// ScrollHandler 是滚轮事件的回调（在 ImGui 处理之后调用）。
type ScrollHandler func(window *glfw.Window, xoff, yoff float64)

// SetGLFWCallbacks 设置 GLFW 回调并转发给 ImGui。
// scrollHandler 在 ImGui 处理完滚轮事件后调用。
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

// BeginFrame 开始新的 ImGui 帧。
func BeginFrame() {
	igopengl3.NewFrame()
	igglfw.NewFrame()
	ig.NewFrame()
}

// EndFrame 完成并渲染 ImGui 绘制数据。
func EndFrame() {
	ig.Render()
	igopengl3.RenderDrawData(ig.CurrentDrawData())
}

// IO 返回当前 ImGui IO。
func IO() *ig.IO {
	return ig.CurrentIO()
}

// RenderMenuBar 渲染顶部菜单栏（File -> Exit）。
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

// FrameHeight 返回当前 ImGui 帧高度（菜单栏高度）。
func FrameHeight() float32 {
	return ig.FrameHeight()
}

// RenderRightPanel 渲染地图信息 / 格子信息 / 图层开关面板。
// 对应 C++ RenderRightPanel。
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

	// 如果已锁定格子则使用锁定的，否则使用鼠标悬停的
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

	// 背景层
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

	// 中间层
	ig.TextColored(ig.NewVec4(1, 1, 0.5, 1), "Middle Layer")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("  lib: %d, image: %d", info.MiddleLib, info.MiddleImage))
	} else {
		ig.Text("  lib: -, image: -")
	}

	// 前景层
	ig.TextColored(ig.NewVec4(0.5, 0.8, 1, 1), "Front Layer")
	if tileValid {
		info := m.InfoAt(tileX, tileY)
		ig.Text(fmt.Sprintf("  lib: %d, image: %d", info.FrontLib, info.FrontImage))
	} else {
		ig.Text("  lib: -, image: -")
	}

	// 动画信息
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

// RenderMinimapWindow 在独立 ImGui 窗口中渲染小地图。
// 对应 C++ 小地图窗口，支持点击导航。
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

	// InvisibleButton 捕获小地图图片上的鼠标事件
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
		mlog.Logf(mlog.LevelTrace, "minimap", "导航: click=(%.1f, %.1f) world=(%.1f, %.1f) camRaw=(%.1f, %.1f)", px, py, worldX, worldY, camX, camY)
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
		mlog.Logf(mlog.LevelTrace, "minimap", "导航结果: cam=(%.1f, %.1f)", camX, camY)
	}

	if ig.IsItemActivated() {
		// 点击视口矩形外 = 跳转，内部 = 开始拖拽
		vx := float32(state.Cam.X) / mapW * minimapSize
		vy := float32(state.Cam.Y) / mapH * minimapSize
		vw := viewW / mapW * minimapSize
		vh := viewH / mapH * minimapSize
		mlog.Logf(mlog.LevelDebug, "minimap", "点击: mmPos=(%.1f, %.1f) vpRect=(%.1f, %.1f)-(%.1f, %.1f) size=(%.1f, %.1f)", mmMx, mmMy, vx, vy, vx+vw, vy+vh, vw, vh)
		inRect := mmMx >= vx && mmMx <= vx+vw && mmMy >= vy && mmMy <= vy+vh
		mlog.Logf(mlog.LevelDebug, "minimap", "点击 inRect=%v", inRect)
		if !inRect {
			minimapToWorld(mmMx, mmMy)
		}
	}

	if ig.IsItemActive() {
		minimapToWorld(mmMx, mmMy)
	}

	ig.End()
}

// RightPanelWidth 返回右侧面板宽度，用于视口计算。
func RightPanelWidth() int {
	return rightPanelWidth
}
