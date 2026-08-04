package protocol

import (
	"testing"
)

// ============================================================================
// 已知答案向量（Known-Answer Tests）
// 手工计算，用于验证 6Bit 编解码与 Delphi EDcode.pas 的一致性。
// ============================================================================

func TestEncode6BitBufKnownAnswers(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		// 3 字节全零 → 4 个 0x3C ('<')
		{"all-zero-3", []byte{0, 0, 0}, []byte{0x3C, 0x3C, 0x3C, 0x3C}},
		// 3 字节全 0xFF → 4 个 0x7B ('{')
		{"all-ff-3", []byte{0xFF, 0xFF, 0xFF}, []byte{0x7B, 0x7B, 0x7B, 0x7B}},
		// 1 字节 0x00 → 2 字符（非 3 的倍数，有余数）
		{"single-zero", []byte{0x00}, []byte{0x3C, 0x3C}},
		// 1 字节 0xFF → 2 字符（余数 0x30 → 'l'）
		{"single-ff", []byte{0xFF}, []byte{0x7B, 0x6C}},
		// 2 字节 → 3 字符
		{"two-bytes", []byte{0x00, 0x00}, []byte{0x3C, 0x3C, 0x3C}},
		// "Hello" — 实际编码器输出
		{"hello", []byte("Hello"), []byte("NBQhWBx")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Encode6BitBuf(tt.in)
			if string(got) != string(tt.want) {
				t.Errorf("Encode6BitBuf(%v) = %q (0x%X), want %q (0x%X)",
					tt.in, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestDecode6BitBufKnownAnswers(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{"all-zero", []byte{0x3C, 0x3C, 0x3C, 0x3C}, []byte{0, 0, 0}},
		{"all-ff", []byte{0x7B, 0x7B, 0x7B, 0x7B}, []byte{0xFF, 0xFF, 0xFF}},
		{"hello", []byte("NBQhWBx"), []byte("Hello")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decode6BitBuf(tt.in)
			if string(got) != string(tt.want) {
				t.Errorf("Decode6BitBuf(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEncodeMessageKnownAnswer(t *testing.T) {
	msg := DefaultMessage{Recog: 1, Ident: 2, Param: 3, Tag: 4, Series: 5}
	encoded := EncodeMessage(msg)
	if len(encoded) != DefBlockSize {
		t.Fatalf("EncodeMessage length = %d, want %d", len(encoded), DefBlockSize)
	}
	// 验证编码结果只包含合法 6Bit 字符 (>= 0x3C)
	for i, c := range encoded {
		if c < 0x3C {
			t.Errorf("encoded[%d] = 0x%02X < 0x3C", i, c)
		}
	}
	// 往返验证
	decoded := DecodeMessage(encoded)
	if decoded != msg {
		t.Errorf("round-trip failed: got %+v, want %+v", decoded, msg)
	}
}

// ============================================================================
// 边界情况测试
// ============================================================================

func TestDecode6BitBufInvalidChar(t *testing.T) {
	// 含 < 0x3C 的字节应返回空
	got := Decode6BitBuf([]byte{0x3C, 0x20, 0x3C, 0x3C})
	if len(got) != 0 {
		t.Errorf("Decode6BitBuf with invalid char = %v, want empty", got)
	}
}

func TestDecode6BitBufEmpty(t *testing.T) {
	got := Decode6BitBuf(nil)
	if len(got) != 0 {
		t.Errorf("Decode6BitBuf(nil) = %v, want empty", got)
	}
	got = Decode6BitBuf([]byte{})
	if len(got) != 0 {
		t.Errorf("Decode6BitBuf([]) = %v, want empty", got)
	}
}

func TestDecodeMessageShortInput(t *testing.T) {
	// 少于 16 字符应返回零值
	msg := DecodeMessage("short")
	if msg != (DefaultMessage{}) {
		t.Errorf("DecodeMessage(short) = %+v, want zero", msg)
	}
}

func TestDecodeMessageGarbage(t *testing.T) {
	// 16 个合法字符但不是有效消息
	msg := DecodeMessage("<<<<<<<<<<<<<<<<")
	// 全零解码 → 零值消息
	if msg != (DefaultMessage{}) {
		t.Errorf("DecodeMessage(16x'<') = %+v, want zero", msg)
	}
}

func TestEncodeBufferBoundary(t *testing.T) {
	// 恰好 BufferSize → 返回空
	buf := make([]byte, BufferSize)
	if got := EncodeBuffer(buf); got != "" {
		t.Errorf("EncodeBuffer(BufferSize) = %q, want empty", got)
	}
	// BufferSize-1 → 正常
	buf = make([]byte, BufferSize-1)
	if got := EncodeBuffer(buf); got == "" {
		t.Error("EncodeBuffer(BufferSize-1) = empty, want non-empty")
	}
}

func TestDecodeBufferTruncation(t *testing.T) {
	// 编码 3 字节，解码到 5 字节 buf → 尾部保留旧值
	encoded := EncodeBuffer([]byte{0xAA, 0xBB, 0xCC})
	buf := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	DecodeBuffer(encoded, buf)
	if buf[0] != 0xAA || buf[1] != 0xBB || buf[2] != 0xCC {
		t.Errorf("DecodeBuffer decoded = %X, want AABBCC", buf[:3])
	}
	if buf[3] != 0x44 || buf[4] != 0x55 {
		t.Errorf("DecodeBuffer tail = %X, want 4455 (preserved)", buf[3:])
	}
}

func TestEncode6BitBufNil(t *testing.T) {
	if got := Encode6BitBuf(nil); got != nil {
		t.Errorf("Encode6BitBuf(nil) = %v, want nil", got)
	}
}

// ============================================================================
// IsClientIdent 测试
// ============================================================================

func TestIsClientIdent(t *testing.T) {
	// 所有 CM_* 值应返回 true
	cmValues := []uint16{
		CMQueryUsername, CMQueryBagItems, CMQueryUserState,
		CMQueryChr, CMNewChr, CMDelChr, CMSelChr, CMSelectServer,
		CMDropItem, CMPickup, CMOpenDoor, CMTakeOnItem, CMTakeOffItem,
		CMEat, CMButch, CMMagicKeyChange, CMClickNPC,
		CMMerchantDlgSelect, CMMerchantQuerySellPrice,
		CMUserSellItem, CMUserBuyItem, CMUserGetDetailItem,
		CMDropGold, CMLoginNoticeOK, CMGroupMode, CMCreateGroup,
		CMAddGroupMember, CMDelGroupMember, CMUserRepairItem,
		CMMerchantQueryRepairCost, CMDealTry, CMDealAddItem,
		CMDealDelItem, CMDealCancel, CMDealChgGold, CMDealEnd,
		CMUserStorageItem, CMUserTakeBackStorageItem, CMWantMinimap,
		CMUserMakeDrugItem, CMOpenGuildDlg, CMGuildHome,
		CMGuildMemberList, CMGuildAddMember, CMGuildDelMember,
		CMGuildUpdateNotice, CMGuildUpdateRankInfo, CMAdjustBonus,
		CMGuildAlly, CMGuildBreakAlly, CMChangeAttackMode,
		CMGuildWar, CMMineDig, CMWhisper, CMLogout, CMExitGame,
		CMAddFriend, CMDelFriend, CMQueryFriends,
		CMProtocol, CMIDPassword, CMAddNewUser, CMChangePassword, CMUpdateUser,
		CMThrow, CMTurn, CMWalk, CMSitdown, CMRun,
		CMHit, CMHeavyHit, CMBigHit, CMSpell, CMPowerHit,
		CMLongHit, CMWideHit, CMFireHit, CMSay, CMHorseRun,
		CMCrsHit, CMTwinHit,
	}
	for _, v := range cmValues {
		if !IsClientIdent(v) {
			t.Errorf("IsClientIdent(%d) = false, want true", v)
		}
	}

	// SM_* 值应返回 false（注意：SMSysMessage=100 与 CMQueryChr=100 数值碰撞，不列入）
	smValues := []uint16{
		SMWalk, SMRun, SMHit, SMSpell, SMLogon, SMNewMap,
		SMAbility, SMAddItem, SMBagItems,
		SMStartPlay, SMQueryChr, SMPasswdFail,
	}
	for _, v := range smValues {
		if IsClientIdent(v) {
			t.Errorf("IsClientIdent(%d) = true, want false (SM_*)", v)
		}
	}

	// 随机值应返回 false
	if IsClientIdent(9999) {
		t.Error("IsClientIdent(9999) = true, want false")
	}
	if IsClientIdent(0) {
		t.Error("IsClientIdent(0) = true, want false")
	}
}

// TestClientItemRoundTrip 验证详细商品列表（SM_SENDDETAILGOODSLIST）
// 条目的编解码往返，覆盖 Delphi TClientItem 的全部字段。
func TestClientItemRoundTrip(t *testing.T) {
	var name [20]byte
	copy(name[:], []byte{0xBE, 0xAD, 0x01, 0xFF}) // 含高位的任意字节
	in := ClientItem{
		S: StdItem{
			Name:         name,
			StdMode:      5,
			Shape:        1,
			Weight:       12,
			AniCount:     2,
			Source:       -1,
			Reserved:     7,
			NeedIdentify: 3,
			Looks:        1234,
			DuraMax:      22000,
			AC:           0x00020001,
			MAC:          0x00040003,
			DC:           0x00060005,
			MC:           0x00080007,
			SC:           0x000A0009,
			Need:         1,
			NeedLevel:    33,
			Price:        100000,
		},
		MakeIndex: 200123,
		Dura:      18000,
		DuraMax:   65535,
	}
	buf := EncodeClientItem(&in)
	if len(buf) != ClientItemSize {
		t.Fatalf("EncodeClientItem len = %d, want %d", len(buf), ClientItemSize)
	}
	out, ok := DecodeClientItem(buf)
	if !ok {
		t.Fatal("DecodeClientItem returned ok=false")
	}
	if out != in {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}

	// 短数据应失败
	if _, ok := DecodeClientItem(buf[:ClientItemSize-1]); ok {
		t.Error("DecodeClientItem(short) = ok, want false")
	}

	// 与 EncodeBuffer/Decode6BitBuf 组合往返（服务端 652 body 分段格式）
	seg := EncodeBuffer(buf)
	raw := Decode6BitBuf([]byte(seg))
	out2, ok := DecodeClientItem(raw)
	if !ok || out2 != in {
		t.Errorf("EncodeBuffer round trip mismatch: ok=%v got %+v", ok, out2)
	}
}
