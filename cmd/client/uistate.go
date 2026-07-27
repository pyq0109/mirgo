package main

import (
	"fmt"

	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/wil"
)

// Equipment/status panel — port of DStateWin (FState.pas:981-1084 window and
// slot layout, 2804-2998 page contents, 3041-3185 slot icons, 3257-3467
// interactions, 5277-5398 key select dialog).
type stateSlotDef struct {
	slot int // protocol.U*
	name string
	x, y int
	w, h int
}

var stateSlots = []stateSlotDef{
	{protocol.UNecklace, "Necklace", 182, 65, 34, 31},
	{protocol.UHelmet, "Helmet", 115, 73, 18, 18},
	{protocol.URightHand, "Torch", 182, 105, 34, 31},
	{protocol.UArmRingR, "Bracelet R", 25, 141, 34, 31},
	{protocol.UArmRingL, "Bracelet L", 182, 141, 34, 31},
	{protocol.URingR, "Ring R", 25, 180, 34, 31},
	{protocol.URingL, "Ring L", 182, 180, 34, 31},
	{protocol.UWeapon, "Weapon", 47, 70, 47, 87},
	{protocol.UDress, "Dress", 96, 122, 53, 112},
	{protocol.UBujuk, "Amulet", 25, 232, 34, 31},
	{protocol.UBelt, "Belt", 77, 232, 34, 31},
	{protocol.UBoots, "Boots", 128, 232, 34, 31},
	{protocol.UCharm, "Charm", 182, 232, 34, 31},
}

func unpackLoHi(v uint32) (int, int) {
	return int(v & 0xFFFF), int(v >> 16)
}

// stateItemFile resolves the WIL for an equipped-item Looks value
// (ClMain.pas:6179-6210): Looks<10000 → StateItem.wil, else St<N>.wil.
func (s *PlayScene) stateItemFile(looks int) (*wil.File, int) {
	if looks < 10000 {
		return s.resources.StateItem, looks
	}
	return s.resources.GetExtraWil(fmt.Sprintf("St%d.wil", looks/10000)), looks % 10000
}

// equippedLooks resolves the StateItem sprite index for an equipped item —
// its Looks value (FState:3051 etc.), falling back to the raw DB index.
func (s *PlayScene) equippedLooks(item *protocol.UserItem) int {
	if def := s.State.ItemDefs[int(item.WIndex)]; def != nil {
		return int(def.Looks)
	}
	return int(item.WIndex)
}

