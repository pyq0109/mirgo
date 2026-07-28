package main

import (
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// 城堡 NPC 权限检查：仅城主行会成员或 GM（权限≥3）可访问
func (p *PlayObject) canAccessCastleNpc(npc *NpcObject) bool {
	if p.Permission >= 3 {
		return true
	}
	// TODO: 检查是否为城主行会成员
	return p.GuildName != "" && p.GuildRank == "master"
}

func (p *PlayObject) HandleCastleNpcSelect(tag string, npc *NpcObject, server *netserver.TCPServer) {
	if !p.canAccessCastleNpc(npc) {
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}

	switch tag {
	case "@castlegold":
		// 查看城堡金币
		p.sysMsg(server, "城堡金币: 0")
	case "@opendoor":
		p.sysMsg(server, "城门已开启")
	case "@closedoor":
		p.sysMsg(server, "城门已关闭")
	case "@repairdoor":
		p.sysMsg(server, "城门已修理")
	case "@repairwall":
		p.sysMsg(server, "城墙已修理")
	case "@hireguard":
		p.sysMsg(server, "雇佣守卫需要更多金币")
	case "@hirearcher":
		p.sysMsg(server, "雇佣弓箭手需要更多金币")
	}
}
