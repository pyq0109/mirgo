package main

import (
	"encoding/binary"
	"strings"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MerchantGoods struct {
	ItemName string `json:"itemName"`
	Price    int    `json:"price"`
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

	// 距离验证：玩家可能已经走开
	if !p.isNearNpc(npc) {
		p.CurrentNpc = nil
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
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

	// 先尝试脚本标签跳转（带安全校验, Delphi ObjBase.pas:25401-25430）
	label := strings.TrimPrefix(tag, "@")
	if script := npc.GetScript(); script != nil {
		if sections, exists := script.Labels[label]; exists {
			extJmp := false
			for _, s := range sections {
				if s.ExtJmp {
					extJmp = true
					break
				}
			}
			if p.labelIsCanJmp(label) || extJmp {
				script.Execute(label, p, npc, server)
			}
			return
		}
	}

	// 城堡 NPC 专用命令
	if npc.Castle && p.Engine != nil && p.Engine.Castle != nil {
		lower := strings.ToLower(tag)
		switch {
		case lower == "@castlegold" || lower == "@opendoor" || lower == "@closedoor" ||
			lower == "@repairdoor" || lower == "@repairwall" ||
			lower == "@hireguard" || lower == "@hirearcher" ||
			strings.HasPrefix(lower, "@declarewar") ||
			strings.HasPrefix(lower, "@withdraw") || strings.HasPrefix(lower, "@castletax"):
			p.HandleCastleNpcSelect(tag, npc, server)
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
			// 发送制药列表（使用专用消息ID 712）
			p.sendMakeDrugList(server, npc)
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
		stock uint16
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
		entries = append(entries, goodsEntry{idx: uint16(def.Idx), price: uint16(price), stock: uint16(len(stock.Items))})
	}
	npc.mu.RUnlock()

	buf := make([]byte, 0, 2+len(entries)*6)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(entries)))
	buf = append(buf, count...)
	for _, e := range entries {
		entry := make([]byte, 6)
		binary.LittleEndian.PutUint16(entry[0:2], e.idx)
		binary.LittleEndian.PutUint16(entry[2:4], e.price)
		binary.LittleEndian.PutUint16(entry[4:6], e.stock)
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
	buf := make([]byte, 0, 2+len(goods)*6)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(goods)))
	buf = append(buf, count...)
	for _, g := range goods {
		entry := make([]byte, 6)
		def := p.ItemDB.GetByName(g.ItemName)
		if def != nil {
			binary.LittleEndian.PutUint16(entry[0:2], uint16(def.Idx))
		}
		binary.LittleEndian.PutUint16(entry[2:4], uint16(g.Price))
		binary.LittleEndian.PutUint16(entry[4:6], 9999) // DB回退路径使用无限库存
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(len(goods)), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

// sendMakeDrugList 发送制药配方列表（消息ID 712）
func (p *PlayObject) sendMakeDrugList(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}
	// 简化实现：发送可制作的药品列表（StdMode 0-3 为药水类）
	var goods []MerchantGoods
	for i := range p.ItemDB.Items {
		item := &p.ItemDB.Items[i]
		if item.StdMode <= 3 && item.Price > 0 {
			goods = append(goods, MerchantGoods{ItemName: item.Name, Price: int(item.Price)})
		}
		if len(goods) >= 50 {
			break
		}
	}
	buf := make([]byte, 0, 2+len(goods)*6)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(goods)))
	buf = append(buf, count...)
	for _, g := range goods {
		entry := make([]byte, 6)
		def := p.ItemDB.GetByName(g.ItemName)
		if def != nil {
			binary.LittleEndian.PutUint16(entry[0:2], uint16(def.Idx))
		}
		binary.LittleEndian.PutUint16(entry[2:4], uint16(g.Price))
		binary.LittleEndian.PutUint16(entry[4:6], 9999) // 无限库存
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendUserMakeDrugItemList, npc.ID, uint16(len(goods)), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

// calcBuyPrice 计算物品实例的购买价格。
// Delphi: ObjNpc.pas:1838-1919 GetUserItemPrice + ObjNpc.pas:1385-1416 GetUserPrice。
func (p *PlayObject) calcBuyPrice(npc *NpcObject, def *ItemDef, item *protocol.UserItem) int {
	// 基础价格：PriceList 优先，否则 StdItem.Price
	price := int(def.Price)
	if npc != nil {
		npc.mu.RLock()
		if ip, ok := npc.PriceList[def.Idx]; ok && ip.Price > 0 {
			price = ip.Price
		}
		npc.mu.RUnlock()
	}

	// StdMode > 4 的装备：附加值 + 耐久比（Delphi ObjNpc.pas:1880-1915）
	if def.StdMode > 4 && item != nil {
		addon := 0
		for i := 0; i < 8 && i < len(item.BtValue); i++ {
			// 武器(5)/衣服(6)：跳过 idx 4/9，idx 6 只计 (val-10)*2
			if (def.StdMode == 5 || def.StdMode == 6) && (i == 4 || i == 9) {
				continue
			}
			v := int(item.BtValue[i])
			if (def.StdMode == 5 || def.StdMode == 6) && i == 6 {
				if v > 10 {
					addon += (v - 10) * 2
				}
			} else {
				addon += v
			}
		}
		if addon > 0 {
			price = price / 5 * addon
		}
		// DuraMax 比率
		if def.DuraMax > 0 && item.DuraMax > 0 {
			price = price * int(item.DuraMax) / int(def.DuraMax)
		}
		// Dura 比率：线性折旧至半价
		if item.DuraMax > 0 {
			loss := price / 2 * (int(item.DuraMax) - int(item.Dura)) / int(item.DuraMax)
			price -= loss
			if price < 2 {
				price = 2
			}
		}
	}

	// PriceRate% + 城堡成员折扣（Delphi ObjNpc.pas:1385-1416）
	if npc != nil {
		rate := npc.PriceRate
		if npc.Castle && p.Engine != nil && p.Engine.Castle != nil &&
			p.Engine.Castle.IsDefendingGuild(p.GuildName) {
			rate = rate * p.Engine.Config.GetCastleDiscount() / 100
			if rate < p.Engine.Config.GetCastleMinRate() {
				rate = p.Engine.Config.GetCastleMinRate()
			}
		}
		price = price * rate / 100
	}
	if price <= 0 {
		price = 1
	}
	return price
}

func (p *PlayObject) HandleBuyItem(msg SendMessage, server *netserver.TCPServer) {
	// 距离验证：确保玩家仍在NPC附近
	if p.CurrentNpc != nil && !p.isNearNpc(p.CurrentNpc) {
		p.sendBuyFail(server)
		return
	}

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
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		p.sendBuyFail(server)
		return
	}

	var npc *NpcObject
	if p.CurrentNpc != nil {
		npc = p.CurrentNpc
	}

	// 先从库存选取物品，再按实例计算价格（Delphi ObjNpc.pas:1922-2028）
	var item *protocol.UserItem
	if npc != nil && len(npc.GoodsList) > 0 {
		npc.mu.Lock()
		if stock, ok := npc.GoodsList[def.Name]; ok && len(stock.Items) > 0 {
			item = stock.Items[0]
			stock.Items = stock.Items[1:]
		}
		npc.mu.Unlock()
	}
	if item == nil {
		item = p.ItemDB.CreateUserItem(def.Idx)
	}
	if item == nil {
		p.sendBuyFail(server)
		return
	}

	price := p.calcBuyPrice(npc, def, item)
	if p.Gold < price {
		// 放回库存
		if npc != nil {
			npc.mu.Lock()
			stock := npc.GoodsList[def.Name]
			if stock == nil {
				stock = &GoodsStock{}
				npc.GoodsList[def.Name] = stock
			}
			stock.Items = append([]*protocol.UserItem{item}, stock.Items...)
			npc.mu.Unlock()
		}
		p.sendBuyFail(server)
		return
	}

	// 分配唯一 MakeIndex
	if p.Engine != nil {
		p.Engine.mu.Lock()
		item.MakeIndex = int32(p.Engine.nextItemID)
		p.Engine.nextItemID++
		p.Engine.mu.Unlock()
	}
	p.ItemList = append(p.ItemList, item)

	p.Gold -= price
	// 城堡税（Delphi Castle.pas:1022-1061）
	if npc != nil && npc.Castle && p.Engine != nil && p.Engine.Castle != nil {
		p.Engine.Castle.CollectTax(int64(price * p.Engine.Config.GetCastleTaxRate() / 100))
	}
	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemSuccess, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
}

