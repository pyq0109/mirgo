package main

import (
	"testing"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
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
// newTestPlayer 构造带空会话的测试玩家（sysMsg 等发送路径安全）。
func newTestPlayer() (*PlayObject, *netserver.TCPServer) {
	p := &PlayObject{BaseObject: NewBaseObject("tester", 1)}
	p.Session = &netserver.Session{ID: 1}
	return p, &netserver.TCPServer{}
}

// TestGoldShape 金币堆外观阈值（Delphi GetGoldShape，M2Share.pas:3568-3576）。
func TestGoldShape(t *testing.T) {
	cases := []struct{ gold, want int }{
		{1, 112}, {29, 112}, {30, 113}, {69, 113}, {70, 114},
		{299, 114}, {300, 115}, {999, 115}, {1000, 116}, {5000, 116},
	}
	for _, c := range cases {
		if got := goldShape(c.gold); got != c.want {
			t.Errorf("goldShape(%d) = %d, want %d", c.gold, got, c.want)
		}
	}
}

// TestAddGroundItemGoldMerge 金币合并（Envir.pas:205-228）：
// 同格合并 ≤2000；超限不合并；外观随数量变化。
func TestAddGroundItemGoldMerge(t *testing.T) {
	env := newTestEnv(40, 40)
	now := time.Now().UnixMilli()

	first := &GroundItem{ID: 1, Name: "金币", X: 10, Y: 10, Gold: 500, DropTick: now}
	if placed := env.AddGroundItem(first); placed != first {
		t.Fatal("first gold pile should be placed as new")
	}
	if first.Looks != 115 {
		t.Errorf("500 gold looks = %d, want 115", first.Looks)
	}

	// 合并：500+1500 = 2000 恰好不超限。
	second := &GroundItem{ID: 2, Name: "金币", X: 10, Y: 10, Gold: 1500, DropTick: now}
	if placed := env.AddGroundItem(second); placed != first {
		t.Fatal("second pile should merge into first")
	}
	if first.Gold != 2000 || first.Looks != 116 {
		t.Errorf("merged pile = %d gold looks=%d, want 2000/116", first.Gold, first.Looks)
	}
	if len(env.GroundItems) != 1 {
		t.Fatalf("ground items = %d, want 1", len(env.GroundItems))
	}

	// 再堆 1 金币：2001 > 2000，另起新堆。
	third := &GroundItem{ID: 3, Name: "金币", X: 10, Y: 10, Gold: 1, DropTick: now}
	if placed := env.AddGroundItem(third); placed != third {
		t.Fatal("over-cap gold should start a new pile")
	}
	if len(env.GroundItems) != 2 {
		t.Fatalf("ground items = %d, want 2", len(env.GroundItems))
	}
}

// TestAddGroundItemCellCap 每格地面物品 ≤5 件（Envir.pas:230-234）。
func TestAddGroundItemCellCap(t *testing.T) {
	env := newTestEnv(40, 40)
	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		gi := &GroundItem{ID: int32(i + 1), Name: "金创药", X: 5, Y: 5, DropTick: now}
		if env.AddGroundItem(gi) == nil {
			t.Fatalf("item %d should be placed", i+1)
		}
	}
	sixth := &GroundItem{ID: 6, Name: "金创药", X: 5, Y: 5, DropTick: now}
	if env.AddGroundItem(sixth) != nil {
		t.Fatal("6th item in same cell should be rejected")
	}
	if len(env.GroundItems) != 5 {
		t.Fatalf("ground items = %d, want 5", len(env.GroundItems))
	}
	// 相邻格不受影响。
	other := &GroundItem{ID: 7, Name: "金创药", X: 6, Y: 5, DropTick: now}
	if env.AddGroundItem(other) == nil {
		t.Fatal("adjacent cell should accept item")
	}
}

