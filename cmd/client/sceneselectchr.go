package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
)

const (
	selectedFrame = 16
	freezeFrame   = 13
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
	cursorBlink time.Time

	deleteConfirm bool
	deleteName    string

	startFunc  func(charName string)
	exitFunc   func()
	newChrFunc func(name string, hair, job, sex int)
	delChrFunc func(name string)
}

const (
	selOX = float32(112)
	selOY = float32(84)
)

var selButtonAreas = []loginArea{
	{selOX + 134, selOY + 424, 70, 20},
	{selOX + 602, selOY + 424, 70, 20},
	{selOX + 374, selOY + 427, 70, 20},
	{selOX + 349, selOY + 467, 70, 20},
	{selOX + 349, selOY + 505, 70, 20},
	{selOX + 349, selOY + 543, 70, 20},
}

var createJobAreas = []loginArea{
	{440, 305, 60, 22},
	{510, 305, 60, 22},
	{580, 305, 60, 22},
}

var createSexAreas = []loginArea{
	{440, 345, 60, 22},
	{510, 345, 60, 22},
}

var (
	createConfirmArea = loginArea{420, 400, 80, 28}
	createCancelArea  = loginArea{530, 400, 80, 28}
	deleteYesArea     = loginArea{420, 370, 80, 28}
	deleteNoArea      = loginArea{530, 370, 80, 28}
)

