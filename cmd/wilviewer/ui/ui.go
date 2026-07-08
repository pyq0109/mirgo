package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/go-gl/glfw/v3.4/glfw"

	ig "github.com/AllenDang/cimgui-go/imgui"
	igglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	igopengl3 "github.com/AllenDang/cimgui-go/impl/opengl3"

	"github.com/pyq0109/mirgo/cmd/wilviewer/renderer"
	mlog "github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	leftPanelWidth  = 250
	rightPanelWidth = 380
	thumbnailSize   = 64
	thumbCellPad    = 4
	thumbCellSize   = thumbnailSize + thumbCellPad*2
)

// UIState holds the shared state between the UI and the main loop.
type UIState struct {
	DataDir    string // root data directory
	WILFile    *wil.File
	Renderer   *renderer.WILRenderer
	CurrentIdx int
	Mode       string // "browse" or "animation"

	// Grid state.
	GridScrollTo int // scroll to this image index in grid (-1 = no scroll)

	// Animation state.
	AnimPlaying   bool
	AnimDirection int     // 0-7
	AnimAction    string  // "stand", "walk", "run", etc.
	AnimSpeed     float64 // playback speed multiplier
	animFrameIdx  int     // current frame in sequence
	animLastTick  float64 // glfw timer for animation
}

// toImGuiWindow converts a go-gl/glfw Window to the cimgui-go GLFWwindow type.
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow {
	return igglfw.NewGLFWwindowFromC(unsafe.Pointer(w.Handle()))
}

// Init initializes ImGui with the given GLFW window.
func Init(window *glfw.Window) {
	ig.CreateContext()

	// Load larger default font.
	fontCfg := ig.NewFontConfig()
	fontCfg.SetSizePixels(20.0)
	ig.CurrentIO().Fonts().AddFontDefaultV(fontCfg)
	fontCfg.Destroy()

	ig.StyleColorsDark()
	ig.CurrentStyle().ScaleAllSizes(1.5)

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

// RenderLeftPanel renders the directory tree panel on the left side.
func RenderLeftPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	ig.SetNextWindowPosV(ig.NewVec2(0, menuH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(leftPanelWidth, float32(glfwH)-menuH), ig.CondAlways)

	ig.BeginV("Files", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.DataDir == "" {
		ig.Text("No data directory")
		ig.End()
		return
	}

	ig.Text(filepath.Base(state.DataDir))
	ig.Separator()

	entries, err := os.ReadDir(state.DataDir)
	if err != nil {
		ig.Text("Error reading dir:")
		ig.Text(err.Error())
		ig.End()
		return
	}

	// Collect and sort .wil filenames.
	var wilFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".wil") {
			wilFiles = append(wilFiles, name)
		}
	}
	sort.Strings(wilFiles)

	blue := color.RGBA{R: 102, G: 179, B: 255, A: 255}   // animation
	green := color.RGBA{R: 102, G: 255, B: 102, A: 255}  // static
	yellow := color.RGBA{R: 255, G: 255, B: 102, A: 255} // mixed

	ig.BeginChildStr("filetree")
	for _, name := range wilFiles {
		selected := state.WILFile != nil && strings.EqualFold(state.WILFile.Title, strings.TrimSuffix(name, filepath.Ext(name)))
		cat := wilCategory(name)
		var c color.RGBA
		switch cat {
		case "anim":
			c = blue
		case "static":
			c = green
		case "mixed":
			c = yellow
		default:
			c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
		}
		ig.PushStyleColorVec4(ig.ColText, ig.NewVec4(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255))
		if ig.SelectableBoolV(name, selected, 0, ig.NewVec2(0, 0)) {
			wilPath := filepath.Join(state.DataDir, name)
			mlog.Logf(mlog.LevelInfo, "UI", "选择文件: %s (分类=%s)", name, cat)
			newFile, err := wil.Load(wilPath)
			if err != nil {
				mlog.Logf(mlog.LevelError, "UI", "加载失败: %s, err=%v", wilPath, err)
				ig.PopStyleColor()
				continue
			}
			mlog.Logf(mlog.LevelInfo, "UI", "加载成功: title=%s, images=%d", newFile.Title, newFile.Count)
			state.WILFile = newFile
			state.CurrentIdx = 0
			state.GridScrollTo = 0
			state.Renderer.SetWILFile(newFile)
			state.AnimPlaying = false
			state.animFrameIdx = 0
		}
		ig.PopStyleColor()
	}
	ig.EndChild()

	ig.End()
}

