package main

import (
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/log"
)

// PlayObject represents a player character on the server.
type PlayObject struct {
	*BaseObject

	// Connection
	Session *netserver.Session

	// Account
	AccountName string
	SessionID   int64

	// State
	ReadyToRun bool
}

// NewPlayObject creates a new player object.
func NewPlayObject(session *netserver.Session, name string, id int32) *PlayObject {
	base := NewBaseObject(name, id)
	return &PlayObject{
		BaseObject:  base,
		Session:     session,
		AccountName: session.AccountName,
	}
}

// Operate processes one game tick for this player.
func (p *PlayObject) Operate() {
	// Process messages
	for {
		msg, ok := p.GetMsg()
		if !ok {
			break
		}
		p.ProcessMessage(msg)
	}
}

// ProcessMessage handles a single message.
func (p *PlayObject) ProcessMessage(msg SendMessage) {
	switch msg.Ident {
	case protocol.CMTurn:
		p.HandleTurn(msg)
	case protocol.CMWalk:
		p.HandleWalk(msg)
	case protocol.CMRun:
		p.HandleRun(msg)
	case protocol.CMHit:
		p.HandleHit(msg)
	case protocol.CMSpell:
		p.HandleSpell(msg)
	}
}

// HandleTurn handles a turn request.
func (p *PlayObject) HandleTurn(msg SendMessage) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.TurnTo(int(dir))

	// Broadcast turn to nearby players
	// TODO: Implement broadcasting
	log.Logf(log.LevelDebug, "PlayObject", "%s turned to %d", p.Name, dir)
}

// HandleWalk handles a walk request.
func (p *PlayObject) HandleWalk(msg SendMessage) {
	dir := int(msg.Param1)
	if dir < 0 || dir > 7 {
		return
	}

	if p.WalkTo(dir) {
		// Broadcast walk to nearby players
		// TODO: Implement broadcasting
		log.Logf(log.LevelDebug, "PlayObject", "%s walked to (%d,%d)", p.Name, p.CurrX, p.CurrY)
	}
}

// HandleRun handles a run request.
func (p *PlayObject) HandleRun(msg SendMessage) {
	dir := int(msg.Param1)
	if dir < 0 || dir > 7 {
		return
	}

	// Run = walk 2 tiles
	if p.WalkTo(dir) {
		p.WalkTo(dir)
		log.Logf(log.LevelDebug, "PlayObject", "%s ran to (%d,%d)", p.Name, p.CurrX, p.CurrY)
	}
}

// HandleHit handles an attack request.
func (p *PlayObject) HandleHit(msg SendMessage) {
	// TODO: Implement combat
	log.Logf(log.LevelDebug, "PlayObject", "%s attacked", p.Name)
}

// HandleSpell handles a spell cast request.
func (p *PlayObject) HandleSpell(msg SendMessage) {
	// TODO: Implement magic system
	log.Logf(log.LevelDebug, "PlayObject", "%s cast spell %d", p.Name, msg.Param1)
}

// SendMapInfo sends the current map information to the client.
func (p *PlayObject) SendMapInfo(server *netserver.TCPServer) {
	mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, int32(p.CurrX), uint16(p.CurrY), 0, 0)
	server.Send(p.Session.ID, mapResp, p.MapName)
}

// SendLogon sends the logon message to the client.
func (p *PlayObject) SendLogon(server *netserver.TCPServer) {
	logonResp := protocol.MakeDefaultMsg(protocol.SMLogon, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, logonResp, "")
}

// SendAbility sends the ability information to the client.
func (p *PlayObject) SendAbility(server *netserver.TCPServer) {
	// TODO: Encode and send ability data
	abilResp := protocol.MakeDefaultMsg(protocol.SMAbility, int32(p.WAbil.Level), 0, 0, 0)
	server.Send(p.Session.ID, abilResp, "")
}
