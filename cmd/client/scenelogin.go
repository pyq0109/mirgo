package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// 登录输入框 (IntroScn.pas:499-512)。
var inputFields = []loginArea{
	{loginOX + 255, loginOY + 511, 112, 19}, // 账号
	{loginOX + 495, loginOY + 511, 112, 19}, // 密码
}

// 底部按钮 (FState.pas:763-774)。仅绘制按下态按钮；
// 常态面板已烘焙在 ChrSel.wil[22] 背景中。
var buttonAreas = []loginArea{
	{loginOX + 90, loginOY + 558, 70, 20},  // 确定 [62]
	{loginOX + 268, loginOY + 558, 70, 20}, // 修改密码 [53]
	{loginOX + 447, loginOY + 558, 70, 20}, // 新建账号 [61]
	{loginOX + 613, loginOY + 558, 70, 20}, // 关闭 [64]
}

var buttonImages = []int{62, 53, 61, 64}

// 注册窗口输入框 (IntroScn.pas:279-410)，基准 nx=800/2-320=80,
// ny=600/2-238=62。顺序：账号、密码、确认密码、姓名、身份证、生日、
// 问题1、答案1、问题2、答案2、电话、手机、邮箱。
type fieldDef struct {
	x, y, w, h float32
	maxLen     int
	masked     bool
	password   bool // 输入时拒绝 '~', ''', ' ' (IntroScn.pas:640-641)
}

const (
	regNX = 80
	regNY = 62
)

var regFieldDefs = []fieldDef{
	{regNX + 86, regNY + 91, 104, 13, 10, false, false},   // 账号
	{regNX + 86, regNY + 118, 104, 13, 10, true, true},    // 密码
	{regNX + 86, regNY + 149, 104, 12, 10, true, true},    // 确认密码
	{regNX + 86, regNY + 190, 105, 13, 20, false, false},  // 真实姓名
	{regNX + 86, regNY + 207, 105, 13, 14, false, false},  // 身份证号
	{regNX + 86, regNY + 217, 105, 13, 10, false, false},  // 生日
	{regNX + 263, regNY + 118, 124, 13, 20, false, false}, // 问题1
	{regNX + 263, regNY + 149, 124, 12, 12, false, false}, // 答案1
	{regNX + 263, regNY + 190, 124, 13, 20, false, false}, // 问题2
	{regNX + 263, regNY + 218, 124, 13, 12, false, false}, // 答案2
	{regNX + 263, regNY + 285, 124, 13, 14, false, false}, // 电话
	{regNX + 263, regNY + 315, 124, 12, 13, false, false}, // 手机
	{regNX + 263, regNY + 368, 124, 13, 40, false, false}, // 邮箱
}

// 注册窗口按钮 (FState.pas:868-876)：确定 [51]、取消 [52]、关闭 [83]。
var regButtonAreas = []loginArea{
	{loginOX + 305, loginOY + 530, 70, 20},
	{loginOX + 445, loginOY + 530, 70, 20},
	{loginOX + 587, loginOY + 33, 20, 20},
}

var regButtonImages = []int{51, 52, 83}

// 修改密码窗口输入框 (IntroScn.pas:412-480)，基准 nx=800/2-210=190,
// ny=600/2-150=150。顺序：账号、旧密码、新密码、确认密码。
const (
	chgNX = 190
	chgNY = 150
)

var chgFieldDefs = []fieldDef{
	{chgNX + 191, chgNY + 92, 104, 13, 10, false, false},
	{chgNX + 191, chgNY + 119, 104, 13, 10, true, false},
	{chgNX + 191, chgNY + 145, 104, 13, 10, true, true},
	{chgNX + 191, chgNY + 172, 104, 13, 10, true, true},
}

// 修改密码按钮，相对于居中的 [50] 窗口
// (FState.pas:887-892)：确定 [361] 位于 +81,+141；取消 [365] 位于 +160,+141。
var chgButtonImages = []int{361, 365}

// serverInfo 保存 SM_PASSOKSELECTSERVER 返回的服务器条目。
type serverInfo struct {
	Name   string
	Status int
}

// 选服对话框：Prguse[256] 背景，[79] 按钮（按下态 [80]），
// 关闭 [83] (FState.pas:810-847 英文版)。中文版 [160-166]
// 的图片在当前资源包中是 1×1 占位图。
const (
	srvDlgImg   = 256
	srvBtnImg   = 79
	srvCloseImg = 83
	srvCloseDX  = float32(245)
	srvCloseDY  = float32(31)
	srvCloseW   = float32(20)
	srvCloseH   = float32(20)
)

// srvButtonTop 根据服务器数量返回第 i 个服务器按钮的窗口内 Top 值
// (FState.pas:2456-2474)。
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

// serverDisplayName 根据服务器状态返回按钮文本和颜色
// (FState.pas:2250-2272)：0 维护/clDkGray, 1 正常/clLime,
// 2 流畅/clGreen, 3 繁忙/clMaroon, 4 满员/clRed。
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