// RenderGridPanel renders the center grid of texture thumbnails.
func RenderGridPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	gridX := float32(leftPanelWidth)
	gridY := menuH
	gridW := float32(glfwW) - gridX - float32(rightPanelWidth)
	gridH := float32(glfwH) - menuH

	ig.SetNextWindowPosV(ig.NewVec2(gridX, gridY), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(gridW, gridH), ig.CondAlways)

	ig.BeginV("Grid", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize|ig.WindowFlagsNoScrollbar)

	if state.WILFile == nil {
		ig.Text("Select a .wil file from the left panel")
		ig.End()
		return
	}

	wf := state.WILFile

	// Calculate columns based on available width.
	availW := ig.ContentRegionAvail().X
	cols := int(availW) / thumbCellSize
	if cols < 1 {
		cols = 1
	}

	ig.BeginChildStr("gridscroll")

	selectedIdx := state.CurrentIdx
	col := 0
	for i := 0; i < wf.Count; i++ {
		img := wf.Images[i]
		if img == nil || img.RGBA == nil {
			col++
			if col >= cols {
				col = 0
			}
			continue
		}

		tex := state.Renderer.GetOrCreateTexture(i)
		if tex == 0 {
			col++
			if col >= cols {
				col = 0
			}
			continue
		}

		// Highlight selected cell.
		if i == selectedIdx {
			ig.PushStyleColorVec4(ig.ColBorder, ig.NewVec4(0.2, 0.6, 1.0, 1.0))
			ig.PushStyleVarFloat(ig.StyleVarFrameBorderSize, 2.0)
		}

		// Cell begin.
		ig.PushIDInt(int32(i))

		// UV: full image (aspect ratio handled by ImageButton sizing).
		uv0 := ig.NewVec2(0, 1)
		uv1 := ig.NewVec2(1, 0)

		texRef := ig.NewTextureRefTextureID(ig.TextureID(tex))
		size := ig.NewVec2(thumbnailSize, thumbnailSize)
		pressed := ig.ImageButtonV(fmt.Sprintf("##thumb%d", i), *texRef, size, uv0, uv1, ig.NewVec4(0.15, 0.15, 0.15, 1), ig.NewVec4(1, 1, 1, 1))

		// Hover tooltip.
		if ig.IsItemHovered() {
			ig.SetTooltip(fmt.Sprintf("#%d  %dx%d", i, img.Width, img.Height))
		}

		// Click to select.
		if pressed {
			state.CurrentIdx = i
			state.GridScrollTo = i
			mlog.Logf(mlog.LevelDebug, "Grid", "选中图像: idx=%d", i)
		}

		ig.PopID()

		if i == selectedIdx {
			ig.PopStyleVar()
			ig.PopStyleColor()
		}

		// SameLine until we fill the row.
		col++
		if col < cols {
			ig.SameLine()
		} else {
			col = 0
		}
	}

	// Auto-scroll to selected image.
	if state.GridScrollTo >= 0 {
		row := state.GridScrollTo / cols
		scrollY := float32(row) * float32(thumbCellSize)
		ig.SetScrollYFloat(scrollY)
		state.GridScrollTo = -1
	}

	ig.EndChild()
	ig.End()
}

