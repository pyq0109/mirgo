package main

// ActionInfo 定义动画帧信息（对应 Delphi TActionInfo）。
type ActionInfo struct {
	Start   int // 起始帧索引
	Frame   int // 有效帧数
	Skip    int // 跳过帧数（方向间的填充）
	FTime   int // 帧显示时间（毫秒）
	UseTick int // 移动 tick 控制
}

// HumanAction 包含人物角色的 14 个动作。
type HumanAction struct {
	ActStand       ActionInfo
	ActWalk        ActionInfo
	ActRun         ActionInfo
	ActRushLeft    ActionInfo
	ActRushRight   ActionInfo
	ActWarMode     ActionInfo
	ActHit         ActionInfo
	ActHeavyHit    ActionInfo
	ActBigHit      ActionInfo
	ActFireHitReady ActionInfo
	ActSpell       ActionInfo
	ActSitdown     ActionInfo
	ActStruck      ActionInfo
	ActDie         ActionInfo
}

// MonsterAction 包含怪物角色的 7 个动作。
type MonsterAction struct {
	ActStand   ActionInfo
	ActWalk    ActionInfo
	ActAttack  ActionInfo
	ActCritical ActionInfo
	ActStruck  ActionInfo
	ActDie     ActionInfo
	ActDeath   ActionInfo
}

// HA 是人物动作模板（对应 Delphi HA 常量）。
var HA = HumanAction{
	ActStand:       ActionInfo{Start: 0, Frame: 4, Skip: 4, FTime: 200, UseTick: 0},
	ActWalk:        ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 90, UseTick: 2},
	ActRun:         ActionInfo{Start: 128, Frame: 6, Skip: 2, FTime: 120, UseTick: 3},
	ActRushLeft:    ActionInfo{Start: 128, Frame: 3, Skip: 5, FTime: 120, UseTick: 3},
	ActRushRight:   ActionInfo{Start: 131, Frame: 3, Skip: 5, FTime: 120, UseTick: 3},
	ActWarMode:     ActionInfo{Start: 192, Frame: 1, Skip: 0, FTime: 200, UseTick: 0},
	ActHit:         ActionInfo{Start: 200, Frame: 6, Skip: 2, FTime: 85, UseTick: 0},
	ActHeavyHit:    ActionInfo{Start: 264, Frame: 6, Skip: 2, FTime: 90, UseTick: 0},
	ActBigHit:      ActionInfo{Start: 328, Frame: 8, Skip: 0, FTime: 70, UseTick: 0},
	ActFireHitReady: ActionInfo{Start: 192, Frame: 6, Skip: 4, FTime: 70, UseTick: 0},
	ActSpell:       ActionInfo{Start: 392, Frame: 6, Skip: 2, FTime: 60, UseTick: 0},
	ActSitdown:     ActionInfo{Start: 456, Frame: 2, Skip: 0, FTime: 300, UseTick: 0},
	ActStruck:      ActionInfo{Start: 472, Frame: 3, Skip: 5, FTime: 70, UseTick: 0},
	ActDie:         ActionInfo{Start: 536, Frame: 4, Skip: 4, FTime: 120, UseTick: 0},
}

// 常量
const (
	HumanFrame    = 600
	MonFrame      = 280
	ExpMonFrame   = 360
	SculMonFrame  = 440
	MerchantFrame = 60
)

// CalcFrame 计算动作的帧索引范围。
func CalcFrame(action ActionInfo, dir int) (startFrame, endFrame int) {
	startFrame = action.Start + dir*(action.Frame+action.Skip)
	endFrame = startFrame + action.Frame - 1
	return
}

