package main

import (
	"fmt"
	"time"
)

// 底部 HUD — 移植自 DBottom + DBottomDirectPaint (FState.pas:1179-1189,
// 3560-3708): 两段混合底板、HP/MP 球、经验/负重条、昼夜图标、聊天板;
// 另有 4 个状态按钮 (:1194-1205, :3733-3821)、9 个功能按钮
// (DlgConf MShare:474-494, 处理函数 :5409-5603, 提示 :6739-6770)
// 以及腰带 (:1245-1273, :3836-3920).
const (
	chatBoardX   = 208                // FState:3693
	chatBoardTop = ScreenHeight - 130 // FState:3694 = 470
	chatLineH    = 12                 // FState:3703
)

var beltLefts = [6]int{285, 328, 371, 414, 457, 500} // FState:1245-1273 (+43 steps)

func (s *PlayScene) buildHUD() {
	ui := s.ui
	prg := s.resources.Prguse

	bottom := NewUIControl("DBottom", KindWindow)
	bottom.Left = 0
	if prg != nil {
		bottom.SetImgIndex(prg, ImgBottomBar)
	} else {
		bottom.Width, bottom.Height = ScreenWidth, BottomBarImageH
	}
	// 底部锚定: Top = SCREENHEIGHT - 图像高度 (FState:1184-1189).
	bottom.Top = ScreenHeight - bottom.Height
	bottom.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintBottomBar(c.Top, proj) }
	bottom.OnMouseDown = func(c *UIControl, button, x, y int) { s.bottomMouseDown(x, y) }
	ui.Root.AddChild(bottom)

	// 4 个状态按钮 (DlgConf 坐标, 相对 DBottom). Delphi 只在按下时
	// 绘制它们 — 常态由底板美术呈现 (DMyStateDirectPaint :3733-3747).
	stateDefs := []struct {
		img, x, y, page int
		bag             bool
		hint            string
	}{
		{ImgBtnState, 643, 61, 0, false, "装备(F10,C)"},
		{ImgBtnBag, 682, 41, -1, true, "物品(F9,B)"},
		{ImgBtnMagic, 722, 21, 3, false, "魔法(F11,E)"},
		{ImgBtnOption, 764, 11, -2, false, "声音(F12)"},
	}
	for _, d := range stateDefs {
		btn := NewUIControl("DMyState", KindButton)
		btn.Left, btn.Top = d.x, d.y
		btn.ClickSound = sGlassButtonClick
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
				s.State.ShowBag = true // OpenItemBag = 显示 (FState:3805)
			case page == -2:
				if gSound != nil {
					if gSound.ToggleSFX() {
						s.AddChatMessage("[音乐打开]")
					} else {
						s.AddChatMessage("[音乐关闭]")
					}
				}
			default:
				s.State.StatePage = page
				s.State.ShowEquip = true // OpenMyStatus = 显示 (FState:3801-3809)
			}
		}
		hint := d.hint
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.buttonHint(hint) }
		bottom.AddChild(btn)
	}

	// 沿底栏排列的 9 个功能按钮 (DlgConf, Top=104).
	botDefs := []struct {
		name    string
		img     int
		x       int
		hint    string
		onClick func()
	}{
		{"DBotMiniMap", ImgBotMinimap, 219, "小地图(M)", func() { s.showMinimap = !s.showMinimap }},
		{"DBotTrade", ImgBotTrade, 249, "交易(W)", func() { s.tryDeal() }},
		{"DBotGuild", ImgBotGuild, 279, "行会(G)", func() { s.toggleGuild() }},
		{"DBotGroup", ImgBotGroup, 309, "组队(S)", func() { s.State.ShowGroupDlg = !s.State.ShowGroupDlg }},
		{"DBotPlusAbil", ImgBotPlusAbil, 339, "技能点(N)", func() { s.State.ShowPlusAbil = !s.State.ShowPlusAbil }},
		{"DBotFriend", ImgBotFriend, 369, "好友(V)", func() { s.State.ShowFriend = !s.State.ShowFriend }},
		{"DBotLogout", ImgBotLogout, 530, "选择人物\\Alt-X", func() {
			if s.sendLogout != nil {
				s.sendLogout()
			}
		}},
		{"DBotExit", ImgBotExit, 560, "退出游戏\\Alt-Q", func() {
			if s.sendExit != nil {
				s.sendExit()
			}
		}},
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
				idx = img + 1 // 抬起/按下成对图素的约定
			case isPlus && s.State.ShowPlusAbil:
				idx = img + 2 // 面板打开时常亮 (FState:3779)
			case isPlus && time.Now().UnixMilli()/500%2 == 1:
				idx = img + 2 // 有技能点时闪烁 (FState:3770-3795)
			}
			ui.BlitImage(prg, idx, c.AbsX(), c.AbsY(), proj)
		}
		cb := d.onClick
		btn.OnClick = func(c *UIControl, x, y int) { cb() }
		hint := d.hint
		btn.OnMouseMove = func(c *UIControl, x, y int) { s.buttonHint(hint) }
		bottom.AddChild(btn)
		if isPlus {
			btn.Visible = false // 仅有技能点时显示 (ClMain:3523-3527)
			s.hudPlusAbil = btn
		}
	}

	// HP/MP 悬停按钮: 隐形命中区域, 显示当前/上限提示
	// (DButtonHP/DButtonMP, FState:1727-1735, 6761-6762).
	hpBtn := NewUIControl("DButtonHP", KindButton)
	hpBtn.Left, hpBtn.Top = 40, 91
	hpBtn.Width, hpBtn.Height = 45, 90
	hpBtn.OnMouseMove = func(c *UIControl, x, y int) {
		s.buttonHint(fmt.Sprintf("生命值(%d/%d)", s.State.HP, s.State.MaxHP))
	}
	bottom.AddChild(hpBtn)
	mpBtn := NewUIControl("DButtonMP", KindButton)
	mpBtn.Left, mpBtn.Top = 87, 91
	mpBtn.Width, mpBtn.Height = 45, 90
	mpBtn.OnMouseMove = func(c *UIControl, x, y int) {
		s.buttonHint(fmt.Sprintf("魔法值(%d/%d)", s.State.MP, s.State.MaxMP))
	}
	bottom.AddChild(mpBtn)

	// 腰带: 6 个隐形命中格, 物品图标居中 (FState:1245-1273,
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

