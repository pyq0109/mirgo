package main

import "testing"

// TestParseGoodsText 验证 Delphi 商品列表文本解析
// （ClMain.pas:5565-5580 语义：每 4 段一组，名字/价格/库存任一为空终止）。
func TestParseGoodsText(t *testing.T) {
	goods := parseGoodsText("金创药(小量)/0/100/200/乌木剑/1/500/9/")
	if len(goods) != 2 {
		t.Fatalf("len = %d, want 2", len(goods))
	}
	g0 := goods[0]
	if g0.Name != "金创药(小量)" || g0.SubMenu != 0 || g0.Price != 100 || g0.Stock != 200 || g0.Grade != -1 {
		t.Errorf("goods[0] = %+v", g0)
	}
	g1 := goods[1]
	if g1.Name != "乌木剑" || g1.SubMenu != 1 || g1.Price != 500 || g1.Stock != 9 {
		t.Errorf("goods[1] = %+v", g1)
	}

	// 名字为空 → 立即终止（Delphi break 语义）
	if got := parseGoodsText("/0/100/200/"); len(got) != 0 {
		t.Errorf("empty-name: len = %d, want 0", len(got))
	}

	// 残缺尾部（不足 4 段）忽略
	if got := parseGoodsText("甲/0/10/5/乙/1"); len(got) != 1 {
		t.Errorf("truncated tail: len = %d, want 1", len(got))
	}

	// 价格为空 → 终止
	if got := parseGoodsText("甲/0//5/"); len(got) != 0 {
		t.Errorf("empty-price: len = %d, want 0", len(got))
	}

	// 空 body
	if got := parseGoodsText(""); len(got) != 0 {
		t.Errorf("empty body: len = %d, want 0", len(got))
	}
}
