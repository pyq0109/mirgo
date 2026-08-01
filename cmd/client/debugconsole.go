package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

const (
	wireOff   = 0
	wireHover = 1
	wireAll   = 2
)

// gDebug 是全局控制台引用。由 main.go 在创建后设置,
// 供 UI 事件日志等无法直接持有控制台引用的代码使用。
var gDebug *DebugConsole

// debugClickLog 开关: 为 true 时鼠标点击命中信息输出到控制台。
var debugClickLog bool

func clickLogf(format string, args ...interface{}) {
	if debugClickLog && gDebug != nil {
		gDebug.Printf(format, args...)
	}
}

// DebugCmd 是一条已注册的调试命令。args 不含命令名本身;
// 对带子命令的命令 (如 "ui tree"), args 为 ["tree"]。
type DebugCmd struct {
	Help string
	Fn   func(args []string)
}

// DebugConsole 是跨场景的全局调试叠加层。它由 main.go 持有,
// 在 GLFW 回调层拦截输入、在场景渲染之后绘制自身。场景相关的
// 命令通过 Register/Unregister 动态增删, 场景专有状态 (网格、
// 标签、光照等) 由场景自行保存并通过 StatusExtra 汇报。
type DebugConsole struct {
	Visible   bool
	Lines     []string
	ScrollOff int
	Input     string
	History   []string // 命令历史 (↑↓ 浏览)
	HistIdx   int

	// 全局调试状态
	WireMode int // 0=off, 1=hover, 2=all
	HoverIdx int // 悬停的 WireBounds 下标 (-1=无)
	LockIdx  int // 锁定的 WireBounds 下标 (-1=无)
	ShowFPS  bool
	ShowHUD  bool

	// 场景扩展: 当前场景 (如 PlayScene) 设置, 用于状态栏补充信息
	// 以及标记线框是否已由场景在世界空间渲染。
	StatusExtra func() string
	wireHandled bool // 场景已渲染线框时置 true, 避免屏幕空间重复渲染

	// 每帧由 main.go 更新的鼠标逻辑坐标 (800x600 空间)
	mouseX, mouseY float64

	// 文本选择状态 (行级)
	selStart    int  // 选中起始行 (-1=无选择)
	selEnd      int  // 选中结束行
	selDragging bool // 鼠标拖拽中
	SetClipboard  func(string)  // 剪贴板写入回调 (由 main.go 设置)
	GetClipboard  func() string // 剪贴板读取回调 (由 main.go 设置)

	// 面板高度 (可拖拽调节)
	panelH   int  // 当前高度
	resizing bool // 正在拖拽顶边调节高度

	// 输入行光标位置 (rune 偏移)
	cursorPos int

	// 滚动条显隐 (悬停/滚动时渐显)
	sbAlpha    float32 // 当前透明度 0..1
	sbLastTick int64   // 上次滚动/悬停的时间戳

	cmds    map[string]DebugCmd
	gl      *engine.GLState
	text    *engine.TextRenderer
	sceneMgr *engine.SceneManager

	fps      float64
	lastFrame time.Time
}

// --- 分类颜色 ---

var categoryColors = map[float32][4]float32{
	1: {0, 0.8, 0.8, 0.8}, // OBJ  — cyan
	2: {1, 0, 0, 0.8},     // ACTOR — red
	3: {1, 0, 1, 0.8},     // FX   — magenta
	4: {1, 1, 1, 0.8},     // ITEM — white
}

var categoryNames = map[float32]string{
	0: "?", 1: "OBJ", 2: "ACTOR", 3: "FX", 4: "ITEM",
}

func catColor(cat float32) [4]float32 {
	if c, ok := categoryColors[cat]; ok {
		return c
	}
	return [4]float32{0.5, 0.5, 0.5, 0.6}
}

