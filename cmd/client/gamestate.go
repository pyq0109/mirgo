package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ClientItemDef is the client-side copy of a server ItemDef, delivered once
// at login via SMStdItems (layout defined in PlayObject.SendStdItems).
type ClientItemDef struct {
	Name      string
	Looks     uint16 // Items.wil / StateItem.wil index
	StdMode   byte
	Shape     byte
	Weight    byte
	NeedLevel byte
	AC, ACMax uint16
	MAC, MACMax uint16
	DC, DCMax   uint16
	MC, MCMax   uint16
	SC, SCMax   uint16
	Price     uint32
}

type BagItem struct {
	Idx       uint16 // WIndex: item DB index
	Dura      uint16
	DuraMax   uint16
	MakeIndex int32
	Def       *ClientItemDef // resolved against GameState.ItemDefs (may be nil)
}

// Looks returns the Items.wil sprite index, falling back to the raw WIndex
// when the def DB is unavailable.
func (b *BagItem) Looks() uint16 {
	if b.Def != nil {
		return b.Def.Looks
	}
	return b.Idx
}

type LearnedMagic struct {
	MagID    uint16
	Level    byte
	Key      byte
	IconIdx  uint16 // MagIcon.wil index (def.Effect*2)
	CurTrain uint16
	MaxTrain uint16
	Name     string
}

type GameState struct {
	MySelf     *Actor
	Actors     *ActorManager
	UseItems   [13]*protocol.UserItem
	MagicList  []protocol.UserMagic
	DayBright  int
	MapName    string
	MapTitle   string
	LightLevel int

	ItemDefs map[int]*ClientItemDef

	// Bag is a fixed 46-slot array (Delphi g_ItemArr model): client-owned
	// layout, server is authoritative for contents (full re-sync on change).
	// Nil slot = empty.
	BagItems [protocol.MaxBagItem]*BagItem
	Level    int
	HP, MP   int
	MaxHP, MaxMP int
	Gold     int

	// Full ability block (SMAbility body).
	Exp, MaxExp               int
	Weight, MaxWeight         int
	WearWeight, MaxWearWeight int
	HandWeight, MaxHandWeight int
	AC, MAC, DC, MC, SC       uint32 // packed lo | hi<<16
	Hit, Speed                int
	BonusPoint                int

	Sex, Hair int
	Job       int // 0 warrior / 1 mage / 2 taoist (SMLogon body slot 3)

	ShowBag      bool
	ShowEquip    bool
	StatePage    int  // equipment panel page 0-3 (FState StatePage)
	ShowGroupDlg bool // group dialog (panel itself arrives in P9)
	ShowPlusAbil bool // adjust-ability panel (P10)

	Magics        []LearnedMagic
	ShowNpcDialog bool

	InDeal          bool
	DealPartner     string
	DealItems       [10]*BagItem // own offered items (DDGrid 5×2)
	DealRemoteItems [20]*BagItem // partner's offered items (DDRGrid 5×4)
	DealGold        int
	DealRemoteGold  int
	DealEnd         bool // locked after pressing confirm (g_boDealEnd)

	GuildName      string
	GuildRank      string
	ShowGuild      bool
	GuildMembers   []string // "name/rank/online" lines
	GuildNotice    string
	GuildCommander bool // may use admin buttons
	GuildTopLine   int

	GroupMembers []string // names, leader first
	AllowGroup   bool

	StorageItems []BagItem

	SysMessages []string

	// Belt slots hold references to bag items (Delphi keeps belt items in
	// g_ItemArr[0..5]; the server still counts them as bag items — the bag
	// grid render skips belt-referenced items).
	BeltItems  [6]*BagItem
	AttackMode int

	ShowShop    bool
	ShopGoods   []ShopItem
	ShopNpcID   int32
	ShopMode    int
}

type ShopItem struct {
	ItemIdx uint16
	Price   int
	Name    string
}

func NewGameState() *GameState {
	return &GameState{
		Actors:   NewActorManager(),
		ItemDefs: make(map[int]*ClientItemDef),
	}
}

