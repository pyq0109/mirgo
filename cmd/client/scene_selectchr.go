package main

import (
	"fmt"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

// CharacterSlot represents a character in the selection screen.
type CharacterSlot struct {
	Name  string
	Job   byte
	Hair  byte
	Level byte
	Sex   byte
	Valid bool
}

// jobNames maps job ID to display name.
var jobNames = []string{"战士", "法师", "道士"}

// SelectChrScene handles the character selection screen (stSelectChr).
type SelectChrScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	// Character data
	Characters [2]CharacterSlot
	Selected   int // 0 or 1, -1 if none

	// UI state
	errorMsg string

	// Callbacks
	startFunc func(charName string)
	exitFunc  func()
}

// Button areas for select character scene (relative to 800x600 centered in 1024x768).
const (
	selOX = float32(112) // (1024-800)/2
	selOY = float32(84)  // (768-600)/2
)

// Button positions from Delphi FState.pas.
var selButtonAreas = []loginArea{
	{selOX + 134, selOY + 424, 70, 20},  // Select Chr 1 (0)
	{selOX + 602, selOY + 424, 70, 20},  // Select Chr 2 (1)
	{selOX + 374, selOY + 427, 70, 20},  // Start Game (2)
	{selOX + 349, selOY + 467, 70, 20},  // New Chr (3)
	{selOX + 349, selOY + 505, 70, 20},  // Erase Chr (4)
	{selOX + 349, selOY + 543, 70, 20},  // Exit (5)
}

// NewSelectChrScene creates a new character selection scene.
func NewSelectChrScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *SelectChrScene {
	return &SelectChrScene{
		gl:        gl,
		resources: resources,
		text:      text,
		Selected:  -1,
	}
}

// Open is called when the scene becomes active.
func (s *SelectChrScene) Open() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Opened")
	s.errorMsg = ""
}

// Close is called when the scene becomes inactive.
func (s *SelectChrScene) Close() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Closed")
}

// Update updates the scene state.
func (s *SelectChrScene) Update(dt float64) {
}

// Render renders the character selection scene.
func (s *SelectChrScene) Render(gl *engine.GLState, proj [16]float32) {
	ox, oy := selOX, selOY

	// Background
	if s.resources.Prguse != nil {
		tex := s.resources.GetTexture(s.resources.Prguse, 65)
		if tex != 0 {
			w, h := s.getPrguseSize(65)
			gl.DrawQuad(tex, ox, oy, float32(w), float32(h), proj)
		}
	} else {
		gl.DrawQuadColor(0, 0, 1024, 768, 0.1, 0.15, 0.1, 1.0, proj)
	}

	// Draw character slots
	for i := 0; i < 2; i++ {
		s.renderCharSlot(gl, proj, ox, oy, i)
	}

	// Draw buttons
	s.renderButtons(gl, proj, ox, oy)

	// Draw text
	s.renderText(gl, proj, ox, oy)
}

// renderCharSlot renders a single character slot.
func (s *SelectChrScene) renderCharSlot(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
	ch := s.Characters[idx]
	if !ch.Valid {
		return
	}

	// Slot position: slot 0 on left, slot 1 on right (+340px)
	slotX := ox + float32(71)
	if idx == 1 {
		slotX = ox + float32(71+340)
	}
	slotY := oy + float32(52)

	// Draw a placeholder colored rectangle for the character
	var r, g, b float32
	switch ch.Job {
	case 0: // Warrior
		r, g, b = 0.8, 0.3, 0.3
	case 1: // Wizard
		r, g, b = 0.3, 0.3, 0.8
	case 2: // Taoist
		r, g, b = 0.3, 0.8, 0.3
	}
	gl.DrawQuadColor(slotX, slotY, 100, 200, r, g, b, 0.8, proj)

	// Highlight selected
	if idx == s.Selected {
		gl.DrawQuadColor(slotX-2, slotY-2, 104, 204, 1.0, 1.0, 0.0, 0.5, proj)
	}
}

// renderButtons renders the button textures.
func (s *SelectChrScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.resources.Prguse == nil {
		return
	}
	buttons := []struct {
		index int
		x, y  float32
	}{
		{66, ox + 134, oy + 424}, // Select Chr 1
		{67, ox + 602, oy + 424}, // Select Chr 2
		{68, ox + 374, oy + 427}, // Start Game
		{69, ox + 349, oy + 467}, // New Chr
		{70, ox + 349, oy + 505}, // Erase Chr
		{72, ox + 349, oy + 543}, // Exit
	}
	for _, btn := range buttons {
		tex := s.resources.GetTexture(s.resources.Prguse, btn.index)
		if tex != 0 {
			w, h := s.getPrguseSize(btn.index)
			gl.DrawQuad(tex, btn.x, btn.y, float32(w), float32(h), proj)
		}
	}
}

