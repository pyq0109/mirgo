package protocol

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// TestEncode6BitBufRoundTrip tests that encoding then decoding returns the original data.
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

// TestEncodeMessageRoundTrip tests message encoding/decoding round trip.
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

// TestEncodeStringRoundTrip tests string encoding/decoding round trip.
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

// TestEncodeBufferRoundTrip tests buffer encoding/decoding round trip.
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

// TestMakeDefaultMsg tests message creation.
func TestMakeDefaultMsg(t *testing.T) {
	msg := MakeDefaultMsg(CMIDPassword, 0, 0, 0, 0)

	if msg.Ident != CMIDPassword {
		t.Errorf("Ident: got %d, want %d", msg.Ident, CMIDPassword)
	}
	if msg.Recog != 0 {
		t.Errorf("Recog: got %d, want %d", msg.Recog, 0)
	}
}

// TestGetCodeMsgSize tests the encoded size calculation.
func TestGetCodeMsgSize(t *testing.T) {
	testCases := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 2},   // ceil(1 * 4 / 3) = 2
		{2, 3},   // ceil(2 * 4 / 3) = 3
		{3, 4},   // ceil(3 * 4 / 3) = 4
		{12, 16}, // TDefaultMessage size
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

// TestDefaultMessageSize verifies the DefaultMessage struct size.
func TestDefaultMessageSize(t *testing.T) {
	// DefaultMessage should be 12 bytes (4+2+2+2+2)
	var msg DefaultMessage
	size := unsafe.Sizeof(msg)
	if size != 12 {
		t.Errorf("DefaultMessage size: got %d, want 12", size)
	}

	// Verify binary encoding size
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(msg.Recog))
	binary.LittleEndian.PutUint16(buf[4:6], msg.Ident)
	binary.LittleEndian.PutUint16(buf[6:8], msg.Param)
	binary.LittleEndian.PutUint16(buf[8:10], msg.Tag)
	binary.LittleEndian.PutUint16(buf[10:12], msg.Series)
}

// TestFeatureEncoding tests the feature encoding/decoding functions.
func TestFeatureEncoding(t *testing.T) {
	// Test human feature
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

	// Test monster feature
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

// TestConstants verifies key constants match the Delphi source.
func TestConstants(t *testing.T) {
	// Direction constants
	if DRUp != 0 || DRUpRight != 1 || DRRight != 2 || DRDownRight != 3 {
		t.Error("Direction constants mismatch")
	}
	if DRDown != 4 || DRDownLeft != 5 || DRLeft != 6 || DRUpLeft != 7 {
		t.Error("Direction constants mismatch")
	}

	// Grid constants
	if UnitX != 48 || UnitY != 32 {
		t.Error("Grid constants mismatch")
	}

	// Equipment slots
	if UDress != 0 || UWeapon != 1 || URightHand != 2 {
		t.Error("Equipment slot constants mismatch")
	}
	if UNecklace != 3 || UHelmet != 4 {
		t.Error("Equipment slot constants mismatch (necklace/helmet)")
	}

	// Item types
	if ItemWeapon != 0 || ItemArmor != 1 || ItemAccessory != 2 {
		t.Error("Item type constants mismatch")
	}

	// Message IDs
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

// TestFrameFormatting tests the frame formatting functions.
func TestFrameFormatting(t *testing.T) {
	// Test server frame
	serverFrame := FormatServerFrame("testdata")
	if serverFrame != "#testdata!" {
		t.Errorf("FormatServerFrame: got %q, want %q", serverFrame, "#testdata!")
	}

	// Test client frame
	code := byte(1)
	clientFrame := FormatClientFrame("testdata", &code)
	if clientFrame != "#1testdata!" {
		t.Errorf("FormatClientFrame: got %q, want %q", clientFrame, "#1testdata!")
	}
	if code != 2 {
		t.Errorf("code should increment: got %d, want 2", code)
	}

	// Test code wrap around - code increments to 10, then next call resets to 1
	code = 9
	FormatClientFrame("test", &code)
	if code != 10 {
		t.Errorf("code should be 10: got %d", code)
	}
	// Next call should wrap
	FormatClientFrame("test2", &code)
	if code != 2 {
		t.Errorf("code should wrap to 2: got %d", code)
	}
}

// TestEncodeMessageWithBody tests message encoding with body.
func TestEncodeMessageWithBody(t *testing.T) {
	msg := MakeDefaultMsg(CMIDPassword, 0, 0, 0, 0)
	body := EncodeString("testuser/testpass")

	result := EncodeMessageWithBody(msg, body)

	// Should be DefBlockSize + len(body)
	if len(result) != DefBlockSize+len(body) {
		t.Errorf("unexpected length: got %d, want %d", len(result), DefBlockSize+len(body))
	}
}
