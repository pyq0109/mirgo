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

// GLFW key codes (matching go-gl/glfw constants).
const (
	keyBackspace = 259
	keyEnter     = 257
	keyTab       = 258
	keyKPEnter   = 335
	keyEscape    = 256
)

// GLFW mouse button actions.
const (
	mouseRelease = 0
	mousePress   = 1
)

// loginArea defines a clickable region.
type loginArea struct {
	X, Y, W, H float32
}

// loginMode mirrors Delphi TLoginState (IntroScn.pas:17).
type loginMode int

const (
	modeLogin loginMode = iota
	modeRegister
	modeChgPw
	modeServerSelect
)

// Screen offset: the window is the fixed 800×600 Delphi game area.
const (
	loginOX = float32(0)
	loginOY = float32(0)
)

// Door animation (IntroScn.pas:495,826,841,845): 10 frames, 300ms each.
const (
	doorFrameCount = 10
	doorFrameTime  = 300 * time.Millisecond
)

// Login input fields (IntroScn.pas:499-512).
var inputFields = []loginArea{
	{loginOX + 255, loginOY + 511, 112, 19}, // ID
	{loginOX + 495, loginOY + 511, 112, 19}, // Password
}

// Bottom buttons (FState.pas:763-774). Only pressed buttons are drawn; the
// normal-state artwork is baked into the ChrSel.wil[22] background.
var buttonAreas = []loginArea{
	{loginOX + 90, loginOY + 558, 70, 20},  // OK [62]
	{loginOX + 268, loginOY + 558, 70, 20}, // ChangePW [53]
	{loginOX + 447, loginOY + 558, 70, 20}, // NewAccount [61]
	{loginOX + 613, loginOY + 558, 70, 20}, // Close [64]
}

var buttonImages = []int{62, 53, 61, 64}

// Register window fields (IntroScn.pas:279-410), base nx=800/2-320=80,
// ny=600/2-238=62. Order: account, password, confirm, name, SSNo, birthday,
// quiz1, answer1, quiz2, answer2, phone, mobile, email.
type fieldDef struct {
	x, y, w, h float32
	maxLen     int
	masked     bool
	password   bool // rejects '~', ''', ' ' at input time (IntroScn.pas:640-641)
}

const (
	regNX = 80
	regNY = 62
)

var regFieldDefs = []fieldDef{
	{regNX + 86, regNY + 91, 104, 13, 10, false, false},   // account
	{regNX + 86, regNY + 118, 104, 13, 10, true, true},    // password
	{regNX + 86, regNY + 149, 104, 12, 10, true, true},    // confirm
	{regNX + 86, regNY + 190, 105, 13, 20, false, false},  // real name
	{regNX + 86, regNY + 207, 105, 13, 14, false, false},  // SSNo
	{regNX + 86, regNY + 217, 105, 13, 10, false, false},  // birthday
	{regNX + 263, regNY + 118, 124, 13, 20, false, false}, // quiz1
	{regNX + 263, regNY + 149, 124, 12, 12, false, false}, // answer1
	{regNX + 263, regNY + 190, 124, 13, 20, false, false}, // quiz2
	{regNX + 263, regNY + 218, 124, 13, 12, false, false}, // answer2
	{regNX + 263, regNY + 285, 124, 13, 14, false, false}, // phone
	{regNX + 263, regNY + 315, 124, 12, 13, false, false}, // mobile
	{regNX + 263, regNY + 368, 124, 13, 40, false, false}, // email
}

// Register window buttons (FState.pas:868-876): Ok [51], Cancel [52], Close [83].
var regButtonAreas = []loginArea{
	{loginOX + 305, loginOY + 530, 70, 20},
	{loginOX + 445, loginOY + 530, 70, 20},
	{loginOX + 587, loginOY + 33, 20, 20},
}

var regButtonImages = []int{51, 52, 83}

// Change-password window fields (IntroScn.pas:412-480), base nx=800/2-210=190,
// ny=600/2-150=150. Order: account, old password, new password, repeat.
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

// Change-password buttons relative to the centered [50] window
// (FState.pas:887-892): Ok [361] at +81,+141; Cancel [365] at +160,+141.
var chgButtonImages = []int{361, 365}

