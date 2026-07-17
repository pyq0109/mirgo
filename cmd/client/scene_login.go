package main

import (
	"fmt"
	"time"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// LoginScene handles the login screen (stLogin).
// Renders: ChrSel background, door animation, login buttons
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager

	// Door animation state
	doorOpening  bool
	doorFrame    int
	doorMaxFrame int
	doorStartTime time.Time

	// Login state
	showLoginUI bool
}

// NewLoginScene creates a new login scene.
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager) *LoginScene {
	return &LoginScene{
		gl:            gl,
		resources:     resources,
		doorMaxFrame:  10,
		showLoginUI:   true,
	}
}

// Open is called when the scene becomes active.
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "Opened")
	s.showLoginUI = true
	s.doorOpening = false
	s.doorFrame = 0
}

// Close is called when the scene becomes inactive.
func (s *LoginScene) Close() {
	log.Logf(log.LevelInfo, "LoginScene", "Closed")
}

// Update updates the scene state.
func (s *LoginScene) Update(dt float64) {
	if !s.doorOpening {
		return
	}

	// Advance door frame every 300ms
	if time.Since(s.doorStartTime) > 300*time.Millisecond {
		s.doorStartTime = time.Now()
		s.doorFrame++
		if s.doorFrame >= s.doorMaxFrame {
			s.doorFrame = s.doorMaxFrame - 1
		}
	}
}

// Render renders the login scene.
func (s *LoginScene) Render(gl *engine.GLState, proj [16]float32) {
	// Screen center offset (800x600 original, we use 1024x768)
	ox := float32((1024 - 800) / 2)
	oy := float32((768 - 600) / 2)

	// Background: ChrSel.wil index 22
	if bgTex, err := s.getChrSelTexture(22); err == nil {
		w, h := s.getChrSelSize(22)
		gl.DrawQuad(bgTex, ox, oy, float32(w), float32(h), proj)
	}

	// Door animation: ChrSel.wil indices 23-32
	if s.doorOpening {
		doorIdx := 23 + s.doorFrame
		if doorTex, err := s.getChrSelTexture(doorIdx); err == nil {
			w, h := s.getChrSelSize(doorIdx)
			// Door position: offset (252, 106) from screen center
			gl.DrawQuad(doorTex, ox+252, oy+106, float32(w), float32(h), proj)
		}
	}

	// Login UI buttons (only shown when door is not opening)
	if s.showLoginUI && !s.doorOpening {
		s.renderButtons(gl, proj, ox, oy)
	}
}

// renderButtons renders the login UI buttons.
func (s *LoginScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	// OK button: Prguse.wil index 62, position (90, 558)
	if tex, err := s.getPrguseTexture(62); err == nil {
		w, h := s.getPrguseSize(62)
		gl.DrawQuad(tex, ox+90, oy+558, float32(w), float32(h), proj)
	}
	// Change Password button: Prguse.wil index 53, position (268, 558)
	if tex, err := s.getPrguseTexture(53); err == nil {
		w, h := s.getPrguseSize(53)
		gl.DrawQuad(tex, ox+268, oy+558, float32(w), float32(h), proj)
	}
	// New Account button: Prguse.wil index 61, position (447, 558)
	if tex, err := s.getPrguseTexture(61); err == nil {
		w, h := s.getPrguseSize(61)
		gl.DrawQuad(tex, ox+447, oy+558, float32(w), float32(h), proj)
	}
	// Close button: Prguse.wil index 64, position (613, 558)
	if tex, err := s.getPrguseTexture(64); err == nil {
		w, h := s.getPrguseSize(64)
		gl.DrawQuad(tex, ox+613, oy+558, float32(w), float32(h), proj)
	}
}

// OpenLoginDoor starts the door opening animation.
func (s *LoginScene) OpenLoginDoor() {
	log.Logf(log.LevelInfo, "LoginScene", "Opening door")
	s.doorOpening = true
	s.doorStartTime = time.Now()
	s.showLoginUI = false
}

// IsDoorFullyOpen returns true if the door animation is complete.
func (s *LoginScene) IsDoorFullyOpen() bool {
	return s.doorOpening && s.doorFrame >= s.doorMaxFrame-1
}

// getChrSelTexture gets a texture from ChrSel.wil.
func (s *LoginScene) getChrSelTexture(index int) (uint32, error) {
	if s.resources.ChrSel == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	tex := s.resources.GetTexture(s.resources.ChrSel, index)
	return tex, nil
}

// getChrSelSize gets the size of a texture from ChrSel.wil.
func (s *LoginScene) getChrSelSize(index int) (int, int) {
	if s.resources.ChrSel == nil || index >= len(s.resources.ChrSel.Images) {
		return 0, 0
	}
	img := s.resources.ChrSel.Images[index]
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

// getPrguseTexture gets a texture from Prguse.wil.
func (s *LoginScene) getPrguseTexture(index int) (uint32, error) {
	if s.resources.Prguse == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	tex := s.resources.GetTexture(s.resources.Prguse, index)
	return tex, nil
}

// getPrguseSize gets the size of a texture from Prguse.wil.
func (s *LoginScene) getPrguseSize(index int) (int, int) {
	if s.resources.Prguse == nil || index >= len(s.resources.Prguse.Images) {
		return 0, 0
	}
	img := s.resources.Prguse.Images[index]
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
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
