package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerConfig 保存服务端配置（与 serverconfig/server.jsonc 格式对应）。
type ServerConfig struct {
	Server struct {
		Name   string `json:"name"`
		Index  int    `json:"index"`
		Listen struct {
			Addr string `json:"addr"`
		} `json:"listen"`
		Limits struct {
			MaxPlayers int `json:"maxPlayers"`
			HumLimit   int `json:"humLimit"`
			MonLimit   int `json:"monLimit"`
		} `json:"limits"`
	} `json:"server"`
	Database struct {
		Path string `json:"path"`
	} `json:"database"`
	Game struct {
		HomeMap         string         `json:"homeMap"`
		HomeX           int            `json:"homeX"`
		HomeY           int            `json:"homeY"`
		GroupMembersMax int            `json:"groupMembersMax"`
		BuildGuild      int            `json:"buildGuild"`
		GuildWarFee     int            `json:"guildWarFee"`
		DisableRun      bool           `json:"disableRun"`
		GMRunAll        bool           `json:"gmRunAll"`
		GMAccounts      map[string]int `json:"gmAccounts"`
		WalkInterval    int64          `json:"walkInterval"`
		RunInterval     int64          `json:"runInterval"`
		SpeedHackKick   bool           `json:"speedHackKick"`
		SpeedHackMax    int            `json:"speedHackMax"`
		ZenLimit        int            `json:"zenLimit"`   // 单 tick 刷怪时间预算(ms)，默认 50
		MonGenRate      int            `json:"monGenRate"` // 刷怪批次比率，默认 10
		WalkOnly        bool           `json:"walkOnly"`        // 禁止跑步（仅步行模式）
		HitIntervalTime int64          `json:"hitIntervalTime"` // 攻击最小间隔(ms)，默认 1400
		ActionInterval  int64          `json:"actionInterval"`  // 全局动作最小间隔(ms)，默认 350
	} `json:"game"`
	Commands struct {
		Names       map[string]string `json:"names"`
		Permissions map[string]int    `json:"permissions"`
	} `json:"commands"`
	Plugins struct {
		Enabled map[string]bool `json:"enabled"`
	} `json:"plugins"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Database: struct {
			Path string `json:"path"`
		}{
			Path: "serverdata/mir2.db",
		},
	}
}

// LoadConfig 从 serverconfig 目录加载配置。
func LoadConfig(configDir string) (*ServerConfig, error) {
	configFile := filepath.Join(configDir, "server.jsonc")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// 移除 JSONC 注释（以 // 开头的行）
	lines := string(data)
	var cleanLines []string
	for _, line := range splitLines(lines) {
		trimmed := trimSpace(line)
		if len(trimmed) >= 2 && trimmed[:2] == "//" {
			continue
		}
		cleanLines = append(cleanLines, line)
	}
	cleanData := joinLines(cleanLines)

	config := DefaultConfig()
	if err := json.Unmarshal([]byte(cleanData), config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return config, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// GetListenAddr 返回监听地址。
func (c *ServerConfig) GetListenAddr() string {
	if c.Server.Listen.Addr != "" {
		return c.Server.Listen.Addr
	}
	return ":7000"
}

// GetDatabasePath 返回数据库路径。
func (c *ServerConfig) GetDatabasePath() string {
	if c.Database.Path != "" {
		return c.Database.Path
	}
	return "serverdata/mir2.db"
}

// GetHomeMap 返回主城地图名。
func (c *ServerConfig) GetHomeMap() string {
	if c.Game.HomeMap != "" {
		return c.Game.HomeMap
	}
	return "0"
}

// GetHomeX 返回主城 X 坐标。
func (c *ServerConfig) GetHomeX() int {
	if c.Game.HomeX > 0 {
		return c.Game.HomeX
	}
	return 289
}

// GetHomeY 返回主城 Y 坐标。
func (c *ServerConfig) GetHomeY() int {
	if c.Game.HomeY > 0 {
		return c.Game.HomeY
	}
	return 618
}

func (c *ServerConfig) GetWalkInterval() int64 {
	if c.Game.WalkInterval > 0 {
		return c.Game.WalkInterval
	}
	return 600
}

func (c *ServerConfig) GetRunInterval() int64 {
	if c.Game.RunInterval > 0 {
		return c.Game.RunInterval
	}
	return 600
}

func (c *ServerConfig) GetSpeedHackMax() int {
	if c.Game.SpeedHackMax > 0 {
		return c.Game.SpeedHackMax
	}
	return 4
}

func (c *ServerConfig) GetZenLimit() int64 {
	if c.Game.ZenLimit > 0 {
		return int64(c.Game.ZenLimit)
	}
	return 50
}

func (c *ServerConfig) GetMonGenRate() int {
	if c.Game.MonGenRate > 0 {
		return c.Game.MonGenRate
	}
	return 10
}

func (c *ServerConfig) GetHitIntervalTime() int64 {
	if c.Game.HitIntervalTime > 0 {
		return c.Game.HitIntervalTime
	}
	return 1400
}

func (c *ServerConfig) GetActionInterval() int64 {
	if c.Game.ActionInterval > 0 {
		return c.Game.ActionInterval
	}
	return 350
}

// GetServerHostPort 以 host/port 形式返回服务端地址。
// 对于监听地址 ":7000"，返回 ("localhost", 7000)。
func (c *ServerConfig) GetServerHostPort() (string, int) {
	addr := c.GetListenAddr()
	host := "localhost"
	port := 7000
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		if idx > 0 {
			host = addr[:idx]
		}
		fmt.Sscanf(addr[idx+1:], "%d", &port)
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return host, port
}
