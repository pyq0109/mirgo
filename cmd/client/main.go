package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
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

	// Delphi runs at fixed 800×600 (SWH800, Share.pas:22-24).
	window, err := engine.NewWindow(800, 600, "MIR2 Client")
	if err != nil {
		log.Logf(log.LevelError, "Client", "Failed to create window: %v", err)
		os.Exit(1)
	}
	window.SetResizable(false)
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
	playScene.SetText(textRenderer)
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
	loginScene.SetRegisterFunc(func(id, password string) {
		log.Logf(log.LevelInfo, "Client", "[Callback] RegisterFunc: id=%s", id)
		if handler == nil {
			var err error
			handler, err = connectToServer(*serverAddr, loginScene, selectServerScene, playScene, selectChrScene, noticeScene, sceneMgr)
			if err != nil {
				log.Logf(log.LevelError, "Client", "[Callback] RegisterFunc: connect failed: %v", err)
				loginScene.SetError("连接服务器失败")
				handler = nil
				return
			}
			handler.onFail = func() {
				handler = nil
			}
		}
		regMsg := protocol.MakeDefaultMsg(protocol.CMAddNewUser, 0, 0, 0, 0)
		handler.Send(regMsg, id+"/"+password)
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
	selectChrScene.SetNewChrFunc(func(name string, hair, job, sex int) {
		log.Logf(log.LevelInfo, "Client", "[Callback] ChrNewFunc: name=%s hair=%d job=%d sex=%d", name, hair, job, sex)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] ChrNewFunc: handler is nil")
			return
		}
		handler.SendNewChr(name, hair, job, sex)
	})
	selectChrScene.SetDelChrFunc(func(name string) {
		log.Logf(log.LevelInfo, "Client", "[Callback] ChrDelFunc: name=%s", name)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[Callback] ChrDelFunc: handler is nil")
			return
		}
		handler.SendDelChr(name)
	})
	selectChrScene.SetExitFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[Callback] ChrExitFunc: returning to login")
		if handler != nil {
			handler.Close()
			handler = nil
		}
		sceneMgr.ChangeScene(engine.SceneLogin)
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
		if action == glfw.Press && key == glfw.KeyEscape &&
			!playScene.State.ShowNpcDialog && !playScene.State.InDeal &&
			!playScene.State.ShowGuild && !playScene.State.ShowShop {
			if handler != nil {
				handler.Close()
				handler = nil
			}
			w.SetShouldClose(true)
		}
		sceneMgr.OnKey(int(key), int(action))
	})

	glfwWindow.SetCharCallback(func(w *glfw.Window, char rune) {
		sceneMgr.OnChar(char)
	})

	glfwWindow.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		switch action {
		case glfw.Press:
			x, y := w.GetCursorPos()
			sceneMgr.OnMouse(x, y, int(button), 1)
		case glfw.Release:
			x, y := w.GetCursorPos()
			sceneMgr.OnMouse(x, y, int(button), 0)
		}
	})

	glfwWindow.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		sceneMgr.OnMouseMove(xpos, ypos)
	})

	glfwWindow.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		sceneMgr.OnScroll(xoff, yoff)
	})

	log.Logf(log.LevelInfo, "Client", "Login scene ready")
	window.Run(func(dt float64) {
		sceneMgr.Update(dt)
	}, func() {
		// Keep GL viewport in sync with the framebuffer (HiDPI safety); all
		// UI coordinates are in the fixed 800×600 logical space.
		w, h := window.GetFramebufferSize()
		glState.SetViewport(0, 0, int32(w), int32(h))
		proj := engine.OrthoProj(800, 600)
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

// SendNewChr sends a create character request.
func (h *NetHandler) SendNewChr(name string, hair, job, sex int) {
	msg := protocol.MakeDefaultMsg(protocol.CMNewChr, 0, 0, 0, 0)
	h.Send(msg, fmt.Sprintf("%s/%s/%d/%d/%d", h.loginID, name, hair, job, sex))
}

// SendDelChr sends a delete character request.
func (h *NetHandler) SendDelChr(name string) {
	msg := protocol.MakeDefaultMsg(protocol.CMDelChr, 0, 0, 0, 0)
	h.Send(msg, name)
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
	var recvBuf []byte // accumulates bytes across Read calls; the 100ms deadline frequently splits frames
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

		// Parse all complete frames; accumulate across reads since a frame may span the 100ms deadline.
		if n > 0 {
			recvBuf = append(recvBuf, buf[:n]...)
			if len(recvBuf) > 64*1024 {
				log.Logf(log.LevelError, "Client", "Recv buffer overflow, disconnecting")
				return
			}
			data := recvBuf
			for len(data) > 2 {
				if data[0] != '#' {
					data = data[1:] // tolerate noise/misalignment, resync on next '#'
					continue
				}
				endIdx := -1
				for i := 1; i < len(data); i++ {
					if data[i] == '!' {
						endIdx = i
						break
					}
				}
				if endIdx < 0 {
					break // partial frame; wait for more data
				}

				payload := string(data[1:endIdx])
				data = data[endIdx+1:]

				if len(payload) > 0 && payload[0] == '+' {
					h.handleControlMsg(payload)
					continue
				}

				if len(payload) >= protocol.DefBlockSize {
					msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
					body := ""
					if len(payload) > protocol.DefBlockSize {
						body = protocol.DecodeString(payload[protocol.DefBlockSize:])
					}
					h.HandleMessage(msg, body)
				}
			}
			// Compact: keep any trailing partial frame, drop the consumed prefix.
			recvBuf = recvBuf[:copy(recvBuf, data)]
		}
	}
}

func (h *NetHandler) handleControlMsg(payload string) {
	switch {
	case strings.HasPrefix(payload, "+GOOD"):
		log.Logf(log.LevelDebug, "Client", "<<< +GOOD")
		h.playScene.ActionLock = false
	case strings.HasPrefix(payload, "+FAIL"):
		log.Logf(log.LevelDebug, "Client", "<<< +FAIL")
		h.playScene.ActionLock = false
		h.playScene.actionFailLock = true
		h.playScene.actionFailLockTime = time.Now().UnixMilli()
		if h.playScene.State.MySelf != nil {
			h.playScene.State.MySelf.MoveFail()
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
		x := int(msg.Recog)
		y := int(msg.Param)
		log.Logf(log.LevelInfo, "Client", "Map: %s (%d,%d)", mapName, x, y)
		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "Failed to load map: %v", err)
			return
		}

	case protocol.SMLogon:
		log.Logf(log.LevelInfo, "Client", "Game started (id=%d x=%d y=%d dir=%d)",
			msg.Recog, msg.Param, msg.Tag, msg.Series)
		actor := NewActor(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF)
		actor.Type = ActorHuman
		if body != "" {
			actor.updateFeatureFromBody(body)
			// Body slot 3 carries the job (see PlayObject.encodeLogonBody).
			if raw := []byte(body); len(raw) >= 12 {
				h.playScene.State.Job = int(binary.LittleEndian.Uint32(raw[8:12]))
			}
		}
		h.playScene.State.MySelf = actor
		h.playScene.State.Sex = actor.Sex
		h.playScene.State.Hair = actor.Hair
		h.playScene.State.Actors.Add(actor)
		actor.SendMsg(protocol.SMTurn, actor.CurrX, actor.CurrY, actor.Dir, 0, 0)
		h.sceneMgr.ChangeScene(engine.ScenePlayGame)
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMTurn:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor == nil {
			actor = NewActorFromMessage(msg, body)
			h.playScene.State.Actors.Add(actor)
		} else {
			actor.updateFeatureFromBody(body)
		}
		actor.SendMsg(protocol.SMTurn, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)

	case protocol.SMWalk:
		if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
			break
		}
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMWalk, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMRun:
		if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
			break
		}
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMRun, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMDisappear, protocol.SMGhost, protocol.SMHide:
		if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
			break
		}
		h.playScene.State.Actors.Remove(msg.Recog)

	case protocol.SMMoveFail:
		log.Logf(log.LevelDebug, "Client", "MoveFail from server")
		h.playScene.ActionLock = false
		if h.playScene.State.MySelf != nil {
			h.playScene.State.MySelf.MoveFail()
			my := h.playScene.State.MySelf
			my.CurrX = int(msg.Param)
			my.CurrY = int(msg.Tag)
			my.Dir = int(msg.Series) & 0xFF
			my.Rx = my.CurrX
			my.Ry = my.CurrY
			my.ShiftX = 0
			my.ShiftY = 0
		}

	case protocol.SMAbility:
		st := h.playScene.State
		if len(body) >= 60 {
			st.ParseAbility(body)
		} else {
			st.Level = int(msg.Recog)
		}
		log.Logf(log.LevelInfo, "Client", "Ability: level=%d hp=%d/%d mp=%d/%d exp=%d/%d weight=%d/%d",
			st.Level, st.HP, st.MaxHP, st.MP, st.MaxMP, st.Exp, st.MaxExp, st.Weight, st.MaxWeight)

	case protocol.SMStdItems:
		h.playScene.State.ParseItemDefs(body)
		log.Logf(log.LevelInfo, "Client", "Item DB synced: %d defs", len(h.playScene.State.ItemDefs))

	case protocol.SMBagItems:
		log.Logf(log.LevelInfo, "Client", "Received bag items: count=%d", msg.Recog)
		h.playScene.State.ParseBagItems(body)

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

	case protocol.SMNewIDSuccess:
		log.Logf(log.LevelInfo, "Client", "Registration successful")
		if h.loginScene != nil {
			h.loginScene.registerMode = false
			h.loginScene.connecting = false
			h.loginScene.errorMsg = "注册成功，请登录"
		}

	case protocol.SMNewIDFail:
		log.Logf(log.LevelWarn, "Client", "Registration failed: code=%d", msg.Recog)
		if h.loginScene != nil {
			h.loginScene.connecting = false
			switch msg.Recog {
			case 1:
				h.loginScene.errorMsg = "账号已存在"
			case 2:
				h.loginScene.errorMsg = "账号名不合法"
			default:
				h.loginScene.errorMsg = "注册失败"
			}
		}

	case protocol.SMSendUseItems:
		log.Logf(log.LevelInfo, "Client", "Received use items (equipment)")
		h.playScene.State.ParseUseItems(body)

	case protocol.SMSendMyMagic:
		log.Logf(log.LevelInfo, "Client", "Received magic list: count=%d", msg.Recog)
		h.playScene.State.ParseMagics(body)

	case protocol.SMHear:
		log.Logf(log.LevelInfo, "Client", "Chat: %s", body)
		h.playScene.AddChatMessage(body)

	case protocol.SMMerchantSay:
		log.Logf(log.LevelInfo, "Client", "NPC says: %s", body)
		h.playScene.parseNpcDialog(body)
		h.playScene.State.ShowNpcDialog = true

	case protocol.SMDayChanging:
		log.Logf(log.LevelInfo, "Client", "Day changing: bright=%d", msg.Recog)
		h.playScene.State.DayBright = int(msg.Recog)

	case protocol.SMMapDescription:
		log.Logf(log.LevelInfo, "Client", "Map description: %s", body)
		h.playScene.State.MapTitle = body

	case protocol.SMSubAbility:
		log.Logf(log.LevelInfo, "Client", "Received sub ability")

	case protocol.SMUsername:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.UserName = protocol.DecodeString(body)
			log.Logf(log.LevelDebug, "Client", "Actor name: %s", actor.UserName)
		}

	case protocol.SMChangeLight:
		log.Logf(log.LevelInfo, "Client", "Light change: %d", msg.Recog)
		h.playScene.State.LightLevel = int(msg.Recog)

	case protocol.SMHealthSpellChanged:
		// Recog=HP, Param=MaxHP, Tag=MP, Series=MaxMP (server changed in
		// tandem; the old HP<<16|MP packing made HP show the MP value).
		st := h.playScene.State
		st.HP = int(msg.Recog)
		st.MaxHP = int(msg.Param)
		st.MP = int(msg.Tag)
		st.MaxMP = int(msg.Series)

	case protocol.SMCharStatusChanged:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.State = int32(msg.Param)<<16 | int32(msg.Tag)
		}

	case protocol.SMClearObjects:
		log.Logf(log.LevelInfo, "Client", "Clear objects (map switch)")
		h.playScene.State.Actors.Clear()
		h.playScene.State.MySelf = nil

	case protocol.SMChangeMap:
		mapName := body
		newX := int(msg.Param)
		newY := int(msg.Tag)
		log.Logf(log.LevelInfo, "Client", "Change map: %s (%d,%d)", mapName, newX, newY)
		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "Failed to load map on change: %v", err)
			return
		}
		actor := NewActor(msg.Recog, newX, newY, 0)
		actor.Type = ActorHuman
		h.playScene.State.MySelf = actor
		h.playScene.State.Actors.Add(actor)
		actor.SendMsg(protocol.SMTurn, newX, newY, 0, 0, 0)

	// =====================================================================
	// Combat
	// =====================================================================

	case protocol.SMHit, protocol.SMHeavyHit, protocol.SMBigHit, protocol.SMPowerHit, protocol.SMLongHit, protocol.SMWideHit, protocol.SMFireHit, protocol.SMCrsHit, protocol.SMTwinHit:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMStruck:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMStruck, actor.CurrX, actor.CurrY, int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMDeath, protocol.SMNowDeath:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.Death = true
			actor.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), 0, 0, 0)
			if h.playScene.State.MySelf != nil && actor.RecogID == h.playScene.State.MySelf.RecogID {
				h.playScene.deathGray = true
			}
		}

	case protocol.SMAlive:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.Death = false
			actor.Skeleton = false
			actor.SendMsg(protocol.SMAlive, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
			if h.playScene.State.MySelf != nil && actor.RecogID == h.playScene.State.MySelf.RecogID {
				h.playScene.deathGray = false
			}
		}

	case protocol.SMWinExp:
		// The server follows up with a full SMAbility; apply the delta now so
		// the exp bar moves immediately.
		h.playScene.State.Exp += int(msg.Recog)
		log.Logf(log.LevelInfo, "Client", "Won exp: %d", msg.Recog)

	case protocol.SMLevelUp:
		h.playScene.State.Level = int(msg.Recog)
		log.Logf(log.LevelInfo, "Client", "Level up: %d", msg.Recog)

	case protocol.SMItemShow:
		log.Logf(log.LevelDebug, "Client", "Item show: id=%d x=%d y=%d looks=%d name=%s",
			msg.Recog, msg.Param, msg.Tag, msg.Series, body)
		h.playScene.AddGroundItem(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series), body)

	case protocol.SMItemHide:
		h.playScene.RemoveGroundItem(msg.Recog)

	case protocol.SMDealMenu:
		log.Logf(log.LevelInfo, "Client", "Trade: partner=%s", body)
		h.playScene.resetDeal()
		h.playScene.State.InDeal = true
		h.playScene.State.DealPartner = body

	case protocol.SMDealSuccess:
		log.Logf(log.LevelInfo, "Client", "Trade completed")
		h.playScene.State.InDeal = false
		h.playScene.State.DealPartner = ""
		h.playScene.resetDeal()

	case protocol.SMDealCancel:
		log.Logf(log.LevelInfo, "Client", "Trade cancelled")
		h.playScene.State.InDeal = false
		h.playScene.State.DealPartner = ""
		h.playScene.resetDeal()

	case protocol.SMDealChgGoldOK:
		h.playScene.State.DealGold = int(msg.Recog)
		h.playScene.State.Gold = int(msg.Param)

	case protocol.SMDealRemoteChgGold:
		h.playScene.State.DealRemoteGold = int(msg.Recog)

	case protocol.SMBuildGuildOK:
		h.playScene.addChatMessage("Guild created")

	case protocol.SMGuildMessage:
		log.Logf(log.LevelInfo, "Client", "Guild chat: %s", body)
		h.playScene.AddChatMessage("[行会] " + body)

	case protocol.SMStorageOK:
		log.Logf(log.LevelInfo, "Client", "Item stored")

	case protocol.SMStorageFull:
		log.Logf(log.LevelInfo, "Client", "Storage full")

	case protocol.SMTakeBackStorageItemOK:
		log.Logf(log.LevelInfo, "Client", "Item retrieved from storage")

	case protocol.SMSysMessage:
		log.Logf(log.LevelInfo, "Client", "System: %s", body)
		h.playScene.AddChatMessage("[系统] " + body)

	case protocol.SMGoldChanged:
		log.Logf(log.LevelInfo, "Client", "Gold: %d", msg.Recog)
		h.playScene.State.Gold = int(msg.Recog)

	case protocol.SMBackStep:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMBackStep, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMSitdown:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMSitdown, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMRush:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMRush, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMRushKung:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMRushKung, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMSkeleton:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.Skeleton = true
			actor.SendMsg(protocol.SMSkeleton, int(msg.Param), int(msg.Tag), 0, 0, 0)
		}

	case protocol.SMThrow:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMThrow, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMSpell:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMSpell, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMMagicFire:
		log.Logf(log.LevelDebug, "Client", "Magic fire: magID=%d x=%d y=%d series=%d", msg.Recog, msg.Param, msg.Tag, msg.Series)
		fx := float64(msg.Param)*engine.TileWidth + engine.TileWidth/2
		fy := float64(msg.Tag)*engine.TileHeight + engine.TileHeight/2
		effType := int(msg.Series & 0xFF)
		effNum := int(msg.Series >> 8)
		switch effType {
		case 2:
			h.playScene.effects.AddGround(fx, fy, effNum, 10, 50)
		default:
			// fly(1) carries no start coordinate in this message; degrade to explosion
			h.playScene.effects.AddExplosion(fx, fy, effNum, 10, 50)
		}

	case protocol.SMMagicFireFail:
		log.Logf(log.LevelDebug, "Client", "Magic fire failed")

	case protocol.SMShowEvent:
		h.playScene.events.AddEvent(&MapEvent{
			ServerID:  msg.Recog,
			EType:     int(msg.Param),
			X:         int(msg.Tag),
			Y:         int(msg.Series),
			FrameTime: 50,
		})

	case protocol.SMHideEvent:
		h.playScene.events.DelEventByID(msg.Recog)

	case protocol.SMAddMagic:
		log.Logf(log.LevelInfo, "Client", "Learned magic: %d", msg.Recog)

	case protocol.SMDelMagic:
		log.Logf(log.LevelInfo, "Client", "Forgot magic: %d", msg.Recog)

	case protocol.SMFeatureChanged:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil && body != "" {
			actor.updateFeatureFromBody(body)
		}

	case protocol.SMChangeNameColor:
		log.Logf(log.LevelDebug, "Client", "Name color changed: %d", msg.Recog)

	case protocol.SMCry:
		log.Logf(log.LevelInfo, "Client", "Cry: %s", body)
		h.playScene.AddChatMessage("[喊话] " + body)

	case protocol.SMWhisper:
		log.Logf(log.LevelInfo, "Client", "Whisper: %s", body)
		h.playScene.AddChatMessage("[私聊] " + body)

	case protocol.SMGroupMessage:
		log.Logf(log.LevelInfo, "Client", "Group: %s", body)
		h.playScene.AddChatMessage("[组队] " + body)

	case protocol.SMAddItem:
		log.Logf(log.LevelInfo, "Client", "Item added")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMDelItem:
		log.Logf(log.LevelInfo, "Client", "Item deleted: idx=%d", msg.Recog)
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMUpdateItem:
		log.Logf(log.LevelInfo, "Client", "Item updated")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMDelItems:
		log.Logf(log.LevelInfo, "Client", "Items cleared")

	case protocol.SMDropItemSuccess:
		log.Logf(log.LevelInfo, "Client", "Drop item success")

	case protocol.SMDropItemFail:
		log.Logf(log.LevelInfo, "Client", "Drop item failed")

	case protocol.SMWeightChanged:
		h.playScene.State.Weight = int(msg.Recog)
		h.playScene.State.MaxWeight = int(msg.Param)
		log.Logf(log.LevelDebug, "Client", "Weight changed: %d/%d", msg.Recog, msg.Param)

	case protocol.SMAdjustBonus:
		h.playScene.State.BonusPoint = int(msg.Recog)

	case protocol.SMSendUserState:
		// Body: 130 bytes of equipment followed by the player name.
		raw := []byte(body)
		name := ""
		if len(raw) > 130 {
			name = string(raw[130:])
		}
		h.playScene.parseInspect(name, body)

	case protocol.SMDuraChange:
		log.Logf(log.LevelDebug, "Client", "Durability changed: makeIndex=%d dura=%d/%d", msg.Recog, msg.Param, msg.Tag)
		for _, it := range h.playScene.State.UseItems {
			if it != nil && it.MakeIndex == msg.Recog {
				it.Dura = msg.Param
				it.DuraMax = msg.Tag
			}
		}
		for _, it := range h.playScene.State.BagItems {
			if it != nil && it.MakeIndex == msg.Recog {
				it.Dura = msg.Param
				it.DuraMax = msg.Tag
			}
		}

	case protocol.SMEatOK:
		log.Logf(log.LevelInfo, "Client", "Ate item OK")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMEatFail:
		log.Logf(log.LevelInfo, "Client", "Eat item failed")

	case protocol.SMTakeOnOK:
		log.Logf(log.LevelInfo, "Client", "Equip OK: slot=%d", msg.Recog)

	case protocol.SMTakeOnFail:
		log.Logf(log.LevelInfo, "Client", "Equip failed: code=%d", msg.Recog)

	case protocol.SMTakeOffOK:
		log.Logf(log.LevelInfo, "Client", "Unequip OK: slot=%d", msg.Recog)

	case protocol.SMTakeOffFail:
		log.Logf(log.LevelInfo, "Client", "Unequip failed")

	case protocol.SMMerchantDlgClose:
		h.playScene.State.ShowNpcDialog = false

	case protocol.SMSendGoodsList:
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 0
		h.playScene.State.ShopNpcID = msg.Recog
		h.playScene.menuTop = 0
		h.playScene.menuIndex = -1
		raw := []byte(body)
		if len(raw) >= 2 {
			count := int(binary.LittleEndian.Uint16(raw[0:2]))
			h.playScene.State.ShopGoods = make([]ShopItem, 0, count)
			for i := 0; i < count; i++ {
				off := 2 + i*4
				if off+4 > len(raw) {
					break
				}
				itemIdx := binary.LittleEndian.Uint16(raw[off : off+2])
				price := int(binary.LittleEndian.Uint16(raw[off+2 : off+4]))
				h.playScene.State.ShopGoods = append(h.playScene.State.ShopGoods, ShopItem{ItemIdx: itemIdx, Price: price})
			}
		}

	case protocol.SMSendUserSell:
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 1
		h.playScene.State.ShopNpcID = msg.Recog
		h.playScene.sellItem = nil
		h.playScene.sellPriceStr = ""

	case protocol.SMSendUserRepair:
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 2
		h.playScene.State.ShopNpcID = msg.Recog
		h.playScene.sellItem = nil
		h.playScene.sellPriceStr = ""

	case protocol.SMBuyItemSuccess:
		log.Logf(log.LevelInfo, "Client", "Buy success")

	case protocol.SMBuyItemFail:
		log.Logf(log.LevelInfo, "Client", "Buy failed: code=%d", msg.Recog)

	case protocol.SMUserSellItemOK:
		log.Logf(log.LevelInfo, "Client", "Sell success")

	case protocol.SMUserSellItemFail:
		log.Logf(log.LevelInfo, "Client", "Sell failed")

	case protocol.SMSendBuyPrice:
		h.playScene.sellPriceStr = strconv.Itoa(int(msg.Recog))
		log.Logf(log.LevelInfo, "Client", "Buy price: %d", msg.Recog)

	case protocol.SMUserRepairItemOK:
		log.Logf(log.LevelInfo, "Client", "Repair success")

	case protocol.SMUserRepairItemFail:
		log.Logf(log.LevelInfo, "Client", "Repair failed")

	case protocol.SMSendRepairCost:
		h.playScene.sellPriceStr = strconv.Itoa(int(msg.Recog))
		log.Logf(log.LevelInfo, "Client", "Repair cost: %d", msg.Recog)

	case protocol.SMSendUserStorageItem:
		// Storage reuses the shop panels (Delphi BoStorageMenu): the list is
		// DMenuDlg in storage mode, deposits go through the DSellDlg spot.
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 3
		h.playScene.menuTop = 0
		h.playScene.menuIndex = -1
		h.playScene.sellItem = nil
		h.playScene.sellPriceStr = ""

	case protocol.SMSaveItemList:
		// Storage list rows: name from the item DB, "price" carries the
		// MakeIndex used to take the item back (Delphi
		// ClientGetSaveItemList, ClMain.pas:5651-5690).
		st := h.playScene.State
		st.ShowShop = true
		st.ShopMode = 3
		st.ShopGoods = nil
		raw := []byte(body)
		if len(raw) >= 2 {
			count := int(binary.LittleEndian.Uint16(raw[0:2]))
			st.StorageItems = make([]BagItem, 0, count)
			for i := 0; i < count; i++ {
				off := 2 + i*10
				if off+10 > len(raw) {
					break
				}
				item := BagItem{
					Idx:       binary.LittleEndian.Uint16(raw[off : off+2]),
					Dura:      binary.LittleEndian.Uint16(raw[off+2 : off+4]),
					DuraMax:   binary.LittleEndian.Uint16(raw[off+4 : off+6]),
					MakeIndex: int32(binary.LittleEndian.Uint32(raw[off+6 : off+10])),
				}
				item.Def = st.ItemDefs[int(item.Idx)]
				st.StorageItems = append(st.StorageItems, item)
				name := ""
				if item.Def != nil {
					name = item.Def.Name
				}
				st.ShopGoods = append(st.ShopGoods, ShopItem{
					ItemIdx: item.Idx,
					Price:   int(item.MakeIndex),
					Name:    name,
				})
			}
		}

	case protocol.SMStorageFail:
		log.Logf(log.LevelInfo, "Client", "Storage failed")

	case protocol.SMTakeBackStorageItemFail:
		log.Logf(log.LevelInfo, "Client", "Take back storage failed")

	case protocol.SMGroupModeChanged:
		h.playScene.State.AllowGroup = msg.Recog != 0

	case protocol.SMCreateGroupOK:
		h.playScene.addChatMessage("[Group] created")

	case protocol.SMCreateGroupFail:
		h.playScene.addChatMessage("[Group] create failed (target unavailable?)")

	case protocol.SMGroupAddMemOK:
		h.playScene.addChatMessage("[Group] added " + body)

	case protocol.SMGroupAddMemFail:
		h.playScene.addChatMessage("[Group] add failed")

	case protocol.SMGroupDelMemOK:
		h.playScene.addChatMessage("[Group] removed " + body)

	case protocol.SMGroupDelMemFail:
		h.playScene.addChatMessage("[Group] remove failed")

	case protocol.SMGroupCancel:
		h.playScene.State.GroupMembers = nil
		h.playScene.addChatMessage("[Group] disbanded")

	case protocol.SMGroupMembers:
		if strings.TrimSpace(body) == "" {
			h.playScene.State.GroupMembers = nil
		} else {
			h.playScene.State.GroupMembers = strings.Split(body, "\n")
		}

	case protocol.SMOpenGuildDlg:
		// Body: name\nrank\nperm\nnotice (server HandleOpenGuildDlg).
		st := h.playScene.State
		parts := strings.SplitN(body, "\n", 4)
		if len(parts) > 0 {
			st.GuildName = parts[0]
		}
		if len(parts) > 1 {
			st.GuildRank = parts[1]
		}
		if len(parts) > 2 {
			st.GuildCommander = parts[2] == "1"
		}
		if len(parts) > 3 {
			st.GuildNotice = parts[3]
		}
		st.ShowGuild = true
		st.GuildTopLine = 0

	case protocol.SMOpenGuildDlgFail:
		h.playScene.addChatMessage("You are not in a guild")

	case protocol.SMChangeGuildName:
		h.playScene.State.GuildName = body

	case protocol.SMSendGuildMemberList:
		if strings.TrimSpace(body) == "" {
			h.playScene.State.GuildMembers = nil
		} else {
			h.playScene.State.GuildMembers = strings.Split(body, "\n")
		}

	case protocol.SMGuildAddMemberOK:
		h.playScene.addChatMessage("[Guild] added " + body)

	case protocol.SMGuildAddMemberFail:
		h.playScene.addChatMessage("[Guild] add failed")

	case protocol.SMGuildDelMemberOK:
		h.playScene.addChatMessage("[Guild] removed " + body)

	case protocol.SMGuildDelMemberFail:
		h.playScene.addChatMessage("[Guild] remove failed")

	case protocol.SMBuildGuildFail:
		h.playScene.addChatMessage("Guild creation failed")

	case protocol.SMGuildMakeAllyOK:
		h.playScene.addChatMessage("[Guild] alliance formed")

	case protocol.SMGuildMakeAllyFail:
		h.playScene.addChatMessage("[Guild] alliance failed")

	case protocol.SMGuildBreakAllyOK:
		h.playScene.addChatMessage("[Guild] alliance broken")

	case protocol.SMGuildBreakAllyFail:
		h.playScene.addChatMessage("[Guild] break ally failed")

	case protocol.SMDlgMsg:
		h.playScene.addChatMessage(body)

	case protocol.SMDealTryFail:
		log.Logf(log.LevelInfo, "Client", "Trade request failed")

	case protocol.SMDealAddItemOK:
		// Recog = grid slot; body carries the offered item.
		if item := h.parseTradeItem(body); item != nil {
			slot := int(msg.Recog)
			if slot >= 0 && slot < len(h.playScene.State.DealItems) {
				h.playScene.State.DealItems[slot] = item
			}
		}

	case protocol.SMDealAddItemFail:
		log.Logf(log.LevelDebug, "Client", "Trade add item failed")

	case protocol.SMDealDelItemOK:
		// Recog = MakeIndex taken back; the bag re-sync returns it.
		st := h.playScene.State
		for i, it := range st.DealItems {
			if it != nil && it.MakeIndex == msg.Recog {
				st.DealItems[i] = nil
				break
			}
		}

	case protocol.SMDealDelItemFail:
		log.Logf(log.LevelDebug, "Client", "Trade del item failed")

	case protocol.SMDealRemoteAddItem:
		if item := h.parseTradeItem(body); item != nil {
			slot := int(msg.Recog)
			if slot >= 0 && slot < len(h.playScene.State.DealRemoteItems) {
				h.playScene.State.DealRemoteItems[slot] = item
			}
		}

	case protocol.SMDealRemoteDelItem:
		st := h.playScene.State
		for i, it := range st.DealRemoteItems {
			if it != nil && it.MakeIndex == msg.Recog {
				st.DealRemoteItems[i] = nil
				break
			}
		}

	case protocol.SMSpaceMoveHide:
		log.Logf(log.LevelDebug, "Client", "Space move hide: %d", msg.Recog)

	case protocol.SMSpaceMoveShow:
		log.Logf(log.LevelDebug, "Client", "Space move show: %d", msg.Recog)

	case protocol.SMOpenHealth:
		log.Logf(log.LevelDebug, "Client", "Open health: %d", msg.Recog)

	case protocol.SMCloseHealth:
		log.Logf(log.LevelDebug, "Client", "Close health: %d", msg.Recog)

	case protocol.SMBreakWeapon:
		log.Logf(log.LevelInfo, "Client", "Weapon broken!")

	case protocol.SMButch:
		log.Logf(log.LevelDebug, "Client", "Butch")

	case protocol.SMReadMinimapOK:
		log.Logf(log.LevelDebug, "Client", "Minimap data received")

	case protocol.SMMonsterSay:
		log.Logf(log.LevelDebug, "Client", "Monster say: %s", body)

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

	playScene.SetSendMove(func(ident int, dir int) {
		moveMsg := protocol.MakeDefaultMsg(uint16(ident), 0, uint16(dir), 0, 0)
		handler.Send(moveMsg, "")
	})

	playScene.SetSendAttack(func(ident int, dir int) {
		hitMsg := protocol.MakeDefaultMsg(uint16(ident), 0, uint16(dir), 0, 0)
		handler.Send(hitMsg, "")
	})

	playScene.SetSendPickup(func() {
		pickupMsg := protocol.MakeDefaultMsg(protocol.CMPickup, 0, 0, 0, 0)
		handler.Send(pickupMsg, "")
	})

	playScene.SetSendChat(func(text string) {
		sayMsg := protocol.MakeDefaultMsg(protocol.CMSay, 0, 0, 0, 0)
		handler.Send(sayMsg, text)
	})

	playScene.SetSendSpell(func(magID int, x, y int) {
		spellMsg := protocol.MakeDefaultMsg(protocol.CMSpell, 0, uint16(magID), uint16(x), uint16(y))
		handler.Send(spellMsg, "")
	})

	playScene.SetSendNpcClick(func(npcID int) {
		clickMsg := protocol.MakeDefaultMsg(protocol.CMClickNPC, int32(npcID), 0, 0, 0)
		handler.Send(clickMsg, "")
	})

	playScene.SetSendDealCancel(func() {
		cancelMsg := protocol.MakeDefaultMsg(protocol.CMDealCancel, 0, 0, 0, 0)
		handler.Send(cancelMsg, "")
	})

	playScene.SetSendUseItem(func(makeIndex int32) {
		// Recog carries the 32-bit MakeIndex (Delphi SendEatItem convention).
		msg := protocol.MakeDefaultMsg(protocol.CMEat, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendAttackMode(func(mode int) {
		msg := protocol.MakeDefaultMsg(protocol.CMChangeAttackMode, 0, uint16(mode), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendBuyItem(func(itemIdx int) {
		msg := protocol.MakeDefaultMsg(protocol.CMUserBuyItem, 0, uint16(itemIdx), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendSellItem(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMUserSellItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDropItem(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMDropItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDropGold(func(amount int) {
		msg := protocol.MakeDefaultMsg(protocol.CMDropGold, int32(amount), 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDealTry(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMDealTry, 0, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendTakeOn(func(makeIndex int32, slot int) {
		msg := protocol.MakeDefaultMsg(protocol.CMTakeOnItem, makeIndex, uint16(slot), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendTakeOff(func(slot int) {
		msg := protocol.MakeDefaultMsg(protocol.CMTakeOffItem, 0, uint16(slot), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendMagicKey(func(magID, key int) {
		msg := protocol.MakeDefaultMsg(protocol.CMMagicKeyChange, int32(magID), uint16(key), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendMerchantSelect(func(npcID int32, tag string) {
		msg := protocol.MakeDefaultMsg(protocol.CMMerchantDlgSelect, npcID, 0, 0, 0)
		handler.Send(msg, tag)
	})

	playScene.SetSendQueryPrice(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMMerchantQuerySellPrice, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendQueryRepair(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMMerchantQueryRepairCost, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendRepairItem(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMUserRepairItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendStorageItem(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMUserStorageItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendTakeBackStorage(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMUserTakeBackStorageItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDealAdd(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMDealAddItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDealDel(func(makeIndex int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMDealDelItem, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDealChgGold(func(amount int) {
		msg := protocol.MakeDefaultMsg(protocol.CMDealChgGold, int32(amount), 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendDealEnd(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMDealEnd, 0, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendOpenGuild(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMOpenGuildDlg, 0, 0, 0, 0)
		handler.Send(msg, "")
	})
	playScene.SetSendGuildMemberList(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildMemberList, 0, 0, 0, 0)
		handler.Send(msg, "")
	})
	playScene.SetSendGuildAdd(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildAddMember, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendGuildDel(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildDelMember, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendGuildUpdateNotice(func(text string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildUpdateNotice, 0, 0, 0, 0)
		handler.Send(msg, text)
	})
	playScene.SetSendGuildUpdateRank(func(text string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildUpdateRankInfo, 0, 0, 0, 0)
		handler.Send(msg, text)
	})
	playScene.SetSendGuildAlly(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildAlly, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendGuildBreakAlly(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildBreakAlly, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendGuildHome(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMGuildHome, 0, 0, 0, 0)
		handler.Send(msg, "")
	})
	playScene.SetSendGroupMode(func(allow int) {
		msg := protocol.MakeDefaultMsg(protocol.CMGroupMode, 0, uint16(allow), 0, 0)
		handler.Send(msg, "")
	})
	playScene.SetSendCreateGroup(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMCreateGroup, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendAddGroupMember(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMAddGroupMember, 0, 0, 0, 0)
		handler.Send(msg, name)
	})
	playScene.SetSendDelGroupMember(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMDelGroupMember, 0, 0, 0, 0)
		handler.Send(msg, name)
	})

	playScene.SetSendAdjustBonus(func(remaining int, deltas [9]int) {
		buf := make([]byte, 18)
		for i, d := range deltas {
			binary.LittleEndian.PutUint16(buf[i*2:i*2+2], uint16(d))
		}
		// Send encodes the body once; pass raw bytes.
		msg := protocol.MakeDefaultMsg(protocol.CMAdjustBonus, int32(remaining), 0, 0, 0)
		handler.Send(msg, string(buf))
	})

	playScene.SetSendQueryUserState(func(targetID int32) {
		msg := protocol.MakeDefaultMsg(protocol.CMQueryUserState, targetID, 0, 0, 0)
		handler.Send(msg, "")
	})

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
	glState.DrawQuadColor(0, 0, ScreenWidth, ScreenHeight, r, g, b, 1.0, proj)
	glState.DrawQuadColor(350, 250, 100, 100, 1.0, 1.0, 1.0, 1.0, proj)
}
func (s *DebugScene) OnKey(key int, action int)                    {}
func (s *DebugScene) OnMouse(x, y float64, button int, action int) {}
func (s *DebugScene) OnScroll(x, y float64)                        {}

// parseTradeItem decodes the SMDealAddItemOK/SMDealRemoteAddItem payload
// (server encodeDealItem: MakeIndex i32, WIndex u16, Dura u16, DuraMax u16).
func (h *NetHandler) parseTradeItem(body string) *BagItem {
	raw := []byte(body)
	if len(raw) < 10 {
		return nil
	}
	item := &BagItem{
		MakeIndex: int32(binary.LittleEndian.Uint32(raw[0:4])),
		Idx:       binary.LittleEndian.Uint16(raw[4:6]),
		Dura:      binary.LittleEndian.Uint16(raw[6:8]),
		DuraMax:   binary.LittleEndian.Uint16(raw[8:10]),
	}
	item.Def = h.playScene.State.ItemDefs[int(item.Idx)]
	return item
}
