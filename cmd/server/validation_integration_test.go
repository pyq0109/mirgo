package main

// 集成测试：用生产消息处理器（handleConnectedMessage/handleAuthenticatedMessage）
// 验证账号/密码/角色名校验，覆盖 newTestServer 简化 handler 未覆盖的场景。

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

// newRealTestServer 创建使用生产 handler 的测试服务器。
func newRealTestServer(t *testing.T) *testServer {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	log.SetLevel(log.LevelError)

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}

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
		clearSessionThrottles(session.ID)
	})
	ts.server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body, rawBody string) {
		switch session.State {
		case netserver.StateConnected:
			handleConnectedMessage(ts.server, session, msg, body, rawBody, config, db, sessionMgr)
		case netserver.StateAuthenticated:
			// userEngine/mapMgr 仅被 CMSelectServer 之外的游戏准备消息使用，
			// 本测试只覆盖选角阶段消息，传 nil 安全。
			handleAuthenticatedMessage(ts.server, session, msg, body, rawBody, config, db, nil, nil)
		}
	})

	if err := ts.server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	return ts
}

// sendEncodedBody 发送 body 为预编码段的消息（注册/补全资料使用）。
func (c *mockClient) sendEncodedBody(msg protocol.DefaultMessage, encodedBody string) {
	c.t.Helper()
	encoded := protocol.EncodeMessage(msg) + encodedBody
	frame := protocol.FormatClientFrame(encoded, &c.code)
	if _, err := c.conn.Write([]byte(frame)); err != nil {
		c.t.Fatalf("sendEncodedBody msg=%d: %v", msg.Ident, err)
	}
}

// makeRegBody 构造注册/补全资料消息的 body（两段 EncodeBuffer）。
func makeRegBody(account, password, userName, quiz2 string) string {
	var ue protocol.UserEntry
	var ua protocol.UserEntryAdd
	ue.SetAccount(account)
	ue.SetPassword(password)
	ue.SetUserName(userName)
	ue.SetQuiz("问题1")
	ue.SetAnswer("答案1")
	ua.SetQuiz2(quiz2)
	ua.SetAnswer2("答案2")
	ua.SetBirthDay("1990/1/1")
	return protocol.EncodeBuffer(ue.Bytes()) + protocol.EncodeBuffer(ua.Bytes())
}

