package main

import (
	"encoding/binary"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)



func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// dealGoldMsg 构造改金币回包（Delphi MakeLong 语义）：Recog=交易金币，
// Param/Tag=Lo/Hi(钱包金币)，客户端按 Param|Tag<<16 组装 32 位
//（ClMain.pas:4819/4824）。失败回包也必须携带当前值，否则客户端
// 会把金币显示清零。
func dealGoldMsg(ident uint16, dealGold, walletGold int) protocol.DefaultMessage {
	return protocol.MakeDefaultMsg(ident, int32(dealGold), uint16(walletGold), uint16(uint32(walletGold)>>16), 0)
}

type DealState struct {
	Partner   *PlayObject
	Items     []*protocol.UserItem
	Gold      int
	Confirmed bool  // CMDealEnd 后锁定，直到交易完成
	LastActionTick int64 // 最近一次加/减物/改金时间（反连点用）
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
	// Delphi（ObjBase.pas:17647-17697）：dwTryDealTime=3000ms 冷却。
	now := time.Now().UnixMilli()
	if now-p.tryDealTick < 3000 {
		return
	}
	p.tryDealTick = now
	// 目标：必须相邻（Delphi 面对面判定；Go 用曼哈顿距离 ≤1 等价）。
	var target *PlayObject
	targetName := msg.Msg
	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	bestDist := 999
	for _, obj := range objs {
		other, ok := obj.(*PlayObject)
		if !ok || other.ID == p.ID || other.Deal != nil {
			continue
		}
		d := absInt(other.CurrX-p.CurrX) + absInt(other.CurrY-p.CurrY)
		if d > 1 {
			continue
		}
		if targetName != "" {
			if other.Name == targetName {
				target = other
				break
			}
			continue
		}
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
	// Param1 = MakeIndex（由 Recog 路由）。
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMDealAddItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if len(p.Deal.Items) >= p.Engine.Config.GetMaxTradeItems() {
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.Deal.Items = append(p.Deal.Items, item)
	p.Deal.LastActionTick = time.Now().UnixMilli()
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
	p.Deal.LastActionTick = time.Now().UnixMilli()
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
		// Delphi（ClMain:4824）：失败回包携带当前交易金币与钱包金币，
		// 客户端按 MakeLong(Param,Tag) 恢复显示。发全 0 会把客户端金币清零。
		server.Send(p.Session.ID, dealGoldMsg(protocol.SMDealChgGoldFail, p.Deal.Gold, p.Gold), "")
		return
	}
	p.Gold = p.Gold + p.Deal.Gold - gold
	p.Deal.Gold = gold
	p.Deal.LastActionTick = time.Now().UnixMilli()
	server.Send(p.Session.ID, dealGoldMsg(protocol.SMDealChgGoldOK, gold, p.Gold), "")
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
	// Delphi（ObjBase.pas:17844-17849）：双方确认前最近交易动作
	// 间隔 <1000ms 判连点作弊，直接取消。
	now := time.Now().UnixMilli()
	if now-p.Deal.LastActionTick < 1000 || now-partner.Deal.LastActionTick < 1000 {
		p.sysMsg(server, "交易确认过快，已取消")
		p.CancelDeal(server)
		return
	}
	maxBag := p.Engine.Config.GetMaxBagSlots()
	if len(p.ItemList)+len(partner.Deal.Items) > maxBag ||
		len(partner.ItemList)+len(p.Deal.Items) > maxBag {
		p.CancelDeal(server)
		return
	}
	// Delphi（ObjBase.pas:17858/17868）：金币上限双向校验。
	maxGold := p.Engine.Config.GetMaxGold()
	if p.Gold+partner.Deal.Gold > maxGold {
		p.sysMsg(server, "金币已达到上限")
		p.CancelDeal(server)
		return
	}
	if partner.Gold+p.Deal.Gold > maxGold {
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
	log.Logf(log.LevelInfo, "Trade", "%s completed trade with %s", p.Name, partner.Name)
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
