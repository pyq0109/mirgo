package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene 是 stLoginNotice 公告场景。Delphi 的 TLoginNotice 场景本体
// 是空壳（IntroScn.pas:1553-1561），公告内容由 ClientGetSendNotice 以
// 模态对话框显示（DMessageDlg DialogSize:=2，ClMain.pas:5732-5749），
// 用户点击 Ok 后才发送 CM_LOGINNOTICEOK。Go 在此场景中用 UIManager
// 控件树实现该模态，使 ui 调试命令在本场景同样可用。
type NoticeScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	ui  *UIManager
	dlg *UIControl // DMessageDlg 模态

	noticeLines []string // 折行后的公告文本
	hasNotice   bool
	onConfirm   func()
	confirmed   bool
}

func NewNoticeScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *NoticeScene {
	return &NoticeScene{gl: gl, resources: resources, text: text}
}

// SetNotice 显示公告模态。onConfirm 在用户点击 Ok/回车时调用一次
//（NetHandler 在其中发送 CM_LOGINNOTICEOK）。
func (s *NoticeScene) SetNotice(msg string, onConfirm func()) {
	s.noticeLines = wrapDialogText(s.text, msg, 260)
	s.hasNotice = true
	s.confirmed = false
	s.onConfirm = onConfirm
	if s.ui != nil {
		s.buildDlg()
	}
}

func (s *NoticeScene) confirm() {
	if s.confirmed || !s.hasNotice {
		return
	}
	s.confirmed = true
	if s.ui != nil && s.dlg != nil {
		s.ui.CloseModal(s.dlg)
	}
	if s.onConfirm != nil {
		s.onConfirm()
	}
}

func (s *NoticeScene) Open() {
	s.ui = NewUIManager(s.gl, s.resources, s.text)
	gActiveUI = s.ui
	if s.hasNotice && s.dlg == nil {
		s.buildDlg()
	}
	log.Logf(log.LevelInfo, "NoticeScene", "opened")
}

func (s *NoticeScene) Close() {
	log.Logf(log.LevelInfo, "NoticeScene", "closed")
	s.hasNotice = false
	s.confirmed = false
	s.onConfirm = nil
	s.dlg = nil
	gActiveUI = nil
}

// buildDlg 构建 DMessageDlg 模态：Delphi DMessageDlg DialogSize:=2 →
// DlgTall：背景 Prguse[380]，文本起点 (23,20)，Ok 按钮锚点 (105,305)
//（FState.pas:2033,2010-2037,2060-2083）。
func (s *NoticeScene) buildDlg() {
	prg := s.resources.Prguse

	win := NewUIControl("DMessageDlg", KindWindow)
	win.Floating = true
	if prg != nil {
		win.SetImgIndex(prg, ImgModalTall)
	} else {
		win.Width, win.Height = 300, 340
	}
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if prg != nil {
			s.ui.BlitImage(prg, ImgModalTall, c.AbsX(), c.AbsY(), proj)
		}
		if s.text == nil {
			return
		}
		y := float32(c.AbsY()) + 20
		for _, ln := range s.noticeLines {
			s.text.DrawTextOutline(ln, float32(c.AbsX())+23, y, 1, 1, 1, 1, 0, 0, 0, 1, proj)
			y += 14
		}
	}
	win.OnKeyDown = func(c *UIControl, key int) {
		if key == keyEnter || key == keyKPEnter {
			s.confirm()
		}
	}

	btn := NewUIControl("DMessageDlgBtn", KindButton)
	if prg != nil {
		btn.SetImgIndex(prg, ImgModalOk)
	} else {
		btn.Width, btn.Height = 66, 26
	}
	btn.Left = 105
	btn.Top = 305
	btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		idx := ImgModalOk
		if c.Downed {
			idx++ // 弹起/按下图片对惯例 (DWinCtl TDButton)
		}
		if prg != nil {
			s.ui.BlitImage(prg, idx, c.AbsX(), c.AbsY(), proj)
		}
	}
	btn.OnClick = func(c *UIControl, x, y int) { s.confirm() }
	win.AddChild(btn)

	s.dlg = win
	s.ui.ShowModal(win)
}

func (s *NoticeScene) Update(dt float64) {}

func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	// 黑色底（过渡画面）
	gl.DrawQuadColor(0, 0, float32(ScreenWidth), float32(ScreenHeight), 0, 0, 0, 1, proj)
	if s.ui == nil {
		return
	}
	s.ui.Paint(proj)
	if s.ui.ShowBounds {
		s.ui.RenderDebugBounds(proj)
	}
}

func (s *NoticeScene) OnKey(key int, action int) {
	if action != 1 { // GLFW press
		return
	}
	if s.ui != nil && s.ui.RouteKeyDown(key) {
		return
	}
	if key == keyEnter || key == keyKPEnter {
		s.confirm()
	}
}

func (s *NoticeScene) OnMouse(x, y float64, button int, action int, mods int) {
	if s.ui == nil {
		return
	}
	mx, my := int(x), int(y)
	switch action {
	case 1: // GLFW press
		s.ui.RouteMouseDown(mx, my, button)
	case 0: // GLFW release
		s.ui.RouteMouseUp(mx, my, button)
	}
}

func (s *NoticeScene) OnScroll(x, y float64) {}
