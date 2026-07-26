package main

import (
	"github.com/pyq0109/mirgo/internal/mapformat"
)

// ObjectType constants
const (
	OS_EVENTOBJECT  = 1
	OS_MOVINGOBJECT = 2
	OS_ITEMOBJECT   = 3
	OS_GATEOBJECT   = 4
	OS_MAPEVENT     = 5
	OS_DOOR         = 6
)

// OSObject represents an object on the map.
type OSObject struct {
	Type byte
	Obj  interface{}
}

// MapCellInfo represents a single map cell.
type MapCellInfo struct {
	Flag    byte
	ObjList []OSObject
}

// MapFlag contains map properties.
type MapFlag struct {
	Safe     bool
	Fight    bool
	Dark     bool
	NoDrug   bool
	NoRecall bool
}

// Door represents a door on the map.
type Door struct {
	ID       byte
	X, Y     int
	State    byte
	OpenTick int64
}

// GroundItem represents an item on the ground.
type GroundItem struct {
	ID       int32
	Name     string
	Looks    int
	X, Y     int
	DropTick int64
	Gold     int
}

// Environment represents a single map.
type Environment struct {
	Name        string
	Width       int
	Height      int
	Cells       []MapCellInfo
	Flag        MapFlag
	Doors       []Door
	GroundItems []*GroundItem
	Events      []*MapEvent
	eventIDSeq  int32

	rawMap *mapformat.MapData
}

// NewEnvironment creates an environment from a map file.
func NewEnvironment(name string, m *mapformat.MapData) *Environment {
	env := &Environment{
		Name:   name,
		Width:  m.Width,
		Height: m.Height,
		Cells:  make([]MapCellInfo, m.Width*m.Height),
		rawMap: m,
	}

	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			info := m.InfoAt(x, y)
			if info == nil {
				continue
			}

			idx := y*m.Width + x
			env.Cells[idx].Flag = 0
			if info.Collision {
				env.Cells[idx].Flag = 1
			}

			if info.FrontDoorIndex&0x80 != 0 {
				env.Doors = append(env.Doors, Door{
					ID: info.FrontDoorIndex & 0x7F,
					X:  x,
					Y:  y,
				})
			}
		}
	}

	return env
}

// CanWalk checks if a position is walkable (terrain + entities + doors).
func (e *Environment) CanWalk(x, y int) bool {
	return e.CanWalkEx(x, y, false)
}

// CanWalkEx checks walkability. When ignoreEntities is true, only terrain and
// doors are checked — entity collision is skipped (Delphi CanWalkEx boFlag).
func (e *Environment) CanWalkEx(x, y int, ignoreEntities bool) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	if e.Cells[idx].Flag != 0 {
		return false
	}
	if e.rawMap != nil {
		info := e.rawMap.InfoAt(x, y)
		if info != nil && info.FrontDoorIndex&0x80 != 0 && info.FrontDoorOffset&0x80 == 0 {
			return false
		}
	}
	if ignoreEntities {
		return true
	}
	for _, o := range e.Cells[idx].ObjList {
		if o.Type != OS_MOVINGOBJECT {
			continue
		}
		switch obj := o.Obj.(type) {
		case *PlayObject:
			if !obj.Ghost && !obj.Death && !obj.Hidden {
				return false
			}
		}
	}
	return true
}

func (e *Environment) getPlayerByID(id int32) *PlayObject {
	for i := range e.Cells {
		for _, o := range e.Cells[i].ObjList {
			if o.Type != OS_MOVINGOBJECT {
				continue
			}
			if p, ok := o.Obj.(*PlayObject); ok && p.ID == id {
				return p
			}
		}
	}
	return nil
}

func (e *Environment) getNpcByID(id int32) (*NpcObject, bool) {
	for i := range e.Cells {
		for _, o := range e.Cells[i].ObjList {
			if o.Type != OS_MOVINGOBJECT {
				continue
			}
			if npc, ok := o.Obj.(*NpcObject); ok && npc.ID == id {
				return npc, true
			}
		}
	}
	return nil, false
}

func (e *Environment) getObjectByID(id int32) interface{} {
	for i := range e.Cells {
		for _, o := range e.Cells[i].ObjList {
			if o.Type != OS_MOVINGOBJECT {
				continue
			}
			switch obj := o.Obj.(type) {
			case *PlayObject:
				if obj.ID == id {
					return obj
				}
			case *MonsterObject:
				if obj.ID == id {
					return obj
				}
			case *NpcObject:
				if obj.ID == id {
					return obj
				}
			}
		}
	}
	return nil
}

