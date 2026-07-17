package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	clientVersion = 120040918
	runLoginCode  = 9
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

	textRenderer, err := engine.NewTextRenderer(glState, "", 16)
	if err != nil {
		log.Logf(log.LevelWarn, "Client", "Failed to load font: %v", err)
	}
	defer func() {
		if textRenderer != nil {
			textRenderer.Destroy()
		}
	}()

	sceneMgr := engine.NewSceneManager()
	playScene := NewPlayScene(glState, resources, *mapDir)
	loginScene := NewLoginScene(glState, resources, textRenderer)
	selectChrScene := NewSelectChrScene(glState, resources, textRenderer)
	noticeScene := NewNoticeScene(glState, resources, textRenderer)

	sceneMgr.RegisterScene(engine.SceneIntro, &DebugScene{name: "Intro"})
	sceneMgr.RegisterScene(engine.SceneLogin, loginScene)
	sceneMgr.RegisterScene(engine.SceneSelectChr, selectChrScene)
	sceneMgr.RegisterScene(engine.SceneLoginNotice, noticeScene)
	sceneMgr.RegisterScene(engine.ScenePlayGame, playScene)

	sceneMgr.ChangeScene(engine.SceneLogin)

	var handler *NetHandler

	glfwWindow := window.GetWindow()

	// Wire login scene callbacks.
	loginScene.SetLoginFunc(func(id, password string) {
		if handler != nil {
			return
		}
		var err error
		handler, err = connectToServer(*serverAddr, loginScene, playScene, selectChrScene, noticeScene, sceneMgr)
		if err != nil {
			log.Logf(log.LevelError, "Client", "Failed to connect: %v", err)
			loginScene.SetError("连接服务器失败")
			handler = nil
			return
		}
		handler.loginID = id
		handler.SendLogin(id, password)
	})
	loginScene.SetCloseFunc(func() { glfwWindow.SetShouldClose(true) })

	// Wire select character scene callbacks.
	selectChrScene.SetStartFunc(func(charName string) {
		if handler == nil {
			return
		}
		handler.charName = charName
		handler.SendSelChr(charName)
	})
	selectChrScene.SetExitFunc(func() {
		if handler != nil {
			handler.Close()
			handler = nil
		}
		glfwWindow.SetShouldClose(true)
	})

	// Wire notice scene callbacks.
	noticeScene.SetConfirmFunc(func() {
		if handler == nil {
			return
		}
		okMsg := protocol.MakeDefaultMsg(protocol.CMLoginNoticeOK, 0, 0, 0, 0)
		handler.Send(okMsg, "")
	})

	glfwWindow.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			switch key {
			case glfw.KeyEscape:
				if handler != nil {
					handler.Close()
					handler = nil
				}
				w.SetShouldClose(true)
			}
		}
		sceneMgr.OnKey(int(key), int(action))
	})

	glfwWindow.SetCharCallback(func(w *glfw.Window, char rune) {
		sceneMgr.OnChar(char)
	})

	glfwWindow.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		if action == glfw.Press {
			x, y := w.GetCursorPos()
			sceneMgr.OnMouse(x, y, int(button), 1)
		}
	})

	log.Logf(log.LevelInfo, "Client", "Login scene ready")
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

// ============================================================================
// NetHandler
// ============================================================================

// NetHandler handles network communication.
type NetHandler struct {
	conn           net.Conn
	loginScene     *LoginScene
	playScene      *PlayScene
	selectChrScene *SelectChrScene
	noticeScene    *NoticeScene
	sceneMgr       *engine.SceneManager
	code           byte
	done           chan struct{}

	// Auth state
	loginID       string
	certification int
	charName      string

	// Reconnection callback (set by main)
	onReconnect func(addr string, loginID string, certification int)
}

// Close stops the read loop and closes the connection.
func (h *NetHandler) Close() {
	select {
	case <-h.done:
		// Already closed
	default:
		close(h.done)
	}
	h.conn.Close()
}

// Send encodes and sends a message to the server.
func (h *NetHandler) Send(msg protocol.DefaultMessage, body string) error {
	encoded := protocol.EncodeMessage(msg)
	if body != "" {
		encoded += protocol.EncodeString(body)
	}
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendRawString sends a raw string without TDefaultMessage header.
func (h *NetHandler) SendRawString(s string) error {
	encoded := protocol.EncodeString(s)
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendLogin sends the login credentials.
func (h *NetHandler) SendLogin(id, password string) {
	loginMsg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	h.Send(loginMsg, id+"/"+password)
}

// SendSelectServer sends the server selection.
func (h *NetHandler) SendSelectServer(serverName string) {
	selMsg := protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0)
	h.Send(selMsg, serverName)
}

// SendQueryChr queries the character list (includes loginId/certification).
func (h *NetHandler) SendQueryChr() {
	queryMsg := protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0)
	h.Send(queryMsg, fmt.Sprintf("%s/%d", h.loginID, h.certification))
}

