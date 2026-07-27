package main

import (
	"encoding/binary"
	"fmt"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// Adjust-ability panel — port of DAdjustAbility (FState.pas:1557-1619
// layout, 6510-6710 behavior) — plus the read-only inspect window
// DUserState1 (:1088-1163).

// Row order matches Delphi AdjustAbilHints / TNakedAbility fields.
var abilStatNames = [9]string{"DC", "MC", "SC", "AC", "MAC", "HP", "MP", "Hit", "Speed"}
var abilRowTops = [9]int{101, 121, 140, 160, 181, 201, 220, 240, 261}

// Per-row hover hints (Delphi AdjustAbilHints, FState:18-28).
var abilHints = [9]string{
	"攻击力(DC)", "魔法力(MC)", "道术(SC)", "防御力(AC)", "魔法防御(MAC)",
	"生命值(HP)", "魔法值(MP)", "准确(Hit)", "敏捷(Speed)",
}

// DUserState1 has its own slot layout, distinct from DStateWin
// (FState:1095-1158).
var inspectSlots = []stateSlotDef{
	{protocol.UNecklace, "项链", 168, 87, 34, 31},
	{protocol.UHelmet, "头盔", 115, 93, 18, 18},
	{protocol.URightHand, "火炬", 168, 125, 34, 31},
	{protocol.UArmRingR, "右手镯", 42, 176, 34, 31},
	{protocol.UArmRingL, "左手镯", 168, 176, 34, 31},
	{protocol.URingR, "右戒指", 42, 215, 34, 31},
	{protocol.URingL, "左戒指", 168, 215, 34, 31},
	{protocol.UWeapon, "武器", 47, 80, 47, 87},
	{protocol.UDress, "衣服", 96, 122, 53, 112},
	{protocol.UBujuk, "护身符", 42, 254, 34, 31},
	{protocol.UBelt, "腰带", 84, 254, 34, 31},
	{protocol.UBoots, "鞋", 126, 254, 34, 31},
	{protocol.UCharm, "魅力石", 168, 254, 34, 31},
}

func (s *PlayScene) buildAbilPanel() {
	ui := s.ui
	prg := s.resources.Prguse

	win := NewUIControl("DAdjustAbility", KindWindow)
	win.Floating = false // DFM: not draggable
	if prg != nil {
		win.SetImgIndex(prg, ImgAdjustBg)
	} else {
		win.Width, win.Height = 300, 340
	}
	win.Left, win.Top = 0, 0
	win.Visible = false
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintAbilPanel(c, proj) }
	// Row hover hints (DAdjustAbilityMouseMove, FState:6712-6737).
	win.OnMouseMove = func(c *UIControl, x, y int) {
		lx, ly := x, y
		if lx < 50 || lx >= 150 {
			return
		}
		for i, top := range abilRowTops {
			if ly >= top-3 && ly < top-3+20 {
				s.tooltip.Show(c.AbsX()+lx+10, c.AbsY()+ly+5, abilHints[i], [4]float32{1, 1, 1, 1}, false)
				return
			}
		}
	}
	ui.Root.AddChild(win)
	s.hudAbil = win

	for i := 0; i < 9; i++ {
		stat := i
		plus := NewUIControl("DPlus", KindButton)
		plus.Left, plus.Top = 217, abilRowTops[i]
		if prg != nil {
			plus.SetImgIndex(prg, ImgAdjustPlus)
		} else {
			plus.Width, plus.Height = 10, 12
		}
		plus.OnClick = func(c *UIControl, x, y int) { s.abilAdjust(stat, +1) }
		win.AddChild(plus)

		minus := NewUIControl("DMinus", KindButton)
		minus.Left, minus.Top = 227, abilRowTops[i]
		if prg != nil {
			minus.SetImgIndex(prg, ImgAdjustMinus)
		} else {
			minus.Width, minus.Height = 10, 12
		}
		minus.OnClick = func(c *UIControl, x, y int) { s.abilAdjust(stat, -1) }
		win.AddChild(minus)
	}

	ok := NewUIControl("DAdjustAbilOk", KindButton)
	ok.Left, ok.Top = 255, 301
	if prg != nil {
		ok.SetImgIndex(prg, ImgAdjustOk)
	}
	ok.OnClick = func(c *UIControl, x, y int) { s.abilCommit() }
	win.AddChild(ok)

	closeBtn := NewUIControl("DAdjustAbilClose", KindButton)
	closeBtn.Left, closeBtn.Top = 282, 19
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseSmall)
	}
	closeBtn.Width, closeBtn.Height = 14, 20
	closeBtn.OnClick = func(c *UIControl, x, y int) {
		// Closing discards unspent allocation (FState:6504-6508).
		s.abilDeltas = [9]int{}
		s.abilPointsLeft = s.State.BonusPoint
		s.State.ShowPlusAbil = false
	}
	win.AddChild(closeBtn)

	// Inspect window (DUserState1 [370], read-only equipment view).
	inspect := NewUIControl("DUserState1", KindWindow)
	inspect.Floating = false // DFM: not draggable
	if prg != nil {
		inspect.SetImgIndex(prg, ImgStateBg)
	} else {
		inspect.Width, inspect.Height = 240, 300
	}
	inspect.Left = ScreenWidth - 2*inspect.Width
	inspect.Top = 0
	inspect.Visible = false
	inspect.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintInspect(c, proj) }
	// Slot hover tooltips (FState:5995-6036).
	inspect.OnMouseMove = func(c *UIControl, x, y int) {
		for _, def := range inspectSlots {
			if x < def.x || x >= def.x+def.w || y < def.y || y >= def.y+def.h {
				continue
			}
			item := s.inspectItems[def.slot]
			if item == nil {
				return
			}
			bag := &BagItem{
				Idx:       item.WIndex,
				Dura:      item.Dura,
				DuraMax:   item.DuraMax,
				MakeIndex: item.MakeIndex,
				Def:       s.State.ItemDefs[int(item.WIndex)],
			}
			text, _ := GetMouseItemInfo(s.State, bag)
			color := [4]float32{1, 1, 1, 1}
			if item.DuraMax > 0 && item.Dura == 0 {
				color = [4]float32{1, 0, 0, 1}
			}
			s.tooltip.Show(c.AbsX()+def.x-30, c.AbsY()+def.y+50, text, color, false)
			return
		}
	}
	ui.Root.AddChild(inspect)
	s.hudInspect = inspect

	inspectClose := NewUIControl("DCloseUS1", KindButton)
	inspectClose.Left, inspectClose.Top = 20, 223
	if prg != nil {
		inspectClose.SetImgIndex(prg, ImgCloseSmall)
	}
	inspectClose.Width, inspectClose.Height = 14, 20
	inspectClose.OnClick = func(c *UIControl, x, y int) { s.showInspect = false }
	inspect.AddChild(inspectClose)
}

