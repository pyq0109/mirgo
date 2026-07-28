package main

import (
	"encoding/binary"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// ClientItemDef 是服务端 ItemDef 的客户端副本，登录时通过 SMStdItems
// 一次性下发（布局定义见 PlayObject.SendStdItems）。
type ClientItemDef struct {
	Name      string
	Looks     uint16 // Items.wil / StateItem.wil 索引
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
	Idx       uint16 // WIndex: 物品数据库索引
	Dura      uint16
	DuraMax   uint16
	MakeIndex int32
	Def       *ClientItemDef // 关联 GameState.ItemDefs（可能为 nil）
}

// Looks 返回 Items.wil 精灵索引，当物品定义库不可用时回退到原始 WIndex。
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
	IconIdx  uint16 // MagIcon.wil 索引（def.Effect*2）
	CurTrain uint16
	MaxTrain uint16
	Name     string
}

type GameState struct {
	MySelf     *Actor
	Actors     *ActorManager
	UseItems   [13]*protocol.UserItem
	MagicList  []protocol.UserMagic
	DayBright   int
	MapDarkness int
	MapName     string
	MapTitle   string
	LightLevel int

	// ServerName 是所选服务器的显示名称，绘制在选角场景顶部居中位置
	// （IntroScn:1539-1545, g_sServerName）。
	// 当前未赋值：SMSelectServerOK 仅携带 addr/port/cert，
	// 网络流程有意不做改动。
	ServerName string

	ItemDefs map[int]*ClientItemDef

	// Bag 是固定 46 格数组（对应 Delphi g_ItemArr 模型）：布局由客户端管理，
	// 内容由服务端权威（变更时全量重同步）。
	// nil 格 = 空。
	BagItems [protocol.MaxBagItem]*BagItem
	Level    int
	HP, MP   int
	MaxHP, MaxMP int
	Gold     int

	// 完整属性块（SMAbility body）。
	Exp, MaxExp               int
	Weight, MaxWeight         int
	WearWeight, MaxWearWeight int
	HandWeight, MaxHandWeight int
	AC, MAC, DC, MC, SC       uint32 // 打包格式 lo | hi<<16
	Hit, Speed                int
	BonusPoint                int

	Sex, Hair int
	Job       int // 0 战士 / 1 法师 / 2 道士（SMLogon body 第 3 字段）

	ShowBag      bool
	ShowEquip    bool
	StatePage    int  // 装备面板页码 0-3（FState StatePage）
	ShowGroupDlg bool // 组队对话框（面板本身在 P9 实现）
	ShowPlusAbil bool // 属性调整面板（P10）

	Magics        []LearnedMagic
	ShowNpcDialog bool

	InDeal          bool
	DealPartner     string
	DealItems       [10]*BagItem // 己方放入的物品（DDGrid 5×2）
	DealRemoteItems [20]*BagItem // 对方放入的物品（DDRGrid 5×4）
	DealGold        int
	DealRemoteGold  int
	DealEnd         bool // 点击确认后锁定（g_boDealEnd）

	GuildName      string
	GuildRank      string
	ShowGuild      bool
	GuildMembers   []string // "名字/职位/在线" 行
	GuildNotice    string
	GuildCommander bool // 可使用管理按钮
	GuildTopLine   int

	GroupMembers []string // 成员名列表，队长在前
	AllowGroup   bool

	StorageItems []BagItem

	SysMessages []string

	// 腰带格存放背包物品的引用（Delphi 将腰带物品放在 g_ItemArr[0..5]；
	// 服务端仍将其计为背包物品——背包格子渲染时跳过已被腰带引用的物品）。
	BeltItems  [6]*BagItem
	AttackMode int

	ShowShop    bool
	ShopGoods   []ShopItem
	ShopNpcID   int32
	ShopMode    int

	SoundEnabled bool
	BGMEnabled   bool
	MapMusic     int
}

type ShopItem struct {
	ItemIdx uint16
	Price   int
	Name    string
}

func NewGameState() *GameState {
	return &GameState{
		Actors:       NewActorManager(),
		ItemDefs:     make(map[int]*ClientItemDef),
		SoundEnabled: true,
		BGMEnabled:   true,
		MapMusic:     -1,
	}
}

func (gs *GameState) Reset() {
	gs.MySelf = nil
	gs.Actors.Clear()
}

// ItemDef 将物品数据库索引解析为定义（未知则返回 nil）。
func (gs *GameState) ItemDef(idx int) *ClientItemDef {
	return gs.ItemDefs[idx]
}

// linkBagDefs 为已解析的背包/仓库物品关联定义（在数据库晚于背包数据到达时，
// 以及每次背包解析后调用）。
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

// ParseItemDefs 解析 SMStdItems body（布局见 PlayObject.SendStdItems），
// 并重新关联已有物品。
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
	// 腰带布局由客户端管理；通过实例 ID 在重同步时保留。
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

// BeltHolds 判断物品是否被腰带格引用（背包格子会隐藏已被腰带引用的物品）。
func (gs *GameState) BeltHolds(b *BagItem) bool {
	for _, x := range gs.BeltItems {
		if x == b {
			return true
		}
	}
	return false
}

// FindBagItemByMakeIndex 返回物品所在格索引，未找到返回 -1。
func (gs *GameState) FindBagItemByMakeIndex(makeIndex int32) int {
	for i, b := range gs.BagItems {
		if b != nil && b.MakeIndex == makeIndex {
			return i
		}
	}
	return -1
}

// ParseMagics 解析扩展 SMSendMyMagic body：count u16，之后每条魔法
// MagID u16, Level u8, Key u8, IconIdx u16, CurTrain u16, MaxTrain u16,
// NameLen u8 + Name。
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

// ParseAbility 解析 SMAbility body（60 字节，见 PlayObject.SendAbility）。
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
