package netserver

import (
	"testing"
	"time"
)

func TestAllowMessageTokenBucket(t *testing.T) {
	s := NewTCPServer(":0")
	s.SetMsgRateLimit(10, 3) // 10 条/秒，突发 3
	session := &Session{ID: 1}

	// 突发容量内全部放行
	for i := 0; i < 3; i++ {
		if !s.allowMessage(session) {
			t.Fatalf("第 %d 条在突发容量内应放行", i+1)
		}
	}
	// 突发耗尽后立即拒绝
	if s.allowMessage(session) {
		t.Fatal("突发容量耗尽后应拒绝")
	}
	// 100ms 后补充 1 个令牌（10 条/秒）
	time.Sleep(110 * time.Millisecond)
	if !s.allowMessage(session) {
		t.Fatal("令牌补充后应放行")
	}
}

func TestAllowMessageDisabled(t *testing.T) {
	s := NewTCPServer(":0") // msgRate=0 默认不限流
	session := &Session{ID: 1}
	for i := 0; i < 1000; i++ {
		if !s.allowMessage(session) {
			t.Fatal("未配置限流时应全部放行")
		}
	}
}

func TestSessionSeqCheck(t *testing.T) {
	session := &Session{ID: 1}
	check := func(code byte) {
		if session.hasLastCode && code == session.lastCode {
			session.SeqErrCount++
		}
		session.lastCode = code
		session.hasLastCode = true
	}
	check('1')
	check('2')
	check('3')
	if session.SeqErrCount != 0 {
		t.Fatal("递增序号不应计违规")
	}
	check('3') // 重复
	if session.SeqErrCount != 1 {
		t.Fatalf("重复序号应计 1 次违规，实际 %d", session.SeqErrCount)
	}
}
