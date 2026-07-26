package main

import (
	"encoding/binary"
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type PlayObject struct {
	*BaseObject

	Session     *netserver.Session
	AccountName string
	SessionID   int64
	ReadyToRun  bool
	MapMgr      *MapManager
	ItemDB      *ItemDB
	MagicDB     *MagicDB
	Engine      *UserEngine

	LearnedMagics []*PlayerMagic

	VisibleActors  map[int32]*VisibleEntry
	lastVisionTick int64
	lastRegenTick  int64
	deathTick      int64

	PkPoint         int
	LastPkDecayTick int64
	OnHorse         bool
	HorseType       byte
	Permission      byte

	Deal         *DealState
	GuildName    string
	GuildRank    string
	StorageItems []*protocol.UserItem
}

type VisibleEntry struct {
	ID   int32
	Flag int
}

func NewPlayObject(session *netserver.Session, name string, id int32) *PlayObject {
	base := NewBaseObject(name, id)
	return &PlayObject{
		BaseObject:    base,
		Session:       session,
		AccountName:   session.AccountName,
		VisibleActors: make(map[int32]*VisibleEntry),
	}
}

func (p *PlayObject) Operate(server *netserver.TCPServer) {
	for {
		msg, ok := p.GetMsg()
		if !ok {
			break
		}
		p.ProcessMessage(msg, server)
	}

	now := time.Now().UnixMilli()

	p.DecayPkPoint(now)

	if p.Death {
		if now-p.deathTick > 3000 {
			p.resurrect(server)
		}
		return
	}

	p.Regenerate(server, now)

	if now-p.lastVisionTick >= 1000 {
		p.lastVisionTick = now
		p.SearchViewRange(server)
	}
}

func (p *PlayObject) ProcessMessage(msg SendMessage, server *netserver.TCPServer) {
	switch msg.Ident {
	case protocol.CMTurn:
		p.HandleTurn(msg, server)
	case protocol.CMWalk:
		p.HandleWalk(msg, server)
	case protocol.CMRun:
		p.HandleRun(msg, server)
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit, protocol.CMPowerHit, protocol.CMLongHit, protocol.CMWideHit, protocol.CMFireHit:
		p.HandleHit(msg, server)
	case protocol.CMSpell:
		p.HandleSpellFull(msg, server)
	case protocol.CMPickup:
		p.HandlePickup(msg, server)
	case protocol.CMTakeOnItem:
		p.HandleTakeOnItem(msg, server)
	case protocol.CMTakeOffItem:
		p.HandleTakeOffItem(msg, server)
	case protocol.CMEat:
		p.HandleEatItem(msg, server)
	case protocol.CMSay:
		p.HandleSay(msg, server)
	case protocol.CMClickNPC:
		p.HandleNpcClick(msg, server)
	case protocol.CMCreateGroup:
		p.HandleCreateGroup(msg, server)
	case protocol.CMDealTry:
		p.HandleDealTry(msg, server)
	case protocol.CMDealAddItem:
		p.HandleDealAddItem(msg, server)
	case protocol.CMDealDelItem:
		p.HandleDealCancel(server)
	case protocol.CMDealCancel:
		p.HandleDealCancel(server)
	case protocol.CMDealChgGold:
		p.HandleDealChgGold(msg, server)
	case protocol.CMDealEnd:
		p.HandleDealEnd(server)
	case protocol.CMUserStorageItem:
		p.HandleStorageItem(msg, server)
	case protocol.CMUserTakeBackStorageItem:
		p.HandleTakeBackStorageItem(msg, server)
	case protocol.CMOpenGuildDlg:
		p.HandleBuildGuild(msg, server)
	case protocol.CMHorseRun:
		p.HandleHorseRun(msg, server)
	case RM_WALK:
		p.sendMovementToClient(server, protocol.SMWalk, msg)
	case RM_RUN:
		p.sendMovementToClient(server, protocol.SMRun, msg)
	case RM_TURN:
		p.sendTurnToClient(server, msg)
	case RM_DISAPPEAR:
		p.sendDisappearToClient(server, msg)
	case RM_HIT:
		p.sendHitToClient(server, protocol.SMHit, msg)
	case RM_STRUCK:
		p.sendStruckToClient(server, msg)
	case RM_DEATH:
		p.sendDeathToClient(server, msg)
	case RM_SPELL:
		p.sendSpellToClient(server, msg)
	}
}

func (p *PlayObject) HandleTurn(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.TurnTo(dir)
	p.SendRefMsg(RM_TURN, dir, p.CurrX, p.CurrY, p.Name)
	server.SendRaw(p.Session.ID, "#+GOOD!")
}

func (p *PlayObject) HandleWalk(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	if p.WalkTo(dir) {
		p.SendRefMsg(RM_WALK, dir, p.CurrX, p.CurrY, "")
		server.SendRaw(p.Session.ID, "#+GOOD!")
		p.CheckMapRoute(server)
	} else {
		p.sendMoveFail(server)
	}
}

func (p *PlayObject) HandleRun(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	if !p.WalkTo(dir) {
		p.sendMoveFail(server)
		return
	}
	p.WalkTo(dir)
	p.SendRefMsg(RM_RUN, dir, p.CurrX, p.CurrY, "")
	server.SendRaw(p.Session.ID, "#+GOOD!")
	p.CheckMapRoute(server)
}

func (p *PlayObject) HandleHorseRun(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.OnHorse = true
	moved := false
	for i := 0; i < 3; i++ {
		if !p.WalkTo(dir) {
			break
		}
		moved = true
	}
	if !moved {
		p.sendMoveFail(server)
		return
	}
	p.SendRefMsg(RM_RUN, dir, p.CurrX, p.CurrY, "")
	server.SendRaw(p.Session.ID, "#+GOOD!")
	p.CheckMapRoute(server)
}

func (p *PlayObject) HandleHit(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.Dir = dir
	p.SendRefMsg(RM_HIT, dir, p.CurrX, p.CurrY, "")

	if p.envir == nil {
		return
	}

	if IsSafeZone(p.envir, p.CurrX, p.CurrY) {
		return
	}

	var multiplier float64
	switch msg.Ident {
	case protocol.CMHeavyHit:
		multiplier = 1.5
	case protocol.CMBigHit:
		multiplier = 2.0
	case protocol.CMPowerHit:
		multiplier = 1.3
	case protocol.CMLongHit:
		multiplier = 1.2
	case protocol.CMWideHit:
		multiplier = 1.1
	case protocol.CMFireHit:
		multiplier = 2.5
	default:
		multiplier = 1.0
	}

	dx, dy := dirToOffset(dir)

	if msg.Ident == protocol.CMWideHit {
		targets := p.findWideTargets()
		for _, target := range targets {
			damage := p.calcDamage(target)
			damage = int(float64(damage) * multiplier)
			if damage < 1 {
				damage = 1
			}
			p.applyDamage(server, target, damage, dir)
		}
		return
	}

	target := p.findAttackTarget(p.CurrX+dx, p.CurrY+dy)
	if target == nil && msg.Ident == protocol.CMLongHit {
		target = p.findAttackTarget(p.CurrX+dx*2, p.CurrY+dy*2)
	}
	if target == nil {
		return
	}

	if IsSafeZone(p.envir, target.CurrX, target.CurrY) {
		return
	}

	damage := p.calcDamage(target)
	damage = int(float64(damage) * multiplier)
	if damage < 1 {
		damage = 1
	}
	p.applyDamage(server, target, damage, dir)
}

func (p *PlayObject) findWideTargets() []*BaseObject {
	var targets []*BaseObject
	if p.envir == nil {
		return targets
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := p.CurrX+dx, p.CurrY+dy
			t := p.findAttackTarget(x, y)
			if t != nil && !IsSafeZone(p.envir, t.CurrX, t.CurrY) {
				targets = append(targets, t)
			}
		}
	}
	return targets
}

