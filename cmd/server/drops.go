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
func randomUpgradeItem(ui *protocol.UserItem, def *ItemDef, chances []int) {
	c0, c1, c2, c3, c4 := chances[0], chances[1], chances[2], chances[3], chances[4]
	switch {
	case def.StdMode >= 5 && def.StdMode <= 6: // 武器
		if rand.Intn(c0) == 0 {
			ui.BtValue[0]++ // DC
		}
		if rand.Intn(c0) == 0 {
			ui.BtValue[1]++ // MC
		}
		if rand.Intn(c0) == 0 {
			ui.BtValue[2]++ // SC
		}
		if rand.Intn(c1) == 0 {
			ui.BtValue[5]++ // AC
		}
		if rand.Intn(c2) == 0 {
			ui.BtValue[6]++ // MAC
		}
	case def.StdMode == 10 || def.StdMode == 11: // 衣服
		if rand.Intn(c3) == 0 {
			ui.BtValue[0]++ // AC
		}
		if rand.Intn(c3) == 0 {
			ui.BtValue[1]++ // MAC
		}
		if rand.Intn(c4) == 0 {
			ui.BtValue[2]++ // DC
		}
		if rand.Intn(c4) == 0 {
			ui.BtValue[3]++ // MC
		}
		if rand.Intn(c4) == 0 {
			ui.BtValue[4]++ // SC
		}
	case def.StdMode >= 15 && def.StdMode <= 26: // 首饰
		if rand.Intn(c0) == 0 {
			ui.BtValue[0]++ // DC
		}
		if rand.Intn(c0) == 0 {
			ui.BtValue[1]++ // MC
		}
		if rand.Intn(c0) == 0 {
			ui.BtValue[2]++ // SC
		}
	}
}

// createDropItem 创建掉落物品实例，含随机耐久和随机属性。
func createDropItem(itemDB *ItemDB, name string, nextItemID *int32, cfg *ServerConfig) (int, *protocol.UserItem) {
	looks := 0
	var ui *protocol.UserItem
	if itemDB != nil {
		if def := itemDB.GetByName(name); def != nil {
			looks = int(def.Looks)
			// 矿石随机外观（Delphi DropItemDown，ObjBase.pas:1608-1611；
			// 源码 StdMode=45，GEEM2 库矿石为 43，两者均适用）。
			if (def.StdMode == 43 || def.StdMode == 45) && def.Shape > 0 {
				looks += rand.Intn(int(def.Shape))
			}
			ui = itemDB.CreateUserItem(def.Idx)
			if ui != nil {
				// 装备类随机耐久（Delphi: MonGetRandomItems, UsrEngn.pas:1576）
				if def.StdMode >= 5 && ui.DuraMax > 0 {
					ui.Dura = uint16(int(ui.DuraMax) * (cfg.GetEquipDuraMin() + rand.Intn(cfg.GetEquipDuraRand())) / 100)
				}
				// 肉落地扣 2000 耐久（ObjBase.pas:1597-1603）。
				if def.StdMode == 40 {
					if int(ui.Dura) > 2000 {
						ui.Dura -= 2000
					} else {
						ui.Dura = 0
					}
				}
				// 随机附加属性（Delphi: nMonRandomAddValue）
				if def.StdMode >= 5 && rand.Intn(cfg.GetAddValueChance()) == 0 {
					randomUpgradeItem(ui, def, cfg.GetEquipUpgChances())
				}
				ui.MakeIndex = *nextItemID
			}
		}
	}
	return looks, ui
}

func (m *MonsterObject) DropLoot(envir *Environment, nextItemID *int32, server *netserver.TCPServer, itemDB *ItemDB, cfg *ServerConfig) {
	m.DropLootWithTable(envir, nextItemID, server, nil, itemDB, cfg)
}

func (m *MonsterObject) DropLootWithTable(envir *Environment, nextItemID *int32, server *netserver.TCPServer, dt *DropTable, itemDB *ItemDB, cfg *ServerConfig) {
	now := time.Now().UnixMilli()
	ownerID := m.LastHiterID
	var dropped []*GroundItem

	addGold := func(totalGold int) {
		// 分堆（Delphi: ScatterGolds, ObjBase.pas:20655）
		for totalGold > 0 && len(dropped) < cfg.GetMaxGoldPiles() {
			pile := totalGold
			if pile > cfg.GetMaxGoldPerPile() {
				pile = cfg.GetMaxGoldPerPile()
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
			// 可能与已有金堆合并（返回已有堆）；合并/拒绝时跳过广播重复堆。
			if placed := envir.AddGroundItem(item); placed == item {
				dropped = append(dropped, item)
			}
		}
	}

	addItem := func(name string) {
		x, y := getDropPosition(envir, m.CurrX, m.CurrY, 3)
		looks, ui := createDropItem(itemDB, name, nextItemID, cfg)
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
		// 每格满 5 件时拒绝落地（Delphi 同样丢弃该物品）。
		if envir.AddGroundItem(gi) != nil {
			dropped = append(dropped, gi)
		}
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
		if rand.Intn(100) < cfg.GetFallbackGoldRate() {
			addGold(10 + rand.Intn(41))
		}
		if rand.Intn(100) < cfg.GetFallbackItemRate() {
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
