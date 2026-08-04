package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/engine"
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

// 修复 2.6 配套：腰带来源的手持物品取消时优先回原腰带格；
// 格被占用（交换后）时放入背包首个空位（Delphi CancelItemMoving
// idx 0..5 分支，FState.pas:1829-1834）。
func TestCancelBeltOrigin(t *testing.T) {
	item := &BagItem{MakeIndex: 100}

	// 情况 1：原腰带格为空 → 还原到腰带格，且不重复放回背包。
	gs := NewGameState()
	var m ItemMoveState
	m.Begin(3, item)
	m.FromBelt = 2
	m.Cancel(gs)
	if gs.BeltItems[2] == nil || gs.BeltItems[2].MakeIndex != 100 {
		t.Fatal("cancel should restore belt item to its belt slot")
	}
	if gs.BagItems[3] != nil {
		t.Fatal("belt-origin cancel must not also restore to the bag slot")
	}
	if m.Moving {
		t.Fatal("cancel must end the move state")
	}

	// 情况 2：原腰带格已被占用（交换场景）→ AddItemBag 首个空位。
	gs2 := NewGameState()
	gs2.BeltItems[2] = &BagItem{MakeIndex: 200}
	var m2 ItemMoveState
	m2.Begin(3, item)
	m2.FromBelt = 2
	m2.Cancel(gs2)
	if gs2.BeltItems[2] == nil || gs2.BeltItems[2].MakeIndex != 200 {
		t.Fatal("occupied belt slot must keep its item")
	}
	if gs2.BagItems[0] == nil || gs2.BagItems[0].MakeIndex != 100 {
		t.Fatal("cancel with occupied belt slot should AddItemBag to first empty slot")
	}
}

// returnItemToBag：放入首个空位（Delphi AddItemBag）。
func TestReturnItemToBag(t *testing.T) {
	gs := NewGameState()
	gs.BagItems[0] = &BagItem{MakeIndex: 1}
	returnItemToBag(gs, BagItem{MakeIndex: 2})
	if gs.BagItems[1] == nil || gs.BagItems[1].MakeIndex != 2 {
		t.Fatal("returnItemToBag should use the first empty slot")
	}
}

// 修复 1.2：技能书（StdMode=4, Shape<100）双击使用必须先弹学习确认
//（Delphi EatItem -1 分支, ClMain.pas:1981-1994）；药品直接使用。
func TestUseHeldItemSkillBookConfirmation(t *testing.T) {
	s := NewPlayScene(nil, &engine.ResourceManager{}, "", nil)
	var used []int32
	s.sendUseItem = func(mi int32) { used = append(used, mi) }

	book := BagItem{MakeIndex: 42, Def: &ClientItemDef{Name: "火球术", StdMode: 4, Shape: 1}}
	s.useHeldItem(book)
	if len(used) != 0 {
		t.Fatal("skill book must not be used before confirmation")
	}
	modal := s.ui.Modal
	if modal == nil || !modal.Visible {
		t.Fatal("confirmation modal should be open")
	}
	// 按钮集 [MrYes,MrNo] 按 Cancel,No,Yes,Ok 顺序排列 → 子控件序 [No, Yes]。
	if len(modal.Children) != 2 {
		t.Fatalf("expected 2 dialog buttons, got %d", len(modal.Children))
	}
	modal.Children[1].OnClick(modal.Children[1], 0, 0) // MrYes
	if len(used) != 1 || used[0] != 42 {
		t.Fatalf("confirming should use the book once, got %v", used)
	}

	// 拒绝 → 不学习, 物品放回背包首个空位 (ClMain:1984-1987)。
	book2 := BagItem{MakeIndex: 43, Def: &ClientItemDef{Name: "火球术", StdMode: 4, Shape: 1}}
	s.useHeldItem(book2)
	modal = s.ui.Modal
	if modal == nil || !modal.Visible {
		t.Fatal("second confirmation modal should be open")
	}
	modal.Children[0].OnClick(modal.Children[0], 0, 0) // MrNo
	if len(used) != 1 {
		t.Fatal("declining must not use the book")
	}
	if s.State.BagItems[0] == nil || s.State.BagItems[0].MakeIndex != 43 {
		t.Fatal("declined book should return to the bag")
	}

	// 药品直接使用, 无确认。
	potion := BagItem{MakeIndex: 44, Def: &ClientItemDef{Name: "金创药", StdMode: 0}}
	s.useHeldItem(potion)
	if len(used) != 2 || used[1] != 44 {
		t.Fatalf("potion should be used immediately without dialog, got %v", used)
	}
	if s.ui.Modal != nil && s.ui.Modal.Visible {
		t.Fatal("potion use must not open a modal")
	}
}
