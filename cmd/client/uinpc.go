package main

import (
	"strconv"
	"strings"
	"time"
)

// NPC 对话框 + 商店面板 — 移植自 DMerchantDlg (FState.pas:4806-4880
// 富文本绘制, 5116-5162 点击), DMenuDlg 购买列表 (4887-5075) 及
// DSellDlg 出售/修理/寄存槽 (5164-5264)。
const (
	npcTextX    = 30 // FState:4820
	npcTextY    = 30 // FState:4821
	npcLineH    = 16 // FState:4874
	menuMaxRows = 10 // MAXMENU
	menuRowH    = 13 // LISTLINEHEIGHT
)

type npcSegment struct {
	text string
	tag  string // 非空 = 可点击链接, 携带此值
}

type npcClickPoint struct {
	x, y, w, h int
	tag        string
}

// parseNpcDialog 解析 SMMerchantSay 消息体 (Delphi 标签语法:
// <显示文本/链接值>, 行以 '\' 分隔; FState:4830-4870)。
func (s *PlayScene) parseNpcDialog(body string) {
	s.npcLines = nil
	s.npcLineCentered = nil
	s.npcClicks = nil
	s.npcSelectTag = ""
	s.npcScrollOffset = 0
	for _, line := range strings.Split(body, "\\") {
		var segs []npcSegment
		centered := false
		rest := line
		for {
			lt := strings.IndexByte(rest, '<')
			if lt < 0 {
				if rest != "" {
					segs = append(segs, npcSegment{text: rest})
				}
				break
			}
			gt := strings.IndexByte(rest[lt:], '>')
			if gt < 0 {
				segs = append(segs, npcSegment{text: rest})
				break
			}
			if lt > 0 {
				segs = append(segs, npcSegment{text: rest[:lt]})
			}
			tag := rest[lt+1 : lt+gt]
			rest = rest[lt+gt+1:]
			if tag == "C" {
				centered = true
				continue
			}
			if tag == "/C" {
				continue
			}
			if tag == "" {
				// 空标签隐藏商店窗口 (FState:4847-4850)。
				s.State.ShowShop = false
				continue
			}
			// <显示文本/链接值>
			display, link := tag, ""
			if slash := strings.IndexByte(tag, '/'); slash >= 0 {
				display, link = tag[:slash], tag[slash+1:]
			}
			segs = append(segs, npcSegment{text: display, tag: link})
		}
		s.npcLines = append(s.npcLines, segs)
		s.npcLineCentered = append(s.npcLineCentered, centered)
	}
}