// RenderInfoPanel renders the right-top panel with file info and controls.
func RenderInfoPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	infoH := (float32(glfwH) - menuH) * 0.4
	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), menuH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(rightPanelWidth, infoH), ig.CondAlways)

	ig.BeginV("WIL Info", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.WILFile == nil {
		ig.Text("Select a .wil file from")
		ig.Text("the left panel")
		ig.Separator()
		ig.Text("Controls:")
		ig.BulletText("Arrow keys: Navigate images")
		ig.BulletText("Scroll: Zoom in/out")
		ig.BulletText("ESC: Quit")
		ig.End()
		return
	}

	wf := state.WILFile

	// File info.
	ig.Text(fmt.Sprintf("Title: %s", wf.Title))
	ig.Text(fmt.Sprintf("Images: %d", wf.Count))
	ig.Separator()

	// Mode selection.
	ig.Text("Mode:")
	if ig.RadioButtonBool("Browse", state.Mode == "browse") {
		state.Mode = "browse"
		mlog.Logf(mlog.LevelInfo, "UI", "模式切换: browse")
	}
	ig.SameLine()
	if ig.RadioButtonBool("Animation", state.Mode == "animation") {
		state.Mode = "animation"
		mlog.Logf(mlog.LevelInfo, "UI", "模式切换: animation")
	}
	ig.Separator()

	// Current image info.
	if state.CurrentIdx >= 0 && state.CurrentIdx < len(wf.Images) {
		img := wf.Images[state.CurrentIdx]
		if img != nil {
			ig.Text(fmt.Sprintf("Index: %d", state.CurrentIdx))
			ig.Text(fmt.Sprintf("Size: %d x %d", img.Width, img.Height))
			ig.Text(fmt.Sprintf("Hotspot: (%d, %d)", img.HotX, img.HotY))
		}
	}
	ig.Separator()

	// Navigation.
	navW := rightPanelWidth - 20
	ig.PushItemWidth(float32(navW))
	if ig.Button("<<") {
		if state.CurrentIdx > 0 {
			state.CurrentIdx = 0
			state.GridScrollTo = 0
		}
	}
	ig.SameLine()
	if ig.Button("<") {
		if state.CurrentIdx > 0 {
			state.CurrentIdx--
			state.GridScrollTo = state.CurrentIdx
		}
	}
	ig.SameLine()
	ig.Text(fmt.Sprintf("%d / %d", state.CurrentIdx, wf.Count-1))
	ig.SameLine()
	if ig.Button(">") {
		if state.CurrentIdx < wf.Count-1 {
			state.CurrentIdx++
			state.GridScrollTo = state.CurrentIdx
		}
	}
	ig.SameLine()
	if ig.Button(">>") {
		if state.CurrentIdx < wf.Count-1 {
			state.CurrentIdx = wf.Count - 1
			state.GridScrollTo = state.CurrentIdx
		}
	}
	ig.PopItemWidth()
	ig.Separator()

	// Export.
	if ig.Button("Export PNG") {
		if state.CurrentIdx >= 0 && state.CurrentIdx < len(wf.Images) {
			dir := state.DataDir + "/export"
			os.MkdirAll(dir, 0755)
			path := dir + "/" + formatIdx(state.CurrentIdx) + ".png"
			mlog.Logf(mlog.LevelInfo, "Export", "用户点击导出单张: idx=%d, path=%s", state.CurrentIdx, path)
			if err := state.Renderer.ExportPNG(state.CurrentIdx, path); err != nil {
				ig.TextColored(ig.NewVec4(1, 0.3, 0.3, 1), "Export failed")
			} else {
				ig.TextColored(ig.NewVec4(0.3, 1, 0.3, 1), "Exported: "+path)
			}
		}
	}
	ig.SameLine()
	if ig.Button("Export All") {
		dir := state.DataDir + "/export"
		os.MkdirAll(dir, 0755)
		mlog.Logf(mlog.LevelInfo, "Export", "用户点击批量导出: dir=%s", dir)
		n, err := state.Renderer.ExportAllPNG(dir)
		if err != nil {
			ig.TextColored(ig.NewVec4(1, 0.3, 0.3, 1), "Export failed")
		} else {
			ig.TextColored(ig.NewVec4(0.3, 1, 0.3, 1), fmt.Sprintf("Exported %d images", n))
		}
	}

	ig.End()
}

// RenderPreviewPanel renders the right-bottom panel with image preview or animation.
func RenderPreviewPanel(state *UIState, glfwW, glfwH int32, menuH float32) {
	infoH := (float32(glfwH) - menuH) * 0.4
	previewY := menuH + infoH
	previewH := float32(glfwH) - previewY

	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), previewY), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(rightPanelWidth, previewH), ig.CondAlways)

	ig.BeginV("Preview", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.WILFile == nil {
		ig.End()
		return
	}

	wf := state.WILFile

	// Animation controls (only in animation mode).
	if state.Mode == "animation" {
		renderAnimationControls(state, wf)
		ig.End()
		return
	}

	// Browse mode: show selected image as ImGui Image.
	if state.CurrentIdx >= 0 && state.CurrentIdx < len(wf.Images) {
		img := wf.Images[state.CurrentIdx]
		if img != nil && img.RGBA != nil {
			tex := state.Renderer.GetOrCreateTexture(state.CurrentIdx)
			if tex != 0 {
				// Calculate image size to fit in available region, keeping aspect ratio.
				avail := ig.ContentRegionAvail()
				imgW := float32(img.Width)
				imgH := float32(img.Height)
				scale := math.Min(float64(avail.X)/float64(imgW), float64(avail.Y)/float64(imgH))
				if scale > 4.0 {
					scale = 4.0 // cap at 4x
				}
				drawW := float32(float64(imgW) * scale)
				drawH := float32(float64(imgH) * scale)

				// Center the image.
				offsetX := (avail.X - drawW) / 2
				offsetY := (avail.Y - drawH) / 2
				if offsetX > 0 || offsetY > 0 {
					ig.SetCursorPos(ig.NewVec2(ig.CursorPosX()+offsetX, ig.CursorPosY()+offsetY))
				}

				texRef := ig.NewTextureRefTextureID(ig.TextureID(tex))
				ig.ImageWithBgV(*texRef, ig.NewVec2(drawW, drawH), ig.NewVec2(0, 1), ig.NewVec2(1, 0), ig.NewVec4(0, 0, 0, 0), ig.NewVec4(1, 1, 1, 1))
			}
		}
	}

	ig.End()
}

