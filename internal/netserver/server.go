// Package netserver 为 MIR2 游戏服务端提供 TCP 服务端基础设施。
package netserver

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// Delphi RunGate 背压参数（GateShare.pas:9-10,98）：
// 累计发送 >=512B 未回显时在下一发送块前插入 '*' 探针；
// >=2048B 未回显则暂停发送等待回显，超时 dwClientCheckTimeOut=50ms
//（{3000} 为 GateShare.pas:98 注释中的旧值）后恢复。
const (
	sendCheckSize        = 512
	sendCheckSizeMax     = 2048
	clientCheckTimeoutMs = 50
)

// SessionState 表示客户端会话的连接状态。
type SessionState int32

const (
	StateConnected SessionState = iota
	StateAuthenticated
	StateCharacterSelected
	StateInGame
)

// Session 表示一个已连接的客户端。
type Session struct {
	ID            int64
	Conn          net.Conn
	State         SessionState
	AccountName   string
	CharacterID   int64
	Certification int32
	SendChan      chan []byte
	closeOnce     sync.Once // 保护 Conn.Close + close(SendChan) 只执行一次

	// 封包序号校验（Delphi RunGate/Main.pas:363-413）
	lastCode    byte
	hasLastCode bool
	SeqErrCount int

	// 背压：已发送但未被客户端 '*' 回显确认的字节数
	//（= Delphi nCheckSendLength，RunGate/Main.pas:501-553）
	unackedBytes int64
	// boSendCheck：本轮已累计 >=512B 且尚未在发送块前插入 '*' 探针
	//（RunGate/Main.pas:529-533）
	probeSent atomic.Bool
	// dwTimeOutTime：背压暂停到期时间戳（毫秒）；0 表示未暂停
	//（RunGate/Main.pas:534-537）
	pauseUntil atomic.Int64
	// 背压 '*' 回显恢复信号（缓冲 1，writeLoop 暂停等待用）
	resumeSig chan struct{}

	// 每连接消息限流令牌桶（路线图 6.3 网关补偿层）
	msgTokens  float64
	lastRefill time.Time

	// 连续发送丢弃计数（超阈值视为客户端无响应，断开）
	dropCount int
}

// MessageHandler 处理来自客户端的消息。body 是 6Bit 解码后的 body
// 字符串；rawBody 是消息头之后仍处于编码状态的负载，用于那些 body 由
// 多段独立 EncodeBuffer 组成、必须先切分再解码的消息（ClMain.pas:2844）。
type MessageHandler func(session *Session, msg protocol.DefaultMessage, body, rawBody string)

// RawMessageHandler 在标准解析之前处理原始字符串消息（如 **login）。
// 若消息已处理则返回 true，返回 false 则继续走标准解析。
type RawMessageHandler func(session *Session, raw string) bool

// ConnectHandler 处理新的客户端连接。
type ConnectHandler func(session *Session)

// DisconnectHandler 处理客户端断开。
type DisconnectHandler func(session *Session)

// TCPServer 管理 TCP 连接与消息路由。
type TCPServer struct {
	listener net.Listener
	sessions map[int64]*Session
	mu       sync.RWMutex
	nextID   atomic.Int64
	addr     string

	onConnect    ConnectHandler
	onDisconnect DisconnectHandler
	onMessage    MessageHandler
	onRawMessage RawMessageHandler

	// 每连接入站消息限流（令牌桶；msgRate<=0 表示不限流）
	msgRate  float64
	msgBurst int

	done chan struct{}
	wg   sync.WaitGroup
}

// NewTCPServer 创建一个新的 TCP 服务端。
func NewTCPServer(addr string) *TCPServer {
	return &TCPServer{
		sessions: make(map[int64]*Session),
		addr:     addr,
		done:     make(chan struct{}),
	}
}

// SetMsgRateLimit 设置每连接入站消息速率（条/秒）与突发容量。
// rate<=0 关闭限流（默认）。
func (s *TCPServer) SetMsgRateLimit(ratePerSec float64, burst int) {
	s.msgRate = ratePerSec
	s.msgBurst = burst
}

// SetConnectHandler 设置连接处理器。
func (s *TCPServer) SetConnectHandler(h ConnectHandler) {
	s.onConnect = h
}

// SetDisconnectHandler 设置断开处理器。
func (s *TCPServer) SetDisconnectHandler(h DisconnectHandler) {
	s.onDisconnect = h
}

// SetMessageHandler 设置消息处理器。
func (s *TCPServer) SetMessageHandler(h MessageHandler) {
	s.onMessage = h
}

// SetRawMessageHandler 设置原始消息处理器。
func (s *TCPServer) SetRawMessageHandler(h RawMessageHandler) {
	s.onRawMessage = h
}

// Start 开始监听连接。
func (s *TCPServer) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.listener = ln
	log.Logf(log.LevelInfo, "Server", "监听 %s", s.addr)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop 停止服务端并关闭所有连接。