func (s *PlayScene) buildNpcPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// --- NPC 对话框 (DMerchantDlg [384] @0,0) ---
	npc := NewUIControl("DMerchantDlg", KindWindow)
	npc.Floating = false // DFM: 不可拖动
	if prg != nil {
		npc.SetImgIndex(prg, ImgNpcDlg)
	} else {
		npc.Width, npc.Height = 400, 250
	}
	npc.Left, npc.Top = 0, 0
	npc.Visible = false
	npc.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintNpcDialog(c, proj) }
	npc.OnMouseDown = func(c *UIControl, button, x, y int) { s.npcClick(x, y) }
	npc.OnMouseUp = func(c *UIControl, button, x, y int) { s.npcSelectTag = "" } // FState:5158-5162
	ui.Root.AddChild(npc)
	s.hudNpc = npc

	npcClose := NewUIControl("DMerchantDlgClose", KindButton)
	npcClose.Left, npcClose.Top = 372, 20 // 运行时覆盖 (FState:1303-1305)
	if prg != nil {
		npcClose.SetImgIndex(prg, ImgCloseSmall)
	}
	npcClose.OnClick = func(c *UIControl, x, y int) { s.State.ShowNpcDialog = false }
	npc.AddChild(npcClose)

	// --- 购买列表 (DMenuDlg [385], 运行时位置 0,170 — FState:4751-4752) ---
	menu := NewUIControl("DMenuDlg", KindWindow)
	menu.Floating = false // DFM: 不可拖动
	if prg != nil {
		menu.SetImgIndex(prg, ImgShopBg)
	} else {
		menu.Width, menu.Height = 300, 200
	}
	menu.Left, menu.Top = 0, 170
	menu.Visible = false
	menu.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintShopMenu(c, proj) }
	menu.OnMouseDown = func(c *UIControl, button, x, y int) { s.menuRowClick(x, y) }
	ui.Root.AddChild(menu)
	s.hudMenu = menu

	// 运行时按钮布局 (FState:1330-1341 — DlgConf 中的值已废弃):
	// Prev[387]@(328,42), Next[388]@(328,162),
	// Buy[362]@(100,230), Close[366]@(175,230)。
	menuPrev := NewUIControl("DMenuPrev", KindButton)
	menuPrev.Left, menuPrev.Top = 328, 42
	if prg != nil {
		menuPrev.SetImgIndex(prg, ImgPageUp)
	}
	menuPrev.OnClick = func(c *UIControl, x, y int) {
		s.menuTop -= menuMaxRows - 1
		if s.menuTop < 0 {
			s.menuTop = 0
		}
	}
	menu.AddChild(menuPrev)

	menuNext := NewUIControl("DMenuNext", KindButton)
	menuNext.Left, menuNext.Top = 328, 162
	if prg != nil {
		menuNext.SetImgIndex(prg, ImgPageDown)
	}
	menuNext.OnClick = func(c *UIControl, x, y int) {
		if s.menuTop+menuMaxRows < len(s.State.ShopGoods) {
			s.menuTop += menuMaxRows - 1
		}
	}
	menu.AddChild(menuNext)

	menuBuy := NewUIControl("DMenuBuy", KindButton)
	menuBuy.Left, menuBuy.Top = 100, 230
	if prg != nil {
		menuBuy.SetImgIndex(prg, ImgConfirm)
	}
	menuBuy.OnClick = func(c *UIControl, x, y int) { s.buySelected() }
	menu.AddChild(menuBuy)

	menuClose := NewUIControl("DMenuClose", KindButton)
	menuClose.Left, menuClose.Top = 175, 230
	if prg != nil {
		menuClose.SetImgIndex(prg, ImgCancel)
	}
	menuClose.OnClick = func(c *UIControl, x, y int) { s.State.ShowShop = false }
	menu.AddChild(menuClose)

	// --- 出售/修理/寄存槽 (DSellDlg [392], 运行时位置 260,170) ---
	sell := NewUIControl("DSellDlg", KindWindow)
	sell.Floating = false // DFM: 不可拖动
	if prg != nil {
		sell.SetImgIndex(prg, ImgSellBg)
	} else {
		sell.Width, sell.Height = 200, 180
	}
	sell.Left, sell.Top = 260, 170
	sell.Visible = false
	sell.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintSellDlg(c, proj) }
	ui.Root.AddChild(sell)
	s.hudSell = sell

	// 放置待出售/修理/寄存物品的槽位 (27,67 61×52)。
	spot := NewUIControl("DSellDlgSpot", KindButton)
	spot.Left, spot.Top = 27, 67
	spot.Width, spot.Height = 61, 52
	spot.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if s.sellItem == nil || s.resources.Items == nil {
			return
		}
		looks := int(s.sellItem.Looks())
		if looks < 0 || looks >= s.resources.Items.Count {
			return
		}
		img := s.resources.Items.GetImage(looks)
		tex := s.resources.GetTexture(s.resources.Items, looks)
		if img == nil || img.RGBA == nil || tex == 0 {
			return
		}
		// 在槽位中居中 (FState:5235-5241)。
		s.gl.DrawQuad(tex, float32(c.AbsX())+float32(61-img.Width)/2,
			float32(c.AbsY())+float32(52-img.Height)/2,
			float32(img.Width), float32(img.Height), proj)
	}
	spot.OnClick = func(c *UIControl, x, y int) { s.sellSpotClick() }
	spot.OnMouseMove = func(c *UIControl, x, y int) {
		if s.sellItem == nil {
			return
		}
		text, useable := GetMouseItemInfo(s.State, s.sellItem)
		color := [4]float32{1, 1, 1, 1}
		if !useable {
			color = [4]float32{1, 0.3, 0.3, 1}
		}
		s.tooltip.Show(c.AbsX(), c.AbsY()+52, text, color, false)
	}
	sell.AddChild(spot)

	// 运行时布局 (FState:1352-1357): OK[362]@(28,135), Close[366]@(81,135)。
	sellOk := NewUIControl("DSellDlgOk", KindButton)
	sellOk.Left, sellOk.Top = 28, 135
	if prg != nil {
		sellOk.SetImgIndex(prg, ImgConfirm)
	}
	sellOk.OnClick = func(c *UIControl, x, y int) { s.sellOk() }
	sell.AddChild(sellOk)

	sellClose := NewUIControl("DSellDlgClose", KindButton)
	sellClose.Left, sellClose.Top = 81, 135
	if prg != nil {
		sellClose.SetImgIndex(prg, ImgCancel)
	}
	sellClose.OnClick = func(c *UIControl, x, y int) { s.State.ShowShop = false }
	sell.AddChild(sellClose)
}

