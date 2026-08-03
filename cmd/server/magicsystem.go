package main

import (
	"encoding/binary"
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type PlayerMagic struct {
	MagID      int  `json:"magId"`
	Level      int  `json:"level"`
	Key        byte `json:"key"`
	TrainPoint int  `json:"trainPoint"`
}

// amuletSkills 需要消耗符咒的魔法（Delphi Magic.pas:420-488 符咒组 +
// 无极真气 CheckAmulet，Magic.pas:685）。34 双龙斩为被动切换技、
// 48 气功波（EnergyRepulsor）均不耗符，不在此表。
var amuletSkills = map[int]int{
	6: 1, 13: 1, 15: 1, 16: 1, 17: 1, 18: 1, 19: 1,
	20: 1, 29: 1, 30: 1, 32: 1, 50: 1,
}

func (p *PlayObject) HandleSpellFull(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanCast() {
		p.sendMagicFail(server)
		return
	}
	// Delphi: 受击硬直阻止施法 (ObjBase.pas:25234)
	now := time.Now().UnixMilli()
	if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
		p.sendMagicFail(server)
		return
	}
	if !p.checkActionSpeed(now, p.Engine.Config.GetSpellInterval(), &p.SpellTick, server) {
		return
	}

	magID := msg.Param1
	targetX := msg.Param2
	targetY := msg.Param3

	pm := p.findMagic(magID)
	if pm == nil {
		p.sendMagicFail(server)
		return
	}

	if p.MagicDB == nil {
		p.sendMagicFail(server)
		return
	}
	def := p.MagicDB.GetByID(magID)
	if def == nil {
		p.sendMagicFail(server)
		return
	}

	if int(p.WAbil.MP) < def.Spell {
		p.sendMagicFail(server)
		return
	}

	if def.Job == 2 {
		if need := amuletSkills[magID]; need > 0 && !p.checkAmulet(need) {
			p.sendMagicFail(server)
			return
		}
	}

	// Delphi: 魔法射程校验 abs(我-目标) <= nMagicAttackRage (Magic.pas:266)
	dx := targetX - p.CurrX
	dy := targetY - p.CurrY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		if dx > p.Engine.Config.GetMagicAttackRange() {
			p.sendMagicFail(server)
			return
		}
	} else {
		if dy > p.Engine.Config.GetMagicAttackRange() {
			p.sendMagicFail(server)
			return
		}
	}

	p.WAbil.MP -= uint16(def.Spell)
	p.sendHealthSpell(server)

	p.SendRefMsg(RM_SPELL, magID, p.CurrX, p.CurrY, "")

	power := def.Power + pm.Level*(def.MaxPower-def.Power)/3
	if power < 1 {
		power = def.Power
	}

	switch def.Job {
	case 0:
		p.castWarriorSpell(server, magID, power, targetX, targetY)
	case 1:
		p.castMageSpell(server, magID, power, targetX, targetY)
	case 2:
		p.castTaoistSpell(server, magID, power, targetX, targetY)
		if need := amuletSkills[magID]; need > 0 {
			p.useAmulet(need)
		}
	}

	p.trainSkill(magID)

	log.Logf(log.LevelInfo, "Magic", "%s cast %s (id=%d, power=%d)", p.Name, def.MagName, magID, power)
}

