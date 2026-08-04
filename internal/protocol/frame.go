package protocol

// FrameScanner 从 TCP 字节流中提取 #...! 帧。
// 客户端和服务端共用，避免帧扫描逻辑重复。
type FrameScanner struct {
	buf []byte
	// OnCode 在剥离客户端帧首 code 数字时回调（Delphi RunGate
	// 序号校验入口，Main.pas:363-413）。nil 表示不关心序号。
	OnCode func(code byte)
}

// MaxRecvBuf 是接收缓冲区的最大容量，超出则应断开连接。
const MaxRecvBuf = 64 * 1024

// Feed 追加新收到的数据，返回所有完整帧的 payload（# 和 ! 之间的内容）。
// stripCode 为 true 时剥离帧首字节的 code 数字（客户端帧带 code，服务端帧不带）。
// keepalive 非 nil 时，数据流中的 '*' 字节会被剥离并通过回调通知（调用方负责回显）。
// 如果累积数据超过 MaxRecvBuf，返回 overflow=true。
func (fs *FrameScanner) Feed(data []byte, stripCode bool, keepalive func()) (payloads []string, overflow bool) {
	fs.buf = append(fs.buf, data...)
	if len(fs.buf) > MaxRecvBuf {
		return nil, true
	}

	// 剥离 '*' keepalive 字节
	if keepalive != nil {
		w := 0
		for _, b := range fs.buf {
			if b == '*' {
				keepalive()
				continue
			}
			fs.buf[w] = b
			w++
		}
		fs.buf = fs.buf[:w]
	}

	// 提取所有完整帧
	for len(fs.buf) > 2 {
		if fs.buf[0] != '#' {
			fs.buf = fs.buf[1:] // 跳过噪声，在下一个 '#' 处重新同步
			continue
		}
		endIdx := -1
		for i := 1; i < len(fs.buf); i++ {
			if fs.buf[i] == '!' {
				endIdx = i
				break
			}
		}
		if endIdx < 0 {
			break // 帧不完整，等待更多数据
		}

		frame := fs.buf[1:endIdx]
		fs.buf = fs.buf[endIdx+1:]

		// 可选剥离 code 数字
		start := 0
		if stripCode && len(frame) > 0 && frame[0] >= '0' && frame[0] <= '9' {
			if fs.OnCode != nil {
				fs.OnCode(frame[0])
			}
			start = 1
		}
		if start < len(frame) {
			payloads = append(payloads, string(frame[start:]))
		}
	}

	// 压缩：把未消费的数据移到 buf 头部
	if len(fs.buf) > 0 && len(fs.buf) < cap(fs.buf)/2 {
		compact := make([]byte, len(fs.buf))
		copy(compact, fs.buf)
		fs.buf = compact
	}

	return payloads, false
}

// Pending 返回尚未消费的字节数（不完整帧）。
func (fs *FrameScanner) Pending() int {
	return len(fs.buf)
}

// Reset 清空缓冲区。
func (fs *FrameScanner) Reset() {
	fs.buf = fs.buf[:0]
}
