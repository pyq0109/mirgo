package main

import (
	"flag"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	dataDir := flag.String("data", "asset/client/Data", "Path to client data directory")
	mapDir := flag.String("maps", "asset/client/Map", "Path to map directory")
	serverAddr := flag.String("server", "localhost:7000", "Server address")
	flag.Parse()

	log.Logf(log.LevelInfo, "Client", "Starting MIR2 Client...")
	log.Logf(log.LevelInfo, "Client", "Server: %s", *serverAddr)

	window, err := engine.NewWindow(1024, 768, "MIR2 Client")
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to create window: %v", err)
		os.Exit(1)
	}
	defer window.Destroy()

	glState, err := engine.NewGLState()
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to create GL state: %v", err)
		os.Exit(1)
	}
	defer glState.Destroy()

	resources, err := engine.NewResourceManager(*dataDir, glState)
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to load resources: %v", err)
		os.Exit(1)
	}
	defer resources.Destroy()
	log.Logf(log.LevelInfo, "Client", "WIL resources loaded")

	sceneMgr := engine.NewSceneManager()
	playScene := NewPlayScene(glState, resources, *mapDir)
	loginScene := NewLoginScene(glState, resources)
	selectChrScene := NewSelectChrScene(glState, resources)
	noticeScene := NewNoticeScene(glState, resources)

	sceneMgr.RegisterScene(engine.SceneIntro, &DebugScene{name: "Intro"})
	sceneMgr.RegisterScene(engine.SceneLogin, loginScene)
	sceneMgr.RegisterScene(engine.SceneSelectChr, selectChrScene)
	sceneMgr.RegisterScene(engine.SceneLoginNotice, noticeScene)
	sceneMgr.RegisterScene(engine.ScenePlayGame, playScene)

	// Start at login scene
	sceneMgr.ChangeScene(engine.SceneLogin)

	// Network state
	var handler *NetHandler

	glfwWindow := window.GetWindow()

	glfwWindow.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			switch key {
			case glfw.KeyEscape:
				if handler != nil {
					handler.Close()
					handler = nil
				}
				w.SetShouldClose(true)
			case glfw.KeyF9:
				if handler == nil {
					var err error
					handler, err = connectToServer(*serverAddr, playScene, selectChrScene, noticeScene, sceneMgr)
					if err != nil {
						log.Logf(log.LevelError, "Client", "Failed to connect: %v", err)
						handler = nil
					}
				}
			}
		}
		sceneMgr.OnKey(int(key), int(action))
	})

	log.Logf(log.LevelInfo, "Client", "Press F9 to connect to server")
	window.Run(func(dt float64) {
		sceneMgr.Update(dt)
	}, func() {
		w, h := window.GetFramebufferSize()
		proj := engine.OrthoProj(float32(w), float32(h))
		sceneMgr.Render(glState, proj)
	})

	if handler != nil {
		handler.Close()
	}
	log.Logf(log.LevelInfo, "Client", "Client stopped")
}

// NetHandler handles network communication.
type NetHandler struct {
	conn           net.Conn
	playScene      *PlayScene
	selectChrScene *SelectChrScene
	noticeScene    *NoticeScene
	sceneMgr       *engine.SceneManager
	code           byte
	done           chan struct{}
}

func (h *NetHandler) Close() {
	close(h.done)
	h.conn.Close()
}

func connectToServer(addr string, playScene *PlayScene, selectChrScene *SelectChrScene, noticeScene *NoticeScene, sceneMgr *engine.SceneManager) (*NetHandler, error) {
	log.Logf(log.LevelInfo, "Client", "Connecting to %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	log.Logf(log.LevelInfo, "Client", "Connected to server")

	handler := &NetHandler{
		conn:           conn,
		playScene:      playScene,
		selectChrScene: selectChrScene,
		noticeScene:    noticeScene,
		sceneMgr:       sceneMgr,
		done:           make(chan struct{}),
	}

	// Send protocol version
	protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0)
	handler.Send(protoMsg, "")

	// Send login
	loginMsg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	handler.Send(loginMsg, "test/test123")

	// Start read loop
	go handler.ReadLoop()

	return handler, nil
}

