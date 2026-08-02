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

func (p *PlayObject) HandleStorageItem(msg SendMessage, server *netserver.TCPServer) {
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
	item := p.StorageItems[storageIdx]
	p.StorageItems = append(p.StorageItems[:storageIdx], p.StorageItems[storageIdx+1:]...)
	p.ItemList = append(p.ItemList, item)
	resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemOK, makeIndex, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.SendBagItemsFull(server)
	p.sendStorageList(server)
}