func (s *TCPServer) Stop() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}

	s.mu.Lock()
	for _, session := range s.sessions {
		session.Conn.Close()
	}
	s.sessions = make(map[int64]*Session)
	s.mu.Unlock()

	s.wg.Wait()
	log.Logf(log.LevelInfo, "Server", "服务端已停止")
}

func (s *TCPServer) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				log.Logf(log.LevelError, "Server", "Accept 出错：%v", err)
				continue
			}
		}

		sessionID := s.nextID.Add(1)
		session := &Session{
			ID:        sessionID,
			Conn:      conn,
			State:     StateConnected,
			SendChan:  make(chan []byte, 256),
			resumeSig: make(chan struct{}, 1),
		}

		s.mu.Lock()
		s.sessions[sessionID] = session
		s.mu.Unlock()

		log.Logf(log.LevelInfo, "Server", "客户端已连接：%s（ID：%d）", conn.RemoteAddr(), sessionID)

		if s.onConnect != nil {
			s.onConnect(session)
		}

		s.wg.Add(2)
		go s.readLoop(session)
		go s.writeLoop(session)
	}
}

func (s *TCPServer) readLoop(session *Session) {
	defer s.wg.Done()
	defer s.removeSession(session)

	buf := make([]byte, 4096)
	var scanner protocol.FrameScanner
	// Delphi RunGate 序号校验（Main.pas:363-413）：重复序号累计违规
	scanner.OnCode = func(code byte) {
		if session.hasLastCode && code == session.lastCode {
			session.SeqErrCount++
		}
		session.lastCode = code
		session.hasLastCode = true
	}
	// 客户端 '*' 回显 = 接收确认，立即恢复背压并清零（RunGate/Main.pas:1010-1016）
	keepalive := func() {
		session.resumeSend()
	}
	for {
		n, err := session.Conn.Read(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				log.Logf(log.LevelDebug, "Server", "来自 %d 的读错误：%v", session.ID, err)
				return
			}
		}

		if n > 0 {
			payloads, overflow := scanner.Feed(buf[:n], true, keepalive)
			if overflow {
				log.Logf(log.LevelWarn, "Server", "来自 %d 的接收缓冲区溢出，断开连接", session.ID)
				return
			}
			// Delphi: 重复序号 >10 次拒绝服务；缓冲积压 >20000 直接记满
			//（RunGate/Main.pas:363-413）
			if scanner.Pending() > 20000 {
				session.SeqErrCount = 99
			}
			if session.SeqErrCount > 10 {
				log.Logf(log.LevelWarn, "Server", "来自 %d 的封包序号重复 %d 次，断开连接", session.ID, session.SeqErrCount)
				return
			}
			for _, payload := range payloads {
				s.dispatchPayload(session, payload)
			}
		}
	}
}

// dispatchPayload 处理一帧 payload。
// 先尝试标准消息解析（通过 IsClientIdent 验证 Ident），
// 仅在 Ident 不被识别时才尝试 raw 消息检测，
// 避免 Recog 字段恰好解码为 '+'/'*' 开头时的误路由。
func (s *TCPServer) dispatchPayload(session *Session, payload string) {
	// 每连接入站限流（令牌桶；超限丢弃并计违规，路线图 6.3）
	if !s.allowMessage(session) {
		session.SeqErrCount++
		if session.SeqErrCount > 10 {
			log.Logf(log.LevelWarn, "Server", "session %d 消息速率超限且违规累计 >10，断开连接", session.ID)
			s.removeSession(session)
		}
		return
	}
	// 先尝试标准消息
	if len(payload) >= protocol.DefBlockSize {
		msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
		if protocol.IsClientIdent(msg.Ident) {
			body, rawBody := "", ""
			if len(payload) > protocol.DefBlockSize {
				rawBody = payload[protocol.DefBlockSize:]
				body = protocol.DecodeString(rawBody)
			}
			log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
				session.ID, protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)
			if s.onMessage != nil {
				s.onMessage(session, msg, body, rawBody)
			}
			return
		}
	}

	// 非标准消息 → 尝试 raw 检测（**runlogin、+PWR/n 等）
	if s.onRawMessage != nil {
		decoded := protocol.DecodeString(payload)
		if (len(decoded) >= 2 && decoded[0] == '*' && decoded[1] == '*') ||
			(len(decoded) >= 1 && decoded[0] == '+') {
			log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] RAW %q", session.ID, decoded)
			s.onRawMessage(session, decoded)
		}
	}
}

// allowMessage 每连接令牌桶限流。msgRate<=0 时不限流。
func (s *TCPServer) allowMessage(session *Session) bool {
	if s.msgRate <= 0 {
		return true
	}
	now := time.Now()
	if session.lastRefill.IsZero() {
		session.lastRefill = now
		session.msgTokens = float64(s.msgBurst)
	}
	session.msgTokens += now.Sub(session.lastRefill).Seconds() * s.msgRate
	session.lastRefill = now
	if session.msgTokens > float64(s.msgBurst) {
		session.msgTokens = float64(s.msgBurst)
	}
	if session.msgTokens < 1 {
		return false
	}
	session.msgTokens--
	return true
}

