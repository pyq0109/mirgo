package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// ServerConfig represents the main server configuration.
type ServerConfig struct {
	Server   ServerSection   `json:"server"`
	Database DatabaseSection `json:"database"`
	Game     GameSection     `json:"game"`
	Commands CommandsSection `json:"commands"`
	Plugins  PluginsSection  `json:"plugins"`
}

// ServerSection contains server identity and network settings.
type ServerSection struct {
	Name      string        `json:"name"`
	Index     int           `json:"index"`
	Listen    ListenConfig  `json:"listen"`
	Limits    LimitsConfig  `json:"limits"`
}

// ListenConfig contains network listen settings.
type ListenConfig struct {
	Addr string `json:"addr"`
}

// LimitsConfig contains server capacity limits.
type LimitsConfig struct {
	MaxPlayers int `json:"maxPlayers"`
	HumLimit   int `json:"humLimit"`
	MonLimit   int `json:"monLimit"`
}

// DatabaseSection contains database file paths.
type DatabaseSection struct {
	Accounts   string `json:"accounts"`
	Characters string `json:"characters"`
	Guilds     string `json:"guilds"`
}

// GameSection contains game world settings.
type GameSection struct {
	HomeMap          string `json:"homeMap"`
	HomeX            int    `json:"homeX"`
	HomeY            int    `json:"homeY"`
	GroupMembersMax  int    `json:"groupMembersMax"`
	BuildGuild       int    `json:"buildGuild"`
	GuildWarFee      int    `json:"guildWarFee"`
	// Additional game settings can be added here
}

// CommandsSection contains GM command definitions.
type CommandsSection struct {
	Names       map[string]string `json:"names"`
	Permissions map[string]int    `json:"permissions"`
}

// PluginsSection contains plugin toggles.
type PluginsSection struct {
	Enabled map[string]bool `json:"enabled"`
}

// ConvertServer converts the main server configuration files.
func ConvertServer(inputDir, outputDir string) error {
	// Parse !setup.txt
	setupFile := filepath.Join(inputDir, "!setup.txt")
	setup, err := ParseINI(setupFile)
	if err != nil {
		return fmt.Errorf("parsing !setup.txt: %w", err)
	}

	// Parse Command.ini
	cmdFile := filepath.Join(inputDir, "Command.ini")
	commands, err := ParseINI(cmdFile)
	if err != nil {
		return fmt.Errorf("parsing Command.ini: %w", err)
	}

	// Parse 系统插件.ini
	pluginFile := filepath.Join(inputDir, "系统插件.ini")
	plugins, err := ParseINI(pluginFile)
	if err != nil {
		return fmt.Errorf("parsing 系统插件.ini: %w", err)
	}

	// Build server config
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
			Accounts:   "serverdata/accounts.db",
			Characters: "serverdata/characters.db",
			Guilds:     "serverdata/guilds.db",
		},
		Game: GameSection{
			HomeMap:         getINIValue(setup, "Setup", "HomeMap", "0"),
			HomeX:           getINIInt(setup, "Setup", "HomeX", 289),
			HomeY:           getINIInt(setup, "Setup", "HomeY", 618),
			GroupMembersMax: getINIInt(setup, "Setup", "GroupMembersMax", 10),
			BuildGuild:      getINIInt(setup, "Setup", "BuildGuild", 1000000),
			GuildWarFee:     getINIInt(setup, "Setup", "GuildWarFee", 30000),
		},
		Commands: CommandsSection{
			Names:       getINISection(commands, "Command"),
			Permissions: getINISectionInt(commands, "Permission"),
		},
		Plugins: PluginsSection{
			Enabled: getINISectionBool(plugins, "Plugins"),
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling server config: %w", err)
	}

	// Write output
	outputFile := filepath.Join(outputDir, "server.jsonc")
	comment := "服务器主配置\n来源: asset/server/!setup.txt + Command.ini + 系统插件.ini\n说明: 合并后的服务端配置，移除了多进程地址配置"

	return WriteJSONC(outputFile, string(data), comment)
}

// Helper functions

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