func catName(cat float32) string {
	if n, ok := categoryNames[cat]; ok {
		return n
	}
	return "?"
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// NewDebugConsole 创建控制台并注册所有场景通用的核心命令。
func NewDebugConsole(gl *engine.GLState, text *engine.TextRenderer, sceneMgr *engine.SceneManager) *DebugConsole {
	dc := &DebugConsole{
		HoverIdx: -1,
		LockIdx:  -1,
		HistIdx:  0,
		selStart: -1,
		panelH:   320,
		cmds:     make(map[string]DebugCmd),
		gl:       gl,
		text:     text,
		sceneMgr: sceneMgr,
	}
	dc.Register("help", "list all commands", dc.cmdHelp)
	dc.Register("clear", "clear console output", dc.cmdClear)
	dc.Register("scene", "show current scene", dc.cmdScene)
	dc.Register("wire", "wireframe: wire | wire all | wire 0", dc.cmdWire)
	dc.Register("fps", "toggle FPS display", dc.cmdFPS)
	dc.Register("hud", "toggle debug status bar", dc.cmdHUD)
	dc.Register("ui", "ui tree|bounds|hit|find|events|state|inspect|show|hide|move|click|hover|list", dc.cmdUI)
	dc.Register("click", "click <x> <y> [right] — simulate click", dc.cmdClick)
	dc.Register("clicklog", "toggle verbose click hit logging", dc.cmdClickLog)
	dc.Register("dump", "dump console output to log", dc.cmdDump)
	return dc
}

// Register 注册 (或覆盖) 一条命令。
func (dc *DebugConsole) Register(name, help string, fn func(args []string)) {
	dc.cmds[name] = DebugCmd{Help: help, Fn: fn}
}

// Unregister 删除一条命令。
func (dc *DebugConsole) Unregister(name string) {
	delete(dc.cmds, name)
}

// SetMouse 每帧由 main.go 更新鼠标逻辑坐标。
func (dc *DebugConsole) SetMouse(x, y float64) {
	dc.mouseX, dc.mouseY = x, y
}

// Toggle 切换控制台显隐。
func (dc *DebugConsole) Toggle() {
	dc.Visible = !dc.Visible
	dc.ScrollOff = 0
	if dc.Visible && len(dc.Lines) == 0 {
		dc.cmdHelp(nil)
	}
}

// --- 输出 ---

func (dc *DebugConsole) Printf(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	dc.Lines = append(dc.Lines, line)
	if len(dc.Lines) > 200 {
		dc.Lines = dc.Lines[len(dc.Lines)-200:]
	}
	dc.ScrollOff = 0
}

// Print 输出可能含换行的文本, 按行拆分加入日志。
func (dc *DebugConsole) Print(text string) {
	for _, line := range strings.Split(text, "\n") {
		dc.Lines = append(dc.Lines, line)
	}
	if len(dc.Lines) > 200 {
		dc.Lines = dc.Lines[len(dc.Lines)-200:]
	}
	dc.ScrollOff = 0
}

// --- 输入 ---

func (dc *DebugConsole) OnKey(key int, action int, mods int) {
	if action == 0 {
		return
	}
	const modCtrl = 0x0002
	if mods&modCtrl != 0 && key == 67 { // Ctrl+C
		dc.CopySelection()
		return
	}
	if mods&modCtrl != 0 && key == 86 { // Ctrl+V
		if dc.GetClipboard != nil {
			clip := dc.GetClipboard()
			clip = strings.ReplaceAll(clip, "\n", " ")
			clip = strings.ReplaceAll(clip, "\r", "")
			remaining := 120 - utf8.RuneCountInString(dc.Input)
			runes := []rune(clip)
			if len(runes) > remaining {
				runes = runes[:remaining]
			}
			dc.insertAtCursor(string(runes))
		}
		return
	}
	if mods&modCtrl != 0 && key == 65 { // Ctrl+A
		if len(dc.Lines) > 0 {
			dc.selStart = 0
			dc.selEnd = len(dc.Lines) - 1
		}
		return
	}
	switch key {
	case 258: // Tab — 自动补全
		dc.tabComplete()
		dc.cursorPos = utf8.RuneCountInString(dc.Input)
	case 257: // Enter
		if action == 1 {
			dc.execute(dc.Input)
			dc.Input = ""
			dc.cursorPos = 0
		}
	case 259: // Backspace
		if dc.cursorPos > 0 {
			runes := []rune(dc.Input)
			dc.Input = string(append(runes[:dc.cursorPos-1], runes[dc.cursorPos:]...))
			dc.cursorPos--
		}
	case 261: // Delete
		runes := []rune(dc.Input)
		if dc.cursorPos < len(runes) {
			dc.Input = string(append(runes[:dc.cursorPos], runes[dc.cursorPos+1:]...))
		}
	case 263: // Left
		if dc.cursorPos > 0 {
			dc.cursorPos--
		}
	case 262: // Right
		if dc.cursorPos < utf8.RuneCountInString(dc.Input) {
			dc.cursorPos++
		}
	case 256: // Escape
		if action == 1 {
			dc.Visible = false
		}
	case 265: // Up — 历史上一条
		dc.histPrev()
		dc.cursorPos = utf8.RuneCountInString(dc.Input)
	case 264: // Down — 历史下一条
		dc.histNext()
		dc.cursorPos = utf8.RuneCountInString(dc.Input)
	case 266: // PageUp
		dc.ScrollOff += 5
		dc.clampScroll()
		dc.sbLastTick = time.Now().UnixMilli()
	case 267: // PageDown
		dc.ScrollOff -= 5
		dc.clampScroll()
		dc.sbLastTick = time.Now().UnixMilli()
	case 268: // Home — 光标移到行首
		dc.cursorPos = 0
	case 269: // End — 光标移到行尾
		dc.cursorPos = utf8.RuneCountInString(dc.Input)
	}
}

func (dc *DebugConsole) OnChar(char rune) {
	if char == '`' {
		return
	}
	if char >= 32 && char != 127 {
		if utf8.RuneCountInString(dc.Input) < 120 {
			dc.insertAtCursor(string(char))
		}
	}
}

func (dc *DebugConsole) insertAtCursor(s string) {
	runes := []rune(dc.Input)
	if dc.cursorPos > len(runes) {
		dc.cursorPos = len(runes)
	}
	newRunes := append(runes[:dc.cursorPos], append([]rune(s), runes[dc.cursorPos:]...)...)
	dc.Input = string(newRunes)
	dc.cursorPos += utf8.RuneCountInString(s)
}

// OnScroll 鼠标滚轮滚动输出区域。yoff>0 向上滚（看更早的内容）。
func (dc *DebugConsole) OnScroll(yoff float64) {
	delta := int(-yoff * 3)
	dc.ScrollOff -= delta
	dc.clampScroll()
	dc.sbLastTick = time.Now().UnixMilli()
}

func (dc *DebugConsole) clampScroll() {
	if dc.ScrollOff < 0 {
		dc.ScrollOff = 0
	}
	lineH := 14
	if dc.text != nil {
		if lh := dc.text.LineHeight(); lh > 0 {
			lineH = lh
		}
	}
	outputH := dc.panelH - 40
	maxVisible := outputH / lineH
	if maxVisible < 1 {
		maxVisible = 1
	}
	maxOff := len(dc.Lines) - maxVisible
	if maxOff < 0 {
		maxOff = 0
	}
	if dc.ScrollOff > maxOff {
		dc.ScrollOff = maxOff
	}
}

// OnMouseButton 处理控制台可见时的鼠标选择。
// 返回 true 表示事件被消费（鼠标在输出区域内）。
func (dc *DebugConsole) OnMouseButton(x, y float64, button int, action int) bool {
	if button != 0 { // 仅左键
		return false
	}
	baseY := float64(ScreenHeight - dc.panelH)

	if action == 1 { // press
		// 顶边拖拽手柄 (±5px)
		if y >= baseY-5 && y <= baseY+5 {
			dc.resizing = true
			return true
		}
	}
	if action == 0 { // release
		dc.resizing = false
	}

	outputTop := baseY + 8
	outputBot := baseY + float64(dc.panelH) - 40
	if y < outputTop || y > outputBot {
		if action == 1 {
			dc.selStart = -1
			dc.selDragging = false
		}
		return false
	}
	lineH := float32(14)
	if dc.text != nil {
		if lh := dc.text.LineHeight(); lh > 0 {
			lineH = float32(lh)
		}
	}
	outputH := float32(dc.panelH - 40)
	maxVisible := int(outputH / lineH)
	total := len(dc.Lines)
	endIdx := total - dc.ScrollOff
	if endIdx > total {
		endIdx = total
	}
	if endIdx < 0 {
		endIdx = 0
	}
	startIdx := endIdx - maxVisible
	if startIdx < 0 {
		startIdx = 0
	}
	relLine := int((float32(y) - float32(outputTop)) / lineH)
	lineIdx := startIdx + relLine
	if lineIdx >= endIdx {
		lineIdx = endIdx - 1
	}
	if lineIdx < 0 {
		lineIdx = 0
	}
	switch action {
	case 1: // press
		dc.selStart = lineIdx
		dc.selEnd = lineIdx
		dc.selDragging = true
	case 0: // release
		dc.selDragging = false
	}
	return true
}

// OnMouseMoveSelect 拖拽时更新选择范围。
func (dc *DebugConsole) OnMouseMoveSelect(x, y float64) {
	if dc.resizing {
		newH := ScreenHeight - int(y)
		if newH < 150 {
			newH = 150
		}
		if newH > 550 {
			newH = 550
		}
		dc.panelH = newH
		return
	}
	if !dc.selDragging {
		return
	}
	baseY := float32(ScreenHeight - dc.panelH)
	outputTop := baseY + 8
	lineH := float32(14)
	if dc.text != nil {
		if lh := dc.text.LineHeight(); lh > 0 {
			lineH = float32(lh)
		}
	}
	outputH := float32(dc.panelH - 40)
	maxVisible := int(outputH / lineH)
	total := len(dc.Lines)
	endIdx := total - dc.ScrollOff
	if endIdx > total {
		endIdx = total
	}
	if endIdx < 0 {
		endIdx = 0
	}
	startIdx := endIdx - maxVisible
	if startIdx < 0 {
		startIdx = 0
	}
	relLine := int((float32(y) - float32(outputTop)) / lineH)
	lineIdx := startIdx + relLine
	if lineIdx >= endIdx {
		lineIdx = endIdx - 1
	}
	if lineIdx < 0 {
		lineIdx = 0
	}
	dc.selEnd = lineIdx
}

// CopySelection 将选中行复制到剪贴板。
func (dc *DebugConsole) CopySelection() {
	if dc.selStart < 0 || dc.SetClipboard == nil {
		return
	}
	lo, hi := dc.selStart, dc.selEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(dc.Lines) {
		hi = len(dc.Lines) - 1
	}
	var sb strings.Builder
	for i := lo; i <= hi; i++ {
		if i > lo {
			sb.WriteByte('\n')
		}
		sb.WriteString(dc.Lines[i])
	}
	dc.SetClipboard(sb.String())
	dc.Printf("copied %d line(s)", hi-lo+1)
}

func (dc *DebugConsole) histPrev() {
	if len(dc.History) == 0 {
		return
	}
	dc.HistIdx--
	if dc.HistIdx < 0 {
		dc.HistIdx = 0
	}
	dc.Input = dc.History[dc.HistIdx]
}

func (dc *DebugConsole) histNext() {
	if len(dc.History) == 0 {
		return
	}
	dc.HistIdx++
	if dc.HistIdx >= len(dc.History) {
		dc.HistIdx = len(dc.History)
		dc.Input = ""
		return
	}
	dc.Input = dc.History[dc.HistIdx]
}

func (dc *DebugConsole) tabComplete() {
	prefix := strings.TrimSpace(dc.Input)
	if prefix == "" {
		return
	}
	// 只补全第一个 token (命令名)
	parts := strings.SplitN(prefix, " ", 2)
	cmdPrefix := parts[0]
	var matches []string
	for name := range dc.cmds {
		if strings.HasPrefix(name, cmdPrefix) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		return
	}
	sort.Strings(matches)
	if len(matches) == 1 {
		rest := ""
		if len(parts) > 1 {
			rest = " " + parts[1]
		}
		dc.Input = matches[0] + rest
	} else {
		dc.Printf("%s", "  "+strings.Join(matches, "  "))
	}
}

// --- 指令解析 ---

func (dc *DebugConsole) execute(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	dc.Printf("> %s", input)
	dc.History = append(dc.History, input)
	if len(dc.History) > 100 {
		dc.History = dc.History[len(dc.History)-100:]
	}
	dc.HistIdx = len(dc.History)

	parts := strings.Fields(input)
	cmd, ok := dc.cmds[parts[0]]
	if !ok {
		dc.Printf("unknown: %s  (help)", parts[0])
		return
	}
	cmd.Fn(parts[1:])
}

// --- 核心命令 ---

func (dc *DebugConsole) cmdHelp(args []string) {
	dc.Printf("=== debug commands ===")
	names := make([]string, 0, len(dc.cmds))
	for n := range dc.cmds {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		dc.Printf("  %-8s %s", n, dc.cmds[n].Help)
	}
}

func (dc *DebugConsole) cmdClear(args []string) {
	dc.Lines = dc.Lines[:0]
	dc.ScrollOff = 0
}

func (dc *DebugConsole) cmdScene(args []string) {
	if dc.sceneMgr != nil {
		dc.Printf("scene: %s", dc.sceneMgr.CurrentType())
	}
}

func (dc *DebugConsole) cmdWire(args []string) {
	if len(args) == 0 {
		if dc.WireMode == wireHover {
			dc.WireMode = wireOff
		} else {
			dc.WireMode = wireHover
		}
		dc.LockIdx = -1
	} else {
		switch args[0] {
		case "all":
			dc.WireMode = wireAll
		case "0", "off":
			dc.WireMode = wireOff
			dc.LockIdx = -1
		default:
			dc.Printf("usage: wire | wire all | wire 0")
			return
		}
	}
	dc.Printf("wire: %s", []string{"off", "hover", "all"}[dc.WireMode])
	if gActiveUI != nil {
		gActiveUI.ShowBounds = dc.WireMode > 0
	}
}

func (dc *DebugConsole) cmdFPS(args []string) {
	dc.ShowFPS = !dc.ShowFPS
	dc.Printf("fps %s", onOff(dc.ShowFPS))
}

func (dc *DebugConsole) cmdHUD(args []string) {
	dc.ShowHUD = !dc.ShowHUD
	dc.Printf("hud %s", onOff(dc.ShowHUD))
}

func (dc *DebugConsole) cmdUI(args []string) {
	ui := gActiveUI
	if ui == nil {
		dc.Printf("no UI in current scene")
		return
	}
	if len(args) == 0 {
		dc.Printf("usage: ui tree|bounds|hit|find|events|state|inspect|show|hide|move|click|hover|list")
		return
	}
	switch args[0] {
	case "tree":
		depth := 3
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &depth)
		}
		dc.Print(ui.DebugTree(depth))
	case "bounds":
		ui.ShowBounds = !ui.ShowBounds
		dc.Printf("ui bounds %s", onOff(ui.ShowBounds))
	case "hit":
		dc.Print(ui.DebugHitTest(int(dc.mouseX), int(dc.mouseY)))
	case "focus":
		dc.Print(ui.DebugFocus())
	case "find":
		if len(args) < 2 {
			dc.Printf("usage: ui find <name>")
			return
		}
		dc.Print(ui.DebugFind(strings.Join(args[1:], " ")))
	case "events":
		debugUIEvents = !debugUIEvents
		dc.Printf("ui events %s", onOff(debugUIEvents))
	case "state":
		dc.Printf("modal=%s capture=%s focus=%s",
			debugCtlName(ui.Modal), debugCtlName(ui.Capture), debugCtlName(ui.Focused))
		vis := 0
		var countVis func(c *UIControl)
		countVis = func(c *UIControl) {
			if c.Visible {
				vis++
			}
			for _, ch := range c.Children {
				countVis(ch)
			}
		}
		countVis(ui.Root)
		dc.Printf("root children=%d visible controls=%d", len(ui.Root.Children), vis)
	case "inspect":
		if len(args) < 2 {
			dc.Printf("usage: ui inspect <name>")
			return
		}
		dc.Print(ui.DebugInspect(strings.Join(args[1:], " ")))
	case "show":
		if len(args) < 2 {
			dc.Printf("usage: ui show <name>")
			return
		}
		c := ui.FindControl(strings.Join(args[1:], " "))
		if c == nil {
			dc.Printf("control %q not found", strings.Join(args[1:], " "))
			return
		}
		for p := c; p != nil; p = p.Parent {
			p.Visible = true
		}
		dc.Printf("%s -> visible (ancestors unhidden)", c.Name)
	case "hide":
		if len(args) < 2 {
			dc.Printf("usage: ui hide <name>")
			return
		}
		c := ui.FindControl(strings.Join(args[1:], " "))
		if c == nil {
			dc.Printf("control %q not found", strings.Join(args[1:], " "))
			return
		}
		c.Visible = false
		dc.Printf("%s -> hidden", c.Name)
	case "move":
		if len(args) < 4 {
			dc.Printf("usage: ui move <name> <x> <y>")
			return
		}
		name := strings.Join(args[1:len(args)-2], " ")
		x, err1 := strconv.Atoi(args[len(args)-2])
		y, err2 := strconv.Atoi(args[len(args)-1])
		if err1 != nil || err2 != nil {
			dc.Printf("usage: ui move <name> <x> <y>  (x,y must be integers)")
			return
		}
		c := ui.FindControl(name)
		if c == nil {
			dc.Printf("control %q not found", name)
			return
		}
		oldX, oldY := c.Left, c.Top
		c.Left, c.Top = x, y
		dc.Printf("%s moved (%d,%d) -> (%d,%d)", c.Name, oldX, oldY, x, y)
	case "click":
		if len(args) < 2 {
			dc.Printf("usage: ui click <name>")
			return
		}
		c := ui.FindControl(strings.Join(args[1:], " "))
		if c == nil {
			dc.Printf("control %q not found", strings.Join(args[1:], " "))
			return
		}
		w, h := c.effectiveSize()
		cx, cy := c.AbsX()+w/2, c.AbsY()+h/2
		ui.RouteMouseDown(cx, cy, 0)
		ui.RouteMouseUp(cx, cy, 0)
		dc.Printf("clicked %s at (%d,%d)", c.Name, cx, cy)
	case "hover":
		ui.ShowHoverInfo = !ui.ShowHoverInfo
		ui.ShowBounds = ui.ShowHoverInfo
		dc.Printf("ui hover %s", onOff(ui.ShowHoverInfo))
	case "list":
		filter := ""
		if len(args) > 1 {
			filter = args[1]
		}
		dc.Print(ui.DebugList(filter))
	default:
		dc.Printf("unknown ui subcommand: %s", args[0])
	}
}

