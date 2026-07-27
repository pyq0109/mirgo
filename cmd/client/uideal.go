package main

import (
	"strconv"
	"time"
)

// Trade windows — port of DDealDlg/DDealRemoteDlg (FState.pas:1459-1491
// layout, 5613-5865 behavior): own 5×2 grid, partner 5×4 grid, gold input,
// confirm/close with the 4s action throttle (g_dwDealActionTick).
func (s *PlayScene) buildDealPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// Partner panel [390] top-right, own panel [389] below it
	// (FState OpenDealDlg repositioning :5617-5620).
	remote := NewUIControl("DDealRemoteDlg", KindWindow)
	if prg != nil {
		remote.SetImgIndex(prg, ImgDealRem)
	} else {
		remote.Width, remote.Height = 236, 200
	}
	remote.Left = ScreenWidth - 236 - 100
	remote.Top = 0
	remote.Visible = false
	remote.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintDealRemote(c, proj) }
	ui.Root.AddChild(remote)
	s.hudDealRemote = remote

	own := NewUIControl("DDealDlg", KindWindow)
	if prg != nil {
		own.SetImgIndex(prg, ImgDealBg)
	} else {
		own.Width, own.Height = 236, 200
	}
	own.Left = ScreenWidth - 236 - 100
	own.Top = remote.Height - 15
	own.Visible = false
	own.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintDealOwn(c, proj) }
	ui.Root.AddChild(own)
	s.hudDealOwn = own

	// Own offer grid: 5×2 @21,56, cells 36×33 (FState:1465-1468).
	grid := NewUIControl("DDGrid", KindGrid)
	grid.Left, grid.Top = 21, 56
	grid.ColCount, grid.RowCount = 5, 2
	grid.ColWidth, grid.RowHeight = 36, 33
	grid.OnGridPaint = func(c *UIControl, col, row, x, y, w, h int, selected bool, proj [16]float32) {
		idx := col + row*5
		s.paintDealCell(s.State.DealItems[idx], x, y, w, h, proj)
	}
	grid.OnGridSelect = func(c *UIControl, col, row int) { s.dealGridSelect(col + row*5) }
	grid.OnGridMouseMove = func(c *UIControl, col, row int) {
		s.dealCellHover(s.State.DealItems[col+row*5], c.AbsX()+col*36, c.AbsY()+(row+1)*33)
	}
	own.AddChild(grid)

	// Partner grid: 5×4 read-only (paint walks 0..19, FState:5789-5807).
	rgrid := NewUIControl("DDRGrid", KindControl)
	rgrid.Left, rgrid.Top = 21, 56
	rgrid.Width, rgrid.Height = 5*36, 4*33
	rgrid.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		for idx := 0; idx < 20; idx++ {
			x := c.AbsX() + (idx%5)*36
			y := c.AbsY() + (idx/5)*33
			s.paintDealCell(s.State.DealRemoteItems[idx], x, y, 36, 33, proj)
		}
	}
	remote.AddChild(rgrid)

	// Gold button [28] (FState:1475-1477).
	goldBtn := NewUIControl("DDGold", KindButton)
	goldBtn.Left, goldBtn.Top = 11, 137
	if prg != nil {
		goldBtn.SetImgIndex(prg, ImgGoldBtn)
	}
	goldBtn.OnClick = func(c *UIControl, x, y int) { s.dealGoldClick() }
	own.AddChild(goldBtn)

	// Confirm [391] / close [64].
	ok := NewUIControl("DDealOk", KindButton)
	ok.Left, ok.Top = 155, 128
	if prg != nil {
		ok.SetImgIndex(prg, ImgDealOk)
	}
	ok.OnClick = func(c *UIControl, x, y int) { s.dealConfirm() }
	own.AddChild(ok)

	closeBtn := NewUIControl("DDealClose", KindButton)
	closeBtn.Left, closeBtn.Top = 220, 42
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseMed)
	}
	closeBtn.OnClick = func(c *UIControl, x, y int) {
		if s.sendDealCancel != nil {
			s.sendDealCancel()
		}
		s.State.InDeal = false
	}
	own.AddChild(closeBtn)
}

func (s *PlayScene) syncDealWindows() {
	if s.hudDealOwn != nil {
		s.hudDealOwn.Visible = s.State.InDeal
	}
	if s.hudDealRemote != nil {
		s.hudDealRemote.Visible = s.State.InDeal
	}
}

func (s *PlayScene) paintDealCell(item *BagItem, x, y, w, h int, proj [16]float32) {
	s.gl.DrawQuadColor(float32(x), float32(y), float32(w), float32(h), 0.1, 0.1, 0.15, 0.35, proj)
	if item == nil || s.resources.Items == nil {
		return
	}
	looks := int(item.Looks())
	if looks < 0 || looks >= s.resources.Items.Count {
		return
	}
	img := s.resources.Items.GetImage(looks)
	tex := s.resources.GetTexture(s.resources.Items, looks)
	if img == nil || img.RGBA == nil || tex == 0 {
		return
	}
	// Centered with Delphi's -1/+1 nudge (FState:5767-5773).
	iw, ih := float32(img.Width), float32(img.Height)
	s.gl.DrawQuad(tex, float32(x)+(float32(w)-iw)/2-1, float32(y)+(float32(h)-ih)/2+1, iw, ih, proj)
}