func (p *PlayObject) castWarriorSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	switch magID {
	case 3:
	case 4:
		p.doSpellDamageToAdjacent(server, power)
	case 7:
		p.doSpellDamageArea(server, power, 1)
	case 12:
		dx, dy := dirToOffset(p.Dir)
		p.doSpellDamageAt(server, power, p.CurrX+dx, p.CurrY+dy)
		p.doSpellDamageAt(server, power, p.CurrX+dx*2, p.CurrY+dy*2)
	case 25, 26:
		p.doSpellDamageToAdjacent(server, power*2)
	case 27: // 冲撞（RushKung）— F2: 含目标推动/伤害/回弹
		dx, dy := dirToOffset(p.Dir)
		if p.envir == nil {
			break
		}
		// 检测路径上的目标（1-3格）
		var target interface{}
		var targetDist int
		for dist := 1; dist <= 3; dist++ {
			tx, ty := p.CurrX+dx*dist, p.CurrY+dy*dist
			obj := p.envir.GetMovingObject(tx, ty)
			if obj != nil {
				target = obj
				targetDist = dist
				break
			}
		}
		if target == nil {
			// 无目标：直接冲3格
			rushDist := 3
			for d := 3; d >= 1; d-- {
				if p.envir.CanWalk(p.CurrX+dx*d, p.CurrY+dy*d) {
					rushDist = d
					break
				}
			}
			rushX := p.CurrX + dx*rushDist
			rushY := p.CurrY + dy*rushDist
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.CurrX, p.CurrY = rushX, rushY
			p.envir.AddObject(rushX, rushY, OS_MOVINGOBJECT, p)
			p.SendRefMsg(RM_RUSHKUNG, p.Dir, p.CurrX, p.CurrY, "")
		} else {
			// 有目标：尝试推动目标
			pushX := p.CurrX + dx*(targetDist+1)
			pushY := p.CurrY + dy*(targetDist+1)
			pushed := false
			if p.envir.CanWalk(pushX, pushY) {
				// 推动目标
				switch t := target.(type) {
				case *MonsterObject:
					if !t.Death && !t.IsSafeZoneGuard {
						p.envir.RemoveObject(t.CurrX, t.CurrY, OS_MOVINGOBJECT, t)
						t.CurrX, t.CurrY = pushX, pushY
						p.envir.AddObject(pushX, pushY, OS_MOVINGOBJECT, t)
						p.envir.broadcastRefMsg(t.BaseObject, RM_WALK, t.ID, pushX, pushY, p.Dir)
						// 伤害
						dmg := rand.Intn(power+1) + power/2
						t.WAbil.HP -= uint16(min(int(t.WAbil.HP), dmg))
						pushed = true
					}
				case *PlayObject:
					if !t.Ghost && !t.Death && p.CanAttackTarget(t.BaseObject) {
						p.envir.RemoveObject(t.CurrX, t.CurrY, OS_MOVINGOBJECT, t)
						t.CurrX, t.CurrY = pushX, pushY
						p.envir.AddObject(pushX, pushY, OS_MOVINGOBJECT, t)
						p.envir.broadcastRefMsg(t.BaseObject, RM_WALK, t.ID, pushX, pushY, p.Dir)
						pushed = true
					}
				}
			}
			if pushed {
				// 冲到目标位置
				rushX := p.CurrX + dx*targetDist
				rushY := p.CurrY + dy*targetDist
				p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
				p.CurrX, p.CurrY = rushX, rushY
				p.envir.AddObject(rushX, rushY, OS_MOVINGOBJECT, p)
				p.SendRefMsg(RM_RUSHKUNG, p.Dir, p.CurrX, p.CurrY, "")
			} else {
				// 被阻挡：回弹1格
				backX := p.CurrX - dx
				backY := p.CurrY - dy
				if p.envir.CanWalk(backX, backY) {
					p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
					p.CurrX, p.CurrY = backX, backY
					p.envir.AddObject(backX, backY, OS_MOVINGOBJECT, p)
				}
				// 回弹消息（Param=8 表示碰撞回弹）
				p.SendRefMsg(RM_RUSHKUNG, 8, p.CurrX, p.CurrY, "")
			}
		}
	case 35: // Wind Tebo: push facing target
		dx, dy := dirToOffset(p.Dir)
		obj := p.envir.GetMovingObject(p.CurrX+dx, p.CurrY+dy)
		if obj != nil {
			if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
				if int(p.WAbil.Level) > int(mon.WAbil.Level) && rand.Intn(20) < 12 {
					pushX := mon.CurrX + dx*2
					pushY := mon.CurrY + dy*2
					if p.envir.CanWalk(pushX, pushY) {
						p.envir.RemoveObject(mon.CurrX, mon.CurrY, OS_MOVINGOBJECT, mon)
						mon.CurrX, mon.CurrY = pushX, pushY
						p.envir.AddObject(pushX, pushY, OS_MOVINGOBJECT, mon)
						mon.SendRefMsg(RM_WALK, mon.Dir, pushX, pushY, "")
					}
				}
			}
		}
	case 39: // Group DeDing: AoE physical around target
		radius := 1
		objs := p.envir.GetRangeObjects(tx, ty, radius)
		for _, obj := range objs {
			switch t := obj.(type) {
			case *MonsterObject:
				if !t.Death && !t.Ghost {
					damage := power
					if rand.Intn(int(t.SpeedPoint)+1) >= p.HitPoint {
						damage = 0
					}
					if damage > 0 {
						p.applyDamage(server, t.BaseObject, damage, p.Dir)
					}
				}
			case *PlayObject:
				if !t.Ghost && !t.Death && t.ID != p.ID {
					damage := power
					p.applyDamage(server, t.BaseObject, damage, p.Dir)
				}
			}
		}
	}
}

