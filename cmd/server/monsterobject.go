package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const LA_UNDEAD = 1

type MonsterObject struct {
	*BaseObject
	Race        byte
	RaceImg     byte
	Appr        uint16
	MaxHP       int
	WalkSpeed   int64
	AttackSpeed int64
	Exp         int

	AIBehavior int // 0=近战, 1=远程, 2=逃跑, 3=范围, 4=召唤, 5+=扩展

	TargetID       int32
	MasterID       int32 // 所属召唤者怪物 ID（0 = 野生）
	HomeX, HomeY   int
	WalkTick       int64
	SearchTick     int64
	HitTick        int64
	DeathTick      int64
	LootDropped    bool
	lastSummonTick int64

	LastHiterID   int32
	LastHiterTick int64
	FocusTick     int64

	walkCount      int
	walkWaitTick   int64
	walkWaitLocked bool // Delphi m_boWalkWaitLocked：步频休息锁存
	WalkStep       int
	WalkWait       int64

	lastRegenTick int64
	runTick       int64 // Delphi m_dwRunTick：Run 节流（250ms）

	ViewRange int
	CoolEye   int
	MagID     int

	slaveName string

	lastThinkTick int64

	FixedHide      bool  // 潜地状态（m_boFixedHideMode）
	StoneMode      bool  // 石化伏击状态（m_boStoneMode）
	StickMode      bool  // 固定模式（m_boStickMode，不可移动）
	Animal         bool  // 动物标志（m_boAnimal）
	spawnTick      int64 // 出生时间（自爆计时用）
	searchInterval int64 // SearchViewRange 随机间隔
	burstCount     int   // 连射计数（飞斧用）
	spitPoison     bool  // 喷吐附带绿毒（Race 82/119 true，Race 118 false）

	// TScultureKingMonster（Race 102）：危险等级召唤（ObjMon.pas:1444）
	dangerLevel int

	// TCentipedeKingMonster（Race 107）：潜地/出土节奏（ObjMon2.pas:153）
	attickTick int64

	// TCowKingMonster（Race 92）：血量阶段狂暴
	rageState       int // 0=正常 1=停滞 2=狂暴
	rageTick        int64
	hpBracket       int
	saveAttackSpeed int64
	saveWalkSpeed   int64

	// 主人-奴隶系统（玩家宠物）
	PlayerMasterID int32 // 玩家主人 ID（0=非玩家宠物，区别于 MasterID 的怪物父子关系）
	SlaveRelax     bool  // 休息模式（不传送跟随）

	// 幽灵两阶段清理
	ghostTick int64

	// 逃跑改进（Delphi GetNextPosition 5 格）
	fleeTargetX, fleeTargetY int
	fleeExpireTick           int64

	// 飞斧延迟伤害（Delphi FlyAxeAttack: max(dx,dy)*50+600ms）
	pendingAxeDmg    int
	pendingAxeTarget int32
	pendingAxeTick   int64

	// 吸血配置（Delphi btGetBackHP）
	leechDivisor int

	// 升级骷髅（Delphi TWhiteSkeleton 经验升级）
	petXP    int
	petLevel int
	petMaxXP int

	// 变形（Delphi TElfMonster Race 113 双形态切换）
	transformForm  int
	transformTick  int64
	transformAppr2 uint16
	saveDC, saveAC, saveMC uint32

	// 火焰光环（Delphi TFireMonster 十字火事件）
	lastAuraTick int64

	// 不死属性（Delphi m_btLifeAttrib）
	LifeAttrib byte

	// 疯狂/厌恶模式（Delphi m_boCrazyMode/m_boNastyMode）
	CrazyMode bool // 攻击一切对象
	NastyMode bool // 攻击所有非NPC对象

	// 运行时缓存（Run 期间有效）
	engine *UserEngine
}

