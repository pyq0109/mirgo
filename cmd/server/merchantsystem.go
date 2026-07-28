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
		log.Logf(log.LevelWarn, "Merchant", "merchant config directory not found: %v", err)
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
	log.Logf(log.LevelInfo, "Merchant", "loaded %d merchant configs", len(merchantConfigs))
}

func getMerchantConfig(npcName string) *MerchantConfig {
	return merchantConfigs[strings.ToLower(npcName)]
}

// HandleMerchantDlgSelect 处理点击 NPC 对话链接（Delphi TMerchant.UserSelect, ObjNpc.pas:1419-1607）。
func (p *PlayObject) HandleMerchantDlgSelect(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	npc, ok := p.envir.getNpcByID(int32(msg.Param1))
	if !ok {
		return
	}

	tag := strings.TrimSpace(msg.Msg)

	// 管理返回导航
	if strings.EqualFold(tag, "@back") {
		backLabel := p.ScriptGoBackLabel
		if backLabel == "" {
			backLabel = "main"
		}
		p.ScriptGoBackLabel = p.ScriptCurrLabel
		p.ScriptCurrLabel = backLabel
		if script := npc.GetScript(); script != nil {
			script.Execute(backLabel, p, npc, server)
		}
		return
	}

	// 记录导航历史
	if !strings.EqualFold(tag, "@exit") {
		p.ScriptGoBackLabel = p.ScriptCurrLabel
		p.ScriptCurrLabel = strings.TrimPrefix(tag, "@")
	}

	// @@ 前缀命令（需要用户输入）
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

	// 先尝试脚本标签跳转
	label := strings.TrimPrefix(tag, "@")
	if script := npc.GetScript(); script != nil {
		if _, exists := script.Labels[label]; exists {
			script.Execute(label, p, npc, server)
			return
		}
	}

	// 内置商人命令分发
	switch strings.ToLower(tag) {
	case "@buy", "@trading":
		if npc.CanBuy {
			p.sendGoodsList(server, npc)
		}
	case "@sell":
		if npc.CanSell {
			p.sendSellMode(server, npc)
		}
	case "@repair":
		if npc.CanRepair {
			p.sendRepairMode(server, npc)
		}
	case "@s_repair", "@superrepair":
		if npc.CanSRepair {
			p.sendRepairMode(server, npc)
		}
	case "@storage":
		if npc.CanStorage {
			p.sendStorageMenu(server)
		}
	case "@getback":
		if npc.CanGetback {
			p.sendStorageMenu(server)
		}
	case "@exit":
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	case "@makedrug":
		if npc.CanMakeDrug {
			// 发送制药列表（简化：发送通用商品列表）
			p.sendGoodsListFromDB(server, npc)
		}
	case "@upgradenow":
		if npc.CanUpgrade {
			p.HandleUpgradeWeapon(npc, server)
		}
	case "@getbackupgnow":
		if npc.CanGetBackup {
			p.HandleGetBackupWeapon(npc, server)
		}
	}
}

func (p *PlayObject) sendGoodsList(server *netserver.TCPServer, npc *NpcObject) {
	// 优先从三层商品架构发送
	if len(npc.GoodsList) > 0 {
		p.sendGoodsListFromStock(server, npc)
		return
	}
	// 回退：从 ItemDB 发送通用商品
	p.sendGoodsListFromDB(server, npc)
}

