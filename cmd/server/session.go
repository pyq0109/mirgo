package main

import (
	"sync"

	"github.com/pyq0109/mirgo/internal/netserver"
)

// SessionManager manages client sessions.
type SessionManager struct {
	sessions map[int64]*netserver.Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*netserver.Session),
	}
}

// Add adds a session.
func (m *SessionManager) Add(session *netserver.Session) {
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
}

// Remove removes a session.
func (m *SessionManager) Remove(id int64) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// Get returns a session by ID.
func (m *SessionManager) Get(id int64) *netserver.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetByAccount returns a session by account name.
func (m *SessionManager) GetByAccount(name string) *netserver.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.AccountName == name {
			return s
		}
	}
	return nil
}

// Count returns the number of sessions.
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
