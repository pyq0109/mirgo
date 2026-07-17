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
	log.Logf(log.LevelInfo, "Client", "Data: %s", *dataDir)
	log.Logf(log.LevelInfo, "Client", "Maps: %s", *mapDir)
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

	sceneMgr.RegisterScene(engine.SceneIntro, &DebugScene{name: "Intro"})
	sceneMgr.RegisterScene(engine.SceneLogin, &DebugScene{name: "Login"})
	sceneMgr.RegisterScene(engine.SceneSelectChr, &DebugScene{name: "SelectChr"})
	sceneMgr.RegisterScene(engine.SceneLoginNotice, &DebugScene{name: "LoginNotice"})
	sceneMgr.RegisterScene(engine.ScenePlayGame, playScene)

	// 启动时显示登录场景，等待服务端指示加载哪张地图
	sceneMgr.ChangeScene(engine.SceneLogin)

	// Network state
	var conn net.Conn
	
	glfwWindow := window.GetWindow()

	glfwWindow.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			switch key {
			case glfw.KeyEscape:
				if conn != nil {
					conn.Close()
					conn = nil
				}
				w.SetShouldClose(true)
			case glfw.KeyF9:
				if conn == nil {
					var err error
					conn, _, err = connectToServer(*serverAddr, playScene, sceneMgr)
					if err != nil {
						log.Logf(log.LevelError, "Client", "Failed to connect: %v", err)
						conn = nil
											}
				}
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

	log.Logf(log.LevelInfo, "Client", "Press F9 to connect to server")
	window.Run(func(dt float64) {
		sceneMgr.Update(dt)
	}, func() {
		w, h := window.GetFramebufferSize()
		proj := engine.OrthoProj(float32(w), float32(h))
		sceneMgr.Render(glState, proj)
	})

	if conn != nil {
		conn.Close()
	}
	log.Logf(log.LevelInfo, "Client", "Client stopped")
}

// NetHandler handles network communication.
type NetHandler struct {
	conn      net.Conn
	playScene *PlayScene
	sceneMgr  *engine.SceneManager
	code      byte
	done      chan struct{}
}

func connectToServer(addr string, playScene *PlayScene, sceneMgr *engine.SceneManager) (net.Conn, *NetHandler, error) {
	log.Logf(log.LevelInfo, "Client", "Connecting to %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	log.Logf(log.LevelInfo, "Client", "Connected to server")

	handler := &NetHandler{
		conn:      conn,
		playScene: playScene,
		sceneMgr:  sceneMgr,
		done:      make(chan struct{}),
	}

	// 发送协议版本
	protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0)
	handler.Send(protoMsg, "")

	// 发送登录
	loginMsg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	handler.Send(loginMsg, "test/test123")

	// 启动读取循环
	go handler.ReadLoop()

	return conn, handler, nil
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
	log.Logf(log.LevelDebug, "Client", "Received: msg=%d body=%q", msg.Ident, body)

	switch msg.Ident {
	case protocol.SMPassOKSelectServer:
		log.Logf(log.LevelInfo, "Client", "Login successful")
		// 登录成功，查询角色
		queryMsg := protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0)
		h.Send(queryMsg, "")

	case protocol.SMQueryChr:
		log.Logf(log.LevelInfo, "Client", "Received character list")
		// 收到角色列表，自动选择第一个角色
		selMsg := protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0)
		h.Send(selMsg, "")

	case protocol.SMNewMap:
		// 服务端告知加载哪张地图
		// body = 地图名, msg.Param = X, msg.Tag = Y
		mapName := body
		x := msg.Param
		y := msg.Tag
		log.Logf(log.LevelInfo, "Client", "Server requests map: %s(%d,%d)", mapName, x, y)

		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "Failed to load map: %v", err)
			return
		}
		h.sceneMgr.ChangeScene(engine.ScenePlayGame)

	case protocol.SMStartPlay:
		log.Logf(log.LevelInfo, "Client", "Start play")

	case protocol.SMLogon:
		log.Logf(log.LevelInfo, "Client", "Game started")
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
	case "SelectChr":
		r, g, b = 0.1, 0.3, 0.2
	case "LoginNotice":
		r, g, b = 0.3, 0.2, 0.1
	case "PlayGame":
		r, g, b = 0.1, 0.1, 0.1
	}
	glState.DrawQuadColor(0, 0, 1024, 768, r, g, b, 1.0, proj)
	glState.DrawQuadColor(462, 334, 100, 100, 1.0, 1.0, 1.0, 1.0, proj)

	// Draw scene name
	// TODO: Use ImGui for text rendering
}

func (s *DebugScene) OnKey(key int, action int) {}
func (s *DebugScene) OnMouse(x, y float64, button int, action int) {}
func (s *DebugScene) OnScroll(x, y float64) {}


