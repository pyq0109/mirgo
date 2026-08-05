package main

import (
	"fmt"
	"strings"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// 背包面板 — 移植自 DItemBag/DItemGrid (FState.pas:1167-1174 窗口+网格,
// 4527-4661 网格交互, 4602-4641 双击, 1279-1292 金币/关闭,
// 4451-4520 背包直接绘制 + 修理/关闭按钮)。
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
	// 背包直接绘制: 背景 + 金币数 + 悬浮物品信息
	// (DItemBagDirectPaint, FState:4451-4484)。
	bag.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		s.paintBagWindow(c, proj)
	}
	ui.Root.AddChild(bag)
	s.hudBag = bag

	// 物品网格 (FState:1171-1174): 起点 (33,43), 8×6 @ 36×32, 但
	// 点击区域运行时裁剪为 286×162 (原版第 6 行几乎
	// 不可点击)。
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

	// 金币按钮 (运行时 (133,231), FState:1280-1281 — DlgConf 中的
	// (10,190) 已废弃): 拿起金币堆 (Index = moveIdxBagGold)
	// 以便丢到地上。
	gold := NewUIControl("DGold", KindButton)
	gold.Left, gold.Top = 133, 231
	if prg != nil {
		gold.SetImgIndex(prg, ImgGoldBtn)
	} else {
		gold.Width, gold.Height = 40, 30
	}
	gold.OnClick = func(c *UIControl, x, y int) {
		// 手持交易金币点背包金币按钮 = 交易金币归零
		//（Delphi DGoldClick → DealZeroGold，FState:4673-4681/5820-5826）。
		if s.itemMove.Moving && s.itemMove.Index == moveIdxDealGold {
			if s.sendDealChgGold != nil {
				s.sendDealChgGold(0)
			}
			s.itemMove.End()
			return
		}
		// 手持其他物品时点击 = 取消手持（Delphi 二次点击取消）。
		if s.itemMove.Moving {
			s.itemMove.Cancel(s.State)
			return
		}
		if s.State.Gold <= 0 {
			return
		}
		gSound.PlaySound(sMoney)
		// 金币无物品实例; 放置目标会弹出数量输入。
		s.itemMove.Moving = true
		s.itemMove.Index = moveIdxBagGold
		s.itemMove.FromBelt = -1
	}
	bag.AddChild(gold)

	// 修理按钮: 运行时图片 [64], 点击区域 (10,10) 48×22, 仅按下时
	// 在 bag+(254,183) 绘制面板 (FState:1283-1287, 4496-4506 —
	// 绘制偏移是 DFM 遗留; 该控件无点击处理, 纯装饰)。
	repair := NewUIControl("DRepairItem", KindButton)
	repair.Left, repair.Top = 10, 10
	repair.Width, repair.Height = 48, 22
	repair.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if c.Downed && prg != nil {
			s.ui.BlitImage(prg, ImgCloseMed, s.hudBag.AbsX()+254, s.hudBag.AbsY()+183, proj)
		}
	}
	bag.AddChild(repair)

	// 关闭按钮: 运行时 (314,20) 14×20 [371], 仅按下时
	// 绘制 (FState:1288-1292, 4508-4525)。
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

