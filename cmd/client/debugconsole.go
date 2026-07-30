package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pyq0109/mirgo/internal/engine"
)

const (
	wireOff   = 0
	wireHover = 1
	wireAll   = 2
)

// gDebug 是全局控制台引用。由 main.go 在创建后设置,
// 供 UI 事件日志等无法直接持有控制台引用的代码使用。
var gDebug *DebugConsole

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

func (dc *DebugConsole) OnKey(key int, action int) {
	if action == 0 {
		return
	}
	switch key {
	case 257: // Enter
		if action == 1 {
			dc.execute(dc.Input)
			dc.Input = ""
		}
	case 259: // Backspace
		if dc.Input != "" {
			runes := []rune(dc.Input)
			dc.Input = string(runes[:len(runes)-1])
		}
	case 256: // Escape
		if action == 1 {
			dc.Visible = false
		}
	case 265: // Up — 历史上一条
		dc.histPrev()
	case 264: // Down — 历史下一条
		dc.histNext()
	case 266: // PageUp
		dc.ScrollOff += 5
		if max := len(dc.Lines) - 1; dc.ScrollOff > max {
			dc.ScrollOff = max
		}
		if dc.ScrollOff < 0 {
			dc.ScrollOff = 0
		}
	case 267: // PageDown
		dc.ScrollOff -= 5
		if dc.ScrollOff < 0 {
			dc.ScrollOff = 0
		}
	case 268: // Home
		if max := len(dc.Lines) - 1; max > 0 {
			dc.ScrollOff = max
		}
	case 269: // End
		dc.ScrollOff = 0
	}
}

func (dc *DebugConsole) OnChar(char rune) {
	if char == '`' {
		return
	}
	if char >= 32 && char != 127 {
		if utf8.RuneCountInString(dc.Input) < 120 {
			dc.Input += string(char)
		}
	}
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
}

func (dc *DebugConsole) cmdFPS(args []string) {
	dc.ShowFPS = !dc.ShowFPS
	dc.Printf("fps %s", onOff(dc.ShowFPS))
}

func (dc *DebugConsole) cmdHUD(args []string) {
	dc.ShowHUD = !dc.ShowHUD
	dc.Printf("hud %s", onOff(dc.ShowHUD))
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
		return
	}
	if dc.ShowHUD || dc.WireMode > 0 {
		dc.renderStatusBar(proj)
	}
}

func (dc *DebugConsole) renderPanel(proj [16]float32) {
	gl := dc.gl
	text := dc.text
	const consoleH = 220
	const baseY = float32(ScreenHeight - consoleH)
	gl.DrawQuadColor(0, baseY, 800, consoleH, 0, 0, 0, 0.75, proj)

	lineH := float32(text.LineHeight())
	if lineH <= 0 {
		lineH = 14
	}
	outputTop := baseY + 8
	maxVisible := int(180 / lineH)
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
		text.DrawText(dc.Lines[i], 8, y, 0.8, 0.8, 0.8, 1.0, proj)
	}

	gl.DrawQuadColor(0, baseY+190, 800, 1, 0.5, 0.5, 0.5, 0.5, proj)
	inputY := baseY + 196
	prompt := "> " + dc.Input
	text.DrawText(prompt, 8, inputY, 0, 1, 0, 1, proj)
	if time.Now().UnixMilli()%1000 < 500 {
		cursorX := 8 + float32(text.MeasureText(prompt))
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
