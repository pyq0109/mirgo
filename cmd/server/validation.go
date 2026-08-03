package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pyq0109/mirgo/internal/log"
)

// denyChrNameList 是创建角色时的禁止字黑名单，对应 Delphi DBServer 启动时
// 加载的 DenyChrName.txt（UsrSoc.pas:199,356-377）。整名大小写不敏感匹配。
var denyChrNameList []string

// loadDenyChrNameList 从配置目录加载 DenyChrName.txt。文件不存在时为空列表
//（不报错），逐行读取并跳过空行。
func loadDenyChrNameList(configDir string) {
	path := filepath.Join(configDir, "DenyChrName.txt")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Logf(log.LevelInfo, "Server", "no DenyChrName.txt in %s (deny list empty)", configDir)
			return
		}
		log.Logf(log.LevelWarn, "Server", "failed to open DenyChrName.txt: %v", err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			denyChrNameList = append(denyChrNameList, line)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Logf(log.LevelWarn, "Server", "error reading DenyChrName.txt: %v", err)
	}
	log.Logf(log.LevelInfo, "Server", "loaded %d denied character names", len(denyChrNameList))
}

// 本文件移植 Delphi 服务端的账号/角色名校验逻辑。
//
// Delphi 原版按 GBK 字节逐字节校验；Go 全链路为 UTF-8，因此做语义移植：
//   - 字节循环 → rune 循环
//   - GBK 汉字（双字节）→ unicode.Is(unicode.Han, r)

// checkAccountName 移植 CheckAccountName（LoginSrv/LSShare.pas:169-192）。
//
// Delphi 规则：每个字节要么在 '0'..'z'（0x30..0x7A）范围内，要么是一个 GBK
// 汉字（首字节 $B0..$C8，尾字节 $A1..$FE）。空串非法。
//
// 注意 '0'..'z' 这个范围忠实保留了 Delphi 的宽松行为——除数字和大小写字母外，
// 还允许 :;<=>?@[\]^_` 等符号（0x3A-0x40、0x5B-0x60）。按用户决定严格照搬。
func checkAccountName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= 'z' {
			continue
		}
		if unicode.Is(unicode.Han, r) {
			continue
		}
		return false
	}
	return true
}

// checkChrName 移植 CheckChrName（DBServer/DBShare.pas:265-304）并合并显式
// 禁止符号表（DBServer/UsrSoc.pas:757-789）。
//
// Delphi CheckChrName 的单字节分支只允许 0-9、a-z、A-Z，双字节按 GBK 处理；
// UsrSoc.NewChr 又额外枚举禁止了 #$A1（全角空格首字节）、空格、/ @ ? ' " \
// . , : ; ` ~ ! # $ % ^ & * ( ) - _ + = | [ { ] } 等符号。
//
// 用白名单实现：仅允许数字、字母、汉字。这已覆盖 Delphi 两份清单的并集——
// 空格、所有 ASCII 标点、全角空格（U+3000）等非汉字字符全部被拒。
func checkChrName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			continue
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case unicode.Is(unicode.Han, r):
			continue
		default:
			return false
		}
	}
	return true
}

// checkDenyChrName 移植 CheckDenyChrName（DBServer/UsrSoc.pas:981-996）。
//
// 对黑名单做整名、大小写不敏感的精确比较；命中任一返回 false（名字不可用）。
func checkDenyChrName(s string, denyList []string) bool {
	for _, denied := range denyList {
		if strings.EqualFold(s, denied) {
			return false
		}
	}
	return true
}