// SendSelChr sends the character selection.
func (h *NetHandler) SendSelChr(charName string) {
	selMsg := protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0)
	h.Send(selMsg, h.loginID+"/"+charName)
}

// SendRunLogin sends the run login to the game server.
func (h *NetHandler) SendRunLogin() {
	s := fmt.Sprintf("**%s/%s/%d/%d/%d", h.loginID, h.charName, h.certification, clientVersion, runLoginCode)
	h.SendRawString(s)
}

// Reconnect disconnects and reconnects to a new server address.
func (h *NetHandler) Reconnect(addr string) error {
	log.Logf(log.LevelInfo, "Client", "Reconnecting to %s...", addr)

	// Stop old read loop
	select {
	case <-h.done:
	default:
		close(h.done)
	}
	h.conn.Close()

	// Wait briefly for read loop to exit
	time.Sleep(100 * time.Millisecond)

	// Connect to new server
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("reconnect to %s: %w", addr, err)
	}
	log.Logf(log.LevelInfo, "Client", "Reconnected to %s", addr)

	h.conn = conn
	h.done = make(chan struct{})
	h.code = 0

	// Start new read loop
	go h.ReadLoop()
	return nil
}

// ReadLoop reads messages from the server.
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
			// Check if we were intentionally closed
			select {
			case <-h.done:
				return
			default:
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

// HandleMessage processes a server message.
func (h *NetHandler) HandleMessage(msg protocol.DefaultMessage, body string) {
	log.Logf(log.LevelDebug, "Client", "Received: msg=%d body=%q", msg.Ident, body)

	switch msg.Ident {

	// =====================================================================
	// Login Phase
	// =====================================================================

	case protocol.SMPasswdFail:
		log.Logf(log.LevelWarn, "Client", "Login failed: code=%d", msg.Recog)
		if h.loginScene != nil {
			switch msg.Recog {
			case -1:
				h.loginScene.SetError("密码错误")
			case -2:
				h.loginScene.SetError("密码错误超过3次，账号被锁定")
			case -3:
				h.loginScene.SetError("账号已经登录")
			case -4:
				h.loginScene.SetError("账号服务失败")
			case -5:
				h.loginScene.SetError("账号被封禁")
			default:
				h.loginScene.SetError("登录失败")
			}
		}

	case protocol.SMPassOKSelectServer:
		log.Logf(log.LevelInfo, "Client", "Login successful")
		if h.loginScene != nil {
			h.loginScene.OpenLoginDoor()
		}
		// Parse server list from body: "name1/status1/name2/status2/..."
		serverName := parseFirstServer(body)
		log.Logf(log.LevelInfo, "Client", "Selecting server: %s", serverName)
		h.SendSelectServer(serverName)

	case protocol.SMSelectServerOK:
		// Body: "selChrAddr/selChrPort/certification"
		addr, cert, err := parseAddrPortCert(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "Parse SM_SELECTSERVER_OK: %v", err)
			return
		}
		h.certification = cert
		log.Logf(log.LevelInfo, "Client", "SelChr server: %s (cert=%d)", addr, cert)

		// Reconnect to selection server
		if err := h.Reconnect(addr); err != nil {
			log.Logf(log.LevelError, "Client", "Reconnect to selchr: %v", err)
			return
		}

		// Send protocol version + query characters
		protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, clientVersion, 0, 0, 0)
		h.Send(protoMsg, "")

		// Wait briefly before querying characters
		time.Sleep(200 * time.Millisecond)
		h.SendQueryChr()

	case protocol.SMQueryChr:
		// Body: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
		log.Logf(log.LevelInfo, "Client", "Received character list: %s", body)
		chars, selectedIdx := parseQueryChrBody(body)
		if h.selectChrScene != nil {
			h.selectChrScene.SetCharactersFromServer(chars, selectedIdx)
		}

	case protocol.SMQueryChrFail:
		log.Logf(log.LevelWarn, "Client", "Query characters failed")
		// Show empty selection
		if h.selectChrScene != nil {
			h.selectChrScene.SetCharactersFromServer(nil, -1)
		}

	case protocol.SMNewChrSuccess:
		log.Logf(log.LevelInfo, "Client", "Character created")
		h.SendQueryChr()

	case protocol.SMNewChrFail:
		log.Logf(log.LevelWarn, "Client", "Create character failed: code=%d", msg.Recog)
		if h.selectChrScene != nil {
			switch msg.Recog {
			case 0:
				h.selectChrScene.SetError("名字不合法")
			case 2:
				h.selectChrScene.SetError("名字已被使用")
			case 3:
				h.selectChrScene.SetError("最多创建2个角色")
			default:
				h.selectChrScene.SetError("创建角色失败")
			}
		}

	case protocol.SMDelChrSuccess:
		log.Logf(log.LevelInfo, "Client", "Character deleted")
		h.SendQueryChr()

	case protocol.SMDelChrFail:
		log.Logf(log.LevelWarn, "Client", "Delete character failed")

	// =====================================================================
	// SelectChr → Play Transition
	// =====================================================================

	case protocol.SMStartPlay:
		// Body: "runAddr/runPort"
		addr, err := parseAddrPort(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "Parse SM_STARTPLAY: %v", err)
			return
		}
		log.Logf(log.LevelInfo, "Client", "Game server: %s", addr)

		// Reconnect to game server
		if err := h.Reconnect(addr); err != nil {
			log.Logf(log.LevelError, "Client", "Reconnect to game: %v", err)
			return
		}

		// Send run login (no TDefaultMessage header)
		h.SendRunLogin()

		// Switch to notice scene
		h.sceneMgr.ChangeScene(engine.SceneLoginNotice)

	case protocol.SMStartFail:
		log.Logf(log.LevelWarn, "Client", "Start play failed: server full")
		if h.selectChrScene != nil {
			h.selectChrScene.SetError("服务器已满")
		}

	// =====================================================================
	// Notice Phase
	// =====================================================================

	case protocol.SMSendNotice:
		log.Logf(log.LevelInfo, "Client", "Received notice")
		if h.noticeScene != nil {
			// Replace #27 line separators with newlines
			noticeText := strings.ReplaceAll(body, string(rune(27)), "\n")
			h.noticeScene.SetNotice(noticeText)
		}
		// Do NOT auto-send CMLoginNoticeOK — wait for user to click OK

	// =====================================================================
	// Game Phase
	// =====================================================================

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
		log.Logf(log.LevelInfo, "Client", "Game started (id=%d x=%d y=%d dir=%d)",
			msg.Recog, msg.Param, msg.Tag, msg.Series)
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

