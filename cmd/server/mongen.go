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
		// log.Logf(log.LevelInfo, "MonGen", "已在 %s(%d,%d) 生成 NPC %s", npc.Name, npc.MapName, npc.CurrX, npc.CurrY)
	}
}

func (e *UserEngine) loadMonGenFromFile(homeMap string) bool {
	if e.monGenPath == "" {
		return false
	}

	data, err := os.ReadFile(e.monGenPath)
	if err != nil {
		log.Logf(log.LevelWarn, "MonGen", "加载 %s 失败: %v，使用默认配置", e.monGenPath, err)
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
		log.Logf(log.LevelWarn, "MonGen", "解析 %s 失败: %v", e.monGenPath, err)
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

	log.Logf(log.LevelInfo, "MonGen", "已从 %s 加载 %d 个刷怪点", e.monGenPath, len(e.MonGenList))
	return len(e.MonGenList) > 0
}

func (e *UserEngine) ProcessMonsters(server *netserver.TCPServer, now int64) {
	// Delphi: round-robin 每 tick 只处理一个刷怪器
	if len(e.MonGenList) > 0 {
		entry := &e.MonGenList[e.currMonGen]
		e.currMonGen = (e.currMonGen + 1) % len(e.MonGenList)

		if now-entry.LastTick > entry.ZenTime {
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
				e.SpawnMonster(entry, server, now)
				live++
			}
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
				// Delphi TZilKinZombi: 死亡分裂
				if m.AIBehavior == AISplit && m.envir != nil {
					e.spawnSplitZombies(m, server, now)
				}
			}
			if m.DeathTick > 0 && now-m.DeathTick > 180000 {
				m.Ghost = true
				if m.envir != nil {
					m.envir.RemoveObject(m.CurrX, m.CurrY, OS_MOVINGOBJECT, m)
				}
				log.Logf(log.LevelInfo, "MonGen", "尸体已移除: %s (id=%d)", m.Name, m.ID)
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

func (e *UserEngine) SpawnMonster(entry *MonGenEntry, server *netserver.TCPServer, now int64) *MonsterObject {
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

	walkSpeed := int64(1400)
	attackSpeed := int64(2000)
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
	mon.HitPoint = def.Hit
	mon.SpeedPoint = def.Speed
	mon.BaseObject.WAbil.DC = uint32(def.DC) | uint32(def.DCMax)<<16
	mon.BaseObject.WAbil.AC = uint32(def.AC) | uint32(def.MAC)<<16
	mon.ViewRange = def.ViewRange
	mon.CoolEye = def.CoolEye
	if def.WalkStep > 0 {
		mon.WalkStep = def.WalkStep
	}
	if def.WalkWait > 0 {
		mon.WalkWait = int64(def.WalkWait)
	}

	// Delphi 对齐：特殊 Race 初始状态
	mon.spawnTick = now
	mon.searchInterval = 3000 + rand.Int63n(2000) // 3-5秒随机
	switch byte(def.Race) {
	case 51, 53, 84:
		mon.Animal = true
	case 52: // Delphi: 1/30 概率为逃跑鹿（TChickenDeer），29/30 为被动鹿
		mon.Animal = true
		if rand.Intn(30) == 0 {
			mon.AIBehavior = AIFlee
		} else {
			mon.AIBehavior = AIPassive
		}
	case 83: // TSlowATMonster — 慢速近战，攻击冷却翻倍
		mon.AttackSpeed *= 2
	case 85: // TStickMonster — 出生即潜地
		mon.FixedHide = true
	case 101: // TScultureMonster — 出生即石化
		mon.StoneMode = true
	case 107: // TCentipedeKingMonster — 固定Boss，不可移动
		mon.StickMode = true
	}

	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	e.Monsters = append(e.Monsters, mon)
	entry.LiveList = append(entry.LiveList, mon)

	// log.Logf(log.LevelInfo, "MonGen", "已生成 %s (id=%d) 于 %s(%d,%d)", mon.Name, mon.ID, mon.MapName, x, y)
	return mon
}

// spawnSplitZombies — TZilKinZombi 死亡分裂：生成 2-3 个小僵尸
func (e *UserEngine) spawnSplitZombies(parent *MonsterObject, server *netserver.TCPServer, now int64) {
	count := 2 + rand.Intn(2) // 2-3 个
	for i := 0; i < count; i++ {
		cx := parent.CurrX + rand.Intn(3) - 1
		cy := parent.CurrY + rand.Intn(3) - 1
		if !parent.envir.CanWalk(cx, cy) {
			continue
		}
		id := e.nextMonsterID
		e.nextMonsterID++
		child := NewMonsterObject(parent.Name, id, parent.Race, parent.RaceImg, parent.Appr,
			parent.MaxHP/3, parent.WalkSpeed, parent.AttackSpeed, parent.Exp/3)
		child.MapName = parent.MapName
		child.CurrX = cx
		child.CurrY = cy
		child.HomeX = parent.HomeX
		child.HomeY = parent.HomeY
		child.envir = parent.envir
		child.HitPoint = parent.HitPoint
		child.SpeedPoint = parent.SpeedPoint
		child.WAbil.DC = parent.WAbil.DC
		child.WAbil.AC = parent.WAbil.AC
		child.WAbil.HP = child.WAbil.MaxHP
		child.AIBehavior = AIMelee // 分裂体为普通近战
		child.spawnTick = now
		child.searchInterval = 3000 + rand.Int63n(2000)
		parent.envir.AddObject(cx, cy, OS_MOVINGOBJECT, child)
		e.Monsters = append(e.Monsters, child)
		child.SendRefMsg(RM_TURN, child.Dir, cx, cy, child.Name)
	}
	log.Logf(log.LevelInfo, "Monster", "%s 分裂成小僵尸", parent.Name)
}
