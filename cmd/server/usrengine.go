package main

import (
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/storage"
)

// UserEngine manages all players and game world state.
type UserEngine struct {
	// Players
	PlayObjectList map[int32]*PlayObject
	mu             sync.RWMutex

	// Database
	db *storage.Database

	// Map manager
	mapMgr *MapManager
}

// NewUserEngine creates a new user engine.
func NewUserEngine(db *storage.Database, mapMgr *MapManager) *UserEngine {
	return &UserEngine{
		PlayObjectList: make(map[int32]*PlayObject),
		db:             db,
		mapMgr:         mapMgr,
	}
}

// AddPlayer adds a player to the engine.
func (e *UserEngine) AddPlayer(player *PlayObject) {
	e.mu.Lock()
	e.PlayObjectList[player.ID] = player
	e.mu.Unlock()
	log.Logf(log.LevelInfo, "UserEngine", "Player %s added (total: %d)", player.Name, len(e.PlayObjectList))
}

// RemovePlayer removes a player from the engine.
func (e *UserEngine) RemovePlayer(id int32) {
	e.mu.Lock()
	delete(e.PlayObjectList, id)
	e.mu.Unlock()
	log.Logf(log.LevelInfo, "UserEngine", "Player %d removed (total: %d)", id, len(e.PlayObjectList))
}

// GetPlayer returns a player by ID.
func (e *UserEngine) GetPlayer(id int32) *PlayObject {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.PlayObjectList[id]
}

// ProcessHumans processes all player actions for one tick.
func (e *UserEngine) ProcessHumans() {
	e.mu.RLock()
	players := make([]*PlayObject, 0, len(e.PlayObjectList))
	for _, p := range e.PlayObjectList {
		players = append(players, p)
	}
	e.mu.RUnlock()

	for _, player := range players {
		player.Operate()
	}
}

// GetPlayerCount returns the number of online players.
func (e *UserEngine) GetPlayerCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.PlayObjectList)
}
