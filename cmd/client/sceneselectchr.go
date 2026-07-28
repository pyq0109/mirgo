package main

import (
	"fmt"
	"math/rand"
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

	Characters [2]CharacterSlot
	Selected   int

	errorMsg string

	createMode  bool
	createName  string
	createJob   int
	createSex   int
	createIndex int
	createDown  int // 创建对话框中按下的按钮 (-1 无; 0-2 职业, 3-4 性别)
	cursorBlink time.Time

	deleteConfirm bool
	deleteName    string

	downButton  int // 当前按下的选择按钮 (-1 无)，用于仅绘制按下态
	selEffIndex int
	selEffTick  time.Time

	// ServerName 绘制在画面顶部居中位置 (IntroScn:1539-1545)。
	// 对应 GameState.ServerName；当前未赋值，因为 SMSelectServerOK
	// 的消息体只有 addr/port/cert，不携带服务器名，且网络流程刻意未改动。
	// 仅在非空时绘制。
	ServerName string

	startFunc  func(charName string)
	exitFunc   func()
	newChrFunc func(name string, hair, job, sex int)
	delChrFunc func(name string)

	compLogged       bool // 主界面布局日志（仅输出一次）
	createDlgLogged  bool // 创建对话框布局日志（仅输出一次）
	deleteDlgLogged  bool // 删除对话框布局日志（仅输出一次）
}

const (
	selOX = float32(0)
	selOY = float32(0)
)

// selButtonAreas 是选角界面按钮的碰撞矩形；尺寸取自 Delphi 通过
// SetImgIndex 获取的图片范围 (FState:904-925)。坐标使用烘焙在
// 背景 bg[65] 中的旧布局 (FState 中的注释值 {…})。
var selButtonAreas = []loginArea{
	{selOX + 134, selOY + 454, 76, 33},  // ImgSelSelect1 [66]
	{selOX + 685, selOY + 454, 76, 33},  // ImgSelSelect2 [67]
	{selOX + 385, selOY + 456, 44, 21},  // ImgSelStart [68]
	{selOX + 348, selOY + 486, 120, 21}, // ImgSelNewChr [69]
	{selOX + 347, selOY + 506, 120, 21}, // ImgSelErase [70]
	{selOX + 379, selOY + 547, 56, 20},  // ImgSelExit [72]
}

// selButtons 将每个选择按钮映射到其 Prguse 图片和屏幕坐标。
// 按钮面板烘焙在背景 [65] 中，仅在按下时叠加绘制
// (DscSelect1DirectPaint, FState:2693-2705)。
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
		gl:         gl,
		resources:  resources,
		text:       text,
		Selected:   -1,
		downButton: -1,
		createDown: -1,
	}
}

func (s *SelectChrScene) traceDraw(tag, wil string, idx int, x, y, w, h float32) {
	log.Logf(log.LevelTrace, "Render", "selchr %s %s[%d] pos=(%.0f,%.0f) size=(%.0f,%.0f)", tag, wil, idx, x, y, w, h)
}

