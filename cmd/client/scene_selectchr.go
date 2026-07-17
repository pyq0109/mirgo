package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/storage"
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

// SelectChrScene handles the character selection screen (stSelectChr).
// Renders: background, 2 character slots with freeze/unfreeze animations, character info
type SelectChrScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager

	// Character data
	Characters [2]CharacterSlot
	Selected   int // 0 or 1
}

// NewSelectChrScene creates a new character selection scene.
func NewSelectChrScene(gl *engine.GLState, resources *engine.ResourceManager) *SelectChrScene {
	return &SelectChrScene{
		gl:        gl,
		resources: resources,
	}
}

// Open is called when the scene becomes active.
func (s *SelectChrScene) Open() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Opened")
}

// Close is called when the scene becomes inactive.
func (s *SelectChrScene) Close() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Closed")
}

// Update updates the scene state.
func (s *SelectChrScene) Update(dt float64) {
	// TODO: Update freeze/unfreeze animations
}

// Render renders the character selection scene.
func (s *SelectChrScene) Render(gl *engine.GLState, proj [16]float32) {
	// Render background
	gl.DrawQuadColor(0, 0, 1024, 768, 0.1, 0.15, 0.1, 1.0, proj)

	// TODO: Render WMainImages[65] background
	// TODO: Render character slots with freeze/unfreeze animations
	// TODO: Render character name/level/job text

	// Placeholder: draw character slots
	gl.DrawQuadColor(100, 200, 200, 400, 0.15, 0.2, 0.15, 0.9, proj) // Slot 1
	gl.DrawQuadColor(724, 200, 200, 400, 0.15, 0.2, 0.15, 0.9, proj) // Slot 2
}

// OnKey handles keyboard input.
func (s *SelectChrScene) OnKey(key int, action int) {
}

// OnMouse handles mouse button input.
func (s *SelectChrScene) OnMouse(x, y float64, button int, action int) {
}

// OnScroll handles mouse scroll input.
func (s *SelectChrScene) OnScroll(x, y float64) {
}

// SetCharacters sets the character data from server response.
func (s *SelectChrScene) SetCharacters(chars []storage.CharacterInfo) {
	for i, c := range chars {
		if i >= 2 {
			break
		}
		s.Characters[i] = CharacterSlot{
			Name:  c.Name,
			Job:   byte(c.Job),
			Level: byte(c.Level),
			Sex:   byte(c.Sex),
			Valid: true,
		}
	}
}