func (p *PlayObject) castMageSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	// Delphi: 弹道魔法遮挡检测 (ObjBase.pas:24055-24089)
	switch magID {
	case 1, 5, 10, 11, 44, 45:
		if !p.magCanHitTarget(tx, ty) {
			p.sendMagicFire(server, magID, tx, ty)
			return
		}
	}

	switch magID {
	case 1, 5, 44:
		// Delphi: 弹道魔法飞行延迟 (Magic.pas:285-292, ObjBase.pas:4565-4582)
		p.PendingMagics = append(p.PendingMagics, PendingMagic{
			MagID:    magID,
			Power:    power,
			TargetX:  tx,
			TargetY:  ty,
			FireTick: time.Now().UnixMilli() + p.Engine.Config.GetProjectileDelay(),
		})
	case 10, 11:
		p.doSpellDamageAt(server, power, tx, ty)
	case 9:
		dx, dy := dirToOffset(p.Dir)
		for i := 1; i <= 4; i++ {
			p.doSpellDamageAt(server, power*2/3, p.CurrX+dx*i, p.CurrY+dy*i)
		}
	case 23, 33:
		p.doSpellDamageAreaAt(server, power, tx, ty, 2)
	case 24:
		p.doSpellDamageAreaAt(server, power, tx, ty, 3)
	case 47: // 火龙气焰（Delphi SKILL_47，Magic.pas:665-672）：
		// 目标点 3×3 MC 伤害，半径 nFireBoomRage 默认 1（M2Share.pas:2077）。
		// 伤害公式与爆裂火焰(23)相同，DB 动画号不同。
		p.doSpellDamageAreaAt(server, power, tx, ty, 1)
	case 42:
		objs := p.envir.GetRangeObjects(tx, ty, 3)
		for _, obj := range objs {
			if mon, ok := obj.(*MonsterObject); ok && !mon.Death && !mon.Ghost {
				damage := power * 2 / 3
				p.applyDamage(server, mon.BaseObject, damage, p.Dir)
			}
		}
	case 31:
		p.MakePoison(STATE_BUBBLEDEFENCE, 600, 0)
	case 8:
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				tx2, ty2 := p.CurrX+dx, p.CurrY+dy
				obj := p.envir.GetMovingObject(tx2, ty2)
				if obj == nil {
					continue
				}
				if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
					pushX := mon.CurrX + dx*2
					pushY := mon.CurrY + dy*2
					if p.envir.CanWalk(pushX, pushY) {
						p.envir.RemoveObject(mon.CurrX, mon.CurrY, OS_MOVINGOBJECT, mon)
						mon.CurrX, mon.CurrY = pushX, pushY
						p.envir.AddObject(pushX, pushY, OS_MOVINGOBJECT, mon)
						mon.SendRefMsg(RM_WALK, mon.Dir, pushX, pushY, "")
					}
				}
			}
		}
	case 22:
		fireDuration := p.Engine.Config.GetFireWallDuration()
		perTick := power / int(fireDuration/100)
		if perTick < 1 {
			perTick = 1
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				p.envir.AddFireEvent(server, tx+dx, ty+dy, perTick, fireDuration, p.ID)
			}
		}
	case 21:
		if p.envir != nil && p.envir.CanWalk(tx, ty) {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.CurrX, p.CurrY = tx, ty
			p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.VisibleActors = make(map[int32]*VisibleEntry)
		}
	case 45: // Flame Disruptor: single target with MAC check
		target := p.findAttackTarget(tx, ty)
		if target != nil {
			antiMagic := int(target.WAbil.MAC & 0xFFFF)
			if rand.Intn(10) >= antiMagic {
				damage := power
				if mon, ok := p.envir.GetMovingObject(tx, ty).(*MonsterObject); ok && mon.LifeAttrib == LA_UNDEAD {
					damage = damage * 3 / 2
				}
				p.applyDamage(server, target, damage, p.Dir)
			}
		}
	case 46: // Mirroring: spawn a clone monster that fights for the player
		cloneX := p.CurrX + 1
		cloneY := p.CurrY
		if !p.envir.CanWalk(cloneX, cloneY) {
			cloneX = p.CurrX - 1
		}
		if p.envir.CanWalk(cloneX, cloneY) {
			p.cleanSlaveList()
			if len(p.SlaveIDs) < MaxSlaveCount {
				cloneHP := int(p.WAbil.MaxHP) / 2
				clone := NewMonsterObject(p.Name+"(影)", p.Engine.nextMonsterID, 19, 11, 160, cloneHP, 400, 1000, 0)
				p.Engine.nextMonsterID++
				clone.MapName = p.MapName
				clone.CurrX = cloneX
				clone.CurrY = cloneY
				clone.envir = p.envir
				clone.WAbil.Level = p.WAbil.Level
				clone.WAbil.DC = p.WAbil.DC / 2
				clone.WAbil.MaxHP = uint16(cloneHP)
				clone.WAbil.HP = uint16(cloneHP)
				clone.initAITimers(time.Now().UnixMilli(), p.Engine.Config)
				clone.PlayerMasterID = p.ID
				p.envir.AddObject(cloneX, cloneY, OS_MOVINGOBJECT, clone)
				p.Engine.Monsters = append(p.Engine.Monsters, clone)
				clone.SendRefMsg(RM_TURN, clone.Dir, cloneX, cloneY, clone.Name)
				p.addSlave(clone.ID)
			}
		}
	}
	p.sendMagicFire(server, magID, tx, ty)
}

