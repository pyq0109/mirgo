package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	guildBuildCost = 1000000
	guildWarCost   = 500000
	guildWarTime   = int64(10800000)
)

type GuildWar struct {
	GuildName string
	StartTick int64
	EndTick   int64
}

type Guild struct {
	Name       string
	Leader     string
	Members    []string
	Ranks      map[string]string
	Notice     string
	WarGuilds  []GuildWar
	AllyGuilds []string
	BuildPoint int
	AuraePoint int
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

func (g *Guild) IsMember(name string) bool {
	for _, m := range g.Members {
		if m == name {
			return true
		}
	}
	return false
}

func (g *Guild) IsWarGuild(name string) bool {
	now := time.Now().UnixMilli()
	for _, w := range g.WarGuilds {
		if w.GuildName == name && now < w.EndTick {
			return true
		}
	}
	return false
}

func (g *Guild) IsAllyGuild(name string) bool {
	for _, a := range g.AllyGuilds {
		if a == name {
			return true
		}
	}
	return false
}

func (g *Guild) GetRank(name string) string {
	if g.Ranks == nil {
		if name == g.Leader {
			return "掌门人"
		}
		return "成员"
	}
	if r, ok := g.Ranks[name]; ok {
		return r
	}
	return "成员"
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
	guild := &Guild{
		Name:    guildName,
		Leader:  p.Name,
		Members: []string{p.Name},
		Ranks:   map[string]string{p.Name: "掌门人"},
	}
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

func (p *PlayObject) HandleGuildAddMember(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		return
	}
	if p.GuildRank != "掌门人" && p.GuildRank != "副掌门" && p.GuildRank != "长老" {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildAddMemberFail, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetName := msg.Msg
	target := p.Engine.GetPlayerByName(targetName)
	if target == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildAddMemberFail, 2, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if guild.IsMember(targetName) {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildAddMemberFail, 3, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if p.Engine.PlayerGuild(targetName) != nil {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildAddMemberFail, 4, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	guild.Members = append(guild.Members, targetName)
	if guild.Ranks == nil {
		guild.Ranks = make(map[string]string)
	}
	guild.Ranks[targetName] = "成员"
	target.GuildName = guild.Name
	target.GuildRank = "成员"

	resp := protocol.MakeDefaultMsg(protocol.SMGuildAddMemberOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetName))
	notifyResp := protocol.MakeDefaultMsg(protocol.SMChangeGuildName, 0, 0, 0, 0)
	server.Send(target.Session.ID, notifyResp, protocol.EncodeString(guild.Name))

	p.sendGuildMemberList(server, guild)
	log.Logf(log.LevelInfo, "Guild", "%s added %s to guild %s", p.Name, targetName, guild.Name)
}

func (p *PlayObject) HandleGuildDelMember(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		return
	}
	if p.GuildRank != "掌门人" && p.GuildRank != "副掌门" {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildDelMemberFail, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetName := msg.Msg
	if targetName == guild.Leader {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildDelMemberFail, 2, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	found := false
	var remaining []string
	for _, m := range guild.Members {
		if m == targetName {
			found = true
			continue
		}
		remaining = append(remaining, m)
	}
	if !found {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildDelMemberFail, 3, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	guild.Members = remaining
	delete(guild.Ranks, targetName)

	target := p.Engine.GetPlayerByName(targetName)
	if target != nil {
		target.GuildName = ""
		target.GuildRank = ""
		notifyResp := protocol.MakeDefaultMsg(protocol.SMChangeGuildName, 0, 0, 0, 0)
		server.Send(target.Session.ID, notifyResp, protocol.EncodeString(""))
	}

	resp := protocol.MakeDefaultMsg(protocol.SMGuildDelMemberOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetName))
	p.sendGuildMemberList(server, guild)
	log.Logf(log.LevelInfo, "Guild", "%s removed %s from guild %s", p.Name, targetName, guild.Name)
}

func (p *PlayObject) HandleGuildAlly(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil || p.GuildRank != "掌门人" {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildMakeAllyFail, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetGuildName := msg.Msg
	targetGuild := p.Engine.FindGuild(targetGuildName)
	if targetGuild == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildMakeAllyFail, 2, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if guild.IsAllyGuild(targetGuildName) {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildMakeAllyFail, 3, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	guild.AllyGuilds = append(guild.AllyGuilds, targetGuildName)
	targetGuild.AllyGuilds = append(targetGuild.AllyGuilds, guild.Name)

	resp := protocol.MakeDefaultMsg(protocol.SMGuildMakeAllyOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetGuildName))
	log.Logf(log.LevelInfo, "Guild", "%s allied %s with %s", p.Name, guild.Name, targetGuildName)
}

func (p *PlayObject) HandleGuildBreakAlly(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil || p.GuildRank != "掌门人" {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildBreakAllyFail, 1, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetGuildName := msg.Msg
	if !guild.IsAllyGuild(targetGuildName) {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildBreakAllyFail, 2, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	var remaining []string
	for _, a := range guild.AllyGuilds {
		if a != targetGuildName {
			remaining = append(remaining, a)
		}
	}
	guild.AllyGuilds = remaining

	targetGuild := p.Engine.FindGuild(targetGuildName)
	if targetGuild != nil {
		var tRemaining []string
		for _, a := range targetGuild.AllyGuilds {
			if a != guild.Name {
				tRemaining = append(tRemaining, a)
			}
		}
		targetGuild.AllyGuilds = tRemaining
	}

	resp := protocol.MakeDefaultMsg(protocol.SMGuildBreakAllyOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetGuildName))
	log.Logf(log.LevelInfo, "Guild", "%s broke alliance %s - %s", p.Name, guild.Name, targetGuildName)
}

func (p *PlayObject) HandleGuildUpdateNotice(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		return
	}
	if p.GuildRank != "掌门人" && p.GuildRank != "副掌门" {
		return
	}
	guild.Notice = msg.Msg
	log.Logf(log.LevelInfo, "Guild", "%s updated guild notice for %s", p.Name, guild.Name)
}

func (p *PlayObject) sendGuildMemberList(server *netserver.TCPServer, guild *Guild) {
	memberStr := ""
	for i, m := range guild.Members {
		if i > 0 {
			memberStr += "/"
		}
		memberStr += m + "/" + guild.GetRank(m)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGuildMemberList, int32(len(guild.Members)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(memberStr))
}

func (p *PlayObject) HandleGuildWar(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil || p.GuildRank != "掌门人" {
		return
	}
	targetGuildName := msg.Msg
	targetGuild := p.Engine.FindGuild(targetGuildName)
	if targetGuild == nil {
		return
	}
	if p.Gold < guildWarCost {
		return
	}
	p.Gold -= guildWarCost
	now := time.Now().UnixMilli()
	war := GuildWar{GuildName: targetGuildName, StartTick: now, EndTick: now + guildWarTime}
	guild.WarGuilds = append(guild.WarGuilds, war)
	targetGuild.WarGuilds = append(targetGuild.WarGuilds, GuildWar{GuildName: guild.Name, StartTick: now, EndTick: now + guildWarTime})

	sysMsg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
	text := "行会战争开始: " + guild.Name + " vs " + targetGuildName
	for _, memberName := range guild.Members {
		member := p.Engine.GetPlayerByName(memberName)
		if member != nil {
			server.Send(member.Session.ID, sysMsg, protocol.EncodeString(text))
		}
	}
	for _, memberName := range targetGuild.Members {
		member := p.Engine.GetPlayerByName(memberName)
		if member != nil {
			server.Send(member.Session.ID, sysMsg, protocol.EncodeString(text))
		}
	}
	log.Logf(log.LevelInfo, "Guild", "War declared: %s vs %s", guild.Name, targetGuildName)
}