// parseServerList 解析 SM_PASSOKSELECTSERVER 消息体中的服务器列表。
// 消息体格式："name1/status1/name2/status2/..."
func parseServerList(body string) []serverInfo {
	var servers []serverInfo
	if body == "" {
		// 默认服务器
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

// splitSlash 按 '/' 分割字符串。
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

// regHelps 是各注册输入框旁的帮助文本，以 clSilver 色绘制在
// 当前焦点输入框旁 (IntroScn.pas:709-786; Delphi 原文是 GBK
// 乱码，已替换为简洁中文)。
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

// LoginScene 处理登录界面 (Delphi TLoginScene, IntroScn.pas:62)。
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer // 默认 16px
	textSmall *engine.TextRenderer // 13px 用于输入框 (Delphi Font.Size=10)
	textSrv   *engine.TextRenderer // 15px 用于服务器名 (Delphi Font.Size=11)

	// 开门动画 (IntroScn.pas:804-851)；淡出由 globalFade 处理。
	doorOpening   bool
	doorFading    bool
	doorFrame     int
	doorStartTime time.Time

	// 模式 (TLoginState)
	mode        loginMode
	showLoginUI bool

	// 登录输入框
	userID          string
	password        string
	focusedField    int // 0=账号, 1=密码, -1=无
	waitingResponse bool
	cursorBlink     time.Time

	// 注册输入框 (13个) / 修改密码输入框 (4个)
	regFields [13]string
	regFocus  int
	chgFields [4]string
	chgFocus  int

	// 鼠标：当前模式下按住的按钮索引 (-1 = 无)
	pressedButton int

	compLoggedModes uint8 // 已输出布局日志的模式位掩码

	// 模态对话框 (Delphi DMessageDlg, FState.pas:1938-2158)
	dlgMsg        string
	dlgLines      []string
	dlgSize       int   // 0=小[381], 1=中[360], 2=大[380]
	dlgButtons    []int // 按钮图片索引（361=确定, 363=是, 365=取消, 367=否）
	dlgPressedBtn int   // dlgButtons 中的索引，-1=无

	connecting bool

	// 选服浮层 (Delphi DSelServerDlg, FState.pas:778-857)
	servers []serverInfo

	// 回调
	loginFunc        func(id, password string)
	registerFunc     func(ue protocol.UserEntry, ua protocol.UserEntryAdd)
	chgpwFunc        func(id, oldpw, newpw string)
	selectFunc       func(serverName string)
	closeFunc        func()
	doorCompleteFunc func()
}

func (s *LoginScene) traceDraw(tag, wil string, idx int, x, y, w, h float32) {
	log.Logf(log.LevelTrace, "Render", "login %s %s[%d] pos=(%.0f,%.0f) size=(%.0f,%.0f)", tag, wil, idx, x, y, w, h)
}

var loginModeNames = [...]string{"login", "register", "chgpw", "serverselect"}

func (s *LoginScene) logComponentLayout() {
	name := loginModeNames[s.mode]
	log.Logf(log.LevelInfo, "LoginScene", "=== 组件布局 (mode=%s) ===", name)
	log.Logf(log.LevelInfo, "LoginScene", "  背景      ChrSel[22]       pos=(%.0f,%.0f) size=(800,600)", loginOX, loginOY)

	switch s.mode {
	case modeLogin:
		fieldNames := [2]string{"ID", "Password"}
		for i, a := range inputFields {
			log.Logf(log.LevelInfo, "LoginScene", "  输入框    %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f)", fieldNames[i], a.X, a.Y, a.W, a.H)
		}
		btnNames := [4]string{"OK", "ChangePW", "NewAccount", "Close"}
		for i, a := range buttonAreas {
			log.Logf(log.LevelInfo, "LoginScene", "  按钮      %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f) img=Prguse[%d]", btnNames[i], a.X, a.Y, a.W, a.H, buttonImages[i])
		}

	case modeRegister:
		wx, wy, ww, wh := s.windowOrigin(63)
		log.Logf(log.LevelInfo, "LoginScene", "  窗口      Prguse[63]       pos=(%.0f,%.0f) size=(%d,%d)", wx, wy, ww, wh)
		regNames := [13]string{"account", "password", "confirm", "name", "SSNo", "birthday", "quiz1", "answer1", "quiz2", "answer2", "phone", "mobile", "email"}
		for i, def := range regFieldDefs {
			log.Logf(log.LevelInfo, "LoginScene", "  输入框    %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f)", regNames[i], def.x, def.y, def.w, def.h)
		}
		regBtnNames := [3]string{"Ok", "Cancel", "Close"}
		for i, a := range regButtonAreas {
			log.Logf(log.LevelInfo, "LoginScene", "  按钮      %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f) img=Prguse[%d]", regBtnNames[i], a.X, a.Y, a.W, a.H, regButtonImages[i])
		}
		log.Logf(log.LevelInfo, "LoginScene", "  标题文本  %-12s pos=(362,121)", "创建新账号")

	case modeChgPw:
		wx, wy, ww, wh := s.windowOrigin(50)
		log.Logf(log.LevelInfo, "LoginScene", "  窗口      Prguse[50]       pos=(%.0f,%.0f) size=(%d,%d)", wx, wy, ww, wh)
		chgNames := [4]string{"account", "old-pw", "new-pw", "repeat"}
		for i, def := range chgFieldDefs {
			log.Logf(log.LevelInfo, "LoginScene", "  输入框    %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f)", chgNames[i], def.x, def.y, def.w, def.h)
		}
		chgBtnNames := [2]string{"Ok", "Cancel"}
		for i, off := range []loginArea{{wx + 81, wy + 141, 0, 0}, {wx + 160, wy + 141, 0, 0}} {
			bw, bh := s.getPrguseSize(chgButtonImages[i])
			log.Logf(log.LevelInfo, "LoginScene", "  按钮      %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", chgBtnNames[i], off.X, off.Y, bw, bh, chgButtonImages[i])
		}

	case modeServerSelect:
		wx, wy := s.srvWindowOrigin()
		ww, wh := s.getPrguseSize(srvDlgImg)
		log.Logf(log.LevelInfo, "LoginScene", "  窗口      Prguse[%d]      pos=(%.0f,%.0f) size=(%d,%d)", srvDlgImg, wx, wy, ww, wh)
		cw, ch := s.getPrguseSize(srvCloseImg)
		log.Logf(log.LevelInfo, "LoginScene", "  按钮      %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", "Close", wx+srvCloseDX, wy+srvCloseDY, cw, ch, srvCloseImg)
		bw, bh := s.getPrguseSize(srvBtnImg)
		count := len(s.servers)
		for i := 0; i < count && i < 6; i++ {
			bx := wx + 65
			by := wy + srvButtonTop(i, count)
			log.Logf(log.LevelInfo, "LoginScene", "  服务器按钮 %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", s.servers[i].Name, bx, by, bw, bh, srvBtnImg)
		}
	}
}

// NewLoginScene 创建登录场景。
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *LoginScene {
	s := &LoginScene{
		gl:             gl,
		resources:      resources,
		text:           text,
		textSmall:      text,
		textSrv:        text,
		showLoginUI:    true,
		focusedField:   0,
		pressedButton:  -1,
		cursorBlink:    time.Now(),
		compLoggedModes: 0,
	}
	// Delphi 输入框使用 Font.Size=10 ≈ 13px @96DPI (IntroScn.pas:260)。
	if t, err := text.WithSize(13); err == nil {
		s.textSmall = t
	}
	// Delphi 服务器名使用 Font.Size=11 ≈ 15px @96DPI (FState.pas:2274)。
	if t, err := text.WithSize(15); err == nil {
		s.textSrv = t
	}
	return s
}

// Open 在场景激活时调用。
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "已打开")
	s.mode = modeLogin
	s.showLoginUI = true
	s.doorOpening = false
	s.doorFading = false
	s.doorFrame = 0
	s.userID = ""
	s.password = ""
	s.focusedField = 0
	s.waitingResponse = false
	s.dlgMsg = ""
	s.dlgLines = nil
	s.connecting = false
	s.pressedButton = -1
	s.servers = nil
	s.cursorBlink = time.Now()
	s.compLoggedModes = 0
}

// Close 在场景失活时调用。
func (s *LoginScene) Close() {
	log.Logf(log.LevelInfo, "LoginScene", "已关闭")
}