func (p *PlayObject) castTaoistSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	switch magID {
	case 2:
		p.healTarget(server, power, tx, ty)
	case 29:
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				heal := power
				hp := int(other.WAbil.HP) + heal
				if hp > int(other.WAbil.MaxHP) {
					hp = int(other.WAbil.MaxHP)
				}
				other.WAbil.HP = uint16(hp)
				other.sendHealthSpell(server)
			}
		}
	case 6:
		target := p.findAttackTarget(tx, ty)
		if target != nil {
			if tp := p.envir.getPlayerByBase(target); tp != nil {
				tp.MakePoison(POISON_DECHEALTH, 100, 4)
			} else {
				target.StatusTimeArr[POISON_DECHEALTH] = 100
			}
		}
	case 13:
		target := p.findAttackTarget(tx, ty)
		if target != nil {
			damage := power
			antiMagic := int(target.WAbil.MAC & 0xFFFF)
			if antiMagic > 0 && rand.Intn(100) < antiMagic {
				damage = damage / 2
			}
			p.applyDamage(server, target, damage, p.Dir)
		}
	case 14: // 圣光结界：在目标位置创建圣幕，伤害亡灵
		if p.envir != nil {
			damage := power / 2
			if damage < 10 {
				damage = 10
			}
			p.envir.AddHolyCurtainEvent(server, tx, ty, damage, 8000, p.ID)
		}
	case 15: // 神圣战甲术（Delphi SKILL_DEJIWONHO，Magic.pas:456-460）：
		// 目标点 7×7 友方物理防御提升。持续 = GetPower13(60)≈60s + SC下限×10
		//（MagMakeDefenceArea+DefenceUp，ObjBase.pas:24207-24275）。
		scLo := int(p.WAbil.SC & 0xFFFF)
		duration := int16((60 + scLo*10) * 10) // 秒 → ticks（100ms）
		objs := p.envir.GetRangeObjects(tx, ty, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				other.MakePoison(STATE_DEFENCEUP, duration, 0)
				other.RecalcAbilitys()
				other.SendAbility(server)
			}
		}
	case 16:
		// Delphi: 幽灵盾 — STATE_DEFENCEUP + STATE_MAGDEFENCEUP (ObjBase.pas:24255, 24309)
		lv := 0
		if m := p.findMagic(16); m != nil {
			lv = m.Level
		}
		duration := int16(30 + lv*15) // 30/45/60/75 秒
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				other.MakePoison(STATE_DEFENCEUP, duration, 0)
				other.MakePoison(STATE_MAGDEFENCEUP, duration, 0)
				other.RecalcAbilitys()
				other.SendAbility(server)
			}
		}
	case 17, 30:
		p.cleanSlaveList()
		if len(p.SlaveIDs) >= MaxSlaveCount {
			p.sendMagicFail(server)
			return
		}
		petX := p.CurrX + 1
		petY := p.CurrY
		if !p.envir.CanWalk(petX, petY) {
			petX = p.CurrX - 1
		}
		if p.envir.CanWalk(petX, petY) {
			cfg := p.Engine.Config
			petName := "骷髅"
			if magID == 30 {
				petName = "神兽"
			}
			petHP := cfg.GetSummonPetHPBase() + int(p.WAbil.Level)*cfg.GetSummonPetHPPerLv()
			pet := NewMonsterObject(petName, p.Engine.nextMonsterID, 19, 11, 160, petHP, 400, 1000, 0)
			p.Engine.nextMonsterID++
			pet.MapName = p.MapName
			pet.CurrX = petX
			pet.CurrY = petY
			pet.envir = p.envir
			pet.WAbil.Level = p.WAbil.Level
			pet.WAbil.DC = uint32(cfg.GetSummonPetDCBase() + int(p.WAbil.Level)/cfg.GetSummonPetDCPerLv())
			pet.WAbil.MaxHP = uint16(petHP)
			pet.WAbil.HP = uint16(petHP)
			pet.initAITimers(time.Now().UnixMilli(), cfg)
			pet.PlayerMasterID = p.ID
			p.envir.AddObject(petX, petY, OS_MOVINGOBJECT, pet)
			p.Engine.Monsters = append(p.Engine.Monsters, pet)
			pet.SendRefMsg(RM_TURN, pet.Dir, petX, petY, petName)
			p.addSlave(pet.ID)
		}
	case 18:
		p.MakePoison(STATE_TRANSPARENT, 300, 0)
	case 19:
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				other.MakePoison(STATE_TRANSPARENT, 300, 0)
			}
		}
	case 20:
		target := p.envir.GetMovingObject(tx, ty)
		if target != nil {
			if mon, ok := target.(*MonsterObject); ok && !mon.Death {
				if int(mon.WAbil.HP) < mon.MaxHP/3 && rand.Intn(100) < 30+int(p.WAbil.Level) {
					mon.TargetID = 0
					mon.LastHiterID = 0
					mon.PlayerMasterID = p.ID
					p.addSlave(mon.ID)
					log.Logf(log.LevelInfo, "Magic", "%s tamed %s", p.Name, mon.Name)
				}
			}
		}
	case 32:
		target := p.envir.GetMovingObject(tx, ty)
		if target != nil {
			if mon, ok := target.(*MonsterObject); ok && !mon.Death {
				if mon.Race >= 10 && mon.Race <= 19 {
					mon.Death = true
					mon.DeathTick = time.Now().UnixMilli()
					mon.WAbil.HP = 0
					p.envir.broadcastDeathMsg(mon.BaseObject, mon.ID, mon.CurrX, mon.CurrY, mon.Dir, true)
					p.awardExp(server, mon)
				} else {
					damage := power * 3
					p.applyDamage(server, mon.BaseObject, damage, p.Dir)
				}
			}
		}
	case 48: // 气功波（DB magId 48；Delphi SKILL_ENERGYREPULSOR=37，Magic.pas）：
		// 以自身为中心推开周围怪物。
		for dy2 := -1; dy2 <= 1; dy2++ {
			for dx2 := -1; dx2 <= 1; dx2++ {
				if dx2 == 0 && dy2 == 0 {
					continue
				}
				obj := p.envir.GetMovingObject(p.CurrX+dx2, p.CurrY+dy2)
				if obj == nil {
					continue
				}
				if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
					pushX := mon.CurrX + dx2*3
					pushY := mon.CurrY + dy2*3
					if p.envir.CanWalk(pushX, pushY) {
						p.envir.RemoveObject(mon.CurrX, mon.CurrY, OS_MOVINGOBJECT, mon)
						mon.CurrX, mon.CurrY = pushX, pushY
						p.envir.AddObject(pushX, pushY, OS_MOVINGOBJECT, mon)
						damage := power / 3
						p.applyDamage(server, mon.BaseObject, damage, p.Dir)
					}
				}
			}
		}
	case 28: // Show HP: reveal target's health bar
		target := p.findAttackTarget(tx, ty)
		if target != nil {
			if tp := p.envir.getPlayerByBase(target); tp != nil {
				msg := protocol.MakeDefaultMsg(protocol.SMOpenHealth, tp.ID, uint16(tp.WAbil.HP), uint16(tp.WAbil.MaxHP), 0)
				server.Send(p.Session.ID, msg, protocol.EncodeString(tp.Name))
			} else if mon := p.envir.getMonsterByBase(target); mon != nil {
				msg := protocol.MakeDefaultMsg(protocol.SMOpenHealth, mon.ID, uint16(mon.WAbil.HP), uint16(mon.WAbil.MaxHP), 0)
				server.Send(p.Session.ID, msg, protocol.EncodeString(mon.Name))
			}
		}
	case 49: // 净化术：Delphi SKILL_49 原样为空实现（Magic.pas:678-680，
		// 仅置 boTrain 涨熟练度；RM_SPELL/RM_MAGICFIRE 由框架统一发出）。
	case 50: // 无极真气（DB magId 50；Delphi SKILL_UENHANCER=36，Magic.pas:681-700）：
		// 自身攻击提升，加值 = 等级+1+Random(等级)，持续 = 60 + SC下限×10 秒。
		// Delphi 效果为 DC 高字 +2+value（ObjBase.pas:3424-3425），
		// Go 复用 BuffDC 机制（上下字同时加，闭环近似）。
		lv := 0
		if m := p.findMagic(50); m != nil {
			lv = m.Level
		}
		value := lv + 1
		if lv > 0 {
			value += rand.Intn(lv)
		}
		scLo := int(p.WAbil.SC & 0xFFFF)
		durSec := 60 + scLo*10
		p.BuffDC = 2 + value
		p.BuffExpireTick = time.Now().UnixMilli() + int64(durSec)*1000
		p.RecalcAbilitys()
		p.SendAbility(server)
	case 41: // Summon Angel
		p.cleanSlaveList()
		if len(p.SlaveIDs) >= MaxSlaveCount {
			p.sendMagicFail(server)
			return
		}
		petX := p.CurrX + 1
		petY := p.CurrY
		if !p.envir.CanWalk(petX, petY) {
			petX = p.CurrX - 1
		}
		if p.envir.CanWalk(petX, petY) {
			cfg := p.Engine.Config
			petHP := cfg.GetAngelHPBase() + int(p.WAbil.Level)*cfg.GetAngelHPPerLv()
			pet := NewMonsterObject("天使", p.Engine.nextMonsterID, 19, 11, 160, petHP, 400, 1000, 0)
			p.Engine.nextMonsterID++
			pet.MapName = p.MapName
			pet.CurrX = petX
			pet.CurrY = petY
			pet.envir = p.envir
			pet.WAbil.Level = p.WAbil.Level
			pet.WAbil.DC = uint32(cfg.GetAngelDCBase() + int(p.WAbil.Level)*cfg.GetAngelDCPerLv())
			pet.WAbil.MAC = uint32(3 + int(p.WAbil.Level)/3)
			pet.WAbil.MaxHP = uint16(petHP)
			pet.WAbil.HP = uint16(petHP)
			pet.initAITimers(time.Now().UnixMilli(), cfg)
			pet.PlayerMasterID = p.ID
			p.envir.AddObject(petX, petY, OS_MOVINGOBJECT, pet)
			p.Engine.Monsters = append(p.Engine.Monsters, pet)
			pet.SendRefMsg(RM_TURN, pet.Dir, petX, petY, "天使")
			p.addSlave(pet.ID)
		}
	}
	p.sendMagicFire(server, magID, tx, ty)
}

