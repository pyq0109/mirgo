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
	e.LoadNpcs()
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
}

func (e *UserEngine) LoadNpcs() {
	if e.npcConfigDir == "" {
		return
	}

	npcListPath := filepath.Join(e.npcConfigDir, "npc_list.jsonc")
	merchantListPath := filepath.Join(e.npcConfigDir, "merchant_list.jsonc")
	npcScriptsDir := filepath.Join(e.npcConfigDir, "npc_scripts")
	merchantScriptsDir := filepath.Join(e.npcConfigDir, "merchant_scripts")

	npcDefs := LoadNpcList(npcListPath)
	merchantDefs := LoadMerchantList(merchantListPath)

	npcCount := 0
	merchantCount := 0

	for _, def := range npcDefs {
		env := e.mapMgr.FindMap(def.MapName)
		if env == nil {
			continue
		}

		npc := NewNpcObject(def.Name, e.nextMonsterID, uint16(def.Body))
		e.nextMonsterID++
		npc.MapName = def.MapName
		npc.CurrX = def.X
		npc.CurrY = def.Y
		npc.Face = def.Face
		npc.Race = def.Race
		npc.envir = env

		scriptPath := filepath.Join(npcScriptsDir, def.Name+"-"+def.MapName+".txt")
		if _, err := os.Stat(scriptPath); err == nil {
			npc.Script = scriptPath
		}

		env.AddObject(npc.CurrX, npc.CurrY, OS_MOVINGOBJECT, npc)
		e.Npcs = append(e.Npcs, npc)
		npcCount++
	}

	for _, def := range merchantDefs {
		env := e.mapMgr.FindMap(def.MapName)
		if env == nil {
			continue
		}

		npc := NewNpcObject(def.Name, e.nextMonsterID, uint16(def.Body))
		e.nextMonsterID++
		npc.MapName = def.MapName
		npc.CurrX = def.X
		npc.CurrY = def.Y
		npc.Face = def.Face
		npc.IsMerchant = true
		npc.MerchantID = def.ID
		npc.Castle = def.Castle != 0
		npc.envir = env

		scriptPath := filepath.Join(merchantScriptsDir, def.ID+"-"+def.MapName+".txt")
		if _, err := os.Stat(scriptPath); err == nil {
			npc.Script = scriptPath
		}

		env.AddObject(npc.CurrX, npc.CurrY, OS_MOVINGOBJECT, npc)
		e.Npcs = append(e.Npcs, npc)
		merchantCount++
	}

	log.Logf(log.LevelInfo, "MonGen", "spawned %d NPCs + %d merchants from config", npcCount, merchantCount)
}

