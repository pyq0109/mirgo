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
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// fade 实现 Delphi MakeDark 渐变过渡（ClMain.pas:1114-1130）。
// g_nFadeIndex 范围 1（全暗）到 29（全亮）；每帧 index 按 step 移动，
// 到达边界后触发 onDone。
type fadeState struct {
	active bool
	index  float64 // 1..29
	step   float64 // +2（淡入）、-2（淡出）、-4（快速淡出）
	onDone func()
}

func (f *fadeState) startOut(fast bool, done func()) {
	f.active = true
	f.index = 29
	if fast {
		f.step = -4
	} else {
		f.step = -2
	}
	f.onDone = done
}

func (f *fadeState) startIn(done func()) {
	f.active = true
	f.index = 1
	f.step = 2
	f.onDone = done
}

func (f *fadeState) tick() {
	if !f.active {
		return
	}
	f.index += f.step
	if (f.step > 0 && f.index >= 29) || (f.step < 0 && f.index <= 1) {
		if f.step > 0 {
			f.index = 29
		} else {
			f.index = 1
		}
		f.active = false
		if f.onDone != nil {
			fn := f.onDone
			f.onDone = nil
			fn()
		}
	}
}

func (f *fadeState) alpha() float32 {
	if !f.active {
		return 0
	}
	return float32(1 - f.index/30)
}

var globalFade fadeState

const (
	clientVersion = 120040918
	runLoginCode  = 9
)

func init() {
	runtime.LockOSThread()
}

func main() {
	dataDir := flag.String("data", "asset/client/Data", "client data directory path")
	mapDir := flag.String("maps", "asset/client/Map", "map directory path")
	serverAddr := flag.String("server", "localhost:7000", "server address")
	logLevel := flag.String("loglevel", "debug", "log level: trace/debug/info/warn/error")
	flag.Parse()

	log.SetLevel(log.ParseLevel(*logLevel))
	log.Logf(log.LevelInfo, "Client", "starting MIR2 client...")
	log.Logf(log.LevelInfo, "Client", "server: %s", *serverAddr)

	// Delphi 固定运行在 800×600（SWH800, Share.pas:22-24）。
	window, err := engine.NewWindow(800, 600, "MIR2 Client")
	if err != nil {
		log.Logf(log.LevelError, "Client", "failed to create window: %v", err)
		os.Exit(1)
	}
	window.SetResizable(true)
	defer window.Destroy()

	glState, err := engine.NewGLState()
	if err != nil {
		log.Logf(log.LevelError, "Client", "failed to create GL state: %v", err)
		os.Exit(1)
	}
	defer glState.Destroy()

	resources, err := engine.NewResourceManager(*dataDir, glState)
	if err != nil {
		log.Logf(log.LevelError, "Client", "failed to load resources: %v", err)
		os.Exit(1)
	}
	defer resources.Destroy()
	log.Logf(log.LevelInfo, "Client", "WIL resources loaded")

	var sndErr error
	gSound, sndErr = NewSoundEngine(*dataDir)
	if sndErr != nil {
		log.Logf(log.LevelWarn, "Client", "sound init failed: %v (silent mode)", sndErr)
		gSound = nil
	}
	defer func() {
		if gSound != nil {
			gSound.Close()
		}
	}()

	textRenderer, err := engine.NewTextRenderer(glState, "", 9)
	if err != nil {
		log.Logf(log.LevelWarn, "Client", "failed to load font: %v", err)
	}
	defer func() {
		if textRenderer != nil {
			textRenderer.Destroy()
		}
	}()

	sceneMgr := engine.NewSceneManager()

	// 全局调试控制台 (跨场景可用)。使用 8pt 小字体 (Delphi 物品提示 8pt), 加载失败时退回主字体。
	var consoleText *engine.TextRenderer
	if textRenderer != nil {
		if small, err := textRenderer.WithSize(8); err == nil {
			consoleText = small
		} else {
			consoleText = textRenderer
		}
	}
	dbgConsole := NewDebugConsole(glState, consoleText, sceneMgr)
	gDebug = dbgConsole

	playScene := NewPlayScene(glState, resources, *mapDir, dbgConsole)
	playScene.SetText(textRenderer)
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
	dbgConsole.SetClipboard = glfwWindow.SetClipboardString
	dbgConsole.GetClipboard = glfwWindow.GetClipboardString

	winW, winH = glfwWindow.GetSize()
	glfwWindow.SetSizeLimits(ScreenWidth, ScreenHeight, glfw.DontCare, glfw.DontCare)
	glfwWindow.SetSizeCallback(func(_ *glfw.Window, w, h int) {
		winW, winH = w, h
		playScene.OnResize()
	})

	// 连接登录场景回调。
	loginScene.SetLoginFunc(func(id, password string) {
		log.Logf(log.LevelInfo, "Client", "[callback] LoginFunc called: id=%s", id)
		if handler != nil {
			log.Logf(log.LevelWarn, "Client", "[callback] LoginFunc: handler already exists, skipping")
			return
		}
		var err error
		log.Logf(log.LevelInfo, "Client", "[callback] LoginFunc: connecting to %s...", *serverAddr)
		handler, err = connectToServer(*serverAddr, loginScene, playScene, selectChrScene, noticeScene, sceneMgr)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[callback] LoginFunc: connection failed: %v", err)
			loginScene.SetError("连接服务器失败")
			handler = nil
			return
		}
		handler.onFail = func() {
			log.Logf(log.LevelInfo, "Client", "[callback] onFail: resetting handler")
			handler = nil
		}
		handler.loginID = id
		log.Logf(log.LevelInfo, "Client", "[callback] LoginFunc: sending login id=%s", id)
		handler.SendLogin(id, password)
	})
	loginScene.SetCloseFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[callback] CloseFunc: closing window")
		glfwWindow.SetShouldClose(true)
	})
	loginScene.SetRegisterFunc(func(ue protocol.UserEntry, ua protocol.UserEntryAdd) {
		log.Logf(log.LevelInfo, "Client", "[callback] RegisterFunc: id=%s", ue.Account())
		if handler == nil {
			var err error
			handler, err = connectToServer(*serverAddr, loginScene, playScene, selectChrScene, noticeScene, sceneMgr)
			if err != nil {
				log.Logf(log.LevelError, "Client", "[callback] RegisterFunc: connection failed: %v", err)
				loginScene.SetError("连接服务器失败")
				handler = nil
				return
			}
			handler.onFail = func() {
				handler = nil
			}
		}
		handler.registerID = ue.Account()
		regMsg := protocol.MakeDefaultMsg(protocol.CMAddNewUser, 0, 0, 0, 0)
		// Body = EncodeBuffer(TUserEntry) + EncodeBuffer(TUserEntryAdd)（ClMain.pas:2844）。
		handler.SendEncoded(regMsg, protocol.EncodeBuffer(ue.Bytes())+protocol.EncodeBuffer(ua.Bytes()))
	})
	loginScene.SetChgPwFunc(func(id, oldpw, newpw string) {
		log.Logf(log.LevelInfo, "Client", "[callback] ChgPwFunc: id=%s", id)
		if handler == nil {
			var err error
			handler, err = connectToServer(*serverAddr, loginScene, playScene, selectChrScene, noticeScene, sceneMgr)
			if err != nil {
				log.Logf(log.LevelError, "Client", "[callback] ChgPwFunc: connection failed: %v", err)
				loginScene.SetError("连接服务器失败")
				handler = nil
				return
			}
			handler.onFail = func() {
				handler = nil
			}
		}
		handler.SendChgPw(id, oldpw, newpw)
	})

	// 连接选服功能（DSelServerDlg 覆盖在登录场景上）。关闭按钮复用
	// loginScene.closeFunc，会退出程序。
	loginScene.SetSelectFunc(func(serverName string) {
		log.Logf(log.LevelInfo, "Client", "[callback] ServerSelectFunc: server=%s", serverName)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[callback] ServerSelectFunc: handler is nil")
			return
		}
		handler.SendSelectServer(serverName)
	})

	// 连接选角场景回调。
	selectChrScene.SetStartFunc(func(charName string) {
		log.Logf(log.LevelInfo, "Client", "[callback] ChrStartFunc: char=%s", charName)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[callback] ChrStartFunc: handler is nil")
			return
		}
		handler.charName = charName
		handler.SendSelChr(charName)
	})
	selectChrScene.SetNewChrFunc(func(name string, hair, job, sex int) {
		log.Logf(log.LevelInfo, "Client", "[callback] ChrNewFunc: name=%s hair=%d job=%d sex=%d", name, hair, job, sex)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[callback] ChrNewFunc: handler is nil")
			return
		}
		handler.SendNewChr(name, hair, job, sex)
	})
	selectChrScene.SetDelChrFunc(func(name string) {
		log.Logf(log.LevelInfo, "Client", "[callback] ChrDelFunc: name=%s", name)
		if handler == nil {
			log.Logf(log.LevelWarn, "Client", "[callback] ChrDelFunc: handler is nil")
			return
		}
		handler.SendDelChr(name)
	})
	selectChrScene.SetExitFunc(func() {
		log.Logf(log.LevelInfo, "Client", "[callback] ChrExitFunc: returning to login")
		if handler != nil {
			handler.Close()
			handler = nil
		}
		sceneMgr.ChangeScene(engine.SceneLogin)
	})

	glfwWindow.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		log.Logf(log.LevelTrace, "Key", "key=%d action=%d mods=%d scene=%s",
			int(key), int(action), int(mods), sceneMgr.CurrentType())
		// 反引号 (`) 切换调试控制台, 跨场景生效。
		if key == glfw.KeyGraveAccent && action == glfw.Press {
			dbgConsole.Toggle()
			return
		}
		// 控制台打开时所有按键交给控制台, 不再转发给场景。
		if dbgConsole.Visible {
			dbgConsole.OnKey(int(key), int(action), int(mods))
			return
		}
		// Esc 仅在游戏前场景退出；原版游戏中没有全局 Esc 退出，
		// 游戏内通过 DBotExit 按钮退出（ClMain:1575-1612）。
		if action == glfw.Press && key == glfw.KeyEscape && sceneMgr.CurrentType() != engine.ScenePlayGame {
			if handler != nil {
				handler.Close()
				handler = nil
			}
			w.SetShouldClose(true)
		}
		sceneMgr.OnKey(int(key), int(action))
	})

	glfwWindow.SetCharCallback(func(w *glfw.Window, char rune) {
		log.Logf(log.LevelTrace, "Key", "char=%q scene=%s", char, sceneMgr.CurrentType())
		if dbgConsole.Visible {
			dbgConsole.OnChar(char)
			return
		}
		sceneMgr.OnChar(char)
	})

	glfwWindow.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		switch action {
		case glfw.Press:
			x, y := w.GetCursorPos()
			log.Logf(log.LevelTrace, "Mouse", "press button=%d mods=%d pos=(%.0f,%.0f) scene=%s",
				int(button), int(mods), x, y, sceneMgr.CurrentType())
			dbgConsole.SetMouse(x, y)
			if dbgConsole.Visible {
				if dbgConsole.OnMouseButton(x, y, int(button), 1) {
					return
				}
			}
			// 非游戏场景的线框点击锁定 (屏幕空间)。游戏场景由
			// PlayScene 自行在世界空间处理。
			if button == glfw.MouseButtonLeft && dbgConsole.WireMode > 0 &&
				sceneMgr.CurrentType() != engine.ScenePlayGame && dbgConsole.ClickInspectScreen() {
				return
			}
			sceneMgr.OnMouse(x, y, int(button), 1, int(mods))
		case glfw.Release:
			x, y := w.GetCursorPos()
			log.Logf(log.LevelTrace, "Mouse", "release button=%d pos=(%.0f,%.0f) scene=%s",
				int(button), x, y, sceneMgr.CurrentType())
			if dbgConsole.Visible {
				dbgConsole.OnMouseButton(x, y, int(button), 0)
			}
			sceneMgr.OnMouse(x, y, int(button), 0, int(mods))
		}
	})

	glfwWindow.SetCursorPosCallback(func(w *glfw.Window, xpos, ypos float64) {
		dbgConsole.SetMouse(xpos, ypos)
		if dbgConsole.Visible {
			dbgConsole.OnMouseMoveSelect(xpos, ypos)
		}
		sceneMgr.OnMouseMove(xpos, ypos)
	})

	glfwWindow.SetScrollCallback(func(w *glfw.Window, xoff, yoff float64) {
		if dbgConsole.Visible {
			dbgConsole.OnScroll(yoff)
			return
		}
		sceneMgr.OnScroll(xoff, yoff)
	})

	log.Logf(log.LevelInfo, "Client", "login scene ready")
	window.Run(func(dt float64) {
		// 在主线程上分发读协程排队的网络消息，在场景更新之前执行，
		// 确保所有状态修改都是单线程的。
		if handler != nil {
			handler.Pump()
		}
		sceneMgr.Update(dt)
		globalFade.tick()
	}, func() {
		w, h := window.GetFramebufferSize()
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		glState.SetViewport(0, 0, int32(w), int32(h))
		proj := engine.OrthoProj(float32(winW), float32(winH))

		// 统一线框录制: 对所有场景 blanket 录制 (分类 0)。PlayScene 在
		// 内部做分段录制并置 wireHandled=true, 从而覆盖这里的设置。
		dbgConsole.wireHandled = false
		if dbgConsole.WireMode > 0 {
			glState.WireBounds = glState.WireBounds[:0]
			glState.WireRecording = true
			glState.WireRecord = true
			glState.WireCategory = 0
		}
		sceneMgr.Render(glState, proj)
		if dbgConsole.WireMode > 0 {
			glState.WireRecording = false
		}

		if a := globalFade.alpha(); a > 0 {
			glState.DrawQuadColor(0, 0, float32(winW), float32(winH), 0, 0, 0, a, proj)
		}

		// 调试控制台叠加层 (场景之后渲染, 重置为完整 800×600 视口)。
		glState.SetViewport(0, 0, int32(w), int32(h))
		if dbgConsole.WireMode > 0 && !dbgConsole.wireHandled {
			dbgConsole.RenderWireOverlay(proj)
		}
		dbgConsole.Render(proj)
	})

	if handler != nil {
		handler.Close()
	}
	log.Logf(log.LevelInfo, "Client", "client stopped")
}

