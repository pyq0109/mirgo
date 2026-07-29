package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

type MagicEffectType int

const (
	EffExplosion    MagicEffectType = 0
	EffFly          MagicEffectType = 1
	EffGround       MagicEffectType = 2
	EffFlyAxe       MagicEffectType = 3
	EffFireGun      MagicEffectType = 5
	EffLightning    MagicEffectType = 6
	EffIce          MagicEffectType = 7
	EffBujaukExplo  MagicEffectType = 8
	EffBujaukGround MagicEffectType = 9
	EffFlyArrow     MagicEffectType = 11
	EffReady        MagicEffectType = 12
	EffThunder2     MagicEffectType = 13
	EffFlyBug       MagicEffectType = 14
)

type LightSource struct {
	X, Y  float64
	Level int
}

type MagicEffect struct {
	Type             MagicEffectType
	X, Y             float64
	StartX, StartY   float64
	TargetX, TargetY float64
	Frame            int
	MaxFrame         int
	FrameTime        int64
	LastTick         int64
	BaseIdx          int
	ImgLib           int
	Done             bool
	Light            int
	Dir16            int
	ExplosionBase    int
	Exploding        bool
}

func flyDir16(sx, sy, tx, ty float64) int {
	fx := tx - sx
	fy := ty - sy
	if fx == 0 {
		if fy < 0 {
			return 0
		}
		return 8
	}
	if fy == 0 {
		if fx < 0 {
			return 12
		}
		return 4
	}
	switch {
	case fx > 0 && fy < 0:
		r := 4
		if -fy > fx/4 {
			r = 3
		}
		if -fy > fx/1.9 {
			r = 2
		}
		if -fy > fx*1.4 {
			r = 1
		}
		if -fy > fx*4 {
			r = 0
		}
		return r
	case fx > 0 && fy > 0:
		r := 4
		if fy > fx/4 {
			r = 5
		}
		if fy > fx/1.9 {
			r = 6
		}
		if fy > fx*1.4 {
			r = 7
		}
		if fy > fx*4 {
			r = 8
		}
		return r
	case fx < 0 && fy > 0:
		r := 12
		if fy > -fx/4 {
			r = 11
		}
		if fy > -fx/1.9 {
			r = 10
		}
		if fy > -fx*1.4 {
			r = 9
		}
		if fy > -fx*4 {
			r = 8
		}
		return r
	default:
		r := 12
		if -fy > -fx/4 {
			r = 13
		}
		if -fy > -fx/1.9 {
			r = 14
		}
		if -fy > -fx*1.4 {
			r = 15
		}
		if -fy > -fx*4 {
			r = 0
		}
		return r
	}
}

type EffectManager struct {
	effects []*MagicEffect
}

func NewEffectManager() *EffectManager {
	return &EffectManager{}
}

// AddExplosion 在目标点播放爆炸动画。effNum 为魔法 EffectNumber，
// 帧基址 = EffectBase[effNum-1] + EXPLOSIONBASE(170)。
func (em *EffectManager) AddExplosion(x, y float64, effNum, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffExplosion, X: x, Y: y,
		BaseIdx: effectBaseIdx(effNum-1) + 170, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
	})
}

// AddFly 发射飞行弹道。effNum 为魔法 EffectNumber，
// 飞行帧基址 = EffectBase[effNum-1] + FLYBASE(10)，到达后切换爆炸。
func (em *EffectManager) AddFly(sx, sy, tx, ty float64, effNum, maxFrame int, frameTime int64) {
	base := effectBaseIdx(effNum - 1)
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFly, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: base + 10, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
		Dir16: flyDir16(sx, sy, tx, ty), ExplosionBase: base + 170,
	})
}

// AddGround 在目标点播放地面效果。帧基址 = EffectBase[effNum-1]。
func (em *EffectManager) AddGround(x, y float64, effNum, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffGround, X: x, Y: y,
		BaseIdx: effectBaseIdx(effNum - 1), MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 3,
	})
}

func (em *EffectManager) AddLightning(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffLightning, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 4,
	})
}

func (em *EffectManager) AddFireGun(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFireGun, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) AddIce(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffIce, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2, ImgLib: 1,
	})
}

