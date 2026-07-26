package main

import (
	"github.com/pyq0109/mirgo/internal/protocol"
)

type GameState struct {
	MySelf   *Actor
	Actors   *ActorManager
	UseItems [13]*protocol.UserItem
	MagicList []protocol.UserMagic
	DayBright int
	MapName   string
}

func NewGameState() *GameState {
	return &GameState{
		Actors: NewActorManager(),
	}
}

func (gs *GameState) Reset() {
	gs.MySelf = nil
	gs.Actors.Clear()
}
