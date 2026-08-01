package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// GLFW 键码 (与 go-gl/glfw 常量一致)。
const (
	keyBackspace = 259
	keyEnter     = 257
	keyTab       = 258
	keyKPEnter   = 335
	keyEscape    = 256
)

// GLFW 鼠标按键动作。
const (
	mouseRelease = 0
	mousePress   = 1
)

// loginArea 定义一个可点击区域。
type loginArea struct {
	X, Y, W, H float32
}

// loginMode 对应 Delphi TLoginState (IntroScn.pas:17)。
type loginMode int

const (
	modeLogin loginMode = iota
	modeRegister
	modeChgPw
	modeServerSelect
)

// 屏幕偏移：窗口即 Delphi 的固定 800×600 游戏区域。
const (
	loginOX = float32(0)
	loginOY = float32(0)
)

// 开门动画 (IntroScn.pas:495,826,841,845)：10 帧，每帧 300ms。
const (
	doorFrameCount = 10
	doorFrameTime  = 300 * time.Millisecond
)

// 登录面板 Prguse[60] (296×254)，居中绘制。
const loginPanelImg = 60

// 登录面板内输入框偏移（相对于面板左上角，像素扫描 Prguse[60] 精确值）。
var loginFieldOffsets = []loginArea{
	{96, 85, 140, 17},  // 账号 (black y=85..101, x=96..235)
	{96, 117, 141, 17}, // 密码 (black y=117..133, x=96..236)
}

// 登录面板内按钮偏移（相对于面板左上角）。
// 按钮视觉已烘焙在 Prguse[60] 中，仅用碰撞区域。
// 顺序：0=提交, 1=修改密码, 2=新用户, 3=关闭。
var loginBtnOffsets = []loginArea{
	{168, 165, 73, 32},  // 提交
	{131, 204, 117, 32}, // 修改密码
	{26, 204, 102, 32},  // 新用户
	{252, 28, 16, 23},   // 关闭 (X) —— 对齐 Delphi DLoginClose (FState.pas:797-798, Prguse[64]=16x23)
}

// 注册窗口输入框 (IntroScn.pas:279-410)，基准 nx=800/2-320=80,
// ny=600/2-238=62。顺序：账号、密码、确认密码、姓名、身份证、生日、
// 问题1、答案1、问题2、答案2、电话、手机、邮箱。
type fieldDef struct {
	x, y, w, h float32
	maxLen     int
	masked     bool
	password   bool // 输入时拒绝 '~', ''', ' ' (IntroScn.pas:640-641)
}

// 注册窗口基准偏移 = Prguse[63] 窗口左上角 (800×600 居中)。
const (
	regNX = 80
	regNY = 64
)

// 注册窗口输入框——像素扫描 Prguse[63] 精确值。
var regFieldDefs = []fieldDef{
	{regNX + 161, regNY + 116, 117, 16, 10, false, false}, // 账号
	{regNX + 161, regNY + 137, 117, 16, 10, true, true},   // 密码
	{regNX + 161, regNY + 158, 117, 16, 10, true, true},   // 确认密码
	{regNX + 161, regNY + 187, 117, 16, 20, false, false}, // 真实姓名
	{regNX + 161, regNY + 208, 117, 16, 14, false, false}, // 身份证号
	{regNX + 161, regNY + 229, 117, 16, 10, false, false}, // 生日
	{regNX + 161, regNY + 256, 164, 16, 20, false, false}, // 问题1
	{regNX + 161, regNY + 276, 164, 16, 12, false, false}, // 答案1
	{regNX + 161, regNY + 297, 164, 16, 20, false, false}, // 问题2
	{regNX + 161, regNY + 317, 164, 16, 12, false, false}, // 答案2
	{regNX + 161, regNY + 347, 117, 16, 14, false, false}, // 电话
	{regNX + 161, regNY + 368, 117, 16, 13, false, false}, // 手机
	{regNX + 161, regNY + 388, 166, 16, 40, false, false}, // 邮箱
}

// 注册窗口按钮——按十周年版 Prguse[63] 实际图片测量。
var regButtonAreas = []loginArea{
	{regNX + 157, regNY + 415, 82, 30}, // 提交 (对齐 Delphi DNewAccountOk: 157+1,415+1)
	{regNX + 445, regNY + 418, 82, 30}, // 取消 (对齐 Delphi DNewAccountCancel: 445+1,418+1)
	{regNX + 587, regNY + 33, 16, 23},  // 关闭 (X) (对齐 Delphi DNewAccountClose 587,33; Prguse[64]=16x23)
}

var regButtonImages = []int{62, 52, 64} // 提交按下=62, 取消按下=52, 关闭X按下=64

// 修改密码窗口输入框——按十周年版 Prguse[50] 实际图片测量。
const (
	chgNX = 190
	chgNY = 150
)

var chgFieldDefs = []fieldDef{
	{chgNX + 239, chgNY + 118, 137, 16, 10, false, false}, // 用户名 (对齐 Delphi m_EdChgId)
	{chgNX + 239, chgNY + 150, 137, 16, 10, true, false},  // 当前密码 (m_EdChgCurrentpw)
	{chgNX + 239, chgNY + 177, 137, 16, 10, true, true},   // 新密码 (m_EdChgNewPw)
	{chgNX + 239, chgNY + 208, 137, 16, 10, true, true},   // 重复密码 (m_EdChgRepeat)
}

var chgButtonImages = []int{81, 52} // 同意按下=81, 取消按下=52

