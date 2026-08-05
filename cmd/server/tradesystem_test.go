package main

import (
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// 交易改金币回包按 Delphi MakeLong 语义拆分 32 位钱包金币
//（ClMain.pas:4819/4824）：Param=Lo、Tag=Hi，客户端组装还原。
func TestDealGoldMsgLayout(t *testing.T) {
	cases := []struct {
		name       string
		ident      uint16
		dealGold   int
		walletGold int
	}{
		{"OK 小额", protocol.SMDealChgGoldOK, 500, 1234},
		{"OK 超 u16", protocol.SMDealChgGoldOK, 0, 70000},
		{"OK 上限", protocol.SMDealChgGoldOK, 1000000, 9000000},
		{"Fail 携带当前值", protocol.SMDealChgGoldFail, 300, 88888},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := dealGoldMsg(c.ident, c.dealGold, c.walletGold)
			if int(msg.Ident) != int(c.ident) {
				t.Errorf("ident = %d, want %d", msg.Ident, c.ident)
			}
			if int(msg.Recog) != c.dealGold {
				t.Errorf("recog = %d, want %d", msg.Recog, c.dealGold)
			}
			got := int(uint32(msg.Param) | uint32(msg.Tag)<<16)
			if got != c.walletGold {
				t.Errorf("wallet gold roundtrip = %d, want %d", got, c.walletGold)
			}
		})
	}
}
