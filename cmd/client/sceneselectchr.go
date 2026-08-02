package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

const (
	selectedFrame   = 16
	freezeFrame     = 13
	selectEffFrames = 14 // ChrSel[4..17] 选中特效循环 (IntroScn:1442-1454)
	darkLevelMax    = 30 // 亮度渐变遮罩起始值 (IntroScn:1299)
)

// CharacterSlot 表示选角界面中的一个角色。
type CharacterSlot struct {
	Name  string
	Job   byte
	Hair  byte
	Level byte
	Sex   byte
	Valid bool

	FreezeState bool
	Unfreezing  bool
	Freezing    bool
	AniIndex    int
	AniTick     time.Time

	// DarkLevel 驱动选中时的亮度渐变：立绘上覆盖一层黑色遮罩，
	// 透明度为 DarkLevel/darkLevelMax，每 25ms 递减一次直到消失
	// (IntroScn:1299,1485-1494,1510-1514)。
	DarkLevel int
	DarkTick  time.Time
}

// jobNames 将职业 ID 映射为显示名称。
var jobNames = []string{"战士", "法师", "道士"}

// SelectChrScene 处理角色选择界面 (stSelectChr)。
type SelectChrScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	ui *UIManager

	Characters [2]CharacterSlot
	Selected   int

	errorMsg string

	createMode  bool
	createName  string
	createJob   int
	createSex   int
	createIndex int
	cursorBlink time.Time

	deleteConfirm bool
	deleteName    string

	selEffIndex int
	selEffTick  time.Time

	// ServerName 绘制在画面顶部居中位置 (IntroScn:1539-1545)。
	ServerName string

	startFunc  func(charName string)
	exitFunc   func()
	newChrFunc func(name string, hair, job, sex int)
	delChrFunc func(name string)

	mainButtons [6]*UIControl
	createPanel *UIControl
	deletePanel *UIControl
	editName    *EditBox
	jobBtns     [3]*UIControl
	sexBtns     [2]*UIControl
}

const (
	selOX = float32(0)
	selOY = float32(0)
)

// selButtonAreas 是选角界面按钮的碰撞矩形；尺寸取自 Delphi 通过
// SetImgIndex 获取的图片范围 (FState:904-925)。
var selButtonAreas = []loginArea{
	{selOX + 134, selOY + 454, 76, 33},  // ImgSelSelect1 [66]
	{selOX + 685, selOY + 454, 76, 33},  // ImgSelSelect2 [67]
	{selOX + 385, selOY + 456, 44, 21},  // ImgSelStart [68]
	{selOX + 348, selOY + 486, 120, 21}, // ImgSelNewChr [69]
	{selOX + 347, selOY + 506, 120, 21}, // ImgSelErase [70]
	{selOX + 379, selOY + 547, 56, 20},  // ImgSelExit [72]
}

// selButtons 将每个选择按钮映射到其 Prguse 图片和屏幕坐标。
// 按钮面板烘焙在背景 [65] 中，仅在按下时叠加绘制。
var selButtons = []struct {
	img  int
	x, y float32
}{
	{ImgSelSelect1, selOX + 134, selOY + 454},
	{ImgSelSelect2, selOX + 685, selOY + 454},
	{ImgSelStart, selOX + 385, selOY + 456},
	{ImgSelNewChr, selOX + 348, selOY + 486},
	{ImgSelErase, selOX + 347, selOY + 506},
	{ImgSelExit, selOX + 379, selOY + 547},
}

// NewSelectChrScene 创建角色选择场景。
func NewSelectChrScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *SelectChrScene {
	return &SelectChrScene{
		gl:        gl,
		resources: resources,
		text:      text,
		Selected:  -1,
	}
}

func (s *SelectChrScene) Open() {
	log.Logf(log.LevelInfo, "SelectChrScene", "opened")
	gSound.PlayBGM(bmgSelect)
	s.errorMsg = ""
	s.createMode = false
	s.deleteConfirm = false
	s.ui = NewUIManager(s.gl, s.resources, s.text)
	s.buildUI()
	gActiveUI = s.ui
	s.registerDebugCmds()
}

