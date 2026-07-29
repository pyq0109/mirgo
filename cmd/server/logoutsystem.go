package main

import (
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

// LogoutPlayer 保存角色、清理世界、返回选角状态。
func (e *UserEngine) LogoutPlayer(server *netserver.TCPServer, db *storage.Database, p *PlayObject) {
	saveCharacterData(db, p)
	p.NotifyFriendsOffline(server)
	p.Ghost = true
	p.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
	if p.envir != nil {
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	}
	e.RemovePlayer(p.ID)
	resp := protocol.MakeDefaultMsg(protocol.SMLogoutOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.Session.State = netserver.StateAuthenticated
	p.Session.CharacterID = 0
	log.Logf(log.LevelInfo, "Server", "player %s logged out", p.Name)
}

// ExitPlayer 保存角色、清理世界、断开连接。
func (e *UserEngine) ExitPlayer(server *netserver.TCPServer, db *storage.Database, p *PlayObject) {
	saveCharacterData(db, p)
	p.NotifyFriendsOffline(server)
	p.Ghost = true
	p.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
	if p.envir != nil {
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	}
	e.RemovePlayer(p.ID)
	resp := protocol.MakeDefaultMsg(protocol.SMExitOK, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	server.CloseSession(p.Session.ID)
	log.Logf(log.LevelInfo, "Server", "player %s exited game", p.Name)
}
