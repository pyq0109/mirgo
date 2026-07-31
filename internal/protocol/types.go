package protocol

// ============================================================================
// 常量
// ============================================================================

// 方向常量
const (
	DRUp        = 0
	DRUpRight   = 1
	DRRight     = 2
	DRDownRight = 3
	DRDown      = 4
	DRDownLeft  = 5
	DRLeft      = 6
	DRUpLeft    = 7
)

// 网格常量
const (
	UnitX           = 48 // 地砖宽度（像素）
	UnitY           = 32 // 地砖高度（像素）
	LogicalMapUnit  = 40 // 逻辑地图单位大小
	HalfX           = 24 // 地砖半宽
	HalfY           = 16 // 地砖半高
	MapNameLen      = 16 // 地图名最大长度
	ActorNameLen    = 14 // 角色名最大长度
)

// 装备槽常量（1.50 版本 - 10 个槽）
const (
	UDress     = 0 // 衣服
	UWeapon    = 1 // 武器
	URightHand = 2 // 盾牌/右手
	UNecklace  = 3 // 项链
	UHelmet    = 4 // 头盔
	UArmRingL  = 5 // 左手镯
	UArmRingR  = 6 // 右手镯
	URingL     = 7 // 左戒指
	URingR     = 8 // 右戒指
	UBujuk     = 9 // 护符槽
)

// 装备槽常量（1.70 版本 - 13 个槽）
const (
	UBelt  = 10 // 腰带
	UBoots = 11 // 靴子
	UCharm = 12 // 宝石/石头
)

// 物品类型常量
const (
	ItemWeapon   = 0  // 武器
	ItemArmor    = 1  // 防具
	ItemAccessory = 2 // 饰品
	ItemEtc      = 3  // 杂物
	ItemGold     = 10 // 金币
)

// 毒药类型常量
const (
	PoisonDecHealth   = 0  // 固定毒伤
	PoisonDamageArmor = 1  // 破甲毒
	PoisonLockSpell   = 2  // 锁魔法
	PoisonDontMove    = 4  // 锁移动
	PoisonStone       = 5  // 石化
)

// 状态常量
const (
	StateTransparent     = 8
	StateDefenceUp       = 9
	StateMagDefenceUp    = 10
	StateBubbleDefenceUp = 11
)

// 状态标志常量
const (
	StateStoneMode  = 0x00000001
	StateOpenHealth = 0x00000002
)

// 事件类型常量
const (
	ETDigOutZombi = 1
	ETMine        = 2
	ETPileStones  = 3
	ETHolyCurtain = 4
	ETFire        = 5
	ETSculPiece   = 6
)

// 种族类型常量
const (
	RCPlayObject   = 0
	RCNpc          = 10
	RCGuard        = 12
	RCPeaceNpc     = 15
	RCMerchant     = 50 // NPC商人 (Delphi RCC_MERCHANT)，与RCAnimal共享值50但语义不同
	RCAnimal       = 50 // 动物类怪物
	RCMonster      = 80
	RCArcherGuard  = 112
)

// 攻击模式常量
const (
	HAMAll      = 0 // 全体攻击
	HAMPeace    = 1 // 和平模式
	HAMGroup    = 2 // 组队模式
	HAMGuild    = 3 // 行会模式
	HAMPKAttack = 4 // PK 攻击模式
)

// 最大值常量
const (
	MaxBagItem         = 46 // 背包最大物品数
	HowManyMagics      = 20 // 最大已学魔法数
	UserItemMax        = 46
	MaxSkillLevel      = 3
	MaxStatusAttribute = 12
	MaxLevel           = 500
	SlaveMaxLevel      = 50
	GroupMax           = 11
)

// 版本常量
const (
	VersionNumber      = 20020522
	ClientVersionNumber = 120040918
)

// ============================================================================
// 数据结构
// ============================================================================

