package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const MaxDealItems = 12

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type DealState struct {
	Partner   *PlayObject
	Items     []*protocol.UserItem
	Gold      int
	Confirmed bool // locked after CMDealEnd until the trade resolves
}

// encodeDealItem serializes a traded item for the client (matches the bag
// item layout consumed by the client's deal grids).
func encodeDealItem(item *protocol.UserItem) string {
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(item.MakeIndex))
	binary.LittleEndian.PutUint16(buf[4:6], item.WIndex)
	binary.LittleEndian.PutUint16(buf[6:8], item.Dura)
	binary.LittleEndian.PutUint16(buf[8:10], item.DuraMax)
	return protocol.EncodeBuffer(buf)
}

func (p *PlayObject) HandleDealTry(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal != nil || p.envir == nil {
		return
	}
	// Target: named player in range, or the closest adjacent player when no
	// name is given (Delphi's front-actor selection is commented out too;
	// adjacent is the usable default).
	var target *PlayObject
	targetName := msg.Msg
	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	bestDist := 999
	for _, obj := range objs {
		other, ok := obj.(*PlayObject)
		if !ok || other.ID == p.ID || other.Deal != nil {
			continue
		}
		if targetName != "" {
			if other.Name == targetName {
				target = other
				break
			}
			continue
		}
		d := absInt(other.CurrX-p.CurrX) + absInt(other.CurrY-p.CurrY)
		if d < bestDist {
			bestDist = d
			target = other
		}
	}
	if target == nil {
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
	if p.Deal == nil || p.Deal.Partner == nil || p.Deal.Confirmed {
		return
	}
	// Param1 = MakeIndex (routed from Recog).
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMDealAddItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if len(p.Deal.Items) >= MaxDealItems {
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.Deal.Items = append(p.Deal.Items, item)
	// Own client: confirmation carries the item so it lands in the grid;
	// bag re-synced separately.
	resp := protocol.MakeDefaultMsg(protocol.SMDealAddItemOK, int32(len(p.Deal.Items)-1), 0, 0, 0)
	server.Send(p.Session.ID, resp, encodeDealItem(item))
	p.SendBagItemsFull(server)
	// Partner sees the offered item.
	remoteResp := protocol.MakeDefaultMsg(protocol.SMDealRemoteAddItem, int32(len(p.Deal.Items)-1), 0, 0, 0)
	server.Send(p.Deal.Partner.Session.ID, remoteResp, encodeDealItem(item))
}

// HandleDealDelItem takes an offered item back into the bag
// (Delphi SendDelDealItem / DealItemReturnBag, FState:5713-5720).
func (p *PlayObject) HandleDealDelItem(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal == nil || p.Deal.Confirmed {
		return
	}
	makeIndex := int32(msg.Param1)
	idx := -1
	for i, item := range p.Deal.Items {
		if item != nil && item.MakeIndex == makeIndex {
			idx = i
			break
		}
	}
	if idx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMDealDelItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	item := p.Deal.Items[idx]
	p.Deal.Items = append(p.Deal.Items[:idx], p.Deal.Items[idx+1:]...)
	p.ItemList = append(p.ItemList, item)
	resp := protocol.MakeDefaultMsg(protocol.SMDealDelItemOK, makeIndex, 0, 0, 0)
	server.Send(p.Session.ID, resp, encodeDealItem(item))
	p.SendBagItemsFull(server)
	if p.Deal.Partner != nil {
		remoteResp := protocol.MakeDefaultMsg(protocol.SMDealRemoteDelItem, makeIndex, 0, 0, 0)
		server.Send(p.Deal.Partner.Session.ID, remoteResp, encodeDealItem(item))
	}
}

func (p *PlayObject) HandleDealChgGold(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal == nil || p.Deal.Confirmed {
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
	// Recog = deal gold, Param = remaining wallet gold.
	resp := protocol.MakeDefaultMsg(protocol.SMDealChgGoldOK, int32(gold), uint16(p.Gold), 0, 0)
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
	// Refresh both inventories and gold.
	p.SendBagItemsFull(server)
	partner.SendBagItemsFull(server)
	goldMsg := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldMsg, "")
	partnerGold := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(partner.Gold), 0, 0, 0)
	server.Send(partner.Session.ID, partnerGold, "")
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
	p.SendBagItemsFull(server)
	goldMsg := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldMsg, "")
	if partner != nil && partner.Deal != nil {
		partner.ItemList = append(partner.ItemList, partner.Deal.Items...)
		partner.Gold += partner.Deal.Gold
		partner.Deal = nil
		server.Send(partner.Session.ID, cancelMsg, "")
		partner.SendBagItemsFull(server)
		partnerGold := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(partner.Gold), 0, 0, 0)
		server.Send(partner.Session.ID, partnerGold, "")
	}
	p.Deal = nil
}
