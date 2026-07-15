package protocol

// ============================================================================
// Constants
// ============================================================================

// Direction constants
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

// Grid constants
const (
	UnitX           = 48 // Tile width in pixels
	UnitY           = 32 // Tile height in pixels
	LogicalMapUnit  = 40 // Logical map unit size
	HalfX           = 24 // Half tile width
	HalfY           = 16 // Half tile height
	MapNameLen      = 16 // Maximum map name length
	ActorNameLen    = 14 // Maximum actor name length
)

// Equipment slot constants (1.50 version - 10 slots)
const (
	UDress     = 0 // Clothes
	UWeapon    = 1 // Weapon
	URightHand = 2 // Shield/Right hand
	UNecklace  = 3 // Necklace
	UHelmet    = 4 // Helmet
	UArmRingL  = 5 // Left bracelet
	UArmRingR  = 6 // Right bracelet
	URingL     = 7 // Left ring
	URingR     = 8 // Right ring
	UBujuk     = 9 // Amulet slot
)

// Equipment slot constants (1.70 version - 13 slots)
const (
	UBelt  = 10 // Belt
	UBoots = 11 // Boots
	UCharm = 12 // Charm/Stone
)

// Item type constants
const (
	ItemWeapon   = 0  // Weapons
	ItemArmor    = 1  // Armor
	ItemAccessory = 2 // Accessories
	ItemEtc      = 3  // Miscellaneous
	ItemGold     = 10 // Gold coins
)

// Poison type constants
const (
	PoisonDecHealth   = 0  // Fixed poison damage
	PoisonDamageArmor = 1  // Armor damage poison
	PoisonLockSpell   = 2  // Spell lock
	PoisonDontMove    = 4  // Movement lock
	PoisonStone       = 5  // Stone/petrify
)

// State constants
const (
	StateTransparent     = 8
	StateDefenceUp       = 9
	StateMagDefenceUp    = 10
	StateBubbleDefenceUp = 11
)

// State flag constants
const (
	StateStoneMode  = 0x00000001
	StateOpenHealth = 0x00000002
)

// Event type constants
const (
	ETDigOutZombi = 1
	ETMine        = 2
	ETPileStones  = 3
	ETHolyCurtain = 4
	ETFire        = 5
	ETSculPiece   = 6
)

// Race type constants
const (
	RCPlayObject   = 0
	RCNpc          = 10
	RCGuard        = 11
	RCPeaceNpc     = 15
	RCAnimal       = 50
	RCMonster      = 80
	RCArcherGuard  = 112
)

// Attack mode constants
const (
	HAMAll      = 0 // Attack all
	HAMPeace    = 1 // Peace mode
	HAMGroup    = 2 // Group mode
	HAMGuild    = 3 // Guild mode
	HAMPKAttack = 4 // PK attack mode
)

// Maximum constants
const (
	MaxBagItem         = 46 // Maximum bag items
	HowManyMagics      = 20 // Maximum learned spells
	UserItemMax        = 46
	MaxSkillLevel      = 3
	MaxStatusAttribute = 12
	MaxLevel           = 500
	SlaveMaxLevel      = 50
	GroupMax           = 11
)

// Version constants
const (
	VersionNumber      = 20020522
	ClientVersionNumber = 120040918
)

// ============================================================================
// Data Structures
// ============================================================================

// StdItem is the item definition structure.
// Size: 60 bytes (packed)
type StdItem struct {
	Name         [20]byte // Item name (null-terminated)
	StdMode      byte     // Item type/category
	Shape        byte     // Shape/subtype
	Weight       byte     // Weight
	AniCount     byte     // Animation frame count (0=static)
	Source       int8     // Source/holy value
	Reserved     byte     // Reserved
	NeedIdentify byte     // Needs identification
	Looks        uint16   // Appearance (WIL image index)
	DuraMax      uint32   // Max durability
	AC           uint32   // Physical defense (Lo=base, Hi=max)
	MAC          uint32   // Magic defense
	DC           uint32   // Physical attack
	MC           uint32   // Magic attack
	SC           uint32   // Soul/Taoist attack
	Need         uint32   // Requirement type (0=level, 1=DC, 2=MC, 3=SC)
	NeedLevel    uint32   // Requirement value
	Price        uint32   // Price
}

// GetName returns the item name as a string.
func (s *StdItem) GetName() string {
	for i, b := range s.Name {
		if b == 0 {
			return string(s.Name[:i])
		}
	}
	return string(s.Name[:])
}

// UserItem is an item instance carried by a player.
// Size: 24 bytes
type UserItem struct {
	MakeIndex int32      // Unique instance ID
	WIndex    uint16     // Item definition index (1-based into StdItemList)
	Dura      uint16     // Current durability
	DuraMax   uint16     // Max durability
	BtValue   [14]byte   // Custom values (upgrade stats, etc.)
}

// ClientItem is a client-side item with full definition.
type ClientItem struct {
	S         StdItem  // Item definition
	MakeIndex int32    // Unique instance ID
	Dura      uint16   // Current durability
	DuraMax   uint16   // Max durability
}

