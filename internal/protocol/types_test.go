package protocol

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// TestEncode6BitBufRoundTrip 测试编码后再解码能还原原始数据。
func TestEncode6BitBufRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x41}},
		{"two bytes", []byte{0x41, 0x42}},
		{"three bytes", []byte{0x41, 0x42, 0x43}},
		{"four bytes", []byte{0x41, 0x42, 0x43, 0x44}},
		{"zeros", []byte{0x00, 0x00, 0x00}},
		{"max values", []byte{0xFF, 0xFF, 0xFF}},
		{"mixed", []byte{0x00, 0x55, 0xAA, 0xFF, 0x01, 0x02}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := Encode6BitBuf(tc.data)
			decoded := Decode6BitBuf(encoded)

			if len(decoded) != len(tc.data) {
				t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(tc.data))
			}

			for i := range tc.data {
				if decoded[i] != tc.data[i] {
					t.Fatalf("byte %d mismatch: got 0x%02X, want 0x%02X", i, decoded[i], tc.data[i])
				}
			}
		})
	}
}

// TestEncodeMessageRoundTrip 测试消息编解码的往返。
func TestEncodeMessageRoundTrip(t *testing.T) {
	msg := DefaultMessage{
		Recog:  12345,
		Ident:  100,
		Param:  200,
		Tag:    300,
		Series: 400,
	}

	encoded := EncodeMessage(msg)
	if len(encoded) != DefBlockSize {
		t.Fatalf("encoded length: got %d, want %d", len(encoded), DefBlockSize)
	}

	decoded := DecodeMessage(encoded)

	if decoded.Recog != msg.Recog {
		t.Errorf("Recog: got %d, want %d", decoded.Recog, msg.Recog)
	}
	if decoded.Ident != msg.Ident {
		t.Errorf("Ident: got %d, want %d", decoded.Ident, msg.Ident)
	}
	if decoded.Param != msg.Param {
		t.Errorf("Param: got %d, want %d", decoded.Param, msg.Param)
	}
	if decoded.Tag != msg.Tag {
		t.Errorf("Tag: got %d, want %d", decoded.Tag, msg.Tag)
	}
	if decoded.Series != msg.Series {
		t.Errorf("Series: got %d, want %d", decoded.Series, msg.Series)
	}
}

// TestEncodeStringRoundTrip 测试字符串编解码的往返。
func TestEncodeStringRoundTrip(t *testing.T) {
	testCases := []string{
		"",
		"Hello",
		"Hello, World!",
		"测试中文",
		"**account/chrname/12345/120040918/9",
	}

	for _, str := range testCases {
		t.Run(str, func(t *testing.T) {
			encoded := EncodeString(str)
			decoded := DecodeString(encoded)

			if decoded != str {
				t.Fatalf("round trip failed: got %q, want %q", decoded, str)
			}
		})
	}
}

// TestEncodeBufferRoundTrip 测试缓冲区编解码的往返。
func TestEncodeBufferRoundTrip(t *testing.T) {
	original := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C}

	encoded := EncodeBuffer(original)
	decoded := make([]byte, len(original))
	DecodeBuffer(encoded, decoded)

	for i := range original {
		if decoded[i] != original[i] {
			t.Fatalf("byte %d mismatch: got 0x%02X, want 0x%02X", i, decoded[i], original[i])
		}
	}
}

// TestMakeDefaultMsg 测试消息创建。
func TestMakeDefaultMsg(t *testing.T) {
	msg := MakeDefaultMsg(CMIDPassword, 0, 0, 0, 0)

	if msg.Ident != CMIDPassword {
		t.Errorf("Ident: got %d, want %d", msg.Ident, CMIDPassword)
	}
	if msg.Recog != 0 {
		t.Errorf("Recog: got %d, want %d", msg.Recog, 0)
	}
}

// TestGetCodeMsgSize 测试编码后大小的计算。
func TestGetCodeMsgSize(t *testing.T) {
	testCases := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 2},   // ceil(1 * 4 / 3) = 2
		{2, 3},   // ceil(2 * 4 / 3) = 3
		{3, 4},   // ceil(3 * 4 / 3) = 4
		{12, 16}, // TDefaultMessage 大小
		{16, 22},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			result := GetCodeMsgSize(tc.input)
			if result != tc.expected {
				t.Errorf("GetCodeMsgSize(%d) = %d, want %d", tc.input, result, tc.expected)
			}
		})
	}
}