func (e *UserEngine) loadMonGenFromFile(homeMap string) bool {
	if e.monGenPath == "" {
		return false
	}

	data, err := os.ReadFile(e.monGenPath)
	if err != nil {
		log.Logf(log.LevelWarn, "MonGen", "failed to load %s: %v, using default config", e.monGenPath, err)
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
		log.Logf(log.LevelWarn, "MonGen", "failed to parse %s: %v", e.monGenPath, err)
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

	log.Logf(log.LevelInfo, "MonGen", "loaded %d spawn points from %s", len(e.MonGenList), e.monGenPath)
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
				log.Logf(log.LevelInfo, "MonGen", "corpse removed: %s (id=%d)", m.Name, m.ID)
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

	mon := NewMonsterObject(entry.MonName, id, byte(def.Race), byte(def.RaceImg), uint16(def.Appr), def.HP, 1400, 2000, def.Exp)
	mon.MapName = entry.MapName
	mon.CurrX = x
	mon.CurrY = y
	mon.HomeX = entry.X
	mon.HomeY = entry.Y
	mon.envir = env
	e.initMonsterFromDef(mon, def, now)

	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	e.Monsters = append(e.Monsters, mon)
	entry.LiveList = append(entry.LiveList, mon)

	// log.Logf(log.LevelInfo, "MonGen", "spawned %s (id=%d) at %s(%d,%d)", mon.Name, mon.ID, mon.MapName, x, y)
	return mon
}

// initMonsterFromDef — Delphi MonInitialize (UsrEngn.pas:2578)：
// 从数据库定义加载完整属性，设置 AI 计时器与特殊 Race 初始状态。
func (e *UserEngine) initMonsterFromDef(mon *MonsterObject, def *MonsterDef, now int64) {
	if def.Speed > 0 {
		mon.WalkSpeed = int64(2000 - def.Speed*100)
		if mon.WalkSpeed < 200 {
			mon.WalkSpeed = 200
		}
		mon.AttackSpeed = mon.WalkSpeed + 900
	}
	mon.HitPoint = def.Hit
	mon.SpeedPoint = def.Speed
	mon.WAbil.Level = uint16(def.Lvl)
	mon.WAbil.MaxHP = uint16(def.HP)
	mon.WAbil.HP = uint16(def.HP)
	mon.WAbil.AC = uint32(def.AC) | uint32(def.AC)<<16    // MakeLong(wAC, wAC)
	mon.WAbil.MAC = uint32(def.MAC) | uint32(def.MAC)<<16 // MakeLong(wMAC, wMAC)
	mon.WAbil.DC = uint32(def.DC) | uint32(def.DCMax)<<16
	mon.WAbil.MC = uint32(def.MC) | uint32(def.MC)<<16
	mon.WAbil.SC = uint32(def.SC) | uint32(def.SC)<<16
	mon.ViewRange = def.ViewRange
	mon.CoolEye = def.CoolEye
	if def.WalkStep > 0 {
		mon.WalkStep = def.WalkStep
	}
	if def.WalkWait > 0 {
		mon.WalkWait = int64(def.WalkWait)
	}
	mon.initAITimers(now)

	// Delphi 对齐：特殊 Race 初始状态
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
	case 82, 119: // TSpitSpider/TBigPoisionSpider — 喷吐附带绿毒
		mon.spitPoison = true
	case 83: // TSlowATMonster — 慢速近战，攻击冷却翻倍
		mon.AttackSpeed *= 2
	case 85: // TStickMonster — 出生即潜地，固定不可移动
		mon.FixedHide = true
		mon.StickMode = true
	case 101: // TScultureMonster — 出生即石化
		mon.StoneMode = true
	case 102: // TScultureKingMonster — 出生即石化 + 危险等级召唤
		mon.StoneMode = true
		mon.dangerLevel = 5
	case 107: // TCentipedeKingMonster — 出生即潜地，固定不可移动
		mon.StickMode = true
		mon.FixedHide = true
	case 103, 116: // TBeeQueen/TSpiderHouseMonster — 固定召唤巢穴
		mon.StickMode = true
	case 110, 111, 112: // TCastleDoor/TWallStructure/TArcherGuard — 固定
		mon.StickMode = true
	case 115: // TBigHeartMonster — 固定脉冲，视野 16 格
		mon.StickMode = true
		if mon.ViewRange <= 0 {
			mon.ViewRange = 16
		}
	}
}

// SpawnMonsterByName 按数据库定义在指定位置生成怪物（GM 命令/NPC 脚本/子体生成共用）。
// 数据库无对应条目时使用通用近战怪兜底。位置不可走时在 3×3 范围内另找空位。
func (e *UserEngine) SpawnMonsterByName(mapName string, x, y int, name string, now int64) *MonsterObject {
	if e.mapMgr == nil {
		return nil
	}
	env := e.mapMgr.FindMap(mapName)
	if env == nil {
		return nil
	}
	if !env.CanWalk(x, y) {
		placed := false
		for dy := -1; dy <= 1 && !placed; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if env.CanWalk(x+dx, y+dy) {
					x, y = x+dx, y+dy
					placed = true
					break
				}
			}
		}
		if !placed {
			return nil
		}
	}

	def := e.MonsterDB.GetByName(name)
	if def == nil {
		def = &MonsterDef{Name: name, Race: 81, RaceImg: 50, Appr: 190, HP: 100, Exp: 50, DC: 10, DCMax: 15, Speed: 12, Hit: 5}
	}

	e.mu.Lock()
	id := e.nextMonsterID
	e.nextMonsterID++
	e.mu.Unlock()

	mon := NewMonsterObject(def.Name, id, byte(def.Race), byte(def.RaceImg), uint16(def.Appr), def.HP, 1400, 2000, def.Exp)
	mon.MapName = mapName
	mon.CurrX, mon.CurrY = x, y
	mon.HomeX, mon.HomeY = x, y
	mon.envir = env
	e.initMonsterFromDef(mon, def, now)

	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	e.mu.Lock()
	e.Monsters = append(e.Monsters, mon)
	e.mu.Unlock()
	mon.SendRefMsg(RM_TURN, mon.Dir, x, y, mon.Name)
	return mon
}

