package main

import (
	"fmt"
	"time"
)

// Bottom HUD — port of DBottom + DBottomDirectPaint (FState.pas:1179-1189,
// 3560-3708): two-part blended board, HP/MP orbs, exp/weight strips, day
// icon, chat board; plus the 4 state buttons (:1194-1205, :3733-3821), the
// 9 function buttons (DlgConf MShare:474-494, handlers :5409-5603, hints
// :6739-6770) and the belt (:1245-1273, :3836-3920).
const (
	chatBoardX   = 208                // FState:3693
	chatBoardTop = ScreenHeight - 130 // FState:3694 = 470
	chatLineH    = 12                 // FState:3703
)

var beltLefts = [6]int{285, 328, 371, 415, 459, 503} // DlgConf DBelt1..6

func (s *PlayScene) buildHUD() {
	ui := s.ui
	prg := s.resources.Prguse

	bottom := NewUIControl("DBottom", KindWindow)
	bottom.Left, bottom.Top = 0, BottomBarTop
	if prg != nil {
		bottom.SetImgIndex(prg, ImgBottomBar)
	} else {
		bottom.Width, bottom.Height = ScreenWidth, BottomBarHeight
	}
	bottom.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintBottomBar(proj) }
	bottom.OnMouseDown = func(c *UIControl, button, x, y int) { s.bottomMouseDown(x, y) }
	ui.Root.AddChild(bottom)

	// 4 state buttons (DlgConf coords, DBottom-relative). Delphi draws them
	// only while pressed — the board artwork carries the resting state
	// (DMyStateDirectPaint :3733-3747).
	stateDefs := []struct {
		img, x, y, page int
		bag             bool
		hint            string
	}{
		{ImgBtnState, 643, 61, 0, false, "Equip(F10,C)"},
		{ImgBtnBag, 682, 41, -1, true, "Bag(F9,B)"},
		{ImgBtnMagic, 722, 21, 3, false, "Magic(F11,E)"},
		{ImgBtnOption, 764, 11, -2, false, "Sound(F12)"},
	}
	for _, d := range stateDefs {
		btn := NewUIControl("DMyState", KindButton)
		btn.Left, btn.Top = d.x, d.y
		if prg != nil {
			btn.SetImgIndex(prg, d.img)
		}
		img := d.img
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if c.Downed {
				ui.BlitImage(prg, img, c.AbsX(), c.AbsY(), proj)
			}
		}
		page, bag := d.page, d.bag
		btn.OnClick = func(c *UIControl, x, y int) {
			switch {
			case bag:
				s.State.ShowBag = true // OpenItemBag = show (FState:3805)
			case page == -2:
				s.addChatMessage("[Sound] toggled (audio not implemented)")
			default:
				s.State.StatePage = page
				s.State.ShowEquip = true // OpenMyStatus = show (FState:3801-3809)
			}
		}
		hint := d.hint
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.buttonHint(hint) }
		bottom.AddChild(btn)
	}

	// 9 function buttons along the bar (DlgConf, Top=104).
	botDefs := []struct {
		name    string
		img     int
		x       int
		hint    string
		onClick func()
	}{
		{"DBotMiniMap", ImgBotMinimap, 219, "Minimap(M)", func() { s.showMinimap = !s.showMinimap }},
		{"DBotTrade", ImgBotTrade, 249, "Trade(W)", func() { s.tryDeal() }},
		{"DBotGuild", ImgBotGuild, 279, "Guild(G)", func() { s.toggleGuild() }},
		{"DBotGroup", ImgBotGroup, 309, "Group(S)", func() { s.State.ShowGroupDlg = !s.State.ShowGroupDlg }},
		{"DBotPlusAbil", ImgBotPlusAbil, 339, "Bonus(N)", func() { s.State.ShowPlusAbil = !s.State.ShowPlusAbil }},
		{"DBotFriend", ImgBotFriend, 369, "Friend(V)", func() { s.addChatMessage("Friends: not implemented yet") }},
		{"DBotLogout", ImgBotLogout, 530, "Char select (Alt-X)", func() { s.addChatMessage("Logout: not implemented yet") }},
		{"DBotExit", ImgBotExit, 560, "Quit game (Alt-Q)", func() { s.addChatMessage("Exit: not implemented yet") }},
	}
	for _, d := range botDefs {
		btn := NewUIControl(d.name, KindButton)
		btn.Left, btn.Top = d.x, 104
		if prg != nil {
			btn.SetImgIndex(prg, d.img)
		}
		img := d.img
		isPlus := d.name == "DBotPlusAbil"
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			idx := img
			switch {
			case c.Downed:
				idx = img + 1 // up/down pair convention
			case isPlus && time.Now().UnixMilli()/500%2 == 1:
				idx = img + 2 // bonus-point blink (FState:3770-3795)
			}
			ui.BlitImage(prg, idx, c.AbsX(), c.AbsY(), proj)
		}
		cb := d.onClick
		btn.OnClick = func(c *UIControl, x, y int) { cb() }
		hint := d.hint
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.buttonHint(hint) }
		bottom.AddChild(btn)
		if isPlus {
			btn.Visible = false // shown only when bonus points exist (ClMain:3523-3527)
			s.hudPlusAbil = btn
		}
	}

	// Belt: 6 invisible hit cells with centered item icons (FState:1245-1273,
	// 3836-3920).
	for i := 0; i < 6; i++ {
		slot := i
		btn := NewUIControl("DBelt", KindButton)
		btn.Left, btn.Top = beltLefts[slot], 59
		btn.Width, btn.Height = 32, 29
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			s.paintBeltSlot(slot, c.AbsX(), c.AbsY(), proj)
		}
		btn.OnClick = func(c *UIControl, x, y int) { s.beltClick(slot) }
		btn.OnDblClick = func(c *UIControl, x, y int) { s.beltDblClick(slot) }
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.beltHint(slot, c.AbsX(), c.AbsY()) }
		bottom.AddChild(btn)
	}
}

