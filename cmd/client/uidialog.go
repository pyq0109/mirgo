package main

import (
	"strings"
)

// DialogBox — port of DMsgDlg (FState.pas:1938-2158). Non-blocking: results
// arrive via callback (Delphi's blocking pump loop at :2098-2117 cannot run
// under GLFW). Three sizes, Ok/Yes/No/Cancel buttons laid out right-to-left
// from lx=324 with a 110px step (:2060-2083), Enter/Esc semantics of
// DMsgDlgKeyDown (:2139-2158).
type ModalResult int

const (
	MrNone ModalResult = iota
	MrOk
	MrYes
	MrNo
	MrCancel
)

// Dialog sizes (FState.pas:2002-2042): small [381], normal [360], tall [380].
const (
	DlgSmall  = 0
	DlgNormal = 1
	DlgTall   = 2
)

var dialogImages = map[int]int{DlgSmall: ImgModalSmall, DlgNormal: ImgModalNormal, DlgTall: ImgModalTall}

var dialogButtonImages = map[ModalResult]int{
	MrOk:     ImgModalOk,     // 361 (+1 = 362 pressed)
	MrYes:    ImgModalYes,    // 363 (+1 = 364)
	MrNo:     ImgModalNo,     // 367 (+1 = 368)
	MrCancel: ImgModalCancel, // 365 (+1 = 366)
}

type DialogBox struct {
	scene    *PlayScene
	win      *UIControl
	msgLines []string
	buttons  []ModalResult
	edit     *EditBox // non-nil in input mode (Delphi mbAbort dialogs)
	onResult func(mr ModalResult, text string)
}

func (d *DialogBox) complete(mr ModalResult) {
	text := ""
	if d.edit != nil {
		text = d.edit.Text
	}
	d.scene.ui.CloseModal(d.win)
	if d.onResult != nil {
		d.onResult(mr, text)
	}
}

// handleKey mirrors DMsgDlgKeyDown (FState.pas:2139-2158): Enter confirms
// when a single Ok/Yes is offered; Esc cancels when Cancel is present.
func (d *DialogBox) handleKey(key int) bool {
	has := func(mr ModalResult) bool {
		for _, b := range d.buttons {
			if b == mr {
				return true
			}
		}
		return false
	}
	switch key {
	case keyEnter, keyKPEnter:
		if len(d.buttons) == 1 && has(MrOk) {
			d.complete(MrOk)
			return true
		}
		if len(d.buttons) == 1 && has(MrYes) {
			d.complete(MrYes)
			return true
		}
	case keyEscape:
		if has(MrCancel) {
			d.complete(MrCancel)
			return true
		}
	}
	return false
}

// showDialog builds and shows the modal. inputMode adds a centered edit box
// (Delphi EdDlgEdit, :2086-2095); the result text is delivered with MrOk.
func showDialog(scene *PlayScene, size int, msg string, buttons []ModalResult, inputMode bool, onResult func(ModalResult, string)) *DialogBox {
	img, ok := dialogImages[size]
	if !ok {
		img = ImgModalNormal
	}
	d := &DialogBox{scene: scene, buttons: buttons, onResult: onResult}
	d.msgLines = wrapDialogText(scene, msg, 260)

	win := NewUIControl("DMsgDlg", KindWindow)
	win.Floating = true // DMessageDlg sets Floating := TRUE (:2045)
	// Size the window from its background image before placing children.
	if scene.resources.Prguse != nil {
		win.SetImgIndex(scene.resources.Prguse, img)
	} else {
		win.Width, win.Height = 300, 170
	}
	win.OnKeyDown = func(c *UIControl, key int) { d.handleKey(key) }
	win.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		scene.ui.BlitImage(scene.resources.Prguse, img, c.AbsX(), c.AbsY(), proj)
		if scene.text == nil {
			return
		}
		lh := scene.text.LineHeight()
		y := c.AbsY() + 36
		for _, ln := range d.msgLines {
			lw := scene.text.MeasureText(ln)
			scene.text.DrawText(ln, float32(c.AbsX()+(c.Width-lw)/2), float32(y), 1, 1, 1, 1, proj)
			y += lh + 2
		}
	}
	d.win = win

	// Buttons: right-to-left from lx=324, step 110, Top=126 (:2060-2083).
	lx := 324
	for _, mr := range buttons {
		btnImg := dialogButtonImages[mr]
		btn := NewUIControl("DMsgDlgBtn", KindButton)
		btn.SetImgIndex(scene.resources.Prguse, btnImg)
		lx -= btn.Width
		btn.Left = lx
		btn.Top = 126
		btn.FaceIndex = btnImg
		result := mr
		btn.OnDirectPaint = func(c *UIControl, proj [16]float32) {
			idx := c.FaceIndex
			if c.Downed {
				idx++ // up/down image pair convention (DWinCtl TDButton)
			}
			scene.ui.BlitImage(scene.resources.Prguse, idx, c.AbsX(), c.AbsY(), proj)
		}
		btn.OnClick = func(c *UIControl, x, y int) { d.complete(result) }
		win.AddChild(btn)
		lx -= 110 - btn.Width // net 110px step between buttons
	}

	if inputMode {
		edit := NewEditBox(scene, "EdDlgEdit", 200, 20)
		edit.MaxLen = 30 // EdDlgEdit.MaxLength := 30 (:662)
		edit.Ctrl.Left = (win.Width - edit.Ctrl.Width) / 2
		edit.Ctrl.Top = 92
		edit.OnEnter = func(text string) { d.complete(MrOk) }
		edit.OnEsc = func() {
			for _, b := range d.buttons {
				if b == MrCancel {
					d.complete(MrCancel)
					return
				}
			}
		}
		win.AddChild(edit.Ctrl)
		d.edit = edit
	}

	// Center and show (FState.pas:2050-2051).
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2

	scene.ui.ShowModal(win)
	if d.edit != nil {
		scene.ui.SetFocus(d.edit.Ctrl)
	}
	return d
}

// ShowConfirm shows a message box; cb receives the chosen button.
func ShowConfirm(scene *PlayScene, msg string, buttons []ModalResult, size int, cb func(ModalResult)) {
	showDialog(scene, size, msg, buttons, false, func(mr ModalResult, _ string) {
		if cb != nil {
			cb(mr)
		}
	})
}

// ShowInput shows a number/text input box (Delphi mbAbort dialogs). cb gets
// ok=false on cancel.
func ShowInput(scene *PlayScene, msg string, cb func(ok bool, text string)) {
	showDialog(scene, DlgNormal, msg, []ModalResult{MrOk, MrCancel}, true, func(mr ModalResult, text string) {
		if cb != nil {
			cb(mr == MrOk, text)
		}
	})
}

// wrapDialogText splits msg on '\' (Delphi hard breaks, :2317-2323) and wraps
// long lines at maxWidth pixels.
func wrapDialogText(scene *PlayScene, msg string, maxWidth int) []string {
	var out []string
	for _, seg := range strings.Split(msg, "\\") {
		if scene.text == nil || scene.text.MeasureText(seg) <= maxWidth {
			out = append(out, seg)
			continue
		}
		runes := []rune(seg)
		start := 0
		for end := 1; end <= len(runes); end++ {
			if scene.text.MeasureText(string(runes[start:end])) > maxWidth || end == len(runes) {
				cut := end - 1
				if cut <= start {
					cut = end
				}
				out = append(out, string(runes[start:cut]))
				start = cut
			}
		}
	}
	return out
}
