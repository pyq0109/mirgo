package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
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
	AICritical  = 17 // 远程双暴击（Race 130）
	AIFireball  = 18 // 远程火球（Race 215）
	AISpit      = 19 // 锥形喷吐（Race 82/118/119）
	AISpawnHive = 20 // 固定召唤巢穴（Race 103 蜂王 / 116 蜘蛛巢）
	AICentiKing = 21 // 蜈蚣王潜地毒 AoE（Race 107）
	AICowKing          = 22 // 牛魔王 AoE + 狂暴阶段（Race 92）
	AIPulse            = 23 // 固定远程脉冲（Race 115 触角神）
	AILightning        = 24 // 线性闪电（Race 94 TLightingZombi）
	AIFireAura         = 25 // 火焰光环（TFireMonster）
	AITransform        = 26 // 变形（Race 113 TElfMonster）
	AILevelingSkeleton = 27 // 升级骷髅（Race 100 TWhiteSkeleton）
	AIBoneKing         = 28 // 骷髅王召唤（TBoneKingMonster）
	AIGreenPoison      = 29 // 绿毒触碰（Race 208 TGreenMonster）
	AIRedPoison        = 30 // 红毒触碰（Race 209 TRedMonster）
	AIKhazard          = 31 // 对角2格拖拽+绿毒（Race 206 TKhazard）
	AIFrostTiger       = 32 // 隐身伏击（Race 210 TFrostTiger）
	AITrainer          = 33 // 训练师沙袋（Race 55 TRAINER）
	AISoccerBall       = 34 // 足球（Race 120 TSoccerBall）：可推动物体，被踢滚动反弹（ObjMon2.pas:303-366）
)

func (o *MonsterObject) runExtendedAI(server *netserver.TCPServer, e *UserEngine, target *PlayObject, dist int, now int64) {
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
		o.runCloneAI(server, e, target, dist, now)
	case AIPoison:
		o.runPoisonAI(server, target, dist, now)
	case AIDualAxe:
		o.runDualAxeAI(server, target, dist, now)
	case AISplit:
		o.runSplitAI(server, target, dist, now)
	case AIStone:
		o.runStoneAI(server, target, dist, now)
	case AILeech:
		o.runLeechAI(server, target, dist, now)
	case AICritical:
		o.runCriticalAI(server, target, dist, now)
	case AIFireball:
		o.runFireballAI(server, target, dist, now)
	case AISpit:
		o.runSpitAI(server, target, dist, now)
	case AISpawnHive:
		o.runSpawnHiveAI(server, e, target, now)
	case AICentiKing:
		o.runCentiKingAI(server, target, dist, now)
	case AICowKing:
		o.runCowKingAI(server, target, dist, now)
	case AIPulse:
		o.runPulseAI(server, now)
	case AILightning:
		o.runLightningAI(server, target, dist, now)
	case AIFireAura:
		o.runFireAuraAI(server, target, dist, now)
	case AITransform:
		o.runTransformAI(server, target, dist, now)
	case AILevelingSkeleton:
		o.runLevelingSkeletonAI(server, target, dist, now)
	case AIBoneKing:
		o.runBoneKingAI(server, e, target, dist, now)
	case AIGreenPoison:
		o.runTouchPoisonAI(server, target, dist, now, POISON_DECHEALTH)
	case AIRedPoison:
		o.runTouchPoisonAI(server, target, dist, now, POISON_DAMAGEARMOR)
	case AIKhazard:
		o.runKhazardAI(server, target, dist, now)
	case AIFrostTiger:
		o.runFrostTigerAI(server, target, dist, now)
	case AITrainer:
		// Delphi TTrainer：不搜索不攻击，仅累计伤害（ObjNpc.pas:2642-2676）
		o.checkTrainingReport(server, now)
	default:
		o.runBaseAI(server, target, dist, now)
	}
}

func (o *MonsterObject) runBaseAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	switch {
	case dist <= 1:
		o.meleeAttack(server, target, now)
	default:
		o.chaseTarget(target.BaseObject, now)
	}
}

func (o *MonsterObject) runBurrowAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	burrowTriggerDist := 3
	reburrowDist := 4
	if o.engine != nil && o.engine.Config != nil {
		burrowTriggerDist = o.engine.Config.GetBurrowTriggerDist()
		reburrowDist = o.engine.Config.GetReburrowDist()
	}
	// Delphi TStickMonster/TDigOutZombi: 使用 FixedHide（m_boFixedHideMode）
	if o.FixedHide {
		// 潜地中：目标进入 4 格内出现（Delphi nComeOutValue=4，|dx|<4 && |dy|<4）
		if dist <= burrowTriggerDist {
			o.FixedHide = false
			o.SendRefMsg(RM_DIGUP, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s emerged from underground", o.Name)
		} else if !o.StickMode && now-o.WalkTick >= o.WalkSpeed {
			// Delphi TDigOutZombi: 移动型可地下静默接近目标
			o.WalkTick = now
			dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			o.WalkTo(dir) // 无广播
		}
		return
	}
	if o.StickMode {
		// Delphi TStickMonster（ObjMon2.pas:252）：固定伏击，不追击；
		// 目标超出 nAttackRange=4 → ComeDown 回潜
		if dist <= 1 {
			o.meleeAttack(server, target, now)
		} else if dist > reburrowDist {
			o.FixedHide = true
			o.TargetID = 0
			o.SendRefMsg(RM_DIGDOWN, 0, o.CurrX, o.CurrY, "")
		}
		return
	}
	// 移动型潜地（TDigOutZombi/TElfWarriorMonster/TSandMobObject）：可追击
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		// 10% 概率回潜
		if rand.Intn(10) == 0 {
			o.FixedHide = true
			o.SendRefMsg(RM_DIGDOWN, 0, o.CurrX, o.CurrY, "")
		}
	} else {
		o.chaseTarget(target.BaseObject, now)
		// 远距离低概率回潜
		if rand.Intn(30) == 0 && dist > reburrowDist {
			o.FixedHide = true
			o.SendRefMsg(RM_DIGDOWN, 0, o.CurrX, o.CurrY, "")
		}
	}
}

