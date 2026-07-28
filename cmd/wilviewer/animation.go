package main

import (
	"time"
)

// AnimationPlayer 控制动画播放。
type AnimationPlayer struct {
	// 配置
	action    ActionInfo
	direction int     // 方向(0-7)
	speed     float64 // 速度倍率

	// 状态
	playing   bool
	frameIdx  int       // 当前帧索引
	lastUpdate time.Time
	frames    []int     // 计算后的帧序列
}

// NewAnimationPlayer 创建新的动画播放器。
func NewAnimationPlayer(action ActionInfo, direction int) *AnimationPlayer {
	p := &AnimationPlayer{
		action:    action,
		direction: direction,
		speed:     1.0,
		playing:   false,
		frameIdx:  0,
		frames:    CalcFrames(action, direction),
	}
	return p
}

// Play 开始播放动画。
func (p *AnimationPlayer) Play() {
	p.playing = true
	p.lastUpdate = time.Now()
}

// Pause 暂停动画。
func (p *AnimationPlayer) Pause() {
	p.playing = false
}

// Stop 停止动画并重置到第一帧。
func (p *AnimationPlayer) Stop() {
	p.playing = false
	p.frameIdx = 0
}

// NextFrame 前进到下一帧。
func (p *AnimationPlayer) NextFrame() {
	if len(p.frames) == 0 {
		return
	}
	p.frameIdx = (p.frameIdx + 1) % len(p.frames)
}

// PrevFrame 回退到上一帧。
func (p *AnimationPlayer) PrevFrame() {
	if len(p.frames) == 0 {
		return
	}
	p.frameIdx = (p.frameIdx - 1 + len(p.frames)) % len(p.frames)
}

// SetDirection 更改动画方向。
func (p *AnimationPlayer) SetDirection(dir int) {
	if dir < 0 || dir > 7 {
		return
	}
	p.direction = dir
	p.frames = CalcFrames(p.action, dir)
	p.frameIdx = 0
}

// SetSpeed 更改播放速度。
func (p *AnimationPlayer) SetSpeed(speed float64) {
	if speed <= 0 {
		return
	}
	p.speed = speed
}

// GetCurrentFrame 返回当前帧索引。
func (p *AnimationPlayer) GetCurrentFrame() int {
	if len(p.frames) == 0 {
		return 0
	}
	return p.frames[p.frameIdx]
}

// IsPlaying 返回动画是否正在播放。
func (p *AnimationPlayer) IsPlaying() bool {
	return p.playing
}

// GetDirection 返回当前方向。
func (p *AnimationPlayer) GetDirection() int {
	return p.direction
}

// GetSpeed 返回当前速度。
func (p *AnimationPlayer) GetSpeed() float64 {
	return p.speed
}

// GetFrameCount 返回总帧数。
func (p *AnimationPlayer) GetFrameCount() int {
	return len(p.frames)
}

// GetFrameIndex 返回当前帧在序列中的索引。
func (p *AnimationPlayer) GetFrameIndex() int {
	return p.frameIdx
}

// Update 根据经过的时间更新动画状态。
func (p *AnimationPlayer) Update() {
	if !p.playing || len(p.frames) == 0 {
		return
	}

	now := time.Now()
	elapsed := now.Sub(p.lastUpdate)
	p.lastUpdate = now

	// 计算需要前进多少帧
	interval := time.Duration(float64(p.action.Interval) / p.speed * float64(time.Millisecond))
	if elapsed >= interval {
		framesToAdvance := int(elapsed / interval)
		if framesToAdvance > 0 {
			p.frameIdx = (p.frameIdx + framesToAdvance) % len(p.frames)
		}
	}
}