// TestDefaultMessageSize 验证 DefaultMessage 结构体大小。
func TestDefaultMessageSize(t *testing.T) {
	// DefaultMessage 应为 12 字节（4+2+2+2+2）
	var msg DefaultMessage
	size := unsafe.Sizeof(msg)
	if size != 12 {
		t.Errorf("DefaultMessage size: got %d, want 12", size)
	}

	// 验证二进制编码大小
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(msg.Recog))
	binary.LittleEndian.PutUint16(buf[4:6], msg.Ident)
	binary.LittleEndian.PutUint16(buf[6:8], msg.Param)
	binary.LittleEndian.PutUint16(buf[8:10], msg.Tag)
	binary.LittleEndian.PutUint16(buf[10:12], msg.Series)
}

// TestFeatureEncoding 测试 feature 编解码函数。
func TestFeatureEncoding(t *testing.T) {
	// 测试人物 feature
	raceImg := byte(0)
	dress := byte(1)
	weapon := byte(2)
	hair := byte(3)

	feature := MakeHumanFeature(raceImg, dress, weapon, hair)

	gotRaceImg, gotDress, gotWeapon, gotHair := ParseHumanFeature(feature)

	if gotRaceImg != raceImg {
		t.Errorf("raceImg: got %d, want %d", gotRaceImg, raceImg)
	}
	if gotDress != dress {
		t.Errorf("dress: got %d, want %d", gotDress, dress)
	}
	if gotWeapon != weapon {
		t.Errorf("weapon: got %d, want %d", gotWeapon, weapon)
	}
	if gotHair != hair {
		t.Errorf("hair: got %d, want %d", gotHair, hair)
	}

	// 测试怪物 feature
	mRaceImg := byte(80)
	mWeapon := byte(0)
	mAppr := uint16(10)

	mFeature := MakeMonsterFeature(mRaceImg, mWeapon, mAppr)

	gotMRaceImg, gotMWeapon, gotMAppr := ParseMonsterFeature(mFeature)

	if gotMRaceImg != mRaceImg {
		t.Errorf("raceImg: got %d, want %d", gotMRaceImg, mRaceImg)
	}
	if gotMWeapon != mWeapon {
		t.Errorf("weapon: got %d, want %d", gotMWeapon, mWeapon)
	}
	if gotMAppr != mAppr {
		t.Errorf("appr: got %d, want %d", gotMAppr, mAppr)
	}
}

// TestConstants 验证关键常量与 Delphi 源码一致。
func TestConstants(t *testing.T) {
	// 方向常量
	if DRUp != 0 || DRUpRight != 1 || DRRight != 2 || DRDownRight != 3 {
		t.Error("Direction constants mismatch")
	}
	if DRDown != 4 || DRDownLeft != 5 || DRLeft != 6 || DRUpLeft != 7 {
		t.Error("Direction constants mismatch")
	}

	// 网格常量
	if UnitX != 48 || UnitY != 32 {
		t.Error("Grid constants mismatch")
	}

	// 装备槽
	if UDress != 0 || UWeapon != 1 || URightHand != 2 {
		t.Error("Equipment slot constants mismatch")
	}
	if UNecklace != 3 || UHelmet != 4 {
		t.Error("Equipment slot constants mismatch (necklace/helmet)")
	}

	// 物品类型
	if ItemWeapon != 0 || ItemArmor != 1 || ItemAccessory != 2 {
		t.Error("Item type constants mismatch")
	}

	// 消息 ID
	if CMQueryChr != 100 || CMNewChr != 101 || CMDelChr != 102 {
		t.Error("CM message ID constants mismatch")
	}
	if CMSelChr != 103 || CMSelectServer != 104 {
		t.Error("CM message ID constants mismatch")
	}
	if SMLogon != 50 || SMNewMap != 51 || SMAbility != 52 {
		t.Error("SM message ID constants mismatch")
	}
}

// TestFrameFormatting 测试帧格式化函数。
func TestFrameFormatting(t *testing.T) {
	// 测试服务端帧
	serverFrame := FormatServerFrame("testdata")
	if serverFrame != "#testdata!" {
		t.Errorf("FormatServerFrame: got %q, want %q", serverFrame, "#testdata!")
	}

	// 测试客户端帧
	code := byte(1)
	clientFrame := FormatClientFrame("testdata", &code)
	if clientFrame != "#1testdata!" {
		t.Errorf("FormatClientFrame: got %q, want %q", clientFrame, "#1testdata!")
	}
	if code != 2 {
		t.Errorf("code should increment: got %d, want 2", code)
	}

	// 测试 code 回绕——code 自增到 10，下一次调用重置为 1
	code = 9
	FormatClientFrame("test", &code)
	if code != 10 {
		t.Errorf("code should be 10: got %d", code)
	}
	// 下一次调用应回绕
	FormatClientFrame("test2", &code)
	if code != 2 {
		t.Errorf("code should wrap to 2: got %d", code)
	}
}

