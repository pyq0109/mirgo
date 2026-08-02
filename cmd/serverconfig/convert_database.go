package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// StdItem 表示数据库中的物品定义。
// Expands 对应 Expand1–Expand5，Elements 对应 Element1–Element24。
type StdItem struct {
	Idx               int    `json:"idx"`
	Name              string `json:"name"`
	StdMode           int    `json:"stdMode"`
	Shape             int    `json:"shape"`
	Weight            int    `json:"weight"`
	AniCount          int    `json:"aniCount"`
	Source            int    `json:"source"`
	Reserved          int    `json:"reserved"`
	Looks             int    `json:"looks"`
	DuraMax           int    `json:"duraMax"`
	AC                int    `json:"ac"`
	ACMax             int    `json:"acMax"`
	MAC               int    `json:"mac"`
	MACMax            int    `json:"macMax"`
	DC                int    `json:"dc"`
	DCMax             int    `json:"dcMax"`
	MC                int    `json:"mc"`
	MCMax             int    `json:"mcMax"`
	SC                int    `json:"sc"`
	SCMax             int    `json:"scMax"`
	Need              int    `json:"need"`
	NeedLevel         int    `json:"needLevel"`
	Price             int    `json:"price"`
	Stock             int    `json:"stock"`
	Color             int    `json:"color"`
	OverLap           int    `json:"overLap"`
	HP                int    `json:"hp"`
	MP                int    `json:"mp"`
	Light             int    `json:"light"`
	Horse             int    `json:"horse"`
	Element           int    `json:"element"`
	Expands           []int  `json:"expands"`
	InsuranceCurrency int    `json:"insuranceCurrency"`
	InsuranceGold     int    `json:"insuranceGold"`
	Elements          []int  `json:"elements"`
}

// MonsterDef 表示数据库中的怪物定义。
type MonsterDef struct {
	Name               string `json:"name"`
	Race               int    `json:"race"`
	RaceImg            int    `json:"raceImg"`
	Appr               int    `json:"appr"`
	Lvl                int    `json:"lvl"`
	Undead             int    `json:"undead"`
	CoolEye            int    `json:"coolEye"`
	Exp                int    `json:"exp"`
	HP                 int    `json:"hp"`
	MP                 int    `json:"mp"`
	AC                 int    `json:"ac"`
	MAC                int    `json:"mac"`
	DC                 int    `json:"dc"`
	DCMax              int    `json:"dcMax"`
	MC                 int    `json:"mc"`
	SC                 int    `json:"sc"`
	Speed              int    `json:"speed"`
	Hit                int    `json:"hit"`
	WalkSpeed          int    `json:"walkSpeed"`
	WalkStep           int    `json:"walkStep"`
	WalkWait           int    `json:"walkWait"`
	AttackSpd          int    `json:"attackSpd"`
	AttackState        int    `json:"attackState"`
	ExploreItem        int    `json:"exploreItem"`
	AttackSource       int    `json:"attackSource"`
	DisableSimpleActor int    `json:"disableSimpleActor"`
}

