package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MerchantGoods struct {
	ItemName string `json:"itemName"`
	Price    int    `json:"price"`
}

type MerchantConfig struct {
	NpcName    string          `json:"npcName"`
	MapName    string          `json:"mapName"`
	BuyRate    float64         `json:"buyRate"`
	SellRate   float64         `json:"sellRate"`
	RepairRate float64         `json:"repairRate"`
	Goods      []MerchantGoods `json:"goods"`
}

var merchantConfigs map[string]*MerchantConfig

func LoadMerchantConfigs(dir string) {
	merchantConfigs = make(map[string]*MerchantConfig)
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Logf(log.LevelWarn, "Merchant", "No merchant config dir: %v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonc") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var clean []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			clean = append(clean, line)
		}
		var cfg MerchantConfig
		if json.Unmarshal([]byte(strings.Join(clean, "\n")), &cfg) != nil {
			continue
		}
		if cfg.BuyRate == 0 {
			cfg.BuyRate = 1.0
		}
		if cfg.SellRate == 0 {
			cfg.SellRate = 0.5
		}
		if cfg.RepairRate == 0 {
			cfg.RepairRate = 0.05
		}
		key := strings.ToLower(cfg.NpcName)
		merchantConfigs[key] = &cfg
	}
	log.Logf(log.LevelInfo, "Merchant", "Loaded %d merchant configs", len(merchantConfigs))
}

func getMerchantConfig(npcName string) *MerchantConfig {
	return merchantConfigs[strings.ToLower(npcName)]
}

// HandleMerchantDlgSelect processes a clicked NPC dialog link. The body
// carries the tag value after '/' in <text/tag> (Delphi
// SendMerchantDlgSelect, ClMain.pas:3094-3110): @buy/@sell/@switch etc.
func (p *PlayObject) HandleMerchantDlgSelect(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	npc, ok := p.envir.getNpcByID(int32(msg.Param1))
	if !ok {
		return
	}
	tag := strings.TrimSpace(msg.Msg)
	switch strings.ToLower(tag) {
	case "@buy":
		p.sendGoodsList(server, npc)
	case "@sell":
		p.sendSellMode(server, npc)
	case "@repair":
		p.sendRepairMode(server, npc)
	default:
		// '@@' tags arrive as "@@cmd\r\ninput" (ClMain.pas:3100-3105).
		if strings.HasPrefix(tag, "@@") {
			cmd, input := tag, ""
			if i := strings.Index(tag, "\r\n"); i >= 0 {
				cmd, input = tag[:i], tag[i+2:]
			}
			if cmd == "@@buildguildnow" && input != "" {
				build := msg
				build.Msg = input
				p.HandleBuildGuild(build, server)
				return
			}
		}
		// Jump to the script label (labels are stored without '@').
		if npc.Script != "" {
			if script, err := LoadNpcScript(npc.Script); err == nil {
				script.Execute(strings.TrimPrefix(tag, "@"), p, npc, server)
			}
		}
	}
}

