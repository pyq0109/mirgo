package main

import (
	"os"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/protocol"
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
	Need byte `json:"need"`
	// Delphi NeedLevel 为 DWord：分支 10-13/40-44/70/81/82 按
	// LoWord/HiWord 打包双条件（ObjBase.pas:23001-23260）。
	NeedLevel uint32 `json:"needLevel"`
	Price     uint32 `json:"price"`
	Source    int16  `json:"source"`
	AniCount  byte   `json:"aniCount"`
	Reserved  int    `json:"reserved"`
	OverLap   int    `json:"overLap"`
	HP        int    `json:"hp"`
	MP        int    `json:"mp"`
	Light     int    `json:"light"`
}

type ItemDB struct {
	Items  []ItemDef
	byName map[string]*ItemDef
	byIdx  map[int]*ItemDef

	// UnbindList 解包表（Shape→物品名，Envir/UnbindList.txt，
	// Delphi g_UnbindList，LocalDB.pas:1617-1649）。
	UnbindList map[int]string
	// DisableTakeOff 禁止脱下物品集合（0 基 DB 索引，
	// Envir/DisableTakeOffList.txt，M2Share.pas:4578-4660）。
	DisableTakeOff map[int]bool
}

func LoadItemDB(path string) (*ItemDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Items []ItemDef `json:"items"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		return nil, err
	}

	db := &ItemDB{
		Items:          raw.Items,
		byName:         make(map[string]*ItemDef),
		byIdx:          make(map[int]*ItemDef),
		UnbindList:     make(map[int]string),
		DisableTakeOff: make(map[int]bool),
	}
	for i := range db.Items {
		item := &db.Items[i]
		db.byName[item.Name] = item
		db.byIdx[item.Idx] = item
	}
	log.Logf(log.LevelInfo, "ItemDB", "loaded %d items from %s", len(db.Items), path)
	return db, nil
}

// LoadUnbindList 装载解包表（Shape→物品名）。
// 来源 serverconfig/items/unbind_list.jsonc（转换器读 Envir/UnbindList.txt）。
func (db *ItemDB) LoadUnbindList(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "ItemDB", "no unbind list at %s (StdMode 31 解包不可用)", path)
		return
	}
	var raw struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		log.Logf(log.LevelError, "ItemDB", "failed to parse unbind list %s: %v", path, err)
		return
	}
	for _, it := range raw.Items {
		db.UnbindList[it.ID] = it.Name
	}
	log.Logf(log.LevelInfo, "ItemDB", "loaded %d unbind entries from %s", len(db.UnbindList), path)
}

// LoadDisableTakeOffList 装载禁止脱下物品表（0 基 DB 索引）。
// Delphi 文件格式为 "物品名 索引"，索引 = wIndex-1（M2Share.pas:4642-4660
// 按 nItemIdx-1 匹配）；Go 的 WIndex 即 0 基索引，直接入集合。
func (db *ItemDB) LoadDisableTakeOffList(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // 空表为常态（Delphi 无文件时创建空文件）
	}
	var raw struct {
		Items []struct {
			Name string `json:"name"`
			Idx  int    `json:"idx"`
		} `json:"items"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		log.Logf(log.LevelError, "ItemDB", "failed to parse disable-takeoff list %s: %v", path, err)
		return
	}
	for _, it := range raw.Items {
		db.DisableTakeOff[it.Idx] = true
	}
	log.Logf(log.LevelInfo, "ItemDB", "loaded %d disable-takeoff entries from %s", len(db.DisableTakeOff), path)
}

// InDisableTakeOffList 判断物品实例是否禁止脱下/死亡掉落。
func (db *ItemDB) InDisableTakeOffList(wIndex uint16) bool {
	return db.DisableTakeOff[int(wIndex)]
}

func (db *ItemDB) GetByName(name string) *ItemDef {
	return db.byName[name]
}

func (db *ItemDB) GetByIdx(idx int) *ItemDef {
	return db.byIdx[idx]
}

func (db *ItemDB) CreateUserItem(idx int) *protocol.UserItem {
	def := db.GetByIdx(idx)
	if def == nil {
		return nil
	}
	dura := uint16(def.DuraMax)
	if dura == 0 {
		dura = 1000
	}
	return &protocol.UserItem{
		WIndex:  uint16(idx),
		Dura:    dura,
		DuraMax: dura,
	}
}

// StdItemOf 将 ItemDef 打包为 protocol.StdItem（详细商品列表
// TClientItem.S 用）。Lo/Hi 打包约定见 types.go：低字节=基础值，
// 高字节=最大值（对应 Delphi CopyStdItemToOStdItem 的字段布局）。
func StdItemOf(def *ItemDef) protocol.StdItem {
	var s protocol.StdItem
	copy(s.Name[:], def.Name)
	s.StdMode = def.StdMode
	s.Shape = def.Shape
	s.Weight = def.Weight
	s.AniCount = def.AniCount
	s.Source = int8(def.Source)
	s.Reserved = byte(def.Reserved)
	s.Looks = def.Looks
	s.DuraMax = def.DuraMax
	s.AC = def.ACMax<<16 | def.AC
	s.MAC = def.MACMax<<16 | def.MAC
	s.DC = def.DCMax<<16 | def.DC
	s.MC = def.MCMax<<16 | def.MC
	s.SC = def.SCMax<<16 | def.SC
	s.Need = uint32(def.Need)
	s.NeedLevel = def.NeedLevel
	s.Price = def.Price
	return s
}
