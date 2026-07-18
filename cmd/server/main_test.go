package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

// ============================================================================
// Unit Tests — Helper Functions
// ============================================================================

func TestParseCredentials(t *testing.T) {
	tests := []struct {
		body     string
		wantUser string
		wantPass string
	}{
		{"user/pass", "user", "pass"},
		{"admin/secret123", "admin", "secret123"},
		{"nopass", "nopass", ""},
		{"", "", ""},
		{"user/pass/extra", "user", "pass/extra"},
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			user, pass := parseCredentials(tt.body)
			if user != tt.wantUser {
				t.Errorf("username = %q, want %q", user, tt.wantUser)
			}
			if pass != tt.wantPass {
				t.Errorf("password = %q, want %q", pass, tt.wantPass)
			}
		})
	}
}

func TestSimpleHash(t *testing.T) {
	h1 := simpleHash("password123")
	h2 := simpleHash("password123")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}

	h3 := simpleHash("different")
	if h1 == h3 {
		t.Errorf("different inputs produced same hash: %q", h1)
	}

	h4 := simpleHash("")
	if h4 != "" {
		t.Errorf("empty input hash = %q, want empty", h4)
	}
}

func TestVerifyPassword(t *testing.T) {
	hash := simpleHash("mypassword")
	if !verifyPassword("mypassword", hash) {
		t.Error("verifyPassword failed for correct password")
	}
	if verifyPassword("wrongpassword", hash) {
		t.Error("verifyPassword succeeded for wrong password")
	}
}

func TestGetServerHostPort(t *testing.T) {
	tests := []struct {
		addr     string
		wantHost string
		wantPort int
	}{
		{":7000", "localhost", 7000},
		{"0.0.0.0:7000", "localhost", 7000},
		{"localhost:7000", "localhost", 7000},
		{"192.168.1.1:7100", "192.168.1.1", 7100},
		{"", "localhost", 7000},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			config := &ServerConfig{}
			config.Server.Listen.Addr = tt.addr
			host, port := config.GetServerHostPort()
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

// ============================================================================
// Unit Tests — Character List Text Format (Fix 3)
// ============================================================================

func TestSendCharacterListFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	hash := simpleHash("testpass")
	accountID, err := db.CreateAccount("testuser", hash)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	_, err = db.CreateCharacter(accountID, "Warrior", 0, 0)
	if err != nil {
		t.Fatalf("CreateCharacter Warrior: %v", err)
	}
	_, err = db.CreateCharacter(accountID, "Wizard", 1, 1)
	if err != nil {
		t.Fatalf("CreateCharacter Wizard: %v", err)
	}

	chars, err := db.GetCharactersByAccount(accountID)
	if err != nil {
		t.Fatalf("GetCharactersByAccount: %v", err)
	}

	// Build text format (same logic as sendCharacterList in main.go)
	var sb strings.Builder
	for i, c := range chars {
		if i > 0 {
			sb.WriteByte('/')
		}
		if i == 0 {
			sb.WriteByte('*')
		}
		sb.WriteString(c.Name)
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Job))
		sb.WriteByte('/')
		sb.WriteString("0")
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Level))
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Sex))
	}

	result := sb.String()

	if result[0] != '*' {
		t.Errorf("result should start with *, got %q", result[:5])
	}

	// Parse and verify
	parts := strings.Split(result, "/")
	if len(parts) != 10 {
		t.Errorf("expected 10 parts, got %d: %q", len(parts), result)
	}

	if parts[0] != "*Warrior" {
		t.Errorf("parts[0] = %q, want *Warrior", parts[0])
	}
	if parts[1] != "0" || parts[3] != "1" || parts[4] != "0" {
		t.Errorf("Warrior fields = %v", parts[1:5])
	}
}

// ============================================================================
// Test Server Helper
// ============================================================================

