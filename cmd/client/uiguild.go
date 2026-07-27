package main

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Guild panel — port of DGuildDlg (FState.pas:1495-1552 layout,
// 6187-6485 behavior) and group dialog DGroupDlg (:1435-1455,
// 5461-5566).

func (s *PlayScene) buildGuildPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// --- Guild panel [180] @(0,-3) ---
	guild := NewUIControl("DGuildDlg", KindWindow)
	guild.Floating = false // DFM: not draggable
	if prg != nil {
		guild.SetImgIndex(prg, ImgGuildBg)
	} else {
		guild.Width, guild.Height = 600, 440
	}
	guild.Left, guild.Top = 0, -3
	guild.Visible = false
	guild.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintGuildPanel(c, proj) }
	ui.Root.AddChild(guild)
	s.hudGuild = guild

	guildBtns := []struct {
		name    string
		img     int
		x, y    int
		admin   bool // hidden unless commander
		onClick func()
	}{
		{"DGDClose", ImgCloseSmall, 568, 20, false, func() {
			// Closing also leaves guild-chat mode (FState:6364-6368).
			s.State.ShowGuild = false
			s.guildChatMode = false
		}},
		{"DGDHome", 198, 27, 387, false, func() { s.sendGuildHomeSafe() }},
		{"DGDList", 200, 27, 407, false, func() { s.sendGuildMemberListSafe() }},
		{"DGDChat", 190, 112, 407, false, func() { s.guildChatMode = !s.guildChatMode }},
		{"DGDAddMem", 182, 235, 387, true, func() {
			ShowInput(s, "输入要加入的成员名字", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildAdd != nil {
					s.sendGuildAdd(text)
				}
			})
		}},
		{"DGDDelMem", 192, 235, 407, true, func() {
			ShowInput(s, "输入要删除的成员名字", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildDel != nil {
					s.sendGuildDel(text)
				}
			})
		}},
		{"DGDEditNotice", 196, 320, 387, true, func() { s.openGuildNoticeEditor() }},
		{"DGDEditGrade", 194, 320, 407, true, func() { s.openGuildRankEditor() }},
		{"DGDAlly", 184, 404, 387, true, func() {
			ShowConfirm(s, "确定要与对方行会结盟吗?", []ModalResult{MrYes, MrCancel}, DlgNormal, func(mr ModalResult) {
				if mr != MrYes {
					return
				}
				ShowInput(s, "输入结盟行会名字", func(ok bool, text string) {
					if ok && text != "" && s.sendGuildAlly != nil {
						s.sendGuildAlly(text)
					}
				})
			})
		}},
		{"DGDBreakAlly", 186, 404, 407, true, func() {
			ShowInput(s, "输入取消结盟的行会名字", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildBreakAlly != nil {
					s.sendGuildBreakAlly(text)
				}
			})
		}},
		// War buttons are dead in the original (no click handlers anywhere in
		// FState.pas); kept visible for commander parity, no behavior.
		{"DGDWar", 202, 488, 387, true, nil},
		{"DGDCancelWar", 188, 488, 407, true, nil},
	}
	for _, d := range guildBtns {
		def := d
		btn := NewUIControl(def.name, KindButton)
		btn.Left, btn.Top = def.x, def.y
		if prg != nil {
			btn.SetImgIndex(prg, def.img)
		} else {
			btn.Width, btn.Height = 40, 18
		}
		if def.onClick != nil {
			btn.OnClick = func(c *UIControl, x, y int) { def.onClick() }
		}
		guild.AddChild(btn)
		if def.admin {
			s.guildAdminBtns = append(s.guildAdminBtns, btn)
		}
	}

	// Member list scroll (FState:1538-1543).
	up := NewUIControl("DGDUp", KindButton)
	up.Left, up.Top = 567, 110
	if prg != nil {
		up.SetImgIndex(prg, ImgScrollUp)
	}
	up.OnClick = func(c *UIControl, x, y int) {
		s.State.GuildTopLine -= 3
		if s.State.GuildTopLine < 0 {
			s.State.GuildTopLine = 0
		}
	}
	guild.AddChild(up)

	down := NewUIControl("DGDDown", KindButton)
	down.Left, down.Top = 567, 364
	if prg != nil {
		down.SetImgIndex(prg, ImgScrollDown)
	}
	down.OnClick = func(c *UIControl, x, y int) {
		if s.State.GuildTopLine+12 < len(s.State.GuildMembers) {
			s.State.GuildTopLine += 3
		}
	}
	guild.AddChild(down)

	// --- Group dialog [120] centered ---
	group := NewUIControl("DGroupDlg", KindWindow)
	group.Floating = false // DFM: not draggable
	if prg != nil {
		group.SetImgIndex(prg, ImgGroupBg)
	} else {
		group.Width, group.Height = 320, 290
	}
	group.Left = (ScreenWidth - group.Width) / 2
	group.Top = (ScreenHeight - group.Height) / 2
	group.Visible = false
	group.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintGroupPanel(c, proj) }
	ui.Root.AddChild(group)
	s.hudGroup = group

	groupClose := NewUIControl("DGrpDlgClose", KindButton)
	groupClose.Left, groupClose.Top = 296, 21
	if prg != nil {
		groupClose.SetImgIndex(prg, ImgCloseSmall)
	}
	groupClose.OnClick = func(c *UIControl, x, y int) { s.State.ShowGroupDlg = false }
	group.AddChild(groupClose)

	allow := NewUIControl("DGrpAllowGroup", KindButton)
	allow.Left, allow.Top = 147, 30
	if prg != nil {
		allow.SetImgIndex(prg, ImgGroupAllow)
	}
	allow.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		// Downed → FaceIndex-1 (346); else AllowGroup → FaceIndex; else
		// nothing (baked background) — FState:5494-5512.
		if prg == nil {
			return
		}
		switch {
		case c.Downed:
			s.ui.BlitImage(prg, ImgGroupAllow-1, c.AbsX(), c.AbsY(), proj)
		case s.State.AllowGroup:
			s.ui.BlitImage(prg, ImgGroupAllow, c.AbsX(), c.AbsY(), proj)
		}
	}
	allow.OnClick = func(c *UIControl, x, y int) {
		if s.groupThrottled() {
			return
		}
		s.State.AllowGroup = !s.State.AllowGroup
		if s.sendGroupMode != nil {
			v := 0
			if s.State.AllowGroup {
				v = 1
			}
			s.sendGroupMode(v)
		}
	}
	group.AddChild(allow)

	groupBtns := []struct {
		name    string
		img     int
		x       int
		onClick func()
	}{
		{"DGrpCreate", ImgGroupCreate, 46, func() {
			if len(s.State.GroupMembers) > 0 { // must be ungrouped (FState:5527)
				return
			}
			ShowInput(s, "输入要邀请的玩家(空=独自组队)", func(ok bool, text string) {
				if ok && s.sendCreateGroup != nil {
					s.sendCreateGroup(text)
				}
			})
		}},
		{"DGrpAddMem", ImgGroupAdd, 123, func() {
			if len(s.State.GroupMembers) == 0 { // must be grouped (FState:5542)
				return
			}
			ShowInput(s, "输入要加入的成员名字", func(ok bool, text string) {
				if ok && text != "" && s.sendAddGroupMember != nil {
					s.sendAddGroupMember(text)
				}
			})
		}},
		{"DGrpDelMem", ImgGroupDel, 202, func() {
			if len(s.State.GroupMembers) == 0 { // must be grouped (FState:5557)
				return
			}
			ShowInput(s, "输入要删除的成员名字", func(ok bool, text string) {
				if ok && text != "" && s.sendDelGroupMember != nil {
					s.sendDelGroupMember(text)
				}
			})
		}},
	}
	for _, d := range groupBtns {
		def := d
		btn := NewUIControl(def.name, KindButton)
		btn.Left, btn.Top = def.x, 255
		if prg != nil {
			btn.SetImgIndex(prg, def.img)
		} else {
			btn.Width, btn.Height = 60, 22
		}
		btn.OnClick = func(c *UIControl, x, y int) {
			if s.groupThrottled() {
				return
			}
			def.onClick()
		}
		group.AddChild(btn)
	}
}