func (dc *DebugConsole) cmdClick(args []string) {
	ui := gActiveUI
	if ui == nil {
		dc.Printf("no UI in current scene")
		return
	}
	if len(args) < 2 {
		dc.Printf("usage: click <x> <y> [right]")
		return
	}
	x, err1 := strconv.Atoi(args[0])
	y, err2 := strconv.Atoi(args[1])
	if err1 != nil || err2 != nil {
		dc.Printf("usage: click <x> <y> [right]")
		return
	}
	button := 0
	if len(args) >= 3 && args[2] == "right" {
		button = 1
	}
	consumed := ui.RouteMouseDown(x, y, button)
	ui.RouteMouseUp(x, y, button)
	dc.Printf("click (%d,%d) btn=%d consumed=%v", x, y, button, consumed)
}

func (dc *DebugConsole) cmdClickLog(args []string) {
	debugClickLog = !debugClickLog
	dc.Printf("clicklog %s", onOff(debugClickLog))
}

func (dc *DebugConsole) cmdDump(args []string) {
	log.Logf(log.LevelInfo, "ConsoleDump", "=== console output (%d lines) ===", len(dc.Lines))
	for _, line := range dc.Lines {
		log.Logf(log.LevelInfo, "ConsoleDump", "%s", line)
	}
	dc.Printf("dumped %d lines to log", len(dc.Lines))
}

