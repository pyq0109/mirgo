package main

import (
	"github.com/pyq0109/mirgo/internal/protocol"
)

// Bag panel — port of DItemBag/DItemGrid (FState.pas:1167-1174 window+grid,
// 4527-4661 grid interactions, 4602-4641 double-click, 1279-1292 gold/close).
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
	ui.Root.AddChild(bag)
	s.hudBag = bag

	// Item grid (FState:1171-1174: 33,43 286×162 = 8×6 @ 36×32).
	grid := NewUIControl("DItemGrid", KindGrid)
	grid.Left, grid.Top = 33, 43
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

	// Gold button (DlgConf DGold 29 @10,190): pick up the gold stack
	// (Index = moveIdxBagGold) so it can be dropped on the ground.
	gold := NewUIControl("DGold", KindButton)
	gold.Left, gold.Top = 10, 190
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

	// Close button (DlgConf DClosebag 371 @309,203 14×20).
	closeBtn := NewUIControl("DClosebag", KindButton)
	closeBtn.Left, closeBtn.Top = 309, 203
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseSmall)
	}
	closeBtn.Width, closeBtn.Height = 14, 20
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.State.ShowBag = false }
	bag.AddChild(closeBtn)
}

// syncBagWindow keeps the control visible state and Delphi repositioning
// (bag slides to x=475 while NPC/shop/trade windows are open,
// FState:4707-4708,4756-4757,5621-5622).
func (s *PlayScene) syncBagWindow() {
	if s.hudBag == nil {
		return
	}
	s.hudBag.Visible = s.State.ShowBag
	if s.State.ShowNpcDialog || s.State.ShowShop || s.State.InDeal {
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

func (s *PlayScene) paintBagCell(col, row, x, y, w, h int, selected bool, proj [16]float32) {
	idx := bagSlotIndex(col, row)
	fx, fy := float32(x), float32(y)
	s.gl.DrawQuadColor(fx, fy, float32(w), float32(h), 0.1, 0.1, 0.15, 0.35, proj)
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
				if iw > float32(w)-4 {
					iw = float32(w) - 4
				}
				if ih > float32(h)-4 {
					ih = float32(h) - 4
				}
				s.gl.DrawQuad(tex, fx+(float32(w)-iw)/2-2, fy+(float32(h)-ih)/2-2, iw, ih, proj)
			}
		}
	}
	if selected {
		s.gl.DrawQuadColor(fx, fy, float32(w), 2, 1, 1, 0.4, 0.8, proj)
		s.gl.DrawQuadColor(fx, fy+float32(h)-2, float32(w), 2, 1, 1, 0.4, 0.8, proj)
		s.gl.DrawQuadColor(fx, fy, 2, float32(h), 1, 1, 0.4, 0.8, proj)
		s.gl.DrawQuadColor(fx+float32(w)-2, fy, 2, float32(h), 1, 1, 0.4, 0.8, proj)
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

// bagGridDblClick: use or auto-equip (FState.pas:4602-4641).
func (s *PlayScene) bagGridDblClick(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 || s.itemMove.Moving {
		return
	}
	item := s.State.BagItems[idx]
	if item == nil || s.State.BeltHolds(item) {
		return
	}
	stdMode := byte(0xFF)
	if item.Def != nil {
		stdMode = item.Def.StdMode
	}
	if stdMode <= 4 || stdMode == 31 {
		// Consumable (DItemGridDblClick → EatItem).
		if s.sendUseItem != nil {
			s.sendUseItem(item.MakeIndex)
		}
		return
	}
	// Otherwise try to equip it.
	slot := getTakeOnPosition(stdMode)
	if slot >= 0 && s.sendTakeOn != nil {
		s.sendTakeOn(item.MakeIndex, slot)
		s.State.BagItems[idx] = nil // optimistic; server re-sync confirms
	}
}

// bagGridHover shows the item tooltip below the cell (FState:4527-4555).
func (s *PlayScene) bagGridHover(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 || s.hudBag == nil {
		return
	}
	item := s.State.BagItems[idx]
	if item == nil || s.State.BeltHolds(item) {
		return
	}
	text, useable := GetMouseItemInfo(s.State, item)
	color := [4]float32{1, 1, 1, 1}
	if !useable {
		color = [4]float32{1, 0.3, 0.3, 1}
	}
	// Anchor at the cell's bottom-left (Delphi SurfaceX/Y of the cell).
	ax := s.hudBag.AbsX() + 33 + col*36
	ay := s.hudBag.AbsY() + 43 + (row+1)*32
	s.tooltip.Show(ax, ay, text, color, true)
}
