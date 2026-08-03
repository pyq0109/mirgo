package main

import (
	"os"

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
	byName map[string]*MagicDef
}

func LoadMagicDB(path string) (*MagicDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Magics []MagicDef `json:"magic"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		return nil, err
	}

	db := &MagicDB{
		Magics: raw.Magics,
		byID:   make(map[int]*MagicDef),
		byName: make(map[string]*MagicDef),
	}
	for i := range db.Magics {
		magic := &db.Magics[i]
		db.byID[magic.MagID] = magic
		db.byName[magic.MagName] = magic
	}
	log.Logf(log.LevelInfo, "MagicDB", "loaded %d magics from %s", len(db.Magics), path)
	return db, nil
}

func (db *MagicDB) GetByID(id int) *MagicDef {
	return db.byID[id]
}

func (db *MagicDB) GetByName(name string) *MagicDef {
	return db.byName[name]
}