func (s *TCPServer) writeLoop(session *Session) {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			return
		case data, ok := <-session.SendChan:
			if !ok {
				return
			}
			// Delphi 背压：>=2048B 未回显则暂停发送，等待客户端 '*' 回显，
			// 超时 dwClientCheckTimeOut=50ms 后恢复（Main.pas:520-527,534-537）。
			if pu := session.pauseUntil.Load(); pu > 0 {
				if wait := time.Until(time.UnixMilli(pu)); wait > 0 {
					select {
					case <-s.done:
						return
					case <-session.resumeSig:
					case <-time.After(wait):
					}
				}
				session.resumeSend()
			}
			// >=512B 未回显时在发送块前插入 '*' 探针，客户端收到后立即回显
			//（Main.pas:529-533；回显由 readLoop keepalive → resumeSend 处理）。
			if atomic.LoadInt64(&session.unackedBytes) >= sendCheckSize && !session.probeSent.Load() {
				session.probeSent.Store(true)
				data = append([]byte{'*'}, data...)
			}
			_, err := session.Conn.Write(data)
			if err != nil {
				log.Logf(log.LevelDebug, "Server", "写入 %d 出错：%v", session.ID, err)
				return
			}
			if n := atomic.AddInt64(&session.unackedBytes, int64(len(data))); n >= sendCheckSizeMax {
				session.pauseUntil.Store(time.Now().UnixMilli() + clientCheckTimeoutMs)
			}
		}
	}
}

// resumeSend 清零背压计数并唤醒暂停中的 writeLoop。
// Delphi 收到 '*' 回显时的恢复路径（Main.pas:1010-1016）；
// 超时恢复（Main.pas:520-527）同样走这里——原版超时不清 boSendCheck，
// 此处一并清零，保证下一轮累计到 512B 时能重新发探针。
func (session *Session) resumeSend() {
	atomic.StoreInt64(&session.unackedBytes, 0)
	session.probeSent.Store(false)
	session.pauseUntil.Store(0)
	select {
	case session.resumeSig <- struct{}{}:
	default:
	}
}

func (s *TCPServer) removeSession(session *Session) {
	s.mu.Lock()
	_, exists := s.sessions[session.ID]
	if exists {
		delete(s.sessions, session.ID)
	}
	s.mu.Unlock()

	if !exists {
		return // 已被另一路径移除
	}

	session.closeOnce.Do(func() {
		session.Conn.Close()
		close(session.SendChan)
	})

	log.Logf(log.LevelInfo, "Server", "客户端已断开：%d", session.ID)

	if s.onDisconnect != nil {
		s.onDisconnect(session)
	}
}

// Send 向指定会话发送一条消息。body 必须已由调用方编码。
func (s *TCPServer) Send(sessionID int64, msg protocol.DefaultMessage, body string) error {
	log.Logf(log.LevelInfo, "Server", ">>> SEND [%d] %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
		sessionID, protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)

	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %d not found", sessionID)
	}

	encoded := protocol.EncodeMessage(msg)
	if body != "" {
		encoded += body
	}

	frame := protocol.FormatServerFrame(encoded)

	select {
	case session.SendChan <- []byte(frame):
		session.dropCount = 0
		return nil
	default:
		session.dropCount++
		log.Logf(log.LevelWarn, "Server", "发送缓冲区满，丢弃 %s（session %d，连续 %d 次）", protocol.MsgName(msg.Ident), sessionID, session.dropCount)
		if session.dropCount > 32 {
			log.Logf(log.LevelWarn, "Server", "session %d 发送持续积压，视为无响应断开", sessionID)
			s.removeSession(session)
		}
		return fmt.Errorf("send buffer full for session %d", sessionID)
	}
}

// GetSession 按 ID 返回一个会话。
func (s *TCPServer) GetSession(id int64) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

// SendRaw 发送原始字节（如 #+GOOD!、#+FAIL!）。
// 使用带超时的阻塞发送，因为 ACK 消息不可丢弃。
func (s *TCPServer) SendRaw(sessionID int64, raw string) error {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %d not found", sessionID)
	}
	select {
	case session.SendChan <- []byte(raw):
		return nil
	case <-time.After(time.Second):
		log.Logf(log.LevelWarn, "Server", "发送缓冲区满（1s 超时），丢弃 RAW %q（session %d）", raw, sessionID)
		return fmt.Errorf("send buffer full for session %d", sessionID)
	}
}

// CloseSession 强制断开一个会话（如踢出加速外挂）。
func (s *TCPServer) CloseSession(sessionID int64) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if ok {
		s.removeSession(session)
	}
}

// GetSessionCount 返回已连接的会话数。
func (s *TCPServer) GetSessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
