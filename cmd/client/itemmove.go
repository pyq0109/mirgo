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

// getTakeOnPosition maps an item StdMode to its equipment slot
// (FState.pas:3269 GetTakeOnPosition; mirrors server getEquipSlot).
func getTakeOnPosition(stdMode byte) int {
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
