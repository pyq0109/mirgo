package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ClFunc.pas:618-634 是 StdMode→装备槽 的权威映射表。
func TestGetEquipSlotMapping(t *testing.T) {
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
		if got := getEquipSlot(c.stdMode); got != c.want {
			t.Errorf("getEquipSlot(%d) = %d, want %d", c.stdMode, got, c.want)
		}
	}
}

func TestValidEquipSlot(t *testing.T) {
	// 双槽物品可装备任一侧 (FState:3300-3318)。
	if !validEquipSlot(22, protocol.URingL) || !validEquipSlot(23, protocol.URingR) {
		t.Error("rings should fit either ring slot")
	}
	if !validEquipSlot(24, protocol.UArmRingL) || !validEquipSlot(26, protocol.UArmRingR) {
		t.Error("bracelets should fit either bracelet slot")
	}
	if validEquipSlot(22, protocol.UArmRingL) {
		t.Error("ring must not fit bracelet slot")
	}
	// 单槽物品只能装备主槽位。
	if !validEquipSlot(15, protocol.UHelmet) || validEquipSlot(15, protocol.UNecklace) {
		t.Error("helmet fits only UHelmet")
	}
	if !validEquipSlot(52, protocol.UBoots) || validEquipSlot(52, protocol.UBelt) {
		t.Error("boots fit only UBoots")
	}
	if validEquipSlot(0, protocol.UDress) {
		t.Error("unmappable StdMode fits no slot")
	}
}