func (gs *GameState) Reset() {
	gs.MySelf = nil
	gs.Actors.Clear()
}

// ItemDef resolves an item DB index to its definition (nil if unknown).
func (gs *GameState) ItemDef(idx int) *ClientItemDef {
	return gs.ItemDefs[idx]
}

// linkBagDefs attaches defs to already-parsed bag/storage items (called when
// the DB arrives after the first bag payload, and after each bag parse).
func (gs *GameState) linkBagDefs() {
	for _, b := range gs.BagItems {
		if b != nil {
			b.Def = gs.ItemDefs[int(b.Idx)]
		}
	}
	for i := range gs.StorageItems {
		gs.StorageItems[i].Def = gs.ItemDefs[int(gs.StorageItems[i].Idx)]
	}
}

// ParseItemDefs parses the SMStdItems body (see PlayObject.SendStdItems for
// the layout) and re-links existing items.
func (gs *GameState) ParseItemDefs(body string) {
	raw := []byte(body)
	if len(raw) < 2 {
		return
	}
	count := int(binary.LittleEndian.Uint16(raw[0:2]))
	off := 2
	for i := 0; i < count && off+26 <= len(raw); i++ {
		def := &ClientItemDef{
			Looks:     binary.LittleEndian.Uint16(raw[off+2 : off+4]),
			StdMode:   raw[off+4],
			Shape:     raw[off+5],
			Weight:    raw[off+6],
			NeedLevel: raw[off+7],
			AC:        binary.LittleEndian.Uint16(raw[off+8 : off+10]),
			ACMax:     binary.LittleEndian.Uint16(raw[off+10 : off+12]),
			MAC:       binary.LittleEndian.Uint16(raw[off+12 : off+14]),
			MACMax:    binary.LittleEndian.Uint16(raw[off+14 : off+16]),
			DC:        binary.LittleEndian.Uint16(raw[off+16 : off+18]),
			DCMax:     binary.LittleEndian.Uint16(raw[off+18 : off+20]),
			MC:        binary.LittleEndian.Uint16(raw[off+20 : off+22]),
			MCMax:     binary.LittleEndian.Uint16(raw[off+22 : off+24]),
			SC:        binary.LittleEndian.Uint16(raw[off+24 : off+26]),
		}
		idx := int(binary.LittleEndian.Uint16(raw[off : off+2]))
		off += 26
		def.SCMax = binary.LittleEndian.Uint16(raw[off : off+2])
		def.Price = binary.LittleEndian.Uint32(raw[off+2 : off+6])
		nameLen := int(raw[off+6])
		off += 7
		if off+nameLen > len(raw) {
			break
		}
		def.Name = string(raw[off : off+nameLen])
		off += nameLen
		gs.ItemDefs[idx] = def
	}
	gs.linkBagDefs()
}

func (gs *GameState) ParseBagItems(body string) {
	// Belt layout is client-owned; preserve it across re-syncs by instance id.
	var beltIDs [6]int32
	for i, b := range gs.BeltItems {
		if b != nil {
			beltIDs[i] = b.MakeIndex
		}
	}

	for i := range gs.BagItems {
		gs.BagItems[i] = nil
	}
	raw := []byte(body)
	if len(raw) >= 2 {
		count := int(binary.LittleEndian.Uint16(raw[0:2]))
		for i := 0; i < count && i < len(gs.BagItems); i++ {
			off := 2 + i*10
			if off+10 > len(raw) {
				break
			}
			item := &BagItem{
				Idx:       binary.LittleEndian.Uint16(raw[off : off+2]),
				Dura:      binary.LittleEndian.Uint16(raw[off+2 : off+4]),
				DuraMax:   binary.LittleEndian.Uint16(raw[off+4 : off+6]),
				MakeIndex: int32(binary.LittleEndian.Uint32(raw[off+6 : off+10])),
			}
			item.Def = gs.ItemDefs[int(item.Idx)]
			gs.BagItems[i] = item
		}
	}

	for i, id := range beltIDs {
		gs.BeltItems[i] = nil
		if id == 0 {
			continue
		}
		for _, b := range gs.BagItems {
			if b != nil && b.MakeIndex == id {
				gs.BeltItems[i] = b
				break
			}
		}
	}
}