// groupThrottled enforces the shared 5s gate on all party operations
// (Delphi g_dwChangeGroupModeTick, FState:5514-5520).
func (s *PlayScene) groupThrottled() bool {
	now := time.Now().UnixMilli()
	if now < s.guildActionTick {
		return true
	}
	s.guildActionTick = now + 5000
	return false
}

func (s *PlayScene) syncGuildWindows() {
	if s.hudGuild != nil {
		s.hudGuild.Visible = s.State.ShowGuild
		for _, btn := range s.guildAdminBtns {
			btn.Visible = s.State.GuildCommander
		}
	}
	if s.hudGroup != nil {
		s.hudGroup.Visible = s.State.ShowGroupDlg
	}
}

// paintGuildPanel renders guild name + member/chat list (FState:6318-6351).
// Chat mode swaps in the guild chat buffer (FState:6477-6485).
func (s *PlayScene) paintGuildPanel(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgGuildBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	st := s.State
	s.text.DrawText(st.GuildName, float32(c.AbsX()+320), float32(c.AbsY()+13), 1, 1, 1, 1, proj)
	bx, by := c.AbsX()+24, c.AbsY()+41
	if s.guildChatMode {
		for i := 0; i < len(s.guildChats); i++ {
			if i*14 > 356 {
				break
			}
			s.text.DrawText(s.guildChats[i], float32(bx), float32(by+i*14), 0.2, 0.8, 0.2, 1, proj)
		}
		return
	}
	// Notice first (Delphi folds it into the list as a <Notice> segment;
	// the Go server format lacks segments, so it renders as a header block).
	row := 0
	if st.GuildNotice != "" {
		s.text.DrawText("<公告>", float32(bx), float32(by), 1, 1, 1, 1, proj)
		row++
		for _, ln := range strings.Split(st.GuildNotice, "\n") {
			if row*14 > 356 {
				break
			}
			s.text.DrawText(ln, float32(bx), float32(by+row*14), 0.75, 0.75, 0.75, 1, proj)
			row++
		}
	}
	for i := st.GuildTopLine; i < len(st.GuildMembers); i++ {
		if row*14 > 356 {
			break
		}
		name, rank, online := parseGuildMemberLine(st.GuildMembers[i])
		r, g, b := float32(0.75), float32(0.75), float32(0.75)
		if online {
			r, g, b = 0.9, 0.9, 0.9
		}
		s.text.DrawText(name+" ["+rank+"]", float32(bx), float32(by+row*14), r, g, b, 1, proj)
		row++
	}
}

