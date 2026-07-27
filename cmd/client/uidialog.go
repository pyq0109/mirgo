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

// dialogButtonLayout gives the button anchor (left edge of the right-most
// button, top) per dialog size (FState:2002-2042).
var dialogButtonLayout = map[int]struct{ lx, ly int }{
	DlgSmall:  {90, 36},
	DlgNormal: {324, 126},
	DlgTall:   {105, 305},
}

// dialogMsgLayout gives the message text origin (top-left, window-relative)
// per dialog size (FState.pas:2010-2037).
var dialogMsgLayout = map[int]struct{ lx, ly int }{
	DlgSmall:  {39, 38},
	DlgNormal: {39, 38},
	DlgTall:   {23, 20},
}

// dialogButtonOrder is the fixed right-to-left placement order, regardless
// of the caller's button set (FState:2060-2083: Cancel, No, Yes, Ok).
var dialogButtonOrder = []ModalResult{MrCancel, MrNo, MrYes, MrOk}

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
		// Left-aligned message text at the size-specific origin, 14px line
		// spacing, white with a black outline (FState.pas:2010-2037, 2314-2323).
		ml, hasML := dialogMsgLayout[size]
		if !hasML {
			ml = dialogMsgLayout[DlgNormal]
		}
		y := c.AbsY() + ml.ly
		for _, ln := range d.msgLines {
			scene.text.DrawTextOutline(ln, float32(c.AbsX()+ml.lx), float32(y),
				1, 1, 1, 1, 0, 0, 0, 1, proj)
			y += 14
		}
	}
	d.win = win

	// Buttons: fixed Cancel→No→Yes→Ok placement order, right-to-left from
	// the size-specific anchor, 110px step (FState:2060-2083).
	layout, hasLayout := dialogButtonLayout[size]
	if !hasLayout {
		layout = dialogButtonLayout[DlgNormal]
	}
	ordered := make([]ModalResult, 0, len(buttons))
	for _, mr := range dialogButtonOrder {
		for _, b := range buttons {
			if b == mr {
				ordered = append(ordered, mr)
				break
			}
		}
	}
	lx, ly := layout.lx, layout.ly
	for _, mr := range ordered {
		btnImg := dialogButtonImages[mr]
		btn := NewUIControl("DMsgDlgBtn", KindButton)
		btn.SetImgIndex(scene.resources.Prguse, btnImg)
		btn.Left = lx
		btn.Top = ly
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
		lx -= 110
	}

	if inputMode {
		// Width = window width - 170, centered horizontally (x=85) and
		// vertically centered minus 10px, window-relative (FState.pas:2089-2094).
		edit := NewEditBox(scene, "EdDlgEdit", win.Width-170, 20)
		edit.MaxLen = 30 // EdDlgEdit.MaxLength := 30 (:662)
		edit.Ctrl.Left = (win.Width - edit.Ctrl.Width) / 2
		edit.Ctrl.Top = win.Height/2 - edit.Ctrl.Height/2 - 10
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