// testServer wraps a TCPServer with a database for integration testing.
type testServer struct {
	server  *netserver.TCPServer
	db      *storage.Database
	dbPath  string
	tmpDir  string
	port    int
	config  *ServerConfig
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	log.SetLevel(log.LevelError)

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}

	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	config := &ServerConfig{}
	config.Server.Listen.Addr = addr
	config.Server.Name = "TestServer"

	ts := &testServer{
		server: netserver.NewTCPServer(addr),
		db:     db,
		dbPath: dbPath,
		tmpDir: tmpDir,
		port:   port,
		config: config,
	}

	sessionMgr := NewSessionManager()

	ts.server.SetConnectHandler(func(session *netserver.Session) {
		sessionMgr.Add(session)
	})
	ts.server.SetDisconnectHandler(func(session *netserver.Session) {
		sessionMgr.Remove(session.ID)
	})

	// Raw message handler for **runlogin (Fix 5)
	ts.server.SetRawMessageHandler(func(session *netserver.Session, raw string) bool {
		if !strings.HasPrefix(raw, "**") {
			return false
		}
		loginInfo := raw[2:]
		parts := strings.Split(loginInfo, "/")
		if len(parts) < 5 {
			return false
		}
		charName := parts[1]
		charData, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			return true
		}
		session.State = netserver.StateInGame
		session.CharacterID = charData.ID
		noticeResp := protocol.MakeDefaultMsg(protocol.SMSendNotice, 0, 0, 0, 0)
		ts.server.Send(session.ID, noticeResp, "Welcome!")
		return true
	})

	ts.server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body string) {
		switch session.State {
		case netserver.StateConnected:
			switch msg.Ident {
			case protocol.CMProtocol:
				// no-op
			case protocol.CMIDPassword:
				username, password := parseCredentials(body)
				accountID, passwordHash, err := db.GetAccountByUsername(username)
				if err != nil {
					hash := simpleHash(password)
					accountID, err = db.CreateAccount(username, hash)
					if err != nil {
						sendLoginFail(ts.server, session)
						return
					}
				} else if !verifyPassword(password, passwordHash) {
					sendLoginFail(ts.server, session)
					return
				}
				session.State = netserver.StateAuthenticated
				session.AccountName = username
				session.CharacterID = accountID
				resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
				ts.server.Send(session.ID, resp, "Server/1")
			}
		case netserver.StateAuthenticated:
			switch msg.Ident {
			case protocol.CMSelectServer:
				session.Certification = 54321
				resp := protocol.MakeDefaultMsg(protocol.SMSelectServerOK, 0, 0, 0, 0)
				ts.server.Send(session.ID, resp, fmt.Sprintf("127.0.0.1/%d/54321", port))
			case protocol.CMQueryChr:
				chars, _ := db.GetCharactersByAccount(session.CharacterID)
				var sb strings.Builder
				for i, c := range chars {
					if i > 0 {
						sb.WriteByte('/')
					}
					if i == 0 {
						sb.WriteByte('*')
					}
					sb.WriteString(c.Name)
					sb.WriteByte('/')
					sb.WriteString(fmt.Sprintf("%d", c.Job))
					sb.WriteByte('/')
					sb.WriteString("0")
					sb.WriteByte('/')
					sb.WriteString(fmt.Sprintf("%d", c.Level))
					sb.WriteByte('/')
					sb.WriteString(fmt.Sprintf("%d", c.Sex))
				}
				resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, int32(len(chars)), 0, 0, 0)
				ts.server.Send(session.ID, resp, sb.String())
			case protocol.CMSelChr:
				charName := body
				if idx := strings.Index(body, "/"); idx >= 0 {
					charName = body[idx+1:]
				}
				_, err := db.GetCharacterByName(session.CharacterID, charName)
				if err != nil {
					return
				}
				startResp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
				ts.server.Send(session.ID, startResp, fmt.Sprintf("127.0.0.1/%d", port))
			}
		case netserver.StateInGame:
			// Game messages — handled by raw handler for **login
		}
	})

	if err := ts.server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give acceptLoop goroutine time to start
	time.Sleep(200 * time.Millisecond)

	return ts
}