func objectBase(obj interface{}) *BaseObject {
	switch o := obj.(type) {
	case *PlayObject:
		return o.BaseObject
	case *MonsterObject:
		return o.BaseObject
	case *NpcObject:
		return o.BaseObject
	}
	return nil
}

func objectFeature(obj interface{}) int32 {
	switch o := obj.(type) {
	case *PlayObject:
		return o.Feature()
	case *MonsterObject:
		return o.Feature()
	case *NpcObject:
		return o.Feature()
	}
	return 0
}

// objectFeatureEx: low byte = horse type, high byte = dress effect (Delphi MakeWord(HorseType, DressEffType)).
func objectFeatureEx(obj interface{}) int32 {
	if p, ok := obj.(*PlayObject); ok {
		return p.FeatureEx()
	}
	return 0
}

// AddObject adds an object to the map at the given position.
func (e *Environment) AddObject(x, y int, objType byte, obj interface{}) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	e.Cells[idx].ObjList = append(e.Cells[idx].ObjList, OSObject{
		Type: objType,
		Obj:  obj,
	})
	return true
}

// RemoveObject removes an object from the map.
func (e *Environment) RemoveObject(x, y int, objType byte, obj interface{}) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	cell := &e.Cells[idx]
	for i, o := range cell.ObjList {
		if o.Type == objType && o.Obj == obj {
			cell.ObjList = append(cell.ObjList[:i], cell.ObjList[i+1:]...)
			return true
		}
	}
	return false
}

// GetMovingObject returns the first moving object at the given position.
func (e *Environment) GetMovingObject(x, y int) interface{} {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return nil
	}
	idx := y*e.Width + x
	for _, o := range e.Cells[idx].ObjList {
		if o.Type == OS_MOVINGOBJECT {
			return o.Obj
		}
	}
	return nil
}

// GetRangeObjects returns all objects within a radius.
func (e *Environment) GetRangeObjects(x, y, radius int) []interface{} {
	var result []interface{}
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			cx, cy := x+dx, y+dy
			if cx < 0 || cx >= e.Width || cy < 0 || cy >= e.Height {
				continue
			}
			idx := cy*e.Width + cx
			for _, o := range e.Cells[idx].ObjList {
				result = append(result, o.Obj)
			}
		}
	}
	return result
}

// RawMap returns the underlying map data.
func (e *Environment) RawMap() *mapformat.MapData {
	return e.rawMap
}

func (e *Environment) getMonsterByBase(base *BaseObject) *MonsterObject {
	for i := range e.Cells {
		for _, o := range e.Cells[i].ObjList {
			if o.Type != OS_MOVINGOBJECT {
				continue
			}
			if mon, ok := o.Obj.(*MonsterObject); ok && mon.BaseObject == base {
				return mon
			}
		}
	}
	return nil
}

func (e *Environment) getPlayerByBase(base *BaseObject) *PlayObject {
	for i := range e.Cells {
		for _, o := range e.Cells[i].ObjList {
			if o.Type != OS_MOVINGOBJECT {
				continue
			}
			if p, ok := o.Obj.(*PlayObject); ok && p.BaseObject == base {
				return p
			}
		}
	}
	return nil
}

func (e *Environment) broadcastRefMsg(center *BaseObject, ident int, sourceID int32, param1, param2, param3 int) {
	objs := e.GetRangeObjects(center.CurrX, center.CurrY, viewRange)
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost {
			continue
		}
		p.msgMu.Lock()
		p.msgList = append(p.msgList, SendMessage{
			Ident:    ident,
			Param1:   param1,
			Param2:   param2,
			Param3:   param3,
			SourceID: sourceID,
		})
		p.msgMu.Unlock()
	}
}

func (e *Environment) AddGroundItem(item *GroundItem) {
	e.GroundItems = append(e.GroundItems, item)
	e.AddObject(item.X, item.Y, OS_ITEMOBJECT, item)
}

func (e *Environment) RemoveGroundItem(id int32) {
	for i, item := range e.GroundItems {
		if item.ID == id {
			e.RemoveObject(item.X, item.Y, OS_ITEMOBJECT, item)
			e.GroundItems = append(e.GroundItems[:i], e.GroundItems[i+1:]...)
			return
		}
	}
}

func (e *Environment) GetGroundItemAt(x, y int) *GroundItem {
	for _, item := range e.GroundItems {
		if item.X == x && item.Y == y {
			return item
		}
	}
	return nil
}