func (p *PlayObject) doSpellDamageAt(server *netserver.TCPServer, damage, tx, ty int) {
	if p.envir == nil {
		return
	}
	target := p.findAttackTarget(tx, ty)
	if target != nil {
		antiMagic := int(target.WAbil.MAC & 0xFFFF)
		if antiMagic > 0 && rand.Intn(100) < antiMagic {
			damage = damage / 2
		}
		p.applyDamage(server, target, damage, p.Dir)
	}
}

func (p *PlayObject) doSpellDamageToAdjacent(server *netserver.TCPServer, damage int) {
	dx, dy := dirToOffset(p.Dir)
	p.doSpellDamageAt(server, damage, p.CurrX+dx, p.CurrY+dy)
}

func (p *PlayObject) doSpellDamageArea(server *netserver.TCPServer, damage, radius int) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			target := p.findAttackTarget(p.CurrX+dx, p.CurrY+dy)
			if target != nil {
				p.applyDamage(server, target, damage/2, p.Dir)
			}
		}
	}
}

func (p *PlayObject) doSpellDamageAreaAt(server *netserver.TCPServer, damage, cx, cy, radius int) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			target := p.findAttackTarget(cx+dx, cy+dy)
			if target != nil {
				p.applyDamage(server, target, damage/2, p.Dir)
			}
		}
	}
}