// NewSelectChrScene creates a new character selection scene.
func NewSelectChrScene(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *SelectChrScene {
	return &SelectChrScene{
		gl:        gl,
		resources: resources,
		text:      text,
		Selected:  -1,
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
}

func (s *SelectChrScene) Render(gl *engine.GLState, proj [16]float32) {
	ox, oy := selOX, selOY

	if s.resources.Prguse != nil {
		tex := s.resources.GetTexture(s.resources.Prguse, 65)
		if tex != 0 {
			w, h := s.getPrguseSize(65)
			gl.DrawQuad(tex, ox, oy, float32(w), float32(h), proj)
		}
	} else {
		gl.DrawQuadColor(0, 0, 1024, 768, 0.1, 0.15, 0.1, 1.0, proj)
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

func (s *SelectChrScene) renderCharSlot(gl *engine.GLState, proj [16]float32, ox, oy float32, idx int) {
	ch := s.Characters[idx]
	if !ch.Valid {
		return
	}

	slotX := ox + float32(71)
	if idx == 1 {
		slotX = ox + float32(71+340)
	}
	slotY := oy + float32(52)

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
				drawX := slotX + (100-float32(img.Width))/2
				drawY := slotY + 200 - float32(img.Height)
				gl.DrawQuad(tex, drawX, drawY, float32(img.Width), float32(img.Height), proj)
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
		gl.DrawQuadColor(slotX-2, slotY-2, 104, 204, 1.0, 1.0, 0.0, 0.3, proj)
	}
}

func (s *SelectChrScene) renderButtons(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.resources.Prguse == nil {
		return
	}
	buttons := []struct {
		index int
		x, y  float32
	}{
		{66, ox + 134, oy + 424},
		{67, ox + 602, oy + 424},
		{68, ox + 374, oy + 427},
		{69, ox + 349, oy + 467},
		{70, ox + 349, oy + 505},
		{72, ox + 349, oy + 543},
	}
	for _, btn := range buttons {
		tex := s.resources.GetTexture(s.resources.Prguse, btn.index)
		if tex != 0 {
			w, h := s.getPrguseSize(btn.index)
			gl.DrawQuad(tex, btn.x, btn.y, float32(w), float32(h), proj)
		}
	}
}

func (s *SelectChrScene) renderText(gl *engine.GLState, proj [16]float32, ox, oy float32) {
	if s.text == nil {
		return
	}

	for i := 0; i < 2; i++ {
		ch := s.Characters[i]
		if !ch.Valid {
			continue
		}
		textX := ox + float32(50)
		if i == 1 {
			textX = ox + float32(50+340)
		}
		textY := oy + float32(270)

		s.drawTextOutline(ch.Name, textX, textY, 1.0, 1.0, 0.8, 1.0, proj)

		jobName := "未知"
		if int(ch.Job) < len(jobNames) {
			jobName = jobNames[ch.Job]
		}
		info := fmt.Sprintf("Lv.%d %s", ch.Level, jobName)
		s.drawTextOutline(info, textX, textY+20, 0.8, 0.8, 0.8, 1.0, proj)
	}

	if s.errorMsg != "" {
		s.text.DrawText(s.errorMsg, ox+250, oy+400, 1.0, 0.3, 0.3, 1.0, proj)
	}
}

func (s *SelectChrScene) renderCreateDialog(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, 1024, 768, 0, 0, 0, 0.5, proj)
	gl.DrawQuadColor(352, 200, 320, 260, 0.12, 0.12, 0.2, 0.95, proj)
	gl.DrawQuadColor(354, 202, 316, 256, 0.18, 0.18, 0.28, 0.95, proj)

	if s.text == nil {
		return
	}

	s.text.DrawText("创建角色", 460, 215, 1.0, 1.0, 0.5, 1.0, proj)

	s.text.DrawText("角色名:", 372, 262, 0.9, 0.9, 0.9, 1.0, proj)
	gl.DrawQuadColor(440, 258, 200, 22, 0.1, 0.1, 0.1, 1.0, proj)
	s.text.DrawText(s.createName, 444, 262, 1.0, 1.0, 0.8, 1.0, proj)

	showCursor := time.Since(s.cursorBlink) < 250*time.Millisecond
	if time.Since(s.cursorBlink) > 500*time.Millisecond {
		s.cursorBlink = time.Now()
		showCursor = true
	}
	if showCursor {
		cx := float32(444) + float32(s.text.MeasureText(s.createName))
		s.text.DrawText("|", cx, 262, 1.0, 1.0, 0.0, 1.0, proj)
	}

	s.text.DrawText("职业:", 372, 308, 0.9, 0.9, 0.9, 1.0, proj)
	for i, name := range jobNames {
		var r, g, b float32
		if i == s.createJob {
			r, g, b = 1.0, 1.0, 0.3
		} else {
			r, g, b = 0.7, 0.7, 0.7
		}
		gl.DrawQuadColor(createJobAreas[i].X, createJobAreas[i].Y, createJobAreas[i].W, createJobAreas[i].H, 0.2, 0.2, 0.3, 0.8, proj)
		s.text.DrawText(name, createJobAreas[i].X+12, createJobAreas[i].Y+3, r, g, b, 1.0, proj)
	}

	s.text.DrawText("性别:", 372, 348, 0.9, 0.9, 0.9, 1.0, proj)
	sexNames := []string{"男", "女"}
	for i, name := range sexNames {
		var r, g, b float32
		if i == s.createSex {
			r, g, b = 1.0, 1.0, 0.3
		} else {
			r, g, b = 0.7, 0.7, 0.7
		}
		gl.DrawQuadColor(createSexAreas[i].X, createSexAreas[i].Y, createSexAreas[i].W, createSexAreas[i].H, 0.2, 0.2, 0.3, 0.8, proj)
		s.text.DrawText(name, createSexAreas[i].X+20, createSexAreas[i].Y+3, r, g, b, 1.0, proj)
	}

	gl.DrawQuadColor(createConfirmArea.X, createConfirmArea.Y, createConfirmArea.W, createConfirmArea.H, 0.25, 0.35, 0.25, 1.0, proj)
	s.text.DrawText("确 定", createConfirmArea.X+18, createConfirmArea.Y+5, 1.0, 1.0, 1.0, 1.0, proj)

	gl.DrawQuadColor(createCancelArea.X, createCancelArea.Y, createCancelArea.W, createCancelArea.H, 0.35, 0.25, 0.25, 1.0, proj)
	s.text.DrawText("取 消", createCancelArea.X+18, createCancelArea.Y+5, 1.0, 1.0, 1.0, 1.0, proj)
}

func (s *SelectChrScene) renderDeleteDialog(gl *engine.GLState, proj [16]float32) {
	gl.DrawQuadColor(0, 0, 1024, 768, 0, 0, 0, 0.5, proj)
	gl.DrawQuadColor(352, 280, 320, 140, 0.12, 0.12, 0.2, 0.95, proj)
	gl.DrawQuadColor(354, 282, 316, 136, 0.18, 0.18, 0.28, 0.95, proj)

	if s.text == nil {
		return
	}

	msg := fmt.Sprintf("确定删除角色 \"%s\"？", s.deleteName)
	s.text.DrawText(msg, 380, 320, 1.0, 0.8, 0.3, 1.0, proj)

	gl.DrawQuadColor(deleteYesArea.X, deleteYesArea.Y, deleteYesArea.W, deleteYesArea.H, 0.35, 0.25, 0.25, 1.0, proj)
	s.text.DrawText("确 定", deleteYesArea.X+18, deleteYesArea.Y+5, 1.0, 1.0, 1.0, 1.0, proj)

	gl.DrawQuadColor(deleteNoArea.X, deleteNoArea.Y, deleteNoArea.W, deleteNoArea.H, 0.25, 0.35, 0.25, 1.0, proj)
	s.text.DrawText("取 消", deleteNoArea.X+18, deleteNoArea.Y+5, 1.0, 1.0, 1.0, 1.0, proj)
}

func (s *SelectChrScene) drawTextOutline(text string, x, y float32, r, g, b, a float32, proj [16]float32) {
	if s.text == nil {
		return
	}
	s.text.DrawText(text, x-1, y, 0, 0, 0, a, proj)
	s.text.DrawText(text, x+1, y, 0, 0, 0, a, proj)
	s.text.DrawText(text, x, y-1, 0, 0, 0, a, proj)
	s.text.DrawText(text, x, y+1, 0, 0, 0, a, proj)
	s.text.DrawText(text, x, y, r, g, b, a, proj)
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

	if s.createMode {
		for i, area := range createJobAreas {
			if hitTest(fx, fy, area) {
				s.createJob = i
				return
			}
		}
		for i, area := range createSexAreas {
			if hitTest(fx, fy, area) {
				s.createSex = i
				return
			}
		}
		if hitTest(fx, fy, createConfirmArea) {
			s.confirmCreate()
			return
		}
		if hitTest(fx, fy, createCancelArea) {
			s.createMode = false
			return
		}
		return
	}

	if s.deleteConfirm {
		if hitTest(fx, fy, deleteYesArea) {
			s.confirmDelete()
			return
		}
		if hitTest(fx, fy, deleteNoArea) {
			s.deleteConfirm = false
			return
		}
		return
	}

	for i, btn := range selButtonAreas {
		if hitTest(fx, fy, btn) {
			s.handleButton(i)
			return
		}
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
