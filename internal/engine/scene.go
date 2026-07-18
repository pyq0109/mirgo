package engine

import "github.com/pyq0109/mirgo/internal/log"

// SceneType represents the type of game scene.
// Matches Delphi TSceneType enum from IntroScn.pas
type SceneType int

const (
	SceneIntro         SceneType = iota // 0: Startup screen (empty)
	SceneLogin                          // 1: Login screen with ID/Password
	SceneSelectServer                   // 2: Server selection dialog
	SceneSelectChr                      // 3: Character selection
	SceneNewChr                         // 4: Character creation (unused)
	SceneLoading                        // 5: Loading screen (unused)
	SceneLoginNotice                    // 6: Login notice/announcement
	ScenePlayGame                       // 7: Main game play
)

// String returns the scene type name.
func (t SceneType) String() string {
	switch t {
	case SceneIntro:
		return "Intro"
	case SceneLogin:
		return "Login"
	case SceneSelectServer:
		return "SelectServer"
	case SceneSelectChr:
		return "SelectChr"
	case SceneNewChr:
		return "NewChr"
	case SceneLoading:
		return "Loading"
	case SceneLoginNotice:
		return "LoginNotice"
	case ScenePlayGame:
		return "PlayGame"
	default:
		return "Unknown"
	}
}

// Scene is the interface for all game scenes.
type Scene interface {
	// Open called when the scene becomes active.
	Open()
	// Close is called when the scene becomes inactive.
	Close()
	// Update updates the scene state.
	Update(dt float64)
	// Render renders the scene.
	Render(gl *GLState, proj [16]float32)
	// OnKey handles keyboard input.
	OnKey(key int, action int)
	// OnMouse handles mouse button input.
	OnMouse(x, y float64, button int, action int)
	// OnScroll handles mouse scroll input.
	OnScroll(x, y float64)
}

// SceneManager manages scene transitions.
type SceneManager struct {
	currentType SceneType
	current     Scene
	scenes      map[SceneType]Scene
}

// NewSceneManager creates a new scene manager.
func NewSceneManager() *SceneManager {
	return &SceneManager{
		scenes: make(map[SceneType]Scene),
	}
}

// RegisterScene registers a scene for the given type.
func (m *SceneManager) RegisterScene(t SceneType, scene Scene) {
	m.scenes[t] = scene
}

// ChangeScene transitions to a new scene.
func (m *SceneManager) ChangeScene(t SceneType) {
	log.Logf(log.LevelInfo, "Scene", "ChangeScene: %s → %s", m.currentType, t)
	if m.current != nil {
		m.current.Close()
	}
	m.currentType = t
	m.current = m.scenes[t]
	if m.current != nil {
		m.current.Open()
	}
}

// CurrentType returns the current scene type.
func (m *SceneManager) CurrentType() SceneType {
	return m.currentType
}

// Current returns the current scene.
func (m *SceneManager) Current() Scene {
	return m.current
}

// Update updates the current scene.
func (m *SceneManager) Update(dt float64) {
	if m.current != nil {
		m.current.Update(dt)
	}
}

// Render renders the current scene.
func (m *SceneManager) Render(gl *GLState, proj [16]float32) {
	if m.current != nil {
		m.current.Render(gl, proj)
	}
}

// OnKey forwards keyboard input to the current scene.
func (m *SceneManager) OnKey(key int, action int) {
	if m.current != nil {
		m.current.OnKey(key, action)
	}
}

// OnMouse forwards mouse button input to the current scene.
func (m *SceneManager) OnMouse(x, y float64, button int, action int) {
	if m.current != nil {
		m.current.OnMouse(x, y, button, action)
	}
}

// OnScroll forwards mouse scroll input to the current scene.
func (m *SceneManager) OnScroll(x, y float64) {
	if m.current != nil {
		m.current.OnScroll(x, y)
	}
}

// OnChar forwards character input to the current scene if it supports it.
func (m *SceneManager) OnChar(char rune) {
	if s, ok := m.current.(interface{ OnChar(rune) }); ok {
		s.OnChar(char)
	}
}