func (p *PlayObject) findAttackTarget(x, y int) *BaseObject {
	if p.envir == nil {
		return nil
	}
	if x < 0 || x >= p.envir.Width || y < 0 || y >= p.envir.Height {
		return nil
	}
	idx := y*p.envir.Width + x
	for _, o := range p.envir.Cells[idx].ObjList {
		if o.Type != OS_MOVINGOBJECT {
			continue
		}
		switch obj := o.Obj.(type) {
		case *MonsterObject:
			if !obj.Death && !obj.Ghost {
				return obj.BaseObject
			}
		case *PlayObject:
			if obj.ID != p.ID && !obj.Death && !obj.Ghost {
				return obj.BaseObject
			}
		}
	}
	return nil
}

func (p *PlayObject) calcDamage(target *BaseObject) int {
	dc := int(p.WAbil.DC&0xFFFF) + int(p.WAbil.DC>>16)
	ac := int(target.WAbil.AC & 0xFFFF)

	damage := dc - ac
	if damage < 1 {
		damage = 1
	}
	variance := damage / 5
	if variance > 0 {
		damage = damage - variance + rand.Intn(variance*2+1)
	}
	if damage < 1 {
		damage = 1
	}
	return damage
}

func (p *PlayObject) applyDamage(server *netserver.TCPServer, target *BaseObject, damage int, dir int) {
	hp := int(target.WAbil.HP)
	hp -= damage
	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	p.envir.broadcastRefMsg(target, RM_STRUCK, p.ID, target.CurrX, target.CurrY, dir)

	if hp <= 0 {
		target.Death = true
		p.envir.broadcastRefMsg(target, RM_DEATH, target.ID, target.CurrX, target.CurrY, dir)

		if mon := p.envir.getMonsterByBase(target); mon != nil {
			mon.DeathTick = time.Now().UnixMilli()
			p.awardExp(server, mon)
		}
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			tp.deathTick = time.Now().UnixMilli()
			p.OnPlayerKilled(tp)
		}
	} else {
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			tp.sendHealthSpell(server)
		}
	}

	log.Logf(log.LevelInfo, "Combat", "%s hit %s for %d damage (HP: %d/%d)",
		p.Name, target.Name, damage, hp, target.WAbil.MaxHP)
}