func (p *PlayObject) sendGoodsListFromStock(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}

	type goodsEntry struct {
		idx   uint16
		price uint16
	}
	var entries []goodsEntry

	npc.mu.RLock()
	for name, stock := range npc.GoodsList {
		if len(stock.Items) == 0 {
			continue
		}
		def := p.ItemDB.GetByName(name)
		if def == nil {
			continue
		}
		price := int(def.Price)
		if ip, ok := npc.PriceList[def.Idx]; ok && ip.Price > 0 {
			price = ip.Price
		}
		price = price * npc.PriceRate / 100
		if price <= 0 {
			price = 1
		}
		entries = append(entries, goodsEntry{idx: uint16(def.Idx), price: uint16(price)})
	}
	npc.mu.RUnlock()

	buf := make([]byte, 0, 2+len(entries)*4)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(entries)))
	buf = append(buf, count...)
	for _, e := range entries {
		entry := make([]byte, 4)
		binary.LittleEndian.PutUint16(entry[0:2], e.idx)
		binary.LittleEndian.PutUint16(entry[2:4], e.price)
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(len(entries)), 0, 0)
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

	// 查找当前交互的 NPC
	var npc *NpcObject
	if p.CurrentNpc != nil {
		npc = p.CurrentNpc
	}

	// 计算价格
	price := int(def.Price)
	if npc != nil {
		npc.mu.RLock()
		if ip, ok := npc.PriceList[def.Idx]; ok && ip.Price > 0 {
			price = ip.Price
		}
		price = price * npc.PriceRate / 100
		npc.mu.RUnlock()
	}
	if price <= 0 {
		price = 100
	}
	if p.Gold < price {
		p.sendBuyFail(server)
		return
	}

	// 尝试从 NPC 库存购买
	bought := false
	if npc != nil && len(npc.GoodsList) > 0 {
		npc.mu.Lock()
		if stock, ok := npc.GoodsList[def.Name]; ok && len(stock.Items) > 0 {
			item := stock.Items[0]
			stock.Items = stock.Items[1:]
			// 分配唯一 MakeIndex
			if p.Engine != nil {
				p.Engine.mu.Lock()
				item.MakeIndex = int32(p.Engine.nextItemID)
				p.Engine.nextItemID++
				p.Engine.mu.Unlock()
			}
			p.ItemList = append(p.ItemList, item)
			bought = true
		}
		npc.mu.Unlock()
	}

	// 回退：从 ItemDB 创建（无库存 NPC）
	if !bought {
		if !p.GiveItem(itemIdx) {
			p.sendBuyFail(server)
			return
		}
	}

	p.Gold -= price
	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemSuccess, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
}

func (p *PlayObject) HandleSellItem(msg SendMessage, server *netserver.TCPServer) {
	// Param1 = MakeIndex（实例 ID；客户端布局由客户端维护）。
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

	// 将物品添加到 NPC 商品列表（深拷贝，修复 Delphi 原始 bug）
	if p.CurrentNpc != nil && p.CurrentNpc.IsMerchant {
		npc := p.CurrentNpc
		itemCopy := &protocol.UserItem{
			WIndex:  item.WIndex,
			Dura:    item.Dura,
			DuraMax: item.DuraMax,
		}
		copy(itemCopy.BtValue[:], item.BtValue[:])
		npc.mu.Lock()
		stock := npc.GoodsList[def.Name]
		if stock == nil {
			stock = &GoodsStock{}
			npc.GoodsList[def.Name] = stock
		}
		if len(stock.Items) < 5000 {
			stock.Items = append(stock.Items, itemCopy)
		}
		npc.mu.Unlock()
	}

	resp := protocol.MakeDefaultMsg(protocol.SMUserSellItemOK, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
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

	// 判断是否为特殊修理（通过 CurrentNpc 的 CanSRepair 标志）
	isSpecial := false
	if p.CurrentNpc != nil && p.CurrentNpc.CanSRepair {
		isSpecial = true
	}

	cost := p.calcRepairCost(def, item, isSpecial)
	if p.Gold < cost {
		p.sendRepairFail(server)
		return
	}

	p.Gold -= cost
	if isSpecial {
		// 特殊修理：不降低 DuraMax
		item.Dura = item.DuraMax
	} else {
		// 普通修理：永久降低 DuraMax（Delphi nRepairItemDecDura=30）
		lostDura := int(item.DuraMax) - int(item.Dura)
		decay := lostDura / 30
		if decay > 0 && int(item.DuraMax) > decay {
			item.DuraMax -= uint16(decay)
		}
		item.Dura = item.DuraMax
	}
	p.sendDuraChange(server, item)

	resp := protocol.MakeDefaultMsg(protocol.SMUserRepairItemOK, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
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
	isSpecial := p.CurrentNpc != nil && p.CurrentNpc.CanSRepair
	cost := p.calcRepairCost(def, item, isSpecial)
	resp := protocol.MakeDefaultMsg(protocol.SMSendRepairCost, int32(cost), uint16(makeIndex), 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) calcRepairCost(def *ItemDef, item *protocol.UserItem, isSpecial bool) int {
	if item.DuraMax == 0 {
		return 0
	}
	// Delphi 公式: price / 3 / DuraMax * (DuraMax - Dura)
	lostDura := int(item.DuraMax) - int(item.Dura)
	cost := int(def.Price) / 3 * lostDura / int(item.DuraMax)
	if cost < 1 {
		cost = 1
	}
	if isSpecial {
		cost *= 3 // 特殊修理费用为普通的 3 倍
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