func (p *PlayObject) healTarget(server *netserver.TCPServer, amount, tx, ty int) {
	if tx == p.CurrX && ty == p.CurrY {
		cap := p.Engine.Config.GetHealingPoolCap()
		p.IncHealing += amount
		if p.IncHealing > cap {
			p.IncHealing = cap
		}
	}
}

func (p *PlayObject) findMagic(magID int) *PlayerMagic {
	for _, pm := range p.LearnedMagics {
		if pm.MagID == magID {
			return pm
		}
	}
	return nil
}

func (p *PlayObject) learnMagic(magID, level int, key byte) {
	if p.findMagic(magID) != nil {
		return
	}
	p.LearnedMagics = append(p.LearnedMagics, &PlayerMagic{
		MagID: magID,
		Level: level,
		Key:   key,
	})
}

func (p *PlayObject) removeMagic(magID int) {
	for i, pm := range p.LearnedMagics {
		if pm.MagID == magID {
			p.LearnedMagics = append(p.LearnedMagics[:i], p.LearnedMagics[i+1:]...)
			return
		}
	}
}

func (p *PlayObject) sendMagicFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMagicFireFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

// magicEffType 将魔法分类为客户端特效类型，打包到
// SMMagicFire.Series 低字节：0=爆炸, 1=飞行, 2=地面。magic_db 中的
// effectType 字段是 Delphi 的动画集选择器（0..14），和这个枚举不对应，
// 所以这里按 magID 来区分类型；effnum（高字节）直接取自 magic_db 的 "effect"。
var magicEffType = map[int]int{
	1: 1, 5: 1, 10: 1, 11: 1, 13: 1, 44: 1, // projectiles → fly
	22: 2, 23: 2, 24: 2, 33: 2, 47: 2, // area effects → ground
}

