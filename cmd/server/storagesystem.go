package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)



// sendStorageMenu 打开仓库界面（Delphi BoStorageMenu）：先发送模式消息，
// 再发送完整物品列表。
func (p *PlayObject) sendStorageMenu(server *netserver.TCPServer) {
	mode := protocol.MakeDefaultMsg(protocol.SMSendUserStorageItem, 0, 0, 0, 0)
	server.Send(p.Session.ID, mode, "")
	p.sendStorageList(server)
}

// sendStorageList 发送仓库内容（每条 10 字节：WIndex, Dura,
// DuraMax, MakeIndex — 客户端据此构建列表行）。
func (p *PlayObject) sendStorageList(server *netserver.TCPServer) {
	buf := make([]byte, 2, 2+len(p.StorageItems)*10)
	binary.LittleEndian.PutUint16(buf, uint16(len(p.StorageItems)))
	for _, item := range p.StorageItems {
		if item == nil {
			continue
		}
		entry := make([]byte, 10)
		binary.LittleEndian.PutUint16(entry[0:2], item.WIndex)
		binary.LittleEndian.PutUint16(entry[2:4], item.Dura)
		binary.LittleEndian.PutUint16(entry[4:6], item.DuraMax)
		binary.LittleEndian.PutUint32(entry[6:10], uint32(item.MakeIndex))
		buf = append(buf, entry...)
	}
	resp := protocol.MakeDefaultMsg(protocol.SMSaveItemList, int32(len(p.StorageItems)), 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

// nearStorageNpc 仓库 NPC 距离校验（Delphi ObjBase.pas:24714-24718：
// 同地图且 |Δx|<15 且 |Δy|<15）。
func (p *PlayObject) nearStorageNpc() bool {
	npc := p.CurrentNpc
	if npc == nil {
		return false
	}
	if npc.MapName != p.MapName {
		return false
	}
	return absInt(npc.CurrX-p.CurrX) < 15 && absInt(npc.CurrY-p.CurrY) < 15
}

func (p *PlayObject) HandleStorageItem(msg SendMessage, server *netserver.TCPServer) {
	if !p.nearStorageNpc() {
		return
	}
	// Param1 = MakeIndex（实例ID；客户端布局由客户端维护）。
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		return
	}
	if len(p.StorageItems) >= p.Engine.Config.GetMaxStorageSlots() {
		resp := protocol.MakeDefaultMsg(protocol.SMStorageFull, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.StorageItems = append(p.StorageItems, item)
	resp := protocol.MakeDefaultMsg(protocol.SMStorageOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	p.sendStorageList(server)
}

func (p *PlayObject) HandleTakeBackStorageItem(msg SendMessage, server *netserver.TCPServer) {
	if !p.nearStorageNpc() {
		return
	}
	// 密码锁定期间拒绝取回（Delphi m_boCanGetBackItem，ObjBase.pas:24770-24778）。
	if p.StoragePwdLocked {
		p.sysMsg(server, "仓库已上锁，请先输入 @unlockstorage 密码 解锁")
		resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	// Param1 = 仓库物品的 MakeIndex（Delphi 传递列表条目的 MakeIndex，
	// 客户端将其存储为该行的 "price" 字段）。
	makeIndex := int32(msg.Param1)
	storageIdx := -1
	for i, item := range p.StorageItems {
		if item != nil && item.MakeIndex == makeIndex {
			storageIdx = i
			break
		}
	}
	if storageIdx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemFullBag, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	// 负重检查（Delphi ObjBase.pas:24801 IsAddWeightAvailable）。
	item := p.StorageItems[storageIdx]
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			if int(p.WAbil.Weight)+int(def.Weight) > int(p.WAbil.MaxWeight) && p.WAbil.MaxWeight > 0 {
				p.sysMsg(server, "物品太重，无法携带更多")
				resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemFail, 0, 0, 0, 0)
				server.Send(p.Session.ID, resp, "")
				return
			}
		}
	}
	p.StorageItems = append(p.StorageItems[:storageIdx], p.StorageItems[storageIdx+1:]...)
	p.ItemList = append(p.ItemList, item)
	resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemOK, makeIndex, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	p.sendStorageList(server)
}

// setStoragePassword 设置仓库密码（Delphi ObjBase.pas:7229-7252：
// 4-7 位；设置后即上锁，需解锁才能取回）。
func (p *PlayObject) setStoragePassword(server *netserver.TCPServer, pwd string) {
	if p.StoragePassword != "" {
		p.sysMsg(server, "已设置过密码，请用 @chgstoragepwd 旧密码 新密码 修改")
		return
	}
	if len(pwd) < 4 || len(pwd) > 7 {
		p.sysMsg(server, "仓库密码长度必须为 4-7 位")
		return
	}
	p.StoragePassword = pwd
	p.StoragePwdLocked = true
	p.storagePwdFail = 0
	p.sysMsg(server, "仓库密码设置成功，仓库已上锁")
}

// changeStoragePassword 修改仓库密码（先校验旧密码）。
func (p *PlayObject) changeStoragePassword(server *netserver.TCPServer, oldPwd, newPwd string) {
	if p.StoragePassword == "" {
		p.sysMsg(server, "尚未设置密码，请用 @setstoragepwd 密码 设置")
		return
	}
	if oldPwd != p.StoragePassword {
		p.sysMsg(server, "旧密码错误")
		return
	}
	if len(newPwd) < 4 || len(newPwd) > 7 {
		p.sysMsg(server, "仓库密码长度必须为 4-7 位")
		return
	}
	p.StoragePassword = newPwd
	p.sysMsg(server, "仓库密码修改成功")
}

// unlockStorage 解锁仓库（Delphi ObjBase.pas:7261-7300：错 >3 次锁定提示）。
func (p *PlayObject) unlockStorage(server *netserver.TCPServer, pwd string) {
	if p.StoragePassword == "" {
		p.sysMsg(server, "尚未设置仓库密码")
		return
	}
	if pwd == p.StoragePassword {
		p.StoragePwdLocked = false
		p.storagePwdFail = 0
		p.sysMsg(server, "仓库已解锁")
		return
	}
	p.storagePwdFail++
	if p.storagePwdFail > 3 {
		p.StoragePwdLocked = true
		p.sysMsg(server, "密码错误次数过多，仓库已锁定")
		return
	}
	p.sysMsg(server, "密码错误")
}