func (s *PlayScene) syncAbilWindows() {
	if s.hudAbil != nil {
		if s.State.ShowPlusAbil && !s.hudAbil.Visible {
			// Opening snapshots the point budget (g_nSaveBonusPoint).
			s.abilDeltas = [9]int{}
			s.abilPointsLeft = s.State.BonusPoint
		}
		s.hudAbil.Visible = s.State.ShowPlusAbil
	}
	if s.hudInspect != nil {
		s.hudInspect.Visible = s.showInspect
	}
}

// abilAdjust moves points between the budget and one stat (FState:6633-6704):
// holding Ctrl steps by 10 when enough points remain (:6638,6657).
func (s *PlayScene) abilAdjust(stat, delta int) {
	step := 1
	if s.ctrlDown && s.abilPointsLeft >= 10 {
		step = 10
	}
	if delta > 0 {
		if s.abilPointsLeft <= 0 {
			return
		}
		if step > s.abilPointsLeft {
			step = s.abilPointsLeft
		}
		s.abilDeltas[stat] += step
		s.abilPointsLeft -= step
		return
	}
	if s.abilDeltas[stat] <= 0 {
		return
	}
	if step > s.abilDeltas[stat] {
		step = s.abilDeltas[stat]
	}
	s.abilDeltas[stat] -= step
	s.abilPointsLeft += step
}

// abilCommit sends the allocation (Delphi SendAdjustBonus: Recog =
// remaining points, body = TNakedAbility).
func (s *PlayScene) abilCommit() {
	if s.sendAdjustBonus != nil {
		s.sendAdjustBonus(s.abilPointsLeft, s.abilDeltas)
	}
	s.abilDeltas = [9]int{}
	s.abilPointsLeft = s.State.BonusPoint
	s.State.ShowPlusAbil = false
}

// paintAbilPanel renders the three columns (FState:6510-6631): current
// values, remaining points, and pending allocation.
func (s *PlayScene) paintAbilPanel(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgAdjustBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	st := s.State
	l := c.AbsX() + 100
	m := c.AbsY() + 101
	rowY := [9]int{m - 4, m + 16, m + 36, m + 56, m + 76, m + 96, m + 116, m + 136, m + 156}

	// Header hints (FState:6541-6547), silver, 14px apart.
	hints := [4]string{
		"恭喜! 你的等级提升了.",
		"你可以将获得的点数分配到下面的属性中.",
		"分配完成后按确定按钮生效.",
		"按住 Ctrl 可以每次分配 10 点.",
	}
	for i, h := range hints {
		s.text.DrawText(h, float32(c.AbsX()+36), float32(c.AbsY()+22+i*14), 0.75, 0.75, 0.75, 1, proj)
	}

	values := [9]string{
		loHiStr(st.DC), loHiStr(st.MC), loHiStr(st.SC), loHiStr(st.AC), loHiStr(st.MAC),
		fmt.Sprintf("%d/%d", st.HP, st.MaxHP),
		fmt.Sprintf("%d/%d", st.MP, st.MaxMP),
		fmt.Sprintf("%d", st.Hit),
		fmt.Sprintf("%d", st.Speed),
	}
	for i := 0; i < 9; i++ {
		s.text.DrawText(values[i], float32(l), float32(rowY[i]), 1, 1, 1, 1, proj)
		if s.abilDeltas[i] > 0 {
			s.text.DrawText("+"+fmt.Sprintf("%d", s.abilDeltas[i]), float32(c.AbsX()+195), float32(rowY[i]), 1, 1, 0.4, 1, proj)
		}
	}
	s.text.DrawText(fmt.Sprintf("%d", s.abilPointsLeft), float32(l), float32(m+177), 1, 1, 0, 1, proj)
}

