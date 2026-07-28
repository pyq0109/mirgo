package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type NpcDef struct {
	Name    string `json:"name"`
	Race    int    `json:"race"`
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Face    int    `json:"face"`
	Body    int    `json:"body"`
}

type MerchantDef struct {
	ID      string `json:"id"`
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Name    string `json:"name"`
	Face    int    `json:"face"`
	Body    int    `json:"body"`
	Castle  int    `json:"castle"`
}

func LoadNpcList(path string) []NpcDef {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "NpcList", "failed to load %s: %v", path, err)
		return nil
	}

	clean := stripJSONCComments(string(data))
	var raw struct {
		Npcs []NpcDef `json:"npcs"`
	}
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		log.Logf(log.LevelWarn, "NpcList", "failed to parse %s: %v", path, err)
		return nil
	}

	log.Logf(log.LevelInfo, "NpcList", "loaded %d NPC definitions from %s", len(raw.Npcs), path)
	return raw.Npcs
}

func LoadMerchantList(path string) []MerchantDef {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "MerchantList", "failed to load %s: %v", path, err)
		return nil
	}

	clean := stripJSONCComments(string(data))
	var raw struct {
		Merchants []MerchantDef `json:"merchants"`
	}
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		log.Logf(log.LevelWarn, "MerchantList", "failed to parse %s: %v", path, err)
		return nil
	}

	log.Logf(log.LevelInfo, "MerchantList", "loaded %d merchant definitions from %s", len(raw.Merchants), path)
	return raw.Merchants
}

func stripJSONCComments(data string) string {
	lines := strings.Split(data, "\n")
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		clean = append(clean, line)
	}
	return strings.Join(clean, "\n")
}