// serverInfo 保存 SM_PASSOKSELECTSERVER 返回的服务器条目。
type serverInfo struct {
	Name   string
	Status int
}

// 选服对话框常量。
const (
	srvDlgImg   = 256
	srvBtnImg   = 286
	srvCloseImg = 83
	srvCloseDX  = float32(245)
	srvCloseDY  = float32(31)
	srvCloseW   = float32(20)
	srvCloseH   = float32(20)
)

func srvButtonTop(i, count int) float32 {
	switch count {
	case 1:
		return 204
	case 2:
		if i == 0 {
			return 190
		}
		return 235
	default:
		return float32(100 + i*45)
	}
}

func serverDisplayName(srv serverInfo) (string, float32, float32, float32) {
	switch srv.Status {
	case 0:
		return srv.Name + "(维护)", 0.25, 0.25, 0.25
	case 1:
		return srv.Name + "(正常)", 0, 1, 0
	case 2:
		return srv.Name + "(流畅)", 0, 0.5, 0
	case 3:
		return srv.Name + "(繁忙)", 0.5, 0, 0
	case 4:
		return srv.Name + "(满员)", 1, 0, 0
	default:
		return srv.Name, 1, 1, 0
	}
}

func parseServerList(body string) []serverInfo {
	var servers []serverInfo
	if body == "" {
		servers = append(servers, serverInfo{Name: "Server", Status: 1})
		return servers
	}
	parts := splitSlash(body)
	for i := 0; i+1 < len(parts); i += 2 {
		name := parts[i]
		status := 0
		fmt.Sscanf(parts[i+1], "%d", &status)
		if name != "" {
			servers = append(servers, serverInfo{Name: name, Status: status})
		}
	}
	if len(servers) == 0 {
		servers = append(servers, serverInfo{Name: "Server", Status: 1})
	}
	return servers
}

func splitSlash(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

var regHelps = [13]string{
	"请输入账号(必填)",
	"请输入密码(至少4位)",
	"请再次输入密码确认",
	"请输入真实姓名",
	"请输入身份证号",
	"生日格式: 1977/10/15",
	"请输入密码提示问题",
	"请输入提示问题的答案",
	"请输入第二个提示问题",
	"请输入第二个问题的答案",
	"请输入电话号码",
	"请输入手机号码",
	"请输入邮箱地址",
}

// 选服按钮显示尺寸。
const (
	srvBtnDispW = float32(180)
	srvBtnDispH = float32(35)
)

func srvBtnY(i, count int) float32 {
	firstY := float32(130)
	if count <= 2 {
		firstY = 180
	} else if count == 3 {
		firstY = 155
	}
	return firstY + float32(i)*42
}

// dlgGeom 保存各尺寸 DMessageDlg 的布局参数。
type dlgGeom struct {
	bgImg                int
	msgLX, msgLY         float32
	btnLX, btnLY         float32
}

var dlgSizes = [3]dlgGeom{
	{bgImg: 381, msgLX: 39, msgLY: 38, btnLX: 90, btnLY: 36},   // 小
	{bgImg: 360, msgLX: 39, msgLY: 38, btnLX: 324, btnLY: 126}, // 中
	{bgImg: 380, msgLX: 23, msgLY: 20, btnLX: 105, btnLY: 305}, // 大
}

// LoginScene 处理登录界面 (Delphi TLoginScene, IntroScn.pas:62)。
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer
	textSmall *engine.TextRenderer
	textSrv   *engine.TextRenderer

	ui *UIManager

	// 开门动画
	doorOpening   bool
	doorFading    bool
	doorFrame     int
	doorStartTime time.Time

	// 模式
	mode        loginMode
	showLoginUI bool

	connecting bool
	regCache   [13]string

	// 控件引用
	loginPanel *UIControl
	regPanel   *UIControl
	chgPanel   *UIControl
	srvPanel   *UIControl
	editID     *EditBox
	editPW     *EditBox
	regEdits   [13]*EditBox
	chgEdits   [4]*EditBox
	srvBtns    []*UIControl

	// 选服
	servers []serverInfo

	// 回调
	loginFunc        func(id, password string)
	registerFunc     func(ue protocol.UserEntry, ua protocol.UserEntryAdd)
	chgpwFunc        func(id, oldpw, newpw string)
	selectFunc       func(serverName string)
	closeFunc        func()
	doorCompleteFunc func()
}

// NewLoginScene 创建登录场景。
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *LoginScene {
	s := &LoginScene{
		gl:          gl,
		resources:   resources,
		text:        text,
		textSmall:   text,
		textSrv:     text,
		showLoginUI: true,
	}
	if t, err := text.WithSize(13); err == nil {
		s.textSmall = t
	}
	if t, err := text.WithSize(15); err == nil {
		s.textSrv = t
	}
	return s
}

// Open 在场景激活时调用。
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "opened")
	gSound.PlayBGM(bmgIntro)
	s.ui = NewUIManager(s.gl, s.resources, s.textSmall)
	s.buildLoginUI()
	gActiveUI = s.ui
	s.mode = modeLogin
	s.showLoginUI = true
	s.doorOpening = false
	s.doorFading = false
	s.doorFrame = 0
	s.connecting = false
	s.servers = nil
	s.registerDebugCmds()
	if s.editID != nil {
		s.ui.SetFocus(s.editID.Ctrl)
	}
}

// Close 在场景失活时调用。
func (s *LoginScene) Close() {
	s.unregisterDebugCmds()
	gActiveUI = nil
	gSound.SilenceSound()
	log.Logf(log.LevelInfo, "LoginScene", "closed")
}

