package main

import (
	"time"
)

// AnimationPlayer controls animation playback.
type AnimationPlayer struct {
	// Configuration
	action    ActionInfo
	direction int     // 方向(0-7)
	speed     float64 // 速度倍率

	// State
	playing   bool
	frameIdx  int       // 当前帧索引
	lastUpdate time.Time
	frames    []int     // 计算后的帧序列
}

// NewAnimationPlayer creates a new animation player.
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

// Play starts the animation.
func (p *AnimationPlayer) Play() {
	p.playing = true
	p.lastUpdate = time.Now()
}

// Pause pauses the animation.
func (p *AnimationPlayer) Pause() {
	p.playing = false
}

// Stop stops the animation and resets to first frame.
func (p *AnimationPlayer) Stop() {
	p.playing = false
	p.frameIdx = 0
}

// NextFrame advances to the next frame.
func (p *AnimationPlayer) NextFrame() {
	if len(p.frames) == 0 {
		return
	}
	p.frameIdx = (p.frameIdx + 1) % len(p.frames)
}

// PrevFrame goes back to the previous frame.
func (p *AnimationPlayer) PrevFrame() {
	if len(p.frames) == 0 {
		return
	}
	p.frameIdx = (p.frameIdx - 1 + len(p.frames)) % len(p.frames)
}

// SetDirection changes the animation direction.
func (p *AnimationPlayer) SetDirection(dir int) {
	if dir < 0 || dir > 7 {
		return
	}
	p.direction = dir
	p.frames = CalcFrames(p.action, dir)
	p.frameIdx = 0
}

// SetSpeed changes the playback speed.
func (p *AnimationPlayer) SetSpeed(speed float64) {
	if speed <= 0 {
		return
	}
	p.speed = speed
}

// GetCurrentFrame returns the current frame index.
func (p *AnimationPlayer) GetCurrentFrame() int {
	if len(p.frames) == 0 {
		return 0
	}
	return p.frames[p.frameIdx]
}

// IsPlaying returns whether the animation is playing.
func (p *AnimationPlayer) IsPlaying() bool {
	return p.playing
}

// GetDirection returns the current direction.
func (p *AnimationPlayer) GetDirection() int {
	return p.direction
}

// GetSpeed returns the current speed.
func (p *AnimationPlayer) GetSpeed() float64 {
	return p.speed
}

// GetFrameCount returns the total number of frames.
func (p *AnimationPlayer) GetFrameCount() int {
	return len(p.frames)
}

// GetFrameIndex returns the current frame index in the sequence.
func (p *AnimationPlayer) GetFrameIndex() int {
	return p.frameIdx
}

// Update updates the animation state based on elapsed time.
func (p *AnimationPlayer) Update() {
	if !p.playing || len(p.frames) == 0 {
		return
	}

	now := time.Now()
	elapsed := now.Sub(p.lastUpdate)
	p.lastUpdate = now

	// Calculate how many frames to advance
	interval := time.Duration(float64(p.action.Interval) / p.speed * float64(time.Millisecond))
	if elapsed >= interval {
		framesToAdvance := int(elapsed / interval)
		if framesToAdvance > 0 {
			p.frameIdx = (p.frameIdx + framesToAdvance) % len(p.frames)
		}
	}
}
