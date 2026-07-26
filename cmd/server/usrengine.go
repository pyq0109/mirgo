package main

import (
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/storage"
)

type UserEngine struct {
	PlayObjectList map[int32]*PlayObject
	mu             sync.RWMutex

	db     *storage.Database
	mapMgr *MapManager
}

func NewUserEngine(db *storage.Database, mapMgr *MapManager) *UserEngine {
	return &UserEngine{
		PlayObjectList: make(map[int32]*PlayObject),
		db:             db,
		mapMgr:         mapMgr,
	}
}

func (e *UserEngine) AddPlayer(player *PlayObject) {
	e.mu.Lock()
	e.PlayObjectList[player.ID] = player
	e.mu.Unlock()
	log.Logf(log.LevelInfo, "UserEngine", "Player %s added (total: %d)", player.Name, len(e.PlayObjectList))
}

func (e *UserEngine) RemovePlayer(id int32) {
	e.mu.Lock()
	delete(e.PlayObjectList, id)
	e.mu.Unlock()
	log.Logf(log.LevelInfo, "UserEngine", "Player %d removed (total: %d)", id, len(e.PlayObjectList))
}

func (e *UserEngine) GetPlayer(id int32) *PlayObject {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.PlayObjectList[id]
}

func (e *UserEngine) ProcessHumans(server *netserver.TCPServer) {
	e.mu.RLock()
	players := make([]*PlayObject, 0, len(e.PlayObjectList))
	for _, p := range e.PlayObjectList {
		players = append(players, p)
	}
	e.mu.RUnlock()

	for _, player := range players {
		player.Operate(server)
	}
}

func (e *UserEngine) ProcessDoors(currentTick int64) {
	e.mapMgr.mu.RLock()
	defer e.mapMgr.mu.RUnlock()
	for _, env := range e.mapMgr.maps {
		ProcessDoors(env, currentTick)
	}
}

func (e *UserEngine) GetPlayerCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.PlayObjectList)
}
