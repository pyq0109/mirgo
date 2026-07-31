package main

import (
	"fmt"
	"strings"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// 本文件集中了 PlayScene 专有的调试功能: 世界空间线框/网格/标签
// 渲染、点击检查、管线 dump, 以及向全局控制台注册的场景命令。
// 场景进入 (Open) 时注册命令并设置 StatusExtra, 离开 (Close) 时注销。

// registerDebugCmds 在场景变为活动时向全局控制台注册命令。
func (s *PlayScene) registerDebugCmds() {
	dc := s.dbg
	dc.Register("grid", "toggle tile grid overlay", func(args []string) {
		s.ShowGrid = !s.ShowGrid
		dc.Printf("grid %s", onOff(s.ShowGrid))
	})
	dc.Register("label", "toggle actor #ID type name", func(args []string) {
		s.ShowLabel = !s.ShowLabel
		dc.Printf("label %s", onOff(s.ShowLabel))
	})
	dc.Register("light", "toggle lighting/fog off", func(args []string) {
		s.DisableLight = !s.DisableLight
		dc.Printf("light %s", map[bool]string{true: "disabled", false: "enabled"}[s.DisableLight])
	})
	dc.Register("hpbar", "toggle HP bar off", func(args []string) {
		s.DisableHPBar = !s.DisableHPBar
		dc.Printf("hpbar %s", map[bool]string{true: "disabled", false: "enabled"}[s.DisableHPBar])
	})
	dc.Register("kill", "kill all: remove monsters & NPCs (client)", func(args []string) {
		if len(args) >= 1 && args[0] == "all" {
			dc.Printf("kill all: removed %d", s.killAll())
		} else {
			dc.Printf("usage: kill all")
		}
	})
	dc.Register("nomob", "stop server spawning + kill all", func(args []string) {
		if s.sendChat != nil {
			s.sendChat("@nomob")
			dc.Printf("sent @nomob to server")
		}
	})
	dc.Register("panel", "panel <name> [on|off] — toggle UI panel", func(args []string) {
		s.cmdPanel(args)
	})
	dc.Register("key", "key <name> — simulate shortcut (b/c/m/enter/esc/f1-f12/1-6)", func(args []string) {
		s.cmdKey(args)
	})
	dc.Register("itemmove", "itemmove [reset] — show/reset item drag state", func(args []string) {
		s.cmdItemMove(args)
	})
	dc.StatusExtra = s.debugStatusExtra
}

// unregisterDebugCmds 在场景变为非活动时注销命令。
func (s *PlayScene) unregisterDebugCmds() {
	dc := s.dbg
	for _, name := range []string{"grid", "label", "light", "hpbar", "kill", "nomob", "panel", "key", "itemmove"} {
		dc.Unregister(name)
	}
	dc.StatusExtra = nil
}

// debugStatusExtra 为全局状态栏补充 PlayScene 专有信息。
func (s *PlayScene) debugStatusExtra() string {
	var parts []string
	if s.ShowGrid {
		parts = append(parts, "GRID")
	}
	if s.ShowLabel {
		parts = append(parts, "LABEL")
	}
	if s.DisableLight {
		parts = append(parts, "NO-LIGHT")
	}
	if s.DisableHPBar {
		parts = append(parts, "NO-HPBAR")
	}
	if s.ui != nil && s.ui.ShowBounds {
		parts = append(parts, "UI-BOUNDS")
	}
	if s.State.Actors != nil {
		parts = append(parts, fmt.Sprintf("%d actors", len(s.State.Actors.All())))
	}
	if s.cam != nil {
		tx, ty := s.cam.WorldToTile(
			s.cam.X+float64(s.cam.ViewW)/(2*s.cam.Zoom),
			s.cam.Y+float64(s.cam.ViewH)/(2*s.cam.Zoom))
		parts = append(parts, fmt.Sprintf("(%d,%d)", tx, ty))
	}
	return strings.Join(parts, " ")
}

// cmdPanel 操作 GameState 中的面板可见性标志。
func (s *PlayScene) cmdPanel(args []string) {
	dc := s.dbg
	if len(args) == 0 {
		dc.Printf("usage: panel <name> [on|off]")
		dc.Printf("  names: bag state guild group friend abil npc shop deal minimap")
		return
	}
	name := strings.ToLower(args[0])
	var flag *bool
	switch name {
	case "bag":
		flag = &s.State.ShowBag
	case "state", "equip":
		flag = &s.State.ShowEquip
	case "guild":
		flag = &s.State.ShowGuild
	case "group":
		flag = &s.State.ShowGroupDlg
	case "friend":
		flag = &s.State.ShowFriend
	case "abil":
		flag = &s.State.ShowPlusAbil
	case "npc":
		flag = &s.State.ShowNpcDialog
	case "shop":
		flag = &s.State.ShowShop
	case "deal":
		flag = &s.State.InDeal
	case "minimap":
		flag = &s.showMinimap
	default:
		dc.Printf("unknown panel: %s", name)
		return
	}
	if len(args) >= 2 {
		switch strings.ToLower(args[1]) {
		case "on":
			*flag = true
		case "off":
			*flag = false
		}
	} else {
		*flag = !*flag
	}
	dc.Printf("panel %s = %s", name, onOff(*flag))
}

// cmdKey 模拟键盘快捷键。
func (s *PlayScene) cmdKey(args []string) {
	dc := s.dbg
	if len(args) == 0 {
		dc.Printf("usage: key <name>  (b c e g m n s v w enter esc f1-f12 1-6)")
		return
	}
	keyMap := map[string]int{
		"b": 66, "c": 67, "e": 69, "g": 71, "m": 77,
		"n": 78, "s": 83, "v": 86, "w": 87, "z": 90,
		"enter": 257, "esc": 256,
		"f1": 290, "f2": 291, "f3": 292, "f4": 293,
		"f5": 294, "f6": 295, "f7": 296, "f8": 297,
		"f9": 298, "f10": 299, "f11": 300, "f12": 301,
		"1": 49, "2": 50, "3": 51, "4": 52, "5": 53, "6": 54,
	}
	name := strings.ToLower(args[0])
	code, ok := keyMap[name]
	if !ok {
		dc.Printf("unknown key: %s", name)
		return
	}
	s.OnKey(code, 1)
	dc.Printf("key %s -> OnKey(%d)", name, code)
}

// cmdItemMove 显示或重置物品拖拽状态。
func (s *PlayScene) cmdItemMove(args []string) {
	dc := s.dbg
	m := &s.itemMove
	if len(args) >= 1 && args[0] == "reset" {
		m.End()
		dc.Printf("itemmove reset")
		return
	}
	if !m.Moving {
		dc.Printf("itemmove: idle (not moving)")
		return
	}
	var src string
	switch {
	case m.Index >= 0:
		src = fmt.Sprintf("bag[%d]", m.Index)
	case m.Index == -97:
		src = "deal-gold"
	case m.Index == -98:
		src = "bag-gold"
	case m.Index == -99:
		src = "sell-spot"
	case m.Index <= -20 && m.Index > -30:
		src = fmt.Sprintf("deal[%d]", -m.Index-20)
	default:
		src = fmt.Sprintf("equip[%d]", -(m.Index + 1))
	}
	itemName := fmt.Sprintf("idx=%d", m.Item.Idx)
	if m.Item.Def != nil {
		itemName = m.Item.Def.Name
	}
	dc.Printf("itemmove: moving src=%s item=%s belt=%d waitSlot=%d",
		src, itemName, m.FromBelt, m.WaitSlot)
}

// --- 悬停检测 (每帧, 世界空间) ---

func (s *PlayScene) updateHover() {
	dc := s.dbg
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

// --- 线框渲染 (世界视口) ---

func (s *PlayScene) renderWireframes(proj [16]float32) {
	dc := s.dbg
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

// --- 悬停浮空信息 (世界视口) ---

func (s *PlayScene) renderHoverInfo(proj [16]float32) {
	dc := s.dbg
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

// --- 点击锁定 (世界空间) ---

func (s *PlayScene) clickInspect(sx, sy float64) bool {
	dc := s.dbg
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
			s.dumpBound(dc.LockIdx)
		}
		return true
	}

	dc.LockIdx = -1
	return false
}

func (s *PlayScene) dumpBound(idx int) {
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
				s.dumpPipeline(a, bounds, camX, camY, zoom)
				break
			}
		}
	}
	if found == 0 {
		log.Logf(log.LevelInfo, "DebugConsole", "  no actor at this bound — likely map object / effect / item")
	}
	log.Logf(log.LevelInfo, "DebugConsole", "=== END ===")
}

// dumpPipeline 将 actor 各渲染层信息写入日志。
func (s *PlayScene) dumpPipeline(a *Actor, bounds []LayerBounds, camX, camY, zoom float32) {
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

// --- 瓦片网格 (世界视口) ---

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

// --- actor 标签 (世界视口) ---

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

func (s *PlayScene) killAll() int {
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
