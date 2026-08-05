package main

import (
	"strings"
	"testing"
)

// TestDelphiRound Delphi Round 银行家舍入（半取偶）。
func TestDelphiRound(t *testing.T) {
	cases := []struct{ v, scale, want int }{
		{2500, 1000, 2}, // 2.5 → 2（半取偶）
		{3500, 1000, 4}, // 3.5 → 4（半取偶）
		{2499, 1000, 2},
		{2501, 1000, 3},
		{100, 100, 1},
		{150, 100, 2}, // 1.5 → 2
		{0, 1000, 0},
	}
	for _, c := range cases {
		if got := delphiRound(c.v, c.scale); got != c.want {
			t.Errorf("delphiRound(%d,%d) = %d, want %d", c.v, c.scale, got, c.want)
		}
	}
}

func newTooltipState() *GameState {
	gs := NewGameState()
	gs.Level = 30
	gs.Job = 0
	gs.DC = 20 << 16
	gs.MC = 10 << 16
	gs.SC = 5 << 16
	return gs
}

// TestTooltipWeapon 武器分支：攻击范围、持久、需求行与红字判定。
func TestTooltipWeapon(t *testing.T) {
	gs := newTooltipState()
	item := &BagItem{
		Idx: 7, Dura: 9000, DuraMax: 10000,
		Def: &ClientItemDef{Name: "铁剑", StdMode: 5, Weight: 10,
			DC: 1, DCMax: 3, Need: 0, NeedLevel: 20},
	}
	text, useable := GetMouseItemInfo(gs, item)
	if !useable {
		t.Error("level 30 >= need 20 should be useable")
	}
	for _, want := range []string{"铁剑", "重量:10", "持久力:9/10", "攻击:1-3", "需要等级: 20"} {
		if !contains(text, want) {
			t.Errorf("tooltip %q missing %q", text, want)
		}
	}

	// 等级不足 → 红字。
	item.Def.NeedLevel = 40
	_, useable = GetMouseItemInfo(gs, item)
	if useable {
		t.Error("level 30 < need 40 should be unusable")
	}

	// Reserved&1 → (*) 前缀。
	item.Def.NeedLevel = 20
	item.Def.Reserved = 1
	text, _ = GetMouseItemInfo(gs, item)
	if !contains(text, "(*)铁剑") {
		t.Errorf("tooltip %q missing (*) prefix", text)
	}
}

// TestTooltipPotion 药水分支按 Shape 生成恢复文案。
func TestTooltipPotion(t *testing.T) {
	gs := newTooltipState()
	item := &BagItem{
		Idx: 1, Dura: 0, DuraMax: 0,
		Def: &ClientItemDef{Name: "金创药", StdMode: 0, Shape: 0, Weight: 1, AC: 50},
	}
	text, useable := GetMouseItemInfo(gs, item)
	if !useable {
		t.Error("potion should always be useable")
	}
	if !contains(text, "恢复 50生命值") {
		t.Errorf("tooltip %q missing HP restore text", text)
	}

	// Shape 1 立即恢复。
	item.Def.Shape = 1
	text, _ = GetMouseItemInfo(gs, item)
	if !contains(text, "立即恢复 50生命值") {
		t.Errorf("tooltip %q missing instant restore text", text)
	}
}

// TestTooltipSkillBook 技能书分支：职业+等级判定。
func TestTooltipSkillBook(t *testing.T) {
	gs := newTooltipState() // Job=0 战士
	book := &BagItem{
		Idx: 2, Def: &ClientItemDef{Name: "攻杀剑术", StdMode: 4, Shape: 0, Weight: 1, NeedLevel: 20},
	}
	text, useable := GetMouseItemInfo(gs, book)
	if !useable || !contains(text, "武士秘籍") || !contains(text, "需要等级: 20") {
		t.Errorf("warrior book tooltip = %q useable=%v, want 武士秘籍 + useable", text, useable)
	}
	book.Def.Shape = 1 // 法师秘籍
	_, useable = GetMouseItemInfo(gs, book)
	if useable {
		t.Error("wizard book should be unusable for warrior")
	}
}

// TestTooltipAccessory 首饰族分型（23 中毒戒指、62 鞋、63 宝石）。
func TestTooltipAccessory(t *testing.T) {
	gs := newTooltipState()
	ring := &BagItem{
		Idx: 3, Dura: 5000, DuraMax: 5000,
		Def: &ClientItemDef{Name: "降妖除魔戒指", StdMode: 23, Weight: 1, ACMax: 3, Need: 0, NeedLevel: 10},
	}
	text, _ := GetMouseItemInfo(gs, ring)
	if !contains(text, "毒物躲避:+30%") || !contains(text, "持久:5/5") {
		t.Errorf("ring tooltip = %q, missing poison dodge or dura", text)
	}

	boots := &BagItem{
		Idx: 4, Dura: 5000, DuraMax: 5000,
		Def: &ClientItemDef{Name: "战神之靴", StdMode: 62, Weight: 2, ACMax: 5, MACMax: 10, Need: 0, NeedLevel: 10},
	}
	text, _ = GetMouseItemInfo(gs, boots)
	if !contains(text, "手执负重:+5") || !contains(text, "装备负重:+10") {
		t.Errorf("boots tooltip = %q, missing weight lines", text)
	}
}

// TestTooltipSpecial 毒药数量、矿石纯度、肉品质。
func TestTooltipSpecial(t *testing.T) {
	gs := newTooltipState()
	poison := &BagItem{
		Idx: 5, Dura: 300, DuraMax: 1000,
		Def: &ClientItemDef{Name: "毒药", StdMode: 25, Weight: 1},
	}
	text, _ := GetMouseItemInfo(gs, poison)
	if !contains(text, "数量:3/10") {
		t.Errorf("poison tooltip = %q, want 数量:3/10", text)
	}

	ore := &BagItem{
		Idx: 6, Dura: 8500, DuraMax: 10000,
		Def: &ClientItemDef{Name: "铁矿", StdMode: 43, Weight: 5},
	}
	text, _ = GetMouseItemInfo(gs, ore)
	if !contains(text, "纯度:8") { // Round(8500/1000)=8（银行家舍入 8.5→8）
		t.Errorf("ore tooltip = %q, want 纯度:8", text)
	}

	meat := &BagItem{
		Idx: 8, Dura: 4000, DuraMax: 10000,
		Def: &ClientItemDef{Name: "牛肉", StdMode: 40, Weight: 2},
	}
	text, _ = GetMouseItemInfo(gs, meat)
	if !contains(text, "品质:4/10") {
		t.Errorf("meat tooltip = %q, want 品质:4/10", text)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