// ============================================================================
// Message Parsing Helpers
// ============================================================================

// parseFirstServer extracts the first server name from the server list body.
// Body format: "name1/status1/name2/status2/..."
func parseFirstServer(body string) string {
	if body == "" {
		return "Server"
	}
	parts := strings.Split(body, "/")
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return "Server"
}

// parseAddrPortCert parses "addr/port/certification".
func parseAddrPortCert(body string) (addr string, cert int, err error) {
	parts := strings.Split(body, "/")
	if len(parts) < 3 {
		return "", 0, fmt.Errorf("expected 3 parts, got %d: %q", len(parts), body)
	}
	port := parts[1]
	var c int
	_, scanErr := fmt.Sscanf(parts[2], "%d", &c)
	if scanErr != nil {
		return "", 0, fmt.Errorf("parse certification: %v", scanErr)
	}
	return parts[0] + ":" + port, c, nil
}

// parseAddrPort parses "addr/port".
func parseAddrPort(body string) (addr string, err error) {
	parts := strings.Split(body, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("expected 2 parts, got %d: %q", len(parts), body)
	}
	return parts[0] + ":" + parts[1], nil
}

// parsedChar holds a parsed character from SM_QUERYCHR.
type parsedChar struct {
	Name  string
	Job   int
	Hair  int
	Level int
	Sex   int
}

// parseQueryChrBody parses the character list body.
// Body format: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
// '*' prefix on a name means it was the last selected character.
func parseQueryChrBody(body string) (chars []parsedChar, selectedIdx int) {
	selectedIdx = -1
	if body == "" {
		return
	}

	parts := strings.Split(body, "/")
	// Each character has 5 fields: name, job, hair, level, sex
	for i := 0; i+4 < len(parts); i += 5 {
		name := parts[i]
		if name == "" {
			continue
		}

		// Check for selected marker
		if name[0] == '*' {
			name = name[1:]
			selectedIdx = len(chars)
		}

		var job, hair, level, sex int
		fmt.Sscanf(parts[i+1], "%d", &job)
		fmt.Sscanf(parts[i+2], "%d", &hair)
		fmt.Sscanf(parts[i+3], "%d", &level)
		fmt.Sscanf(parts[i+4], "%d", &sex)

		chars = append(chars, parsedChar{
			Name:  name,
			Job:   job,
			Hair:  hair,
			Level: level,
			Sex:   sex,
		})
	}
	return
}

// connectToServer creates a new NetHandler and connects to the login server.
func connectToServer(addr string, loginScene *LoginScene, playScene *PlayScene, selectChrScene *SelectChrScene, noticeScene *NoticeScene, sceneMgr *engine.SceneManager) (*NetHandler, error) {
	log.Logf(log.LevelInfo, "Client", "Connecting to %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	log.Logf(log.LevelInfo, "Client", "Connected to server")

	handler := &NetHandler{
		conn:           conn,
		loginScene:     loginScene,
		playScene:      playScene,
		selectChrScene: selectChrScene,
		noticeScene:    noticeScene,
		sceneMgr:       sceneMgr,
		done:           make(chan struct{}),
	}

	// Send protocol version
	protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, clientVersion, 0, 0, 0)
	handler.Send(protoMsg, "")

	go handler.ReadLoop()
	return handler, nil
}

// ============================================================================
// DebugScene
// ============================================================================

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