func (s *SelectChrScene) Close() {
	s.unregisterDebugCmds()
	gActiveUI = nil
	gSound.SilenceSound()
	log.Logf(log.LevelInfo, "SelectChrScene", "closed")
}

// ---------------------------------------------------------------------------
// 控件树构建
// ---------------------------------------------------------------------------

func (s *SelectChrScene) buildUI() {
	prg := s.resources.Prguse

	// Root 绘制所有非交互游戏画面：背景、角色立绘、动画、文字。
	s.ui.Root.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		s.paintBackground(proj)
		for i := 0; i < 2; i++ {
			s.paintCharSlot(proj, i)
		}
		s.paintInfoText(proj)
	}

	// --- 6 个主按钮（烘焙在背景中，仅按下时叠加绘制）---
	btnNames := [6]string{"BtnSelect1", "BtnSelect2", "BtnStart", "BtnNewChr", "BtnErase", "BtnExit"}
	for i, area := range selButtonAreas {
		idx := i
		btn := NewUIControl(btnNames[i], KindButton)
		btn.Left = int(area.X)
		btn.Top = int(area.Y)
		btn.Width = int(area.W)
		btn.Height = int(area.H)
		btn.ClickSound = sRockButtonClick
		btn.OnClick = func(c *UIControl, x, y int) { s.handleButton(idx) }
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if c.Downed && prg != nil {
				s.ui.BlitImage(prg, selButtons[idx].img, c.AbsX(), c.AbsY(), proj)
			}
		}
		s.ui.Root.AddChild(btn)
		s.mainButtons[i] = btn
	}

	// --- 创建角色面板 ---
	cp := NewUIControl("DCreatePanel", KindWindow)
	if prg != nil {
		cp.SetImgIndex(prg, ImgCreateBg)
	} else {
		cp.Width, cp.Height = 260, 320
	}
	cp.Visible = false
	s.ui.Root.AddChild(cp)
	s.createPanel = cp

	s.editName = NewEditBox(s.gl, s.text, "EdChrName", 128, 20)
	s.editName.MaxLen = 14
	s.editName.Ctrl.Left = 62
	s.editName.Ctrl.Top = 104
	cp.AddChild(s.editName.Ctrl)

	// 子控件坐标实测自运行资源 Prguse.wil #73 的烘焙布局 (300x417)；
	// 职业/性别 left = #73 顶高光左-1，使选中高亮 (55-59) 与烘焙灰图逐像素对齐。
	jobLefts := [3]int{47, 92, 137}
	for i := 0; i < 3; i++ {
		idx := i
		jb := NewUIControl(fmt.Sprintf("BtnJob%d", i), KindButton)
		if prg != nil {
			jb.SetImgIndex(prg, ImgCreateJob1+i)
		} else {
			jb.Width, jb.Height = 44, 36
		}
		jb.Left = jobLefts[i]
		jb.Top = 156
		jb.OnClick = func(c *UIControl, x, y int) { s.createJob = idx }
		jb.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if prg == nil {
				return
			}
			if c.Downed {
				s.ui.BlitImage(prg, ImgCreateJob1+idx, c.AbsX(), c.AbsY(), proj)
			} else if s.createJob == idx {
				s.ui.BlitImage(prg, ImgClassHi1+idx, c.AbsX(), c.AbsY(), proj)
			}
			// 普通态不绘制：灰色图标已烘焙在 ImgCreateBg(#73) 中 (FState:2725-2761)
		}
		cp.AddChild(jb)
		s.jobBtns[i] = jb
	}

	sexLefts := [2]int{92, 137}
	sexHi := [2]int{ImgGenderHiM, ImgGenderHiF}
	for i := 0; i < 2; i++ {
		idx := i
		sb := NewUIControl(fmt.Sprintf("BtnSex%d", i), KindButton)
		if prg != nil {
			sb.SetImgIndex(prg, ImgCreateMale+i)
		} else {
			sb.Width, sb.Height = 44, 35
		}
		sb.Left = sexLefts[i]
		sb.Top = 230
		sb.OnClick = func(c *UIControl, x, y int) { s.createSex = idx }
		sb.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			if prg == nil {
				return
			}
			if c.Downed {
				s.ui.BlitImage(prg, ImgCreateMale+idx, c.AbsX(), c.AbsY(), proj)
			} else if s.createSex == idx {
				s.ui.BlitImage(prg, sexHi[idx], c.AbsX(), c.AbsY(), proj)
			}
			// 普通态不绘制：灰色图标已烘焙在 ImgCreateBg(#73) 中 (FState:2725-2761)
		}
		cp.AddChild(sb)
		s.sexBtns[i] = sb
	}

	okBtn := NewUIControl("BtnCreateOk", KindButton)
	okBtn.Width, okBtn.Height = 90, 29
	okBtn.Left = 95
	okBtn.Top = 359
	okBtn.OnClick = func(c *UIControl, x, y int) { s.confirmCreate() }
	cp.AddChild(okBtn)

	closeBtn := NewUIControl("BtnCreateClose", KindButton)
	closeBtn.Width, closeBtn.Height = 18, 20
	closeBtn.Left = 246
	closeBtn.Top = 30
	closeBtn.OnClick = func(c *UIControl, x, y int) { s.createMode = false }
	cp.AddChild(closeBtn)

	// --- 删除确认面板 ---
	dp := NewUIControl("DDeletePanel", KindWindow)
	if prg != nil {
		dp.SetImgIndex(prg, ImgModalNormal)
	} else {
		dp.Width, dp.Height = 380, 180
	}
	dp.Left = (ScreenWidth - dp.Width) / 2
	dp.Top = (ScreenHeight - dp.Height) / 2
	dp.Visible = false
	dp.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		s.gl.DrawQuadColor(0, 0, float32(ScreenWidth), float32(ScreenHeight), 0, 0, 0, 0.5, proj)
		if prg != nil {
			s.ui.BlitImage(prg, ImgModalNormal, c.AbsX(), c.AbsY(), proj)
		} else {
			s.gl.DrawQuadColor(float32(c.AbsX()), float32(c.AbsY()),
				float32(c.Width), float32(c.Height), 0.12, 0.12, 0.2, 0.95, proj)
		}
		if s.text != nil {
			msg := fmt.Sprintf("确定删除角色 \"%s\"？", s.deleteName)
			s.text.DrawTextOutline(msg, float32(c.AbsX())+39, float32(c.AbsY())+38,
				1, 1, 1, 1, 0, 0, 0, 1, proj)
		}
	}
	s.ui.Root.AddChild(dp)
	s.deletePanel = dp

	delImgs := [3]int{ImgModalYes, ImgModalNo, ImgModalCancel}
	delNames := [3]string{"BtnDelYes", "BtnDelNo", "BtnDelCancel"}
	delXs := [3]int{104, 214, 324}
	for i := 0; i < 3; i++ {
		idx := i
		btn := NewUIControl(delNames[i], KindButton)
		if prg != nil {
			btn.SetImgIndex(prg, delImgs[i])
		} else {
			btn.Width, btn.Height = 96, 34
		}
		btn.Left = delXs[i]
		btn.Top = 126
		btn.ClickSound = sRockButtonClick
		btn.OnClick = func(c *UIControl, x, y int) {
			if idx == 0 {
				s.confirmDelete()
			} else {
				s.deleteConfirm = false
			}
		}
		btnImg := delImgs[i]
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			imgIdx := btnImg
			if c.Downed {
				imgIdx++
			}
			if prg != nil {
				s.ui.BlitImage(prg, imgIdx, c.AbsX(), c.AbsY(), proj)
			}
		}
		dp.AddChild(btn)
	}
}

