package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MonsterObject struct {
	*BaseObject
	Race        byte
	RaceImg     byte
	Appr        uint16
	MaxHP       int
	WalkSpeed   int64
	AttackSpeed int64
	Exp         int

	AIBehavior int // 0=melee, 1=ranged, 2=flee, 3=area, 4=summoner

	TargetID       int32
	HomeX, HomeY   int
	WalkTick       int64
	SearchTick     int64
	HitTick        int64
	DeathTick      int64
	LootDropped    bool
	lastSummonTick int64
	minionCount    int
}

func getAIBehavior(race byte) int {
	switch {
	case race >= 51 && race <= 55:
		return 1
	case race == 56 || race == 57:
		return 2
	case race >= 60 && race <= 65:
		return 3
	case race >= 70 && race <= 75:
		return 4
	default:
		return 0
	}
}

func NewMonsterObject(name string, id int32, race, raceImg byte, appr uint16, hp int, walkSpeed, attackSpeed int64, exp int) *MonsterObject {
	base := NewBaseObject(name, id)
	return &MonsterObject{
		BaseObject:  base,
		Race:        race,
		RaceImg:     raceImg,
		Appr:        appr,
		MaxHP:       hp,
		WalkSpeed:   walkSpeed,
		AttackSpeed: attackSpeed,
		Exp:         exp,
		AIBehavior:  getAIBehavior(race),
	}
}

func (o *MonsterObject) Feature() int32 {
	return protocol.MakeMonsterFeature(o.RaceImg, 0, o.Appr)
}

func (o *MonsterObject) Run(server *netserver.TCPServer, now int64, userEngine *UserEngine) {
	if o.Ghost || o.Death {
		return
	}

	o.searchTarget(now, userEngine)
	o.validateTarget(userEngine)

	if o.TargetID != 0 {
		target := userEngine.GetPlayer(o.TargetID)
		if target == nil || target.Death || target.Ghost {
			o.TargetID = 0
			return
		}
		dist := abs(target.CurrX-o.CurrX) + abs(target.CurrY-o.CurrY)

		switch o.AIBehavior {
		case 0:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target)
			}
		case 1:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else if dist <= 5 {
				if now-o.HitTick > o.AttackSpeed {
					o.HitTick = now
					o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
					damage := o.calcMonsterDamage(target.BaseObject)
					o.applyMonsterDamageToPlayer(server, target, damage)
					o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
				}
			} else {
				o.chaseTarget(target)
			}
		case 2:
			if int(o.WAbil.HP) < o.MaxHP/5 {
				fleeDir := dirToward(target.CurrX, target.CurrY, o.CurrX, o.CurrY)
				if now-o.WalkTick >= o.WalkSpeed {
					o.WalkTick = now
					if o.WalkTo(fleeDir) {
						o.SendRefMsg(RM_WALK, fleeDir, o.CurrX, o.CurrY, "")
					}
				}
			} else if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target)
			}
		case 3:
			if dist <= 1 && now-o.HitTick > o.AttackSpeed {
				o.HitTick = now
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						tx, ty := o.CurrX+dx, o.CurrY+dy
						obj := o.envir.GetMovingObject(tx, ty)
						if obj == nil {
							continue
						}
						if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
							damage := o.calcMonsterDamage(p.BaseObject)
							o.applyMonsterDamageToPlayer(server, p, damage)
						}
					}
				}
				o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
				o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
			} else if dist > 1 {
				o.chaseTarget(target)
			}
		case 4:
			if now-o.lastSummonTick > 30000 && o.minionCount < 3 {
				o.lastSummonTick = now
				o.minionCount++
				log.Logf(log.LevelInfo, "Monster", "%s summoned a minion", o.Name)
			}
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target)
			}
		}
	} else {
		if now-o.WalkTick > o.WalkSpeed*3 {
			if rand.Intn(20) == 0 {
				o.WalkTick = now
				if rand.Intn(4) == 0 {
					o.TurnTo(rand.Intn(8))
				} else {
					dir := rand.Intn(8)
					if o.WalkTo(dir) {
						o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
					}
				}
			}
		}
	}
}