// Update 推进开门动画。
func (s *LoginScene) Update(dt float64) {
	if !s.doorOpening {
		return
	}
	if !s.doorFading {
		if time.Since(s.doorStartTime) > doorFrameTime {
			s.doorStartTime = time.Now()
			if s.doorFrame < doorFrameCount-1 {
				s.doorFrame++
			} else {
				s.doorFading = true
				log.Logf(log.LevelInfo, "LoginScene", "door animation done, starting fade-out")
				globalFade.startOut(false, func() {
					if s.doorCompleteFunc != nil {
						s.doorCompleteFunc()
						s.doorCompleteFunc = nil
					}
				})
			}
		}
	}
}

// Render 渲染登录场景。
func (s *LoginScene) Render(gl *engine.GLState, proj [16]float32) {
	s.drawBackground(proj)
	if s.doorOpening {
		s.drawDoor(proj)
	}
	s.syncUI()
	if s.ui != nil {
		s.ui.Paint(proj)
	}
}

func (s *LoginScene) drawBackground(proj [16]float32) {
	bgTex, bgErr := s.getChrSelTexture(22)
	if bgErr == nil && bgTex != 0 {
		w, h := s.getChrSelSize(22)
		s.gl.DrawQuad(bgTex, loginOX, loginOY, float32(w), float32(h), proj)
	} else {
		s.gl.DrawQuadColor(0, 0, 800, 600, 0.05, 0.05, 0.1, 1, proj)
	}
}

func (s *LoginScene) drawDoor(proj [16]float32) {
	doorIdx := 23 + s.doorFrame
	if doorTex, err := s.getChrSelTexture(doorIdx); err == nil {
		w, h := s.getChrSelSize(doorIdx)
		s.gl.DrawQuad(doorTex, loginOX+152, loginOY+96, float32(w), float32(h), proj)
	}
}

func (s *LoginScene) syncUI() {
	show := s.showLoginUI && !s.doorOpening
	if s.loginPanel != nil {
		s.loginPanel.Visible = show && s.mode == modeLogin
	}
	if s.regPanel != nil {
		s.regPanel.Visible = show && s.mode == modeRegister
	}
	if s.chgPanel != nil {
		s.chgPanel.Visible = show && s.mode == modeChgPw
	}
	if s.srvPanel != nil {
		s.srvPanel.Visible = show && s.mode == modeServerSelect
	}
}

// ---------------------------------------------------------------------------
// 控件树构建
// ---------------------------------------------------------------------------

