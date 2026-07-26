package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/protocol"
)

type BagItem struct {
	Idx       uint16
	Dura      uint16
	DuraMax   uint16
	MakeIndex int32
}

type LearnedMagic struct {
	MagID uint16
	Level byte
	Key   byte
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

	BagItems []BagItem
	Level    int
	HP, MP   int
	MaxHP, MaxMP int
	Gold     int

	ShowBag   bool
	ShowEquip bool
	ShowChar  bool

	Magics        []LearnedMagic
	NpcDialog     string
	ShowNpcDialog bool

	InDeal      bool
	DealPartner string
	DealItems   []string
	DealGold    int

	GuildName string
	GuildRank string
	ShowGuild bool

	ShowStorage  bool
	StorageItems []BagItem

	SysMessages []string
}

func NewGameState() *GameState {
	return &GameState{
		Actors: NewActorManager(),
	}
}

func (gs *GameState) Reset() {
	gs.MySelf = nil
	gs.Actors.Clear()
}

func (gs *GameState) ParseBagItems(body string) {
	raw := []byte(body)
	if len(raw) < 2 {
		gs.BagItems = nil
		return
	}
	count := int(binary.LittleEndian.Uint16(raw[0:2]))
	gs.BagItems = make([]BagItem, 0, count)
	for i := 0; i < count; i++ {
		off := 2 + i*10
		if off+10 > len(raw) {
			break
		}
		item := BagItem{
			Idx:       binary.LittleEndian.Uint16(raw[off : off+2]),
			Dura:      binary.LittleEndian.Uint16(raw[off+2 : off+4]),
			DuraMax:   binary.LittleEndian.Uint16(raw[off+4 : off+6]),
			MakeIndex: int32(binary.LittleEndian.Uint32(raw[off+6 : off+10])),
		}
		gs.BagItems = append(gs.BagItems, item)
	}
}

func (gs *GameState) ParseMagics(body string) {
	raw := []byte(body)
	if len(raw) < 2 {
		return
	}
	count := int(binary.LittleEndian.Uint16(raw[0:2]))
	gs.Magics = make([]LearnedMagic, 0, count)
	for i := 0; i < count; i++ {
		off := 2 + i*6
		if off+6 > len(raw) {
			break
		}
		m := LearnedMagic{
			MagID: binary.LittleEndian.Uint16(raw[off : off+2]),
			Level: raw[off+2],
			Key:   raw[off+3],
		}
		gs.Magics = append(gs.Magics, m)
	}
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
