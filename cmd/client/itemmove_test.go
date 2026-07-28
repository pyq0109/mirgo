package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ClFunc.pas:618-634 是权威的 StdMode→slot 映射表；客户端
// 必须与服务端 getEquipSlot 保持一致。
func TestGetTakeOnPositionMapping(t *testing.T) {
	cases := []struct {
		stdMode byte
		want    int
	}{
		{5, protocol.UWeapon}, {6, protocol.UWeapon},
		{10, protocol.UDress}, {11, protocol.UDress},
		{15, protocol.UHelmet}, {16, protocol.UHelmet},
		{19, protocol.UNecklace}, {20, protocol.UNecklace}, {21, protocol.UNecklace},
		{22, protocol.URingL}, {23, protocol.URingL},
		{24, protocol.UArmRingL}, {26, protocol.UArmRingL},
		{28, protocol.URightHand}, {29, protocol.URightHand}, {30, protocol.URightHand},
		{25, protocol.UBujuk}, {51, protocol.UBujuk},
		{52, protocol.UBoots}, {62, protocol.UBoots},
		{53, protocol.UCharm}, {63, protocol.UCharm},
		{54, protocol.UBelt}, {64, protocol.UBelt},
		{0, -1}, {99, -1},
	}
	for _, c := range cases {
		if got := getTakeOnPosition(c.stdMode); got != c.want {
			t.Errorf("getTakeOnPosition(%d) = %d, want %d", c.stdMode, got, c.want)
		}
	}
}

func TestTakeOnSlotMatches(t *testing.T) {
	if !takeOnSlotMatches(22, protocol.URingL) || !takeOnSlotMatches(23, protocol.URingR) {
		t.Error("rings should fit either ring slot")
	}
	if !takeOnSlotMatches(24, protocol.UArmRingR) || !takeOnSlotMatches(26, protocol.UArmRingL) {
		t.Error("bracelets should fit either bracelet slot")
	}
	if takeOnSlotMatches(22, protocol.UArmRingL) {
		t.Error("ring must not fit bracelet slot")
	}
	if !takeOnSlotMatches(15, protocol.UHelmet) || takeOnSlotMatches(15, protocol.UNecklace) {
		t.Error("helmet fits only UHelmet")
	}
	if takeOnSlotMatches(99, protocol.UDress) {
		t.Error("unmappable StdMode fits no slot")
	}
}
