// Package protocol implements the MIR2 network protocol encoding/decoding.
// This is a faithful port of the Delphi Common/EDcode.pas 6Bit encoding algorithm.
package protocol

import (
	"encoding/binary"
	"strings"
)

const (
	// Offset constant for 6Bit encoding
	encodeOffset = 0x3C // 60, character '<'

	// BufferSize is the maximum buffer size for encoding/decoding
	BufferSize = 10000

	// DefBlockSize is the encoded size of a TDefaultMessage
	DefBlockSize = 16
)

// Encode6BitBuf encodes source bytes into 6Bit encoded bytes.
// Each 3 input bytes produce 4 output characters.
// This is a faithful port of the Delphi Encode6BitBuf procedure.
func Encode6BitBuf(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	// Pre-calculate output size: ceil(len * 4 / 3)
	dstLen := (len(src)*4 + 2) / 3
	dst := make([]byte, dstLen)

	nRestCount := 0
	btRest := byte(0)
	nDestPos := 0

	for i := 0; i < len(src); i++ {
		if nDestPos >= dstLen {
			break
		}
		btCh := src[i]

		btMade := (btRest | (btCh >> (2 + uint(nRestCount)))) & 0x3F
		btRest = ((btCh << (8 - (2 + uint(nRestCount)))) >> 2) & 0x3F
		nRestCount += 2

		if nRestCount < 6 {
			dst[nDestPos] = btMade + encodeOffset
			nDestPos++
		} else {
			if nDestPos < dstLen-1 {
				dst[nDestPos] = btMade + encodeOffset
				dst[nDestPos+1] = btRest + encodeOffset
				nDestPos += 2
			} else {
				dst[nDestPos] = btMade + encodeOffset
				nDestPos++
			}
			nRestCount = 0
			btRest = 0
		}
	}

	if nRestCount > 0 {
		if nDestPos < dstLen {
			dst[nDestPos] = btRest + encodeOffset
			nDestPos++
		}
	}

	return dst[:nDestPos]
}

// Decode6BitBuf decodes 6Bit encoded bytes back to original bytes.
// This is a faithful port of the Delphi Decode6BitBuf procedure.
func Decode6BitBuf(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	// Masks for bit extraction at different positions
	masks := [7]byte{0, 0, 0xFC, 0xF8, 0xF0, 0xE0, 0xC0}

	dst := make([]byte, len(src)) // Output will be smaller, but allocate max
	nBitPos := 2
	nMadeBit := 0
	nBufPos := 0
	btTmp := byte(0)

	for i := 0; i < len(src); i++ {
		if int(src[i])-encodeOffset < 0 {
			nBufPos = 0
			break
		}
		btCh := src[i] - encodeOffset

		if nBufPos >= len(dst) {
			break
		}

		if (nMadeBit + 6) >= 8 {
			btByte := btTmp | ((btCh & 0x3F) >> (6 - uint(nBitPos)))
			dst[nBufPos] = btByte
			nBufPos++
			nMadeBit = 0

			if nBitPos < 6 {
				nBitPos += 2
			} else {
				nBitPos = 2
				continue
			}
		}

		btTmp = (btCh << uint(nBitPos)) & masks[nBitPos]
		nMadeBit += 8 - nBitPos
	}

	return dst[:nBufPos]
}

// EncodeMessage encodes a TDefaultMessage into a 6Bit encoded string.
// The message is 12 bytes, encoded to 16 characters.
func EncodeMessage(msg DefaultMessage) string {
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(msg.Recog))
	binary.LittleEndian.PutUint16(buf[4:6], msg.Ident)
	binary.LittleEndian.PutUint16(buf[6:8], msg.Param)
	binary.LittleEndian.PutUint16(buf[8:10], msg.Tag)
	binary.LittleEndian.PutUint16(buf[10:12], msg.Series)

	encoded := Encode6BitBuf(buf)
	return string(encoded)
}

// DecodeMessage decodes a 6Bit encoded string into a TDefaultMessage.
func DecodeMessage(str string) DefaultMessage {
	var msg DefaultMessage

	if len(str) < DefBlockSize {
		return msg
	}

	decoded := Decode6BitBuf([]byte(str))
	if len(decoded) < 12 {
		return msg
	}

	msg.Recog = int32(binary.LittleEndian.Uint32(decoded[0:4]))
	msg.Ident = binary.LittleEndian.Uint16(decoded[4:6])
	msg.Param = binary.LittleEndian.Uint16(decoded[6:8])
	msg.Tag = binary.LittleEndian.Uint16(decoded[8:10])
	msg.Series = binary.LittleEndian.Uint16(decoded[10:12])

	return msg
}

// EncodeString encodes a string into a 6Bit encoded string.
func EncodeString(str string) string {
	if str == "" {
		return ""
	}
	encoded := Encode6BitBuf([]byte(str))
	return string(encoded)
}

// DecodeString decodes a 6Bit encoded string back to the original string.
func DecodeString(str string) string {
	if str == "" {
		return ""
	}
	decoded := Decode6BitBuf([]byte(str))
	return string(decoded)
}

// EncodeBuffer encodes a byte buffer into a 6Bit encoded string.
func EncodeBuffer(buf []byte) string {
	if len(buf) == 0 || len(buf) >= BufferSize {
		return ""
	}
	encoded := Encode6BitBuf(buf)
	return string(encoded)
}

// DecodeBuffer decodes a 6Bit encoded string into a byte buffer.
func DecodeBuffer(str string, buf []byte) {
	if str == "" || len(buf) == 0 {
		return
	}
	decoded := Decode6BitBuf([]byte(str))
	copy(buf, decoded)
}

// MakeDefaultMsg creates a TDefaultMessage with the given parameters.
func MakeDefaultMsg(ident uint16, recog int32, param, tag, series uint16) DefaultMessage {
	return DefaultMessage{
		Recog:  recog,
		Ident:  ident,
		Param:  param,
		Tag:    tag,
		Series: series,
	}
}

// GetCodeMsgSize returns the encoded size for a given raw size.
// Formula: ceil(n * 4 / 3)
func GetCodeMsgSize(n int) int {
	return (n*4 + 2) / 3
}

// EncodeMessageWithBody encodes a message with an optional body string.
// This is the standard frame format: EncodeMessage + body
func EncodeMessageWithBody(msg DefaultMessage, body string) string {
	var sb strings.Builder
	sb.WriteString(EncodeMessage(msg))
	if body != "" {
		sb.WriteString(body)
	}
	return sb.String()
}

// FormatClientFrame formats a client-to-server message frame.
// Format: #<code><payload>!
func FormatClientFrame(payload string, code *byte) string {
	if *code >= 10 {
		*code = 1
	}
	frame := "#" + string('0'+*code) + payload + "!"
	*code++
	return frame
}

// FormatServerFrame formats a server-to-client message frame.
// Format: #<payload>!
func FormatServerFrame(payload string) string {
	return "#" + payload + "!"
}
