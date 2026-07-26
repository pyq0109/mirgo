package main

import (
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const MaxDealItems = 12

type DealState struct {
	Partner   *PlayObject
	Items     []*protocol.UserItem
	Gold      int
	Confirmed bool
}

func (p *PlayObject) HandleDealTry(msg SendMessage, server *netserver.TCPServer) {
	targetName := msg.Msg
	if p.Deal != nil {
		return
	}
	var target *PlayObject
	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	for _, obj := range objs {
		if other, ok := obj.(*PlayObject); ok && other.Name == targetName && other.ID != p.ID {
			target = other
			break
		}
	}
	if target == nil || target.Deal != nil {
		resp := protocol.MakeDefaultMsg(protocol.SMDealTryFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	p.Deal = &DealState{Partner: target}
	target.Deal = &DealState{Partner: p}
	menuMsg := protocol.MakeDefaultMsg(protocol.SMDealMenu, 0, 0, 0, 0)
	server.Send(p.Session.ID, menuMsg, protocol.EncodeString(target.Name))
	server.Send(target.Session.ID, menuMsg, protocol.EncodeString(p.Name))
	log.Logf(log.LevelInfo, "Trade", "%s initiated trade with %s", p.Name, target.Name)
}

func (p *PlayObject) HandleDealAddItem(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal == nil || p.Deal.Partner == nil {
		return
	}
	if p.Deal.Partner.Deal != nil && p.Deal.Partner.Deal.Confirmed {
		return
	}
	bagIdx := msg.Param1
	if bagIdx < 0 || bagIdx >= len(p.ItemList) {
		return
	}
	if len(p.Deal.Items) >= MaxDealItems {
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.Deal.Items = append(p.Deal.Items, item)
	p.Deal.Confirmed = false
	resp := protocol.MakeDefaultMsg(protocol.SMDealAddItemOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	remoteResp := protocol.MakeDefaultMsg(protocol.SMDealRemoteAddItem, int32(len(p.Deal.Items)-1), 0, 0, 0)
	server.Send(p.Deal.Partner.Session.ID, remoteResp, "")
}

func (p *PlayObject) HandleDealChgGold(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal == nil {
		return
	}
	gold := msg.Param1
	if gold < 0 {
		return
	}
	if p.Gold+p.Deal.Gold < gold {
		resp := protocol.MakeDefaultMsg(protocol.SMDealChgGoldFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	p.Gold = p.Gold + p.Deal.Gold - gold
	p.Deal.Gold = gold
	p.Deal.Confirmed = false
	resp := protocol.MakeDefaultMsg(protocol.SMDealChgGoldOK, int32(gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	remoteResp := protocol.MakeDefaultMsg(protocol.SMDealRemoteChgGold, int32(gold), 0, 0, 0)
	server.Send(p.Deal.Partner.Session.ID, remoteResp, "")
}

func (p *PlayObject) HandleDealEnd(server *netserver.TCPServer) {
	if p.Deal == nil || p.Deal.Partner == nil {
		return
	}
	p.Deal.Confirmed = true
	partner := p.Deal.Partner
	if partner.Deal == nil || !partner.Deal.Confirmed {
		return
	}
	if len(p.ItemList)+len(partner.Deal.Items) > MaxBagItems ||
		len(partner.ItemList)+len(p.Deal.Items) > MaxBagItems {
		p.CancelDeal(server)
		return
	}
	p.ItemList = append(p.ItemList, partner.Deal.Items...)
	partner.ItemList = append(partner.ItemList, p.Deal.Items...)
	p.Gold += partner.Deal.Gold
	partner.Gold += p.Deal.Gold
	succMsg := protocol.MakeDefaultMsg(protocol.SMDealSuccess, 0, 0, 0, 0)
	server.Send(p.Session.ID, succMsg, "")
	server.Send(partner.Session.ID, succMsg, "")
	log.Logf(log.LevelInfo, "Trade", "%s and %s completed trade", p.Name, partner.Name)
	p.Deal = nil
	partner.Deal = nil
}

func (p *PlayObject) HandleDealCancel(server *netserver.TCPServer) {
	p.CancelDeal(server)
}

func (p *PlayObject) CancelDeal(server *netserver.TCPServer) {
	if p.Deal == nil {
		return
	}
	partner := p.Deal.Partner
	p.ItemList = append(p.ItemList, p.Deal.Items...)
	p.Gold += p.Deal.Gold
	cancelMsg := protocol.MakeDefaultMsg(protocol.SMDealCancel, 0, 0, 0, 0)
	server.Send(p.Session.ID, cancelMsg, "")
	if partner != nil && partner.Deal != nil {
		partner.ItemList = append(partner.ItemList, partner.Deal.Items...)
		partner.Gold += partner.Deal.Gold
		partner.Deal = nil
		server.Send(partner.Session.ID, cancelMsg, "")
	}
	p.Deal = nil
}