// Update 推进开门动画；黑屏淡出由全局 MakeDark 系统处理
// (ClMain.pas:1114-1130)。
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
				log.Logf(log.LevelInfo, "LoginScene", "开门动画完成, 开始淡出")
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
	ox, oy := loginOX, loginOY

	// 背景：ChrSel.wil[22] (IntroScn.pas:818,821)
	bgTex, bgErr := s.getChrSelTexture(22)
	if bgErr == nil && bgTex != 0 {
		w, h := s.getChrSelSize(22)
		s.traceDraw("bg", "ChrSel", 22, ox, oy, float32(w), float32(h))
		gl.DrawQuad(bgTex, ox, oy, float32(w), float32(h), proj)
	} else {
		log.Logf(log.LevelWarn, "LoginScene", "ChrSel[22] 背景不可用 (tex=%d err=%v)", bgTex, bgErr)
		gl.DrawQuadColor(0, 0, 800, 600, 0.05, 0.05, 0.1, 1, proj)
	}

	// 开门动画：ChrSel.wil[23..32] (IntroScn.pas:841,845——原始坐标
	// {152},{96} 与十周年版 ChrSel.wil 资源匹配)。
	if s.doorOpening {
		doorIdx := 23 + s.doorFrame
		if doorTex, err := s.getChrSelTexture(doorIdx); err == nil {
			w, h := s.getChrSelSize(doorIdx)
			s.traceDraw("door", "ChrSel", doorIdx, ox+152, oy+96, float32(w), float32(h))
			gl.DrawQuad(doorTex, ox+152, oy+96, float32(w), float32(h), proj)
		}
	}

	if s.showLoginUI && !s.doorOpening {
		switch s.mode {
		case modeRegister:
			s.renderRegisterWindow(gl, proj, ox, oy)
		case modeChgPw:
			s.renderChgPwWindow(gl, proj, ox, oy)
		case modeServerSelect:
			s.renderServerSelect(gl, proj)
		default:
			s.renderButtons(gl, proj, ox, oy)
			s.renderInputFields(gl, proj, ox, oy)
		}
		if s.compLoggedModes&(1<<s.mode) == 0 {
			s.logComponentLayout()
			s.compLoggedModes |= 1 << s.mode
		}
	}

	// 模态对话框绘制在最上层。
	if s.dlgMsg != "" {
		s.renderDialog(gl, proj)
	}
}

// renderButtons 绘制底部四个登录按钮。Delphi 原版将常态面板烘焙在
// ChrSel[22] 中，仅在按下时叠加绘制 (DLoginNewDirectPaint,
// FState.pas:2342-2354)。由于当前资源包的 ChrSel[22] 没有按钮面板，
// 所以始终绘制 Prguse 图片。按下态在同一位置绘制（无偏移, FState.pas:2351）。
func (s *LoginScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	for i, idx := range buttonImages {
		a := s.buttonArea(i)
		if tex, err := s.getPrguseTexture(idx); err == nil {
			w, h := s.getPrguseSize(idx)
			s.traceDraw("btn", "Prguse", idx, a.X, a.Y, float32(w), float32(h))
			gl.DrawQuad(tex, a.X, a.Y, float32(w), float32(h), proj)
		}
	}
}

// renderInputFields 渲染账号/密码文本和闪烁光标。
// 标签已烘焙在背景中；Delphi 的原生 TEdit 是黑底白字
// (IntroScn.pas:255-274)。
func (s *LoginScene) renderInputFields(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}
	// 提交后等待服务器响应期间隐藏输入框
	// (IntroScn.pas:551-554)。
	if s.waitingResponse {
		return
	}

	idX, idY := ox+255, oy+511
	passX, passY := ox+495, oy+511
	masked := strings.Repeat("*", len(s.password))

	// TEdit Color=clBlack：白色文字下方的不透明黑色底框
	// (IntroScn.pas:258,270)。
	s.traceDraw("field", "quad", -1, idX, idY, 112, 19)
	gl.DrawQuadColor(idX, idY, 112, 19, 0, 0, 0, 1, proj)
	s.traceDraw("field", "quad", -1, passX, passY, 112, 19)
	gl.DrawQuadColor(passX, passY, 112, 19, 0, 0, 0, 1, proj)

	s.textSmall.DrawText(s.userID, idX, idY, 1.0, 1.0, 1.0, 1.0, proj)
	s.textSmall.DrawText(masked, passX, passY, 1.0, 1.0, 1.0, 1.0, proj)

	if time.Since(s.cursorBlink) > 500*time.Millisecond {
		s.cursorBlink = time.Now()
	}
	if time.Since(s.cursorBlink) < 250*time.Millisecond && s.focusedField >= 0 {
		var cx, cy float32
		if s.focusedField == 0 {
			cx = idX + float32(s.textSmall.MeasureText(s.userID))
			cy = idY
		} else {
			cx = passX + float32(s.textSmall.MeasureText(masked))
			cy = passY
		}
		// 1px 白色竖线，模拟原生 TEdit 光标 (IntroScn.pas:255-274)。
		gl.DrawQuadColor(cx, cy, 1, 19, 1, 1, 1, 1, proj)
	}
}

// windowOrigin 返回居中 Prguse 窗口图片的左上角坐标。
func (s *LoginScene) windowOrigin(index int) (float32, float32, int, int) {
	w, h := s.getPrguseSize(index)
	return loginOX + float32(800-w)/2, loginOY + float32(600-h)/2, w, h
}

