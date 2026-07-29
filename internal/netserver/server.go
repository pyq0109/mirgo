// Package netserver 为 MIR2 游戏服务端提供 TCP 服务端基础设施。
package netserver

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// SessionState 表示客户端会话的连接状态。
type SessionState int32

const (
	StateConnected         SessionState = iota
	StateAuthenticated
	StateCharacterSelected
	StateInGame
)

// maxRecvBuf 限制单连接累计接收数据量；超过该值的对端会被断开。
const maxRecvBuf = 64 * 1024

// Session 表示一个已连接的客户端。
type Session struct {
	ID            int64
	Conn          net.Conn
	State         SessionState
	AccountName   string
	CharacterID   int64
	Certification int32
	SendChan      chan []byte
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
	listener    net.Listener
	sessions    map[int64]*Session
	mu          sync.RWMutex
	nextID      atomic.Int64
	addr        string

	onConnect    ConnectHandler
	onDisconnect DisconnectHandler
	onMessage    MessageHandler
	onRawMessage RawMessageHandler

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
			ID:       sessionID,
			Conn:     conn,
			State:    StateConnected,
			SendChan: make(chan []byte, 100),
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
	var recvBuf []byte // 跨多次 Read 累积字节，避免被 TCP 拆分的帧丢失
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

		// 解析消息帧：#<code><payload>!
		// 一次 Read() 可能到达多个帧，一个帧也可能跨多次调用。
		if n > 0 {
			recvBuf = append(recvBuf, buf[:n]...)
			if len(recvBuf) > maxRecvBuf {
				log.Logf(log.LevelWarn, "Server", "来自 %d 的接收缓冲区溢出，断开连接", session.ID)
				return
			}
			data := recvBuf
			// 处理所有完整帧，把末尾不完整的帧留到下次 Read。
			for len(data) > 2 {
				// 找到第一个帧的结尾
				if data[0] != '#' {
					data = data[1:] // 容忍噪声/错位，在下一个 '#' 处重新同步
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
					break // 帧不完整；等待更多数据
				}

				frame := data[1:endIdx] // # 和 ! 之间的内容
				data = data[endIdx+1:]   // 跳过 !

				// 如果存在 code 数字则跳过
				payloadStart := 0
				if len(frame) > 0 && frame[0] >= '0' && frame[0] <= '9' {
					payloadStart = 1
				}
				payload := string(frame[payloadStart:])

				if len(payload) == 0 {
					continue
				}

				// 检查是否为原始消息（如 **login、+PWR/100）
				handled := false
				if s.onRawMessage != nil {
					decoded := protocol.DecodeString(payload)
					if (len(decoded) >= 2 && decoded[0] == '*' && decoded[1] == '*') ||
						(len(decoded) >= 1 && decoded[0] == '+') {
						log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] RAW %q", session.ID, decoded)
						handled = s.onRawMessage(session, decoded)
					}
				}

				if !handled && s.onMessage != nil && len(payload) >= protocol.DefBlockSize {
					msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
					body, rawBody := "", ""
					if len(payload) > protocol.DefBlockSize {
						rawBody = payload[protocol.DefBlockSize:]
						body = protocol.DecodeString(rawBody)
					}
					log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
						session.ID, protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)
					s.onMessage(session, msg, body, rawBody)
				}
			}
			// 压缩：保留末尾不完整的帧，丢弃已消费的前缀。
			recvBuf = recvBuf[:copy(recvBuf, data)]
		}
	}
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
			_, err := session.Conn.Write(data)
			if err != nil {
				log.Logf(log.LevelDebug, "Server", "写入 %d 出错：%v", session.ID, err)
				return
			}
		}
	}
}

func (s *TCPServer) removeSession(session *Session) {
	s.mu.Lock()
	delete(s.sessions, session.ID)
	s.mu.Unlock()

	session.Conn.Close()
	close(session.SendChan)

	log.Logf(log.LevelInfo, "Server", "客户端已断开：%d", session.ID)

	if s.onDisconnect != nil {
		s.onDisconnect(session)
	}
}

// Send 向指定会话发送一条消息。
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
		return nil
	default:
		return fmt.Errorf("send buffer full for session %d", sessionID)
	}
}

// GetSession 按 ID 返回一个会话。
func (s *TCPServer) GetSession(id int64) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

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
	default:
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
