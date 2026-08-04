package main

import (
	"strconv"
	"strings"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// atoiClamped 解析金币数量输入（对应 Delphi GetValidStrVal+Str_ToInt,
// FState.pas:1878-1879），结果限制在 [lo, hi] 范围内。
func atoiClamped(s string, lo, hi int) int {
	s = strings.ReplaceAll(s, " ", "")
	v, err := strconv.Atoi(s)
	if err != nil {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// goldStr 将金币数量格式化为千分位分隔（对应 Delphi GetGoldStr,
// ClFunc.pas:98-115）。
func goldStr(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + goldStr(-n)
	}
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	out := s[:first]
	for i := first; i < len(s); i += 3 {
		out += "," + s[i:i+3]
	}
	return out
}

// 物品拖拽系统——移植自 Delphi g_boItemMoving/g_MovingItem
// （MShare.pas:365-368, FState.pas:613-621,1812-1886）。有符号 Index
// 编码了手持物品的来源位置。
const (
	// Index >= 0: 从背包格 Index 拿起。
	// Index -1..-13: 从装备格 -(Index+1) 拿起。
	moveIdxDealGold = -97 // 从交易对话框拖出的金币
	moveIdxBagGold  = -98 // 从背包金币按钮拖出的金币
	moveIdxSellSpot = -99 // 放在售卖/修理/仓库格上的物品
)

// moveIdxDeal 返回交易格对应的移动索引（−20..−29）。
func moveIdxDeal(slot int) int { return -slot - 20 }

// moveDealSlot 将交易格移动索引解码回格号。
func moveDealSlot(idx int) int { return -idx - 20 }

// moveEquipSlot 将装备移动索引解码回格号。
func moveEquipSlot(idx int) int { return -(idx + 1) }

type ItemMoveState struct {
	Moving   bool
	Index    int
	Item     BagItem
	FromBelt int // 物品拿起的腰带格，无则 -1

	// g_WaitingUseItem: 等待服务端确认的穿戴操作（FState.pas:3366）。
	WaitUse  BagItem
	WaitSlot int
}

// Begin 拿起物品（从原位移除由调用方负责）。
func (m *ItemMoveState) Begin(index int, item *BagItem) {
	if item == nil {
		return
	}
	m.Moving = true
	m.Index = index
	m.Item = *item
	m.FromBelt = -1
}

func (m *ItemMoveState) End() {
	m.Moving = false
	m.Index = 0
	m.Item = BagItem{}
	m.FromBelt = -1
}

// Cancel 将手持物品放回原处（FState.pas:1812-1839）。
// 仅视觉恢复——背包内容仍以服务端为准。
func (m *ItemMoveState) Cancel(gs *GameState) {
	if !m.Moving {
		return
	}
	if m.FromBelt >= 0 && m.FromBelt < len(gs.BeltItems) {
		// 腰带来源：放回原腰带格；格被占用时放入背包首个空位
		// （Delphi CancelItemMoving idx 0..5 分支：原位空则还原，
		// 否则 AddItemBag，FState.pas:1829-1834）。
		if gs.BeltItems[m.FromBelt] == nil {
			it := m.Item
			gs.BeltItems[m.FromBelt] = &it
		} else {
			returnItemToBag(gs, m.Item)
		}
		m.End()
		return
	}
	switch {
	case m.Index >= 0: // 背包格
		if m.Index < len(gs.BagItems) && gs.BagItems[m.Index] == nil {
			it := m.Item
			gs.BagItems[m.Index] = &it
		}
	case m.Index >= -13: // 装备格
		slot := moveEquipSlot(m.Index)
		if slot >= 0 && slot < 13 && gs.UseItems[slot] == nil {
			gs.UseItems[slot] = &protocol.UserItem{
				MakeIndex: m.Item.MakeIndex,
				WIndex:    m.Item.Idx,
				Dura:      m.Item.Dura,
				DuraMax:   m.Item.DuraMax,
			}
		}
	}
	// 交易格来源：放回原交易格。
	if m.Index >= -29 && m.Index <= -20 {
		slot := moveDealSlot(m.Index)
		if slot >= 0 && slot < len(gs.DealItems) && gs.DealItems[slot] == nil {
			it := m.Item
			gs.DealItems[slot] = &it
		}
	}
	m.End()
}

// getTakeOnPosition 将物品 StdMode 映射到主装备格（ClFunc.pas:618-634）。
// 双格物品（戒指/手镯）返回左格；equipSlotClick 通过 takeOnSlotMatches
// 决定穿左还是右。
func getTakeOnPosition(stdMode byte) int {
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

// takeOnSlotMatches 判断 slot 是否为 stdMode 的合法穿戴目标，
// 双格物品左右均可（FState:3300-3318）。
func takeOnSlotMatches(stdMode byte, slot int) bool {
	switch stdMode {
	case 22, 23:
		return slot == protocol.URingL || slot == protocol.URingR
	case 24, 26:
		return slot == protocol.UArmRingL || slot == protocol.UArmRingR
	}
	return getTakeOnPosition(stdMode) == slot
}
