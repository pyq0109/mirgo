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

	Config *ServerConfig

	db     *storage.Database
	mapMgr *MapManager
	ItemDB *ItemDB

	MonsterDB  *MonsterDB
	DropTables *DropTable
	monGenPath string

	Monsters      []*MonsterObject
	Npcs          []*NpcObject
	MonGenList    []MonGenEntry
	nextMonsterID int32
	nextItemID    int32

	MagicDB *MagicDB
	Parties map[int32]*Party
	Guilds  []*Guild
}

func NewUserEngine(db *storage.Database, mapMgr *MapManager) *UserEngine {
	return &UserEngine{
		PlayObjectList: make(map[int32]*PlayObject),
		db:             db,
		mapMgr:         mapMgr,
		nextMonsterID:  100000,
		nextItemID:     200000,
		Parties:        make(map[int32]*Party),
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

func (e *UserEngine) ProcessEvents(server *netserver.TCPServer, now int64) {
	e.mapMgr.mu.RLock()
	defer e.mapMgr.mu.RUnlock()
	for _, env := range e.mapMgr.maps {
		env.ProcessMapEvents(server, now)
	}
}

func (e *UserEngine) GetPlayerCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.PlayObjectList)
}

func (e *UserEngine) GetPlayerByName(name string) *PlayObject {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, p := range e.PlayObjectList {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (e *UserEngine) SaveAllPlayers(db *storage.Database) {
	e.mu.RLock()
	players := make([]*PlayObject, 0, len(e.PlayObjectList))
	for _, p := range e.PlayObjectList {
		players = append(players, p)
	}
	e.mu.RUnlock()
	for _, p := range players {
		saveCharacterData(db, p)
	}
}