// TestCheckItemNeed 装备需求全分支（ObjBase.pas:23001-23260）。
func TestCheckItemNeed(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("tester", 1)}
	p.WAbil.Level = 30
	p.WAbil.DC = 20 << 16
	p.WAbil.MC = 10 << 16
	p.WAbil.SC = 5 << 16
	p.ReNewLevel = 2
	p.CreditPoint = 50
	p.Job = 0

	cases := []struct {
		name string
		def  ItemDef
		want bool
	}{
		{"Need0 等级满足", ItemDef{Need: 0, NeedLevel: 30}, true},
		{"Need0 等级不足", ItemDef{Need: 0, NeedLevel: 31}, false},
		{"Need1 DC满足", ItemDef{Need: 1, NeedLevel: 20}, true},
		{"Need1 DC不足", ItemDef{Need: 1, NeedLevel: 21}, false},
		{"Need2 MC满足", ItemDef{Need: 2, NeedLevel: 10}, true},
		{"Need3 SC满足", ItemDef{Need: 3, NeedLevel: 5}, true},
		{"Need4 转生满足", ItemDef{Need: 4, NeedLevel: 2}, true},
		{"Need4 转生不足", ItemDef{Need: 4, NeedLevel: 3}, false},
		{"Need5 声望满足", ItemDef{Need: 5, NeedLevel: 50}, true},
		{"Need5 声望不足", ItemDef{Need: 5, NeedLevel: 51}, false},
		{"Need6 无行会", ItemDef{Need: 6}, false},
		{"Need8 会员系统未实装", ItemDef{Need: 8}, false},
		{"Need10 职业+等级满足", ItemDef{Need: 10, NeedLevel: 0 | 30<<16}, true},
		{"Need10 职业不符", ItemDef{Need: 10, NeedLevel: 1 | 30<<16}, false},
		{"Need11 职业+DC", ItemDef{Need: 11, NeedLevel: 0 | 20<<16}, true},
		{"Need40 转生+等级", ItemDef{Need: 40, NeedLevel: 2 | 30<<16}, true},
		{"Need40 转生不足", ItemDef{Need: 40, NeedLevel: 3 | 30<<16}, false},
		{"Need44 转生+声望", ItemDef{Need: 44, NeedLevel: 2 | 50<<16}, true},
		{"GEEM2 Need200 无需求", ItemDef{Need: 200, NeedLevel: 0}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := p.checkItemNeed(&c.def); got != c.want {
				t.Errorf("checkItemNeed(%+v) = %v, want %v", c.def, got, c.want)
			}
		})
	}

	// 行会/城堡分支。
	p.GuildName = "测试行会"
	p.GuildRank = "掌门人"
	if !p.checkItemNeed(&ItemDef{Need: 6}) {
		t.Error("Need6 with guild should pass")
	}
	if !p.checkItemNeed(&ItemDef{Need: 60}) {
		t.Error("Need60 guild master should pass")
	}
	p.GuildRank = "成员"
	if p.checkItemNeed(&ItemDef{Need: 60}) {
		t.Error("Need60 non-master should fail")
	}
}

// TestCanTakeOffItem 四道禁脱校验（ObjBase.pas:17119-17151）。
func TestCanTakeOffItem(t *testing.T) {
	p, _ := newTestPlayer()
	p.ItemDB = newTestItemDB(ItemDef{Idx: 10, Name: "戒指", StdMode: 22, Reserved: 0})

	item := &protocol.UserItem{MakeIndex: 1, WIndex: 10}
	def := p.ItemDB.GetByIdx(10)

	// 普通物品可脱。
	if !p.canTakeOffItem(item, def) {
		t.Fatal("plain item should be removable")
	}
	// 首饰封印 btValue[7]≠0 禁脱。
	item.BtValue[7] = 1
	if p.canTakeOffItem(item, def) {
		t.Fatal("sealed accessory should be locked")
	}
	// 解锁药绕过封印。
	p.UserUnLockDurg = true
	if !p.canTakeOffItem(item, def) {
		t.Fatal("unlock potion should bypass seal")
	}
	item.BtValue[7] = 0
	p.UserUnLockDurg = false

	// Reserved&2 禁脱（可被解锁药绕过）。
	def.Reserved = 2
	if p.canTakeOffItem(item, def) {
		t.Fatal("Reserved&2 should be locked")
	}
	p.UserUnLockDurg = true
	if !p.canTakeOffItem(item, def) {
		t.Fatal("unlock potion should bypass Reserved&2")
	}
	p.UserUnLockDurg = false

	// Reserved&4 永久禁脱（解锁药无效）。
	def.Reserved = 4
	if p.canTakeOffItem(item, def) {
		t.Fatal("Reserved&4 should be permanently locked")
	}
	def.Reserved = 0

	// 禁脱列表。
	p.ItemDB.DisableTakeOff[10] = true
	if p.canTakeOffItem(item, def) {
		t.Fatal("disable-takeoff list item should be locked")
	}
}

