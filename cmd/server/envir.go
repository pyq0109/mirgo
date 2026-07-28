package main

import (
	"github.com/pyq0109/mirgo/internal/mapformat"
)

// ObjectType 常量
const (
	OS_EVENTOBJECT  = 1
	OS_MOVINGOBJECT = 2
	OS_ITEMOBJECT   = 3
	OS_GATEOBJECT   = 4
	OS_MAPEVENT     = 5
	OS_DOOR         = 6
)

// OSObject 表示地图上的一个对象。
type OSObject struct {
	Type byte
	Obj  interface{}
}

// MapCellInfo 表示单个地图格子。
type MapCellInfo struct {
	Flag    byte
	ObjList []OSObject
}

// MapFlag 包含地图属性。
type MapFlag struct {
	Safe     bool
	Fight    bool
	Dark     bool
	NoDrug   bool
	NoRecall bool
}

// Door 表示地图上的一扇门。
type Door struct {
	ID       byte
	X, Y     int
	State    byte
	OpenTick int64
}

// GroundItem 表示地面上的一个物品。
type GroundItem struct {
	ID       int32
	Name     string
	Looks    int
	X, Y     int
	DropTick int64
	Gold     int
}

// Environment 表示一张地图。
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

// NewEnvironment 从地图文件创建环境。
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

// CanWalk 检查某个位置是否可通行（地形 + 实体 + 门）。
func (e *Environment) CanWalk(x, y int) bool {
	return e.CanWalkEx(x, y, false)
}

// CanWalkEx 检查可通行性。当 ignoreEntities 为 true 时，只检查地形和门，
// 跳过实体碰撞（对应 Delphi CanWalkEx boFlag）。
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

// CanSafeWalk 检查可通行性并排除危险地形（Delphi CanSafeWalk：lava 等）。
// 当前地图格式无独立危险标志位，等同于 CanWalk；待 mapformat 扩展后补充。
func (e *Environment) CanSafeWalk(x, y int) bool {
	return e.CanWalk(x, y)
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

// objectFeatureEx: 低字节 = 马类型，高字节 = 衣服特效（对应 Delphi MakeWord(HorseType, DressEffType)）。
func objectFeatureEx(obj interface{}) int32 {
	if p, ok := obj.(*PlayObject); ok {
		return p.FeatureEx()
	}
	return 0
}

// AddObject 在指定位置向地图添加一个对象。
func (e *Environment) AddObject(x, y int, objType byte, obj interface{}) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	cell := &e.Cells[idx]
	base := objectBase(obj)
	for _, o := range cell.ObjList {
		if o.Type == objType && objectBase(o.Obj) == base {
			return true
		}
	}
	cell.ObjList = append(cell.ObjList, OSObject{
		Type: objType,
		Obj:  obj,
	})
	return true
}

// RemoveObject 从地图移除一个对象。
func (e *Environment) RemoveObject(x, y int, objType byte, obj interface{}) bool {
	if x < 0 || x >= e.Width || y < 0 || y >= e.Height {
		return false
	}
	idx := y*e.Width + x
	cell := &e.Cells[idx]
	target := objectBase(obj)
	for i, o := range cell.ObjList {
		if o.Type == objType && objectBase(o.Obj) == target {
			cell.ObjList = append(cell.ObjList[:i], cell.ObjList[i+1:]...)
			return true
		}
	}
	return false
}

// GetMovingObject 返回指定位置的第一个移动对象。
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

// GetRangeObjects 返回指定半径内的所有对象。
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

// RawMap 返回底层地图数据。
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

// broadcastDeathMsg 向附近玩家广播死亡消息。justDied=true 发送
// SM_NOWDEATH（客户端播放死亡动画）；justDied=false 发送 SM_DEATH
// （客户端直接显示尸体）。dir 是死者的朝向
// （参考 Delphi ObjBase.pas:5523-5549, :21071）。
func (e *Environment) broadcastDeathMsg(center *BaseObject, sourceID int32, x, y, dir int, justDied bool) {
	param3 := 0
	if justDied {
		param3 = 1
	}
	objs := e.GetRangeObjects(center.CurrX, center.CurrY, viewRange)
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost {
			continue
		}
		p.msgMu.Lock()
		p.msgList = append(p.msgList, SendMessage{
			Ident:    RM_DEATH,
			Param1:   x,
			Param2:   y,
			Param3:   param3,
			Dir:      dir,
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

// CanFlyLine 检查 (x1,y1) 到 (x2,y2) 之间是否有视线（Bresenham 逐格检查 CanFly）。
// 对应 Delphi m_PEnvir.CanFly 视线检查。
func (e *Environment) CanFlyLine(x1, y1, x2, y2 int) bool {
	if e.rawMap == nil {
		return true
	}
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx, sy := 1, 1
	if x1 >= x2 {
		sx = -1
	}
	if y1 >= y2 {
		sy = -1
	}
	err := dx - dy
	cx, cy := x1, y1
	for {
		if cx == x2 && cy == y2 {
			return true
		}
		if !e.rawMap.CanFly(cx, cy) {
			return false
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			cx += sx
		}
		if e2 < dx {
			err += dx
			cy += sy
		}
	}
}