func (s *SelectChrScene) logComponentLayout(ox, oy float32) {
	log.Logf(log.LevelInfo, "SelectChrScene", "=== component layout (main) ===")
	bw, bh := s.getPrguseSize(ImgSelBg)
	log.Logf(log.LevelInfo, "SelectChrScene", "  background Prguse[%d]       pos=(%.0f,%.0f) size=(%d,%d)", ImgSelBg, ox, oy, bw, bh)

	btnNames := [6]string{"Select1", "Select2", "Start", "NewChr", "Erase", "Exit"}
	for i, a := range selButtonAreas {
		log.Logf(log.LevelInfo, "SelectChrScene", "  button     %-12s pos=(%.0f,%.0f) size=(%.0f,%.0f) img=Prguse[%d]", btnNames[i], a.X, a.Y, a.W, a.H, selButtons[i].img)
	}

	for i := 0; i < 2; i++ {
		ch := s.Characters[i]
		if !ch.Valid {
			log.Logf(log.LevelInfo, "SelectChrScene", "  character slot %d (empty)", i)
			continue
		}
		px, py := s.slotPos(int(ch.Job), int(ch.Sex), i, ox, oy)
		jn := "未知"
		if int(ch.Job) < len(jobNames) {
			jn = jobNames[ch.Job]
		}
		log.Logf(log.LevelInfo, "SelectChrScene", "  character slot %d %-8s     pos=(%.0f,%.0f) job=%s sex=%d", i, ch.Name, px, py, jn, ch.Sex)
	}

	type textPos struct{ nameX, nameY, levelX, levelY, jobX, jobY float32 }
	textPositions := [2]textPos{
		{136, 476, 136, 513, 136, 548},
		{586, 476, 666, 513, 638, 548},
	}
	for i, tp := range textPositions {
		log.Logf(log.LevelInfo, "SelectChrScene", "  text       slot%d-Name     pos=(%.0f,%.0f)", i, ox+tp.nameX, oy+tp.nameY)
		log.Logf(log.LevelInfo, "SelectChrScene", "  text       slot%d-Level   pos=(%.0f,%.0f)", i, ox+tp.levelX, oy+tp.levelY)
		log.Logf(log.LevelInfo, "SelectChrScene", "  text       slot%d-Job     pos=(%.0f,%.0f)", i, ox+tp.jobX, oy+tp.jobY)
	}
	if s.ServerName != "" && s.text != nil {
		x := float32(ScreenWidth)/2 - float32(s.text.MeasureText(s.ServerName))/2
		log.Logf(log.LevelInfo, "SelectChrScene", "  text       ServerName     pos=(%.0f,%.0f)", x, oy+8)
	}
}

func (s *SelectChrScene) logCreateDialogLayout() {
	winX, winY := s.createWinPos()
	cw, ch := s.getPrguseSize(ImgCreateBg)
	log.Logf(log.LevelInfo, "SelectChrScene", "=== component layout (create character dialog) ===")
	log.Logf(log.LevelInfo, "SelectChrScene", "  window     Prguse[%d]       pos=(%.0f,%.0f) size=(%d,%d)", ImgCreateBg, winX, winY, cw, ch)
	log.Logf(log.LevelInfo, "SelectChrScene", "  input      %-12s pos=(%.0f,%.0f) size=(129,21)", "Name", winX+63, winY+79)

	jobNames3 := [3]string{"Warrior", "Mage", "Taoist"}
	jobXs := [3]float32{36, 103, 168}
	for i := 0; i < 3; i++ {
		jw, jh := s.getPrguseSize(ImgCreateJob1 + i)
		log.Logf(log.LevelInfo, "SelectChrScene", "  job button %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", jobNames3[i], winX+jobXs[i], winY+139, jw, jh, ImgCreateJob1+i)
	}
	sexNames := [2]string{"Male", "Female"}
	sexXs := [2]float32{70, 137}
	sexImgs := [2]int{ImgCreateMale, ImgCreateFemale}
	for i := 0; i < 2; i++ {
		sw, sh := s.getPrguseSize(sexImgs[i])
		log.Logf(log.LevelInfo, "SelectChrScene", "  sex button %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", sexNames[i], winX+sexXs[i], winY+211, sw, sh, sexImgs[i])
	}
	okW, okH := s.getPrguseSize(ImgCreateOk)
	log.Logf(log.LevelInfo, "SelectChrScene", "  button     %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", "OK", winX+46, winY+273, okW, okH, ImgCreateOk)
	cancelW, cancelH := s.getPrguseSize(ImgCreateCancel)
	log.Logf(log.LevelInfo, "SelectChrScene", "  button     %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", "Cancel", winX+138, winY+273, cancelW, cancelH, ImgCreateCancel)
}