// TestUsePackItem StdMode 31 解包（ObjBase.pas:17394-17413）：
// (count+6-1) ≤ maxBag 校验 + 发 6 件 + 消耗打包物品。
func TestUsePackItem(t *testing.T) {
	p, srv := newTestPlayer()
	p.ItemDB = newTestItemDB(
		ItemDef{Idx: 1, Name: "打包药材", StdMode: 31, AniCount: 0, Shape: 100},
		ItemDef{Idx: 2, Name: "强效金创药", StdMode: 0},
	)
	p.ItemDB.UnbindList[100] = "强效金创药"
	p.Engine = &UserEngine{nextItemID: 1000, Config: &ServerConfig{}}
	p.ItemList = []*protocol.UserItem{{MakeIndex: 5, WIndex: 1}}

	def := p.ItemDB.GetByIdx(1)
	if !p.usePackItem(def, 0, srv) {
		t.Fatal("unpack should succeed")
	}
	if len(p.ItemList) != 6 {
		t.Fatalf("bag = %d items, want 6", len(p.ItemList))
	}
	for _, it := range p.ItemList {
		if it.WIndex != 2 {
			t.Fatalf("unbound item WIndex = %d, want 2", it.WIndex)
		}
	}

	// 背包空位不足（count+6-1 > 46）拒绝且不消耗。
	p2, srv2 := newTestPlayer()
	p2.ItemDB = p.ItemDB
	p2.Engine = &UserEngine{nextItemID: 2000, Config: &ServerConfig{}}
	for i := 0; i < 42; i++ {
		p2.ItemList = append(p2.ItemList, &protocol.UserItem{MakeIndex: int32(i + 10), WIndex: 2})
	}
	p2.ItemList = append(p2.ItemList, &protocol.UserItem{MakeIndex: 99, WIndex: 1})
	if p2.usePackItem(p2.ItemDB.GetByIdx(1), 42, srv2) {
		t.Fatal("unpack should fail with insufficient bag space")
	}
	if len(p2.ItemList) != 43 {
		t.Fatalf("bag unchanged = %d, want 43", len(p2.ItemList))
	}

	// 无解包表项 → 失败。
	p3, srv3 := newTestPlayer()
	p3.ItemDB = newTestItemDB(ItemDef{Idx: 1, Name: "神秘包", StdMode: 31, AniCount: 0, Shape: 99})
	p3.Engine = &UserEngine{nextItemID: 3000, Config: &ServerConfig{}}
	p3.ItemList = []*protocol.UserItem{{MakeIndex: 5, WIndex: 1}}
	if p3.usePackItem(p3.ItemDB.GetByIdx(1), 0, srv3) {
		t.Fatal("unpack without unbind entry should fail")
	}
}

// TestQuestCheckItemAndTake CHECKITEM 记录最高耐久实例，
// TAKECHECKITEM 精确扣除（ObjBase.pas:24539-24619）。
func TestQuestCheckItemAndTake(t *testing.T) {
	p, srv := newTestPlayer()
	p.ItemDB = newTestItemDB(ItemDef{Idx: 20, Name: "肉", StdMode: 40})
	p.ItemList = []*protocol.UserItem{
		{MakeIndex: 1, WIndex: 20, Dura: 100},
		{MakeIndex: 2, WIndex: 20, Dura: 900},
		{MakeIndex: 3, WIndex: 20, Dura: 500},
	}

	n, best := p.questCheckItem("肉")
	if n != 3 || best == nil || best.MakeIndex != 2 {
		t.Fatalf("questCheckItem = (%d,%v), want (3, MakeIndex=2)", n, best)
	}

	p.questTakeCheckItem(srv)
	if len(p.ItemList) != 2 {
		t.Fatalf("bag = %d after take, want 2", len(p.ItemList))
	}
	for _, it := range p.ItemList {
		if it.MakeIndex == 2 {
			t.Fatal("taken item should be MakeIndex=2")
		}
	}
}