func (o *MonsterObject) runExplodeAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	explodeTimer := int64(60000)
	if o.engine != nil && o.engine.Config != nil {
		explodeTimer = o.engine.Config.GetExplodeTimer()
	}
	// Delphi: 60秒自毁计时器（ObjMon2.pas:804-814）
	if o.spawnTick > 0 && now-o.spawnTick > explodeTimer {
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
	o.chaseTarget(target.BaseObject, now)
}

func (o *MonsterObject) explode(server *netserver.TCPServer, target *PlayObject, now int64) {
	if o.envir == nil {
		return
	}
	o.HitTick = now
	// Delphi: 50% 物理 + 50% 魔法（GetHitStruckDamage + GetMagStruckDamage）
	explodePowerDiv := 2
	explodePowerMin := 50
	if o.engine != nil && o.engine.Config != nil {
		explodePowerDiv = o.engine.Config.GetExplodePowerDiv()
		explodePowerMin = o.engine.Config.GetExplodePowerMin()
	}
	power := o.MaxHP / explodePowerDiv
	if power < explodePowerMin {
		power = explodePowerMin
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			obj := o.envir.GetMovingObject(o.CurrX+dx, o.CurrY+dy)
			if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
				damage := halfPhysHalfMag(p.BaseObject, power)
				o.applyMonsterDamageToPlayer(server, p, damage, now)
			}
		}
	}
	o.envir.broadcastDeathMsg(o.BaseObject, o.ID, o.CurrX, o.CurrY, o.Dir, true)
	o.Death = true
	o.DeathTick = now
	o.WAbil.HP = 0
	log.Logf(log.LevelInfo, "Monster", "%s self-destructed", o.Name)
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
		// Delphi SpaceMove：仅校验地形；占用重叠由 dup mode 驱散。
		if o.envir != nil && o.envir.CanWalkEx(tx, ty, true) {
			o.envir.RemoveObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)
			o.CurrX, o.CurrY = tx, ty
			o.envir.AddObject(tx, ty, OS_MOVINGOBJECT, o)
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s teleported to (%d,%d)", o.Name, tx, ty)
			return
		}
	}
	o.chaseTarget(target.BaseObject, now)
}

func (o *MonsterObject) runMagicCastAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if o.StatusTimeArr[POISON_LOCKSPELL] > 0 {
		o.chaseTarget(target.BaseObject, now)
		return
	}
	if dist <= 6 && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) && now-o.HitTick > o.AttackSpeed {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		damage := o.calcMagicCastDamage(target.BaseObject)
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		o.FocusTick = now
		return
	}
	if dist > 6 || !o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		o.chaseTarget(target.BaseObject, now)
	}
}

// calcMagicCastDamage uses the monster's MagID to look up power from magic_db;
// falls back to MC-based damage if no magic is configured.
func (o *MonsterObject) calcMagicCastDamage(target *BaseObject) int {
	if o.MagID > 0 && o.engine != nil && o.engine.MagicDB != nil {
		if def := o.engine.MagicDB.GetByID(o.MagID); def != nil {
			power := def.Power
			if def.MaxPower > def.Power {
				power = def.Power + rand.Intn(def.MaxPower-def.Power+1)
			}
			if power < 1 {
				power = 1
			}
			loMAC := int(target.WAbil.MAC & 0xFFFF)
			hiMAC := int(target.WAbil.MAC >> 16)
			antiMagic := loMAC
			if hiMAC > loMAC {
				antiMagic = loMAC + rand.Intn(hiMAC-loMAC+1)
			}
			damage := power - antiMagic
			if damage < 1 {
				damage = 1
			}
			return damage
		}
	}
	return o.calcMonsterMagicDamage(target)
}

