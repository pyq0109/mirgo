package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServerConfig 保存服务端配置（与 serverconfig/server.jsonc 格式对应）。
type ServerConfig struct {
	Server struct {
		Name   string `json:"name"`
		Index  int    `json:"index"`
		Listen struct {
			Addr string `json:"addr"`
		} `json:"listen"`
		Limits struct {
			MaxPlayers int `json:"maxPlayers"`
			HumLimit   int `json:"humLimit"`
			MonLimit   int `json:"monLimit"`
		} `json:"limits"`
	} `json:"server"`
	Database struct {
		Path string `json:"path"`
	} `json:"database"`
	Game struct {
		HomeMap          string         `json:"homeMap"`
		HomeX            int            `json:"homeX"`
		HomeY            int            `json:"homeY"`
		// 红名回城点（Delphi sRedHomeMap/nRedHomeX/nRedHomeY，
		// PKLevel>=2 时使用回城卷的目的地）。
		RedHomeMap       string         `json:"redHomeMap"`
		RedHomeX         int            `json:"redHomeX"`
		RedHomeY         int            `json:"redHomeY"`
		GroupMembersMax  int            `json:"groupMembersMax"`
		BuildGuild       int            `json:"buildGuild"`
		GuildWarFee      int            `json:"guildWarFee"`
		DisableRun       bool           `json:"disableRun"`
		GMRunAll         bool           `json:"gmRunAll"`
		GMAccounts       map[string]int `json:"gmAccounts"`
		WalkInterval     int64          `json:"walkInterval"`
		RunInterval      int64          `json:"runInterval"`
		SpeedHackKick    bool           `json:"speedHackKick"`
		SpeedHackMax     int            `json:"speedHackMax"`
		ZenLimit         int            `json:"zenLimit"`
		MonGenRate       int            `json:"monGenRate"`
		UserFull         int            `json:"userFull"`
		ZenFastStep      int            `json:"zenFastStep"`
		WalkOnly         bool           `json:"walkOnly"`
		HitIntervalTime  int64          `json:"hitIntervalTime"`
		ActionInterval   int64          `json:"actionInterval"`
		RunLongHitInterval int64        `json:"runLongHitInterval"`
		RunHitInterval   int64          `json:"runHitInterval"`
		WalkHitInterval  int64          `json:"walkHitInterval"`
		RunMagicInterval int64          `json:"runMagicInterval"`
		StruckTime       int64          `json:"struckTime"`
		TurnInterval     int64          `json:"turnInterval"`
		SpellInterval    int64          `json:"spellInterval"`
		DigUpInterval    int64          `json:"digUpInterval"`
		MagicAttackRange int            `json:"magicAttackRange"`
		TickInterval     int64          `json:"tickInterval"`
		SaveInterval     int            `json:"saveInterval"`
		GuildSaveInterval int           `json:"guildSaveInterval"`
		DisableOnlineCount  bool        `json:"disableOnlineCount"`
		SendOnlineTime      int64       `json:"sendOnlineTime"`
		SendOnlineCountRate int         `json:"sendOnlineCountRate"`
		SendOnlineCountMsg  string      `json:"sendOnlineCountMsg"`
		MsgRateLimit        float64     `json:"msgRateLimit"`
		MsgBurst            int         `json:"msgBurst"`
	} `json:"game"`
	Login struct {
		MaxAttempts     int `json:"maxAttempts"`
		LockoutSecs     int `json:"lockoutSecs"`
		MaxCharsPerAcct int `json:"maxCharsPerAcct"`
		MinNameLen      int `json:"minNameLen"`
		MaxNameLen      int `json:"maxNameLen"`
		MinAcctLen      int `json:"minAcctLen"`
		MaxAcctLen      int `json:"maxAcctLen"`
		MinPassLen      int `json:"minPassLen"`
		MaxPassLen      int `json:"maxPassLen"`
	} `json:"login"`
	Player struct {
		MaxBagSlots        int   `json:"maxBagSlots"`
		MaxStorageSlots    int   `json:"maxStorageSlots"`
		// 金币上限（Delphi nHumanMaxGold=10,000,000，M2Share.pas:1763）。
		MaxGold            int   `json:"maxGold"`
		MaxTradeItems      int   `json:"maxTradeItems"`
		MaxFriends         int   `json:"maxFriends"`
		MaxApprentices     int   `json:"maxApprentices"`
		MaxSlaves          int   `json:"maxSlaves"`
		MaxSlaveLevel      int   `json:"maxSlaveLevel"`
		BonusPerLevel      int   `json:"bonusPerLevel"`
		ViewRange          int   `json:"viewRange"`
		VisionInterval     int64 `json:"visionInterval"`
		VisionRand         int64 `json:"visionRand"`
		DeathSkeletonDelay int64 `json:"deathSkeletonDelay"`
		AutoReviveDelay    int64 `json:"autoReviveDelay"`
		ReviveHPRatio      int   `json:"reviveHPRatio"`
		HPRegenInterval    int64 `json:"hpRegenInterval"`
		MPRegenInterval    int64 `json:"mpRegenInterval"`
		HPRegenRatio       int   `json:"hpRegenRatio"`
		MPRegenRatio       int   `json:"mpRegenRatio"`
		DropEquipRate      int   `json:"dropEquipRate"`
		DropEquipRateRed   int   `json:"dropEquipRateRed"`
		DropBagRate        int   `json:"dropBagRate"`
		ExpPenaltyRatio    int   `json:"expPenaltyRatio"`
		NPCInteractDist    int   `json:"npcInteractDist"`
		SpeedDecayInterval int64 `json:"speedDecayInterval"`
		BonusHPPerPoint    int   `json:"bonusHPPerPoint"`
		BonusMPPerPoint    int   `json:"bonusMPPerPoint"`
	} `json:"player"`
	Combat struct {
		FireHitWindow     int64 `json:"fireHitWindow"`
		FireHitCooldown   int64 `json:"fireHitCooldown"`
		TwinHitCooldown   int64 `json:"twinHitCooldown"`
		MagicShieldRatio  int   `json:"magicShieldRatio"`
		RedPoisonBonus    int   `json:"redPoisonBonus"`
		ParalysisDenom    int   `json:"paralysisDenom"`
		ParalysisDuration int64 `json:"paralysisDuration"`
		MapEnterProtect   int64 `json:"mapEnterProtect"`
		PKProtectLevel    int   `json:"pkProtectLevel"`
		PKProtectDiff     int   `json:"pkProtectDiff"`
		PKRedProtectLevel int   `json:"pkRedProtectLevel"`
		ProjectileDelay   int64 `json:"projectileDelay"`
		MaxLOSCheck       int   `json:"maxLOSCheck"`
	} `json:"combat"`
	PK struct {
		PointsPerKill       int   `json:"pointsPerKill"`
		DecayInterval       int64 `json:"decayInterval"`
		DecayAmount         int   `json:"decayAmount"`
		SelfDefenseDuration int64 `json:"selfDefenseDuration"`
		WeaponDuraLoss      int   `json:"weaponDuraLoss"`
	} `json:"pk"`
	Monster struct {
		AITickInterval       int64 `json:"aiTickInterval"`
		CorpseDelay          int64 `json:"corpseDelay"`
		GhostDelay           int64 `json:"ghostDelay"`
		HPRegenInterval      int64 `json:"hpRegenInterval"`
		HPRegenDivisor       int   `json:"hpRegenDivisor"`
		OverlapThinkInterval int64 `json:"overlapThinkInterval"`
		SearchInterval       int64 `json:"searchInterval"`
		SearchRand           int64 `json:"searchRand"`
		SearchNoTarget       int64 `json:"searchNoTarget"`
		SearchHasTarget      int64 `json:"searchHasTarget"`
		TargetFocusTimeout   int64 `json:"targetFocusTimeout"`
		TargetLossDist       int   `json:"targetLossDist"`
		DefaultViewRange     int   `json:"defaultViewRange"`
		DefaultWalkStep      int   `json:"defaultWalkStep"`
		DefaultWalkWait      int64 `json:"defaultWalkWait"`
		DefaultWalkSpeed     int64 `json:"defaultWalkSpeed"`
		DefaultAttackSpeed   int64 `json:"defaultAttackSpeed"`
		MinWalkSpeed         int64 `json:"minWalkSpeed"`
		AttackSpeedOffset    int64 `json:"attackSpeedOffset"`
		SpawnMaxTries        int   `json:"spawnMaxTries"`
		SlaveTeleportDist    int   `json:"slaveTeleportDist"`
		SlaveFollowDist      int   `json:"slaveFollowDist"`
		StruckPenaltyBase    int   `json:"struckPenaltyBase"`
		StruckPenaltyLevel   int   `json:"struckPenaltyLevel"`
		StruckPenaltyMin     int   `json:"struckPenaltyMin"`
		TargetSwitchChance   int   `json:"targetSwitchChance"`
		RangedDistance       int   `json:"rangedDistance"`
		AreaDistance         int   `json:"areaDistance"`
		GuardMaxHP           int   `json:"guardMaxHP"`
		GuardViewRange       int   `json:"guardViewRange"`
	} `json:"monster"`
	MonsterAI struct {
		BurrowTriggerDist   int   `json:"burrowTriggerDist"`
		ReburrowDist        int   `json:"reburrowDist"`
		ExplodeTimer        int64 `json:"explodeTimer"`
		ExplodePowerDiv     int   `json:"explodePowerDiv"`
		ExplodePowerMin     int   `json:"explodePowerMin"`
		DualAxeRange        int   `json:"dualAxeRange"`
		LeechRange          int   `json:"leechRange"`
		LeechBoostRatio     int   `json:"leechBoostRatio"`
		CritRange           int   `json:"critRange"`
		CritChance          int   `json:"critChance"`
		FireballRange       int   `json:"fireballRange"`
		SpitRange           int   `json:"spitRange"`
		PulseRange          int   `json:"pulseRange"`
		LightningRange      int   `json:"lightningRange"`
		CloneThreshold      int   `json:"cloneThreshold"`
		CloneCooldown       int64 `json:"cloneCooldown"`
		SummonMaxMinions    int   `json:"summonMaxMinions"`
		HiveMaxChildren     int   `json:"hiveMaxChildren"`
		CentipedeCooldown   int64 `json:"centipedeCooldown"`
		CentipedeAoEInterval int64 `json:"centipedeAoEInterval"`
		ZumaMaxSlaves       int   `json:"zumaMaxSlaves"`
		CowKingStunSpeed    int64 `json:"cowKingStunSpeed"`
		CowKingRageDuration int64 `json:"cowKingRageDuration"`
		CowKingBerserkAtk   int64 `json:"cowKingBerserkAtk"`
		CowKingBerserkWalk  int64 `json:"cowKingBerserkWalk"`
		FireAuraDuration    int64 `json:"fireAuraDuration"`
		TransformCooldown   int64 `json:"transformCooldown"`
		BoneKingCooldown    int64 `json:"boneKingCooldown"`
		BoneKingMaxChildren int   `json:"boneKingMaxChildren"`
		FleeChance          int   `json:"fleeChance"`
	} `json:"monsterAI"`
	Magic struct {
		FireWallDuration  int64 `json:"fireWallDuration"`
		HealingPoolCap    int   `json:"healingPoolCap"`
		TrainThreshold0   int   `json:"trainThreshold0"`
		TrainThreshold1   int   `json:"trainThreshold1"`
		TrainThreshold2   int   `json:"trainThreshold2"`
		SummonPetHPBase   int   `json:"summonPetHPBase"`
		SummonPetHPPerLv  int   `json:"summonPetHPPerLv"`
		SummonPetDCBase   int   `json:"summonPetDCBase"`
		SummonPetDCPerLv  int   `json:"summonPetDCPerLv"`
		AngelHPBase       int   `json:"angelHPBase"`
		AngelHPPerLv      int   `json:"angelHPPerLv"`
		AngelDCBase       int   `json:"angelDCBase"`
		AngelDCPerLv      int   `json:"angelDCPerLv"`
	} `json:"magic"`
	Economy struct {
		GuildCreateCost      int    `json:"guildCreateCost"`
		GuildWarDuration     int64  `json:"guildWarDuration"`
		UpgradeFee           int    `json:"upgradeFee"`
		UpgradeWaitTime      int64  `json:"upgradeWaitTime"`
		UpgradeMaxPoints     int    `json:"upgradeMaxPoints"`
		UpgradeMaterial      string `json:"upgradeMaterial"`
		UpgradeCurseChance   int    `json:"upgradeCurseChance"`
		RepairDuraDivisor    int    `json:"repairDuraDivisor"`
		SpecialRepairMult    int    `json:"specialRepairMult"`
		CastleDiscount       int    `json:"castleDiscount"`
		CastleMinRate        int    `json:"castleMinRate"`
		CastleTaxRate        int    `json:"castleTaxRate"`
		DrugBasePrice        int    `json:"drugBasePrice"`
		SpouseRecallCooldown int64  `json:"spouseRecallCooldown"`
		DoorCloseDelay       int64  `json:"doorCloseDelay"`
		PileStonesDuration   int64  `json:"pileStonesDuration"`
	} `json:"economy"`
	Drop struct {
		EquipUpgChances  []int `json:"equipUpgChances"`
		EquipDuraMin     int   `json:"equipDuraMin"`
		EquipDuraRand    int   `json:"equipDuraRand"`
		AddValueChance   int   `json:"addValueChance"`
		MaxGoldPiles     int   `json:"maxGoldPiles"`
		MaxGoldPerPile   int   `json:"maxGoldPerPile"`
		FallbackGoldRate int   `json:"fallbackGoldRate"`
		FallbackItemRate int   `json:"fallbackItemRate"`
		FallbackExp      int   `json:"fallbackExp"`
		// 廉价物品管控（Delphi boControlDropItem）：开启后丢弃/掉落
		// Price<500 的物品直接删除不落地、<1000 金币禁丢。
		ControlDropItem bool  `json:"controlDropItem"`
		// 地面物品消失时间 ms（Delphi dwClearDropOnFloorItemTime=3600000）。
		GroundItemDespawnMs int64 `json:"groundItemDespawnMs"`
	} `json:"drop"`
	Mining struct {
		StoneRate int `json:"stoneRate"`
		OreRate   int `json:"oreRate"`
		DuraLoss  int `json:"duraLoss"`
	} `json:"mining"`
	Commands struct {
		Names       map[string]string `json:"names"`
		Permissions map[string]int    `json:"permissions"`
	} `json:"commands"`
	Plugins struct {
		Enabled map[string]bool `json:"enabled"`
	} `json:"plugins"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Database: struct {
			Path string `json:"path"`
		}{
			Path: "serverdata/mir2.db",
		},
	}
}

// LoadConfig 从 serverconfig 目录加载配置。
func LoadConfig(configDir string) (*ServerConfig, error) {
	configFile := filepath.Join(configDir, "server.jsonc")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	config := DefaultConfig()
	if err := parseJSONC(data, config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return config, nil
}

// --- Existing getters ---

func (c *ServerConfig) GetListenAddr() string {
	if c.Server.Listen.Addr != "" {
		return c.Server.Listen.Addr
	}
	return ":7000"
}

func (c *ServerConfig) GetDatabasePath() string {
	if c.Database.Path != "" {
		return c.Database.Path
	}
	return "serverdata/mir2.db"
}

func (c *ServerConfig) GetHomeMap() string {
	if c.Game.HomeMap != "" {
		return c.Game.HomeMap
	}
	return "0"
}

func (c *ServerConfig) GetHomeX() int {
	if c.Game.HomeX > 0 {
		return c.Game.HomeX
	}
	return 289
}

func (c *ServerConfig) GetHomeY() int {
	if c.Game.HomeY > 0 {
		return c.Game.HomeY
	}
	return 618
}

// GetRedHomeMap/GetRedHomeX/GetRedHomeY 红名回城点（Delphi
// g_Config.sRedHomeMap）；未配置时回退普通回城点。
func (c *ServerConfig) GetRedHomeMap() string {
	if c.Game.RedHomeMap != "" {
		return c.Game.RedHomeMap
	}
	return c.GetHomeMap()
}

func (c *ServerConfig) GetRedHomeX() int {
	if c.Game.RedHomeMap != "" {
		return c.Game.RedHomeX
	}
	return c.GetHomeX()
}

func (c *ServerConfig) GetRedHomeY() int {
	if c.Game.RedHomeMap != "" {
		return c.Game.RedHomeY
	}
	return c.GetHomeY()
}

func (c *ServerConfig) GetWalkInterval() int64 {
	if c.Game.WalkInterval > 0 {
		return c.Game.WalkInterval
	}
	return 600
}

func (c *ServerConfig) GetRunInterval() int64 {
	if c.Game.RunInterval > 0 {
		return c.Game.RunInterval
	}
	return 600
}

func (c *ServerConfig) GetSpeedHackMax() int {
	if c.Game.SpeedHackMax > 0 {
		return c.Game.SpeedHackMax
	}
	return 4
}

func (c *ServerConfig) GetZenLimit() int64 {
	if c.Game.ZenLimit > 0 {
		return int64(c.Game.ZenLimit)
	}
	return 50
}

func (c *ServerConfig) GetMonGenRate() int {
	if c.Game.MonGenRate > 0 {
		return c.Game.MonGenRate
	}
	return 10
}

// GetUserFull — Delphi g_Config.nUserFull（M2Share.pas:1582 默认 1000）：
// 在线人数达到该值后刷怪开始加速。
func (c *ServerConfig) GetUserFull() int {
	if c.Game.UserFull > 0 {
		return c.Game.UserFull
	}
	return 1000
}

// GetZenFastStep — Delphi g_Config.nZenFastStep（M2Share.pas:1583 默认 300）：
// 每多出该数量在线人数，刷怪间隔加速一档。
func (c *ServerConfig) GetZenFastStep() int {
	if c.Game.ZenFastStep > 0 {
		return c.Game.ZenFastStep
	}
	return 300
}

func (c *ServerConfig) GetHitIntervalTime() int64 {
	if c.Game.HitIntervalTime > 0 {
		return c.Game.HitIntervalTime
	}
	return 1400
}

func (c *ServerConfig) GetActionInterval() int64 {
	if c.Game.ActionInterval > 0 {
		return c.Game.ActionInterval
	}
	return 400 // Delphi dwActionIntervalTime 默认 400（ActionSpeedConfig.pas:137）
}

// GetRunLongHitInterval — Delphi dwRunLongHitIntervalTime（跑位刺杀，默认 800）。
func (c *ServerConfig) GetRunLongHitInterval() int64 {
	if c.Game.RunLongHitInterval > 0 {
		return c.Game.RunLongHitInterval
	}
	return 800
}

// GetRunHitInterval — Delphi dwRunHitIntervalTime（跑位普攻，默认 800）。
func (c *ServerConfig) GetRunHitInterval() int64 {
	if c.Game.RunHitInterval > 0 {
		return c.Game.RunHitInterval
	}
	return 800
}

// GetWalkHitInterval — Delphi dwWalkHitIntervalTime（走位普攻，默认 800）。
func (c *ServerConfig) GetWalkHitInterval() int64 {
	if c.Game.WalkHitInterval > 0 {
		return c.Game.WalkHitInterval
	}
	return 800
}

// GetRunMagicInterval — Delphi dwRunMagicIntervalTime（跑位魔法，默认 900）。
func (c *ServerConfig) GetRunMagicInterval() int64 {
	if c.Game.RunMagicInterval > 0 {
		return c.Game.RunMagicInterval
	}
	return 900
}

func (c *ServerConfig) GetStruckTime() int64 {
	if c.Game.StruckTime > 0 {
		return c.Game.StruckTime
	}
	return 500
}

func (c *ServerConfig) GetTurnInterval() int64 {
	if c.Game.TurnInterval > 0 {
		return c.Game.TurnInterval
	}
	return 300
}

func (c *ServerConfig) GetSpellInterval() int64 {
	if c.Game.SpellInterval > 0 {
		return c.Game.SpellInterval
	}
	return 600
}

func (c *ServerConfig) GetDigUpInterval() int64 {
	if c.Game.DigUpInterval > 0 {
		return c.Game.DigUpInterval
	}
	return 1000
}

func (c *ServerConfig) GetMagicAttackRange() int {
	if c.Game.MagicAttackRange > 0 {
		return c.Game.MagicAttackRange
	}
	return 12
}

func (c *ServerConfig) GetServerHostPort() (string, int) {
	addr := c.GetListenAddr()
	host := "localhost"
	port := 7000
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		if idx > 0 {
			host = addr[:idx]
		}
		fmt.Sscanf(addr[idx+1:], "%d", &port)
	}
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return host, port
}

// --- Game getters ---

func (c *ServerConfig) GetTickInterval() int64 {
	if c.Game.TickInterval > 0 {
		return c.Game.TickInterval
	}
	return 100
}

func (c *ServerConfig) GetSaveInterval() int {
	if c.Game.SaveInterval > 0 {
		return c.Game.SaveInterval
	}
	return 300
}

func (c *ServerConfig) GetGuildSaveInterval() int {
	if c.Game.GuildSaveInterval > 0 {
		return c.Game.GuildSaveInterval
	}
	return 600
}

// GetSendOnlineTime — Delphi dwSendOnlineTime（M2Share.pas:1793 默认 5 分钟）。
func (c *ServerConfig) GetSendOnlineTime() int64 {
	if c.Game.SendOnlineTime > 0 {
		return c.Game.SendOnlineTime
	}
	return 5 * 60 * 1000
}

// GetSendOnlineCountRate — Delphi nSendOnlineCountRate（M2Share.pas:1792 默认 10，即 ×rate/10）。
func (c *ServerConfig) GetSendOnlineCountRate() int {
	if c.Game.SendOnlineCountRate > 0 {
		return c.Game.SendOnlineCountRate
	}
	return 10
}

// GetSendOnlineCountMsg — Delphi g_sSendOnlineCountMsg（M2Share.pas:3135）。
func (c *ServerConfig) GetSendOnlineCountMsg() string {
	if c.Game.SendOnlineCountMsg != "" {
		return c.Game.SendOnlineCountMsg
	}
	return "当前在线人数: %c"
}

// GetMsgRateLimit — 每连接入站消息速率（条/秒，路线图 6.3 网关补偿层）。
func (c *ServerConfig) GetMsgRateLimit() float64 {
	if c.Game.MsgRateLimit > 0 {
		return c.Game.MsgRateLimit
	}
	return 60
}

// GetMsgBurst — 每连接消息令牌桶突发容量。
func (c *ServerConfig) GetMsgBurst() int {
	if c.Game.MsgBurst > 0 {
		return c.Game.MsgBurst
	}
	return 40
}

// --- Login getters ---

func (c *ServerConfig) GetLoginMaxAttempts() int {
	if c.Login.MaxAttempts > 0 {
		return c.Login.MaxAttempts
	}
	return 5
}

func (c *ServerConfig) GetLoginLockoutSecs() int {
	if c.Login.LockoutSecs > 0 {
		return c.Login.LockoutSecs
	}
	return 60
}

func (c *ServerConfig) GetMaxCharsPerAcct() int {
	if c.Login.MaxCharsPerAcct > 0 {
		return c.Login.MaxCharsPerAcct
	}
	return 2
}

func (c *ServerConfig) GetMinNameLen() int {
	if c.Login.MinNameLen > 0 {
		return c.Login.MinNameLen
	}
	return 3
}

func (c *ServerConfig) GetMaxNameLen() int {
	if c.Login.MaxNameLen > 0 {
		return c.Login.MaxNameLen
	}
	return 14
}

func (c *ServerConfig) GetMinAcctLen() int {
	if c.Login.MinAcctLen > 0 {
		return c.Login.MinAcctLen
	}
	return 3
}

func (c *ServerConfig) GetMaxAcctLen() int {
	if c.Login.MaxAcctLen > 0 {
		return c.Login.MaxAcctLen
	}
	return 10
}

func (c *ServerConfig) GetMinPassLen() int {
	if c.Login.MinPassLen > 0 {
		return c.Login.MinPassLen
	}
	return 3
}

func (c *ServerConfig) GetMaxPassLen() int {
	if c.Login.MaxPassLen > 0 {
		return c.Login.MaxPassLen
	}
	return 10
}

// --- Player getters ---

func (c *ServerConfig) GetMaxBagSlots() int {
	if c.Player.MaxBagSlots > 0 {
		return c.Player.MaxBagSlots
	}
	return 46
}

func (c *ServerConfig) GetMaxStorageSlots() int {
	if c.Player.MaxStorageSlots > 0 {
		return c.Player.MaxStorageSlots
	}
	// Delphi 运行时上限 39（ObjBase.pas:24720 Count<39）；
	// 存档数组 TStorageItems[0..49]（Grobal2.pas:811）只是序列化容量。
	return 39
}

func (c *ServerConfig) GetMaxGold() int {
	if c.Player.MaxGold > 0 {
		return c.Player.MaxGold
	}
	return 10000000 // Delphi nHumanMaxGold（M2Share.pas:1763）
}

func (c *ServerConfig) GetMaxTradeItems() int {
	if c.Player.MaxTradeItems > 0 {
		return c.Player.MaxTradeItems
	}
	return 12
}

func (c *ServerConfig) GetMaxFriends() int {
	if c.Player.MaxFriends > 0 {
		return c.Player.MaxFriends
	}
	return 50
}

func (c *ServerConfig) GetMaxApprentices() int {
	if c.Player.MaxApprentices > 0 {
		return c.Player.MaxApprentices
	}
	return 3
}

func (c *ServerConfig) GetMaxSlaves() int {
	if c.Player.MaxSlaves > 0 {
		return c.Player.MaxSlaves
	}
	return 2
}

func (c *ServerConfig) GetMaxSlaveLevel() int {
	if c.Player.MaxSlaveLevel > 0 {
		return c.Player.MaxSlaveLevel
	}
	return 7
}

func (c *ServerConfig) GetBonusPerLevel() int {
	if c.Player.BonusPerLevel > 0 {
		return c.Player.BonusPerLevel
	}
	return 3
}

func (c *ServerConfig) GetViewRange() int {
	if c.Player.ViewRange > 0 {
		return c.Player.ViewRange
	}
	return 12
}

func (c *ServerConfig) GetVisionInterval() int64 {
	if c.Player.VisionInterval > 0 {
		return c.Player.VisionInterval
	}
	return 2000
}

func (c *ServerConfig) GetVisionRand() int64 {
	if c.Player.VisionRand > 0 {
		return c.Player.VisionRand
	}
	return 2000
}

func (c *ServerConfig) GetDeathSkeletonDelay() int64 {
	if c.Player.DeathSkeletonDelay > 0 {
		return c.Player.DeathSkeletonDelay
	}
	return 10000
}

func (c *ServerConfig) GetAutoReviveDelay() int64 {
	if c.Player.AutoReviveDelay > 0 {
		return c.Player.AutoReviveDelay
	}
	return 180000
}

func (c *ServerConfig) GetReviveHPRatio() int {
	if c.Player.ReviveHPRatio > 0 {
		return c.Player.ReviveHPRatio
	}
	return 2
}

func (c *ServerConfig) GetHPRegenInterval() int64 {
	if c.Player.HPRegenInterval > 0 {
		return c.Player.HPRegenInterval
	}
	return 10000
}

func (c *ServerConfig) GetMPRegenInterval() int64 {
	if c.Player.MPRegenInterval > 0 {
		return c.Player.MPRegenInterval
	}
	return 10000
}

func (c *ServerConfig) GetHPRegenRatio() int {
	if c.Player.HPRegenRatio > 0 {
		return c.Player.HPRegenRatio
	}
	return 20
}

func (c *ServerConfig) GetMPRegenRatio() int {
	if c.Player.MPRegenRatio > 0 {
		return c.Player.MPRegenRatio
	}
	return 15
}

func (c *ServerConfig) GetDropEquipRate() int {
	if c.Player.DropEquipRate > 0 {
		return c.Player.DropEquipRate
	}
	return 30
}

func (c *ServerConfig) GetDropEquipRateRed() int {
	if c.Player.DropEquipRateRed > 0 {
		return c.Player.DropEquipRateRed
	}
	return 15
}

func (c *ServerConfig) GetDropBagRate() int {
	if c.Player.DropBagRate > 0 {
		return c.Player.DropBagRate
	}
	return 10
}

func (c *ServerConfig) GetExpPenaltyRatio() int {
	if c.Player.ExpPenaltyRatio > 0 {
		return c.Player.ExpPenaltyRatio
	}
	return 20
}

func (c *ServerConfig) GetNPCInteractDist() int {
	if c.Player.NPCInteractDist > 0 {
		return c.Player.NPCInteractDist
	}
	return 15
}

func (c *ServerConfig) GetSpeedDecayInterval() int64 {
	if c.Player.SpeedDecayInterval > 0 {
		return c.Player.SpeedDecayInterval
	}
	return 10000
}

func (c *ServerConfig) GetBonusHPPerPoint() int {
	if c.Player.BonusHPPerPoint > 0 {
		return c.Player.BonusHPPerPoint
	}
	return 5
}

func (c *ServerConfig) GetBonusMPPerPoint() int {
	if c.Player.BonusMPPerPoint > 0 {
		return c.Player.BonusMPPerPoint
	}
	return 5
}

// --- Combat getters ---

func (c *ServerConfig) GetFireHitWindow() int64 {
	if c.Combat.FireHitWindow > 0 {
		return c.Combat.FireHitWindow
	}
	return 20000
}

func (c *ServerConfig) GetFireHitCooldown() int64 {
	if c.Combat.FireHitCooldown > 0 {
		return c.Combat.FireHitCooldown
	}
	return 10000
}

func (c *ServerConfig) GetTwinHitCooldown() int64 {
	if c.Combat.TwinHitCooldown > 0 {
		return c.Combat.TwinHitCooldown
	}
	return 60000
}

func (c *ServerConfig) GetMagicShieldRatio() int {
	if c.Combat.MagicShieldRatio > 0 {
		return c.Combat.MagicShieldRatio
	}
	return 150
}

func (c *ServerConfig) GetRedPoisonBonus() int {
	if c.Combat.RedPoisonBonus > 0 {
		return c.Combat.RedPoisonBonus
	}
	return 5
}

func (c *ServerConfig) GetParalysisDenom() int {
	if c.Combat.ParalysisDenom > 0 {
		return c.Combat.ParalysisDenom
	}
	return 5
}

func (c *ServerConfig) GetParalysisDuration() int64 {
	if c.Combat.ParalysisDuration > 0 {
		return c.Combat.ParalysisDuration
	}
	return 50
}

func (c *ServerConfig) GetMapEnterProtect() int64 {
	if c.Combat.MapEnterProtect > 0 {
		return c.Combat.MapEnterProtect
	}
	return 3000
}

func (c *ServerConfig) GetPKProtectLevel() int {
	if c.Combat.PKProtectLevel > 0 {
		return c.Combat.PKProtectLevel
	}
	return 7
}

func (c *ServerConfig) GetPKProtectDiff() int {
	if c.Combat.PKProtectDiff > 0 {
		return c.Combat.PKProtectDiff
	}
	return 10
}

func (c *ServerConfig) GetPKRedProtectLevel() int {
	if c.Combat.PKRedProtectLevel > 0 {
		return c.Combat.PKRedProtectLevel
	}
	return 15
}

func (c *ServerConfig) GetProjectileDelay() int64 {
	if c.Combat.ProjectileDelay > 0 {
		return c.Combat.ProjectileDelay
	}
	return 600
}

func (c *ServerConfig) GetMaxLOSCheck() int {
	if c.Combat.MaxLOSCheck > 0 {
		return c.Combat.MaxLOSCheck
	}
	return 13
}

// --- PK getters ---

func (c *ServerConfig) GetPKPointsPerKill() int {
	if c.PK.PointsPerKill > 0 {
		return c.PK.PointsPerKill
	}
	return 100
}

func (c *ServerConfig) GetPKDecayInterval() int64 {
	if c.PK.DecayInterval > 0 {
		return c.PK.DecayInterval
	}
	return 120000
}

func (c *ServerConfig) GetPKDecayAmount() int {
	if c.PK.DecayAmount > 0 {
		return c.PK.DecayAmount
	}
	return 1
}

func (c *ServerConfig) GetSelfDefenseDuration() int64 {
	if c.PK.SelfDefenseDuration > 0 {
		return c.PK.SelfDefenseDuration
	}
	return 60000
}

func (c *ServerConfig) GetPKWeaponDuraLoss() int {
	if c.PK.WeaponDuraLoss > 0 {
		return c.PK.WeaponDuraLoss
	}
	return 100
}

// --- Monster getters ---

func (c *ServerConfig) GetAITickInterval() int64 {
	if c.Monster.AITickInterval > 0 {
		return c.Monster.AITickInterval
	}
	return 250
}

func (c *ServerConfig) GetCorpseDelay() int64 {
	if c.Monster.CorpseDelay > 0 {
		return c.Monster.CorpseDelay
	}
	return 180000
}

func (c *ServerConfig) GetGhostDelay() int64 {
	if c.Monster.GhostDelay > 0 {
		return c.Monster.GhostDelay
	}
	return 300000
}

func (c *ServerConfig) GetMonHPRegenInterval() int64 {
	if c.Monster.HPRegenInterval > 0 {
		return c.Monster.HPRegenInterval
	}
	return 6000
}

func (c *ServerConfig) GetMonHPRegenDivisor() int {
	if c.Monster.HPRegenDivisor > 0 {
		return c.Monster.HPRegenDivisor
	}
	return 75
}

func (c *ServerConfig) GetOverlapThinkInterval() int64 {
	if c.Monster.OverlapThinkInterval > 0 {
		return c.Monster.OverlapThinkInterval
	}
	return 3000
}

func (c *ServerConfig) GetSearchInterval() int64 {
	if c.Monster.SearchInterval > 0 {
		return c.Monster.SearchInterval
	}
	return 3000
}

func (c *ServerConfig) GetSearchRand() int64 {
	if c.Monster.SearchRand > 0 {
		return c.Monster.SearchRand
	}
	return 2000
}

func (c *ServerConfig) GetSearchNoTarget() int64 {
	if c.Monster.SearchNoTarget > 0 {
		return c.Monster.SearchNoTarget
	}
	return 1000
}

func (c *ServerConfig) GetSearchHasTarget() int64 {
	if c.Monster.SearchHasTarget > 0 {
		return c.Monster.SearchHasTarget
	}
	return 8000
}

func (c *ServerConfig) GetTargetFocusTimeout() int64 {
	if c.Monster.TargetFocusTimeout > 0 {
		return c.Monster.TargetFocusTimeout
	}
	return 30000
}

func (c *ServerConfig) GetTargetLossDist() int {
	if c.Monster.TargetLossDist > 0 {
		return c.Monster.TargetLossDist
	}
	return 15
}

func (c *ServerConfig) GetDefaultViewRange() int {
	if c.Monster.DefaultViewRange > 0 {
		return c.Monster.DefaultViewRange
	}
	return 5
}

func (c *ServerConfig) GetDefaultWalkStep() int {
	if c.Monster.DefaultWalkStep > 0 {
		return c.Monster.DefaultWalkStep
	}
	return 3
}

func (c *ServerConfig) GetDefaultWalkWait() int64 {
	if c.Monster.DefaultWalkWait > 0 {
		return c.Monster.DefaultWalkWait
	}
	return 1000
}

func (c *ServerConfig) GetDefaultWalkSpeed() int64 {
	if c.Monster.DefaultWalkSpeed > 0 {
		return c.Monster.DefaultWalkSpeed
	}
	return 1400
}

func (c *ServerConfig) GetDefaultAttackSpeed() int64 {
	if c.Monster.DefaultAttackSpeed > 0 {
		return c.Monster.DefaultAttackSpeed
	}
	return 2000
}

func (c *ServerConfig) GetMinWalkSpeed() int64 {
	if c.Monster.MinWalkSpeed > 0 {
		return c.Monster.MinWalkSpeed
	}
	return 200
}

func (c *ServerConfig) GetAttackSpeedOffset() int64 {
	if c.Monster.AttackSpeedOffset > 0 {
		return c.Monster.AttackSpeedOffset
	}
	return 900
}

func (c *ServerConfig) GetSpawnMaxTries() int {
	if c.Monster.SpawnMaxTries > 0 {
		return c.Monster.SpawnMaxTries
	}
	return 31
}

func (c *ServerConfig) GetSlaveTeleportDist() int {
	if c.Monster.SlaveTeleportDist > 0 {
		return c.Monster.SlaveTeleportDist
	}
	return 20
}

func (c *ServerConfig) GetSlaveFollowDist() int {
	if c.Monster.SlaveFollowDist > 0 {
		return c.Monster.SlaveFollowDist
	}
	return 3
}

func (c *ServerConfig) GetStruckPenaltyBase() int {
	if c.Monster.StruckPenaltyBase > 0 {
		return c.Monster.StruckPenaltyBase
	}
	return 150
}

func (c *ServerConfig) GetStruckPenaltyLevel() int {
	if c.Monster.StruckPenaltyLevel > 0 {
		return c.Monster.StruckPenaltyLevel
	}
	return 130
}

func (c *ServerConfig) GetStruckPenaltyMin() int {
	if c.Monster.StruckPenaltyMin > 0 {
		return c.Monster.StruckPenaltyMin
	}
	return 20
}

func (c *ServerConfig) GetTargetSwitchChance() int {
	if c.Monster.TargetSwitchChance > 0 {
		return c.Monster.TargetSwitchChance
	}
	return 6
}

func (c *ServerConfig) GetRangedDistance() int {
	if c.Monster.RangedDistance > 0 {
		return c.Monster.RangedDistance
	}
	return 5
}

func (c *ServerConfig) GetAreaDistance() int {
	if c.Monster.AreaDistance > 0 {
		return c.Monster.AreaDistance
	}
	return 6
}

func (c *ServerConfig) GetGuardMaxHP() int {
	if c.Monster.GuardMaxHP > 0 {
		return c.Monster.GuardMaxHP
	}
	return 65535
}

func (c *ServerConfig) GetGuardViewRange() int {
	if c.Monster.GuardViewRange > 0 {
		return c.Monster.GuardViewRange
	}
	return 7
}

// --- MonsterAI getters ---

func (c *ServerConfig) GetBurrowTriggerDist() int {
	if c.MonsterAI.BurrowTriggerDist > 0 {
		return c.MonsterAI.BurrowTriggerDist
	}
	return 3
}

func (c *ServerConfig) GetReburrowDist() int {
	if c.MonsterAI.ReburrowDist > 0 {
		return c.MonsterAI.ReburrowDist
	}
	return 4
}

func (c *ServerConfig) GetExplodeTimer() int64 {
	if c.MonsterAI.ExplodeTimer > 0 {
		return c.MonsterAI.ExplodeTimer
	}
	return 60000
}

func (c *ServerConfig) GetExplodePowerDiv() int {
	if c.MonsterAI.ExplodePowerDiv > 0 {
		return c.MonsterAI.ExplodePowerDiv
	}
	return 2
}

func (c *ServerConfig) GetExplodePowerMin() int {
	if c.MonsterAI.ExplodePowerMin > 0 {
		return c.MonsterAI.ExplodePowerMin
	}
	return 50
}

func (c *ServerConfig) GetDualAxeRange() int {
	if c.MonsterAI.DualAxeRange > 0 {
		return c.MonsterAI.DualAxeRange
	}
	return 7
}

func (c *ServerConfig) GetLeechRange() int {
	if c.MonsterAI.LeechRange > 0 {
		return c.MonsterAI.LeechRange
	}
	return 2
}

func (c *ServerConfig) GetLeechBoostRatio() int {
	if c.MonsterAI.LeechBoostRatio > 0 {
		return c.MonsterAI.LeechBoostRatio
	}
	return 150
}

func (c *ServerConfig) GetCritRange() int {
	if c.MonsterAI.CritRange > 0 {
		return c.MonsterAI.CritRange
	}
	return 7
}

func (c *ServerConfig) GetCritChance() int {
	if c.MonsterAI.CritChance > 0 {
		return c.MonsterAI.CritChance
	}
	return 4
}

func (c *ServerConfig) GetFireballRange() int {
	if c.MonsterAI.FireballRange > 0 {
		return c.MonsterAI.FireballRange
	}
	return 8
}

func (c *ServerConfig) GetSpitRange() int {
	if c.MonsterAI.SpitRange > 0 {
		return c.MonsterAI.SpitRange
	}
	return 2
}

func (c *ServerConfig) GetPulseRange() int {
	if c.MonsterAI.PulseRange > 0 {
		return c.MonsterAI.PulseRange
	}
	return 16
}

func (c *ServerConfig) GetLightningRange() int {
	if c.MonsterAI.LightningRange > 0 {
		return c.MonsterAI.LightningRange
	}
	return 8
}

func (c *ServerConfig) GetCloneThreshold() int {
	if c.MonsterAI.CloneThreshold > 0 {
		return c.MonsterAI.CloneThreshold
	}
	return 3
}

func (c *ServerConfig) GetCloneCooldown() int64 {
	if c.MonsterAI.CloneCooldown > 0 {
		return c.MonsterAI.CloneCooldown
	}
	return 20000
}

func (c *ServerConfig) GetSummonMaxMinions() int {
	if c.MonsterAI.SummonMaxMinions > 0 {
		return c.MonsterAI.SummonMaxMinions
	}
	return 3
}

func (c *ServerConfig) GetHiveMaxChildren() int {
	if c.MonsterAI.HiveMaxChildren > 0 {
		return c.MonsterAI.HiveMaxChildren
	}
	return 15
}

func (c *ServerConfig) GetCentipedeCooldown() int64 {
	if c.MonsterAI.CentipedeCooldown > 0 {
		return c.MonsterAI.CentipedeCooldown
	}
	return 10000
}

func (c *ServerConfig) GetCentipedeAoEInterval() int64 {
	if c.MonsterAI.CentipedeAoEInterval > 0 {
		return c.MonsterAI.CentipedeAoEInterval
	}
	return 3000
}

func (c *ServerConfig) GetZumaMaxSlaves() int {
	if c.MonsterAI.ZumaMaxSlaves > 0 {
		return c.MonsterAI.ZumaMaxSlaves
	}
	return 30
}

func (c *ServerConfig) GetCowKingStunSpeed() int64 {
	if c.MonsterAI.CowKingStunSpeed > 0 {
		return c.MonsterAI.CowKingStunSpeed
	}
	return 10000
}

func (c *ServerConfig) GetCowKingRageDuration() int64 {
	if c.MonsterAI.CowKingRageDuration > 0 {
		return c.MonsterAI.CowKingRageDuration
	}
	return 8000
}

func (c *ServerConfig) GetCowKingBerserkAtk() int64 {
	if c.MonsterAI.CowKingBerserkAtk > 0 {
		return c.MonsterAI.CowKingBerserkAtk
	}
	return 500
}

func (c *ServerConfig) GetCowKingBerserkWalk() int64 {
	if c.MonsterAI.CowKingBerserkWalk > 0 {
		return c.MonsterAI.CowKingBerserkWalk
	}
	return 400
}

func (c *ServerConfig) GetFireAuraDuration() int64 {
	if c.MonsterAI.FireAuraDuration > 0 {
		return c.MonsterAI.FireAuraDuration
	}
	return 20000
}

func (c *ServerConfig) GetTransformCooldown() int64 {
	if c.MonsterAI.TransformCooldown > 0 {
		return c.MonsterAI.TransformCooldown
	}
	return 5000
}

func (c *ServerConfig) GetBoneKingCooldown() int64 {
	if c.MonsterAI.BoneKingCooldown > 0 {
		return c.MonsterAI.BoneKingCooldown
	}
	return 15000
}

func (c *ServerConfig) GetBoneKingMaxChildren() int {
	if c.MonsterAI.BoneKingMaxChildren > 0 {
		return c.MonsterAI.BoneKingMaxChildren
	}
	return 8
}

func (c *ServerConfig) GetFleeChance() int {
	if c.MonsterAI.FleeChance > 0 {
		return c.MonsterAI.FleeChance
	}
	return 30
}

// --- Magic getters ---

func (c *ServerConfig) GetFireWallDuration() int64 {
	if c.Magic.FireWallDuration > 0 {
		return c.Magic.FireWallDuration
	}
	return 8000
}

func (c *ServerConfig) GetHealingPoolCap() int {
	if c.Magic.HealingPoolCap > 0 {
		return c.Magic.HealingPoolCap
	}
	return 300
}

func (c *ServerConfig) GetTrainThreshold0() int {
	if c.Magic.TrainThreshold0 > 0 {
		return c.Magic.TrainThreshold0
	}
	return 20
}

func (c *ServerConfig) GetTrainThreshold1() int {
	if c.Magic.TrainThreshold1 > 0 {
		return c.Magic.TrainThreshold1
	}
	return 50
}

func (c *ServerConfig) GetTrainThreshold2() int {
	if c.Magic.TrainThreshold2 > 0 {
		return c.Magic.TrainThreshold2
	}
	return 100
}

func (c *ServerConfig) GetSummonPetHPBase() int {
	if c.Magic.SummonPetHPBase > 0 {
		return c.Magic.SummonPetHPBase
	}
	return 50
}

func (c *ServerConfig) GetSummonPetHPPerLv() int {
	if c.Magic.SummonPetHPPerLv > 0 {
		return c.Magic.SummonPetHPPerLv
	}
	return 5
}

func (c *ServerConfig) GetSummonPetDCBase() int {
	if c.Magic.SummonPetDCBase > 0 {
		return c.Magic.SummonPetDCBase
	}
	return 5
}

func (c *ServerConfig) GetSummonPetDCPerLv() int {
	if c.Magic.SummonPetDCPerLv > 0 {
		return c.Magic.SummonPetDCPerLv
	}
	return 2
}

func (c *ServerConfig) GetAngelHPBase() int {
	if c.Magic.AngelHPBase > 0 {
		return c.Magic.AngelHPBase
	}
	return 80
}

func (c *ServerConfig) GetAngelHPPerLv() int {
	if c.Magic.AngelHPPerLv > 0 {
		return c.Magic.AngelHPPerLv
	}
	return 8
}

func (c *ServerConfig) GetAngelDCBase() int {
	if c.Magic.AngelDCBase > 0 {
		return c.Magic.AngelDCBase
	}
	return 8
}

func (c *ServerConfig) GetAngelDCPerLv() int {
	if c.Magic.AngelDCPerLv > 0 {
		return c.Magic.AngelDCPerLv
	}
	return 1
}

// --- Economy getters ---

func (c *ServerConfig) GetGuildCreateCost() int {
	if c.Economy.GuildCreateCost > 0 {
		return c.Economy.GuildCreateCost
	}
	return 1000000
}

func (c *ServerConfig) GetGuildWarDuration() int64 {
	if c.Economy.GuildWarDuration > 0 {
		return c.Economy.GuildWarDuration
	}
	return 10800000
}

func (c *ServerConfig) GetUpgradeFee() int {
	if c.Economy.UpgradeFee > 0 {
		return c.Economy.UpgradeFee
	}
	return 10000
}

func (c *ServerConfig) GetUpgradeWaitTime() int64 {
	if c.Economy.UpgradeWaitTime > 0 {
		return c.Economy.UpgradeWaitTime
	}
	return 3600000
}

func (c *ServerConfig) GetUpgradeMaxPoints() int {
	if c.Economy.UpgradeMaxPoints > 0 {
		return c.Economy.UpgradeMaxPoints
	}
	return 7
}

func (c *ServerConfig) GetUpgradeMaterial() string {
	if c.Economy.UpgradeMaterial != "" {
		return c.Economy.UpgradeMaterial
	}
	return "黑铁矿"
}

func (c *ServerConfig) GetUpgradeCurseChance() int {
	if c.Economy.UpgradeCurseChance > 0 {
		return c.Economy.UpgradeCurseChance
	}
	return 30
}

func (c *ServerConfig) GetRepairDuraDivisor() int {
	if c.Economy.RepairDuraDivisor > 0 {
		return c.Economy.RepairDuraDivisor
	}
	return 30
}

func (c *ServerConfig) GetSpecialRepairMult() int {
	if c.Economy.SpecialRepairMult > 0 {
		return c.Economy.SpecialRepairMult
	}
	return 3
}

func (c *ServerConfig) GetCastleDiscount() int {
	if c.Economy.CastleDiscount > 0 {
		return c.Economy.CastleDiscount
	}
	return 80
}

func (c *ServerConfig) GetCastleMinRate() int {
	if c.Economy.CastleMinRate > 0 {
		return c.Economy.CastleMinRate
	}
	return 60
}

func (c *ServerConfig) GetCastleTaxRate() int {
	if c.Economy.CastleTaxRate > 0 {
		return c.Economy.CastleTaxRate
	}
	return 5
}

func (c *ServerConfig) GetDrugBasePrice() int {
	if c.Economy.DrugBasePrice > 0 {
		return c.Economy.DrugBasePrice
	}
	return 500
}

func (c *ServerConfig) GetSpouseRecallCooldown() int64 {
	if c.Economy.SpouseRecallCooldown > 0 {
		return c.Economy.SpouseRecallCooldown
	}
	return 60000
}

func (c *ServerConfig) GetDoorCloseDelay() int64 {
	if c.Economy.DoorCloseDelay > 0 {
		return c.Economy.DoorCloseDelay
	}
	return 5000
}

func (c *ServerConfig) GetPileStonesDuration() int64 {
	if c.Economy.PileStonesDuration > 0 {
		return c.Economy.PileStonesDuration
	}
	return 300000
}

// --- Drop getters ---

var defaultEquipUpgChances = []int{15, 24, 20, 30, 40}

func (c *ServerConfig) GetEquipUpgChances() []int {
	if len(c.Drop.EquipUpgChances) > 0 {
		return c.Drop.EquipUpgChances
	}
	return defaultEquipUpgChances
}

func (c *ServerConfig) GetEquipDuraMin() int {
	if c.Drop.EquipDuraMin > 0 {
		return c.Drop.EquipDuraMin
	}
	return 20
}

func (c *ServerConfig) GetEquipDuraRand() int {
	if c.Drop.EquipDuraRand > 0 {
		return c.Drop.EquipDuraRand
	}
	return 80
}

func (c *ServerConfig) GetAddValueChance() int {
	if c.Drop.AddValueChance > 0 {
		return c.Drop.AddValueChance
	}
	return 10
}

func (c *ServerConfig) GetMaxGoldPiles() int {
	if c.Drop.MaxGoldPiles > 0 {
		return c.Drop.MaxGoldPiles
	}
	return 17
}

func (c *ServerConfig) GetMaxGoldPerPile() int {
	if c.Drop.MaxGoldPerPile > 0 {
		return c.Drop.MaxGoldPerPile
	}
	return 2000
}

func (c *ServerConfig) GetControlDropItem() bool {
	return c.Drop.ControlDropItem
}

func (c *ServerConfig) GetGroundItemDespawnMs() int64 {
	if c.Drop.GroundItemDespawnMs > 0 {
		return c.Drop.GroundItemDespawnMs
	}
	return 3600000 // Delphi dwClearDropOnFloorItemTime（M2Share.pas:1797）
}

func (c *ServerConfig) GetFallbackGoldRate() int {
	if c.Drop.FallbackGoldRate > 0 {
		return c.Drop.FallbackGoldRate
	}
	return 30
}

func (c *ServerConfig) GetFallbackItemRate() int {
	if c.Drop.FallbackItemRate > 0 {
		return c.Drop.FallbackItemRate
	}
	return 10
}

func (c *ServerConfig) GetFallbackExp() int {
	if c.Drop.FallbackExp > 0 {
		return c.Drop.FallbackExp
	}
	return 10
}

// --- Mining getters ---

func (c *ServerConfig) GetMiningStoneRate() int {
	if c.Mining.StoneRate > 0 {
		return c.Mining.StoneRate
	}
	return 4
}

func (c *ServerConfig) GetMiningOreRate() int {
	if c.Mining.OreRate > 0 {
		return c.Mining.OreRate
	}
	return 12
}

func (c *ServerConfig) GetMiningDuraLoss() int {
	if c.Mining.DuraLoss > 0 {
		return c.Mining.DuraLoss
	}
	return 100
}

// --- Game (extra) getters ---

func (c *ServerConfig) GetGroupMembersMax() int {
	if c.Game.GroupMembersMax > 0 {
		return c.Game.GroupMembersMax
	}
	return 11
}

func (c *ServerConfig) GetGuildWarFee() int {
	if c.Game.GuildWarFee > 0 {
		return c.Game.GuildWarFee
	}
	return 500000
}

func (c *ServerConfig) GetBuildGuild() int {
	if c.Game.BuildGuild > 0 {
		return c.Game.BuildGuild
	}
	return 1000000
}
