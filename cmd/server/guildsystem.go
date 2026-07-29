package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
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

// SaveGuilds 将内存中全部行会持久化到 SQLite。
func (ue *UserEngine) SaveGuilds() {
	for _, g := range ue.Guilds {
		members := make([]storage.GuildMember, 0, len(g.Members))
		for _, name := range g.Members {
			members = append(members, storage.GuildMember{Name: name, Rank: g.GetRank(name)})
		}
		if err := ue.db.SaveGuild(g.Name, g.Leader, g.Notice, members); err != nil {
			log.Logf(log.LevelError, "Guild", "failed to save guild %s: %v", g.Name, err)
		}
	}
}

// LoadGuilds 在启动时从 SQLite 恢复全部行会到内存。
func (ue *UserEngine) LoadGuilds() {
	records, err := ue.db.LoadGuilds()
	if err != nil {
		log.Logf(log.LevelError, "Guild", "failed to load guilds: %v", err)
		return
	}
	for _, rec := range records {
		g := &Guild{
			Name:   rec.Name,
			Leader: rec.Master,
			Notice: rec.Notice,
			Ranks:  make(map[string]string, len(rec.Members)),
		}
		for _, m := range rec.Members {
			g.Members = append(g.Members, m.Name)
			g.Ranks[m.Name] = m.Rank
		}
		ue.Guilds = append(ue.Guilds, g)
	}
	log.Logf(log.LevelInfo, "Guild", "loaded %d guilds from database", len(ue.Guilds))
}

// HandleOpenGuildDlg 响应 CMOpenGuildDlg，返回行会概览
//（Delphi SMOpenGuildDlg；旧版曾在此处误创建行会）。
func (p *PlayObject) HandleOpenGuildDlg(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMOpenGuildDlgFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	rank := guild.GetRank(p.Name)
	perm := 0
	if guild.Leader == p.Name || rank == "掌门人" || rank == "长老" {
		perm = 1
	}
	p.GuildName = guild.Name
	p.GuildRank = rank
	body := guild.Name + "\n" + rank + "\n" + strconv.Itoa(perm) + "\n" + guild.Notice
	resp := protocol.MakeDefaultMsg(protocol.SMOpenGuildDlg, int32(len(guild.Members)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(body))
}

// HandleGuildMemberListRequest 响应 CMGuildMemberList，
// 返回 "名字/职位/在线" 格式的行（Delphi 成员列表内容）。
func (p *PlayObject) HandleGuildMemberListRequest(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil {
		return
	}
	var sb strings.Builder
	for i, name := range guild.Members {
		if i > 0 {
			sb.WriteByte('\n')
		}
		online := 0
		if p.Engine.GetPlayerByName(name) != nil {
			online = 1
		}
		sb.WriteString(name)
		sb.WriteByte('/')
		sb.WriteString(guild.GetRank(name))
		sb.WriteByte('/')
		sb.WriteString(strconv.Itoa(online))
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGuildMemberList, int32(len(guild.Members)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
}

// HandleGuildUpdateRankInfo 存储来自职位编辑器的 "名字/职位" 行。
func (p *PlayObject) HandleGuildUpdateRankInfo(msg SendMessage, server *netserver.TCPServer) {
	guild := p.Engine.PlayerGuild(p.Name)
	if guild == nil || guild.Leader != p.Name {
		resp := protocol.MakeDefaultMsg(protocol.SMGuildRankUpdateFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if guild.Ranks == nil {
		guild.Ranks = make(map[string]string)
	}
	for _, line := range strings.Split(msg.Msg, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "/", 2)
		if len(parts) == 2 && parts[0] != "" && guild.IsMember(parts[0]) {
			guild.Ranks[parts[0]] = parts[1]
		}
	}
	confirm := protocol.MakeDefaultMsg(protocol.SMDlgMsg, 0, 0, 0, 0)
	server.Send(p.Session.ID, confirm, protocol.EncodeString("Rank info updated"))
}

// HandleGuildHome 是占位实现，等待行会地图功能就绪。
func (p *PlayObject) HandleGuildHome(msg SendMessage, server *netserver.TCPServer) {
	confirm := protocol.MakeDefaultMsg(protocol.SMDlgMsg, 0, 0, 0, 0)
	server.Send(p.Session.ID, confirm, protocol.EncodeString("Guild home is not configured"))
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
	p.Engine.SaveGuilds()
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
	log.Logf(log.LevelInfo, "Guild", "%s allied guild %s with %s", p.Name, guild.Name, targetGuildName)
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
	log.Logf(log.LevelInfo, "Guild", "%s updated notice for guild %s", p.Name, guild.Name)
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
	log.Logf(log.LevelInfo, "Guild", "guild war declared: %s vs %s", guild.Name, targetGuildName)
}