// paintBagWindow 绘制背包背景及窗口内的金币数和悬浮物品信息行
// (DItemBagDirectPaint, FState:4451-4484)。
func (s *PlayScene) paintBagWindow(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgBagBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	// 金币数, 白色 (Left+50, Top+232)。
	s.text.DrawText(goldStr(s.State.Gold), float32(c.AbsX()+50), float32(c.AbsY()+232), 1, 1, 1, 1, proj)
	// 悬浮物品信息: 名称黄色, 后续行白色, 不可用时末行红色
	// (Left+70, Top+215 / +229 / +243)。
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

// syncBagWindow 同步控件可见状态及 Delphi 的重定位逻辑:
// NPC/商店窗口打开时背包滑到 x=475 (FState:4707,
// 4756), 交易时保持在 (0,0) — OpenDealDlg 设置 Left:=0
// (FState:5621; 被注释的 //475 是过期值)。
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

// bagSlotIndex 将网格坐标映射为背包槽位 (超出 46 格容量的格子无效)。
func bagSlotIndex(col, row int) int {
	idx := col + row*bagGridCols
	if idx >= protocol.MaxBagItem {
		return -1
	}
	return idx
}

// paintBagCell 绘制单个背包格 (DItemGridGridPaint, FState:
// 4643-4661): 无格子背景 (已烘焙在 [3] 中), 图标原始尺寸居中
// 带 (-1,+1) 偏移, 无选中高亮 (处理函数忽略 gdSelected)。
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

// bagGridSelect: 拿起/放下/交换 (FState.pas:4557-4600)。
func (s *PlayScene) bagGridSelect(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 {
		return
	}
	st := s.State

	if !s.itemMove.Moving {
		// 拿起此格中的物品。
		if item := st.BagItems[idx]; item != nil && !st.BeltHolds(item) {
			s.playItemClickSound(item)
			s.itemMove.Begin(idx, item)
			st.BagItems[idx] = nil
		}
		return
	}

	mi := s.itemMove.Index
	switch {
	case mi >= -13 && mi < 0:
		// 装备 → 背包: 服务端脱装备。Delphi 无空格限制，
		// 直接发包（FState:4577-4582），服务端放入首个空位。
		if s.sendTakeOff != nil {
			s.sendTakeOff(moveEquipSlot(mi))
			s.itemMove.End()
		}
	case mi >= -29 && mi <= -20:
		// 交易物品 → 背包: 取回交易品 (FState:4584-4585)。
		if st.BagItems[idx] == nil && s.sendDealDel != nil {
			s.sendDealDel(s.itemMove.Item.MakeIndex)
			s.itemMove.End()
		}
	case mi >= 0:
		// 背包 → 背包: 客户端本地放置/交换 (服务端维护物品列表;
		// 布局由客户端管理直到下次服务端同步)。
		if mi < len(st.BagItems) {
			target := st.BagItems[idx]
			held := s.itemMove.Item
			st.BagItems[idx] = &held
			st.BagItems[mi] = target
		}
		s.itemMove.End()
	}
}

// bagGridDblClick 双击背包格 (FState:4602-4641)。双击的第一次点击
// 会拿起格子物品, 因此双击触发时走的是手持分支 (:4622-4637):
// Ctrl+双击 = 整理到背包首个空位; 普通双击 = 使用 (Delphi 双击
// 不穿装备, 装备须拖到格位)。
func (s *PlayScene) bagGridDblClick(col, row int) {
	idx := bagSlotIndex(col, row)
	if idx < 0 {
		return
	}
	if !s.itemMove.Moving {
		return
	}
	if s.ctrlDown {
		// Ctrl+双击 = 整理 (FState:4626-4631, AddItemBag)。
		it := s.itemMove.Item
		s.itemMove.End()
		returnItemToBag(s.State, it)
		return
	}
	if s.itemMove.Index == idx {
		item := s.itemMove.Item
		if item.Def != nil && (item.Def.StdMode <= 4 || item.Def.StdMode == 31) {
			s.useHeldItem(item)
			s.itemMove.End()
		}
	}
}

// returnItemToBag 将物品放回背包首个空位 (Delphi AddItemBag,
// FState.pas 同名函数)。仅本地视觉恢复, 背包内容仍以服务端为准。
func returnItemToBag(gs *GameState, item BagItem) {
	for i := range gs.BagItems {
		if gs.BagItems[i] == nil {
			it := item
			gs.BagItems[i] = &it
			return
		}
	}
}

// takeOnTargetSlot 为手持物品选择装备槽: 主槽位,
// 或双槽物品的空余侧 (戒指/手镯)。
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

// useHeldItem 使用手持物品 (Delphi EatItem -1 分支,
// ClMain.pas:1975-1999)。StdMode=4 且 Shape<100 的技能书先弹学习
// 确认; 取消时物品放回背包首个空位 (ClMain.pas:1984-1987)。
func (s *PlayScene) useHeldItem(item BagItem) {
	if item.Def == nil || s.sendUseItem == nil {
		return
	}
	send := func() {
		if idx := itemUseSoundIdx(item.Def.StdMode); idx >= 0 {
			gSound.PlaySound(idx)
		}
		if s.sendUseItem != nil {
			s.sendUseItem(item.MakeIndex)
		}
	}
	if item.Def.StdMode == 4 && item.Def.Shape < 100 {
		prompt := "确认开始学习"
		if item.Def.Shape >= 50 {
			prompt = "您确认开始学习"
		}
		ShowConfirm(s, fmt.Sprintf("%s \"%s\"?", prompt, item.Def.Name),
			[]ModalResult{MrYes, MrNo}, DlgNormal, func(mr ModalResult) {
				if mr == MrYes {
					send()
				} else {
					returnItemToBag(s.State, item)
				}
			})
		return
	}
	send()
}

// bagGridHover 在格子下方显示物品提示 (FState:4527-4555) 并
// 更新窗口内信息区域。
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
	// 锚定在格子左下角, 向下展开 (Delphi
	// ShowHint drawUp=FALSE, FState:4548-4550)。
	ax := s.hudBag.AbsX() + 33 + col*36
	ay := s.hudBag.AbsY() + 43 + (row+1)*32
	s.tooltip.Show(ax, ay, text, color, false)
}

// playItemClickSound 按物品类型播放点击音效（Delphi SoundUtil:293-310）。
func (s *PlayScene) playItemClickSound(item *BagItem) {
	if item == nil || item.Def == nil {
		gSound.PlaySound(sItmClick)
		return
	}
	name := ""
	if item.Def != nil {
		name = item.Def.Name
	}
	gSound.PlaySound(itemClickSoundIdx(item.Def.StdMode, name))
}
