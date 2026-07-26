package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type PlayerMagic struct {
	MagID int  `json:"magId"`
	Level int  `json:"level"`
	Key   byte `json:"key"`
}

func (p *PlayObject) HandleSpellFull(msg SendMessage, server *netserver.TCPServer) {
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
		log.Logf(log.LevelInfo, "Magic", "%s activated magic shield", p.Name)
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
	case 2, 29:
		p.healTarget(server, power, tx, ty)
	case 6, 34:
		p.doSpellDamageAt(server, power/2, tx, ty)
	case 17, 30:
		log.Logf(log.LevelInfo, "Magic", "%s summoned a creature", p.Name)
	case 18, 19:
		log.Logf(log.LevelInfo, "Magic", "%s became invisible", p.Name)
	}
	p.sendMagicFire(server, magID, tx, ty)
}

func (p *PlayObject) doSpellDamageAt(server *netserver.TCPServer, damage, tx, ty int) {
	if p.envir == nil {
		return
	}
	target := p.findAttackTarget(tx, ty)
	if target != nil {
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
