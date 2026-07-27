package main

import (
	"strconv"
	"strings"
	"time"
)

// NPC dialog + shop panels — port of DMerchantDlg (FState.pas:4806-4880
// rich text paint, 5116-5162 click), DMenuDlg buy list (4887-5075) and
// DSellDlg sell/repair/storage spot (5164-5264).
const (
	npcTextX    = 30 // FState:4820
	npcTextY    = 30 // FState:4821
	npcLineH    = 16 // FState:4874
	menuMaxRows = 10 // MAXMENU
	menuRowH    = 13 // LISTLINEHEIGHT
)

type npcSegment struct {
	text string
	tag  string // non-empty = clickable link carrying this value
}

type npcClickPoint struct {
	x, y, w, h int
	tag        string
}

// parseNpcDialog parses the SMMerchantSay body (Delphi tag syntax:
// <display text/link value>, lines separated by '\'; FState:4830-4870).
func (s *PlayScene) parseNpcDialog(body string) {
	s.npcLines = nil
	s.npcClicks = nil
	s.npcSelectTag = ""
	for _, line := range strings.Split(body, "\\") {
		var segs []npcSegment
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
			if tag == "C" || tag == "/C" {
				continue // centering markers (unused in this Delphi revision too)
			}
			if tag == "" {
				// Empty tag hides the shop windows (FState:4847-4850).
				s.State.ShowShop = false
				continue
			}
			// <display text/link value>
			display, link := tag, ""
			if slash := strings.IndexByte(tag, '/'); slash >= 0 {
				display, link = tag[:slash], tag[slash+1:]
			}
			segs = append(segs, npcSegment{text: display, tag: link})
		}
		s.npcLines = append(s.npcLines, segs)
	}
}

func (s *PlayScene) buildNpcPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// --- NPC dialog (DMerchantDlg [384] @0,0) ---
	npc := NewUIControl("DMerchantDlg", KindWindow)
	npc.Floating = false // DFM: not draggable
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
	npcClose.Left, npcClose.Top = 372, 20 // runtime override (FState:1303-1305)
	if prg != nil {
		npcClose.SetImgIndex(prg, ImgCloseSmall)
	}
	npcClose.OnClick = func(c *UIControl, x, y int) { s.State.ShowNpcDialog = false }
	npc.AddChild(npcClose)

	// --- Buy list (DMenuDlg [385], runtime pos 0,170 — FState:4751-4752) ---
	menu := NewUIControl("DMenuDlg", KindWindow)
	menu.Floating = false // DFM: not draggable
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

	// Runtime button layout (FState:1330-1341 — the DlgConf entries are
	// dead values): Prev[387]@(328,42), Next[388]@(328,162),
	// Buy[362]@(100,230), Close[366]@(175,230).
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

	// --- Sell/repair/storage spot (DSellDlg [392], runtime pos 260,170) ---
	sell := NewUIControl("DSellDlg", KindWindow)
	sell.Floating = false // DFM: not draggable
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

	// The spot where the item to sell/repair/store is placed (27,67 61×52).
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
		// Centered in the spot (FState:5235-5241).
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

	// Runtime layout (FState:1352-1357): OK[362]@(28,135), Close[366]@(81,135).
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

// syncMerchantWindows keeps dialog/shop windows visible per state, and
// clears transient state once after everything closes.
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