// renderRegisterWindow 渲染 DNewAccount：Prguse[63] 背景居中，
// 13 个黑色输入框，确定[51]/取消[52]/关闭[83] (FState.pas:862-876)。
func (s *LoginScene) renderRegisterWindow(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if tex, err := s.getPrguseTexture(63); err == nil {
		wx, wy, w, h := s.windowOrigin(63)
		s.traceDraw("reg-bg", "Prguse", 63, wx, wy, float32(w), float32(h))
		gl.DrawQuad(tex, wx, wy, float32(w), float32(h), proj)
	}

	s.renderFieldGroup(gl, proj, regFieldDefs[:], s.regFields[:], s.regFocus)

	for i, area := range regButtonAreas {
		idx := regButtonImages[i]
		if tex, err := s.getPrguseTexture(idx); err == nil {
			w, h := s.getPrguseSize(idx)
			s.traceDraw("reg-btn", "Prguse", idx, area.X, area.Y, float32(w), float32(h))
			gl.DrawQuad(tex, area.X, area.Y, float32(w), float32(h), proj)
		}
	}

	if s.text != nil {
		// 标题 NewAccountTitle 位于 (362,121)，白色+黑色描边，粗体
		// (FState.pas:2669)。
		log.Logf(log.LevelTrace, "Render", "login 标题 pos=(%.0f,%.0f)", float32(362), float32(121))
		s.text.DrawTextBoldOutline("创建新账号", 362, 121, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		// 各输入框帮助文本 NAHelps，clSilver 色，随焦点切换
		// (IntroScn.pas:709-786; FState.pas:2664-2668, 507,124+i*14)。
		if s.regFocus >= 0 && s.regFocus < len(regHelps) {
			s.textSmall.DrawText(regHelps[s.regFocus], 507, 124, 0.75, 0.75, 0.75, 1, proj)
		}
	}

	if s.text != nil && s.connecting {
		s.text.DrawText("注册中...", ox+350, oy+420, 0.5, 0.8, 1.0, 1.0, proj)
	}
}

// renderChgPwWindow 渲染 DChgPw：Prguse[50] 背景居中，4 个输入框，
// 确定[361]/取消[365] 位于窗口+81,+141 / +160,+141 (FState.pas:881-892)。
func (s *LoginScene) renderChgPwWindow(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	wx, wy, _, _ := s.windowOrigin(50)
	if tex, err := s.getPrguseTexture(50); err == nil {
		w, h := s.getPrguseSize(50)
		s.traceDraw("chgpw-bg", "Prguse", 50, wx, wy, float32(w), float32(h))
		gl.DrawQuad(tex, wx, wy, float32(w), float32(h), proj)
	}

	s.renderFieldGroup(gl, proj, chgFieldDefs[:], s.chgFields[:], s.chgFocus)

	for i, off := range []loginArea{{wx + 81, wy + 141, 0, 0}, {wx + 160, wy + 141, 0, 0}} {
		idx := chgButtonImages[i]
		if tex, err := s.getPrguseTexture(idx); err == nil {
			w, h := s.getPrguseSize(idx)
			s.traceDraw("chgpw-btn", "Prguse", idx, off.X, off.Y, float32(w), float32(h))
			gl.DrawQuad(tex, off.X, off.Y, float32(w), float32(h), proj)
		}
	}
}

// srvWindowOrigin 返回运行时居中的 [256] 对话框左上角坐标
// (FState.pas:813-814: (SCREENWIDTH-w)/2, (SCREENHEIGHT-h)/2)。
func (s *LoginScene) srvWindowOrigin() (float32, float32) {
	w, h := s.getPrguseSize(srvDlgImg)
	return loginOX + float32(800-w)/2, loginOY + float32(600-h)/2
}

// renderServerSelect 渲染 DSelServerDlg：对话框 [256]，最多六个 [79]
// 按钮（按下态 -> [80]），关闭 [83] (FState.pas:810-847, 2220-2280)。
func (s *LoginScene) renderServerSelect(gl *engine.GLState, proj [16]float32) {
	wx, wy := s.srvWindowOrigin()
	if tex, err := s.getPrguseTexture(srvDlgImg); err == nil {
		w, h := s.getPrguseSize(srvDlgImg)
		s.traceDraw("srv-bg", "Prguse", srvDlgImg, wx, wy, float32(w), float32(h))
		gl.DrawQuad(tex, wx, wy, float32(w), float32(h), proj)
	}

	if tex, err := s.getPrguseTexture(srvCloseImg); err == nil {
		cw, ch := s.getPrguseSize(srvCloseImg)
		s.traceDraw("srv-close", "Prguse", srvCloseImg, wx+srvCloseDX, wy+srvCloseDY, float32(cw), float32(ch))
		gl.DrawQuad(tex, wx+srvCloseDX, wy+srvCloseDY, float32(cw), float32(ch), proj)
	}

	bw, bh := s.getPrguseSize(srvBtnImg)
	count := len(s.servers)
	for i := 0; i < count && i < 6; i++ {
		bx := wx + 65
		by := wy + srvButtonTop(i, count)
		idx := srvBtnImg
		if s.pressedButton == i {
			idx = srvBtnImg + 1
		}
		if tex, err := s.getPrguseTexture(idx); err == nil {
			s.traceDraw("srv-btn", "Prguse", idx, bx, by, float32(bw), float32(bh))
			gl.DrawQuad(tex, bx, by, float32(bw), float32(bh), proj)
		}
		if s.textSrv != nil {
			name, r, g, b := serverDisplayName(s.servers[i])
			tw := float32(s.textSrv.MeasureText(name))
			th := float32(s.textSrv.LineHeight())
			tx := bx + (float32(bw)-tw)/2
			ty := by + (float32(bh)-th)/2
			if s.pressedButton == i {
				tx += 2
				ty += 2
			}
			log.Logf(log.LevelTrace, "Render", "login 服务器按钮文本 %q pos=(%.0f,%.0f)", name, tx, ty)
			s.textSrv.DrawTextBoldOutline(name, tx, ty, r, g, b, 1, 0, 0, 0, 1, proj)
		}
	}
}

// renderFieldGroup 绘制黑底白字输入框 (Delphi TEdit:
// Color=clBlack, Font.Color=clWhite, IntroScn.pas:255-274)，
// 并在焦点输入框上绘制闪烁光标。
func (s *LoginScene) renderFieldGroup(gl *engine.GLState, proj [16]float32, defs []fieldDef, values []string, focus int) {
	for _, def := range defs {
		s.traceDraw("field", "quad", -1, def.x, def.y, def.w, def.h)
		gl.DrawQuadColor(def.x, def.y, def.w, def.h, 0, 0, 0, 1, proj)
	}
	if s.textSmall == nil {
		return
	}
	for i, def := range defs {
		text := values[i]
		if def.masked {
			text = strings.Repeat("*", len(text))
		}
		s.textSmall.DrawText(text, def.x+2, def.y, 1.0, 1.0, 1.0, 1.0, proj)
	}

	if time.Since(s.cursorBlink) > 500*time.Millisecond {
		s.cursorBlink = time.Now()
	}
	if focus >= 0 && focus < len(defs) && time.Since(s.cursorBlink) < 250*time.Millisecond {
		def := defs[focus]
		text := values[focus]
		if def.masked {
			text = strings.Repeat("*", len(text))
		}
		cx := def.x + 2 + float32(s.textSmall.MeasureText(text))
		gl.DrawQuadColor(cx, def.y, 1, def.h, 1, 1, 1, 1, proj)
	}
}

// dlgGeom 保存各尺寸 DMessageDlg 的布局参数
// (FState.pas:2002-2042)。
type dlgGeom struct {
	bgImg                    int
	msgLX, msgLY             float32
	btnLX, btnLY             float32
}

var dlgSizes = [3]dlgGeom{
	{bgImg: 381, msgLX: 39, msgLY: 38, btnLX: 90, btnLY: 36},   // 小
	{bgImg: 360, msgLX: 39, msgLY: 38, btnLX: 324, btnLY: 126}, // 中
	{bgImg: 380, msgLX: 23, msgLY: 20, btnLX: 105, btnLY: 305}, // 大
}

// dlgButtonAreas 返回当前对话框的窗口矩形和各按钮矩形。
// 按钮从 btnLX 开始从右向左排列，间距 110px
// (FState.pas:2060-2083)。
func (s *LoginScene) dlgButtonAreas() (loginArea, []loginArea) {
	g := dlgSizes[s.dlgSize]
	w, h := s.getPrguseSize(g.bgImg)
	wx := loginOX + float32(800-w)/2
	wy := loginOY + float32(600-h)/2
	win := loginArea{wx, wy, float32(w), float32(h)}

	btns := make([]loginArea, len(s.dlgButtons))
	lx := g.btnLX
	for i, img := range s.dlgButtons {
		bw, bh := s.getPrguseSize(img)
		btns[i] = loginArea{wx + lx, wy + g.btnLY, float32(bw), float32(bh)}
		lx -= 110
	}
	return win, btns
}

// renderDialog 渲染指定尺寸和按钮的 DMessageDlg
// (FState.pas:739-752, 2002-2083, 2291-2325)。
func (s *LoginScene) renderDialog(gl *engine.GLState, proj [16]float32) {
	g := dlgSizes[s.dlgSize]
	win, btns := s.dlgButtonAreas()

	if tex, err := s.getPrguseTexture(g.bgImg); err == nil {
		s.traceDraw("dlg-bg", "Prguse", g.bgImg, win.X, win.Y, win.W, win.H)
		gl.DrawQuad(tex, win.X, win.Y, win.W, win.H, proj)
	}

	for i, img := range s.dlgButtons {
		idx := img
		if s.dlgPressedBtn == i {
			idx = img + 1
		}
		if tex, err := s.getPrguseTexture(idx); err == nil {
			s.traceDraw("dlg-btn", "Prguse", idx, btns[i].X, btns[i].Y, btns[i].W, btns[i].H)
			gl.DrawQuad(tex, btns[i].X, btns[i].Y, btns[i].W, btns[i].H, proj)
		}
	}

	if s.text == nil {
		return
	}
	y := win.Y + g.msgLY
	for _, ln := range s.dlgLines {
		log.Logf(log.LevelTrace, "Render", "login 对话框文本 %q pos=(%.0f,%.0f)", ln, win.X+g.msgLX, y)
		s.text.DrawTextBoldOutline(ln, win.X+g.msgLX, y, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		y += 14
	}
}

// OnChar 处理 GLFW 字符输入。
func (s *LoginScene) OnChar(char rune) {
	if !s.showLoginUI || s.doorOpening || s.connecting || s.dlgMsg != "" {
		return
	}
	if char < 32 {
		return
	}
	switch s.mode {
	case modeRegister:
		def := regFieldDefs[s.regFocus]
		if def.password && (char == '~' || char == '\'' || char == ' ') {
			return // 输入时过滤 (IntroScn.pas:640-641)
		}
		if utf8.RuneCountInString(s.regFields[s.regFocus]) < def.maxLen {
			s.regFields[s.regFocus] += string(char)
		}
	case modeChgPw:
		def := chgFieldDefs[s.chgFocus]
		if def.password && (char == '~' || char == '\'' || char == ' ') {
			return
		}
		if utf8.RuneCountInString(s.chgFields[s.chgFocus]) < def.maxLen {
			s.chgFields[s.chgFocus] += string(char)
		}
	case modeServerSelect:
		return // 选服浮层不接受文本输入
	default:
		if s.waitingResponse || s.focusedField < 0 {
			return
		}
		if s.focusedField == 0 {
			if utf8.RuneCountInString(s.userID) < 10 {
				s.userID += string(char)
			}
		} else if utf8.RuneCountInString(s.password) < 10 {
			s.password += string(char)
		}
	}
	s.cursorBlink = time.Now()
}

// OnKey 处理键盘输入。
func (s *LoginScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	if !s.showLoginUI || s.doorOpening {
		return
	}
	if s.dlgMsg != "" {
		// DMsgDlgKeyDown：Enter/Esc 关闭仅有确定按钮的对话框 (FState.pas:2139-2158)。
		if key == keyEnter || key == keyKPEnter || key == keyEscape {
			s.closeDialog()
		}
		return
	}
	switch s.mode {
	case modeRegister:
		s.keyRegister(key)
	case modeChgPw:
		s.keyChgPw(key)
	case modeServerSelect:
		if key == keyEscape && s.closeFunc != nil {
			s.closeFunc() // DSelServerDlg 关闭 == FrmMain.Close
		}
	default:
		s.keyLogin(key)
	}
}

func (s *LoginScene) keyLogin(key int) {
	switch key {
	case keyBackspace:
		if s.connecting {
			return
		}
		switch s.focusedField {
		case 0:
			if len(s.userID) > 0 {
				_, size := utf8.DecodeLastRuneInString(s.userID)
				s.userID = s.userID[:len(s.userID)-size]
			}
		case 1:
			if len(s.password) > 0 {
				_, size := utf8.DecodeLastRuneInString(s.password)
				s.password = s.password[:len(s.password)-size]
			}
		}
		s.cursorBlink = time.Now()

	case keyTab:
		if s.connecting {
			return
		}
		if s.focusedField == 0 {
			s.focusedField = 1
		} else {
			s.focusedField = 0
		}
		s.cursorBlink = time.Now()

	case keyEnter, keyKPEnter:
		// 账号输入框按 Enter 将焦点移到密码框
		// (EdLoginIdKeyPress, IntroScn.pas:530-539)；密码框按 Enter
		// 提交登录 (EdLoginPasswdKeyPress, :541-558)。
		if s.focusedField == 0 {
			s.focusedField = 1
			s.cursorBlink = time.Now()
		} else {
			s.submitLogin()
		}
	}
}

func (s *LoginScene) keyRegister(key int) {
	switch key {
	case keyBackspace:
		if s.connecting {
			return
		}
		if f := s.regFields[s.regFocus]; len(f) > 0 {
			_, size := utf8.DecodeLastRuneInString(f)
			s.regFields[s.regFocus] = f[:len(f)-size]
		}
		s.cursorBlink = time.Now()
	case keyTab:
		if s.connecting {
			return
		}
		s.regFocus = (s.regFocus + 1) % len(s.regFields)
		s.cursorBlink = time.Now()
	case keyEnter, keyKPEnter:
		s.submitRegister()
	case keyEscape:
		s.mode = modeLogin
		s.pressedButton = -1
	}
}

func (s *LoginScene) keyChgPw(key int) {
	switch key {
	case keyBackspace:
		if s.connecting {
			return
		}
		if f := s.chgFields[s.chgFocus]; len(f) > 0 {
			_, size := utf8.DecodeLastRuneInString(f)
			s.chgFields[s.chgFocus] = f[:len(f)-size]
		}
		s.cursorBlink = time.Now()
	case keyTab:
		if s.connecting {
			return
		}
		s.chgFocus = (s.chgFocus + 1) % len(s.chgFields)
		s.cursorBlink = time.Now()
	case keyEnter, keyKPEnter:
		s.submitChgPw()
	case keyEscape:
		s.mode = modeLogin // ChgpwCancel (IntroScn.pas:1094-1097)
		s.pressedButton = -1
	}
}

// OnMouse 处理鼠标按键输入。在同一区域内松开时触发点击
// (TDButton.MouseUp, DWinCtl.pas:677-695)。
func (s *LoginScene) OnMouse(x, y float64, button int, action int, mods int) {
	log.Logf(log.LevelDebug, "Mouse", "login pos=(%.0f,%.0f) 按键=%d 动作=%d", x, y, button, action)
	if !s.showLoginUI || s.doorOpening {
		return
	}
	fx, fy := float32(x), float32(y)

	if s.dlgMsg != "" {
		_, btns := s.dlgButtonAreas()
		switch action {
		case mousePress:
			for i, b := range btns {
				if hitTest(fx, fy, b) {
					s.dlgPressedBtn = i
					log.Logf(log.LevelInfo, "LoginScene", "点击 对话框按钮[%d] img=Prguse[%d] pos=(%.0f,%.0f)", i, s.dlgButtons[i], b.X, b.Y)
					break
				}
			}
		case mouseRelease:
			pressed := s.dlgPressedBtn
			s.dlgPressedBtn = -1
			if pressed >= 0 && pressed < len(btns) && hitTest(fx, fy, btns[pressed]) {
				s.closeDialog()
			}
		}
		return
	}

	switch s.mode {
	case modeRegister:
		fields := make([]loginArea, len(regFieldDefs))
		for i, def := range regFieldDefs {
			fields[i] = loginArea{def.x, def.y, def.w, def.h}
		}
		buttons := make([]loginArea, len(regButtonAreas))
		for i := range regButtonAreas {
			buttons[i] = s.regButtonArea(i)
		}
		s.mouseGroup(fx, fy, action, fields, buttons, &s.regFocus, s.handleRegButton)
	case modeChgPw:
		s.mouseChgPw(fx, fy, action)
	case modeServerSelect:
		s.mouseServerSelect(fx, fy, action)
	default:
		s.mouseLogin(fx, fy, action)
	}
}

// buttonArea 返回底部第 i 个按钮的碰撞矩形；尺寸取自按钮图片
// (TDControl.SetImgIndex, DWinCtl.pas:607-621)。
func (s *LoginScene) buttonArea(i int) loginArea {
	a := buttonAreas[i]
	if w, h := s.getPrguseSize(buttonImages[i]); w > 0 && h > 0 {
		a.W, a.H = float32(w), float32(h)
	}
	return a
}

// regButtonArea 返回注册窗口第 i 个按钮的碰撞矩形。
func (s *LoginScene) regButtonArea(i int) loginArea {
	a := regButtonAreas[i]
	if w, h := s.getPrguseSize(regButtonImages[i]); w > 0 && h > 0 {
		a.W, a.H = float32(w), float32(h)
	}
	return a
}

func (s *LoginScene) mouseLogin(fx, fy float32, action int) {
	switch action {
	case mousePress:
		fieldNames := [2]string{"ID", "Password"}
		for i, field := range inputFields {
			if hitTest(fx, fy, field) {
				s.focusedField = i
				s.cursorBlink = time.Now()
				log.Logf(log.LevelInfo, "LoginScene", "点击 输入框 %s pos=(%.0f,%.0f)", fieldNames[i], field.X, field.Y)
				return
			}
		}
		btnNames := [4]string{"OK", "ChangePW", "NewAccount", "Close"}
		for i := range buttonAreas {
			if hitTest(fx, fy, s.buttonArea(i)) {
				s.pressedButton = i
				log.Logf(log.LevelInfo, "LoginScene", "点击 按钮 %s pos=(%.0f,%.0f)", btnNames[i], buttonAreas[i].X, buttonAreas[i].Y)
				return
			}
		}
		s.focusedField = -1
	case mouseRelease:
		if s.pressedButton >= 0 {
			i := s.pressedButton
			s.pressedButton = -1
			if hitTest(fx, fy, s.buttonArea(i)) {
				s.handleButton(i)
			}
		}
	}
}

// mouseGroup 是注册窗口输入框组和按钮行的共用按下/松开逻辑。
func (s *LoginScene) mouseGroup(fx, fy float32, action int, fields, buttons []loginArea, focus *int, onClick func(int)) {
	switch action {
	case mousePress:
		regNames := [13]string{"account", "password", "confirm", "name", "SSNo", "birthday", "quiz1", "answer1", "quiz2", "answer2", "phone", "mobile", "email"}
		for i, field := range fields {
			if hitTest(fx, fy, field) {
				*focus = i
				s.cursorBlink = time.Now()
				name := fmt.Sprintf("field[%d]", i)
				if i < len(regNames) {
					name = regNames[i]
				}
				log.Logf(log.LevelInfo, "LoginScene", "点击 输入框 %s pos=(%.0f,%.0f)", name, field.X, field.Y)
				return
			}
		}
		regBtnNames := [3]string{"Ok", "Cancel", "Close"}
		for i, btn := range buttons {
			if hitTest(fx, fy, btn) {
				s.pressedButton = i
				name := fmt.Sprintf("button[%d]", i)
				if i < len(regBtnNames) {
					name = regBtnNames[i]
				}
				log.Logf(log.LevelInfo, "LoginScene", "点击 按钮 %s pos=(%.0f,%.0f)", name, btn.X, btn.Y)
				return
			}
		}
	case mouseRelease:
		if s.pressedButton >= 0 && s.pressedButton < len(buttons) {
			i := s.pressedButton
			s.pressedButton = -1
			if hitTest(fx, fy, buttons[i]) {
				onClick(i)
			}
		}
	}
}

// mouseChgPw 处理修改密码窗口；其确定/取消按钮是居中 [50] 窗口的
// 子控件 (FState.pas:887-892)。
func (s *LoginScene) mouseChgPw(fx, fy float32, action int) {
	wx, wy, _, _ := s.windowOrigin(50)
	buttons := make([]loginArea, 2)
	for i, off := range []loginArea{{wx + 81, wy + 141, 0, 0}, {wx + 160, wy + 141, 0, 0}} {
		w, h := s.getPrguseSize(chgButtonImages[i])
		buttons[i] = loginArea{off.X, off.Y, float32(w), float32(h)}
	}
	switch action {
	case mousePress:
		chgNames := [4]string{"account", "old-pw", "new-pw", "repeat"}
		for i, def := range chgFieldDefs {
			if hitTest(fx, fy, loginArea{def.x, def.y, def.w, def.h}) {
				s.chgFocus = i
				s.cursorBlink = time.Now()
				log.Logf(log.LevelInfo, "LoginScene", "点击 输入框 %s pos=(%.0f,%.0f)", chgNames[i], def.x, def.y)
				return
			}
		}
		chgBtnNames := [2]string{"Ok", "Cancel"}
		for i, btn := range buttons {
			if hitTest(fx, fy, btn) {
				s.pressedButton = i
				log.Logf(log.LevelInfo, "LoginScene", "点击 按钮 %s pos=(%.0f,%.0f)", chgBtnNames[i], btn.X, btn.Y)
				return
			}
		}
	case mouseRelease:
		if s.pressedButton >= 0 && s.pressedButton < 2 {
			i := s.pressedButton
			s.pressedButton = -1
			if hitTest(fx, fy, buttons[i]) {
				if i == 0 {
					s.submitChgPw()
				} else {
					s.mode = modeLogin // ChgpwCancel
				}
			}
		}
	}
}

// mouseServerSelect 处理 DSelServerDlg 浮层：在服务器按钮上按下/松开
// 即选中该服务器；关闭按钮退出程序
// (FState.pas:2220-2224; DSelServerDlg 关闭 == FrmMain.Close)。
func (s *LoginScene) mouseServerSelect(fx, fy float32, action int) {
	wx, wy := s.srvWindowOrigin()
	count := len(s.servers)
	bw, bh := s.getPrguseSize(srvBtnImg)
	closeArea := loginArea{wx + srvCloseDX, wy + srvCloseDY, srvCloseW, srvCloseH}
	btnArea := func(i int) loginArea {
		return loginArea{wx + 65, wy + srvButtonTop(i, count), float32(bw), float32(bh)}
	}

	switch action {
	case mousePress:
		for i := 0; i < count && i < 6; i++ {
			if hitTest(fx, fy, btnArea(i)) {
				s.pressedButton = i
				log.Logf(log.LevelInfo, "LoginScene", "点击 服务器按钮 %s pos=(%.0f,%.0f)", s.servers[i].Name, wx+65, wy+srvButtonTop(i, count))
				return
			}
		}
		if hitTest(fx, fy, closeArea) {
			s.pressedButton = 6
			log.Logf(log.LevelInfo, "LoginScene", "点击 按钮 Close pos=(%.0f,%.0f)", closeArea.X, closeArea.Y)
		}
	case mouseRelease:
		if s.pressedButton < 0 {
			return
		}
		i := s.pressedButton
		s.pressedButton = -1
		if i == 6 {
			if hitTest(fx, fy, closeArea) && s.closeFunc != nil {
				s.closeFunc()
			}
			return
		}
		if i < count && hitTest(fx, fy, btnArea(i)) {
			name := s.servers[i].Name
			s.mode = modeLogin // 选中服务器后关闭浮层
			if s.selectFunc != nil {
				s.selectFunc(name)
			}
		}
	}
}

// handleButton 分发底部四个登录按钮的点击。
func (s *LoginScene) handleButton(index int) {
	switch index {
	case 0:
		s.submitLogin()
	case 1: // lsChgPw (IntroScn.pas:971-974)
		s.mode = modeChgPw
		s.chgFocus = 0
		s.chgFields = [4]string{}
		s.cursorBlink = time.Now()
	case 2: // lsNewId (IntroScn.pas:929-934)
		s.mode = modeRegister
		s.regFocus = 0
		s.regFields = [13]string{}
		s.cursorBlink = time.Now()
	case 3:
		if s.closeFunc != nil {
			s.closeFunc()
		}
	}
}

// handleRegButton 分发注册窗口按钮：确定/取消/关闭。
func (s *LoginScene) handleRegButton(index int) {
	if index == 0 {
		s.submitRegister()
		return
	}
	s.mode = modeLogin // NewAccountClose (IntroScn.pas:1072-1076)
}

// submitLogin 校验输入并触发登录回调。
func (s *LoginScene) submitLogin() {
	if s.connecting {
		return
	}
	if s.userID == "" || s.password == "" {
		s.ShowMessage("请输入账号和密码")
		return
	}
	if s.loginFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}
	s.connecting = true
	s.waitingResponse = true // 等待期间隐藏输入框 (IntroScn.pas:551-554)
	pw := strings.ReplaceAll(strings.ReplaceAll(s.password, "~", "_"), "'", "_")
	id := strings.ToLower(s.userID) // m_sLoginId := LowerCase (IntroScn.pas:534,548)
	log.Logf(log.LevelInfo, "LoginScene", "提交登录: %s", id)
	s.loginFunc(id, pw)
}

// submitRegister 校验所有字段 (CheckUserEntrys, IntroScn.pas:976-1029)
// 并发送注册请求。
func (s *LoginScene) submitRegister() {
	if s.connecting {
		return
	}
	s.regFields[0] = strings.TrimSpace(s.regFields[0])
	s.regFields[3] = strings.TrimSpace(s.regFields[3])
	s.regFields[6] = strings.TrimSpace(s.regFields[6])

	if len(s.regFields[0]) < 3 {
		s.ShowMessage("输入账号的长度必须至少3位.")
		s.regFocus = 0
		return
	}
	if !validBirthDay(s.regFields[5]) { // NewIdCheckBirthDay (:609-632)
		s.regFocus = 5
		return
	}
	if len(s.regFields[1]) < 3 {
		s.regFocus = 1
		return
	}
	if s.regFields[1] != s.regFields[2] {
		s.regFocus = 2
		return
	}
	if len(s.regFields[6]) < 1 {
		s.regFocus = 6
		return
	}
	if len(s.regFields[7]) < 1 {
		s.regFocus = 7
		return
	}
	if len(s.regFields[8]) < 1 {
		s.regFocus = 8
		return
	}
	if len(s.regFields[9]) < 1 {
		s.regFocus = 9
		return
	}
	if len(s.regFields[3]) < 1 {
		s.regFocus = 3
		return
	}
	if len(s.regFields[4]) < 1 { // 非英文分支 (:1022-1027)
		s.regFocus = 4
		return
	}
	if s.registerFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}

	var ue protocol.UserEntry
	var ua protocol.UserEntryAdd
	ue.SetAccount(strings.ToLower(s.regFields[0])) // :1039
	ue.SetPassword(s.regFields[1])
	ue.SetUserName(s.regFields[3])
	ue.SetSSNo(s.regFields[4])
	ue.SetQuiz(s.regFields[6])
	ue.SetAnswer(strings.TrimSpace(s.regFields[7]))
	ue.SetPhone(s.regFields[10])
	ue.SetEMail(strings.TrimSpace(s.regFields[12]))
	ua.SetQuiz2(s.regFields[8])
	ua.SetAnswer2(strings.TrimSpace(s.regFields[9]))
	ua.SetBirthDay(s.regFields[5])
	ua.SetMobilePhone(s.regFields[11])

	log.Logf(log.LevelInfo, "LoginScene", "提交注册: %s", ue.Account())
	s.connecting = true
	s.registerFunc(ue, ua)
	s.mode = modeLogin // NewAccountClose (:1068,1072-1076)
}