func (s *SelectChrScene) logDeleteDialogLayout() {
	winX, winY := s.deleteWinPos()
	dw, dh := s.getPrguseSize(ImgModalNormal)
	log.Logf(log.LevelInfo, "SelectChrScene", "=== component layout (delete confirm dialog) ===")
	log.Logf(log.LevelInfo, "SelectChrScene", "  window     Prguse[%d]      pos=(%.0f,%.0f) size=(%d,%d)", ImgModalNormal, winX, winY, dw, dh)
	log.Logf(log.LevelInfo, "SelectChrScene", "  text       delete message pos=(%.0f,%.0f)", winX+39, winY+38)
	imgs := [3]int{ImgModalYes, ImgModalNo, ImgModalCancel}
	names := [3]string{"Yes", "No", "Cancel"}
	lxs := [3]float32{104, 214, 324}
	for i, img := range imgs {
		bw, bh := s.getPrguseSize(img)
		log.Logf(log.LevelInfo, "SelectChrScene", "  button     %-12s pos=(%.0f,%.0f) size=(%d,%d) img=Prguse[%d]", names[i], winX+lxs[i], winY+126, bw, bh, img)
	}
}

func (s *SelectChrScene) Open() {
	log.Logf(log.LevelInfo, "SelectChrScene", "opened")
	gSound.PlayBGM(bmgSelect)
	s.errorMsg = ""
	s.createMode = false
	s.deleteConfirm = false
	s.compLogged = false
	s.createDlgLogged = false
	s.deleteDlgLogged = false
}

func (s *SelectChrScene) Close() {
	gSound.SilenceSound()
	log.Logf(log.LevelInfo, "SelectChrScene", "closed")
}

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

	// 选中特效以 50ms/帧 推进，前提是有有效角色被选中
	// (IntroScn:1449-1454)。
	if s.Selected >= 0 && s.Selected < 2 && s.Characters[s.Selected].Valid {
		if now.Sub(s.selEffTick) > 50*time.Millisecond {
			s.selEffTick = now
			s.selEffIndex = (s.selEffIndex + 1) % selectEffFrames
		}
	}
}

func (s *SelectChrScene) Render(gl *engine.GLState, proj [16]float32) {
	ox, oy := selOX, selOY

	if s.resources.Prguse != nil {
		tex := s.resources.GetTexture(s.resources.Prguse, ImgSelBg)
		if tex != 0 {
			w, h := s.getPrguseSize(ImgSelBg)
			s.traceDraw("bg", "Prguse", ImgSelBg, ox, oy, float32(w), float32(h))
			gl.DrawQuad(tex, ox, oy, float32(w), float32(h), proj)
		}
	} else {
		gl.DrawQuadColor(0, 0, ScreenWidth, ScreenHeight, 0.1, 0.15, 0.1, 1.0, proj)
	}

	for i := 0; i < 2; i++ {
		s.renderCharSlot(gl, proj, ox, oy, i)
	}

	s.renderButtons(gl, proj, ox, oy)
	s.renderText(gl, proj, ox, oy)

	if !s.compLogged {
		s.logComponentLayout(ox, oy)
		s.compLogged = true
	}

	if s.createMode {
		s.renderCreateDialog(gl, proj)
		if !s.createDlgLogged {
			s.logCreateDialogLayout()
			s.createDlgLogged = true
		}
	} else {
		s.createDlgLogged = false
	}
	if s.deleteConfirm {
		s.renderDeleteDialog(gl, proj)
		if !s.deleteDlgLogged {
			s.logDeleteDialogLayout()
			s.deleteDlgLogged = true
		}
	} else {
		s.deleteDlgLogged = false
	}
}

// slotPos 返回指定职业/性别在槽位 idx 中冻结帧的左上角坐标 (bx,by)。
// 对应 IntroScn.pas:1390-1438；槽位 1 偏移 (+340,+2)。
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

// standingPos 返回站立帧的左上角坐标 (fx,fy)。女法师和女道士的
// 站立帧相对冻结帧有偏移 (IntroScn:1413-1414 fx=bx-30,fy=by-14;
// IntroScn:1426-1427 fx=bx-23,fy=by-20)。
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

