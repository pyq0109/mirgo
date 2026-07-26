package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

type MagicEffectType int

const (
	EffExplosion MagicEffectType = iota
	EffFly
	EffGround
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
}

type EffectManager struct {
	effects []*MagicEffect
}

func NewEffectManager() *EffectManager {
	return &EffectManager{}
}

func (em *EffectManager) AddExplosion(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffExplosion, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
	})
}

func (em *EffectManager) AddFly(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFly, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
	})
}

func (em *EffectManager) AddGround(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffGround, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 3,
	})
}

func (em *EffectManager) Update(now int64) {
	alive := em.effects[:0]
	for _, eff := range em.effects {
		if now-eff.LastTick >= eff.FrameTime {
			eff.LastTick = now
			eff.Frame++
			if eff.Type == EffFly && eff.MaxFrame > 0 {
				t := float64(eff.Frame) / float64(eff.MaxFrame)
				if t > 1.0 {
					t = 1.0
				}
				eff.X = eff.StartX + (eff.TargetX-eff.StartX)*t
				eff.Y = eff.StartY + (eff.TargetY-eff.StartY)*t
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
		idx := eff.BaseIdx + eff.Frame

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

		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		glState.DrawQuad(tex, drawX, drawY, w, h, proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
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