// ============================================================================
// NetHandler
// ============================================================================

// netEvent 是由读协程排队、通过 Pump 在主线程分发的已解码服务器消息，
// 确保所有 actor/scene 修改都是单线程的（ReadLoop 和渲染循环之间无数据竞争）。
type netEvent struct {
	isCtrl  bool
	ctrl    string
	msg     protocol.DefaultMessage
	body    string
	rawBody string // 未解码的 body 原文（SMTurn 需按 Delphi 方式分段解码名称）
}

// NetHandler 处理网络通信。
type NetHandler struct {
	conn           net.Conn
	loginScene     *LoginScene
	playScene      *PlayScene
	selectChrScene *SelectChrScene
	noticeScene    *NoticeScene
	sceneMgr       *engine.SceneManager
	code           byte
	done           chan struct{}

	// 认证状态
	loginID       string
	password      string // 保存密码用于重连后重新认证
	registerID    string // 最近提交的注册账号（Delphi MakeNewId, ClMain.pas:2842）
	certification int
	charName      string
	reconnecting  bool // 重连后等待重新认证时为 true

	// 入站队列：ReadLoop 追加，主线程通过 Pump 消费。
	queueMu sync.Mutex
	queue   []netEvent

	// ReadLoop 异常退出时向此 channel 发送错误，Pump 检查并触发断线处理。
	errCh chan error

	// 回调（由 main 设置）
	onReconnect func(addr string, loginID string, certification int)
	onFail      func() // 登录失败时调用，在 main 中重置 handler
}

// enqueue 追加已解码消息供主线程分发（从 ReadLoop 调用）。
func (h *NetHandler) enqueue(e netEvent) {
	h.queueMu.Lock()
	h.queue = append(h.queue, e)
	h.queueMu.Unlock()
}

// Pump 消费入站队列并在调用方（主）线程上分发。
// 每帧在场景更新前调用一次。
func (h *NetHandler) Pump() {
	// 检查 ReadLoop 是否异常退出
	select {
	case err := <-h.errCh:
		log.Logf(log.LevelError, "Client", "connection lost: %v", err)
		if h.onFail != nil {
			h.onFail()
		}
		return
	default:
	}

	h.queueMu.Lock()
	events := h.queue
	h.queue = nil
	h.queueMu.Unlock()
	for _, e := range events {
		if e.isCtrl {
			h.handleControlMsg(e.ctrl)
		} else {
			h.HandleMessage(e.msg, e.body, e.rawBody)
		}
	}
}

// Close 停止读循环并关闭连接。
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

