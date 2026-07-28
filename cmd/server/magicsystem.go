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

func (p *PlayObject) HandleSpellFull(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanCast() {
		p.sendMagicFail(server)
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

	p.WAbil.MP -= uint16(def.Spell)
	p.sendHealthSpell(server)

	p.SendRefMsg(RM_SPELL, p.Dir, p.CurrX, p.CurrY, "")

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
	}

	p.trainSkill(magID)

	log.Logf(log.LevelInfo, "Magic", "%s 施放 %s (id=%d, 威力=%d)", p.Name, def.MagName, magID, power)
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
	case 27:
		dx, dy := dirToOffset(p.Dir)
		rushX := p.CurrX + dx*3
		rushY := p.CurrY + dy*3
		if p.envir != nil && p.envir.CanWalk(rushX, rushY) {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.CurrX, p.CurrY = rushX, rushY
			p.envir.AddObject(rushX, rushY, OS_MOVINGOBJECT, p)
			p.SendRefMsg(RM_WALK, p.Dir, p.CurrX, p.CurrY, "")
		}
	case 37:
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
	}
}

func (p *PlayObject) castMageSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	switch magID {
	case 1, 5, 44:
		p.doSpellDamageAt(server, power, tx, ty)
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
	case 42:
		objs := p.envir.GetRangeObjects(tx, ty, 3)
		for _, obj := range objs {
			if mon, ok := obj.(*MonsterObject); ok && !mon.Death && !mon.Ghost {
				damage := power * 2 / 3
				p.applyDamage(server, mon.BaseObject, damage, p.Dir)
			}
		}
	case 31:
		p.MakePoison(STATE_BUBBLEDEFENCE, 600)
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
		const fireDuration = int64(8000)
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
	case 6, 34:
		if magID == 34 {
			target := p.findAttackTarget(tx, ty)
			if target != nil {
				if tp := p.envir.getPlayerByBase(target); tp != nil {
					tp.StatusTimeArr[POISON_DECHEALTH] = 0
					tp.StatusTimeArr[POISON_DAMAGEARMOR] = 0
				}
			}
		} else {
			target := p.findAttackTarget(tx, ty)
			if target != nil {
				if tp := p.envir.getPlayerByBase(target); tp != nil {
					tp.MakePoison(POISON_DECHEALTH, 100)
				} else {
					target.StatusTimeArr[POISON_DECHEALTH] = 100
				}
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
	case 16:
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				other.WAbil.AC += uint32(power / 5)
				other.WAbil.MAC += uint32(power / 5)
			}
		}
	case 17, 30:
		petX := p.CurrX + 1
		petY := p.CurrY
		if !p.envir.CanWalk(petX, petY) {
			petX = p.CurrX - 1
		}
		if p.envir.CanWalk(petX, petY) {
			petName := "骷髅"
			if magID == 30 {
				petName = "神兽"
			}
			petHP := 50 + int(p.WAbil.Level)*5
			pet := NewMonsterObject(petName, p.Engine.nextMonsterID, 19, 11, 160, petHP, 400, 1000, 0)
			p.Engine.nextMonsterID++
			pet.MapName = p.MapName
			pet.CurrX = petX
			pet.CurrY = petY
			pet.envir = p.envir
			pet.WAbil.DC = uint32(5 + int(p.WAbil.Level)/2)
			p.envir.AddObject(petX, petY, OS_MOVINGOBJECT, pet)
			p.Engine.Monsters = append(p.Engine.Monsters, pet)
			pet.SendRefMsg(RM_TURN, pet.Dir, petX, petY, petName)
		}
	case 18:
		p.MakePoison(STATE_TRANSPARENT, 300)
	case 19:
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, 3)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost && !other.Death {
				other.MakePoison(STATE_TRANSPARENT, 300)
			}
		}
	case 20:
		target := p.envir.GetMovingObject(tx, ty)
		if target != nil {
			if mon, ok := target.(*MonsterObject); ok && !mon.Death {
				if int(mon.WAbil.HP) < mon.MaxHP/3 && rand.Intn(100) < 30+int(p.WAbil.Level) {
					mon.TargetID = 0
					mon.LastHiterID = 0
					log.Logf(log.LevelInfo, "Magic", "%s 驯服了 %s", p.Name, mon.Name)
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
	case 48:
		objs := p.envir.GetRangeObjects(tx, ty, 2)
		for _, obj := range objs {
			switch t := obj.(type) {
			case *MonsterObject:
				if !t.Death && !t.Ghost {
					t.StatusTimeArr[POISON_DECHEALTH] = 80
				}
			case *PlayObject:
				if !t.Ghost && !t.Death {
					t.MakePoison(POISON_DECHEALTH, 80)
				}
			}
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
		hp := int(p.WAbil.HP) + amount
		if hp > int(p.WAbil.MaxHP) {
			hp = int(p.WAbil.MaxHP)
		}
		p.WAbil.HP = uint16(hp)
		p.sendHealthSpell(server)
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
	22: 2, 23: 2, 24: 2, 33: 2, // area effects → ground
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
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) trainSkill(magID int) {
	for _, pm := range p.LearnedMagics {
		if pm.MagID == magID && pm.Level < 3 {
			pm.TrainPoint++
			threshold := 20
			if pm.Level == 1 {
				threshold = 50
			}
			if pm.Level == 2 {
				threshold = 100
			}
			if pm.TrainPoint >= threshold {
				pm.TrainPoint = 0
				pm.Level++
				log.Logf(log.LevelInfo, "Magic", "%s 技能 %d 升级到 %d", p.Name, magID, pm.Level)
			}
			return
		}
	}
}