// --- FPS 统计 ---

func (dc *DebugConsole) updateFPS() {
	now := time.Now()
	if !dc.lastFrame.IsZero() {
		if dt := now.Sub(dc.lastFrame).Seconds(); dt > 0 {
			inst := 1 / dt
			if dc.fps == 0 {
				dc.fps = inst
			} else {
				dc.fps = dc.fps*0.9 + inst*0.1
			}
		}
	}
	dc.lastFrame = now
}

// --- 渲染 ---

// Render 在场景之后绘制控制台 (面板或状态栏)。
func (dc *DebugConsole) Render(proj [16]float32) {
	dc.updateFPS()
	if dc.text == nil {
		return
	}
	if dc.Visible {
		dc.renderPanel(proj)
	} else if dc.ShowHUD || dc.WireMode > 0 {
		dc.renderStatusBar(proj)
	}
	if gActiveUI != nil && gActiveUI.ShowHoverInfo {
		dc.renderHoverInfo(proj)
	}
}

func (dc *DebugConsole) renderHoverInfo(proj [16]float32) {
	gActiveUI.RenderInteractiveOverlay(proj)
	c := gActiveUI.HoveredControl(int(dc.mouseX), int(dc.mouseY))
	if c == nil {
		return
	}
	// 高亮悬停组件
	w, h := c.effectiveSize()
	cx, cy := float32(c.AbsX()), float32(c.AbsY())
	cw, ch := float32(w), float32(h)
	dc.gl.DrawQuadColor(cx, cy, cw, ch, 0.3, 0.6, 1.0, 0.12, proj)
	drawWireRect(dc.gl, cx-1, cy-1, cw+2, ch+2, 0.3, 0.7, 1.0, 1.0, proj)
	// 高亮父级 panel
	if c.Parent != nil && c.Parent.Kind == KindWindow && c.Parent.Visible {
		pw, ph := c.Parent.effectiveSize()
		px, py := float32(c.Parent.AbsX()), float32(c.Parent.AbsY())
		drawWireRect(dc.gl, px-1, py-1, float32(pw)+2, float32(ph)+2, 0.3, 0.7, 1.0, 1.0, proj)
	}
	info := fmt.Sprintf("%s (%s) abs=(%d,%d) %dx%d", c.Name, c.Kind, c.AbsX(), c.AbsY(), w, h)
	dc.drawHoverLabel(proj, info)
}