func (s *LoginScene) buildLoginUI() {
	prg := s.resources.Prguse

	// --- 登录面板 ---
	lp := NewUIControl("DLoginPanel", KindWindow)
	if prg != nil {
		lp.SetImgIndex(prg, loginPanelImg)
	} else {
		lp.Width, lp.Height = 296, 254
	}
	lp.Left = (ScreenWidth - lp.Width) / 2
	lp.Top = (ScreenHeight - lp.Height) / 2
	lp.Visible = false
	s.ui.Root.AddChild(lp)
	s.loginPanel = lp

	// 输入框
	fo := loginFieldOffsets[0]
	s.editID = NewEditBox(s.gl, s.textSmall, "EdLoginID", int(fo.W), int(fo.H))
	s.editID.MaxLen = 10
	s.editID.Ctrl.Left = int(fo.X)
	s.editID.Ctrl.Top = int(fo.Y)
	s.editID.OnEnter = func(string) { s.ui.SetFocus(s.editPW.Ctrl) }
	lp.AddChild(s.editID.Ctrl)

	fp := loginFieldOffsets[1]
	s.editPW = NewEditBox(s.gl, s.textSmall, "EdLoginPW", int(fp.W), int(fp.H))
	s.editPW.MaxLen = 10
	s.editPW.Masked = true
	s.editPW.Ctrl.Left = int(fp.X)
	s.editPW.Ctrl.Top = int(fp.Y)
	s.editPW.OnEnter = func(string) { s.submitLogin() }
	lp.AddChild(s.editPW.Ctrl)

	// 按钮（不可见，覆盖在烘焙图片上）
	btnNames := [4]string{"BtnOK", "BtnChangePW", "BtnNewAccount", "BtnClose"}
	for i, off := range loginBtnOffsets {
		btn := NewUIControl(btnNames[i], KindButton)
		btn.Left = int(off.X)
		btn.Top = int(off.Y)
		btn.Width = int(off.W)
		btn.Height = int(off.H)
		btn.ClickSound = sRockButtonClick
		idx := i
		btn.OnClick = func(c *UIControl, x, y int) { s.handleLoginButton(idx) }
		lp.AddChild(btn)
	}

	// --- 注册面板 ---
	rp := NewUIControl("DRegPanel", KindWindow)
	if prg != nil {
		rp.SetImgIndex(prg, 63)
	} else {
		rp.Width, rp.Height = 640, 472
	}
	rp.Left = (ScreenWidth - rp.Width) / 2
	rp.Top = (ScreenHeight - rp.Height) / 2
	rp.Visible = false
	s.ui.Root.AddChild(rp)
	s.regPanel = rp

	for i, def := range regFieldDefs {
		eb := NewEditBox(s.gl, s.textSmall, fmt.Sprintf("EdReg%d", i), int(def.w), int(def.h))
		eb.MaxLen = def.maxLen
		eb.Masked = def.masked
		eb.Ctrl.Left = int(def.x) - rp.Left
		eb.Ctrl.Top = int(def.y) - rp.Top
		fieldIdx := i
		eb.OnEnter = func(string) { s.regFieldEnter(fieldIdx) }
		eb.OnEsc = func() { s.backToLogin() }
		if def.password {
			origOnChar := eb.Ctrl.OnChar
			eb.Ctrl.OnChar = func(c *UIControl, ch rune) {
				if ch == '~' || ch == '\'' || ch == ' ' {
					return
				}
				origOnChar(c, ch)
			}
		}
		rp.AddChild(eb.Ctrl)
		s.regEdits[i] = eb
	}

	regBtnNames := [3]string{"BtnRegOk", "BtnRegCancel", "BtnRegClose"}
	for i, area := range regButtonAreas {
		btn := NewUIControl(regBtnNames[i], KindButton)
		if prg != nil {
			btn.SetImgIndex(prg, regButtonImages[i])
		} else {
			btn.Width, btn.Height = int(area.W), int(area.H)
		}
		btn.Left = int(area.X) - rp.Left
		btn.Top = int(area.Y) - rp.Top
		btn.ClickSound = sRockButtonClick
		idx := i
		btn.OnClick = func(c *UIControl, x, y int) { s.handleRegButton(idx) }
		// 休息态烘焙在 Prguse[63], 仅按下绘制凹陷图; 石按钮 +1 行程, 关闭X 图已内凹不偏移
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if c.Downed {
				dx, dy := c.AbsX(), c.AbsY()
				if idx < 2 {
					dx, dy = dx+1, dy+1
				}
				s.ui.BlitImage(prg, regButtonImages[idx], dx, dy, proj)
			}
		}
		rp.AddChild(btn)
	}

	// 注册帮助文本
	rp.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if prg != nil {
			s.ui.BlitImage(prg, 63, c.AbsX(), c.AbsY(), proj)
		}
		focused := s.ui.Focused
		for i, eb := range s.regEdits {
			if focused == eb.Ctrl && i < len(regHelps) {
				s.textSmall.DrawText(regHelps[i], float32(c.AbsX()+390), float32(c.AbsY()+90),
					0.75, 0.75, 0.75, 1, proj)
				break
			}
		}
		if s.connecting && s.text != nil {
			s.text.DrawText("注册中...", float32(c.AbsX()+350), float32(c.AbsY()+420),
				0.5, 0.8, 1.0, 1.0, proj)
		}
	}

	// --- 修改密码面板 ---
	cp := NewUIControl("DChgPanel", KindWindow)
	if prg != nil {
		cp.SetImgIndex(prg, 50)
	} else {
		cp.Width, cp.Height = 420, 300
	}
	cp.Left = (ScreenWidth - cp.Width) / 2
	cp.Top = (ScreenHeight - cp.Height) / 2
	cp.Visible = false
	s.ui.Root.AddChild(cp)
	s.chgPanel = cp

	for i, def := range chgFieldDefs {
		eb := NewEditBox(s.gl, s.textSmall, fmt.Sprintf("EdChg%d", i), int(def.w), int(def.h))
		eb.MaxLen = def.maxLen
		eb.Masked = def.masked
		eb.Ctrl.Left = int(def.x) - cp.Left
		eb.Ctrl.Top = int(def.y) - cp.Top
		fieldIdx := i
		eb.OnEnter = func(string) { s.chgFieldEnter(fieldIdx) }
		eb.OnEsc = func() { s.backToLogin() }
		if def.password {
			origOnChar := eb.Ctrl.OnChar
			eb.Ctrl.OnChar = func(c *UIControl, ch rune) {
				if ch == '~' || ch == '\'' || ch == ' ' {
					return
				}
				origOnChar(c, ch)
			}
		}
		cp.AddChild(eb.Ctrl)
		s.chgEdits[i] = eb
	}

	// 修改密码按钮
	chgBtnOffsets := []struct{ x, y int }{{180, 252}, {275, 251}} // 对齐 Delphi DChgpwOk 180+1,252+1 / DChgPwCancel 275+1,251+1
	chgBtnNames := [2]string{"BtnChgOk", "BtnChgCancel"}
	for i, off := range chgBtnOffsets {
		btn := NewUIControl(chgBtnNames[i], KindButton)
		if prg != nil {
			btn.SetImgIndex(prg, chgButtonImages[i])
		} else {
			btn.Width, btn.Height = 70, 26
		}
		btn.Left = off.x
		btn.Top = off.y
		btn.ClickSound = sRockButtonClick
		idx := i
		btn.OnClick = func(c *UIControl, x, y int) {
			if idx == 0 {
				s.submitChgPw()
			} else {
				s.backToLogin()
			}
		}
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if c.Downed { // 休息态烘焙在 Prguse[50], 仅按下绘制凹陷图(+1 行程)
				s.ui.BlitImage(prg, chgButtonImages[idx], c.AbsX()+1, c.AbsY()+1, proj)
			}
		}
		cp.AddChild(btn)
	}

	// --- 选服面板 ---
	sp := NewUIControl("DSrvPanel", KindWindow)
	if prg != nil {
		sp.SetImgIndex(prg, srvDlgImg)
	} else {
		sp.Width, sp.Height = 300, 450
	}
	sp.Left = (ScreenWidth - sp.Width) / 2
	sp.Top = (ScreenHeight - sp.Height) / 2
	sp.Visible = false
	s.ui.Root.AddChild(sp)
	s.srvPanel = sp

	// 关闭按钮
	srvClose := NewUIControl("BtnSrvClose", KindButton)
	if prg != nil {
		srvClose.SetImgIndex(prg, srvCloseImg)
	} else {
		srvClose.Width, srvClose.Height = int(srvCloseW), int(srvCloseH)
	}
	srvClose.Left = int(srvCloseDX)
	srvClose.Top = int(srvCloseDY)
	srvClose.OnClick = func(c *UIControl, x, y int) {
		if s.closeFunc != nil {
			s.closeFunc()
		}
	}
	sp.AddChild(srvClose)

	// 服务器按钮（最多6个）
	s.srvBtns = make([]*UIControl, 6)
	for i := 0; i < 6; i++ {
		btn := NewUIControl(fmt.Sprintf("BtnSrv%d", i), KindButton)
		btn.Width = int(srvBtnDispW)
		btn.Height = int(srvBtnDispH)
		btn.Visible = false
		srvIdx := i
		btn.OnClick = func(c *UIControl, x, y int) { s.selectServer(srvIdx) }
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			s.paintSrvButton(c, srvIdx, proj)
		}
		sp.AddChild(btn)
		s.srvBtns[i] = btn
	}
}

