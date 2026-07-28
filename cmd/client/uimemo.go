package main

import (
	"strings"
	"time"
	"unicode/utf8"
)

// MemoBox — 多行文本编辑器, 移植自行会公告/职位编辑器所用的 VCL TMemo
// (FState.pas:6232-6237, 6285-6290). 通过 UIManager 获取焦点;
// 支持回车换行、方向键移动光标和滚动.
type MemoBox struct {
	Ctrl       *UIControl
	scene      *PlayScene
	Lines      []string
	curX, curY int // 字符索引 / 行索引
	scroll     int
	MaxLen     int // 所有行的字符总数上限
	lineH      int
}

func NewMemoBox(scene *PlayScene, name string, w, h int) *MemoBox {
	m := &MemoBox{scene: scene, Lines: []string{""}, MaxLen: 4000, lineH: 14}
	m.Ctrl = NewUIControl(name, KindControl)
	m.Ctrl.Width = w
	m.Ctrl.Height = h
	m.Ctrl.EnableFocus = true
	m.Ctrl.OnChar = func(c *UIControl, ch rune) {
		if ch < 32 || ch == 127 {
			return
		}
		if m.totalLen() >= m.MaxLen {
			return
		}
		line := []rune(m.Lines[m.curY])
		ins := append([]rune{ch}, line[m.curX:]...)
		m.Lines[m.curY] = string(append(line[:m.curX], ins...))
		m.curX++
	}
	m.Ctrl.OnKeyDown = func(c *UIControl, key int) {
		switch key {
		case keyBackspace:
			if m.curX > 0 {
				line := []rune(m.Lines[m.curY])
				m.Lines[m.curY] = string(append(line[:m.curX-1], line[m.curX:]...))
				m.curX--
			} else if m.curY > 0 {
				// 与上一行合并.
				prev := m.Lines[m.curY-1]
				m.curX = utf8.RuneCountInString(prev)
				m.Lines[m.curY-1] = prev + m.Lines[m.curY]
				m.Lines = append(m.Lines[:m.curY], m.Lines[m.curY+1:]...)
				m.curY--
			}
		case keyEnter, keyKPEnter:
			if m.totalLen() >= m.MaxLen {
				return
			}
			line := []rune(m.Lines[m.curY])
			rest := string(line[m.curX:])
			m.Lines[m.curY] = string(line[:m.curX])
			m.Lines = append(m.Lines[:m.curY+1], append([]string{rest}, m.Lines[m.curY+1:]...)...)
			m.curY++
			m.curX = 0
		case 265: // 上
			if m.curY > 0 {
				m.curY--
				m.clampX()
			}
		case 264: // 下
			if m.curY < len(m.Lines)-1 {
				m.curY++
				m.clampX()
			}
		case 263: // 左
			if m.curX > 0 {
				m.curX--
			} else if m.curY > 0 {
				m.curY--
				m.curX = utf8.RuneCountInString(m.Lines[m.curY])
			}
		case 262: // 右
			if m.curX < utf8.RuneCountInString(m.Lines[m.curY]) {
				m.curX++
			} else if m.curY < len(m.Lines)-1 {
				m.curY++
				m.curX = 0
			}
		}
		m.keepVisible()
	}
	m.Ctrl.OnDirectPaint = func(c *UIControl, proj [16]float32) { m.paint(proj) }
	return m
}

func (m *MemoBox) totalLen() int {
	n := 0
	for _, ln := range m.Lines {
		n += utf8.RuneCountInString(ln)
	}
	return n
}

func (m *MemoBox) clampX() {
	if n := utf8.RuneCountInString(m.Lines[m.curY]); m.curX > n {
		m.curX = n
	}
}

// keepVisible 滚动以保证光标所在行可见.
func (m *MemoBox) keepVisible() {
	vis := m.Ctrl.Height / m.lineH
	if vis < 1 {
		vis = 1
	}
	if m.curY < m.scroll {
		m.scroll = m.curY
	}
	if m.curY >= m.scroll+vis {
		m.scroll = m.curY - vis + 1
	}
}

// SetText 载入文本, 按 CR/LF 分行.
func (m *MemoBox) SetText(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	m.Lines = strings.Split(text, "\n")
	if len(m.Lines) == 0 {
		m.Lines = []string{""}
	}
	m.curX, m.curY, m.scroll = 0, 0, 0
}

// JoinedText 以 CR 分隔返回内容, 空行替换为单个空格
// (Delphi 公告拼接方式, FState:6256-6261).
func (m *MemoBox) JoinedText() string {
	parts := make([]string, len(m.Lines))
	for i, ln := range m.Lines {
		if ln == "" {
			ln = " "
		}
		parts[i] = ln
	}
	return strings.Join(parts, "\r")
}

func (m *MemoBox) paint(proj [16]float32) {
	s := m.scene
	if s == nil || s.text == nil {
		return
	}
	x, y := float32(m.Ctrl.AbsX()), float32(m.Ctrl.AbsY())
	s.gl.DrawQuadColor(x, y, float32(m.Ctrl.Width), float32(m.Ctrl.Height), 0, 0, 0, 0.6, proj)
	vis := m.Ctrl.Height / m.lineH
	focused := s.ui.Focused == m.Ctrl
	blink := time.Now().UnixMilli()%1000 < 500
	for i := m.scroll; i < len(m.Lines) && i < m.scroll+vis; i++ {
		ln := m.Lines[i]
		ly := y + float32((i-m.scroll)*m.lineH)
		s.text.DrawText(ln, x+2, ly, 1, 1, 1, 1, proj)
		if focused && i == m.curY && blink {
			pre := string([]rune(ln)[:m.curX])
			cx := x + 2 + float32(s.text.MeasureText(pre))
			s.text.DrawText("|", cx, ly, 1, 1, 1, 1, proj)
		}
	}
}