// syncMerchantWindows 根据状态控制对话框/商店窗口可见性,
// 全部关闭后清除临时状态。
func (s *PlayScene) syncMerchantWindows() {
	if s.hudNpc != nil {
		s.hudNpc.Visible = s.State.ShowNpcDialog
	}
	if s.hudMenu != nil {
		s.hudMenu.Visible = s.State.ShowShop && s.State.ShopMode == 0
	}
	if s.hudSell != nil {
		s.hudSell.Visible = s.State.ShowShop && s.State.ShopMode != 0
	}
	open := s.State.ShowNpcDialog || s.State.ShowShop
	if s.merchantWasOpen && !open {
		s.clearMerchantState()
	}
	s.merchantWasOpen = open
}

// paintNpcDialog 渲染解析后的富文本及可点击链接
// (FState:4806-4880)。
func (s *PlayScene) paintNpcDialog(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgNpcDlg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	// 每次绘制重建点击区域 (Delphi 只构建一次; 我们需要
	// 实时区域用于命中检测)。
	s.npcClicks = s.npcClicks[:0]
	ax, ay := c.AbsX(), c.AbsY()
	// NPC 名字 (Delphi FState:4810 MerchantName)
	if s.State.NpcDialogName != "" {
		s.text.DrawTextOutline(s.State.NpcDialogName, float32(ax+npcTextX), float32(ay+10),
			1, 1, 0, 1, 0, 0, 0, 1, proj)
	}
	// 滚动窗口：计算最大可见行数
	maxVisible := (c.Height - npcTextY - 10) / npcLineH
	if maxVisible < 1 {
		maxVisible = 1
	}
	start := s.npcScrollOffset
	end := start + maxVisible
	if end > len(s.npcLines) {
		end = len(s.npcLines)
	}
	for i := start; i < end; i++ {
		segs := s.npcLines[i]
		row := i - start
		y := npcTextY + row*npcLineH
		// 居中行：计算总宽度后偏移
		x := npcTextX
		if i < len(s.npcLineCentered) && s.npcLineCentered[i] {
			totalW := 0
			for _, seg := range segs {
				totalW += s.text.MeasureText(seg.text)
			}
			x = (c.Width - totalW) / 2
			if x < npcTextX {
				x = npcTextX
			}
		}
		for _, seg := range segs {
			w := s.text.MeasureText(seg.text)
			if seg.tag != "" {
				r, g, b := 1.0, 1.0, 0.0
				if s.npcSelectTag == seg.tag {
					r, g, b = 1.0, 0.0, 0.0 // 按下链接, clRed (FState:4864-4865)
				}
				s.text.DrawTextOutline(seg.text, float32(ax+x), float32(ay+y),
					float32(r), float32(g), float32(b), 1, 0, 0, 0, 1, proj)
				// 下划线 (Delphi 中链接带 fsUnderline)。
				s.gl.DrawQuadColor(float32(ax+x), float32(ay+y+13), float32(w), 1,
					float32(r), float32(g), float32(b), 1, proj)
				s.npcClicks = append(s.npcClicks, npcClickPoint{
					x: ax + x, y: ay + y, w: w, h: 14, tag: seg.tag,
				})
			} else {
				s.text.DrawTextOutline(seg.text, float32(ax+x), float32(ay+y),
					1, 1, 1, 1, 0, 0, 0, 1, proj)
			}
			x += w
		}
	}
}

// npcClick 命中检测链接并发送选择 (FState:5116-5135)。
func (s *PlayScene) npcClick(x, y int) {
	absX, absY := x+s.hudNpc.AbsX(), y+s.hudNpc.AbsY()
	now := time.Now().UnixMilli()
	for _, cp := range s.npcClicks {
		if absX < cp.x || absX >= cp.x+cp.w || absY < cp.y || absY >= cp.y+cp.h {
			continue
		}
		if now < s.npcLastClickTick { // 5 秒防连点 (FState:5121)
			return
		}
		s.npcLastClickTick = now + 5000
		gSound.PlaySound(sGlassButtonClick)
		s.npcSelectTag = cp.tag
		s.selectNpcTag(cp.tag)
		return
	}
}

