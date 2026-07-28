package main

import (
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const makeDrugPrice = 500

type DrugRecipe struct {
	Product   string
	Materials []DrugMaterial
}

type DrugMaterial struct {
	Name  string
	Count int
}

func (p *PlayObject) HandleMakeDrugItem(msg SendMessage, server *netserver.TCPServer) {
	if p.CurrentNpc == nil || !p.CurrentNpc.CanMakeDrug {
		return
	}
	if p.ItemDB == nil {
		return
	}

	// 客户端发送物品索引
	itemIdx := int(msg.Param1)
	def := p.ItemDB.GetByIdx(itemIdx)
	if def == nil {
		p.sendMakeDrugFail(server)
		return
	}

	// 检查金币
	if p.Gold < makeDrugPrice {
		p.sendMakeDrugFail(server)
		return
	}

	// 检查背包空间
	if len(p.ItemList) >= MaxBagItems {
		p.sendMakeDrugFail(server)
		return
	}

	// 简化实现：制药只需要金币（完整实现需要配方材料）
	p.Gold -= makeDrugPrice
	p.GiveItem(itemIdx)
	p.SendBagItemsFull(server)

	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
	p.sendMakeDrugSuccess(server)
}

func (p *PlayObject) sendMakeDrugSuccess(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMakeDrugSuccess, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMakeDrugFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMakeDrugFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
