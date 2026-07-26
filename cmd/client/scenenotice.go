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

	// Callback
	confirmFunc func()
}

// OK button area for notice scene.
var noticeOKButton = loginArea{412, 520, 200, 40}

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

// Render renders the notice scene.
func (s *NoticeScene) Render(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, 1024, 768, 0.05, 0.05, 0.12, 1.0, proj)

	panelX, panelY := float32(200), float32(130)
	panelW, panelH := float32(624), float32(380)

	gl.DrawQuadColor(panelX-2, panelY-2, panelW+4, panelH+4, 0.4, 0.35, 0.2, 1.0, proj)
	gl.DrawQuadColor(panelX, panelY, panelW, panelH, 0.08, 0.08, 0.15, 0.95, proj)
	gl.DrawQuadColor(panelX+2, panelY+2, panelW-4, 28, 0.15, 0.12, 0.08, 0.9, proj)

	gl.DrawQuadColor(noticeOKButton.X-1, noticeOKButton.Y-1, noticeOKButton.W+2, noticeOKButton.H+2, 0.5, 0.45, 0.3, 1.0, proj)
	gl.DrawQuadColor(noticeOKButton.X, noticeOKButton.Y, noticeOKButton.W, noticeOKButton.H, 0.2, 0.18, 0.12, 1.0, proj)

	if s.text == nil {
		return
	}

	titleText := "服务器公告"
	tw := s.text.MeasureText(titleText)
	s.text.DrawText(titleText, panelX+(panelW-float32(tw))/2, panelY+6, 1.0, 0.9, 0.4, 1.0, proj)

	y := panelY + 40
	for _, line := range s.Lines {
		if y > panelY+panelH-20 {
			break
		}
		s.text.DrawText(line, panelX+20, y, 0.9, 0.9, 0.9, 1.0, proj)
		y += 22
	}

	okText := "确 定"
	ow := s.text.MeasureText(okText)
	s.text.DrawText(okText, noticeOKButton.X+(noticeOKButton.W-float32(ow))/2, noticeOKButton.Y+10, 1.0, 1.0, 0.8, 1.0, proj)
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
	if hitTest(fx, fy, noticeOKButton) {
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
