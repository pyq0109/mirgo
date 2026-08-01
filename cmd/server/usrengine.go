package main

import (
	"sync"
	"time"

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
	npcConfigDir string

	Monsters      []*MonsterObject
	Npcs          []*NpcObject
	MonGenList    []MonGenEntry
	nextMonsterID int32
	nextItemID    int32
	currMonGen    int // round-robin 刷怪器索引

	MagicDB *MagicDB
	Parties map[int32]*Party
	Guilds  []*Guild
	Castle  *CastleObject

	NoMonGen bool
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
	log.Logf(log.LevelInfo, "UserEngine", "player %s joined (total: %d)", player.Name, len(e.PlayObjectList))
}

func (e *UserEngine) RemovePlayer(id int32) {
	e.mu.Lock()
	delete(e.PlayObjectList, id)
	e.mu.Unlock()
	log.Logf(log.LevelInfo, "UserEngine", "player %d removed (total: %d)", id, len(e.PlayObjectList))
}



func (e *UserEngine) GetPlayer(id int32) *PlayObject {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.PlayObjectList[id]
}

func (e *UserEngine) GetMonster(id int32) *MonsterObject {
	for _, mon := range e.Monsters {
		if mon.ID == id && !mon.Ghost {
			return mon
		}
	}
	return nil
}

func (e *UserEngine) clearMapMonsters(env *Environment) {
	for _, mon := range e.Monsters {
		if mon.envir == env && !mon.Death && !mon.Ghost && !mon.IsSafeZoneGuard {
			mon.Death = true
			mon.DeathTick = time.Now().UnixMilli()
			mon.WAbil.HP = 0
		}
	}
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

func (e *UserEngine) ProcessMineRegen(now int64) {
	e.mapMgr.ProcessMineRegen(now)
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

func (e *UserEngine) ProcessNpcs() {
	for _, npc := range e.Npcs {
		if npc.IsMerchant && len(npc.RefillConfig) > 0 {
			npc.RefillGoods(e.ItemDB)
		}
		npc.SaveData(e.db)
	}
}

func (e *UserEngine) ProcessNpcIdle() {
	for _, npc := range e.Npcs {
		if npc.IsMerchant {
			// 城堡战 NPC 隐藏（Delphi ObjNpc.pas:1642-1657）
			if npc.Castle && e.Castle != nil && e.Castle.IsAtWar() {
				if !npc.FixedHideMode {
					npc.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
					npc.FixedHideMode = true
				}
			} else {
				if npc.FixedHideMode {
					npc.FixedHideMode = false
					npc.SendRefMsg(RM_HIT, npc.Dir, npc.CurrX, npc.CurrY, "")
				}
			}
			npc.idleAnimate()
		}
	}
}

func (e *UserEngine) InvalidateAllNpcScripts() {
	for _, npc := range e.Npcs {
		npc.InvalidateScript()
	}
	log.Logf(log.LevelInfo, "NPC", "all NPC scripts invalidated (%d NPCs)", len(e.Npcs))
}

// awardExpForMonster — 怪物被毒杀时，将经验奖励给 LastHiterID。
func (e *UserEngine) awardExpForMonster(mon *MonsterObject, server *netserver.TCPServer) {
	if mon.LastHiterID == 0 {
		return
	}
	killer := e.GetPlayer(mon.LastHiterID)
	if killer == nil || killer.Ghost || killer.Death {
		return
	}
	killer.awardExp(server, mon)
}

// CountMapHumans 统计指定地图的在线玩家数（Delphi ObjNpc.pas:11131-11153）。
func (e *UserEngine) CountMapHumans(mapName string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, p := range e.PlayObjectList {
		if p.MapName == mapName && !p.Ghost {
			count++
		}
	}
	return count
}

// CountMapMonsters 统计指定地图的怪物数（Delphi ObjNpc.pas:11157-11182）。
func (e *UserEngine) CountMapMonsters(mapName string) int {
	count := 0
	for _, mon := range e.Monsters {
		if mon.MapName == mapName && !mon.Death && !mon.Ghost {
			count++
		}
	}
	return count
}
