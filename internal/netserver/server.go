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
	ID          int64
	Conn        net.Conn
	State       SessionState
	AccountName string
	CharacterID int64
	SendChan    chan []byte
}

// MessageHandler handles incoming messages from clients.
type MessageHandler func(session *Session, msg protocol.DefaultMessage, body string)

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

		// Parse message frame: #<code><payload>!
		// Client sends: #<digit><payload>! (digit is 1-9)
		// Server sends: #<payload>! (no digit)
		if s.onMessage != nil && n > 0 {
			data := buf[:n]
			if len(data) > 2 && data[0] == '#' && data[len(data)-1] == '!' {
				// Skip the '#' prefix and '!' suffix
				payloadStart := 1
				// Check if next char is a digit (client code)
				if len(data) > 3 && data[1] >= '1' && data[1] <= '9' {
					payloadStart = 2 // Skip the code digit
				}
				payload := string(data[payloadStart : len(data)-1])
				if len(payload) >= protocol.DefBlockSize {
					msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
					body := ""
					if len(payload) > protocol.DefBlockSize {
						body = protocol.DecodeString(payload[protocol.DefBlockSize:])
					}
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
