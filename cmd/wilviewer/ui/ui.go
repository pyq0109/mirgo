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
	rightPanelWidth = 320
	thumbnailSize   = 64
)

// UIState 持有 UI 和主循环之间的共享状态。
type UIState struct {
	DataDir        string // 根资源目录
	WILFile        *wil.File
	CurrentWILName string // 当前打开的 .wil 文件名
	Renderer       *renderer.WILRenderer
	CurrentIdx     int
	Mode           string // "browse" 或 "animation"

	// 网格状态
	GridScrollTo int // 滚动到网格中此图片索引（-1 = 不滚动）

	// 动画状态
	AnimPlaying   bool
	AnimDirection int     // 0-7
	AnimAction    string  // "stand", "walk", "run" 等
	AnimSpeed     float64 // 播放速度倍率
	animFrameIdx  int     // 当前帧在序列中的索引
	animLastTick  float64 // 动画用 glfw 计时器
}

// toImGuiWindow 将 go-gl/glfw Window 转换为 cimgui-go 的 GLFWwindow 类型。
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow {
	return igglfw.NewGLFWwindowFromC(unsafe.Pointer(w.Handle()))
}

// Init 初始化 ImGui 并绑定 GLFW 窗口。
func Init(window *glfw.Window) {
	ig.CreateContext()

	// 加载更大的默认字体
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

// Shutdown 关闭 ImGui 后端并销毁上下文。
func Shutdown() {
	igopengl3.Shutdown()
	igglfw.Shutdown()
	ig.DestroyContext()
}

// ScrollHandler 是滚轮事件的回调（在 ImGui 处理之后调用）。
type ScrollHandler func(window *glfw.Window, xoff, yoff float64)

// KeyHandler 是键盘事件的回调（在 ImGui 处理之后调用）。
type KeyHandler func(window *glfw.Window, key glfw.Key, action glfw.Action)

// SetGLFWCallbacks 设置 GLFW 回调并转发给 ImGui。
func SetGLFWCallbacks(window *glfw.Window, scrollHandler ScrollHandler, keyHandler KeyHandler) {
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
		if keyHandler != nil {
			keyHandler(w, key, action)
		}
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

// RenderLeftPanel 渲染左侧目录树面板。
func RenderLeftPanel(state *UIState, glfwW, glfwH int32) {
	ig.SetNextWindowPosV(ig.NewVec2(0, 0), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(leftPanelWidth, float32(glfwH)), ig.CondAlways)

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

	// 收集并排序 .wil 文件名
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

	blue := color.RGBA{R: 102, G: 179, B: 255, A: 255}   // 动画
	green := color.RGBA{R: 102, G: 255, B: 102, A: 255}  // 静态
	yellow := color.RGBA{R: 255, G: 255, B: 102, A: 255} // 混合

	ig.BeginChildStr("filetree")
	for _, name := range wilFiles {
		selected := strings.EqualFold(state.CurrentWILName, name)
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
			state.CurrentWILName = name
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

// RenderGridPanel 渲染中间的纹理缩略图网格。
func RenderGridPanel(state *UIState, glfwW, glfwH int32) {
	gridX := float32(leftPanelWidth)
	gridW := float32(glfwW) - gridX - float32(rightPanelWidth)
	gridH := float32(glfwH)

	ig.SetNextWindowPosV(ig.NewVec2(gridX, 0), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(gridW, gridH), ig.CondAlways)

	ig.BeginV("Grid", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize|ig.WindowFlagsNoScrollbar|ig.WindowFlagsNoTitleBar)

	if state.WILFile == nil {
		ig.Text("Select a .wil file from the left panel")
		ig.End()
		return
	}

	wf := state.WILFile

	ig.BeginChildStr("gridscroll")

	// 根据子窗口的实际可用宽度计算列数
	// 按钮宽度 = thumbnailSize + 2*FramePadding.X; 步长 = 按钮宽度 + ItemSpacing.X
	availW := ig.ContentRegionAvail().X
	style := ig.CurrentStyle()
	framePadX := int(style.FramePadding().X)
	itemSpacingX := int(style.ItemSpacing().X)
	buttonW := thumbnailSize + framePadX*2
	step := buttonW + itemSpacingX
	if step < 1 {
		step = 1
	}
	cols := (int(availW) + itemSpacingX) / step
	if cols < 1 {
		cols = 1
	}

	selectedIdx := state.CurrentIdx
	col := 0
	for i := 0; i < wf.Count; i++ {
		img := wf.GetImage(i)
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

		// 高亮选中的格子
		if i == selectedIdx {
			ig.PushStyleColorVec4(ig.ColBorder, ig.NewVec4(0.2, 0.6, 1.0, 1.0))
			ig.PushStyleVarFloat(ig.StyleVarFrameBorderSize, 2.0)
		}

		// 格子开始
		ig.PushIDInt(int32(i))

		// UV: 完整图像（宽高比由 ImageButton 尺寸处理）
		uv0 := ig.NewVec2(0, 0)
		uv1 := ig.NewVec2(1, 1)

		texRef := ig.NewTextureRefTextureID(ig.TextureID(tex))
		size := ig.NewVec2(thumbnailSize, thumbnailSize)
		pressed := ig.ImageButtonV(fmt.Sprintf("##thumb%d", i), *texRef, size, uv0, uv1, ig.NewVec4(0.15, 0.15, 0.15, 1), ig.NewVec4(1, 1, 1, 1))

		// 悬停提示
		if ig.IsItemHovered() {
			ig.SetTooltip(fmt.Sprintf("#%d  %dx%d", i, img.Width, img.Height))
		}

		// 点击选中
		if pressed {
			state.CurrentIdx = i
			mlog.Logf(mlog.LevelDebug, "Grid", "选中图像: idx=%d", i)
		}

		ig.PopID()

		if i == selectedIdx {
			ig.PopStyleVar()
			ig.PopStyleColor()
		}

		// 同行排列直到填满一行
		col++
		if col < cols {
			ig.SameLine()
		} else {
			col = 0
		}
	}

	// 自动滚动到选中的图像
	if state.GridScrollTo >= 0 {
		row := state.GridScrollTo / cols
		rowHeight := ig.FrameHeight() + ig.CurrentStyle().ItemSpacing().Y
		scrollY := float32(row) * rowHeight
		ig.SetScrollYFloat(scrollY)
		state.GridScrollTo = -1
	}

	ig.EndChild()
	ig.End()
}

// RenderInfoPanel 渲染右上方的文件信息和控制面板。
func RenderInfoPanel(state *UIState, glfwW, glfwH int32) {
	infoH := float32(glfwH) * 0.4
	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), 0), ig.CondAlways, ig.NewVec2(0, 0))
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

	// 文件信息
	ig.Text(fmt.Sprintf("Title: %s", wf.Title))
	ig.Text(fmt.Sprintf("Images: %d", wf.Count))
	ig.Separator()

	// 模式选择
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

	// 当前图像信息
	if state.CurrentIdx >= 0 && state.CurrentIdx < wf.Count {
		img := wf.GetImage(state.CurrentIdx)
		if img != nil {
			ig.Text(fmt.Sprintf("Index: %d", state.CurrentIdx))
			ig.Text(fmt.Sprintf("Size: %d x %d", img.Width, img.Height))
			ig.Text(fmt.Sprintf("Hotspot: (%d, %d)", img.HotX, img.HotY))
		}
	}
	ig.Separator()

	// 导航
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

	// 导出
	if ig.Button("Export PNG") {
		if state.CurrentIdx >= 0 && state.CurrentIdx < wf.Count {
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

// RenderPreviewPanel 渲染右下方的图像预览或动画面板。
func RenderPreviewPanel(state *UIState, glfwW, glfwH int32) {
	infoH := float32(glfwH) * 0.4
	previewH := float32(glfwH) - infoH

	ig.SetNextWindowPosV(ig.NewVec2(float32(glfwW-rightPanelWidth), infoH), ig.CondAlways, ig.NewVec2(0, 0))
	ig.SetNextWindowSizeV(ig.NewVec2(rightPanelWidth, previewH), ig.CondAlways)

	ig.BeginV("Preview", nil, ig.WindowFlagsNoMove|ig.WindowFlagsNoResize)

	if state.WILFile == nil {
		ig.End()
		return
	}

	wf := state.WILFile

	// 动画控制（仅动画模式）
	if state.Mode == "animation" {
		renderAnimationControls(state, wf)
		ig.End()
		return
	}

	// 浏览模式：将选中图像显示为 ImGui Image
	if state.CurrentIdx >= 0 && state.CurrentIdx < wf.Count {
		img := wf.GetImage(state.CurrentIdx)
		if img != nil && img.RGBA != nil {
			tex := state.Renderer.GetOrCreateTexture(state.CurrentIdx)
			if tex != 0 {
				// 计算图像尺寸以适应可用区域，保持宽高比
				avail := ig.ContentRegionAvail()
				imgW := float32(img.Width)
				imgH := float32(img.Height)
				scale := math.Min(float64(avail.X)/float64(imgW), float64(avail.Y)/float64(imgH))
				if scale > 4.0 {
					scale = 4.0 // 最大 4 倍
				}
				drawW := float32(float64(imgW) * scale)
				drawH := float32(float64(imgH) * scale)

				// 居中图像
				offsetX := (avail.X - drawW) / 2
				offsetY := (avail.Y - drawH) / 2
				if offsetX > 0 || offsetY > 0 {
					ig.SetCursorPos(ig.NewVec2(ig.CursorPosX()+offsetX, ig.CursorPosY()+offsetY))
				}

				texRef := ig.NewTextureRefTextureID(ig.TextureID(tex))
				ig.ImageWithBgV(*texRef, ig.NewVec2(drawW, drawH), ig.NewVec2(0, 0), ig.NewVec2(1, 1), ig.NewVec4(0, 0, 0, 0), ig.NewVec4(1, 1, 1, 1))
			}
		}
	}

	ig.End()
}

// renderAnimationControls 渲染动画控制面板。
func renderAnimationControls(state *UIState, wf *wil.File) {
	ig.TextColored(ig.NewVec4(0.4, 0.7, 1.0, 1), "Animation Controls")

	// 动作选择
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

	// 方向选择
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

	// 播放控制
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
			state.animLastTick = glfw.GetTime()
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

	// 速度控制
	ig.Text("Speed:")
	if state.AnimSpeed == 0 {
		state.AnimSpeed = 1.0
	}
	speedF32 := float32(state.AnimSpeed)
	ig.SliderFloat("##speed", &speedF32, 0.1, 5.0)
	state.AnimSpeed = float64(speedF32)

	// 帧信息
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

		// 显示动画帧预览
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
				ig.ImageWithBgV(*texRef, ig.NewVec2(drawW, drawH), ig.NewVec2(0, 0), ig.NewVec2(1, 1), ig.NewVec4(0, 0, 0, 0), ig.NewVec4(1, 1, 1, 1))
			}
		}

		// 自动推进动画
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

// calcAnimFrames 计算动画的帧索引序列。
// 帧数对应 Delphi Actor.pas 中的 TActionInfo 定义。
func calcAnimFrames(action string, direction int, maxCount int) []int {
	var start, frameCount int
	switch action {
	case "stand":
		start = 0
		frameCount = 4
	case "walk":
		start = 64
		frameCount = 60
	case "run":
		start = 128
		frameCount = 60
	case "attack":
		start = 192
		frameCount = 60
	case "spell":
		start = 256
		frameCount = 60
	case "hit":
		start = 320
		frameCount = 30
	case "death":
		start = 384
		frameCount = 30
	default:
		frames := make([]int, maxCount)
		for i := range frames {
			frames[i] = i
		}
		return frames
	}

	// 帧数少于方向数的动作（如 stand=4），所有方向共享帧序列；否则按 8 方向分配
	dirFrames := frameCount
	if frameCount >= 8 {
		dirFrames = frameCount / 8
		start = start + direction*dirFrames
	}

	frames := make([]int, 0, dirFrames)
	for i := 0; i < dirFrames; i++ {
		idx := start + i
		if idx < maxCount {
			frames = append(frames, idx)
		}
	}
	return frames
}

// RightPanelWidth 返回右侧面板宽度，用于视口计算。
func RightPanelWidth() int {
	return rightPanelWidth
}

// LeftPanelWidth 返回左侧面板宽度。
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

// wilCategory 根据文件名为 WIL 文件分类。
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
