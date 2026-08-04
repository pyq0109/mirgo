package main

import "testing"

func TestFilterChatText(t *testing.T) {
	wordFilter.mu.Lock()
	wordFilter.words = []string{"脏话", "badword", "金币大派送"}
	wordFilter.mu.Unlock()
	defer func() {
		wordFilter.mu.Lock()
		wordFilter.words = nil
		wordFilter.mu.Unlock()
	}()

	cases := []struct {
		in, want string
	}{
		{"大家好", "大家好"},
		{"这里有个脏话啊", "这里有个**啊"},
		{"BADWORD is here", "******* is here"},   // 大小写不敏感
		{"badword", "*******"},
		{"金币大派送快来", "*****快来"},
		{"", ""},
	}
	for _, c := range cases {
		if got := filterChatText(c.in); got != c.want {
			t.Errorf("filterChatText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsIPBlocked(t *testing.T) {
	blockList.mu.Lock()
	blockList.ips = []string{"10.0.0.5", "192.168.1."}
	blockList.accounts = map[string]bool{"baduser": true}
	blockList.mu.Unlock()
	defer func() {
		blockList.mu.Lock()
		blockList.ips = nil
		blockList.accounts = nil
		blockList.mu.Unlock()
	}()

	if !isIPBlocked("10.0.0.5") {
		t.Error("精确 IP 应命中")
	}
	if isIPBlocked("10.0.0.6") {
		t.Error("未列出的 IP 不应命中")
	}
	if !isIPBlocked("192.168.1.123") {
		t.Error("前缀匹配应命中")
	}
	if isIPBlocked("192.168.2.1") {
		t.Error("不同网段不应命中")
	}
	if !isAccountBlocked("BadUser") {
		t.Error("账号黑名单应大小写不敏感命中")
	}
	if isAccountBlocked("gooduser") {
		t.Error("未列出的账号不应命中")
	}
}