// selectNpcTag 发送链接标签; '@@' 前缀标签先弹出输入框
// (ClMain.pas:3094-3110)。
func (s *PlayScene) selectNpcTag(tag string) {
	if strings.HasPrefix(tag, "@@") {
		ShowInput(s, tag, func(ok bool, text string) {
			if ok {
				s.sendNpcSelect(tag + "\r\n" + text)
			}
		})
		return
	}
	s.sendNpcSelect(tag)
}

func (s *PlayScene) sendNpcSelect(tag string) {
	if s.sendMerchantSelect != nil {
		s.sendMerchantSelect(int32(s.State.ShopNpcID), tag)
	}
}

// paintShopMenu 渲染购买列表 (FState:4887-4934)。
func (s *PlayScene) paintShopMenu(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgShopBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	ax, ay := c.AbsX(), c.AbsY()
	// 表头 (FState:4913-4917 购买模式 / 4943-4946 寄存模式), 白色。
	if s.State.ShopMode == 3 {
		s.text.DrawText("保管物品", float32(ax+27), float32(ay+31), 1, 1, 1, 1, proj)
		s.text.DrawText("持久度", float32(ax+164), float32(ay+31), 1, 1, 1, 1, proj)
	} else {
		s.text.DrawText("物品列表", float32(ax+27), float32(ay+31), 1, 1, 1, 1, proj)
		s.text.DrawText("价格", float32(ax+164), float32(ay+31), 1, 1, 1, 1, proj)
		s.text.DrawText("有库存", float32(ax+262), float32(ay+31), 1, 1, 1, 1, proj)
	}
	goods := s.State.ShopGoods
	rows := len(goods) - s.menuTop
	if rows > menuMaxRows {
		rows = menuMaxRows
	}
	for m := 0; m < rows; m++ {
		idx := s.menuTop + m
		g := goods[idx]
		y := ay + 50 + m*menuRowH
		if idx == s.menuIndex {
			// Delphi 用 clRed 绘制 char(7) (:4923-4925); 引擎字体
			// 无该控制字符字形, 以 '>' 替代。
			s.text.DrawText(">", float32(ax+25), float32(y), 1, 0, 0, 1, proj)
		}
		name := g.Name
		if name == "" {
			if def := s.State.ItemDefs[int(g.ItemIdx)]; def != nil {
				name = def.Name
			} else {
				name = "Item#" + strconv.Itoa(int(g.ItemIdx))
			}
		}
		s.text.DrawText(name, float32(ax+38), float32(y), 1, 1, 1, 1, proj)
		if s.State.ShopMode == 3 {
			// 持久度列 (寄存物品的 Dura/DuraMax; Delphi 的
			// Stock/Grade 布局需要服务端商品格式, 归入 B5 批次)。
			dura := ""
			for i := range s.State.StorageItems {
				if s.State.StorageItems[i].MakeIndex == int32(g.Price) {
					it := s.State.StorageItems[i]
					dura = strconv.Itoa(int(it.Dura)) + "/" + strconv.Itoa(int(it.DuraMax))
					break
				}
			}
			s.text.DrawText(dura, float32(ax+170), float32(y), 1, 1, 1, 1, proj)
		} else {
			s.text.DrawText(strconv.Itoa(g.Price)+" 金币", float32(ax+170), float32(y), 1, 1, 1, 1, proj)
			stockStr := strconv.Itoa(g.Stock)
			if g.Stock <= 0 {
				stockStr = "∞"
			}
			s.text.DrawText(stockStr, float32(ax+265), float32(y), 1, 1, 1, 1, proj)
		}
	}
}

// menuRowClick 选择商品行 (FState:4969-5017)。
func (s *PlayScene) menuRowClick(x, y int) {
	if x < 14 || x > 279 || y < 32 {
		return
	}
	idx := (y-32)/menuRowH + s.menuTop
	if idx >= 0 && idx < len(s.State.ShopGoods) {
		s.menuIndex = idx
		gSound.PlaySound(sGlassButtonClick)
	}
}

// buySelected 对选中行执行操作: 购买 (商店) 或取回 (寄存),
// FState:5028-5052 (Go 闭环中无子菜单)。
func (s *PlayScene) buySelected() {
	now := time.Now().UnixMilli()
	if now < s.lastBuyTick {
		return
	}
	s.lastBuyTick = now + 5000
	if s.menuIndex >= 0 && s.menuIndex < len(s.State.ShopGoods) {
		g := s.State.ShopGoods[s.menuIndex]
		if s.State.ShopMode == 3 {
			// 寄存行: "price" 字段携带 MakeIndex (FState:5041-5043)。
			if s.sendTakeBackStorage != nil {
				s.sendTakeBackStorage(int32(g.Price))
			}
			return
		}
		if s.sendBuyItem != nil {
			s.sendBuyItem(int(g.ItemIdx))
		}
	}
}

