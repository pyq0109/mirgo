package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func (m *MonsterObject) DropLoot(envir *Environment, nextItemID *int32, server *netserver.TCPServer) {
	now := time.Now().UnixMilli()
	var dropped []*GroundItem

	if rand.Intn(100) < 30 {
		gold := 10 + rand.Intn(41)
		item := &GroundItem{
			ID:       *nextItemID,
			Name:     "金币",
			Looks:    112,
			X:        m.CurrX,
			Y:        m.CurrY,
			DropTick: now,
			Gold:     gold,
		}
		*nextItemID++
		envir.AddGroundItem(item)
		dropped = append(dropped, item)
	}

	if rand.Intn(100) < 10 {
		item := &GroundItem{
			ID:       *nextItemID,
			Name:     "金创药(小)",
			Looks:    0,
			X:        m.CurrX,
			Y:        m.CurrY,
			DropTick: now,
		}
		*nextItemID++
		envir.AddGroundItem(item)
		dropped = append(dropped, item)
	}

	for _, item := range dropped {
		resp := protocol.MakeDefaultMsg(protocol.SMItemShow, item.ID, uint16(item.X), uint16(item.Y), uint16(item.Looks))
		objs := envir.GetRangeObjects(item.X, item.Y, viewRange)
		for _, obj := range objs {
			p, ok := obj.(*PlayObject)
			if !ok || p.Ghost {
				continue
			}
			server.Send(p.Session.ID, resp, protocol.EncodeString(item.Name))
		}
	}
}
