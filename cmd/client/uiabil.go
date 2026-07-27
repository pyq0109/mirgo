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

func (s *PlayScene) buildAbilPanel() {
	ui := s.ui
	prg := s.resources.Prguse

	win := NewUIControl("DAdjustAbility", KindWindow)
	win.Floating = true
	if prg != nil {
		win.SetImgIndex(prg, ImgAdjustBg)
	} else {
		win.Width, win.Height = 300, 340
	}
	win.Left, win.Top = 0, 0
	win.Visible = false
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintAbilPanel(c, proj) }
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
	inspect.Floating = true
	if prg != nil {
		inspect.SetImgIndex(prg, ImgStateBg)
	} else {
		inspect.Width, inspect.Height = 240, 300
	}
	inspect.Left = ScreenWidth - 2*inspect.Width
	inspect.Top = 0
	inspect.Visible = false
	inspect.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintInspect(c, proj) }
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

// abilAdjust moves points between the budget and one stat (FState:6633-6704,
// without the Ctrl×10 accelerator).
func (s *PlayScene) abilAdjust(stat, delta int) {
	if delta > 0 {
		if s.abilPointsLeft <= 0 {
			return
		}
		s.abilDeltas[stat]++
		s.abilPointsLeft--
		return
	}
	if s.abilDeltas[stat] <= 0 {
		return
	}
	s.abilDeltas[stat]--
	s.abilPointsLeft++
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

	values := [9]string{
		loHiStr(st.DC), loHiStr(st.MC), loHiStr(st.SC), loHiStr(st.AC), loHiStr(st.MAC),
		fmt.Sprintf("%d/%d", st.HP, st.MaxHP),
		fmt.Sprintf("%d/%d", st.MP, st.MaxMP),
		fmt.Sprintf("%d", st.Hit),
		fmt.Sprintf("%d", st.Speed),
	}
	for i := 0; i < 9; i++ {
		s.text.DrawText(abilStatNames[i], float32(c.AbsX()+40), float32(rowY[i]), 0.9, 0.9, 0.9, 1, proj)
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

// paintInspect renders the inspected player's 13 slots using the shared
// slot layout (DUserState1, FState:1088-1163).
func (s *PlayScene) paintInspect(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgStateBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text != nil {
		nw := s.text.MeasureText(s.inspectName)
		s.text.DrawText(s.inspectName, float32(c.AbsX()+122-nw/2), float32(c.AbsY()+23), 1, 1, 1, 1, proj)
	}
	for _, def := range stateSlots {
		item := s.inspectItems[def.slot]
		if item == nil {
			continue
		}
		f, idx := s.stateItemFile(int(item.WIndex))
		if f == nil {
			continue
		}
		img := f.GetImage(idx)
		tex := s.resources.GetTexture(f, idx)
		if img == nil || img.RGBA == nil || tex == 0 {
			continue
		}
		x := c.AbsX() + def.x + (def.w-img.Width)/2
		y := c.AbsY() + def.y + (def.h-img.Height)/2
		s.gl.DrawQuad(tex, float32(x), float32(y), float32(img.Width), float32(img.Height), proj)
	}
}

// parseInspect fills the inspect window from an SMSendUserState body.
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