func (s *LoginScene) paintSrvButton(c *UIControl, idx int, proj [16]float32) {
	if idx >= len(s.servers) {
		return
	}
	prg := s.resources.Prguse
	imgIdx := srvBtnImg
	if c.Downed {
		imgIdx = srvBtnImg + 1
	}
	if prg != nil {
		img := prg.GetImage(imgIdx)
		if img != nil {
			tex := s.resources.GetTexture(prg, imgIdx)
			if tex != 0 {
				s.gl.DrawQuad(tex, float32(c.AbsX()), float32(c.AbsY()), srvBtnDispW, srvBtnDispH, proj)
			}
		}
	}
	if s.textSrv != nil {
		name, r, g, b := serverDisplayName(s.servers[idx])
		tw := float32(s.textSrv.MeasureText(name))
		th := float32(s.textSrv.LineHeight())
		tx := float32(c.AbsX()) + (srvBtnDispW-tw)/2
		ty := float32(c.AbsY()) + (srvBtnDispH-th)/2
		if c.Downed {
			tx += 1
			ty += 1
		}
		s.textSrv.DrawTextBoldOutline(name, tx, ty, r, g, b, 1, 0, 0, 0, 1, proj)
	}
}

// layoutSrvButtons 根据当前服务器列表更新按钮位置和可见性。
func (s *LoginScene) layoutSrvButtons() {
	count := len(s.servers)
	bxOff := (s.srvPanel.Width - int(srvBtnDispW)) / 2
	for i := 0; i < 6; i++ {
		btn := s.srvBtns[i]
		if i < count {
			btn.Visible = true
			btn.Left = bxOff
			btn.Top = int(srvBtnY(i, count))
		} else {
			btn.Visible = false
		}
	}
}

// ---------------------------------------------------------------------------
// 输入路由
// ---------------------------------------------------------------------------

// OnChar 处理 GLFW 字符输入。
func (s *LoginScene) OnChar(char rune) {
	if !s.showLoginUI || s.doorOpening || s.connecting {
		return
	}
	if s.ui != nil {
		s.ui.RouteChar(char)
	}
}

// OnKey 处理键盘输入。
func (s *LoginScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	if !s.showLoginUI || s.doorOpening {
		return
	}
	if s.ui == nil {
		return
	}
	if s.ui.Modal != nil {
		s.ui.RouteKeyDown(key)
		return
	}
	if s.mode == modeServerSelect && key == keyEscape {
		if s.closeFunc != nil {
			s.closeFunc()
		}
		return
	}
	if key == keyTab {
		s.cycleFocus()
		return
	}
	if !s.connecting {
		s.ui.RouteKeyDown(key)
	}
}

// OnMouse 处理鼠标按键输入。
func (s *LoginScene) OnMouse(x, y float64, button int, action int, mods int) {
	if !s.showLoginUI || s.doorOpening {
		return
	}
	if s.ui == nil {
		return
	}
	switch action {
	case mousePress:
		s.ui.RouteMouseDown(int(x), int(y), button)
	case mouseRelease:
		s.ui.RouteMouseUp(int(x), int(y), button)
	}
}

// OnScroll 处理鼠标滚轮输入。
func (s *LoginScene) OnScroll(x, y float64) {}

func (s *LoginScene) cycleFocus() {
	var edits []*EditBox
	switch s.mode {
	case modeLogin:
		edits = []*EditBox{s.editID, s.editPW}
	case modeRegister:
		edits = s.regEdits[:]
	case modeChgPw:
		edits = s.chgEdits[:]
	default:
		return
	}
	if len(edits) == 0 || s.ui == nil {
		return
	}
	idx := -1
	for i, e := range edits {
		if e != nil && s.ui.Focused == e.Ctrl {
			idx = i
			break
		}
	}
	next := (idx + 1) % len(edits)
	if edits[next] != nil {
		s.ui.SetFocus(edits[next].Ctrl)
	}
}

// ---------------------------------------------------------------------------
// 按钮/字段回调
// ---------------------------------------------------------------------------

func (s *LoginScene) backToLogin() {
	s.mode = modeLogin
	if s.editID != nil && s.ui != nil {
		s.ui.SetFocus(s.editID.Ctrl)
	}
}

func (s *LoginScene) handleLoginButton(index int) {
	switch index {
	case 0:
		s.submitLogin()
	case 1:
		s.mode = modeChgPw
		for _, eb := range s.chgEdits {
			eb.Clear()
		}
		s.ui.SetFocus(s.chgEdits[0].Ctrl)
	case 2:
		s.mode = modeRegister
		for _, eb := range s.regEdits {
			eb.Clear()
		}
		s.ui.SetFocus(s.regEdits[0].Ctrl)
	case 3:
		if s.closeFunc != nil {
			s.closeFunc()
		}
	}
}