// paintNpcDialog renders parsed rich text with clickable links
// (FState:4806-4880).
func (s *PlayScene) paintNpcDialog(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgNpcDlg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	// Rebuild click rects each paint (Delphi builds once; we need them
	// live for hit testing).
	s.npcClicks = s.npcClicks[:0]
	ax, ay := c.AbsX(), c.AbsY()
	for i, segs := range s.npcLines {
		x := npcTextX
		y := npcTextY + i*npcLineH
		for _, seg := range segs {
			w := s.text.MeasureText(seg.text)
			if seg.tag != "" {
				r, g, b := 1.0, 1.0, 0.0
				if s.npcSelectTag == seg.tag {
					r, g, b = 1.0, 0.0, 0.0 // pressed link, clRed (FState:4864-4865)
				}
				s.text.DrawTextOutline(seg.text, float32(ax+x), float32(ay+y),
					float32(r), float32(g), float32(b), 1, 0, 0, 0, 1, proj)
				// Underline (links are fsUnderline in Delphi).
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

// npcClick hit-tests links and sends the selection (FState:5116-5135).
func (s *PlayScene) npcClick(x, y int) {
	absX, absY := x+s.hudNpc.AbsX(), y+s.hudNpc.AbsY()
	now := time.Now().UnixMilli()
	for _, cp := range s.npcClicks {
		if absX < cp.x || absX >= cp.x+cp.w || absY < cp.y || absY >= cp.y+cp.h {
			continue
		}
		if now < s.npcLastClickTick { // 5s anti-spam (FState:5121)
			return
		}
		s.npcLastClickTick = now + 5000
		s.npcSelectTag = cp.tag
		s.selectNpcTag(cp.tag)
		return
	}
}

// selectNpcTag sends the link tag; '@@' tags prompt for input first
// (ClMain.pas:3094-3110).
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

// paintShopMenu renders the buy list (FState:4887-4934).
func (s *PlayScene) paintShopMenu(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgShopBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	ax, ay := c.AbsX(), c.AbsY()
	// Headers (FState:4913-4917 buy mode / 4943-4946 storage mode), white.
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
			// Delphi draws char(7) in clRed (:4923-4925); the engine font
			// has no glyph for the control char, so '>' stands in.
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
			// Durability column (Dura/DuraMax of the stored instance; the
			// Delphi Stock/Grade layout needs the server-side goods format,
			// tracked in batch B5).
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
			s.text.DrawText("-", float32(ax+265), float32(y), 1, 1, 1, 1, proj)
		}
	}
}

// menuRowClick selects a goods row (FState:4969-5017).
func (s *PlayScene) menuRowClick(x, y int) {
	if x < 14 || x > 279 || y < 32 {
		return
	}
	idx := (y-32)/menuRowH + s.menuTop
	if idx >= 0 && idx < len(s.State.ShopGoods) {
		s.menuIndex = idx
	}
}

// buySelected acts on the selected row: buy (shop) or take back (storage),
// FState:5028-5052 (no sub-menus in the Go closed loop).
func (s *PlayScene) buySelected() {
	now := time.Now().UnixMilli()
	if now < s.lastBuyTick {
		return
	}
	s.lastBuyTick = now + 5000
	if s.menuIndex >= 0 && s.menuIndex < len(s.State.ShopGoods) {
		g := s.State.ShopGoods[s.menuIndex]
		if s.State.ShopMode == 3 {
			// Storage row: "price" carries the MakeIndex (FState:5041-5043).
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

// paintSellDlg renders the sell/repair/storage spot panel (FState:5164-5188).
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

// sellSpotClick picks up / places the sell item (FState:5195-5227).
func (s *PlayScene) sellSpotClick() {
	s.sellPriceStr = ""
	if s.itemMove.Moving {
		mi := s.itemMove.Index
		// Accept bag items and the spot's own item being re-placed
		// (FState:5210).
		if mi >= 0 || mi == moveIdxSellSpot {
			if s.sellItem != nil {
				// Occupied spot: swap held ↔ spot; the cursor keeps the old
				// spot item (Index = -99, FState:5212-5216).
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
		// Pick the spot item back up.
		s.itemMove.Begin(moveIdxSellSpot, s.sellItem)
		s.sellItem = nil
	}
}

// pumpSellQuery sends the delayed price query (ClMain.pas:3513-3521:
// 500ms after the item lands on the spot).
func (s *PlayScene) pumpSellQuery() {
	if !s.queryPrice || s.sellItem == nil {
		return
	}
	if time.Now().UnixMilli()-s.queryPriceTick < 500 {
		return
	}
	s.queryPrice = false
	switch s.State.ShopMode {
	case 1: // sell
		if s.sendQueryPrice != nil {
			s.sendQueryPrice(s.sellItem.MakeIndex)
		}
	case 2: // repair
		if s.sendQueryRepair != nil {
			s.sendQueryRepair(s.sellItem.MakeIndex)
		}
	}
}

// sellOk confirms sell/repair/storage (FState:5251-5264). The item leaves
// the spot visually; sellWait keeps it so SMUserSellItemFail can restore it
// (Delphi g_SellDlgItemSellWait, :5253,5260).
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

// sellConfirmed drops the pending item after the server accepted the sale
// or stored it; the full bag refresh removes it from the bag.
func (s *PlayScene) sellConfirmed() {
	s.sellItem = nil
	s.sellWait = nil
	s.sellPriceStr = ""
}

// sellFailed restores the pending item to the sell spot.
func (s *PlayScene) sellFailed() {
	if s.sellWait != nil {
		s.sellItem = s.sellWait
		s.sellWait = nil
	}
}

// clearMerchantState resets transient shop/dialog state when windows close.
func (s *PlayScene) clearMerchantState() {
	// The spot item never left the bag (client-owned visual), so closing
	// just drops the references — Delphi AddItemBag's effect is a no-op here
	// (FState:4795-4801).
	s.sellItem = nil
	s.sellWait = nil
	s.sellPriceStr = ""
	s.queryPrice = false
	s.menuTop = 0
	s.menuIndex = -1
	s.npcLines = nil
	s.npcClicks = nil
}