// TestIncGold 金币上限整体拒收（ObjBase.pas:1978-1987）。
func TestIncGold(t *testing.T) {
	p, _ := newTestPlayer()
	p.Engine = &UserEngine{Config: &ServerConfig{}}
	p.Gold = 9999999
	if p.incGold(1) != true {
		t.Fatal("9999999+1 should succeed")
	}
	if p.Gold != 10000000 {
		t.Fatalf("gold = %d, want 10000000", p.Gold)
	}
	if p.incGold(1) != false {
		t.Fatal("over-cap incGold should fail without partial accept")
	}
	if p.Gold != 10000000 {
		t.Fatalf("gold unchanged = %d", p.Gold)
	}
}

// TestStoragePassword 仓库密码流程（ObjBase.pas:7229-7323）：
// 4-7 位、设置后上锁、解锁、错 >3 次锁定。
func TestStoragePassword(t *testing.T) {
	p, srv := newTestPlayer()

	p.setStoragePassword(srv, "abc") // 太短
	if p.StoragePassword != "" {
		t.Fatal("3-char password should be rejected")
	}
	p.setStoragePassword(srv, "12345678") // 太长
	if p.StoragePassword != "" {
		t.Fatal("8-char password should be rejected")
	}
	p.setStoragePassword(srv, "abcd")
	if p.StoragePassword != "abcd" || !p.StoragePwdLocked {
		t.Fatal("valid password should be set and lock storage")
	}
	p.setStoragePassword(srv, "xxxx") // 重复设置
	if p.StoragePassword != "abcd" {
		t.Fatal("second set should be rejected")
	}

	// 错误解锁 4 次 → 仍锁定；正确密码解锁。
	for i := 0; i < 4; i++ {
		p.unlockStorage(srv, "wrong")
	}
	if !p.StoragePwdLocked {
		t.Fatal("storage should stay locked after wrong attempts")
	}
	p.unlockStorage(srv, "abcd")
	if p.StoragePwdLocked {
		t.Fatal("correct password should unlock")
	}

	// 修改密码。
	p.changeStoragePassword(srv, "wrong", "efgh")
	if p.StoragePassword != "abcd" {
		t.Fatal("change with wrong old password should fail")
	}
	p.changeStoragePassword(srv, "abcd", "efgh")
	if p.StoragePassword != "efgh" {
		t.Fatal("change with correct old password should succeed")
	}
}

// TestCheckItemsNeedAutoTakeOff Need 6/7/60/70 复查与自动脱下
//（ObjBase.pas:6622-6666/25985+）。
func TestCheckItemsNeedAutoTakeOff(t *testing.T) {
	p, srv := newTestPlayer()
	p.ItemDB = newTestItemDB(
		ItemDef{Idx: 30, Name: "行会戒指", StdMode: 22, Need: 6},
		ItemDef{Idx: 31, Name: "普通戒指", StdMode: 22, Need: 0},
	)
	p.Engine = &UserEngine{Config: &ServerConfig{}}
	p.GuildName = ""
	p.UseItems[protocol.URingL] = &protocol.UserItem{MakeIndex: 1, WIndex: 30}
	p.UseItems[protocol.URingR] = &protocol.UserItem{MakeIndex: 2, WIndex: 31}

	p.checkAutoTakeOff(srv)

	if p.UseItems[protocol.URingL] != nil {
		t.Fatal("guild item should be auto-removed without guild")
	}
	if p.UseItems[protocol.URingR] == nil {
		t.Fatal("plain item should stay equipped")
	}
	if len(p.ItemList) != 1 || p.ItemList[0].MakeIndex != 1 {
		t.Fatal("removed guild item should go to bag")
	}
}