func (s *PlayScene) buildState() {
	ui := s.ui
	prg := s.resources.Prguse

	win := NewUIControl("DStateWin", KindWindow)
	win.Floating = false // DFM: not draggable
	if prg != nil {
		win.SetImgIndex(prg, ImgStateBg)
	} else {
		win.Width, win.Height = 240, 300
	}
	win.Left = ScreenWidth - win.Width
	win.Top = 0
	win.Visible = false
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintStatePage(c, proj) }
	// Page 3 magic-row hit area on the window body (FState:3187-3198):
	// x∈[33,199], y∈[55,240], row height 37 — clicking a row opens the key
	// binding dialog for that magic.
	win.OnClick = func(c *UIControl, x, y int) {
		if s.State.StatePage != 3 {
			return
		}
		if x >= 33 && x <= 199 && y >= 55 && y <= 240 {
			idx := s.magicPage*5 + (y-55)/37
			if idx < len(s.State.Magics) {
				s.openKeySelDlg(idx)
			}
		}
	}
	ui.Root.AddChild(win)
	s.hudState = win

	// Close / page turn buttons (FState:1076-1084).
	closeBtn := NewUIControl("DCloseState", KindButton)
	closeBtn.Left, closeBtn.Top = 223, 20
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseSmall)
	}
	closeBtn.Width, closeBtn.Height = 14, 20
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.State.ShowEquip = false }
	win.AddChild(closeBtn)

	prev := NewUIControl("DPrevState", KindButton)
	prev.Left, prev.Top = 224, 65
	if prg != nil {
		prev.SetImgIndex(prg, ImgPageUp)
	}
	prev.OnClick = func(c *UIControl, x, y int) {
		s.State.StatePage = (s.State.StatePage + MaxStatePage - 1) % MaxStatePage
	}
	win.AddChild(prev)

	next := NewUIControl("DNextState", KindButton)
	next.Left, next.Top = 224, 190
	if prg != nil {
		next.SetImgIndex(prg, ImgPageDown)
	}
	next.OnClick = func(c *UIControl, x, y int) {
		s.State.StatePage = (s.State.StatePage + 1) % MaxStatePage
	}
	win.AddChild(next)

	// Magic page scroll (FState:1069-1074; only visible on page 3).
	pgUp := NewUIControl("DStPageUp", KindButton)
	pgUp.Left, pgUp.Top = 202, 52
	if prg != nil {
		pgUp.SetImgIndex(prg, ImgScrollUp)
	}
	pgUp.OnClick = func(c *UIControl, x, y int) {
		if s.magicPage > 0 {
			s.magicPage--
		}
	}
	win.AddChild(pgUp)
	s.statePageUp = pgUp

	pgDown := NewUIControl("DStPageDown", KindButton)
	pgDown.Left, pgDown.Top = 202, 212
	if prg != nil {
		pgDown.SetImgIndex(prg, ImgScrollDown)
	}
	pgDown.OnClick = func(c *UIControl, x, y int) {
		maxPage := (len(s.State.Magics) + 4) / 5
		if maxPage > 0 {
			maxPage--
		}
		if s.magicPage < maxPage {
			s.magicPage++
		}
	}
	win.AddChild(pgDown)
	s.statePageDown = pgDown

	// 13 equipment slot buttons (FState:987-1042), page 0 only.
	for _, d := range stateSlots {
		def := d
		btn := NewUIControl("DSW", KindButton)
		btn.Left, btn.Top = def.x, def.y
		btn.Width, btn.Height = def.w, def.h
		btn.Tag = def.slot
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			s.paintEquipSlot(def, c.AbsX(), c.AbsY(), proj)
		}
		btn.OnClick = func(c *UIControl, x, y int) { s.equipSlotClick(def.slot) }
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.equipSlotHover(def, c.AbsX(), c.AbsY()) }
		win.AddChild(btn)
		s.stateSlotBtns[def.slot] = btn
	}

	// 5 magic icon buttons (DStMag1..5, FState:1044-1067: Left=30, Top =
	// 37/82/127/172/216 — the 5th is 211+5, not 37+4*45), page 3 only.
	magTops := [5]int{37, 82, 127, 172, 216}
	for i := 0; i < 5; i++ {
		row := i
		btn := NewUIControl("DStMag", KindButton)
		btn.Left, btn.Top = 30, magTops[row]
		btn.Width, btn.Height = 31, 33
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			idx := s.magicPage*5 + row
			if idx >= len(s.State.Magics) || s.resources.MagIcon == nil {
				return
			}
			iconIdx := int(s.State.Magics[idx].IconIdx)
			if c.Downed {
				iconIdx++
			}
			s.ui.BlitImage(s.resources.MagIcon, iconIdx, c.AbsX(), c.AbsY(), proj)
		}
		btn.OnClick = func(c *UIControl, x, y int) {
			idx := s.magicPage*5 + row
			if idx < len(s.State.Magics) {
				s.openKeySelDlg(idx)
			}
		}
		win.AddChild(btn)
		s.stateMagBtns[i] = btn
	}
}

// syncStateWindow keeps panel + per-page child visibility in sync.
func (s *PlayScene) syncStateWindow() {
	win := s.hudState
	if win == nil {
		return
	}
	win.Visible = s.State.ShowEquip
	page := s.State.StatePage
	for _, btn := range s.stateSlotBtns {
		if btn != nil {
			btn.Visible = page == 0
		}
	}
	for _, btn := range s.stateMagBtns {
		if btn != nil {
			btn.Visible = page == 3
		}
	}
	if s.statePageUp != nil {
		s.statePageUp.Visible = page == 3
	}
	if s.statePageDown != nil {
		s.statePageDown.Visible = page == 3
	}
}

// paintEquipSlot draws the equipped item icon centered in its slot
// (FState:3041-3185): no cell background (baked into [370]), icon centered
// on both axes from the item's Looks sprite.
func (s *PlayScene) paintEquipSlot(def stateSlotDef, ax, ay int, proj [16]float32) {
	item := s.State.UseItems[def.slot]
	if item == nil {
		return
	}
	f, idx := s.stateItemFile(s.equippedLooks(item))
	if f == nil {
		return
	}
	img := f.GetImage(idx)
	tex := s.resources.GetTexture(f, idx)
	if img == nil || img.RGBA == nil || tex == 0 {
		return
	}
	iw, ih := float32(img.Width), float32(img.Height)
	s.gl.DrawQuad(tex, float32(ax)+float32(def.w-int(iw))/2, float32(ay)+float32(def.h-int(ih))/2, iw, ih, proj)
}