func (s *SelectChrScene) syncUI() {
	for _, btn := range s.mainButtons {
		if btn != nil {
			btn.Visible = !s.createMode && !s.deleteConfirm
		}
	}
	if s.createPanel != nil {
		s.createPanel.Visible = s.createMode
		if s.createMode {
			if s.createIndex == 1 {
				s.createPanel.Left, s.createPanel.Top = 87, 63
			} else {
				s.createPanel.Left, s.createPanel.Top = 469, 63
			}
		}
	}
	if s.deletePanel != nil {
		s.deletePanel.Visible = s.deleteConfirm
	}
	if !s.createMode && s.ui != nil && s.editName != nil && s.ui.Focused == s.editName.Ctrl {
		s.ui.ReleaseFocus()
	}
}

// ---------------------------------------------------------------------------
// 更新 / 渲染
// ---------------------------------------------------------------------------

func (s *SelectChrScene) Update(dt float64) {
	now := time.Now()
	for i := 0; i < 2; i++ {
		ch := &s.Characters[i]
		if !ch.Valid {
			continue
		}
		if ch.DarkLevel > 0 && now.Sub(ch.DarkTick) > 25*time.Millisecond {
			ch.DarkTick = now
			ch.DarkLevel--
		}
		if ch.Unfreezing {
			if now.Sub(ch.AniTick) > 50*time.Millisecond {
				ch.AniTick = now
				ch.AniIndex++
				if ch.AniIndex >= freezeFrame {
					ch.Unfreezing = false
					ch.FreezeState = false
					ch.AniIndex = 0
				}
			}
		} else if ch.Freezing {
			if now.Sub(ch.AniTick) > 50*time.Millisecond {
				ch.AniTick = now
				ch.AniIndex++
				if ch.AniIndex >= freezeFrame {
					ch.Freezing = false
					ch.FreezeState = true
					ch.AniIndex = 0
				}
			}
		} else if !ch.FreezeState && i == s.Selected {
			if now.Sub(ch.AniTick) > 300*time.Millisecond {
				ch.AniTick = now
				ch.AniIndex = (ch.AniIndex + 1) % selectedFrame
			}
		}
	}

	if s.Selected >= 0 && s.Selected < 2 && s.Characters[s.Selected].Valid {
		if now.Sub(s.selEffTick) > 50*time.Millisecond {
			s.selEffTick = now
			s.selEffIndex = (s.selEffIndex + 1) % selectEffFrames
		}
	}
}

