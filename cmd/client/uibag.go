package main

import (
	"strings"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// Bag panel — port of DItemBag/DItemGrid (FState.pas:1167-1174 window+grid,
// 4527-4661 grid interactions, 4602-4641 double-click, 1279-1292 gold/close,
// 4451-4520 bag direct paint + repair/close buttons).
const bagGridCols = 8

func (s *PlayScene) buildBag() {
	ui := s.ui
	prg := s.resources.Prguse

	bag := NewUIControl("DItemBag", KindWindow)
	bag.Floating = true
	bag.Left, bag.Top = 0, 0
	if prg != nil {
		bag.SetImgIndex(prg, ImgBagBg)
	} else {
		bag.Width, bag.Height = 320, 260
	}
	bag.Visible = false
	// Bag direct paint: background + gold amount + hovered item info
	// (DItemBagDirectPaint, FState:4451-4484).
	bag.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		s.paintBagWindow(c, proj)
	}
	ui.Root.AddChild(bag)
	s.hudBag = bag

	// Item grid (FState:1171-1174): origin (33,43), 8×6 @ 36×32, but the
	// hit area is runtime-clipped to 286×162 (the 6th row is barely
	// clickable in the original).
	grid := NewUIControl("DItemGrid", KindGrid)
	grid.Left, grid.Top = 33, 43
	grid.Width, grid.Height = 286, 162
	grid.ColCount, grid.RowCount = bagGridCols, 6
	grid.ColWidth, grid.RowHeight = 36, 32
	grid.OnGridPaint = func(c *UIControl, col, row, x, y, w, h int, selected bool, proj [16]float32) {
		s.paintBagCell(col, row, x, y, w, h, selected, proj)
	}
	grid.OnGridSelect = func(c *UIControl, col, row int) { s.bagGridSelect(col, row) }
	grid.OnDblClick = func(c *UIControl, x, y int) {
		if col, row, ok := c.ColRowAt(x, y); ok {
			s.bagGridDblClick(col, row)
		}
	}
	grid.OnGridMouseMove = func(c *UIControl, col, row int) { s.bagGridHover(col, row) }
	bag.AddChild(grid)

	// Gold button (runtime (133,231), FState:1280-1281 — the DlgConf
	// (10,190) entry is dead data): pick up the gold stack
	// (Index = moveIdxBagGold) so it can be dropped on the ground.
	gold := NewUIControl("DGold", KindButton)
	gold.Left, gold.Top = 133, 231
	if prg != nil {
		gold.SetImgIndex(prg, ImgGoldBtn)
	} else {
		gold.Width, gold.Height = 40, 30
	}
	gold.OnClick = func(c *UIControl, x, y int) {
		if s.itemMove.Moving || s.State.Gold <= 0 {
			return
		}
		// Gold has no item instance; the drop target prompts for the amount.
		s.itemMove.Moving = true
		s.itemMove.Index = moveIdxBagGold
		s.itemMove.FromBelt = -1
	}
	bag.AddChild(gold)

	// Repair button: runtime image [64], hit rect (10,10) 48×22, face drawn
	// at bag+(254,183) only while pressed (FState:1283-1287, 4496-4506 —
	// the paint offset is a DFM leftover; the control has no click handler
	// and is decorative).
	repair := NewUIControl("DRepairItem", KindButton)
	repair.Left, repair.Top = 10, 10
	repair.Width, repair.Height = 48, 22
	repair.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if c.Downed && prg != nil {
			s.ui.BlitImage(prg, ImgCloseMed, s.hudBag.AbsX()+254, s.hudBag.AbsY()+183, proj)
		}
	}
	bag.AddChild(repair)

	// Close button: runtime (314,20) 14×20 [371], painted only while
	// pressed (FState:1288-1292, 4508-4525).
	closeBtn := NewUIControl("DClosebag", KindButton)
	closeBtn.Left, closeBtn.Top = 314, 20
	closeBtn.Width, closeBtn.Height = 14, 20
	closeBtn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if c.Downed && prg != nil {
			s.ui.BlitImage(prg, ImgCloseSmall, c.AbsX(), c.AbsY(), proj)
		}
	}
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.State.ShowBag = false }
	bag.AddChild(closeBtn)
}

