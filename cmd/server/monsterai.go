package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
)

const (
	AIMelee     = 0
	AIRanged    = 1
	AIFlee      = 2
	AIArea      = 3
	AISummoner  = 4
	AIBurrow    = 5
	AIExplode   = 6
	AITeleport  = 7
	AIMagicCast = 8
	AIClone     = 9
	AIPoison    = 10
	AIGuard     = 11
)

func getExtendedAIBehavior(race byte) int {
	switch {
	case race >= 80 && race <= 82:
		return AIBurrow
	case race == 83:
		return AIExplode
	case race >= 84 && race <= 86:
		return AITeleport
	case race >= 87 && race <= 89:
		return AIMagicCast
	case race >= 90 && race <= 92:
		return AIClone
	case race >= 93 && race <= 95:
		return AIPoison
	case race >= 96 && race <= 98:
		return AIGuard
	default:
		return getAIBehavior(race)
	}
}

func (o *MonsterObject) runExtendedAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	switch o.AIBehavior {
	case AIBurrow:
		o.runBurrowAI(server, target, dist, now)
	case AIExplode:
		o.runExplodeAI(server, target, dist, now)
	case AITeleport:
		o.runTeleportAI(server, target, dist, now)
	case AIMagicCast:
		o.runMagicCastAI(server, target, dist, now)
	case AIClone:
		o.runCloneAI(server, target, dist, now)
	case AIPoison:
		o.runPoisonAI(server, target, dist, now)
	case AIGuard:
		o.runGuardAI(server, target, dist, now)
	default:
		o.runBaseAI(server, target, dist, now)
	}
}

func (o *MonsterObject) runBaseAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	switch {
	case dist <= 1:
		o.meleeAttack(server, target, now)
	default:
		o.chaseTarget(target)
	}
}

func (o *MonsterObject) runBurrowAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if o.Hidden {
		if dist <= 2 {
			o.Hidden = false
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s emerged from ground", o.Name)
		}
		return
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		if rand.Intn(10) == 0 {
			o.Hidden = true
			o.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
		}
	} else {
		o.chaseTarget(target)
		if rand.Intn(30) == 0 && dist > 4 {
			o.Hidden = true
			o.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
		}
	}
}

func (o *MonsterObject) runExplodeAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		if now-o.HitTick < o.AttackSpeed {
			return
		}
		o.HitTick = now
		damage := o.MaxHP / 2
		if damage < 50 {
			damage = 50
		}
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		o.envir.broadcastRefMsg(o.BaseObject, RM_DEATH, o.ID, o.CurrX, o.CurrY, 0)
		o.Death = true
		o.DeathTick = now
		o.WAbil.HP = 0
		log.Logf(log.LevelInfo, "Monster", "%s exploded for %d damage", o.Name, damage)
		return
	}
	o.chaseTarget(target)
}

func (o *MonsterObject) runTeleportAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if dist > 3 && now-o.WalkTick > o.WalkSpeed*2 {
		o.WalkTick = now
		tx := target.CurrX + rand.Intn(3) - 1
		ty := target.CurrY + rand.Intn(3) - 1
		if o.envir != nil && o.envir.CanWalk(tx, ty) {
			o.envir.RemoveObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)
			o.CurrX, o.CurrY = tx, ty
			o.envir.AddObject(tx, ty, OS_MOVINGOBJECT, o)
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s teleported to (%d,%d)", o.Name, tx, ty)
			return
		}
	}
	o.chaseTarget(target)
}

func (o *MonsterObject) runMagicCastAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if dist <= 6 && now-o.HitTick > o.AttackSpeed {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		damage := o.calcMonsterDamage(target.BaseObject)
		damage = damage * 3 / 2
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	if dist > 6 {
		o.chaseTarget(target)
	}
}

func (o *MonsterObject) runCloneAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target)
	}

	if int(o.WAbil.HP) < o.MaxHP/3 && o.minionCount < 2 && now-o.lastSummonTick > 20000 {
		o.lastSummonTick = now
		o.minionCount++
		if o.envir != nil && o.Engine != nil {
			cx := o.CurrX + rand.Intn(3) - 1
			cy := o.CurrY + rand.Intn(3) - 1
			if o.envir.CanWalk(cx, cy) {
				o.Engine.mu.Lock()
				id := o.Engine.nextMonsterID
				o.Engine.nextMonsterID++
				clone := NewMonsterObject(o.Name+"(分身)", id, o.Race, o.RaceImg, o.Appr, o.MaxHP/3, o.WalkSpeed, o.AttackSpeed, o.Exp/3)
				clone.CurrX = cx
				clone.CurrY = cy
				clone.MapName = o.MapName
				clone.envir = o.envir
				clone.WAbil.DC = o.WAbil.DC
				clone.WAbil.HP = clone.WAbil.MaxHP
				clone.TargetID = o.TargetID
				o.envir.AddObject(cx, cy, OS_MOVINGOBJECT, clone)
				o.Engine.Monsters = append(o.Engine.Monsters, clone)
				o.Engine.mu.Unlock()
				clone.SendRefMsg(RM_TURN, clone.Dir, cx, cy, clone.Name)
				log.Logf(log.LevelInfo, "Monster", "%s cloned at (%d,%d)", o.Name, cx, cy)
			}
		}
	}
}

func (o *MonsterObject) runPoisonAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		if now-o.HitTick < o.AttackSpeed {
			return
		}
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		damage := o.calcMonsterDamage(target.BaseObject)
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		if rand.Intn(3) == 0 {
			target.MakePoison(POISON_DECHEALTH, 80)
		}
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	if dist <= 4 && now-o.HitTick > o.AttackSpeed*2 {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		target.MakePoison(POISON_DECHEALTH, 60)
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	o.chaseTarget(target)
}

func (o *MonsterObject) runGuardAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if target.PkPoint < 200 {
		return
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else if dist <= 7 {
		o.chaseTarget(target)
	}
}

func (o *MonsterObject) InitEngine(engine *UserEngine) {
	o.Engine = engine
}

func (o *MonsterObject) ProcessDeath(server *netserver.TCPServer, now int64) {
	if !o.Death || o.LootDropped {
		return
	}
	if now-o.DeathTick > 3000 {
		o.LootDropped = true
	}
}

func (o *MonsterObject) RespawnCheck(now int64) bool {
	if !o.Death {
		return false
	}
	if now-o.DeathTick > 60000 {
		o.Death = false
		o.LootDropped = false
		o.WAbil.HP = uint16(o.MaxHP)
		o.TargetID = 0
		o.Hidden = false
		o.minionCount = 0
		o.CurrX = o.HomeX
		o.CurrY = o.HomeY
		if o.envir != nil {
			o.envir.AddObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)
		}
		return true
	}
	return false
}
