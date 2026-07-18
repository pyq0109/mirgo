package main

import (
	"fmt"
	"time"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// GLFW key codes (matching go-gl/glfw constants).
const (
	keyBackspace = 259
	keyEnter     = 257
	keyTab       = 258
	keyKPEnter   = 335
	keyEscape    = 256
)

// loginArea defines a clickable region.
type loginArea struct {
	X, Y, W, H float32
}

// LoginScene handles the login screen (stLogin).
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	// Door animation state
	doorOpening   bool
	doorFrame     int
	doorMaxFrame  int
	doorStartTime time.Time

	// Login UI state
	showLoginUI  bool
	userID       string
	password     string
	focusedField int // 0=id, 1=password, -1=none
	cursorBlink  time.Time

	// Feedback
	errorMsg   string
	connecting bool

	// Callbacks
	loginFunc        func(id, password string)
	closeFunc        func()
	doorCompleteFunc func() // Called when door animation finishes
}

// Screen offset: 800x600 game area centered in 1024x768 window.
const (
	loginOX = float32(112) // (1024-800)/2
	loginOY = float32(84)  // (768-600)/2
)

// Input field positions (from Delphi IntroScn.pas).
var inputFields = []loginArea{
	{loginOX + 255, loginOY + 511, 112, 19}, // ID field
	{loginOX + 495, loginOY + 511, 112, 19}, // Password field
}

// Button positions (from Delphi FState.pas).
var buttonAreas = []loginArea{
	{loginOX + 90, loginOY + 558, 70, 20},  // OK (index 0)
	{loginOX + 268, loginOY + 558, 70, 20}, // ChangePW (index 1)
	{loginOX + 447, loginOY + 558, 70, 20}, // NewAccount (index 2)
	{loginOX + 613, loginOY + 558, 70, 20}, // Close (index 3)
}

// NewLoginScene creates a new login scene.
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *LoginScene {
	return &LoginScene{
		gl:            gl,
		resources:     resources,
		text:          text,
		doorMaxFrame:  10,
		showLoginUI:   true,
		focusedField:  0, // Start with ID field focused
		cursorBlink:   time.Now(),
	}
}

// Open is called when the scene becomes active.
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "Opened")
	s.showLoginUI = true
	s.doorOpening = false
	s.doorFrame = 0
	s.userID = ""
	s.password = ""
	s.focusedField = 0
	s.errorMsg = ""
	s.connecting = false
	s.cursorBlink = time.Now()
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
	if time.Since(s.doorStartTime) > 300*time.Millisecond {
		s.doorStartTime = time.Now()
		s.doorFrame++
		log.Logf(log.LevelDebug, "LoginScene", "Door animation frame %d/%d", s.doorFrame, s.doorMaxFrame)
		if s.doorFrame >= s.doorMaxFrame {
			s.doorFrame = s.doorMaxFrame - 1
			log.Logf(log.LevelInfo, "LoginScene", "Door animation complete")
			if s.doorCompleteFunc != nil {
				s.doorCompleteFunc()
				s.doorCompleteFunc = nil // Only call once
			}
		}
	}
}

// Render renders the login scene.
func (s *LoginScene) Render(gl *engine.GLState, proj [16]float32) {
	ox, oy := loginOX, loginOY

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
			gl.DrawQuad(doorTex, ox+252, oy+106, float32(w), float32(h), proj)
		}
	}

	// Login UI (only shown when door is not opening)
	if s.showLoginUI && !s.doorOpening {
		s.renderButtons(gl, proj, ox, oy)
		s.renderInputFields(gl, proj, ox, oy)
	}
}

// renderButtons renders the login UI button textures.
func (s *LoginScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	buttons := []struct {
		index int
		x, y  float32
	}{
		{62, ox + 90, oy + 558},  // OK
		{53, ox + 268, oy + 558}, // ChangePW
		{61, ox + 447, oy + 558}, // NewAccount
		{64, ox + 613, oy + 558}, // Close
	}
	for _, btn := range buttons {
		if tex, err := s.getPrguseTexture(btn.index); err == nil {
			w, h := s.getPrguseSize(btn.index)
			gl.DrawQuad(tex, btn.x, btn.y, float32(w), float32(h), proj)
		}
	}
}

// renderInputFields renders input field text, cursor, labels, and error messages.
func (s *LoginScene) renderInputFields(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}

	// Labels
	s.text.DrawText("账号:", ox+210, oy+511, 1.0, 1.0, 1.0, 1.0, proj)
	s.text.DrawText("密码:", ox+450, oy+511, 1.0, 1.0, 1.0, 1.0, proj)

	// ID field content
	idX := ox + 255
	idY := oy + 511
	s.text.DrawText(s.userID, idX, idY, 1.0, 1.0, 0.8, 1.0, proj)

	// Password field content (masked)
	passX := ox + 495
	passY := oy + 511
	masked := ""
	for range s.password {
		masked += "*"
	}
	s.text.DrawText(masked, passX, passY, 1.0, 1.0, 0.8, 1.0, proj)

	// Cursor blinking (500ms interval)
	if time.Since(s.cursorBlink) > 500*time.Millisecond {
		s.cursorBlink = time.Now()
	}
	showCursor := time.Since(s.cursorBlink) < 250*time.Millisecond

	if showCursor && s.focusedField >= 0 {
		var cx float32
		var cy float32
		var text string
		if s.focusedField == 0 {
			cx = idX + float32(s.text.MeasureText(s.userID))
			cy = idY
			text = s.userID
		} else {
			cx = passX + float32(s.text.MeasureText(masked))
			cy = passY
			text = masked
		}
		_ = text
		s.text.DrawText("|", cx, cy, 1.0, 1.0, 0.0, 1.0, proj)
	}

	// Error message
	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+200, oy+490, 1.0, 0.3, 0.3, 1.0, proj)
	}

	// Connecting status
	if s.connecting {
		s.text.DrawText("连接中...", ox+300, oy+490, 0.5, 0.8, 1.0, 1.0, proj)
	}
}