// renderText renders character info and error messages.
func (s *SelectChrScene) renderText(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}

	for i := 0; i < 2; i++ {
		ch := s.Characters[i]
		if !ch.Valid {
			continue
		}

		// Position text below each slot
		textX := ox + float32(50)
		if i == 1 {
			textX = ox + float32(50+340)
		}
		textY := oy + float32(270)

		// Character name
		s.text.DrawText(ch.Name, textX, textY, 1.0, 1.0, 0.8, 1.0, proj)

		// Level and job
		jobName := "未知"
		if int(ch.Job) < len(jobNames) {
			jobName = jobNames[ch.Job]
		}
		info := fmt.Sprintf("Lv.%d %s", ch.Level, jobName)
		s.text.DrawText(info, textX, textY+20, 0.8, 0.8, 0.8, 1.0, proj)
	}

	// Error message
	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+250, oy+400, 1.0, 0.3, 0.3, 1.0, proj)
	}
}

// OnKey handles keyboard input.
func (s *SelectChrScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	switch key {
	case keyEnter, keyKPEnter:
		s.startGame()
	}
}

// OnMouse handles mouse button input.
func (s *SelectChrScene) OnMouse(x, y float64, button int, action int) {
	fx, fy := float32(x), float32(y)

	for i, btn := range selButtonAreas {
		if hitTest(fx, fy, btn) {
			s.handleButton(i)
			return
		}
	}
}

// handleButton handles button click actions.
func (s *SelectChrScene) handleButton(index int) {
	switch index {
	case 0: // Select Chr 1
		if s.Characters[0].Valid {
			s.Selected = 0
			log.Logf(log.LevelInfo, "SelectChr", "Selected character 1: %s", s.Characters[0].Name)
		}
	case 1: // Select Chr 2
		if s.Characters[1].Valid {
			s.Selected = 1
			log.Logf(log.LevelInfo, "SelectChr", "Selected character 2: %s", s.Characters[1].Name)
		}
	case 2: // Start Game
		s.startGame()
	case 3: // New Chr
		s.errorMsg = "功能暂未开放"
	case 4: // Erase Chr
		s.errorMsg = "功能暂未开放"
	case 5: // Exit
		if s.exitFunc != nil {
			s.exitFunc()
		}
	}
}

// startGame validates selection and triggers the start callback.
func (s *SelectChrScene) startGame() {
	if s.Selected < 0 || s.Selected >= 2 || !s.Characters[s.Selected].Valid {
		s.errorMsg = "请先选择一个角色"
		return
	}
	if s.startFunc != nil {
		s.startFunc(s.Characters[s.Selected].Name)
	}
}

// SetStartFunc sets the callback for starting the game.
func (s *SelectChrScene) SetStartFunc(fn func(charName string)) {
	s.startFunc = fn
}

// SetExitFunc sets the callback for exiting.
func (s *SelectChrScene) SetExitFunc(fn func()) {
	s.exitFunc = fn
}

// SetError displays an error message.
func (s *SelectChrScene) SetError(msg string) {
	s.errorMsg = msg
}

// SetCharactersFromServer populates characters from parsed server data.
func (s *SelectChrScene) SetCharactersFromServer(chars []parsedChar, selectedIdx int) {
	// Clear existing
	s.Characters = [2]CharacterSlot{}
	s.Selected = -1

	for i, c := range chars {
		if i >= 2 {
			break
		}
		s.Characters[i] = CharacterSlot{
			Name:  c.Name,
			Job:   byte(c.Job),
			Hair:  byte(c.Hair),
			Level: byte(c.Level),
			Sex:   byte(c.Sex),
			Valid: true,
		}
	}

	if selectedIdx >= 0 && selectedIdx < 2 {
		s.Selected = selectedIdx
	} else if s.Characters[0].Valid {
		s.Selected = 0
	}
}

// OnScroll handles mouse scroll input.
func (s *SelectChrScene) OnScroll(x, y float64) {
}

// getPrguseSize gets the size of a Prguse.wil texture.
func (s *SelectChrScene) getPrguseSize(index int) (int, int) {
	if s.resources.Prguse == nil || index >= len(s.resources.Prguse.Images) {
		return 0, 0
	}
	img := s.resources.Prguse.Images[index]
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}
