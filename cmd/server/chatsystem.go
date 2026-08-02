package main

import (
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func (p *PlayObject) HandleSay(msg SendMessage, server *netserver.TCPServer) {
	text := msg.Msg
	if text == "" {
		return
	}

	if p.ShutupTick > 0 && time.Now().UnixMilli() < p.ShutupTick {
		p.sysMsg(server, "你已被禁言")
		return
	}

	if strings.HasPrefix(text, "@") {
		p.HandleGMCommand(text[1:], server)
		return
	}

	if strings.HasPrefix(text, "!~") {
		if guildText := text[2:]; guildText != "" {
			gmsg := msg
			gmsg.Msg = guildText
			p.HandleGuildMessage(gmsg, server)
		}
		return
	}

	if strings.HasPrefix(text, "!") {
		if cryText := text[1:]; cryText != "" {
			p.HandleCry(cryText, server)
		}
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

// HandleCry 喊话：广播到同地图所有玩家。
func (p *PlayObject) HandleCry(text string, server *netserver.TCPServer) {
	if p.Engine == nil {
		return
	}
	cryMsg := protocol.MakeDefaultMsg(protocol.SMCry, p.ID, 0, 0, 0)
	body := protocol.EncodeString(p.Name + "/" + text)
	p.Engine.mu.RLock()
	for _, other := range p.Engine.PlayObjectList {
		if other.Ghost || other.envir != p.envir {
			continue
		}
		server.Send(other.Session.ID, cryMsg, body)
	}
	p.Engine.mu.RUnlock()
	log.Logf(log.LevelInfo, "Chat", "%s cries: %s", p.Name, text)
}

// HandleWhisper 私聊：body 格式为 "目标名/消息内容"。
func (p *PlayObject) HandleWhisper(msg SendMessage, server *netserver.TCPServer) {
	body := msg.Msg
	slashIdx := strings.IndexByte(body, '/')
	if slashIdx <= 0 || slashIdx >= len(body)-1 {
		return
	}
	targetName := body[:slashIdx]
	text := body[slashIdx+1:]

	if p.Engine == nil {
		return
	}
	target := p.Engine.GetPlayerByName(targetName)
	if target == nil || target.Ghost {
		p.sysMsg(server, "对方不在线")
		return
	}

	whisperMsg := protocol.MakeDefaultMsg(protocol.SMWhisper, p.ID, 0, 0, 0)
	// 发给目标：显示发送者名字
	server.Send(target.Session.ID, whisperMsg, protocol.EncodeString(p.Name+"/"+text))
	// 发回给自己：显示目标名字
	server.Send(p.Session.ID, whisperMsg, protocol.EncodeString(targetName+"/"+text))
	log.Logf(log.LevelInfo, "Chat", "%s whispers to %s: %s", p.Name, targetName, text)
}

type Party struct {
	Leader  int32
	Members []int32
}

// partyOf 返回玩家所在的队伍（不在任何队伍中则返回 nil）。
func (p *PlayObject) partyOf() *Party {
	if p.Engine == nil {
		return nil
	}
	for _, party := range p.Engine.Parties {
		for _, id := range party.Members {
			if id == p.ID {
				return party
			}
		}
	}
	return nil
}

// broadcastPartyMembers 向所有成员发送完整成员名列表
//（客户端 GroupMembers 由该消息重建；队长在前，与 Delphi 顺序一致）。
func (p *PlayObject) broadcastPartyMembers(party *Party, server *netserver.TCPServer) {
	var sb strings.Builder
	for i, id := range party.Members {
		if i > 0 {
			sb.WriteByte('\n')
		}
		name := "?"
		if m := p.Engine.GetPlayer(id); m != nil {
			name = m.Name
		}
		sb.WriteString(name)
	}
	body := protocol.EncodeString(sb.String())
	for _, id := range party.Members {
		if m := p.Engine.GetPlayer(id); m != nil {
			resp := protocol.MakeDefaultMsg(protocol.SMGroupMembers, party.Leader, 0, 0, 0)
			server.Send(m.Session.ID, resp, body)
		}
	}
}

// HandleCreateGroup 创建队伍，并可选邀请指定玩家
//（Delphi SendCreateGroup 携带名称，FState:5523-5536）。
func (p *PlayObject) HandleCreateGroup(msg SendMessage, server *netserver.TCPServer) {
	if p.partyOf() != nil || p.Engine == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMCreateGroupFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	party := &Party{
		Leader:  p.ID,
		Members: []int32{p.ID},
	}
	if targetName := strings.TrimSpace(msg.Msg); targetName != "" {
		target := p.Engine.GetPlayerByName(targetName)
		if target == nil || !target.AllowGroup || target.partyOf() != nil {
			resp := protocol.MakeDefaultMsg(protocol.SMCreateGroupFail, 0, 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			return
		}
		party.Members = append(party.Members, target.ID)
	}
	p.Engine.Parties[p.ID] = party
	resp := protocol.MakeDefaultMsg(protocol.SMCreateGroupOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.broadcastPartyMembers(party, server)
	log.Logf(log.LevelInfo, "Party", "%s created a party", p.Name)
}

// HandleGroupMode 切换允许组队标志（CMGroupMode，Param=1/0）。
func (p *PlayObject) HandleGroupMode(msg SendMessage, server *netserver.TCPServer) {
	p.AllowGroup = msg.Param1 != 0
	resp := protocol.MakeDefaultMsg(protocol.SMGroupModeChanged, int32(msg.Param1), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

// HandleAddGroupMember 邀请指定玩家加入调用者的队伍。
func (p *PlayObject) HandleAddGroupMember(msg SendMessage, server *netserver.TCPServer) {
	party := p.partyOf()
	if party == nil || party.Leader != p.ID || p.Engine == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMGroupAddMemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetName := strings.TrimSpace(msg.Msg)
	target := p.Engine.GetPlayerByName(targetName)
	if target == nil || !target.AllowGroup || target.partyOf() != nil || len(party.Members) >= p.Engine.Config.GetGroupMembersMax() {
		resp := protocol.MakeDefaultMsg(protocol.SMGroupAddMemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	party.Members = append(party.Members, target.ID)
	resp := protocol.MakeDefaultMsg(protocol.SMGroupAddMemOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(target.Name))
	p.broadcastPartyMembers(party, server)
	log.Logf(log.LevelInfo, "Party", "%s invited %s to party", p.Name, target.Name)
}

// HandleDelGroupMember 将指定玩家从调用者的队伍中移除；
// 只剩一人时队伍解散。
func (p *PlayObject) HandleDelGroupMember(msg SendMessage, server *netserver.TCPServer) {
	party := p.partyOf()
	if party == nil || party.Leader != p.ID {
		resp := protocol.MakeDefaultMsg(protocol.SMGroupDelMemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	targetName := strings.TrimSpace(msg.Msg)
	idx := -1
	for i, id := range party.Members {
		if m := p.Engine.GetPlayer(id); m != nil && m.Name == targetName {
			idx = i
			break
		}
	}
	if idx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMGroupDelMemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	party.Members = append(party.Members[:idx], party.Members[idx+1:]...)
	resp := protocol.MakeDefaultMsg(protocol.SMGroupDelMemOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetName))
	if removed := p.Engine.GetPlayerByName(targetName); removed != nil {
		cancel := protocol.MakeDefaultMsg(protocol.SMGroupCancel, 0, 0, 0, 0)
		server.Send(removed.Session.ID, cancel, "")
	}
	if len(party.Members) <= 1 {
		delete(p.Engine.Parties, party.Leader)
		cancel := protocol.MakeDefaultMsg(protocol.SMGroupCancel, 0, 0, 0, 0)
		server.Send(p.Session.ID, cancel, "")
		return
	}
	p.broadcastPartyMembers(party, server)
	log.Logf(log.LevelInfo, "Party", "%s removed %s from party", p.Name, targetName)
}
