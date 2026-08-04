package netserver

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// bufConn 非阻塞内存连接：Write 永不阻塞，供 writeLoop 背压测试使用。
type bufConn struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *bufConn) Read(b []byte) (int, error)       { return 0, io.EOF }
func (c *bufConn) Close() error                     { return nil }
func (c *bufConn) LocalAddr() net.Addr              { return nil }
func (c *bufConn) RemoteAddr() net.Addr             { return nil }
func (c *bufConn) SetDeadline(time.Time) error      { return nil }
func (c *bufConn) SetReadDeadline(time.Time) error  { return nil }
func (c *bufConn) SetWriteDeadline(time.Time) error { return nil }
func (c *bufConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(b)
}

func (c *bufConn) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Len()
}

func (c *bufConn) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, c.buf.Len())
	copy(out, c.buf.Bytes())
	return out
}

func newTestSession(conn net.Conn) *Session {
	return &Session{
		ID:        1,
		Conn:      conn,
		SendChan:  make(chan []byte, 256),
		resumeSig: make(chan struct{}, 1),
	}
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("等待 %s 超时", what)
}

// 累计发送 >=512B 未回显时，下一发送块前应插入 '*' 探针（Main.pas:529-533）。
func TestBackpressureProbe(t *testing.T) {
	conn := &bufConn{}
	session := newTestSession(conn)
	server := &TCPServer{done: make(chan struct{})}
	go server.writeLoop(session)

	for i := 0; i < 60; i++ {
		session.SendChan <- bytes.Repeat([]byte{'a'}, 10)
	}
	waitFor(t, 2*time.Second, "60 帧写完", func() bool { return conn.len() >= 601 })

	data := conn.bytes()
	if len(data) != 601 {
		t.Fatalf("写入总字节应为 601（600 数据 + 1 探针），实际 %d", len(data))
	}
	if n := bytes.Count(data, []byte{'*'}); n != 1 {
		t.Fatalf("应恰好插入 1 个 '*' 探针，实际 %d", n)
	}
}

// >=2048B 未回显暂停发送，超时（50ms）后恢复（Main.pas:520-527,534-537）。
// 旧实现为无条件 sleep 3 秒，此测试同时防止回归。
func TestBackpressureTimeoutResume(t *testing.T) {
	conn := &bufConn{}
	session := newTestSession(conn)
	server := &TCPServer{done: make(chan struct{})}
	go server.writeLoop(session)

	big := bytes.Repeat([]byte{'b'}, sendCheckSizeMax+10)
	session.SendChan <- big
	session.SendChan <- []byte("tail")

	start := time.Now()
	total := len(big) + 4
	waitFor(t, 2*time.Second, "背压超时恢复", func() bool { return conn.len() >= total })
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond {
		t.Fatalf("应等待背压超时（~50ms）才发送后续数据，实际只用了 %v", elapsed)
	}
	if elapsed > time.Second {
		t.Fatalf("背压恢复耗时过长：%v（旧实现为 3 秒硬睡眠）", elapsed)
	}
	if session.pauseUntil.Load() != 0 {
		t.Fatal("恢复后暂停状态应清零")
	}
	// 恢复清零后 tail（4B）正常累计
	if n := atomic.LoadInt64(&session.unackedBytes); n != int64(len("tail")) {
		t.Fatalf("恢复后计数应仅含 tail 的 %d 字节，实际 %d", len("tail"), n)
	}
}

// 收到客户端 '*' 回显后立即恢复发送，无需等待超时（Main.pas:1010-1016）。
func TestBackpressureEchoResume(t *testing.T) {
	conn := &bufConn{}
	session := newTestSession(conn)
	server := &TCPServer{done: make(chan struct{})}
	go server.writeLoop(session)

	big := bytes.Repeat([]byte{'c'}, sendCheckSizeMax+10)
	session.SendChan <- big
	waitFor(t, 2*time.Second, "大帧写完", func() bool { return conn.len() >= len(big) })

	session.resumeSend() // 模拟 readLoop keepalive 收到 '*' 回显
	start := time.Now()
	session.SendChan <- []byte("tail")
	waitFor(t, 2*time.Second, "回显恢复后写入 tail", func() bool { return conn.len() >= len(big)+4 })
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("回显后应立即恢复发送，实际耗时 %v", elapsed)
	}
}

// readLoop 收到客户端 '*' 回显 → 背压计数/探针/暂停状态全部清零。
func TestReadLoopEchoResetsBackpressure(t *testing.T) {
	client, serverSide := net.Pipe()
	defer client.Close()
	session := newTestSession(serverSide)
	server := &TCPServer{
		done:     make(chan struct{}),
		sessions: make(map[int64]*Session),
	}
	server.wg.Add(1)
	go server.readLoop(session)

	atomic.StoreInt64(&session.unackedBytes, 3000)
	session.probeSent.Store(true)
	session.pauseUntil.Store(time.Now().UnixMilli() + 5000)

	if _, err := client.Write([]byte{'*'}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, "回显重置背压状态", func() bool {
		return atomic.LoadInt64(&session.unackedBytes) == 0 &&
			!session.probeSent.Load() && session.pauseUntil.Load() == 0
	})
}
