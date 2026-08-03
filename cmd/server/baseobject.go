package main

import (
	"sync"
	"time"

	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	RM_WALK      = 10002
	RM_RUN       = 10003
	RM_HORSERUN  = 10004
	RM_TURN      = 10005
	RM_DISAPPEAR = 10006
	RM_STRUCK    = 10007
	RM_DEATH     = 10008
	RM_SKELETON  = 10009
	RM_LOGON     = 10010
	RM_HIT       = 10011
	RM_HEAVYHIT  = 10013
	RM_BIGHIT    = 10014
	RM_POWERHIT  = 10015
	RM_LONGHIT   = 10016
	RM_WIDEHIT   = 10017
	RM_FIREHIT   = 10018
	RM_SPELL     = 10012
	RM_FEATURECHANGED = 10019
	RM_DIGUP     = 10020
	RM_DIGDOWN   = 10021
	RM_BREAKWEAPON = 10022
	RM_RUSHKUNG  = 10023
	RM_CHANGENAMECOLOR = 10024
	RM_CRSHIT    = 10025
	RM_TWINHIT   = 10026
)

const viewRange = 12

// BaseObject 是服务端所有游戏对象的基类。
type BaseObject struct {
	// 标识
	Name string
	ID   int32

	// 位置
	MapName   string
	CurrX     int
	CurrY     int
	Dir       int
	ViewRange int

	// 外观
	Gender    byte
	Hair      byte
	Job       byte
	DressLook byte
	WeaponLook byte

	// 属性
	Abil  protocol.Ability
	WAbil protocol.Ability

	// 战斗
	HitPoint    int
	SpeedPoint  int
	HitSpeed    int
	Luck        int
	Gold        int
	UndeadBonus int // 不死系易伤加成（Delphi btUndead）

	// 装备特殊属性（Delphi RecalcAbilitys / ApplyItemParameters）
	AntiPoison    int
	PoisonRecover int
	HealthRecover int
	SpellRecover  int
	AntiMagic     int

	// 状态
	StatusTimeArr [12]int16
	Death         bool
	Ghost         bool
	Hidden        bool

	// 背包
	UseItems [13]*protocol.UserItem
	ItemList []*protocol.UserItem

	// 魔法
	MagicList []*protocol.UserMagic

	// 消息队列（线程安全）
	msgList []SendMessage
	msgMu   sync.Mutex

	// 地图引用
	envir *Environment

	// outer 指向具体外层对象（*MonsterObject/*PlayObject/*NpcObject）。
	// 地图格子必须存具体类型，否则 objectBase 匹配/类型断言全部失效。
	outer interface{}
}

// self 返回存入地图格子的对象：优先具体外层对象，回退裸 BaseObject。
func (o *BaseObject) self() interface{} {
	if o.outer != nil {
		return o.outer
	}
	return o
}

type SendMessage struct {
	Ident    int
	Param1   int
	Param2   int
	Param3   int
	Dir      int
	SourceID int32
	Msg      string
	// Delphi dwDeliveryTime/boLateDelivery（ObjBase.pas:788,41）：
	// 延迟重投消息到期才出队，LateDelivery 标记跳过速度/硬直复查。
	DeliveryTime int64
	LateDelivery bool
}

// NewBaseObject 创建一个新的基础对象。
func NewBaseObject(name string, id int32) *BaseObject {
	return &BaseObject{
		Name:      name,
		ID:        id,
		ViewRange: viewRange,
	}
}

// SendMsg 向对象的消息队列添加一条消息。
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

// SendDelayMsg 向自己的队列添加一条延迟消息，delayMs 后到期执行
// （Delphi SendDelayMsg，ObjBase.pas:19367-19403）。
func (o *BaseObject) SendDelayMsg(ident, param1, param2, param3 int, msg string, delayMs int64) {
	o.msgMu.Lock()
	o.msgList = append(o.msgList, SendMessage{
		Ident:        ident,
		Param1:       param1,
		Param2:       param2,
		Param3:       param3,
		Msg:          msg,
		DeliveryTime: time.Now().UnixMilli() + delayMs,
		LateDelivery: true,
	})
	o.msgMu.Unlock()
}