// renderAnimationControls renders the animation control panel.
func renderAnimationControls(state *UIState, wf *wil.File) {
	ig.TextColored(ig.NewVec4(0.4, 0.7, 1.0, 1), "Animation Controls")

	// Action selection.
	ig.Text("Action:")
	actions := []string{"stand", "walk", "run", "attack", "spell", "hit", "death"}
	for i, a := range actions {
		if i > 0 {
			ig.SameLine()
		}
		if ig.RadioButtonBool(a, state.AnimAction == a) {
			state.AnimAction = a
			state.AnimPlaying = false
			state.animFrameIdx = 0
			mlog.Logf(mlog.LevelInfo, "Anim", "动作切换: %s, direction=%d", a, state.AnimDirection)
		}
	}
	if state.AnimAction == "" {
		state.AnimAction = "stand"
	}

	// Direction selection.
	ig.Text("Direction:")
	dirNames := []string{"Up", "UpRight", "Right", "DownRight", "Down", "DownLeft", "Left", "UpLeft"}
	dirArrows := []string{"\u2191", "\u2197", "\u2192", "\u2198", "\u2193", "\u2199", "\u2190", "\u2196"}
	for i := 0; i < 8; i++ {
		if i > 0 {
			ig.SameLine()
		}
		if ig.RadioButtonBool(dirArrows[i], state.AnimDirection == i) {
			state.AnimDirection = i
			state.animFrameIdx = 0
			mlog.Logf(mlog.LevelInfo, "Anim", "方向切换: %s (%d), action=%s", dirNames[i], i, state.AnimAction)
		}
		if ig.IsItemHovered() {
			ig.SetTooltip(dirNames[i])
		}
	}

	// Playback controls.
	ig.Text("Playback:")
	if ig.Button("|<") {
		state.animFrameIdx = 0
	}
	ig.SameLine()
	if ig.Button("<") {
		if state.animFrameIdx > 0 {
			state.animFrameIdx--
		}
	}
	ig.SameLine()
	playLabel := "Play"
	if state.AnimPlaying {
		playLabel = "Pause"
	}
	if ig.Button(playLabel) {
		state.AnimPlaying = !state.AnimPlaying
		if state.AnimPlaying {
			mlog.Logf(mlog.LevelInfo, "Anim", "播放: action=%s, dir=%d, speed=%.1f", state.AnimAction, state.AnimDirection, state.AnimSpeed)
		} else {
			mlog.Logf(mlog.LevelInfo, "Anim", "暂停: action=%s, dir=%d, frame=%d", state.AnimAction, state.AnimDirection, state.animFrameIdx)
		}
	}
	ig.SameLine()
	if ig.Button(">") {
		state.animFrameIdx++
	}
	ig.SameLine()
	if ig.Button(">|") {
		state.animFrameIdx = 0
		state.AnimPlaying = false
	}

	// Speed control.
	ig.Text("Speed:")
	if state.AnimSpeed == 0 {
		state.AnimSpeed = 1.0
	}
	speedF32 := float32(state.AnimSpeed)
	ig.SliderFloat("##speed", &speedF32, 0.1, 5.0)
	state.AnimSpeed = float64(speedF32)

	// Frame info.
	frames := calcAnimFrames(state.AnimAction, state.AnimDirection, wf.Count)
	totalFrames := len(frames)
	if totalFrames > 0 {
		if state.animFrameIdx >= totalFrames {
			state.animFrameIdx = 0
		}
		actualFrame := frames[state.animFrameIdx]
		state.CurrentIdx = actualFrame

		dirName := "N/A"
		if state.AnimDirection >= 0 && state.AnimDirection < 8 {
			dirName = dirNames[state.AnimDirection]
		}
		ig.Text(fmt.Sprintf("Frame: %d/%d (image %d)", state.animFrameIdx+1, totalFrames, actualFrame))
		ig.Text(fmt.Sprintf("Direction: %s (%d)", dirName, state.AnimDirection))
		ig.Text(fmt.Sprintf("Action: %s", state.AnimAction))

		// Show animation frame preview.
		tex := state.Renderer.GetOrCreateTexture(actualFrame)
		if tex != 0 {
			img := state.Renderer.GetImage(actualFrame)
			if img != nil {
				avail := ig.ContentRegionAvail()
				imgW := float32(img.Width)
				imgH := float32(img.Height)
				scale := math.Min(float64(avail.X)/float64(imgW), float64(avail.Y)/float64(imgH))
				if scale > 4.0 {
					scale = 4.0
				}
				drawW := float32(float64(imgW) * scale)
				drawH := float32(float64(imgH) * scale)
				offsetX := (avail.X - drawW) / 2
				offsetY := (avail.Y - drawH) / 2
				if offsetX > 0 || offsetY > 0 {
					ig.SetCursorPos(ig.NewVec2(ig.CursorPosX()+offsetX, ig.CursorPosY()+offsetY))
				}
				texRef := ig.NewTextureRefTextureID(ig.TextureID(tex))
				ig.ImageWithBgV(*texRef, ig.NewVec2(drawW, drawH), ig.NewVec2(0, 1), ig.NewVec2(1, 0), ig.NewVec4(0, 0, 0, 0), ig.NewVec4(1, 1, 1, 1))
			}
		}

		// Auto-advance animation.
		if state.AnimPlaying {
			now := glfw.GetTime()
			interval := 0.1 / state.AnimSpeed
			if now-state.animLastTick >= interval {
				state.animLastTick = now
				state.animFrameIdx++
				if state.animFrameIdx >= totalFrames {
					state.animFrameIdx = 0
				}
			}
		}
	} else {
		ig.Text("No animation frames available")
	}
}

