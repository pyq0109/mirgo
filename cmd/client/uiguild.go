package main

import (
	"strings"
)

// Guild panel — port of DGuildDlg (FState.pas:1495-1552 layout,
// 6187-6485 behavior) and group dialog DGroupDlg (:1435-1455,
// 5461-5566).

func (s *PlayScene) buildGuildPanels() {
	ui := s.ui
	prg := s.resources.Prguse

	// --- Guild panel [180] @(0,-3) ---
	guild := NewUIControl("DGuildDlg", KindWindow)
	guild.Floating = true
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
		{"DGDClose", ImgCloseSmall, 568, 20, false, func() { s.State.ShowGuild = false }},
		{"DGDHome", 198, 27, 387, false, func() { s.sendGuildHomeSafe() }},
		{"DGDList", 200, 27, 407, false, func() { s.sendGuildMemberListSafe() }},
		{"DGDChat", 190, 112, 407, false, func() { s.guildChatMode = !s.guildChatMode }},
		{"DGDAddMem", 182, 235, 387, true, func() {
			ShowInput(s, "Player name to add", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildAdd != nil {
					s.sendGuildAdd(text)
				}
			})
		}},
		{"DGDDelMem", 192, 235, 407, true, func() {
			ShowInput(s, "Player name to remove", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildDel != nil {
					s.sendGuildDel(text)
				}
			})
		}},
		{"DGDEditNotice", 196, 320, 387, true, func() {
			ShowInput(s, "Guild notice", func(ok bool, text string) {
				if ok && s.sendGuildUpdateNotice != nil {
					s.sendGuildUpdateNotice(text)
				}
			})
		}},
		{"DGDEditGrade", 194, 320, 407, true, func() {
			ShowInput(s, "Rank line: name/rank", func(ok bool, text string) {
				if ok && s.sendGuildUpdateRank != nil {
					s.sendGuildUpdateRank(text)
				}
			})
		}},
		{"DGDAlly", 184, 404, 387, true, func() {
			ShowInput(s, "Ally guild name", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildAlly != nil {
					s.sendGuildAlly(text)
				}
			})
		}},
		{"DGDBreakAlly", 186, 404, 407, true, func() {
			ShowInput(s, "Ally guild to break", func(ok bool, text string) {
				if ok && text != "" && s.sendGuildBreakAlly != nil {
					s.sendGuildBreakAlly(text)
				}
			})
		}},
		{"DGDWar", 202, 488, 387, true, func() { s.addChatMessage("Guild war: not implemented") }},
		{"DGDCancelWar", 188, 488, 407, true, func() { s.addChatMessage("Guild war: not implemented") }},
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
		btn.OnClick = func(c *UIControl, x, y int) { def.onClick() }
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
	group.Floating = true
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
		// Drawn only while group-allow is on (FState:5494-5512).
		if s.State.AllowGroup && prg != nil {
			s.ui.BlitImage(prg, ImgGroupAllow, c.AbsX(), c.AbsY(), proj)
		}
	}
	allow.OnClick = func(c *UIControl, x, y int) {
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
			ShowInput(s, "Player to invite (empty = solo)", func(ok bool, text string) {
				if ok && s.sendCreateGroup != nil {
					s.sendCreateGroup(text)
				}
			})
		}},
		{"DGrpAddMem", ImgGroupAdd, 123, func() {
			ShowInput(s, "Player name to add", func(ok bool, text string) {
				if ok && text != "" && s.sendAddGroupMember != nil {
					s.sendAddGroupMember(text)
				}
			})
		}},
		{"DGrpDelMem", ImgGroupDel, 202, func() {
			ShowInput(s, "Player name to remove", func(ok bool, text string) {
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
		btn.OnClick = func(c *UIControl, x, y int) { def.onClick() }
		group.AddChild(btn)
	}
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

// paintGuildPanel renders guild name + member list (FState:6318-6351).
func (s *PlayScene) paintGuildPanel(c *UIControl, proj [16]float32) {
	prg := s.resources.Prguse
	if prg != nil {
		s.ui.BlitImage(prg, ImgGuildBg, c.AbsX(), c.AbsY(), proj)
	}
	if s.text == nil {
		return
	}
	st := s.State
	s.text.DrawText(st.GuildName, float32(c.AbsX()+320), float32(c.AbsY()+13), 1, 1, 0.8, 1, proj)
	bx, by := c.AbsX()+24, c.AbsY()+41
	for i := st.GuildTopLine; i < len(st.GuildMembers); i++ {
		n := i - st.GuildTopLine
		if n*14 > 356 {
			break
		}
		name, rank, online := parseGuildMemberLine(st.GuildMembers[i])
		r, g, b := float32(0.75), float32(0.75), float32(0.75)
		if online {
			r, g, b = 0.9, 0.9, 0.9
		}
		s.text.DrawText(name+" ["+rank+"]", float32(bx), float32(by+n*14), r, g, b, 1, proj)
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
	s.text.DrawText(s.State.GroupMembers[0]+" (leader)", float32(c.AbsX()+28), float32(c.AbsY()+80), 0.75, 0.75, 0.75, 1, proj)
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

func (s *PlayScene) sendGuildHomeSafe() {
	if s.sendGuildHome != nil {
		s.sendGuildHome()
	}
}

func (s *PlayScene) sendGuildMemberListSafe() {
	if s.sendGuildMemberList != nil {
		s.sendGuildMemberList()
	}
}
