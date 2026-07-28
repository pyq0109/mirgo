package main

import (
	"sync"

	"github.com/pyq0109/mirgo/internal/netserver"
)

// SessionManager 管理客户端会话。
type SessionManager struct {
	sessions map[int64]*netserver.Session
	mu       sync.RWMutex
}

// NewSessionManager 创建新的会话管理器。
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*netserver.Session),
	}
}

// Add 添加一个会话。
func (m *SessionManager) Add(session *netserver.Session) {
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
}

// Remove 移除一个会话。
func (m *SessionManager) Remove(id int64) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// Get 根据 ID 返回会话。
func (m *SessionManager) Get(id int64) *netserver.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetByAccount 根据账号名返回会话。
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

// Count 返回会话数量。
func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
