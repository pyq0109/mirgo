package main

import (
	"strings"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene handles the login notice/announcement screen (stLoginNotice).
type NoticeScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	// Notice data
	NoticeText string
	Lines      []string
	Ready      bool

	okBtnArea [4]float32

	// Callback
	confirmFunc func()
}

// NewNoticeScene creates a new notice scene.
func NewNoticeScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *NoticeScene {
	return &NoticeScene{
		gl:        gl,
		resources: resources,
		text:      text,
	}
}

// Open is called when the scene becomes active.
func (s *NoticeScene) Open() {
	log.Logf(log.LevelInfo, "NoticeScene", "Opened")
	s.Ready = false
}

// Close is called when the scene becomes inactive.
func (s *NoticeScene) Close() {
	log.Logf(log.LevelInfo, "NoticeScene", "Closed")
}

// Update updates the scene state.
func (s *NoticeScene) Update(dt float64) {
}

// Render renders the notice scene. The login notice is a vertical DMessageDlg
// (DialogSize=2, Prguse[380] 256×359) on an empty black-screen scene; Delphi
// draws no overlay mask (ClMain.pas:5732-5748; FState.pas:2029-2040).
func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	// stLoginNotice background is an empty black screen.
	gl.DrawQuadColor(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 1, proj)

	if s.resources == nil || s.resources.Prguse == nil {
		return
	}

	// Prguse[380] centered: ((800-256)/2, (600-359)/2) = (272,120).
	bgImg := s.resources.Prguse.GetImage(380)
	if bgImg == nil {
		return
	}
	bgW := float32(bgImg.Width)
	bgH := float32(bgImg.Height)
	bgX := (ScreenWidth - bgW) / 2
	bgY := (ScreenHeight - bgH) / 2
	if bgTex := s.resources.GetTexture(s.resources.Prguse, 380); bgTex != 0 {
		gl.DrawQuad(bgTex, bgX, bgY, bgW, bgH, proj)
	}

	// Body window+(23,20), line spacing 14, white + black outline
	// (FState.pas:2036-2037, 2314-2323).
	if s.text != nil {
		y := bgY + 20
		for _, line := range s.Lines {
			s.text.DrawTextOutline(line, bgX+23, y, 1, 1, 1, 1, 0, 0, 0, 1, proj)
			y += 14
		}
	}

	// Ok button [361] window+(105,305) (FState.pas:2038-2039, 2079-2080).
	okImg := s.resources.Prguse.GetImage(361)
	if okImg == nil {
		return
	}
	okX := bgX + 105
	okY := bgY + 305
	okW := float32(okImg.Width)
	okH := float32(okImg.Height)
	if okTex := s.resources.GetTexture(s.resources.Prguse, 361); okTex != 0 {
		gl.DrawQuad(okTex, okX, okY, okW, okH, proj)
	}
	s.okBtnArea = [4]float32{okX, okY, okW, okH}
}

// OnKey handles keyboard input.
func (s *NoticeScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	switch key {
	case keyEnter, keyKPEnter:
		log.Logf(log.LevelInfo, "NoticeScene", "Enter pressed, confirming notice")
		s.confirm()
	}
}

// OnMouse handles mouse button input.
func (s *NoticeScene) OnMouse(x, y float64, button int, action int) {
	fx, fy := float32(x), float32(y)
	a := s.okBtnArea
	if fx >= a[0] && fx <= a[0]+a[2] && fy >= a[1] && fy <= a[1]+a[3] {
		log.Logf(log.LevelInfo, "NoticeScene", "OK button clicked")
		s.confirm()
	}
}

// OnScroll handles mouse scroll input.
func (s *NoticeScene) OnScroll(x, y float64) {
}

// confirm sends the notice confirmation and triggers the callback.
func (s *NoticeScene) confirm() {
	if s.confirmFunc != nil {
		s.confirmFunc()
	}
}

// SetConfirmFunc sets the callback for when the user confirms the notice.
func (s *NoticeScene) SetConfirmFunc(fn func()) {
	s.confirmFunc = fn
}

// SetNotice sets the notice text and splits into lines.
func (s *NoticeScene) SetNotice(text string) {
	log.Logf(log.LevelInfo, "NoticeScene", "SetNotice: %d chars, %d lines", len(text), len(strings.Split(text, "\n")))
	s.NoticeText = text
	s.Lines = strings.Split(text, "\n")
	s.Ready = true
}