// StdItem 是物品定义结构。
// 大小：60 字节（紧凑排列）
type StdItem struct {
	Name         [20]byte // 物品名（以 null 结尾）
	StdMode      byte     // 物品类型/分类
	Shape        byte     // 外形/子类型
	Weight       byte     // 重量
	AniCount     byte     // 动画帧数（0=静态）
	Source       int8     // 来源/神圣值
	Reserved     byte     // 保留
	NeedIdentify byte     // 需要鉴定
	Looks        uint16   // 外观（WIL 图像索引）
	DuraMax      uint32   // 最大耐久
	AC           uint32   // 物理防御（Lo=基础，Hi=最大）
	MAC          uint32   // 魔法防御
	DC           uint32   // 物理攻击
	MC           uint32   // 魔法攻击
	SC           uint32   // 灵魂/道术攻击
	Need         uint32   // 需求类型（0=等级，1=DC，2=MC，3=SC）
	NeedLevel    uint32   // 需求值
	Price        uint32   // 价格
}

// GetName 以字符串形式返回物品名。
func (s *StdItem) GetName() string {
	for i, b := range s.Name {
		if b == 0 {
			return string(s.Name[:i])
		}
	}
	return string(s.Name[:])
}

// UserItem 是玩家携带的物品实例。
// 大小：24 字节
type UserItem struct {
	MakeIndex int32      // 唯一实例 ID
	WIndex    uint16     // 物品定义索引（从 1 开始，指向 StdItemList）
	Dura      uint16     // 当前耐久
	DuraMax   uint16     // 最大耐久
	BtValue   [14]byte   // 自定义数值（升级属性等）
}

// ClientItem 是带完整定义的客户端物品。
type ClientItem struct {
	S         StdItem  // 物品定义
	MakeIndex int32    // 唯一实例 ID
	Dura      uint16   // 当前耐久
	DuraMax   uint16   // 最大耐久
}

// Ability 表示角色能力。
// 大小：50 字节（紧凑排列）—— 对应 Delphi TAbility（Grobal2.pas:734）
type Ability struct {
	Level         uint16 // 角色等级
	AC            uint32 // 物理防御
	MAC           uint32 // 魔法防御
	DC            uint32 // 物理攻击
	MC            uint32 // 魔法攻击
	SC            uint32 // 灵魂/道术攻击
	HP            uint16 // 当前 HP
	MP            uint16 // 当前 MP
	MaxHP         uint16 // 最大 HP
	MaxMP         uint16 // 最大 MP
	Exp           uint32 // 当前经验
	MaxExp        uint32 // 升级所需经验
	Weight        uint16 // 当前重量
	MaxWeight     uint16 // 最大重量
	WearWeight    uint16 // 当前穿戴重量
	MaxWearWeight uint16 // 最大穿戴重量
	HandWeight    uint16 // 当前手持重量
	MaxHandWeight uint16 // 最大手持重量
}

// NakedAbility 表示加成属性。
type NakedAbility struct {
	DC    uint16
	MC    uint16
	SC    uint16
	AC    uint16
	MAC   uint16
	HP    uint16
	MP    uint16
	Hit   uint8
	Speed int32
	X2    uint8
}

// AddAbility 表示额外的装备加成。
type AddAbility struct {
	DC      uint16
	MC      uint16
	SC      uint16
	AC      uint16
	MAC     uint16
	HP      uint16
	MP      uint16
	Hit     uint16
	Speed   uint16
	AntiPoison uint16
	PoisonRecover uint16
	HealthRecover uint16
	SpellRecover uint16
}

// Magic 表示魔法定义。
type Magic struct {
	WMagicID   uint16       // 技能 ID
	SMagicName [13]byte     // 技能名
	BtEffectType byte       // 效果类型
	BtEffect   byte         // 效果 ID
	WSpell     uint16       // MP 消耗
	WPower     uint16       // 基础威力
	TrainLevel [4]byte      // 各等级修炼需求
	MaxTrain   [4]uint32    // 各等级最大修炼值
	BtTrainLv  byte         // 最大修炼等级
	BtJob      byte         // 职业需求
	DwDelayTime int32       // 延迟时间
	BtDefSpell byte         // 默认魔法消耗
	BtDefPower byte         // 默认威力
	WMaxPower  uint16       // 最大威力
	BtDefMaxPower byte      // 默认最大威力
	SDescr     [16]byte     // 描述
}

// GetName 以字符串形式返回魔法名。
func (m *Magic) GetName() string {
	for i, b := range m.SMagicName {
		if b == 0 {
			return string(m.SMagicName[:i])
		}
	}
	return string(m.SMagicName[:])
}

// UserMagic 表示玩家已学的魔法。
type UserMagic struct {
	MagicInfo  *Magic  // 指向魔法定义的引用
	Level      byte    // 当前等级（0-3）
	MagIdx     uint16  // 魔法索引
	TranPoint  uint32  // 修炼值
	Key        byte    // 快捷键绑定
}

