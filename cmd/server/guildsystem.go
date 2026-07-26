package main

import (
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	guildBuildCost = 1000000
	guildWarCost   = 500000
	guildWarTime   = int64(10800000)
)

type Guild struct {
	Name       string
	Leader     string
	Members    []string
	Notice     string
	WarGuilds  []string
	AllyGuilds []string
}

func (ue *UserEngine) FindGuild(name string) *Guild {
	for _, g := range ue.Guilds {
		if g.Name == name {
			return g
		}
	}
	return nil
}

func (ue *UserEngine) PlayerGuild(name string) *Guild {
	for _, g := range ue.Guilds {
		for _, m := range g.Members {
			if m == name {
				return g
			}
		}
	}
	return nil
}

func (p *PlayObject) HandleBuildGuild(msg SendMessage, server *netserver.TCPServer) {
	guildName := msg.Msg
	if guildName == "" {
		return
	}
	if p.Engine.PlayerGuild(p.Name) != nil {
		resp := protocol.MakeDefaultMsg(protocol.SMBuildGuildFail, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if p.Engine.FindGuild(guildName) != nil {
		resp := protocol.MakeDefaultMsg(protocol.SMBuildGuildFail, 2, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if p.Gold < guildBuildCost {
		resp := protocol.MakeDefaultMsg(protocol.SMBuildGuildFail, 3, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	p.Gold -= guildBuildCost
	guild := &Guild{Name: guildName, Leader: p.Name, Members: []string{p.Name}}
	p.Engine.Guilds = append(p.Engine.Guilds, guild)
	p.GuildName = guildName
	p.GuildRank = "掌门人"
	resp := protocol.MakeDefaultMsg(protocol.SMBuildGuildOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	log.Logf(log.LevelInfo, "Guild", "%s created guild %s", p.Name, guildName)
}

func (p *PlayObject) HandleGuildMessage(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		return
	}
	text := msg.Msg
	for _, memberName := range guild.Members {
		member := p.Engine.GetPlayerByName(memberName)
		if member != nil {
			guildMsg := protocol.MakeDefaultMsg(protocol.SMGuildMessage, p.ID, 0, 0, 0)
			server.Send(member.Session.ID, guildMsg, protocol.EncodeString(p.Name+"/"+text))
		}
	}
}