func (s *SelectChrScene) Render(gl *engine.GLState, proj [16]float32) {
	s.syncUI()
	if s.ui != nil {
		s.ui.Paint(proj)
	}
}

// ---------------------------------------------------------------------------
// Root.OnDirectPaint 绘制辅助
// ---------------------------------------------------------------------------

func (s *SelectChrScene) paintBackground(proj [16]float32) {
	if s.resources.Prguse != nil {
		tex := s.resources.GetTexture(s.resources.Prguse, ImgSelBg)
		if tex != 0 {
			w, h := s.getPrguseSize(ImgSelBg)
			s.gl.DrawQuad(tex, selOX, selOY, float32(w), float32(h), proj)
			return
		}
	}
	s.gl.DrawQuadColor(0, 0, float32(ScreenWidth), float32(ScreenHeight), 0.1, 0.15, 0.1, 1.0, proj)
}

func (s *SelectChrScene) paintCharSlot(proj [16]float32, idx int) {
	ox, oy := selOX, selOY
	gl := s.gl

	if s.createMode && idx == s.createIndex {
		s.paintCreatePreview(proj, idx)
		return
	}

	ch := s.Characters[idx]
	if !ch.Valid {
		return
	}

	job := int(ch.Job)
	sex := int(ch.Sex)
	if job > 2 {
		job = 0
	}
	if sex > 1 {
		sex = 0
	}

	var imgIdx int
	var drawX, drawY float32
	standing := !ch.FreezeState && !ch.Unfreezing && !ch.Freezing
	if ch.FreezeState || ch.Freezing {
		frame := 0
		if ch.Freezing {
			frame = freezeFrame - 1 - ch.AniIndex
		}
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120 + frame
		drawX, drawY = s.slotPos(job, sex, idx, ox, oy)
	} else if ch.Unfreezing {
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120 + ch.AniIndex
		drawX, drawY = s.slotPos(job, sex, idx, ox, oy)
	} else if idx == s.Selected {
		imgIdx = 40 + int(ch.Job)*40 + int(ch.Sex)*120 + ch.AniIndex%selectedFrame
		drawX, drawY = s.standingPos(job, sex, idx, ox, oy)
	} else {
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120
		drawX, drawY = s.slotPos(job, sex, idx, ox, oy)
	}

	drawn := false
	var sprW, sprH float32
	if s.resources.ChrSel != nil && imgIdx >= 0 && imgIdx < s.resources.ChrSel.Count {
		img := s.resources.ChrSel.GetImage(imgIdx)
		if img != nil && img.Width > 0 && img.Height > 0 {
			tex := s.resources.GetTexture(s.resources.ChrSel, imgIdx)
			if tex != 0 {
				sprW, sprH = float32(img.Width), float32(img.Height)
				gl.DrawQuad(tex, drawX, drawY, sprW, sprH, proj)
				drawn = true
			}
		}
	}

	if !drawn {
		var r, g, b float32
		switch ch.Job {
		case 0:
			r, g, b = 0.8, 0.3, 0.3
		case 1:
			r, g, b = 0.3, 0.3, 0.8
		case 2:
			r, g, b = 0.3, 0.8, 0.3
		}
		sprW, sprH = 100, 200
		gl.DrawQuadColor(drawX, drawY, sprW, sprH, r, g, b, 0.8, proj)
	}

	if standing && idx == s.Selected && ch.DarkLevel > 0 {
		alpha := float32(ch.DarkLevel) / darkLevelMax
		gl.DrawQuadColor(drawX, drawY, sprW, sprH, 0, 0, 0, alpha, proj)
	}

	if ch.Unfreezing {
		s.paintSelectEffect(proj, idx)
	}
}

