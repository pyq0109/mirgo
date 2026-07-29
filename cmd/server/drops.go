package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func (m *MonsterObject) DropLoot(envir *Environment, nextItemID *int32, server *netserver.TCPServer, itemDB *ItemDB) {
	m.DropLootWithTable(envir, nextItemID, server, nil, itemDB)
}

func (m *MonsterObject) DropLootWithTable(envir *Environment, nextItemID *int32, server *netserver.TCPServer, dt *DropTable, itemDB *ItemDB) {
	now := time.Now().UnixMilli()
	var dropped []*GroundItem

	if dt != nil {
		drops := dt.GetDrops(m.Name)
		for _, entry := range drops {
			if entry.Chance > 0 && rand.Intn(entry.Chance) == 0 {
				if entry.Gold > 0 {
					gold := entry.Gold
					if entry.Count > entry.Gold {
						gold = entry.Gold + rand.Intn(entry.Count-entry.Gold+1)
					}
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
				} else {
					gi := &GroundItem{
						ID:       *nextItemID,
						Name:     entry.ItemName,
						Looks:    0,
						X:        m.CurrX,
						Y:        m.CurrY,
						DropTick: now,
					}
					if itemDB != nil {
						if def := itemDB.GetByName(entry.ItemName); def != nil {
							gi.Looks = int(def.Looks)
							ui := itemDB.CreateUserItem(def.Idx)
							if ui != nil {
								ui.MakeIndex = *nextItemID
								gi.UserItem = ui
							}
						}
					}
					*nextItemID++
					envir.AddGroundItem(gi)
					dropped = append(dropped, gi)
				}
			}
		}
	} else {
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
			gi := &GroundItem{
				ID:       *nextItemID,
				Name:     "金创药(小量)",
				Looks:    0,
				X:        m.CurrX,
				Y:        m.CurrY,
				DropTick: now,
			}
			if itemDB != nil {
				if def := itemDB.GetByName("金创药(小量)"); def != nil {
					gi.Looks = int(def.Looks)
					ui := itemDB.CreateUserItem(def.Idx)
					if ui != nil {
						ui.MakeIndex = *nextItemID
						gi.UserItem = ui
					}
				}
			}
			*nextItemID++
			envir.AddGroundItem(gi)
			dropped = append(dropped, gi)
		}
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
