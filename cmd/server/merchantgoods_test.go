package main

import "testing"

// TestGoodsSubMenu 验证子菜单判定（Delphi ObjNpc.pas:1435-1437）：
// StdMode<=4 / 42 / 31 为消耗品（无子菜单），其余装备类有子菜单。
func TestGoodsSubMenu(t *testing.T) {
	tests := []struct {
		stdMode byte
		want    int
	}{
		{0, 0}, {2, 0}, {4, 0}, // 消耗品
		{31, 0}, {42, 0}, // 特殊消耗品
		{5, 1}, {6, 1}, {10, 1}, {25, 1}, {30, 1}, // 武器/衣服/药品等
	}
	for _, tt := range tests {
		if got := goodsSubMenu(&ItemDef{StdMode: tt.stdMode}); got != tt.want {
			t.Errorf("goodsSubMenu(StdMode=%d) = %d, want %d", tt.stdMode, got, tt.want)
		}
	}
}

// TestStdItemOf 验证 ItemDef → protocol.StdItem 的 Lo/Hi 打包
// （详细商品列表 TClientItem.S 用）。
func TestStdItemOf(t *testing.T) {
	def := &ItemDef{
		Idx:     42,
		Name:    "乌木剑",
		StdMode: 5,
		Shape:   1,
		Weight:  7,
		Looks:   100,
		DuraMax: 22000,
		AC:      1, ACMax: 3,
		DC: 2, DCMax: 5,
		Need: 1, NeedLevel: 10,
		Price:  3000,
		Source: -2,
	}
	s := StdItemOf(def)
	if s.GetName() != "乌木剑" {
		t.Errorf("Name = %q, want 乌木剑", s.GetName())
	}
	if s.AC != 3<<16|1 {
		t.Errorf("AC = %#x, want lo=1 hi=3", s.AC)
	}
	if s.DC != 5<<16|2 {
		t.Errorf("DC = %#x, want lo=2 hi=5", s.DC)
	}
	if s.Source != -2 || s.StdMode != 5 || s.Looks != 100 || s.DuraMax != 22000 {
		t.Errorf("fields mismatch: %+v", s)
	}
	if s.Need != 1 || s.NeedLevel != 10 || s.Price != 3000 {
		t.Errorf("need/price mismatch: %+v", s)
	}
}
