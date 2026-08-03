package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene 是 stLoginNotice 公告场景。Delphi 的 TLoginNotice 场景本体
// 是空壳（IntroScn.pas:1553-1561），公告内容由 ClientGetSendNotice 以
// 模态对话框显示（DMessageDlg DialogSize:=2，ClMain.pas:5732-5749），
// 用户点击 Ok 后才发送 CM_LOGINNOTICEOK。Go 在此场景中直接实现该模态。
type NoticeScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	noticeLines []string // 折行后的公告文本
	hasNotice   bool
	btnX, btnY  int // Ok 按钮绝对坐标（Render 时计算）
	btnW, btnH  int
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
}

func (s *NoticeScene) confirm() {
	if s.confirmed || !s.hasNotice {
		return
	}
	s.confirmed = true
	if s.onConfirm != nil {
		s.onConfirm()
	}
}

func (s *NoticeScene) Open() {
	log.Logf(log.LevelInfo, "NoticeScene", "opened")
}

func (s *NoticeScene) Close() {
	log.Logf(log.LevelInfo, "NoticeScene", "closed")
	s.hasNotice = false
	s.confirmed = false
	s.onConfirm = nil
}

func (s *NoticeScene) Update(dt float64) {}

func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	// 黑色底（过渡画面）
	gl.DrawQuadColor(0, 0, float32(ScreenWidth), float32(ScreenHeight), 0, 0, 0, 1, proj)
	if !s.hasNotice {
		return
	}

	// Delphi DMessageDlg DialogSize:=2 → DlgTall：背景 Prguse[380]，
	// 文本起点 (23,20)，Ok 按钮锚点 (105,305)（FState.pas:2033,2010-2037,2060-2083）。
	winImg := ImgModalTall
	winW, winH := 300, 340
	if s.resources != nil && s.resources.Prguse != nil {
		if img := s.resources.Prguse.GetImage(winImg); img != nil {
			winW, winH = img.Width, img.Height
		}
	}
	winX := (ScreenWidth - winW) / 2
	winY := (ScreenHeight - winH) / 2
	if s.resources != nil && s.resources.Prguse != nil {
		s.blitPrguse(winImg, winX, winY, proj)
	}
	if s.text != nil {
		y := winY + 20
		for _, ln := range s.noticeLines {
			s.text.DrawTextOutline(ln, float32(winX+23), float32(y),
				1, 1, 1, 1, 0, 0, 0, 1, proj)
			y += 14
		}
	}

	// Ok 按钮（ImgModalOk=361）
	btnW, btnH := 66, 26
	if s.resources != nil && s.resources.Prguse != nil {
		if img := s.resources.Prguse.GetImage(ImgModalOk); img != nil {
			btnW, btnH = img.Width, img.Height
		}
	}
	s.btnX = winX + 105
	s.btnY = winY + 305
	s.btnW, s.btnH = btnW, btnH
	if s.resources != nil && s.resources.Prguse != nil {
		s.blitPrguse(ImgModalOk, s.btnX, s.btnY, proj)
	}
}

func (s *NoticeScene) blitPrguse(idx, x, y int, proj [16]float32) {
	f := s.resources.Prguse
	img := f.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := s.resources.GetTexture(f, idx)
	if tex == 0 {
		return
	}
	s.gl.DrawQuad(tex, float32(x), float32(y), float32(img.Width), float32(img.Height), proj)
}

func (s *NoticeScene) OnKey(key int, action int) {
	if action != 1 { // GLFW press
		return
	}
	if key == keyEnter || key == keyKPEnter {
		s.confirm()
	}
}

func (s *NoticeScene) OnMouse(x, y float64, button int, action int, mods int) {
	if !s.hasNotice || s.confirmed || action != 1 || button != 0 {
		return
	}
	mx, my := int(x), int(y)
	if mx >= s.btnX && mx < s.btnX+s.btnW && my >= s.btnY && my < s.btnY+s.btnH {
		s.confirm()
	}
}

func (s *NoticeScene) OnScroll(x, y float64) {}