// GetRaceByPM 根据种族返回怪物动作模板。
func GetRaceByPM(race int, appr int) *MonsterAction {
	switch race {
	case 9:
		return &MA9
	case 10:
		return &MA10
	case 11:
		return &MA11
	case 12, 24:
		return &MA12
	case 13, 14, 17, 18, 23:
		return &MA14
	case 15, 22:
		return &MA15
	case 16:
		return &MA16
	case 19, 20, 21, 37, 40, 45, 52, 53, 64, 65, 66, 67, 68, 69, 73, 74, 79:
		return &MA19
	case 30, 31:
		return &MA17
	case 41, 42:
		return &MA20
	case 48, 49:
		return &MA23
	case 32:
		return &MA24
	case 33:
		return &MA25
	case 34, 90:
		return &MA30
	case 35:
		return &MA31
	case 36:
		return &MA32
	case 43:
		return &MA21
	case 47:
		return &MA22
	case 50:
		return getRaceByPM50(appr)
	case 54:
		return &MA28
	case 55:
		return &MA29
	case 60, 61, 62, 70, 71, 72:
		return &MA33
	case 63:
		return &MA34
	case 75, 77:
		return &MA39
	case 76:
		return &MA38
	case 78:
		return &MA40
	case 80:
		return &MA42
	case 81:
		return &MA43
	case 83:
		return &MA44
	case 84, 85, 86, 87, 88, 89:
		return &MA45
	case 98:
		return &MA27
	case 99:
		return &MA26
	default:
		return &MA19
	}
}

func getRaceByPM50(appr int) *MonsterAction {
	switch {
	case appr == 23:
		return &MA36
	case appr == 24 || appr == 25 || appr == 27 || appr == 32:
		return &MA37
	case appr >= 35 && appr <= 41:
		return &MA41
	case appr >= 42 && appr <= 47:
		return &MA46
	case appr >= 48 && appr <= 50 || appr >= 52 && appr <= 53:
		return &MA41
	default:
		return &MA35
	}
}

// 怪物动作模板（对应 Delphi MA9-MA47）
var MA9 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 1, Skip: 7, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 120, UseTick: 3},
	ActAttack:  ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 150, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 1, Skip: 7, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 1, Skip: 7, FTime: 0, UseTick: 0},
}

var MA10 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 4, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 120, UseTick: 3},
	ActAttack:  ActionInfo{Start: 128, Frame: 4, Skip: 4, FTime: 150, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 192, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 208, Frame: 4, Skip: 4, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 272, Frame: 1, Skip: 0, FTime: 0, UseTick: 0},
}

var MA11 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 120, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 1, Skip: 0, FTime: 0, UseTick: 0},
}

var MA12 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 4, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 120, UseTick: 3},
	ActAttack:  ActionInfo{Start: 128, Frame: 6, Skip: 2, FTime: 150, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 192, Frame: 2, Skip: 0, FTime: 150, UseTick: 0},
	ActDie:     ActionInfo{Start: 208, Frame: 4, Skip: 4, FTime: 160, UseTick: 0},
	ActDeath:   ActionInfo{Start: 272, Frame: 1, Skip: 0, FTime: 0, UseTick: 0},
}

var MA14 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 120, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 100, UseTick: 0},
}

var MA15 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 120, UseTick: 0},
	ActDeath:   ActionInfo{Start: 1, Frame: 1, Skip: 0, FTime: 100, UseTick: 0},
}

var MA16 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 160, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 4, Skip: 6, FTime: 160, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 1, Skip: 0, FTime: 160, UseTick: 0},
}

var MA17 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 60, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 100, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 1, Skip: 0, FTime: 140, UseTick: 0},
}

var MA19 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 1, Skip: 0, FTime: 140, UseTick: 0},
}

var MA20 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 100, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 170, UseTick: 0},
}

var MA21 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 20, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 30, Frame: 10, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA22 = MonsterAction{
	ActStand:   ActionInfo{Start: 80, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 240, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 320, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 160, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 0, UseTick: 0},
}

var MA23 = MonsterAction{
	ActStand:   ActionInfo{Start: 20, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 100, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 180, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 260, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 280, Frame: 10, Skip: 0, FTime: 160, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 20, Skip: 0, FTime: 100, UseTick: 0},
}

var MA24 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 240, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActStruck:  ActionInfo{Start: 320, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 420, Frame: 1, Skip: 0, FTime: 140, UseTick: 0},
}

var MA25 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 70, Frame: 10, Skip: 0, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 20, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 50, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 50, Frame: 20, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
}

