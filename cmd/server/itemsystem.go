package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"strings"
	"time"

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

	// Delphi CheckTakeOnItems (ObjBase.pas:22970): 性别检查。
	if def.StdMode == 10 && p.Gender != 0 {
		p.sendTakeOnFail(server, 2)
		return
	}
	if def.StdMode == 11 && p.Gender != 1 {
		p.sendTakeOnFail(server, 2)
		return
	}

	// Delphi CheckTakeOnItems: Need 类型检查（0=等级 1=DC 2=MC 3=SC）。
	if def.NeedLevel > 0 {
		ok := false
		switch def.Need {
		case 0:
			ok = p.WAbil.Level >= uint16(def.NeedLevel)
		case 1:
			ok = int(p.WAbil.DC>>16) >= int(def.NeedLevel)
		case 2:
			ok = int(p.WAbil.MC>>16) >= int(def.NeedLevel)
		case 3:
			ok = int(p.WAbil.SC>>16) >= int(def.NeedLevel)
		default:
			ok = p.WAbil.Level >= uint16(def.NeedLevel)
		}
		if !ok {
			p.sendTakeOnFail(server, 2)
			return
		}
	}

	{
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

	used := false
	switch def.StdMode {
	case 0: // 药水（Delphi EatItems, ObjBase.pas:23324）：AC=HP, MAC=MP。
		if def.AC > 0 {
			hp := int(p.WAbil.HP) + int(def.AC)
			if hp > int(p.WAbil.MaxHP) {
				hp = int(p.WAbil.MaxHP)
			}
			p.WAbil.HP = uint16(hp)
			used = true
		}
		if def.MAC > 0 {
			mp := int(p.WAbil.MP) + int(def.MAC)
			if mp > int(p.WAbil.MaxMP) {
				mp = int(p.WAbil.MaxMP)
			}
			p.WAbil.MP = uint16(mp)
			used = true
		}
	case 1, 2: // 食物/杂项。
		if def.AC > 0 {
			hp := int(p.WAbil.HP) + int(def.AC)
			if hp > int(p.WAbil.MaxHP) {
				hp = int(p.WAbil.MaxHP)
			}
			p.WAbil.HP = uint16(hp)
			used = true
		}
	case 3: // 特殊消耗品，按 Shape 分发。
		used = p.useSpecialItem(def, server)
	case 4: // 技能书（Delphi ReadBook, ObjBase.pas:23443）。
		used = p.readBook(def, server)
	}

	if !used {
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

// useSpecialItem 处理 StdMode=3 的特殊消耗品（Delphi EatUseItems）。
func (p *PlayObject) useSpecialItem(def *ItemDef, server *netserver.TCPServer) bool {
	switch def.Shape {
	case 1: // 地牢逃脱卷：传送至安全区。
		return p.teleportToSafe(server)
	case 2: // 随机传送卷：当前地图随机位置。
		return p.teleportRandom(server)
	case 3, 5: // 回城卷 / 行会回城卷。
		return p.teleportToSafe(server)
	case 4: // 祝福油：武器幸运 +1。
		weapon := p.UseItems[protocol.UWeapon]
		if weapon == nil {
			p.sysMsg(server, "请先装备武器")
			return false
		}
		if weapon.BtValue[7] >= 7 {
			p.sysMsg(server, "幸运已达上限")
			return false
		}
		weapon.BtValue[7]++
		p.sendDuraChange(server, weapon)
		p.RecalcAbilitys()
		return true
	case 9, 10: // 修复油 / 战神油：修复武器耐久。
		weapon := p.UseItems[protocol.UWeapon]
		if weapon == nil {
			p.sysMsg(server, "请先装备武器")
			return false
		}
		weapon.Dura = weapon.DuraMax
		p.sendDuraChange(server, weapon)
		return true
	case 12: // 临时 Buff（神水/精酿）。
		return p.applyBuff(def, server)
	}
	return false
}

// teleportToSafe 传送至安全区（回城卷/地牢逃脱卷）。
func (p *PlayObject) teleportToSafe(server *netserver.TCPServer) bool {
	safeMap, safeX, safeY := GetSafeZonePoint()
	if p.MapMgr == nil {
		return false
	}
	env := p.MapMgr.FindMap(safeMap)
	if env == nil {
		return false
	}
	if env.Name == p.MapName {
		// 同地图内传送。
		if p.envir != nil {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.envir.broadcastRefMsg(p.BaseObject, RM_DISAPPEAR, p.ID, p.CurrX, p.CurrY, p.Dir)
		}
		p.CurrX, p.CurrY = safeX, safeY
		p.envir.AddObject(safeX, safeY, OS_MOVINGOBJECT, p)
		p.envir.broadcastRefMsg(p.BaseObject, RM_LOGON, p.ID, safeX, safeY, p.Dir)
	} else {
		p.EnterAnotherMap(server, env, safeX, safeY)
	}
	return true
}

// teleportRandom 当前地图随机传送。
func (p *PlayObject) teleportRandom(server *netserver.TCPServer) bool {
	if p.envir == nil {
		return false
	}
	for tries := 0; tries < 50; tries++ {
		x := rand.Intn(p.envir.Width)
		y := rand.Intn(p.envir.Height)
		if p.envir.CanWalk(x, y) {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.envir.broadcastRefMsg(p.BaseObject, RM_DISAPPEAR, p.ID, p.CurrX, p.CurrY, p.Dir)
			p.CurrX, p.CurrY = x, y
			p.envir.AddObject(x, y, OS_MOVINGOBJECT, p)
			p.envir.broadcastRefMsg(p.BaseObject, RM_LOGON, p.ID, x, y, p.Dir)
			return true
		}
	}
	return false
}

// applyBuff 应用临时 Buff（StdMode=3, Shape=12）。
// 数据编码：DC/MC/SC=加成值, AC(ACMax=0)=HP, MAC(MACMax=0)=MP, ACMax=HitSpeed, MACMax=持续秒数。
func (p *PlayObject) applyBuff(def *ItemDef, server *netserver.TCPServer) bool {
	duration := int(def.MACMax)
	if duration <= 0 {
		return false
	}
	p.BuffDC = int(def.DC)
	p.BuffMC = int(def.MC)
	p.BuffSC = int(def.SC)
	p.BuffHP = 0
	p.BuffMP = 0
	if def.ACMax == 0 {
		p.BuffHP = int(def.AC)
	}
	if def.MACMax != 0 && def.MAC == 0 {
		// MACMax is duration, MAC is MP bonus only when MACMax is not being used as duration.
	}
	p.BuffHitSpeed = int(def.ACMax)
	p.BuffExpireTick = time.Now().UnixMilli() + int64(duration)*1000
	p.RecalcAbilitys()
	p.SendAbility(server)
	return true
}

// readBook 学习技能书（StdMode=4, Delphi ReadBook ObjBase.pas:23443）。
func (p *PlayObject) readBook(def *ItemDef, server *netserver.TCPServer) bool {
	if p.MagicDB == nil {
		return false
	}
	magic := p.MagicDB.GetByName(def.Name)
	if magic == nil {
		p.sysMsg(server, "这本技能书无法学习")
		return false
	}
	if magic.Job >= 0 && magic.Job != int(p.Job) {
		p.sysMsg(server, "职业不符，无法学习")
		return false
	}
	if magic.NeedL1 > 0 && int(p.WAbil.Level) < magic.NeedL1 {
		p.sysMsg(server, "等级不足，无法学习")
		return false
	}
	for _, pm := range p.LearnedMagics {
		if pm.MagID == magic.MagID {
			p.sysMsg(server, "已经学会了这个技能")
			return false
		}
	}
	p.LearnedMagics = append(p.LearnedMagics, &PlayerMagic{
		MagID: magic.MagID,
		Level: 0,
	})
	p.SendMyMagicFull(server)
	p.SendSpecialAttackFlags(server)
	log.Logf(log.LevelInfo, "Items", "%s learned magic %s (ID=%d)", p.Name, def.Name, magic.MagID)
	return true
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
		// 重置特殊属性。
		p.Luck = 0
		p.HitSpeed = 0
		p.AntiPoison = 0
		p.PoisonRecover = 0
		p.HealthRecover = 0
		p.SpellRecover = 0
		p.AntiMagic = 0

		for i := 0; i < 13; i++ {
			if p.UseItems[i] == nil {
				continue
			}
			item := p.UseItems[i]
			// Delphi: 耐久为 0 的装备不提供属性 (ObjBase.pas:22478-22516)
			if item.Dura <= 0 {
				continue
			}
			def := p.ItemDB.GetByIdx(int(item.WIndex))
			if def == nil {
				continue
			}
			// 基础五维。
			p.WAbil.AC += uint32(def.AC) | uint32(def.ACMax)<<16
			p.WAbil.MAC += uint32(def.MAC) | uint32(def.MACMax)<<16
			p.WAbil.DC += uint32(def.DC) | uint32(def.DCMax)<<16
			p.WAbil.MC += uint32(def.MC) | uint32(def.MCMax)<<16
			p.WAbil.SC += uint32(def.SC) | uint32(def.SCMax)<<16

			// Delphi ApplyItemParameters (ItmUnit.pas:556): 按 StdMode 映射特殊属性。
			switch {
			case def.StdMode == 5 || def.StdMode == 6: // 武器
				p.HitPoint += int(def.ACMax)
				if def.MACMax > 10 {
					p.HitSpeed += int(def.MACMax)
				} else {
					p.HitSpeed -= int(def.MACMax)
				}
				p.Luck += int(def.AC)
				// BtValue[10]: 升级结果加成。
				switch {
				case item.BtValue[10] >= 10 && item.BtValue[10] <= 12:
					v := uint32(item.BtValue[10] - 9)
					p.WAbil.DC += v | v<<16
				case item.BtValue[10] >= 20 && item.BtValue[10] <= 22:
					v := uint32(item.BtValue[10] - 19)
					p.WAbil.MC += v | v<<16
				case item.BtValue[10] >= 30 && item.BtValue[10] <= 32:
					v := uint32(item.BtValue[10] - 29)
					p.WAbil.SC += v | v<<16
				}
				// BtValue[0..2]: 升级点数。
				if item.BtValue[0] > 0 {
					p.WAbil.DC += uint32(item.BtValue[0]) | uint32(item.BtValue[0])<<16
				}
				if item.BtValue[1] > 0 {
					p.WAbil.MC += uint32(item.BtValue[1]) | uint32(item.BtValue[1])<<16
				}
				if item.BtValue[2] > 0 {
					p.WAbil.SC += uint32(item.BtValue[2]) | uint32(item.BtValue[2])<<16
				}
				// BtValue[12]: 诅咒惩罚（每级 -5% DC）。
				if item.BtValue[12] > 0 {
					penalty := int(item.BtValue[12]) * 5
					dcMin := int(p.WAbil.DC & 0xFFFF)
					dcMax := int(p.WAbil.DC >> 16)
					dcMin = dcMin * (100 - penalty) / 100
					dcMax = dcMax * (100 - penalty) / 100
					p.WAbil.DC = uint32(dcMin) | uint32(dcMax)<<16
				}
			case def.StdMode == 10 || def.StdMode == 11: // 衣服
				if def.Source > 0 {
					p.Luck += int(def.Source)
				} else if def.Source < 0 {
					p.Luck += int(def.Source)
				}
				// BtValue[0..4]: 随机加成 AC/MAC/DC/MC/SC。
				p.WAbil.AC += uint32(item.BtValue[0]) | uint32(item.BtValue[0])<<16
				p.WAbil.MAC += uint32(item.BtValue[1]) | uint32(item.BtValue[1])<<16
				p.WAbil.DC += uint32(item.BtValue[2]) | uint32(item.BtValue[2])<<16
				p.WAbil.MC += uint32(item.BtValue[3]) | uint32(item.BtValue[3])<<16
				p.WAbil.SC += uint32(item.BtValue[4]) | uint32(item.BtValue[4])<<16
			case def.StdMode == 19: // 项链（抗魔）
				p.AntiMagic += int(def.ACMax)
				p.Luck += int(def.MACMax)
			case def.StdMode == 20 || def.StdMode == 24: // 项链（命中+敏捷）
				p.HitPoint += int(def.ACMax)
				p.SpeedPoint += int(def.MACMax)
			case def.StdMode == 21 || def.StdMode == 54 || def.StdMode == 64: // 项链（恢复）
				p.HealthRecover += int(def.ACMax)
				p.SpellRecover += int(def.MACMax)
				p.HitSpeed += int(def.AC)
				p.HitSpeed -= int(def.MAC)
			case def.StdMode == 22 || def.StdMode == 23: // 戒指（抗毒）
				p.AntiPoison += int(def.ACMax)
				p.PoisonRecover += int(def.MACMax)
			case def.StdMode == 62: // 腰带（负重）
				p.WAbil.MaxHandWeight += uint16(def.ACMax)
				p.WAbil.MaxWeight += uint16(def.MAC)
				p.WAbil.MaxWearWeight += uint16(def.MACMax)
			case def.StdMode == 63: // 宝石（HP/MP）
				p.WAbil.MaxHP += uint16(def.AC)
				p.WAbil.MaxMP += uint16(def.ACMax)
			}

			// 饰品 BtValue[0..4] 随机加成（StdMode 15,19-26,51-54,62-64）。
			if def.StdMode >= 15 && def.StdMode <= 26 || def.StdMode >= 51 && def.StdMode <= 64 {
				if def.StdMode != 62 && def.StdMode != 63 {
					p.WAbil.AC += uint32(item.BtValue[0]) | uint32(item.BtValue[0])<<16
					p.WAbil.MAC += uint32(item.BtValue[1]) | uint32(item.BtValue[1])<<16
					p.WAbil.DC += uint32(item.BtValue[2]) | uint32(item.BtValue[2])<<16
					p.WAbil.MC += uint32(item.BtValue[3]) | uint32(item.BtValue[3])<<16
					p.WAbil.SC += uint32(item.BtValue[4]) | uint32(item.BtValue[4])<<16
				}
			}
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

		p.checkSetBonuses()

		if p.HasMuscle {
			p.WAbil.MaxWeight *= 2
			p.WAbil.MaxWearWeight *= 2
			p.WAbil.MaxHandWeight *= 2
		}
	}

	// 临时 Buff 加成（StdMode 3, Shape 12 神水/精酿）。
	if p.BuffExpireTick > 0 && time.Now().UnixMilli() < p.BuffExpireTick {
		if p.BuffDC > 0 {
			v := uint32(p.BuffDC)
			p.WAbil.DC += v | v<<16
		}
		if p.BuffMC > 0 {
			v := uint32(p.BuffMC)
			p.WAbil.MC += v | v<<16
		}
		if p.BuffSC > 0 {
			v := uint32(p.BuffSC)
			p.WAbil.SC += v | v<<16
		}
		if p.BuffHP > 0 {
			p.WAbil.MaxHP += uint16(p.BuffHP)
		}
		if p.BuffMP > 0 {
			p.WAbil.MaxMP += uint16(p.BuffMP)
		}
		p.HitSpeed += p.BuffHitSpeed
	} else if p.BuffExpireTick > 0 {
		p.BuffExpireTick = 0
		p.BuffDC, p.BuffMC, p.BuffSC = 0, 0, 0
		p.BuffHP, p.BuffMP, p.BuffHitSpeed = 0, 0, 0
	}

	if p.WAbil.HP > p.WAbil.MaxHP {
		p.WAbil.HP = p.WAbil.MaxHP
	}
	if p.WAbil.MP > p.WAbil.MaxMP {
		p.WAbil.MP = p.WAbil.MaxMP
	}

	p.CheckSpecialItemEffects()
	p.calcSkillCombatStats()
}

// calcSkillCombatStats — 根据已学技能等级计算 HitPlus/HitDouble。
// Delphi: m_nHitPlus 来自攻杀剑术 Power; m_nHitDouble 来自烈火/狂风 Power。
func (p *PlayObject) calcSkillCombatStats() {
	p.HitPlus = 0
	p.HitDouble = 0

	if pm := p.findMagic(7); pm != nil && p.MagicDB != nil {
		if def := p.MagicDB.GetByID(7); def != nil {
			p.HitPlus = def.Power + pm.Level*(def.MaxPower-def.Power)/3
		}
	}
	if pm := p.findMagic(26); pm != nil && p.MagicDB != nil {
		if def := p.MagicDB.GetByID(26); def != nil {
			p.HitDouble = def.Power + pm.Level*(def.MaxPower-def.Power)/3
		}
	}
	if pm := p.findMagic(38); pm != nil && p.MagicDB != nil {
		if def := p.MagicDB.GetByID(38); def != nil {
			v := def.Power + pm.Level*(def.MaxPower-def.Power)/3
			if v > p.HitDouble {
				p.HitDouble = v
			}
		}
	}

	// 初始化 PowerHit 蓄力计数器
	if p.findMagic(7) != nil && p.PowerHitCount <= 0 {
		maxCycle := 7
		if pm := p.findMagic(7); pm != nil {
			maxCycle = 7 - pm.Level
		}
		if maxCycle < 1 {
			maxCycle = 1
		}
		p.PowerHitCount = rand.Intn(maxCycle) + 1
	}
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

	p.applyRingSkills()
}

// applyRingSkills 按 Delphi AddItemSkill/DelItemSkill 逻辑：
// 火焰戒指(115)授予火球术(MagID 1)，恢复戒指(116)授予治愈术(MagID 2)。
func (p *PlayObject) applyRingSkills() {
	const skillFireBall = 1
	const skillHealing = 2

	if p.HasFlame {
		p.learnMagic(skillFireBall, 1, 0)
	} else {
		p.removeMagicIfNoTrain(skillFireBall)
	}

	if p.HasRecovery {
		p.learnMagic(skillHealing, 1, 0)
	} else {
		p.removeMagicIfNoTrain(skillHealing)
	}
}

// removeMagicIfNoTrain 仅移除训练点为0的技能（戒指授予的技能），
// 避免误删玩家自行学习的同名技能。
func (p *PlayObject) removeMagicIfNoTrain(magID int) {
	for i, pm := range p.LearnedMagics {
		if pm.MagID == magID && pm.TrainPoint == 0 && pm.Level <= 1 {
			p.LearnedMagics = append(p.LearnedMagics[:i], p.LearnedMagics[i+1:]...)
			return
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
		for _, setName := range []string{"记忆", "魔血", "虹魔", "精神", "力量", "神秘", "技巧", "灵魂", "鬼灵", "祈祷", "诅咒", "平凡", "清心", "道不", "五行"} {
			if strings.Contains(name, setName) {
				nameCounts[setName]++
			}
		}
	}

	// 记忆套(4件)：允许行会传送（Delphi m_boRecallSuite → @recall 命令）
	p.HasRecallSuite = nameCounts["记忆"] >= 4

	// 魔血套(3件)：MP 转 HP（Delphi m_nMoXieSuite=50）
	if nameCounts["魔血"] >= 3 {
		convert := 50
		if p.WAbil.MaxMP <= uint16(convert) {
			convert = int(p.WAbil.MaxMP) - 1
		}
		if convert > 0 {
			p.WAbil.MaxMP -= uint16(convert)
			p.WAbil.MaxHP = uint16(min(65535, int(p.WAbil.MaxHP)+convert))
		}
	}

	// 虹魔套(3件)：命中 +2（Delphi m_AddAbil.wHitPoint += 2）
	if nameCounts["虹魔"] >= 3 {
		p.HitPoint += 2
	}

	// 精神套(4件)：DC +2/+5，攻速 +2（Delphi m_bopirit）
	if nameCounts["精神"] >= 4 {
		p.WAbil.DC += 2 | 7<<16
		p.SpeedPoint += 2
	}

	// 力量套(3件)：DC +1/+3，攻速 +1（Delphi m_boSmashSet）
	if nameCounts["力量"] >= 3 {
		p.WAbil.DC += 1 | 4<<16
		p.SpeedPoint++
	}

	// 神秘套(3件)：负重 +5/+20，MC +1/+2（Delphi m_boHwanDevilSet）
	if nameCounts["神秘"] >= 3 {
		p.WAbil.MaxHandWeight += 5
		p.WAbil.MaxWeight += 20
		p.WAbil.MC += 1 | 3<<16
	}

	// 技巧套(3件)：SC +1/+2（Delphi m_boPuritySet）
	if nameCounts["技巧"] >= 3 {
		p.WAbil.SC += 1 | 3<<16
	}

	// 平凡套(2件)：HP +50（Delphi m_boMundaneSet）
	if nameCounts["平凡"] >= 2 {
		p.WAbil.MaxHP = uint16(min(65535, int(p.WAbil.MaxHP)+50))
	}

	// 清心套(2件)：MP +50（Delphi m_boNokChiSet）
	if nameCounts["清心"] >= 2 {
		p.WAbil.MaxMP = uint16(min(65535, int(p.WAbil.MaxMP)+50))
	}

	// 道不套(2件)：HP +30, MP +30（Delphi m_boTaoBuSet）
	if nameCounts["道不"] >= 2 {
		p.WAbil.MaxHP = uint16(min(65535, int(p.WAbil.MaxHP)+30))
		p.WAbil.MaxMP = uint16(min(65535, int(p.WAbil.MaxMP)+30))
	}

	// 五行套(3件)：HP +30%，命中 +2（Delphi m_boFiveStringSet）
	if nameCounts["五行"] >= 3 {
		p.WAbil.MaxHP = uint16(min(65535, int(p.WAbil.MaxHP)*130/100))
		p.HitPoint += 2
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
