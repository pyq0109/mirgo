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

	AIBehavior int // 0=melee, 1=ranged, 2=flee, 3=area, 4=summoner, 5+=extended

	TargetID       int32
	HomeX, HomeY   int
	WalkTick       int64
	SearchTick     int64
	HitTick        int64
	DeathTick      int64
	LootDropped    bool
	lastSummonTick int64
	minionCount    int

	LastHiterID   int32
	LastHiterTick int64
	FocusTick     int64

	walkCount    int
	walkWaitTick int64
	WalkStep     int
	WalkWait     int64

	lastRegenTick int64
	Engine        *UserEngine

	ViewRange int
	CoolEye   int

	lastThinkTick int64
}

func getAIBehavior(race byte) int {
	// Race→AI mapping based on Delphi factory (UsrEngn.pas:1831-1938).
	// Animals (50+) are mostly basic melee unless specifically noted.
	switch race {
	case 52: // TChickenDeer — flee
		return AIFlee
	case 82: // TSpitSpider — ranged + poison
		return AIRanged
	case 90: // TGasAttackMonster — area
		return AIArea
	case 91: // TMagCowMonster — magic
		return AIMagicCast
	case 94: // TLightingZombi — ranged lightning
		return AIRanged
	case 95: // TDigOutZombi — burrow
		return AIBurrow
	case 102: // TScultureKingMonster — summoner
		return AISummoner
	case 105: // TGasMothMonster — area
		return AIArea
	case 117: // TExplosionSpider — explode
		return AIExplode
	case 118: // THighRiskSpider — ranged
		return AIRanged
	case 119: // TBigPoisionSpider — ranged + poison
		return AIRanged
	case 200: // TElectronicScolpionMon — ranged
		return AIRanged
	default:
		// 51(chicken), 53(wolf), 80(oma), 81(oma knight), 83(slow),
		// 84(scorpion), 85(stick), 87(dual axe), 92(cow king TODO),
		// 96(zilkin TODO), 100(white skeleton), 101(sculpture TODO),
		// 103(bee queen), 107(centipede king TODO), 130(double critical)
		return AIMelee
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
		WalkStep:    3,
		WalkWait:    1000,
	}
}

func (o *MonsterObject) Feature() int32 {
	return protocol.MakeMonsterFeature(o.RaceImg, 0, o.Appr)
}

func (o *MonsterObject) OnStruck(attackerID int32, now int64) {
	o.LastHiterID = attackerID
	o.LastHiterTick = now
	o.WalkTick += 800
	penalty := int64(150)
	if lvl := int64(o.WAbil.Level) * 4; lvl < 130 {
		penalty = 150 - lvl
	} else {
		penalty = 20
	}
	o.HitTick += penalty
	if o.TargetID == 0 || rand.Intn(6) == 0 {
		o.TargetID = attackerID
		o.FocusTick = now
	}
}