func (h *NetHandler) Send(msg protocol.DefaultMessage, body string) error {
	encoded := protocol.EncodeMessage(msg)
	if body != "" {
		encoded += protocol.EncodeString(body)
	}
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

func (h *NetHandler) ReadLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-h.done:
			return
		default:
		}

		h.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := h.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Logf(log.LevelError, "Client", "Read error: %v", err)
			return
		}

		data := buf[:n]
		if len(data) > 2 && data[0] == '#' && data[len(data)-1] == '!' {
			payload := string(data[1 : len(data)-1])
			if len(payload) >= protocol.DefBlockSize {
				msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
				body := ""
				if len(payload) > protocol.DefBlockSize {
					body = protocol.DecodeString(payload[protocol.DefBlockSize:])
				}
				h.HandleMessage(msg, body)
			}
		}
	}
}

func (h *NetHandler) HandleMessage(msg protocol.DefaultMessage, body string) {
	log.Logf(log.LevelDebug, "Client", "Received: msg=%d", msg.Ident)

	switch msg.Ident {
	case protocol.SMPassOKSelectServer:
		log.Logf(log.LevelInfo, "Client", "Login successful")
		// Auto-select server (simplified)
		selMsg := protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0)
		h.Send(selMsg, "Server")

	case protocol.SMSelectServerOK:
		log.Logf(log.LevelInfo, "Client", "Server selected")
		h.sceneMgr.ChangeScene(engine.SceneSelectChr)
		// Query characters
		queryMsg := protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0)
		h.Send(queryMsg, "")

	case protocol.SMQueryChr:
		log.Logf(log.LevelInfo, "Client", "Received character list")
		// Auto-select first character (simplified)
		selMsg := protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0)
		h.Send(selMsg, "")

	case protocol.SMStartPlay:
		log.Logf(log.LevelInfo, "Client", "Start play")
		h.sceneMgr.ChangeScene(engine.SceneLoginNotice)

	case protocol.SMSendNotice:
		log.Logf(log.LevelInfo, "Client", "Received notice")
		h.noticeScene.SetNotice(body)
		// Auto-confirm notice
		okMsg := protocol.MakeDefaultMsg(protocol.CMLoginNoticeOK, 0, 0, 0, 0)
		h.Send(okMsg, "")

	case protocol.SMNewMap:
		mapName := body
		x := msg.Param
		y := msg.Tag
		log.Logf(log.LevelInfo, "Client", "Map: %s (%d,%d)", mapName, x, y)
		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "Failed to load map: %v", err)
			return
		}

	case protocol.SMLogon:
		log.Logf(log.LevelInfo, "Client", "Game started")
		h.sceneMgr.ChangeScene(engine.ScenePlayGame)
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMAbility:
		log.Logf(log.LevelInfo, "Client", "Received ability")

	case protocol.SMBagItems:
		log.Logf(log.LevelInfo, "Client", "Received bag items")

	default:
		log.Logf(log.LevelDebug, "Client", "Unhandled: %d", msg.Ident)
	}
}

// DebugScene is a placeholder scene.
type DebugScene struct {
	name string
}

func (s *DebugScene) Open() {
	log.Logf(log.LevelInfo, "Scene", "Opened: %s", s.name)
}

func (s *DebugScene) Close() {
	log.Logf(log.LevelInfo, "Scene", "Closed: %s", s.name)
}

func (s *DebugScene) Update(dt float64) {}

func (s *DebugScene) Render(glState *engine.GLState, proj [16]float32) {
	var r, g, b float32
	switch s.name {
	case "Intro":
		r, g, b = 0.2, 0.1, 0.3
	case "Login":
		r, g, b = 0.1, 0.2, 0.3
	}
	glState.DrawQuadColor(0, 0, 1024, 768, r, g, b, 1.0, proj)
	glState.DrawQuadColor(462, 334, 100, 100, 1.0, 1.0, 1.0, 1.0, proj)
}

func (s *DebugScene) OnKey(key int, action int)                    {}
func (s *DebugScene) OnMouse(x, y float64, button int, action int) {}
func (s *DebugScene) OnScroll(x, y float64)                        {}
