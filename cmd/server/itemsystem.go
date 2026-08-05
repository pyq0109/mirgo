package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)



func (p *PlayObject) GiveItem(itemIdx int) bool {
	if p.Engine != nil && len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
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
	p.WAbil.MaxHP += uint16(deltas[5] * p.Engine.Config.GetBonusHPPerPoint())
	p.WAbil.MaxMP += uint16(deltas[6] * p.Engine.Config.GetBonusMPPerPoint())
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

	// Delphi CheckTakeOnItems: Need 全分支检查（ObjBase.pas:23001-23260）。
	if !p.checkItemNeed(def) {
		p.sendTakeOnFail(server, 2)
		return
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

	// 被顶下的旧物品同样过四道禁脱校验（Delphi ObjBase.pas:17119-17151）。
	if oldItem != nil {
		oldDef := p.ItemDB.GetByIdx(int(oldItem.WIndex))
		if oldDef != nil && !p.canTakeOffItem(oldItem, oldDef) {
			p.sysMsg(server, "无法取下物品！！！")
			p.sendTakeOnFail(server, 4)
			return
		}
	}

	// 穿上首饰时清除"神秘未鉴定"标记（Delphi ObjBase.pas:17153-17155）。
	if isAccessoryStdMode(def.StdMode) && item.BtValue[8] != 0 {
		item.BtValue[8] = 0
	}

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

	// Delphi ClientTakeOffItems（ObjBase.pas:17228）：交易中禁止脱下。
	if p.Deal != nil {
		p.sendTakeOffFail(server)
		return
	}
	if slot < 0 || slot > 12 {
		p.sendTakeOffFail(server)
		return
	}
	if p.UseItems[slot] == nil {
		p.sendTakeOffFail(server)
		return
	}
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		p.sendTakeOffFail(server)
		return
	}

	item := p.UseItems[slot]
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil && !p.canTakeOffItem(item, def) {
			p.sysMsg(server, "无法取下物品！！！")
			p.sendTakeOffFail(server)
			return
		}
	}
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
		// Delphi: NODRUG 地图禁止使用药水（MapFlag.boNODRUG）
		if p.envir != nil && p.envir.Flag.NoDrug {
			p.sysMsg(server, "这张地图禁止使用药品")
			p.sendEatFail(server)
			return
		}
		if def.Shape == 1 {
			// 太阳水：即时回复（Delphi IncHealthSpell）
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
		} else if def.Shape == 2 {
			// 解锁药（Delphi ObjBase.pas:23344-23348）：解除 Reserved&2
			// 与首饰封印（btValue[7]）的禁脱状态。
			p.UserUnLockDurg = true
			used = true
		} else {
			// 金创药/魔法药：渐进回复（Delphi m_nIncHealth/m_nIncSpell）
			if def.AC > 0 {
				p.IncHealth += int(def.AC)
				used = true
			}
			if def.MAC > 0 {
				p.IncSpell += int(def.MAC)
				used = true
			}
		}
	case 1: // 食物（Delphi ObjBase.pas:23371-23379）：饥饿度 += DuraMax/10，上限 5000。
		p.HungerStatus += int(def.DuraMax) / 10
		if p.HungerStatus > 5000 {
			p.HungerStatus = 5000
		}
		p.sendMyStatus(server)
		used = true
	case 2: // 杂项：直接消耗无任何效果（Delphi ObjBase.pas:23380）。
		used = true
	case 3: // 特殊消耗品，按 Shape 分发。
		used = p.useSpecialItem(def, server)
	case 4: // 技能书（Delphi ReadBook, ObjBase.pas:23443）。
		used = p.readBook(def, server)
	case 31: // 打包物品：解包或触发 @StdModeFunc（ObjBase.pas:17394-17413）。
		used = p.usePackItem(def, bagIdx, server)
		if used {
			return // usePackItem 已处理消耗与回包
		}
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