func (s *LoginScene) handleRegButton(index int) {
	if index == 0 {
		s.submitRegister()
		return
	}
	s.backToLogin()
}

func (s *LoginScene) regFieldEnter(idx int) {
	if !s.validateRegField(idx) {
		return
	}
	next := (idx + 1) % len(s.regEdits)
	s.ui.SetFocus(s.regEdits[next].Ctrl)
}

func (s *LoginScene) chgFieldEnter(idx int) {
	if !s.validateChgField(idx) {
		return
	}
	next := (idx + 1) % len(s.chgEdits)
	s.ui.SetFocus(s.chgEdits[next].Ctrl)
}

func (s *LoginScene) selectServer(idx int) {
	if idx >= len(s.servers) {
		return
	}
	name := s.servers[idx].Name
	s.mode = modeLogin
	if s.selectFunc != nil {
		s.selectFunc(name)
	}
}

// ---------------------------------------------------------------------------
// 提交逻辑
// ---------------------------------------------------------------------------

func (s *LoginScene) submitLogin() {
	if s.connecting {
		return
	}
	id := s.editID.Text
	pw := s.editPW.Text
	if id == "" || pw == "" {
		s.ShowMessage("请输入账号和密码")
		return
	}
	if s.loginFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}
	s.connecting = true
	pw = strings.ReplaceAll(strings.ReplaceAll(pw, "~", "_"), "'", "_")
	id = strings.ToLower(id)
	log.Logf(log.LevelInfo, "LoginScene", "submit login: %s", id)
	s.loginFunc(id, pw)
}

func (s *LoginScene) validateRegField(idx int) bool {
	fields := make([]string, 13)
	for i, eb := range s.regEdits {
		fields[i] = eb.Text
	}
	switch idx {
	case 0:
		fields[0] = strings.TrimSpace(fields[0])
		s.regEdits[0].Text = fields[0]
		if len(fields[0]) < 3 {
			s.ShowMessage("输入账号的长度必须至少3位.")
			return false
		}
	case 1:
		if len(fields[1]) < 4 {
			s.ShowMessage("密码长度必须至少 4位.")
			return false
		}
	case 2:
		if fields[1] != fields[2] {
			s.ShowMessage("两次输入的密码不一致，请重新输入.")
			return false
		}
	case 5:
		if !validBirthDay(fields[5]) {
			s.ui.SetFocus(s.regEdits[5].Ctrl)
			return false
		}
	default:
		fields[idx] = strings.TrimSpace(fields[idx])
		s.regEdits[idx].Text = fields[idx]
		if fields[idx] == "" {
			return false
		}
	}
	return true
}

func (s *LoginScene) validateChgField(idx int) bool {
	if s.chgEdits[idx].Text == "" {
		return false
	}
	if idx == 3 && s.chgEdits[2].Text != s.chgEdits[3].Text {
		s.ShowMessage("两次确认不一致确认.")
		s.ui.SetFocus(s.chgEdits[2].Ctrl)
		return false
	}
	return true
}

func (s *LoginScene) submitRegister() {
	if s.connecting {
		return
	}
	fields := make([]string, 13)
	for i, eb := range s.regEdits {
		fields[i] = eb.Text
	}
	fields[0] = strings.TrimSpace(fields[0])
	fields[3] = strings.TrimSpace(fields[3])
	fields[6] = strings.TrimSpace(fields[6])
	s.regEdits[0].Text = fields[0]
	s.regEdits[3].Text = fields[3]
	s.regEdits[6].Text = fields[6]

	if len(fields[0]) < 3 {
		s.ShowMessage("输入账号的长度必须至少3位.")
		s.ui.SetFocus(s.regEdits[0].Ctrl)
		return
	}
	if !validBirthDay(fields[5]) {
		s.ui.SetFocus(s.regEdits[5].Ctrl)
		return
	}
	if len(fields[1]) < 3 {
		s.ui.SetFocus(s.regEdits[1].Ctrl)
		return
	}
	if fields[1] != fields[2] {
		s.ui.SetFocus(s.regEdits[2].Ctrl)
		return
	}
	if len(fields[6]) < 1 {
		s.ui.SetFocus(s.regEdits[6].Ctrl)
		return
	}
	if len(fields[7]) < 1 {
		s.ui.SetFocus(s.regEdits[7].Ctrl)
		return
	}
	if len(fields[8]) < 1 {
		s.ui.SetFocus(s.regEdits[8].Ctrl)
		return
	}
	if len(fields[9]) < 1 {
		s.ui.SetFocus(s.regEdits[9].Ctrl)
		return
	}
	if len(fields[3]) < 1 {
		s.ui.SetFocus(s.regEdits[3].Ctrl)
		return
	}
	if len(fields[4]) < 1 {
		s.ui.SetFocus(s.regEdits[4].Ctrl)
		return
	}
	if s.registerFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}

	var ue protocol.UserEntry
	var ua protocol.UserEntryAdd
	ue.SetAccount(strings.ToLower(fields[0]))
	ue.SetPassword(fields[1])
	ue.SetUserName(fields[3])
	ue.SetSSNo(fields[4])
	ue.SetQuiz(fields[6])
	ue.SetAnswer(strings.TrimSpace(fields[7]))
	ue.SetPhone(fields[10])
	ue.SetEMail(strings.TrimSpace(fields[12]))
	ua.SetQuiz2(fields[8])
	ua.SetAnswer2(strings.TrimSpace(fields[9]))
	ua.SetBirthDay(fields[5])
	ua.SetMobilePhone(fields[11])

	log.Logf(log.LevelInfo, "LoginScene", "submit register: %s", ue.Account())
	for i, f := range fields {
		s.regCache[i] = f
	}
	s.connecting = true
	s.registerFunc(ue, ua)
	s.mode = modeLogin
}