func parseGuildMemberLine(line string) (name, rank string, online bool) {
	parts := strings.Split(line, "/")
	if len(parts) > 0 {
		name = parts[0]
	}
	if len(parts) > 1 {
		rank = parts[1]
	}
	if len(parts) > 2 {
		online = parts[2] == "1"
	}
	return
}

// paintGroupPanel renders the member list (FState:5461-5487): leader first,
// then two columns.
func (s *PlayScene) paintGroupPanel(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgGroupBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil || len(s.State.GroupMembers) == 0 {
		return
	}
	s.text.DrawText(s.State.GroupMembers[0], float32(c.AbsX()+28), float32(c.AbsY()+80), 0.75, 0.75, 0.75, 1, proj)
	for n := 1; n < len(s.State.GroupMembers); n++ {
		lx := c.AbsX() + 28 + ((n-1)%2)*100
		ly := c.AbsY() + 96 + ((n-1)/2)*16
		s.text.DrawText(s.State.GroupMembers[n], float32(lx), float32(ly), 0.75, 0.75, 0.75, 1, proj)
	}
}

// toggleGuild closes the panel, or asks the server for the guild overview
// when opening (Delphi DBotGuildClick sends CMOpenGuildDlg, :5433-5442).
func (s *PlayScene) toggleGuild() {
	if s.State.ShowGuild {
		s.State.ShowGuild = false
		return
	}
	if s.sendOpenGuild != nil {
		s.sendOpenGuild()
	} else {
		s.State.ShowGuild = true
	}
}

// sendGuildHomeSafe / sendGuildMemberListSafe share a 3s query gate and
// leave guild-chat mode (Delphi g_dwQueryMsgTick, FState:6370-6386).
func (s *PlayScene) sendGuildHomeSafe() {
	now := time.Now().UnixMilli()
	if now < s.guildQueryTick {
		return
	}
	s.guildQueryTick = now + 3000
	s.guildChatMode = false
	if s.sendGuildHome != nil {
		s.sendGuildHome()
	}
}