// validBirthDay 对应 NewIdCheckBirthDay (IntroScn.pas:609-632)：yyyy/mm/dd 格式。
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

// submitChgPw 发送修改密码请求 (ChgpwOk, IntroScn.pas:1078-1092)。
func (s *LoginScene) submitChgPw() {
	if s.connecting {
		return
	}
	if s.chgFields[2] != s.chgFields[3] {
		s.ShowMessage("两次确认不一致确认.")
		s.chgFocus = 2
		return
	}
	if s.chgpwFunc == nil {
		s.ShowMessage("未连接到服务器")
		return
	}
	log.Logf(log.LevelInfo, "LoginScene", "提交修改密码: %s", s.chgFields[0])
	s.connecting = true
	s.chgpwFunc(s.chgFields[0], s.chgFields[1], s.chgFields[2])
	s.mode = modeLogin // ChgpwCancel (:1087,1094-1097)
	s.chgFields = [4]string{}
}

// ShowMessage 显示中等尺寸、仅确定按钮的模态 DMessageDlg
// (FState.pas:1938-2158)。
func (s *LoginScene) ShowMessage(msg string) {
	s.ShowMessageEx(msg, 1, []int{361})
}

// ShowMessageEx 显示指定尺寸的模态 DMessageDlg
// (0=小, 1=中, 2=大) 和按钮组 (361=确定, 363=是,
// 365=取消, 367=否)。按钮从右向左排列
// (FState.pas:2002-2083)。
func (s *LoginScene) ShowMessageEx(msg string, size int, buttons []int) {
	s.dlgMsg = msg
	s.dlgLines = strings.Split(msg, "\\")
	s.dlgSize = size
	s.dlgButtons = buttons
	s.dlgPressedBtn = -1
	s.connecting = false
	s.waitingResponse = false
}