func validBirthDay(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return false
	}
	year, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	day, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	if year <= 1890 || year > 2101 {
		return false
	}
	if month <= 0 || month > 12 {
		return false
	}
	return day > 0 && day <= 31
}

func (s *LoginScene) submitChgPw() {
	if s.connecting {
		return
	}
	if s.chgEdits[2].Text != s.chgEdits[3].Text {
		s.ShowMessage("两次确认不一致确认.")
		s.ui.SetFocus(s.chgEdits[2].Ctrl)
		return
	}
	if s.chgpwFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}
	log.Logf(log.LevelInfo, "LoginScene", "submit change password: %s", s.chgEdits[0].Text)
	s.connecting = true
	s.chgpwFunc(s.chgEdits[0].Text, s.chgEdits[1].Text, s.chgEdits[2].Text)
	s.mode = modeLogin
	for _, eb := range s.chgEdits {
		eb.Clear()
	}
}

// ---------------------------------------------------------------------------
// 模态对话框
// ---------------------------------------------------------------------------

func (s *LoginScene) showModal(msg string, size int, btnImgs []int) {
	if s.ui == nil {
		return
	}
	g := dlgSizes[size]
	prg := s.resources.Prguse

	win := NewUIControl("DLoginDlg", KindWindow)
	win.Floating = true
	if prg != nil {
		win.SetImgIndex(prg, g.bgImg)
	} else {
		win.Width, win.Height = 300, 170
	}
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2

	lines := strings.Split(msg, "\\")
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		if prg != nil {
			s.ui.BlitImage(prg, g.bgImg, c.AbsX(), c.AbsY(), proj)
		}
		if s.text == nil {
			return
		}
		y := float32(c.AbsY()) + g.msgLY
		for _, ln := range lines {
			s.text.DrawTextBoldOutline(ln, float32(c.AbsX())+g.msgLX, y, 1, 1, 1, 1, 0, 0, 0, 1, proj)
			y += 14
		}
	}

	win.OnKeyDown = func(c *UIControl, key int) {
		if key == keyEnter || key == keyKPEnter || key == keyEscape {
			s.closeModal(win)
		}
	}

	// 按钮从 btnLX 起从右向左排列，间距 110px
	lx := int(g.btnLX)
	for _, img := range btnImgs {
		btn := NewUIControl("DDlgBtn", KindButton)
		if prg != nil {
			btn.SetImgIndex(prg, img)
		} else {
			btn.Width, btn.Height = 70, 26
		}
		btn.Left = lx
		btn.Top = int(g.btnLY)
		btnImg := img
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			idx := btnImg
			if c.Downed {
				idx++
			}
			if prg != nil {
				s.ui.BlitImage(prg, idx, c.AbsX(), c.AbsY(), proj)
			}
		}
		btn.OnClick = func(c *UIControl, x, y int) { s.closeModal(win) }
		win.AddChild(btn)
		lx -= 110
	}

	s.ui.ShowModal(win)
}

func (s *LoginScene) closeModal(win *UIControl) {
	if s.ui != nil {
		s.ui.CloseModal(win)
	}
	s.connecting = false
	if s.mode == modeLogin && s.editID != nil {
		s.ui.SetFocus(s.editID.Ctrl)
	}
}

// ShowMessage 显示中等尺寸、仅确定按钮的模态对话框。
func (s *LoginScene) ShowMessage(msg string) {
	s.ShowMessageEx(msg, 1, []int{361})
}

// ShowMessageEx 显示指定尺寸的模态对话框。
func (s *LoginScene) ShowMessageEx(msg string, size int, buttons []int) {
	s.connecting = false
	s.showModal(msg, size, buttons)
}

// SetError 保留给现有调用方。
func (s *LoginScene) SetError(msg string) {
	log.Logf(log.LevelWarn, "LoginScene", "error: %s", msg)
	s.ShowMessage(msg)
}

// RegistrationDone 处理 SM_NEWID_SUCCESS。
func (s *LoginScene) RegistrationDone() {
	s.mode = modeLogin
	for _, eb := range s.regEdits {
		eb.Clear()
	}
	s.ShowMessage("您的账号已经注册成功.\\请牢记您的账号和密码.\\请不要以任何原因将账号和密码告诉任何人.")
}

// RegistrationFailed 处理 SM_NEWID_FAIL：返回注册模式并恢复已填字段。
func (s *LoginScene) RegistrationFailed(msg string) {
	s.mode = modeRegister
	for i, eb := range s.regEdits {
		eb.Text = s.regCache[i]
	}
	if s.regEdits[0] != nil && s.ui != nil {
		s.ui.SetFocus(s.regEdits[0].Ctrl)
	}
	s.ShowMessage(msg)
}

// ChgPwResult 显示修改密码结果对话框。
func (s *LoginScene) ChgPwResult(msg string) {
	s.ShowMessage(msg)
}

// ---------------------------------------------------------------------------
// 回调设置
// ---------------------------------------------------------------------------