// paintBagWindow draws the bag background plus the in-window gold amount and
// hovered-item info lines (DItemBagDirectPaint, FState:4451-4484).
func (s *PlayScene) paintBagWindow(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgBagBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	// Gold amount, white (Left+50, Top+232).
	s.text.DrawText(goldStr(s.State.Gold), float32(c.AbsX()+50), float32(c.AbsY()+232), 1, 1, 1, 1, proj)
	// Hovered item info: name yellow, following lines white, last line red
	// when unusable (Left+70, Top+215 / +229 / +243).
	it := s.bagHoverItem
	if it == nil || s.State.FindBagItemByMakeIndex(it.MakeIndex) < 0 {
		return
	}
	text, useable := GetMouseItemInfo(s.State, it)
	lines := strings.Split(text, "\\")
	ax, ay := c.AbsX()+70, c.AbsY()+215
	for i, ln := range lines {
		if i > 2 {
			break
		}
		r, g, b := 1.0, 1.0, 1.0
		switch {
		case i == 0:
			r, g, b = 1.0, 1.0, 0.0
		case i == 2 && !useable:
			r, g, b = 1.0, 0.0, 0.0
		}
		s.text.DrawText(ln, float32(ax), float32(ay+i*14), float32(r), float32(g), float32(b), 1, proj)
	}
}

// syncBagWindow keeps the control visible state and Delphi repositioning:
// the bag slides to x=475 while NPC/shop windows are open (FState:4707,
// 4756), but stays at (0,0) during trade — OpenDealDlg sets Left:=0
// (FState:5621; the commented-out //475 is a stale value).
func (s *PlayScene) syncBagWindow() {
	if s.hudBag == nil {
		return
	}
	s.hudBag.Visible = s.State.ShowBag
	if s.State.ShowNpcDialog || s.State.ShowShop {
		s.hudBag.Left = 475
	} else {
		s.hudBag.Left = 0
	}
}

// bagSlotIndex maps a grid cell to the bag slot (cells beyond the 46-slot
// capacity are invalid).
func bagSlotIndex(col, row int) int {
	idx := col + row*bagGridCols
	if idx >= protocol.MaxBagItem {
		return -1
	}
	return idx
}

// paintBagCell draws a single bag cell (DItemGridGridPaint, FState:
// 4643-4661): no cell background (baked into [3]), the icon centered at
// full size with a (-1,+1) nudge, and no selection highlight (the handler
// ignores gdSelected).
func (s *PlayScene) paintBagCell(col, row, x, y, w, h int, selected bool, proj [16]float32) {
	idx := bagSlotIndex(col, row)
	if idx < 0 {
		return
	}
	item := s.State.BagItems[idx]
	if item != nil && !s.State.BeltHolds(item) && s.resources.Items != nil {
		looks := int(item.Looks())
		if looks >= 0 && looks < s.resources.Items.Count {
			img := s.resources.Items.GetImage(looks)
			tex := s.resources.GetTexture(s.resources.Items, looks)
			if img != nil && img.RGBA != nil && tex != 0 {
				iw, ih := float32(img.Width), float32(img.Height)
				fx, fy := float32(x), float32(y)
				s.gl.DrawQuad(tex, fx+(float32(w)-iw)/2-1, fy+(float32(h)-ih)/2+1, iw, ih, proj)
			}
		}
	}
}

