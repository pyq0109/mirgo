// Package protocol 实现 MIR2 网络协议的编解码。
// 忠实移植自 Delphi Common/EDcode.pas 的 6Bit 编解码算法。
package protocol

import (
	"encoding/binary"
	"strings"
)

const (
	// 6Bit 编码的偏移常量
	encodeOffset = 0x3C // 60，字符 '<'

	// BufferSize 是编解码的最大缓冲区大小
	BufferSize = 10000

	// DefBlockSize 是 TDefaultMessage 编码后的大小
	DefBlockSize = 16
)

// Encode6BitBuf 将源字节编码为 6Bit 编码字节。
// 每 3 个输入字节产生 4 个输出字符。
// 忠实移植自 Delphi 的 Encode6BitBuf 过程。
func Encode6BitBuf(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	// 预先计算输出大小：ceil(len * 4 / 3)
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

// Decode6BitBuf 将 6Bit 编码字节解码回原始字节。
// 忠实移植自 Delphi 的 Decode6BitBuf 过程。
func Decode6BitBuf(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	// 不同位置的位提取掩码
	masks := [7]byte{0, 0, 0xFC, 0xF8, 0xF0, 0xE0, 0xC0}

	dst := make([]byte, len(src)) // 输出会更小，但按最大值分配
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

// EncodeMessage 将 TDefaultMessage 编码为 6Bit 编码字符串。
// 消息为 12 字节，编码后为 16 个字符。
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

// DecodeMessage 将 6Bit 编码字符串解码为 TDefaultMessage。
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

// EncodeString 将字符串编码为 6Bit 编码字符串。
func EncodeString(str string) string {
	if str == "" {
		return ""
	}
	encoded := Encode6BitBuf([]byte(str))
	return string(encoded)
}

// DecodeString 将 6Bit 编码字符串解码回原始字符串。
func DecodeString(str string) string {
	if str == "" {
		return ""
	}
	decoded := Decode6BitBuf([]byte(str))
	return string(decoded)
}

// EncodeBuffer 将字节缓冲区编码为 6Bit 编码字符串。
func EncodeBuffer(buf []byte) string {
	if len(buf) == 0 || len(buf) >= BufferSize {
		return ""
	}
	encoded := Encode6BitBuf(buf)
	return string(encoded)
}

// DecodeBuffer 将 6Bit 编码字符串解码到字节缓冲区。
func DecodeBuffer(str string, buf []byte) {
	if str == "" || len(buf) == 0 {
		return
	}
	decoded := Decode6BitBuf([]byte(str))
	copy(buf, decoded)
}

// ClientItemSize 是 ClientItem 二进制编码的字节数。
// StdItem 65 字节（Name[20] + 7 字节字段 + Looks u16 + 9 个 u32）
// + MakeIndex i32 + Dura u16 + DuraMax u16。
// 对应 Delphi TClientItem（Grobal2.pas:563-568）。
const ClientItemSize = 73

// EncodeClientItem 将 ClientItem 编码为小端二进制。
// 用于 SM_SENDDETAILGOODSLIST 详细商品列表条目。
func EncodeClientItem(item *ClientItem) []byte {
	buf := make([]byte, ClientItemSize)
	copy(buf[0:20], item.S.Name[:])
	buf[20] = item.S.StdMode
	buf[21] = item.S.Shape
	buf[22] = item.S.Weight
	buf[23] = item.S.AniCount
	buf[24] = byte(item.S.Source)
	buf[25] = item.S.Reserved
	buf[26] = item.S.NeedIdentify
	binary.LittleEndian.PutUint16(buf[27:29], item.S.Looks)
	binary.LittleEndian.PutUint32(buf[29:33], item.S.DuraMax)
	binary.LittleEndian.PutUint32(buf[33:37], item.S.AC)
	binary.LittleEndian.PutUint32(buf[37:41], item.S.MAC)
	binary.LittleEndian.PutUint32(buf[41:45], item.S.DC)
	binary.LittleEndian.PutUint32(buf[45:49], item.S.MC)
	binary.LittleEndian.PutUint32(buf[49:53], item.S.SC)
	binary.LittleEndian.PutUint32(buf[53:57], item.S.Need)
	binary.LittleEndian.PutUint32(buf[57:61], item.S.NeedLevel)
	binary.LittleEndian.PutUint32(buf[61:65], item.S.Price)
	binary.LittleEndian.PutUint32(buf[65:69], uint32(item.MakeIndex))
	binary.LittleEndian.PutUint16(buf[69:71], item.Dura)
	binary.LittleEndian.PutUint16(buf[71:73], item.DuraMax)
	return buf
}

// DecodeClientItem 将小端二进制解码为 ClientItem。
// 数据不足 ClientItemSize 时返回 ok=false。
func DecodeClientItem(buf []byte) (item ClientItem, ok bool) {
	if len(buf) < ClientItemSize {
		return item, false
	}
	copy(item.S.Name[:], buf[0:20])
	item.S.StdMode = buf[20]
	item.S.Shape = buf[21]
	item.S.Weight = buf[22]
	item.S.AniCount = buf[23]
	item.S.Source = int8(buf[24])
	item.S.Reserved = buf[25]
	item.S.NeedIdentify = buf[26]
	item.S.Looks = binary.LittleEndian.Uint16(buf[27:29])
	item.S.DuraMax = binary.LittleEndian.Uint32(buf[29:33])
	item.S.AC = binary.LittleEndian.Uint32(buf[33:37])
	item.S.MAC = binary.LittleEndian.Uint32(buf[37:41])
	item.S.DC = binary.LittleEndian.Uint32(buf[41:45])
	item.S.MC = binary.LittleEndian.Uint32(buf[45:49])
	item.S.SC = binary.LittleEndian.Uint32(buf[49:53])
	item.S.Need = binary.LittleEndian.Uint32(buf[53:57])
	item.S.NeedLevel = binary.LittleEndian.Uint32(buf[57:61])
	item.S.Price = binary.LittleEndian.Uint32(buf[61:65])
	item.MakeIndex = int32(binary.LittleEndian.Uint32(buf[65:69]))
	item.Dura = binary.LittleEndian.Uint16(buf[69:71])
	item.DuraMax = binary.LittleEndian.Uint16(buf[71:73])
	return item, true
}

// MakeDefaultMsg 用给定参数创建一个 TDefaultMessage。
func MakeDefaultMsg(ident uint16, recog int32, param, tag, series uint16) DefaultMessage {
	return DefaultMessage{
		Recog:  recog,
		Ident:  ident,
		Param:  param,
		Tag:    tag,
		Series: series,
	}
}

// GetCodeMsgSize 返回给定原始大小编码后的大小。
// 公式：ceil(n * 4 / 3)
func GetCodeMsgSize(n int) int {
	return (n*4 + 2) / 3
}

// EncodeMessageWithBody 编码消息并附带可选的 body 字符串。
// 这是标准帧格式：EncodeMessage + body
func EncodeMessageWithBody(msg DefaultMessage, body string) string {
	var sb strings.Builder
	sb.WriteString(EncodeMessage(msg))
	if body != "" {
		sb.WriteString(body)
	}
	return sb.String()
}

// FormatClientFrame 格式化客户端到服务端的消息帧。
// 格式：#<code><payload>!
func FormatClientFrame(payload string, code *byte) string {
	if *code >= 10 {
		*code = 1
	}
	frame := "#" + string('0'+*code) + payload + "!"
	*code++
	return frame
}

// FormatServerFrame 格式化服务端到客户端的消息帧。
// 格式：#<payload>!
func FormatServerFrame(payload string) string {
	return "#" + payload + "!"
}
