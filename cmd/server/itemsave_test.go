package main

import (
	"encoding/json"
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// 仓库物品必须完整保存 BtValue（升级/幸运/诅咒等附加属性），
// 否则存仓库再取回会丢失强化数据（对照 Delphi THumData 整体
// 序列化，仓库与背包同格式）。
func TestStorageBtValueRoundTrip(t *testing.T) {
	item := &protocol.UserItem{
		MakeIndex: 42,
		WIndex:    100,
		Dura:      5000,
		DuraMax:   7000,
	}
	item.BtValue[0] = 3 // DC 升级
	item.BtValue[3] = 2 // 祝福
	item.BtValue[4] = 1 // 诅咒
	item.BtValue[10] = 11

	saved := savedUserItem{
		MakeIndex: item.MakeIndex,
		WIndex:    item.WIndex,
		Dura:      item.Dura,
		DuraMax:   item.DuraMax,
		BtValue:   item.BtValue,
	}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var loaded savedUserItem
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.BtValue != item.BtValue {
		t.Fatalf("BtValue mismatch: got %v want %v", loaded.BtValue, item.BtValue)
	}
}

// 旧存档无 btValue 字段时加载为零值，向后兼容。
func TestStorageBtValueLegacyArchive(t *testing.T) {
	legacy := `{"makeIndex":1,"wIndex":2,"dura":3,"duraMax":4}`
	var loaded savedUserItem
	if err := json.Unmarshal([]byte(legacy), &loaded); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	var zero [14]byte
	if loaded.BtValue != zero {
		t.Fatalf("legacy BtValue should be zero, got %v", loaded.BtValue)
	}
}
