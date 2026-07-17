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
	// Background
	gl.DrawQuadColor(0, 0, 1024, 768, 0.1, 0.1, 0.2, 1.0, proj)

	// Notice panel
	gl.DrawQuadColor(200, 150, 624, 350, 0.15, 0.15, 0.25, 0.9, proj)

	// OK button
	gl.DrawQuadColor(noticeOKButton.X, noticeOKButton.Y, noticeOKButton.W, noticeOKButton.H, 0.3, 0.3, 0.4, 1.0, proj)

	// Render text
	if s.text != nil {
		// Title
		s.text.DrawText("服务器公告", 400, 160, 1.0, 1.0, 0.5, 1.0, proj)

		// Notice text lines
		y := float32(200)
		for _, line := range s.Lines {
			if y > 480 {
				break
			}
			s.text.DrawText(line, 220, y, 0.9, 0.9, 0.9, 1.0, proj)
			y += 20
		}

		// OK button text
		okText := "确 定"
		tw := s.text.MeasureText(okText)
		s.text.DrawText(okText, noticeOKButton.X+(noticeOKButton.W-float32(tw))/2, noticeOKButton.Y+10, 1.0, 1.0, 1.0, 1.0, proj)
	}
}

// OnKey handles keyboard input.
func (s *NoticeScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	switch key {
	case keyEnter, keyKPEnter:
		s.confirm()
	}
}

// OnMouse handles mouse button input.
func (s *NoticeScene) OnMouse(x, y float64, button int, action int) {
	fx, fy := float32(x), float32(y)
	if hitTest(fx, fy, noticeOKButton) {
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
	s.NoticeText = text
	s.Lines = strings.Split(text, "\n")
	s.Ready = true
}