// Send 编码并发送消息到服务器。
func (h *NetHandler) Send(msg protocol.DefaultMessage, body string) error {
	log.Logf(log.LevelInfo, "Client", ">>> send %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
		protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)
	encoded := protocol.EncodeMessage(msg)
	if body != "" {
		encoded += protocol.EncodeString(body)
	}
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendEncoded 发送带有已编码 body 的消息。用于由多个独立 EncodeBuffer
// 段组成的 body（ClMain.pas:2844），这些段不能作为单个字符串通过 EncodeString。
func (h *NetHandler) SendEncoded(msg protocol.DefaultMessage, encodedBody string) error {
	log.Logf(log.LevelInfo, "Client", ">>> send %s Recog=%d Param=%d Tag=%d Series=%d (encoded body, %d chars)",
		protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, len(encodedBody))
	encoded := protocol.EncodeMessage(msg) + encodedBody
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendChgPw 发送密码修改请求: id + #9 + passwd + #9 + newpasswd
// （ClMain.pas:2864-2870）。
func (h *NetHandler) SendChgPw(id, passwd, newpasswd string) {
	msg := protocol.MakeDefaultMsg(protocol.CMChangePassword, 0, 0, 0, 0)
	h.Send(msg, id+"\t"+passwd+"\t"+newpasswd)
}

// SendRawString 发送不带 TDefaultMessage 头的原始字符串。
func (h *NetHandler) SendRawString(s string) error {
	log.Logf(log.LevelInfo, "Client", ">>> send RAW %q", s)
	encoded := protocol.EncodeString(s)
	frame := protocol.FormatClientFrame(encoded, &h.code)
	_, err := h.conn.Write([]byte(frame))
	return err
}

// SendLogin 发送登录凭据。
func (h *NetHandler) SendLogin(id, password string) {
	h.loginID = id
	h.password = password
	loginMsg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	h.Send(loginMsg, id+"/"+password)
}

// SendSelectServer 发送选服请求。
func (h *NetHandler) SendSelectServer(serverName string) {
	selMsg := protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0)
	h.Send(selMsg, serverName)
}

// SendQueryChr 查询角色列表（包含 loginId/certification）。
func (h *NetHandler) SendQueryChr() {
	queryMsg := protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0)
	h.Send(queryMsg, fmt.Sprintf("%s/%d", h.loginID, h.certification))
}

// SendSelChr 发送选角请求。
func (h *NetHandler) SendSelChr(charName string) {
	selMsg := protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0)
	h.Send(selMsg, h.loginID+"/"+charName)
}

// SendNewChr 发送创建角色请求。
func (h *NetHandler) SendNewChr(name string, hair, job, sex int) {
	msg := protocol.MakeDefaultMsg(protocol.CMNewChr, 0, 0, 0, 0)
	h.Send(msg, fmt.Sprintf("%s/%s/%d/%d/%d", h.loginID, name, hair, job, sex))
}

// SendDelChr 发送删除角色请求。
func (h *NetHandler) SendDelChr(name string) {
	msg := protocol.MakeDefaultMsg(protocol.CMDelChr, 0, 0, 0, 0)
	h.Send(msg, name)
}

// SendRunLogin 发送进入游戏服务器的 run login。
func (h *NetHandler) SendRunLogin() {
	s := fmt.Sprintf("**%s/%s/%d/%d/%d", h.loginID, h.charName, h.certification, clientVersion, runLoginCode)
	h.SendRawString(s)
}

// Reconnect 断开并重连到新的服务器地址。
func (h *NetHandler) Reconnect(addr string) error {
	log.Logf(log.LevelInfo, "Client", "reconnect: disconnecting from current server")
	// 停止旧的读循环
	select {
	case <-h.done:
		log.Logf(log.LevelDebug, "Client", "reconnect: done channel already closed")
	default:
		close(h.done)
	}
	h.conn.Close()
	log.Logf(log.LevelInfo, "Client", "reconnect: old connection closed, waiting 100ms...")

	// 短暂等待读循环退出
	time.Sleep(100 * time.Millisecond)

	// 连接新服务器
	log.Logf(log.LevelInfo, "Client", "reconnect: connecting to %s...", addr)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		log.Logf(log.LevelError, "Client", "reconnect: failed to connect to %s: %v", addr, err)
		return fmt.Errorf("reconnect to %s: %w", addr, err)
	}
	log.Logf(log.LevelInfo, "Client", "reconnect: connected to %s", addr)

	h.conn = conn
	h.done = make(chan struct{})
	h.errCh = make(chan error, 1)
	h.code = 0

	// 启动新的读循环
	log.Logf(log.LevelInfo, "Client", "reconnect: starting new ReadLoop")
	go h.ReadLoop()
	return nil
}

// ReadLoop 从服务器读取消息。
func (h *NetHandler) ReadLoop() {
	log.Logf(log.LevelInfo, "Client", "ReadLoop started")
	buf := make([]byte, 4096)
	var scanner protocol.FrameScanner
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
			select {
			case <-h.done:
				log.Logf(log.LevelInfo, "Client", "ReadLoop stopped (closed)")
				return
			default:
			}
			log.Logf(log.LevelError, "Client", "ReadLoop error: %v", err)
			h.signalError(fmt.Errorf("read: %w", err))
			return
		}

		if n > 0 {
			payloads, overflow := scanner.Feed(buf[:n], false, func() {
				// Delphi 协议：剥离 '*' 并回显
				h.conn.Write([]byte{'*'})
			})
			if overflow {
				log.Logf(log.LevelError, "Client", "receive buffer overflow, disconnecting")
				h.signalError(fmt.Errorf("receive buffer overflow"))
				return
			}
			for _, payload := range payloads {
				if len(payload) > 0 && payload[0] == '+' {
					h.enqueue(netEvent{isCtrl: true, ctrl: payload})
					continue
				}
				if len(payload) >= protocol.DefBlockSize {
					msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
					body := ""
					rawBody := ""
					if len(payload) > protocol.DefBlockSize {
						rawBody = payload[protocol.DefBlockSize:]
						body = protocol.DecodeString(rawBody)
					}
					h.enqueue(netEvent{msg: msg, body: body, rawBody: rawBody})
				}
			}
		}
	}
}

// signalError 向主线程报告 ReadLoop 异常退出（非阻塞，最多保留一个错误）。
func (h *NetHandler) signalError(err error) {
	select {
	case h.errCh <- err:
	default:
	}
}

func (h *NetHandler) handleControlMsg(payload string) {
	ps := h.playScene
	switch {
	case strings.HasPrefix(payload, "+GOOD"):
		log.Logf(log.LevelDebug, "Client", "<<< +GOOD")
		ps.ActionLock = false
	case strings.HasPrefix(payload, "+FAIL"):
		log.Logf(log.LevelDebug, "Client", "<<< +FAIL")
		ps.ActionLock = false
		ps.targetX = -1
		ps.targetY = -1
		ps.clearAutoPath()
		ps.actionFailLockUntil = time.Now().UnixMilli() + 1000
		if ps.State.MySelf != nil {
			ps.State.MySelf.MoveFail()
		}
	// 特殊攻击标记（ClMain:3625-3640）
	case strings.HasPrefix(payload, "+PWR"):
		ps.canPowerHit = true
	case strings.HasPrefix(payload, "+LNG"):
		ps.canLongHit = true
	case strings.HasPrefix(payload, "+ULNG"):
		ps.canLongHit = false
	case strings.HasPrefix(payload, "+WID"):
		ps.canWideHit = true
	case strings.HasPrefix(payload, "+UWID"):
		ps.canWideHit = false
	case strings.HasPrefix(payload, "+CRS"):
		ps.canCrsHit = true
	case strings.HasPrefix(payload, "+UCRS"):
		ps.canCrsHit = false
	case strings.HasPrefix(payload, "+TWN"):
		ps.canTwnHit = true
	case strings.HasPrefix(payload, "+UTWN"):
		ps.canTwnHit = false
	case strings.HasPrefix(payload, "+FIR"):
		ps.canFireHit = true
		ps.lastFireHitTick = time.Now().UnixMilli()
	case strings.HasPrefix(payload, "+UFIR"):
		ps.canFireHit = false
	case strings.HasPrefix(payload, "+STN"):
		ps.canStnHit = true
	case strings.HasPrefix(payload, "+USTN"):
		ps.canStnHit = false
	}
}

// turnCharDescEncodedLen 是 8 字节 TCharDesc 经 6Bit 编码后的字符数：ceil(8*4/3)=11。
// SMTurn body = EncodeBuffer(charDesc) + EncodeString(name)，两段需分别解码
// （整体解码会在段边界处错位），与 Delphi ClMain.pas:3907-3914 一致。
const turnCharDescEncodedLen = 11

// decodeTurnName 从 SMTurn 的原始编码 body 中提取角色/NPC 名称。
func decodeTurnName(rawBody string) string {
	if len(rawBody) <= turnCharDescEncodedLen {
		return ""
	}
	name := protocol.DecodeString(rawBody[turnCharDescEncodedLen:])
	// Delphi 名称可能带 "名字/名字颜色" 后缀，取 '/' 前的部分。
	if i := strings.IndexByte(name, '/'); i >= 0 {
		name = name[:i]
	}
	return name
}