func (o *MonsterObject) calcMonsterDamage(target *BaseObject) int {
	loDC := int(o.WAbil.DC & 0xFFFF)
	hiDC := int(o.WAbil.DC >> 16)
	attack := loDC
	if hiDC > loDC {
		attack = loDC + rand.Intn(hiDC-loDC+1)
	}
	if attack < 1 {
		attack = 1
	}

	loAC := int(target.WAbil.AC & 0xFFFF)
	damage := attack - loAC
	if damage < 1 {
		damage = 1
	}
	return damage
}

func (o *MonsterObject) applyMonsterDamageToPlayer(server *netserver.TCPServer, target *PlayObject, damage int) {
	hp := int(target.WAbil.HP)
	hp -= damage
	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	if o.envir != nil {
		o.envir.broadcastRefMsg(target.BaseObject, RM_STRUCK, o.ID, target.CurrX, target.CurrY, o.Dir)
	}

	if hp <= 0 {
		target.Death = true
		target.deathTick = time.Now().UnixMilli()
		if o.envir != nil {
			o.envir.broadcastRefMsg(target.BaseObject, RM_DEATH, target.ID, target.CurrX, target.CurrY, o.Dir)
		}
		log.Logf(log.LevelInfo, "Combat", "%s killed %s", o.Name, target.Name)
	} else {
		target.sendHealthSpell(server)
	}
}

func (o *MonsterObject) chaseTarget(target *PlayObject) {
	now := time.Now().UnixMilli()
	if now-o.WalkTick < o.WalkSpeed {
		return
	}
	o.WalkTick = now
	dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	if o.WalkTo(dir) {
		o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
	}
}

func (o *MonsterObject) meleeAttack(server *netserver.TCPServer, target *PlayObject, now int64) {
	if now-o.HitTick < o.AttackSpeed {
		return
	}
	o.HitTick = now
	o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	if IsSafeZone(o.envir, target.CurrX, target.CurrY) {
		return
	}
	damage := o.calcMonsterDamage(target.BaseObject)
	o.applyMonsterDamageToPlayer(server, target, damage)
	o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
}

func (o *MonsterObject) searchTarget(now int64, userEngine *UserEngine) {
	hasTarget := o.TargetID != 0
	interval := int64(1000)
	if hasTarget {
		interval = 8000
	}
	if now-o.SearchTick <= interval {
		return
	}
	o.SearchTick = now

	if o.envir == nil {
		return
	}

	objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, 12)
	var best *PlayObject
	bestDist := 999999
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death || p.Hidden {
			continue
		}
		d := abs(p.CurrX-o.CurrX) + abs(p.CurrY-o.CurrY)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	if best != nil {
		o.TargetID = best.ID
	}
}

func (o *MonsterObject) validateTarget(userEngine *UserEngine) {
	if o.TargetID == 0 {
		return
	}
	target := userEngine.GetPlayer(o.TargetID)
	if target == nil || target.Ghost || target.Death {
		o.TargetID = 0
		return
	}
	if target.MapName != o.MapName {
		o.TargetID = 0
		return
	}
	dx := abs(target.CurrX - o.CurrX)
	dy := abs(target.CurrY - o.CurrY)
	if dx > 15 || dy > 15 {
		o.TargetID = 0
	}
}

func dirToward(fromX, fromY, toX, toY int) int {
	dx := toX - fromX
	dy := toY - fromY

	sx, sy := 0, 0
	if dx > 0 {
		sx = 1
	} else if dx < 0 {
		sx = -1
	}
	if dy > 0 {
		sy = 1
	} else if dy < 0 {
		sy = -1
	}

	switch {
	case sx == 0 && sy == -1:
		return 0
	case sx == 1 && sy == -1:
		return 1
	case sx == 1 && sy == 0:
		return 2
	case sx == 1 && sy == 1:
		return 3
	case sx == 0 && sy == 1:
		return 4
	case sx == -1 && sy == 1:
		return 5
	case sx == -1 && sy == 0:
		return 6
	case sx == -1 && sy == -1:
		return 7
	}
	return 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
