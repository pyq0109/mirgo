package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
)

// ExpTable represents the experience table configuration.
type ExpTable struct {
	HighLevel         int            `json:"highLevel"`
	KillMonExpMultiple int           `json:"killMonExpMultiple"`
	BaseExp           int64          `json:"baseExp"`
	Levels            map[string]int64 `json:"levels"`
}

// StringsConfig represents the server strings configuration.
type StringsConfig struct {
	Messages map[string]string `json:"messages"`
}

// GlobalVars represents the global variables configuration.
type GlobalVars struct {
	Variables map[string]int `json:"variables"`
}

// ConvertExpTable converts the experience table configuration.
func ConvertExpTable(inputDir, outputDir string) error {
	expFile := filepath.Join(inputDir, "Exps.ini")
	expINI, err := ParseINI(expFile)
	if err != nil {
		return fmt.Errorf("parsing Exps.ini: %w", err)
	}

	expTable := ExpTable{
		HighLevel:         getINIInt(expINI, "Exp", "HighLevel", 1000),
		KillMonExpMultiple: getINIInt(expINI, "Exp", "KillMonExpMultiple", 1),
		Levels:            make(map[string]int64),
	}

	// Parse base exp
	if baseExp, ok := expINI["Exp"]["BaseExp"]; ok {
		if n, err := strconv.ParseInt(baseExp, 10, 64); err == nil {
			expTable.BaseExp = n
		}
	}

	// Parse level exp values
	if expSection, ok := expINI["Exp"]; ok {
		for k, v := range expSection {
			if len(k) > 5 && k[:5] == "Level" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					expTable.Levels[k] = n
				}
			}
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(expTable, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling exp table: %w", err)
	}

	// Write output
	outputFile := filepath.Join(outputDir, "exp_table.jsonc")
	comment := "经验值表\n来源: asset/server/Exps.ini\n说明: 各等级所需经验值"

	return WriteJSONC(outputFile, string(data), comment)
}

// ConvertStrings converts the server strings configuration.
func ConvertStrings(inputDir, outputDir string) error {
	strFile := filepath.Join(inputDir, "String.ini")
	strINI, err := ParseINI(strFile)
	if err != nil {
		return fmt.Errorf("parsing String.ini: %w", err)
	}

	config := StringsConfig{
		Messages: make(map[string]string),
	}

	// Get all messages from [String] section
	if strSection, ok := strINI["String"]; ok {
		for k, v := range strSection {
			config.Messages[k] = v
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling strings: %w", err)
	}

	// Write output
	outputFile := filepath.Join(outputDir, "strings.jsonc")
	comment := "服务端提示文本\n来源: asset/server/String.ini\n说明: 游戏内提示消息（约210条）"

	return WriteJSONC(outputFile, string(data), comment)
}

// ConvertGlobalVars converts the global variables configuration.
func ConvertGlobalVars(inputDir, outputDir string) error {
	gvFile := filepath.Join(inputDir, "GlobalVal.ini")
	gvINI, err := ParseINI(gvFile)
	if err != nil {
		return fmt.Errorf("parsing GlobalVal.ini: %w", err)
	}

	config := GlobalVars{
		Variables: make(map[string]int),
	}

	// Get all variables from any section
	for _, section := range gvINI {
		for k, v := range section {
			if n, err := strconv.Atoi(v); err == nil {
				config.Variables[k] = n
			}
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling global vars: %w", err)
	}

	// Write output
	outputFile := filepath.Join(outputDir, "global_vars.jsonc")
	comment := "全局变量\n来源: asset/server/GlobalVal.ini\n说明: 2000个全局变量（GlobalVal0~GlobalVal1999）"

	return WriteJSONC(outputFile, string(data), comment)
}