func (p *PlayObject) sendHitToClient(server *netserver.TCPServer, smIdent uint16, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendStruckToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMStruck, msg.SourceID, uint16(msg.Param1), uint16(msg.Param2), uint16(msg.Param3))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendDeathToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMDeath, msg.SourceID, uint16(msg.Param1), uint16(msg.Param2), 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) awardExp(server *netserver.TCPServer, mon *MonsterObject) {
	exp := mon.Exp
	if exp <= 0 {
		exp = 10
	}
	p.WAbil.Exp += uint32(exp)

	expMsg := protocol.MakeDefaultMsg(protocol.SMWinExp, int32(exp), 0, 0, 0)
	server.Send(p.Session.ID, expMsg, "")

	maxExp := p.GetMaxExp()
	if p.WAbil.Exp >= maxExp {
		p.WAbil.Exp -= maxExp
		p.WAbil.Level++
		p.WAbil.MaxHP += 15
		p.WAbil.HP = p.WAbil.MaxHP
		p.WAbil.MaxMP += 10
		p.WAbil.MP = p.WAbil.MaxMP

		levelMsg := protocol.MakeDefaultMsg(protocol.SMLevelUp, int32(p.WAbil.Level), 0, 0, 0)
		server.Send(p.Session.ID, levelMsg, "")

		log.Logf(log.LevelInfo, "Combat", "%s leveled up to %d", p.Name, p.WAbil.Level)
	}
}

func (p *PlayObject) GetMaxExp() uint32 {
	level := int(p.WAbil.Level)
	if level <= 0 {
		level = 1
	}
	return uint32(level * level * 100)
}