func (dc *DebugConsole) drawHoverLabel(proj [16]float32, info string) {
	gl := dc.gl
	text := dc.text
	x := float32(dc.mouseX) + 12
	y := float32(dc.mouseY) + 12
	w := float32(text.MeasureText(info)) + 8
	lineH := float32(text.LineHeight())
	if lineH <= 0 {
		lineH = 14
	}
	if x+w > 800 {
		x = 800 - w
	}
	if y+lineH+4 > 600 {
		y = float32(dc.mouseY) - lineH - 8
	}
	gl.DrawQuadColor(x, y, w, lineH+4, 0, 0, 0, 0.8, proj)
	text.DrawText(info, x+4, y+2, 1, 1, 0, 1, proj)
}

func (dc *DebugConsole) renderPanel(proj [16]float32) {
	gl := dc.gl
	text := dc.text
	consoleH := float32(dc.panelH)
	baseY := float32(ScreenHeight) - consoleH
	gl.DrawQuadColor(0, baseY, 800, consoleH, 0, 0, 0, 0.75, proj)

	// 顶边拖拽手柄 (悬停时高亮)
	handleAlpha := float32(0.4)
	if dc.mouseY >= float64(baseY)-3 && dc.mouseY <= float64(baseY)+6 {
		handleAlpha = 0.9
	}
	gl.DrawQuadColor(0, baseY, 800, 3, 0.6, 0.6, 0.6, handleAlpha, proj)

	lineH := float32(text.LineHeight())
	if lineH <= 0 {
		lineH = 14
	}
	outputTop := baseY + 8
	outputH := consoleH - 40
	maxVisible := int(outputH / lineH)
	if maxVisible < 1 {
		maxVisible = 1
	}

	total := len(dc.Lines)
	endIdx := total - dc.ScrollOff
	if endIdx > total {
		endIdx = total
	}
	if endIdx < 0 {
		endIdx = 0
	}
	startIdx := endIdx - maxVisible
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < endIdx; i++ {
		y := outputTop + float32(i-startIdx)*lineH
		if dc.selStart >= 0 {
			lo, hi := dc.selStart, dc.selEnd
			if lo > hi {
				lo, hi = hi, lo
			}
			if i >= lo && i <= hi {
				gl.DrawQuadColor(0, y, 800, lineH, 0.2, 0.4, 0.8, 0.4, proj)
			}
		}
		line := dc.Lines[i]
		const maxTextW = 780
		if text.MeasureText(line) > maxTextW {
			runes := []rune(line)
			for len(runes) > 3 && text.MeasureText(string(runes)+"...") > maxTextW {
				runes = runes[:len(runes)-1]
			}
			line = string(runes) + "..."
		}
		text.DrawText(line, 8, y, 0.8, 0.8, 0.8, 1.0, proj)
	}

	// 滚动条 (细、渐显)
	now := time.Now().UnixMilli()
	mouseInOutput := dc.mouseY >= float64(outputTop) && dc.mouseY <= float64(outputTop+outputH)
	if mouseInOutput || now-dc.sbLastTick < 1500 {
		dc.sbAlpha += 0.15
		if dc.sbAlpha > 1 {
			dc.sbAlpha = 1
		}
	} else {
		dc.sbAlpha -= 0.05
		if dc.sbAlpha < 0 {
			dc.sbAlpha = 0
		}
	}
	if total > maxVisible && dc.sbAlpha > 0.01 {
		const sbW = float32(3)
		sbX := float32(795)
		trackTop := outputTop
		trackH := outputH
		gl.DrawQuadColor(sbX, trackTop, sbW, trackH, 0.4, 0.4, 0.4, 0.3*dc.sbAlpha, proj)
		thumbRatio := float32(maxVisible) / float32(total)
		thumbH := trackH * thumbRatio
		if thumbH < 12 {
			thumbH = 12
		}
		maxScroll := total - maxVisible
		scrollFrac := float32(0)
		if maxScroll > 0 {
			scrollFrac = float32(maxScroll-dc.ScrollOff) / float32(maxScroll)
		}
		thumbY := trackTop + (trackH-thumbH)*scrollFrac
		gl.DrawQuadColor(sbX, thumbY, sbW, thumbH, 0.8, 0.8, 0.8, 0.7*dc.sbAlpha, proj)
	}

	sepY := baseY + consoleH - 30
	gl.DrawQuadColor(0, sepY, 800, 1, 0.5, 0.5, 0.5, 0.5, proj)
	inputY := sepY + 6
	prompt := "> " + dc.Input
	text.DrawText(prompt, 8, inputY, 0, 1, 0, 1, proj)
	if time.Now().UnixMilli()%1000 < 500 {
		runes := []rune(dc.Input)
		cp := dc.cursorPos
		if cp > len(runes) {
			cp = len(runes)
		}
		beforePrompt := "> " + string(runes[:cp])
		cursorX := 8 + float32(text.MeasureText(beforePrompt))
		gl.DrawQuadColor(cursorX+1, inputY, 2, lineH, 0, 1, 0, 1, proj)
	}
}

