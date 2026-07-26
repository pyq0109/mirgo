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

	log.Logf(log.LevelInfo, "Magic", "%s cast %s (id=%d, power=%d)", p.Name, def.MagName, magID, power)
}

func (p *PlayObject) castWarriorSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	switch magID {
	case 4:
		p.doSpellDamageToAdjacent(server, power)
	case 7, 12:
		p.doSpellDamageArea(server, power, 1)
	case 25, 26:
		p.doSpellDamageToAdjacent(server, power*2)
	}
}

func (p *PlayObject) castMageSpell(server *netserver.TCPServer, magID, power, tx, ty int) {
	switch magID {
	case 1, 5, 44:
		p.doSpellDamageAt(server, power, tx, ty)
	case 10, 11:
		p.doSpellDamageAt(server, power, tx, ty)
	case 23, 33:
		p.doSpellDamageAreaAt(server, power, tx, ty, 2)
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
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				fx, fy := tx+dx, ty+dy
				obj := p.envir.GetMovingObject(fx, fy)
				if obj == nil {
					continue
				}
				if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
					damage := power / 2
					hp := int(mon.WAbil.HP) - damage
					if hp <= 0 {
						mon.Death = true
						mon.DeathTick = time.Now().UnixMilli()
						mon.WAbil.HP = 0
						p.envir.broadcastRefMsg(mon.BaseObject, RM_DEATH, mon.ID, mon.CurrX, mon.CurrY, 0)
						p.awardExp(server, mon)
					} else {
						mon.WAbil.HP = uint16(hp)
					}
				}
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

func (p *PlayObject) sendMagicFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMagicFireFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMagicFire(server *netserver.TCPServer, magID, tx, ty int) {
	resp := protocol.MakeDefaultMsg(protocol.SMMagicFire, int32(magID), uint16(tx), uint16(ty), 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendMyMagicFull(server *netserver.TCPServer) {
	buf := make([]byte, 0, 2+len(p.LearnedMagics)*6)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(p.LearnedMagics)))
	buf = append(buf, count...)
	for _, pm := range p.LearnedMagics {
		entry := make([]byte, 6)
		binary.LittleEndian.PutUint16(entry[0:2], uint16(pm.MagID))
		entry[2] = byte(pm.Level)
		entry[3] = pm.Key
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendMyMagic, int32(len(p.LearnedMagics)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
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
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj)))
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
				log.Logf(log.LevelInfo, "Magic", "%s skill %d leveled up to %d", p.Name, magID, pm.Level)
			}
			return
		}
	}
}