// buttonHint shows a yellow hover hint at the cursor (DBotMouseMove,
// FState:6739-6770).
func (s *PlayScene) buttonHint(text string) {
	s.tooltip.Show(int(s.mouseX), int(s.mouseY)+16, text, [4]float32{1, 1, 0, 1}, false)
}

// paintBottomBar renders the board and everything painted inside
// DBottomDirectPaint (FState.pas:3560-3708). Absolute coords.
func (s *PlayScene) paintBottomBar(proj [16]float32) {
	st := s.State
	prg := s.resources.Prguse
	barY := float32(BottomBarTop)

	if s.hudPlusAbil != nil {
		s.hudPlusAbil.Visible = st.BonusPoint > 0
	}

	// Board: upper 120px blended, lower part opaque (FState:3577-3594).
	if prg != nil {
		img := prg.GetImage(ImgBottomBar)
		tex := prg != nil && img != nil && img.RGBA != nil
		if tex {
			t := s.resources.GetTexture(prg, ImgBottomBar)
			w, h := float32(img.Width), float32(img.Height)
			blendH := float32(120)
			if blendH > h {
				blendH = h
			}
			s.gl.DrawQuadSub(t, w, h, 0, 0, w, blendH, 0, barY, w, blendH, 1, 1, 1, 0.5, proj)
			if h > blendH {
				s.gl.DrawQuadSub(t, w, h, 0, blendH, w, h-blendH, 0, barY+blendH, w, h-blendH, 1, 1, 1, 1, proj)
			}
		} else {
			s.gl.DrawQuadColor(0, barY, ScreenWidth, BottomBarHeight, 0.08, 0.08, 0.12, 0.95, proj)
		}
	} else {
		s.gl.DrawQuadColor(0, barY, ScreenWidth, BottomBarHeight, 0.08, 0.08, 0.12, 0.95, proj)
	}

	if prg != nil {
		// Day/night icon (FState:3597-3604).
		dayImg := 0
		switch st.DayBright {
		case 0:
			dayImg = ImgDayNight
		case 1:
			dayImg = ImgDayMorning
		case 2:
			dayImg = ImgDayNoon
		case 3:
			dayImg = ImgDayDusk
		}
		if dayImg != 0 {
			s.ui.BlitImage(prg, dayImg, 748, int(barY)+79, proj)
		}

		// HP/MP orbs (FState:3606-3638).
		if st.MaxHP > 0 && st.MaxMP > 0 {
			if st.Job == 0 && st.Level < 28 {
				// Warrior below level 28: single HP column [5]+[6].
				if base := prg.GetImage(ImgWarHPBase); base != nil && base.RGBA != nil {
					t := s.resources.GetTexture(prg, ImgWarHPBase)
					s.gl.DrawQuadSub(t, float32(base.Width), float32(base.Height),
						0, 0, float32(base.Width)-2, float32(base.Height),
						38, barY+90, float32(base.Width)-2, float32(base.Height), 1, 1, 1, 1, proj)
				}
				if fill := prg.GetImage(ImgWarHPFill); fill != nil && fill.RGBA != nil {
					t := s.resources.GetTexture(prg, ImgWarHPFill)
					fh := float32(fill.Height)
					crop := fh / float32(st.MaxHP) * float32(st.MaxHP-st.HP)
					if crop < 0 {
						crop = 0
					}
					s.gl.DrawQuadSub(t, float32(fill.Width), fh,
						0, crop, float32(fill.Width)-2, fh-crop,
						38, barY+90+crop, float32(fill.Width)-2, fh-crop, 1, 1, 1, 1, proj)
				}
			} else if orb := prg.GetImage(ImgHPMPBar); orb != nil && orb.RGBA != nil {
				t := s.resources.GetTexture(prg, ImgHPMPBar)
				w, h := float32(orb.Width), float32(orb.Height)
				half := w / 2
				// HP: left half, cropped from the top by the missing ratio.
				hpCrop := h / float32(st.MaxHP) * float32(st.MaxHP-st.HP)
				if hpCrop < 0 {
					hpCrop = 0
				}
				s.gl.DrawQuadSub(t, w, h, 0, hpCrop, half-1, h-hpCrop,
					40, barY+91+hpCrop, half-1, h-hpCrop, 1, 1, 1, 1, proj)
				// MP: right half.
				mpCrop := h / float32(st.MaxMP) * float32(st.MaxMP-st.MP)
				if mpCrop < 0 {
					mpCrop = 0
				}
				s.gl.DrawQuadSub(t, w, h, half+1, mpCrop, half-1, h-mpCrop,
					40+half+1, barY+91+mpCrop, half-1, h-mpCrop, 1, 1, 1, 1, proj)
			}
		}

		// Exp and weight strips (FState:3646-3676), shared image [7].
		if strip := prg.GetImage(ImgStripBar); strip != nil && strip.RGBA != nil {
			t := s.resources.GetTexture(prg, ImgStripBar)
			sw, sh := float32(strip.Width), float32(strip.Height)
			if st.MaxExp > 0 {
				ew := sw * float32(st.Exp) / float32(st.MaxExp)
				if ew > sw {
					ew = sw
				}
				s.gl.DrawQuadSub(t, sw, sh, 0, 0, ew, sh, 666, 527, ew, sh, 1, 1, 1, 1, proj)
			}
			if st.MaxWeight > 0 {
				ww := sw * float32(st.Weight) / float32(st.MaxWeight)
				if ww > sw {
					ww = sw
				}
				s.gl.DrawQuadSub(t, sw, sh, 0, 0, ww, sh, 666, 560, ww, sh, 1, 1, 1, 1, proj)
			}
		}
	}

	if s.text != nil {
		// Level (FState:3643, PomiTextOut at (660, SCREENHEIGHT-104)).
		s.text.DrawTextOutline(fmt.Sprintf("%d", st.Level), 660, ScreenHeight-104,
			1, 1, 1, 1, 0, 0, 0, 1, proj)

		// Skill quick bar (Go extension; Delphi manages F-keys via the state
		// panel). 8 slots above the belt.
		if s.resources.MagIcon != nil {
			for i := 0; i < 8; i++ {
				sx := 285 + float32(i)*38
				sy := float32(468)
				s.gl.DrawQuadColor(sx, sy, 34, 34, 0.1, 0.1, 0.15, 0.5, proj)
				var mag *LearnedMagic
				for j := range st.Magics {
					if st.Magics[j].Key == byte('1'+i) {
						mag = &st.Magics[j]
						break
					}
				}
				if mag != nil {
					idx := int(mag.IconIdx)
					if mimg := s.resources.MagIcon.GetImage(idx); mimg != nil && mimg.RGBA != nil {
						if t := s.resources.GetTexture(s.resources.MagIcon, idx); t != 0 {
							s.gl.DrawQuad(t, sx+1, sy+1, 32, 32, proj)
						}
					}
				}
				s.text.DrawText(fmt.Sprintf("F%d", i+1), sx+2, sy+22, 0.7, 0.7, 0.7, 1, proj)
			}
		}

		// Chat board: 9 lines × 12px (FState:3692-3706).
		total := len(s.chatMessages)
		end := total - s.chatScroll
		if end > total {
			end = total
		}
		start := end - ViewChatLine
		if start < 0 {
			start = 0
		}
		for i, row := start, 0; i < end; i, row = i+1, row+1 {
			msg := s.chatMessages[i]
			r, g, b := s.chatColor(msg.Text)
			s.text.DrawText(msg.Text, chatBoardX, float32(chatBoardTop+row*chatLineH), r, g, b, 1, proj)
		}

		// Chat input line.
		if s.chatMode {
			s.gl.DrawQuadColor(chatBoardX, 582, 384, 16, 0, 0, 0, 0.7, proj)
			cursor := ""
			if time.Now().UnixMilli()%1000 < 500 {
				cursor = "|"
			}
			s.text.DrawText(s.chatInput+cursor, chatBoardX+2, 583, 1, 1, 1, 1, proj)
		}
	}

	// Attack mode indicator (Go extension; kept inside the bar now).
	if s.text != nil {
		amNames := []string{"Peace", "Group", "Guild", "All", "PK"}
		amColors := [][4]float32{{0.5, 1, 0.5, 1}, {0.5, 0.5, 1, 1}, {0.5, 1, 1, 1}, {1, 1, 1, 1}, {1, 0.3, 0.3, 1}}
		if st.AttackMode >= 0 && st.AttackMode < len(amNames) {
			c := amColors[st.AttackMode]
			s.text.DrawText("["+amNames[st.AttackMode]+"]", 10, 582, c[0], c[1], c[2], 1, proj)
		}
	}
}

