package main

// ActionInfo defines an animation action template.
type ActionInfo struct {
	Start    int // 起始帧索引
	Frame    int // 帧数
	Skip     int // 跳帧数
	Interval int // 帧间隔(ms)
}

// 人类角色动作模板 (根据 Delphi 源码 Actor.pas)
var HumanActions = map[string]ActionInfo{
	"stand":  {Start: 0, Frame: 4, Skip: 0, Interval: 200},
	"walk":   {Start: 64, Frame: 60, Skip: 0, Interval: 100},
	"run":    {Start: 128, Frame: 60, Skip: 0, Interval: 80},
	"attack": {Start: 192, Frame: 60, Skip: 0, Interval: 80},
	"spell":  {Start: 256, Frame: 60, Skip: 0, Interval: 80},
	"hit":    {Start: 320, Frame: 30, Skip: 0, Interval: 100},
	"death":  {Start: 384, Frame: 30, Skip: 0, Interval: 150},
}

// 怪物动作模板 (根据 Delphi 源码 Actor.pas)
// 每种怪物类型有不同的动作模板
var MonsterActions = map[int]map[string]ActionInfo{
	// MA9 - 基础怪物（无方向）
	9: {
		"stand":  {Start: 0, Frame: 10, Skip: 0, Interval: 200},
		"walk":   {Start: 10, Frame: 10, Skip: 0, Interval: 100},
		"attack": {Start: 20, Frame: 10, Skip: 0, Interval: 100},
		"hit":    {Start: 30, Frame: 10, Skip: 0, Interval: 100},
		"death":  {Start: 40, Frame: 10, Skip: 0, Interval: 150},
	},
	// MA10 - 基础怪物（带方向）
	10: {
		"stand":  {Start: 0, Frame: 10, Skip: 0, Interval: 200},
		"walk":   {Start: 80, Frame: 10, Skip: 0, Interval: 100},
		"attack": {Start: 160, Frame: 10, Skip: 0, Interval: 100},
		"hit":    {Start: 240, Frame: 10, Skip: 0, Interval: 100},
		"death":  {Start: 320, Frame: 10, Skip: 0, Interval: 150},
	},
	// 可以继续添加更多怪物类型...
}

// NpcActions NPC动作模板 (根据 Delphi 源码 Actor.pas)
var NpcActions = map[string]ActionInfo{
	"stand": {Start: 0, Frame: 60, Skip: 0, Interval: 200},
}

// CalcFrames 计算某动作某方向的帧序列
func CalcFrames(action ActionInfo, direction int) []int {
	dirFrames := action.Frame
	start := action.Start

	// 对于帧数>=8的动作，按8方向分配；否则所有方向共享帧序列
	if action.Frame >= 8 && direction >= 0 && direction < 8 {
		dirFrames = action.Frame / 8
		start = action.Start + direction*dirFrames
	}

	// 生成帧序列
	frames := make([]int, 0, dirFrames)
	for i := 0; i < dirFrames; i++ {
		// 跳帧处理
		if action.Skip > 0 && i%(action.Skip+1) == action.Skip {
			continue
		}
		frames = append(frames, start+i)
	}
	return frames
}
