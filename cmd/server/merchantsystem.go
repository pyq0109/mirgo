package main

import (
	"sort"
	"strconv"
	"strings"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

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

// goodsSubMenu 按 Delphi 规则判定子菜单标志（ObjNpc.pas:1435-1437）：
// 消耗品（StdMode<=4 / 42 / 31）无子菜单，装备类有。
func goodsSubMenu(def *ItemDef) int {
	if def.StdMode <= 4 || def.StdMode == 42 || def.StdMode == 31 {
		return 0
	}
	return 1
}

// sendGoodsListFromStock 发送商品列表（Delphi TMerchant.UserSelect 内嵌 BuyItem，
// ObjNpc.pas:1424-1457）：body 为文本 "名字/子菜单/价格/库存/..." 经 EncodeString，
// 条目数放消息头 Param（ObjBase.pas:5855-5862）。
// 顺序按 RefillConfig（脚本 [goods] 顺序）保证确定性，Delphi m_GoodsList 为有序 TList。
func (p *PlayObject) sendGoodsListFromStock(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}

	var sb strings.Builder
	count := 0
	seen := make(map[string]bool)
	appendEntry := func(name string) {
		stock := npc.GoodsList[name]
		if stock == nil || len(stock.Items) == 0 {
			return
		}
		def := p.ItemDB.GetByName(name)
		if def == nil {
			return
		}
		price := int(def.Price)
		if ip, ok := npc.PriceList[def.Idx]; ok && ip.Price > 0 {
			price = ip.Price
		}
		price = price * npc.PriceRate / 100
		if price <= 0 {
			price = 1
		}
		sb.WriteString(name + "/" + strconv.Itoa(goodsSubMenu(def)) + "/" +
			strconv.Itoa(price) + "/" + strconv.Itoa(len(stock.Items)) + "/")
		count++
		seen[name] = true
	}

	npc.mu.RLock()
	for _, cfg := range npc.RefillConfig {
		appendEntry(cfg.ItemName)
	}
	// RefillConfig 之外的库存（如玩家卖入的物品）按名字排序补充。
	var extras []string
	for name := range npc.GoodsList {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		appendEntry(name)
	}
	npc.mu.RUnlock()

	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(count), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
}

