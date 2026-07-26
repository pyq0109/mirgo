package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
)

type MapEvent struct {
	Type     int
	X, Y    int
	Damage   int
	Duration int64
	EndTick  int64
	OwnerID  int32
}

func (e *Environment) ProcessMapEvents(server *netserver.TCPServer, now int64) {
	if len(e.Events) == 0 {
		return
	}
	alive := e.Events[:0]
	for _, ev := range e.Events {
		if now > ev.EndTick {
			continue
		}
		if ev.Type == 1 && ev.Damage > 0 {
			obj := e.GetMovingObject(ev.X, ev.Y)
			if obj != nil {
				if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
					hp := int(mon.WAbil.HP) - ev.Damage
					if hp <= 0 {
						mon.Death = true
						mon.DeathTick = now
						mon.WAbil.HP = 0
					} else {
						mon.WAbil.HP = uint16(hp)
					}
				}
			}
		}
		alive = append(alive, ev)
	}
	e.Events = alive
}

func (e *Environment) AddFireEvent(x, y, damage int, durationMs int64, ownerID int32) {
	e.Events = append(e.Events, &MapEvent{
		Type:    1,
		X:       x,
		Y:       y,
		Damage:  damage,
		EndTick: time.Now().UnixMilli() + durationMs,
		OwnerID: ownerID,
	})
}
