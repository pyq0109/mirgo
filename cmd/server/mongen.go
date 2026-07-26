package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MonGenEntry struct {
	MapName  string
	X, Y     int
	MonName  string
	Range    int
	Count    int
	ZenTime  int64
	LastTick int64
	LiveList []*MonsterObject
}

type monGenSpawn struct {
	MapName  string `json:"mapName"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Name     string `json:"name"`
	Range    int    `json:"range"`
	Count    int    `json:"count"`
	Interval int    `json:"interval"`
}

func (e *UserEngine) InitWorld(mapMgr *MapManager) {
	e.mapMgr = mapMgr
	e.LoadMonGen()
}

func (e *UserEngine) LoadMonGen() {
	homeMap := "0"
	if e.mapMgr != nil {
		if env := e.mapMgr.FindMap("0"); env == nil {
			if env2 := e.mapMgr.FindMap("3"); env2 != nil {
				homeMap = "3"
			}
		}
	}

	if !e.loadMonGenFromFile(homeMap) {
		e.MonGenList = append(e.MonGenList, MonGenEntry{
			MapName: homeMap,
			X:       289,
			Y:       618,
			MonName: "鸡",
			Range:   10,
			Count:   5,
			ZenTime: 30000,
		})
	}

	npc := NewNpcObject("Merchant", e.nextMonsterID, 1)
	e.nextMonsterID++
	scriptPath := filepath.Join("serverconfig", "npcs", "npc_scripts", npc.Name+".txt")
	if _, err := os.Stat(scriptPath); err == nil {
		npc.Script = scriptPath
	}
	if env := e.mapMgr.FindMap(homeMap); env != nil {
		npc.MapName = homeMap
		npc.CurrX = 291
		npc.CurrY = 615
		npc.envir = env
		env.AddObject(npc.CurrX, npc.CurrY, OS_MOVINGOBJECT, npc)
		e.Npcs = append(e.Npcs, npc)
		log.Logf(log.LevelInfo, "MonGen", "Spawned NPC %s at %s(%d,%d)", npc.Name, npc.MapName, npc.CurrX, npc.CurrY)
	}
}

func (e *UserEngine) loadMonGenFromFile(homeMap string) bool {
	if e.monGenPath == "" {
		return false
	}

	data, err := os.ReadFile(e.monGenPath)
	if err != nil {
		log.Logf(log.LevelWarn, "MonGen", "Failed to load %s: %v, using defaults", e.monGenPath, err)
		return false
	}

	lines := strings.Split(string(data), "\n")
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		clean = append(clean, line)
	}

	var raw struct {
		Spawns []monGenSpawn `json:"spawns"`
	}
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &raw); err != nil {
		log.Logf(log.LevelWarn, "MonGen", "Failed to parse %s: %v", e.monGenPath, err)
		return false
	}

	for _, spawn := range raw.Spawns {
		if e.mapMgr != nil && e.mapMgr.FindMap(spawn.MapName) == nil {
			continue
		}
		interval := spawn.Interval
		if interval <= 0 {
			interval = 10
		}
		e.MonGenList = append(e.MonGenList, MonGenEntry{
			MapName: spawn.MapName,
			X:       spawn.X,
			Y:       spawn.Y,
			MonName: spawn.Name,
			Range:   spawn.Range,
			Count:   spawn.Count,
			ZenTime: int64(interval) * 1000,
		})
	}

	log.Logf(log.LevelInfo, "MonGen", "Loaded %d spawn points from %s", len(e.MonGenList), e.monGenPath)
	return len(e.MonGenList) > 0
}

func (e *UserEngine) ProcessMonsters(server *netserver.TCPServer, now int64) {
	for i := range e.MonGenList {
		entry := &e.MonGenList[i]
		if now-entry.LastTick <= entry.ZenTime {
			continue
		}
		entry.LastTick = now

		live := 0
		newList := make([]*MonsterObject, 0, len(entry.LiveList))
		for _, m := range entry.LiveList {
			if !m.Ghost && !m.Death {
				live++
				newList = append(newList, m)
			}
		}
		entry.LiveList = newList

		for live < entry.Count {
			e.SpawnMonster(entry, server)
			live++
		}
	}

	for _, m := range e.Monsters {
		if m.Ghost {
			continue
		}
		if m.Death {
			if !m.LootDropped {
				m.LootDropped = true
				if m.envir != nil {
					m.DropLootWithTable(m.envir, &e.nextItemID, server, e.DropTables)
				}
			}
			if m.DeathTick > 0 && now-m.DeathTick > 30000 {
				m.Ghost = true
				if m.envir != nil {
					m.envir.RemoveObject(m.CurrX, m.CurrY, OS_MOVINGOBJECT, m)
				}
				log.Logf(log.LevelInfo, "MonGen", "Corpse removed: %s (id=%d)", m.Name, m.ID)
			}
			continue
		}
		m.Run(server, now, e)
	}

	e.despawnGroundItems(server, now)
}

func (e *UserEngine) despawnGroundItems(server *netserver.TCPServer, now int64) {
	e.mapMgr.mu.RLock()
	defer e.mapMgr.mu.RUnlock()
	for _, env := range e.mapMgr.maps {
		var remaining []*GroundItem
		for _, item := range env.GroundItems {
			if now-item.DropTick > 60000 {
				env.RemoveGroundItem(item.ID)
				resp := protocol.MakeDefaultMsg(protocol.SMItemHide, item.ID, 0, 0, 0)
				objs := env.GetRangeObjects(item.X, item.Y, viewRange)
				for _, obj := range objs {
					p, ok := obj.(*PlayObject)
					if !ok || p.Ghost {
						continue
					}
					server.Send(p.Session.ID, resp, "")
				}
			} else {
				remaining = append(remaining, item)
			}
		}
		env.GroundItems = remaining
	}
}

func (e *UserEngine) SpawnMonster(entry *MonGenEntry, server *netserver.TCPServer) *MonsterObject {
	env := e.mapMgr.FindMap(entry.MapName)
	if env == nil {
		return nil
	}

	x, y := entry.X, entry.Y
	for tries := 0; tries < 31; tries++ {
		tx := entry.X + rand.Intn(entry.Range*2+1) - entry.Range
		ty := entry.Y + rand.Intn(entry.Range*2+1) - entry.Range
		if env.CanWalk(tx, ty) {
			x, y = tx, ty
			break
		}
	}

	id := e.nextMonsterID
	e.nextMonsterID++

	def := e.MonsterDB.GetByName(entry.MonName)
	if def == nil {
		def = &MonsterDef{Race: 51, RaceImg: 11, Appr: 160, HP: 5, Exp: 9, DC: 1, DCMax: 1, Speed: 10}
	}

	walkSpeed := int64(600)
	attackSpeed := int64(1500)
	if def.Speed > 0 {
		walkSpeed = int64(2000 - def.Speed*100)
		if walkSpeed < 200 {
			walkSpeed = 200
		}
		attackSpeed = walkSpeed + 900
	}

	mon := NewMonsterObject(entry.MonName, id, byte(def.Race), byte(def.RaceImg), uint16(def.Appr), def.HP, walkSpeed, attackSpeed, def.Exp)
	mon.MapName = entry.MapName
	mon.CurrX = x
	mon.CurrY = y
	mon.HomeX = entry.X
	mon.HomeY = entry.Y
	mon.envir = env
	mon.HitPoint = mon.MaxHP
	mon.BaseObject.WAbil.DC = uint32(def.DC) | uint32(def.DCMax)<<16
	mon.BaseObject.WAbil.AC = uint32(def.AC) | uint32(def.MAC)<<16

	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	e.Monsters = append(e.Monsters, mon)
	entry.LiveList = append(entry.LiveList, mon)

	log.Logf(log.LevelInfo, "MonGen", "Spawned %s (id=%d) at %s(%d,%d)", mon.Name, mon.ID, mon.MapName, x, y)
	return mon
}
