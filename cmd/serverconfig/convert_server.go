package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// ServerConfig 表示服务器主配置。
type ServerConfig struct {
	Server   ServerSection   `json:"server"`
	Database DatabaseSection `json:"database"`
	Game     GameSection     `json:"game"`
	Commands CommandsSection `json:"commands"`
	Plugins  PluginsSection  `json:"plugins"`
}

// ServerSection 包含服务器标识和网络设置。
type ServerSection struct {
	Name      string        `json:"name"`
	Index     int           `json:"index"`
	Listen    ListenConfig  `json:"listen"`
	Limits    LimitsConfig  `json:"limits"`
}

// ListenConfig 包含网络监听设置。
type ListenConfig struct {
	Addr string `json:"addr"`
}

// LimitsConfig 包含服务器容量限制。
type LimitsConfig struct {
	MaxPlayers int `json:"maxPlayers"`
	HumLimit   int `json:"humLimit"`
	MonLimit   int `json:"monLimit"`
}

// DatabaseSection 包含数据库文件路径。
type DatabaseSection struct {
	Path string `json:"path"`
}

// GameSection 包含游戏世界设置。
type GameSection struct {
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
}

// CommandsSection 包含 GM 命令定义。
type CommandsSection struct {
	Names       map[string]string `json:"names"`
	Permissions map[string]int    `json:"permissions"`
}

// PluginsSection 包含插件开关。
type PluginsSection struct {
	Enabled map[string]bool `json:"enabled"`
}

// ConvertServer 转换服务器主配置文件。
func ConvertServer(inputDir, outputDir string) error {
	// 解析 !setup.txt
	setupFile := filepath.Join(inputDir, "!setup.txt")
	setup, err := ParseINI(setupFile)
	if err != nil {
		return fmt.Errorf("parsing !setup.txt: %w", err)
	}

	// 解析 Command.ini
	cmdFile := filepath.Join(inputDir, "Command.ini")
	commands, err := ParseINI(cmdFile)
	if err != nil {
		return fmt.Errorf("parsing Command.ini: %w", err)
	}

	// Parse 系统插件.ini
	pluginFile := filepath.Join(inputDir, "系统插件.ini")
	plugins, err := ParseINI(pluginFile)
	if err != nil {
		// 插件文件为可选项
		plugins = make(map[string]map[string]string)
	}

	// 构建服务器配置
	config := ServerConfig{
		Server: ServerSection{
			Name:  getINIValue(setup, "Server", "ServerName", "Mir2 Server"),
			Index: getINIInt(setup, "Server", "ServerIndex", 0),
			Listen: ListenConfig{
				Addr: "0.0.0.0:7000",
			},
			Limits: LimitsConfig{
				MaxPlayers: getINIInt(setup, "Server", "UserFull", 10000),
				HumLimit:   getINIInt(setup, "Server", "HumLimit", 30),
				MonLimit:   getINIInt(setup, "Server", "MonLimit", 30),
			},
		},
		Database: DatabaseSection{
			Path: "serverdata/mir2.db",
		},
		Game: GameSection{
			HomeMap:         getINIValue(setup, "Setup", "HomeMap", "0"),
			HomeX:           getINIInt(setup, "Setup", "HomeX", 289),
			HomeY:           getINIInt(setup, "Setup", "HomeY", 618),
			GroupMembersMax: getINIInt(setup, "Setup", "GroupMembersMax", 10),
			BuildGuild:      getINIInt(setup, "Setup", "BuildGuild", 1000000),
			GuildWarFee:     getINIInt(setup, "Setup", "GuildWarFee", 30000),
			DisableRun:      getINIBool(setup, "Setup", "DiableHumanRun", false),
			GMRunAll:        getINIBool(setup, "Setup", "GMRunAll", true),
			GMAccounts:      make(map[string]int),
			WalkInterval:    int64(getINIInt(setup, "Setup", "WalkIntervalTime", 600)),
			RunInterval:     int64(getINIInt(setup, "Setup", "RunIntervalTime", 600)),
			SpeedHackKick:   getINIBool(setup, "Setup", "KickOverSpeed", false),
			SpeedHackMax:    getINIInt(setup, "Setup", "OverSpeedKickCount", 4),
		},
		Commands: CommandsSection{
			Names:       getINISection(commands, "Command"),
			Permissions: getINISectionInt(commands, "Permission"),
		},
		Plugins: PluginsSection{
			Enabled: getINISectionBool(plugins, "Plugins"),
		},
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling server config: %w", err)
	}

	// 写入输出文件
	outputFile := filepath.Join(outputDir, "server.jsonc")
	comment := "服务器主配置\n来源: asset/server/!setup.txt + Command.ini + 系统插件.ini\n说明: 合并后的服务端配置，移除了多进程地址配置"

	return WriteJSONC(outputFile, string(data), comment)
}

// 辅助函数

func getINIValue(ini map[string]map[string]string, section, key, defaultVal string) string {
	if sec, ok := ini[section]; ok {
		if val, ok := sec[key]; ok {
			return val
		}
	}
	return defaultVal
}

func getINIInt(ini map[string]map[string]string, section, key string, defaultVal int) int {
	if sec, ok := ini[section]; ok {
		if val, ok := sec[key]; ok {
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
				return n
			}
		}
	}
	return defaultVal
}

func getINISection(ini map[string]map[string]string, section string) map[string]string {
	if sec, ok := ini[section]; ok {
		result := make(map[string]string)
		for k, v := range sec {
			result[k] = v
		}
		return result
	}
	return make(map[string]string)
}

func getINISectionInt(ini map[string]map[string]string, section string) map[string]int {
	if sec, ok := ini[section]; ok {
		result := make(map[string]int)
		for k, v := range sec {
			var n int
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
				result[k] = n
			}
		}
		return result
	}
	return make(map[string]int)
}

func getINIBool(ini map[string]map[string]string, section, key string, defaultVal bool) bool {
	if sec, ok := ini[section]; ok {
		if val, ok := sec[key]; ok {
			return val == "1" || val == "true" || val == "TRUE"
		}
	}
	return defaultVal
}

func getINISectionBool(ini map[string]map[string]string, section string) map[string]bool {
	if sec, ok := ini[section]; ok {
		result := make(map[string]bool)
		for k, v := range sec {
			result[k] = v == "1" || v == "true" || v == "TRUE"
		}
		return result
	}
	return make(map[string]bool)
}