// HandleMessage 处理服务器消息。
func (h *NetHandler) HandleMessage(msg protocol.DefaultMessage, body, rawBody string) {
	log.Logf(log.LevelInfo, "Client", "<<< recv %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
		protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)

	switch msg.Ident {

	// =====================================================================
	// 登录阶段
	// =====================================================================

	case protocol.SMPasswdFail:
		// 措辞与 ClMain.pas:3708-3713 保持一致。
		log.Logf(log.LevelWarn, "Client", "login failed: code=%d", msg.Recog)
		if h.loginScene != nil {
			switch msg.Recog {
			case -1:
				h.loginScene.SetError("密码错误！")
			case -2:
				h.loginScene.SetError("密码输入错误超过3次，您的账号被暂时锁定，请稍后再登录！")
			case -3:
				h.loginScene.SetError("这个账号已经登录，请稍后再登录！")
			case -4:
				h.loginScene.SetError("付费账号服务失败！\\请使用免费账号登录.\\或者申请付费注册.")
			case -5:
				h.loginScene.SetError("您的游戏账号被禁止了.")
			default:
				h.loginScene.SetError("ID不存在或者未知错误.请稍后重试.")
			}
		}
		// 关闭连接并重置 handler，以便用户重试
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMPassOKSelectServer:
		if h.reconnecting {
			// 重连后重新认证成功 — 切换到 LoginScene 播放开门动画
			h.reconnecting = false
			log.Logf(log.LevelInfo, "Client", "re-authentication succeeded, switching to LoginScene to play door animation")
			h.sceneMgr.ChangeScene(engine.SceneLogin)
			if h.loginScene != nil {
				h.loginScene.OpenLoginDoor()
				h.loginScene.SetDoorCompleteFunc(func() {
					log.Logf(log.LevelInfo, "Client", "door animation complete, switching to SelectChr")
					h.sceneMgr.ChangeScene(engine.SceneSelectChr)
					globalFade.startIn(nil)
					time.Sleep(100 * time.Millisecond)
					h.SendQueryChr()
				})
			}
		} else {
			// 首次登录 — 在登录场景上显示选服覆盖层
			//（Delphi DSelServerDlg, FState.pas:2453-2517）。
			log.Logf(log.LevelInfo, "Client", "login succeeded, showing server select")
			servers := parseServerList(body)
			if h.loginScene != nil {
				h.loginScene.ShowServerSelect(servers)
			}
		}

	case protocol.SMSelectServerOK:
		// 消息体: "selChrAddr/selChrPort/certification"
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] parsing body=%q", body)
		addr, cert, err := parseAddrPortCert(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[SMSelectServerOK] parse error: %v", err)
			return
		}
		h.certification = cert
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] addr=%s cert=%d", addr, cert)

		// 重连到选角服务器
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] reconnecting to %s...", addr)
		if err := h.Reconnect(addr); err != nil {
			log.Logf(log.LevelError, "Client", "[SMSelectServerOK] reconnect failed: %v", err)
			return
		}
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] reconnected, re-authenticating...")

		// 在新连接上重新认证
		h.reconnecting = true
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] setting reconnecting=true")
		protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, clientVersion, 0, 0, 0)
		h.Send(protoMsg, "")
		h.SendLogin(h.loginID, h.password)
		log.Logf(log.LevelInfo, "Client", "[SMSelectServerOK] re-auth sent, waiting for SM_PASSOKSELECTSERVER")

	case protocol.SMQueryChr:
		// 消息体: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
		log.Logf(log.LevelInfo, "Client", "received character list: %s", body)
		chars, selectedIdx := parseQueryChrBody(body)
		if h.selectChrScene != nil {
			h.selectChrScene.SetCharactersFromServer(chars, selectedIdx)
		}

	case protocol.SMQueryChrFail:
		log.Logf(log.LevelWarn, "Client", "character query failed")
		// 显示空选择
		if h.selectChrScene != nil {
			h.selectChrScene.SetCharactersFromServer(nil, -1)
		}

	case protocol.SMNewChrSuccess:
		log.Logf(log.LevelInfo, "Client", "character created")
		h.SendQueryChr()

	case protocol.SMNewChrFail:
		log.Logf(log.LevelWarn, "Client", "failed to create character: code=%d", msg.Recog)
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
		log.Logf(log.LevelInfo, "Client", "character deleted")
		h.SendQueryChr()

	case protocol.SMDelChrFail:
		log.Logf(log.LevelWarn, "Client", "failed to delete character")

	// =====================================================================
	// 选角 → 进入游戏过渡
	// =====================================================================

	case protocol.SMStartPlay:
		// 消息体: "runAddr/runPort"
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] body=%q", body)
		_, err := parseAddrPort(body)
		if err != nil {
			log.Logf(log.LevelError, "Client", "[SMStartPlay] parse error: %v", err)
			return
		}
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] single server mode, sending run login")

		// 单服务器模式：在现有连接上发送 run login
		h.SendRunLogin()
		log.Logf(log.LevelInfo, "Client", "[SMStartPlay] run login sent, switching to LoginNotice scene")

		// Delphi: g_boDoFastFadeOut (IntroScn.pas:1199)
		globalFade.startOut(true, nil)
		h.sceneMgr.ChangeScene(engine.SceneLoginNotice)

	case protocol.SMStartFail:
		// Delphi: DMessageDlg('此服务器满员') 然后 ClientGetSelectServer
		//（ClMain.pas:3782-3788）。
		log.Logf(log.LevelWarn, "Client", "failed to start game: server full")
		h.sceneMgr.ChangeScene(engine.SceneLogin)
		if h.loginScene != nil {
			h.loginScene.SetError("此服务器满员，请稍后重试.")
			h.loginScene.ShowServerSelect(h.loginScene.Servers())
		}

	// =====================================================================
	// 公告阶段
	// =====================================================================

	case protocol.SMSendNotice:
		log.Logf(log.LevelInfo, "Client", "received notice, auto-confirming (Delphi empty scene)")
		h.Send(protocol.MakeDefaultMsg(protocol.CMLoginNoticeOK, 0, 0, 0, 0), "")

	// =====================================================================
	// 游戏阶段
	// =====================================================================

	case protocol.SMNewMap:
		mapName := body
		x := int(msg.Recog)
		y := int(msg.Param)
		log.Logf(log.LevelInfo, "Client", "map: %s (%d,%d)", mapName, x, y)
		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "failed to load map: %v", err)
			return
		}
		h.playScene.State.MapDarkness = int(msg.Tag)

	case protocol.SMLogon:
		log.Logf(log.LevelInfo, "Client", "game started (id=%d x=%d y=%d dir=%d)",
			msg.Recog, msg.Param, msg.Tag, msg.Series)
		actor := NewActor(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF)
		actor.Type = ActorHuman
		if body != "" {
			actor.updateFeatureFromBody(body)
			// Body 第 3 槽位携带职业（见 PlayObject.encodeLogonBody）。
			if raw := []byte(body); len(raw) >= 12 {
				h.playScene.State.Job = int(binary.LittleEndian.Uint32(raw[8:12]))
			}
		}
		h.playScene.State.MySelf = actor
		actor.IsSelf = true
		actor.MapRef = h.playScene.mapData
		h.playScene.State.Sex = actor.Sex
		h.playScene.State.Hair = actor.Hair
		h.playScene.State.Actors.Add(actor)
		actor.SendMsg(protocol.SMTurn, actor.CurrX, actor.CurrY, actor.Dir, 0, 0)
		h.sceneMgr.ChangeScene(engine.ScenePlayGame)
		globalFade.startIn(nil)
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
		if name := decodeTurnName(rawBody); name != "" {
			actor.UserName = name
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

	case protocol.SMHorseRun:
		if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
			break // 自身使用本地预测
		}
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(protocol.SMHorseRun, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMDisappear, protocol.SMGhost, protocol.SMHide:
		if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
			break
		}
		h.playScene.State.Actors.Remove(msg.Recog)

	case protocol.SMMoveFail:
		log.Logf(log.LevelDebug, "Client", "server returned move failure")
		h.playScene.ActionLock = false
		h.playScene.targetX = -1
		h.playScene.targetY = -1
		h.playScene.clearAutoPath()
		h.playScene.actionFailLockUntil = time.Now().UnixMilli() + 1000
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
		log.Logf(log.LevelInfo, "Client", "stats: level=%d hp=%d/%d mp=%d/%d exp=%d/%d weight=%d/%d",
			st.Level, st.HP, st.MaxHP, st.MP, st.MaxMP, st.Exp, st.MaxExp, st.Weight, st.MaxWeight)

	case protocol.SMStdItems:
		h.playScene.State.ParseItemDefs(body)
		log.Logf(log.LevelInfo, "Client", "item database synced: %d definitions", len(h.playScene.State.ItemDefs))

	case protocol.SMBagItems:
		log.Logf(log.LevelInfo, "Client", "received bag items: count=%d", msg.Recog)
		h.playScene.State.ParseBagItems(body)

	case protocol.SMVersionFail:
		log.Logf(log.LevelWarn, "Client", "version mismatch")
		if h.loginScene != nil {
			h.loginScene.SetError("客户端版本不匹配")
		}
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMLogoutOK:
		log.Logf(log.LevelInfo, "Client", "logout OK, returning to char select")
		h.sceneMgr.ChangeScene(engine.SceneSelectChr)

	case protocol.SMExitOK:
		log.Logf(log.LevelInfo, "Client", "exit OK, closing")
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMFriendList:
		h.parseFriendList(body)

	case protocol.SMAddFriendOK:
		name := protocol.DecodeString(body)
		h.playScene.AddChatMessage("已添加好友: " + name)
		if h.playScene.sendQueryFriends != nil {
			h.playScene.sendQueryFriends()
		}

	case protocol.SMAddFriendFail:
		h.playScene.AddChatMessage("添加好友失败")

	case protocol.SMDelFriendOK:
		name := protocol.DecodeString(body)
		h.playScene.AddChatMessage("已删除好友: " + name)
		if h.playScene.sendQueryFriends != nil {
			h.playScene.sendQueryFriends()
		}

	case protocol.SMDelFriendFail:
		h.playScene.AddChatMessage("删除好友失败")

	case protocol.SMFriendOnline:
		name := protocol.DecodeString(body)
		for i := range h.playScene.State.Friends {
			if h.playScene.State.Friends[i].Name == name {
				h.playScene.State.Friends[i].Online = true
			}
		}
		h.playScene.AddChatMessage(name + " 上线了")

	case protocol.SMFriendOffline:
		name := protocol.DecodeString(body)
		for i := range h.playScene.State.Friends {
			if h.playScene.State.Friends[i].Name == name {
				h.playScene.State.Friends[i].Online = false
			}
		}

	case protocol.SMCertificationFail:
		log.Logf(log.LevelWarn, "Client", "certification failed")
		if h.loginScene != nil {
			h.loginScene.SetError("认证失败")
		}
		h.Close()
		if h.onFail != nil {
			h.onFail()
		}

	case protocol.SMNewIDSuccess:
		log.Logf(log.LevelInfo, "Client", "registration succeeded")
		if h.loginScene != nil {
			h.loginScene.RegistrationDone()
		}

	case protocol.SMNewIDFail:
		// Recog 与 ClMain.pas:3694-3702 一致: 0=已存在, -2=繁忙, 其他=非法。
		log.Logf(log.LevelWarn, "Client", "registration failed: code=%d", msg.Recog)
		if h.loginScene != nil {
			switch msg.Recog {
			case 0:
				h.loginScene.RegistrationFailed("\"" + h.registerID + "\"这个账号已经存在了无法使用.\\ 请使用一个不同的名字.")
			case -2:
				h.loginScene.RegistrationFailed("创建账号失败，系统繁忙.")
			default:
				h.loginScene.RegistrationFailed(fmt.Sprintf("账号创建失败，请确认账号是否包含空格、非法字符. Code: %d", msg.Recog))
			}
		}

	case protocol.SMChgPasswdSuccess:
		log.Logf(log.LevelInfo, "Client", "password changed")
		if h.loginScene != nil {
			h.loginScene.ChgPwResult("当前密码修改成功.")
		}

	case protocol.SMChgPasswdFail:
		// Recog 与 ClMain.pas:3765-3770 一致: -1=原密码错误, -2=两次密码不一致。
		log.Logf(log.LevelWarn, "Client", "password change failed: code=%d", msg.Recog)
		if h.loginScene != nil {
			switch msg.Recog {
			case -1:
				h.loginScene.ChgPwResult("密码修改错误.确认原密码在进行修改.")
			case -2:
				h.loginScene.ChgPwResult("密码修改失败.两个密码不一致.")
			default:
				h.loginScene.ChgPwResult("密码修改错误.请稍后重试.")
			}
		}

	case protocol.SMSendUseItems:
		log.Logf(log.LevelInfo, "Client", "received use items (equipment)")
		h.playScene.State.ParseUseItems(body)

	case protocol.SMSendMyMagic:
		log.Logf(log.LevelInfo, "Client", "received magic list: count=%d", msg.Recog)
		h.playScene.State.ParseMagics(body)

	case protocol.SMHear:
		log.Logf(log.LevelInfo, "Client", "chat: %s", body)
		h.playScene.AddChatMessage(body)

	case protocol.SMMerchantSay:
		h.playScene.State.ShopNpcID = msg.Recog // 记录当前对话的NPC ID
		npcName, dialogText := body, ""
		if idx := strings.IndexByte(body, '/'); idx >= 0 {
			npcName, dialogText = body[:idx], body[idx+1:]
		}
		h.playScene.State.NpcDialogName = npcName
		h.playScene.parseNpcDialog(dialogText)
		h.playScene.State.ShowNpcDialog = true

	case protocol.SMDayChanging:
		log.Logf(log.LevelInfo, "Client", "day changing: bright=%d", msg.Recog)
		h.playScene.State.DayBright = int(msg.Recog)

	case protocol.SMMapDescription:
		log.Logf(log.LevelInfo, "Client", "map description: %s", body)
		h.playScene.State.MapTitle = body
		h.playScene.State.MapMusic = int(msg.Recog)
		gSound.PlayMapMusic(int(msg.Recog))

	case protocol.SMSubAbility:
		log.Logf(log.LevelInfo, "Client", "received sub ability")

	case protocol.SMUsername:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.UserName = protocol.DecodeString(body)
			log.Logf(log.LevelDebug, "Client", "actor name: %s", actor.UserName)
		}

	case protocol.SMChangeLight:
		log.Logf(log.LevelInfo, "Client", "light changed: %d", msg.Recog)
		h.playScene.State.LightLevel = int(msg.Recog)

	case protocol.SMHealthSpellChanged:
		// Recog=HP, Param=MaxHP, Tag=MP, Series=MaxMP（服务端已同步修改；
		// 旧的 HP<<16|MP 打包方式会导致 HP 显示 MP 的值）。
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
		log.Logf(log.LevelInfo, "Client", "clearing objects (map switch)")
		h.playScene.State.Actors.Clear()
		h.playScene.State.MySelf = nil

	case protocol.SMChangeMap:
		mapName := body
		newX := int(msg.Param)
		newY := int(msg.Tag)
		log.Logf(log.LevelInfo, "Client", "changing map: %s (%d,%d)", mapName, newX, newY)
		if err := h.playScene.LoadMap(mapName); err != nil {
			log.Logf(log.LevelError, "Client", "failed to load map on switch: %v", err)
			return
		}
		h.playScene.State.MapDarkness = int(msg.Series)
		actor := NewActor(msg.Recog, newX, newY, 0)
		actor.Type = ActorHuman
		h.playScene.State.MySelf = actor
		actor.IsSelf = true
		actor.MapRef = h.playScene.mapData
		h.playScene.State.Actors.Add(actor)
		actor.SendMsg(protocol.SMTurn, newX, newY, 0, 0, 0)

	// =====================================================================
	// 战斗
	// =====================================================================

	case protocol.SMHit, protocol.SMHeavyHit, protocol.SMBigHit, protocol.SMPowerHit, protocol.SMLongHit, protocol.SMWideHit, protocol.SMFireHit, protocol.SMCrsHit, protocol.SMTwinHit:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			// 本地预测已播放自身攻击动画，跳过服务端广播
			if h.playScene.State.MySelf != nil && msg.Recog == h.playScene.State.MySelf.RecogID {
				break
			}
			actor.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMDigUp:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			actor.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMStruck:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil {
			hiterID := int32(0)
			damage := uint16(0)
			if raw := []byte(body); len(raw) >= 4 {
				hiterID = int32(binary.LittleEndian.Uint32(raw[0:4]))
				if len(raw) >= 6 {
					damage = binary.LittleEndian.Uint16(raw[4:6])
				}
			}
			actor.HiterCode = hiterID
			if hiterID != 0 && actor.MagicStruckSound < 1 {
				if hiter := h.playScene.State.Actors.Get(hiterID); hiter != nil && hiter.Type == ActorHuman {
					if actor.Type == ActorHuman && actor.Dress/2 == 3 {
						actor.StruckSound = struckArmorSoundIdx(hiter.Weapon)
					} else {
						actor.StruckSound = struckBodySoundIdx(hiter.Weapon)
					}
					actor.StruckWeaponSound = struckWeaponSoundIdx(hiter.Weapon)
				} else if hiter != nil {
					actor.StruckSound = sStruckBodyFist
				}
			}
			if damage > 0 {
				h.playScene.addFloatingText(actor.CurrX, actor.CurrY, strconv.Itoa(int(damage)), 1.0, 0.3, 0.3)
			}
			actor.SendMsg(protocol.SMStruck, actor.CurrX, actor.CurrY, int(msg.Series)&0xFF, 0, int(hiterID))
		}

	case protocol.SMDeath, protocol.SMNowDeath:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil && actor.Type != ActorNPC {
			actor.Death = true
			actor.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
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
		// 服务器随后会发送完整的 SMAbility；先应用增量，
		// 让经验条立即变化。
		h.playScene.State.Exp += int(msg.Recog)
		log.Logf(log.LevelInfo, "Client", "gained exp: %d", msg.Recog)

	case protocol.SMLevelUp:
		h.playScene.State.Level = int(msg.Recog)
		log.Logf(log.LevelInfo, "Client", "level up: %d", msg.Recog)

	case protocol.SMItemShow:
		log.Logf(log.LevelDebug, "Client", "item show: id=%d x=%d y=%d looks=%d name=%s",
			msg.Recog, msg.Param, msg.Tag, msg.Series, body)
		h.playScene.AddGroundItem(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series), body)

	case protocol.SMItemHide:
		h.playScene.RemoveGroundItem(msg.Recog)

	case protocol.SMOpenDoorOK:
		if h.playScene.mapData != nil {
			h.playScene.mapData.OpenDoor(int(msg.Param), int(msg.Tag))
		}

	case protocol.SMOpenDoorLock:
		h.playScene.AddChatMessage("[系统] 门已上锁")

	case protocol.SMCloseDoor:
		if h.playScene.mapData != nil {
			h.playScene.mapData.CloseDoor(int(msg.Param), int(msg.Tag))
		}

	case protocol.SMDealMenu:
		log.Logf(log.LevelInfo, "Client", "trade: partner=%s", body)
		h.playScene.resetDeal()
		h.playScene.State.InDeal = true
		h.playScene.State.DealPartner = body

	case protocol.SMDealSuccess:
		log.Logf(log.LevelInfo, "Client", "trade complete")
		h.playScene.State.InDeal = false
		h.playScene.State.DealPartner = ""
		h.playScene.resetDeal()

	case protocol.SMDealCancel:
		log.Logf(log.LevelInfo, "Client", "trade cancelled")
		h.playScene.State.InDeal = false
		h.playScene.State.DealPartner = ""
		h.playScene.resetDeal()

	case protocol.SMDealChgGoldOK:
		h.playScene.State.DealGold = int(msg.Recog)
		h.playScene.State.Gold = int(msg.Param)

	case protocol.SMDealRemoteChgGold:
		h.playScene.State.DealRemoteGold = int(msg.Recog)
		gSound.PlaySound(sMoney)

	case protocol.SMBuildGuildOK:
		h.playScene.AddChatMessage("Guild created")

	case protocol.SMGuildMessage:
		log.Logf(log.LevelInfo, "Client", "guild chat: %s", body)
		h.playScene.addGuildChat(body)
		h.playScene.AddChatMessage("[行会] " + body)

	case protocol.SMStorageOK:
		log.Logf(log.LevelInfo, "Client", "item stored")
		h.playScene.sellConfirmed()

	case protocol.SMStorageFull:
		log.Logf(log.LevelInfo, "Client", "storage full")
		h.playScene.sellFailed()

	case protocol.SMTakeBackStorageItemOK:
		log.Logf(log.LevelInfo, "Client", "item retrieved from storage")

	case protocol.SMSysMessage:
		log.Logf(log.LevelInfo, "Client", "system message: %s", body)
		h.playScene.AddChatMessage("[系统] " + body)

	case protocol.SMGoldChanged:
		log.Logf(log.LevelInfo, "Client", "gold: %d", msg.Recog)
		h.playScene.State.Gold = int(msg.Recog)
		gSound.PlaySound(sMoney)

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
			if raw := []byte(body); len(raw) >= 12 {
				actor.MagicSerial = int(binary.LittleEndian.Uint32(raw[8:12]))
				// body[12] = 魔法 Effect 号（Delphi TUseMagicInfo.EffectNumber），驱动施法身特效。
				if len(raw) >= 13 {
					actor.EffectNumber = int(raw[12])
				}
			}
			actor.SendMsg(protocol.SMSpell, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF, 0, 0)
		}

	case protocol.SMMagicFire:
		log.Logf(log.LevelDebug, "Client", "magic fire: magID=%d x=%d y=%d series=%d", msg.Recog, msg.Param, msg.Tag, msg.Series)
		// F4: 确认施法，释放动画暂停
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.SpellConfirmed = true
		}
		fx := float64(msg.Param)*engine.TileWidth + engine.TileWidth/2
		fy := float64(msg.Tag)*engine.TileHeight + engine.TileHeight/2
		effType := int(msg.Series & 0xFF)
		effNum := int(msg.Series >> 8)
		if magID := int(msg.Recog); magID > 0 {
			gSound.PlaySound(10000 + magID*10 + 2)
		}
		var sx, sy float64
		if my := h.playScene.State.MySelf; my != nil {
			sx = float64(my.CurrX)*engine.TileWidth + engine.TileWidth/2
			sy = float64(my.CurrY)*engine.TileHeight + engine.TileHeight/2
		} else {
			sx, sy = fx, fy
		}
		switch MagicEffectType(effType) {
		case EffFly:
			h.playScene.effects.AddFly(sx, sy, fx, fy, effNum, 6, 50)
		case EffGround:
			h.playScene.effects.AddGround(fx, fy, effNum, 10, 50)
		case EffFlyAxe:
			h.playScene.effects.AddFlyAxe(sx, sy, fx, fy, 447, 3, 50)
		case EffFireGun:
			h.playScene.effects.AddFireGun(sx, sy, fx, fy, 930, 6, 50)
		case EffLightning:
			h.playScene.effects.AddLightning(fx, fy, 970, 10, 80)
		case EffIce:
			h.playScene.effects.AddIce(fx, fy, 10, 6, 80)
		case EffBujaukExplo:
			h.playScene.effects.AddBujaukExplo(fx, fy, 10, 80)
		case EffBujaukGround:
			h.playScene.effects.AddBujaukGround(fx, fy, 10, 80)
		case EffFlyArrow:
			h.playScene.effects.AddFlyArrow(sx, sy, fx, fy, 2607, 1, 50)
		case EffReady:
			h.playScene.effects.AddReady(sx, sy, effNum, 10, 50)
		case EffThunder2:
			h.playScene.effects.AddThunder2(fx, fy, 6, 80)
		case EffFlyBug:
			h.playScene.effects.AddFlyBug(sx, sy, fx, fy, 3, 50)
		default:
			h.playScene.effects.AddExplosion(fx, fy, effNum, 10, 50)
		}

	case protocol.SMMagicFireFail:
		// F4: 施法失败，取消动画暂停
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.SpellConfirmed = true // 释放暂停让动画自然结束
			actor.UseMagic = false
		}

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
		log.Logf(log.LevelInfo, "Client", "learned magic: %d", msg.Recog)

	case protocol.SMDelMagic:
		log.Logf(log.LevelInfo, "Client", "forgot magic: %d", msg.Recog)

	case protocol.SMFeatureChanged:
		actor := h.playScene.State.Actors.Get(msg.Recog)
		if actor != nil && body != "" {
			actor.updateFeatureFromBody(body)
		}

	case protocol.SMChangeNameColor:
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.NameColor = int(msg.Param)
		}

	case protocol.SMCry:
		log.Logf(log.LevelInfo, "Client", "cry: %s", body)
		h.playScene.AddChatMessage("[喊话] " + body)

	case protocol.SMWhisper:
		log.Logf(log.LevelInfo, "Client", "whisper: %s", body)
		h.playScene.AddChatMessage("[私聊] " + body)

	case protocol.SMGroupMessage:
		log.Logf(log.LevelInfo, "Client", "group: %s", body)
		h.playScene.AddChatMessage("[组队] " + body)

	case protocol.SMAddItem:
		log.Logf(log.LevelInfo, "Client", "item added")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMDelItem:
		log.Logf(log.LevelInfo, "Client", "item removed: idx=%d", msg.Recog)
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMUpdateItem:
		log.Logf(log.LevelInfo, "Client", "item updated")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMDelItems:
		log.Logf(log.LevelInfo, "Client", "items cleared")

	case protocol.SMDropItemSuccess:
		log.Logf(log.LevelInfo, "Client", "item dropped")

	case protocol.SMDropItemFail:
		log.Logf(log.LevelInfo, "Client", "failed to drop item")

	case protocol.SMWeightChanged:
		h.playScene.State.Weight = int(msg.Recog)
		h.playScene.State.MaxWeight = int(msg.Param)
		// F3: 更新超重标志
		if h.playScene.State.MySelf != nil {
			h.playScene.State.MySelf.Overweight = int(msg.Recog) > int(msg.Param) && int(msg.Param) > 0
		}

	case protocol.SMAdjustBonus:
		h.playScene.State.BonusPoint = int(msg.Recog)

	case protocol.SMSendUserState:
		// 消息体: 130 字节装备数据后跟玩家名字。
		raw := []byte(body)
		name := ""
		if len(raw) > 130 {
			name = string(raw[130:])
		}
		h.playScene.parseInspect(name, body)

	case protocol.SMDuraChange:
		log.Logf(log.LevelDebug, "Client", "durability changed: makeIndex=%d dura=%d/%d", msg.Recog, msg.Param, msg.Tag)
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
		log.Logf(log.LevelInfo, "Client", "item used")
		queryBag := protocol.MakeDefaultMsg(protocol.CMQueryBagItems, 0, 0, 0, 0)
		h.Send(queryBag, "")

	case protocol.SMEatFail:
		log.Logf(log.LevelInfo, "Client", "failed to use item")

	case protocol.SMTakeOnOK:
		// 背包/装备状态通过服务器随此消息发送的完整 SMBagItems/SMSendUseItems
		// 刷新到达（客户端在请求时已做了乐观视觉更新）。
		log.Logf(log.LevelInfo, "Client", "equip succeeded: slot=%d", msg.Recog)

	case protocol.SMTakeOnFail:
		// 随此消息发送的完整刷新会回滚客户端的乐观更新。
		log.Logf(log.LevelWarn, "Client", "equip failed: code=%d", msg.Recog)

	case protocol.SMTakeOffOK:
		log.Logf(log.LevelInfo, "Client", "unequip succeeded: slot=%d", msg.Recog)

	case protocol.SMTakeOffFail:
		log.Logf(log.LevelWarn, "Client", "unequip failed")

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
				off := 2 + i*6
				if off+6 > len(raw) {
					break
				}
				itemIdx := binary.LittleEndian.Uint16(raw[off : off+2])
				price := int(binary.LittleEndian.Uint16(raw[off+2 : off+4]))
				stock := int(binary.LittleEndian.Uint16(raw[off+4 : off+6]))
				h.playScene.State.ShopGoods = append(h.playScene.State.ShopGoods, ShopItem{ItemIdx: itemIdx, Price: price, Stock: stock})
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
		log.Logf(log.LevelInfo, "Client", "purchase succeeded")

	case protocol.SMBuyItemFail:
		log.Logf(log.LevelInfo, "Client", "purchase failed: code=%d", msg.Recog)

	case protocol.SMUserSellItemOK:
		log.Logf(log.LevelInfo, "Client", "sell succeeded")
		h.playScene.sellConfirmed()

	case protocol.SMUserSellItemFail:
		log.Logf(log.LevelInfo, "Client", "sell failed")
		h.playScene.sellFailed()

	case protocol.SMSendBuyPrice:
		h.playScene.sellPriceStr = strconv.Itoa(int(msg.Recog))
		log.Logf(log.LevelInfo, "Client", "buy price: %d", msg.Recog)

	case protocol.SMUserRepairItemOK:
		log.Logf(log.LevelInfo, "Client", "repair succeeded")

	case protocol.SMUserRepairItemFail:
		log.Logf(log.LevelInfo, "Client", "repair failed")

	case protocol.SMSendRepairCost:
		h.playScene.sellPriceStr = strconv.Itoa(int(msg.Recog))
		log.Logf(log.LevelInfo, "Client", "repair cost: %d", msg.Recog)

	case protocol.SMSendUserMakeDrugItemList: // 712
		// 制药列表：复用商店面板，ShopMode=4
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 4
		h.playScene.State.ShopNpcID = msg.Recog
		h.playScene.menuTop = 0
		h.playScene.menuIndex = -1
		raw := []byte(body)
		if len(raw) >= 2 {
			count := int(binary.LittleEndian.Uint16(raw[0:2]))
			h.playScene.State.ShopGoods = make([]ShopItem, 0, count)
			for i := 0; i < count; i++ {
				off := 2 + i*6
				if off+6 > len(raw) {
					break
				}
				itemIdx := binary.LittleEndian.Uint16(raw[off : off+2])
				price := int(binary.LittleEndian.Uint16(raw[off+2 : off+4]))
				stock := int(binary.LittleEndian.Uint16(raw[off+4 : off+6]))
				h.playScene.State.ShopGoods = append(h.playScene.State.ShopGoods, ShopItem{ItemIdx: itemIdx, Price: price, Stock: stock})
			}
		}

	case protocol.SMMakeDrugSuccess: // 713
		log.Logf(log.LevelInfo, "Client", "drug crafting succeeded")
		h.playScene.State.ShowShop = false

	case protocol.SMMakeDrugFail: // 714
		log.Logf(log.LevelInfo, "Client", "drug crafting failed")
		h.playScene.State.ShowShop = false

	case protocol.SMSendUserStorageItem:
		// 仓库复用商店面板（Delphi BoStorageMenu）：列表是
		// 存储模式的 DMenuDlg，存取通过 DSellDlg 位置。
		h.playScene.State.ShowShop = true
		h.playScene.State.ShopMode = 3
		h.playScene.menuTop = 0
		h.playScene.menuIndex = -1
		h.playScene.sellItem = nil
		h.playScene.sellPriceStr = ""

	case protocol.SMSaveItemList:
		// 仓库列表行：名字来自物品数据库，"price" 携带用于取回物品的
		// MakeIndex（Delphi ClientGetSaveItemList, ClMain.pas:5651-5690）。
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
		log.Logf(log.LevelInfo, "Client", "storage deposit failed")

	case protocol.SMTakeBackStorageItemFail:
		log.Logf(log.LevelInfo, "Client", "storage retrieval failed")

	case protocol.SMGroupModeChanged:
		h.playScene.State.AllowGroup = msg.Recog != 0

	case protocol.SMCreateGroupOK:
		h.playScene.AddChatMessage("[Group] created")

	case protocol.SMCreateGroupFail:
		h.playScene.AddChatMessage("[Group] create failed (target unavailable?)")

	case protocol.SMGroupAddMemOK:
		h.playScene.AddChatMessage("[Group] added " + body)

	case protocol.SMGroupAddMemFail:
		h.playScene.AddChatMessage("[Group] add failed")

	case protocol.SMGroupDelMemOK:
		h.playScene.AddChatMessage("[Group] removed " + body)

	case protocol.SMGroupDelMemFail:
		h.playScene.AddChatMessage("[Group] remove failed")

	case protocol.SMGroupCancel:
		h.playScene.State.GroupMembers = nil
		h.playScene.AddChatMessage("[Group] disbanded")

	case protocol.SMGroupMembers:
		if strings.TrimSpace(body) == "" {
			h.playScene.State.GroupMembers = nil
		} else {
			h.playScene.State.GroupMembers = strings.Split(body, "\n")
		}

	case protocol.SMOpenGuildDlg:
		// 消息体: name\nrank\nperm\nnotice（服务端 HandleOpenGuildDlg）。
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
		h.playScene.AddChatMessage("You are not in a guild")

	case protocol.SMChangeGuildName:
		h.playScene.State.GuildName = body

	case protocol.SMSendGuildMemberList:
		if strings.TrimSpace(body) == "" {
			h.playScene.State.GuildMembers = nil
		} else {
			h.playScene.State.GuildMembers = strings.Split(body, "\n")
		}

	case protocol.SMGuildAddMemberOK:
		h.playScene.AddChatMessage("[Guild] added " + body)

	case protocol.SMGuildAddMemberFail:
		h.playScene.AddChatMessage("[Guild] add failed")

	case protocol.SMGuildDelMemberOK:
		h.playScene.AddChatMessage("[Guild] removed " + body)

	case protocol.SMGuildDelMemberFail:
		h.playScene.AddChatMessage("[Guild] remove failed")

	case protocol.SMBuildGuildFail:
		h.playScene.AddChatMessage("Guild creation failed")

	case protocol.SMGuildMakeAllyOK:
		h.playScene.AddChatMessage("[Guild] alliance formed")

	case protocol.SMGuildMakeAllyFail:
		h.playScene.AddChatMessage("[Guild] alliance failed")

	case protocol.SMGuildBreakAllyOK:
		h.playScene.AddChatMessage("[Guild] alliance broken")

	case protocol.SMGuildBreakAllyFail:
		h.playScene.AddChatMessage("[Guild] break ally failed")

	case protocol.SMDlgMsg:
		h.playScene.AddChatMessage(body)

	case protocol.SMMenuOK:
		// MESSAGEBOX 模态弹窗 (Delphi SM_MENU_OK 767, ClMain.pas:4917-4921)
		ShowConfirm(h.playScene, body, []ModalResult{MrOk}, DlgNormal, nil)

	case protocol.SMDealTryFail:
		log.Logf(log.LevelInfo, "Client", "trade request failed")

	case protocol.SMDealAddItemOK:
		// Recog = 格子槽位；body 携带提供的物品。
		if item := h.parseTradeItem(body); item != nil {
			slot := int(msg.Recog)
			if slot >= 0 && slot < len(h.playScene.State.DealItems) {
				h.playScene.State.DealItems[slot] = item
			}
		}

	case protocol.SMDealAddItemFail:
		log.Logf(log.LevelDebug, "Client", "trade add item failed")

	case protocol.SMDealDelItemOK:
		// Recog = 取回的 MakeIndex；背包重新同步会归还。
		st := h.playScene.State
		for i, it := range st.DealItems {
			if it != nil && it.MakeIndex == msg.Recog {
				st.DealItems[i] = nil
				break
			}
		}

	case protocol.SMDealDelItemFail:
		log.Logf(log.LevelDebug, "Client", "trade remove item failed")

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
		// D4: 传送隐藏动画（垂直收缩）
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.ScrollHideState = 1
			actor.ScrollHideFrame = 0
			actor.ScrollHideTick = time.Now().UnixMilli()
		}
		gSound.PlaySound(sSpacemoveOut)

	case protocol.SMSpaceMoveShow:
		// D4: 传送显示动画（从地面展开）
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.ScrollHideState = 3
			actor.ScrollHideFrame = 10
			actor.ScrollHideTick = time.Now().UnixMilli()
		}
		gSound.PlaySound(sSpacemoveIn)

	case protocol.SMOpenHealth:
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.ShowHP = true
			actor.ShowHPVal = int(msg.Param)
			actor.ShowMaxHPVal = int(msg.Tag)
		}

	case protocol.SMCloseHealth:
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.ShowHP = false
		}

	case protocol.SMBreakWeapon:
		h.playScene.AddChatMessage("你的武器已损坏！")
		if h.playScene.State.MySelf != nil {
			h.playScene.addFloatingText(h.playScene.State.MySelf.CurrX, h.playScene.State.MySelf.CurrY, "武器损坏", 1.0, 0.2, 0.2)
		}

	case protocol.SMButch:
		h.playScene.AddChatMessage("屠宰成功")

	case protocol.SMReadMinimapOK:
		// Recog 携带 Mmap.wil 中本地图的小地图图像索引。
		if h.playScene != nil {
			h.playScene.State.MinimapIndex = int(msg.Recog)
		}
		log.Logf(log.LevelDebug, "Client", "received minimap data: index=%d", msg.Recog)

	case protocol.SMMonsterSay:
		if actor := h.playScene.State.Actors.Get(msg.Recog); actor != nil {
			actor.Say(protocol.DecodeString(body))
		}

	default:
		log.Logf(log.LevelDebug, "Client", "unhandled: %d", msg.Ident)
	}
}