func (o *MonsterObject) Run(server *netserver.TCPServer, now int64, userEngine *UserEngine) {
	if o.Ghost || o.Death {
		return
	}

	if int(o.WAbil.HP) < o.MaxHP {
		if now-o.lastRegenTick >= 6000 {
			o.lastRegenTick = now
			regen := o.MaxHP/75 + 1
			hp := int(o.WAbil.HP) + regen
			if hp > o.MaxHP {
				hp = o.MaxHP
			}
			o.WAbil.HP = uint16(hp)
		}
	}

	if now-o.lastThinkTick >= 3000 {
		o.lastThinkTick = now
		if o.envir != nil {
			objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, 0)
			for _, obj := range objs {
				if obj != o {
					if _, isMon := obj.(*MonsterObject); isMon {
						o.WalkTo(rand.Intn(8))
						break
					}
				}
			}
		}
	}

	o.searchTarget(now, userEngine)
	o.validateTarget(now, userEngine)

	if o.TargetID != 0 {
		target := userEngine.GetPlayer(o.TargetID)
		if target == nil || target.Death || target.Ghost {
			o.TargetID = 0
			return
		}
		dx := abs(target.CurrX - o.CurrX)
		dy := abs(target.CurrY - o.CurrY)
		dist := dx
		if dy > dist {
			dist = dy
		}

		switch o.AIBehavior {
		case 0:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target, now)
			}
		case 1:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else if dist <= 5 {
				if now-o.HitTick > o.AttackSpeed {
					o.HitTick = now
					o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
					if rand.Intn(100) < 80 {
						damage := o.calcMonsterDamage(target.BaseObject)
						o.applyMonsterDamageToPlayer(server, target, damage, now)
						o.FocusTick = now
					}
					o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
				}
			} else {
				o.chaseTarget(target, now)
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
				o.chaseTarget(target, now)
			}
		case 3:
			if dist <= 1 && now-o.HitTick > o.AttackSpeed {
				o.HitTick = now
				for a_dy := -1; a_dy <= 1; a_dy++ {
					for a_dx := -1; a_dx <= 1; a_dx++ {
						tx, ty := o.CurrX+a_dx, o.CurrY+a_dy
						obj := o.envir.GetMovingObject(tx, ty)
						if obj == nil {
							continue
						}
						if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
							damage := o.calcMonsterDamage(p.BaseObject)
							o.applyMonsterDamageToPlayer(server, p, damage, now)
						}
					}
				}
				o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
				o.FocusTick = now
				o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
			} else if dist > 1 {
				o.chaseTarget(target, now)
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
				o.chaseTarget(target, now)
			}
		default:
			o.runExtendedAI(server, target, dist, now)
		}
	} else {
		if now-o.WalkTick > o.WalkSpeed {
			if rand.Intn(20) == 0 {
				o.WalkTick = now
				if rand.Intn(4) == 0 {
					o.TurnTo(rand.Intn(8))
				} else {
					if o.WalkTo(o.Dir) {
						o.SendRefMsg(RM_WALK, o.Dir, o.CurrX, o.CurrY, "")
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

func (o *MonsterObject) applyMonsterDamageToPlayer(server *netserver.TCPServer, target *PlayObject, damage int, now int64) {
	hp := int(target.WAbil.HP)
	hp -= damage
	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	if o.envir != nil {
		o.envir.broadcastRefMsg(target.BaseObject, RM_STRUCK, target.ID, target.CurrX, target.CurrY, o.Dir)
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

func (o *MonsterObject) chaseTarget(target *PlayObject, now int64) {
	if now-o.WalkTick < o.WalkSpeed {
		return
	}
	o.walkCount++
	if o.walkCount > o.WalkStep && o.WalkStep > 0 {
		if now-o.walkWaitTick < o.WalkWait {
			return
		}
		o.walkCount = 0
		o.walkWaitTick = now
	}
	o.WalkTick = now

	dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	if o.WalkTo(dir) {
		o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
		return
	}
	for i := 0; i < 7; i++ {
		altDir := (dir + i + 1) % 8
		if o.WalkTo(altDir) {
			o.SendRefMsg(RM_WALK, altDir, o.CurrX, o.CurrY, "")
			return
		}
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
	if rand.Intn(100) >= 80 {
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		return
	}
	damage := o.calcMonsterDamage(target.BaseObject)
	o.applyMonsterDamageToPlayer(server, target, damage, now)
	o.FocusTick = now
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

	vr := o.ViewRange
	if vr <= 0 {
		vr = 5
	}
	objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, vr)
	var best *PlayObject
	bestDist := 999999
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death {
			continue
		}
		if p.Hidden && rand.Intn(100) >= o.CoolEye {
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

func (o *MonsterObject) validateTarget(now int64, userEngine *UserEngine) {
	if o.TargetID == 0 {
		return
	}
	if o.FocusTick > 0 && now-o.FocusTick > 30000 {
		o.TargetID = 0
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