func getAIBehavior(race byte) int {
	// Race 到 AI 的映射，参考 Delphi 工厂方法（UsrEngn.pas:1819-1938）。
	switch race {
	case 51: // 鸡 — 被动动物
		return AIPassive
	case 52: // 鹿 — 由 SpawnMonster 掷骰决定 AIFlee/AIPassive
		return AIPassive
	case 53: // 狼 — TATMonster + 动物（主动搜索）
		return AIMelee
	case 80: // TMonster — 基础游荡（不主动搜索）
		return AIPassive
	case 82: // TSpitSpider — 2格锥形喷吐
		return AISpit
	case 83: // TSlowATMonster — 慢速近战（AttackSpeed 在 SpawnMonster 中翻倍）
		return AIMelee
	case 85: // TStickMonster — 固定潜地伏击
		return AIBurrow
	case 87: // TDualAxeMonster — 远程飞斧
		return AIDualAxe
	case 90: // TGasAttackMonster — 毒气近战
		return AIPoison
	case 91: // TMagCowMonster — 魔法近战
		return AIMagicCast
	case 92: // TCowKingMonster — 牛魔王（目标点 AoE + 血量阶段狂暴）
		return AICowKing
	case 93: // TThornDarkMonster — 远程飞斧变种
		return AIRanged
	case 94: // TLightingZombi — 线性闪电（穿透一条线上所有目标）
		return AILightning
	case 95: // TDigOutZombi — 潜地僵尸
		return AIBurrow
	case 96: // TZilKinZombi — 死亡分裂
		return AISplit
	case 100: // TWhiteSkeleton — 升级骷髅（道士召唤，经验升级）
		return AILevelingSkeleton
	case 101: // TScultureMonster — 石化伏击
		return AIStone
	case 102: // TScultureKingMonster — 祖玛教主（石化 + 危险等级召唤 + 魔法近战）
		return AISummoner
	case 103: // TBeeQueen — 蜂王（固定召唤蜂群）
		return AISpawnHive
	case 104: // TArcherMonster — 远程弓箭
		return AIRanged
	case 105: // TGasMothMonster — 毒气近战
		return AIPoison
	case 106: // TGasDungMonster — 毒气近战
		return AIPoison
	case 107: // TCentipedeKingMonster — 蜈蚣王（潜地 + 毒 AoE）
		return AICentiKing
	case 110: // TCastleDoor — 城门（静态建筑，攻城战未实现）
		return AIPassive
	case 111: // TWallStructure — 城墙（静态建筑）
		return AIPassive
	case 112: // TArcherGuard — 弓箭守卫（固定炮台，只打红名/攻击者）
		return AIGuard
	case 113: // TElfMonster — 变形精灵（HP 阈值切换形态/属性）
		return AITransform
	case 114: // TElfWarriorMonster — 潜地战士
		return AIBurrow
	case 115: // TBigHeartMonster — 触角神（固定 16 格脉冲）
		return AIPulse
	case 116: // TSpiderHouseMonster — 蜘蛛巢（固定召唤蜘蛛）
		return AISpawnHive
	case 117: // TExplosionSpider — 自爆
		return AIExplode
	case 118: // THighRiskSpider — 非毒喷吐
		return AISpit
	case 119: // TBigPoisionSpider — 毒喷吐
		return AISpit
	case 130: // TDoubleCriticalMonster — 远程双暴击
		return AICritical
	case 131: // TRonObject — 范围攻击
		return AIArea
	case 132: // TSandMobObject — 潜地沙怪
		return AIBurrow
	case 133: // TMagicMonObject — 魔法攻击
		return AIMagicCast
	case 200: // TElectronicScolpionMon — 闪电吸血
		return AILeech
	case 201: // TClone — 分身怪（DB 未使用）
		return AIClone
	case 203: // TTeleMonster — 瞬移怪（DB 未使用）
		return AITeleport
	case 215: // TFireBallMonster — 远程火球
		return AIFireball
	default:
		// 81(沃玛战士), 84(蝎子), 86/88/89(TATMonster), 97(牛怪),
		// 100(骷髅战士), 113(精灵), 150/156(人形怪) 等
		return AIMelee
	}
}

