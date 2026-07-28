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

	FixedHide      bool  // 潜地状态（m_boFixedHideMode）
	StoneMode      bool  // 石化伏击状态（m_boStoneMode）
	Animal         bool  // 动物标志（m_boAnimal）
	spawnTick      int64 // 出生时间（自爆计时用）
	searchInterval int64 // SearchViewRange 随机间隔
	burstCount     int   // 连射计数（飞斧用）
}

func getAIBehavior(race byte) int {
	// Race→AI mapping based on Delphi factory (UsrEngn.pas:1831-1938).
	switch race {
	case 51: // 鸡 — 被动动物
		return AIPassive
	case 52: // TChickenDeer — 逃跑
		return AIFlee
	case 80: // TMonster — 基础游荡（不主动搜索）
		return AIPassive
	case 82: // TSpitSpider — 远程喷吐
		return AIRanged
	case 85: // TStickMonster — 固定潜地伏击
		return AIBurrow
	case 87: // TDualAxeMonster — 远程飞斧
		return AIDualAxe
	case 90: // TGasAttackMonster — 毒气近战
		return AIPoison
	case 91: // TMagCowMonster — 魔法近战
		return AIMagicCast
	case 94: // TLightingZombi — 远程闪电
		return AIRanged
	case 95: // TDigOutZombi — 潜地僵尸
		return AIBurrow
	case 96: // TZilKinZombi — 死亡分裂
		return AISplit
	case 101: // TScultureMonster — 石化伏击
		return AIStone
	case 102: // TScultureKingMonster — 召唤
		return AISummoner
	case 105: // TGasMothMonster — 毒气近战
		return AIPoison
	case 107: // TCentipedeKingMonster — 固定Boss（TODO: StickMode）
		return AIArea
	case 117: // TExplosionSpider — 自爆
		return AIExplode
	case 118: // THighRiskSpider — 远程
		return AIRanged
	case 119: // TBigPoisionSpider — 远程+毒
		return AIRanged
	case 131: // TRonObject — 范围攻击
		return AIArea
	case 200: // TElectronicScolpionMon — 闪电吸血
		return AILeech
	default:
		// 53(wolf), 81(oma knight), 83(slow), 84(scorpion),
		// 92(cow king), 100(white skeleton), 103(bee queen),
		// 113(elf), 130(double critical)
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

func (o *MonsterObject) OnStruck(attackerID int32, now int64, userEngine *UserEngine) {
	o.LastHiterID = attackerID
	o.LastHiterTick = now
	// Delphi: 攻击延迟惩罚 m_dwHitTick += 150 - min(130, Level*4)
	penalty := int64(150)
	if lvl := int64(o.WAbil.Level) * 4; lvl < 130 {
		penalty = 150 - lvl
	} else {
		penalty = 20
	}
	o.HitTick += penalty
	// Delphi: 无目标 OR 当前目标相邻 OR 1/6随机 → 切换目标
	switchTarget := o.TargetID == 0 || rand.Intn(6) == 0
	if !switchTarget && o.TargetID != 0 && userEngine != nil {
		if cur := userEngine.GetPlayer(o.TargetID); cur != nil {
			dx := abs(cur.CurrX - o.CurrX)
			dy := abs(cur.CurrY - o.CurrY)
			if dx <= 1 && dy <= 1 {
				switchTarget = true
			}
		}
	}
	if switchTarget {
		o.TargetID = attackerID
		o.FocusTick = now
	}
}

func (o *MonsterObject) Run(server *netserver.TCPServer, now int64, userEngine *UserEngine) {
	if o.Ghost || o.Death {
		return
	}

	// 状态效果 tick（所有对象都需要，包括石化中的怪物）
	for i := 0; i < 12; i++ {
		if o.StatusTimeArr[i] > 0 {
			o.StatusTimeArr[i]--
		}
	}
	if o.StatusTimeArr[POISON_DECHEALTH] > 0 {
		hp := int(o.WAbil.HP) - 2
		if hp < 1 {
			hp = 1
		}
		o.WAbil.HP = uint16(hp)
	}

	// Delphi: 石化状态跳过 AI（POISON_STONE 状态效果）
	if o.StatusTimeArr[POISON_STONE] > 0 {
		return
	}

	// Delphi: m_boStoneMode 石化伏击（TScultureMonster）
	// 石化中只搜索目标，检测到玩家后解除石化
	if o.StoneMode {
		o.searchTarget(now, userEngine)
		if o.TargetID != 0 {
			o.StoneMode = false
			o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
			log.Logf(log.LevelInfo, "Monster", "%s broke out of stone", o.Name)
		}
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

	// AIPassive（Race 51/80）不主动搜索目标，仅通过 OnStruck 获得
	if o.AIBehavior != AIPassive {
		o.searchTarget(now, userEngine)
	}
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
			// Delphi TRonObject: 目标在6格内 → AroundAttack（1格半径AoE）
			if dist <= 6 && now-o.HitTick > o.AttackSpeed {
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
			} else if dist > 6 {
				o.chaseTarget(target, now)
			}
		case 4:
			// Delphi 召唤类：创建真实 minion 对象
			if now-o.lastSummonTick > 30000 && o.minionCount < 3 && o.Engine != nil && o.envir != nil {
				o.lastSummonTick = now
				cx := o.CurrX + rand.Intn(3) - 1
				cy := o.CurrY + rand.Intn(3) - 1
				if o.envir.CanWalk(cx, cy) {
					o.Engine.mu.Lock()
					id := o.Engine.nextMonsterID
					o.Engine.nextMonsterID++
					minion := NewMonsterObject(o.Name+"(召唤)", id, o.Race, o.RaceImg, o.Appr,
						o.MaxHP/2, o.WalkSpeed, o.AttackSpeed, o.Exp/3)
					minion.CurrX = cx
					minion.CurrY = cy
					minion.MapName = o.MapName
					minion.envir = o.envir
					minion.HitPoint = o.HitPoint
					minion.SpeedPoint = o.SpeedPoint
					minion.WAbil.DC = o.WAbil.DC
					minion.WAbil.AC = o.WAbil.AC
					minion.WAbil.HP = minion.WAbil.MaxHP
					minion.TargetID = o.TargetID
					minion.spawnTick = now
					minion.searchInterval = 3000 + rand.Int63n(2000)
					o.envir.AddObject(cx, cy, OS_MOVINGOBJECT, minion)
					o.Engine.Monsters = append(o.Engine.Monsters, minion)
					o.Engine.mu.Unlock()
					minion.SendRefMsg(RM_TURN, minion.Dir, cx, cy, minion.Name)
					o.minionCount++
					log.Logf(log.LevelInfo, "Monster", "%s summoned a minion at (%d,%d)", o.Name, cx, cy)
				}
			}
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target, now)
			}
		case AIPassive:
			// 被动型：有目标时正常追击/攻击（目标仅来自 OnStruck）
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target, now)
			}
		default:
			o.runExtendedAI(server, target, dist, now)
		}
	} else {
		// 潜地怪物无目标时不闲逛
		if o.FixedHide {
			return
		}
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

	// Delphi: nArmor = loAC + Random(hiAC - loAC + 1)
	loAC := int(target.WAbil.AC & 0xFFFF)
	hiAC := int(target.WAbil.AC >> 16)
	armor := loAC
	if hiAC > loAC {
		armor = loAC + rand.Intn(hiAC-loAC+1)
	}
	damage := attack - armor
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
			o.envir.broadcastDeathMsg(target.BaseObject, target.ID, target.CurrX, target.CurrY, target.Dir, true)
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
	// Delphi: n20 := Random(3)，顺/逆时针随机绕行
	clockwise := rand.Intn(3) != 0
	for i := 0; i < 7; i++ {
		var altDir int
		if clockwise {
			altDir = (dir + i + 1) % 8
		} else {
			altDir = (dir - i - 1 + 8) % 8
		}
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
	// Delphi: Random(SpeedPoint) < HitPoint → 命中
	spd := target.SpeedPoint
	if spd < 1 {
		spd = 1
	}
	if rand.Intn(spd) >= o.HitPoint {
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
	interval := int64(1000) // 无目标时 1 秒搜索
	if hasTarget {
		// Delphi: m_dwSearchTime = 3000 + Random(2000)
		interval = o.searchInterval
		if interval <= 0 {
			interval = 8000
		}
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
	// Delphi: 曼哈顿距离 > 15 格则丢失目标
	dx := abs(target.CurrX - o.CurrX)
	dy := abs(target.CurrY - o.CurrY)
	if dx+dy > 15 {
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