func (p *PlayObject) magicEffectParams(magID int) (effType, effNum int) {
	effType = magicEffType[magID]
	if p.MagicDB != nil {
		if def := p.MagicDB.GetByID(magID); def != nil {
			effNum = def.Effect
		}
	}
	return effType, effNum
}

func (p *PlayObject) sendMagicFire(server *netserver.TCPServer, magID, tx, ty int) {
	effType, effNum := p.magicEffectParams(magID)
	series := uint16(effType&0xFF) | uint16(effNum&0xFF)<<8
	resp := protocol.MakeDefaultMsg(protocol.SMMagicFire, int32(magID), uint16(tx), uint16(ty), series)
	server.Send(p.Session.ID, resp, "")
}

// magicMaxTrain 是训练曲线的占位值，等 magic DB 支持
// 每级 MaxTrain 后替换（对应 Delphi Magic.MaxTrain[4]）。
var magicMaxTrain = [4]int{150, 600, 1800, 0}

// encodeMyMagicBody 序列化魔法列表。每个魔法：MagID u16,
// Level u8, Key u8, IconIdx u16 (def.Effect*2 — 对应 Delphi
// MagIcon[btEffect*2]), CurTrain u16, MaxTrain u16, NameLen u8 + Name。
// 客户端对应：GameState.ParseMagics。
func (p *PlayObject) encodeMyMagicBody() string {
	buf := make([]byte, 0, 2+len(p.LearnedMagics)*20)
	var count [2]byte
	binary.LittleEndian.PutUint16(count[:], uint16(len(p.LearnedMagics)))
	buf = append(buf, count[:]...)
	for _, pm := range p.LearnedMagics {
		iconIdx := uint16(0)
		name := ""
		if p.MagicDB != nil {
			if def := p.MagicDB.GetByID(pm.MagID); def != nil {
				iconIdx = uint16(def.Effect * 2)
				name = def.MagName
			}
		}
		lv := pm.Level
		if lv > 3 {
			lv = 3
		}
		// MagID u16, Level u8, Key u8, IconIdx u16, CurTrain u16, MaxTrain u16。
		entry := make([]byte, 10)
		binary.LittleEndian.PutUint16(entry[0:2], uint16(pm.MagID))
		entry[2] = byte(lv)
		entry[3] = pm.Key
		binary.LittleEndian.PutUint16(entry[4:6], iconIdx)
		binary.LittleEndian.PutUint16(entry[6:8], uint16(pm.TrainPoint))
		binary.LittleEndian.PutUint16(entry[8:10], uint16(magicMaxTrain[lv]))
		buf = append(buf, entry...)
		nameBytes := []byte(name)
		if len(nameBytes) > 255 {
			nameBytes = nameBytes[:255]
		}
		buf = append(buf, byte(len(nameBytes)))
		buf = append(buf, nameBytes...)
	}
	return protocol.EncodeBuffer(buf)
}

func (p *PlayObject) SendMyMagicFull(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendMyMagic, int32(len(p.LearnedMagics)), 0, 0, 0)
	server.Send(p.Session.ID, resp, p.encodeMyMagicBody())
}

// HandleMagicKeyChange 重新绑定魔法快捷键。Recog=魔法 id（→Param1），
// Param=按键字节 '1'..'8' 或 0 表示解绑（→Param2）— 参考 ClMain.pas:3086-3092。
func (p *PlayObject) HandleMagicKeyChange(msg SendMessage, server *netserver.TCPServer) {
	magID := msg.Param1
	key := byte(msg.Param2)
	pm := p.findMagic(magID)
	if pm == nil {
		return
	}
	if key != 0 {
		// 解绑当前占用该按键的其他魔法（客户端发送前也会做同样的
		// 处理，参考 ClMain.pas:3522-3528；服务端同样强制执行）。
		for i := range p.LearnedMagics {
			if p.LearnedMagics[i].MagID != magID && p.LearnedMagics[i].Key == key {
				p.LearnedMagics[i].Key = 0
			}
		}
	}
	pm.Key = key
	p.SendMyMagicFull(server)
}

