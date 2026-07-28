package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type MagicDef struct {
	MagID      int    `json:"magId"`
	MagName    string `json:"magName"`
	EffectType int    `json:"effectType"`
	Effect     int    `json:"effect"`
	Spell      int    `json:"spell"`
	Power      int    `json:"power"`
	MaxPower   int    `json:"maxPower"`
	Job        int    `json:"job"`
	NeedL1     int    `json:"needL1"`
	Delay      int    `json:"delay"`
}

type MagicDB struct {
	Magics []MagicDef
	byID   map[int]*MagicDef
}

func LoadMagicDB(path string) (*MagicDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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

	var raw struct {
		Magics []MagicDef `json:"magic"`
	}
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &raw); err != nil {
		return nil, err
	}

	db := &MagicDB{
		Magics: raw.Magics,
		byID:   make(map[int]*MagicDef),
	}
	for i := range db.Magics {
		magic := &db.Magics[i]
		db.byID[magic.MagID] = magic
	}
	log.Logf(log.LevelInfo, "MagicDB", "从 %s 加载了 %d 个魔法", path, len(db.Magics))
	return db, nil
}

func (db *MagicDB) GetByID(id int) *MagicDef {
	return db.byID[id]
}
