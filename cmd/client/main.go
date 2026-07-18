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
	selectServerScene := NewSelectServerScene(glState, resources, textRenderer)
	selectChrScene := NewSelectChrScene(glState, resources, textRenderer)
	noticeScene := NewNoticeScene(glState, resources, textRenderer)

	sceneMgr.RegisterScene(engine.SceneIntro, &DebugScene{name: "Intro"})
	sceneMgr.RegisterScene(engine.SceneLogin, loginScene)
	sceneMgr.RegisterScene(engine.SceneSelectServer, selectServerScene)
	sceneMgr.RegisterScene(engine.SceneSelectChr, selectChrScene)
	sceneMgr.RegisterScene(engine.SceneLoginNotice, noticeScene)
	sceneMgr.RegisterScene(engine.ScenePlayGame, playScene)

	sceneMgr.ChangeScene(engine.SceneLogin)

	var handler *NetHandler

	glfwWindow := window.GetWindow()

	// Wire login scene callbacks.
	loginScene.SetLoginFunc(func(id, password string) {
		log.Logf(log.LevelInfo, "Client", "[Callback] LoginFunc called: id=%s", id)
		if handler != nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] LoginFunc: handler already exists, skipping")
			return
		}
		var err error
		log.Logf(log.LevelInfo, "Client", "[Callback] LoginFunc: connecting to %s...", *serverAddr)
		handler, err = connectToServer(*serverAddr, loginScene, selectServerScene, playScene, selectChrScene, noticeScene, sceneMgr)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[Callback] LoginFunc: connect failed: %v", err)
			loginScene.SetError("连接服务器失败")
			handler = nil
			return
		}
		handler.onFail = func() {
			log.Logf(log.LevelInfo, "Client", "[Callback] onFail: resetting handler")
			handler = nil
		}
		handler.loginID = id
		log.Logf(log.LevelInfo, "Client", "[Callback] LoginFunc: sending login for id=%s", id)
		handler.SendLogin(id, password)
	})
	loginScene.SetCloseFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[Callback] CloseFunc: closing window")
		glfwWindow.SetShouldClose(true)
	})

	// Wire server selection scene callbacks.
	selectServerScene.SetSelectFunc(func(serverName string) {
		log.Logf(log.LevelInfo, "Client", "[Callback] ServerSelectFunc: server=%s", serverName)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] ServerSelectFunc: handler is nil")
			return
		}
		handler.SendSelectServer(serverName)
	})
	selectServerScene.SetCloseFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[Callback] ServerSelectClose: returning to login")
		if handler != nil {
			handler.Close()
			handler = nil
		}
		sceneMgr.ChangeScene(engine.SceneLogin)
	})

	// Wire select character scene callbacks.
	selectChrScene.SetStartFunc(func(charName string) {
		log.Logf(log.LevelInfo, "Client", "[Callback] ChrStartFunc: char=%s", charName)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] ChrStartFunc: handler is nil")
			return
		}
		handler.charName = charName
		handler.SendSelChr(charName)
	})
	selectChrScene.SetExitFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[Callback] ChrExitFunc: exiting")
		if handler != nil {
			handler.Close()
			handler = nil
		}
		glfwWindow.SetShouldClose(true)
	})

	// Wire notice scene callbacks.
	noticeScene.SetConfirmFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[Callback] NoticeConfirmFunc")
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] NoticeConfirmFunc: handler is nil")
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
	conn               net.Conn
	loginScene         *LoginScene
	selectServerScene  *SelectServerScene
	playScene          *PlayScene
	selectChrScene     *SelectChrScene
	noticeScene        *NoticeScene
	sceneMgr           *engine.SceneManager
	code               byte
	done               chan struct{}

	// Auth state
	loginID       string
	password      string // Stored for re-authentication after reconnect
	certification int
	charName      string
	reconnecting  bool // True when waiting for re-auth after reconnect

	// Callbacks (set by main)
	onReconnect func(addr string, loginID string, certification int)
	onFail      func() // Called when login fails, resets handler in main
}

// Close stops the read loop and closes the connection.
func (h *NetHandler) Close() {
	log.Logf(log.LevelInfo, "Client", "NetHandler.Close()")
	select {
	case <-h.done:
		log.Logf(log.LevelDebug, "Client", "NetHandler.Close: already closed")
	default:
		close(h.done)
	}
	h.conn.Close()
	log.Logf(log.LevelInfo, "Client", "NetHandler.Close: connection closed")
}

// Send encodes and sends a message to the server.
func (h *NetHandler) Send(msg protocol.DefaultMessage, body string) error {
	log.Logf(log.LevelInfo, "Client", ">>> SEND %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
		protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)
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
	log.Logf(log.LevelInfo, "Client", ">>> SEND RAW %q", s)
	encoded := protocol.EncodeString(s)
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendLogin sends the login credentials.
func (h *NetHandler) SendLogin(id, password string) {
	h.loginID = id
	h.password = password
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
	log.Logf(log.LevelInfo, "Client", "Reconnect: disconnecting from current server")
	// Stop old read loop
	select {
	case <-h.done:
		log.Logf(log.LevelDebug, "Client", "Reconnect: done channel already closed")
	default:
		close(h.done)
	}
	h.conn.Close()
	log.Logf(log.LevelInfo, "Client", "Reconnect: old connection closed, waiting 100ms...")

	// Wait briefly for read loop to exit
	time.Sleep(100 * time.Millisecond)

	// Connect to new server
	log.Logf(log.LevelInfo, "Client", "Reconnect: connecting to %s...", addr)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		log.Logf(log.LevelError, "Client", "Reconnect: failed to connect to %s: %v", addr, err)
		return fmt.Errorf("reconnect to %s: %w", addr, err)
	}
	log.Logf(log.LevelInfo, "Client", "Reconnect: connected to %s", addr)

	h.conn = conn
	h.done = make(chan struct{})
	h.code = 0

	// Start new read loop
	log.Logf(log.LevelInfo, "Client", "Reconnect: starting new ReadLoop")
	go h.ReadLoop()
	return nil
}