// serverInfo holds a server entry from SM_PASSOKSELECTSERVER.
type serverInfo struct {
	Name   string
	Status int
}

// Server-select dialog: Prguse[256] background, [79] buttons (pressed [80]),
// close [83] (FState.pas:810-847 English version). The Chinese version
// [160-166] images are 1×1 placeholders in this asset build.
const (
	srvDlgImg   = 256
	srvBtnImg   = 79
	srvCloseImg = 83
	srvCloseDX  = float32(245)
	srvCloseDY  = float32(31)
	srvCloseW   = float32(20)
	srvCloseH   = float32(20)
)

// srvButtonTop returns the window-relative Top of server button i for the
// given server count (FState.pas:2456-2474).
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

// serverDisplayName returns the button label and color for a server's status
// (FState.pas:2250-2272): 0 maintenance/clDkGray, 1 normal/clLime,
// 2 smooth/clGreen, 3 crowded/clMaroon, 4 full/clRed.
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

// parseServerList parses the server list from SM_PASSOKSELECTSERVER body.
// Body format: "name1/status1/name2/status2/..."
func parseServerList(body string) []serverInfo {
	var servers []serverInfo
	if body == "" {
		// Default server
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

// splitSlash splits a string by '/'.
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

// regHelps is the per-field registration help text drawn in clSilver next to
// the focused edit (IntroScn.pas:709-786; the Delphi literals are GBK
// mojibake, replaced with concise Chinese).
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

// LoginScene handles the login screen (Delphi TLoginScene, IntroScn.pas:62).
type LoginScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer // default 16px
	textSmall *engine.TextRenderer // 13px for input fields (Delphi Font.Size=10)
	textSrv   *engine.TextRenderer // 15px for server names (Delphi Font.Size=11)

	// Door animation (IntroScn.pas:804-851); fade handled by globalFade.
	doorOpening   bool
	doorFading    bool
	doorFrame     int
	doorStartTime time.Time

	// Mode (TLoginState)
	mode        loginMode
	showLoginUI bool

	// Login fields
	userID          string
	password        string
	focusedField    int // 0=id, 1=password, -1=none
	waitingResponse bool
	cursorBlink     time.Time

	// Register fields (13) / change-password fields (4)
	regFields [13]string
	regFocus  int
	chgFields [4]string
	chgFocus  int

	// Mouse: currently pressed button index per mode (-1 = none)
	pressedButton int

	// Modal dialog (Delphi DMessageDlg, FState.pas:1938-2158)
	dlgMsg        string
	dlgLines      []string
	dlgSize       int   // 0=small[381], 1=medium[360], 2=large[380]
	dlgButtons    []int // button image indices (361=Ok, 363=Yes, 365=Cancel, 367=No)
	dlgPressedBtn int   // index into dlgButtons, -1=none

	connecting bool

	// Server select overlay (Delphi DSelServerDlg, FState.pas:778-857)
	servers []serverInfo

	// Callbacks
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

// NewLoginScene creates a new login scene.
func NewLoginScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *LoginScene {
	s := &LoginScene{
		gl:            gl,
		resources:     resources,
		text:          text,
		textSmall:     text,
		textSrv:       text,
		showLoginUI:   true,
		focusedField:  0,
		pressedButton: -1,
		cursorBlink:   time.Now(),
	}
	// Delphi input fields use Font.Size=10 ≈ 13px @96DPI (IntroScn.pas:260).
	if t, err := text.WithSize(13); err == nil {
		s.textSmall = t
	}
	// Delphi server names use Font.Size=11 ≈ 15px @96DPI (FState.pas:2274).
	if t, err := text.WithSize(15); err == nil {
		s.textSrv = t
	}
	return s
}

// Open is called when the scene becomes active.
func (s *LoginScene) Open() {
	log.Logf(log.LevelInfo, "LoginScene", "Opened")
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
}

// Close is called when the scene becomes inactive.
func (s *LoginScene) Close() {
	log.Logf(log.LevelInfo, "LoginScene", "Closed")
}

// Update advances the door animation; the fade to black is handled by
// the global MakeDark system (ClMain.pas:1114-1130).
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
				log.Logf(log.LevelInfo, "LoginScene", "Door animation complete, starting fade out")
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

// Render renders the login scene.
func (s *LoginScene) Render(gl *engine.GLState, proj [16]float32) {
	ox, oy := loginOX, loginOY

	// Background: ChrSel.wil[22] (IntroScn.pas:818,821)
	bgTex, bgErr := s.getChrSelTexture(22)
	if bgErr == nil && bgTex != 0 {
		w, h := s.getChrSelSize(22)
		s.traceDraw("bg", "ChrSel", 22, ox, oy, float32(w), float32(h))
		gl.DrawQuad(bgTex, ox, oy, float32(w), float32(h), proj)
	} else {
		log.Logf(log.LevelWarn, "LoginScene", "ChrSel[22] background unavailable (tex=%d err=%v)", bgTex, bgErr)
		gl.DrawQuadColor(0, 0, 800, 600, 0.05, 0.05, 0.1, 1, proj)
	}

	// Door animation: ChrSel.wil[23..32] (IntroScn.pas:841,845)
	if s.doorOpening {
		doorIdx := 23 + s.doorFrame
		if doorTex, err := s.getChrSelTexture(doorIdx); err == nil {
			w, h := s.getChrSelSize(doorIdx)
			s.traceDraw("door", "ChrSel", doorIdx, ox+252, oy+106, float32(w), float32(h))
			gl.DrawQuad(doorTex, ox+252, oy+106, float32(w), float32(h), proj)
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
	}

	// Modal dialog on top of everything.
	if s.dlgMsg != "" {
		s.renderDialog(gl, proj)
	}
}

// renderButtons draws the four bottom login buttons. The Delphi original
// bakes normal-state art into ChrSel[22] and only draws the overlay when
// pressed (DLoginNewDirectPaint, FState.pas:2342-2354). Since this asset
// build's ChrSel[22] lacks button art, we always draw the Prguse images.
// Pressed state draws at the same position (no offset, FState.pas:2351).
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

// renderInputFields renders the account/password text and blinking cursor.
// Labels are baked into the background; Delphi's native TEdits are black-box
// white-text (IntroScn.pas:255-274).
func (s *LoginScene) renderInputFields(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}
	// After submit the edits are hidden while waiting for the server
	// (IntroScn.pas:551-554).
	if s.waitingResponse {
		return
	}

	idX, idY := ox+255, oy+511
	passX, passY := ox+495, oy+511
	masked := strings.Repeat("*", len(s.password))

	// TEdit Color=clBlack: opaque black box under the white text
	// (IntroScn.pas:258,270).
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
		// 1px white vertical line matching native TEdit caret (IntroScn.pas:255-274).
		gl.DrawQuadColor(cx, cy, 1, 19, 1, 1, 1, 1, proj)
	}
}

// windowOrigin returns the top-left of a centered Prguse window image.
func (s *LoginScene) windowOrigin(index int) (float32, float32, int, int) {
	w, h := s.getPrguseSize(index)
	return loginOX + float32(800-w)/2, loginOY + float32(600-h)/2, w, h
}

// renderRegisterWindow renders DNewAccount: background Prguse[63] centered,
// 13 black edit boxes, Ok[51]/Cancel[52]/Close[83] (FState.pas:862-876).
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
		// Title NewAccountTitle at (362,121), white + black outline, bold
		// (FState.pas:2669).
		log.Logf(log.LevelTrace, "Render", "login title pos=(%.0f,%.0f)", float32(362), float32(121))
		s.text.DrawTextBoldOutline("创建新账号", 362, 121, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		// Per-field help NAHelps in clSilver, switching with the focused edit
		// (IntroScn.pas:709-786; FState.pas:2664-2668, 507,124+i*14).
		if s.regFocus >= 0 && s.regFocus < len(regHelps) {
			s.textSmall.DrawText(regHelps[s.regFocus], 507, 124, 0.75, 0.75, 0.75, 1, proj)
		}
	}

	if s.text != nil && s.connecting {
		s.text.DrawText("注册中...", ox+350, oy+420, 0.5, 0.8, 1.0, 1.0, proj)
	}
}

