package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MapEvent struct {
	ServerID int32
	Type     int
	X, Y     int
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
			e.broadcastHideEvent(server, ev)
			continue
		}
		if ev.Type == 1 && ev.Damage > 0 {
			obj := e.GetMovingObject(ev.X, ev.Y)
			switch o := obj.(type) {
			case *MonsterObject:
				if !o.Death {
					hp := int(o.WAbil.HP) - ev.Damage
					if hp <= 0 {
						o.Death = true
						o.DeathTick = now
						o.WAbil.HP = 0
						e.broadcastDeathMsg(o.BaseObject, o.ID, o.CurrX, o.CurrY, o.Dir, true)
						if owner := e.getPlayerByID(ev.OwnerID); owner != nil {
							owner.awardExp(server, o)
						}
					} else {
						o.WAbil.HP = uint16(hp)
					}
				}
			case *PlayObject:
				// 火墙对玩家造成伤害但不会致死：将 HP 钳制在 1 可以避免
				// 死亡/PK/掉落流程，地图事件无法干净地驱动这些流程。
				if !o.Ghost && !o.Death {
					hp := int(o.WAbil.HP) - ev.Damage
					if hp < 1 {
						hp = 1
					}
					o.WAbil.HP = uint16(hp)
					o.sendHealthSpell(server)
				}
			}
		}
		alive = append(alive, ev)
	}
	e.Events = alive
}

func (e *Environment) AddFireEvent(server *netserver.TCPServer, x, y, damage int, durationMs int64, ownerID int32) {
	e.eventIDSeq++
	ev := &MapEvent{
		ServerID: e.eventIDSeq,
		Type:     1,
		X:        x,
		Y:        y,
		Damage:   damage,
		Duration: durationMs,
		EndTick:  time.Now().UnixMilli() + durationMs,
		OwnerID:  ownerID,
	}
	e.Events = append(e.Events, ev)
	e.broadcastShowEvent(server, ev)
}

func (e *Environment) broadcastShowEvent(server *netserver.TCPServer, ev *MapEvent) {
	resp := protocol.MakeDefaultMsg(protocol.SMShowEvent, ev.ServerID, uint16(ev.Type), uint16(ev.X), uint16(ev.Y))
	e.sendToRangePlayers(server, ev.X, ev.Y, resp)
}

func (e *Environment) broadcastHideEvent(server *netserver.TCPServer, ev *MapEvent) {
	resp := protocol.MakeDefaultMsg(protocol.SMHideEvent, ev.ServerID, 0, 0, 0)
	e.sendToRangePlayers(server, ev.X, ev.Y, resp)
}

func (e *Environment) sendToRangePlayers(server *netserver.TCPServer, x, y int, resp protocol.DefaultMessage) {
	objs := e.GetRangeObjects(x, y, viewRange)
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost {
			continue
		}
		server.Send(p.Session.ID, resp, "")
	}
}
