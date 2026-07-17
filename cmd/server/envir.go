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

// Environment represents a single map.
type Environment struct {
	Name   string
	Width  int
	Height int
	Cells  []MapCellInfo
	Flag   MapFlag
	Doors  []Door

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

// CanWalk checks if a position is walkable.
func (e *Environment) CanWalk(x, y int) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	return e.Cells[idx].Flag == 0
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