// HandleGetDetailItem — Delphi ClientUserBuyItem 的 CM_USERGETDETAILITEM 分支
//（ObjBase.pas:16157-16185）+ Merchant.ClientGetDetailGoodsList（ObjNpc.pas:2031）：
// 查询商人处某物品的实例明细（最多 10 条，含耐久/价格/MakeIndex）。
// Recog=商人ID，Param1=页偏移，Msg=物品名。
func (p *PlayObject) HandleGetDetailItem(msg SendMessage, server *netserver.TCPServer) {
	// Delphi: m_boDealing 交易中禁止（ObjBase.pas:16162）
	if p.Deal != nil {
		return
	}
	var npc *NpcObject
	if p.envir != nil {
		npc, _ = p.envir.getNpcByID(int32(msg.Param1))
	}
	// Delphi: 商人存在、可购买、同图、距离≤15（ObjBase.pas:16163-16168）
	if npc == nil || !npc.IsMerchant || !npc.CanBuy || npc.envir != p.envir ||
		abs(npc.CurrX-p.CurrX) > 15 || abs(npc.CurrY-p.CurrY) > 15 {
		return
	}
	if p.ItemDB == nil {
		return
	}

	name := msg.Msg
	offset := msg.Param2
	var items []*protocol.UserItem
	npc.mu.RLock()
	if stock, ok := npc.GoodsList[name]; ok {
		items = append(items, stock.Items...)
	}
	npc.mu.RUnlock()

	if offset < 0 {
		offset = 0
	}
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + 10
	if end > len(items) {
		end = len(items)
	}

	buf := make([]byte, 0, 2+(end-offset)*14)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(end-offset))
	buf = append(buf, count...)
	for i := offset; i < end; i++ {
		item := items[i]
		def := p.ItemDB.GetByIdx(int(item.WIndex))
		if def == nil {
			continue
		}
		entry := make([]byte, 14)
		binary.LittleEndian.PutUint16(entry[0:2], item.WIndex)
		binary.LittleEndian.PutUint16(entry[2:4], item.Dura)
		binary.LittleEndian.PutUint16(entry[4:6], item.DuraMax)
		binary.LittleEndian.PutUint32(entry[6:10], uint32(item.MakeIndex))
		binary.LittleEndian.PutUint32(entry[10:14], uint32(p.calcBuyPrice(npc, def, item)))
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendDetailGoodsList, npc.ID, uint16(end-offset), uint16(offset), 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) HandleSellItem(msg SendMessage, server *netserver.TCPServer) {
	// 距离验证：确保玩家仍在NPC附近
	if p.CurrentNpc != nil && !p.isNearNpc(p.CurrentNpc) {
		p.sendSellFail(server)
		return
	}

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

	// Delphi ObjNpc.pas:2134-2145：药水(StdMode=25)和卷轴(StdMode=30) Dura<4000 不可出售
	if (def.StdMode == 25 || def.StdMode == 30) && item.Dura < 4000 {
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
	// 距离验证：确保玩家仍在NPC附近
	if p.CurrentNpc != nil && !p.isNearNpc(p.CurrentNpc) {
		p.sendRepairFail(server)
		return
	}

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
		// 普通修理：永久降低 DuraMax（Delphi nRepairItemDecDura）
		lostDura := int(item.DuraMax) - int(item.Dura)
		decay := lostDura / p.Engine.Config.GetRepairDuraDivisor()
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

	// Delphi ObjNpc.pas:2476-2486：修理后跳转脚本标签
	if p.CurrentNpc != nil {
		if script := p.CurrentNpc.GetScript(); script != nil {
			if isSpecial {
				script.Execute("~@s_repair", p, p.CurrentNpc, server)
			} else {
				script.Execute("~@repair", p, p.CurrentNpc, server)
			}
		}
	}
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
	cfg := p.Engine.Config
	// Delphi 公式: price / 3 / DuraMax * (DuraMax - Dura)
	lostDura := int(item.DuraMax) - int(item.Dura)
	cost := int(def.Price) / 3 * lostDura / int(item.DuraMax)
	if cost < 1 {
		cost = 1
	}
	if isSpecial {
		cost *= cfg.GetSpecialRepairMult()
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