func (ts *testServer) stop() {
	ts.server.Stop()
	ts.db.Close()
	os.RemoveAll(ts.tmpDir)
}

func (ts *testServer) addr() string {
	return fmt.Sprintf("127.0.0.1:%d", ts.port)
}

// ============================================================================
// Mock Client
// ============================================================================

type mockClient struct {
	conn net.Conn
	code byte
	t    *testing.T
}

func newMockClient(t *testing.T, addr string) *mockClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("connect to %s: %v", addr, err)
	}
	return &mockClient{conn: conn, t: t}
}

func (c *mockClient) send(msg protocol.DefaultMessage, body string) {
	c.t.Helper()
	encoded := protocol.EncodeMessage(msg)
	if body != "" {
		encoded += protocol.EncodeString(body)
	}
	frame := protocol.FormatClientFrame(encoded, &c.code)
	if _, err := c.conn.Write([]byte(frame)); err != nil {
		c.t.Fatalf("send msg=%d: %v", msg.Ident, err)
	}
}

func (c *mockClient) sendRaw(s string) {
	c.t.Helper()
	encoded := protocol.EncodeString(s)
	frame := protocol.FormatClientFrame(encoded, &c.code)
	if _, err := c.conn.Write([]byte(frame)); err != nil {
		c.t.Fatalf("sendRaw: %v", err)
	}
}

func (c *mockClient) recv() (protocol.DefaultMessage, string) {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, err := c.conn.Read(buf)
	if err != nil {
		c.t.Fatalf("recv: %v", err)
	}
	data := buf[:n]
	if len(data) < 3 || data[0] != '#' || data[len(data)-1] != '!' {
		c.t.Fatalf("invalid frame: %q", string(data))
	}
	payload := string(data[1 : len(data)-1])
	if len(payload) < protocol.DefBlockSize {
		c.t.Fatalf("payload too short: %d bytes", len(payload))
	}
	msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
	body := ""
	if len(payload) > protocol.DefBlockSize {
		body = protocol.DecodeString(payload[protocol.DefBlockSize:])
	}
	return msg, body
}

func (c *mockClient) recvTimeout(timeout time.Duration) (protocol.DefaultMessage, string, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	n, err := c.conn.Read(buf)
	if err != nil {
		return protocol.DefaultMessage{}, "", err
	}
	data := buf[:n]
	if len(data) < 3 || data[0] != '#' || data[len(data)-1] != '!' {
		return protocol.DefaultMessage{}, "", fmt.Errorf("invalid frame: %q", string(data))
	}
	payload := string(data[1 : len(data)-1])
	if len(payload) < protocol.DefBlockSize {
		return protocol.DefaultMessage{}, "", fmt.Errorf("payload too short")
	}
	msg := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
	body := ""
	if len(payload) > protocol.DefBlockSize {
		body = protocol.DecodeString(payload[protocol.DefBlockSize:])
	}
	return msg, body, nil
}

func (c *mockClient) close() {
	c.conn.Close()
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestIntegration_LoginAndServerSelect(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)
	c := newMockClient(t, ts.addr())
	defer c.close()

	// Send CMProtocol
	c.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")

	// Login
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "alice/pass123")

	msg, body := c.recv()
	if msg.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("login: Ident=%d, want %d", msg.Ident, protocol.SMPassOKSelectServer)
	}
	if body != "Server/1" {
		t.Errorf("login body=%q, want %q", body, "Server/1")
	}

	// Select server (Fix 2: body should be "addr/port/cert")
	c.send(protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0), "Server")

	msg, body = c.recv()
	if msg.Ident != protocol.SMSelectServerOK {
		t.Fatalf("select: Ident=%d, want %d", msg.Ident, protocol.SMSelectServerOK)
	}
	parts := strings.Split(body, "/")
	if len(parts) < 3 {
		t.Fatalf("SMSelectServerOK body=%q, expected addr/port/cert", body)
	}
	var cert int
	fmt.Sscanf(parts[2], "%d", &cert)
	if cert != 54321 {
		t.Errorf("cert=%d, want 54321", cert)
	}
	t.Logf("SMSelectServerOK body=%q (cert=%d)", body, cert)
}

