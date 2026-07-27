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
	selectEffFrames = 14 // ChrSel[4..17] selection-effect loop (IntroScn:1442-1454)
)

// CharacterSlot represents a character in the selection screen.
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
}

// jobNames maps job ID to display name.
var jobNames = []string{"战士", "法师", "道士"}

// SelectChrScene handles the character selection screen (stSelectChr).
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
	createDown  int // create-dialog button held down (-1 none; 0-2 class, 3-4 gender)
	cursorBlink time.Time

	deleteConfirm bool
	deleteName    string

	downButton  int // select button held down (-1 none), for pressed-only blit
	selEffIndex int
	selEffTick  time.Time

	// ServerName is drawn centered at the top of the screen (IntroScn:1539-1545).
	// Mirrors GameState.ServerName; currently never assigned because the server
	// name is not carried by SMSelectServerOK (body is addr/port/cert only) and
	// the network flow is intentionally left untouched. Drawn only when non-empty.
	ServerName string

	startFunc  func(charName string)
	exitFunc   func()
	newChrFunc func(name string, hair, job, sex int)
	delChrFunc func(name string)
}

const (
	selOX = float32(0)
	selOY = float32(0)
)

// selButtonAreas are the select-screen button hit rectangles; sizes match the
// image extents Delphi derives via SetImgIndex (FState:904-925). Order parallels
// selButtons.
var selButtonAreas = []loginArea{
	{selOX + 134, selOY + 424, 76, 33},  // ImgSelSelect1 [66]
	{selOX + 602, selOY + 424, 76, 33},  // ImgSelSelect2 [67]
	{selOX + 374, selOY + 427, 44, 21},  // ImgSelStart [68]
	{selOX + 349, selOY + 467, 120, 21}, // ImgSelNewChr [69]
	{selOX + 349, selOY + 505, 120, 21}, // ImgSelErase [70]
	{selOX + 349, selOY + 543, 56, 20},  // ImgSelExit [72]
}

// selButtons maps each select button to its Prguse image and screen position
// (FState:904-925). The faces are baked into the background [65] and are blit
// only while pressed (DscSelect1DirectPaint, FState:2693-2705).
var selButtons = []struct {
	img  int
	x, y float32
}{
	{ImgSelSelect1, selOX + 134, selOY + 424},
	{ImgSelSelect2, selOX + 602, selOY + 424},
	{ImgSelStart, selOX + 374, selOY + 427},
	{ImgSelNewChr, selOX + 349, selOY + 467},
	{ImgSelErase, selOX + 349, selOY + 505},
	{ImgSelExit, selOX + 349, selOY + 543},
}

// NewSelectChrScene creates a new character selection scene.
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

func (s *SelectChrScene) Open() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Opened")
	s.errorMsg = ""
	s.createMode = false
	s.deleteConfirm = false
}

func (s *SelectChrScene) Close() {
	log.Logf(log.LevelInfo, "SelectChrScene", "Closed")
}

