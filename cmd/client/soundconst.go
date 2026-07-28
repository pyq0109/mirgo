package main

import "strings"

// BGM 路径常量（Delphi SoundUtil.pas:31-34）
const (
	bmgIntro    = "wav/log-in-long2.wav"
	bmgSelect   = "wav/sellect-loop2.wav"
	bmgGameover = "wav/game over2.wav"
)

// 脚步声索引（Delphi SoundUtil.pas:36-67）
// 布局：每种地表 4 个连续索引 = 走左/走右/跑左/跑右
const (
	sWalkGroundL = 1 + iota*4 // 土
	sWalkGroundR
	sRunGroundL
	sRunGroundR

	sWalkStoneL // 石
	sWalkStoneR
	sRunStoneL
	sRunStoneR

	sWalkLawnL // 草
	sWalkLawnR
	sRunLawnL
	sRunLawnR

	sWalkRoughL // 粗糙
	sWalkRoughR
	sRunRoughL
	sRunRoughR

	sWalkWoodL // 木
	sWalkWoodR
	sRunWoodL
	sRunWoodR

	sWalkCaveL // 洞
	sWalkCaveR
	sRunCaveL
	sRunCaveR

	sWalkRoomL // 房
	sWalkRoomR
	sRunRoomL
	sRunRoomR

	sWalkWaterL // 水
	sWalkWaterR
	sRunWaterL
	sRunWaterR
)

// 击打声（Delphi SoundUtil.pas:70-77）
const (
	sHitShort  = 50
	sHitWooden = 51
	sHitSword  = 52
	sHitDo     = 53
	sHitAxe    = 54
	sHitClub   = 55
	sHitLong   = 56
	sHitFist   = 57
)

// 受击武器声（Delphi SoundUtil.pas:79-84）
const (
	sStruckShort  = 60
	sStruckWooden = 61
	sStruckSword  = 62
	sStruckDo     = 63
	sStruckAxe    = 64
	sStruckClub   = 65
)

// 受击肉体声（Delphi SoundUtil.pas:86-89）
const (
	sStruckBodySword     = 70
	sStruckBodyAxe       = 71
	sStruckBodyLongstick = 72
	sStruckBodyFist      = 73
)

// 受击护甲声（Delphi SoundUtil.pas:91-94）
const (
	sStruckArmorSword     = 80
	sStruckArmorAxe       = 81
	sStruckArmorLongstick = 82
	sStruckArmorFist      = 83
)

// 石矿声（Delphi SoundUtil.pas:105-106）
const (
	sStrikeStone    = 91
	sDropStonepiece = 92
)

// 系统/UI 声（Delphi SoundUtil.pas:108-119）
const (
	sRockDoorOpen    = 100
	sMeltstone       = 101
	sMainTheme       = 102
	sNormButtonClick = 103
	sRockButtonClick = 104
	sGlassButtonClick = 105
	sMoney           = 106
	sEatDrug         = 107
	sClickDrug       = 108
	sSpacemoveOut    = 109
	sSpacemoveIn     = 110
)

// 物品点击声（Delphi SoundUtil.pas:121-128）
const (
	sClickWeapon   = 111
	sClickArmor    = 112
	sClickRing     = 113
	sClickArmring  = 114
	sClickNecklace = 115
	sClickHelmet   = 116
	sClickGrobes   = 117
	sItmClick      = 118
)

// 特殊攻击声（Delphi SoundUtil.pas:130-137）
const (
	sYedoMan      = 130
	sYedoWoman    = 131
	sLongHit      = 132
	sWideHit      = 133
	sRushL        = 134
	sRushR        = 135
	sFirehitReady = 136
	sFirehit      = 137
)

// 人声（Delphi SoundUtil.pas:139-142）
const (
	sManStruck = 138
	sWomStruck = 139
	sManDie    = 144
	sWomDie    = 145
)

// weaponSoundIdx 按武器类型返回挥击声索引（Delphi Actor.pas:2250-2262）。
func weaponSoundIdx(weapon int) int {
	switch weapon / 2 {
	case 6, 20:
		return sHitShort
	case 1:
		return sHitWooden
	case 2, 13, 9, 5, 14, 22:
		return sHitSword
	case 4, 17, 10, 15, 16, 23:
		return sHitDo
	case 3, 7, 11:
		return sHitAxe
	case 24:
		return sHitClub
	case 8, 12, 18, 21:
		return sHitLong
	default:
		return sHitFist
	}
}

// struckBodySoundIdx 按攻击者武器返回肉体受击声（Delphi Actor.pas:2283-2290）。
func struckBodySoundIdx(attackWeapon int) int {
	switch attackWeapon / 2 {
	case 6, 1, 2, 4, 5, 9, 10, 13, 14, 15, 16, 17:
		return sStruckBodySword
	case 3, 7, 11:
		return sStruckBodyAxe
	case 8, 12, 18:
		return sStruckBodyLongstick
	default:
		return sStruckBodyFist
	}
}

// struckArmorSoundIdx 按攻击者武器返回护甲受击声（Delphi Actor.pas:2275-2282）。
func struckArmorSoundIdx(attackWeapon int) int {
	switch attackWeapon / 2 {
	case 6, 1, 2, 4, 5, 9, 10, 13, 14, 15, 16, 17:
		return sStruckArmorSword
	case 3, 7, 11:
		return sStruckArmorAxe
	case 8, 12, 18:
		return sStruckArmorLongstick
	default:
		return sStruckArmorFist
	}
}