// buttonHint 在光标处显示黄色悬停提示 (DBotMouseMove,
// FState:6739-6770; 其中 Local/Surface 换算结果即光标坐标).
func (s *PlayScene) buttonHint(text string) {
	s.tooltip.Show(int(s.mouseX), int(s.mouseY), text, [4]float32{1, 1, 0, 1}, false)
}

// paintBottomBar 绘制底板及 DBottomDirectPaint 内的全部内容
// (FState.pas:3560-3708). barY 是底板控件的绝对顶边.
func (s *PlayScene) paintBottomBar(barY int, proj [16]float32) {
	st := s.State
	prg := s.resources.Prguse
	by := float32(barY)

	if s.hudPlusAbil != nil {
		s.hudPlusAbil.Visible = st.BonusPoint > 0
	}

	// 底板: 上 120px 做颜色键混合 (WIL 解码器已将黑色烘焙为 alpha=0,
	// 因此 alpha 1.0 即可精确复现 DDBLTFAST_SRCCOLORKEY,
	// FState:3577-3586), 下半部分不透明 (:3587-3593).
	barH := float32(ScreenHeight - barY)
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
			s.gl.DrawQuadSub(t, w, h, 0, 0, w, blendH, 0, by, w, blendH, 1, 1, 1, 1, proj)
			if h > blendH {
				s.gl.DrawQuadSub(t, w, h, 0, blendH, w, h-blendH, 0, by+blendH, w, h-blendH, 1, 1, 1, 1, proj)
			}
		} else {
			s.gl.DrawQuadColor(0, by, ScreenWidth, barH, 0.08, 0.08, 0.12, 0.95, proj)
		}
	} else {
		s.gl.DrawQuadColor(0, by, ScreenWidth, barH, 0.08, 0.08, 0.12, 0.95, proj)
	}

	if prg != nil {
		// 昼夜图标 (FState:3597-3604).
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
			s.ui.BlitImage(prg, dayImg, 748, barY+79, proj)
		}

		// HP/MP 球 (FState:3606-3638).
		if st.MaxHP > 0 && st.MaxMP > 0 {
			if st.Job == 0 && st.Level < 28 {
				// 28 级以下战士: 单血条 [5]+[6].
				if base := prg.GetImage(ImgWarHPBase); base != nil && base.RGBA != nil {
					t := s.resources.GetTexture(prg, ImgWarHPBase)
					s.gl.DrawQuadSub(t, float32(base.Width), float32(base.Height),
						0, 0, float32(base.Width)-2, float32(base.Height),
						38, by+90, float32(base.Width)-2, float32(base.Height), 1, 1, 1, 1, proj)
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
						38, by+90+crop, float32(fill.Width)-2, fh-crop, 1, 1, 1, 1, proj)
				}
			} else if orb := prg.GetImage(ImgHPMPBar); orb != nil && orb.RGBA != nil {
				t := s.resources.GetTexture(prg, ImgHPMPBar)
				w, h := float32(orb.Width), float32(orb.Height)
				half := float32(int(w) / 2) // Delphi 整数除法 (FState:3627,3633)
				// HP: 左半边 (宽 half-1), 按缺失比例从顶部裁剪
				// (:3627-3630).
				hpCrop := h / float32(st.MaxHP) * float32(st.MaxHP-st.HP)
				if hpCrop < 0 {
					hpCrop = 0
				}
				s.gl.DrawQuadSub(t, w, h, 0, hpCrop, half-1, h-hpCrop,
					40, by+91+hpCrop, half-1, h-hpCrop, 1, 1, 1, 1, proj)
				// MP: 右半边, rc.Left=half+1, rc.Right=w-1 → 宽
				// half-2 (:3633-3636).
				mpCrop := h / float32(st.MaxMP) * float32(st.MaxMP-st.MP)
				if mpCrop < 0 {
					mpCrop = 0
				}
				s.gl.DrawQuadSub(t, w, h, half+1, mpCrop, half-2, h-mpCrop,
					40+half+1, by+91+mpCrop, half-2, h-mpCrop, 1, 1, 1, 1, proj)
			}
		}

		// 经验条和负重条 (FState:3646-3676), 共用图素 [7].
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
		// 等级 (FState:3643, PomiTextOut 位于 (660, SCREENHEIGHT-104)).
		s.text.DrawTextOutline(fmt.Sprintf("%d", st.Level), 660, ScreenHeight-104,
			1, 1, 1, 1, 0, 0, 0, 1, proj)

		// 聊天板: 9 行 × 12px (FState:3692-3706).
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
			// 不透明黑色行背景 (Delphi SetBkMode OPAQUE 配黑色背景色,
			// FState:3696-3703).
			lw := s.text.MeasureText(msg.Text)
			s.gl.DrawQuadColor(chatBoardX, float32(chatBoardTop+row*chatLineH),
				float32(lw), chatLineH, 0, 0, 0, 1, proj)
			s.text.DrawText(msg.Text, chatBoardX, float32(chatBoardTop+row*chatLineH), r, g, b, 1, proj)
		}

		// 聊天输入行 — EdChat (208,581) 386×12, clSilver 背景、黑色文字,
		// MaxLength 70 (PlayScn.pas:267-280). 模态对话框弹出时隐藏
		// (HideAllControls 会隐藏 VCL TEdit, FState:2084).
		if s.chatMode && s.ui.Modal == nil {
			s.gl.DrawQuadColor(chatBoardX, 581, 386, 12, 0.75, 0.75, 0.75, 1, proj)
			cursor := ""
			if time.Now().UnixMilli()%1000 < 500 {
				cursor = "|"
			}
			s.text.DrawText(s.chatInput+cursor, chatBoardX+2, 581, 0, 0, 0, 1, proj)
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

// paintBeltSlot 绘制腰带格图标 + 格号 (FState:3836-3853).
// 格子本身没有背景 — 格位美术已烘焙在底板图像中; 图标按原始尺寸
// 居中 (不缩放), 格号绘制于 (Left+13, Top+19).
func (s *PlayScene) paintBeltSlot(slot, ax, ay int, proj [16]float32) {
	item := s.State.BeltItems[slot]
	if item != nil && s.resources.Items != nil {
		looks := int(item.Looks())
		if looks >= 0 && looks < s.resources.Items.Count {
			img := s.resources.Items.GetImage(looks)
			tex := s.resources.GetTexture(s.resources.Items, looks)
			if img != nil && img.RGBA != nil && tex != 0 {
				iw, ih := float32(img.Width), float32(img.Height)
				s.gl.DrawQuad(tex, float32(ax)+(32-iw)/2, float32(ay)+(29-ih)/2, iw, ih, proj)
			}
		}
	}
	if s.text != nil {
		s.text.DrawTextOutline(fmt.Sprintf("%d", slot+1), float32(ax)+13, float32(ay)+19,
			1, 1, 1, 1, 0, 0, 0, 1, proj)
	}
}

// beltClick 拾取/放置腰带物品 (FState:3868-3900).
func (s *PlayScene) beltClick(slot int) {
	st := s.State
	if s.itemMove.Moving {
		// 放置: 只有药品类物品可放上腰带 (StdMode <= 3,
		// :3883-3897). 从背包拿起的物品保留其背包格位 (服务端模型);
		// 腰带只保存引用.
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
	s.playItemClickSound(item)
	bagSlot := st.FindBagItemByMakeIndex(item.MakeIndex)
	s.itemMove.Begin(bagSlot, item)
	s.itemMove.FromBelt = slot
	st.BeltItems[slot] = nil
	if bagSlot >= 0 {
		st.BagItems[bagSlot] = nil
	}
}

// beltDblClick 使用该格的物品 (FState:3902-3920). 双击的第一下会拿起
// 物品, 因此通常走手持分支 (:3912-3917).
func (s *PlayScene) beltDblClick(slot int) {
	if s.itemMove.Moving {
		if s.itemMove.FromBelt == slot && s.itemMove.Item.Def != nil {
			stdMode := s.itemMove.Item.Def.StdMode
			if (stdMode <= 4 || stdMode == 31) && s.sendUseItem != nil {
				if idx := itemUseSoundIdx(stdMode); idx >= 0 {
					gSound.PlaySound(idx)
				}
				s.sendUseItem(s.itemMove.Item.MakeIndex)
				s.itemMove.End()
			}
		}
		return
	}
	if item := s.State.BeltItems[slot]; item != nil && item.Def != nil && s.sendUseItem != nil {
		if stdMode := item.Def.StdMode; stdMode <= 4 || stdMode == 31 {
			if idx := itemUseSoundIdx(stdMode); idx >= 0 {
				gSound.PlaySound(idx)
			}
			s.sendUseItem(item.MakeIndex)
		}
	}
}

// beltHint 显示物品提示 (FState:3855-3866).
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

// bottomMouseDown: 点击聊天行预填私聊 (FState:1896-1927).
// 坐标相对 DBottom.
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

// extractUserName 从聊天行中提取发送者名字 (FState:1898-1908).
func extractUserName(line string) string {
	s := line
	if len(s) > 0 && s[0] == '[' { // 跳过 "[频道]" 前缀
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

// tryDeal 发送 CMDealTry (DBotTradeClick, FState:5425-5431).
func (s *PlayScene) tryDeal() {
	if s.sendDealTry != nil {
		s.sendDealTry()
	}
}