// useSpecialItem 处理 StdMode=3 的特殊消耗品（Delphi EatUseItems，
// ObjBase.pas:23518-23575）。
func (p *PlayObject) useSpecialItem(def *ItemDef, server *netserver.TCPServer) bool {
	switch def.Shape {
	case 1: // 地牢逃脱卷：传送至安全区。
		return p.teleportToSafe(server)
	case 2: // 随机传送卷：当前地图随机位置。
		if p.envir != nil && p.envir.Flag.NoRandomMove {
			p.sysMsg(server, "这张地图禁止随机传送")
			return false
		}
		return p.teleportRandom(server)
	case 3: // 回城卷 PK 版（ObjBase.pas:23536-23545）：红名进红名监狱。
		if p.PKLevel() >= 2 {
			cfg := p.Engine.Config
			return p.teleportToMap(server, cfg.GetRedHomeMap(), cfg.GetRedHomeX(), cfg.GetRedHomeY())
		}
		return p.teleportToSafe(server)
	case 4: // 祝福油：Delphi WeaptonMakeLuck (ObjBase.pas:23621-23671)
		weapon := p.UseItems[protocol.UWeapon]
		if weapon == nil {
			p.sysMsg(server, "请先装备武器")
			return false
		}
		p.weaponMakeLuck(server, weapon)
		return true
	case 5: // 行会回城卷（ObjBase.pas:23550-23574）：需行会且本行会占领城堡。
		if p.GuildName == "" {
			p.sysMsg(server, "此处无法使用")
			return false
		}
		if !p.isCastleMember() || p.Engine.Castle == nil {
			p.sysMsg(server, "无效")
			return false
		}
		cc := p.Engine.Castle.Config
		return p.teleportToMap(server, cc.MapName, cc.PalaceX, cc.PalaceY)
	case 9: // 修理卷轴（Delphi RepairWeapon，ObjBase.pas:23673-23690）。
		weapon := p.UseItems[protocol.UWeapon]
		if weapon == nil {
			p.sysMsg(server, "请先装备武器")
			return false
		}
		if weapon.DuraMax <= weapon.Dura {
			return false
		}
		weapon.DuraMax -= (weapon.DuraMax - weapon.Dura) / 30
		add := int(weapon.DuraMax - weapon.Dura)
		if add > 5000 {
			add = 5000
		}
		if add <= 0 {
			return false
		}
		weapon.Dura += uint16(add)
		p.sendDuraChange(server, weapon)
		p.sysMsg(server, "武器修复成功...")
		return true
	case 10: // 特修卷轴（Delphi SuperRepairWeapon，ObjBase.pas:23692-23700）。
		weapon := p.UseItems[protocol.UWeapon]
		if weapon == nil {
			p.sysMsg(server, "请先装备武器")
			return false
		}
		weapon.Dura = weapon.DuraMax
		p.sendDuraChange(server, weapon)
		p.sysMsg(server, "武器修复成功...")
		return true
	case 12: // 临时 Buff（神水/精酿）。
		return p.applyBuff(def, server)
	}
	return false
}

// teleportToMap 传送至指定地图坐标（Delphi BaseObjectMove）。
func (p *PlayObject) teleportToMap(server *netserver.TCPServer, mapName string, x, y int) bool {
	if p.MapMgr == nil {
		return false
	}
	env := p.MapMgr.FindMap(mapName)
	if env == nil {
		return false
	}
	if env.Name == p.MapName {
		// 同地图内传送。
		if p.envir != nil {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.envir.broadcastRefMsg(p.BaseObject, RM_DISAPPEAR, p.ID, p.CurrX, p.CurrY, p.Dir)
		}
		p.CurrX, p.CurrY = x, y
		p.envir.AddObject(x, y, OS_MOVINGOBJECT, p)
		p.envir.broadcastRefMsg(p.BaseObject, RM_LOGON, p.ID, x, y, p.Dir)
	} else {
		p.EnterAnotherMap(server, env, x, y)
	}
	return true
}