func (dc *DebugConsole) renderStatusBar(proj [16]float32) {
	gl := dc.gl
	text := dc.text
	gl.DrawQuadColor(0, 0, 800, 14, 0, 0, 0, 0.6, proj)

	wireNames := []string{"-", "HOVER", "ALL"}
	parts := []string{fmt.Sprintf("WIRE:%s", wireNames[dc.WireMode])}
	if dc.ShowFPS {
		parts = append(parts, fmt.Sprintf("FPS:%.0f", dc.fps))
	}
	if dc.sceneMgr != nil {
		parts = append(parts, dc.sceneMgr.CurrentType().String())
	}
	if dc.HoverIdx >= 0 {
		parts = append(parts, fmt.Sprintf("hover:#%d", dc.HoverIdx))
	}
	if dc.LockIdx >= 0 {
		parts = append(parts, fmt.Sprintf("lock:#%d", dc.LockIdx))
	}
	if dc.StatusExtra != nil {
		if extra := dc.StatusExtra(); extra != "" {
			parts = append(parts, extra)
		}
	}
	status := strings.Join(parts, "  ")
	text.DrawText(status, 4, 1, 0.6, 0.8, 0.6, 1, proj)
}

// --- 屏幕空间线框 (非 PlayScene 场景) ---

// wireNoise 过滤全屏背景 (>90% 屏) 与细碎文字 quad (<8x8)。
func wireNoise(b [5]float32) bool {
	if b[2] > 720 && b[3] > 540 {
		return true
	}
	if b[2] < 8 || b[3] < 8 {
		return true
	}
	return false
}

