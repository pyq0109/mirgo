package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// parseProtocolConsts 解析 internal/protocol/message.go 中指定前缀的常量名集合。
func parseProtocolConsts(t *testing.T, prefix string) map[string]uint16 {
	t.Helper()
	fset := token.NewFileSet()
	path := filepath.Join("..", "..", "internal", "protocol", "message.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	consts := make(map[string]uint16)
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			name := vs.Names[0].Name
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			bl, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			v, err := strconv.ParseUint(bl.Value, 0, 16)
			if err != nil {
				continue
			}
			consts[name] = uint16(v)
		}
	}
	if len(consts) == 0 {
		t.Fatalf("no %s constants parsed from %s", prefix, path)
	}
	return consts
}

// TestSMHandlingCoverage — 路线图 6.1 协议覆盖测试：
// 每个 SM 常量必须在客户端代码中被处理（引用），或在豁免表中
// 显式注明原因。新增 SM 不处理会被测试拦截。
func TestSMHandlingCoverage(t *testing.T) {
	// 豁免表：已知不处理的 SM（必须附原因）。
	unhandled := map[string]string{
		"SMSpell2":             "Delphi 旧版魔法消息变体，Go 闭环使用 SMSpell 单一通道",
		"SMCertificationSuccess": "Delphi 登录器认证消息（500），Go 单进程闭环走 SMPassOKSelectServer 流程",
		"SMIDNotFound":         "Delphi 登录错误消息（502），Go 登录失败走 onFail/SMQueryChrFail 流程",
		"SMReconnect":          "换服重连（802）——P3 设计决策：转服默认不做 → backlog",
		"SMTimeCheckMsg":       "客户端反作弊（810）——backlog，服务端已有超速检测替代",
		"SMItemUpdate":         "Delphi 单物品更新（1500），Go 闭环用 SMItemShow/SMItemHide/背包全量同步",
	}

	// 收集客户端全部源码中的 protocol.SMxxx 引用
	references := make(map[string]bool)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		re := regexp.MustCompile(`protocol\.(SM\w+)\b`)
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			references[m[1]] = true
		}
	}
	for name := range parseProtocolConsts(t, "SM") {
		if _, exempt := unhandled[name]; exempt {
			continue
		}
		if !references[name] {
			t.Errorf("SM 常量 %s 未被客户端处理：补 case，或加入豁免表并注明原因", name)
		}
	}
	for name, reason := range unhandled {
		if reason == "" {
			t.Errorf("豁免表 %s 的原因不能为空", name)
		}
		if references[name] {
			t.Errorf("豁免表 %s 已被客户端引用，请从豁免表移除", name)
		}
	}
}