// struckWeaponSoundIdx 按攻击者武器返回受击武器声（Delphi Actor.pas:2335-2353）。
// 注意 Delphi 原文有双重 div：先 weapon/2 再 case attackweapon/2。
func struckWeaponSoundIdx(attackWeapon int) int {
	w := attackWeapon / 2
	switch w / 2 {
	case 6, 20:
		return sStruckShort
	case 1:
		return sStruckWooden
	case 2, 13, 9, 5, 14, 22:
		return sStruckSword
	case 4, 17, 10, 15, 16, 23:
		return sStruckDo
	case 3, 7, 11:
		return sStruckAxe
	case 24:
		return sStruckClub
	case 8, 12, 18, 21:
		return sStruckWooden // Delphi 原为 s_struck_long，已改为 wooden（Actor.pas:2349 注释 //long）
	default:
		return -1 // else 分支被注释（Actor.pas:2350）
	}
}

// itemClickSoundIdx 按物品 StdMode 返回点击声索引（Delphi SoundUtil.pas:293-310）。
func itemClickSoundIdx(stdMode byte, name string) int {
	switch stdMode {
	case 0, 31:
		return sClickDrug
	case 5, 6:
		return sClickWeapon
	case 10, 11:
		return sClickArmor
	case 22, 23:
		return sClickRing
	case 24, 26:
		// Delphi 按中文名子串判断（SoundUtil:300-305）
		if strings.Contains(name, "手镯") || strings.Contains(name, "手套") {
			return sClickGrobes
		}
		return sClickArmring
	case 19, 20, 21:
		return sClickNecklace
	case 15:
		return sClickHelmet
	default:
		return sItmClick
	}
}

// itemUseSoundIdx 按 StdMode 返回使用声索引（Delphi SoundUtil.pas:312-319）。
func itemUseSoundIdx(stdMode byte) int {
	switch stdMode {
	case 0:
		return sClickDrug
	case 1, 2:
		return sEatDrug
	default:
		return -1
	}
}

// terrainFootstepSound 按地图格数据返回脚步声基值（走左）。
// 对应 Delphi Actor.pas:2129-2238 的 SetSound 脚步声部分。
// bidx = area*10000 + (bkImg & 0x7FFF) - 1
func terrainFootstepSound(bkImg uint16, area uint8, midImg, frImg uint16) int {
	bidx := int(area)*10000 + int(bkImg&0x7FFF) - 1

	result := sWalkGroundL // 默认土

	// 主映射表（Actor.pas:2151-2196）
	switch {
	case inRanges(bidx, 330, 349, 450, 454, 550, 554, 750, 754, 950, 954,
		1250, 1254, 1400, 1424, 1455, 1474, 1500, 1524, 1550, 1574):
		result = sWalkLawnL
	case inRanges(bidx, 250, 254, 1005, 1009, 1050, 1054, 1060, 1064, 1450, 1454, 1650, 1654):
		result = sWalkRoughL
	case inRanges(bidx, 605, 609, 650, 654, 660, 664, 2000, 2049, 3025, 3049,
		2400, 2424, 4625, 4649, 4675, 4678):
		result = sWalkStoneL
	case inRanges(bidx, 1825, 1924, 2150, 2174, 3075, 3099, 3325, 3349, 3375, 3399):
		result = sWalkCaveL
	case bidx == 3230 || bidx == 3231 || bidx == 3246 || bidx == 3277:
		result = sWalkWoodL
	case inRanges(bidx, 3780, 3799):
		result = sWalkWoodL
	case inRanges(bidx, 3825, 4434):
		if (bidx-3825)%25 == 0 {
			result = sWalkWoodL
		} else {
			result = sWalkGroundL
		}
	case inRanges(bidx, 2075, 2099, 2125, 2149):
		result = sWalkRoomL
	case inRanges(bidx, 1800, 1824):
		result = sWalkWaterL
	}

	// 覆盖规则（Actor.pas:2199-2211）
	if inRanges(bidx, 825, 1349) && ((bidx-825)/25)%2 == 0 {
		result = sWalkStoneL
	}
	if inRanges(bidx, 1375, 1799) && ((bidx-1375)/25)%2 == 0 {
		result = sWalkCaveL
	}
	if bidx == 1385 || bidx == 1386 || bidx == 1391 || bidx == 1392 {
		result = sWalkWoodL
	}

	// 中层图修正（Actor.pas:2213-2219）
	midIdx := int(midImg&0x7FFF) - 1
	if midIdx >= 0 && midIdx <= 115 {
		result = sWalkGroundL
	} else if midIdx >= 120 && midIdx <= 124 {
		result = sWalkLawnL
	}

	// 前景图修正（Actor.pas:2222-2235）
	frIdx := int(frImg&0x7FFF) - 1
	if frIdx >= 0 {
		switch {
		case inRanges(frIdx, 221, 289, 583, 658, 1183, 1206, 7163, 7295, 7404, 7414):
			result = sWalkStoneL
		case inRanges(frIdx, 3125, 3267, 3757, 3948, 6030, 6999):
			result = sWalkWoodL
		case inRanges(frIdx, 3316, 3589):
			result = sWalkRoomL
		}
	}

	return result
}

// inRanges 检查 v 是否在任意 [lo, hi] 区间内。
// ranges 参数为 lo1, hi1, lo2, hi2, ... 的偶数长度序列。
func inRanges(v int, ranges ...int) bool {
	for i := 0; i+1 < len(ranges); i += 2 {
		if v >= ranges[i] && v <= ranges[i+1] {
			return true
		}
	}
	return false
}