// teleportToSafe 传送至安全区（回城卷/地牢逃脱卷）。
func (p *PlayObject) teleportToSafe(server *netserver.TCPServer) bool {
	safeMap, safeX, safeY := GetSafeZonePoint()
	return p.teleportToMap(server, safeMap, safeX, safeY)
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

// giveItemByName 按物品名创建实例并放入背包（Delphi
// CopyToUserItemFromName，UsrEngn.pas:1625-1627）。背包满返回 false。
func (p *PlayObject) giveItemByName(name string) bool {
	if p.ItemDB == nil || p.Engine == nil {
		return false
	}
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		return false
	}
	def := p.ItemDB.GetByName(name)
	if def == nil {
		return false
	}
	item := p.ItemDB.CreateUserItem(def.Idx)
	if item == nil {
		return false
	}
	item.MakeIndex = p.Engine.allocItemID()
	p.ItemList = append(p.ItemList, item)
	return true
}

// usePackItem 处理 StdMode=31 打包物品（Delphi ObjBase.pas:17394-17413）。
// AniCount=0：解包——背包空位校验（count+6-1 ≤ maxBag），消耗本物品，
// 按解包表（Shape→物品名）发 6 件（ObjBase.pas:17301-17336）。
// AniCount≠0：触发 NPC 脚本 @StdModeFunc<AniCount>（Delphi
// UseStdmodeFunItem，ObjBase.pas:17447-17455；Go 无 FunctionNPC，
// 改为扫描全部 NPC 脚本找同名 label，找不到则不消耗）。
// 成功时本函数自行消耗物品并回包，调用方不再走通用流程。
func (p *PlayObject) usePackItem(def *ItemDef, bagIdx int, server *netserver.TCPServer) bool {
	if def.AniCount == 0 {
		name := ""
		if p.ItemDB != nil {
			name = p.ItemDB.UnbindList[int(def.Shape)]
		}
		if name == "" {
			return false
		}
		maxBag := p.Engine.Config.GetMaxBagSlots()
		if len(p.ItemList)+6-1 > maxBag {
			p.sysMsg(server, "你不能携带更多的物品.")
			return false
		}
		p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
		for i := 0; i < 6; i++ {
			if !p.giveItemByName(name) {
				break
			}
		}
	} else {
		label := fmt.Sprintf("StdModeFunc%d", def.AniCount)
		var target *NpcObject
		var script *NpcScript
		for _, npc := range p.Engine.Npcs {
			if s := npc.GetScript(); s != nil {
				if _, ok := s.Labels[label]; ok {
					target, script = npc, s
					break
				}
			}
		}
		if target == nil {
			return false
		}
		p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
		script.Execute(label, p, target, server)
	}

	resp := protocol.MakeDefaultMsg(protocol.SMEatOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)
	log.Logf(log.LevelInfo, "Items", "%s opened pack item %s", p.Name, def.Name)
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
				// Delphi: BtValue[3]=祝福, BtValue[4]=诅咒 (ItmUnit.pas:569-570)
				p.Luck += int(item.BtValue[3])
				p.Luck -= int(item.BtValue[4])
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

	// Delphi: STATE_DEFENCEUP/STATE_MAGDEFENCEUP 状态加成 (ObjBase.pas:3418-3421)
	if p.StatusTimeArr[STATE_DEFENCEUP] > 0 {
		bonus := uint32(2 + int(p.WAbil.Level)/7)
		p.WAbil.AC += bonus << 16
	}
	if p.StatusTimeArr[STATE_MAGDEFENCEUP] > 0 {
		bonus := uint32(2 + int(p.WAbil.Level)/7)
		p.WAbil.MAC += bonus << 16
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
	// 性别位无条件折叠进外观字节：裸体女=1（Hum.wil 奇数块为女体）。
	// Delphi: nDress/nWeapon := 0 后在 if 外执行 Inc(…, m_btGender)（ObjBase.pas:20010,20021）
	p.DressLook = p.Gender
	p.WeaponLook = p.Gender
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

// checkItemsNeed 行会/城堡类装备需求复查（Delphi CheckItemsNeed，
// ObjBase.pas:25985+）：仅 Need 6/7/60/70 会在装备后失效。
func (p *PlayObject) checkItemsNeed(def *ItemDef) bool {
	switch def.Need {
	case 6:
		return p.GuildName != ""
	case 60:
		return p.GuildName != "" && p.isGuildMaster()
	case 7:
		return p.GuildName != "" && p.isCastleMember()
	case 70:
		return p.GuildName != "" && p.isCastleMember() && p.isGuildMaster()
	}
	return true
}

// checkAutoTakeOff 需求不满足的装备自动脱下（Delphi ObjBase.pas:6622-6666）：
// 脱入背包，背包满则落地。行会/城堡状态变化后调用。
func (p *PlayObject) checkAutoTakeOff(server *netserver.TCPServer) {
	if p.ItemDB == nil {
		return
	}
	changed := false
	for i := 0; i < 13; i++ {
		it := p.UseItems[i]
		if it == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(it.WIndex))
		if def == nil {
			p.UseItems[i] = nil
			changed = true
			continue
		}
		if p.checkItemsNeed(def) {
			continue
		}
		if len(p.ItemList) < p.Engine.Config.GetMaxBagSlots() {
			p.ItemList = append(p.ItemList, it)
		} else if p.envir != nil {
			p.dropItemToGround(it, server, time.Now().UnixMilli())
		} else {
			continue
		}
		p.UseItems[i] = nil
		changed = true
	}
	if changed {
		p.RecalcAbilitys()
		p.updateAppearance()
		p.SendUseItemsFull(server)
		p.SendBagItemsFull(server)
		p.sendWeightChanged(server)
	}
}

// isGuildMaster 判断玩家是否行会掌门（Go 行会职务为字符串，
// 兼容 "master" 与 "掌门人" 两种取值）。
func (p *PlayObject) isGuildMaster() bool {
	return p.GuildRank == "master" || p.GuildRank == "掌门人"
}

// isCastleMember 判断玩家是否占领城堡行会的成员
//（Delphi g_CastleManager.IsCastleMember，Castle.pas:762）。
func (p *PlayObject) isCastleMember() bool {
	return p.Engine != nil && p.Engine.Castle != nil &&
		p.Engine.Castle.OwnerGuild != "" && p.Engine.Castle.OwnerGuild == p.GuildName
}

// checkItemNeed 装备需求全分支（Delphi CheckTakeOnItems 的 Need case，
// ObjBase.pas:23001-23260）。NeedLevel 打包约定：LoWord=条件1、HiWord=条件2。
// 会员系（8/81/82）依赖的会员系统未实装，按需求不满足处理。
func (p *PlayObject) checkItemNeed(def *ItemDef) bool {
	nl := def.NeedLevel
	lo := int(nl & 0xFFFF)
	hi := int(nl >> 16)
	switch def.Need {
	case 0:
		return int(p.WAbil.Level) >= int(nl)
	case 1:
		return int(p.WAbil.DC>>16) >= int(nl)
	case 2:
		return int(p.WAbil.MC>>16) >= int(nl)
	case 3:
		return int(p.WAbil.SC>>16) >= int(nl)
	case 4: // 转生等级
		return p.ReNewLevel >= int(nl)
	case 5: // 声望（Delphi m_btCreditPoint）
		return p.CreditPoint >= int(nl)
	case 6: // 已加入行会
		return p.GuildName != ""
	case 7: // 沙城成员（行会占领城堡）
		return p.GuildName != "" && p.isCastleMember()
	case 8, 81, 82: // 会员系：会员系统未实装
		return false
	case 10:
		return int(p.Job) == lo && int(p.WAbil.Level) >= hi
	case 11:
		return int(p.Job) == lo && int(p.WAbil.DC>>16) >= hi
	case 12:
		return int(p.Job) == lo && int(p.WAbil.MC>>16) >= hi
	case 13:
		return int(p.Job) == lo && int(p.WAbil.SC>>16) >= hi
	case 40:
		return p.ReNewLevel >= lo && int(p.WAbil.Level) >= hi
	case 41:
		return p.ReNewLevel >= lo && int(p.WAbil.DC>>16) >= hi
	case 42:
		return p.ReNewLevel >= lo && int(p.WAbil.MC>>16) >= hi
	case 43:
		return p.ReNewLevel >= lo && int(p.WAbil.SC>>16) >= hi
	case 44:
		return p.ReNewLevel >= lo && p.CreditPoint >= hi
	case 60: // 行会掌门
		return p.GuildName != "" && p.isGuildMaster()
	case 70: // 沙城掌门 + 等级
		return p.GuildName != "" && p.isCastleMember() && p.isGuildMaster() &&
			int(p.WAbil.Level) >= int(nl)
	default:
		// GEEM2 库存在 Need=200（宝玉类）等 Delphi 无对应分支的取值；
		// 兼容现有内容：NeedLevel=0 视为无需求，否则按等级判定。
		if nl == 0 {
			return true
		}
		return int(p.WAbil.Level) >= int(nl)
	}
}

// isAccessoryStdMode 首饰类 StdMode 集合（封印标记 btValue[7] 生效范围，
// ObjBase.pas:17123-17131）。
func isAccessoryStdMode(stdMode byte) bool {
	switch stdMode {
	case 15, 19, 20, 21, 22, 23, 24, 26:
		return true
	}
	return false
}

// canTakeOffItem 四道禁脱校验（Delphi ObjBase.pas:17119-17151/17237-17263）：
// ① 首饰封印 btValue[7]≠0；② Reserved&2 禁脱（①②可被解锁药绕过）；
// ③ Reserved&4 永久禁脱；④ 禁脱列表。
func (p *PlayObject) canTakeOffItem(item *protocol.UserItem, def *ItemDef) bool {
	if isAccessoryStdMode(def.StdMode) && !p.UserUnLockDurg && item.BtValue[7] != 0 {
		return false
	}
	if !p.UserUnLockDurg && def.Reserved&2 != 0 {
		return false
	}
	if def.Reserved&4 != 0 {
		return false
	}
	if p.ItemDB != nil && p.ItemDB.InDisableTakeOffList(item.WIndex) {
		return false
	}
	return true
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

// sendMyStatus 发送饥饿状态（Delphi RefMyStatus，ObjBase.pas:6193-6196）。
func (p *PlayObject) sendMyStatus(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMyStatus, 0, uint16(p.HungerStatus), 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendEatFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMEatFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	// 客户端双击使用时物品已离手（Delphi g_EatingItem），失败时补发
	// 全量背包恢复显示（Delphi 客户端本地放回，Go 走全量刷新架构）。
	p.SendBagItemsFull(server)
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
	p.HasUnParalysis = false
	p.HasSuperman = false
	p.HasUnMagicShield = false
	p.HasUnRevival = false
	p.HasGuildMove = false
	p.HasNoDropItem = false
	p.HasNoDropUseItem = false

	if p.ItemDB == nil {
		return
	}
	// Delphi（ObjBase.pas:2960-3060/3134-3310）同时按 AniCount 与
	// Shape 两套编码识别特效。
	applyCode := func(code byte) {
		switch code {
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
		case 117, 170:
			p.HasAngry = true
		case 118:
			p.HasMagicShield = true
		case 119:
			p.HasMuscle = true
		case 121:
			p.HasProbe = true
		case 139:
			p.HasUnParalysis = true
		case 140:
			p.HasSuperman = true
		case 143:
			p.HasUnMagicShield = true
		case 144:
			p.HasUnRevival = true
		case 145:
			p.HasGuildMove = true
		case 171:
			p.HasNoDropItem = true
		case 172:
			p.HasNoDropUseItem = true
		}
	}
	for i := 0; i < 13; i++ {
		if p.UseItems[i] == nil {
			continue
		}
		def := p.ItemDB.GetByIdx(int(p.UseItems[i].WIndex))
		if def == nil {
			continue
		}
		applyCode(def.AniCount)
		applyCode(def.Shape)
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
	p.HongMoSuite = 0

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

	// 虹魔套(3件)：命中 +2，吸血 5%（Delphi m_nHongMoSuite）
	if nameCounts["虹魔"] >= 3 {
		p.HitPoint += 2
		p.HongMoSuite = 5
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

// questCheckItem 对应 Delphi QuestCheckItem（ObjBase.pas:24539-24567）：
// 按名字清点背包物品，返回数量与耐久最高的匹配实例，并把该实例
// 记入 CheckedItemMakeIndex 供 TAKECHECKITEM 收取（无命中则清除旧记录）。
func (p *PlayObject) questCheckItem(name string) (int, *protocol.UserItem) {
	p.CheckedItemMakeIndex = 0
	if p.ItemDB == nil {
		return 0, nil
	}
	def := p.ItemDB.GetByName(name)
	if def == nil {
		return 0, nil
	}
	count := 0
	var best *protocol.UserItem
	for _, item := range p.ItemList {
		if item == nil || int(item.WIndex) != def.Idx {
			continue
		}
		count++
		if best == nil || item.Dura > best.Dura {
			best = item
		}
	}
	if best != nil {
		p.CheckedItemMakeIndex = best.MakeIndex
	}
	return count, best
}

// questTakeCheckItem 收取 questCheckItem 记录的实例
//（Delphi QuestTakeCheckItem，ObjBase.pas:24588-24619）：先查背包再查装备。
func (p *PlayObject) questTakeCheckItem(server *netserver.TCPServer) {
	if p.CheckedItemMakeIndex == 0 {
		return
	}
	if idx := p.findBagItem(p.CheckedItemMakeIndex); idx >= 0 {
		p.ItemList = append(p.ItemList[:idx], p.ItemList[idx+1:]...)
		p.CheckedItemMakeIndex = 0
		p.RecalcAbilitys()
		p.SendBagItemsFull(server)
		p.sendWeightChanged(server)
		return
	}
	for i, item := range p.UseItems {
		if item != nil && item.MakeIndex == p.CheckedItemMakeIndex {
			p.UseItems[i] = nil
			p.CheckedItemMakeIndex = 0
			p.RecalcAbilitys()
			p.updateAppearance()
			p.SendUseItemsFull(server)
			p.sendWeightChanged(server)
			return
		}
	}
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

// weaponMakeLuck — Delphi WeaptonMakeLuck (ObjBase.pas:23621-23671)
// BtValue[3] = 祝福等级(0~7), BtValue[4] = 诅咒等级(0~10)
func (p *PlayObject) weaponMakeLuck(server *netserver.TCPServer, weapon *protocol.UserItem) {
	// 1/20 概率失败 → 变诅咒
	if rand.Intn(20) == 0 {
		p.weaponMakeUnlock(server, weapon)
		return
	}

	made := false
	luck := weapon.BtValue[3]
	curse := weapon.BtValue[4]

	// 计算 nRand = |DCMax - DC| / 5
	nRand := 1
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(weapon.WIndex)); def != nil {
			diff := int(def.DCMax) - int(def.DC)
			if diff < 0 {
				diff = -diff
			}
			nRand = diff / 5
			if nRand < 1 {
				nRand = 1
			}
		}
	}

	switch {
	case curse > 0:
		// 有诅咒 → 先消诅咒（100%成功）
		weapon.BtValue[4]--
		p.sysMsg(server, "武器充满了祝福...")
		made = true
	case luck < 1:
		// 祝福 < 1 → 100%成功
		weapon.BtValue[3]++
		made = true
	case luck < 3:
		// 祝福 < 3 → 1/(nRand + 6) 概率
		if rand.Intn(nRand+6) == 0 {
			weapon.BtValue[3]++
			made = true
		}
	case luck < 7:
		// 祝福 < 7 → 1/(nRand * 40) 概率
		if rand.Intn(nRand*40) == 0 {
			weapon.BtValue[3]++
			made = true
		}
	}

	p.RecalcAbilitys()
	p.SendAbility(server)
	if !made {
		p.sysMsg(server, "无效")
	} else {
		p.sendDuraChange(server, weapon)
	}
}

// weaponMakeUnlock — Delphi MakeWeaponUnlock (ObjBase.pas:2393-2414)
func (p *PlayObject) weaponMakeUnlock(server *netserver.TCPServer, weapon *protocol.UserItem) {
	if weapon.BtValue[3] > 0 {
		weapon.BtValue[3]--
		p.sysMsg(server, "武器被诅咒了...")
	} else if weapon.BtValue[4] < 10 {
		weapon.BtValue[4]++
		p.sysMsg(server, "武器被诅咒了...")
	}
	p.RecalcAbilitys()
	p.SendAbility(server)
	p.sendDuraChange(server, weapon)
}