// paintCreatePreview 为空槽位绘制实时创建预览 (IntroScn:1339-1353)。
func (s *SelectChrScene) paintCreatePreview(proj [16]float32, idx int) {
	job, sex := s.createJob, s.createSex
	if job > 2 {
		job = 0
	}
	if sex > 1 {
		sex = 0
	}
	slotX, slotY := s.standingPos(job, sex, idx, selOX, selOY)
	frame := int(time.Now().UnixMilli()/300) % selectedFrame
	imgIdx := 40 + job*40 + sex*120 + frame
	if s.resources.ChrSel == nil || imgIdx >= s.resources.ChrSel.Count {
		return
	}
	img := s.resources.ChrSel.GetImage(imgIdx)
	if img == nil || img.Width == 0 || img.Height == 0 {
		return
	}
	tex := s.resources.GetTexture(s.resources.ChrSel, imgIdx)
	if tex == 0 {
		return
	}
	s.gl.DrawQuad(tex, slotX, slotY, float32(img.Width), float32(img.Height), proj)
}

// paintSelectEffect 绘制 14 帧选中光效 (ChrSel[4..17]) (IntroScn:1442-1454)。
func (s *SelectChrScene) paintSelectEffect(proj [16]float32, idx int) {
	if s.resources.ChrSel == nil {
		return
	}
	imgIdx := 4 + s.selEffIndex
	if imgIdx >= s.resources.ChrSel.Count {
		return
	}
	img := s.resources.ChrSel.GetImage(imgIdx)
	if img == nil || img.Width == 0 || img.Height == 0 {
		return
	}
	tex := s.resources.GetTexture(s.resources.ChrSel, imgIdx)
	if tex == 0 {
		return
	}
	ex, ey := float32(90), float32(58)
	if idx == 1 {
		ex, ey = 430, 60
	}
	s.gl.DrawQuadAdditive(tex, selOX+ex, selOY+ey, float32(img.Width), float32(img.Height), proj)
}

