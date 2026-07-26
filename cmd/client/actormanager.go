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
		return actors[i].Ry < actors[j].Ry
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
			raceImg := byte(feature & 0xFF)
			if raceImg == 0 {
				actor.Type = ActorHuman
			} else {
				actor.Type = ActorMonster
				actor.Appearance = int(uint16((feature >> 16) & 0xFFFF))
				actor.MonAction = GetRaceByPM(int(raceImg))
			}
		}
	}

	if actor.Type == ActorHuman && actor.Dress == 0 && actor.Hair == 0 && actor.Weapon == 0 {
		actor.Type = ActorHuman
	}

	return actor
}
