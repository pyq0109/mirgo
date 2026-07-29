package main

import (
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const maxFriends = 50

// HandleAddFriend 添加好友（CMAddFriend, body=目标玩家名）。
func (p *PlayObject) HandleAddFriend(msg SendMessage, server *netserver.TCPServer) {
	targetName := strings.TrimSpace(msg.Msg)
	if targetName == "" || targetName == p.Name {
		p.sendAddFriendFail(server)
		return
	}
	if len(p.Friends) >= maxFriends {
		p.sendAddFriendFail(server)
		return
	}
	for _, f := range p.Friends {
		if f == targetName {
			p.sendAddFriendFail(server)
			return
		}
	}
	p.Friends = append(p.Friends, targetName)
	p.sendAddFriendOK(server, targetName)
	log.Logf(log.LevelInfo, "Friend", "%s added friend %s", p.Name, targetName)
}

// HandleDelFriend 删除好友（CMDelFriend, body=目标玩家名）。
func (p *PlayObject) HandleDelFriend(msg SendMessage, server *netserver.TCPServer) {
	targetName := strings.TrimSpace(msg.Msg)
	idx := -1
	for i, f := range p.Friends {
		if f == targetName {
			idx = i
			break
		}
	}
	if idx < 0 {
		p.sendDelFriendFail(server)
		return
	}
	p.Friends = append(p.Friends[:idx], p.Friends[idx+1:]...)
	resp := protocol.MakeDefaultMsg(protocol.SMDelFriendOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(targetName))
	log.Logf(log.LevelInfo, "Friend", "%s removed friend %s", p.Name, targetName)
}

// HandleQueryFriends 发送好友列表（CMQueryFriends）。
func (p *PlayObject) HandleQueryFriends(server *netserver.TCPServer) {
	p.SendFriendList(server)
}

// SendFriendList 发送完整好友列表，附带在线状态。
func (p *PlayObject) SendFriendList(server *netserver.TCPServer) {
	var sb strings.Builder
	for i, name := range p.Friends {
		if i > 0 {
			sb.WriteByte('\n')
		}
		online := 0
		if p.Engine != nil && p.Engine.GetPlayerByName(name) != nil {
			online = 1
		}
		sb.WriteString(name)
		sb.WriteByte('/')
		if online == 1 {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMFriendList, int32(len(p.Friends)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
}

// NotifyFriendsOnline 上线时通知好友。
func (p *PlayObject) NotifyFriendsOnline(server *netserver.TCPServer) {
	if p.Engine == nil {
		return
	}
	msg := protocol.MakeDefaultMsg(protocol.SMFriendOnline, 0, 0, 0, 0)
	body := protocol.EncodeString(p.Name)
	for _, name := range p.Friends {
		if friend := p.Engine.GetPlayerByName(name); friend != nil {
			server.Send(friend.Session.ID, msg, body)
		}
	}
}

// NotifyFriendsOffline 下线时通知好友。
func (p *PlayObject) NotifyFriendsOffline(server *netserver.TCPServer) {
	if p.Engine == nil {
		return
	}
	msg := protocol.MakeDefaultMsg(protocol.SMFriendOffline, 0, 0, 0, 0)
	body := protocol.EncodeString(p.Name)
	for _, name := range p.Friends {
		if friend := p.Engine.GetPlayerByName(name); friend != nil {
			server.Send(friend.Session.ID, msg, body)
		}
	}
}

func (p *PlayObject) sendAddFriendOK(server *netserver.TCPServer, name string) {
	resp := protocol.MakeDefaultMsg(protocol.SMAddFriendOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(name))
}

func (p *PlayObject) sendAddFriendFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMAddFriendFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendDelFriendFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMDelFriendFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
