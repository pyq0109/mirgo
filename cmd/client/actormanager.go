package main

import (
	"encoding/binary"
	"sort"
	"sync"

	"github.com/pyq0109/mirgo/internal/protocol"
)

type ActorManager struct {
	actors map[int32]*Actor
	mu     sync.RWMutex
}

func NewActorManager() *ActorManager {
	return &ActorManager{
		actors: make(map[int32]*Actor),
	}
}

func (m *ActorManager) Add(actor *Actor) {
	m.mu.Lock()
	m.actors[actor.RecogID] = actor
	m.mu.Unlock()
}

func (m *ActorManager) Remove(recogID int32) {
	m.mu.Lock()
	delete(m.actors, recogID)
	m.mu.Unlock()
}

func (m *ActorManager) Get(recogID int32) *Actor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.actors[recogID]
}

func (m *ActorManager) Clear() {
	m.mu.Lock()
	m.actors = make(map[int32]*Actor)
	m.mu.Unlock()
}

func (m *ActorManager) All() []*Actor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Actor, 0, len(m.actors))
	for _, a := range m.actors {
		result = append(result, a)
	}
	return result
}

func (m *ActorManager) Update(now int64, moveTick bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, actor := range m.actors {
		// Delphi 只对非自身角色设置消息积压加速标志（Actor.pas:2667）
		actor.MsgMuch = !actor.IsSelf && len(actor.MsgList) >= 2
		if moveTick {
			actor.LockEndFrame = false
		}
		actor.ProcMsg()
		if moveTick && actor.Move(now) {
			continue
		}
		actor.Run(now)
	}
}

func (m *ActorManager) SortedByY() []*Actor {
	m.mu.RLock()
	actors := make([]*Actor, 0, len(m.actors))
	for _, a := range m.actors {
		actors = append(actors, a)
	}
	m.mu.RUnlock()
	sort.Slice(actors, func(i, j int) bool {
		if actors[i].Ry != actors[j].Ry {
			return actors[i].Ry < actors[j].Ry
		}
		// 死亡角色在底层（先绘制），活人踩在上面——
		// ActorDied 把死者移到列表头部（PlayScn.pas:2151-2169）；
		// 渲染循环从头部开始绘制（:1229-1240）。
		if actors[i].Death != actors[j].Death {
			return actors[i].Death
		}
		// 确定性排序：map 迭代顺序是随机的，没有稳定键的话
		// 同行角色会跨帧交换绘制顺序（闪烁）。
		return actors[i].RecogID < actors[j].RecogID
	})
	return actors
}

func NewActorFromMessage(msg protocol.DefaultMessage, body string) *Actor {
	actor := NewActor(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series)&0xFF)
	actor.Rx = actor.CurrX
	actor.Ry = actor.CurrY

	if len(body) > 0 {
		raw := []byte(body)
		if len(raw) >= 4 {
			feature := int32(binary.LittleEndian.Uint32(raw[0:4]))
			_, dress, weapon, hair := protocol.ParseHumanFeature(feature)
			actor.Dress = int(dress)
			actor.Weapon = int(weapon)
			actor.Hair = int(hair)
			actor.Sex = int(hair) % 2
			raceImg := byte(feature & 0xFF)
			actor.Race = int(raceImg) // 保存种族值，用于点击判断和渲染路径选择
			if raceImg == 0 {
				actor.Type = ActorHuman
			} else if raceImg == protocol.RCNpc || raceImg == protocol.RCPeaceNpc || raceImg == protocol.RCMerchant {
				actor.Type = ActorNPC
				actor.Appearance = int(uint16((feature >> 16) & 0xFFFF))
			} else {
				actor.Type = ActorMonster
				actor.Appearance = int(uint16((feature >> 16) & 0xFFFF))
				actor.MonAction = GetRaceByPM(int(raceImg), actor.Appearance)
			}
		}
	}

	return actor
}
