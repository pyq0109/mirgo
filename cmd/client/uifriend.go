package main

import (
	"fmt"
)

// 好友面板 — 简化版 DFrdFriendDlg

// toggleFriend 开关好友面板（Delphi OpenFriendDlg 纯本地 toggle，
// FState:6795-6798）。好友系统为 Go 扩展功能（Delphi 好友窗按钮均无
// 处理器），列表需向服务端请求，故打开时补发 CMQueryFriends——
// V 键与底栏好友按钮共用此入口保持一致。
func (s *PlayScene) toggleFriend() {
	s.State.ShowFriend = !s.State.ShowFriend
	if s.State.ShowFriend && s.sendQueryFriends != nil {
		s.sendQueryFriends()
	}
}

func (s *PlayScene) buildFriendPanel() {
	ui := s.ui
	prg := s.resources.Prguse

	friend := NewUIControl("DFrdFriendDlg", KindWindow)
	friend.Floating = true
	if prg != nil {
		friend.SetImgIndex(prg, ImgGuildBg)
	} else {
		friend.Width, friend.Height = 300, 400
	}
	friend.Left, friend.Top = 500, 50
	friend.Visible = false
	friend.OnDirectPaint = func(c *UIControl, proj [16]float32) { s.paintFriendPanel(c, proj) }
	ui.Root.AddChild(friend)
	s.hudFriend = friend

	closeBtn := NewUIControl("DFrdClose", KindButton)
	closeBtn.Left, closeBtn.Top = 268, 8
	closeBtn.Width, closeBtn.Height = 24, 24
	if prg != nil {
		closeBtn.SetImgIndex(prg, ImgCloseSmall)
	}
	closeBtn.OnClick = func(c *UIControl, x, y int) {
		s.State.ShowFriend = false
	}
	friend.AddChild(closeBtn)

	addBtn := NewUIControl("DFrdAdd", KindButton)
	addBtn.Left, addBtn.Top = 20, 360
	addBtn.Width, addBtn.Height = 80, 24
	addBtn.OnClick = func(c *UIControl, x, y int) {
		ShowInput(s, "输入要添加的好友名字", func(ok bool, text string) {
			if ok && text != "" && s.sendAddFriend != nil {
				s.sendAddFriend(text)
			}
		})
	}
	friend.AddChild(addBtn)

	delBtn := NewUIControl("DFrdDel", KindButton)
	delBtn.Left, delBtn.Top = 110, 360
	delBtn.Width, delBtn.Height = 80, 24
	delBtn.OnClick = func(c *UIControl, x, y int) {
		if s.friendSelected >= 0 && s.friendSelected < len(s.State.Friends) {
			name := s.State.Friends[s.friendSelected].Name
			if s.sendDelFriend != nil {
				s.sendDelFriend(name)
			}
		}
	}
	friend.AddChild(delBtn)
}

func (s *PlayScene) paintFriendPanel(c *UIControl, proj [16]float32) {
	c.Visible = s.State.ShowFriend
	if !c.Visible || s.text == nil {
		return
	}
	x := float32(c.Left + 20)
	y := float32(c.Top + 35)
	s.text.DrawText("好友列表", x, y, 1, 1, 0.6, 1, proj)
	y += 25

	for i, f := range s.State.Friends {
		color := [4]float32{0.6, 0.6, 0.6, 1}
		status := "离线"
		if f.Online {
			color = [4]float32{0.2, 1.0, 0.2, 1}
			status = "在线"
		}
		if i == s.friendSelected {
			s.gl.DrawQuadColor(x-2, y-2, 260, 18, 0.3, 0.3, 0.5, 0.5, proj)
		}
		line := fmt.Sprintf("%s  [%s]", f.Name, status)
		s.text.DrawText(line, x, y, color[0], color[1], color[2], color[3], proj)
		y += 20
	}
	if len(s.State.Friends) == 0 {
		s.text.DrawText("暂无好友", x, y, 0.6, 0.6, 0.6, 1, proj)
	}
}
