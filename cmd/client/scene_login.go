package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// LoginScene handles the login screen (stLogin).
// Renders: ChrSel background, door animation, ID/Password input placeholders
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager

	// State
	connecting bool
	errorMsg   string
}

// NewLoginScene creates a new login scene.
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager) *LoginScene {
	return &LoginScene{
		gl:        gl,
		resources: resources,
	}
}

// Open is called when the scene becomes active.
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "Opened")
}

// Close is called when the scene becomes inactive.
func (s *LoginScene) Close() {
	log.Logf(log.LevelInfo, "LoginScene", "Closed")
}

// Update updates the scene state.
func (s *LoginScene) Update(dt float64) {
}

// Render renders the login scene.
func (s *LoginScene) Render(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, 1024, 768, 0.05, 0.05, 0.15, 1.0, proj)
	gl.DrawQuadColor(362, 284, 300, 200, 0.1, 0.1, 0.2, 0.9, proj)
	gl.DrawQuadColor(370, 350, 280, 30, 0.2, 0.2, 0.3, 1.0, proj)
	gl.DrawQuadColor(370, 390, 280, 30, 0.2, 0.2, 0.3, 1.0, proj)
}

// OnKey handles keyboard input.
func (s *LoginScene) OnKey(key int, action int) {
}

// OnMouse handles mouse button input.
func (s *LoginScene) OnMouse(x, y float64, button int, action int) {
}

// OnScroll handles mouse scroll input.
func (s *LoginScene) OnScroll(x, y float64) {
}
