package main

import (
	"encoding/json"

	"github.com/tailscale/hujson"
)

// parseJSONC 解析 JSONC 数据（允许 // 与 /* */ 注释、尾逗号）到 v。
func parseJSONC(data []byte, v any) error {
	// hujson 要求 // 行注释以换行结束；为末尾无换行的文件补齐，
	// 避免“文件以行注释结尾”时报 unexpected EOF。
	if n := len(data); n > 0 && data[n-1] != '\n' {
		data = append(append([]byte(nil), data...), '\n')
	}
	clean, err := hujson.Standardize(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(clean, v)
}
