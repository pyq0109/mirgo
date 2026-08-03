package main

import "testing"

func TestCheckAccountName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"小写字母数字", "abc123", true},
		{"大写字母", "ABCxyz", true},
		{"下划线在0x30-0x7A范围内", "abc_1", true},
		{"Delphi宽松范围内的符号", "a:b@c[d]e", true},
		{"汉字", "测试账号", true},
		{"字母数字混合汉字", "user1测试", true},
		{"空串", "", false},
		{"空格", "ab c", false},
		{"前导空格", " abc", false},
		{"感叹号低于0x30", "abc!", false},
		{"井号低于0x30", "#abc", false},
		{"竖线高于0x7A", "abc|", false},
		{"波浪号高于0x7A", "abc~", false},
		{"花括号高于0x7A", "abc{", false},
	}
	for _, tt := range tests {
		if got := checkAccountName(tt.in); got != tt.want {
			t.Errorf("%s: checkAccountName(%q) = %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestCheckChrName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"汉字", "战士甲", true},
		{"字母数字", "abc123", true},
		{"混合", "勇者abc", true},
		{"空串", "", false},
		{"空格", "a b", false},
		{"斜杠", "a/b", false},
		{"at符号", "名@字", false},
		{"全角空格", "名\u3000字", false},
		{"标点", "勇,士", false},
		{"连字符", "a-b", false},
		{"emoji", "勇😀士", false},
	}
	for _, tt := range tests {
		if got := checkChrName(tt.in); got != tt.want {
			t.Errorf("%s: checkChrName(%q) = %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestCheckDenyChrName(t *testing.T) {
	list := []string{"gm", "管理员", "System"}
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"正常名字", "战士", true},
		{"命中黑名单", "gm", false},
		{"大小写不敏感命中", "GM", false},
		{"中文黑名单", "管理员", false},
		{"部分包含不算命中", "gmx", true},
		{"空黑名单外的名字", "勇者", true},
	}
	for _, tt := range tests {
		if got := checkDenyChrName(tt.in, list); got != tt.want {
			t.Errorf("%s: checkDenyChrName(%q) = %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
	// 空黑名单一切名字可用
	if !checkDenyChrName("任意", nil) {
		t.Error("空黑名单应允许任意名字")
	}
}