func (o *MonsterObject) runCloneAI(server *netserver.TCPServer, e *UserEngine, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}

	if e == nil || o.envir == nil {
		return
	}
	cloneThreshold := 3
	cloneCooldown := int64(20000)
	if e.Config != nil {
		cloneThreshold = e.Config.GetCloneThreshold()
		cloneCooldown = e.Config.GetCloneCooldown()
	}
	if int(o.WAbil.HP) < o.MaxHP/cloneThreshold && e.countLiveChildren(o.ID) < 2 && now-o.lastSummonTick > cloneCooldown {
		o.lastSummonTick = now
		cx := o.CurrX + rand.Intn(3) - 1
		cy := o.CurrY + rand.Intn(3) - 1
		if clone := e.spawnChild(o, "", cx, cy, now); clone != nil {
			clone.MaxHP = o.MaxHP / cloneThreshold
			clone.WAbil.MaxHP = uint16(clone.MaxHP)
			clone.WAbil.HP = uint16(clone.MaxHP)
			log.Logf(log.LevelInfo, "Monster", "%s spawned clone at (%d,%d)", o.Name, cx, cy)
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
			target.MakePoison(POISON_DECHEALTH, 80, 3)
		}
		if rand.Intn(4) == 0 {
			target.MakePoison(POISON_DAMAGEARMOR, 80, 0)
		}
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	if dist <= 4 && o.StatusTimeArr[POISON_LOCKSPELL] <= 0 && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) && now-o.HitTick > o.AttackSpeed*2 {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		target.MakePoison(POISON_DECHEALTH, 60, 3)
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runGuardAI — Delphi TArcherGuard (ObjMon2.pas:887-997)：固定炮台，永不移动。
// 目标规则（无攻城战版本）：30 秒内的攻击者，或 PkPoint >= 200 的红名玩家
// （== Delphi PKLevel >= 2）；视野 12 格内远程物理攻击。
func (o *MonsterObject) runGuardAI(server *netserver.TCPServer, now int64) {
	if o.envir == nil {
		return
	}
	vr := o.ViewRange
	if vr <= 0 {
		vr = 12
	}
	vr += o.CoolEye
	var best *PlayObject
	bestDist := 999999
	for _, obj := range o.envir.GetRangeObjects(o.CurrX, o.CurrY, vr) {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death {
			continue
		}
		if p.Hidden && o.CoolEye <= int(p.WAbil.Level) && rand.Intn(100) >= o.CoolEye {
			continue
		}
		isLastHiter := p.ID == o.LastHiterID && now-o.LastHiterTick < 30000
		if !isLastHiter && p.PkPoint < 200 {
			continue
		}
		d := abs(p.CurrX-o.CurrX) + abs(p.CurrY-o.CurrY)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	if best == nil {
		o.TargetID = 0
		return
	}
	o.TargetID = best.ID
	if now-o.HitTick > o.AttackSpeed && bestDist <= 12 {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, best.CurrX, best.CurrY)
		damage := o.calcMonsterDamage(best.BaseObject)
		o.applyMonsterDamageToPlayer(server, best, damage, now)
		o.FocusTick = now
		// Delphi 发 RM_FLYAXE(10202)，客户端尚未实现 → 用 RM_HIT 代替
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
	}
}

// runDualAxeAI — TDualAxeMonster (Race 87): 7格远程飞斧，连射机制
// Delphi ObjAxeMon.pas:65-100, FlyAxeAttack 延迟 = max(dx,dy)*50+600ms
func (o *MonsterObject) runDualAxeAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	// 结算延迟伤害
	if o.pendingAxeDmg > 0 && now >= o.pendingAxeTick {
		if o.engine != nil {
			if t := o.engine.GetPlayer(o.pendingAxeTarget); t != nil && !t.Death && !t.Ghost {
				o.applyMonsterDamageToPlayer(server, t, o.pendingAxeDmg, now)
			}
		}
		o.pendingAxeDmg = 0
		o.pendingAxeTarget = 0
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	dualAxeRange := 7
	if o.engine != nil && o.engine.Config != nil {
		dualAxeRange = o.engine.Config.GetDualAxeRange()
	}
	if dist <= dualAxeRange && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			o.burstCount++
			if o.burstCount >= 2 {
				if rand.Intn(5) == 0 {
					o.burstCount = 0
				}
				o.HitTick = now + o.AttackSpeed
			}
			spd := target.SpeedPoint
			if spd < 1 {
				spd = 1
			}
			if rand.Intn(spd) < o.HitPoint {
				damage := o.calcMonsterDamage(target.BaseObject)
				// Delphi FlyAxeAttack: 伤害延迟与距离成正比
				maxD := abs(target.CurrX - o.CurrX)
				if d2 := abs(target.CurrY - o.CurrY); d2 > maxD {
					maxD = d2
				}
				o.pendingAxeDmg = damage
				o.pendingAxeTarget = target.ID
				o.pendingAxeTick = now + int64(maxD)*50 + 600
				o.FocusTick = now
			}
			o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		}
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runSplitAI — TZilKinZombi (Race 96): 正常近战 + 死亡分裂
// Delphi ObjMon.pas:147-158
func (o *MonsterObject) runSplitAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runStoneAI — TScultureMonster (Race 101): 石化伏击
// 初始石化，玩家进入视野解除石化并攻击
func (o *MonsterObject) runStoneAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runLeechAI — TElectronicScolpionMon (Race 200): 闪电吸血
// Delphi ObjMon.pas:1842-1881: 2格内闪电攻击，MC 伤害，吸血 damage/btGetBackHP
func (o *MonsterObject) runLeechAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if o.StatusTimeArr[POISON_LOCKSPELL] > 0 {
		o.chaseTarget(target.BaseObject, now)
		return
	}
	leechRange := 2
	leechBoostRatio := 150
	if o.engine != nil && o.engine.Config != nil {
		leechRange = o.engine.Config.GetLeechRange()
		leechBoostRatio = o.engine.Config.GetLeechBoostRatio()
	}
	if dist <= leechRange && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		if now-o.HitTick < o.AttackSpeed {
			return
		}
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		damage := o.calcMonsterMagicDamage(target.BaseObject)
		if int(o.WAbil.HP) < o.MaxHP/2 {
			damage = damage * leechBoostRatio / 100
		}
		o.applyMonsterDamageToPlayer(server, target, damage, now)
		divisor := o.leechDivisor
		if divisor < 1 {
			divisor = 5
		}
		leech := damage / divisor
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
	o.chaseTarget(target.BaseObject, now)
}

