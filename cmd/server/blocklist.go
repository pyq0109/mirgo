package main

// 黑名单（路线图批次6，P2 网关补偿）：
// - BlockIPList.txt：IP 黑名单（连接期拒绝），Delphi RunGate GateShare.pas:93-94
// - DenyAccountList.txt：账号黑名单（登录期拒绝），Delphi M2Server Envir 目录
// 机器码黑名单（DenyMachineIDList）不做：Go 客户端不上报机器码。
//
// 文件格式：每行一条；'#'/';' 开头为注释；IP 以 '.' 结尾表示前缀匹配
//（如 "192.168.1." 封禁整个 C 段）。

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
)

var blockList struct {
	mu       sync.RWMutex
	ips      []string
	accounts map[string]bool
}

// loadBlockLists 加载 IP/账号黑名单；文件缺失时对应列表为空。
func loadBlockLists(configDir string) {
	blockList.mu.Lock()
	defer blockList.mu.Unlock()
	blockList.ips = readListFile(filepath.Join(configDir, "BlockIPList.txt"))
	accounts := make(map[string]bool)
	for _, a := range readListFile(filepath.Join(configDir, "DenyAccountList.txt")) {
		accounts[strings.ToLower(a)] = true
	}
	blockList.accounts = accounts
	log.Logf(log.LevelInfo, "BlockList", "loaded %d blocked IPs, %d blocked accounts",
		len(blockList.ips), len(blockList.accounts))
}

func readListFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		w := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if w == "" || strings.HasPrefix(w, "#") || strings.HasPrefix(w, ";") {
			continue
		}
		out = append(out, w)
	}
	return out
}

// isIPBlocked 检查 IP 是否命中黑名单（精确或前缀匹配）。
func isIPBlocked(ip string) bool {
	blockList.mu.RLock()
	defer blockList.mu.RUnlock()
	for _, entry := range blockList.ips {
		if entry == ip {
			return true
		}
		if strings.HasSuffix(entry, ".") && strings.HasPrefix(ip, entry) {
			return true
		}
	}
	return false
}

// sessionIP 提取会话的远端 IP（去掉端口）。
func sessionIP(conn net.Conn) string {
	if conn == nil || conn.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return conn.RemoteAddr().String()
	}
	return host
}

// isAccountBlocked 检查账号是否命中黑名单（大小写不敏感）。
func isAccountBlocked(account string) bool {
	blockList.mu.RLock()
	defer blockList.mu.RUnlock()
	return blockList.accounts[strings.ToLower(account)]
}