// RenderWireOverlay 在屏幕空间 (OrthoProj 800x600) 绘制线框。
// 仅当场景未自行渲染线框 (wireHandled=false) 时由 main.go 调用。
func (dc *DebugConsole) RenderWireOverlay(proj [16]float32) {
	gl := dc.gl
	if dc.WireMode == wireOff {
		return
	}
	dc.updateHoverScreen()
	switch dc.WireMode {
	case wireHover:
		if dc.LockIdx >= 0 && dc.LockIdx < len(gl.WireBounds) {
			wb := gl.WireBounds[dc.LockIdx]
			c := catColor(wb[4])
			drawWireRect(gl, wb[0], wb[1], wb[2], wb[3], c[0]*1.5, c[1]*1.5, c[2]*1.5, 1, proj)
		}
		if dc.HoverIdx >= 0 && dc.HoverIdx < len(gl.WireBounds) && dc.HoverIdx != dc.LockIdx {
			wb := gl.WireBounds[dc.HoverIdx]
			c := catColor(wb[4])
			drawWireRect(gl, wb[0], wb[1], wb[2], wb[3], c[0], c[1], c[2], c[3], proj)
		}
	case wireAll:
		for _, wb := range gl.WireBounds {
			if wireNoise(wb) {
				continue
			}
			c := catColor(wb[4])
			drawWireRect(gl, wb[0], wb[1], wb[2], wb[3], c[0], c[1], c[2], c[3], proj)
		}
	}
}

