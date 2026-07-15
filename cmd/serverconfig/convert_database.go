package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// StdItem represents an item definition from the database.
type StdItem struct {
	Idx       int    `json:"idx"`
	Name      string `json:"name"`
	StdMode   int    `json:"stdMode"`
	Shape     int    `json:"shape"`
	Weight    int    `json:"weight"`
	Looks     int    `json:"looks"`
	DuraMax   int    `json:"duraMax"`
	AC        int    `json:"ac"`
	ACMax     int    `json:"acMax"`
	MAC       int    `json:"mac"`
	MACMax    int    `json:"macMax"`
	DC        int    `json:"dc"`
	DCMax     int    `json:"dcMax"`
	MC        int    `json:"mc"`
	MCMax     int    `json:"mcMax"`
	SC        int    `json:"sc"`
	SCMax     int    `json:"scMax"`
	Need      int    `json:"need"`
	NeedLevel int    `json:"needLevel"`
	Price     int    `json:"price"`
}

// MonsterDef represents a monster definition from the database.
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

// MagicDef represents a magic definition from the database.
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
	L1Train    int    `json:"l1Train"`
	NeedL2     int    `json:"needL2"`
	L2Train    int    `json:"l2Train"`
	NeedL3     int    `json:"needL3"`
	L3Train    int    `json:"l3Train"`
	Delay      int    `json:"delay"`
}

// ConvertDatabase converts the SQLite database to JSONC files.
func ConvertDatabase(inputDir, outputDir string) error {
	dbFile := filepath.Join(inputDir, "数据库", "GEEM2.db")

	if !FileExists(dbFile) {
		return fmt.Errorf("database file not found: %s", dbFile)
	}

	db, err := ParseSQLite(dbFile)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Convert StdItems
	if err := convertStdItems(db, outputDir); err != nil {
		return fmt.Errorf("converting StdItems: %w", err)
	}

	// Convert Monster
	if err := convertMonster(db, outputDir); err != nil {
		return fmt.Errorf("converting Monster: %w", err)
	}

	// Convert Magic
	if err := convertMagic(db, outputDir); err != nil {
		return fmt.Errorf("converting Magic: %w", err)
	}

	return nil
}

func convertStdItems(db *sql.DB, outputDir string) error {
	rows, err := db.Query("SELECT Idx, Name, Stdmode, Shape, Weight, Looks, DuraMax, Ac, Ac2, Mac, Mac2, Dc, Dc2, Mc, Mc2, Sc, Sc2, Need, NeedLevel, Price FROM StdItems")
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []StdItem
	for rows.Next() {
		var item StdItem
		err := rows.Scan(
			&item.Idx, &item.Name, &item.StdMode, &item.Shape, &item.Weight,
			&item.Looks, &item.DuraMax, &item.AC, &item.ACMax, &item.MAC, &item.MACMax,
			&item.DC, &item.DCMax, &item.MC, &item.MCMax, &item.SC, &item.SCMax,
			&item.Need, &item.NeedLevel, &item.Price,
		)
		if err != nil {
			return err
		}
		items = append(items, item)
	}

	result := map[string]interface{}{
		"_source":      "asset/server/数据库/GEEM2.db",
		"_description": "所有物品的基础属性模板",
		"items":        items,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "std_items.jsonc")
	comment := fmt.Sprintf("物品定义表\n来源: asset/server/数据库/GEEM2.db → StdItems 表\n数量: %d 个物品", len(items))

	return WriteJSONC(outputFile, string(data), comment)
}

func convertMonster(db *sql.DB, outputDir string) error {
	rows, err := db.Query("SELECT Name, Race, RaceImg, Appr, Lvl, Undead, Exp, HP, AC, MAC, DC, DCMAX, MC, SC, Speed, Hit FROM Monster")
	if err != nil {
		return err
	}
	defer rows.Close()

	var monsters []MonsterDef
	for rows.Next() {
		var m MonsterDef
		err := rows.Scan(
			&m.Name, &m.Race, &m.RaceImg, &m.Appr, &m.Lvl, &m.Undead,
			&m.Exp, &m.HP, &m.AC, &m.MAC, &m.DC, &m.DCMax,
			&m.MC, &m.SC, &m.Speed, &m.Hit,
		)
		if err != nil {
			return err
		}
		monsters = append(monsters, m)
	}

	result := map[string]interface{}{
		"_source":      "asset/server/数据库/GEEM2.db",
		"_description": "所有怪物的基础属性模板",
		"monsters":     monsters,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "monsters", "monster_db.jsonc")
	comment := fmt.Sprintf("怪物定义表\n来源: asset/server/数据库/GEEM2.db → Monster 表\n数量: %d 个怪物", len(monsters))

	return WriteJSONC(outputFile, string(data), comment)
}

func convertMagic(db *sql.DB, outputDir string) error {
	rows, err := db.Query("SELECT MagID, MagName, EffectType, Effect, Spell, Power, MaxPower, Job, NeedL1, L1Train, NeedL2, L2Train, NeedL3, L3Train, Delay FROM Magic")
	if err != nil {
		return err
	}
	defer rows.Close()

	var magics []MagicDef
	for rows.Next() {
		var m MagicDef
		err := rows.Scan(
			&m.MagID, &m.MagName, &m.EffectType, &m.Effect, &m.Spell,
			&m.Power, &m.MaxPower, &m.Job, &m.NeedL1, &m.L1Train,
			&m.NeedL2, &m.L2Train, &m.NeedL3, &m.L3Train, &m.Delay,
		)
		if err != nil {
			return err
		}
		magics = append(magics, m)
	}

	result := map[string]interface{}{
		"_source":      "asset/server/数据库/GEEM2.db",
		"_description": "所有魔法的基础属性模板",
		"magic":        magics,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "magic", "magic_db.jsonc")
	comment := fmt.Sprintf("魔法定义表\n来源: asset/server/数据库/GEEM2.db → Magic 表\n数量: %d 个魔法", len(magics))

	return WriteJSONC(outputFile, string(data), comment)
}