// TestFoodHungerStatus 食物（StdMode 1）饥饿度 += DuraMax/10 上限 5000
//（ObjBase.pas:23371-23379）。
func TestFoodHungerStatus(t *testing.T) {
	p, srv := newTestPlayer()
	p.ItemDB = newTestItemDB(ItemDef{Idx: 40, Name: "烤肉", StdMode: 1, DuraMax: 3000})
	p.Engine = &UserEngine{Config: &ServerConfig{}}
	p.HungerStatus = 4900
	p.ItemList = []*protocol.UserItem{{MakeIndex: 1, WIndex: 40, Dura: 3000}}

	msg := SendMessage{Param1: 1}
	p.HandleEatItem(msg, srv)

	if len(p.ItemList) != 0 {
		t.Fatal("food should be consumed")
	}
	if p.HungerStatus != 5000 {
		t.Fatalf("hunger = %d, want capped 5000", p.HungerStatus)
	}
}

// TestDropDeathItemsPvP 被玩家杀默认不掉装备（Delphi
// boKillByHumanDropUseItem=False）；被怪杀才掉。
func TestDropDeathItemsPvP(t *testing.T) {
	srv := &netserver.TCPServer{}
	newPlayer := func() *PlayObject {
		p := &PlayObject{BaseObject: NewBaseObject("victim", 1)}
		p.Session = &netserver.Session{ID: 1}
		p.ItemDB = newTestItemDB(ItemDef{Idx: 50, Name: "铁剑", StdMode: 5})
		p.Engine = &UserEngine{Config: &ServerConfig{}}
		p.envir = newTestEnv(40, 40)
		p.UseItems[protocol.UWeapon] = &protocol.UserItem{MakeIndex: 1, WIndex: 50}
		return p
	}

	// PvP：装备保留。
	p1 := newPlayer()
	p1.DropDeathItems(srv, false)
	if p1.UseItems[protocol.UWeapon] == nil {
		t.Fatal("PvP death should keep equipment (Delphi default)")
	}

	// PvE：多次尝试后装备应按概率掉落（1/30）。
	dropped := 0
	for i := 0; i < 300; i++ {
		p := newPlayer()
		p.DropDeathItems(srv, true)
		if p.UseItems[protocol.UWeapon] == nil {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatal("PvE death should drop equipment with 1/30 probability")
	}
	if dropped > 60 {
		t.Fatalf("PvE drop count %d too high for 1/30 rate", dropped)
	}
}

// TestDropDeathItemsReserved Reserved&8 死亡销毁；Reserved&2（即
// Delphi "Reserved and 10" 检查中存活的位）死亡保护
//（ObjBase.pas:20689-20763，修正原版"地上副本+身上仍装备"复制 bug
// 为落地前拦截）。
func TestDropDeathItemsReserved(t *testing.T) {
	srv := &netserver.TCPServer{}
	newPlayer := func(reserved int) *PlayObject {
		p := &PlayObject{BaseObject: NewBaseObject("victim", 1)}
		p.Session = &netserver.Session{ID: 1}
		p.ItemDB = newTestItemDB(ItemDef{Idx: 50, Name: "铁剑", StdMode: 5, Reserved: reserved})
		p.Engine = &UserEngine{Config: &ServerConfig{}}
		p.envir = newTestEnv(40, 40)
		p.UseItems[protocol.UWeapon] = &protocol.UserItem{MakeIndex: 1, WIndex: 50}
		return p
	}

	// &8：跑多次确保销毁（不受 1/30 概率影响）。
	for i := 0; i < 5; i++ {
		p := newPlayer(8)
		p.DropDeathItems(srv, true)
		if p.UseItems[protocol.UWeapon] != nil {
			t.Fatal("Reserved&8 item should be destroyed on death")
		}
		if len(p.envir.GroundItems) != 0 {
			t.Fatal("Reserved&8 item should not land on ground")
		}
	}

	// &2（Reserved and 10 ≠ 0 的存活位）：始终保留在身上，不落地。
	for i := 0; i < 300; i++ {
		p := newPlayer(2)
		p.DropDeathItems(srv, true)
		if p.UseItems[protocol.UWeapon] == nil {
			t.Fatal("Reserved&2 item should stay equipped on death")
		}
		if len(p.envir.GroundItems) != 0 {
			t.Fatal("Reserved&2 item should never land on ground")
		}
	}
}
