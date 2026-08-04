package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// newTestItemDB 构造最小物品库（单测用）。
func newTestItemDB(defs ...ItemDef) *ItemDB {
	db := &ItemDB{
		Items:  defs,
		byName: make(map[string]*ItemDef),
		byIdx:  make(map[int]*ItemDef),
	}
	for i := range db.Items {
		item := &db.Items[i]
		db.byName[item.Name] = item
		db.byIdx[item.Idx] = item
	}
	return db
}

func TestCondCheckItemType(t *testing.T) {
	db := newTestItemDB(ItemDef{Idx: 100, Name: "木剑", StdMode: 5})
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	p.ItemDB = db

	// 未装备 → false
	if condCheckItemType([]string{"CHECKITEMTYPE", "0", "5"}, p) {
		t.Fatal("空槽位应返回 false")
	}

	p.UseItems[0] = &protocol.UserItem{WIndex: 100}
	// 类型匹配 → true
	if !condCheckItemType([]string{"CHECKITEMTYPE", "0", "5"}, p) {
		t.Fatal("StdMode 匹配应返回 true")
	}
	// 类型不匹配 → false
	if condCheckItemType([]string{"CHECKITEMTYPE", "0", "6"}, p) {
		t.Fatal("StdMode 不匹配应返回 false")
	}
	// 非法槽位 → false
	if condCheckItemType([]string{"CHECKITEMTYPE", "99", "5"}, p) {
		t.Fatal("非法槽位应返回 false")
	}
	// 参数不足 → false
	if condCheckItemType([]string{"CHECKITEMTYPE"}, p) {
		t.Fatal("参数不足应返回 false")
	}
}

func TestCondCheckHorseAndCastleWar(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	if condCheckHorse(nil, p) {
		t.Fatal("未骑乘应返回 false")
	}
	p.OnHorse = true
	if !condCheckHorse(nil, p) {
		t.Fatal("骑乘应返回 true")
	}

	// 无引擎/城堡 → false
	if condCheckCastleWar(nil, p) {
		t.Fatal("无城堡应返回 false")
	}
}

func TestCondCheckMonArea(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	p.MapName = "0"
	mon := NewMonsterObject("鹿", 10, 52, 0, 0, 10, 100, 100, 5)
	mon.MapName = "0"
	mon.CurrX, mon.CurrY = p.CurrX+2, p.CurrY
	p.Engine = &UserEngine{Monsters: []*MonsterObject{mon}}

	if !condCheckMonArea([]string{"CHECKMONAREA", "鹿"}, p) {
		t.Fatal("范围内存在怪物应返回 true")
	}
	if condCheckMonArea([]string{"CHECKMONAREA", "狼"}, p) {
		t.Fatal("名称不匹配应返回 false")
	}
	if condCheckMonArea([]string{"CHECKMONAREA", "鹿", "2"}, p) {
		t.Fatal("数量不足应返回 false")
	}
	// 超出范围
	mon.CurrX = p.CurrX + viewRange + 5
	if condCheckMonArea([]string{"CHECKMONAREA", "鹿"}, p) {
		t.Fatal("范围外怪物不应计入")
	}
}

func TestActFame(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	actIncFame([]string{"INCFAME", "3"}, p, nil, nil)
	if p.CreditPoint != 3 {
		t.Fatalf("INCFAME 3 后应为 3，实际 %d", p.CreditPoint)
	}
	actDecFame([]string{"DECFAME", "5"}, p, nil, nil)
	if p.CreditPoint != 0 {
		t.Fatalf("DECFAME 不应低于 0，实际 %d", p.CreditPoint)
	}
}

func TestActHorseAndHair(t *testing.T) {
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	// HORSECALL/KILLHORSE 无 session 时仅切换状态（不广播，broadcastFeatureChanged
	// 依赖 envir；此处验证状态字段）
	actHorseCall([]string{"HORSECALL"}, p, nil, nil)
	if !p.OnHorse {
		t.Fatal("HORSECALL 应上马")
	}
	actKillHorse([]string{"KILLHORSE"}, p, nil, nil)
	if p.OnHorse {
		t.Fatal("KILLHORSE 应下马")
	}
}

// TestScriptRegistryDispatch 验证扩展命令经由 evalOneCondition/execOneAction
// 的注册表分支分发（大小写不敏感）。
func TestScriptRegistryDispatch(t *testing.T) {
	s := &NpcScript{}
	p := &PlayObject{BaseObject: NewBaseObject("test", 1)}
	p.OnHorse = true

	if !s.evalOneCondition("checkhorse", p) {
		t.Fatal("evalOneCondition 应经注册表分发 checkhorse")
	}
	if s.evalOneCondition("CHECKCASTLEWAR", p) {
		t.Fatal("无城堡时 CHECKCASTLEWAR 应为 false")
	}
}