func NewMonsterObject(name string, id int32, race, raceImg byte, appr uint16, hp int, walkSpeed, attackSpeed int64, exp int) *MonsterObject {
	base := NewBaseObject(name, id)
	mon := &MonsterObject{
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
	base.outer = mon
	return mon
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
	o.engine = userEngine

	// Delphi: 奴隶跟随/传送/叛变（ObjMon.pas:468-497）
	if o.PlayerMasterID != 0 {
		o.slaveFollow(userEngine, now)
		if o.Ghost || o.Death {
			return
		}
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
			log.Logf(log.LevelInfo, "Monster", "%s released from petrification", o.Name)
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
		// Delphi dup mode (ObjMon.pas:359-381)：本格 GetXYObjCount >= 2 → 随机方向逃逸。
		if o.envir != nil {
			self := o.self()
			overlapped := false
			for _, obj := range o.envir.GetRangeObjects(o.CurrX, o.CurrY, 0) {
				if obj != self && blocksMovement(obj) {
					overlapped = true
					break
				}
			}
			if overlapped {
				dir := rand.Intn(8)
				if o.WalkTo(dir) {
					o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
				}
			}
		}
	}

	// AIPassive（Race 51/80）不主动搜索目标，仅通过 OnStruck 获得；
	// AIGuard（Race 112）使用自己的目标规则（红名/攻击者），不走通用搜索
	if o.AIBehavior != AIPassive && o.AIBehavior != AIGuard {
		o.searchTarget(now, userEngine)
	}
	o.validateTarget(now, userEngine)

	// Delphi TArcherGuard (ObjMon2.pas:887)：固定炮台，自带扫描与攻击
	if o.AIBehavior == AIGuard {
		o.runGuardAI(server, now)
		return
	}

	if o.TargetID != 0 {
		target := userEngine.GetPlayer(o.TargetID)
		if target == nil || target.Death || target.Ghost {
			if monTarget := userEngine.GetMonster(o.TargetID); monTarget != nil && !monTarget.Death {
				o.attackMonsterTarget(server, monTarget, now)
			} else {
				o.TargetID = 0
			}
			return
		}
		dx := abs(target.CurrX - o.CurrX)
		dy := abs(target.CurrY - o.CurrY)
		dist := dx
		if dy > dist {
			dist = dy
		}

		switch o.AIBehavior {
		case AIMelee:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target.BaseObject, now)
			}
		case AIRanged:
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else if dist <= 5 && o.envir.CanFlyLine(o.CurrX, o.CurrY, target.CurrX, target.CurrY) {
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
				o.chaseTarget(target.BaseObject, now)
			}
		case AIFlee:
			// Delphi TChickenDeer (ObjMon.pas:542-598)：计算远离方向 5 格目标点，跑到位
			if o.StatusTimeArr[POISON_DONTMOVE] > 0 {
				break
			}
			if o.fleeTargetX == 0 && o.fleeTargetY == 0 || now > o.fleeExpireTick {
				fleeDir := dirToward(target.CurrX, target.CurrY, o.CurrX, o.CurrY)
				fdx, fdy := dirToOffset(fleeDir)
				o.fleeTargetX = o.CurrX + fdx*5
				o.fleeTargetY = o.CurrY + fdy*5
				o.fleeExpireTick = now + 5000
			}
			if now-o.WalkTick >= o.WalkSpeed {
				o.WalkTick = now
				if o.CurrX == o.fleeTargetX && o.CurrY == o.fleeTargetY {
					o.fleeTargetX, o.fleeTargetY = 0, 0
					break
				}
				dir := dirToward(o.CurrX, o.CurrY, o.fleeTargetX, o.fleeTargetY)
				if o.WalkTo(dir) {
					o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
				} else {
					for i := 1; i < 8; i++ {
						altDir := (dir + i) % 8
						if o.WalkTo(altDir) {
							o.SendRefMsg(RM_WALK, altDir, o.CurrX, o.CurrY, "")
							break
						}
					}
				}
			}
		case AIArea:
			// Delphi TRonObject: 目标在6格内 → AroundAttack（1格半径AoE）
			if o.envir == nil {
				return
			}
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
				o.chaseTarget(target.BaseObject, now)
			}
		case AISummoner:
			if dist <= 1 {
				o.magicMeleeAttack(server, target, now)
			} else {
				o.chaseTarget(target.BaseObject, now)
			}
			const maxMinions = 3
			if o.slaveName != "" && now-o.lastSummonTick > 10000 {
				live := userEngine.countLiveChildren(o.ID)
				if live < maxMinions {
					o.lastSummonTick = now
					for i := live; i < maxMinions; i++ {
						x := o.CurrX + rand.Intn(5) - 2
						y := o.CurrY + rand.Intn(5) - 2
						if child := userEngine.SpawnMonsterByName(o.MapName, x, y, o.slaveName, now); child != nil {
							child.MasterID = o.ID
							child.TargetID = o.TargetID
						}
					}
				}
			}
			// Delphi TScultureKingMonster danger level mechanic
			if o.dangerLevel > 0 && o.MaxHP > 0 &&
				int64(o.dangerLevel) > int64(o.WAbil.HP)*5/int64(o.MaxHP) {
				o.dangerLevel--
				o.callSlave(userEngine, now)
			}
			if int(o.WAbil.HP) >= o.MaxHP {
				o.dangerLevel = 5
			}
		case AIPassive:
			// 被动型：有目标时正常追击/攻击（目标仅来自 OnStruck）
			if dist <= 1 {
				o.meleeAttack(server, target, now)
			} else {
				o.chaseTarget(target.BaseObject, now)
			}
		default:
			o.runExtendedAI(server, userEngine, target, dist, now)
		}
	} else {
		// 潜地/固定/定身怪物无目标时不闲逛
		if o.FixedHide || o.StickMode || o.StatusTimeArr[POISON_DONTMOVE] > 0 {
			// Delphi TStickMonster/TCentipedeKingMonster：出土状态下丢失目标 → 回潜
			if o.StickMode && !o.FixedHide && (o.AIBehavior == AIBurrow || o.AIBehavior == AICentiKing) {
				o.FixedHide = true
				o.SendRefMsg(RM_DIGDOWN, 0, o.CurrX, o.CurrY, "")
			}
			return
		}
		if now-o.WalkTick > o.WalkSpeed {
			// Delphi m_boWalkWaitLocked：突发-休息循环
			if o.walkWaitLocked {
				if now-o.walkWaitTick >= o.WalkWait {
					o.walkWaitLocked = false
				} else {
					return
				}
			}
			if rand.Intn(20) == 0 {
				o.WalkTick = now
				o.walkCount++
				if o.WalkStep > 0 && o.walkCount > o.WalkStep {
					o.walkCount = 0
					o.walkWaitLocked = true
					o.walkWaitTick = now
				} else if rand.Intn(4) == 0 {
					o.TurnTo(rand.Intn(8))
				} else {
					if o.monsterWalkTo(o.Dir, userEngine) {
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

	// Delphi: 不死系加成（ObjBase.pas:22000）
	if o.LifeAttrib == LA_UNDEAD {
		attack += target.UndeadBonus
	}

	// Delphi: nArmor = loAC + Random(hiAC - loAC + 1)
	loAC := int(target.WAbil.AC & 0xFFFF)
	hiAC := int(target.WAbil.AC >> 16)
	armor := loAC
	if hiAC > loAC {
		armor = loAC + rand.Intn(hiAC-loAC+1)
	}
	// Delphi: 红毒（POISON_DAMAGEARMOR）降低防御力
	if target.StatusTimeArr[POISON_DAMAGEARMOR] > 0 {
		armor /= 2
	}
	damage := attack - armor
	if damage < 1 {
		damage = 1
	}
	return damage
}

// calcMonsterMagicDamage — Delphi GetMagStruckDamage (ObjBase.pas:22441)：
// DC 掷骰作为伤害源，目标 MAC 掷骰减伤（祖玛教主/蜈蚣王/触角神的魔法路径）。
func (o *MonsterObject) calcMonsterMagicDamage(target *BaseObject) int {
	loDC := int(o.WAbil.DC & 0xFFFF)
	hiDC := int(o.WAbil.DC >> 16)
	attack := loDC
	if hiDC > loDC {
		attack = loDC + rand.Intn(hiDC-loDC+1)
	}
	if attack < 1 {
		attack = 1
	}

	loMAC := int(target.WAbil.MAC & 0xFFFF)
	hiMAC := int(target.WAbil.MAC >> 16)
	antiMagic := loMAC
	if hiMAC > loMAC {
		antiMagic = loMAC + rand.Intn(hiMAC-loMAC+1)
	}
	damage := attack - antiMagic
	if damage < 1 {
		damage = 1
	}
	return damage
}

// magicMeleeAttack — 相邻魔法近战（祖玛教主 Attack：DC 掷骰、魔法减伤路径）。
func (o *MonsterObject) magicMeleeAttack(server *netserver.TCPServer, target *PlayObject, now int64) {
	if now-o.HitTick < o.AttackSpeed {
		return
	}
	o.HitTick = now
	o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	if IsSafeZone(o.envir, target.CurrX, target.CurrY) {
		return
	}
	spd := target.SpeedPoint
	if spd < 1 {
		spd = 1
	}
	if rand.Intn(spd) >= o.HitPoint {
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
		return
	}
	damage := o.calcMonsterMagicDamage(target.BaseObject)
	o.applyMonsterDamageToPlayer(server, target, damage, now)
	o.FocusTick = now
	o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
}

// callSlave — Delphi TScultureKingMonster.CallSlave (ObjMon.pas:1490)：
// 一次生成 6-11 只祖玛随从（存活上限 30），优先生成于面前格。
func (o *MonsterObject) callSlave(e *UserEngine, now int64) {
	if e == nil || o.envir == nil {
		return
	}
	names := []string{"祖玛雕像", "祖玛卫士", "祖玛弓箭手"}
	count := 6 + rand.Intn(6)
	fdx, fdy := dirToOffset(o.Dir)
	for i := 0; i < count; i++ {
		if e.countLiveChildren(o.ID) >= 30 {
			break
		}
		x, y := o.CurrX+fdx, o.CurrY+fdy
		if !o.envir.CanWalk(x, y) {
			placed := false
			for tries := 0; tries < 8; tries++ {
				tx := o.CurrX + rand.Intn(3) - 1
				ty := o.CurrY + rand.Intn(3) - 1
				if o.envir.CanWalk(tx, ty) {
					x, y = tx, ty
					placed = true
					break
				}
			}
			if !placed {
				continue
			}
		}
		e.spawnChild(o, names[rand.Intn(len(names))], x, y, now)
	}
	log.Logf(log.LevelInfo, "Monster", "%s called Zuma slaves", o.Name)
}

// initAITimers — 初始化 AI 计时器为 now 附近（Delphi 出生错峰：
// m_dwHitTick/m_dwWalkTick := GetTickCount - Random(3000)，防止首 tick 齐射）。
func (o *MonsterObject) initAITimers(now int64) {
	o.spawnTick = now
	o.SearchTick = now
	o.WalkTick = now - rand.Int63n(3000)
	o.HitTick = now - rand.Int63n(3000)
	o.lastRegenTick = now
	o.lastThinkTick = now
	o.lastSummonTick = now
	o.attickTick = now
	o.searchInterval = 3000 + rand.Int63n(2000) // Delphi: 3000 + Random(2000)
}

func (o *MonsterObject) applyMonsterDamageToPlayer(server *netserver.TCPServer, target *PlayObject, damage int, now int64) {
	// Delphi: 安全区内怪物不能攻击玩家（IsProperTarget 安全区检查）
	if IsSafeZone(o.envir, target.CurrX, target.CurrY) {
		return
	}
	target.LastHiterID = o.ID
	target.LastHiterTick = now
	// Delphi TWhiteSkeleton: 战斗获得经验
	if o.AIBehavior == AILevelingSkeleton {
		o.gainPetXP(damage / 10)
	}
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

func (o *MonsterObject) chaseTarget(target *BaseObject, now int64) {
	if o.StickMode || o.StatusTimeArr[POISON_DONTMOVE] > 0 {
		return
	}
	if now-o.WalkTick < o.WalkSpeed {
		return
	}
	// Delphi m_boWalkWaitLocked：突发-休息循环
	if o.walkWaitLocked {
		if now-o.walkWaitTick >= o.WalkWait {
			o.walkWaitLocked = false
		} else {
			return
		}
	}
	o.walkCount++
	if o.WalkStep > 0 && o.walkCount > o.WalkStep {
		o.walkCount = 0
		o.walkWaitLocked = true
		o.walkWaitTick = now
		return
	}
	o.WalkTick = now

	dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
	if o.monsterWalkTo(dir, o.engine) {
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
		if o.monsterWalkTo(altDir, o.engine) {
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
	// Delphi IsProperTarget（ObjBase.pas:21344-21373）：奴隶目标受主人约束
	if o.PlayerMasterID != 0 {
		o.searchTargetAsSlave(now, userEngine)
		return
	}

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
	vr += o.CoolEye
	objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, vr)
	var best *PlayObject
	bestDist := 999999
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death {
			continue
		}
		// F11: GM 不可被攻击
		if p.Permission > 9 {
			continue
		}
		// CoolEye > player level: always detect invisible; otherwise probabilistic
		if p.Hidden && o.CoolEye <= int(p.WAbil.Level) && rand.Intn(100) >= o.CoolEye {
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
		return
	}

	if o.CrazyMode || o.NastyMode {
		var bestMon *MonsterObject
		bestMonDist := 999999
		for _, obj := range objs {
			m, ok := obj.(*MonsterObject)
			if !ok || m.Ghost || m.Death || m.ID == o.ID {
				continue
			}
			if m.MasterID == o.ID || o.MasterID == m.ID {
				continue
			}
			d := abs(m.CurrX-o.CurrX) + abs(m.CurrY-o.CurrY)
			if d < bestMonDist {
				bestMonDist = d
				bestMon = m
			}
		}
		if bestMon != nil {
			o.TargetID = bestMon.ID
		}
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

// slaveFollow — Delphi 主人跟随与瞬移（ObjMon.pas:468-497）：
// 主人死亡/离线 → 叛变；距离 >20 格或跨图 → 瞬移到主人背后。
func (o *MonsterObject) slaveFollow(e *UserEngine, now int64) {
	master := e.GetPlayer(o.PlayerMasterID)
	if master == nil || master.Ghost {
		o.PlayerMasterID = 0
		return
	}
	if master.Death {
		o.PlayerMasterID = 0
		o.TargetID = 0
		return
	}
	if master.MapName == o.MapName {
		dx := abs(master.CurrX - o.CurrX)
		dy := abs(master.CurrY - o.CurrY)
		dist := dx
		if dy > dist {
			dist = dy
		}
		if dist <= 3 {
			return
		}
	}
	if o.SlaveRelax {
		return
	}
	// Delphi: dist > 20 或不同地图 → SpaceMove 瞬移
	needTeleport := master.MapName != o.MapName
	if !needTeleport {
		dx := abs(master.CurrX - o.CurrX)
		dy := abs(master.CurrY - o.CurrY)
		dist := dx
		if dy > dist {
			dist = dy
		}
		needTeleport = dist > 20
	}
	if !needTeleport {
		return
	}
	// GetBackPosition: 主人背后一格。Delphi SpaceMove：仅校验地形，不可用则随机重试，无果放弃。
	env := master.envir
	if env == nil {
		return
	}
	bdx, bdy := dirToOffset((master.Dir + 4) % 8)
	tx, ty := master.CurrX+bdx, master.CurrY+bdy
	if !env.CanWalkEx(tx, ty, true) {
		placed := false
		for tries := 0; tries < 30; tries++ {
			rx := master.CurrX + rand.Intn(7) - 3
			ry := master.CurrY + rand.Intn(7) - 3
			if env.CanWalkEx(rx, ry, true) {
				tx, ty = rx, ry
				placed = true
				break
			}
		}
		if !placed {
			return
		}
	}
	if o.envir != nil {
		o.envir.RemoveObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)
	}
	o.MapName = master.MapName
	o.CurrX, o.CurrY = tx, ty
	o.envir = env
	env.AddObject(tx, ty, OS_MOVINGOBJECT, o)
	o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, o.Name)
}

// searchTargetAsSlave — Delphi IsProperTarget 奴隶规则（ObjBase.pas:21344-21373）：
// 优先攻击主人的 LastHiter，不攻击同主人奴隶，不攻击安全区目标。
func (o *MonsterObject) searchTargetAsSlave(now int64, userEngine *UserEngine) {
	hasTarget := o.TargetID != 0
	interval := int64(1000)
	if hasTarget {
		interval = o.searchInterval
		if interval <= 0 {
			interval = 8000
		}
	}
	if now-o.SearchTick <= interval {
		return
	}
	o.SearchTick = now

	master := userEngine.GetPlayer(o.PlayerMasterID)
	if master == nil || master.Death || master.Ghost {
		return
	}
	// 优先：主人的 LastHiter（10s 内）
	if master.LastHiterID != 0 && now-master.LastHiterTick < 10000 {
		if t := userEngine.GetPlayer(master.LastHiterID); t != nil && !t.Death && !t.Ghost {
			if !IsSafeZone(o.envir, t.CurrX, t.CurrY) {
				o.TargetID = t.ID
				return
			}
		}
	}
	// 其次：正在攻击主人的玩家（视野内 LastHiter 指向主人的）
	if o.envir == nil {
		return
	}
	vr := o.ViewRange
	if vr <= 0 {
		vr = 5
	}
	vr += o.CoolEye
	for _, obj := range o.envir.GetRangeObjects(o.CurrX, o.CurrY, vr) {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death {
			continue
		}
		if p.LastHiterID == master.ID && now-p.LastHiterTick < 5000 {
			continue
		}
		if p.Hidden && o.CoolEye <= int(p.WAbil.Level) && rand.Intn(100) >= o.CoolEye {
			continue
		}
		if IsSafeZone(o.envir, p.CurrX, p.CurrY) {
			continue
		}
		o.TargetID = p.ID
		return
	}
}

// monsterWalkTo — 怪物专用移动：危险地形检查 + 主人面前阻挡（Delphi WalkTo 守卫）。
func (o *MonsterObject) monsterWalkTo(dir int, e *UserEngine) bool {
	dx, dy := dirToOffset(dir)
	newX, newY := o.CurrX+dx, o.CurrY+dy
	if o.envir != nil && !o.envir.CanSafeWalk(newX, newY) {
		return false
	}
	if o.PlayerMasterID != 0 && e != nil {
		if master := e.GetPlayer(o.PlayerMasterID); master != nil {
			fdx, fdy := dirToOffset(master.Dir)
			if newX == master.CurrX+fdx && newY == master.CurrY+fdy {
				return false
			}
		}
	}
	return o.WalkTo(dir)
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

func (o *MonsterObject) attackMonsterTarget(server *netserver.TCPServer, target *MonsterObject, now int64) {
	dx := abs(target.CurrX - o.CurrX)
	dy := abs(target.CurrY - o.CurrY)
	dist := dx
	if dy > dist {
		dist = dy
	}

	if dist <= 1 {
		if now-o.HitTick > o.AttackSpeed {
			o.HitTick = now
			o.Dir = dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			spd := target.SpeedPoint
			if spd < 1 {
				spd = 1
			}
			if rand.Intn(spd) < o.HitPoint {
				damage := o.calcMonsterDamage(target.BaseObject)
				if int(target.WAbil.HP) <= damage {
					target.WAbil.HP = 0
				} else {
					target.WAbil.HP -= uint16(damage)
				}
				target.LastHiterID = o.ID
				target.LastHiterTick = now
				if target.WAbil.HP == 0 {
					target.Death = true
					target.DeathTick = now
					o.envir.broadcastDeathMsg(target.BaseObject, target.ID, target.CurrX, target.CurrY, target.Dir, true)
				}
			}
			o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
			o.FocusTick = now
		}
	} else {
		o.chaseTarget(target.BaseObject, now)
	}
}