func loHiStr(v uint32) string {
	return fmt.Sprintf("%d-%d", v&0xFFFF, v>>16)
}

// paintInspect renders the inspected player: paper doll + the 13 slots on
// the DUserState1 layout (FState:5872-5964, 1095-1158).
func (s *PlayScene) paintInspect(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgStateBg, c.AbsX(), c.AbsY(), proj)
	}
	ax, ay := c.AbsX(), c.AbsY()

	// Paper doll (FState:5887-5932): body @(38,52), layers share origin
	// (31,96) with each image's own offset. Inspect female hair uses the
	// 480 base (:5900), unlike the own-character doll (441).
	if prg != nil {
		body := ImgBodyMale
		if s.inspectSex == 1 {
			body = ImgBodyFemale
		}
		s.ui.BlitImage(prg, body, ax+38, ay+52, proj)
		ox, oy := ax+31, ay+96
		hair := 440 + s.inspectHair/2
		if s.inspectSex == 1 {
			hair = 480 + s.inspectHair/2
		}
		if img := prg.GetImage(hair); img != nil && img.RGBA != nil {
			if t := s.resources.GetTexture(prg, hair); t != 0 {
				s.gl.DrawQuad(t, float32(ox+int(img.HotX)), float32(oy+int(img.HotY)),
					float32(img.Width), float32(img.Height), proj)
			}
		}
		for _, slot := range []int{protocol.UDress, protocol.UWeapon, protocol.UHelmet} {
			item := s.inspectItems[slot]
			if item == nil {
				continue
			}
			f, idx := s.stateItemFile(s.equippedLooks(item))
			if f == nil {
				continue
			}
			if img := f.GetImage(idx); img != nil && img.RGBA != nil {
				if t := s.resources.GetTexture(f, idx); t != 0 {
					s.gl.DrawQuad(t, float32(ox+int(img.HotX)), float32(oy+int(img.HotY)),
						float32(img.Width), float32(img.Height), proj)
				}
			}
		}
	}

	if s.text != nil {
		nw := s.text.MeasureText(s.inspectName)
		s.text.DrawText(s.inspectName, float32(ax+122-nw/2), float32(ay+23), 1, 1, 1, 1, proj)
	}

	for _, def := range inspectSlots {
		item := s.inspectItems[def.slot]
		if item == nil {
			continue
		}
		f, idx := s.stateItemFile(s.equippedLooks(item))
		if f == nil {
			continue
		}
		img := f.GetImage(idx)
		tex := s.resources.GetTexture(f, idx)
		if img == nil || img.RGBA == nil || tex == 0 {
			continue
		}
		x := ax + def.x + (def.w-img.Width)/2
		y := ay + def.y + (def.h-img.Height)/2
		s.gl.DrawQuad(tex, float32(x), float32(y), float32(img.Width), float32(img.Height), proj)
	}
}

// parseInspect fills the inspect window from an SMSendUserState body;
// sex/hair come from the world actor (the message carries only name +
// equipment).
func (s *PlayScene) parseInspect(name string, body string) {
	raw := []byte(body)
	if len(raw) < 130 {
		return
	}
	for i := 0; i < 13; i++ {
		off := i * 10
		windex := binary.LittleEndian.Uint16(raw[off : off+2])
		if windex == 0 {
			s.inspectItems[i] = nil
			continue
		}
		s.inspectItems[i] = &protocol.UserItem{
			WIndex:    windex,
			Dura:      binary.LittleEndian.Uint16(raw[off+2 : off+4]),
			DuraMax:   binary.LittleEndian.Uint16(raw[off+4 : off+6]),
			MakeIndex: int32(binary.LittleEndian.Uint32(raw[off+6 : off+10])),
		}
	}
	s.inspectName = name
	s.inspectSex, s.inspectHair = 0, 0
	for _, a := range s.State.Actors.All() {
		if a.UserName == name {
			s.inspectSex = a.Sex
			s.inspectHair = a.Hair
			break
		}
	}
	s.showInspect = true
}

// tryInspect queries the equipment of the player under the cursor.
func (s *PlayScene) tryInspect(x, y float64) {
	if s.cam == nil || s.mapData == nil || s.sendQueryUserState == nil || s.State.MySelf == nil {
		return
	}
	wx, wy := s.cam.ScreenToWorld(x, y)
	tx, ty := s.cam.WorldToTile(wx, wy)
	for _, a := range s.State.Actors.All() {
		if a.RecogID == s.State.MySelf.RecogID || a.Type != ActorHuman {
			continue
		}
		if a.CurrX == tx && a.CurrY == ty {
			s.sendQueryUserState(a.RecogID)
			return
		}
	}
}