// renderChgPwWindow renders DChgPw: background Prguse[50] centered, 4 edit
// boxes, Ok[361]/Cancel[365] at window+81,+141 / +160,+141 (FState.pas:881-892).
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

// srvWindowOrigin returns the runtime-centered top-left of the [256] dialog
// (FState.pas:813-814: (SCREENWIDTH-w)/2, (SCREENHEIGHT-h)/2).
func (s *LoginScene) srvWindowOrigin() (float32, float32) {
	w, h := s.getPrguseSize(srvDlgImg)
	return loginOX + float32(800-w)/2, loginOY + float32(600-h)/2
}

// renderServerSelect renders DSelServerDlg: dialog [256], up to six [79]
// buttons (pressed -> [80]), close [83] (FState.pas:810-847, 2220-2280).
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
			log.Logf(log.LevelTrace, "Render", "login srv-btn-text %q pos=(%.0f,%.0f)", name, tx, ty)
			s.textSrv.DrawTextBoldOutline(name, tx, ty, r, g, b, 1, 0, 0, 0, 1, proj)
		}
	}
}

// renderFieldGroup draws black edit boxes with white text (Delphi TEdit:
// Color=clBlack, Font.Color=clWhite, IntroScn.pas:255-274) plus the blinking
// cursor on the focused field.
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

