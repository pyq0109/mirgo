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
	var raw struct {
		Items []ItemDef `json:"items"`
	}
	if err := parseJSONC(data, &raw); err != nil {
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
	log.Logf(log.LevelInfo, "ItemDB", "loaded %d items from %s", len(db.Items), path)
	return db, nil
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
	s.Looks = def.Looks
	s.DuraMax = def.DuraMax
	s.AC = def.ACMax<<16 | def.AC
	s.MAC = def.MACMax<<16 | def.MAC
	s.DC = def.DCMax<<16 | def.DC
	s.MC = def.MCMax<<16 | def.MC
	s.SC = def.SCMax<<16 | def.SC
	s.Need = uint32(def.Need)
	s.NeedLevel = uint32(def.NeedLevel)
	s.Price = def.Price
	return s
}