func (s *PlayScene) paintDealOwn(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgDealBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	name := ""
	if s.State.MySelf != nil {
		name = s.State.MySelf.UserName
	}
	nw := s.text.MeasureText(name)
	s.text.DrawText(name, float32(c.AbsX()+59+(106-nw)/2), float32(c.AbsY()+6), 1, 1, 1, 1, proj)
	s.text.DrawText(strconv.Itoa(s.State.DealGold), float32(c.AbsX()+64), float32(c.AbsY()+131), 1, 0.9, 0.3, 1, proj)
}

func (s *PlayScene) paintDealRemote(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgDealRem, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	nw := s.text.MeasureText(s.State.DealPartner)
	s.text.DrawText(s.State.DealPartner, float32(c.AbsX()+59+(106-nw)/2), float32(c.AbsY()+6), 1, 1, 1, 1, proj)
	s.text.DrawText(strconv.Itoa(s.State.DealRemoteGold), float32(c.AbsX()+64), float32(c.AbsY()+131), 1, 0.9, 0.3, 1, proj)
}

// dealThrottled mirrors g_dwDealActionTick (FState: deal actions set a 4s
// cooldown; handlers gate on it).
func (s *PlayScene) dealThrottled() bool {
	now := time.Now().UnixMilli()
	if now < s.dealActionTick {
		return true
	}
	s.dealActionTick = now + 4000
	return false
}

// dealGridSelect: pick up / place offered items (FState:5722-5756).
func (s *PlayScene) dealGridSelect(idx int) {
	if idx < 0 || idx >= 10 || s.State.DealEnd {
		return
	}
	if s.dealThrottled() {
		return
	}
	st := s.State
	if !s.itemMove.Moving {
		if item := st.DealItems[idx]; item != nil {
			s.itemMove.Begin(moveIdxDeal(idx), item)
			st.DealItems[idx] = nil
		}
		return
	}
	mi := s.itemMove.Index
	switch {
	case mi >= 0:
		// From bag → offer it (server moves the instance out of the bag).
		if st.DealItems[idx] == nil && s.sendDealAdd != nil {
			s.sendDealAdd(s.itemMove.Item.MakeIndex)
			s.itemMove.End()
		}
	case mi >= -29 && mi <= -20:
		// Rearrange own offers (server list order is irrelevant; deletion
		// addresses items by MakeIndex).
		if st.DealItems[idx] == nil {
			it := s.itemMove.Item
			st.DealItems[idx] = &it
			s.itemMove.End()
		}
	case mi == moveIdxBagGold:
		s.itemMove.End()
		s.dealGoldClick()
	}
}

func (s *PlayScene) dealCellHover(item *BagItem, x, y int) {
	if item == nil {
		return
	}
	text, useable := GetMouseItemInfo(s.State, item)
	color := [4]float32{1, 1, 1, 1}
	if !useable {
		color = [4]float32{1, 0.3, 0.3, 1}
	}
	s.tooltip.Show(x, y, text, color, false)
}

// dealGoldClick: pick up deal gold or prompt for the amount to stake
// (FState:5828-5865).
func (s *PlayScene) dealGoldClick() {
	if s.State.DealEnd || s.dealThrottled() {
		return
	}
	if s.itemMove.Moving && (s.itemMove.Index == moveIdxBagGold || s.itemMove.Index == moveIdxDealGold) {
		s.itemMove.End()
		max := s.State.DealGold + s.State.Gold
		ShowInput(s, "Gold amount to trade", func(ok bool, text string) {
			if !ok {
				return
			}
			amount := atoiClamped(text, 0, max)
			if amount > 0 && s.sendDealChgGold != nil {
				s.sendDealChgGold(amount)
			}
		})
		return
	}
	if !s.itemMove.Moving && s.State.DealGold > 0 {
		// Pick the staked gold back up.
		s.itemMove.Moving = true
		s.itemMove.Index = moveIdxDealGold
		s.itemMove.FromBelt = -1
	}
}

// dealConfirm locks the offer (FState:5646-5665).
func (s *PlayScene) dealConfirm() {
	if s.dealThrottled() {
		return
	}
	if s.sendDealEnd != nil {
		s.sendDealEnd()
	}
	s.State.DealEnd = true
}

// resetDeal clears trade state when a deal opens or ends.
func (s *PlayScene) resetDeal() {
	st := s.State
	for i := range st.DealItems {
		st.DealItems[i] = nil
	}
	for i := range st.DealRemoteItems {
		st.DealRemoteItems[i] = nil
	}
	st.DealGold = 0
	st.DealRemoteGold = 0
	st.DealEnd = false
}
