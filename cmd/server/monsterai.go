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
	AIPassive   = 12 // 被动型：不主动搜索（Race 51, 80）
	AIDualAxe   = 13 // 远程飞斧：7格连射（Race 87）
	AISplit     = 14 // 死亡分裂（Race 96）
	AIStone     = 15 // 石化伏击（Race 101）
	AILeech     = 16 // 闪电吸血（Race 200）
)

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
	case AIDualAxe:
		o.runDualAxeAI(server, target, dist, now)
	case AISplit:
		o.runSplitAI(server, target, dist, now)
	case AIStone:
		o.runStoneAI(server, target, dist, now)
	case AILeech:
		o.runLeechAI(server, target, dist, now)
	default:
		o.runBaseAI(server, target, dist, now)
	}
}

func (o *MonsterObject) runBaseAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	switch {
	case dist <= 1:
		o.meleeAttack(server, target, now)
	default:
		o.chaseTarget(target, now)
	}
}

func (o *MonsterObject) runBurrowAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	// Delphi TStickMonster/TDigOutZombi: 使用 FixedHide（m_boFixedHideMode）
	if o.FixedHide {
		// 潜地中：目标进入 3 格内出现（nComeOutValue）
		if dist <= 3 {
			o.FixedHide = false
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s emerged from ground", o.Name)
		}
		return
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		// 10% 概率回潜
		if rand.Intn(10) == 0 {
			o.FixedHide = true
			o.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
		}
	} else {
		o.chaseTarget(target, now)
		// 远距离低概率回潜
		if rand.Intn(30) == 0 && dist > 4 {
			o.FixedHide = true
			o.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
		}
	}
}

func (o *MonsterObject) runExplodeAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	// Delphi: 60秒自毁计时器（ObjMon2.pas:804-814）
	if o.spawnTick > 0 && now-o.spawnTick > 60000 {
		o.explode(server, target, now)
		return
	}
	if dist <= 1 {
		if now-o.HitTick < o.AttackSpeed {
			return
		}
		o.explode(server, target, now)
		return
	}
	o.chaseTarget(target, now)
}

func (o *MonsterObject) explode(server *netserver.TCPServer, target *PlayObject, now int64) {
	o.HitTick = now
	damage := o.MaxHP / 2
	if damage < 50 {
		damage = 50
	}
	// 对 1 格内所有玩家造成伤害
	if o.envir != nil {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				obj := o.envir.GetMovingObject(o.CurrX+dx, o.CurrY+dy)
				if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
					o.applyMonsterDamageToPlayer(server, p, damage, now)
				}
			}
		}
	}
	o.envir.broadcastDeathMsg(o.BaseObject, o.ID, o.CurrX, o.CurrY, o.Dir, true)
	o.Death = true
	o.DeathTick = now
	o.WAbil.HP = 0
	log.Logf(log.LevelInfo, "Monster", "%s exploded for %d damage", o.Name, damage)
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
	o.chaseTarget(target, now)
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
		o.chaseTarget(target, now)
	}
}

func (o *MonsterObject) runCloneAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target, now)
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
	o.chaseTarget(target, now)
}

func (o *MonsterObject) runGuardAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if target.PkPoint < 200 {
		return
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else if dist <= 7 {
		o.chaseTarget(target, now)
	}
}

func (o *MonsterObject) InitEngine(engine *UserEngine) {
	o.Engine = engine
}

// runDualAxeAI — TDualAxeMonster (Race 87): 7格远程飞斧，连射机制
// Delphi ObjAxeMon.pas:65-100
func (o *MonsterObject) runDualAxeAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if dist <= 7 {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			// 连射：burstCount 次快速攻击，之后 1/5 概率重置
			o.burstCount++
			if o.burstCount >= 2 {
				if rand.Intn(5) == 0 {
					o.burstCount = 0
				}
				o.HitTick = now + o.AttackSpeed // 连射结束后正常冷却
			}
			spd := target.SpeedPoint
			if spd < 1 {
				spd = 1
			}
			if rand.Intn(spd) < o.HitPoint {
				damage := o.calcMonsterDamage(target.BaseObject)
				o.applyMonsterDamageToPlayer(server, target, damage, now)
				o.FocusTick = now
			}
			o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		}
		return
	}
	o.chaseTarget(target, now)
}

// runSplitAI — TZilKinZombi (Race 96): 正常近战 + 死亡分裂
// Delphi ObjMon.pas:147-158
func (o *MonsterObject) runSplitAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target, now)
	}
}

// runStoneAI — TScultureMonster (Race 101): 石化伏击
// 初始石化，玩家进入视野解除石化并攻击
func (o *MonsterObject) runStoneAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target, now)
	}
}

// runLeechAI — TElectronicScolpionMon (Race 200): 闪电吸血
// Delphi ObjMon.pas:1842-1881: 2格内闪电攻击，吸血 damage/btGetBackHP
func (o *MonsterObject) runLeechAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 2 {
		if now-o.HitTick < o.AttackSpeed {
			return
		}
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		damage := o.calcMonsterDamage(target.BaseObject)
		// HP < 50% 时魔法模式：伤害增加
		if int(o.WAbil.HP) < o.MaxHP/2 {
			damage = damage * 3 / 2
		}
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		// 吸血：回复 damage/5 HP
		leech := damage / 5
		if leech > 0 {
			hp := int(o.WAbil.HP) + leech
			if hp > o.MaxHP {
				hp = o.MaxHP
			}
			o.WAbil.HP = uint16(hp)
		}
		o.FocusTick = now
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		return
	}
	o.chaseTarget(target, now)
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
