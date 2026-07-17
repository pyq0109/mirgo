package main

import (
	"sort"
	"sync"

	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// ActorManager manages all actors in the game world.
type ActorManager struct {
	actors map[int32]*Actor
	mu     sync.RWMutex
}

// NewActorManager creates a new actor manager.
func NewActorManager() *ActorManager {
	return &ActorManager{
		actors: make(map[int32]*Actor),
	}
}

// Add adds an actor to the manager.
func (m *ActorManager) Add(actor *Actor) {
	m.mu.Lock()
	m.actors[actor.RecogID] = actor
	m.mu.Unlock()
}

// Remove removes an actor from the manager.
func (m *ActorManager) Remove(recogID int32) {
	m.mu.Lock()
	delete(m.actors, recogID)
	m.mu.Unlock()
}

// Get returns an actor by ID.
func (m *ActorManager) Get(recogID int32) *Actor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.actors[recogID]
}

// Clear removes all actors.
func (m *ActorManager) Clear() {
	m.mu.Lock()
	m.actors = make(map[int32]*Actor)
	m.mu.Unlock()
}

// Update updates all actors.
func (m *ActorManager) Update(now int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, actor := range m.actors {
		actor.UpdateAnimation(now)
	}
}

// DrawSorted draws all actors sorted by Y position (for correct overlap).
func (m *ActorManager) DrawSorted(gl *engine.GLState, resources *engine.ResourceManager, cam *engine.Camera2D, proj [16]float32) {
	m.mu.RLock()
	actors := make([]*Actor, 0, len(m.actors))
	for _, a := range m.actors {
		actors = append(actors, a)
	}
	m.mu.RUnlock()

	// Sort by Y position
	sort.Slice(actors, func(i, j int) bool {
		return actors[i].CurrY < actors[j].CurrY
	})

	// Draw each actor
	for _, actor := range actors {
		screenX := float32(float64(actor.CurrX*engine.TileWidth) - cam.X + actor.ShiftX)
		screenY := float32(float64(actor.CurrY*engine.TileHeight) - cam.Y + actor.ShiftY)
		actor.Draw(gl, resources, screenX, screenY, proj)
	}
}

// NewActorFromMessage creates an actor from a server message.
func NewActorFromMessage(msg protocol.DefaultMessage, body string) *Actor {
	actor := NewActor(msg.Recog, int(msg.Param), int(msg.Tag), int(msg.Series))
	// TODO: Parse body for appearance info
	return actor
}