// BeltHolds reports whether the item is referenced by a belt slot (the bag
// grid hides belt-referenced items).
func (gs *GameState) BeltHolds(b *BagItem) bool {
	for _, x := range gs.BeltItems {
		if x == b {
			return true
		}
	}
	return false
}

// FindBagItemByMakeIndex returns the slot of the item, or -1.
func (gs *GameState) FindBagItemByMakeIndex(makeIndex int32) int {
	for i, b := range gs.BagItems {
		if b != nil && b.MakeIndex == makeIndex {
			return i
		}
	}
	return -1
}

// ParseMagics parses the extended SMSendMyMagic body: count u16, then per
// magic MagID u16, Level u8, Key u8, IconIdx u16, CurTrain u16, MaxTrain u16,
// NameLen u8 + Name.
func (gs *GameState) ParseMagics(body string) {
	raw := []byte(body)
	if len(raw) < 2 {
		return
	}
	count := int(binary.LittleEndian.Uint16(raw[0:2]))
	gs.Magics = make([]LearnedMagic, 0, count)
	off := 2
	for i := 0; i < count && off+10 <= len(raw); i++ {
		m := LearnedMagic{
			MagID:    binary.LittleEndian.Uint16(raw[off : off+2]),
			Level:    raw[off+2],
			Key:      raw[off+3],
			IconIdx:  binary.LittleEndian.Uint16(raw[off+4 : off+6]),
			CurTrain: binary.LittleEndian.Uint16(raw[off+6 : off+8]),
			MaxTrain: binary.LittleEndian.Uint16(raw[off+8 : off+10]),
		}
		off += 10
		nameLen := int(raw[off])
		off++
		if off+nameLen > len(raw) {
			break
		}
		m.Name = string(raw[off : off+nameLen])
		off += nameLen
		gs.Magics = append(gs.Magics, m)
	}
}

// ParseAbility parses the SMAbility body (60 bytes, see PlayObject.SendAbility).
func (gs *GameState) ParseAbility(body string) {
	raw := []byte(body)
	if len(raw) < 60 {
		return
	}
	u16 := func(o int) int { return int(binary.LittleEndian.Uint16(raw[o : o+2])) }
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(raw[o : o+4]) }
	gs.Level = u16(0)
	gs.AC = u32(2)
	gs.MAC = u32(6)
	gs.DC = u32(10)
	gs.MC = u32(14)
	gs.SC = u32(18)
	gs.HP = u16(22)
	gs.MaxHP = u16(24)
	gs.MP = u16(26)
	gs.MaxMP = u16(28)
	gs.Exp = int(u32(30))
	gs.MaxExp = int(u32(34))
	gs.Weight = u16(38)
	gs.MaxWeight = u16(40)
	gs.WearWeight = u16(42)
	gs.MaxWearWeight = u16(44)
	gs.HandWeight = u16(46)
	gs.MaxHandWeight = u16(48)
	gs.Hit = u16(50)
	gs.Speed = u16(52)
	gs.BonusPoint = u16(54)
	gs.Gold = int(u32(56))
}

func (gs *GameState) ParseUseItems(body string) {
	raw := []byte(body)
	if len(raw) < 130 {
		return
	}
	for i := 0; i < 13; i++ {
		off := i * 10
		idx := binary.LittleEndian.Uint16(raw[off : off+2])
		if idx == 0 {
			gs.UseItems[i] = nil
		} else {
			gs.UseItems[i] = &protocol.UserItem{
				WIndex:    idx,
				Dura:      binary.LittleEndian.Uint16(raw[off+2 : off+4]),
				DuraMax:   binary.LittleEndian.Uint16(raw[off+4 : off+6]),
				MakeIndex: int32(binary.LittleEndian.Uint32(raw[off+6 : off+10])),
			}
		}
	}
}