// ChrMsg 是消息队列中的角色消息。
type ChrMsg struct {
	Ident   int32
	X       int32
	Y       int32
	Dir     int32
	State   int32
	Feature int32
	Saying  string
	Sound   int32
}

// UserCharacterInfo 表示选角界面中的角色。
type UserCharacterInfo struct {
	Name  [20]byte
	Job   byte
	Hair  byte
	Level byte
	Sex   byte
}

// GetName 以字符串形式返回角色名。
func (u *UserCharacterInfo) GetName() string {
	for i, b := range u.Name {
		if b == 0 {
			return string(u.Name[:i])
		}
	}
	return string(u.Name[:])
}

// UserEntry 是账号注册结构。
type UserEntry struct {
	SAccount  [11]byte
	SPassword [11]byte
	SUserName [21]byte
	SSSNo     [15]byte
	SPhone    [15]byte
	SQuiz     [21]byte
	SAnswer   [13]byte
	SEMail    [41]byte
}

// UserEntryAdd 是附加的用户注册信息。
type UserEntryAdd struct {
	SQuiz2       [21]byte
	SAnswer2     [13]byte
	SBirthDay    [11]byte
	SMobilePhone [16]byte
	SMemo        [41]byte
	SMemo2       [41]byte
}

// UserStateInfo 用于查看其他玩家的信息。
type UserStateInfo struct {
	Feature       int32
	UserName      [20]byte
	GuildName     [15]byte
	GuildRankName [15]byte
	NameColor     uint16
	UseItems      [13]ClientItem
}

// DropItem 表示地面上的物品。
type DropItem struct {
	X           int32
	Y           int32
	Id          int32
	Looks       int32
	Name        string
	FlashTime   uint32
	FlashStepTime uint32
	FlashStep   int32
	BoFlash     bool
}

// StatusTime 是状态效果计时器数组。
type StatusTime [MaxStatusAttribute]int16

// QuestUnit 是任务标志数组。
type QuestUnit [128]byte

// QuestFlag 是任务标志数组。
type QuestFlag [128]byte

// ============================================================================
// Feature 编码辅助函数
// ============================================================================

// MakeHumanFeature 将人物外观编码为 32 位整数。
// 位分布：[31..24]=Dress，[23..16]=Hair，[15..8]=Weapon，[7..0]=RaceImg
func MakeHumanFeature(raceImg, dress, weapon, hair byte) int32 {
	return int32(raceImg) | int32(weapon)<<8 | int32(hair)<<16 | int32(dress)<<24
}

// MakeMonsterFeature 将怪物外观编码为 32 位整数。
// 位分布：[31..16]=Appr，[15..8]=Weapon，[7..0]=RaceImg
func MakeMonsterFeature(raceImg, weapon byte, appr uint16) int32 {
	return int32(raceImg) | int32(weapon)<<8 | int32(appr)<<16
}

// ParseHumanFeature 提取人物外观的各分量。
func ParseHumanFeature(feature int32) (raceImg, dress, weapon, hair byte) {
	raceImg = byte(feature & 0xFF)
	weapon = byte((feature >> 8) & 0xFF)
	hair = byte((feature >> 16) & 0xFF)
	dress = byte((feature >> 24) & 0xFF)
	return
}

// ParseMonsterFeature 提取怪物外观的各分量。
func ParseMonsterFeature(feature int32) (raceImg, weapon byte, appr uint16) {
	raceImg = byte(feature & 0xFF)
	weapon = byte((feature >> 8) & 0xFF)
	appr = uint16((feature >> 16) & 0xFFFF)
	return
}

// ============================================================================
// UserEntry / UserEntryAdd 的网络编码（Delphi Grobal2.pas:592-609）。
// 每个字段都是 Delphi 短字符串：[0]=长度，[1:1+len]=数据，固定 N+1 字节。
// 上面的结构体即为精确的二进制布局；EncodeBuffer 将它们作为两段独立
// 6Bit 编码的内容发送（ClMain.pas:2844），因此服务端必须先按
// UserEntryEncodedSize 切分原始 body 再分别解码。
// ============================================================================

