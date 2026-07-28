package main

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const MaxBagItems = 46

func (p *PlayObject) GiveItem(itemIdx int) bool {
	if len(p.ItemList) >= MaxBagItems {
		return false
	}
	if p.ItemDB == nil {
		return false
	}
	def := p.ItemDB.GetByIdx(itemIdx)
	if def == nil {
		return false
	}
	// 唯一实例ID（Delphi g_MakeItemIdx）；DB idx 会在同名物品堆叠间冲突，
	// 导致基于 MakeIndex 寻址的操作出错。
	makeIndex := int32(itemIdx)
	if p.Engine != nil {
		p.Engine.mu.Lock()
		makeIndex = int32(p.Engine.nextItemID)
		p.Engine.nextItemID++
		p.Engine.mu.Unlock()
	}
	userItem := &protocol.UserItem{
		MakeIndex: makeIndex,
		WIndex:    uint16(itemIdx),
		Dura:      uint16(def.DuraMax),
		DuraMax:   uint16(def.DuraMax),
	}
	p.ItemList = append(p.ItemList, userItem)
	return true
}

// HandleAdjustBonus 应用已分配的加成点（CMAdjustBonus: Recog =
// 剩余点数，body = 按 TNakedAbility 顺序排列的 9×u16 增量
// DC,MC,SC,AC,MAC,HP,MP,Hit,Speed — Delphi SendAdjustBonus,
// ClMain.pas:3373-3379）。
func (p *PlayObject) HandleAdjustBonus(msg SendMessage, server *netserver.TCPServer) {
	raw := []byte(msg.Msg)
	if len(raw) < 18 {
		return
	}
	var deltas [9]int
	spent := 0
	for i := 0; i < 9; i++ {
		deltas[i] = int(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
		spent += deltas[i]
	}
	if spent <= 0 || spent > p.BonusPoint {
		return
	}
	p.BonusPoint -= spent
	// 每点加成效果（闭环数值）。
	p.WAbil.DC += uint32(deltas[0]) << 16
	p.WAbil.MC += uint32(deltas[1]) << 16
	p.WAbil.SC += uint32(deltas[2]) << 16
	p.WAbil.AC += uint32(deltas[3]) << 16
	p.WAbil.MAC += uint32(deltas[4]) << 16
	p.WAbil.MaxHP += uint16(deltas[5] * 5)
	p.WAbil.MaxMP += uint16(deltas[6] * 5)
	p.HitPoint += deltas[7]
	p.SpeedPoint += deltas[8]
	p.RecalcAbilitys()
	p.SendAbility(server)
	p.sendHealthSpell(server)
	resp := protocol.MakeDefaultMsg(protocol.SMAdjustBonus, int32(p.BonusPoint), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	log.Logf(log.LevelInfo, "Items", "%s allocated %d bonus points", p.Name, spent)
}

// findBagItem 返回指定 MakeIndex 物品在背包中的位置，
// 未找到返回 -1。客户端所有物品 CM 消息通过 MakeIndex 寻址实例：
// 客户端槽位布局由客户端维护，因此背包下标不具权威性。
func (p *PlayObject) findBagItem(makeIndex int32) int {
	for i, item := range p.ItemList {
		if item != nil && item.MakeIndex == makeIndex {
			return i
		}
	}
	return -1
}

func (p *PlayObject) sendDuraChange(server *netserver.TCPServer, item *protocol.UserItem) {
	if item == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMDuraChange, item.MakeIndex, item.Dura, item.DuraMax, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendBagItemsFull(server *netserver.TCPServer) {
	buf := make([]byte, 0, 2+len(p.ItemList)*10)
	count := make([]byte, 2)
	binary.LittleEndian.PutUint16(count, uint16(len(p.ItemList)))
	buf = append(buf, count...)
	for _, item := range p.ItemList {
		entry := make([]byte, 10)
		binary.LittleEndian.PutUint16(entry[0:2], item.WIndex)
		binary.LittleEndian.PutUint16(entry[2:4], item.Dura)
		binary.LittleEndian.PutUint16(entry[4:6], item.DuraMax)
		binary.LittleEndian.PutUint32(entry[6:10], uint32(item.MakeIndex))
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMBagItems, int32(len(p.ItemList)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) SendUseItemsFull(server *netserver.TCPServer) {
	buf := make([]byte, 13*10)
	for i := 0; i < 13; i++ {
		if p.UseItems[i] != nil {
			off := i * 10
			binary.LittleEndian.PutUint16(buf[off:off+2], p.UseItems[i].WIndex)
			binary.LittleEndian.PutUint16(buf[off+2:off+4], p.UseItems[i].Dura)
			binary.LittleEndian.PutUint16(buf[off+4:off+6], p.UseItems[i].DuraMax)
			binary.LittleEndian.PutUint32(buf[off+6:off+10], uint32(p.UseItems[i].MakeIndex))
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSendUseItems, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) HandleTakeOnItem(msg SendMessage, server *netserver.TCPServer) {
	// Param1 = MakeIndex（实例ID），Param2 = 目标槽位。
	bagIdx := p.findBagItem(int32(msg.Param1))
	slot := msg.Param2

	if slot < 0 || slot > 12 {
		p.sendTakeOnFail(server, 0)
		return
	}
	if bagIdx < 0 {
		p.sendTakeOnFail(server, 0)
		return
	}

	item := p.ItemList[bagIdx]
	if p.ItemDB == nil {
		p.sendTakeOnFail(server, 0)
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		p.sendTakeOnFail(server, 0)
		return
	}

	if !validEquipSlot(def.StdMode, slot) {
		p.sendTakeOnFail(server, 1)
		return
	}

	if def.NeedLevel > 0 && p.WAbil.Level < uint16(def.NeedLevel) {
		p.sendTakeOnFail(server, 2)
		return
	}

	if def != nil {
		slot := getEquipSlot(def.StdMode)
		if slot == protocol.UWeapon || slot == protocol.URightHand {
			if p.WAbil.HandWeight+uint16(def.Weight) > p.WAbil.MaxHandWeight && p.WAbil.MaxHandWeight > 0 {
				p.sendTakeOnFail(server, 3)
				return
			}
		} else {
			if p.WAbil.WearWeight+uint16(def.Weight) > p.WAbil.MaxWearWeight && p.WAbil.MaxWearWeight > 0 {
				p.sendTakeOnFail(server, 3)
				return
			}
		}
	}

	oldItem := p.UseItems[slot]

	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.UseItems[slot] = item

	if oldItem != nil {
		p.ItemList = append(p.ItemList, oldItem)
	}

	p.RecalcAbilitys()
	p.updateAppearance()

	resp := protocol.MakeDefaultMsg(protocol.SMTakeOnOK, int32(slot), uint16(bagIdx), 0, 0)
	server.Send(p.Session.ID, resp, "")

	p.SendBagItemsFull(server)
	p.SendUseItemsFull(server)
	p.sendHealthSpell(server)
	p.SendAbility(server)
	p.sendWeightChanged(server)

	log.Logf(log.LevelInfo, "Items", "%s equipped %s to slot %d", p.Name, def.Name, slot)
}

func (p *PlayObject) HandleTakeOffItem(msg SendMessage, server *netserver.TCPServer) {
	slot := msg.Param1

	if slot < 0 || slot > 12 {
		p.sendTakeOffFail(server)
		return
	}
	if p.UseItems[slot] == nil {
		p.sendTakeOffFail(server)
		return
	}
	if len(p.ItemList) >= MaxBagItems {
		p.sendTakeOffFail(server)
		return
	}

	item := p.UseItems[slot]
	p.UseItems[slot] = nil
	p.ItemList = append(p.ItemList, item)

	p.RecalcAbilitys()
	p.updateAppearance()

	resp := protocol.MakeDefaultMsg(protocol.SMTakeOffOK, int32(slot), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")

	p.SendBagItemsFull(server)
	p.SendUseItemsFull(server)
	p.sendHealthSpell(server)
	p.SendAbility(server)
	p.sendWeightChanged(server)

	name := "?"
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
		}
	}
	log.Logf(log.LevelInfo, "Items", "%s unequipped %s from slot %d", p.Name, name, slot)
}

func (p *PlayObject) HandleEatItem(msg SendMessage, server *netserver.TCPServer) {
	// Param1 = MakeIndex（实例ID；客户端布局由客户端维护）。
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		p.sendEatFail(server)
		return
	}

	item := p.ItemList[bagIdx]
	if p.ItemDB == nil {
		p.sendEatFail(server)
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		p.sendEatFail(server)
		return
	}

	healed := false
	if def.StdMode == 0 {
		heal := int(def.AC)
		if heal > 0 {
			hp := int(p.WAbil.HP) + heal
			if hp > int(p.WAbil.MaxHP) {
				hp = int(p.WAbil.MaxHP)
			}
			p.WAbil.HP = uint16(hp)
			healed = true
		}
	} else if def.StdMode == 1 {
		heal := int(def.AC)
		if heal > 0 {
			mp := int(p.WAbil.MP) + heal
			if mp > int(p.WAbil.MaxMP) {
				mp = int(p.WAbil.MaxMP)
			}
			p.WAbil.MP = uint16(mp)
			healed = true
		}
	}

	if !healed {
		p.sendEatFail(server)
		return
	}

	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)

	resp := protocol.MakeDefaultMsg(protocol.SMEatOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendHealthSpell(server)
	p.SendAbility(server)
	p.sendWeightChanged(server)

	log.Logf(log.LevelInfo, "Items", "%s used %s", p.Name, def.Name)
}

// makeLong 按 Delphi MakeLong 的方式打包 lo/hi 属性区间（lo word | hi word<<16）。
func makeLong(lo, hi int) uint32 {
	return uint32(uint16(lo)) | uint32(uint16(hi))<<16
}

// levelFormula 对应 Delphi Round((Level / X) * Level)，使用浮点除法。
func levelFormula(level int, divisor float64) int {
	return int(math.Round(float64(level) / divisor * float64(level)))
}

func (p *PlayObject) RecalcAbilitys() {
	level := int(p.WAbil.Level)
	if level < 1 {
		level = 1
	}

	// 各职业基础属性，Delphi RecalcLevelAbilitys（ObjBase.pas:1880-1941），
	// 即注释中的旧版公式（无配置等效值）。
	switch p.Job {
	case 0: // 战士 (jWarr, :1921-1936)
		p.WAbil.MaxHP = uint16(min(65535, 14+int(math.Round((float64(level)/4.0+4.5+float64(level)/20.0)*float64(level)))))
		p.WAbil.MaxMP = uint16(min(65535, 11+int(math.Round(float64(level)*3.5))))
		p.WAbil.MaxWeight = uint16(50 + levelFormula(level, 3))
		p.WAbil.MaxWearWeight = uint16(15 + levelFormula(level, 20))
		p.WAbil.MaxHandWeight = uint16(12 + levelFormula(level, 13))
		p.WAbil.DC = makeLong(max(level/5-1, 1), max(1, level/5))
		p.WAbil.MC = 0
		p.WAbil.SC = 0
		p.WAbil.AC = makeLong(0, level/7)
		p.WAbil.MAC = 0
	case 1: // 法师 (jWizard, :1904-1920)
		p.WAbil.MaxHP = uint16(min(65535, 14+int(math.Round((float64(level)/15.0+1.8)*float64(level)))))
		p.WAbil.MaxMP = uint16(min(65535, 13+int(math.Round((float64(level)/5.0+2.0)*2.2*float64(level)))))
		p.WAbil.MaxWeight = uint16(50 + levelFormula(level, 5))
		p.WAbil.MaxWearWeight = uint16(15 + levelFormula(level, 100))
		p.WAbil.MaxHandWeight = uint16(12 + levelFormula(level, 90))
		n := level / 7
		p.WAbil.DC = makeLong(max(n-1, 0), max(1, n))
		p.WAbil.MC = makeLong(max(n-1, 0), max(1, n))
		p.WAbil.SC = 0
		p.WAbil.AC = 0
		p.WAbil.MAC = 0
	default: // 道士 (jTaos, :1883-1903)
		p.WAbil.MaxHP = uint16(min(65535, 14+int(math.Round((float64(level)/6.0+2.5)*float64(level)))))
		p.WAbil.MaxMP = uint16(min(65535, 13+int(math.Round(float64(level)/8.0*2.2*float64(level)))))
		p.WAbil.MaxWeight = uint16(50 + levelFormula(level, 4))
		p.WAbil.MaxWearWeight = uint16(15 + levelFormula(level, 50))
		p.WAbil.MaxHandWeight = uint16(12 + levelFormula(level, 42))
		n := level / 7
		p.WAbil.DC = makeLong(max(n-1, 0), max(1, n))
		p.WAbil.MC = 0
		p.WAbil.SC = makeLong(max(n-1, 0), max(1, n))
		p.WAbil.AC = 0
		n = int(math.Round(float64(level) / 6.0))
		p.WAbil.MAC = makeLong(n/2, n+1)
	}

	// 命中/敏捷基础值（ObjBase.pas:18563-18566；DEFHIT=5, DEFSPEED=15，
	// 道士天生 +3 敏捷）。
	p.HitPoint = 5
	p.SpeedPoint = 15
	if p.Job == 2 {
		p.SpeedPoint += 3
	}

	// 当前背包重量（各物品定义重量之和）。
	var bagWeight int
	if p.ItemDB != nil {
		for _, item := range p.ItemList {
			if item == nil {
				continue
			}
			if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
				bagWeight += int(def.Weight)
			}
		}
	}
	p.WAbil.Weight = uint16(min(65535, bagWeight))

	if p.ItemDB != nil {
		for i := 0; i < 13; i++ {
			if p.UseItems[i] == nil {
				continue
			}
			def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
			if def == nil {
				continue
			}
			p.WAbil.AC += uint32(def.AC) | uint32(def.ACMax)<<16
			p.WAbil.MAC += uint32(def.MAC) | uint32(def.MACMax)<<16
			p.WAbil.DC += uint32(def.DC) | uint32(def.DCMax)<<16
			p.WAbil.MC += uint32(def.MC) | uint32(def.MCMax)<<16
			p.WAbil.SC += uint32(def.SC) | uint32(def.SCMax)<<16
		}

		var wearWeight, handWeight uint16
		for i := 0; i < 13; i++ {
			if p.UseItems[i] == nil {
				continue
			}
			def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
			if def == nil {
				continue
			}
			if i == protocol.UWeapon || i == protocol.URightHand {
				handWeight += uint16(def.Weight)
			} else {
				wearWeight += uint16(def.Weight)
			}
		}
		p.WAbil.WearWeight = wearWeight
		p.WAbil.HandWeight = handWeight

		var luck int
		for i := 0; i < 13; i++ {
			if p.UseItems[i] == nil {
				continue
			}
			def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
			if def == nil {
				continue
			}
			if def.Source > 0 {
				luck += int(def.Source)
			}
		}
		p.Luck = luck

		p.checkSetBonuses()
	}

	if p.WAbil.HP > p.WAbil.MaxHP {
		p.WAbil.HP = p.WAbil.MaxHP
	}
	if p.WAbil.MP > p.WAbil.MaxMP {
		p.WAbil.MP = p.WAbil.MaxMP
	}

	p.CheckSpecialItemEffects()
}

func (p *PlayObject) updateAppearance() {
	p.DressLook = 0
	p.WeaponLook = 0
	if p.ItemDB == nil {
		return
	}
	if p.UseItems[protocol.UDress] != nil {
		def := p.ItemDB.GetByIdx(int(p.UseItems[protocol.UDress].WIndex))
		if def != nil {
			p.DressLook = def.Shape*2 + p.Gender
		}
	}
	if p.UseItems[protocol.UWeapon] != nil {
		def := p.ItemDB.GetByIdx(int(p.UseItems[protocol.UWeapon].WIndex))
		if def != nil {
			p.WeaponLook = def.Shape*2 + p.Gender
		}
	}
}

// getEquipSlot 将物品 StdMode 映射到主装备槽位
//（ClFunc.pas:618-634）。双槽位物品（戒指/手镯）返回左侧槽位；
// validEquipSlot 接受左右任一侧。
func getEquipSlot(stdMode byte) int {
	switch stdMode {
	case 5, 6:
		return protocol.UWeapon
	case 10, 11:
		return protocol.UDress
	case 15, 16:
		return protocol.UHelmet
	case 19, 20, 21:
		return protocol.UNecklace
	case 22, 23:
		return protocol.URingL
	case 24, 26:
		return protocol.UArmRingL
	case 28, 29, 30:
		return protocol.URightHand
	case 25, 51:
		return protocol.UBujuk
	case 52, 62:
		return protocol.UBoots
	case 53, 63:
		return protocol.UCharm
	case 54, 64:
		return protocol.UBelt
	}
	return -1
}

// validEquipSlot 判断 slot 是否为 stdMode 的合法装备目标，
// 双槽位物品允许左右任一侧（FState:3300-3318）。
func validEquipSlot(stdMode byte, slot int) bool {
	switch stdMode {
	case 22, 23:
		return slot == protocol.URingL || slot == protocol.URingR
	case 24, 26:
		return slot == protocol.UArmRingL || slot == protocol.UArmRingR
	}
	return getEquipSlot(stdMode) == slot
}

func (p *PlayObject) sendTakeOnFail(server *netserver.TCPServer, code int) {
	resp := protocol.MakeDefaultMsg(protocol.SMTakeOnFail, int32(code), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	// 客户端采用乐观装备策略；全量刷新用于恢复权威的背包/装备状态
	//（SMBagItems/SMSendUseItems 处理逻辑）。
	p.SendBagItemsFull(server)
	p.SendUseItemsFull(server)
}

func (p *PlayObject) sendTakeOffFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMTakeOffFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	p.SendUseItemsFull(server)
}

func (p *PlayObject) sendEatFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMEatFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) CheckSpecialItemEffects() {
	p.HasParalysis = false
	p.HasRevival = false
	p.HasTeleport = false
	p.HasProbe = false
	p.HasFlame = false
	p.HasRecovery = false
	p.HasAngry = false
	p.HasMagicShield = false
	p.HasMuscle = false

	if p.ItemDB == nil {
		return
	}
	for i := 0; i < 13; i++ {
		if p.UseItems[i] == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
		if def == nil {
			continue
		}
		switch def.AniCount {
		case 112:
			p.HasTeleport = true
		case 113:
			p.HasParalysis = true
		case 114:
			p.HasRevival = true
		case 115:
			p.HasFlame = true
		case 116:
			p.HasRecovery = true
		case 117:
			p.HasAngry = true
		case 118:
			p.HasMagicShield = true
		case 119:
			p.HasMuscle = true
		case 121:
			p.HasProbe = true
		}
	}
}

func (p *PlayObject) checkSetBonuses() {
	nameCounts := make(map[string]int)
	for i := 0; i < 13; i++ {
		if p.UseItems[i] == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
		if def == nil {
			continue
		}
		name := def.Name
		for _, setName := range []string{"记忆", "魔血", "虹魔", "精神"} {
			if strings.Contains(name, setName) {
				nameCounts[setName]++
			}
		}
	}

	if nameCounts["记忆"] >= 4 {
		p.WAbil.AC += 2 | 2<<16
		p.WAbil.MAC += 2 | 2<<16
	}
	if nameCounts["魔血"] >= 3 {
		p.WAbil.MaxHP += 50
	}
	if nameCounts["虹魔"] >= 3 {
		p.WAbil.DC += 5 | 5<<16
	}
}

func (p *PlayObject) countItem(name string) int {
	count := 0
	if p.ItemDB == nil {
		return 0
	}
	def := p.ItemDB.GetByName(name)
	if def == nil {
		return 0
	}
	for _, item := range p.ItemList {
		if item != nil && int(item.WIndex) == def.Idx {
			count++
		}
	}
	return count
}

func (p *PlayObject) takeItem(name string, count int) {
	if p.ItemDB == nil {
		return
	}
	def := p.ItemDB.GetByName(name)
	if def == nil {
		return
	}
	remaining := make([]*protocol.UserItem, 0, len(p.ItemList))
	taken := 0
	for _, item := range p.ItemList {
		if taken < count && item != nil && int(item.WIndex) == def.Idx {
			taken++
			continue
		}
		remaining = append(remaining, item)
	}
	p.ItemList = remaining
}