// TestUserEntryRoundTrip 验证 Delphi 短字符串布局以及 CM_ADDNEWUSER
// 使用的两段 EncodeBuffer 切分（ClMain.pas:2844）。
func TestUserEntryRoundTrip(t *testing.T) {
	var ue UserEntry
	ue.SetAccount("tester")
	ue.SetPassword("secret")
	ue.SetUserName("Test User")
	ue.SetSSNo("123456-1234567")
	ue.SetPhone("010-1234-5678")
	ue.SetQuiz("quiz one")
	ue.SetAnswer("answer one")
	ue.SetEMail("test@example.com")

	var ua UserEntryAdd
	ua.SetQuiz2("quiz two")
	ua.SetAnswer2("answer two")
	ua.SetBirthDay("1990/01/31")
	ua.SetMobilePhone("13800138000")

	if got := len(ue.Bytes()); got != UserEntrySize {
		t.Fatalf("UserEntry wire size: got %d, want %d", got, UserEntrySize)
	}
	if got := len(ua.Bytes()); got != UserEntryAddSize {
		t.Fatalf("UserEntryAdd wire size: got %d, want %d", got, UserEntryAddSize)
	}

	// 客户端：两段独立编码的内容。
	raw := EncodeBuffer(ue.Bytes()) + EncodeBuffer(ua.Bytes())
	if len(raw) != UserEntryEncodedSize+UserEntryAddEncodedSize {
		t.Fatalf("encoded size: got %d, want %d", len(raw), UserEntryEncodedSize+UserEntryAddEncodedSize)
	}

	// 服务端：在固定边界处切分并分别解码
	//（一次性解码整个拼接内容会导致第二段错位）。
	ueBuf := make([]byte, UserEntrySize)
	DecodeBuffer(raw[:UserEntryEncodedSize], ueBuf)
	uaBuf := make([]byte, UserEntryAddSize)
	DecodeBuffer(raw[UserEntryEncodedSize:], uaBuf)

	gotUE := UserEntryFromBytes(ueBuf)
	gotUA := UserEntryAddFromBytes(uaBuf)

	if gotUE.Account() != "tester" || gotUE.Password() != "secret" {
		t.Errorf("account/password: got %q/%q", gotUE.Account(), gotUE.Password())
	}
	if string(gotUE.SUserName[1:10]) != "Test User" {
		t.Errorf("UserName: got %q", string(gotUE.SUserName[1:10]))
	}
	if string(gotUE.SQuiz[1:9]) != "quiz one" {
		t.Errorf("Quiz: got %q", string(gotUE.SQuiz[1:9]))
	}
	if string(gotUA.SBirthDay[1:11]) != "1990/01/31" {
		t.Errorf("BirthDay: got %q", string(gotUA.SBirthDay[1:11]))
	}
	if string(gotUA.SMobilePhone[1:12]) != "13800138000" {
		t.Errorf("MobilePhone: got %q", string(gotUA.SMobilePhone[1:12]))
	}
}

// TestUserEntryTruncation 验证字段会截断到其短字符串最大长度。
func TestUserEntryTruncation(t *testing.T) {
	var ue UserEntry
	ue.SetAccount("this-account-is-way-too-long")
	if got := ue.Account(); got != "this-accou" {
		t.Errorf("Account truncated: got %q, want %q", got, "this-accou")
	}
	var ua UserEntryAdd
	ua.SetMemo("memo")
	if got := string(ua.SMemo[1:5]); got != "memo" {
		t.Errorf("Memo: got %q, want %q", got, "memo")
	}
}

// TestEncodeMessageWithBody 测试带 body 的消息编码。
func TestEncodeMessageWithBody(t *testing.T) {
	msg := MakeDefaultMsg(CMIDPassword, 0, 0, 0, 0)
	body := EncodeString("testuser/testpass")

	result := EncodeMessageWithBody(msg, body)

	// 应为 DefBlockSize + len(body)
	if len(result) != DefBlockSize+len(body) {
		t.Errorf("unexpected length: got %d, want %d", len(result), DefBlockSize+len(body))
	}
}
