package main

import (
	"time"
	"unicode/utf8"

	"github.com/pyq0109/mirgo/internal/engine"
)

// EditBox — 单行文本输入控件, 移植自 Delphi TEdit 用法
// (EdDlgEdit FState.pas:656-667, 聊天 EdChat PlayScn.pas:427-440)。
// 通过 UIManager 获取焦点; 处理字符输入及 Backspace/Enter/Esc。
type EditBox struct {
	Ctrl    *UIControl
	gl      *engine.GLState
	text    *engine.TextRenderer
	Text    string
	MaxLen  int // 字符数
	Masked  bool // true 时显示 *** 代替明文
	OnEnter func(text string)
	OnEsc   func()
}

func NewEditBox(gl *engine.GLState, text *engine.TextRenderer, name string, w, h int) *EditBox {
	e := &EditBox{gl: gl, text: text, MaxLen: 80}
	e.Ctrl = NewUIControl(name, KindControl)
	e.Ctrl.Width = w
	e.Ctrl.Height = h
	e.Ctrl.EnableFocus = true
	e.Ctrl.OnChar = func(c *UIControl, ch rune) {
		if ch < 32 || ch == 127 {
			return
		}
		if utf8.RuneCountInString(e.Text) >= e.MaxLen {
			return
		}
		e.Text += string(ch)
	}
	e.Ctrl.OnKeyDown = func(c *UIControl, key int) {
		switch key {
		case keyBackspace:
			if len(e.Text) > 0 {
				_, size := utf8.DecodeLastRuneInString(e.Text)
				e.Text = e.Text[:len(e.Text)-size]
			}
		case keyEnter, keyKPEnter:
			if e.OnEnter != nil {
				e.OnEnter(e.Text)
			}
		case keyEscape:
			if e.OnEsc != nil {
				e.OnEsc()
			}
		}
	}
	e.Ctrl.OnDirectPaint = func(c *UIControl, proj [16]float32) {
		e.paint(proj)
	}
	return e
}

func (e *EditBox) Clear() { e.Text = "" }

func (e *EditBox) paint(proj [16]float32) {
	if e.gl == nil || e.text == nil {
		return
	}
	x, y := float32(e.Ctrl.AbsX()), float32(e.Ctrl.AbsY())
	w, h := float32(e.Ctrl.Width), float32(e.Ctrl.Height)
	e.gl.DrawQuadColor(x, y, w, h, 0, 0, 0, 0.6, proj)

	display := e.Text
	if e.Masked {
		runes := []rune(display)
		masked := make([]rune, len(runes))
		for i := range runes {
			masked[i] = '*'
		}
		display = string(masked)
	}
	focused := gActiveUI != nil && gActiveUI.Focused == e.Ctrl
	if focused && time.Now().UnixMilli()%1000 < 500 {
		display += "|"
	}
	e.text.DrawText(display, x+4, y+(h-float32(e.text.LineHeight()))/2,
		1, 1, 1, 1, proj)
}