// calcAnimFrames calculates the frame indices for an animation.
func calcAnimFrames(action string, direction int, maxCount int) []int {
	var start, frameCount int
	switch action {
	case "stand":
		start = 0
		frameCount = 4
	case "walk":
		start = 64
		frameCount = 48
	case "run":
		start = 128
		frameCount = 48
	case "attack":
		start = 192
		frameCount = 48
	case "spell":
		start = 256
		frameCount = 48
	case "hit":
		start = 320
		frameCount = 24
	case "death":
		start = 384
		frameCount = 24
	default:
		frames := make([]int, maxCount)
		for i := range frames {
			frames[i] = i
		}
		return frames
	}

	dirFrames := frameCount / 8
	if dirFrames < 1 {
		dirFrames = 1
	}
	base := start + direction*dirFrames

	frames := make([]int, 0, dirFrames)
	for i := 0; i < dirFrames; i++ {
		idx := base + i
		if idx < maxCount {
			frames = append(frames, idx)
		}
	}
	return frames
}

// RightPanelWidth returns the width of the right panel for viewport calculations.
func RightPanelWidth() int {
	return rightPanelWidth
}

// LeftPanelWidth returns the width of the left panel.
func LeftPanelWidth() int {
	return leftPanelWidth
}

func formatIdx(i int) string {
	if i < 10 {
		return "000" + string(rune('0'+i))
	}
	if i < 100 {
		return fmt.Sprintf("%03d", i)
	}
	if i < 1000 {
		return fmt.Sprintf("%03d", i)
	}
	return fmt.Sprintf("%04d", i)
}

// wilCategory classifies a WIL file by its name.
func wilCategory(name string) string {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	switch {
	case base == "hum" || base == "humeffect" || base == "hair" || base == "weapon":
		return "anim"
	case base == "npc" || base == "dragon":
		return "anim"
	case base == "magic" || base == "magic2":
		return "anim"
	case base == "effect" || base == "event":
		return "anim"
	case strings.HasPrefix(base, "mon"):
		return "anim"
	case base == "items" || base == "stateitem" || base == "dnitems":
		return "static"
	case base == "prguse" || base == "prguse2" || base == "prguse3":
		return "static"
	case base == "chrsel" || base == "mmap" || base == "magicon":
		return "static"
	case base == "tiles" || base == "smtiles":
		return "static"
	case strings.HasPrefix(base, "objects"):
		return "mixed"
	default:
		return "unknown"
	}
}
