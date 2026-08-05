package main

import "time"

// 交易窗口 — 移植自 DDealDlg/DDealRemoteDlg (FState.pas:1459-1491
// 布局, 5613-5865 行为): 己方 5×2 网格, 对方 5×2 网格, 金币输入,
// 确认/关闭带 4 秒操作节流 (g_dwDealActionTick)。
func (s *PlayScene) buildDealPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// 对方面板 [390] 在右上方, 己方面板 [389] 在其下方
	// (FState OpenDealDlg 重定位 :5617-5620)。
	remote := NewUIControl("DDealRemoteDlg", KindWindow)
	remote.Floating = true // DFM: 交易窗口可拖动
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
	own.Floating = true // DFM: 交易窗口可拖动
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

	// 己方物品网格: 5×2 @21,56, 格子 36×33 (FState:1465-1468)。
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

	// 对方网格: 5×2 只读 (RowCount=2, FState:1485-1488; 绘制遍历
	// 0..9, DDRGridGridPaint :5789-5807)。
	rgrid := NewUIControl("DDRGrid", KindControl)
	rgrid.Left, rgrid.Top = 21, 56
	rgrid.Width, rgrid.Height = 5*36, 2*33
	rgrid.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		for idx := 0; idx < 10; idx++ {
			x := c.AbsX() + (idx%5)*36
			y := c.AbsY() + (idx/5)*33
			s.paintDealCell(s.State.DealRemoteItems[idx], x, y, 36, 33, proj)
		}
	}
	remote.AddChild(rgrid)

	// 金币按钮 [28] (FState:1475-1477)。
	goldBtn := NewUIControl("DDGold", KindButton)
	goldBtn.Left, goldBtn.Top = 11, 137
	if prg != nil {
		goldBtn.SetImgIndex(prg, ImgDealGold)
	}
	goldBtn.OnClick = func(c *UIControl, x, y int) { s.dealGoldClick() }
	own.AddChild(goldBtn)

	// 确认 [391] / 关闭 [64]。
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
	// 关闭与确认共用节流门控 (FState:5667-5673)。
	closeBtn.OnClick = func(c *UIControl, x, y int) {
		if s.dealThrottled() {
			return
		}
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
	// 交易打开的瞬间强制打开背包一次 (OpenDealDlg, FState:5623)。
	if s.State.InDeal && !s.dealWasOpen {
		s.State.ShowBag = true
	}
	s.dealWasOpen = s.State.InDeal
}

func (s *PlayScene) paintDealCell(item *BagItem, x, y, w, h int, proj [16]float32) {
	// 无单格背景: 格子底图已烘焙在面板 [389]/[390] 中
	// (DDGridGridPaint/DDRGridGridPaint 只绘制物品, FState:5758-5807)。
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
	// 居中并带 Delphi 的 -1/+1 偏移修正 (FState:5767-5773)。
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
	s.text.DrawText(goldStr(s.State.DealGold), float32(c.AbsX()+64), float32(c.AbsY()+131), 1, 0.9, 0.3, 1, proj)
}

func (s *PlayScene) paintDealRemote(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgDealRem, c.AbsX(), c.AbsY(), proj)
		// DDRGold: 对方侧仅展示的金币按钮 [28]@(11,137),
		// 无点击行为 (FState:1489-1491)。
		s.ui.BlitImage(prg, ImgDealGold, c.AbsX()+11, c.AbsY()+137, proj)
	}
	if s.text == nil {
		return
	}
	nw := s.text.MeasureText(s.State.DealPartner)
	s.text.DrawText(s.State.DealPartner, float32(c.AbsX()+59+(106-nw)/2), float32(c.AbsY()+6), 1, 1, 1, 1, proj)
	s.text.DrawText(goldStr(s.State.DealRemoteGold), float32(c.AbsX()+64), float32(c.AbsY()+131), 1, 0.9, 0.3, 1, proj)
}

// dealThrottled 对应 g_dwDealActionTick (FState: 交易操作设置 4 秒
// 冷却; 各处理函数以此为门控)。
func (s *PlayScene) dealThrottled() bool {
	now := time.Now().UnixMilli()
	if now < s.dealActionTick {
		return true
	}
	s.dealActionTick = now + 4000
	return false
}

// dealGridSelect: 拿起/放置交易物品 (FState:5722-5756)。
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
			s.playItemClickSound(item)
			s.itemMove.Begin(moveIdxDeal(idx), item)
			st.DealItems[idx] = nil
		}
		return
	}
	mi := s.itemMove.Index
	switch {
	case mi >= 0:
		// 从背包 → 放入交易栏。Delphi 无条件发包（FState:5744-5747），
		// 目标格是否占用由服务端决定；回包按服务端指定槽位放置。
		if s.sendDealAdd != nil {
			s.sendDealAdd(s.itemMove.Item.MakeIndex)
			s.itemMove.End()
		}
	case mi >= -29 && mi <= -20:
		// 重新排列己方交易物品 (服务端列表顺序无关; 删除
		// 通过 MakeIndex 定位物品)。
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

// dealGoldClick: 拿起交易金币或弹出输入框输入金额
// (FState:5828-5865)。
func (s *PlayScene) dealGoldClick() {
	if s.State.DealEnd || s.dealThrottled() {
		return
	}
	gSound.PlaySound(sMoney)
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
		// 取回已放入的金币。
		s.itemMove.Moving = true
		s.itemMove.Index = moveIdxDealGold
		s.itemMove.FromBelt = -1
	}
}

// dealConfirm 锁定交易 (FState:5646-5665)。
func (s *PlayScene) dealConfirm() {
	if s.dealThrottled() {
		return
	}
	if s.sendDealEnd != nil {
		s.sendDealEnd()
	}
	s.State.DealEnd = true
	// 确认时若手持己方交易物品则自动放回交易栏
	// (FState:5656-5663: Index -29..-20 → AddDealItem + 结束拖拽)。
	if s.itemMove.Moving && s.itemMove.Index >= -29 && s.itemMove.Index <= -20 {
		if s.sendDealAdd != nil {
			s.sendDealAdd(s.itemMove.Item.MakeIndex)
		}
		s.itemMove.End()
	}
}

// resetDeal 在交易开始或结束时清空交易状态。
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