func (s *LoginScene) closeDialog() {
	s.dlgMsg = ""
	s.dlgLines = nil
	s.dlgButtons = nil
	s.dlgPressedBtn = -1
	if s.mode == modeLogin {
		s.focusedField = 0
		s.cursorBlink = time.Now()
	}
}

// SetError 保留给现有调用方；现在改为显示模态对话框。
func (s *LoginScene) SetError(msg string) {
	log.Logf(log.LevelWarn, "LoginScene", "错误: %s", msg)
	s.ShowMessage(msg)
}

// RegistrationDone 处理 SM_NEWID_SUCCESS：退出注册模式并显示成功对话框
// (ClMain.pas:3684-3691)。
func (s *LoginScene) RegistrationDone() {
	s.mode = modeLogin
	s.regFields = [13]string{}
	s.ShowMessage("您的账号已经注册成功.\\请牢记您的账号和密码.\\请不要以任何原因将账号和密码告诉任何人.")
}

// RegistrationFailed 处理 SM_NEWID_FAIL：返回注册模式并保留已填字段
// (NewIdRetry, IntroScn.pas:936-952)。
func (s *LoginScene) RegistrationFailed(msg string) {
	s.mode = modeRegister
	s.ShowMessage(msg)
}

// ChgPwResult 显示修改密码结果对话框 (ClMain.pas:3762-3771)。
func (s *LoginScene) ChgPwResult(msg string) {
	s.ShowMessage(msg)
}