func (p *PlayObject) sendSpellToClient(server *netserver.TCPServer, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSpell, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	desc := p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj))
	magID := msg.Param1
	magIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(magIDBytes, uint32(magID))
	// 追加魔法 Effect 号（对应 Delphi TUseMagicInfo.EffectNumber），供客户端施法身特效路由。
	eff := byte(0)
	if p.MagicDB != nil {
		if def := p.MagicDB.GetByID(magID); def != nil {
			eff = byte(def.Effect)
		}
	}
	raw := append(append(desc, magIDBytes...), eff)
	body := protocol.EncodeBuffer(raw)
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) trainSkill(magID int) {
	cfg := p.Engine.Config
	for _, pm := range p.LearnedMagics {
		if pm.MagID == magID && pm.Level < 3 {
			pm.TrainPoint++
			threshold := cfg.GetTrainThreshold0()
			if pm.Level == 1 {
				threshold = cfg.GetTrainThreshold1()
			}
			if pm.Level == 2 {
				threshold = cfg.GetTrainThreshold2()
			}
			if pm.TrainPoint >= threshold {
				pm.TrainPoint = 0
				pm.Level++
				log.Logf(log.LevelInfo, "Magic", "%s skill %d leveled up to %d", p.Name, magID, pm.Level)
			}
			return
		}
	}
}

func (p *PlayObject) checkAmulet(count int) bool {
	if p.ItemDB == nil {
		return false
	}
	total := 0
	for _, item := range p.ItemList {
		if item == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(item.WIndex))
		if def != nil && def.StdMode == 25 {
			total += int(item.Dura)
		}
	}
	return total >= count
}

func (p *PlayObject) useAmulet(count int) {
	if p.ItemDB == nil {
		return
	}
	remaining := count
	for i := len(p.ItemList) - 1; i >= 0 && remaining > 0; i-- {
		item := p.ItemList[i]
		if item == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(item.WIndex))
		if def == nil || def.StdMode != 25 {
			continue
		}
		use := int(item.Dura)
		if use > remaining {
			use = remaining
		}
		item.Dura -= uint16(use)
		remaining -= use
		if item.Dura == 0 {
			p.ItemList = append(p.ItemList[:i], p.ItemList[i+1:]...)
		}
	}
}

// SendSpecialAttackFlags 根据已学魔法发送特殊攻击标记（ObjBase:8868-9200）。
// MagID 映射：7=攻杀(+PWR) 12=刺杀(+LNG) 25=半月(+WID) 26=烈火(+FIR) 40=双龙(+TWN)
func (p *PlayObject) SendSpecialAttackFlags(server *netserver.TCPServer) {
	has := map[int]bool{}
	for _, pm := range p.LearnedMagics {
		has[pm.MagID] = true
	}
	send := func(tag string) {
		server.SendRaw(p.Session.ID, "#+"+tag+"!")
	}
	if has[7] {
		send("PWR")
	}
	if has[12] {
		send("LNG")
	} else {
		send("ULNG")
	}
	if has[25] {
		send("WID")
	} else {
		send("UWID")
	}
	if has[26] {
		send("FIR")
	} else {
		send("UFIR")
	}
	if has[40] {
		send("TWN")
	} else {
		send("UTWN")
	}
}

// magCanHitTarget 弹道遮挡检测（Delphi MagCanHitTarget, ObjBase.pas:24055-24089）。
// 沿施法者到目标的方向逐格检查，最多 13 格，有墙则不可命中。
func (p *PlayObject) magCanHitTarget(tx, ty int) bool {
	if p.envir == nil {
		return true
	}
	dx := tx - p.CurrX
	dy := ty - p.CurrY
	steps := dx
	if dy < 0 {
		steps = -dy
	}
	if dx < 0 {
		steps = -dx
	}
	if dy > steps {
		steps = dy
	}
	if steps > p.Engine.Config.GetMaxLOSCheck() {
		steps = p.Engine.Config.GetMaxLOSCheck()
	}
	if steps <= 1 {
		return true
	}
	sx := 0
	if dx > 0 {
		sx = 1
	} else if dx < 0 {
		sx = -1
	}
	sy := 0
	if dy > 0 {
		sy = 1
	} else if dy < 0 {
		sy = -1
	}
	cx, cy := p.CurrX, p.CurrY
	for i := 1; i < steps; i++ {
		cx += sx
		cy += sy
		if cx == tx && cy == ty {
			break
		}
		if !p.envir.CanWalkEx(cx, cy, true) {
			return false
		}
	}
	return true
}

// processPendingMagics 处理延迟魔法（弹道飞行到期后结算伤害）。
// Delphi: RM_DELAYMAGIC (ObjBase.pas:4565-4582)
func (p *PlayObject) processPendingMagics(server *netserver.TCPServer, now int64) {
	if len(p.PendingMagics) == 0 {
		return
	}
	remaining := p.PendingMagics[:0]
	for _, pm := range p.PendingMagics {
		if now < pm.FireTick {
			remaining = append(remaining, pm)
			continue
		}
		// Delphi: 到期后校验目标仍在范围内 (ObjBase.pas:4579)
		if p.envir != nil {
			p.doSpellDamageAt(server, pm.Power, pm.TargetX, pm.TargetY)
		}
	}
	p.PendingMagics = remaining
}