// ============================================================================
// 消息解析辅助函数
// ============================================================================

// parseFirstServer 从服务器列表 body 中提取第一个服务器名称。
// 消息体格式: "name1/status1/name2/status2/..."
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

// parseAddrPortCert 解析 "addr/port/certification"。
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

// parseAddrPort 解析 "addr/port"。
func parseAddrPort(body string) (addr string, err error) {
	parts := strings.Split(body, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("expected 2 parts, got %d: %q", len(parts), body)
	}
	return parts[0] + ":" + parts[1], nil
}

// parsedChar 保存从 SM_QUERYCHR 解析出的角色。
type parsedChar struct {
	Name  string
	Job   int
	Hair  int
	Level int
	Sex   int
}

// parseQueryChrBody 解析角色列表 body。
// 消息体格式: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
// 名字前的 '*' 前缀表示上次选择的角色。
func parseQueryChrBody(body string) (chars []parsedChar, selectedIdx int) {
	selectedIdx = -1
	if body == "" {
		return
	}

	parts := strings.Split(body, "/")
	// 每个角色有 5 个字段: name, job, hair, level, sex
	for i := 0; i+4 < len(parts); i += 5 {
		name := parts[i]
		if name == "" {
			continue
		}

		// 检查选中标记
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

// connectToServer 创建新的 NetHandler 并连接登录服务器。
func connectToServer(addr string, loginScene *LoginScene, playScene *PlayScene, selectChrScene *SelectChrScene, noticeScene *NoticeScene, sceneMgr *engine.SceneManager) (*NetHandler, error) {
	log.Logf(log.LevelInfo, "Client", "connecting to %s...", addr)

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	log.Logf(log.LevelInfo, "Client", "connected to server")

	handler := &NetHandler{
		conn:           conn,
		loginScene:     loginScene,
		playScene:      playScene,
		selectChrScene: selectChrScene,
		noticeScene:    noticeScene,
		sceneMgr:       sceneMgr,
		done:           make(chan struct{}),
		errCh:          make(chan error, 1),
	}

	// 发送协议版本
	protoMsg := protocol.MakeDefaultMsg(protocol.CMProtocol, clientVersion, 0, 0, 0)
	handler.Send(protoMsg, "")

	playScene.SetSendMove(func(ident int, dir int) {
		moveMsg := protocol.MakeDefaultMsg(uint16(ident), 0, uint16(dir), 0, 0)
		handler.Send(moveMsg, "")
	})

	playScene.SetSendOpenDoor(func(doorID, x, y int) {
		doorMsg := protocol.MakeDefaultMsg(protocol.CMOpenDoor, int32(doorID), uint16(x), uint16(y), 0)
		handler.Send(doorMsg, "")
	})

	playScene.SetSendAttack(func(ident int, dir int) {
		hitMsg := protocol.MakeDefaultMsg(uint16(ident), 0, uint16(dir), 0, 0)
		handler.Send(hitMsg, "")
		// 本地预测：立即播放攻击动画（Delphi ReadyAction, Actor.pas:1479-1528）
		if my := playScene.State.MySelf; my != nil {
			my.SendMsg(ident, my.CurrX, my.CurrY, dir, 0, 0)
		}
	})

	playScene.SetSendPickup(func() {
		pickupMsg := protocol.MakeDefaultMsg(protocol.CMPickup, 0, 0, 0, 0)
		handler.Send(pickupMsg, "")
	})

	playScene.SetSendButch(func(targetID int32) {
		butchMsg := protocol.MakeDefaultMsg(protocol.CMButch, targetID, 0, 0, 0)
		handler.Send(butchMsg, "")
	})

	playScene.SetSendChat(func(text string) {
		sayMsg := protocol.MakeDefaultMsg(protocol.CMSay, 0, 0, 0, 0)
		handler.Send(sayMsg, text)
	})

	playScene.SetSendSpell(func(magID int, x, y int) {
		spellMsg := protocol.MakeDefaultMsg(protocol.CMSpell, 0, uint16(magID), uint16(x), uint16(y))
		handler.Send(spellMsg, "")
		if magID == 26 {
			pwr := 100
			for i := range playScene.State.Magics {
				if int(playScene.State.Magics[i].MagID) == 26 {
					pwr = 100 + int(playScene.State.Magics[i].Level)*50
					break
				}
			}
			handler.SendRawString(fmt.Sprintf("+PWR/%d", pwr))
		}
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
		// Recog 携带 32 位 MakeIndex（Delphi SendEatItem 惯例）。
		msg := protocol.MakeDefaultMsg(protocol.CMEat, makeIndex, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendAttackMode(func(mode int) {
		msg := protocol.MakeDefaultMsg(protocol.CMChangeAttackMode, 0, uint16(mode), 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendLogout(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMLogout, 0, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendExit(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMExitGame, 0, 0, 0, 0)
		handler.Send(msg, "")
	})

	playScene.SetSendAddFriend(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMAddFriend, 0, 0, 0, 0)
		handler.Send(msg, name)
	})

	playScene.SetSendDelFriend(func(name string) {
		msg := protocol.MakeDefaultMsg(protocol.CMDelFriend, 0, 0, 0, 0)
		handler.Send(msg, name)
	})

	playScene.SetSendQueryFriends(func() {
		msg := protocol.MakeDefaultMsg(protocol.CMQueryFriends, 0, 0, 0, 0)
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
		// Send 只编码 body 一次；传入原始字节。
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
	log.Logf(log.LevelInfo, "Scene", "opened: %s", s.name)
}
func (s *DebugScene) Close() {
	log.Logf(log.LevelInfo, "Scene", "closed: %s", s.name)
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
	glState.DrawQuadColor(0, 0, float32(winW), float32(winH), r, g, b, 1.0, proj)
	glState.DrawQuadColor(350, 250, 100, 100, 1.0, 1.0, 1.0, 1.0, proj)
}
func (s *DebugScene) OnKey(key int, action int)                              {}
func (s *DebugScene) OnMouse(x, y float64, button int, action int, mods int) {}
func (s *DebugScene) OnScroll(x, y float64)                                  {}

// parseTradeItem 解码 SMDealAddItemOK/SMDealRemoteAddItem 的 payload
// （服务端 encodeDealItem: MakeIndex i32, WIndex u16, Dura u16, DuraMax u16）。
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

func (h *NetHandler) parseFriendList(body string) {
	text := protocol.DecodeString(body)
	h.playScene.State.Friends = h.playScene.State.Friends[:0]
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, "/", 2)
		if len(parts) == 2 {
			h.playScene.State.Friends = append(h.playScene.State.Friends, FriendInfo{
				Name:   parts[0],
				Online: parts[1] == "1",
			})
		}
	}
}
