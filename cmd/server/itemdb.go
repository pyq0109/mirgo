package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
)

type ItemDef struct {
	Idx       int    `json:"idx"`
	Name      string `json:"name"`
	StdMode   byte   `json:"stdMode"`
	Shape     byte   `json:"shape"`
	Weight    byte   `json:"weight"`
	Looks     uint16 `json:"looks"`
	DuraMax   uint32 `json:"duraMax"`
	AC        uint32 `json:"ac"`
	ACMax     uint32 `json:"acMax"`
	MAC       uint32 `json:"mac"`
	MACMax    uint32 `json:"macMax"`
	DC        uint32 `json:"dc"`
	DCMax     uint32 `json:"dcMax"`
	MC        uint32 `json:"mc"`
	MCMax     uint32 `json:"mcMax"`
	SC        uint32 `json:"sc"`
	SCMax     uint32 `json:"scMax"`
	Need      byte   `json:"need"`
	NeedLevel byte   `json:"needLevel"`
	Price     uint32 `json:"price"`
	Source    int16  `json:"source"`
	AniCount  byte   `json:"aniCount"`
}

type ItemDB struct {
	Items  []ItemDef
	byName map[string]*ItemDef
	byIdx  map[int]*ItemDef
}

func LoadItemDB(path string) (*ItemDB, error) {
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
		Items []ItemDef `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.Join(clean, "\n")), &raw); err != nil {
		return nil, err
	}

	db := &ItemDB{
		Items:  raw.Items,
		byName: make(map[string]*ItemDef),
		byIdx:  make(map[int]*ItemDef),
	}
	for i := range db.Items {
		item := &db.Items[i]
		db.byName[item.Name] = item
		db.byIdx[item.Idx] = item
	}
	log.Logf(log.LevelInfo, "ItemDB", "从 %s 加载了 %d 个物品", path, len(db.Items))
	return db, nil
}

func (db *ItemDB) GetByName(name string) *ItemDef {
	return db.byName[name]
}

func (db *ItemDB) GetByIdx(idx int) *ItemDef {
	return db.byIdx[idx]
}