// dlgGeom holds layout parameters for each DMessageDlg size
// (FState.pas:2002-2042).
type dlgGeom struct {
	bgImg                    int
	msgLX, msgLY             float32
	btnLX, btnLY             float32
}

var dlgSizes = [3]dlgGeom{
	{bgImg: 381, msgLX: 39, msgLY: 38, btnLX: 90, btnLY: 36},   // small
	{bgImg: 360, msgLX: 39, msgLY: 38, btnLX: 324, btnLY: 126}, // medium
	{bgImg: 380, msgLX: 23, msgLY: 20, btnLX: 105, btnLY: 305}, // large
}

// dlgButtonAreas returns the window rect and per-button rects for the current
// dialog. Buttons are laid out right-to-left from btnLX with 110px spacing
// (FState.pas:2060-2083).
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

// renderDialog renders DMessageDlg with the configured size and buttons
// (FState.pas:739-752, 2002-2083, 2291-2325).
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
		log.Logf(log.LevelTrace, "Render", "login dlg-text %q pos=(%.0f,%.0f)", ln, win.X+g.msgLX, y)
		s.text.DrawTextBoldOutline(ln, win.X+g.msgLX, y, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		y += 14
	}
}

// OnChar handles character input from GLFW.
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
			return // filtered at input time (IntroScn.pas:640-641)
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
		return // no text input on the server-select overlay
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