// paintSellDlg 渲染出售/修理/寄存槽面板 (FState:5164-5188)。
func (s *PlayScene) paintSellDlg(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgSellBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	label := "价格: "
	switch s.State.ShopMode {
	case 1:
		label = "价格: "
	case 2:
		label = "修理: "
	case 3:
		label = "保管物品: "
	}
	s.text.DrawText(label+s.sellPriceStr, float32(c.AbsX()+28), float32(c.AbsY()+31), 1, 1, 1, 1, proj)
}

// sellSpotClick 拿起/放置待出售物品 (FState:5195-5227)。
func (s *PlayScene) sellSpotClick() {
	s.sellPriceStr = ""
	if s.itemMove.Moving {
		mi := s.itemMove.Index
		// 接受背包物品及槽位自身物品的重新放置
		// (FState:5210)。
		if mi >= 0 || mi == moveIdxSellSpot {
			if s.sellItem != nil {
				// 槽位已占用: 交换手持 ↔ 槽位; 光标保留原槽位物品
				// (Index = -99, FState:5212-5216)。
				old := *s.sellItem
				held := s.itemMove.Item
				s.sellItem = &held
				s.itemMove.Item = old
				s.itemMove.Index = moveIdxSellSpot
			} else {
				it := s.itemMove.Item
				s.sellItem = &it
				s.itemMove.End()
			}
			s.queryPrice = true
			s.queryPriceTick = time.Now().UnixMilli()
		}
		return
	}
	if s.sellItem != nil {
		// 取回槽位物品。
		s.playItemClickSound(s.sellItem)
		s.itemMove.Begin(moveIdxSellSpot, s.sellItem)
		s.sellItem = nil
	}
}

// pumpSellQuery 发送延迟价格查询 (ClMain.pas:3513-3521:
// 物品放入槽位 500ms 后)。
func (s *PlayScene) pumpSellQuery() {
	if !s.queryPrice || s.sellItem == nil {
		return
	}
	if time.Now().UnixMilli()-s.queryPriceTick < 500 {
		return
	}
	s.queryPrice = false
	switch s.State.ShopMode {
	case 1: // 出售
		if s.sendQueryPrice != nil {
			s.sendQueryPrice(s.sellItem.MakeIndex)
		}
	case 2: // 修理
		if s.sendQueryRepair != nil {
			s.sendQueryRepair(s.sellItem.MakeIndex)
		}
	}
}

// sellOk 确认出售/修理/寄存 (FState:5251-5264)。物品从槽位移除;
// sellWait 暂存以便 SMUserSellItemFail 时恢复
// (Delphi g_SellDlgItemSellWait, :5253,5260)。
func (s *PlayScene) sellOk() {
	now := time.Now().UnixMilli()
	if now < s.lastBuyTick || s.sellItem == nil {
		return
	}
	s.lastBuyTick = now + 5000
	mi := s.sellItem.MakeIndex
	s.sellWait = s.sellItem
	s.sellItem = nil
	s.sellPriceStr = ""
	switch s.State.ShopMode {
	case 1:
		if s.sendSellItem != nil {
			s.sendSellItem(mi)
		}
	case 2:
		if s.sendRepairItem != nil {
			s.sendRepairItem(mi)
		}
	case 3:
		if s.sendStorageItem != nil {
			s.sendStorageItem(mi)
		}
	}
}

// sellConfirmed 服务端确认出售或寄存后清除待处理物品;
// 背包全量刷新会将其从背包移除。
func (s *PlayScene) sellConfirmed() {
	s.sellItem = nil
	s.sellWait = nil
	s.sellPriceStr = ""
}

// sellFailed 将待处理物品恢复到出售槽位。
func (s *PlayScene) sellFailed() {
	if s.sellWait != nil {
		s.sellItem = s.sellWait
		s.sellWait = nil
	}
}

// clearMerchantState 窗口关闭时重置临时商店/对话框状态。
func (s *PlayScene) clearMerchantState() {
	// 槽位物品从未离开背包 (客户端本地显示), 关闭时只需
	// 丢弃引用 — Delphi AddItemBag 的效果在此无意义
	// (FState:4795-4801)。
	s.sellItem = nil
	s.sellWait = nil
	s.sellPriceStr = ""
	s.queryPrice = false
	s.menuTop = 0
	s.menuIndex = -1
	s.npcLines = nil
	s.npcClicks = nil
}
