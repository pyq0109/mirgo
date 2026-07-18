package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerConfig holds server configuration (matches serverconfig/server.jsonc format).
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
		HomeMap         string `json:"homeMap"`
		HomeX           int    `json:"homeX"`
		HomeY           int    `json:"homeY"`
		GroupMembersMax int    `json:"groupMembersMax"`
		BuildGuild      int    `json:"buildGuild"`
		GuildWarFee     int    `json:"guildWarFee"`
	} `json:"game"`
	Commands struct {
		Names       map[string]string `json:"names"`
		Permissions map[string]int    `json:"permissions"`
	} `json:"commands"`
	Plugins struct {
		Enabled map[string]bool `json:"enabled"`
	} `json:"plugins"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Database: struct {
			Path string `json:"path"`
		}{
			Path: "serverdata/mir2.db",
		},
	}
}

// LoadConfig loads configuration from serverconfig directory.
func LoadConfig(configDir string) (*ServerConfig, error) {
	configFile := filepath.Join(configDir, "server.jsonc")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Remove JSONC comments (lines starting with //)
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

// GetListenAddr returns the listen address.
func (c *ServerConfig) GetListenAddr() string {
	if c.Server.Listen.Addr != "" {
		return c.Server.Listen.Addr
	}
	return ":7000"
}

// GetDatabasePath returns the database path.
func (c *ServerConfig) GetDatabasePath() string {
	if c.Database.Path != "" {
		return c.Database.Path
	}
	return "serverdata/mir2.db"
}

// GetHomeMap returns the home map name.
func (c *ServerConfig) GetHomeMap() string {
	if c.Game.HomeMap != "" {
		return c.Game.HomeMap
	}
	return "0"
}

// GetHomeX returns the home X coordinate.
func (c *ServerConfig) GetHomeX() int {
	if c.Game.HomeX > 0 {
		return c.Game.HomeX
	}
	return 289
}

// GetHomeY returns the home Y coordinate.
func (c *ServerConfig) GetHomeY() int {
	if c.Game.HomeY > 0 {
		return c.Game.HomeY
	}
	return 618
}

// GetServerHostPort returns the server address as host/port pair.
// For listen address ":7000", returns ("localhost", 7000).
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