func (s *PlayScene) sendGuildMemberListSafe() {
	now := time.Now().UnixMilli()
	if now < s.guildQueryTick {
		return
	}
	s.guildQueryTick = now + 3000
	s.guildChatMode = false
	if s.sendGuildMemberList != nil {
		s.sendGuildMemberList()
	}
}

// addGuildChat appends a guild chat line, capped at 500 entries with the
// oldest 100 trimmed (FState:6465-6475).
func (s *PlayScene) addGuildChat(line string) {
	s.guildChats = append(s.guildChats, line)
	if len(s.guildChats) > 500 {
		s.guildChats = s.guildChats[100:]
	}
}

// openGuildNoticeEditor shows the [204] modal with a multi-line Memo
// (DGuildEditNotice, FState:1546-1552, 6217-6263): Ok[361]@(514,287),
// Close[64]@(584,6), Memo @(16,36) 571×246, 4000-rune cap.
func (s *PlayScene) openGuildNoticeEditor() {
	prg := s.resources.Prguse
	win := NewUIControl("DGuildEditNotice", KindWindow)
	if prg != nil {
		win.SetImgIndex(prg, ImgGuildNoticeBg)
	} else {
		win.Width, win.Height = 603, 330
	}
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2

	memo := NewMemoBox(s, "DGENMemo", 571, 246)
	memo.MaxLen = 4000
	memo.SetText(s.State.GuildNotice)
	memo.Ctrl.Left, memo.Ctrl.Top = 16, 36
	win.AddChild(memo.Ctrl)

	ok := NewUIControl("DGEOk", KindButton)
	ok.Left, ok.Top = 514, 287
	if prg != nil {
		ok.SetImgIndex(prg, ImgModalOk)
	}
	ok.OnClick = func(c *UIControl, x, y int) {
		text := memo.JoinedText()
		if utf8.RuneCountInString(text) > 4000 {
			s.addChatMessage("[系统] 公告内容过长,已截断至4000字符")
		}
		s.ui.CloseModal(win)
		if s.sendGuildUpdateNotice != nil {
			s.sendGuildUpdateNotice(text)
		}
	}
	win.AddChild(ok)

	closeBtn := NewUIControl("DGEClose", KindButton)
	closeBtn.Left, closeBtn.Top = 584, 6
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseMed)
	}
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.ui.CloseModal(win) }
	win.AddChild(closeBtn)

	s.ui.ShowModal(win)
	s.ui.SetFocus(memo.Ctrl)
}

// openGuildRankEditor loads the whole member table into a Memo for editing
// (FState:6265-6316, 5000-rune cap). The Go server currently parses a
// single "name/rank" line; full-table submission is tracked in batch B5.
func (s *PlayScene) openGuildRankEditor() {
	if len(s.State.GuildMembers) == 0 {
		s.addChatMessage("[系统] 请先获取成员列表再编辑职位")
		return
	}
	prg := s.resources.Prguse
	win := NewUIControl("DGuildEditGrade", KindWindow)
	if prg != nil {
		win.SetImgIndex(prg, ImgGuildNoticeBg)
	} else {
		win.Width, win.Height = 603, 330
	}
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2

	memo := NewMemoBox(s, "DGEGMemo", 571, 246)
	memo.MaxLen = 5000
	lines := make([]string, 0, len(s.State.GuildMembers))
	for _, m := range s.State.GuildMembers {
		name, rank, _ := parseGuildMemberLine(m)
		lines = append(lines, name+"/"+rank)
	}
	memo.SetText(strings.Join(lines, "\n"))
	memo.Ctrl.Left, memo.Ctrl.Top = 16, 36
	win.AddChild(memo.Ctrl)

	ok := NewUIControl("DGEGOk", KindButton)
	ok.Left, ok.Top = 514, 287
	if prg != nil {
		ok.SetImgIndex(prg, ImgModalOk)
	}
	ok.OnClick = func(c *UIControl, x, y int) {
		s.ui.CloseModal(win)
		if s.sendGuildUpdateRank != nil {
			s.sendGuildUpdateRank(memo.Lines[0])
		}
	}
	win.AddChild(ok)

	closeBtn := NewUIControl("DGEGClose", KindButton)
	closeBtn.Left, closeBtn.Top = 584, 6
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseMed)
	}
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.ui.CloseModal(win) }
	win.AddChild(closeBtn)

	s.ui.ShowModal(win)
	s.ui.SetFocus(memo.Ctrl)
}