// SetLoginFunc 设置登录尝试的回调。
func (s *LoginScene) SetLoginFunc(fn func(id, password string)) {
	s.loginFunc = fn
}

// SetRegisterFunc 设置注册尝试的回调。
func (s *LoginScene) SetRegisterFunc(fn func(ue protocol.UserEntry, ua protocol.UserEntryAdd)) {
	s.registerFunc = fn
}

// SetChgPwFunc 设置修改密码尝试的回调。
func (s *LoginScene) SetChgPwFunc(fn func(id, oldpw, newpw string)) {
	s.chgpwFunc = fn
}

// SetSelectFunc 设置服务器选择的回调。
func (s *LoginScene) SetSelectFunc(fn func(serverName string)) {
	s.selectFunc = fn
}

// ShowServerSelect 在登录背景上方打开 DSelServerDlg 浮层，
// 而非切换到独立场景 (FState.pas:2453-2517)。
func (s *LoginScene) ShowServerSelect(servers []serverInfo) {
	log.Logf(log.LevelInfo, "LoginScene", "显示选服界面: %d 个服务器", len(servers))
	s.servers = servers
	s.mode = modeServerSelect
	s.pressedButton = -1
	s.showLoginUI = true
}

// Servers 返回已保存的服务器列表。
func (s *LoginScene) Servers() []serverInfo {
	return s.servers
}

