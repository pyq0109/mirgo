package main

import (
	"encoding/binary"
	"testing"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ============================================================================
// 消息解析测试
// ============================================================================

func TestParseFirstServer(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{"empty", "", "Server"},
		{"single server", "Server1/1", "Server1"},
		{"multiple servers", "Server1/1/Server2/2/Server3/3", "Server1"},
		{"name only", "MyServer", "MyServer"},
		{"empty name", "/1", "Server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFirstServer(tt.body)
			if result != tt.expected {
				t.Errorf("parseFirstServer(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}

func TestParseAddrPortCert(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantAddr string
		wantCert int
		wantErr  bool
	}{
		{"valid", "localhost/7000/12345", "localhost:7000", 12345, false},
		{"valid IP", "192.168.1.1/7100/99999", "192.168.1.1:7100", 99999, false},
		{"empty", "", "", 0, true},
		{"too few parts", "localhost/7000", "", 0, true},
		{"invalid cert", "localhost/7000/abc", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cert, err := parseAddrPortCert(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got addr=%s cert=%d", addr, cert)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
			if cert != tt.wantCert {
				t.Errorf("cert = %d, want %d", cert, tt.wantCert)
			}
		})
	}
}

func TestParseAddrPort(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantAddr string
		wantErr  bool
	}{
		{"valid", "localhost/7000", "localhost:7000", false},
		{"valid IP", "192.168.1.1/7100", "192.168.1.1:7100", false},
		{"empty", "", "", true},
		{"too few parts", "localhost", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseAddrPort(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got addr=%s", addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestParseQueryChrBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantChars   int
		wantSelect  int
		wantFirst   parsedChar
	}{
		{
			name:       "empty",
			body:       "",
			wantChars:  0,
			wantSelect: -1,
		},
		{
			name:       "single character selected",
			body:       "*Warrior/0/0/10/1",
			wantChars:  1,
			wantSelect: 0,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "two characters first selected",
			body:       "*Warrior/0/0/10/1/Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: 0,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "two characters second selected",
			body:       "Warrior/0/0/10/1/*Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: 1,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
		{
			name:       "no selected marker",
			body:       "Warrior/0/0/10/1/Wizard/1/0/5/0",
			wantChars:  2,
			wantSelect: -1,
			wantFirst:  parsedChar{Name: "Warrior", Job: 0, Hair: 0, Level: 10, Sex: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, selectedIdx := parseQueryChrBody(tt.body)
			if len(chars) != tt.wantChars {
				t.Fatalf("len(chars) = %d, want %d", len(chars), tt.wantChars)
			}
			if selectedIdx != tt.wantSelect {
				t.Errorf("selectedIdx = %d, want %d", selectedIdx, tt.wantSelect)
			}
			if tt.wantChars > 0 {
				c := chars[0]
				if c.Name != tt.wantFirst.Name {
					t.Errorf("char[0].Name = %q, want %q", c.Name, tt.wantFirst.Name)
				}
				if c.Job != tt.wantFirst.Job {
					t.Errorf("char[0].Job = %d, want %d", c.Job, tt.wantFirst.Job)
				}
				if c.Level != tt.wantFirst.Level {
					t.Errorf("char[0].Level = %d, want %d", c.Level, tt.wantFirst.Level)
				}
				if c.Sex != tt.wantFirst.Sex {
					t.Errorf("char[0].Sex = %d, want %d", c.Sex, tt.wantFirst.Sex)
				}
			}
		})
	}
}

// ============================================================================
// 协议消息格式测试
// ============================================================================

func TestLoginMessageFormat(t *testing.T) {
	// 验证 CMIDPassword 消息格式
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	decoded := protocol.DecodeMessage(encoded)

	if decoded.Ident != protocol.CMIDPassword {
		t.Errorf("Ident = %d, want %d", decoded.Ident, protocol.CMIDPassword)
	}
	if decoded.Recog != 0 {
		t.Errorf("Recog = %d, want 0", decoded.Recog)
	}
}

func TestCredentialBodyFormat(t *testing.T) {
	// 验证 "username/password" 编码往返
	body := "testuser/testpass"
	encoded := protocol.EncodeString(body)
	decoded := protocol.DecodeString(encoded)

	if decoded != body {
		t.Errorf("credential body = %q, want %q", decoded, body)
	}
}

func TestSMSelectServerOKFormat(t *testing.T) {
	// 验证 "addr/port/cert" body 格式
	body := "localhost/7000/12345"
	encoded := protocol.EncodeString(body)
	decoded := protocol.DecodeString(encoded)

	if decoded != body {
		t.Errorf("SMSelectServerOK body = %q, want %q", decoded, body)
	}
}

func TestSMQueryChrTextFormat(t *testing.T) {
	// 验证 "*name/job/hair/level/sex" 文本格式
	body := "*Warrior/0/0/10/1/Wizard/1/0/5/0"
	chars, selectedIdx := parseQueryChrBody(body)

	if len(chars) != 2 {
		t.Fatalf("len(chars) = %d, want 2", len(chars))
	}
	if selectedIdx != 0 {
		t.Errorf("selectedIdx = %d, want 0", selectedIdx)
	}
	if chars[0].Name != "Warrior" || chars[0].Job != 0 || chars[0].Level != 10 || chars[0].Sex != 1 {
		t.Errorf("chars[0] = %+v, want Warrior/0/0/10/1", chars[0])
	}
	if chars[1].Name != "Wizard" || chars[1].Job != 1 || chars[1].Level != 5 || chars[1].Sex != 0 {
		t.Errorf("chars[1] = %+v, want Wizard/1/0/5/0", chars[1])
	}
}

func TestSMStartPlayFormat(t *testing.T) {
	// 验证 "addr/port" body 格式
	body := "localhost/7000"
	addr, err := parseAddrPort(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "localhost:7000" {
		t.Errorf("addr = %q, want %q", addr, "localhost:7000")
	}
}

func TestRunLoginFormat(t *testing.T) {
	// 验证 **loginID/charName/cert/version/code 格式
	loginID := "testuser"
	charName := "Warrior"

	s := "**" + loginID + "/" + charName + "/" + "12345" + "/" + "120040918" + "/9"

	// 验证编码往返
	encoded := protocol.EncodeString(s)
	decoded := protocol.DecodeString(encoded)

	if decoded != s {
		t.Errorf("run login = %q, want %q", decoded, s)
	}

	// 验证解析
	if decoded[:2] != "**" {
		t.Errorf("prefix = %q, want **", decoded[:2])
	}

}

// ============================================================================
// 服务端帧格式测试
// ============================================================================

func TestServerFrameRoundTrip(t *testing.T) {
	// 服务端发送: #<payload>!
	// 客户端期望: #<payload>!（无数字前缀）
	msg := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -1, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	frame := protocol.FormatServerFrame(encoded)

	if frame[0] != '#' || frame[len(frame)-1] != '!' {
		t.Errorf("frame format = %q, expected #...!", frame)
	}

	// 模拟客户端解析
	payload := frame[1 : len(frame)-1]
	if len(payload) >= protocol.DefBlockSize {
		decoded := protocol.DecodeMessage(payload[:protocol.DefBlockSize])
		if decoded.Ident != protocol.SMPasswdFail {
			t.Errorf("Ident = %d, want %d", decoded.Ident, protocol.SMPasswdFail)
		}
		if decoded.Recog != -1 {
			t.Errorf("Recog = %d, want -1", decoded.Recog)
		}
	}
}

func TestClientFrameWithCode(t *testing.T) {
	// 客户端发送: #<code><payload>!
	msg := protocol.MakeDefaultMsg(protocol.CMIDPassword, 0, 0, 0, 0)
	encoded := protocol.EncodeMessage(msg)
	body := protocol.EncodeString("user/pass")
	code := byte(1)
	frame := protocol.FormatClientFrame(encoded+body, &code)

	if frame[0] != '#' {
		t.Errorf("frame[0] = %c, want #", frame[0])
	}
	if frame[1] != '1' {
		t.Errorf("frame[1] = %c, want 1", frame[1])
	}
	if frame[len(frame)-1] != '!' {
		t.Errorf("frame[-1] = %c, want !", frame[len(frame)-1])
	}
}

// ============================================================================
// P2 数据同步——body 布局对照（服务端侧固定在 cmd/server
// uidata_test.go；这些测试将客户端解析器固定到相同布局）
// ============================================================================

func TestParseAbilityLayout(t *testing.T) {
	gs := NewGameState()
	raw := make([]byte, 62)
	u16 := func(o int, v uint16) { binary.LittleEndian.PutUint16(raw[o:o+2], v) }
	u32 := func(o int, v uint32) { binary.LittleEndian.PutUint32(raw[o:o+4], v) }
	u16(0, 42)          // Level
	u32(2, 0x00030001)  // AC lo=1 hi=3
	u16(22, 321)        // HP
	u16(24, 654)        // MaxHP
	u16(26, 111)        // MP
	u16(28, 222)        // MaxMP
	u32(30, 12345)      // Exp
	u32(34, 99999)      // MaxExp
	u16(38, 30)         // Weight
	u16(40, 500)        // MaxWeight
	u16(50, 9)          // Hit
	u16(52, 18)         // Speed
	u16(54, 3)          // BonusPoint
	u32(56, 777)        // Gold
	hitSpeed := int16(-2)
	u16(60, uint16(hitSpeed)) // HitSpeed（int16 位模式，可为负）

	gs.ParseAbility(string(raw))

	if gs.Level != 42 || gs.HP != 321 || gs.MaxHP != 654 || gs.MP != 111 || gs.MaxMP != 222 {
		t.Errorf("core stats = %d/%d/%d/%d/%d, want 42/321/654/111/222",
			gs.Level, gs.HP, gs.MaxHP, gs.MP, gs.MaxMP)
	}
	if gs.AC != 0x00030001 {
		t.Errorf("AC = %#x, want 0x00030001", gs.AC)
	}
	if gs.Exp != 12345 || gs.MaxExp != 99999 {
		t.Errorf("exp = %d/%d, want 12345/99999", gs.Exp, gs.MaxExp)
	}
	if gs.Weight != 30 || gs.MaxWeight != 500 {
		t.Errorf("weight = %d/%d, want 30/500", gs.Weight, gs.MaxWeight)
	}
	if gs.Hit != 9 || gs.Speed != 18 || gs.BonusPoint != 3 || gs.Gold != 777 {
		t.Errorf("hit/speed/bonus/gold = %d/%d/%d/%d, want 9/18/3/777",
			gs.Hit, gs.Speed, gs.BonusPoint, gs.Gold)
	}
	if gs.HitSpeed != -2 {
		t.Errorf("HitSpeed = %d, want -2", gs.HitSpeed)
	}
}

func TestParseItemDefsAndRelink(t *testing.T) {
	gs := NewGameState()
	// 数据库到达前解析的背包物品没有 Def。
	gs.BagItems[0] = &BagItem{Idx: 7, Dura: 900, DuraMax: 1000, MakeIndex: 1}
	if gs.BagItems[0].Def != nil {
		t.Fatal("expected nil Def before DB sync")
	}
	if got := gs.BagItems[0].Looks(); got != 7 {
		t.Errorf("Looks fallback = %d, want raw Idx 7", got)
	}

	// 构造一条记录：固定 32 字节 + v2 扩展段 5 字节 + 名称。
	raw := make([]byte, 2, 64)
	binary.LittleEndian.PutUint16(raw, 1) // count
	rec := make([]byte, 37)
	binary.LittleEndian.PutUint16(rec[0:2], 7)   // Idx
	binary.LittleEndian.PutUint16(rec[2:4], 42)  // Looks
	rec[4], rec[5], rec[6], rec[7] = 5, 1, 10, 2 // StdMode/Shape/Weight/NeedLevel
	binary.LittleEndian.PutUint16(rec[16:18], 1) // DC
	binary.LittleEndian.PutUint16(rec[18:20], 3) // DCMax
	binary.LittleEndian.PutUint32(rec[28:32], 100) // Price
	src := int16(-5)
	binary.LittleEndian.PutUint16(rec[32:34], uint16(src)) // Source
	rec[34], rec[35], rec[36] = 1, 1, 2                    // Reserved/Need/AniCount
	raw = append(raw, rec...)
	raw = append(raw, byte(len("WoodSword")))
	raw = append(raw, "WoodSword"...)

	gs.ParseItemDefs(string(raw))

	def := gs.ItemDefs[7]
	if def == nil {
		t.Fatal("def idx 7 not parsed")
	}
	if def.Name != "WoodSword" || def.Looks != 42 || def.StdMode != 5 || def.NeedLevel != 2 ||
		def.DC != 1 || def.DCMax != 3 || def.Price != 100 {
		t.Errorf("def = %+v, fields mismatch", def)
	}
	if def.Source != -5 || def.Reserved != 1 || def.Need != 1 || def.AniCount != 2 {
		t.Errorf("def ext = %+v, want Source=-5 Reserved=1 Need=1 AniCount=2", def)
	}
	// 已有背包物品已重新关联，现在通过 def 解析 Looks。
	if gs.BagItems[0].Def != def {
		t.Fatal("bag item not relinked to def")
	}
	if got := gs.BagItems[0].Looks(); got != 42 {
		t.Errorf("Looks = %d, want 42 from def", got)
	}
}

func TestParseMagicsExtendedLayout(t *testing.T) {
	gs := NewGameState()
	raw := make([]byte, 2, 32)
	binary.LittleEndian.PutUint16(raw, 1) // count
	rec := make([]byte, 10)
	binary.LittleEndian.PutUint16(rec[0:2], 1)   // MagID
	rec[2] = 2                                   // Level
	rec[3] = '3'                                 // Key
	binary.LittleEndian.PutUint16(rec[4:6], 8)   // IconIdx
	binary.LittleEndian.PutUint16(rec[6:8], 120) // CurTrain
	binary.LittleEndian.PutUint16(rec[8:10], 600) // MaxTrain
	raw = append(raw, rec...)
	raw = append(raw, byte(len("FireBall")))
	raw = append(raw, "FireBall"...)

	gs.ParseMagics(string(raw))

	if len(gs.Magics) != 1 {
		t.Fatalf("magics = %d, want 1", len(gs.Magics))
	}
	m := gs.Magics[0]
	if m.MagID != 1 || m.Level != 2 || m.Key != '3' || m.IconIdx != 8 ||
		m.CurTrain != 120 || m.MaxTrain != 600 || m.Name != "FireBall" {
		t.Errorf("magic = %+v, fields mismatch", m)
	}
}

// TestDecodeTurnName 验证 SMTurn body 按 Delphi 方式分段解码名称：
// body = EncodeBuffer(8字节charDesc) + EncodeString(name)，编码层在第 11 字符处分割。
func TestDecodeTurnName(t *testing.T) {
	// 构造与服务端 playobject.go SearchViewRange 一致的 body。
	charDesc := make([]byte, 8)
	binary.LittleEndian.PutUint32(charDesc[0:4], 65586) // feature: raceImg=50, appr=1
	binary.LittleEndian.PutUint32(charDesc[4:8], 0)     // featureEx

	tests := []struct {
		name string
		body string // 附加在 charDesc 之后的名称段（明文，测试内编码）
		want string
	}{
		{"chinese npc", "传送员", "传送员"},
		{"merchant", "屠夫", "屠夫"},
		{"with color suffix", "铁匠铺老板/255", "铁匠铺老板"},
		{"empty name", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawBody := protocol.EncodeBuffer(charDesc)
			if tt.body != "" {
				rawBody += protocol.EncodeString(tt.body)
			}
			if got := decodeTurnName(rawBody); got != tt.want {
				t.Errorf("decodeTurnName(%q) = %q, want %q", rawBody, got, tt.want)
			}
		})
	}

	// charDesc 单独编码应为 11 字符（ceil(8*4/3)），确保分割点正确。
	if n := len(protocol.EncodeBuffer(charDesc)); n != turnCharDescEncodedLen {
		t.Errorf("encoded charDesc len = %d, want %d", n, turnCharDescEncodedLen)
	}

	// body 过短（仅 charDesc）应返回空名称。
	if got := decodeTurnName(protocol.EncodeBuffer(charDesc)); got != "" {
		t.Errorf("decodeTurnName(charDesc only) = %q, want empty", got)
	}
}

// TestReadyActionCMToSM 验证本地预测的 CM→SM 消息映射（Delphi Actor.pas:1479-1529）。
// 原始协议 CM/SM 编号不对称（CM_FIREHIT=3025 但 SM_FIREHIT=8），烈火/十字斩/双龙斩
// 必须显式映射；若一律 ident-3000，本地玩家这三招的攻击动画/特效会错乱或缺失。
func TestReadyActionCMToSM(t *testing.T) {
	tests := []struct {
		name string
		cm   int
		sm   int
	}{
		{"throw", protocol.CMThrow, protocol.SMThrow},
		{"hit", protocol.CMHit, protocol.SMHit},
		{"heavyhit", protocol.CMHeavyHit, protocol.SMHeavyHit},
		{"bighit", protocol.CMBigHit, protocol.SMBigHit},
		{"spell", protocol.CMSpell, protocol.SMSpell},
		{"powerhit", protocol.CMPowerHit, protocol.SMPowerHit},
		{"longhit", protocol.CMLongHit, protocol.SMLongHit},
		{"widehit", protocol.CMWideHit, protocol.SMWideHit},
		{"firehit", protocol.CMFireHit, protocol.SMFireHit},
		{"crshit", protocol.CMCrsHit, protocol.SMCrsHit},
		{"twinhit", protocol.CMTwinHit, protocol.SMTwinHit},
		{"horserun", protocol.CMHorseRun, protocol.SMHorseRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Actor{}
			a.ReadyAction(ActorMsg{Ident: tt.cm})
			if a.CurrentAction != tt.sm {
				t.Errorf("ReadyAction(CM=%d): CurrentAction = %d, want SM=%d", tt.cm, a.CurrentAction, tt.sm)
			}
		})
	}
}

// TestHitEffectLifecycle 验证攻击特效在切换动作与动画播完时被清除
// （Delphi m_boHitEffect := FALSE，Actor.pas:3152 / 3627），否则会残留在角色身上。
func TestHitEffectLifecycle(t *testing.T) {
	// 攻杀剑术应置 HitEffectNumber=1。
	a := &Actor{}
	a.ReadyAction(ActorMsg{Ident: protocol.CMPowerHit})
	if a.HitEffectNumber != 1 {
		t.Fatalf("攻杀剑术后 HitEffectNumber = %d, want 1", a.HitEffectNumber)
	}
	// 切换到非攻击动作应立即清除（calcHumanFrame 开头复位）。
	a.ReadyAction(ActorMsg{Ident: protocol.CMWalk})
	if a.HitEffectNumber != 0 {
		t.Errorf("切换行走后 HitEffectNumber = %d, want 0（特效残留）", a.HitEffectNumber)
	}

	// 攻击动画播完（Run 动作结束分支）也应清除。
	b := &Actor{}
	b.CurrentAction = protocol.SMPowerHit
	b.HitEffectNumber = 1
	b.FrameTime = 85
	b.EndFrame = 5
	b.CurrentFrame = 5 // 已到末帧
	b.Run(1000)        // 首帧懒初始化 LastFrameTick，不推进
	b.Run(1085)        // now-LastFrameTick >= 85 → 触发动作结束
	if b.HitEffectNumber != 0 || b.CurrentAction != 0 {
		t.Errorf("攻击动画结束后 HitEffectNumber=%d CurrentAction=%d, want 0/0（特效残留）",
			b.HitEffectNumber, b.CurrentAction)
	}
}

// TestStruckAnimation 验证受击动画的 Delphi 语义。
func TestStruckAnimation(t *testing.T) {
	// 1. 受击不改变位置和朝向（Delphi Actor.pas:1534-1569）。
	t.Run("struck keeps pos and dir", func(t *testing.T) {
		a := &Actor{CurrX: 100, CurrY: 200, Dir: 3}
		a.ReadyAction(ActorMsg{Ident: protocol.SMStruck, X: 50, Y: 60, Dir: 7})
		if a.CurrX != 100 || a.CurrY != 200 || a.Dir != 3 {
			t.Errorf("受击后 pos=(%d,%d) dir=%d, want (100,200) dir=3", a.CurrX, a.CurrY, a.Dir)
		}
		if a.CurrentAction != protocol.SMStruck {
			t.Errorf("CurrentAction = %d, want SMStruck", a.CurrentAction)
		}
	})

	// 2. 排队语义：当前动作未结束时 ProcMsg 不消费受击消息。
	t.Run("struck waits in queue", func(t *testing.T) {
		a := &Actor{}
		a.CurrentAction = protocol.SMWalk
		a.SendMsg(protocol.SMStruck, 0, 0, 0, 0, 0)
		a.ProcMsg()
		if a.CurrentAction != protocol.SMWalk {
			t.Errorf("ProcMsg 打断了当前动作: CurrentAction = %d, want SMWalk", a.CurrentAction)
		}
		if a.MsgCount() != 1 {
			t.Errorf("受击消息被消费: MsgCount = %d, want 1", a.MsgCount())
		}
		a.CurrentAction = 0
		a.ProcMsg()
		if a.CurrentAction != protocol.SMStruck {
			t.Errorf("空闲后未消费受击: CurrentAction = %d, want SMStruck", a.CurrentAction)
		}
	})

	// 3. UpdateMsg 合并连续受击为一条（Delphi Actor.pas:1303-1326）。
	t.Run("update msg dedups struck", func(t *testing.T) {
		a := &Actor{}
		a.UpdateMsg(protocol.SMStruck, 0, 0, 0, 0, 111)
		a.UpdateMsg(protocol.SMStruck, 0, 0, 0, 0, 222)
		if a.MsgCount() != 1 {
			t.Fatalf("连续受击未合并: MsgCount = %d, want 1", a.MsgCount())
		}
		msg, _ := a.GetMessage()
		if msg.State != 222 {
			t.Errorf("合并后应保留最新一条: State = %d, want 222", msg.State)
		}
	})

	// 4. 首帧完整显示：ReadyAction 后第一次 Run 不推进帧。
	t.Run("first frame not skipped", func(t *testing.T) {
		a := &Actor{Dir: 0}
		a.ReadyAction(ActorMsg{Ident: protocol.SMStruck})
		start := a.CurrentFrame
		a.Run(1000) // 懒初始化 LastFrameTick
		if a.CurrentFrame != start {
			t.Errorf("第一次 Run 推进了帧: %d -> %d（首帧被跳过）", start, a.CurrentFrame)
		}
		ft := int64(a.FrameTime)
		a.Run(1000 + ft) // 一个 FrameTime 后才推进
		if a.CurrentFrame != start+1 {
			t.Errorf("一个 FrameTime 后帧 = %d, want %d", a.CurrentFrame, start+1)
		}
	})
}

// ============================================================================
// 入站 payload 分类测试
// ============================================================================

// TestClassifyPayloadEqPrefixStandardMsg 回归: '=' (0x3D) 在 6Bit 编码字符集
// 内, 标准消息首字符可能恰为 '=' (Recog 低字节 ∈ 4..7, 如 SM_NEWMAP 的
// Recog=x 坐标)。旧实现按首字符 '=' 一律当短消息丢弃, 导致 SM_NEWMAP
// 丢失、进游戏黑屏。标准消息必须先按长度 (≥DefBlockSize) 判定。
func TestClassifyPayloadEqPrefixStandardMsg(t *testing.T) {
	for recog := int32(0); recog < 16; recog++ {
		msg := protocol.MakeDefaultMsg(protocol.SMNewMap, recog, 13, 0, 0)
		payload := protocol.EncodeMessage(msg) + protocol.EncodeString("0141")
		e, ok := classifyPayload(payload)
		if !ok {
			t.Fatalf("Recog=%d: 标准消息被丢弃 (首字符 %q)", recog, payload[0])
		}
		if e.isCtrl {
			t.Fatalf("Recog=%d: 标准消息被误判为控制消息", recog)
		}
		if e.msg.Ident != protocol.SMNewMap || e.msg.Recog != recog || e.msg.Param != 13 {
			t.Fatalf("Recog=%d: 解码错误 %+v", recog, e.msg)
		}
		if e.body != "0141" {
			t.Fatalf("Recog=%d: body=%q, want 0141", recog, e.body)
		}
	}
	// 锚定 '=' 首字符场景确实存在 (Recog=6 → 首字符 '=')。
	payload := protocol.EncodeMessage(protocol.MakeDefaultMsg(protocol.SMNewMap, 6, 13, 0, 0))
	if payload[0] != '=' {
		t.Fatalf("预期编码首字符 '=', 实际 %q", payload[0])
	}
}

func TestClassifyPayloadControlAndShort(t *testing.T) {
	if e, ok := classifyPayload("+GOOD"); !ok || !e.isCtrl || e.ctrl != "+GOOD" {
		t.Fatal("+GOOD 应分类为控制消息")
	}
	if e, ok := classifyPayload("=DIG"); !ok || !e.isCtrl || e.ctrl != "=DIG" {
		t.Fatal("=DIG 应分类为控制消息")
	}
	if _, ok := classifyPayload("=XX"); ok {
		t.Fatal("未知 '=' 短消息应丢弃")
	}
	if _, ok := classifyPayload("noise"); ok {
		t.Fatal("短噪声应丢弃")
	}
}