// GetMsg 取出第一条已到期的消息（跳过未到期的延迟消息）。
// Delphi GetMessage（ObjBase.pas:19501-19543）：允许到期消息越过未到期消息出队。
func (o *BaseObject) GetMsg() (SendMessage, bool) {
	o.msgMu.Lock()
	defer o.msgMu.Unlock()

	now := time.Now().UnixMilli()
	for i := range o.msgList {
		if o.msgList[i].DeliveryTime != 0 && now < o.msgList[i].DeliveryTime {
			continue
		}
		msg := o.msgList[i]
		o.msgList = append(o.msgList[:i], o.msgList[i+1:]...)
		return msg, true
	}
	return SendMessage{}, false
}

// QueuedMsgCount 统计队列中指定 ident 的消息数（Delphi GetWalkMsgCount/
// GetRunMsgCount，ObjBase.pas:25139-25179），用于延迟重投的积压上限。
func (o *BaseObject) QueuedMsgCount(ident int) int {
	o.msgMu.Lock()
	defer o.msgMu.Unlock()
	n := 0
	for i := range o.msgList {
		if o.msgList[i].Ident == ident {
			n++
		}
	}
	return n
}

// ClearQueuedMsgs 移除队列中指定 ident 集合的消息（切图时丢弃残留移动指令）。
func (o *BaseObject) ClearQueuedMsgs(idents ...int) {
	o.msgMu.Lock()
	defer o.msgMu.Unlock()
	keep := o.msgList[:0]
	for _, m := range o.msgList {
		drop := false
		for _, id := range idents {
			if m.Ident == id {
				drop = true
				break
			}
		}
		if !drop {
			keep = append(keep, m)
		}
	}
	o.msgList = keep
}

// Feature 返回外观特征值。
func (o *BaseObject) Feature() int32 {
	dress := o.DressLook
	weapon := o.WeaponLook
	hair := o.Hair*2 + o.Gender
	return protocol.MakeHumanFeature(0, dress, weapon, hair)
}

func (o *BaseObject) SendRefMsg(ident, param1, param2, param3 int, msg string) {
	if o.envir == nil {
		return
	}
	objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, o.ViewRange)
	sent := make(map[int32]bool)
	for _, obj := range objs {
		if p, ok := obj.(*PlayObject); ok {
			if p.ID == o.ID || p.Ghost || sent[p.ID] {
				continue
			}
			sent[p.ID] = true
			p.msgMu.Lock()
			p.msgList = append(p.msgList, SendMessage{
				Ident:    ident,
				Param1:   param1,
				Param2:   param2,
				Param3:   param3,
				SourceID: o.ID,
				Msg:      msg,
			})
			p.msgMu.Unlock()
		}
	}
}

// WalkTo 将对象向指定方向移动（含地形+移动对象碰撞检测）。
// Delphi: WalkTo → MoveToMovingObject (Envir.pas:287)
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
	self := o.self()
	if o.envir.hasBlockingObject(newX, newY, self) {
		return false
	}

	o.envir.RemoveObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, self)
	o.CurrX = newX
	o.CurrY = newY
	o.Dir = dir
	o.envir.AddObject(o.CurrX, o.CurrY, OS_MOVINGOBJECT, self)

	return true
}

// TurnTo 改变对象的朝向并广播（Delphi ObjBase.pas:20395-20398）。
func (o *BaseObject) TurnTo(dir int) {
	o.Dir = dir
	o.SendRefMsg(RM_TURN, dir, o.CurrX, o.CurrY, "")
}

// dirToOffset 将方向转换为 dx、dy 偏移量。
func dirToOffset(dir int) (dx, dy int) {
	switch dir {
	case 0: // 上
		return 0, -1
	case 1: // 右上
		return 1, -1
	case 2: // 右
		return 1, 0
	case 3: // 右下
		return 1, 1
	case 4: // 下
		return 0, 1
	case 5: // 左下
		return -1, 1
	case 6: // 左
		return -1, 0
	case 7: // 左上
		return -1, -1
	}
	return 0, 0
}