// sendGoodsListFromDB 回退商品列表（Go 闭环专有，Delphi 无对应物）：
// 无 [goods] 脚本的商人从物品库取前 50 件有价商品，格式与库存路径一致。
func (p *PlayObject) sendGoodsListFromDB(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}
	var sb strings.Builder
	count := 0
	for i := range p.ItemDB.Items {
		item := &p.ItemDB.Items[i]
		if item.Price > 0 && item.StdMode < 40 {
			sb.WriteString(item.Name + "/" + strconv.Itoa(goodsSubMenu(item)) + "/" +
				strconv.Itoa(int(item.Price)) + "/9999/")
			count++
		}
		if count >= 50 {
			break
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendGoodsList, npc.ID, uint16(count), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
}

// sendMakeDrugList 发送制药配方列表（消息ID 712）。
// Delphi ClientGetSendMakeDrugList（ClMain.pas:5586-5619）与商品列表同一文本格式。
func (p *PlayObject) sendMakeDrugList(server *netserver.TCPServer, npc *NpcObject) {
	if p.ItemDB == nil {
		return
	}
	var sb strings.Builder
	count := 0
	for i := range p.ItemDB.Items {
		item := &p.ItemDB.Items[i]
		// StdMode 0-3 为药水类
		if item.StdMode <= 3 && item.Price > 0 {
			sb.WriteString(item.Name + "/0/" + strconv.Itoa(int(item.Price)) + "/9999/")
			count++
		}
		if count >= 50 {
			break
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendUserMakeDrugItemList, npc.ID, uint16(count), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
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

// HandleBuyItem — Delphi ClientUserBuyItem + Merchant.ClientBuyItem
//（ObjBase.pas:16157-16177、ObjNpc.pas:1922-2026）。
// Param1=商人ID，Param2=MakeIndex，Msg=物品名。
// 失败码：1=默认（名字/实例不匹配、负重门禁），2=背包满，3=金币不足
//（客户端 1/3 文案与码的原版错位照搬，见 ClMain.pas:4643-4649）。
func (p *PlayObject) HandleBuyItem(msg SendMessage, server *netserver.TCPServer) {
	// Delphi: m_boDealing 交易中禁止（ObjBase.pas:16162）
	if p.Deal != nil {
		return
	}
	var npc *NpcObject
	if p.envir != nil {
		npc, _ = p.envir.getNpcByID(int32(msg.Param1))
	}
	if npc == nil {
		npc = p.CurrentNpc
	}
	// Delphi: 商人存在、同图、距离≤15（ObjBase.pas:16163-16168）
	if npc == nil || !p.isNearNpc(npc) {
		p.sendBuyFail(server, 1)
		return
	}
	if p.ItemDB == nil {
		p.sendBuyFail(server, 1)
		return
	}
	name := msg.Msg
	makeIndex := int32(msg.Param2)
	def := p.ItemDB.GetByName(name)
	if def == nil {
		p.sendBuyFail(server, 1)
		return
	}

	// Delphi: 负重门禁 IsAddWeightAvailable（ObjNpc.pas:1957）；
	// 原版此分支保持 n1C=1（客户端文案与码错位，照搬）。
	if p.WAbil.MaxWeight > 0 && int(p.WAbil.Weight)+int(def.Weight) > int(p.WAbil.MaxWeight) {
		p.sendBuyFail(server, 1)
		return
	}
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		p.sendBuyFail(server, 2) // AddItemToBag 失败：你不能携带更多的物品
		return
	}

	// 按名字找堆栈；消耗品忽略 MakeIndex 取首件，装备须 MakeIndex 精确匹配
	//（ObjNpc.pas:1990-2000）。
	consumable := def.StdMode <= 4 || def.StdMode == 42 || def.StdMode == 31
	fromStock := false
	var item *protocol.UserItem
	npc.mu.Lock()
	if stock, ok := npc.GoodsList[name]; ok && len(stock.Items) > 0 {
		fromStock = true
		if consumable {
			item = stock.Items[0]
			stock.Items = stock.Items[1:]
		} else {
			for i := range stock.Items {
				if stock.Items[i].MakeIndex == makeIndex {
					item = stock.Items[i]
					stock.Items = append(stock.Items[:i], stock.Items[i+1:]...)
					break
				}
			}
		}
		// 堆栈清空则删除（ObjNpc.pas:2010-2014）
		if len(stock.Items) == 0 {
			delete(npc.GoodsList, name)
		}
	}
	npc.mu.Unlock()

	if item == nil {
		if fromStock {
			// 有库存但无匹配实例（如被其他玩家先买走）：按原版失败
			p.sendBuyFail(server, 1)
			return
		}
		// Go 闭环兜底：NPC 完全没有该物品库存（DB 回退商品）时创建新实例
		item = p.ItemDB.CreateUserItem(def.Idx)
		if item == nil {
			p.sendBuyFail(server, 1)
			return
		}
	}

	price := p.calcBuyPrice(npc, def, item)
	if p.Gold < price || price <= 0 {
		// 放回库存头部（沿用既有模式）
		npc.mu.Lock()
		stock := npc.GoodsList[name]
		if stock == nil {
			stock = &GoodsStock{}
			npc.GoodsList[name] = stock
		}
		stock.Items = append([]*protocol.UserItem{item}, stock.Items...)
		npc.mu.Unlock()
		p.sendBuyFail(server, 3) // 金币不足（原版客户端文案 3="你的重量太重了.."，ClMain.pas:4647）
		return
	}

	if item.MakeIndex == 0 {
		item.MakeIndex = p.Engine.allocItemID()
	}
	p.ItemList = append(p.ItemList, item)

	p.Gold -= price
	// 城堡税（Delphi Castle.pas:1022-1061）
	if npc.Castle && p.Engine != nil && p.Engine.Castle != nil {
		p.Engine.Castle.CollectTax(int64(price * p.Engine.Config.GetCastleTaxRate() / 100))
	}
	// Delphi RM_BUYITEM_SUCCESS：Recog=剩余金币、Param/Tag=Lo/Hi(MakeIndex)
	//（ObjBase.pas:5900-5909），客户端据此 SoldOutGoods（FState.pas:5077-5092）。
	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemSuccess, int32(p.Gold),
		uint16(uint32(item.MakeIndex)&0xFFFF), uint16(uint32(item.MakeIndex)>>16), 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
}

// HandleGetDetailItem — Delphi ClientUserBuyItem 的 CM_USERGETDETAILITEM 分支
//（ObjBase.pas:16157-16185）+ Merchant.ClientGetDetailGoodsList（ObjNpc.pas:2031-2110）：
// 查询商人处某物品的实例明细，每页最多 10 条。
// Param1=商人ID，Param2=页偏移（仅 clamp 后回显，Delphi 不用它跳过条目），Msg=物品名。
// body = 各条目 EncodeBuffer(TClientItem) 以 '/' 拼接后再整体 EncodeString。
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

	npc.mu.RLock()
	var items []*protocol.UserItem
	if stock, ok := npc.GoodsList[name]; ok {
		items = append(items, stock.Items...)
	}
	npc.mu.RUnlock()

	var def *ItemDef
	if len(items) > 0 {
		def = p.ItemDB.GetByIdx(int(items[0].WIndex))
	}
	if def == nil || def.Name != name {
		return
	}

	// Delphi ObjNpc.pas:2055-2058：偏移越界时 clamp 为 max(0, count-10)，
	// 之后仅回显，不参与取数。
	if len(items)-1 < offset {
		offset = len(items) - 10
		if offset < 0 {
			offset = 0
		}
	}

	// Delphi ObjNpc.pas:2059-2072：倒序取最多 10 条。
	var sb strings.Builder
	count := 0
	for ii := len(items) - 1; ii >= 0; ii-- {
		item := items[ii]
		ci := protocol.ClientItem{
			S:         StdItemOf(def),
			MakeIndex: item.MakeIndex,
			Dura:      item.Dura,
			// Delphi 把单价塞进 DuraMax 字段（客户端价格列显示它）；
			// 原版 TClientItem.DuraMax 为 Word，溢出行为与原版一致。
			DuraMax: uint16(p.calcBuyPrice(npc, def, item)),
		}
		sb.WriteString(protocol.EncodeBuffer(protocol.EncodeClientItem(&ci)) + "/")
		count++
		if count >= 10 {
			break
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendDetailGoodsList, npc.ID, uint16(count), uint16(offset), 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(sb.String()))
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

	// 将物品添加到 NPC 商品列表（深拷贝，修复 Delphi 原始 bug；
	// 保留 MakeIndex 以便详细列表/按实例购买能匹配）。
	if p.CurrentNpc != nil && p.CurrentNpc.IsMerchant {
		npc := p.CurrentNpc
		itemCopy := &protocol.UserItem{
			MakeIndex: item.MakeIndex,
			WIndex:    item.WIndex,
			Dura:      item.Dura,
			DuraMax:   item.DuraMax,
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

func (p *PlayObject) sendBuyFail(server *netserver.TCPServer, code int) {
	resp := protocol.MakeDefaultMsg(protocol.SMBuyItemFail, int32(code), 0, 0, 0)
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
