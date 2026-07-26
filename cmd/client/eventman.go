package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
)

// fireBurnBase mirrors Delphi clEvent.pas ET_FIRE (non-CUSTOMLIB): the fire
// animation is 6 frames in Magic.wil (g_WMagicImages) starting at FIREBURNBASE,
// cycled as (frame/2) mod 6 with additive blending.
const fireBurnBase = 1630

type MapEvent struct {
	ServerID  int32
	X, Y      int
	EType     int
	Param     int
	Frame     int
	FrameTime int64
	LastTick  int64
}

type EventManager struct {
	events []*MapEvent
}

func NewEventManager() *EventManager {
	return &EventManager{}
}

func (em *EventManager) AddEvent(e *MapEvent) {
	for _, ev := range em.events {
		if ev.ServerID == e.ServerID {
			return
		}
	}
	e.LastTick = time.Now().UnixMilli()
	em.events = append(em.events, e)
}

func (em *EventManager) DelEventByID(serverID int32) {
	for i, ev := range em.events {
		if ev.ServerID == serverID {
			em.events = append(em.events[:i], em.events[i+1:]...)
			return
		}
	}
}

func (em *EventManager) Clear() {
	em.events = em.events[:0]
}

func (em *EventManager) Update(now int64) {
	for _, ev := range em.events {
		if ev.FrameTime > 0 && now-ev.LastTick >= ev.FrameTime {
			ev.LastTick = now
			ev.Frame++
		}
	}
}

func (em *EventManager) Render(glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32) {
	for _, ev := range em.events {
		px := float32(ev.X*engine.TileWidth + engine.TileWidth/2)
		py := float32(ev.Y*engine.TileHeight + engine.TileHeight/2)

		if ev.EType == 1 && resources.Magic != nil {
			idx := fireBurnBase + (ev.Frame/2)%6
			if idx >= 0 && idx < resources.Magic.Count {
				img := resources.Magic.GetImage(idx)
				if img != nil && img.RGBA != nil {
					if tex := resources.GetTexture(resources.Magic, idx); tex != 0 {
						w := float32(img.Width)
						h := float32(img.Height)
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
						glState.DrawQuad(tex, px-w/2, py-h/2, w, h, proj)
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
						continue
					}
				}
			}
		}

		glState.DrawQuadColor(px-engine.TileWidth/2, py-engine.TileHeight/2,
			engine.TileWidth, engine.TileHeight, 1.0, 0.4, 0.1, 0.5, proj)
	}
}
