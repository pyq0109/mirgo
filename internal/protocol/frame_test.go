package protocol

import (
	"testing"
)

func TestFrameScannerBasic(t *testing.T) {
	var fs FrameScanner

	// 单帧
	payloads, overflow := fs.Feed([]byte("#hello!"), false, nil)
	if overflow {
		t.Fatal("unexpected overflow")
	}
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
	if fs.Pending() != 0 {
		t.Fatalf("pending = %d, want 0", fs.Pending())
	}
}

func TestFrameScannerMultipleFrames(t *testing.T) {
	var fs FrameScanner

	payloads, _ := fs.Feed([]byte("#aaa!#bbb!#ccc!"), false, nil)
	if len(payloads) != 3 {
		t.Fatalf("got %d payloads, want 3: %v", len(payloads), payloads)
	}
	want := []string{"aaa", "bbb", "ccc"}
	for i, w := range want {
		if payloads[i] != w {
			t.Errorf("payloads[%d] = %q, want %q", i, payloads[i], w)
		}
	}
}

func TestFrameScannerPartialFrame(t *testing.T) {
	var fs FrameScanner

	// 不完整帧 — 无 '!'
	payloads, _ := fs.Feed([]byte("#hel"), false, nil)
	if len(payloads) != 0 {
		t.Fatalf("got %v, want empty", payloads)
	}
	if fs.Pending() != 4 {
		t.Fatalf("pending = %d, want 4", fs.Pending())
	}

	// 补全帧
	payloads, _ = fs.Feed([]byte("lo!"), false, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
}

func TestFrameScannerNoisePrefix(t *testing.T) {
	var fs FrameScanner

	// 帧前有噪声字节
	payloads, _ := fs.Feed([]byte("xyz#hello!"), false, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
}

func TestFrameScannerStripCode(t *testing.T) {
	var fs FrameScanner

	// 客户端帧带 code 数字
	payloads, _ := fs.Feed([]byte("#3hello!"), true, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}

	// 无 code 数字（服务端帧）
	payloads, _ = fs.Feed([]byte("#hello!"), true, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
}

func TestFrameScannerKeepalive(t *testing.T) {
	var fs FrameScanner
	keepaliveCount := 0

	// '*' 应被剥离并触发回调
	payloads, _ := fs.Feed([]byte("*#hello!*#world!"), false, func() {
		keepaliveCount++
	})
	if keepaliveCount != 2 {
		t.Fatalf("keepalive called %d times, want 2", keepaliveCount)
	}
	if len(payloads) != 2 {
		t.Fatalf("got %d payloads, want 2: %v", len(payloads), payloads)
	}
	if payloads[0] != "hello" || payloads[1] != "world" {
		t.Fatalf("got %v, want [hello world]", payloads)
	}
}

func TestFrameScannerEmptyPayload(t *testing.T) {
	var fs FrameScanner

	// 空帧（## 之间无内容）不产生 payload
	payloads, _ := fs.Feed([]byte("#!#hello!"), false, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
}

func TestFrameScannerOverflow(t *testing.T) {
	var fs FrameScanner

	// 超过 MaxRecvBuf
	big := make([]byte, MaxRecvBuf+1)
	for i := range big {
		big[i] = 'A'
	}
	_, overflow := fs.Feed(big, false, nil)
	if !overflow {
		t.Fatal("expected overflow")
	}
}

func TestFrameScannerReset(t *testing.T) {
	var fs FrameScanner

	fs.Feed([]byte("#partial"), false, nil)
	if fs.Pending() == 0 {
		t.Fatal("expected pending data")
	}
	fs.Reset()
	if fs.Pending() != 0 {
		t.Fatalf("pending after reset = %d, want 0", fs.Pending())
	}
}

func TestFrameScannerSplitAcrossFeeds(t *testing.T) {
	var fs FrameScanner

	// 帧跨多次 Feed
	payloads, _ := fs.Feed([]byte("#hel"), false, nil)
	if len(payloads) != 0 {
		t.Fatalf("got %v, want empty", payloads)
	}
	payloads, _ = fs.Feed([]byte("lo"), false, nil)
	if len(payloads) != 0 {
		t.Fatalf("got %v, want empty", payloads)
	}
	payloads, _ = fs.Feed([]byte("!"), false, nil)
	if len(payloads) != 1 || payloads[0] != "hello" {
		t.Fatalf("got %v, want [hello]", payloads)
	}
}

func TestFrameScannerCodeDigitOnly(t *testing.T) {
	var fs FrameScanner

	// 帧只有 code 数字，无实际 payload → 不产生结果
	payloads, _ := fs.Feed([]byte("#3!"), true, nil)
	if len(payloads) != 0 {
		t.Fatalf("got %v, want empty", payloads)
	}
}

func TestFrameScannerOnCode(t *testing.T) {
	var fs FrameScanner
	var codes []byte
	fs.OnCode = func(code byte) {
		codes = append(codes, code)
	}

	// 三个带 code 的帧
	fs.Feed([]byte("#1abc!#2def!#9ghi!"), true, nil)
	if string(codes) != "129" {
		t.Fatalf("OnCode got %q, want %q", codes, "129")
	}

	// 不带 code 的帧（服务端帧格式）不回调
	codes = nil
	fs.Feed([]byte("#xyz!"), true, nil)
	if len(codes) != 0 {
		t.Fatalf("OnCode got %v, want none", codes)
	}

	// stripCode=false 时不剥离也不回调
	codes = nil
	fs.Feed([]byte("#5uvw!"), false, nil)
	if len(codes) != 0 {
		t.Fatalf("OnCode with stripCode=false got %v, want none", codes)
	}
}
