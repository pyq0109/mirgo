package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type MonsterDef struct {
	Name    string `json:"name"`
	Race    int    `json:"race"`
	RaceImg int    `json:"raceImg"`
	Appr    int    `json:"appr"`
	Lvl     int    `json:"lvl"`
	Undead  int    `json:"undead"`
	Exp     int    `json:"exp"`
	HP      int    `json:"hp"`
	AC      int    `json:"ac"`
	MAC     int    `json:"mac"`
	DC      int    `json:"dc"`
	DCMax   int    `json:"dcMax"`
	MC      int    `json:"mc"`
	SC      int    `json:"sc"`
	Speed   int    `json:"speed"`
	Hit     int    `json:"hit"`
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
		Monsters []MonsterDef `json:"monsters"`
	}
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &raw); err != nil {
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
	log.Logf(log.LevelInfo, "MonsterDB", "Loaded %d monsters from %s", len(db.Monsters), path)
	return db, nil
}

func (db *MonsterDB) GetByName(name string) *MonsterDef {
	return db.byName[name]
}
