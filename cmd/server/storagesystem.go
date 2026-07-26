package main

import (
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const MaxStorageItems = 39

func (p *PlayObject) HandleStorageItem(msg SendMessage, server *netserver.TCPServer) {
	bagIdx := msg.Param1
	if bagIdx < 0 || bagIdx >= len(p.ItemList) {
		return
	}
	if len(p.StorageItems) >= MaxStorageItems {
		resp := protocol.MakeDefaultMsg(protocol.SMStorageFull, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)
	p.StorageItems = append(p.StorageItems, item)
	resp := protocol.MakeDefaultMsg(protocol.SMStorageOK, int32(bagIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) HandleTakeBackStorageItem(msg SendMessage, server *netserver.TCPServer) {
	storageIdx := msg.Param1
	if storageIdx < 0 || storageIdx >= len(p.StorageItems) {
		return
	}
	if len(p.ItemList) >= MaxBagItems {
		resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemFullBag, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	item := p.StorageItems[storageIdx]
	p.StorageItems = append(p.StorageItems[:storageIdx], p.StorageItems[storageIdx+1:]...)
	p.ItemList = append(p.ItemList, item)
	resp := protocol.MakeDefaultMsg(protocol.SMTakeBackStorageItemOK, int32(storageIdx), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