// MagicDef 表示数据库中的魔法定义。
type MagicDef struct {
	MagID       int    `json:"magId"`
	MagName     string `json:"magName"`
	EffectType  int    `json:"effectType"`
	Effect      int    `json:"effect"`
	Spell       int    `json:"spell"`
	Power       int    `json:"power"`
	MaxPower    int    `json:"maxPower"`
	DefSpell    int    `json:"defSpell"`
	DefPower    int    `json:"defPower"`
	DefMaxPower int    `json:"defMaxPower"`
	Job         int    `json:"job"`
	NeedL1      int    `json:"needL1"`
	L1Train     int    `json:"l1Train"`
	NeedL2      int    `json:"needL2"`
	L2Train     int    `json:"l2Train"`
	NeedL3      int    `json:"needL3"`
	L3Train     int    `json:"l3Train"`
	Delay       int    `json:"delay"`
	Descr       string `json:"descr"`
	NeedL4      int    `json:"needL4"`
	L4Train     int    `json:"l4Train"`
	NeedL5      int    `json:"needL5"`
	L5Train     int    `json:"l5Train"`
	NeedL6      int    `json:"needL6"`
	L6Train     int    `json:"l6Train"`
	NeedL7      int    `json:"needL7"`
	L7Train     int    `json:"l7Train"`
	NeedL8      int    `json:"needL8"`
	L8Train     int    `json:"l8Train"`
	NeedL9      int    `json:"needL9"`
	L9Train     int    `json:"l9Train"`
	NeedL10     int    `json:"needL10"`
	L10Train    int    `json:"l10Train"`
	NeedL11     int    `json:"needL11"`
	L11Train    int    `json:"l11Train"`
	NeedL12     int    `json:"needL12"`
	L12Train    int    `json:"l12Train"`
	NeedL13     int    `json:"needL13"`
	L13Train    int    `json:"l13Train"`
	NeedL14     int    `json:"needL14"`
	L14Train    int    `json:"l14Train"`
	NeedL15     int    `json:"needL15"`
	L15Train    int    `json:"l15Train"`
	MaxTrainLv  int    `json:"maxTrainLv"`
	CanUpgrade  int    `json:"canUpgrade"`
	MaxUpgradeLv int   `json:"maxUpgradeLv"`
}

// ConvertDatabase 将 SQLite 数据库转换为 JSONC 文件。
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

	// 转换 StdItems
	if err := convertStdItems(db, outputDir); err != nil {
		return fmt.Errorf("converting StdItems: %w", err)
	}

	// 转换 Monster
	if err := convertMonster(db, outputDir); err != nil {
		return fmt.Errorf("converting Monster: %w", err)
	}

	// 转换 Magic
	if err := convertMagic(db, outputDir); err != nil {
		return fmt.Errorf("converting Magic: %w", err)
	}

	return nil
}

