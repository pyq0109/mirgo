package main

// 聊天敏感词过滤 — Delphi RunGate FilterSayMsg（Main.pas:554-579）：
// 网关级重写 CM_SAY/私聊内容。Go 单进程架构下在消息处理层补偿
//（路线图 6.3 连接中间件链的过滤层）。
//
// 词表文件：serverconfig/WordFilter.txt，每行一个敏感词，
// '#' 或 ';' 开头为注释。命中后按字符数替换为 '*'（大小写不敏感）。

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pyq0109/mirgo/internal/log"
)

var wordFilter struct {
	mu    sync.RWMutex
	words []string
}

// loadWordFilter 加载敏感词表；文件不存在时不过滤（仅日志提示）。
func loadWordFilter(configDir string) {
	path := filepath.Join(configDir, "WordFilter.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelInfo, "WordFilter", "no word filter list at %s (chat unfiltered)", path)
		return
	}
	var words []string
	for _, line := range strings.Split(string(data), "\n") {
		w := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if w == "" || strings.HasPrefix(w, "#") || strings.HasPrefix(w, ";") {
			continue
		}
		words = append(words, w)
	}
	wordFilter.mu.Lock()
	wordFilter.words = words
	wordFilter.mu.Unlock()
	log.Logf(log.LevelInfo, "WordFilter", "loaded %d words from %s", len(words), path)
}

// filterChatText 将命中的敏感词替换为等字符数的 '*'（大小写不敏感，
// Delphi AnsiReplaceText 语义）。
func filterChatText(text string) string {
	wordFilter.mu.RLock()
	words := wordFilter.words
	wordFilter.mu.RUnlock()
	if len(words) == 0 || text == "" {
		return text
	}
	result := text
	for _, w := range words {
		wl := strings.ToLower(w)
		stars := strings.Repeat("*", utf8.RuneCountInString(w))
		lowerText := strings.ToLower(result)
		var sb strings.Builder
		idx := 0
		for {
			i := strings.Index(lowerText[idx:], wl)
			if i < 0 {
				sb.WriteString(result[idx:])
				break
			}
			pos := idx + i
			sb.WriteString(result[idx:pos])
			sb.WriteString(stars)
			idx = pos + len(w)
		}
		result = sb.String()
	}
	return result
}
