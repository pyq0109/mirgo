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
	Confirmed bool // CMDealEnd 后锁定，直到交易完成
}

// encodeDealItem 将交易物品序列化为客户端格式（与客户端交易栏
// 使用的背包物品布局一致）。
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
	// 目标：范围内的指定玩家，未指定名字时取最近的相邻玩家
	//（Delphi 的正前方角色选择也被注释掉了；相邻是可用默认值）。
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
	log.Logf(log.LevelInfo, "Trade", "%s 向 %s 发起交易", p.Name, target.Name)
}

func (p *PlayObject) HandleDealAddItem(msg SendMessage, server *netserver.TCPServer) {
	if p.Deal == nil || p.Deal.Partner == nil || p.Deal.Confirmed {
		return
	}
	// Param1 = MakeIndex（由 Recog 路由）。
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
	// 本方客户端：确认消息携带物品信息以便放入交易栏；
	// 背包另行同步。
	resp := protocol.MakeDefaultMsg(protocol.SMDealAddItemOK, int32(len(p.Deal.Items)-1), 0, 0, 0)
	server.Send(p.Session.ID, resp, encodeDealItem(item))
	p.SendBagItemsFull(server)
	// 对方看到放入的物品。
	remoteResp := protocol.MakeDefaultMsg(protocol.SMDealRemoteAddItem, int32(len(p.Deal.Items)-1), 0, 0, 0)
	server.Send(p.Deal.Partner.Session.ID, remoteResp, encodeDealItem(item))
}

// HandleDealDelItem 将已放入的物品从交易栏取回背包
//（Delphi SendDelDealItem / DealItemReturnBag，FState:5713-5720）。
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
	// Recog = 交易金币，Param = 剩余钱包金币。
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
	// 刷新双方背包和金币。
	p.SendBagItemsFull(server)
	partner.SendBagItemsFull(server)
	goldMsg := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldMsg, "")
	partnerGold := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(partner.Gold), 0, 0, 0)
	server.Send(partner.Session.ID, partnerGold, "")
	log.Logf(log.LevelInfo, "Trade", "%s 和 %s 完成交易", p.Name, partner.Name)
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