func convertStdItems(db *sql.DB, outputDir string) error {
	query := `SELECT Idx, COALESCE(Name,''), Stdmode, Shape, Weight,
		COALESCE(Anicount,0), COALESCE(Source,0), COALESCE(Reserved,0),
		Looks, DuraMax, Ac, Ac2, Mac, Mac2, Dc, Dc2, Mc, Mc2, Sc, Sc2,
		Need, NeedLevel, Price,
		COALESCE(Stock,0), COALESCE(Color,0), COALESCE(OverLap,0),
		COALESCE(HP,0), COALESCE(MP,0), COALESCE(Light,0), COALESCE(Horse,0),
		COALESCE(Element,0),
		COALESCE(Expand1,0), COALESCE(Expand2,0), COALESCE(Expand3,0), COALESCE(Expand4,0), COALESCE(Expand5,0),
		COALESCE(InsuranceCurrency,0), COALESCE(InsuranceGold,0),
		COALESCE(Element1,0), COALESCE(Element2,0), COALESCE(Element3,0), COALESCE(Element4,0),
		COALESCE(Element5,0), COALESCE(Element6,0), COALESCE(Element7,0), COALESCE(Element8,0),
		COALESCE(Element9,0), COALESCE(Element10,0), COALESCE(Element11,0), COALESCE(Element12,0),
		COALESCE(Element13,0), COALESCE(Element14,0), COALESCE(Element15,0), COALESCE(Element16,0),
		COALESCE(Element17,0), COALESCE(Element18,0), COALESCE(Element19,0), COALESCE(Element20,0),
		COALESCE(Element21,0), COALESCE(Element22,0), COALESCE(Element23,0), COALESCE(CAST(Element24 AS INTEGER),0)
		FROM StdItems`
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []StdItem
	for rows.Next() {
		var item StdItem
		var expands [5]int
		var elements [24]int
		args := []interface{}{
			&item.Idx, &item.Name, &item.StdMode, &item.Shape, &item.Weight,
			&item.AniCount, &item.Source, &item.Reserved,
			&item.Looks, &item.DuraMax, &item.AC, &item.ACMax, &item.MAC, &item.MACMax,
			&item.DC, &item.DCMax, &item.MC, &item.MCMax, &item.SC, &item.SCMax,
			&item.Need, &item.NeedLevel, &item.Price,
			&item.Stock, &item.Color, &item.OverLap, &item.HP, &item.MP, &item.Light, &item.Horse, &item.Element,
		}
		for i := 0; i < 5; i++ {
			args = append(args, &expands[i])
		}
		args = append(args, &item.InsuranceCurrency, &item.InsuranceGold)
		for i := 0; i < 24; i++ {
			args = append(args, &elements[i])
		}
		if err := rows.Scan(args...); err != nil {
			return err
		}
		item.Expands = expands[:]
		item.Elements = elements[:]
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
	query := `SELECT Name, Race, RaceImg, Appr, Lvl, Undead, COALESCE(CoolEye,0), Exp, HP, COALESCE(MP,0),
		AC, MAC, DC, DCMAX, MC, SC, SPEED, HIT,
		COALESCE(WALK_SPD,0), COALESCE(WalkStep,0), COALESCE(WalkWait,0), COALESCE(ATTACK_SPD,0),
		COALESCE(AttackState,0), COALESCE(ExploreItem,0), COALESCE(AttackSource,0), COALESCE(DisableSimpleActor,0)
		FROM Monster`
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var monsters []MonsterDef
	for rows.Next() {
		var m MonsterDef
		err := rows.Scan(
			&m.Name, &m.Race, &m.RaceImg, &m.Appr, &m.Lvl, &m.Undead, &m.CoolEye, &m.Exp, &m.HP, &m.MP,
			&m.AC, &m.MAC, &m.DC, &m.DCMax, &m.MC, &m.SC, &m.Speed, &m.Hit,
			&m.WalkSpeed, &m.WalkStep, &m.WalkWait, &m.AttackSpd,
			&m.AttackState, &m.ExploreItem, &m.AttackSource, &m.DisableSimpleActor,
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
	query := `SELECT MagID, MagName, EffectType, Effect, Spell, Power, MaxPower,
		DefSpell, DefPower, DefMaxPower, Job,
		NeedL1, L1Train, NeedL2, L2Train, NeedL3, L3Train, Delay, COALESCE(Descr,''),
		NeedL4, L4Train, NeedL5, L5Train, NeedL6, L6Train, NeedL7, L7Train,
		NeedL8, L8Train, NeedL9, L9Train, NeedL10, L10Train, NeedL11, L11Train,
		NeedL12, L12Train, NeedL13, L13Train, NeedL14, L14Train, NeedL15, L15Train,
		MaxTrainLv, CanUpgrade, MaxUpgradeLv
		FROM Magic`
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var magics []MagicDef
	for rows.Next() {
		var m MagicDef
		err := rows.Scan(
			&m.MagID, &m.MagName, &m.EffectType, &m.Effect, &m.Spell, &m.Power, &m.MaxPower,
			&m.DefSpell, &m.DefPower, &m.DefMaxPower, &m.Job,
			&m.NeedL1, &m.L1Train, &m.NeedL2, &m.L2Train, &m.NeedL3, &m.L3Train, &m.Delay, &m.Descr,
			&m.NeedL4, &m.L4Train, &m.NeedL5, &m.L5Train, &m.NeedL6, &m.L6Train, &m.NeedL7, &m.L7Train,
			&m.NeedL8, &m.L8Train, &m.NeedL9, &m.L9Train, &m.NeedL10, &m.L10Train, &m.NeedL11, &m.L11Train,
			&m.NeedL12, &m.L12Train, &m.NeedL13, &m.L13Train, &m.NeedL14, &m.L14Train, &m.NeedL15, &m.L15Train,
			&m.MaxTrainLv, &m.CanUpgrade, &m.MaxUpgradeLv,
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