func (s *LoginScene) SetLoginFunc(fn func(id, password string))       { s.loginFunc = fn }
func (s *LoginScene) SetRegisterFunc(fn func(ue protocol.UserEntry, ua protocol.UserEntryAdd)) {
	s.registerFunc = fn
}
func (s *LoginScene) SetChgPwFunc(fn func(id, oldpw, newpw string)) { s.chgpwFunc = fn }
func (s *LoginScene) SetSelectFunc(fn func(serverName string))      { s.selectFunc = fn }
func (s *LoginScene) SetCloseFunc(fn func())                        { s.closeFunc = fn }
func (s *LoginScene) SetDoorCompleteFunc(fn func())                 { s.doorCompleteFunc = fn }

// ShowServerSelect 在登录背景上方打开选服浮层。
func (s *LoginScene) ShowServerSelect(servers []serverInfo) {
	log.Logf(log.LevelInfo, "LoginScene", "show server select: %d servers", len(servers))
	s.servers = servers
	s.mode = modeServerSelect
	s.showLoginUI = true
	s.layoutSrvButtons()
}

// Servers 返回已保存的服务器列表。
func (s *LoginScene) Servers() []serverInfo { return s.servers }

// OpenLoginDoor 开始开门动画。
func (s *LoginScene) OpenLoginDoor() {
	log.Logf(log.LevelInfo, "LoginScene", "starting door animation")
	gSound.PlaySound(sRockDoorOpen)
	s.doorOpening = true
	s.doorFading = false
	s.doorFrame = 0
	s.doorStartTime = time.Now()
	s.showLoginUI = false
}

// IsDoorFullyOpen 在最后一帧门画面显示后返回 true。
func (s *LoginScene) IsDoorFullyOpen() bool {
	return s.doorOpening && s.doorFrame >= doorFrameCount-1
}

// ---------------------------------------------------------------------------
// 资源辅助
// ---------------------------------------------------------------------------

func (s *LoginScene) getChrSelTexture(index int) (uint32, error) {
	if s.resources.ChrSel == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.ChrSel, index), nil
}

func (s *LoginScene) getChrSelSize(index int) (int, int) {
	if s.resources.ChrSel == nil || index >= s.resources.ChrSel.Count {
		return 0, 0
	}
	img := s.resources.ChrSel.GetImage(index)
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

func (s *LoginScene) getPrguseTexture(index int) (uint32, error) {
	if s.resources.Prguse == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.Prguse, index), nil
}

func (s *LoginScene) getPrguseSize(index int) (int, int) {
	if s.resources.Prguse == nil || index >= s.resources.Prguse.Count {
		return 0, 0
	}
	img := s.resources.Prguse.GetImage(index)
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

// hitTest 检查 (x, y) 是否在区域内。
func hitTest(x, y float32, a loginArea) bool {
	return x >= a.X && x <= a.X+a.W && y >= a.Y && y <= a.Y+a.H
}

// ---------------------------------------------------------------------------
// 调试命令
// ---------------------------------------------------------------------------

var loginModeNames = [...]string{"login", "register", "chgpw", "serverselect"}

func (s *LoginScene) registerDebugCmds() {
	dc := gDebug
	if dc == nil {
		return
	}
	dc.Register("lstate", "dump login scene state", func(args []string) {
		dc.Printf("mode=%s showUI=%v connecting=%v",
			loginModeNames[s.mode], s.showLoginUI, s.connecting)
		dc.Printf("door: opening=%v fading=%v frame=%d/%d",
			s.doorOpening, s.doorFading, s.doorFrame, doorFrameCount)
		dc.Printf("servers: %d", len(s.servers))
		if s.ui != nil {
			dc.Printf("ui: focus=%s modal=%s", debugCtlName(s.ui.Focused), debugCtlName(s.ui.Modal))
		}
	})
	dc.Register("lmode", "lmode <login|reg|chgpw|server> — force mode", func(args []string) {
		if len(args) == 0 {
			dc.Printf("usage: lmode <login|reg|chgpw|server>")
			return
		}
		switch strings.ToLower(args[0]) {
		case "login":
			s.mode = modeLogin
		case "reg", "register":
			s.mode = modeRegister
		case "chgpw":
			s.mode = modeChgPw
		case "server", "serverselect":
			s.mode = modeServerSelect
		default:
			dc.Printf("unknown mode: %s", args[0])
			return
		}
		s.showLoginUI = true
		s.doorOpening = false
		dc.Printf("mode -> %s", loginModeNames[s.mode])
	})
	dc.Register("ldoor", "ldoor [skip] — trigger/skip door animation", func(args []string) {
		if len(args) >= 1 && args[0] == "skip" {
			s.doorOpening = false
			s.doorFading = false
			s.doorFrame = doorFrameCount - 1
			s.showLoginUI = true
			dc.Printf("door skipped")
			return
		}
		s.OpenLoginDoor()
		dc.Printf("door animation started")
	})
	dc.Register("ldlg", "ldlg [msg] — show/dismiss modal dialog", func(args []string) {
		if len(args) == 0 {
			if s.ui != nil && s.ui.Modal != nil {
				s.ui.CloseModal(s.ui.Modal)
			}
			dc.Printf("dialog dismissed")
			return
		}
		s.ShowMessage(strings.Join(args, " "))
		dc.Printf("dialog shown")
	})
}

func (s *LoginScene) unregisterDebugCmds() {
	if gDebug == nil {
		return
	}
	for _, name := range []string{"lstate", "lmode", "ldoor", "ldlg"} {
		gDebug.Unregister(name)
	}
}