// paintStatePage renders the page body inside DStateWin.OnDirectPaint.
func (s *PlayScene) paintStatePage(c *UIControl, proj [16]float32) {
	st := s.State
	prg := s.resources.Prguse
	ax, ay := c.AbsX(), c.AbsY()

	// Panel background.
	if prg != nil {
		s.ui.BlitImage(prg, ImgStateBg, ax, ay, proj)
	}

	switch st.StatePage {
	case 0:
		s.paintPaperDoll(ax, ay, proj)
	case 1:
		s.paintStateStats(ax, ay, proj)
	case 2:
		if prg != nil {
			s.ui.BlitImage(prg, ImgStatePage2Bg, ax, ay, proj)
		}
		s.paintStateDetails(ax, ay, proj)
	case 3:
		if prg != nil {
			s.ui.BlitImage(prg, ImgStatePage3Bg, ax, ay, proj)
		}
		s.paintMagicList(ax, ay, proj)
	}
}

// paintPaperDoll renders body/hair/dress/weapon/helmet layers (FState:2804-2853).
func (s *PlayScene) paintPaperDoll(ax, ay int, proj [16]float32) {
	st := s.State
	prg := s.resources.Prguse
	if prg == nil || s.text == nil {
		return
	}

	// Body (376 male / 377 female).
	body := ImgBodyMale
	if st.Sex == 1 {
		body = ImgBodyFemale
	}
	s.ui.BlitImage(prg, body, ax, ay, proj)

	// Overlay layers share origin (Left+29, Top+74) and each image's own
	// px/py offset (Delphi GetCachedImage ax/ay = our HotX/HotY).
	ox, oy := ax+29, ay+74
	hair := ImgHairMale + st.Hair/2
	if st.Sex == 1 {
		hair = ImgHairFemale + st.Hair/2
	}
	if img := prg.GetImage(hair); img != nil && img.RGBA != nil {
		if t := s.resources.GetTexture(prg, hair); t != 0 {
			s.gl.DrawQuad(t, float32(ox+int(img.HotX)), float32(oy+int(img.HotY)),
				float32(img.Width), float32(img.Height), proj)
		}
	}
	for _, slot := range []int{protocol.UDress, protocol.UWeapon, protocol.UHelmet} {
		item := st.UseItems[slot]
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

	// Name + guild (FState:3028-3035).
	if st.MySelf != nil {
		name := st.MySelf.UserName
		lw := s.text.MeasureText(name)
		s.text.DrawText(name, float32(ax+122-lw/2), float32(ay+23), 1, 1, 1, 1, proj)
	}
	if st.GuildName != "" {
		s.text.DrawText(st.GuildName+" "+st.GuildRank, float32(ax+65), float32(ay+45), 0.75, 0.75, 0.75, 1, proj)
	}
}

// paintStateStats renders page 1 (FState:2855-2870): pure values only — the
// labels are baked into the [370] artwork.
func (s *PlayScene) paintStateStats(ax, ay int, proj [16]float32) {
	if s.text == nil {
		return
	}
	st := s.State
	l, m := float32(ax+137), float32(ay+99)
	row := func(y float32, v uint32) {
		lo, hi := unpackLoHi(v)
		s.text.DrawText(fmt.Sprintf("%d-%d", lo, hi), l, y, 1, 1, 1, 1, proj)
	}
	row(m-22, st.AC)
	row(m+5, st.MAC)
	row(m+34, st.DC)
	row(m+60, st.MC)
	row(m+88, st.SC)
	s.text.DrawText(fmt.Sprintf("%d/%d", st.HP, st.MaxHP), l, m+116, 1, 1, 1, 1, proj)
	s.text.DrawText(fmt.Sprintf("%d/%d", st.MP, st.MaxMP), l, m+144, 1, 1, 1, 1, proj)
}

// paintStateDetails renders page 2 (FState:2871-2930).
func (s *PlayScene) paintStateDetails(ax, ay int, proj [16]float32) {
	if s.text == nil {
		return
	}
	st := s.State
	bbx, bby := float32(ax+60), float32(ay+70)
	mmx := bbx + 85
	silver := [3]float32{0.75, 0.75, 0.75}
	// Labels stay silver; only the value column turns red on overflow
	// (FState:2884-2930).
	line := func(k int, label, value string, warn bool) {
		s.text.DrawText(label, bbx, bby+float32(k*14), silver[0], silver[1], silver[2], 1, proj)
		r, g, b := silver[0], silver[1], silver[2]
		if warn {
			r, g, b = 1, 0.3, 0.3
		}
		s.text.DrawText(value, mmx, bby+float32(k*14), r, g, b, 1, proj)
	}
	expPct := 0.0
	if st.MaxExp > 0 {
		expPct = 100 * float64(st.Exp) / float64(st.MaxExp)
	}
	line(0, "经验值", fmt.Sprintf("%.2f%%", expPct), false)
	line(1, "负重能力", fmt.Sprintf("%d/%d", st.Weight, st.MaxWeight), st.Weight > st.MaxWeight)
	line(2, "装备重量", fmt.Sprintf("%d/%d", st.WearWeight, st.MaxWearWeight), st.WearWeight > st.MaxWearWeight)
	line(3, "手上重量", fmt.Sprintf("%d/%d", st.HandWeight, st.MaxHandWeight), st.HandWeight > st.MaxHandWeight)
	line(4, "准确度", fmt.Sprintf("%d", st.Hit), false)
	line(5, "敏捷度", fmt.Sprintf("%d", st.Speed), false)
	// Recovery/resistance values need a protocol extension (B5); placeholders.
	line(6, "魔法躲避", "+0%", false)
	line(7, "中毒躲避", "+0%", false)
	line(8, "中毒恢复", "+0%", false)
	line(9, "生命恢复", "+0%", false)
	line(10, "魔法恢复", "+0%", false)
}

// paintMagicList renders page 3 text/icons (FState:2931-2998); the icon
// buttons themselves are DStMag1..5 controls.
func (s *PlayScene) paintMagicList(ax, ay int, proj [16]float32) {
	if s.text == nil || s.resources.Prguse == nil {
		return
	}
	bbx, bby := ax, ay
	for m := 0; m < 5; m++ {
		idx := s.magicPage*5 + m
		if idx >= len(s.State.Magics) {
			break
		}
		mag := s.State.Magics[idx]
		y := bby + 42 + m*44
		s.text.DrawText(mag.Name, float32(bbx+68), float32(y), 0.75, 0.75, 0.75, 1, proj)
		// "lv" mark + level.
		s.ui.BlitImage(s.resources.Prguse, ImgMagicLv, bbx+68, y+15, proj)
		s.text.DrawText(fmt.Sprintf("%d", mag.Level), float32(bbx+84), float32(y+15), 0.75, 0.75, 0.75, 1, proj)
		// "exp" mark + train points.
		s.ui.BlitImage(s.resources.Prguse, ImgMagicExp, bbx+94, y+15, proj)
		train := fmt.Sprintf("%d/%d", mag.CurTrain, mag.MaxTrain)
		if mag.Level >= 3 {
			train = "-"
		}
		s.text.DrawText(train, float32(bbx+114), float32(y+15), 0.75, 0.75, 0.75, 1, proj)
		// Key digit image ('1'..'8' → 248..255).
		if mag.Key >= '1' && mag.Key <= '8' {
			s.ui.BlitImage(s.resources.Prguse, ImgKeyDigit1+int(mag.Key-'1'), bbx+169, y+10, proj)
		}
	}
}

// equipSlotClick: drop held item onto the slot, or pick up the equipped
// item (FState:3257-3402).
func (s *PlayScene) equipSlotClick(slot int) {
	st := s.State
	if s.itemMove.Moving {
		if s.itemMove.Index >= 0 && s.itemMove.Item.Def != nil {
			if takeOnSlotMatches(s.itemMove.Item.Def.StdMode, slot) && s.sendTakeOn != nil {
				s.sendTakeOn(s.itemMove.Item.MakeIndex, slot)
				// Optimistic visual: server re-sync confirms.
				if s.itemMove.Index < len(st.BagItems) {
					st.BagItems[s.itemMove.Index] = nil
				}
				it := s.itemMove.Item
				st.UseItems[slot] = &protocol.UserItem{
					MakeIndex: it.MakeIndex,
					WIndex:    it.Idx,
					Dura:      it.Dura,
					DuraMax:   it.DuraMax,
				}
				s.itemMove.End()
			}
		}
		return
	}
	// Not holding anything: pick up the equipped item.
	item := st.UseItems[slot]
	if item == nil {
		return
	}
	st.UseItems[slot] = nil
	s.itemMove.Begin(-(slot + 1), &BagItem{
		Idx:       item.WIndex,
		Dura:      item.Dura,
		DuraMax:   item.DuraMax,
		MakeIndex: item.MakeIndex,
		Def:       st.ItemDefs[int(item.WIndex)],
	})
}

// equipSlotHover shows the equipped item tooltip (FState:3404-3467).
func (s *PlayScene) equipSlotHover(def stateSlotDef, ax, ay int) {
	item := s.State.UseItems[def.slot]
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
		color = [4]float32{1, 0.3, 0.3, 1}
	}
	s.tooltip.Show(ax, ay+def.h, text, color, false)
}