func (dc *DebugConsole) updateHoverScreen() {
	dc.HoverIdx = -1
	if dc.WireMode == wireOff {
		return
	}
	mx, my := float32(dc.mouseX), float32(dc.mouseY)
	wb := dc.gl.WireBounds
	for i := len(wb) - 1; i >= 0; i-- {
		b := wb[i]
		if wireNoise(b) {
			continue
		}
		if mx >= b[0] && mx <= b[0]+b[2] && my >= b[1] && my <= b[1]+b[3] {
			dc.HoverIdx = i
			return
		}
	}
}

// ClickInspectScreen 处理屏幕空间的线框点击锁定。消费事件时返回 true。
func (dc *DebugConsole) ClickInspectScreen() bool {
	if dc.WireMode == wireOff {
		return false
	}
	if dc.HoverIdx >= 0 {
		if dc.LockIdx == dc.HoverIdx {
			dc.LockIdx = -1
			dc.Printf("unlocked")
		} else {
			dc.LockIdx = dc.HoverIdx
			wb := dc.gl.WireBounds[dc.LockIdx]
			dc.Printf("locked #%d %s (%.0f,%.0f) %.0fx%.0f",
				dc.LockIdx, catName(wb[4]), wb[0], wb[1], wb[2], wb[3])
		}
		return true
	}
	dc.LockIdx = -1
	return false
}

// --- 线框绘制辅助 ---

func drawWireRect(gl *engine.GLState, x, y, w, h, r, g, b, a float32, proj [16]float32) {
	const t = 2.0
	gl.DrawQuadColor(x, y, w, t, r, g, b, a, proj)
	gl.DrawQuadColor(x, y+h-t, w, t, r, g, b, a, proj)
	gl.DrawQuadColor(x, y, t, h, r, g, b, a, proj)
	gl.DrawQuadColor(x+w-t, y, t, h, r, g, b, a, proj)
}
