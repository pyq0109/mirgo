package main

import (
	"strings"
)

// DialogBox — 移植自 DMsgDlg (FState.pas:1938-2158)。非阻塞: 结果
// 通过回调返回 (Delphi 的阻塞消息循环 :2098-2117 在 GLFW 下无法运行)。
// 三种尺寸, Ok/Yes/No/Cancel 按钮从 lx=324 起以 110px 步长
// 从右向左排列 (:2060-2083), Enter/Esc 语义同
// DMsgDlgKeyDown (:2139-2158)。
type ModalResult int

const (
	MrNone ModalResult = iota
	MrOk
	MrYes
	MrNo
	MrCancel
)

// 对话框尺寸 (FState.pas:2002-2042): 小 [381], 标准 [360], 高 [380]。
const (
	DlgSmall  = 0
	DlgNormal = 1
	DlgTall   = 2
)

var dialogImages = map[int]int{DlgSmall: ImgModalSmall, DlgNormal: ImgModalNormal, DlgTall: ImgModalTall}

// dialogButtonLayout 给出各尺寸对话框的按钮锚点 (最右按钮左边缘, 顶部)
// (FState:2002-2042)。
var dialogButtonLayout = map[int]struct{ lx, ly int }{
	DlgSmall:  {90, 36},
	DlgNormal: {324, 126},
	DlgTall:   {105, 305},
}

// dialogMsgLayout 给出各尺寸对话框的消息文本起点 (左上角, 相对窗口)
// (FState.pas:2010-2037)。
var dialogMsgLayout = map[int]struct{ lx, ly int }{
	DlgSmall:  {39, 38},
	DlgNormal: {39, 38},
	DlgTall:   {23, 20},
}

// dialogButtonOrder 是固定的从右向左排列顺序, 与调用方按钮集无关
// (FState:2060-2083: Cancel, No, Yes, Ok)。
var dialogButtonOrder = []ModalResult{MrCancel, MrNo, MrYes, MrOk}

var dialogButtonImages = map[ModalResult]int{
	MrOk:     ImgModalOk,     // 361 (+1 = 362 按下态)
	MrYes:    ImgModalYes,    // 363 (+1 = 364)
	MrNo:     ImgModalNo,     // 367 (+1 = 368)
	MrCancel: ImgModalCancel, // 365 (+1 = 366)
}

type DialogBox struct {
	scene    *PlayScene
	win      *UIControl
	msgLines []string
	buttons  []ModalResult
	edit     *EditBox // 输入模式下非 nil (Delphi mbAbort 对话框)
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

// handleKey 对应 DMsgDlgKeyDown (FState.pas:2139-2158): 仅有单个
// Ok/Yes 时 Enter 确认; 有 Cancel 时 Esc 取消。
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

// showDialog 构建并显示模态框。inputMode 添加居中编辑框
// (Delphi EdDlgEdit, :2086-2095); 结果文本随 MrOk 返回。
func showDialog(scene *PlayScene, size int, msg string, buttons []ModalResult, inputMode bool, onResult func(ModalResult, string)) *DialogBox {
	img, ok := dialogImages[size]
	if !ok {
		img = ImgModalNormal
	}
	d := &DialogBox{scene: scene, buttons: buttons, onResult: onResult}
	d.msgLines = wrapDialogText(scene, msg, 260)

	win := NewUIControl("DMsgDlg", KindWindow)
	win.Floating = true // DMessageDlg 设置 Floating := TRUE (:2045)
	// 先根据背景图确定窗口尺寸, 再放置子控件。
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
		// 消息文本左对齐, 从对应尺寸起点绘制, 14px 行距,
		// 白色带黑色描边 (FState.pas:2010-2037, 2314-2323)。
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

	// 按钮: 固定 Cancel→No→Yes→Ok 排列顺序, 从对应尺寸锚点起
	// 从右向左, 步长 110px (FState:2060-2083)。
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
				idx++ // 弹起/按下图片对惯例 (DWinCtl TDButton)
			}
			scene.ui.BlitImage(scene.resources.Prguse, idx, c.AbsX(), c.AbsY(), proj)
		}
		btn.OnClick = func(c *UIControl, x, y int) { d.complete(result) }
		win.AddChild(btn)
		lx -= 110
	}

	if inputMode {
		// 宽度 = 窗口宽 - 170, 水平居中 (x=85), 垂直居中偏上 10px,
		// 相对窗口定位 (FState.pas:2089-2094)。
		edit := NewEditBox(scene, "EdDlgEdit", win.Width-170, 20)
		edit.MaxLen = 30 // 对应 EdDlgEdit.MaxLength := 30 (:662)
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

	// 居中并显示 (FState.pas:2050-2051)。
	win.Left = (ScreenWidth - win.Width) / 2
	win.Top = (ScreenHeight - win.Height) / 2

	scene.ui.ShowModal(win)
	if d.edit != nil {
		scene.ui.SetFocus(d.edit.Ctrl)
	}
	return d
}

// ShowConfirm 显示消息框; cb 接收所选按钮。
func ShowConfirm(scene *PlayScene, msg string, buttons []ModalResult, size int, cb func(ModalResult)) {
	showDialog(scene, size, msg, buttons, false, func(mr ModalResult, _ string) {
		if cb != nil {
			cb(mr)
		}
	})
}

// ShowInput 显示数字/文本输入框 (Delphi mbAbort 对话框)。取消时
// cb 收到 ok=false。
func ShowInput(scene *PlayScene, msg string, cb func(ok bool, text string)) {
	showDialog(scene, DlgNormal, msg, []ModalResult{MrOk, MrCancel}, true, func(mr ModalResult, text string) {
		if cb != nil {
			cb(mr == MrOk, text)
		}
	})
}

// wrapDialogText 按 '\' 分割 msg (Delphi 硬换行, :2317-2323) 并在
// maxWidth 像素处折行。
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
