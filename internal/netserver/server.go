// Package netserver provides TCP server infrastructure for the MIR2 game server.
package netserver

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// SessionState represents the connection state of a client session.
type SessionState int32

const (
	StateConnected         SessionState = iota
	StateAuthenticated
	StateCharacterSelected
	StateInGame
)

// Session represents a connected client.
type Session struct {
	ID            int64
	Conn          net.Conn
	State         SessionState
	AccountName   string
	CharacterID   int64
	Certification int32
	SendChan      chan []byte
}

// MessageHandler handles incoming messages from clients.
type MessageHandler func(session *Session, msg protocol.DefaultMessage, body string)

// RawMessageHandler handles raw string messages (e.g., **login) before standard parsing.
// Return true if the message was handled, false to fall through to standard parsing.
type RawMessageHandler func(session *Session, raw string) bool

// ConnectHandler handles new client connections.
type ConnectHandler func(session *Session)

// DisconnectHandler handles client disconnections.
type DisconnectHandler func(session *Session)

// TCPServer manages TCP connections and message routing.
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

// NewTCPServer creates a new TCP server.
func NewTCPServer(addr string) *TCPServer {
	return &TCPServer{
		sessions: make(map[int64]*Session),
		addr:     addr,
		done:     make(chan struct{}),
	}
}

// SetConnectHandler sets the connection handler.
func (s *TCPServer) SetConnectHandler(h ConnectHandler) {
	s.onConnect = h
}

// SetDisconnectHandler sets the disconnection handler.
func (s *TCPServer) SetDisconnectHandler(h DisconnectHandler) {
	s.onDisconnect = h
}

// SetMessageHandler sets the message handler.
func (s *TCPServer) SetMessageHandler(h MessageHandler) {
	s.onMessage = h
}

// SetRawMessageHandler sets the raw message handler.
func (s *TCPServer) SetRawMessageHandler(h RawMessageHandler) {
	s.onRawMessage = h
}

// Start starts listening for connections.
func (s *TCPServer) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.listener = ln
	log.Logf(log.LevelInfo, "Server", "Listening on %s", s.addr)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the server and closes all connections.
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
	log.Logf(log.LevelInfo, "Server", "Server stopped")
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
				log.Logf(log.LevelError, "Server", "Accept error: %v", err)
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

		log.Logf(log.LevelInfo, "Server", "Client connected: %s (ID: %d)", conn.RemoteAddr(), sessionID)

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
	for {
		n, err := session.Conn.Read(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				log.Logf(log.LevelDebug, "Server", "Read error from %d: %v", session.ID, err)
				return
			}
		}

		// Parse message frames: #<code><payload>!
		// Multiple frames may arrive in a single Read() call.
		if n > 0 {
			data := buf[:n]
			// Process all frames in the buffer
			for len(data) > 2 {
				// Find the end of the first frame
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
					break // No complete frame yet
				}

				frame := data[1:endIdx] // Content between # and !
				data = data[endIdx+1:]   // Move past the !

				// Skip the code digit if present
				payloadStart := 0
				if len(frame) > 0 && frame[0] >= '0' && frame[0] <= '9' {
					payloadStart = 1
				}
				payload := string(frame[payloadStart:])

				if len(payload) == 0 {
					continue
				}

				// Check for raw message (e.g., **login)
				handled := false
				if s.onRawMessage != nil {
					decoded := protocol.DecodeString(payload)
					if len(decoded) >= 2 && decoded[0] == '*' && decoded[1] == '*' {
						log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] RAW %q", session.ID, decoded)
						handled = s.onRawMessage(session, decoded)
					}
				}

				if !handled && s.onMessage != nil && len(payload) >= protocol.DefBlockSize {
					msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
					body := ""
					if len(payload) > protocol.DefBlockSize {
						body = protocol.DecodeString(payload[protocol.DefBlockSize:])
					}
					log.Logf(log.LevelInfo, "Server", "<<< RECV [%d] %s Recog=%d Param=%d Tag=%d Series=%d body=%q",
						session.ID, protocol.MsgName(msg.Ident), msg.Recog, msg.Param, msg.Tag, msg.Series, body)
					s.onMessage(session, msg, body)
				}
			}
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
				log.Logf(log.LevelDebug, "Server", "Write error to %d: %v", session.ID, err)
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

	log.Logf(log.LevelInfo, "Server", "Client disconnected: %d", session.ID)

	if s.onDisconnect != nil {
		s.onDisconnect(session)
	}
}

// Send sends a message to a specific session.
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
		encoded += protocol.EncodeString(body)
	}

	frame := protocol.FormatServerFrame(encoded)

	select {
	case session.SendChan <- []byte(frame):
		return nil
	default:
		return fmt.Errorf("send buffer full for session %d", sessionID)
	}
}

// GetSession returns a session by ID.
func (s *TCPServer) GetSession(id int64) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

// GetSessionCount returns the number of connected sessions.
func (s *TCPServer) GetSessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