// realLogin 用生产登录流程登录；要求账号已存在且资料完整。
// 前一连接 close 后服务端的 disconnect 清理是异步的，若撞到重复登录
// 判定（-3）则等待后重试。
func realLogin(t *testing.T, ts *testServer, user, pass string) *mockClient {
	t.Helper()
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	var lastResp protocol.DefaultMessage
	for attempt := 0; attempt < 5; attempt++ {
		c := newMockClient(t, ts.addr())
		c.send(msg, user+"/"+pass)
		resp, _ := c.recv()
		if resp.Ident == protocol.SMPassOKSelectServer {
			return c
		}
		lastResp = resp
		c.close()
		if resp.Ident == protocol.SMPasswdFail && resp.Recog == -3 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	t.Fatalf("login %s: expected SMPassOKSelectServer, got %d (Recog=%d)", user, lastResp.Ident, lastResp.Recog)
	return nil
}

// createFullAccount 创建资料完整的账号（登录时不触发 SMNeedUpdateAccount）。
func createFullAccount(t *testing.T, ts *testServer, user, pass string) {
	t.Helper()
	info := &storage.AccountInfo{UserName: "测试用户", Quiz2: "问题2"}
	if _, err := ts.db.CreateAccountWithEntry(user, simpleHash(pass), info); err != nil {
		t.Fatalf("create account %s: %v", user, err)
	}
}

// ============================================================================
// 注册校验（CMAddNewUser）
// ============================================================================

func TestIntegrationReal_RegisterValidation(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	// 合法注册 → SMNewIDSuccess
	c := newMockClient(t, ts.addr())
	regMsg := protocol.MakeDefaultMsg(protocol.CMAddNewUser, 0, 0, 0, 0)
	c.sendEncodedBody(regMsg, makeRegBody("user1", "pass1", "张三", "问题2"))
	resp, _ := c.recv()
	if resp.Ident != protocol.SMNewIDSuccess {
		t.Fatalf("expected SMNewIDSuccess, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 账号含空格 → SMNewIDFail(-1)
	c = newMockClient(t, ts.addr())
	c.sendEncodedBody(regMsg, makeRegBody("bad user", "pass1", "张三", "问题2"))
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewIDFail || resp.Recog != -1 {
		t.Fatalf("expected SMNewIDFail(-1), got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 账号过短 → SMNewIDFail(-1)
	c = newMockClient(t, ts.addr())
	c.sendEncodedBody(regMsg, makeRegBody("ab", "pass1", "张三", "问题2"))
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewIDFail || resp.Recog != -1 {
		t.Fatalf("expected SMNewIDFail(-1) for short account, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 重复账号 → SMNewIDFail(0)
	c = newMockClient(t, ts.addr())
	c.sendEncodedBody(regMsg, makeRegBody("user1", "other", "李四", "问题2"))
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewIDFail || resp.Recog != 0 {
		t.Fatalf("expected SMNewIDFail(0) for duplicate, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 密码过短 → SMNewIDFail(-1)
	c = newMockClient(t, ts.addr())
	c.sendEncodedBody(regMsg, makeRegBody("user2", "ab", "张三", "问题2"))
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewIDFail || resp.Recog != -1 {
		t.Fatalf("expected SMNewIDFail(-1) for short password, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()
}

// ============================================================================
// 创建角色校验（CMNewChr）
// ============================================================================

func newChrMsg(name string) protocol.DefaultMessage {
	return protocol.MakeDefaultMsg(protocol.CMNewChr, 0, 0, 0, 0)
}

func TestIntegrationReal_NewChrValidation(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	createFullAccount(t, ts, "chra1", "pw")

	// 合法角色名 → SMNewChrSuccess
	c := realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("勇者"), "chra1/勇者/1/0/0")
	resp, _ := c.recv()
	if resp.Ident != protocol.SMNewChrSuccess {
		t.Fatalf("expected SMNewChrSuccess, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 含空格 → Recog=0
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("勇 者"), "chra1/勇 者/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 0 {
		t.Fatalf("expected SMNewChrFail(0) for space, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 单字符（rune<2）→ Recog=0
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("勇"), "chra1/勇/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 0 {
		t.Fatalf("expected SMNewChrFail(0) for single rune, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 2 个 ASCII 字符（字节<3）→ Recog=0
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("ab"), "chra1/ab/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 0 {
		t.Fatalf("expected SMNewChrFail(0) for 2-char name, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 含标点 → Recog=0
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("勇@士"), "chra1/勇@士/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 0 {
		t.Fatalf("expected SMNewChrFail(0) for punct, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 已有 1 个角色（勇者），数量未满仍可创建
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("法师乙"), "chra1/法师乙/1/1/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrSuccess {
		t.Fatalf("expected SMNewChrSuccess for 2nd char, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 第 3 个角色 → Recog=3
	c = realLogin(t, ts, "chra1", "pw")
	c.send(newChrMsg("道士丙"), "chra1/道士丙/1/2/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 3 {
		t.Fatalf("expected SMNewChrFail(3) for 3rd char, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 跨账号重名 → Recog=2（全局重名检查）
	createFullAccount(t, ts, "chrb1", "pw")
	c = realLogin(t, ts, "chrb1", "pw")
	c.send(newChrMsg("勇者"), "chrb1/勇者/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 2 {
		t.Fatalf("expected SMNewChrFail(2) for global dup name, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()
}

func TestIntegrationReal_NewChrDenyList(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	// 注入黑名单（生产环境由 DenyChrName.txt 加载）。名字需 >=3 字符
	// 才能通过最小长度校验进入黑名单匹配。
	denyChrNameList = []string{"gmm", "管理员"}
	defer func() { denyChrNameList = nil }()

	createFullAccount(t, ts, "denya", "pw")

	// 大小写不敏感命中
	c := realLogin(t, ts, "denya", "pw")
	c.send(newChrMsg("GMM"), "denya/GMM/1/0/0")
	resp, _ := c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 2 {
		t.Fatalf("expected SMNewChrFail(2) for denied name GMM, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 中文黑名单命中
	c = realLogin(t, ts, "denya", "pw")
	c.send(newChrMsg("管理员"), "denya/管理员/1/0/0")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMNewChrFail || resp.Recog != 2 {
		t.Fatalf("expected SMNewChrFail(2) for denied name 管理员, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()
}

// ============================================================================
// 重复登录（-3）
// ============================================================================

func TestIntegrationReal_DuplicateLoginRejected(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	createFullAccount(t, ts, "dupuser", "pw")

	// 第一个连接登录成功
	c1 := realLogin(t, ts, "dupuser", "pw")
	defer c1.close()

	// 第二个连接用同账号登录 → SMPasswdFail(-3)
	c2 := newMockClient(t, ts.addr())
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	c2.send(msg, "dupuser/pw")
	resp, _ := c2.recv()
	if resp.Ident != protocol.SMPasswdFail || resp.Recog != -3 {
		t.Fatalf("expected SMPasswdFail(-3), got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c2.close()
}

// ============================================================================
// 修改密码校验（CMChangePassword）
// ============================================================================

func TestIntegrationReal_ChangePasswordValidation(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	createFullAccount(t, ts, "chgpw1", "oldpass")

	// 新密码过短 → Recog=0
	c := newMockClient(t, ts.addr())
	msg := protocol.MakeDefaultMsg(protocol.CMChangePassword, 0, 0, 0, 0)
	c.send(msg, "chgpw1\toldpass\tab")
	resp, _ := c.recv()
	if resp.Ident != protocol.SMChgPasswdFail || resp.Recog != 0 {
		t.Fatalf("expected SMChgPasswdFail(0) for short newpw, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 账号不存在 → Recog=0
	c = newMockClient(t, ts.addr())
	c.send(msg, "nosuchuser\toldpass\tnewpass")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMChgPasswdFail || resp.Recog != 0 {
		t.Fatalf("expected SMChgPasswdFail(0) for missing account, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 旧密码错误 5 次 → 第 6 次锁定 Recog=-2（改密限流 5 秒/连接，须用新连接）
	for i := 0; i < 5; i++ {
		c = newMockClient(t, ts.addr())
		c.send(msg, "chgpw1\twrongpw\tnewpass")
		resp, _ = c.recv()
		if resp.Ident != protocol.SMChgPasswdFail || resp.Recog != -1 {
			t.Fatalf("attempt %d: expected SMChgPasswdFail(-1), got %d (Recog=%d)", i+1, resp.Ident, resp.Recog)
		}
		c.close()
	}
	c = newMockClient(t, ts.addr())
	c.send(msg, "chgpw1\toldpass\tnewpass") // 即使旧密码正确，锁定中也拒绝
	resp, _ = c.recv()
	if resp.Ident != protocol.SMChgPasswdFail || resp.Recog != -2 {
		t.Fatalf("expected SMChgPasswdFail(-2) locked, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()
}

func TestIntegrationReal_ChangePasswordSuccess(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	createFullAccount(t, ts, "chgpw2", "oldpass")

	c := newMockClient(t, ts.addr())
	msg := protocol.MakeDefaultMsg(protocol.CMChangePassword, 0, 0, 0, 0)
	c.send(msg, "chgpw2\toldpass\tnewpass")
	resp, _ := c.recv()
	if resp.Ident != protocol.SMChgPasswdSuccess {
		t.Fatalf("expected SMChgPasswdSuccess, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 新密码可登录，旧密码失败
	c = newMockClient(t, ts.addr())
	loginMsg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	c.send(loginMsg, "chgpw2/oldpass")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMPasswdFail || resp.Recog != -1 {
		t.Fatalf("expected SMPasswdFail(-1) with old password, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	c = newMockClient(t, ts.addr())
	c.send(loginMsg, "chgpw2/newpass")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("expected SMPassOKSelectServer with new password, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()
}

// ============================================================================
// 资料补全（SM_NEEDUPDATE_ACCOUNT / CM_UPDATEUSER）
// ============================================================================

func TestIntegrationReal_NeedUpdateAccount(t *testing.T) {
	ts := newRealTestServer(t)
	defer ts.stop()

	// 资料不全的账号（无 UserName/Quiz2）：登录后先发 SMNeedUpdateAccount
	// 再发 SMPassOKSelectServer（LoginSrv/LMain.pas:1239-1282）。
	if _, err := ts.db.CreateAccount("incomplete", simpleHash("pw")); err != nil {
		t.Fatalf("create account: %v", err)
	}

	c := newMockClient(t, ts.addr())
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	c.send(msg, "incomplete/pw")

	resp, _ := c.recv()
	if resp.Ident != protocol.SMNeedUpdateAccount {
		t.Fatalf("expected SMNeedUpdateAccount, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}

	resp, _ = c.recv()
	if resp.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("expected SMPassOKSelectServer after NEEDUPDATE, got %d", resp.Ident)
	}

	// 提交 CMUpdateUser 补全资料
	var ue protocol.UserEntry
	var ua protocol.UserEntryAdd
	ue.SetAccount("incomplete")
	ue.SetPassword("pw")
	ue.SetUserName("补全姓名")
	ue.SetQuiz("问题1")
	ue.SetAnswer("答案1")
	ua.SetQuiz2("问题2")
	ua.SetAnswer2("答案2")
	ua.SetBirthDay("1990/1/1")
	updMsg := protocol.MakeDefaultMsg(protocol.CMUpdateUser, 0, 0, 0, 0)
	c.sendEncodedBody(updMsg, protocol.EncodeBuffer(ue.Bytes())+protocol.EncodeBuffer(ua.Bytes()))

	resp, _ = c.recv()
	if resp.Ident != protocol.SMUpdateIDSuccess {
		t.Fatalf("expected SMUpdateIDSuccess, got %d (Recog=%d)", resp.Ident, resp.Recog)
	}
	c.close()

	// 再次登录不再触发 SMNeedUpdateAccount
	c = newMockClient(t, ts.addr())
	c.send(msg, "incomplete/pw")
	resp, _ = c.recv()
	if resp.Ident != protocol.SMPassOKSelectServer {
		t.Fatalf("expected direct SMPassOKSelectServer after update, got %d", resp.Ident)
	}
	c.close()
}