const (
	UserEntrySize    = 148 // sizeof(TUserEntry)
	UserEntryAddSize = 143 // sizeof(TUserEntryAdd)
	// 6Bit 编码后的段长度：GetCodeMsgSize(148)=198，(143)=191。
	UserEntryEncodedSize    = 198
	UserEntryAddEncodedSize = 191
)

func putShortString(dst []byte, s string) {
	max := len(dst) - 1
	if len(s) > max {
		s = s[:max]
	}
	for i := range dst {
		dst[i] = 0
	}
	dst[0] = byte(len(s))
	copy(dst[1:], s)
}

func getShortString(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	n := int(src[0])
	if n > len(src)-1 {
		n = len(src) - 1
	}
	return string(src[1 : 1+n])
}

func (ue *UserEntry) SetAccount(s string)  { putShortString(ue.SAccount[:], s) }
func (ue *UserEntry) SetPassword(s string) { putShortString(ue.SPassword[:], s) }
func (ue *UserEntry) SetUserName(s string) { putShortString(ue.SUserName[:], s) }
func (ue *UserEntry) SetSSNo(s string)     { putShortString(ue.SSSNo[:], s) }
func (ue *UserEntry) SetPhone(s string)    { putShortString(ue.SPhone[:], s) }
func (ue *UserEntry) SetQuiz(s string)     { putShortString(ue.SQuiz[:], s) }
func (ue *UserEntry) SetAnswer(s string)   { putShortString(ue.SAnswer[:], s) }
func (ue *UserEntry) SetEMail(s string)    { putShortString(ue.SEMail[:], s) }

func (ue *UserEntry) Account() string  { return getShortString(ue.SAccount[:]) }
func (ue *UserEntry) Password() string { return getShortString(ue.SPassword[:]) }

func (ua *UserEntryAdd) SetQuiz2(s string)       { putShortString(ua.SQuiz2[:], s) }
func (ua *UserEntryAdd) SetAnswer2(s string)     { putShortString(ua.SAnswer2[:], s) }
func (ua *UserEntryAdd) SetBirthDay(s string)    { putShortString(ua.SBirthDay[:], s) }
func (ua *UserEntryAdd) SetMobilePhone(s string) { putShortString(ua.SMobilePhone[:], s) }
func (ua *UserEntryAdd) SetMemo(s string)        { putShortString(ua.SMemo[:], s) }
func (ua *UserEntryAdd) SetMemo2(s string)       { putShortString(ua.SMemo2[:], s) }

// Bytes 返回固定大小的网络表示。
func (ue *UserEntry) Bytes() []byte {
	buf := make([]byte, UserEntrySize)
	off := 0
	for _, f := range [][]byte{ue.SAccount[:], ue.SPassword[:], ue.SUserName[:], ue.SSSNo[:], ue.SPhone[:], ue.SQuiz[:], ue.SAnswer[:], ue.SEMail[:]} {
		off += copy(buf[off:], f)
	}
	return buf
}

// Bytes 返回固定大小的网络表示。
func (ua *UserEntryAdd) Bytes() []byte {
	buf := make([]byte, UserEntryAddSize)
	off := 0
	for _, f := range [][]byte{ua.SQuiz2[:], ua.SAnswer2[:], ua.SBirthDay[:], ua.SMobilePhone[:], ua.SMemo[:], ua.SMemo2[:]} {
		off += copy(buf[off:], f)
	}
	return buf
}

// UserEntryFromBytes 解析固定大小的网络表示。
func UserEntryFromBytes(buf []byte) UserEntry {
	var ue UserEntry
	off := 0
	for _, f := range [][]byte{ue.SAccount[:], ue.SPassword[:], ue.SUserName[:], ue.SSSNo[:], ue.SPhone[:], ue.SQuiz[:], ue.SAnswer[:], ue.SEMail[:]} {
		off += copy(f, buf[off:])
		if off >= len(buf) {
			break
		}
	}
	return ue
}

// UserEntryAddFromBytes 解析固定大小的网络表示。
func UserEntryAddFromBytes(buf []byte) UserEntryAdd {
	var ua UserEntryAdd
	off := 0
	for _, f := range [][]byte{ua.SQuiz2[:], ua.SAnswer2[:], ua.SBirthDay[:], ua.SMobilePhone[:], ua.SMemo[:], ua.SMemo2[:]} {
		off += copy(f, buf[off:])
		if off >= len(buf) {
			break
		}
	}
	return ua
}