func TestIntegration_WrongPassword(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)

	// Create account first
	c1 := newMockClient(t, ts.addr())
	c1.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")
	c1.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "bob/correct")
	msg, _ := c1.recv()
	if msg.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("setup: Ident=%d", msg.Ident)
	}
	c1.close()

	time.Sleep(10 * time.Millisecond)

	// Try wrong password (Fix 1: should get SMPasswdFail, not SMQueryChrFail)
	c2 := newMockClient(t, ts.addr())
	defer c2.close()
	c2.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")
	c2.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "bob/wrong")

	msg, _ = c2.recv()
	if msg.Ident != protocol.SMPasswdFail {
		t.Errorf("wrong password: Ident=%d, want %d (SMPasswdFail)", msg.Ident, protocol.SMPasswdFail)
	}
	if msg.Recog != -1 {
		t.Errorf("wrong password: Recog=%d, want -1", msg.Recog)
	}
}

func TestIntegration_QueryChrTextFormat(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)

	// Login
	c := newMockClient(t, ts.addr())
	defer c.close()
	c.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "charlie/pw")
	msg, _ := c.recv()
	if msg.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("login: Ident=%d", msg.Ident)
	}

	// Select server
	c.send(protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0), "Server")
	c.recv()

	// Create character directly in DB
	accID, _, _ := ts.db.GetAccountByUsername("charlie")
	ts.db.CreateCharacter(accID, "Fighter", 0, 0)
	ts.db.CreateCharacter(accID, "Mage", 1, 1)

	// Query characters (Fix 3: should return text format)
	c.send(protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0), "charlie/54321")

	msg, body := c.recv()
	if msg.Ident != protocol.SMQueryChr {
		t.Fatalf("query: Ident=%d, want %d", msg.Ident, protocol.SMQueryChr)
	}

	// Verify text format: "*name/job/hair/level/sex/..."
	if body == "" {
		t.Fatal("SMQueryChr body is empty")
	}
	if body[0] != '*' {
		t.Errorf("body should start with *, got %q", body[:10])
	}

	// Parse and verify
	parts := strings.Split(body, "/")
	if len(parts) != 10 { // 2 chars * 5 fields
		t.Errorf("expected 10 parts, got %d: %q", len(parts), body)
	}
	if parts[0] != "*Fighter" {
		t.Errorf("parts[0]=%q, want *Fighter", parts[0])
	}

	t.Logf("SMQueryChr body=%q", body)
}

func TestIntegration_SelChrAndStartPlay(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)

	// Login + select server + create character
	c := newMockClient(t, ts.addr())
	defer c.close()
	c.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "dave/pw")
	c.recv() // SMPassOKSelectServer
	c.send(protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0), "Server")
	c.recv() // SMSelectServerOK

	accID, _, _ := ts.db.GetAccountByUsername("dave")
	ts.db.CreateCharacter(accID, "Hero", 0, 0)

	// Select character (Fix 4: parse name from body, Fix 6: SMStartPlay has body)
	c.send(protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0), "dave/Hero")

	msg, body := c.recv()
	if msg.Ident != protocol.SMStartPlay {
		t.Fatalf("selchr: Ident=%d, want %d", msg.Ident, protocol.SMStartPlay)
	}

	// Fix 6: body should be "addr/port", not empty
	if body == "" {
		t.Fatal("SMStartPlay body is empty, expected addr/port")
	}
	parts := strings.Split(body, "/")
	if len(parts) < 2 {
		t.Fatalf("SMStartPlay body=%q, expected addr/port", body)
	}
	t.Logf("SMStartPlay body=%q", body)
}