func (s *SelectChrScene) paintInfoText(proj [16]float32) {
	if s.text == nil {
		return
	}
	ox, oy := selOX, selOY

	type textPos struct{ nameX, nameY, levelX, levelY, jobX, jobY float32 }
	textPositions := [2]textPos{
		{136, 476, 136, 513, 136, 548},
		{586, 476, 666, 513, 638, 548},
	}

	for i := 0; i < 2; i++ {
		ch := s.Characters[i]
		if !ch.Valid {
			continue
		}
		tp := textPositions[i]
		s.text.DrawTextOutline(ch.Name, ox+tp.nameX, oy+tp.nameY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		levelStr := fmt.Sprintf("%d", ch.Level)
		s.text.DrawTextOutline(levelStr, ox+tp.levelX, oy+tp.levelY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		jobName := "未知"
		if int(ch.Job) < len(jobNames) {
			jobName = jobNames[ch.Job]
		}
		s.text.DrawTextOutline(jobName, ox+tp.jobX, oy+tp.jobY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	if s.ServerName != "" {
		x := float32(ScreenWidth)/2 - float32(s.text.MeasureText(s.ServerName))/2
		s.text.DrawTextOutline(s.ServerName, x, oy+8, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+250, oy+400, 1.0, 0.3, 0.3, 1.0, proj)
	}
}

// slotPos 返回指定职业/性别在槽位 idx 中冻结帧的左上角坐标
// (IntroScn:1390-1438)。
func (s *SelectChrScene) slotPos(job, sex, idx int, ox, oy float32) (float32, float32) {
	positions := [3][2][2]float32{
		{{71, 52}, {65, 55}},
		{{77, 46}, {171, 97}},
		{{85, 63}, {164, 103}},
	}
	x := ox + positions[job][sex][0]
	y := oy + positions[job][sex][1]
	if idx == 1 {
		x += 340
		y += 2
	}
	return x, y
}

// standingPos 返回站立帧的左上角坐标 (IntroScn:1413-1427)。
func (s *SelectChrScene) standingPos(job, sex, idx int, ox, oy float32) (float32, float32) {
	bx, by := s.slotPos(job, sex, idx, ox, oy)
	switch {
	case job == 1 && sex == 1:
		return bx - 30, by - 14
	case job == 2 && sex == 1:
		return bx - 23, by - 20
	}
	return bx, by
}

// ---------------------------------------------------------------------------
// 输入路由
// ---------------------------------------------------------------------------

func (s *SelectChrScene) OnChar(char rune) {
	if s.ui != nil {
		s.ui.RouteChar(char)
	}
}

func (s *SelectChrScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}
	if s.ui != nil {
		s.ui.RouteKeyDown(key)
	}

	if s.createMode {
		switch key {
		case keyEnter, keyKPEnter:
			s.confirmCreate()
		case keyEscape:
			s.createMode = false
		}
		return
	}
	if s.deleteConfirm {
		switch key {
		case keyEnter, keyKPEnter:
			s.confirmDelete()
		case keyEscape:
			s.deleteConfirm = false
		}
		return
	}
	if key == keyEnter || key == keyKPEnter {
		s.startGame()
	}
}

func (s *SelectChrScene) OnMouse(x, y float64, button int, action int, mods int) {
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

func (s *SelectChrScene) OnScroll(x, y float64) {}

// ---------------------------------------------------------------------------
// 按钮 / 业务逻辑
// ---------------------------------------------------------------------------

func (s *SelectChrScene) handleButton(index int) {
	gSound.PlaySound(sRockButtonClick)
	switch index {
	case 0:
		s.selectChar(0)
	case 1:
		s.selectChar(1)
	case 2:
		s.startGame()
	case 3:
		s.startCreate()
	case 4:
		s.startDelete()
	case 5:
		if s.exitFunc != nil {
			s.exitFunc()
		}
	}
}

func (s *SelectChrScene) selectChar(idx int) {
	if idx == s.Selected {
		return
	}
	if !s.Characters[idx].Valid {
		return
	}
	gSound.PlaySound(sMeltstone)
	log.Logf(log.LevelInfo, "SelectChr", "character selected %d: %s", idx, s.Characters[idx].Name)

	if s.Selected >= 0 && s.Selected < 2 && s.Characters[s.Selected].Valid {
		prev := &s.Characters[s.Selected]
		if !prev.FreezeState {
			prev.Freezing = true
			prev.Unfreezing = false
			prev.AniIndex = 0
			prev.AniTick = time.Now()
		}
	}

	s.Selected = idx
	s.errorMsg = ""
	ch := &s.Characters[idx]
	if ch.FreezeState {
		ch.Unfreezing = true
		ch.Freezing = false
		ch.AniIndex = 0
		ch.AniTick = time.Now()
		ch.DarkLevel = 0
	}
}

func (s *SelectChrScene) startCreate() {
	emptyIdx := -1
	for i := 0; i < 2; i++ {
		if !s.Characters[i].Valid {
			emptyIdx = i
			break
		}
	}
	if emptyIdx < 0 {
		s.errorMsg = "最多创建2个角色"
		return
	}
	s.createMode = true
	s.createName = ""
	s.createJob = 0
	s.createSex = 0
	s.createIndex = emptyIdx
	s.cursorBlink = time.Now()
	s.errorMsg = ""
	if s.editName != nil {
		s.editName.Clear()
	}
	if s.ui != nil && s.editName != nil {
		s.ui.SetFocus(s.editName.Ctrl)
	}
	log.Logf(log.LevelInfo, "SelectChr", "enter create character mode, slot=%d", emptyIdx)
}

func (s *SelectChrScene) confirmCreate() {
	name := ""
	if s.editName != nil {
		name = s.editName.Text
	}
	s.createName = name
	if len([]rune(name)) == 0 {
		s.errorMsg = "请输入角色名"
		return
	}
	hair := 1 + rand.Intn(5)
	log.Logf(log.LevelInfo, "SelectChr", "create character: %s job=%d sex=%d hair=%d", name, s.createJob, s.createSex, hair)
	s.createMode = false
	if s.newChrFunc != nil {
		s.newChrFunc(name, hair, s.createJob, s.createSex)
	}
}

func (s *SelectChrScene) startDelete() {
	if s.Selected < 0 || s.Selected >= 2 || !s.Characters[s.Selected].Valid {
		s.errorMsg = "请先选择一个角色"
		return
	}
	s.deleteConfirm = true
	s.deleteName = s.Characters[s.Selected].Name
	s.errorMsg = ""
	log.Logf(log.LevelInfo, "SelectChr", "delete confirm: %s", s.deleteName)
}

func (s *SelectChrScene) confirmDelete() {
	log.Logf(log.LevelInfo, "SelectChr", "delete character: %s", s.deleteName)
	s.deleteConfirm = false
	if s.delChrFunc != nil {
		s.delChrFunc(s.deleteName)
	}
}

func (s *SelectChrScene) startGame() {
	if s.Selected < 0 || s.Selected >= 2 || !s.Characters[s.Selected].Valid {
		s.errorMsg = "请先选择一个角色"
		return
	}
	charName := s.Characters[s.Selected].Name
	log.Logf(log.LevelInfo, "SelectChrScene", "start game, character: %s", charName)
	if s.startFunc != nil {
		s.startFunc(charName)
	}
}

// ---------------------------------------------------------------------------
// 回调设置 / 外部 API
// ---------------------------------------------------------------------------

func (s *SelectChrScene) SetStartFunc(fn func(charName string))    { s.startFunc = fn }
func (s *SelectChrScene) SetExitFunc(fn func())                    { s.exitFunc = fn }
func (s *SelectChrScene) SetNewChrFunc(fn func(name string, hair, job, sex int)) {
	s.newChrFunc = fn
}
func (s *SelectChrScene) SetDelChrFunc(fn func(name string)) { s.delChrFunc = fn }
func (s *SelectChrScene) SetError(msg string)                { s.errorMsg = msg }
func (s *SelectChrScene) SetServerName(name string)          { s.ServerName = name }

func (s *SelectChrScene) SetCharactersFromServer(chars []parsedChar, selectedIdx int) {
	log.Logf(log.LevelInfo, "SelectChrScene", "SetCharactersFromServer: %d characters, selectedIdx=%d", len(chars), selectedIdx)
	s.Characters = [2]CharacterSlot{}
	s.Selected = -1
	s.createMode = false
	s.deleteConfirm = false

	for i, c := range chars {
		if i >= 2 {
			break
		}
		s.Characters[i] = CharacterSlot{
			Name:        c.Name,
			Job:         byte(c.Job),
			Hair:        byte(c.Hair),
			Level:       byte(c.Level),
			Sex:         byte(c.Sex),
			Valid:       true,
			FreezeState: true,
			AniTick:     time.Now(),
		}
		log.Logf(log.LevelInfo, "SelectChrScene", "  character %d: %s Lv%d Job=%d Sex=%d",
			i, c.Name, c.Level, c.Job, c.Sex)
	}

	if selectedIdx >= 0 && selectedIdx < 2 && s.Characters[selectedIdx].Valid {
		s.Selected = selectedIdx
		s.Characters[selectedIdx].FreezeState = false
	} else if s.Characters[0].Valid {
		s.Selected = 0
		s.Characters[0].FreezeState = false
	}
	log.Logf(log.LevelInfo, "SelectChrScene", "final selected=%d", s.Selected)
}

func (s *SelectChrScene) getPrguseSize(index int) (int, int) {
	if s.resources.Prguse == nil || index >= s.resources.Prguse.Count {
		return 0, 0
	}
	img := s.resources.Prguse.GetImage(index)
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}

// ---------------------------------------------------------------------------
// 调试命令
// ---------------------------------------------------------------------------

func (s *SelectChrScene) registerDebugCmds() {
	dc := gDebug
	if dc == nil {
		return
	}
	dc.Register("cstate", "dump character select state", func(args []string) {
		dc.Printf("selected=%d createMode=%v deleteConfirm=%v error=%q",
			s.Selected, s.createMode, s.deleteConfirm, s.errorMsg)
		for i := range s.Characters {
			ch := &s.Characters[i]
			if !ch.Valid {
				dc.Printf("  slot%d: empty", i)
				continue
			}
			dc.Printf("  slot%d: %s lv%d %s/%s freeze=%v unfrz=%v frz=%v ani=%d dark=%d",
				i, ch.Name, ch.Level, jobNames[ch.Job],
				map[byte]string{0: "M", 1: "F"}[ch.Sex],
				ch.FreezeState, ch.Unfreezing, ch.Freezing, ch.AniIndex, ch.DarkLevel)
		}
		if s.createMode {
			name := ""
			if s.editName != nil {
				name = s.editName.Text
			}
			dc.Printf("  create: name=%q job=%d sex=%d index=%d",
				name, s.createJob, s.createSex, s.createIndex)
		}
		if s.ui != nil {
			dc.Printf("  ui: focus=%s", debugCtlName(s.ui.Focused))
		}
	})
	dc.Register("csel", "csel <0|1> — force select character slot", func(args []string) {
		if len(args) == 0 {
			dc.Printf("usage: csel <0|1>")
			return
		}
		idx := 0
		fmt.Sscanf(args[0], "%d", &idx)
		if idx < 0 || idx > 1 {
			dc.Printf("slot must be 0 or 1")
			return
		}
		if !s.Characters[idx].Valid {
			dc.Printf("slot %d is empty", idx)
			return
		}
		s.selectChar(idx)
		dc.Printf("selected slot %d: %s", idx, s.Characters[idx].Name)
	})
	dc.Register("ccreate", "ccreate [on|off] — toggle create dialog", func(args []string) {
		if len(args) >= 1 {
			switch strings.ToLower(args[0]) {
			case "on":
				s.createMode = true
			case "off":
				s.createMode = false
			}
		} else {
			s.createMode = !s.createMode
		}
		if s.createMode {
			s.createIndex = 0
			if s.Characters[0].Valid {
				s.createIndex = 1
			}
			s.createName = ""
			s.createJob = 0
			s.createSex = 0
			if s.editName != nil {
				s.editName.Clear()
			}
		}
		dc.Printf("createMode=%v (index=%d)", s.createMode, s.createIndex)
	})
	dc.Register("cdel", "cdel [on|off] — toggle delete confirm", func(args []string) {
		if len(args) >= 1 {
			switch strings.ToLower(args[0]) {
			case "on":
				if s.Selected >= 0 && s.Characters[s.Selected].Valid {
					s.deleteConfirm = true
					s.deleteName = s.Characters[s.Selected].Name
				} else {
					dc.Printf("no valid character selected")
					return
				}
			case "off":
				s.deleteConfirm = false
			}
		} else {
			if !s.deleteConfirm {
				if s.Selected >= 0 && s.Characters[s.Selected].Valid {
					s.deleteConfirm = true
					s.deleteName = s.Characters[s.Selected].Name
				} else {
					dc.Printf("no valid character selected")
					return
				}
			} else {
				s.deleteConfirm = false
			}
		}
		dc.Printf("deleteConfirm=%v name=%q", s.deleteConfirm, s.deleteName)
	})
}

func (s *SelectChrScene) unregisterDebugCmds() {
	if gDebug == nil {
		return
	}
	for _, name := range []string{"cstate", "csel", "ccreate", "cdel"} {
		gDebug.Unregister(name)
	}
}