// runCriticalAI — TDoubleCriticalMonster (Race 130): 远程攻击，1/4 概率双倍伤害
// Delphi ObjMon.pas:233
func (o *MonsterObject) runCriticalAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	critRange := 7
	critChance := 4
	if o.engine != nil && o.engine.Config != nil {
		critRange = o.engine.Config.GetCritRange()
		critChance = o.engine.Config.GetCritChance()
	}
	if dist <= critRange && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			spd := target.SpeedPoint
			if spd < 1 {
				spd = 1
			}
			if rand.Intn(spd) < o.HitPoint {
				damage := o.calcMonsterDamage(target.BaseObject)
				if rand.Intn(critChance) == 0 {
					damage *= 2
				}
				o.applyMonsterDamageToPlayer(server, target, damage, now)
				o.FocusTick = now
			}
			o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		}
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runFireballAI — TFireBallMonster (Race 215): 8格火球，MC 伤害
// Delphi ObjMon3.pas:984-1046
func (o *MonsterObject) runFireballAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if o.StatusTimeArr[POISON_LOCKSPELL] > 0 {
		o.chaseTarget(target.BaseObject, now)
		return
	}
	fireballRange := 8
	if o.engine != nil && o.engine.Config != nil {
		fireballRange = o.engine.Config.GetFireballRange()
	}
	if dist <= fireballRange && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			// MC 伤害，MAC 减伤（Delphi GetMagStruckDamage）
			loMC := int(o.WAbil.MC & 0xFFFF)
			hiMC := int(o.WAbil.MC >> 16)
			damage := loMC
			if hiMC > loMC {
				damage = loMC + rand.Intn(hiMC-loMC+1)
			}
			loMAC := int(target.WAbil.MAC & 0xFFFF)
			hiMAC := int(target.WAbil.MAC >> 16)
			antiMagic := loMAC
			if hiMAC > loMAC {
				antiMagic = loMAC + rand.Intn(hiMAC-loMAC+1)
			}
			damage -= antiMagic
			if damage < 1 {
				damage = 1
			}
			o.applyMonsterDamageToPlayer(server, target, damage, now)
			o.FocusTick = now
			o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		}
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runSpitAI — TSpitSpider (Race 82): 2格锥形喷吐，可附带绿毒
// Delphi ObjMon.pas:719-745, TargetInSpitRange (ObjBase.pas:18504-18530)
func (o *MonsterObject) runSpitAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	spitRange := 2
	if o.engine != nil && o.engine.Config != nil {
		spitRange = o.engine.Config.GetSpitRange()
	}
	if dist <= spitRange && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			// 锥形范围伤害：对目标及目标相邻格内所有玩家造成伤害
			if o.envir != nil {
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						tx, ty := target.CurrX+dx, target.CurrY+dy
						// 限制在 2 格范围内
						if abs(tx-o.CurrX) > 2 || abs(ty-o.CurrY) > 2 {
							continue
						}
						obj := o.envir.GetMovingObject(tx, ty)
						if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
							damage := o.calcMonsterDamage(p.BaseObject)
							o.applyMonsterDamageToPlayer(server, p, damage, now)
							// Delphi: THighRiskSpider(118) 为非毒变种
							if o.spitPoison && rand.Intn(3) == 0 {
								p.MakePoison(POISON_DECHEALTH, 60, 3)
							}
						}
					}
				}
			}
			o.FocusTick = now
			o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		}
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runSpawnHiveAI — Delphi TBeeQueen/TSpiderHouseMonster (ObjMon2.pas:368/646)：
// 固定不动、自身不攻击；有目标时每隔 AttackSpeed 尝试生成 1 只子体（存活上限 15）。
// 蜂王在自身格生成；蜘蛛巢只在 (x, y+1) 可走时生成，否则跳过。
func (o *MonsterObject) runSpawnHiveAI(server *netserver.TCPServer, e *UserEngine, target *PlayObject, now int64) {
	if now-o.HitTick <= o.AttackSpeed {
		return
	}
	o.HitTick = now
	o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
	if e == nil || o.envir == nil {
		return
	}
	hiveMaxChildren := 15
	if e.Config != nil {
		hiveMaxChildren = e.Config.GetHiveMaxChildren()
	}
	if e.countLiveChildren(o.ID) >= hiveMaxChildren {
		return
	}
	x, y := o.CurrX, o.CurrY
	name := "小蜜蜂"
	if o.Race == 116 {
		y++ // 蜘蛛巢生成在下方一格（地图坐标）
		name = "小蜘蛛"
		if !o.envir.CanWalk(x, y) {
			return
		}
	}
	e.spawnChild(o, name, x, y, now)
}

// runCentiKingAI — Delphi TCentipedeKingMonster (ObjMon2.pas:153-582)：
// 潜地 → 目标进入 4 格且距上次动作 >10s → 出土（回满血）→ 每 3s+ 对 6 格内
// 所有玩家魔法 AoE + 25% 中毒 → 10s 无法攻击则回潜。
func (o *MonsterObject) runCentiKingAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	centipedeCooldown := int64(10000)
	centipedeAoEInterval := int64(3000)
	if o.engine != nil && o.engine.Config != nil {
		centipedeCooldown = o.engine.Config.GetCentipedeCooldown()
		centipedeAoEInterval = o.engine.Config.GetCentipedeAoEInterval()
	}
	if o.FixedHide {
		// 潜地中：|dx|<4 && |dy|<4（切比雪夫 ≤3）且距上次动作超过 10 秒 → 出土
		if now-o.attickTick > centipedeCooldown && dist <= 3 {
			o.FixedHide = false
			o.WAbil.HP = uint16(o.MaxHP) // Delphi ComeOut: HP 回满
			o.attickTick = now
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s emerged from underground", o.Name)
		}
		return
	}
	if o.envir != nil && now-o.attickTick > centipedeAoEInterval && dist <= 6 {
		o.attickTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		// Delphi: |dx| < 6 && |dy| < 6（严格小于），魔法减伤路径
		for dy := -5; dy <= 5; dy++ {
			for dx := -5; dx <= 5; dx++ {
				obj := o.envir.GetMovingObject(o.CurrX+dx, o.CurrY+dy)
				p, ok := obj.(*PlayObject)
				if !ok || p.Death || p.Ghost {
					continue
				}
				damage := o.calcMonsterMagicDamage(p.BaseObject)
				o.applyMonsterDamageToPlayer(server, p, damage, now)
				if rand.Intn(4) == 0 {
					if rand.Intn(3) != 0 {
						p.MakePoison(POISON_DECHEALTH, 60, 3)
					} else {
						p.MakePoison(POISON_STONE, 5, 0)
					}
				}
			}
		}
		o.FocusTick = now
		return
	}
	// 出土后 10 秒未能攻击 → 回潜
	if now-o.attickTick > centipedeCooldown {
		o.FixedHide = true
		o.TargetID = 0
		o.attickTick = now
		o.SendRefMsg(RM_DIGDOWN, 0, o.CurrX, o.CurrY, "")
	}
}