// ReadLoop reads messages from the server.
func (h *NetHandler) ReadLoop() {
	log.Logf(log.LevelInfo, "Client", "ReadLoop started")
	buf := make([]byte, 4096)
	for {
		select {
		case <-h.done:
			log.Logf(log.LevelInfo, "Client", "ReadLoop stopped (done)")
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
				log.Logf(log.LevelInfo, "Client", "ReadLoop stopped (closed)")
				return
			default:
			}
			log.Logf(log.LevelError, "Client", "ReadLoop error: %v", err)
			return
		}

		// Parse all frames in the buffer (server may send multiple frames in one TCP write)
		data := buf[:n]
		for len(data) > 2 {
			if data[0] != '#' {
				break
			}
			endIdx := -1
			for i := 1; i < len(data); i++ {
				if data[i] == '!' {
					endIdx = i
					break
				}
			}
			if endIdx < 0 {
				break // No complete frame
			}

			payload := string(data[1:endIdx])
			data = data[endIdx+1:]

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
	log.Logf(log.LevelInfo, "Client", "<<< RECV %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
		protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)

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
		// Close connection and reset handler so user can retry
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMPassOKSelectServer:
		if h.reconnecting {
			// Re-authenticated after reconnect — switch to LoginScene for door animation
			h.reconnecting = false
			log.Logf(log.LevelInfo, "Client", "Re-authenticated, switching to LoginScene for door animation")
			h.sceneMgr.ChangeScene(engine.SceneLogin)
			if h.loginScene != nil {
				h.loginScene.OpenLoginDoor()
				h.loginScene.SetDoorCompleteFunc(func() {
					log.Logf(log.LevelInfo, "Client", "Door animation complete, switching to SelectChr")
					h.sceneMgr.ChangeScene(engine.SceneSelectChr)
					time.Sleep(100 * time.Millisecond)
					h.SendQueryChr()
				})
			}
		} else {
			// First login — show server selection dialog
			log.Logf(log.LevelInfo, "Client", "Login successful, showing server selection")
			servers := parseServerList(body)
			if h.selectServerScene != nil {
				h.selectServerScene.SetServers(servers)
			}
			h.sceneMgr.ChangeScene(engine.SceneSelectServer)
		}

	case protocol.SMSelectServerOK:
		// Body: "selChrAddr/selChrPort/certification"
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] Parsing body=%q", body)
		addr, cert, err := parseAddrPortCert(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[SMSelectServerOK] Parse error: %v", err)
			return
		}
		h.certification = cert
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] addr=%s cert=%d", addr, cert)

		// Reconnect to selection server
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] Reconnecting to %s...", addr)
		if err := h.Reconnect(addr); err != nil {
			log.Logf(log.LevelError, "Client", "[SMSelectServerOK] Reconnect failed: %v", err)
			return
		}
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] Reconnected, re-authenticating...")

		// Re-authenticate on the new connection
		h.reconnecting = true
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] Setting reconnecting=true")
		protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, clientVersion, 0, 0, 0)
		h.Send(protoMsg, "")
		h.SendLogin(h.loginID, h.password)
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] Re-auth sent, waiting for SM_PASSOKSELECTSERVER")

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
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] body=%q", body)
		_, err := parseAddrPort(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[SMStartPlay] Parse error: %v", err)
			return
		}
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] Single-server mode, sending run login")

		// Single-server mode: send run login on existing connection
		h.SendRunLogin()
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] Run login sent, switching to LoginNotice scene")

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

	case protocol.SMVersionFail:
		log.Logf(log.LevelWarn, "Client", "Version mismatch")
		if h.loginScene != nil {
			h.loginScene.SetError("客户端版本不匹配")
		}
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMCertificationFail:
		log.Logf(log.LevelWarn, "Client", "Certification failed")
		if h.loginScene != nil {
			h.loginScene.SetError("认证失败")
		}
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

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
func connectToServer(addr string, loginScene *LoginScene, selectServerScene *SelectServerScene, playScene *PlayScene, selectChrScene *SelectChrScene, noticeScene *NoticeScene, sceneMgr *engine.SceneManager) (*NetHandler, error) {
	log.Logf(log.LevelInfo, "Client", "Connecting to %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	log.Logf(log.LevelInfo, "Client", "Connected to server")

	handler := &NetHandler{
		conn:              conn,
		loginScene:        loginScene,
		selectServerScene: selectServerScene,
		playScene:         playScene,
		selectChrScene:    selectChrScene,
		noticeScene:       noticeScene,
		sceneMgr:          sceneMgr,
		done:              make(chan struct{}),
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