// SetCloseFunc 设置关闭程序的回调。
func (s *LoginScene) SetCloseFunc(fn func()) {
	s.closeFunc = fn
}

// SetDoorCompleteFunc 设置开门动画完成时的回调。
func (s *LoginScene) SetDoorCompleteFunc(fn func()) {
	s.doorCompleteFunc = fn
}

// OpenLoginDoor 开始开门动画。
func (s *LoginScene) OpenLoginDoor() {
	log.Logf(log.LevelInfo, "LoginScene", "开始开门动画")
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

// OnScroll 处理鼠标滚轮输入。
func (s *LoginScene) OnScroll(x, y float64) {
}

// hitTest 检查 (x, y) 是否在区域内。
func hitTest(x, y float32, a loginArea) bool {
	return x >= a.X && x <= a.X+a.W && y >= a.Y && y <= a.Y+a.H
}

// getChrSelTexture 从 ChrSel.wil 获取纹理。
func (s *LoginScene) getChrSelTexture(index int) (uint32, error) {
	if s.resources.ChrSel == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.ChrSel, index), nil
}

// getChrSelSize 获取 ChrSel.wil 中纹理的尺寸。
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

// getPrguseTexture 从 Prguse.wil 获取纹理。
func (s *LoginScene) getPrguseTexture(index int) (uint32, error) {
	if s.resources.Prguse == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.Prguse, index), nil
}

// getPrguseSize 获取 Prguse.wil 中纹理的尺寸。
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