func (p *PlayObject) sendHealthSpell(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMHealthSpellChanged,
		int32(p.WAbil.HP)<<16|int32(p.WAbil.MP),
		uint16(p.WAbil.MaxHP), uint16(p.WAbil.MaxMP), 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) Regenerate(server *netserver.TCPServer, now int64) {
	if p.Death {
		return
	}
	if now-p.lastRegenTick < 10000 {
		return
	}
	p.lastRegenTick = now

	changed := false
	maxHP := int(p.WAbil.MaxHP)
	maxMP := int(p.WAbil.MaxMP)

	if int(p.WAbil.HP) < maxHP {
		regen := maxHP / 20
		if regen < 1 {
			regen = 1
		}
		hp := int(p.WAbil.HP) + regen
		if hp > maxHP {
			hp = maxHP
		}
		p.WAbil.HP = uint16(hp)
		changed = true
	}

	if int(p.WAbil.MP) < maxMP {
		regen := maxMP / 15
		if regen < 1 {
			regen = 1
		}
		mp := int(p.WAbil.MP) + regen
		if mp > maxMP {
			mp = maxMP
		}
		p.WAbil.MP = uint16(mp)
		changed = true
	}

	if changed {
		p.sendHealthSpell(server)
	}
}

func (p *PlayObject) resurrect(server *netserver.TCPServer) {
	p.Death = false
	p.WAbil.HP = p.WAbil.MaxHP / 2
	p.WAbil.MP = p.WAbil.MaxMP / 2

	resp := protocol.MakeDefaultMsg(protocol.SMAlive, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, resp, "")

	p.SendRefMsg(RM_TURN, p.Dir, p.CurrX, p.CurrY, p.Name)
	p.sendHealthSpell(server)

	log.Logf(log.LevelInfo, "Combat", "%s resurrected at %s(%d,%d)", p.Name, p.MapName, p.CurrX, p.CurrY)
}

func IsSafeZone(envir *Environment, x, y int) bool {
	if envir == nil || envir.Name != "0" {
		return false
	}
	homeX, homeY := 289, 618
	dx := x - homeX
	dy := y - homeY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= 5 && dy <= 5
}

func (p *PlayObject) HandlePickup(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	var item *GroundItem
	for dy := -1; dy <= 1 && item == nil; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if gi := p.envir.GetGroundItemAt(p.CurrX+dx, p.CurrY+dy); gi != nil {
				item = gi
				break
			}
		}
	}
	if item == nil {
		return
	}

	if item.Gold > 0 {
		p.Gold += item.Gold
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		log.Logf(log.LevelInfo, "PlayObject", "%s picked up %d gold (total: %d)", p.Name, item.Gold, p.Gold)
	} else {
		resp := protocol.MakeDefaultMsg(protocol.SMDropItemSuccess, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, protocol.EncodeString(item.Name))
		log.Logf(log.LevelInfo, "PlayObject", "%s picked up %s", p.Name, item.Name)
	}

	p.envir.RemoveGroundItem(item.ID)

	hideResp := protocol.MakeDefaultMsg(protocol.SMItemHide, item.ID, 0, 0, 0)
	objs := p.envir.GetRangeObjects(item.X, item.Y, viewRange)
	for _, obj := range objs {
		op, ok := obj.(*PlayObject)
		if !ok || op.Ghost {
			continue
		}
		server.Send(op.Session.ID, hideResp, "")
	}
}

func (p *PlayObject) sendMoveFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMoveFail, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMovementToClient(server *netserver.TCPServer, smIdent uint16, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendTurnToClient(server *netserver.TCPServer, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMTurn, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj)))
	if src.Name != "" {
		body += protocol.EncodeString(src.Name)
	}
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendDisappearToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMDisappear, msg.SourceID, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) encodeCharDesc(feature int32) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(feature))
	binary.LittleEndian.PutUint32(buf[4:8], 0)
	return buf
}

