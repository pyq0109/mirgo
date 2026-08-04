package main

import (
	"encoding/binary"
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// 这些测试固定与 Go 客户端共享的闭环消息体布局
// (GameState.ParseAbility / ParseItemDefs / ParseMagics)。
// 两端均按同一文档化布局进行验证。

func TestEncodeAbilityBodyLayout(t *testing.T) {
	p := NewPlayObject(nil, "Tester", 1)
	p.WAbil.Level = 42
	p.WAbil.AC = 0x00030001 // lo=1 hi=3
	p.WAbil.MAC = 2
	p.WAbil.DC = 0x000A0005
	p.WAbil.MC = 6
	p.WAbil.SC = 7
	p.WAbil.HP = 321
	p.WAbil.MaxHP = 654
	p.WAbil.MP = 111
	p.WAbil.MaxMP = 222
	p.WAbil.Exp = 12345
	p.WAbil.Weight = 30
	p.WAbil.MaxWeight = 500
	p.WAbil.WearWeight = 12
	p.WAbil.MaxWearWeight = 100
	p.WAbil.HandWeight = 8
	p.WAbil.MaxHandWeight = 50
	p.HitPoint = 9
	p.SpeedPoint = 18
	p.BonusPoint = 3
	p.Gold = 777
	p.HitSpeed = -2 // 装备攻速修正，可为负（Delphi m_nHitSpeed）

	raw := []byte(decodeTestBody(p.encodeAbilityBody()))
	if len(raw) != 62 {
		t.Fatalf("ability body len = %d, want 62", len(raw))
	}
	u16 := func(o int) int { return int(binary.LittleEndian.Uint16(raw[o : o+2])) }
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(raw[o : o+4]) }

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Level", u16(0), 42},
		{"AC", u32(2), uint32(0x00030001)},
		{"MAC", u32(6), uint32(2)},
		{"DC", u32(10), uint32(0x000A0005)},
		{"MC", u32(14), uint32(6)},
		{"SC", u32(18), uint32(7)},
		{"HP", u16(22), 321},
		{"MaxHP", u16(24), 654},
		{"MP", u16(26), 111},
		{"MaxMP", u16(28), 222},
		{"Exp", u32(30), uint32(12345)},
		{"Weight", u16(38), 30},
		{"MaxWeight", u16(40), 500},
		{"WearWeight", u16(42), 12},
		{"MaxWearWeight", u16(44), 100},
		{"HandWeight", u16(46), 8},
		{"MaxHandWeight", u16(48), 50},
		{"Hit", u16(50), 9},
		{"Speed", u16(52), 18},
		{"BonusPoint", u16(54), 3},
		{"Gold", u32(56), uint32(777)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	// MaxExp 在服务端计算 (GetMaxExp)；偏移 34 必须承载该值。
	if got := u32(34); got != p.GetMaxExp() {
		t.Errorf("MaxExp = %d, want %d", got, p.GetMaxExp())
	}
	// HitSpeed 位于 offset 60，按 int16 位模式编码（可为负）。
	if got := int(int16(u16(60))); got != p.HitSpeed {
		t.Errorf("HitSpeed = %d, want %d", got, p.HitSpeed)
	}
}

func TestEncodeStdItemsBodyLayout(t *testing.T) {
	items := []ItemDef{
		{
			Idx: 7, Name: "WoodSword", StdMode: 5, Shape: 1, Weight: 10,
			Looks: 42, DuraMax: 1000, AC: 0, ACMax: 0, DC: 1, DCMax: 3, Price: 100, NeedLevel: 2,
		},
		{
			Idx: 9, Name: "金创药", StdMode: 0, Shape: 0, Weight: 1,
			Looks: 120, DuraMax: 0, Price: 50,
		},
	}
	raw := []byte(decodeTestBody(encodeStdItemsBody(items)))
	if len(raw) < 2 {
		t.Fatal("body too short")
	}
	count := int(binary.LittleEndian.Uint16(raw[0:2]))
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// 第一条记录。
	off := 2
	if idx := int(binary.LittleEndian.Uint16(raw[off : off+2])); idx != 7 {
		t.Errorf("item0 idx = %d, want 7", idx)
	}
	if looks := binary.LittleEndian.Uint16(raw[off+2 : off+4]); looks != 42 {
		t.Errorf("item0 looks = %d, want 42", looks)
	}
	if raw[off+4] != 5 || raw[off+5] != 1 || raw[off+6] != 10 || raw[off+7] != 2 {
		t.Errorf("item0 std/shape/weight/need = %d/%d/%d/%d, want 5/1/10/2",
			raw[off+4], raw[off+5], raw[off+6], raw[off+7])
	}
	if dc := binary.LittleEndian.Uint16(raw[off+16 : off+18]); dc != 1 {
		t.Errorf("item0 DC = %d, want 1", dc)
	}
	if dcMax := binary.LittleEndian.Uint16(raw[off+18 : off+20]); dcMax != 3 {
		t.Errorf("item0 DCMax = %d, want 3", dcMax)
	}
	// 固定部分: 2+2+4+20+4 = 32 字节，然后是 NameLen u8 + Name。
	if price := binary.LittleEndian.Uint32(raw[off+28 : off+32]); price != 100 {
		t.Errorf("item0 price = %d, want 100", price)
	}
	nameLen := int(raw[off+32])
	if nameLen != len("WoodSword") {
		t.Fatalf("item0 nameLen = %d, want %d", nameLen, len("WoodSword"))
	}
	if name := string(raw[off+33 : off+33+nameLen]); name != "WoodSword" {
		t.Errorf("item0 name = %q, want WoodSword", name)
	}

	// 第二条记录: UTF-8 中文名可正确往返。
	off += 33 + nameLen
	if idx := int(binary.LittleEndian.Uint16(raw[off : off+2])); idx != 9 {
		t.Errorf("item1 idx = %d, want 9", idx)
	}
	nameLen2 := int(raw[off+32])
	name2 := string(raw[off+33 : off+33+nameLen2])
	if name2 != "金创药" {
		t.Errorf("item1 name = %q, want 金创药", name2)
	}
}

func TestEncodeMyMagicBodyLayout(t *testing.T) {
	p := NewPlayObject(nil, "Caster", 2)
	p.MagicDB = &MagicDB{
		Magics: []MagicDef{{MagID: 1, MagName: "火球术", Effect: 4}},
		byID:   map[int]*MagicDef{1: {MagID: 1, MagName: "火球术", Effect: 4}},
	}
	p.LearnedMagics = []*PlayerMagic{
		{MagID: 1, Level: 2, Key: '3', TrainPoint: 120},
	}
	raw := []byte(decodeTestBody(p.encodeMyMagicBody()))
	if len(raw) < 2 {
		t.Fatal("body too short")
	}
	if count := int(binary.LittleEndian.Uint16(raw[0:2])); count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	off := 2
	if magID := binary.LittleEndian.Uint16(raw[off : off+2]); magID != 1 {
		t.Errorf("magID = %d, want 1", magID)
	}
	if raw[off+2] != 2 {
		t.Errorf("level = %d, want 2", raw[off+2])
	}
	if raw[off+3] != '3' {
		t.Errorf("key = %q, want '3'", raw[off+3])
	}
	if icon := binary.LittleEndian.Uint16(raw[off+4 : off+6]); icon != 8 { // Effect*2
		t.Errorf("icon = %d, want 8", icon)
	}
	if cur := binary.LittleEndian.Uint16(raw[off+6 : off+8]); cur != 120 {
		t.Errorf("curTrain = %d, want 120", cur)
	}
	if max := binary.LittleEndian.Uint16(raw[off+8 : off+10]); max != uint16(magicMaxTrain[2]) {
		t.Errorf("maxTrain = %d, want %d", max, magicMaxTrain[2])
	}
	nameLen := int(raw[off+10])
	if name := string(raw[off+11 : off+11+nameLen]); name != "火球术" {
		t.Errorf("name = %q, want 火球术", name)
	}
}

// decodeTestBody 模拟客户端 NetHandler 处理消息体的方式:
// EncodeBuffer 的输出经 DecodeString 解码后得到原始字节字符串。
func decodeTestBody(encoded string) string {
	return protocol.DecodeString(encoded)
}
