package main

// Delphi 真值对照 — MShare.pas:468-541 含原版客户端完整的 DlgConf
// 布局真值 (TConfig 记录字面量)。本测试解析它并与 Go 版 DlgConf
// (uiconst.go) 逐条对照: "数值抄对了吗" 由机器判定。
// 故意不按 Delphi 值调整 UI 时: 在 delphiExempt 登记条目名+原因,
// 对照即跳过该项 — 每次有意偏离都留痕。

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const delphiMSharePath = "../../asset/delphi/Client/MShare.pas"

// delphiExempt 豁免表: 故意与 Delphi 不一致的条目。key=条目名, value=原因。
var delphiExempt = map[string]string{
	"DSellDlgSpot": "Delphi Width/Height=0(自动取尺寸); Go 显式 61×52 (FState:1358-1361 运行时值)",
	"DItemGrid":    "DlgConf 的 Left/Top(29,41) 是死数据; Go 用 FState:1171-1174 硬编码 33,43 (见 uiconst.go 注释)",
}

type delphiDlgEntry struct {
	Image, Left, Top, Width, Height int
}

func TestDlgConfMatchesDelphi(t *testing.T) {
	data, err := os.ReadFile(delphiMSharePath)
	if err != nil {
		t.Skipf("Delphi 源码不可用, 跳过真值对照: %v", err)
	}
	entries, err := parseDelphiDlgConf(string(data))
	if err != nil {
		t.Fatalf("解析 MShare.pas DlgConf 失败: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("解析到 0 条 DlgConf 条目, MShare.pas 格式可能已变")
	}

	for name, goEntry := range DlgConf {
		want, ok := entries[name]
		if !ok {
			t.Errorf("Go DlgConf[%s]: Delphi MShare.pas 无同名条目 (若为有意新增请说明来源)", name)
			continue
		}
		if reason, exempt := delphiExempt[name]; exempt {
			t.Logf("豁免 %s: %s (delphi={Image:%d Left:%d Top:%d W:%d H:%d})",
				name, reason, want.Image, want.Left, want.Top, want.Width, want.Height)
			continue
		}
		got := delphiDlgEntry{goEntry.Image, goEntry.Left, goEntry.Top, goEntry.W, goEntry.H}
		if got != want {
			t.Errorf("DlgConf[%s] 与 Delphi 不一致:\n  go    =(Image:%d Left:%d Top:%d W:%d H:%d)\n  delphi=(Image:%d Left:%d Top:%d W:%d H:%d)\n若为有意调整, 请在 uidelphi_test.go delphiExempt 登记原因",
				name, got.Image, got.Left, got.Top, got.Width, got.Height,
				want.Image, want.Left, want.Top, want.Width, want.Height)
		}
	}
}

// --- 解析器 ---

var (
	delphiDlgConfStartRe = regexp.MustCompile(`DlgConf\s*:\s*TConfig\s*=\s*\(`)
	delphiEntryStartRe   = regexp.MustCompile(`([A-Za-z_]\w*)\s*:\(`)
)

// parseDelphiDlgConf 从 MShare.pas 源码提取 DlgConf 全部条目。
// 处理: {..} 与 // 注释、跨行条目、Left 里的算术表达式 (219+30*2、
// SCREENWIDTH div 2 + (...))。
func parseDelphiDlgConf(src string) (map[string]delphiDlgEntry, error) {
	src = stripDelphiComments(src)
	loc := delphiDlgConfStartRe.FindStringIndex(src)
	if loc == nil {
		return nil, fmt.Errorf("未找到 DlgConf:TConfig=( 块")
	}
	open := loc[1] - 1 // '(' 位置
	depth, end := 0, -1
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("DlgConf 块括号不平衡")
	}

	entries := map[string]delphiDlgEntry{}
	rest := src[open+1 : end]
	for {
		m := delphiEntryStartRe.FindStringSubmatchIndex(rest)
		if m == nil {
			break
		}
		name := rest[m[2]:m[3]]
		openIdx := m[1] - 1 // 条目记录的 '('
		depth, closeIdx := 0, -1
		for i := openIdx; i < len(rest); i++ {
			switch rest[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					closeIdx = i
				}
			}
			if closeIdx >= 0 {
				break
			}
		}
		if closeIdx < 0 {
			return nil, fmt.Errorf("条目 %s 括号不平衡", name)
		}
		e, err := parseDelphiFields(name, rest[openIdx+1:closeIdx])
		if err != nil {
			return nil, err
		}
		entries[name] = e
		rest = rest[closeIdx+1:]
	}
	return entries, nil
}