func (s *PlayScene) chatColor(text string) (r, g, b float32) {
	switch {
	case hasPrefix(text, "[系统]"):
		return 1.0, 0.5, 0.5
	case hasPrefix(text, "[行会]"):
		return 0.5, 1.0, 0.5
	case hasPrefix(text, "[组队]"):
		return 0.5, 0.5, 1.0
	case hasPrefix(text, "[私聊]"):
		return 1.0, 0.5, 1.0
	case hasPrefix(text, "[喊话]"):
		return 1.0, 1.0, 0.0
	default:
		return 1.0, 1.0, 0.8
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// paintBeltSlot draws the belt cell icon + slot number (FState:3836-3853).
func (s *PlayScene) paintBeltSlot(slot, ax, ay int, proj [16]float32) {
	s.gl.DrawQuadColor(float32(ax), float32(ay), 32, 29, 0.1, 0.1, 0.15, 0.4, proj)
	item := s.State.BeltItems[slot]
	if item != nil && s.resources.Items != nil {
		looks := int(item.Looks())
		if looks >= 0 && looks < s.resources.Items.Count {
			img := s.resources.Items.GetImage(looks)
			tex := s.resources.GetTexture(s.resources.Items, looks)
			if img != nil && img.RGBA != nil && tex != 0 {
				iw, ih := float32(img.Width), float32(img.Height)
				if iw > 28 {
					iw = 28
				}
				if ih > 25 {
					ih = 25
				}
				s.gl.DrawQuad(tex, float32(ax)+(32-iw)/2, float32(ay)+(29-ih)/2, iw, ih, proj)
			}
		}
	}
	if s.text != nil {
		s.text.DrawText(fmt.Sprintf("%d", slot+1), float32(ax)+2, float32(ay)+17, 0.6, 0.6, 0.6, 1, proj)
	}
}

// beltClick picks up / places belt items (FState:3868-3900).
func (s *PlayScene) beltClick(slot int) {
	st := s.State
	if s.itemMove.Moving {
		// Placing: only drug-like items go on the belt (StdMode <= 3,
		// :3883-3897). Held items from the bag keep their bag slot (server
		// model); the belt holds a reference.
		if s.itemMove.Index >= 0 && s.itemMove.Item.Def != nil && s.itemMove.Item.Def.StdMode <= 3 {
			it := s.itemMove.Item
			st.BeltItems[slot] = &it
			s.itemMove.End()
		}
		return
	}
	item := st.BeltItems[slot]
	if item == nil {
		return
	}
	bagSlot := st.FindBagItemByMakeIndex(item.MakeIndex)
	s.itemMove.Begin(bagSlot, item)
	s.itemMove.FromBelt = slot
	st.BeltItems[slot] = nil
	if bagSlot >= 0 {
		st.BagItems[bagSlot] = nil
	}
}

// beltDblClick uses the item (FState:3902-3920).
func (s *PlayScene) beltDblClick(slot int) {
	if item := s.State.BeltItems[slot]; item != nil && s.sendUseItem != nil {
		s.sendUseItem(item.MakeIndex)
	}
}

// beltHint shows the item tooltip (FState:3855-3866).
func (s *PlayScene) beltHint(slot, ax, ay int) {
	item := s.State.BeltItems[slot]
	if item == nil {
		return
	}
	text, useable := GetMouseItemInfo(s.State, item)
	color := [4]float32{1, 1, 1, 1}
	if !useable {
		color = [4]float32{1, 0.3, 0.3, 1}
	}
	s.tooltip.Show(ax, ay+29, text, color, false)
}

// bottomMouseDown: clicking a chat line prefills a whisper (FState:1896-1927).
// Coords are DBottom-relative.
func (s *PlayScene) bottomMouseDown(x, y int) {
	absY := y + BottomBarTop
	if x < chatBoardX || x > chatBoardX+474 || absY < chatBoardTop || absY >= chatBoardTop+chatLineH*ViewChatLine {
		return
	}
	row := (absY - chatBoardTop) / chatLineH
	total := len(s.chatMessages)
	end := total - s.chatScroll
	start := end - ViewChatLine
	if start < 0 {
		start = 0
	}
	idx := start + row
	if idx < 0 || idx >= total {
		return
	}
	name := extractUserName(s.chatMessages[idx].Text)
	if name == "" {
		return
	}
	s.chatInput = "/" + name + " "
	s.chatMode = true
}

// extractUserName pulls a sender name out of a chat line (FState:1898-1908).
func extractUserName(line string) string {
	s := line
	if len(s) > 0 && s[0] == '[' { // skip "[channel]" prefix
		if i := indexByte(s, ']'); i >= 0 {
			s = s[i+1:]
		}
	}
	end := len(s)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', ':', '=', '!', '(', ')', '*', '/', '[':
			end = i
		}
		if end != len(s) {
			break
		}
	}
	name := s[:end]
	if name == "" {
		return ""
	}
	switch name[0] {
	case '/', '(', ' ', '[':
		return ""
	}
	return name
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// tryDeal sends CMDealTry (DBotTradeClick, FState:5425-5431).
func (s *PlayScene) tryDeal() {
	if s.sendDealTry != nil {
		s.sendDealTry()
	}
}
