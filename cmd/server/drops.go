package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// getDropPosition 在 (cx,cy) 周围 scatterRange 范围内寻找可行走空位。
// Delphi: GetDropPosition (ObjBase.pas:1586)。
func getDropPosition(envir *Environment, cx, cy, scatterRange int) (int, int) {
	for i := 0; i < 10; i++ {
		dx := rand.Intn(scatterRange*2+1) - scatterRange
		dy := rand.Intn(scatterRange*2+1) - scatterRange
		nx, ny := cx+dx, cy+dy
		if envir.CanWalkEx(nx, ny, true) {
			return nx, ny
		}
	}
	return cx, cy
}

// randomUpgradeItem 随机生成装备附加属性（Delphi: RandomUpgradeItem, ItmUnit.pas:179）。
func randomUpgradeItem(ui *protocol.UserItem, def *ItemDef) {
	switch {
	case def.StdMode >= 5 && def.StdMode <= 6: // 武器
		if rand.Intn(15) == 0 {
			ui.BtValue[0]++ // DC
		}
		if rand.Intn(15) == 0 {
			ui.BtValue[1]++ // MC
		}
		if rand.Intn(15) == 0 {
			ui.BtValue[2]++ // SC
		}
		if rand.Intn(24) == 0 {
			ui.BtValue[5]++ // AC
		}
		if rand.Intn(20) == 0 {
			ui.BtValue[6]++ // MAC
		}
	case def.StdMode == 10 || def.StdMode == 11: // 衣服
		if rand.Intn(30) == 0 {
			ui.BtValue[0]++ // AC
		}
		if rand.Intn(30) == 0 {
			ui.BtValue[1]++ // MAC
		}
		if rand.Intn(40) == 0 {
			ui.BtValue[2]++ // DC
		}
		if rand.Intn(40) == 0 {
			ui.BtValue[3]++ // MC
		}
		if rand.Intn(40) == 0 {
			ui.BtValue[4]++ // SC
		}
	case def.StdMode >= 15 && def.StdMode <= 26: // 首饰
		if rand.Intn(15) == 0 {
			ui.BtValue[0]++ // DC
		}
		if rand.Intn(15) == 0 {
			ui.BtValue[1]++ // MC
		}
		if rand.Intn(15) == 0 {
			ui.BtValue[2]++ // SC
		}
	}
}

// createDropItem 创建掉落物品实例，含随机耐久和随机属性。
func createDropItem(itemDB *ItemDB, name string, nextItemID *int32) (int, *protocol.UserItem) {
	looks := 0
	var ui *protocol.UserItem
	if itemDB != nil {
		if def := itemDB.GetByName(name); def != nil {
			looks = int(def.Looks)
			ui = itemDB.CreateUserItem(def.Idx)
			if ui != nil {
				// 装备类随机耐久 20%-99%（Delphi: MonGetRandomItems, UsrEngn.pas:1576）
				if def.StdMode >= 5 && ui.DuraMax > 0 {
					ui.Dura = uint16(int(ui.DuraMax) * (20 + rand.Intn(80)) / 100)
				}
				// 1/10 概率随机附加属性（Delphi: nMonRandomAddValue=10）
				if def.StdMode >= 5 && rand.Intn(10) == 0 {
					randomUpgradeItem(ui, def)
				}
				ui.MakeIndex = *nextItemID
			}
		}
	}
	return looks, ui
}

func (m *MonsterObject) DropLoot(envir *Environment, nextItemID *int32, server *netserver.TCPServer, itemDB *ItemDB) {
	m.DropLootWithTable(envir, nextItemID, server, nil, itemDB)
}

func (m *MonsterObject) DropLootWithTable(envir *Environment, nextItemID *int32, server *netserver.TCPServer, dt *DropTable, itemDB *ItemDB) {
	now := time.Now().UnixMilli()
	ownerID := m.LastHiterID
	var dropped []*GroundItem

	addGold := func(totalGold int) {
		// 分堆：每堆最多 2000，最多 17 堆（Delphi: ScatterGolds, ObjBase.pas:20655）
		for totalGold > 0 && len(dropped) < 17 {
			pile := totalGold
			if pile > 2000 {
				pile = 2000
			}
			totalGold -= pile
			x, y := getDropPosition(envir, m.CurrX, m.CurrY, 3)
			item := &GroundItem{
				ID:        *nextItemID,
				Name:      "金币",
				Looks:     112,
				X:         x,
				Y:         y,
				DropTick:  now,
				Gold:      pile,
				OwnerID:   ownerID,
				OwnerTick: now,
			}
			*nextItemID++
			envir.AddGroundItem(item)
			dropped = append(dropped, item)
		}
	}

	addItem := func(name string) {
		x, y := getDropPosition(envir, m.CurrX, m.CurrY, 3)
		looks, ui := createDropItem(itemDB, name, nextItemID)
		gi := &GroundItem{
			ID:        *nextItemID,
			Name:      name,
			Looks:     looks,
			X:         x,
			Y:         y,
			DropTick:  now,
			UserItem:  ui,
			OwnerID:   ownerID,
			OwnerTick: now,
		}
		*nextItemID++
		envir.AddGroundItem(gi)
		dropped = append(dropped, gi)
	}

	if dt != nil {
		drops := dt.GetDrops(m.Name)
		for _, entry := range drops {
			if entry.Chance > 0 && rand.Intn(entry.Chance) == 0 {
				if entry.Gold > 0 {
					gold := entry.Gold
					if entry.Count > entry.Gold {
						gold = entry.Gold + rand.Intn(entry.Count-entry.Gold+1)
					}
					addGold(gold)
				} else {
					addItem(entry.ItemName)
				}
			}
		}
	} else {
		if rand.Intn(100) < 30 {
			addGold(10 + rand.Intn(41))
		}
		if rand.Intn(100) < 10 {
			addItem("金创药(小量)")
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
