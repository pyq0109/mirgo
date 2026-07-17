package main

import (
	"flag"
	"os"
	"runtime"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	dataDir := flag.String("data", "asset/client/Data", "Path to game data directory")
	serverAddr := flag.String("server", "localhost:7000", "Server address")
	flag.Parse()

	log.Logf(log.LevelInfo, "Client", "Starting MIR2 Client...")
	log.Logf(log.LevelInfo, "Client", "Data: %s", *dataDir)
	log.Logf(log.LevelInfo, "Client", "Server: %s", *serverAddr)

	// Create window
	window, err := engine.NewWindow(1024, 768, "MIR2 Client")
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to create window: %v", err)
		os.Exit(1)
	}
	defer window.Destroy()

	// Create GL state
	glState, err := engine.NewGLState()
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to create GL state: %v", err)
		os.Exit(1)
	}
	defer glState.Destroy()

	// Load resources
	resources, err := engine.NewResourceManager(*dataDir, glState)
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to load resources: %v", err)
		os.Exit(1)
	}
	defer resources.Destroy()
	log.Logf(log.LevelInfo, "Client", "Resources loaded")

	// Create scene manager
	sceneMgr := engine.NewSceneManager()

	// Register placeholder scenes
	sceneMgr.RegisterScene(engine.SceneIntro, &DebugScene{name: "Intro"})
	sceneMgr.RegisterScene(engine.SceneLogin, &DebugScene{name: "Login"})
	sceneMgr.RegisterScene(engine.SceneSelectChr, &DebugScene{name: "SelectChr"})
	sceneMgr.RegisterScene(engine.SceneLoginNotice, &DebugScene{name: "LoginNotice"})
	sceneMgr.RegisterScene(engine.ScenePlayGame, &DebugScene{name: "PlayGame"})

	sceneMgr.ChangeScene(engine.SceneIntro)

	// Setup GLFW callbacks
	glfwWindow := window.GetWindow()

	glfwWindow.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			switch key {
			case glfw.KeyEscape:
				w.SetShouldClose(true)
			case glfw.KeyF1:
				sceneMgr.ChangeScene(engine.SceneIntro)
			case glfw.KeyF2:
				sceneMgr.ChangeScene(engine.SceneLogin)
			case glfw.KeyF3:
				sceneMgr.ChangeScene(engine.SceneSelectChr)
			case glfw.KeyF4:
				sceneMgr.ChangeScene(engine.SceneLoginNotice)
			case glfw.KeyF5:
				sceneMgr.ChangeScene(engine.ScenePlayGame)
			}
		}
		sceneMgr.OnKey(int(key), int(action))
	})

	glfwWindow.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		x, y := w.GetCursorPos()
		sceneMgr.OnMouse(x, y, int(button), int(action))
	})

	glfwWindow.SetScrollCallback(func(w *glfw.Window, xoff float64, yoff float64) {
		x, y := w.GetCursorPos()
		sceneMgr.OnScroll(x, y)
	})

	// Main loop
	log.Logf(log.LevelInfo, "Client", "Entering main loop...")
	window.Run(func(dt float64) {
		sceneMgr.Update(dt)
	}, func() {
		// Calculate projection
		w, h := window.GetFramebufferSize()
		proj := engine.OrthoProj(float32(w), float32(h))
		sceneMgr.Render(glState, proj)
	})

	log.Logf(log.LevelInfo, "Client", "Client stopped")
}

// DebugScene is a placeholder scene that shows debug info.
type DebugScene struct {
	name string
}

func (s *DebugScene) Open() {
	log.Logf(log.LevelInfo, "Scene", "Opened: %s", s.name)
}

func (s *DebugScene) Close() {
	log.Logf(log.LevelInfo, "Scene", "Closed: %s", s.name)
}

func (s *DebugScene) Update(dt float64) {
	// Nothing to update
}

func (s *DebugScene) Render(glState *engine.GLState, proj [16]float32) {
	// Draw a colored background based on scene
	var r, g, b float32
	switch s.name {
	case "Intro":
		r, g, b = 0.2, 0.1, 0.3
	case "Login":
		r, g, b = 0.1, 0.2, 0.3
	case "SelectChr":
		r, g, b = 0.1, 0.3, 0.2
	case "LoginNotice":
		r, g, b = 0.3, 0.2, 0.1
	case "PlayGame":
		r, g, b = 0.1, 0.1, 0.1
	}

	// Draw background quad
	glState.DrawQuadColor(0, 0, 1024, 768, r, g, b, 1.0, proj)

	// Draw a white test quad in the center
	glState.DrawQuadColor(462, 334, 100, 100, 1.0, 1.0, 1.0, 1.0, proj)
}

func (s *DebugScene) OnKey(key int, action int) {
	// Handled in main
}

func (s *DebugScene) OnMouse(x, y float64, button int, action int) {
	// Nothing
}

func (s *DebugScene) OnScroll(x, y float64) {
	// Nothing
}