var MA26 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 1, Skip: 7, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 56, Frame: 6, Skip: 2, FTime: 500, UseTick: 0},
	ActCritical: ActionInfo{Start: 64, Frame: 6, Skip: 2, FTime: 500, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 4, Skip: 4, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 24, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA27 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 1, Skip: 7, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA28 = MonsterAction{
	ActStand:   ActionInfo{Start: 80, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 0, UseTick: 0},
}

var MA29 = MonsterAction{
	ActStand:   ActionInfo{Start: 80, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 240, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 320, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 140, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 0, UseTick: 0},
}

var MA30 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
	ActAttack:  ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 20, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 30, Frame: 20, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
}

var MA31 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
	ActAttack:  ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 2, Skip: 8, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 20, Frame: 10, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
}

var MA32 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 2, Skip: 8, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 80, Frame: 10, Skip: 0, FTime: 80, UseTick: 0},
	ActDeath:   ActionInfo{Start: 80, Frame: 10, Skip: 0, FTime: 200, UseTick: 3},
}

var MA33 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 200, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 340, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 200, UseTick: 0},
	ActDeath:   ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 200, UseTick: 0},
}

var MA34 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 200, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 320, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 400, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 420, Frame: 20, Skip: 0, FTime: 200, UseTick: 0},
	ActDeath:   ActionInfo{Start: 420, Frame: 20, Skip: 0, FTime: 200, UseTick: 0},
}

var MA35 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 30, Frame: 10, Skip: 0, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA36 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 30, Frame: 20, Skip: 0, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA37 = MonsterAction{
	ActStand:   ActionInfo{Start: 30, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 30, Frame: 4, Skip: 6, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA38 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA39 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 300, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 20, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 30, Frame: 10, Skip: 0, FTime: 80, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA40 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 250, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 210, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 110, UseTick: 0},
	ActCritical: ActionInfo{Start: 580, Frame: 20, Skip: 0, FTime: 135, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 120, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 20, Skip: 0, FTime: 130, UseTick: 0},
	ActDeath:   ActionInfo{Start: 260, Frame: 20, Skip: 0, FTime: 130, UseTick: 0},
}

var MA41 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA42 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 10, Frame: 8, Skip: 2, FTime: 160, UseTick: 0},
	ActAttack:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDie:     ActionInfo{Start: 30, Frame: 10, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 30, Frame: 10, Skip: 0, FTime: 0, UseTick: 0},
}

var MA43 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 160, UseTick: 0},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 160, UseTick: 0},
	ActCritical: ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 100, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 260, Frame: 10, Skip: 0, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 340, Frame: 10, Skip: 0, FTime: 0, UseTick: 0},
}

var MA44 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 300, UseTick: 0},
	ActWalk:    ActionInfo{Start: 10, Frame: 6, Skip: 4, FTime: 160, UseTick: 3},
	ActAttack:  ActionInfo{Start: 20, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 40, Frame: 10, Skip: 0, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 40, Frame: 2, Skip: 8, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 30, Frame: 6, Skip: 4, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA45 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 300, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 10, Skip: 0, FTime: 300, UseTick: 3},
	ActAttack:  ActionInfo{Start: 10, Frame: 10, Skip: 0, FTime: 300, UseTick: 0},
	ActCritical: ActionInfo{Start: 10, Frame: 10, Skip: 0, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 150, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 1, Skip: 9, FTime: 0, UseTick: 0},
}

var MA46 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 20, Skip: 0, FTime: 100, UseTick: 0},
	ActWalk:    ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActAttack:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActCritical: ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActStruck:  ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDie:     ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
	ActDeath:   ActionInfo{Start: 0, Frame: 0, Skip: 0, FTime: 0, UseTick: 0},
}

var MA47 = MonsterAction{
	ActStand:   ActionInfo{Start: 0, Frame: 4, Skip: 6, FTime: 200, UseTick: 0},
	ActWalk:    ActionInfo{Start: 80, Frame: 6, Skip: 4, FTime: 200, UseTick: 3},
	ActAttack:  ActionInfo{Start: 160, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActCritical: ActionInfo{Start: 260, Frame: 6, Skip: 4, FTime: 120, UseTick: 0},
	ActStruck:  ActionInfo{Start: 240, Frame: 2, Skip: 0, FTime: 100, UseTick: 0},
	ActDie:     ActionInfo{Start: 524, Frame: 6, Skip: 0, FTime: 200, UseTick: 0},
	ActDeath:   ActionInfo{Start: 524, Frame: 6, Skip: 0, FTime: 200, UseTick: 0},
}
