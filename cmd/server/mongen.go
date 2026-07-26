package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
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

	e.MonGenList = append(e.MonGenList, MonGenEntry{
		MapName: homeMap,
		X:       289,
		Y:       618,
		MonName: "Hen",
		Range:   10,
		Count:   5,
		ZenTime: 30000,
	})

	npc := NewNpcObject("Merchant", e.nextMonsterID, 1)
	e.nextMonsterID++
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
		if !m.Ghost && !m.Death {
			m.Run(server, now, e)
		}
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

	mon := NewMonsterObject(entry.MonName, id, 19, 50, uint16(id%100+1), 100, 600, 1500, 50)
	mon.MapName = entry.MapName
	mon.CurrX = x
	mon.CurrY = y
	mon.HomeX = entry.X
	mon.HomeY = entry.Y
	mon.envir = env
	mon.HitPoint = mon.MaxHP

	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	e.Monsters = append(e.Monsters, mon)
	entry.LiveList = append(entry.LiveList, mon)

	log.Logf(log.LevelInfo, "MonGen", "Spawned %s (id=%d) at %s(%d,%d)", mon.Name, mon.ID, mon.MapName, x, y)
	return mon
}
