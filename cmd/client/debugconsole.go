package main

import (
	"fmt"
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

type DebugConsole struct {
	Visible   bool
	WireMode  int // 0=off, 1=hover, 2=all
	ShowGrid   bool
	ShowLabel  bool
	ShowHUD    bool
	DisableLight   bool
	DisableHPBar   bool

	HoverIdx int // hovered WireBounds index (-1=none)
	LockIdx  int // locked/selected WireBounds index (-1=none)

	Lines     []string
	ScrollOff int
	Input     string
	scene     *PlayScene
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

// --- 控制台输入 ---

func (dc *DebugConsole) Printf(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	dc.Lines = append(dc.Lines, line)
	if len(dc.Lines) > 200 {
		dc.Lines = dc.Lines[len(dc.Lines)-200:]
	}
	dc.ScrollOff = 0
}

func (dc *DebugConsole) OnKey(key int, action int) {
	if action == 0 {
		return
	}
	switch key {
	case 257:
		if action == 1 {
			dc.execute(dc.Input)
			dc.Input = ""
		}
	case 259:
		if dc.Input != "" {
			runes := []rune(dc.Input)
			dc.Input = string(runes[:len(runes)-1])
		}
	case 256:
		if action == 1 {
			dc.Visible = false
		}
	case 266:
		dc.ScrollOff += 5
		if max := len(dc.Lines) - 1; dc.ScrollOff > max {
			dc.ScrollOff = max
		}
		if dc.ScrollOff < 0 {
			dc.ScrollOff = 0
		}
	case 267:
		dc.ScrollOff -= 5
		if dc.ScrollOff < 0 {
			dc.ScrollOff = 0
		}
	case 268:
		if max := len(dc.Lines) - 1; max > 0 {
			dc.ScrollOff = max
		}
	case 269:
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

// --- 指令解析 ---

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (dc *DebugConsole) execute(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	dc.Printf("> %s", input)
	parts := strings.Fields(input)

	switch parts[0] {
	case "wire":
		if len(parts) == 1 {
			if dc.WireMode == wireHover {
				dc.WireMode = wireOff
			} else {
				dc.WireMode = wireHover
			}
			dc.LockIdx = -1
		} else {
			switch parts[1] {
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

	case "grid":
		dc.ShowGrid = !dc.ShowGrid
		dc.Printf("grid %s", onOff(dc.ShowGrid))

	case "label":
		dc.ShowLabel = !dc.ShowLabel
		dc.Printf("label %s", onOff(dc.ShowLabel))

	case "hud":
		dc.ShowHUD = !dc.ShowHUD
		dc.Printf("hud %s", onOff(dc.ShowHUD))

	case "light":
		dc.DisableLight = !dc.DisableLight
		dc.Printf("light %s", map[bool]string{true: "disabled", false: "enabled"}[dc.DisableLight])

	case "hpbar":
		dc.DisableHPBar = !dc.DisableHPBar
		dc.Printf("hpbar %s", map[bool]string{true: "disabled", false: "enabled"}[dc.DisableHPBar])

	case "kill":
		if len(parts) >= 2 && parts[1] == "all" && dc.scene != nil {
			dc.Printf("kill all: removed %d", dc.killAll(dc.scene))
		} else {
			dc.Printf("usage: kill all")
		}

	case "nomob":
		if dc.scene != nil && dc.scene.sendChat != nil {
			dc.scene.sendChat("@nomob")
			dc.Printf("sent @nomob to server")
		}

	case "help":
		dc.Printf("=== debug commands ===")
		dc.Printf("  wire       hover wireframe (toggle)")
		dc.Printf("  wire all   all objects color-coded")
		dc.Printf("  wire 0     off")
		dc.Printf("  grid       tile grid overlay")
		dc.Printf("  label      actor #ID type name")
		dc.Printf("  hud        debug status bar")
		dc.Printf("  light      toggle lighting/fog off")
		dc.Printf("  hpbar      toggle HP bar off")
		dc.Printf("  kill all   remove monsters & NPCs (client)")
		dc.Printf("  nomob      stop server spawning + kill all")

	default:
		dc.Printf("unknown: %s  (help)", parts[0])
	}
}

// --- 控制台 UI 渲染 ---

func (dc *DebugConsole) Render(glState *engine.GLState, text *engine.TextRenderer, uiProj [16]float32) {
	if text == nil {
		return
	}
	const consoleH = 220
	const baseY = float32(ScreenHeight - consoleH)
	glState.DrawQuadColor(0, baseY, 800, consoleH, 0, 0, 0, 0.75, uiProj)

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
		text.DrawText(dc.Lines[i], 8, y, 0.8, 0.8, 0.8, 1.0, uiProj)
	}

	glState.DrawQuadColor(0, baseY+190, 800, 1, 0.5, 0.5, 0.5, 0.5, uiProj)
	inputY := baseY + 196
	prompt := "> " + dc.Input
	text.DrawText(prompt, 8, inputY, 0, 1, 0, 1, uiProj)
	if time.Now().UnixMilli()%1000 < 500 {
		cursorX := 8 + float32(text.MeasureText(prompt))
		glState.DrawQuadColor(cursorX+1, inputY, 2, lineH, 0, 1, 0, 1, uiProj)
	}
}

// --- 线框绘制辅助 ---

func drawWireRect(gl *engine.GLState, x, y, w, h, r, g, b, a float32, proj [16]float32) {
	const t = 2.0
	gl.DrawQuadColor(x, y, w, t, r, g, b, a, proj)
	gl.DrawQuadColor(x, y+h-t, w, t, r, g, b, a, proj)
	gl.DrawQuadColor(x, y, t, h, r, g, b, a, proj)
	gl.DrawQuadColor(x+w-t, y, t, h, r, g, b, a, proj)
}

// --- 悬停检测（每帧调用）---

func (dc *DebugConsole) UpdateHover(s *PlayScene) {
	dc.HoverIdx = -1
	if dc.WireMode == wireOff {
		return
	}
	if s.mouseY >= float64(MapSurfaceH) {
		return
	}
	wx, wy := s.cam.ScreenToWorld(s.mouseX, s.mouseY)
	wxf, wyf := float32(wx), float32(wy)
	for i := len(s.gl.WireBounds) - 1; i >= 0; i-- {
		wb := s.gl.WireBounds[i]
		if wxf >= wb[0] && wxf <= wb[0]+wb[2] && wyf >= wb[1] && wyf <= wb[1]+wb[3] {
			dc.HoverIdx = i
			return
		}
	}
}

// --- 线框渲染（世界视口）---

func (s *PlayScene) renderWireframes(proj [16]float32) {
	dc := &s.debugConsole
	switch dc.WireMode {
	case wireHover:
		if dc.LockIdx >= 0 && dc.LockIdx < len(s.gl.WireBounds) {
			wb := s.gl.WireBounds[dc.LockIdx]
			c := catColor(wb[4])
			drawWireRect(s.gl, wb[0], wb[1], wb[2], wb[3], c[0]*1.5, c[1]*1.5, c[2]*1.5, 1, proj)
		}
		if dc.HoverIdx >= 0 && dc.HoverIdx < len(s.gl.WireBounds) && dc.HoverIdx != dc.LockIdx {
			wb := s.gl.WireBounds[dc.HoverIdx]
			c := catColor(wb[4])
			drawWireRect(s.gl, wb[0], wb[1], wb[2], wb[3], c[0], c[1], c[2], c[3], proj)
		}
	case wireAll:
		for _, wb := range s.gl.WireBounds {
			c := catColor(wb[4])
			drawWireRect(s.gl, wb[0], wb[1], wb[2], wb[3], c[0], c[1], c[2], c[3], proj)
		}
	}
}

// --- 悬停浮空信息（世界视口）---

func (s *PlayScene) renderHoverInfo(proj [16]float32) {
	dc := &s.debugConsole
	idx := dc.HoverIdx
	if idx < 0 {
		idx = dc.LockIdx
	}
	if idx < 0 || idx >= len(s.gl.WireBounds) || s.text == nil {
		return
	}
	wb := s.gl.WireBounds[idx]
	cat := wb[4]
	info := fmt.Sprintf("#%d %s (%.0f,%.0f) %.0fx%.0f", idx, catName(cat), wb[0], wb[1], wb[2], wb[3])

	camX := float32(s.cam.X)
	camY := float32(s.cam.Y)
	zoom := float32(s.cam.Zoom)
	sx := (wb[0]-camX)*zoom + (wb[2]*zoom)/2
	sy := (wb[1]-camY)*zoom - 4

	lw := float32(s.text.MeasureText(info))
	sx -= lw / 2

	s.gl.DrawQuadColor(sx-2, sy-1, lw+4, float32(s.text.LineHeight())+2, 0, 0, 0, 0.7, proj)
	c := catColor(cat)
	s.text.DrawText(info, sx, sy, c[0], c[1], c[2], 1, proj)
}

// --- 点击锁定 ---

func (dc *DebugConsole) ClickInspect(s *PlayScene, sx, sy float64) bool {
	if sy >= float64(ScreenHeight-220) {
		return false
	}
	if sy >= float64(MapSurfaceH) {
		return false
	}
	if dc.WireMode == wireOff {
		return false
	}

	if dc.HoverIdx >= 0 {
		if dc.LockIdx == dc.HoverIdx {
			dc.LockIdx = -1
			dc.Printf("unlocked")
		} else {
			dc.LockIdx = dc.HoverIdx
			wb := s.gl.WireBounds[dc.LockIdx]
			dc.Printf("locked #%d %s", dc.LockIdx, catName(wb[4]))
			dc.dumpBound(s, dc.LockIdx)
		}
		return true
	}

	dc.LockIdx = -1
	return false
}

func (dc *DebugConsole) dumpBound(s *PlayScene, idx int) {
	wb := s.gl.WireBounds[idx]
	camX := float32(s.cam.X)
	camY := float32(s.cam.Y)
	zoom := float32(s.cam.Zoom)
	screenX := (wb[0] - camX) * zoom
	screenY := (wb[1] - camY) * zoom

	log.Logf(log.LevelInfo, "DebugConsole", "=== BOUND #%d ===  cat=%s  world=(%.0f,%.0f,%.0f,%.0f)  screen=(%.0f,%.0f)",
		idx, catName(wb[4]), wb[0], wb[1], wb[2], wb[3], screenX, screenY)

	wxf, wyf := wb[0]+wb[2]/2, wb[1]+wb[3]/2
	found := 0
	for _, a := range s.State.Actors.All() {
		worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
		worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
		bounds := a.ComputeLayerBounds(s.resources, worldX, worldY)
		for _, lb := range bounds {
			lx, ly := lb.DrawX, lb.DrawY
			lw, lh := float32(lb.Width), float32(lb.Height)
			if wxf >= lx && wxf <= lx+lw && wyf >= ly && wyf <= ly+lh {
				found++
				dc.DumpPipeline(s, a, bounds, worldX, worldY)
				break
			}
		}
	}
	if found == 0 {
		log.Logf(log.LevelInfo, "DebugConsole", "  no actor at this bound — likely map object / effect / item")
	}
	log.Logf(log.LevelInfo, "DebugConsole", "=== END ===")
}

// --- 管线 dump（仅日志）---

func (dc *DebugConsole) DumpPipeline(s *PlayScene, a *Actor, bounds []LayerBounds, worldX, worldY float32) {
	camX := float32(s.cam.X)
	camY := float32(s.cam.Y)
	zoom := float32(s.cam.Zoom)
	log.Logf(log.LevelInfo, "DebugConsole", "  ACTOR #%d %q %s Dir=%d Frame=%d State=0x%08X Rx=%d Ry=%d",
		a.RecogID, a.UserName, actorTypeName(a.Type), a.Dir, a.CurrentFrame, a.State, a.Rx, a.Ry)
	for _, lb := range bounds {
		screenX := (lb.DrawX - camX) * zoom
		screenY := (lb.DrawY - camY) * zoom
		t, ob, oo := ComputeAlphaStats(lb.Img)
		log.Logf(log.LevelInfo, "DebugConsole", "    [%s] %s[%d] hot=(%d,%d) %dx%d draw=(%.0f,%.0f) scr=(%.0f,%.0f) tex=%d T=%d B=%d O=%d",
			lb.LayerName, lb.WilName, lb.ImageIdx, lb.HotX, lb.HotY, lb.Width, lb.Height,
			lb.DrawX, lb.DrawY, screenX, screenY, lb.TexID, t, ob, oo)
	}
}

// --- 状态栏（UI 视口底部一行）---

func (dc *DebugConsole) RenderStatusBar(s *PlayScene, glState *engine.GLState, text *engine.TextRenderer, uiProj [16]float32) {
	if text == nil {
		return
	}
	glState.DrawQuadColor(0, 0, 800, 14, 0, 0, 0, 0.6, uiProj)

	wireNames := []string{"-", "HOVER", "ALL"}
	parts := []string{fmt.Sprintf("WIRE:%s", wireNames[dc.WireMode])}
	if dc.ShowGrid {
		parts = append(parts, "GRID")
	}
	if dc.ShowLabel {
		parts = append(parts, "LABEL")
	}
	if dc.DisableLight {
		parts = append(parts, "NO-LIGHT")
	}
	if dc.DisableHPBar {
		parts = append(parts, "NO-HPBAR")
	}
	if s.State.Actors != nil {
		parts = append(parts, fmt.Sprintf("%d actors", len(s.State.Actors.All())))
	}
	if s.cam != nil {
		tx, ty := s.cam.WorldToTile(s.cam.X+float64(s.cam.ViewW)/(2*s.cam.Zoom), s.cam.Y+float64(s.cam.ViewH)/(2*s.cam.Zoom))
		parts = append(parts, fmt.Sprintf("(%d,%d)", tx, ty))
	}
	if dc.HoverIdx >= 0 {
		parts = append(parts, fmt.Sprintf("hover:#%d", dc.HoverIdx))
	}
	if dc.LockIdx >= 0 {
		parts = append(parts, fmt.Sprintf("lock:#%d", dc.LockIdx))
	}
	status := strings.Join(parts, "  ")
	text.DrawText(status, 4, 1, 0.6, 0.8, 0.6, 1, uiProj)
}

// --- 瓦片网格（世界视口）---

func (s *PlayScene) renderDebugGrid(proj [16]float32) {
	cam := s.cam
	wx0, wy0 := cam.ScreenToWorld(0, 0)
	wx1, wy1 := cam.ScreenToWorld(float64(cam.ViewW), float64(cam.ViewH))
	tx0, ty0 := cam.WorldToTile(wx0, wy0)
	tx1, ty1 := cam.WorldToTile(wx1, wy1)

	const r, g, b, a = 0.3, 0.3, 0.3, 0.4
	for tx := tx0; tx <= tx1+1; tx++ {
		x := float32(tx * engine.TileWidth)
		s.gl.DrawQuadColor(x, float32(wy0), 1, float32(wy1-wy0), r, g, b, a, proj)
	}
	for ty := ty0; ty <= ty1+1; ty++ {
		y := float32(ty * engine.TileHeight)
		s.gl.DrawQuadColor(float32(wx0), y, float32(wx1-wx0), 1, r, g, b, a, proj)
	}
}

// --- actor 标签（世界视口）---

func (s *PlayScene) renderDebugInfo(proj [16]float32) {
	if s.text == nil {
		return
	}
	for _, a := range s.State.Actors.All() {
		worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
		worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
		label := fmt.Sprintf("#%d %s", a.RecogID, actorTypeName(a.Type))
		if a.UserName != "" {
			label = fmt.Sprintf("#%d %s %s", a.RecogID, actorTypeName(a.Type), a.UserName)
		}
		lw := float32(s.text.MeasureText(label))
		lx := worldX + float32(engine.TileWidth)/2 - lw/2
		ly := worldY - 60
		s.text.DrawText(label, lx, ly, 1, 1, 0, 1, proj)
	}
}

// --- kill all ---

func (dc *DebugConsole) killAll(s *PlayScene) int {
	if s.State.Actors == nil {
		return 0
	}
	all := s.State.Actors.All()
	count := 0
	for _, a := range all {
		if a.IsSelf {
			continue
		}
		if a.Type == ActorMonster || a.Type == ActorNPC {
			s.State.Actors.Remove(a.RecogID)
			count++
		}
	}
	return count
}

func actorTypeName(t ActorType) string {
	switch t {
	case ActorHuman:
		return "H"
	case ActorMonster:
		return "M"
	case ActorNPC:
		return "N"
	default:
		return "?"
	}
}
