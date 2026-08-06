package main

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

// TestLogoutThenStartGameAgain 回归: 登出回选角后无法再次开始游戏。
//
// 两个根因:
//  1. session.CharacterID 曾身兼两职——登录时存账号 ID, 进游戏
//     (**runlogin) 时被覆盖为角色 ID, 登出时清零; 账号域查询
//     (sendCharacterList/CMSelChr 校验/**runlogin 加载角色) 全部失效。
//     修复: 独立的 Session.AccountID, 登录时设置, 永不覆盖。
//  2. 服务端要求每次选角前必须先查角 (boChrQueryed, UsrSoc.pas:536-590),
//     选角成功即消费该标志。客户端登出回选角后必须重发 CM_QUERYCHR,
//     否则 CM_SELCHR 被 "Double send _SELCHR" 拒绝。
//     修复: 客户端收到 SM_LOGOUTOK 后立即 SendQueryChr (main.go)。
//
// 流程走生产处理器 handleConnectedMessage/handleAuthenticatedMessage/
// LogoutPlayer: 登录→查角→选角→进游戏→登出→再查角→再选角。
func TestLogoutThenStartGameAgain(t *testing.T) {
	log.SetLevel(log.LevelError)

	tmpDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	defer db.Close()

	accountID, err := db.CreateAccount("tester", simpleHash("pass"))
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := db.CreateCharacter(accountID, "hero", 0, 0, 0); err != nil {
		t.Fatalf("CreateCharacter: %v", err)
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

	server := netserver.NewTCPServer(addr)
	sessionMgr := NewSessionManager()
	userEngine := NewUserEngine(db, NewMapManager(filepath.Join(tmpDir, "maps")))

	server.SetConnectHandler(func(s *netserver.Session) { sessionMgr.Add(s) })
	server.SetDisconnectHandler(func(s *netserver.Session) {
		sessionMgr.Remove(s.ID)
		clearSessionThrottles(s.ID)
	})
	// 镜像生产 **runlogin 处理器中与本回归相关的会话变更:
	// 用 AccountID 查角色、CharacterID 覆盖为角色 ID、状态进 InGame、
	// 注册玩家对象 (供 CM_LOGOUT 的 GetPlayer 找到)。
	server.SetRawMessageHandler(func(s *netserver.Session, raw string) bool {
		if !strings.HasPrefix(raw, "**") {
			return false
		}
		parts := strings.Split(raw[2:], "/")
		if len(parts) < 5 {
			return true
		}
		charData, err := db.GetCharacterByName(s.AccountID, parts[1])
		if err != nil {
			return true
		}
		player := NewPlayObject(s, charData.Name, int32(charData.ID))
		player.Engine = userEngine
		s.State = netserver.StateInGame
		s.CharacterID = charData.ID
		userEngine.AddPlayer(player)
		return true
	})
	server.SetMessageHandler(func(s *netserver.Session, msg protocol.DefaultMessage, body, rawBody string) {
		switch s.State {
		case netserver.StateConnected:
			handleConnectedMessage(server, s, msg, body, rawBody, config, db, sessionMgr)
		case netserver.StateAuthenticated:
			handleAuthenticatedMessage(server, s, msg, body, rawBody, config, db, userEngine, nil)
		case netserver.StateInGame:
			// 生产 in-game 分支的登出路径 (server/main.go)
			if msg.Ident == protocol.CMLogout || msg.Ident == protocol.CMExitGame || msg.Ident == protocol.CMSoftClose {
				if player := userEngine.GetPlayer(int32(s.CharacterID)); player != nil {
					userEngine.LogoutPlayer(server, db, player)
				}
			}
		}
	})
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Stop()
	time.Sleep(200 * time.Millisecond)

	c := newMockClient(t, addr)
	defer c.close()

	// expect 等待指定 Ident 的响应, 跳过中间的其他消息
	// (如登录时可能先到的 SM_NEEDUPDATE_ACCOUNT)。
	expect := func(want uint16, what string) (protocol.DefaultMessage, string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			msg, body, err := c.recvTimeout(time.Until(deadline))
			if err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			if msg.Ident == want {
				return msg, body
			}
		}
		t.Fatalf("%s: 等待 ident=%d 超时", what, want)
		return protocol.DefaultMessage{}, ""
	}

	// 1. 登录
	c.send(protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0), "tester/pass")
	expect(protocol.SMPassOKSelectServer, "登录")

	// 2. 第一次查角
	c.send(protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0), "tester/0")
	msg, body := expect(protocol.SMQueryChr, "第一次查角")
	if msg.Recog != 1 || !strings.Contains(body, "hero") {
		t.Fatalf("第一次查角: Recog=%d body=%q, want 1 个角色且含 hero", msg.Recog, body)
	}

	// 3. 第一次选角 → SM_STARTPLAY
	c.send(protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0), "tester/hero")
	expect(protocol.SMStartPlay, "第一次选角")

	// 4. 进游戏 (CharacterID 被覆盖为角色 ID)
	c.sendRaw("**tester/hero/0/120040918/9")
	time.Sleep(300 * time.Millisecond)

	// 5. 登出 → SM_LOGOUTOK (CharacterID 清零, 回到 Authenticated)
	c.send(protocol.MakeDefaultMsg(protocol.CMLogout, 0, 0, 0, 0), "")
	expect(protocol.SMLogoutOK, "登出")

	// 6. 登出后重新查角: 角色列表必须仍有 1 个角色
	//    (旧实现用清零的 CharacterID 查账号域, 列表为空)。
	c.send(protocol.MakeDefaultMsg(protocol.CMQueryChr, 0, 0, 0, 0), "tester/0")
	msg, body = expect(protocol.SMQueryChr, "登出后查角")
	if msg.Recog != 1 || !strings.Contains(body, "hero") {
		t.Fatalf("登出后查角: Recog=%d body=%q, want 1 个角色且含 hero", msg.Recog, body)
	}

	// 7. 再次选角: 必须再次收到 SM_STARTPLAY
	//    (旧实现被 boChrQueryed 门拒绝, 无任何响应)。
	c.send(protocol.MakeDefaultMsg(protocol.CMSelChr, 0, 0, 0, 0), "tester/hero")
	expect(protocol.SMStartPlay, "登出后再次选角")
}
