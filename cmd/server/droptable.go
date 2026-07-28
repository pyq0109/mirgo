package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type DropEntry struct {
	ItemName string
	Chance   int
	Count    int
	Gold     int
}

type DropTable struct {
	tables map[string][]DropEntry
}

type dropFileItem struct {
	Prob  string `json:"prob"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type dropFile struct {
	Items   []dropFileItem `json:"items"`
	Monster string         `json:"monster"`
}

func LoadDropTables(dir string) *DropTable {
	dt := &DropTable{
		tables: make(map[string][]DropEntry),
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Logf(log.LevelWarn, "DropTable", "directory not found: %s, using default drop tables", dir)
		dt.loadDefaults()
		return dt
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonc") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := dt.loadFile(path); err != nil {
			log.Logf(log.LevelWarn, "DropTable", "failed to load %s: %v", path, err)
			continue
		}
		count++
	}

	if count == 0 {
		log.Logf(log.LevelWarn, "DropTable", "no drop files found in %s, using default drop tables", dir)
		dt.loadDefaults()
		return dt
	}

	log.Logf(log.LevelInfo, "DropTable", "loaded %d drop tables from %s", count, dir)
	return dt
}

func (dt *DropTable) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var clean []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		clean = append(clean, line)
	}

	var df dropFile
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &df); err != nil {
		return err
	}

	if df.Monster == "" {
		return fmt.Errorf("missing monster field")
	}

	var drops []DropEntry
	for _, item := range df.Items {
		chance := parseProb(item.Prob)
		if chance <= 0 {
			continue
		}
		count := item.Count
		if count <= 0 {
			count = 1
		}
		entry := DropEntry{
			ItemName: item.Name,
			Chance:   chance,
			Count:    count,
		}
		if item.Name == "金币" {
			entry.Gold = count
		}
		drops = append(drops, entry)
	}

	dt.tables[df.Monster] = drops
	return nil
}

func parseProb(prob string) int {
	parts := strings.Split(prob, "/")
	if len(parts) != 2 {
		return 0
	}
	var n int
	fmt.Sscanf(parts[1], "%d", &n)
	return n
}

func (dt *DropTable) loadDefaults() {
	defaultDrops := []DropEntry{
		{ItemName: "金币", Chance: 3, Count: 30, Gold: 30},
		{ItemName: "金创药(小量)", Chance: 10, Count: 1},
	}
	dt.tables["*"] = defaultDrops
}

func (dt *DropTable) GetDrops(monsterName string) []DropEntry {
	if drops, ok := dt.tables[monsterName]; ok {
		return drops
	}
	if drops, ok := dt.tables["*"]; ok {
		return drops
	}
	return nil
}