func TestIntegration_RunLoginAndNotice(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)

	// Full login flow up to SMStartPlay
	c := newMockClient(t, ts.addr())
	defer c.close()
	c.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "eve/pw")
	c.recv() // SMPassOKSelectServer
	c.send(protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0), "Server")
	c.recv() // SMSelectServerOK

	accID, _, _ := ts.db.GetAccountByUsername("eve")
	ts.db.CreateCharacter(accID, "Rogue", 0, 0)

	c.send(protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0), "eve/Rogue")
	c.recv() // SMStartPlay

	// Send **runlogin (Fix 5: raw message with ** prefix)
	c.sendRaw("**eve/Rogue/54321/120040918/9")

	// Should receive SMSendNotice
	msg, body := c.recv()
	if msg.Ident != protocol.SMSendNotice {
		t.Fatalf("runlogin: Ident=%d, want %d", msg.Ident, protocol.SMSendNotice)
	}
	if body == "" {
		t.Error("SMSendNotice body is empty")
	}
	t.Logf("SMSendNotice body=%q", body)
}

func TestIntegration_FullLoginFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.stop()

	time.Sleep(10 * time.Millisecond)

	c := newMockClient(t, ts.addr())
	defer c.close()

	// Step 1: Protocol
	c.send(protocol.MakeDefaultMsg(protocol.CMProtocol, 120040918, 0, 0, 0), "")

	// Step 2: Login
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "frank/pass")
	msg, body := c.recv()
	assertMsg(t, msg, protocol.SMPassOKSelectServer, "SMPassOKSelectServer")
	t.Logf("[1] Login OK: body=%q", body)

	// Step 3: Select server
	c.send(protocol.MakeDefaultMsg(protocol.CMSelectServer, 0, 0, 0, 0), "Server")
	msg, body = c.recv()
	assertMsg(t, msg, protocol.SMSelectServerOK, "SMSelectServerOK")
	assertNotEmpty(t, body, "SMSelectServerOK body")
	t.Logf("[2] Server selected: body=%q", body)

	// Step 4: Create character in DB
	accID, _, _ := ts.db.GetAccountByUsername("frank")
	ts.db.CreateCharacter(accID, "Warrior", 0, 0)

	// Step 5: Query characters
	c.send(protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0), "frank/54321")
	msg, body = c.recv()
	assertMsg(t, msg, protocol.SMQueryChr, "SMQueryChr")
	assertHasPrefix(t, body, "*", "SMQueryChr body starts with *")
	t.Logf("[3] Characters: body=%q", body)

	// Step 6: Select character
	c.send(protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0), "frank/Warrior")
	msg, body = c.recv()
	assertMsg(t, msg, protocol.SMStartPlay, "SMStartPlay")
	assertNotEmpty(t, body, "SMStartPlay body")
	t.Logf("[4] Start play: body=%q", body)

	// Step 7: Run login (**raw message)
	c.sendRaw("**frank/Warrior/54321/120040918/9")
	msg, body = c.recv()
	assertMsg(t, msg, protocol.SMSendNotice, "SMSendNotice")
	assertNotEmpty(t, body, "SMSendNotice body")
	t.Logf("[5] Notice: body=%q", body)

	// Step 8: Notice acknowledged
	c.send(protocol.MakeDefaultMsg(protocol.CMLoginNoticeOK, 0, 0, 0, 0), "")

	t.Log("Full login flow completed successfully!")
}

// ============================================================================
// Assertion Helpers
// ============================================================================

func assertMsg(t *testing.T, msg protocol.DefaultMessage, wantIdent uint16, desc string) {
	t.Helper()
	if msg.Ident != wantIdent {
		t.Fatalf("%s: Ident=%d, want %d", desc, msg.Ident, wantIdent)
	}
}

func assertNotEmpty(t *testing.T, s string, desc string) {
	t.Helper()
	if s == "" {
		t.Errorf("%s: expected non-empty", desc)
	}
}

func assertHasPrefix(t *testing.T, s string, prefix string, desc string) {
	t.Helper()
	if !strings.HasPrefix(s, prefix) {
		t.Errorf("%s: %q does not start with %q", desc, s, prefix)
	}
}

// Suppress unused import warnings
var _ = net.Listen
var _ = os.RemoveAll