// bagGridSelect: pick up / drop / swap (FState.pas:4557-4600).
func (s *PlayScene) bagGridSelect(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 {
		return
	}
	st := s.State

	if !s.itemMove.Moving {
		// Pick up the item in this cell.
		if item := st.BagItems[idx]; item != nil && !st.BeltHolds(item) {
			s.itemMove.Begin(idx, item)
			st.BagItems[idx] = nil
		}
		return
	}

	mi := s.itemMove.Index
	switch {
	case mi >= -13 && mi < 0:
		// Equipment → bag: server-side unequip (FState:4577-4582).
		if st.BagItems[idx] == nil && s.sendTakeOff != nil {
			s.sendTakeOff(moveEquipSlot(mi))
			s.itemMove.End()
		}
	case mi >= -29 && mi <= -20:
		// Trade offer → bag: take the offer back (FState:4584-4585).
		if st.BagItems[idx] == nil && s.sendDealDel != nil {
			s.sendDealDel(s.itemMove.Item.MakeIndex)
			s.itemMove.End()
		}
	case mi >= 0:
		// Bag → bag: client-local place/swap (server keeps the item list;
		// layout is client-owned until the next server re-sync).
		if mi < len(st.BagItems) {
			target := st.BagItems[idx]
			held := s.itemMove.Item
			st.BagItems[idx] = &held
			st.BagItems[mi] = target
		}
		s.itemMove.End()
	}
}

// bagGridDblClick uses or equips the item (FState:4602-4641). The first
// click of the double-click lifts the cell's item, so the held branch
// (:4622-4637) is the normal case when the double-click lands.
func (s *PlayScene) bagGridDblClick(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 {
		return
	}
	if s.itemMove.Moving {
		if s.itemMove.Index == idx {
			s.useOrEquipHeld()
		}
		return
	}
}

// takeOnTargetSlot picks the equip slot for a held item: the primary slot,
// or the free side for dual-slot items (rings/bracelets).
func (s *PlayScene) takeOnTargetSlot(stdMode byte) int {
	switch stdMode {
	case 22, 23:
		if s.State.UseItems[protocol.URingL] == nil {
			return protocol.URingL
		}
		return protocol.URingR
	case 24, 26:
		if s.State.UseItems[protocol.UArmRingL] == nil {
			return protocol.UArmRingL
		}
		return protocol.UArmRingR
	}
	return getTakeOnPosition(stdMode)
}

// useOrEquipHeld consumes the held item: consumables are used, equipment is
// equipped (Delphi's held branch only eats, FState:4622-4637; auto-equip is
// a Go extension).
func (s *PlayScene) useOrEquipHeld() {
	item := s.itemMove.Item
	if item.Def == nil {
		return
	}
	stdMode := item.Def.StdMode
	if stdMode <= 4 || stdMode == 31 {
		if s.sendUseItem != nil {
			s.sendUseItem(item.MakeIndex)
			s.itemMove.End()
		}
		return
	}
	slot := s.takeOnTargetSlot(stdMode)
	if slot < 0 || s.sendTakeOn == nil {
		return
	}
	s.sendTakeOn(item.MakeIndex, slot)
	// Optimistic visual: server re-sync confirms.
	it := item
	s.State.UseItems[slot] = &protocol.UserItem{
		MakeIndex: it.MakeIndex,
		WIndex:    it.Idx,
		Dura:      it.Dura,
		DuraMax:   it.DuraMax,
	}
	s.itemMove.End()
}

// bagGridHover shows the item tooltip below the cell (FState:4527-4555) and
// feeds the in-window info area.
func (s *PlayScene) bagGridHover(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 || s.hudBag == nil {
		return
	}
	item := s.State.BagItems[idx]
	if item == nil || s.State.BeltHolds(item) {
		s.bagHoverItem = nil
		return
	}
	s.bagHoverItem = item
	text, useable := GetMouseItemInfo(s.State, item)
	color := [4]float32{1, 1, 1, 1}
	if !useable {
		color = [4]float32{1, 0.3, 0.3, 1}
	}
	// Anchor at the cell's bottom-left, expanding downward (Delphi
	// ShowHint drawUp=FALSE, FState:4548-4550).
	ax := s.hudBag.AbsX() + 33 + col*36
	ay := s.hudBag.AbsY() + 43 + (row+1)*32
	s.tooltip.Show(ax, ay, text, color, false)
}
