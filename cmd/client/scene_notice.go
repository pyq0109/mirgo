package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// NoticeScene handles the login notice/announcement screen (stLoginNotice).
// Renders: notice text, OK button
type NoticeScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager

	// Notice data
	NoticeText string
	Ready      bool
}

// NewNoticeScene creates a new notice scene.
func NewNoticeScene(gl *engine.GLState, resources *engine.ResourceManager) *NoticeScene {
	return &NoticeScene{
		gl:        gl,
		resources: resources,
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
	// Render background
	gl.DrawQuadColor(0, 0, 1024, 768, 0.1, 0.1, 0.2, 1.0, proj)

	// TODO: Render notice text
	// TODO: Render OK button

	// Placeholder
	gl.DrawQuadColor(200, 200, 624, 300, 0.15, 0.15, 0.25, 0.9, proj)
	gl.DrawQuadColor(412, 520, 200, 40, 0.3, 0.3, 0.4, 1.0, proj) // OK button
}

// OnKey handles keyboard input.
func (s *NoticeScene) OnKey(key int, action int) {
}

// OnMouse handles mouse button input.
func (s *NoticeScene) OnMouse(x, y float64, button int, action int) {
}

// OnScroll handles mouse scroll input.
func (s *NoticeScene) OnScroll(x, y float64) {
}

// SetNotice sets the notice text.
func (s *NoticeScene) SetNotice(text string) {
	s.NoticeText = text
	s.Ready = true
}