func (s *SelectChrScene) Update(dt float64) {
	now := time.Now()
	for i := 0; i < 2; i++ {
		ch := &s.Characters[i]
		if !ch.Valid {
			continue
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

	// Selection effect advances at 50ms/frame while a valid slot is selected
	// (IntroScn:1449-1454).
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

	if s.createMode {
		s.renderCreateDialog(gl, proj)
	}
	if s.deleteConfirm {
		s.renderDeleteDialog(gl, proj)
	}
}

// slotPos returns the portrait top-left for a job/sex in slot idx. The table
// matches IntroScn.pas:1390-1438 (bx,by); slot 1 is offset (+340,+2).
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
	slotX, slotY := s.slotPos(job, sex, idx, ox, oy)

	var imgIdx int
	if ch.FreezeState || ch.Freezing {
		frame := 0
		if ch.Freezing {
			frame = freezeFrame - 1 - ch.AniIndex
		}
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120 + frame
	} else if ch.Unfreezing {
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120 + ch.AniIndex
	} else if idx == s.Selected {
		imgIdx = 40 + int(ch.Job)*40 + int(ch.Sex)*120 + ch.AniIndex%selectedFrame
	} else {
		imgIdx = 60 + int(ch.Job)*40 + int(ch.Sex)*120
	}

	drawn := false
	if s.resources.ChrSel != nil && imgIdx >= 0 && imgIdx < s.resources.ChrSel.Count {
		img := s.resources.ChrSel.GetImage(imgIdx)
		if img != nil && img.Width > 0 && img.Height > 0 {
			tex := s.resources.GetTexture(s.resources.ChrSel, imgIdx)
			if tex != 0 {
				// Delphi draws the portrait top-left at the slot origin
				// (IntroScn:1443,1469,1494,1501).
				gl.DrawQuad(tex, slotX, slotY, float32(img.Width), float32(img.Height), proj)
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
		gl.DrawQuadColor(slotX, slotY, 100, 200, r, g, b, 0.8, proj)
	}

	if idx == s.Selected {
		s.renderSelectEffect(gl, proj, ox, oy, idx)
	}
}

// renderCreatePreview draws the live creation portrait for the empty slot,
// driven by createJob/createSex so the preview updates the instant a class or
// gender is chosen (Delphi MakeNewChar → SelectChr(NewIndex), IntroScn:1286,1339-1353).
func (s *SelectChrScene) renderCreatePreview(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
	job, sex := s.createJob, s.createSex
	if job > 2 {
		job = 0
	}
	if sex > 1 {
		sex = 0
	}
	slotX, slotY := s.slotPos(job, sex, idx, ox, oy)
	imgIdx := 60 + job*40 + sex*120 // static standing pose
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
	gl.DrawQuad(tex, slotX, slotY, float32(img.Width), float32(img.Height), proj)
}

// renderSelectEffect draws the 14-frame selection glow (ChrSel[4..17]) at the
// fixed effect origin of the selected slot (IntroScn:1388-1389,1442-1454).
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
	// Delphi blends additively (DrawBlend) and fades via DarkLevel; both are
	// approximated/omitted here (normal alpha, no fade — low priority).
	gl.DrawQuad(tex, ox+ex, oy+ey, float32(img.Width), float32(img.Height), proj)
}

func (s *SelectChrScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.resources.Prguse == nil {
		return
	}
	// The six buttons are baked into the background [65]; only the pressed one
	// is blit on top (DscSelect1DirectPaint draws FaceIndex only when Downed,
	// FState:2693-2705).
	if s.downButton < 0 || s.downButton >= len(selButtons) {
		return
	}
	btn := selButtons[s.downButton]
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

		// Name/level/class are white with a black outline (BoldTextOut,
		// IntroScn:1522-1534); the level is a bare number (IntToStr, :1523).
		s.text.DrawTextOutline(ch.Name, ox+tp.nameX, oy+tp.nameY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		s.text.DrawTextOutline(fmt.Sprintf("%d", ch.Level), ox+tp.levelX, oy+tp.levelY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
		jobName := "未知"
		if int(ch.Job) < len(jobNames) {
			jobName = jobNames[ch.Job]
		}
		s.text.DrawTextOutline(jobName, ox+tp.jobX, oy+tp.jobY, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	// Server name centered at the top (IntroScn:1539-1545).
	if s.ServerName != "" {
		x := float32(ScreenWidth)/2 - float32(s.text.MeasureText(s.ServerName))/2
		s.text.DrawTextOutline(s.ServerName, x, oy+8, 1, 1, 1, 1, 0, 0, 0, 1, proj)
	}

	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+250, oy+400, 1.0, 0.3, 0.3, 1.0, proj)
	}
}

// createWinPos returns the creation-window origin for the current createIndex
// (IntroScn:1272-1278): slot 0 → (469,63), slot 1 → (87,63).
func (s *SelectChrScene) createWinPos() (float32, float32) {
	if s.createIndex == 1 {
		return 87, 63
	}
	return 469, 63
}

// imgArea builds a hit rectangle at (x,y) sized from the Prguse image, with a
// fallback when the asset is unavailable (Delphi sizes controls from SetImgIndex).
func (s *SelectChrScene) imgArea(img int, x, y float32) loginArea {
	w, h := s.getPrguseSize(img)
	if w == 0 || h == 0 {
		w, h = 60, 22
	}
	return loginArea{x, y, float32(w), float32(h)}
}

func (s *SelectChrScene) renderCreateDialog(gl *engine.GLState, proj [16]float32) {
	winX, winY := s.createWinPos()

	// Creation window background [73]; labels and button faces are baked into
	// it, so there is no fullscreen dim and no separate label text (DCreateChr).
	if !s.drawPrguseImage(ImgCreateBg, winX, winY, proj) {
		gl.DrawQuadColor(winX, winY, 260, 320, 0.12, 0.12, 0.2, 0.95, proj)
	}

	// Name edit field: black background, white text, window+(63,79) 129×21,
	// MaxLength 14 (IntroScn:1109-1121,1282-1283).
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

	// Class buttons [74/75/76] (window-relative 36/103/168,139) and gender
	// buttons [77/78] (70/137,211): highlight when selected, own face when
	// pressed, otherwise nothing (DccCloseDirectPaint, FState:2725-2761).
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

	// OK [51] / Cancel [52] are real buttons (not baked in), always drawn
	// (window-relative 46/138,273 — FState:962-965).
	s.drawButton(ImgCreateOk, winX+46, winY+273, proj)
	s.drawButton(ImgCreateCancel, winX+138, winY+273, proj)
}

// renderCreateChoice mirrors DccCloseDirectPaint (FState:2725-2761): pressed →
// draw the button's own face; not pressed but currently selected → draw the
// highlight image; otherwise draw nothing (baked into the creation window).
func (s *SelectChrScene) renderCreateChoice(gl *engine.GLState, proj [16]float32, faceIdx, hiIdx int, x, y float32, selected, downed bool) {
	switch {
	case downed:
		s.drawPrguseImage(faceIdx, x, y, proj)
	case selected:
		s.drawPrguseImage(hiIdx, x, y, proj)
	}
}

// drawButton blits a button image, falling back to a colored quad when the
// asset is missing.
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

// deleteWinPos centers the delete-confirm modal ([360]) on screen.
func (s *SelectChrScene) deleteWinPos() (float32, float32) {
	w, h := s.getPrguseSize(ImgModalNormal)
	if w == 0 || h == 0 {
		w, h = 380, 180
	}
	return float32(ScreenWidth-w) / 2, float32(ScreenHeight-h) / 2
}

// deleteButtonAreas returns the Yes/No/Cancel hit rectangles (screen coords),
// laid out right-to-left from window-relative lx=324 with a 110px step
// (FState:2060-2083; mirrors uidialog.go dialogButtonLayout/dialogButtonOrder).
func (s *SelectChrScene) deleteButtonAreas() [3]loginArea {
	winX, winY := s.deleteWinPos()
	imgs := [3]int{ImgModalYes, ImgModalNo, ImgModalCancel}
	lxs := [3]float32{104, 214, 324} // Yes, No, Cancel (right-to-left)
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

func (s *SelectChrScene) OnMouse(x, y float64, button int, action int) {
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

// mouseSelect fires on release inside the same button (TDButton.MouseUp); the
// pressed button is tracked so renderButtons can blit it while held.
func (s *SelectChrScene) mouseSelect(fx, fy float32, action int) {
	switch action {
	case mousePress:
		s.downButton = -1
		for i, area := range selButtonAreas {
			if hitTest(fx, fy, area) {
				s.downButton = i
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
		for i := 0; i < 3; i++ {
			if hitTest(fx, fy, s.imgArea(ImgCreateJob1+i, winX+jobXs[i], winY+139)) {
				s.createDown = i
				return
			}
		}
		for i := 0; i < 2; i++ {
			if hitTest(fx, fy, s.imgArea(ImgCreateMale+i, winX+sexXs[i], winY+211)) {
				s.createDown = 3 + i
				return
			}
		}
	case mouseRelease:
		down := s.createDown
		s.createDown = -1
		switch {
		case down >= 0 && down < 3 && hitTest(fx, fy, s.imgArea(ImgCreateJob1+down, winX+jobXs[down], winY+139)):
			s.createJob = down // instant preview (renderCreatePreview reads this)
		case down >= 3 && down < 5 && hitTest(fx, fy, s.imgArea(ImgCreateMale+down-3, winX+sexXs[down-3], winY+211)):
			s.createSex = down - 3
		case hitTest(fx, fy, s.imgArea(ImgCreateOk, winX+46, winY+273)):
			s.confirmCreate()
		case hitTest(fx, fy, s.imgArea(ImgCreateCancel, winX+138, winY+273)):
			s.createMode = false
		}
	}
}

func (s *SelectChrScene) mouseDelete(fx, fy float32, action int) {
	if action != mouseRelease {
		return
	}
	areas := s.deleteButtonAreas()
	switch {
	case hitTest(fx, fy, areas[0]): // Yes
		s.confirmDelete()
	case hitTest(fx, fy, areas[1]), hitTest(fx, fy, areas[2]): // No / Cancel
		s.deleteConfirm = false
	}
}

func (s *SelectChrScene) OnScroll(x, y float64) {
}

func (s *SelectChrScene) handleButton(index int) {
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
	log.Logf(log.LevelInfo, "SelectChr", "Selected character %d: %s", idx, s.Characters[idx].Name)

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
	log.Logf(log.LevelInfo, "SelectChr", "Create character mode, slot=%d", emptyIdx)
}

func (s *SelectChrScene) confirmCreate() {
	name := s.createName
	if len([]rune(name)) == 0 {
		s.errorMsg = "请输入角色名"
		return
	}
	hair := 1 + rand.Intn(5)
	log.Logf(log.LevelInfo, "SelectChr", "Creating character: %s job=%d sex=%d hair=%d", name, s.createJob, s.createSex, hair)
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
	log.Logf(log.LevelInfo, "SelectChr", "Delete confirm: %s", s.deleteName)
}

func (s *SelectChrScene) confirmDelete() {
	log.Logf(log.LevelInfo, "SelectChr", "Deleting character: %s", s.deleteName)
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
	log.Logf(log.LevelInfo, "SelectChrScene", "Starting game with character: %s", charName)
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

// SetServerName sets the server name drawn centered at the top of the scene
// (IntroScn:1539-1545). Intended to mirror GameState.ServerName once the
// network flow exposes it.
func (s *SelectChrScene) SetServerName(name string) {
	s.ServerName = name
}

func (s *SelectChrScene) SetCharactersFromServer(chars []parsedChar, selectedIdx int) {
	log.Logf(log.LevelInfo, "SelectChrScene", "SetCharactersFromServer: %d chars, selectedIdx=%d", len(chars), selectedIdx)
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
		log.Logf(log.LevelInfo, "SelectChrScene", "  Character %d: %s Lv%d Job=%d Sex=%d",
			i, c.Name, c.Level, c.Job, c.Sex)
	}

	if selectedIdx >= 0 && selectedIdx < 2 && s.Characters[selectedIdx].Valid {
		s.Selected = selectedIdx
		s.Characters[selectedIdx].Unfreezing = true
	} else if s.Characters[0].Valid {
		s.Selected = 0
		s.Characters[0].Unfreezing = true
	}
	log.Logf(log.LevelInfo, "SelectChrScene", "Final selected=%d", s.Selected)
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
