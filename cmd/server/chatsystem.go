package main

import (
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func (p *PlayObject) HandleSay(msg SendMessage, server *netserver.TCPServer) {
	text := msg.Msg
	if text == "" {
		return
	}

	if strings.HasPrefix(text, "@") {
		p.HandleGMCommand(text[1:], server)
		return
	}

	if p.envir == nil {
		return
	}

	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	for _, obj := range objs {
		other, ok := obj.(*PlayObject)
		if !ok || other.Ghost {
			continue
		}
		hearMsg := protocol.MakeDefaultMsg(protocol.SMHear, p.ID, 0, 0, 0)
		body := protocol.EncodeString(p.Name + "/" + text)
		server.Send(other.Session.ID, hearMsg, body)
	}
	log.Logf(log.LevelInfo, "Chat", "%s: %s", p.Name, text)
}

type Party struct {
	Leader  int32
	Members []int32
}

func (p *PlayObject) HandleCreateGroup(msg SendMessage, server *netserver.TCPServer) {
	targetID := int32(msg.Param1)

	party := &Party{
		Leader:  p.ID,
		Members: []int32{p.ID},
	}

	if targetID != 0 && p.Engine != nil {
		target := p.Engine.GetPlayer(targetID)
		if target != nil {
			party.Members = append(party.Members, targetID)
			grpMsg := protocol.MakeDefaultMsg(protocol.SMGroupMembers, p.ID, 0, 0, 0)
			server.Send(target.Session.ID, grpMsg, protocol.EncodeString(p.Name))
		}
	}

	if p.Engine != nil {
		p.Engine.Parties[p.ID] = party
	}

	resp := protocol.MakeDefaultMsg(protocol.SMCreateGroupOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")

	log.Logf(log.LevelInfo, "Party", "%s created a party", p.Name)
}