// runCowKingAI — Delphi TCowKingMonster：相邻目标点 AoE（半物理+半魔法）+
// 血量阶段：HP 每跌破一个 1/7 档（档 ≥ 2）→ 停滞 8 秒（停攻）→ 狂暴 8 秒
// （AttackSpeed=500, WalkSpeed=400）→ 恢复。
func (o *MonsterObject) runCowKingAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	cowKingStunSpeed := int64(10000)
	cowKingRageDuration := int64(8000)
	cowKingBerserkAtk := int64(500)
	cowKingBerserkWalk := int64(400)
	if o.engine != nil && o.engine.Config != nil {
		cowKingStunSpeed = o.engine.Config.GetCowKingStunSpeed()
		cowKingRageDuration = o.engine.Config.GetCowKingRageDuration()
		cowKingBerserkAtk = o.engine.Config.GetCowKingBerserkAtk()
		cowKingBerserkWalk = o.engine.Config.GetCowKingBerserkWalk()
	}
	if o.MaxHP > 7 {
		bracket := 7 - int(o.WAbil.HP)*7/o.MaxHP
		if bracket >= 2 && bracket != o.hpBracket {
			o.hpBracket = bracket
			o.rageState = 1
			o.rageTick = now
			o.saveAttackSpeed = o.AttackSpeed
			o.saveWalkSpeed = o.WalkSpeed
			o.AttackSpeed = cowKingStunSpeed
		}
	}
	switch o.rageState {
	case 1: // 停滞 → 狂暴
		if now-o.rageTick > cowKingRageDuration {
			o.rageState = 2
			o.rageTick = now
			o.AttackSpeed = cowKingBerserkAtk
			o.WalkSpeed = cowKingBerserkWalk
		}
	case 2: // 狂暴 → 恢复
		if now-o.rageTick > cowKingRageDuration {
			o.rageState = 0
			o.AttackSpeed = o.saveAttackSpeed
			o.WalkSpeed = o.saveWalkSpeed
		}
	}

	if o.envir != nil && dist <= 1 && now-o.HitTick > o.AttackSpeed {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		// Delphi HitMagAttackTarget: DC 掷骰，目标点 ±1 格内半物理+半魔法
		loDC := int(o.WAbil.DC & 0xFFFF)
		hiDC := int(o.WAbil.DC >> 16)
		power := loDC
		if hiDC > loDC {
			power = loDC + rand.Intn(hiDC-loDC+1)
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				obj := o.envir.GetMovingObject(target.CurrX+dx, target.CurrY+dy)
				p, ok := obj.(*PlayObject)
				if !ok || p.Death || p.Ghost {
					continue
				}
				damage := halfPhysHalfMag(p.BaseObject, power)
				o.applyMonsterDamageToPlayer(server, p, damage, now)
			}
		}
		o.FocusTick = now
		return
	}
	if dist > 1 {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runPulseAI — Delphi TBigHeartMonster (ObjMon2.pas:585)：固定不动、不搜索追击，
// 每 AttackSpeed 对 16 格内所有玩家施加魔法伤害。
func (o *MonsterObject) runPulseAI(server *netserver.TCPServer, now int64) {
	if o.envir == nil || now-o.HitTick <= o.AttackSpeed {
		return
	}
	pulseRange := 16
	if o.engine != nil && o.engine.Config != nil {
		pulseRange = o.engine.Config.GetPulseRange()
	}
	o.HitTick = now
	o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
	for dy := -pulseRange; dy <= pulseRange; dy++ {
		for dx := -pulseRange; dx <= pulseRange; dx++ {
			obj := o.envir.GetMovingObject(o.CurrX+dx, o.CurrY+dy)
			p, ok := obj.(*PlayObject)
			if !ok || p.Death || p.Ghost {
				continue
			}
			damage := o.calcMonsterMagicDamage(p.BaseObject)
			o.applyMonsterDamageToPlayer(server, p, damage, now)
		}
	}
}

// runLightningAI — TLightingZombi (Race 94): 线性闪电，穿透一条线上所有目标
// Delphi: 远程闪电，沿方向命中所有玩家
func (o *MonsterObject) runLightningAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
		return
	}
	if o.StatusTimeArr[POISON_LOCKSPELL] > 0 {
		o.chaseTarget(target.BaseObject, now)
		return
	}
	lightningRange := 8
	if o.engine != nil && o.engine.Config != nil {
		lightningRange = o.engine.Config.GetLightningRange()
	}
	if dist <= lightningRange && now-o.HitTick > o.AttackSpeed {
		o.HitTick = now
		o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
		dx, dy := dirToOffset(o.Dir)
		for i := 1; i <= lightningRange; i++ {
			lx, ly := o.CurrX+dx*i, o.CurrY+dy*i
			if o.envir == nil {
				break
			}
			if !o.envir.CanWalkEx(lx, ly, true) {
				break
			}
			obj := o.envir.GetMovingObject(lx, ly)
			if p, ok := obj.(*PlayObject); ok && !p.Death && !p.Ghost {
				damage := o.calcMonsterMagicDamage(p.BaseObject)
				o.applyMonsterDamageToPlayer(server, p, damage, now)
			}
		}
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		o.FocusTick = now
		return
	}
	o.chaseTarget(target.BaseObject, now)
}

