package main

import (
	"encoding/binary"
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
	userItem := &protocol.UserItem{
		MakeIndex: int32(itemIdx),
		WIndex:    uint16(itemIdx),
		Dura:      uint16(def.DuraMax),
		DuraMax:   uint16(def.DuraMax),
	}
	p.ItemList = append(p.ItemList, userItem)
	return true
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
	bagIdx := msg.Param1
	slot := msg.Param2

	if slot < 0 || slot > 12 {
		p.sendTakeOnFail(server, 0)
		return
	}
	if bagIdx < 0 || bagIdx >= len(p.ItemList) {
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

	validSlot := getEquipSlot(def.StdMode)
	if validSlot != slot {
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

	name := "?"
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
		}
	}
	log.Logf(log.LevelInfo, "Items", "%s unequipped %s from slot %d", p.Name, name, slot)
}

func (p *PlayObject) HandleEatItem(msg SendMessage, server *netserver.TCPServer) {
	bagIdx := msg.Param1

	if bagIdx < 0 || bagIdx >= len(p.ItemList) {
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
	p.SendBagItemsFull(server)
	p.sendHealthSpell(server)

	log.Logf(log.LevelInfo, "Items", "%s used %s", p.Name, def.Name)
}

func (p *PlayObject) RecalcAbilitys() {
	level := int(p.WAbil.Level)
	p.WAbil.MaxHP = uint16(50 + level*15)
	p.WAbil.MaxMP = uint16(20 + level*10)
	p.WAbil.AC = uint32(level / 2)
	p.WAbil.MAC = uint32(level / 3)
	p.WAbil.DC = uint32(level/2) | uint32(level)<<16
	p.WAbil.MC = uint32(level/3) | uint32(level/2)<<16
	p.WAbil.SC = uint32(level/3) | uint32(level/2)<<16

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

func getEquipSlot(stdMode byte) int {
	switch {
	case stdMode >= 10 && stdMode <= 12:
		return protocol.UDress
	case stdMode >= 5 && stdMode <= 6:
		return protocol.UWeapon
	case stdMode >= 15 && stdMode <= 17:
		return protocol.UNecklace
	case stdMode >= 20 && stdMode <= 22:
		return protocol.UHelmet
	case stdMode >= 24 && stdMode <= 26:
		return protocol.UArmRingL
	case stdMode >= 28 && stdMode <= 30:
		return protocol.URingL
	default:
		return -1
	}
}

func (p *PlayObject) sendTakeOnFail(server *netserver.TCPServer, code int) {
	resp := protocol.MakeDefaultMsg(protocol.SMTakeOnFail, int32(code), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendTakeOffFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMTakeOffFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
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
