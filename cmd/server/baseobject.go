package main

import (
	"sync"

	"github.com/pyq0109/mirgo/internal/protocol"
)

// BaseObject is the base for all game objects on the server.
type BaseObject struct {
	// Identity
	Name string
	ID   int32

	// Position
	MapName  string
	CurrX    int
	CurrY    int
	Dir      int

	// Appearance
	Gender byte
	Hair   byte
	Job    byte

	// Stats
	Abil  protocol.Ability
	WAbil protocol.Ability // Working abilities (with equipment bonuses)

	// Combat
	HitPoint  int
	HitSpeed  int
	Luck      int

	// State
	StatusTimeArr [12]int16

	// Inventory
	UseItems [13]*protocol.UserItem // Equipped items
	ItemList []*protocol.UserItem   // Bag items

	// Magic
	MagicList []*protocol.UserMagic

	// Messages (thread-safe queue)
	msgList []SendMessage
	msgMu   sync.Mutex

	// Map reference
	envir *Environment
}

// SendMessage represents a message to be processed.
type SendMessage struct {
	Ident    int
	Param1   int
	Param2   int
	Param3   int
	Msg      string
}

// NewBaseObject creates a new base object.
func NewBaseObject(name string, id int32) *BaseObject {
	return &BaseObject{
		Name: name,
		ID:   id,
	}
}

// SendMsg adds a message to the object's message queue.
func (o *BaseObject) SendMsg(ident, param1, param2, param3 int, msg string) {
	o.msgMu.Lock()
	o.msgList = append(o.msgList, SendMessage{
		Ident:  ident,
		Param1: param1,
		Param2: param2,
		Param3: param3,
		Msg:    msg,
	})
	o.msgMu.Unlock()
}

// GetMsg retrieves and removes the next message from the queue.
func (o *BaseObject) GetMsg() (SendMessage, bool) {
	o.msgMu.Lock()
	defer o.msgMu.Unlock()

	if len(o.msgList) == 0 {
		return SendMessage{}, false
	}

	msg := o.msgList[0]
	o.msgList = o.msgList[1:]
	return msg, true
}

// Feature returns the appearance feature integer.
func (o *BaseObject) Feature() int32 {
	return protocol.MakeHumanFeature(0, byte(o.Gender), 0, byte(o.Hair))
}

// WalkTo moves the object in the given direction.
func (o *BaseObject) WalkTo(dir int) bool {
	if o.envir == nil {
		return false
	}

	dx, dy := dirToOffset(dir)
	newX := o.CurrX + dx
	newY := o.CurrY + dy

	if !o.envir.CanWalk(newX, newY) {
		return false
	}

	// Remove from old position
	o.envir.RemoveObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)

	// Update position
	o.CurrX = newX
	o.CurrY = newY
	o.Dir = dir

	// Add to new position
	o.envir.AddObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, o)

	return true
}

// TurnTo changes the object's direction.
func (o *BaseObject) TurnTo(dir int) {
	o.Dir = dir
}

// dirToOffset converts a direction to dx, dy offsets.
func dirToOffset(dir int) (dx, dy int) {
	switch dir {
	case 0: // Up
		return 0, -1
	case 1: // Up-Right
		return 1, -1
	case 2: // Right
		return 1, 0
	case 3: // Down-Right
		return 1, 1
	case 4: // Down
		return 0, 1
	case 5: // Down-Left
		return -1, 1
	case 6: // Left
		return -1, 0
	case 7: // Up-Left
		return -1, -1
	}
	return 0, 0
}