// runFireAuraAI — TFireMonster: 火焰光环，十字形 9 格地图事件（20s, 10 dmg/tick）
// Delphi ObjMon3.pas:1064-1143: 持续区域封锁，非定向攻击
func (o *MonsterObject) runFireAuraAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	fireAuraDuration := int64(20000)
	if o.engine != nil && o.engine.Config != nil {
		fireAuraDuration = o.engine.Config.GetFireAuraDuration()
	}
	if o.envir != nil && now-o.lastAuraTick > o.AttackSpeed {
		o.lastAuraTick = now
		o.SendRefMsg(RM_SPELL, o.Dir, o.CurrX, o.CurrY, "")
		offsets := [][2]int{{0, 0}, {0, -1}, {0, -2}, {0, 1}, {0, 2}, {-1, 0}, {-2, 0}, {1, 0}, {2, 0}}
		for _, pos := range offsets {
			o.envir.AddFireEvent(server, o.CurrX+pos[0], o.CurrY+pos[1], 10, fireAuraDuration, o.ID)
		}
	}
	// 有目标时仍近战/追击
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runTransformAI — TElfMonster (Race 113): 双形态切换
// Delphi ObjMon.pas:207: HP < 50% 切换形态，改变外观和属性
func (o *MonsterObject) runTransformAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	transformCooldown := int64(5000)
	if o.engine != nil && o.engine.Config != nil {
		transformCooldown = o.engine.Config.GetTransformCooldown()
	}
	shouldBeForm := 0
	if int(o.WAbil.HP) < o.MaxHP/2 {
		shouldBeForm = 1
	}
	if o.transformForm != shouldBeForm && now-o.transformTick > transformCooldown {
		o.transformForm = shouldBeForm
		o.transformTick = now
		if shouldBeForm == 1 {
			o.saveDC, o.saveAC, o.saveMC = o.WAbil.DC, o.WAbil.AC, o.WAbil.MC
			o.WAbil.DC = o.WAbil.MC
			o.WAbil.AC = o.WAbil.AC * 3 / 2
			o.Appr = o.transformAppr2
		} else {
			o.WAbil.DC, o.WAbil.AC, o.WAbil.MC = o.saveDC, o.saveAC, o.saveMC
			o.Appr = o.transformAppr2 - 1
		}
		o.SendRefMsg(RM_FEATURECHANGED, int(o.Feature()), o.CurrX, o.CurrY, "")
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runLevelingSkeletonAI — TWhiteSkeleton (Race 100): 道士召唤的升级骷髅
// Delphi ObjMon.pas:159: 通过战斗获得经验升级，属性增长
func (o *MonsterObject) runLevelingSkeletonAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// gainPetXP — 升级骷髅经验获取与升级
func (o *MonsterObject) gainPetXP(amount int) {
	if amount < 1 {
		return
	}
	o.petXP += amount
	if o.petXP >= o.petMaxXP {
		o.petXP -= o.petMaxXP
		o.petLevel++
		o.petMaxXP = o.petMaxXP * 3 / 2
		loDC := int(o.WAbil.DC&0xFFFF) + 2
		hiDC := int(o.WAbil.DC>>16) + 3
		o.WAbil.DC = uint32(loDC) | uint32(hiDC)<<16
		o.MaxHP += 20
		o.WAbil.MaxHP = uint16(o.MaxHP)
		o.WAbil.HP = uint16(o.MaxHP)
		o.SendRefMsg(RM_FEATURECHANGED, int(o.Feature()), o.CurrX, o.CurrY, "")
		log.Logf(log.LevelInfo, "Monster", "%s leveled up to %d", o.Name, o.petLevel)
	}
}

// runBoneKingAI — TBoneKingMonster: 骷髅王召唤
// Delphi ObjMon3.pas:54: 周期召唤骷髅战士
func (o *MonsterObject) runBoneKingAI(server *netserver.TCPServer, e *UserEngine, target *PlayObject, dist int, now int64) {
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
	if e == nil || o.envir == nil {
		return
	}
	boneKingCooldown := int64(15000)
	boneKingMaxChildren := 8
	if e.Config != nil {
		boneKingCooldown = e.Config.GetBoneKingCooldown()
		boneKingMaxChildren = e.Config.GetBoneKingMaxChildren()
	}
	if now-o.lastSummonTick > boneKingCooldown && e.countLiveChildren(o.ID) < boneKingMaxChildren {
		o.lastSummonTick = now
		names := []string{"骷髅战士", "骷髅精灵"}
		for i := 0; i < 3; i++ {
			x := o.CurrX + rand.Intn(3) - 1
			y := o.CurrY + rand.Intn(3) - 1
			e.spawnChild(o, names[rand.Intn(len(names))], x, y, now)
		}
	}
}

// halfPhysHalfMag — Delphi HitMagAttackTarget(nPower div 2, nPower div 2)：
// 一半走物理减伤（AC），一半走魔法减伤（MAC）。
func halfPhysHalfMag(target *BaseObject, power int) int {
	if power < 2 {
		return 1
	}
	half := power / 2

	loAC := int(target.WAbil.AC & 0xFFFF)
	hiAC := int(target.WAbil.AC >> 16)
	armor := loAC
	if hiAC > loAC {
		armor = loAC + rand.Intn(hiAC-loAC+1)
	}
	phys := half - armor
	if phys < 0 {
		phys = 0
	}

	loMAC := int(target.WAbil.MAC & 0xFFFF)
	hiMAC := int(target.WAbil.MAC >> 16)
	antiMagic := loMAC
	if hiMAC > loMAC {
		antiMagic = loMAC + rand.Intn(hiMAC-loMAC+1)
	}
	mag := half - antiMagic
	if mag < 0 {
		mag = 0
	}

	damage := phys + mag
	if damage < 1 {
		damage = 1
	}
	return damage
}

// runTouchPoisonAI — Delphi TGreenMonster/TRedMonster（ObjMon3.pas:1269-1339）：
// 与目标严格对角相邻（|dx|=1 且 |dy|=1，正交贴脸不触发——原版语义忠实还原）时，
// 按 7/(AntiPoison+7) 概率施毒。RedMonster 原版前置检查的是绿毒位
//（ObjMon3.pas:1325，原版固有行为），此处忠实还原。
// kind: POISON_DECHEALTH（绿）/ POISON_DAMAGEARMOR（红）。
func (o *MonsterObject) runTouchPoisonAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64, kind int) {
	dx := abs(target.CurrX-o.CurrX)
	dy := abs(target.CurrY-o.CurrY)
	if dx == 1 && dy == 1 && target.StatusTimeArr[POISON_DECHEALTH] == 0 {
		anti := target.AntiPoison
		if anti < 0 {
			anti = 0
		}
		if rand.Intn(anti+7) <= 6 {
			// Delphi 30s；Go 玩家状态每 100ms -1 → ×10。
			// 注：Go 绿毒每 tick 结算与 Delphi 每 2.5s 结算的速率差异是既有问题。
			if kind == POISON_DECHEALTH {
				target.MakePoison(POISON_DECHEALTH, 300, 1)
			} else {
				target.MakePoison(POISON_DAMAGEARMOR, 300, 0)
			}
		}
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// runKhazardAI — Delphi TKhazard（ObjMon3.pas:1354-1403）：
// 目标处于严格对角距离 2（|dx|=2 且 |dy|=2）时，50% 无条件拖拽 /
// 50% 仅目标半血以下拖拽，把目标拉到面前一格并可能施绿毒 35s。
// 拖拽成功后目标落在面前（dist=1），条件自然失效（无冷却自限）。
// 注：Delphi 分支B 在目标半血以上时 nX/nY 未初始化（原版缺陷），
// Go 统一为"仅满足条件时拖拽"的安全语义。
func (o *MonsterObject) runKhazardAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	dx := abs(target.CurrX - o.CurrX)
	dy := abs(target.CurrY - o.CurrY)
	if dx == 2 && dy == 2 {
		doPull := rand.Intn(2) == 0 // Delphi time1 := Random(2)
		if doPull || int(target.WAbil.HP) <= int(target.WAbil.MaxHP)/2 {
			fx, fy := o.frontPosition()
			o.pullTargetTo(server, target, fx, fy)
			anti := target.AntiPoison
			if anti < 0 {
				anti = 0
			}
			// Delphi Random(1)=0 恒真，判定只剩 Random(AntiPoison+7)<=6
			if rand.Intn(anti+7) <= 6 {
				target.MakePoison(POISON_DECHEALTH, 350, 1)
				return // Delphi Exit：跳过本 tick 基类 AI
			}
		}
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// frontPosition — Delphi GetFrontPosition（ObjBase.pas:4298-4356）：
// 按朝向取面前一格（边界钳制）。
func (o *MonsterObject) frontPosition() (int, int) {
	dx, dy := dirToOffset(o.Dir)
	nx, ny := o.CurrX+dx, o.CurrY+dy
	if o.envir != nil {
		if nx < 0 {
			nx = 0
		}
		if ny < 0 {
			ny = 0
		}
		if nx >= o.envir.Width {
			nx = o.envir.Width - 1
		}
		if ny >= o.envir.Height {
			ny = o.envir.Height - 1
		}
	}
	return nx, ny
}

// pullTargetTo — Delphi SpaceMove 的玩家侧效果（ObjBase.pas:4359-4465）：
// 把目标瞬移到指定格（不可走则回退周围 3×3），并全量重同步客户端视野
//（SMClearObjects + SMChangeMap，与 EnterAnotherMap 同路径）。
func (o *MonsterObject) pullTargetTo(server *netserver.TCPServer, target *PlayObject, x, y int) {
	if o.envir == nil || target == nil || target.Ghost || target.Death {
		return
	}
	tx, ty := x, y
	if !o.envir.CanWalk(tx, ty) {
		found := false
		for ddy := -1; ddy <= 1 && !found; ddy++ {
			for ddx := -1; ddx <= 1 && !found; ddx++ {
				if o.envir.CanWalk(x+ddx, y+ddy) {
					tx, ty, found = x+ddx, y+ddy, true
				}
			}
		}
		if !found {
			return
		}
	}
	if target.envir != nil {
		target.envir.RemoveObject(target.CurrX, target.CurrY, OS_MOVINGOBJECT, target)
	}
	target.envir = o.envir
	target.MapName = o.MapName
	target.CurrX, target.CurrY = tx, ty
	target.ClearQueuedMsgs(protocol.CMWalk, protocol.CMRun, protocol.CMHorseRun)
	o.envir.AddObject(tx, ty, OS_MOVINGOBJECT, target)
	target.VisibleActors = make(map[int32]*VisibleEntry)

	clearMsg := protocol.MakeDefaultMsg(protocol.SMClearObjects, 0, 0, 0, 0)
	server.Send(target.Session.ID, clearMsg, "")
	changeMsg := protocol.MakeDefaultMsg(protocol.SMChangeMap, target.ID, uint16(tx), uint16(ty), uint16(target.dayBright()))
	server.Send(target.Session.ID, changeMsg, protocol.EncodeString(o.MapName))
}

// runFrostTigerAI — Delphi TFrostTiger 有目标分支（ObjMon3.pas:1237-1243）：
// 发现目标立即退出隐身。无目标进入隐身见 enterAmbush。
func (o *MonsterObject) runFrostTigerAI(server *netserver.TCPServer, target *PlayObject, dist int, now int64) {
	if o.StatusTimeArr[STATE_TRANSPARENT] > 0 {
		o.StatusTimeArr[STATE_TRANSPARENT] = 0
		o.broadcastMonsterStatus()
	}
	if dist <= 1 {
		o.meleeAttack(server, target, now)
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}

// enterAmbush — Delphi MagMakePrivateTransparent(Self, 180)（Magic.pas:734-761）：
// 进入隐身 180s，9 格内以本怪为目标的怪物丢失目标（距离>1 必清，贴脸 50%）。
// 怪物状态每 250ms -1 → 180s = 720。
func (o *MonsterObject) enterAmbush(server *netserver.TCPServer) {
	if o.envir != nil {
		for _, obj := range o.envir.GetRangeObjects(o.CurrX, o.CurrY, 9) {
			m, ok := obj.(*MonsterObject)
			if !ok || m == o || m.Death || m.Ghost {
				continue
			}
			if m.TargetID != 0 && (abs(m.CurrX-o.CurrX) > 1 || abs(m.CurrY-o.CurrY) > 1 || rand.Intn(2) == 0) {
				m.TargetID = 0
			}
		}
	}
	o.StatusTimeArr[STATE_TRANSPARENT] = 720
	o.broadcastMonsterStatus()
}

// broadcastMonsterStatus — Delphi StatusChanged（ObjBase.pas:20139-20142）：
// 广播状态位（STATE_TRANSPARENT → 客户端半透明渲染）。
// 状态位 = $80000000 shr i（ObjBase.pas:20074-20088）；客户端重组为
// State = Param<<16 | Tag，故高 16 位放 Param。
func (o *MonsterObject) broadcastMonsterStatus() {
	status := uint32(0)
	for i := 0; i < 12; i++ {
		if o.StatusTimeArr[i] > 0 {
			status |= 0x80000000 >> uint(i)
		}
	}
	o.SendRefMsg(RM_CHARSTATUSCHANGED, int(uint16(status>>16)), int(uint16(status)), 0, "")
}

// kickSoccerBall — Delphi TSoccerBall.Struck（ObjMon2.pas:359-366）：
// 沿击打者朝向滚动，动量累加（+4+rand4，上限 20 格），目标 = 前方 n 格。
func (o *MonsterObject) kickSoccerBall(attackerID int32, e *UserEngine) {
	var hiterDir int
	found := false
	if e != nil {
		if p := e.GetPlayer(attackerID); p != nil {
			hiterDir, found = p.Dir, true
		} else if m := e.GetMonster(attackerID); m != nil {
			hiterDir, found = m.Dir, true
		}
	}
	if !found {
		return
	}
	o.Dir = hiterDir
	o.rollDist += 4 + rand.Intn(4)
	if o.rollDist > 20 {
		o.rollDist = 20
	}
	o.rollTargetX, o.rollTargetY = o.soccerTargetInDir(o.Dir, o.rollDist)
}

// soccerTargetInDir — 从当前位置沿 dir 前进 steps 格，地图边界处钳制。
// （Delphi GetNextPosition 越界时整体不更新目标，ObjMon2.pas:365；
// 此处改为钳制到边界，保证贴边踢球仍有滚动效果。）
func (o *MonsterObject) soccerTargetInDir(dir, steps int) (int, int) {
	tx, ty := o.CurrX, o.CurrY
	if o.envir == nil {
		return tx, ty
	}
	dx, dy := dirToOffset(dir)
	for i := 0; i < steps; i++ {
		nx, ny := tx+dx, ty+dy
		if nx < 0 || nx >= o.envir.Width || ny < 0 || ny >= o.envir.Height {
			break
		}
		tx, ty = nx, ny
	}
	return tx, ty
}

// runSoccerRoll — Delphi TSoccerBall.Run（ObjMon2.pas:320-357）：
// 每 tick 向滚动目标移动一格；路径受阻时按 Delphi 反弹方向表反向并重算目标，
// 到达目标后动量清零（Delphi 原文在可走时反向，按其设计意图实现为受阻反弹）。
func (o *MonsterObject) runSoccerRoll(now int64) {
	if o.rollDist <= 0 {
		return
	}
	if o.CurrX == o.rollTargetX && o.CurrY == o.rollTargetY {
		o.rollDist = 0
		return
	}
	dir := dirToward(o.CurrX, o.CurrY, o.rollTargetX, o.rollTargetY)
	if o.WalkTo(dir) {
		o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
		return
	}
	// Delphi 反弹方向表（ObjMon2.pas:331-340）：正方向 180° 反向，斜方向水平翻转
	bounced := [8]int{4, 7, 6, 5, 0, 3, 2, 1}
	o.Dir = bounced[dir]
	o.rollTargetX, o.rollTargetY = o.soccerTargetInDir(o.Dir, o.rollDist)
	if o.CurrX == o.rollTargetX && o.CurrY == o.rollTargetY {
		o.rollDist = 0
		return
	}
	dir = dirToward(o.CurrX, o.CurrY, o.rollTargetX, o.rollTargetY)
	if o.WalkTo(dir) {
		o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
	} else {
		o.rollDist = 0
	}
}

