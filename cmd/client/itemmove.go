package main

import (
	"strconv"
	"strings"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// atoiClamped parses a gold-amount input (Delphi GetValidStrVal+Str_ToInt,
// FState.pas:1878-1879), clamped to [lo, hi].
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

// goldStr formats a gold amount with thousands separators (Delphi GetGoldStr,
// ClFunc.pas:98-115).
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

// Item drag system — port of Delphi g_boItemMoving/g_MovingItem
// (MShare.pas:365-368, FState.pas:613-621,1812-1886). The signed Index
// encodes where the held item came from.
const (
	// Index >= 0: held from bag slot Index.
	// Index -1..-13: held from equipment slot -(Index+1).
	moveIdxDealGold = -97 // gold dragged from the trade dialog
	moveIdxBagGold  = -98 // gold dragged from the bag gold button
	moveIdxSellSpot = -99 // item placed on the sell/repair/storage spot
)

// moveIdxDeal returns the moving-Index for a trade grid slot (−20..−29).
func moveIdxDeal(slot int) int { return -slot - 20 }

// moveDealSlot decodes a trade-grid moving-Index back to its slot.
func moveDealSlot(idx int) int { return -idx - 20 }

// moveEquipSlot decodes an equipment moving-Index back to its slot.
func moveEquipSlot(idx int) int { return -(idx + 1) }

type ItemMoveState struct {
	Moving   bool
	Index    int
	Item     BagItem
	FromBelt int // belt slot the item was lifted from, or -1

	// g_WaitingUseItem: equip pending server confirmation (FState.pas:3366).
	WaitUse  BagItem
	WaitSlot int
}

// Begin picks the item up (clearing it from the origin is the caller's job).
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

// Cancel restores the held item to its origin (FState.pas:1812-1839).
// Visual only — the server stays authoritative for bag contents.
func (m *ItemMoveState) Cancel(gs *GameState) {
	if !m.Moving {
		return
	}
	switch {
	case m.Index >= 0: // bag slot
		if m.Index < len(gs.BagItems) && gs.BagItems[m.Index] == nil {
			it := m.Item
			gs.BagItems[m.Index] = &it
		}
	case m.Index >= -13: // equipment slot
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
	if m.FromBelt >= 0 && m.FromBelt < len(gs.BeltItems) && gs.BeltItems[m.FromBelt] == nil {
		it := m.Item
		gs.BeltItems[m.FromBelt] = &it
	}
	// Trade grid origin: return to the same offer slot.
	if m.Index >= -29 && m.Index <= -20 {
		slot := moveDealSlot(m.Index)
		if slot >= 0 && slot < len(gs.DealItems) && gs.DealItems[slot] == nil {
			it := m.Item
			gs.DealItems[slot] = &it
		}
	}
	m.End()
}

// getTakeOnPosition maps an item StdMode to its primary equipment slot
// (ClFunc.pas:618-634). Dual-slot items (rings/bracelets) return the left
// slot; equipSlotClick picks left vs right via takeOnSlotMatches.
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

// takeOnSlotMatches reports whether slot is a valid equip target for
// stdMode, accepting either side for dual-slot items (FState:3300-3318).
func takeOnSlotMatches(stdMode byte, slot int) bool {
	switch stdMode {
	case 22, 23:
		return slot == protocol.URingL || slot == protocol.URingR
	case 24, 26:
		return slot == protocol.UArmRingL || slot == protocol.UArmRingR
	}
	return getTakeOnPosition(stdMode) == slot
}