func (p *PlayObject) sendGoodsList(server *netserver.TCPServer, npc *NpcObject) {
	cfg := getMerchantConfig(npc.Name)
	if cfg == nil {
		p.sendGoodsListFromDB(server, npc)
		return
	}
	buf := make([]byte, 0, 2+len(cfg.Goods)*4)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(cfg.Goods)))
	buf = append(buf, count...)
	for _, g := range cfg.Goods {
		entry := make([]byte, 4)
		idx := 0
		if p.ItemDB != nil {
			if def := p.ItemDB.GetByName(g.ItemName); def != nil {
				idx = def.Idx
			}
		}
		binary.LittleEndian.PutUint16(entry[0:2], uint16(idx))
		price := g.Price
		if price == 0 && p.ItemDB != nil {
			if def := p.ItemDB.GetByName(g.ItemName); def != nil {
				price = int(def.Price)
			}
		}
		binary.LittleEndian.PutUint16(entry[2:4], uint16(price))
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(len(cfg.Goods)), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) sendGoodsListFromDB(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}
	var goods []MerchantGoods
	for i := range p.ItemDB.Items {
		item := &p.ItemDB.Items[i]
		if item.Price > 0 && item.StdMode < 40 {
			goods = append(goods, MerchantGoods{ItemName: item.Name, Price: int(item.Price)})
		}
		if len(goods) >= 50 {
			break
		}
	}
	buf := make([]byte, 0, 2+len(goods)*4)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(goods)))
	buf = append(buf, count...)
	for _, g := range goods {
		entry := make([]byte, 4)
		def := p.ItemDB.GetByName(g.ItemName)
		if def != nil {
			binary.LittleEndian.PutUint16(entry[0:2], uint16(def.Idx))
		}
		binary.LittleEndian.PutUint16(entry[2:4], uint16(g.Price))
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(len(goods)), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) HandleBuyItem(msg SendMessage, server *netserver.TCPServer) {
	itemIdx := int(msg.Param1)
	if p.ItemDB == nil {
		p.sendBuyFail(server)
		return
	}
	def := p.ItemDB.GetByIdx(itemIdx)
	if def == nil {
		p.sendBuyFail(server)
		return
	}
	if len(p.ItemList) >= MaxBagItems {
		p.sendBuyFail(server)
		return
	}

	price := int(def.Price)
	if price <= 0 {
		price = 100
	}
	if p.Gold < price {
		p.sendBuyFail(server)
		return
	}

	p.Gold -= price
	if !p.GiveItem(itemIdx) {
		p.Gold += price
		p.sendBuyFail(server)
		return
	}

	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemSuccess, int32(itemIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	log.Logf(log.LevelInfo, "Merchant", "%s bought %s for %d gold", p.Name, def.Name, price)
}

func (p *PlayObject) HandleSellItem(msg SendMessage, server *netserver.TCPServer) {
	// Param1 = MakeIndex (instance id; the client layout is client-owned).
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		p.sendSellFail(server)
		return
	}
	item := p.ItemList[bagIdx]
	if p.ItemDB == nil {
		p.sendSellFail(server)
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		p.sendSellFail(server)
		return
	}

	price := int(def.Price) / 2
	if price < 1 {
		price = 1
	}
	if item.DuraMax > 0 {
		price = price * int(item.Dura) / int(item.DuraMax)
		if price < 1 {
			price = 1
		}
	}

	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.Gold += price

	resp := protocol.MakeDefaultMsg(protocol.SMUserSellItemOK, int32(price), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	log.Logf(log.LevelInfo, "Merchant", "%s sold %s for %d gold", p.Name, def.Name, price)
}

func (p *PlayObject) HandleQuerySellPrice(msg SendMessage, server *netserver.TCPServer) {
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		return
	}
	item := p.ItemList[bagIdx]
	if p.ItemDB == nil {
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		return
	}
	price := int(def.Price) / 2
	if price < 1 {
		price = 1
	}
	if item.DuraMax > 0 {
		price = price * int(item.Dura) / int(item.DuraMax)
		if price < 1 {
			price = 1
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendBuyPrice, int32(price), uint16(msg.Param1), 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) HandleRepairItem(msg SendMessage, server *netserver.TCPServer) {
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		p.sendRepairFail(server)
		return
	}
	item := p.ItemList[bagIdx]
	if item.Dura >= item.DuraMax || item.DuraMax == 0 {
		p.sendRepairFail(server)
		return
	}
	if p.ItemDB == nil {
		p.sendRepairFail(server)
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		p.sendRepairFail(server)
		return
	}

	cost := p.calcRepairCost(def, item)
	if p.Gold < cost {
		p.sendRepairFail(server)
		return
	}

	p.Gold -= cost
	item.Dura = item.DuraMax
	p.sendDuraChange(server, item)

	resp := protocol.MakeDefaultMsg(protocol.SMUserRepairItemOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	log.Logf(log.LevelInfo, "Merchant", "%s repaired %s for %d gold", p.Name, def.Name, cost)
}

func (p *PlayObject) HandleQueryRepairCost(msg SendMessage, server *netserver.TCPServer) {
	makeIndex := int32(msg.Param1)
	bagIdx := p.findBagItem(makeIndex)
	if bagIdx < 0 {
		return
	}
	item := p.ItemList[bagIdx]
	if p.ItemDB == nil {
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		return
	}
	cost := p.calcRepairCost(def, item)
	resp := protocol.MakeDefaultMsg(protocol.SMSendRepairCost, int32(cost), uint16(makeIndex), 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) calcRepairCost(def *ItemDef, item *protocol.UserItem) int {
	if item.DuraMax == 0 {
		return 0
	}
	lostDura := int(item.DuraMax) - int(item.Dura)
	cost := int(def.Price) * lostDura / int(item.DuraMax) / 20
	if cost < 1 {
		cost = 1
	}
	return cost
}

func (p *PlayObject) sendSellMode(server *netserver.TCPServer, npc *NpcObject) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendUserSell, npc.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendRepairMode(server *netserver.TCPServer, npc *NpcObject) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendUserRepair, npc.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendBuyFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendSellFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMUserSellItemFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendRepairFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMUserRepairItemFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