func (p *PlayObject) SearchViewRange(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	for _, entry := range p.VisibleActors {
		entry.Flag = 0
	}

	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	for _, obj := range objs {
		var id int32
		var skip bool
		switch o := obj.(type) {
		case *PlayObject:
			if o.ID == p.ID || o.Ghost || o.Death || o.Hidden {
				skip = true
			} else {
				id = o.ID
			}
		case *MonsterObject:
			if o.Ghost || o.Death || o.Hidden {
				skip = true
			} else {
				id = o.ID
			}
		case *NpcObject:
			id = o.ID
		default:
			skip = true
		}
		if skip {
			continue
		}
		if entry, exists := p.VisibleActors[id]; exists {
			entry.Flag = 1
		} else {
			p.VisibleActors[id] = &VisibleEntry{ID: id, Flag: 2}
		}
	}

	for id, entry := range p.VisibleActors {
		switch entry.Flag {
		case 0:
			resp := protocol.MakeDefaultMsg(protocol.SMDisappear, id, 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			delete(p.VisibleActors, id)
		case 2:
			obj := p.envir.getObjectByID(id)
			base := objectBase(obj)
			if base == nil {
				delete(p.VisibleActors, id)
				continue
			}
			resp := protocol.MakeDefaultMsg(protocol.SMTurn, base.ID, uint16(base.CurrX), uint16(base.CurrY), uint16(base.Dir))
			body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj)))
			if base.Name != "" {
				body += protocol.EncodeString(base.Name)
			}
			server.Send(p.Session.ID, resp, body)
		}
	}
}

func (p *PlayObject) SendMapInfo(server *netserver.TCPServer) {
	mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, int32(p.CurrX), uint16(p.CurrY), 0, 0)
	server.Send(p.Session.ID, mapResp, protocol.EncodeString(p.MapName))
}

func (p *PlayObject) SendLogon(server *netserver.TCPServer) {
	logonResp := protocol.MakeDefaultMsg(protocol.SMLogon, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	body := p.encodeLogonBody()
	server.Send(p.Session.ID, logonResp, body)
}

func (p *PlayObject) encodeLogonBody() string {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(p.Feature()))
	binary.LittleEndian.PutUint32(buf[4:8], 0)
	binary.LittleEndian.PutUint32(buf[8:12], 0)
	binary.LittleEndian.PutUint32(buf[12:16], 0)
	return protocol.EncodeBuffer(buf)
}

func (p *PlayObject) SendAbility(server *netserver.TCPServer) {
	abilResp := protocol.MakeDefaultMsg(protocol.SMAbility, int32(p.WAbil.Level), 0, 0, 0)
	server.Send(p.Session.ID, abilResp, "")
}

func (p *PlayObject) SendUseItems(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendUseItems, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendDayChanging(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMDayChanging, 3, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendMapDescription(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMapDescription, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(p.MapName))
}

func (p *PlayObject) SendSubAbility(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSubAbility, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) CheckMapRoute(server *netserver.TCPServer) {
	if p.MapMgr == nil {
		return
	}
	route := p.MapMgr.FindRoute(p.MapName, p.CurrX, p.CurrY)
	if route == nil {
		return
	}
	newEnvir := p.MapMgr.FindMap(route.DstMap)
	if newEnvir == nil {
		return
	}
	p.EnterAnotherMap(server, newEnvir, route.DstX, route.DstY)
}

func (p *PlayObject) EnterAnotherMap(server *netserver.TCPServer, newEnvir *Environment, newX, newY int) bool {
	p.Ghost = true
	p.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
	p.Ghost = false

	clearMsg := protocol.MakeDefaultMsg(protocol.SMClearObjects, 0, 0, 0, 0)
	server.Send(p.Session.ID, clearMsg, "")

	if p.envir != nil {
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	}

	p.envir = newEnvir
	p.MapName = newEnvir.Name
	p.CurrX = newX
	p.CurrY = newY

	newEnvir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)

	changeMsg := protocol.MakeDefaultMsg(protocol.SMChangeMap, p.ID, uint16(p.CurrX), uint16(p.CurrY), 0)
	server.Send(p.Session.ID, changeMsg, protocol.EncodeString(p.MapName))

	p.VisibleActors = make(map[int32]*VisibleEntry)

	log.Logf(log.LevelInfo, "PlayObject", "%s entered map %s at (%d,%d)", p.Name, p.MapName, p.CurrX, p.CurrY)
	return true
}