func (s *SelectChrScene) renderCharSlot(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
	if s.createMode && idx == s.createIndex {
		s.renderCreatePreview(gl, proj, ox, oy, idx)
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

	// 确定帧索引和绘制坐标。站立状态使用 (fx,fy)，
	// 女法师/女道士的站立坐标与冻结坐标 (bx,by) 不同
	// (IntroScn:1480-1494 vs 1497-1501)。
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
				s.traceDraw("portrait", "ChrSel", imgIdx, drawX, drawY, sprW, sprH)
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

	// 亮度渐变：DarkLevel 递减期间，立绘上的黑色遮罩逐渐消失
	// (IntroScn:1485-1494)。
	if standing && idx == s.Selected && ch.DarkLevel > 0 {
		alpha := float32(ch.DarkLevel) / darkLevelMax
		log.Logf(log.LevelTrace, "Render", "selchr dark overlay slot=%d alpha=%.3f", idx, alpha)
		gl.DrawQuadColor(drawX, drawY, sprW, sprH, 0, 0, 0, alpha, proj)
	}

	// 选中光效仅在解冻动画期间播放
	// (IntroScn:1439-1459, :1444 处的 DrawBlend 位于 Unfreezing 分支内)。
	if ch.Unfreezing {
		s.renderSelectEffect(gl, proj, ox, oy, idx)
	}
}

// renderCreatePreview 为空槽位绘制实时创建预览，
// 由 createJob/createSex 驱动，选择职业或性别后立即更新预览
// (Delphi MakeNewChar → SelectChr(NewIndex), IntroScn:1286,1339-1353)。
func (s *SelectChrScene) renderCreatePreview(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
	job, sex := s.createJob, s.createSex
	if job > 2 {
		job = 0
	}
	if sex > 1 {
		sex = 0
	}
	slotX, slotY := s.slotPos(job, sex, idx, ox, oy)
	imgIdx := 60 + job*40 + sex*120 // 静态站立姿势
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
	s.traceDraw("portrait", "ChrSel", imgIdx, slotX, slotY, float32(img.Width), float32(img.Height))
	gl.DrawQuad(tex, slotX, slotY, float32(img.Width), float32(img.Height), proj)
}

// renderSelectEffect 在选中槽位的固定特效原点绘制 14 帧选中光效
// (ChrSel[4..17]) (IntroScn:1388-1389,1442-1454)。
func (s *SelectChrScene) renderSelectEffect(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
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
	s.traceDraw("effect", "ChrSel", imgIdx, ox+ex, oy+ey, float32(img.Width), float32(img.Height))
	gl.DrawQuadAdditive(tex, ox+ex, oy+ey, float32(img.Width), float32(img.Height), proj)
}

func (s *SelectChrScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.resources.Prguse == nil {
		return
	}
	// 六个按钮烘焙在背景 [65] 中；仅在按下时叠加绘制对应按钮
	// (DscSelect1DirectPaint 只在 Downed 时绘制 FaceIndex,
	// FState:2693-2705)。
	if s.downButton < 0 || s.downButton >= len(selButtons) {
		return
	}
	btn := selButtons[s.downButton]
	bw, bh := s.getPrguseSize(btn.img)
	s.traceDraw("btn", "Prguse", btn.img, btn.x, btn.y, float32(bw), float32(bh))
	s.drawPrguseImage(btn.img, btn.x, btn.y, proj)
}

func (s *SelectChrScene) renderText(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}

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

		// 名字/等级/职业为白色带黑色描边 (BoldTextOut,
		// IntroScn:1522-1534)；等级为纯数字 (IntToStr, :1523)。
		log.Logf(log.LevelTrace, "Render", "selchr info text '%s' pos=(%.0f,%.0f)", ch.Name, ox+tp.nameX, oy+tp.nameY)
		s.text.DrawTextOutline(ch.Name, ox+tp.nameX, oy+tp.nameY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		levelStr := fmt.Sprintf("%d", ch.Level)
		log.Logf(log.LevelTrace, "Render", "selchr info text '%s' pos=(%.0f,%.0f)", levelStr, ox+tp.levelX, oy+tp.levelY)
		s.text.DrawTextOutline(levelStr, ox+tp.levelX, oy+tp.levelY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		jobName := "未知"
		if int(ch.Job) < len(jobNames) {
			jobName = jobNames[ch.Job]
		}
		log.Logf(log.LevelTrace, "Render", "selchr info text '%s' pos=(%.0f,%.0f)", jobName, ox+tp.jobX, oy+tp.jobY)
		s.text.DrawTextOutline(jobName, ox+tp.jobX, oy+tp.jobY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	// 服务器名居中显示在顶部 (IntroScn:1539-1545)。
	if s.ServerName != "" {
		x := float32(ScreenWidth)/2 - float32(s.text.MeasureText(s.ServerName))/2
		log.Logf(log.LevelTrace, "Render", "selchr info text '%s' pos=(%.0f,%.0f)", s.ServerName, x, oy+8)
		s.text.DrawTextOutline(s.ServerName, x, oy+8, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+250, oy+400, 1.0, 0.3, 0.3, 1.0, proj)
	}
}

// createWinPos 根据当前 createIndex 返回创建窗口的左上角坐标
// (IntroScn:1272-1278)：槽位 0 → (469,63)，槽位 1 → (87,63)。
func (s *SelectChrScene) createWinPos() (float32, float32) {
	if s.createIndex == 1 {
		return 87, 63
	}
	return 469, 63
}

// imgArea 以 (x,y) 为左上角、Prguse 图片尺寸为大小构建碰撞矩形，
// 资源不可用时使用默认值 (Delphi 通过 SetImgIndex 确定控件尺寸)。
func (s *SelectChrScene) imgArea(img int, x, y float32) loginArea {
	w, h := s.getPrguseSize(img)
	if w == 0 || h == 0 {
		w, h = 80, 28
	}
	return loginArea{x, y, float32(w), float32(h)}
}

func (s *SelectChrScene) renderCreateDialog(gl *engine.GLState, proj [16]float32) {
	winX, winY := s.createWinPos()

	// 创建窗口背景 [73]；标签和按钮面板都烘焙在其中，
	// 因此没有全屏遮罩也没有单独的标签文本 (DCreateChr)。
	cw, ch := s.getPrguseSize(ImgCreateBg)
	s.traceDraw("create-bg", "Prguse", ImgCreateBg, winX, winY, float32(cw), float32(ch))
	if !s.drawPrguseImage(ImgCreateBg, winX, winY, proj) {
		gl.DrawQuadColor(winX, winY, 260, 320, 0.12, 0.12, 0.2, 0.95, proj)
	}

	// 名字输入框：黑底白字，窗口+(63,79)，129×21，
	// MaxLength 14 (IntroScn:1109-1121,1282-1283)。
	log.Logf(log.LevelTrace, "Render", "selchr name input pos=(%.0f,%.0f) size=(%.0f,%.0f)", winX+63, winY+79, float32(129), float32(21))
	gl.DrawQuadColor(winX+63, winY+79, 129, 21, 0, 0, 0, 1, proj)
	if s.text != nil {
		s.text.DrawText(s.createName, winX+65, winY+81, 1, 1, 1, 1, proj)
		showCursor := time.Since(s.cursorBlink) < 250*time.Millisecond
		if time.Since(s.cursorBlink) > 500*time.Millisecond {
			s.cursorBlink = time.Now()
			showCursor = true
		}
		if showCursor {
			cx := winX + 65 + float32(s.text.MeasureText(s.createName))
			s.text.DrawText("|", cx, winY+81, 1, 1, 1, 1, proj)
		}
	}

	// 职业按钮 [74/75/76] (窗口相对坐标 36/103/168,139) 和性别按钮
	// [77/78] (70/137,211)：选中时高亮，按下时显示自身面板，
	// 其余状态不绘制 (DccCloseDirectPaint, FState:2725-2761)。
	jobXs := [3]float32{36, 103, 168}
	for i := 0; i < 3; i++ {
		s.renderCreateChoice(gl, proj, ImgCreateJob1+i, ImgClassHi1+i,
			winX+jobXs[i], winY+139, s.createJob == i, s.createDown == i)
	}
	sexXs := [2]float32{70, 137}
	sexHi := [2]int{ImgGenderHiM, ImgGenderHiF}
	for i := 0; i < 2; i++ {
		s.renderCreateChoice(gl, proj, ImgCreateMale+i, sexHi[i],
			winX+sexXs[i], winY+211, s.createSex == i, s.createDown == 3+i)
	}

	// 确定 [51] / 取消 [52]：仅在按下 (Downed) 时绘制，
	// 与 DccCloseDirectPaint 一致——常态面板烘焙在 Prguse[73] 中。
	if s.createDown == 5 {
		okW, okH := s.getPrguseSize(ImgCreateOk)
		s.traceDraw("create-ok", "Prguse", ImgCreateOk, winX+46, winY+273, float32(okW), float32(okH))
		s.drawButton(ImgCreateOk, winX+46, winY+273, proj)
	}
	if s.createDown == 6 {
		cancelW, cancelH := s.getPrguseSize(ImgCreateCancel)
		s.traceDraw("create-cancel", "Prguse", ImgCreateCancel, winX+138, winY+273, float32(cancelW), float32(cancelH))
		s.drawButton(ImgCreateCancel, winX+138, winY+273, proj)
	}
}

// renderCreateChoice 对应 DccCloseDirectPaint (FState:2725-2761)：按下时
// 绘制按钮自身面板；未按下但当前选中时绘制高亮图片；否则不绘制
// （已烘焙在创建窗口中）。
func (s *SelectChrScene) renderCreateChoice(gl *engine.GLState, proj [16]float32, faceIdx, hiIdx int, x, y float32, selected, downed bool) {
	switch {
	case downed:
		w, h := s.getPrguseSize(faceIdx)
		s.traceDraw("create-btn", "Prguse", faceIdx, x, y, float32(w), float32(h))
		s.drawPrguseImage(faceIdx, x, y, proj)
	case selected:
		w, h := s.getPrguseSize(hiIdx)
		s.traceDraw("highlight", "Prguse", hiIdx, x, y, float32(w), float32(h))
		s.drawPrguseImage(hiIdx, x, y, proj)
	}
}

// drawButton 绘制按钮图片，资源缺失时回退为纯色矩形。
func (s *SelectChrScene) drawButton(img int, x, y float32, proj [16]float32) {
	if s.drawPrguseImage(img, x, y, proj) {
		return
	}
	w, h := s.getPrguseSize(img)
	if w == 0 || h == 0 {
		w, h = 80, 28
	}
	s.gl.DrawQuadColor(x, y, float32(w), float32(h), 0.25, 0.3, 0.35, 0.9, proj)
}

// deleteWinPos 将删除确认模态框 ([360]) 居中于屏幕。
func (s *SelectChrScene) deleteWinPos() (float32, float32) {
	w, h := s.getPrguseSize(ImgModalNormal)
	if w == 0 || h == 0 {
		w, h = 380, 180
	}
	return float32(ScreenWidth-w) / 2, float32(ScreenHeight-h) / 2
}

// deleteButtonAreas 返回 是/否/取消 按钮的碰撞矩形（屏幕坐标），
// 从窗口相对 lx=324 处开始从右向左排列，间距 110px
// (FState:2060-2083; 与 uidialog.go 的 dialogButtonLayout/dialogButtonOrder 一致)。
func (s *SelectChrScene) deleteButtonAreas() [3]loginArea {
	winX, winY := s.deleteWinPos()
	imgs := [3]int{ImgModalYes, ImgModalNo, ImgModalCancel}
	lxs := [3]float32{104, 214, 324} // 是、否、取消（从右到左）
	var areas [3]loginArea
	for i, img := range imgs {
		w, h := s.getPrguseSize(img)
		if w == 0 || h == 0 {
			w, h = 96, 34
		}
		areas[i] = loginArea{winX + lxs[i], winY + 126, float32(w), float32(h)}
	}
	return areas
}

func (s *SelectChrScene) renderDeleteDialog(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 0.5, proj)
	winX, winY := s.deleteWinPos()
	dw, dh := s.getPrguseSize(ImgModalNormal)
	s.traceDraw("del-bg", "Prguse", ImgModalNormal, winX, winY, float32(dw), float32(dh))
	if !s.drawPrguseImage(ImgModalNormal, winX, winY, proj) {
		gl.DrawQuadColor(winX, winY, 380, 180, 0.12, 0.12, 0.2, 0.95, proj)
	}
	if s.text != nil {
		msg := fmt.Sprintf("确定删除角色 \"%s\"？", s.deleteName)
		s.text.DrawTextOutline(msg, winX+39, winY+38, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}
	areas := s.deleteButtonAreas()
	imgs := [3]int{ImgModalYes, ImgModalNo, ImgModalCancel}
	for i, img := range imgs {
		bw, bh := s.getPrguseSize(img)
		s.traceDraw("del-btn", "Prguse", img, areas[i].X, areas[i].Y, float32(bw), float32(bh))
		s.drawButton(img, areas[i].X, areas[i].Y, proj)
	}
}

func (s *SelectChrScene) OnChar(char rune) {
	if !s.createMode {
		return
	}
	if char < 32 || char == 127 {
		return
	}
	if len([]rune(s.createName)) >= 14 {
		return
	}
	s.createName += string(char)
	s.cursorBlink = time.Now()
}

func (s *SelectChrScene) OnKey(key int, action int) {
	if action != 1 {
		return
	}

	if s.createMode {
		switch key {
		case keyBackspace:
			if len(s.createName) > 0 {
				runes := []rune(s.createName)
				s.createName = string(runes[:len(runes)-1])
			}
			s.cursorBlink = time.Now()
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

	switch key {
	case keyEnter, keyKPEnter:
		s.startGame()
	}
}

func (s *SelectChrScene) OnMouse(x, y float64, button int, action int, mods int) {
	log.Logf(log.LevelDebug, "Mouse", "selchr pos=(%.0f,%.0f) button=%d action=%d", x, y, button, action)
	fx, fy := float32(x), float32(y)
	switch {
	case s.createMode:
		s.mouseCreate(fx, fy, action)
	case s.deleteConfirm:
		s.mouseDelete(fx, fy, action)
	default:
		s.mouseSelect(fx, fy, action)
	}
}

// mouseSelect 在同一按钮内松开时触发 (TDButton.MouseUp)；
// 记录按下按钮以便 renderButtons 在按住期间绘制按下态。
func (s *SelectChrScene) mouseSelect(fx, fy float32, action int) {
	switch action {
	case mousePress:
		s.downButton = -1
		btnNames := [6]string{"Select1", "Select2", "Start", "NewChr", "Erase", "Exit"}
		for i, area := range selButtonAreas {
			if hitTest(fx, fy, area) {
				s.downButton = i
				log.Logf(log.LevelInfo, "SelectChrScene", "click button %s pos=(%.0f,%.0f)", btnNames[i], area.X, area.Y)
				return
			}
		}
	case mouseRelease:
		down := s.downButton
		s.downButton = -1
		if down >= 0 && down < len(selButtonAreas) && hitTest(fx, fy, selButtonAreas[down]) {
			s.handleButton(down)
		}
	}
}

func (s *SelectChrScene) mouseCreate(fx, fy float32, action int) {
	winX, winY := s.createWinPos()
	jobXs := [3]float32{36, 103, 168}
	sexXs := [2]float32{70, 137}
	switch action {
	case mousePress:
		s.createDown = -1
		jobNames3 := [3]string{"Warrior", "Mage", "Taoist"}
		for i := 0; i < 3; i++ {
			if hitTest(fx, fy, s.imgArea(ImgCreateJob1+i, winX+jobXs[i], winY+139)) {
				s.createDown = i
				log.Logf(log.LevelInfo, "SelectChrScene", "click job button %s pos=(%.0f,%.0f)", jobNames3[i], winX+jobXs[i], winY+139)
				return
			}
		}
		sexNames := [2]string{"Male", "Female"}
		for i := 0; i < 2; i++ {
			if hitTest(fx, fy, s.imgArea(ImgCreateMale+i, winX+sexXs[i], winY+211)) {
				s.createDown = 3 + i
				log.Logf(log.LevelInfo, "SelectChrScene", "click sex button %s pos=(%.0f,%.0f)", sexNames[i], winX+sexXs[i], winY+211)
				return
			}
		}
		if hitTest(fx, fy, s.imgArea(ImgCreateOk, winX+46, winY+273)) {
			s.createDown = 5
			return
		}
		if hitTest(fx, fy, s.imgArea(ImgCreateCancel, winX+142, winY+273)) {
			s.createDown = 6
			return
		}
	case mouseRelease:
		down := s.createDown
		s.createDown = -1
		switch {
		case down >= 0 && down < 3 && hitTest(fx, fy, s.imgArea(ImgCreateJob1+down, winX+jobXs[down], winY+139)):
			s.createJob = down
		case down >= 3 && down < 5 && hitTest(fx, fy, s.imgArea(ImgCreateMale+down-3, winX+sexXs[down-3], winY+211)):
			s.createSex = down - 3
		case down == 5 && hitTest(fx, fy, s.imgArea(ImgCreateOk, winX+46, winY+273)):
			log.Logf(log.LevelInfo, "SelectChrScene", "click button OK pos=(%.0f,%.0f)", winX+46, winY+273)
			s.confirmCreate()
		case down == 6 && hitTest(fx, fy, s.imgArea(ImgCreateCancel, winX+142, winY+273)):
			log.Logf(log.LevelInfo, "SelectChrScene", "click button Cancel pos=(%.0f,%.0f)", winX+142, winY+273)
			s.createMode = false
		}
	}
}

func (s *SelectChrScene) mouseDelete(fx, fy float32, action int) {
	if action != mouseRelease {
		return
	}
	areas := s.deleteButtonAreas()
	names := [3]string{"Yes", "No", "Cancel"}
	for i, a := range areas {
		if hitTest(fx, fy, a) {
			log.Logf(log.LevelInfo, "SelectChrScene", "click button %s pos=(%.0f,%.0f)", names[i], a.X, a.Y)
			break
		}
	}
	switch {
	case hitTest(fx, fy, areas[0]): // 是
		s.confirmDelete()
	case hitTest(fx, fy, areas[1]), hitTest(fx, fy, areas[2]): // 否 / 取消
		s.deleteConfirm = false
	}
}

func (s *SelectChrScene) OnScroll(x, y float64) {
}

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
		// Delphi SelChrSelect1Click/2Click 直接设置 DarkLevel=0 (IntroScn:1165,1182)：
		// 手动点击没有亮度渐变。
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
	s.createDown = -1
	s.cursorBlink = time.Now()
	s.errorMsg = ""
	log.Logf(log.LevelInfo, "SelectChr", "enter create character mode, slot=%d", emptyIdx)
}

func (s *SelectChrScene) confirmCreate() {
	name := s.createName
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

func (s *SelectChrScene) SetStartFunc(fn func(charName string)) {
	s.startFunc = fn
}

func (s *SelectChrScene) SetExitFunc(fn func()) {
	s.exitFunc = fn
}

func (s *SelectChrScene) SetNewChrFunc(fn func(name string, hair, job, sex int)) {
	s.newChrFunc = fn
}

func (s *SelectChrScene) SetDelChrFunc(fn func(name string)) {
	s.delChrFunc = fn
}

func (s *SelectChrScene) SetError(msg string) {
	s.errorMsg = msg
}

// SetServerName 设置绘制在场景顶部居中的服务器名
// (IntroScn:1539-1545)。待网络流程提供服务器名后与 GameState.ServerName 同步。
func (s *SelectChrScene) SetServerName(name string) {
	s.ServerName = name
}

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

	// Delphi (ClMain:5100-5112) 对预选角色直接设置 FreezeState=FALSE——
	// 无解冻动画，无亮度渐变。
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

func (s *SelectChrScene) drawPrguseImage(index int, x, y float32, proj [16]float32) bool {
	if s.resources.Prguse == nil || index >= s.resources.Prguse.Count {
		return false
	}
	img := s.resources.Prguse.GetImage(index)
	if img == nil || img.RGBA == nil {
		return false
	}
	tex := s.resources.GetTexture(s.resources.Prguse, index)
	if tex == 0 {
		return false
	}
	s.gl.DrawQuad(tex, x, y, float32(img.Width), float32(img.Height), proj)
	return true
}