func (em *EffectManager) AddFlyAxe(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyAxe, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) AddFlyArrow(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyArrow, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 0,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

// AddReady 在施法者位置播放蓄力动画。帧基址 = EffectBase[effNum-1]。
func (em *EffectManager) AddReady(x, y float64, effNum, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffReady, X: x, Y: y,
		BaseIdx: effectBaseIdx(effNum - 1), MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
	})
}

func (em *EffectManager) AddBujaukExplo(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffBujaukExplo, X: x, Y: y,
		BaseIdx: 1160, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
	})
}

func (em *EffectManager) AddBujaukGround(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffBujaukGround, X: x, Y: y,
		BaseIdx: 1160, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 3,
	})
}

func (em *EffectManager) AddThunder2(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffThunder2, X: x, Y: y,
		BaseIdx: 140, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2, ImgLib: 1,
	})
}

func (em *EffectManager) AddFlyBug(sx, sy, tx, ty float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyBug, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: 447, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) Update(now int64) {
	alive := em.effects[:0]
	for _, eff := range em.effects {
		if now-eff.LastTick >= eff.FrameTime {
			eff.LastTick = now
			eff.Frame++
			switch eff.Type {
			case EffFly, EffFlyAxe, EffFlyArrow, EffFireGun, EffFlyBug:
				if !eff.Exploding && eff.MaxFrame > 0 {
					t := float64(eff.Frame) / float64(eff.MaxFrame)
					if t >= 1.0 {
						t = 1.0
						if eff.ExplosionBase > 0 {
							eff.Exploding = true
							eff.Frame = 0
							eff.MaxFrame = 10
							eff.BaseIdx = eff.ExplosionBase
							break
						}
					}
					eff.X = eff.StartX + (eff.TargetX-eff.StartX)*t
					eff.Y = eff.StartY + (eff.TargetY-eff.StartY)*t
				}
			}
		}
		if eff.Frame >= eff.MaxFrame {
			eff.Done = true
		}
		if !eff.Done {
			alive = append(alive, eff)
		}
	}
	em.effects = alive
}

func (em *EffectManager) Render(glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32) {
	for _, eff := range em.effects {
		var idx int
		switch {
		case eff.Exploding:
			idx = eff.BaseIdx + eff.Frame
		case eff.Type == EffFlyAxe || eff.Type == EffFlyBug:
			idx = eff.BaseIdx + eff.Dir16*10 + eff.Frame
		case eff.Type == EffFly && !eff.Exploding:
			idx = eff.BaseIdx + eff.Dir16*10 + eff.Frame
		case eff.Type == EffFlyArrow:
			idx = eff.BaseIdx + eff.Dir16 + eff.Frame
		default:
			idx = eff.BaseIdx + eff.Frame
		}

		var wilFile *wil.File
		if eff.ImgLib == 0 {
			wilFile = resources.Magic
		} else {
			wilFile = resources.Magic2
		}
		if wilFile == nil || idx < 0 || idx >= wilFile.Count {
			continue
		}

		img := wilFile.GetImage(idx)
		if img == nil || img.RGBA == nil {
			continue
		}

		tex := resources.GetTexture(wilFile, idx)
		if tex == 0 {
			continue
		}

		w := float32(img.Width)
		h := float32(img.Height)
		drawX := float32(eff.X) - w/2
		drawY := float32(eff.Y) - h/2

		switch eff.Type {
		case EffFlyAxe, EffFlyArrow, EffFlyBug:
			if !eff.Exploding {
				glState.DrawQuad(tex, drawX, drawY, w, h, proj)
			} else {
				gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
				glState.DrawQuad(tex, drawX, drawY, w, h, proj)
				gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
			}
		default:
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
			glState.DrawQuad(tex, drawX, drawY, w, h, proj)
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		}
	}
}

func (em *EffectManager) LightSources() []LightSource {
	var lights []LightSource
	for _, eff := range em.effects {
		if eff.Light > 0 {
			lights = append(lights, LightSource{X: eff.X, Y: eff.Y, Level: eff.Light})
		}
	}
	return lights
}