// OnChar handles character input from GLFW.
func (s *LoginScene) OnChar(char rune) {
	if !s.showLoginUI || s.doorOpening || s.connecting {
		return
	}
	if s.focusedField < 0 {
		return
	}
	// Only accept printable ASCII (space to tilde).
	if char < 32 || char > 126 {
		return
	}
	switch s.focusedField {
	case 0: // ID
		if len(s.userID) < 10 {
			s.userID += string(char)
		}
	case 1: // Password
		if len(s.password) < 10 {
			s.password += string(char)
		}
	}
	s.cursorBlink = time.Now()
}

// OnKey handles keyboard input.
func (s *LoginScene) OnKey(key int, action int) {
	if action != 1 { // Only on press
		return
	}
	if !s.showLoginUI || s.doorOpening {
		return
	}

	switch key {
	case keyBackspace:
		if s.connecting {
			return
		}
		switch s.focusedField {
		case 0:
			if len(s.userID) > 0 {
				s.userID = s.userID[:len(s.userID)-1]
			}
		case 1:
			if len(s.password) > 0 {
				s.password = s.password[:len(s.password)-1]
			}
		}
		s.cursorBlink = time.Now()

	case keyTab:
		if s.connecting {
			return
		}
		// Toggle between ID and password fields.
		if s.focusedField == 0 {
			s.focusedField = 1
		} else {
			s.focusedField = 0
		}
		s.cursorBlink = time.Now()

	case keyEnter, keyKPEnter:
		log.Logf(log.LevelInfo, "LoginScene", "Enter pressed, submitting login")
		s.submitLogin()
	}
}

// OnMouse handles mouse button input.
func (s *LoginScene) OnMouse(x, y float64, button int, action int) {
	if !s.showLoginUI || s.doorOpening {
		return
	}
	fx, fy := float32(x), float32(y)

	// Check input field clicks.
	for i, field := range inputFields {
		if hitTest(fx, fy, field) {
			s.focusedField = i
			s.cursorBlink = time.Now()
			return
		}
	}

	// Check button clicks.
	for i, btn := range buttonAreas {
		if hitTest(fx, fy, btn) {
			s.handleButton(i)
			return
		}
	}

	// Click outside — unfocus.
	s.focusedField = -1
}

// handleButton handles button click actions.
func (s *LoginScene) handleButton(index int) {
	buttonNames := []string{"OK", "ChangePW", "NewAccount", "Close"}
	if index < len(buttonNames) {
		log.Logf(log.LevelInfo, "LoginScene", "Button clicked: %s", buttonNames[index])
	}
	switch index {
	case 0: // OK
		s.submitLogin()
	case 1: // ChangePW
		s.errorMsg = "功能暂未开放"
	case 2: // NewAccount
		s.errorMsg = "功能暂未开放"
	case 3: // Close
		if s.closeFunc != nil {
			s.closeFunc()
		}
	}
}

// submitLogin validates input and triggers the login callback.
func (s *LoginScene) submitLogin() {
	if s.connecting {
		return
	}
	if s.userID == "" || s.password == "" {
		s.errorMsg = "请输入账号和密码"
		return
	}
	if s.loginFunc == nil {
		s.errorMsg = "未连接到服务器"
		return
	}
	s.errorMsg = ""
	s.connecting = true
	log.Logf(log.LevelInfo, "LoginScene", "Submitting login: %s", s.userID)
	s.loginFunc(s.userID, s.password)
}

// SetLoginFunc sets the callback for login attempts.
func (s *LoginScene) SetLoginFunc(fn func(id, password string)) {
	s.loginFunc = fn
}

// SetCloseFunc sets the callback for closing the application.
func (s *LoginScene) SetCloseFunc(fn func()) {
	s.closeFunc = fn
}

// SetDoorCompleteFunc sets the callback for when the door animation finishes.
func (s *LoginScene) SetDoorCompleteFunc(fn func()) {
	s.doorCompleteFunc = fn
}

// SetError displays an error message and resets connecting state.
func (s *LoginScene) SetError(msg string) {
	log.Logf(log.LevelWarn, "LoginScene", "Error: %s", msg)
	s.errorMsg = msg
	s.connecting = false
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

// hitTest checks if (x, y) is inside the area.
func hitTest(x, y float32, a loginArea) bool {
	return x >= a.X && x <= a.X+a.W && y >= a.Y && y <= a.Y+a.H
}

// getChrSelTexture gets a texture from ChrSel.wil.
func (s *LoginScene) getChrSelTexture(index int) (uint32, error) {
	if s.resources.ChrSel == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.ChrSel, index), nil
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
	return s.resources.GetTexture(s.resources.Prguse, index), nil
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

// OnScroll handles mouse scroll input.
func (s *LoginScene) OnScroll(x, y float64) {
}