// openKeySelDlg shows the hotkey binding dialog for a magic (DKeySelDlg,
// FState:5277-5398).
func (s *PlayScene) openKeySelDlg(magIdx int) {
	if magIdx >= len(s.State.Magics) {
		return
	}
	mag := s.State.Magics[magIdx]
	prg := s.resources.Prguse

	win := NewUIControl("DKeySelDlg", KindWindow)
	if prg != nil {
		win.SetImgIndex(prg, ImgKeyDlg)
	} else {
		win.Width, win.Height = 320, 170
	}
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if prg != nil {
			s.ui.BlitImage(prg, ImgKeyDlg, c.AbsX(), c.AbsY(), proj)
		}
		if s.resources.MagIcon != nil {
			s.ui.BlitImage(s.resources.MagIcon, int(mag.IconIdx), c.AbsX()+51, c.AbsY()+31, proj)
		}
		if s.text != nil {
			s.text.DrawText(mag.Name+" hotkey", float32(c.AbsX()+95), float32(c.AbsY()+38), 0.75, 0.75, 0.75, 1, proj)
		}
	}

	// Pre-selected key: F1..F8/None only update it (with highlight); the
	// binding is applied when Ok closes the dialog (FState:5277-5398 —
	// DKsF1Click preselects, DKsOkClick just hides, the caller applies).
	selKey := mag.Key

	// F1..F8 buttons, runtime positions (FState:1375-1398).
	xs := []int{57, 89, 121, 153, 192, 224, 256, 288}
	for i, x := range xs {
		key := byte('1' + i)
		img := ImgKeyF1 + i*2
		btn := NewUIControl("DKsF", KindButton)
		btn.Left, btn.Top = x, 78
		if prg != nil {
			btn.SetImgIndex(prg, img)
		}
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			// Face is only blitted when selected or pressed; the idle face
			// is baked into the dialog background (FState:5332-5371, where
			// the +1 pressed variant is commented out).
			if selKey == key || c.Downed {
				s.ui.BlitImage(prg, img, c.AbsX(), c.AbsY(), proj)
			}
		}
		btn.OnClick = func(c *UIControl, x, y int) { selKey = key }
		win.AddChild(btn)
	}

	ok := NewUIControl("DKsOk", KindButton)
	ok.Left, ok.Top = 213, 121
	if prg != nil {
		ok.SetImgIndex(prg, ImgKeyOk)
	}
	ok.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		idx := ImgKeyOk
		if c.Downed {
			idx++
		}
		s.ui.BlitImage(prg, idx, c.AbsX(), c.AbsY(), proj)
	}
	ok.OnClick = func(c *UIControl, x, y int) {
		s.applyMagicKey(magIdx, selKey)
		s.ui.CloseModal(win)
	}
	win.AddChild(ok)

	none := NewUIControl("DKsNone", KindButton)
	none.Left, none.Top = 277, 121
	if prg != nil {
		none.SetImgIndex(prg, ImgKeyNone)
	}
	none.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if selKey == 0 || c.Downed {
			s.ui.BlitImage(prg, ImgKeyNone, c.AbsX(), c.AbsY(), proj)
		}
	}
	none.OnClick = func(c *UIControl, x, y int) { selKey = 0 }
	win.AddChild(none)

	s.ui.ShowModal(win)
}

// applyMagicKey rebinds locally and tells the server: any other magic
// holding the key is explicitly unbound first (ClMain.pas:3520-3532), then
// the new binding is sent.
func (s *PlayScene) applyMagicKey(magIdx int, key byte) {
	if magIdx >= len(s.State.Magics) {
		return
	}
	if key != 0 {
		for i := range s.State.Magics {
			if i != magIdx && s.State.Magics[i].Key == key {
				s.State.Magics[i].Key = 0
				if s.sendMagicKey != nil {
					s.sendMagicKey(int(s.State.Magics[i].MagID), 0)
				}
			}
		}
	}
	s.State.Magics[magIdx].Key = key
	if s.sendMagicKey != nil {
		s.sendMagicKey(int(s.State.Magics[magIdx].MagID), int(key))
	}
}
