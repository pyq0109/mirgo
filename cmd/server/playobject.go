package main

import (
	"encoding/binary"
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

	VisibleActors map[int32]*VisibleEntry
	lastVisionTick int64
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
	case protocol.CMHit:
		p.HandleHit(msg)
	case protocol.CMSpell:
		p.HandleSpell(msg)
	case RM_WALK:
		p.sendMovementToClient(server, protocol.SMWalk, msg)
	case RM_RUN:
		p.sendMovementToClient(server, protocol.SMRun, msg)
	case RM_TURN:
		p.sendTurnToClient(server, msg)
	case RM_DISAPPEAR:
		p.sendDisappearToClient(server, msg)
	}
}

func (p *PlayObject) HandleTurn(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.TurnTo(dir)
	p.SendRefMsg(RM_TURN, dir, p.CurrX, p.CurrY, p.Name)
}

func (p *PlayObject) HandleWalk(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	if p.WalkTo(dir) {
		p.SendRefMsg(RM_WALK, dir, p.CurrX, p.CurrY, "")
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
}

func (p *PlayObject) HandleHit(msg SendMessage) {
	log.Logf(log.LevelDebug, "PlayObject", "%s attacked", p.Name)
}

func (p *PlayObject) HandleSpell(msg SendMessage) {
	log.Logf(log.LevelDebug, "PlayObject", "%s cast spell %d", p.Name, msg.Param1)
}

func (p *PlayObject) sendMoveFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMoveFail, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMovementToClient(server *netserver.TCPServer, smIdent uint16, msg SendMessage) {
	if p.envir == nil {
		return
	}
	src := p.envir.getPlayerByID(msg.SourceID)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(src.BaseObject))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendTurnToClient(server *netserver.TCPServer, msg SendMessage) {
	if p.envir == nil {
		return
	}
	src := p.envir.getPlayerByID(msg.SourceID)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMTurn, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(src.BaseObject))
	if src.Name != "" {
		body += protocol.EncodeString(src.Name)
	}
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendDisappearToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMDisappear, msg.SourceID, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) encodeCharDesc(src *BaseObject) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(src.Feature()))
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
		other, ok := obj.(*PlayObject)
		if !ok || other.ID == p.ID || other.Ghost || other.Death || other.Hidden {
			continue
		}
		if entry, exists := p.VisibleActors[other.ID]; exists {
			entry.Flag = 1
		} else {
			p.VisibleActors[other.ID] = &VisibleEntry{ID: other.ID, Flag: 2}
		}
	}

	for id, entry := range p.VisibleActors {
		switch entry.Flag {
		case 0:
			resp := protocol.MakeDefaultMsg(protocol.SMDisappear, id, 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			delete(p.VisibleActors, id)
		case 2:
			other := p.envir.getPlayerByID(id)
			if other == nil {
				delete(p.VisibleActors, id)
				continue
			}
			resp := protocol.MakeDefaultMsg(protocol.SMTurn, other.ID, uint16(other.CurrX), uint16(other.CurrY), uint16(other.Dir))
			body := protocol.EncodeBuffer(p.encodeCharDesc(other.BaseObject))
			if other.Name != "" {
				body += protocol.EncodeString(other.Name)
			}
			server.Send(p.Session.ID, resp, body)
		}
	}
}

func (p *PlayObject) SendMapInfo(server *netserver.TCPServer) {
	mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, int32(p.CurrX), uint16(p.CurrY), 0, 0)
	server.Send(p.Session.ID, mapResp, p.MapName)
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

func (p *PlayObject) SendMyMagic(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendMyMagic, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendDayChanging(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMDayChanging, 3, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendMapDescription(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMapDescription, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, p.MapName)
}

func (p *PlayObject) SendSubAbility(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSubAbility, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