// countLiveChildren 统计 masterID 名下存活（未死亡、未幽灵）的子体数量。
func (e *UserEngine) countLiveChildren(masterID int32) int {
	n := 0
	for _, m := range e.Monsters {
		if m.MasterID == masterID && !m.Death && !m.Ghost {
			n++
		}
	}
	return n
}

// spawnChild 生成子体怪物（Delphi RegenMonsterByName 的对应实现）。
// childName 非空且数据库有定义时按定义生成；否则克隆父体属性（HP 减半）。
// 子体继承父体的目标，MasterID 记为父体 ID。位置不可走时返回 nil。
func (e *UserEngine) spawnChild(parent *MonsterObject, childName string, x, y int, now int64) *MonsterObject {
	if e.mapMgr == nil || parent.envir == nil {
		return nil
	}
	env := e.mapMgr.FindMap(parent.MapName)
	if env == nil || !env.CanWalk(x, y) {
		return nil
	}
	e.mu.Lock()
	id := e.nextMonsterID
	e.nextMonsterID++
	e.mu.Unlock()

	var child *MonsterObject
	var def *MonsterDef
	if childName != "" && e.MonsterDB != nil {
		def = e.MonsterDB.GetByName(childName)
	}
	if def != nil {
		// 按数据库定义生成（与 SpawnMonster 相同的属性接线）
		child = NewMonsterObject(def.Name, id, byte(def.Race), byte(def.RaceImg), uint16(def.Appr),
			def.HP, 1400, 2000, def.Exp)
		e.initMonsterFromDef(child, def, now)
	} else {
		// 数据库无对应条目：克隆父体（召唤惯例 HP 减半），强制普通近战
		// 避免子体继承召唤/蜂巢等特殊 AI
		child = NewMonsterObject(parent.Name+"(召唤)", id, parent.Race, parent.RaceImg, parent.Appr,
			parent.MaxHP/2, parent.WalkSpeed, parent.AttackSpeed, parent.Exp/3)
		child.AIBehavior = AIMelee
		child.HitPoint = parent.HitPoint
		child.SpeedPoint = parent.SpeedPoint
		child.WAbil.Level = parent.WAbil.Level
		child.WAbil.DC = parent.WAbil.DC
		child.WAbil.AC = parent.WAbil.AC
		child.WAbil.MAC = parent.WAbil.MAC
		child.WAbil.MaxHP = uint16(child.MaxHP)
		child.WAbil.HP = uint16(child.MaxHP)
		child.ViewRange = parent.ViewRange
		child.CoolEye = parent.CoolEye
	}
	child.MapName = parent.MapName
	child.CurrX = x
	child.CurrY = y
	child.HomeX = x
	child.HomeY = y
	child.envir = env
	child.MasterID = parent.ID
	child.TargetID = parent.TargetID
	child.initAITimers(now)

	env.AddObject(x, y, OS_MOVINGOBJECT, child)
	e.mu.Lock()
	e.Monsters = append(e.Monsters, child)
	e.mu.Unlock()
	child.SendRefMsg(RM_TURN, child.Dir, x, y, child.Name)
	return child
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
		child.MasterID = parent.ID
		child.HitPoint = parent.HitPoint
		child.SpeedPoint = parent.SpeedPoint
		child.WAbil.Level = parent.WAbil.Level
		child.WAbil.DC = parent.WAbil.DC
		child.WAbil.AC = parent.WAbil.AC
		child.WAbil.MAC = parent.WAbil.MAC
		child.WAbil.MaxHP = uint16(child.MaxHP)
		child.WAbil.HP = uint16(child.MaxHP)
		child.AIBehavior = AIMelee // 分裂体为普通近战
		child.initAITimers(now)
		parent.envir.AddObject(cx, cy, OS_MOVINGOBJECT, child)
		e.Monsters = append(e.Monsters, child)
		child.SendRefMsg(RM_TURN, child.Dir, cx, cy, child.Name)
	}
	log.Logf(log.LevelInfo, "Monster", "%s split into small zombies", parent.Name)
}