// Ability represents character abilities.
// Size: 50 bytes (packed)
type Ability struct {
	Level         uint16 // Character level
	AC            uint32 // Physical defense
	MAC           uint32 // Magic defense
	DC            uint32 // Physical attack
	MC            uint32 // Magic attack
	SC            uint32 // Soul/Taoist attack
	HP            uint16 // Current HP
	MP            uint16 // Current MP
	MaxHP         uint16 // Max HP
	MaxMP         uint16 // Max MP
	Exp           uint32 // Current experience
	MaxExp        uint32 // Experience to next level
	Weight        uint16 // Current weight
	MaxWeight     uint16 // Max weight
	WearWeight    uint8  // Current wear weight
	MaxWearWeight uint8  // Max wear weight
	HandWeight    uint8  // Current hand weight
	MaxHandWeight uint8  // Max hand weight
}

// NakedAbility represents bonus attributes.
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

// AddAbility represents additional equipment bonuses.
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

// Magic represents a spell definition.
type Magic struct {
	WMagicID   uint16       // Skill ID
	SMagicName [13]byte     // Skill name
	BtEffectType byte       // Effect type
	BtEffect   byte         // Effect ID
	WSpell     uint16       // MP cost
	WPower     uint16       // Base power
	TrainLevel [4]byte      // Level requirements for training
	MaxTrain   [4]uint32    // Max training points per level
	BtTrainLv  byte         // Max train level
	BtJob      byte         // Job requirement
	DwDelayTime int32       // Delay time
	BtDefSpell byte         // Default spell cost
	BtDefPower byte         // Default power
	WMaxPower  uint16       // Max power
	BtDefMaxPower byte      // Default max power
	SDescr     [16]byte     // Description
}

// GetName returns the magic name as a string.
func (m *Magic) GetName() string {
	for i, b := range m.SMagicName {
		if b == 0 {
			return string(m.SMagicName[:i])
		}
	}
	return string(m.SMagicName[:])
}

// UserMagic represents a player's learned spell.
type UserMagic struct {
	MagicInfo  *Magic  // Reference to spell definition
	Level      byte    // Current level (0-3)
	MagIdx     uint16  // Magic index
	TranPoint  uint32  // Training points
	Key        byte    // Hotkey binding
}

// ChrMsg is a character message for the message queue.
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

// UserCharacterInfo represents a character in the selection screen.
type UserCharacterInfo struct {
	Name  [20]byte
	Job   byte
	Hair  byte
	Level byte
	Sex   byte
}

// GetName returns the character name as a string.
func (u *UserCharacterInfo) GetName() string {
	for i, b := range u.Name {
		if b == 0 {
			return string(u.Name[:i])
		}
	}
	return string(u.Name[:])
}

// UserEntry is the account registration structure.
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

// UserEntryAdd is additional user registration info.
type UserEntryAdd struct {
	SQuiz2       [21]byte
	SAnswer2     [13]byte
	SBirthDay    [11]byte
	SMobilePhone [16]byte
	SMemo        [41]byte
	SMemo2       [41]byte
}

// UserStateInfo is used for viewing other players' info.
type UserStateInfo struct {
	Feature       int32
	UserName      [20]byte
	GuildName     [15]byte
	GuildRankName [15]byte
	NameColor     uint16
	UseItems      [13]ClientItem
}

// DropItem represents an item on the ground.
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

// StatusTime is an array of status effect timers.
type StatusTime [MaxStatusAttribute]int16

// QuestUnit is an array of quest flags.
type QuestUnit [128]byte

// QuestFlag is an array of quest flags.
type QuestFlag [128]byte

// ============================================================================
// Feature encoding helpers
// ============================================================================

// MakeHumanFeature encodes human appearance into a 32-bit integer.
// Bits: [31..24]=Dress, [23..16]=Hair, [15..8]=Weapon, [7..0]=RaceImg
func MakeHumanFeature(raceImg, dress, weapon, hair byte) int32 {
	return int32(raceImg) | int32(weapon)<<8 | int32(hair)<<16 | int32(dress)<<24
}

// MakeMonsterFeature encodes monster appearance into a 32-bit integer.
// Bits: [31..16]=Appr, [15..8]=Weapon, [7..0]=RaceImg
func MakeMonsterFeature(raceImg, weapon byte, appr uint16) int32 {
	return int32(raceImg) | int32(weapon)<<8 | int32(appr)<<16
}

// ParseHumanFeature extracts human appearance components.
func ParseHumanFeature(feature int32) (raceImg, dress, weapon, hair byte) {
	raceImg = byte(feature & 0xFF)
	weapon = byte((feature >> 8) & 0xFF)
	hair = byte((feature >> 16) & 0xFF)
	dress = byte((feature >> 24) & 0xFF)
	return
}

// ParseMonsterFeature extracts monster appearance components.
func ParseMonsterFeature(feature int32) (raceImg, weapon byte, appr uint16) {
	raceImg = byte(feature & 0xFF)
	weapon = byte((feature >> 8) & 0xFF)
	appr = uint16((feature >> 16) & 0xFFFF)
	return
}
