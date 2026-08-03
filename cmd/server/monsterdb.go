package main

import (
	"os"

	"github.com/pyq0109/mirgo/internal/log"
)

type MonsterDef struct {
	Name      string `json:"name"`
	Race      int    `json:"race"`
	RaceImg   int    `json:"raceImg"`
	Appr      int    `json:"appr"`
	Lvl       int    `json:"lvl"`
	Undead    int    `json:"undead"`
	Exp       int    `json:"exp"`
	HP        int    `json:"hp"`
	AC        int    `json:"ac"`
	MAC       int    `json:"mac"`
	DC        int    `json:"dc"`
	DCMax     int    `json:"dcMax"`
	MC        int    `json:"mc"`
	SC        int    `json:"sc"`
	Speed     int    `json:"speed"`
	Hit       int    `json:"hit"`
	ViewRange int    `json:"viewRange"`
	CoolEye   int    `json:"coolEye"`
	WalkSpeed int    `json:"walkSpeed"`
	WalkStep  int    `json:"walkStep"`
	WalkWait  int    `json:"walkWait"`
	Slave     string `json:"slave"`
	MagID     int    `json:"magId"`
}

type MonsterDB struct {
	Monsters []MonsterDef
	byName   map[string]*MonsterDef
}

func LoadMonsterDB(path string) (*MonsterDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Monsters []MonsterDef `json:"monsters"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		return nil, err
	}

	db := &MonsterDB{
		Monsters: raw.Monsters,
		byName:   make(map[string]*MonsterDef),
	}
	for i := range db.Monsters {
		mon := &db.Monsters[i]
		db.byName[mon.Name] = mon
	}
	log.Logf(log.LevelInfo, "MonsterDB", "loaded %d monsters from %s", len(db.Monsters), path)
	return db, nil
}

func (db *MonsterDB) GetByName(name string) *MonsterDef {
	return db.byName[name]
}
