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
		switch ev.Type {
		case protocol.ETFire:
			if ev.Damage > 0 {
				e.processFireDamage(server, ev, now)
			}
		case protocol.ETMine:
			// 矿石再生：到期后重置 MineCount（由 mining.go 驱动）
		case protocol.ETPileStones:
			// 纯视觉事件，仅等待过期
		case protocol.ETHolyCurtain:
			e.processHolyCurtain(server, ev, now)
		case protocol.ETSculPiece:
			// 装饰事件，仅等待过期
		}
		alive = append(alive, ev)
	}
	e.Events = alive
}

func (e *Environment) processFireDamage(server *netserver.TCPServer, ev *MapEvent, now int64) {
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

// processHolyCurtain 圣幕事件：对站在上面的亡灵/不死系怪物造成伤害。
func (e *Environment) processHolyCurtain(server *netserver.TCPServer, ev *MapEvent, now int64) {
	obj := e.GetMovingObject(ev.X, ev.Y)
	if mon, ok := obj.(*MonsterObject); ok && !mon.Death {
		if mon.LifeAttrib == LA_UNDEAD {
			hp := int(mon.WAbil.HP) - ev.Damage
			if hp <= 0 {
				mon.Death = true
				mon.DeathTick = now
				mon.WAbil.HP = 0
				e.broadcastDeathMsg(mon.BaseObject, mon.ID, mon.CurrX, mon.CurrY, mon.Dir, true)
				if owner := e.getPlayerByID(ev.OwnerID); owner != nil {
					owner.awardExp(server, mon)
				}
			} else {
				mon.WAbil.HP = uint16(hp)
			}
		}
	}
}

func (e *Environment) AddFireEvent(server *netserver.TCPServer, x, y, damage int, durationMs int64, ownerID int32) {
	e.eventIDSeq++
	ev := &MapEvent{
		ServerID: e.eventIDSeq,
		Type:     protocol.ETFire,
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

// AddHolyCurtainEvent 创建圣幕事件（阻挡移动 + 伤害亡灵）。
func (e *Environment) AddHolyCurtainEvent(server *netserver.TCPServer, x, y, damage int, durationMs int64, ownerID int32) {
	e.eventIDSeq++
	ev := &MapEvent{
		ServerID: e.eventIDSeq,
		Type:     protocol.ETHolyCurtain,
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

// AddPileStonesEvent 创建碎石堆视觉事件（5分钟过期，无游戏逻辑）。
func (e *Environment) AddPileStonesEvent(server *netserver.TCPServer, x, y int) {
	e.eventIDSeq++
	ev := &MapEvent{
		ServerID: e.eventIDSeq,
		Type:     protocol.ETPileStones,
		X:        x,
		Y:        y,
		Duration: 300000,
		EndTick:  time.Now().UnixMilli() + 300000,
	}
	e.Events = append(e.Events, ev)
	e.broadcastShowEvent(server, ev)
}

// AddMineEvent 创建矿石事件（采矿系统使用）。
func (e *Environment) AddMineEvent(server *netserver.TCPServer, x, y int, durationMs int64) {
	e.eventIDSeq++
	ev := &MapEvent{
		ServerID: e.eventIDSeq,
		Type:     protocol.ETMine,
		X:        x,
		Y:        y,
		Duration: durationMs,
		EndTick:  time.Now().UnixMilli() + durationMs,
	}
	e.Events = append(e.Events, ev)
	e.broadcastShowEvent(server, ev)
}

// HasHolyCurtainAt 检查指定位置是否有活跃的圣幕事件（用于碰撞检测）。
func (e *Environment) HasHolyCurtainAt(x, y int) bool {
	now := time.Now().UnixMilli()
	for _, ev := range e.Events {
		if ev.Type == protocol.ETHolyCurtain && ev.X == x && ev.Y == y && now <= ev.EndTick {
			return true
		}
	}
	return false
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