func parseDelphiFields(name, body string) (delphiDlgEntry, error) {
	var e delphiDlgEntry
	n := 0
	for _, field := range strings.Split(body, ";") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		kv := strings.SplitN(field, ":", 2)
		if len(kv) != 2 {
			return e, fmt.Errorf("条目 %s 字段 %q 缺少冒号", name, field)
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val, err := evalDelphiExpr(strings.TrimSpace(kv[1]))
		if err != nil {
			return e, fmt.Errorf("条目 %s 字段 %s: %v", name, key, err)
		}
		switch key {
		case "image":
			e.Image = val
		case "left":
			e.Left = val
		case "top":
			e.Top = val
		case "width":
			e.Width = val
		case "height":
			e.Height = val
		default:
			return e, fmt.Errorf("条目 %s 未知字段 %q", name, key)
		}
		n++
	}
	if n == 0 {
		return e, fmt.Errorf("条目 %s 为空", name)
	}
	return e, nil
}

// stripDelphiComments 去掉 // 行注释与 {..} 块注释 (后者可跨行)。
func stripDelphiComments(src string) string {
	var lines []string
	for _, ln := range strings.Split(src, "\n") {
		if i := strings.Index(ln, "//"); i >= 0 {
			ln = ln[:i]
		}
		lines = append(lines, ln)
	}
	src = strings.Join(lines, "\n")
	var sb strings.Builder
	depth := 0
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				sb.WriteByte(src[i])
			}
		}
	}
	return sb.String()
}

// --- 迷你表达式求值器 (整数, + - * div, 括号, SCREENWIDTH/SCREENHEIGHT) ---

func evalDelphiExpr(s string) (int, error) {
	p := &exprParser{toks: tokenizeExpr(s)}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos != len(p.toks) {
		return 0, fmt.Errorf("表达式 %q 有剩余 token %q", s, strings.Join(p.toks[p.pos:], " "))
	}
	return v, nil
}

func tokenizeExpr(s string) []string {
	var toks []string
	isAlnum := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
		case isAlnum(c):
			j := i
			for j < len(s) && isAlnum(s[j]) {
				j++
			}
			toks = append(toks, strings.ToLower(s[i:j]))
			i = j
		case c == '+' || c == '-' || c == '*' || c == '(' || c == ')':
			toks = append(toks, string(c))
			i++
		default:
			toks = append(toks, string(c)) // 未知字符交给解析器报错
			i++
		}
	}
	return toks
}

type exprParser struct {
	toks []string
	pos  int
}

func (p *exprParser) next() string {
	if p.pos >= len(p.toks) {
		return ""
	}
	t := p.toks[p.pos]
	p.pos++
	return t
}

func (p *exprParser) parseExpr() (int, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		switch t := p.next(); t {
		case "+":
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v += r
		case "-":
			r, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			v -= r
		default:
			if t != "" {
				p.pos--
			}
			return v, nil
		}
	}
}

func (p *exprParser) parseTerm() (int, error) {
	v, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		switch t := p.next(); t {
		case "*":
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			v *= r
		case "div":
			r, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("除以零")
			}
			v /= r
		default:
			if t != "" {
				p.pos--
			}
			return v, nil
		}
	}
}

func (p *exprParser) parseFactor() (int, error) {
	t := p.next()
	switch {
	case t == "":
		return 0, fmt.Errorf("表达式意外结束")
	case t == "(":
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		if p.next() != ")" {
			return 0, fmt.Errorf("缺少右括号")
		}
		return v, nil
	case t == "-":
		v, err := p.parseFactor()
		return -v, err
	case t[0] >= '0' && t[0] <= '9':
		return strconv.Atoi(t)
	case t == "screenwidth":
		return ScreenWidth, nil
	case t == "screenheight":
		return ScreenHeight, nil
	default:
		return 0, fmt.Errorf("未知 token %q", t)
	}
}

// TestDelphiExprEval 求值器自检 (不依赖 Delphi 源码)。
func TestDelphiExprEval(t *testing.T) {
	cases := []struct {
		expr string
		want int
	}{
		{"0", 0},
		{"219 + 30", 249},
		{"219 + 30*2", 279},
		{"SCREENWIDTH div 2 + (SCREENWIDTH div 2 - (400 - 353))", 753},  // DBotMemo
		{"SCREENWIDTH div 2 + (SCREENWIDTH div 2 - (400 - 160)) - 30", 530}, // DBotLogout
	}
	for _, tc := range cases {
		got, err := evalDelphiExpr(tc.expr)
		if err != nil {
			t.Errorf("%q: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q = %d, want %d", tc.expr, got, tc.want)
		}
	}
	if _, err := evalDelphiExpr("1 +"); err == nil {
		t.Error("不完整表达式应报错")
	}
}