// OnKey handles keyboard input.
func (s *LoginScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	if !s.showLoginUI || s.doorOpening {
		return
	}
	if s.dlgMsg != "" {
		// DMsgDlgKeyDown: Enter/Esc close a single-Ok dialog (FState.pas:2139-2158).
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
			s.closeFunc() // DSelServerDlg close == FrmMain.Close
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
		// Enter on the account field moves focus to the password field
		// (EdLoginIdKeyPress, IntroScn.pas:530-539); Enter on the password
		// field submits (EdLoginPasswdKeyPress, :541-558).
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

// OnMouse handles mouse button input. Clicks fire on release inside the same
// region (TDButton.MouseUp, DWinCtl.pas:677-695).
func (s *LoginScene) OnMouse(x, y float64, button int, action int, mods int) {
	log.Logf(log.LevelDebug, "Mouse", "login pos=(%.0f,%.0f) button=%d action=%d", x, y, button, action)
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

// buttonArea returns the hit rectangle for bottom button i; its size comes
// from the button image (TDControl.SetImgIndex, DWinCtl.pas:607-621).
func (s *LoginScene) buttonArea(i int) loginArea {
	a := buttonAreas[i]
	if w, h := s.getPrguseSize(buttonImages[i]); w > 0 && h > 0 {
		a.W, a.H = float32(w), float32(h)
	}
	return a
}

// regButtonArea returns the hit rectangle for register button i.
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
		for i, field := range inputFields {
			if hitTest(fx, fy, field) {
				s.focusedField = i
				s.cursorBlink = time.Now()
				return
			}
		}
		for i := range buttonAreas {
			if hitTest(fx, fy, s.buttonArea(i)) {
				s.pressedButton = i
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

// mouseGroup is the shared press/release logic for the register window's
// field set and button row.
func (s *LoginScene) mouseGroup(fx, fy float32, action int, fields, buttons []loginArea, focus *int, onClick func(int)) {
	switch action {
	case mousePress:
		for i, field := range fields {
			if hitTest(fx, fy, field) {
				*focus = i
				s.cursorBlink = time.Now()
				return
			}
		}
		for i, btn := range buttons {
			if hitTest(fx, fy, btn) {
				s.pressedButton = i
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

// mouseChgPw handles the change-password window; its Ok/Cancel buttons are
// children of the centered [50] window (FState.pas:887-892).
func (s *LoginScene) mouseChgPw(fx, fy float32, action int) {
	wx, wy, _, _ := s.windowOrigin(50)
	buttons := make([]loginArea, 2)
	for i, off := range []loginArea{{wx + 81, wy + 141, 0, 0}, {wx + 160, wy + 141, 0, 0}} {
		w, h := s.getPrguseSize(chgButtonImages[i])
		buttons[i] = loginArea{off.X, off.Y, float32(w), float32(h)}
	}
	switch action {
	case mousePress:
		for i, def := range chgFieldDefs {
			if hitTest(fx, fy, loginArea{def.x, def.y, def.w, def.h}) {
				s.chgFocus = i
				s.cursorBlink = time.Now()
				return
			}
		}
		for i, btn := range buttons {
			if hitTest(fx, fy, btn) {
				s.pressedButton = i
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

// mouseServerSelect handles the DSelServerDlg overlay: press/release on a
// server button selects that server; the close button exits the app
// (FState.pas:2220-2224; DSelServerDlg close == FrmMain.Close).
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
				return
			}
		}
		if hitTest(fx, fy, closeArea) {
			s.pressedButton = 6
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
			s.mode = modeLogin // close the overlay once a server is picked
			if s.selectFunc != nil {
				s.selectFunc(name)
			}
		}
	}
}

// handleButton dispatches the four bottom login buttons.
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

// handleRegButton dispatches the register window buttons: Ok/Cancel/Close.
func (s *LoginScene) handleRegButton(index int) {
	if index == 0 {
		s.submitRegister()
		return
	}
	s.mode = modeLogin // NewAccountClose (IntroScn.pas:1072-1076)
}

// submitLogin validates input and triggers the login callback.
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
	s.waitingResponse = true // hide the edits while waiting (IntroScn.pas:551-554)
	pw := strings.ReplaceAll(strings.ReplaceAll(s.password, "~", "_"), "'", "_")
	id := strings.ToLower(s.userID) // m_sLoginId := LowerCase (IntroScn.pas:534,548)
	log.Logf(log.LevelInfo, "LoginScene", "Submitting login: %s", id)
	s.loginFunc(id, pw)
}

// submitRegister validates all fields (CheckUserEntrys, IntroScn.pas:976-1029)
// and sends the registration.
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
	if len(s.regFields[4]) < 1 { // non-English branch (:1022-1027)
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

	log.Logf(log.LevelInfo, "LoginScene", "Submitting registration: %s", ue.Account())
	s.connecting = true
	s.registerFunc(ue, ua)
	s.mode = modeLogin // NewAccountClose (:1068,1072-1076)
}

// validBirthDay mirrors NewIdCheckBirthDay (IntroScn.pas:609-632): yyyy/mm/dd.
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

// submitChgPw sends the password change request (ChgpwOk, IntroScn.pas:1078-1092).
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
	log.Logf(log.LevelInfo, "LoginScene", "Submitting password change: %s", s.chgFields[0])
	s.connecting = true
	s.chgpwFunc(s.chgFields[0], s.chgFields[1], s.chgFields[2])
	s.mode = modeLogin // ChgpwCancel (:1087,1094-1097)
	s.chgFields = [4]string{}
}

// ShowMessage displays a modal DMessageDlg with medium size and Ok button
// (FState.pas:1938-2158).
func (s *LoginScene) ShowMessage(msg string) {
	s.ShowMessageEx(msg, 1, []int{361})
}

// ShowMessageEx displays a modal DMessageDlg with the given size
// (0=small, 1=medium, 2=large) and button set (361=Ok, 363=Yes,
// 365=Cancel, 367=No). Buttons are laid out right-to-left
// (FState.pas:2002-2083).
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

// SetError is kept for existing call sites; it now shows a modal dialog.
func (s *LoginScene) SetError(msg string) {
	log.Logf(log.LevelWarn, "LoginScene", "Error: %s", msg)
	s.ShowMessage(msg)
}

// RegistrationDone handles SM_NEWID_SUCCESS: leave register mode and show the
// success dialog (ClMain.pas:3684-3691).
func (s *LoginScene) RegistrationDone() {
	s.mode = modeLogin
	s.regFields = [13]string{}
	s.ShowMessage("您的账号已经注册成功.\\请牢记您的账号和密码.\\请不要以任何原因将账号和密码告诉任何人.")
}

// RegistrationFailed handles SM_NEWID_FAIL: return to register mode with the
// fields preserved (NewIdRetry, IntroScn.pas:936-952).
func (s *LoginScene) RegistrationFailed(msg string) {
	s.mode = modeRegister
	s.ShowMessage(msg)
}

// ChgPwResult reports the password-change outcome dialog (ClMain.pas:3762-3771).
func (s *LoginScene) ChgPwResult(msg string) {
	s.ShowMessage(msg)
}

// SetLoginFunc sets the callback for login attempts.
func (s *LoginScene) SetLoginFunc(fn func(id, password string)) {
	s.loginFunc = fn
}

// SetRegisterFunc sets the callback for registration attempts.
func (s *LoginScene) SetRegisterFunc(fn func(ue protocol.UserEntry, ua protocol.UserEntryAdd)) {
	s.registerFunc = fn
}

// SetChgPwFunc sets the callback for password change attempts.
func (s *LoginScene) SetChgPwFunc(fn func(id, oldpw, newpw string)) {
	s.chgpwFunc = fn
}

// SetSelectFunc sets the callback for server selection.
func (s *LoginScene) SetSelectFunc(fn func(serverName string)) {
	s.selectFunc = fn
}

// ShowServerSelect opens the DSelServerDlg overlay on top of the login
// background instead of switching to a separate scene (FState.pas:2453-2517).
func (s *LoginScene) ShowServerSelect(servers []serverInfo) {
	log.Logf(log.LevelInfo, "LoginScene", "Showing server select: %d servers", len(servers))
	s.servers = servers
	s.mode = modeServerSelect
	s.pressedButton = -1
	s.showLoginUI = true
}

// Servers returns the stored server list.
func (s *LoginScene) Servers() []serverInfo {
	return s.servers
}

// SetCloseFunc sets the callback for closing the application.
func (s *LoginScene) SetCloseFunc(fn func()) {
	s.closeFunc = fn
}

// SetDoorCompleteFunc sets the callback for when the door animation finishes.
func (s *LoginScene) SetDoorCompleteFunc(fn func()) {
	s.doorCompleteFunc = fn
}

// OpenLoginDoor starts the door opening animation.
func (s *LoginScene) OpenLoginDoor() {
	log.Logf(log.LevelInfo, "LoginScene", "Opening door")
	s.doorOpening = true
	s.doorFading = false
	s.doorFrame = 0
	s.doorStartTime = time.Now()
	s.showLoginUI = false
}

// IsDoorFullyOpen returns true once the last door frame is shown.
func (s *LoginScene) IsDoorFullyOpen() bool {
	return s.doorOpening && s.doorFrame >= doorFrameCount-1
}

// OnScroll handles mouse scroll input.
func (s *LoginScene) OnScroll(x, y float64) {
}

// hitTest checks if (x, y) is inside the area.
func hitTest(x, y float32, a loginArea) bool {
	return x >= a.X && x <= a.X+a.W && y >= a.Y && y <= a.Y+a.H
}

// getChrSelTexture gets a texture from ChrSel.wil.
func (s *LoginScene) getChrSelTexture(index int) (uint32, error) {
	if s.resources.ChrSel == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.ChrSel, index), nil
}

// getChrSelSize gets the size of a texture from ChrSel.wil.
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

// getPrguseTexture gets a texture from Prguse.wil.
func (s *LoginScene) getPrguseTexture(index int) (uint32, error) {
	if s.resources.Prguse == nil {
		return 0, fmt.Errorf("resource not loaded")
	}
	return s.resources.GetTexture(s.resources.Prguse, index), nil
}

// getPrguseSize gets the size of a texture from Prguse.wil.
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
