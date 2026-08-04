package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/wil"
)

type ActorType int

const (
	ActorHuman   ActorType = 0
	ActorMonster ActorType = 1
	ActorNPC     ActorType = 2
)

type ActorMsg struct {
	Ident   int
	X, Y    int
	Dir     int
	Feature int
	State   int
}

type Actor struct {
	RecogID  int32
	UserName string

	CurrX, CurrY int
	Rx, Ry       int
	Dir          int
	ShiftX, ShiftY float64

	OldX, OldY, OldDir int

	Sex        int
	Race       int
	Hair       int
	Dress      int
	Weapon     int
	Job        int
	Appearance int
	Level      int

	Death    bool
	Skeleton bool
	WarMode  bool
	MsgMuch  bool

	BodySurface   *wil.Image
	HairSurface   *wil.Image
	WeaponSurface *wil.Image

	StartFrame    int
	EndFrame      int
	CurrentFrame  int
	FrameTime     int
	LastFrameTick int64

	CurrentAction  int
	LockEndFrame   bool
	SmoothMoveTime int64
	MoveStep       int
	DefFrameCount  int
	CurrentDefFrame int
	DefFrameTime   int64
	WarModeTime    int64

	RushDir int

	MsgList []ActorMsg
	// msgMu 保护 MsgList：网络读协程 SendMsg/UpdateMsg 入队，
	// 渲染协程 ProcMsg/GetMessage 出队（参考服务端 BaseObject.msgMu）。
	msgMu sync.Mutex

	RealActionMsg ActorMsg
	HasRealAction bool

	Type ActorType

	MonAction *MonsterAction

	HitEffectNumber int

	UseMagic       bool
	SpellFrame     int
	CurEffFrame    int
	EffectFrame     int   // 特效动画帧计数器（魔法盾/武器光效循环）
	EffectFrameTick int64 // 特效帧推进计时
	SpellConfirmed bool // F4: 服务端确认施法（SMMagicFire 到达后置 true）

	Effect        int
	OnHorse       bool
	State         int32
	IsSelf        bool
	// Delphi FrmMain.ServerAcceptNextAction（Actor.pas:2695）：移动动画末帧
	// 等待服务端 #+GOOD 确认的回调，nil 视为允许（他人角色不受门控）。
	AcceptNextAction func() bool
	RedrawPass    bool  // Phase C 覆盖重绘标记（自身翅膀仅在此 pass 绘制）
	WingFrame     int   // 翅膀/特效动画帧（Delphi m_nFrame）
	WingFrameTick int64 // 翅膀帧推进计时（Delphi m_dwFrameTick）
	HairOffset    int   // 头发 WIL 偏移（-1=无头发层）

	SayingArr    [5]string
	SayTime      int64
	SayLineCount int

	// 声音字段（Delphi Actor.pas:662-679）
	FootStepSound       int
	StruckSound         int
	StruckWeaponSound   int
	AppearSound         int
	NormalSound         int
	AttackSound         int
	WeaponSound         int
	ScreamSound         int
	DieSound            int
	Die2Sound           int
	MagicStartSound     int
	MagicFireSound      int
	MagicExplosionSound int
	MagicStruckSound    int
	MagicSerial         int
	BoRunSound          bool
	HiterCode           int32
	MapRef              *mapformat.MapData

	WeaponEffect    int   // weapon glow effect number (0=none)
	EffectNumber    int   // spell effect WIL index for casting animation
	ScrollHideState int   // 0=normal, 1=hiding, 2=hidden, 3=showing
	ScrollHideFrame int   // teleport animation frame (0-10)
	ScrollHideTick  int64 // animation start time (ms)
	Highlighted     bool  // targeted actor highlight
	RushBounce      bool  // RushKung bounce-back animation flag
	RushBounceDir   int   // bounce direction
	Overweight      bool  // F3: 超重标志（移动减速）
	NameColor       int   // SMChangeNameColor: 名字颜色调色板索引（249=红名, 251=黄名, 255=白）
	ShowHP          bool  // SMOpenHealth: 显示头顶HP条
	ShowHPVal       int   // 当前HP
	ShowMaxHPVal    int   // 最大HP

	// NPC 特效覆盖层（Delphi TNpcActor: Actor.pas:760-775）
	NpcUseEffect     bool  // m_boUseEffect
	NpcEffectStart   int   // m_nEffectStart
	NpcEffectFrame   int   // m_nEffectFrame
	NpcEffectEnd     int   // m_nEffectEnd
	NpcEffectTime    int64 // m_dwEffectStartTime
	NpcEffectFTime   int   // m_dwEffectFrameTime
	NpcEffX, NpcEffY int  // m_nEffX, m_nEffY（特效位置偏移）
	NpcEffectHold    bool  // m_bo248（限时循环，appearance 52 挖掘）
	NpcEffectHoldEnd int64 // m_dwUseEffectTick（限时循环截止时间）
}

func NewActor(recogID int32, x, y, dir int) *Actor {
	a := &Actor{
		RecogID: recogID,
		CurrX:   x,
		CurrY:   y,
		Rx:      x,
		Ry:      y,
		Dir:     dir,
	}
	a.initSoundDefaults()
	return a
}

func (a *Actor) SendMsg(ident, x, y, dir, feature, state int) {
	a.msgMu.Lock()
	a.sendMsgLocked(ident, x, y, dir, feature, state)
	a.msgMu.Unlock()
}

// sendMsgLocked 要求调用方已持有 msgMu。
func (a *Actor) sendMsgLocked(ident, x, y, dir, feature, state int) {
	a.MsgList = append(a.MsgList, ActorMsg{
		Ident:   ident,
		X:       x,
		Y:       y,
		Dir:     dir,
		Feature: feature,
		State:   state,
	})
}

func (a *Actor) GetMessage() (ActorMsg, bool) {
	a.msgMu.Lock()
	defer a.msgMu.Unlock()
	if len(a.MsgList) == 0 {
		return ActorMsg{}, false
	}
	msg := a.MsgList[0]
	a.MsgList = a.MsgList[1:]
	return msg, true
}

// MsgCount 返回队列长度（线程安全）。
func (a *Actor) MsgCount() int {
	a.msgMu.Lock()
	defer a.msgMu.Unlock()
	return len(a.MsgList)
}

func (a *Actor) ProcMsg() {
	// Delphi 只在空闲时消费消息（while m_nCurrentAction = 0，Actor.pas:1622），
	// 受击同样排队等待当前动作结束。ReadyAction 总是置非零 CurrentAction，
	// 故每次只消费一条。输入侧由 IsIdle 门控（sceneplay.go），入队即阻断新输入。
	if a.CurrentAction != 0 || a.LockEndFrame {
		return
	}
	msg, ok := a.GetMessage()
	if !ok {
		return
	}
	a.ReadyAction(msg)
}

func (a *Actor) ReadyAction(msg ActorMsg) {
	switch msg.Ident {
	case protocol.SMWalk, protocol.SMRun, protocol.SMTurn, protocol.SMRush, protocol.SMRushKung, protocol.SMBackStep, protocol.SMHorseRun:
		a.updateFeature(msg.Feature)
	}

	if msg.Ident >= 3000 && msg.Ident <= 3099 {
		a.RealActionMsg = msg
		a.HasRealAction = true
		a.OldX = a.CurrX
		a.OldY = a.CurrY
		a.OldDir = a.Dir
		// CM→SM 显式映射（对应 Delphi Actor.pas:1479-1529）。
		// 原始协议 CM/SM 编号不对称（CM_FIREHIT=3025 但 SM_FIREHIT=8），
		// 烈火/十字斩/双龙斩/骑马/投掷必须特殊处理，其余 CM 减 3000 恰好等于对应 SM。
		switch msg.Ident {
		case protocol.CMHorseRun:
			msg.Ident = protocol.SMHorseRun
		case protocol.CMThrow:
			msg.Ident = protocol.SMThrow
		case protocol.CMFireHit:
			msg.Ident = protocol.SMFireHit
		case protocol.CMCrsHit:
			msg.Ident = protocol.SMCrsHit
		case protocol.CMTwinHit:
			msg.Ident = protocol.SMTwinHit
		default:
			msg.Ident = msg.Ident - 3000
		}
	}

	// 受击不更新位置和朝向，动画按当前朝向播放
	// （Delphi ReadyAction 的 SM_STRUCK 分支跳过 else 的坐标/朝向赋值，Actor.pas:1534-1569）。
	if msg.Ident != protocol.SMStruck {
		a.CurrX = msg.X
		a.CurrY = msg.Y
		a.Dir = msg.Dir
	}

	a.CurrentAction = msg.Ident
	if msg.Ident == protocol.SMStruck {
		a.HiterCode = int32(msg.State)
	}
	a.CalcActorFrame()
	a.RunSound()

	if msg.Ident == protocol.SMWalk || msg.Ident == protocol.SMRun || msg.Ident == protocol.SMHorseRun {
		a.Shift(a.Dir, a.MoveStep, 0, a.EndFrame-a.StartFrame+1)
	} else if msg.Ident == protocol.SMBackStep {
		a.Shift((a.Dir+4)%8, a.MoveStep, 0, a.EndFrame-a.StartFrame+1)
	}
}

func (a *Actor) IsIdle() bool {
	return a.CurrentAction == 0 && a.MsgCount() == 0
}

func (a *Actor) MoveFail() {
	a.CurrentAction = 0
	a.LockEndFrame = true
	a.CurrX = a.OldX
	a.CurrY = a.OldY
	a.Dir = a.OldDir
	a.Rx = a.CurrX
	a.Ry = a.CurrY
	a.ShiftX = 0
	a.ShiftY = 0
	a.CleanUserMsgs()
}

func (a *Actor) CleanUserMsgs() {
	a.msgMu.Lock()
	defer a.msgMu.Unlock()
	filtered := a.MsgList[:0]
	for _, m := range a.MsgList {
		if m.Ident >= 3000 && m.Ident <= 3099 {
			continue
		}
		filtered = append(filtered, m)
	}
	a.MsgList = filtered
}

func (a *Actor) UpdateMsg(ident, x, y, dir, feature, state int) {
	a.msgMu.Lock()
	defer a.msgMu.Unlock()
	filtered := a.MsgList[:0]
	for _, m := range a.MsgList {
		if m.Ident >= 3000 && m.Ident <= 3099 {
			continue
		}
		if m.Ident == ident {
			continue
		}
		filtered = append(filtered, m)
	}
	a.MsgList = filtered
	a.sendMsgLocked(ident, x, y, dir, feature, state)
}

func (a *Actor) updateFeature(feature int) {
	if feature == 0 {
		return
	}
	_, dress, weapon, hair := protocol.ParseHumanFeature(int32(feature))
	a.Dress = int(dress)
	a.Weapon = int(weapon)
	a.Hair = int(hair)
	a.Sex = int(hair) % 2
}

func (a *Actor) updateFeatureFromBody(body string) {
	if body == "" {
		return
	}
	raw := []byte(body)
	if len(raw) >= 4 {
		feature := int32(binary.LittleEndian.Uint32(raw[0:4]))
		a.updateFeature(int(feature))
	}
	if len(raw) >= 8 {
		featureEx := int32(binary.LittleEndian.Uint32(raw[4:8]))
		a.OnHorse = featureEx&0xFF != 0
		a.Effect = int((featureEx >> 8) & 0xFF)
	}
}

func (a *Actor) CalcActorFrame() {
	switch a.Type {
	case ActorHuman:
		a.calcHumanFrame()
	case ActorMonster:
		a.calcMonsterFrame()
	case ActorNPC:
		a.calcNPCFrame()
	}
}

func (a *Actor) calcHumanFrame() {
	// 每次重算动作先清除上一动作的命中特效，攻击 case 再按需置位
	// （对应 Delphi m_boHitEffect := FALSE，Actor.pas:3152）
	a.HitEffectNumber = 0
	var action ActionInfo
	switch a.CurrentAction {
	case protocol.SMTurn:
		action = HA.ActStand
		a.MoveStep = 0
		a.Shift(a.Dir, 0, 0, 1)
	case protocol.SMWalk:
		action = HA.ActWalk
		a.MoveStep = 1
	case protocol.SMRun:
		action = HA.ActRun
		a.MoveStep = 2
		// F3: 超重时跑步降为步行速度
		if a.Overweight {
			a.MoveStep = 1
		}
	case protocol.SMHorseRun:
		action = HA.ActRun
		a.MoveStep = 3
		if a.Overweight {
			a.MoveStep = 1
		}
	case protocol.SMRush:
		if a.RushDir == 0 {
			a.RushDir = 1
			action = HA.ActRushLeft
		} else {
			a.RushDir = 0
			action = HA.ActRushRight
		}
		a.MoveStep = 1
	case protocol.SMRushKung:
		action = HA.ActRun
		a.MoveStep = 1
	case protocol.SMBackStep:
		action = HA.ActWalk
		a.MoveStep = 1
	case protocol.SMSitdown:
		action = HA.ActSitdown
		a.MoveStep = 0
	case protocol.SMThrow:
		action = HA.ActHit
		a.HitEffectNumber = 0
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMHeavyHit:
		action = HA.ActHeavyHit
		a.HitEffectNumber = 0
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMBigHit:
		action = HA.ActBigHit
		a.HitEffectNumber = 0
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMHit:
		action = HA.ActHit
		a.HitEffectNumber = 0
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMPowerHit:
		action = HA.ActHit
		a.HitEffectNumber = 1
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMLongHit:
		action = HA.ActHit
		a.HitEffectNumber = 2
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMWideHit:
		action = HA.ActHit
		a.HitEffectNumber = 3
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMFireHit:
		action = HA.ActHit
		a.HitEffectNumber = 4
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMCrsHit:
		action = HA.ActHit
		a.HitEffectNumber = 6
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMTwinHit:
		action = HA.ActHit
		a.HitEffectNumber = 7
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMSpell:
		action = HA.ActSpell
		a.UseMagic = true
		a.SpellFrame = 10
		a.CurEffFrame = 0
		a.WarMode = true
		a.WarModeTime = time.Now().UnixMilli()
	case protocol.SMStruck:
		action = HA.ActStruck
		struckTime := 200 - a.Level*5
		if struckTime < 80 {
			struckTime = 80
		}
		action.FTime = struckTime
		a.Shift(a.Dir, 0, 0, 1)
	case protocol.SMDeath:
		action = HA.ActDie
	case protocol.SMNowDeath:
		action = HA.ActDie
	case protocol.SMSkeleton:
		a.Skeleton = true
		action = HA.ActDie
	case protocol.SMAlive:
		a.Death = false
		a.Skeleton = false
		action = HA.ActStand
	default:
		action = HA.ActStand
		a.MoveStep = 0
	}

	a.StartFrame, a.EndFrame = CalcFrame(action, a.Dir)
	a.CurrentFrame = a.StartFrame
	a.FrameTime = action.FTime
	a.LastFrameTick = 0

	if a.CurrentAction == protocol.SMBackStep {
		a.CurrentFrame = a.EndFrame
		a.Shift((a.Dir+4)%8, a.MoveStep, 0, a.EndFrame-a.StartFrame+1)
	}

	if a.CurrentAction == protocol.SMSkeleton {
		a.CurrentFrame = a.EndFrame
	}

	if a.CurrentAction == protocol.SMDeath {
		a.CurrentFrame = a.EndFrame
	}
}

func (a *Actor) calcMonsterFrame() {
	if a.MonAction == nil {
		return
	}
	var action ActionInfo
	switch a.CurrentAction {
	case protocol.SMTurn:
		action = a.MonAction.ActStand
		a.Shift(a.Dir, 0, 0, 1)
	case protocol.SMWalk:
		action = a.MonAction.ActWalk
		a.MoveStep = 1
	case protocol.SMRun:
		action = a.MonAction.ActWalk
		a.MoveStep = 2
	case protocol.SMHit:
		action = a.MonAction.ActAttack
	case protocol.SMStruck:
		action = a.MonAction.ActStruck
		a.Shift(a.Dir, 0, 0, 1)
	case protocol.SMDeath, protocol.SMNowDeath:
		action = a.MonAction.ActDie
	case protocol.SMSkeleton:
		a.Skeleton = true
		if a.MonAction != nil {
			action = a.MonAction.ActDeath
		}
	default:
		action = a.MonAction.ActStand
	}
	a.StartFrame, a.EndFrame = CalcFrame(action, a.Dir)
	a.CurrentFrame = a.StartFrame
	a.FrameTime = action.FTime
	a.LastFrameTick = 0

	// SM_DEATH 直接显示尸体（死亡动画最后一帧）；只有 SM_NOWDEATH
	// 才播放完整死亡动画（Delphi Actor.pas:1415-1429）。
	if a.CurrentAction == protocol.SMDeath {
		a.CurrentFrame = a.EndFrame
	}
}

// calcNPCFrame 对应 Delphi TNpcActor.CalcActorFrame（Actor.pas:2866-2960）。
// NPC 只处理 SMTurn/SMHit/SMDigUp，其余动作回落到待机；不支持 Walk/Run/Struck/Death。
func (a *Actor) calcNPCFrame() {
	if a.MonAction == nil {
		return
	}
	a.Dir = a.Dir % 3

	var action ActionInfo
	switch a.CurrentAction {
	case protocol.SMTurn:
		action = a.MonAction.ActStand
		a.Shift(a.Dir, 0, 0, 1)
		a.npcActivateTurnEffect()
	case protocol.SMHit:
		// Delphi Actor.pas:2917-2942：appearance 33/34/52 用 ActStand 代替 ActAttack
		switch a.Appearance {
		case 33, 34, 52:
			action = a.MonAction.ActStand
		default:
			action = a.MonAction.ActAttack
		}
		if a.Appearance == 51 {
			a.NpcUseEffect = true
			a.NpcEffectStart = 60
			a.NpcEffectFrame = 60
			a.NpcEffectEnd = 67
			a.NpcEffectFTime = 500
			a.NpcEffectTime = 0
		}
	case protocol.SMDigUp:
		action = a.MonAction.ActStand
		if a.Appearance == 52 {
			a.NpcEffectHold = true
			a.NpcEffectHoldEnd = 0 // 由 Run() 用 now+23000 设置
			a.NpcUseEffect = true
			a.NpcEffectStart = 60
			a.NpcEffectFrame = 60
			a.NpcEffectEnd = 71
			a.NpcEffectFTime = 100
			a.NpcEffectTime = 0
		}
	default:
		action = a.MonAction.ActStand
	}
	a.StartFrame, a.EndFrame = CalcFrame(action, a.Dir)
	a.CurrentFrame = a.StartFrame
	a.FrameTime = action.FTime
	a.LastFrameTick = 0
}

// npcActivateTurnEffect 在 SMTurn 时按 appearance 激活特效（Delphi Actor.pas:2893-2916）。
func (a *Actor) npcActivateTurnEffect() {
	switch {
	case a.Appearance == 33 || a.Appearance == 34:
		if !a.NpcUseEffect {
			a.NpcUseEffect = true
			a.NpcEffectStart = 0
			a.NpcEffectFrame = 0
			a.NpcEffectEnd = 9
			a.NpcEffectFTime = 300
			a.NpcEffectTime = 0
		}
	case a.Appearance >= 42 && a.Appearance <= 47:
		a.NpcUseEffect = true
		a.NpcEffectStart = 0
		a.NpcEffectFrame = 0
		a.NpcEffectEnd = 19
		a.NpcEffectFTime = 100
		a.NpcEffectTime = 0
	case a.Appearance == 51:
		a.NpcUseEffect = true
		a.NpcEffectStart = 60
		a.NpcEffectFrame = 60
		a.NpcEffectEnd = 67
		a.NpcEffectFTime = 500
		a.NpcEffectTime = 0
	}
}

func (a *Actor) Move(now int64) bool {
	switch a.CurrentAction {
	case protocol.SMWalk, protocol.SMRun, protocol.SMRush, protocol.SMRushKung, protocol.SMBackStep, protocol.SMHorseRun:
	default:
		return false
	}

	// Delphi 每 100ms movetick 精确推进一帧，此处无 FrameTime 门控
	// （Actor.pas:2683）；模板 FTime 只驱动非移动动作。
	if a.CurrentAction == protocol.SMBackStep {
		if a.CurrentFrame > a.StartFrame {
			a.CurrentFrame--
			// fastmove：backstep 每 tick 后退 2 帧（Actor.pas:2733-2734）
			if a.CurrentFrame > a.StartFrame {
				a.CurrentFrame--
			}
			cur := a.EndFrame - a.CurrentFrame + 1
			max := a.EndFrame - a.StartFrame + 1
			a.Shift((a.Dir+4)%8, a.MoveStep, cur, max)
		}
		if a.CurrentFrame <= a.StartFrame {
			a.CurrentAction = 0
			a.LockEndFrame = true
			a.SmoothMoveTime = now
		}
	} else {
		if a.CurrentFrame < a.EndFrame {
			a.CurrentFrame++
			// 消息积压加速，但 Rush/RushKung 除外（normmove, Actor.pas:2684）
			if a.MsgMuch && a.CurrentAction != protocol.SMRush && a.CurrentAction != protocol.SMRushKung && a.CurrentFrame < a.EndFrame {
				a.CurrentFrame++
			}
			cur := a.CurrentFrame - a.StartFrame + 1
			max := a.EndFrame - a.StartFrame + 1
			a.Shift(a.Dir, a.MoveStep, cur, max)
			a.PlayFootstep(a.CurrentFrame - a.StartFrame)
		}
		if a.CurrentFrame >= a.EndFrame {
			// Delphi Actor.pas:2693-2699：自身移动动画播完后若服务端确认
			// （#+GOOD）未到，保持动作停在末帧等待，避免步间闪站立姿势。
			if a.IsSelf && a.AcceptNextAction != nil && !a.AcceptNextAction() {
				return true
			}
			a.CurrentAction = 0
			a.LockEndFrame = true
			a.SmoothMoveTime = now
		}
	}
	return true
}

func (a *Actor) Run(now int64) {
	if a.CurrentAction == protocol.SMWalk || a.CurrentAction == protocol.SMRun ||
		a.CurrentAction == protocol.SMRush || a.CurrentAction == protocol.SMRushKung ||
		a.CurrentAction == protocol.SMBackStep || a.CurrentAction == protocol.SMHorseRun {
		return
	}

	if a.CurrentAction != 0 {
		// LastFrameTick==0 是 calcXxxFrame 设置的哨兵：动作刚建立，
		// 首帧需完整显示一个 FrameTime（Delphi m_dwStartTime := GetTickCount，
		// Actor.pas:3416）。否则同一次 Update 中 Run 会立即跳帧。
		if a.LastFrameTick == 0 {
			a.LastFrameTick = now
			return
		}
		ft := a.FrameTime
		if !a.IsSelf && a.UseMagic {
			ft = ft * 10 / 18
		} else if a.MsgMuch {
			ft = ft * 2 / 3
		}
		if now-a.LastFrameTick >= int64(ft) {
			a.LastFrameTick = now
			if a.CurrentFrame < a.EndFrame {
				// F4: 施法动画在 SpellFrame 处暂停，等待服务端确认
				if a.UseMagic && !a.SpellConfirmed && a.CurrentFrame-a.StartFrame >= a.SpellFrame-2 {
					// 保持在施法帧，不前进
				} else {
					a.CurrentFrame++
					// 施法身特效帧随动画推进（对应 Delphi m_nCurEffFrame，Actor.pas:2531-2542）
					if a.UseMagic && a.CurEffFrame < a.SpellFrame-1 {
						a.CurEffFrame++
					}
					a.RunActSound(a.CurrentFrame - a.StartFrame)
				}
			} else {
				a.CurrentAction = 0
				a.UseMagic = false
				a.SpellConfirmed = false
				a.CurEffFrame = 0
				a.HitEffectNumber = 0 // 动作结束清除命中特效（对应 Delphi m_boHitEffect := FALSE，Actor.pas:3627）
			}
		}
		return
	}

	if a.LockEndFrame {
		if now-a.SmoothMoveTime > 200 {
			a.LockEndFrame = false
		} else {
			return
		}
	}

	if a.WarMode && now-a.WarModeTime > 4000 {
		a.WarMode = false
	}

	a.DefaultMotion(now)

	// NPC 特效帧推进（Delphi TNpcActor.Run: Actor.pas:3086-3119）
	if a.Type == ActorNPC && a.NpcUseEffect {
		if a.NpcEffectHold && a.NpcEffectHoldEnd == 0 {
			a.NpcEffectHoldEnd = now + 23000
		}
		if now-a.NpcEffectTime >= int64(a.NpcEffectFTime) {
			a.NpcEffectTime = now
			if a.NpcEffectFrame < a.NpcEffectEnd {
				a.NpcEffectFrame++
			} else {
				if a.NpcEffectHold {
					if now > a.NpcEffectHoldEnd {
						a.NpcUseEffect = false
						a.NpcEffectHold = false
					}
				}
				a.NpcEffectFrame = a.NpcEffectStart
			}
		}
	}
}

func (a *Actor) DefaultMotion(now int64) {
	// D2: 特效动画帧递增（魔法盾/武器光效/施法循环）
	if now-a.EffectFrameTick > 100 {
		a.EffectFrameTick = now
		a.EffectFrame++
	}

	if a.Death {
		a.CurrentFrame = a.getEndFrame()
		a.Shift(a.Dir, 0, 0, 1)
		return
	}

	if a.Type == ActorMonster || a.Type == ActorNPC {
		if a.MonAction != nil {
			action := a.MonAction.ActStand
			a.DefFrameCount = action.Frame
			if now-a.DefFrameTime > int64(action.FTime) {
				a.DefFrameTime = now
				a.CurrentDefFrame++
				if a.CurrentDefFrame >= a.DefFrameCount {
					a.CurrentDefFrame = 0
				}
			}
			start, _ := CalcFrame(action, a.Dir)
			a.CurrentFrame = start + a.CurrentDefFrame
		}
		a.Shift(a.Dir, 0, 0, 1)
		return
	}

	// 翅膀/特效帧推进（Delphi Actor.pas:3435-3460 DefaultMotion）
	// 必须在 WarMode 分支之前，战斗姿态下翅膀仍需动画。
	if a.Effect == 50 && a.CurrentFrame <= 536 {
		if now-a.WingFrameTick >= 100 {
			a.WingFrameTick = now
			a.WingFrame++
			if a.WingFrame >= 20 {
				a.WingFrame = 0
			}
		}
	} else if a.Effect != 0 && a.CurrentFrame < 64 {
		if now-a.WingFrameTick >= 150 {
			a.WingFrameTick = now
			a.WingFrame++
			if a.WingFrame >= 8 {
				a.WingFrame = 0
			}
		}
	}

	if a.WarMode {
		action := HA.ActWarMode
		start, _ := CalcFrame(action, a.Dir)
		a.CurrentFrame = start
		a.Shift(a.Dir, 0, 0, 1)
		return
	}

	action := HA.ActStand
	a.DefFrameCount = action.Frame
	if now-a.DefFrameTime > 500 {
		a.DefFrameTime = now
		a.CurrentDefFrame++
		if a.CurrentDefFrame >= a.DefFrameCount {
			a.CurrentDefFrame = 0
		}
	}
	start, _ := CalcFrame(action, a.Dir)
	a.CurrentFrame = start + a.CurrentDefFrame
	a.Shift(a.Dir, 0, 0, 1)
}

func (a *Actor) getEndFrame() int {
	if a.Type == ActorHuman {
		start, end := CalcFrame(HA.ActDie, a.Dir)
		_ = start
		return end
	}
	if a.MonAction != nil {
		start, end := CalcFrame(a.MonAction.ActDie, a.Dir)
		_ = start
		return end
	}
	return a.CurrentFrame
}

func (a *Actor) Shift(dir, step, cur, max int) {
	if step == 0 || max == 0 {
		a.Rx = a.CurrX
		a.Ry = a.CurrY
		a.ShiftX = 0
		a.ShiftY = 0
		return
	}

	unx := float64(engine.TileWidth * step)
	uny := float64(engine.TileHeight * step)

	var dx, dy int
	switch dir {
	case 0:
		dx, dy = 0, -1
	case 1:
		dx, dy = 1, -1
	case 2:
		dx, dy = 1, 0
	case 3:
		dx, dy = 1, 1
	case 4:
		dx, dy = 0, 1
	case 5:
		dx, dy = -1, 1
	case 6:
		dx, dy = -1, 0
	case 7:
		dx, dy = -1, -1
	}

	fCur := float64(cur)
	fMax := float64(max)

	// 每方向的取整偏移 v（Delphi Actor.pas:1773-1865）：max>=6 时
	// 对角线用 ±2、向下用 -1，使中间步的格子切换点偏移。
	v := 0
	if fMax >= 6 {
		switch dir {
		case 1, 7:
			v = 2 // UPRIGHT/UPLEFT: (max-cur+2)
		case 3, 5:
			v = -2 // DOWNRIGHT/DOWNLEFT: (max-cur-2)
		case 4:
			v = -1 // DOWN: (max-cur-1)
		}
	}
	ss := roundEven((fMax-fCur+float64(v))/fMax) * step

	a.Rx = a.CurrX - dx*ss
	a.Ry = a.CurrY - dy*ss

	if ss == step {
		a.ShiftX = float64(dx) * unx / fMax * fCur
		a.ShiftY = float64(dy) * uny / fMax * fCur
	} else {
		a.ShiftX = -float64(dx) * unx / fMax * (fMax - fCur)
		a.ShiftY = -float64(dy) * uny / fMax * (fMax - fCur)
	}
}

// roundEven 实现 Delphi 的银行家舍入：恰好 .5 时舍入到最近的偶数
// （Round(0.5)=0, Round(1.5)=2, Round(2.5)=2）。
func roundEven(x float64) int {
	f := math.Floor(x)
	diff := x - f
	if diff > 0.5 {
		return int(f) + 1
	}
	if diff < 0.5 {
		return int(f)
	}
	if int(f)%2 == 0 {
		return int(f)
	}
	return int(f) + 1
}

func (a *Actor) GetBodyImage(resources *engine.ResourceManager) *wil.Image {
	switch a.Type {
	case ActorHuman:
		return a.getHumanBodyImage(resources)
	case ActorMonster:
		return a.getMonsterBodyImage(resources)
	case ActorNPC:
		return a.getNPCBodyImage(resources)
	}
	return nil
}

func (a *Actor) getHumanBodyImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Hum == nil {
		return nil
	}
	idx := HumanFrame*a.Dress + a.CurrentFrame
	if idx < 0 || idx >= resources.Hum.Count {
		return nil
	}
	img := resources.Hum.GetImage(idx)
	if img == nil || img.RGBA == nil {
		idx = a.CurrentFrame
		if idx >= 0 && idx < resources.Hum.Count {
			img = resources.Hum.GetImage(idx)
		}
	}
	return img
}

func (a *Actor) getHumanHairImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Hair == nil || a.HairOffset < 0 {
		return nil
	}
	idx := a.HairOffset + a.CurrentFrame
	if idx < 0 || idx >= resources.Hair.Count {
		return nil
	}
	return resources.Hair.GetImage(idx)
}

func (a *Actor) getHumanWeaponImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Weapon == nil || a.Weapon == 0 {
		return nil
	}
	idx := HumanFrame*a.Weapon + a.CurrentFrame
	if idx < 0 || idx >= resources.Weapon.Count {
		return nil
	}
	return resources.Weapon.GetImage(idx)
}

func (a *Actor) getMonsterBodyImage(resources *engine.ResourceManager) *wil.Image {
	monFile := a.getMonFile(resources)
	if monFile == nil {
		return nil
	}
	offset := GetMonOffset(a.Appearance)
	idx := offset + a.CurrentFrame
	if idx < 0 || idx >= monFile.Count {
		return nil
	}
	return monFile.GetImage(idx)
}

func (a *Actor) getMonFile(resources *engine.ResourceManager) *wil.File {
	nrace := a.Appearance / 10
	if nrace >= 0 && nrace < len(resources.Mon) && resources.Mon[nrace] != nil {
		return resources.Mon[nrace]
	}
	return resources.Mon[0]
}

func (a *Actor) getNPCBodyImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Npc == nil {
		return nil
	}
	// Delphi Actor.pas:3053-3054：appearance 42-47 的 body 为 nil（纯特效渲染）
	if a.Appearance >= 42 && a.Appearance <= 47 {
		return nil
	}
	idx := GetNpcOffset(a.Appearance) + a.CurrentFrame
	if idx < 0 || idx >= resources.Npc.Count {
		return nil
	}
	return resources.Npc.GetImage(idx)
}

func (a *Actor) Draw(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	switch a.Type {
	case ActorHuman:
		a.drawHuman(gl, resources, screenX, screenY, proj)
	case ActorMonster:
		a.drawBody(gl, resources, screenX, screenY, proj)
	case ActorNPC:
		a.drawBody(gl, resources, screenX, screenY, proj)
	}
}

// stateTransparentBit 标记处于隐身/透明状态的演员，渲染时整体半透明。
// 与混合位(0x00800000)及高位着色位(0x04000000+)互不冲突。
const stateTransparentBit int32 = 0x00010000

// stateAlpha 返回状态对应的整体不透明度：隐身状态 0.3，其余 1.0。
func stateAlpha(state int32) float32 {
	if state&stateTransparentBit != 0 {
		return 0.3
	}
	return 1.0
}

// drawTintedQuad 统一处理各图层的着色与隐身透明度：
// 着色或半透明时用 DrawQuadTint，否则走普通 DrawQuad。
func drawTintedQuad(gl *engine.GLState, tex uint32, x, y, w, h float32, tr, tg, tb float32, useTint bool, alpha float32, proj [16]float32) {
	if useTint || alpha < 1.0 {
		r, g, b := float32(1), float32(1), float32(1)
		if useTint {
			r, g, b = tr, tg, tb
		}
		gl.DrawQuadTint(tex, x, y, w, h, r, g, b, alpha, proj)
		return
	}
	gl.DrawQuad(tex, x, y, w, h, proj)
}

func fmtAlpha(a float32) string {
	if a >= 1.0 {
		return "1.0"
	}
	return fmt.Sprintf("%.2f", a)
}

func getStateTint(state int32) (float32, float32, float32, bool) {
	switch {
	case state < 0: // $80000000 ceGreen
		return 0.3, 1.0, 0.3, true
	case state&0x40000000 != 0: // ceRed
		return 1.0, 0.3, 0.3, true
	case state&0x20000000 != 0: // ceBlue
		return 0.3, 0.3, 1.0, true
	case state&0x10000000 != 0: // ceYellow
		return 1.0, 1.0, 0.3, true
	case state&0x08000000 != 0: // ceFuchsia
		return 1.0, 0.3, 1.0, true
	case state&0x04000000 != 0: // ceGrayScale
		return 0.6, 0.6, 0.6, true
	}
	return 0, 0, 0, false
}

func (a *Actor) drawBody(glState *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	scale := a.ScrollHideScale()
	if scale <= 0 {
		return
	}
	img := a.GetBodyImage(resources)
	if img == nil || img.RGBA == nil {
		return
	}
	wilFile := getWilFile(resources, a.Type, a.Appearance)
	if wilFile == nil {
		return
	}
	texIdx := a.getTextureIndex()
	tex := resources.GetTexture(wilFile, texIdx)
	if tex == 0 {
		return
	}
	w := float32(img.Width)
	h := float32(img.Height)
	drawX := screenX + float32(img.HotX)
	drawY := screenY + float32(img.HotY)
	if scale < 1.0 {
		drawY = drawY + h*(1-scale)
		h *= scale
	}

	if debugRenderFrame <= 2 {
		// 统计 alpha 分布
		transparent, opaqueBlack, opaqueOther := 0, 0, 0
		if img.RGBA != nil {
			for y := 0; y < img.Height; y++ {
				for x := 0; x < img.Width; x++ {
					off := (y*img.Width + x) * 4
					al := img.RGBA.Pix[off+3]
					if al == 0 {
						transparent++
					} else if img.RGBA.Pix[off+0] == 0 && img.RGBA.Pix[off+1] == 0 && img.RGBA.Pix[off+2] == 0 {
						opaqueBlack++
					} else {
						opaqueOther++
					}
				}
			}
		}
		log.Logf(log.LevelInfo, "Actor", "drawBody type=%d appr=%d texIdx=%d tex=%d pos=(%.0f,%.0f) drawAt=(%.0f,%.0f) size=(%.0f,%.0f) hot=(%d,%d) state=0x%08X alpha=%s transparent=%d opaqueBlack=%d opaqueOther=%d",
			a.Type, a.Appearance, texIdx, tex, screenX, screenY, drawX, drawY, w, h, img.HotX, img.HotY, a.State,
			fmtAlpha(stateAlpha(a.State)), transparent, opaqueBlack, opaqueOther)
	}

	// Delphi Actor.pas:2982-2990：appearance 51 强制混合模式
	blend := a.State&0x00800000 != 0 || (a.Type == ActorNPC && a.Appearance == 51)
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	}
	tr, tg, tb, tinted := getStateTint(a.State)
	drawTintedQuad(glState, tex, drawX, drawY, w, h, tr, tg, tb, tinted, stateAlpha(a.State), proj)
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

// DrawNpcEffect 绘制 NPC 特效覆盖层（Delphi TNpcActor.DrawEff: Actor.pas:3000-3010）。
// 使用加法混合，位置偏移按 appearance 不同（Actor.pas:3055-3082）。
func (a *Actor) DrawNpcEffect(glState *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	if a.Type != ActorNPC || !a.NpcUseEffect || resources.Npc == nil {
		return
	}
	idx := GetNpcOffset(a.Appearance) + a.NpcEffectFrame
	if idx < 0 || idx >= resources.Npc.Count {
		return
	}
	img := resources.Npc.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := resources.GetTexture(resources.Npc, idx)
	if tex == 0 {
		return
	}

	effX, effY := float32(img.HotX), float32(img.HotY)
	switch a.Appearance {
	case 42:
		effX += 71
		effY += 5
	case 43:
		effX += 71
		effY += 37
	case 44:
		effX += 7
		effY += 12
	case 45:
		effX += 6
		effY += 12
	case 46:
		effX += 7
		effY += 12
	case 47:
		effX += 8
		effY += 12
	}

	drawX := screenX + effX + float32(a.ShiftX)
	drawY := screenY + effY + float32(a.ShiftY)

	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	glState.DrawQuad(tex, drawX, drawY, float32(img.Width), float32(img.Height), proj)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
}

func (a *Actor) wingBehind() bool {
	return a.Dir >= 3 && a.Dir <= 5
}

func (a *Actor) drawHuman(glState *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	if a.ScrollHideState == 1 || a.ScrollHideState == 3 {
		a.updateScrollHide()
	}
	scrollScale := a.ScrollHideScale()
	if scrollScale <= 0 {
		return
	}

	wpord := getWordOrder(a.Sex, a.CurrentFrame)

	blend := a.State&0x00800000 != 0
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	}

	// 状态着色与隐身透明度对全部图层（身体/头发/武器/翅膀）一致生效。
	tintR, tintG, tintB, useTint := getStateTint(a.State)
	alpha := stateAlpha(a.State)

	if a.Effect > 0 && a.wingBehind() && (!a.IsSelf || a.RedrawPass) {
		a.drawWingLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, scrollScale, proj)
	}

	if wpord == 0 && a.Weapon >= 2 {
		a.drawWeaponLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, scrollScale, proj)
	}

	bodyIdx := HumanFrame*a.Dress + a.CurrentFrame
	if resources.Hum != nil && bodyIdx >= 0 && bodyIdx < resources.Hum.Count {
		img := resources.Hum.GetImage(bodyIdx)
		if img == nil || img.RGBA == nil {
			// Delphi Actor.pas:3704-3705: 取图失败回退 Dress=0
			bodyIdx = a.CurrentFrame
			if bodyIdx >= 0 && bodyIdx < resources.Hum.Count {
				img = resources.Hum.GetImage(bodyIdx)
			}
		}
		if img != nil && img.RGBA != nil {
			tex := resources.GetTexture(resources.Hum, bodyIdx)
			if tex != 0 {
				w := float32(img.Width)
				h := float32(img.Height)
				bx := screenX + float32(img.HotX)
				by := screenY + float32(img.HotY)
				by, h = applyScale(by, h, scrollScale)
				if debugRenderFrame <= 2 {
					transparent, opaqueBlack, opaqueOther := 0, 0, 0
					for yy := 0; yy < img.Height; yy++ {
						for xx := 0; xx < img.Width; xx++ {
							off := (yy*img.Width + xx) * 4
							al := img.RGBA.Pix[off+3]
							if al == 0 {
								transparent++
							} else if img.RGBA.Pix[off+0] == 0 && img.RGBA.Pix[off+1] == 0 && img.RGBA.Pix[off+2] == 0 {
								opaqueBlack++
							} else {
								opaqueOther++
							}
						}
					}
					log.Logf(log.LevelInfo, "Actor", "drawHuman bodyIdx=%d tex=%d screenPos=(%.0f,%.0f) drawAt=(%.0f,%.0f) size=(%.0f,%.0f) hot=(%d,%d) blend=%v tint=%v alpha=%s T=%d B=%d O=%d",
						bodyIdx, tex, screenX, screenY, bx, by, w, h, img.HotX, img.HotY, blend, useTint, fmtAlpha(alpha), transparent, opaqueBlack, opaqueOther)
				}
				drawTintedQuad(glState, tex, bx, by, w, h, tintR, tintG, tintB, useTint, alpha, proj)
			}
		}
	}

	// 头发偏移（Delphi Actor.pas:3162-3167）
	a.HairOffset = -1
	if a.Hair > 0 && resources.Hair != nil {
		haircount := resources.Hair.Count / HumanFrame / 2
		h := a.Hair
		if haircount > 0 && h > haircount-1 {
			h = haircount - 1
		}
		h = h * 2
		if h > 1 {
			a.HairOffset = HumanFrame * (h + a.Sex)
		}
	}
	if a.HairOffset >= 0 && resources.Hair != nil {
		hairIdx := a.HairOffset + a.CurrentFrame
		if hairIdx >= 0 && hairIdx < resources.Hair.Count {
			img := resources.Hair.GetImage(hairIdx)
			if img != nil && img.RGBA != nil {
				tex := resources.GetTexture(resources.Hair, hairIdx)
				if tex != 0 {
					w := float32(img.Width)
					h := float32(img.Height)
					hx := screenX + float32(img.HotX)
					hy := screenY + float32(img.HotY)
					hy, h = applyScale(hy, h, scrollScale)
					drawTintedQuad(glState, tex, hx, hy, w, h, tintR, tintG, tintB, useTint, alpha, proj)
				}
			}
		}
	}

	if wpord == 1 && a.Weapon >= 2 {
		a.drawWeaponLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, scrollScale, proj)
	}

	if a.Effect > 0 && !a.wingBehind() && (!a.IsSelf || a.RedrawPass) {
		a.drawWingLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, scrollScale, proj)
	}

	// D2 Layer 7: 魔法盾泡泡（STATE_BUBBLEDEFENCEUP = 0x00020000）
	if a.State&0x00020000 != 0 && resources.Magic != nil {
		var bubbleBase int
		if a.CurrentAction == protocol.SMStruck {
			bubbleBase = 3900 // MAGBUBBLESTRUCKBASE
		} else {
			bubbleBase = 3890 // MAGBUBBLEBASE
		}
		frame := bubbleBase + (a.EffectFrame % 3)
		if frame >= 0 && frame < resources.Magic.Count {
			if img := resources.Magic.GetImage(frame); img != nil && img.RGBA != nil {
				if tex := resources.GetTexture(resources.Magic, frame); tex != 0 {
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
					glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
						float32(img.Width), float32(img.Height), proj)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				}
			}
		}
	}

	// D2 Layer 8: 施法身特效（对应 Delphi Actor.pas:3899-3911，GetEffectBase 路由 + CurEffFrame）
	if a.UseMagic && a.EffectNumber > 0 && a.CurEffFrame >= 0 && a.CurEffFrame < a.SpellFrame {
		f, base := getEffectBase(resources, a.EffectNumber-1, 0)
		if f != nil {
			spellIdx := base + a.CurEffFrame
			if spellIdx >= 0 && spellIdx < f.Count {
				if img := f.GetImage(spellIdx); img != nil && img.RGBA != nil {
					if tex := resources.GetTexture(f, spellIdx); tex != 0 {
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
						glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
							float32(img.Width), float32(img.Height), proj)
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
					}
				}
			}
		}
	}

	// D2 Layer 9: 攻击特效（方向性火花）（对应 Delphi Actor.pas:3914-3924，GetEffectBase mtype=1 路由）
	if a.HitEffectNumber > 0 {
		if wilFile, base := getEffectBase(resources, a.HitEffectNumber-1, 1); wilFile != nil {
			hitIdx := base + a.Dir*10 + (a.CurrentFrame - a.StartFrame)
			if hitIdx >= 0 && hitIdx < wilFile.Count {
				if img := wilFile.GetImage(hitIdx); img != nil && img.RGBA != nil {
					if tex := resources.GetTexture(wilFile, hitIdx); tex != 0 {
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
						glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
							float32(img.Width), float32(img.Height), proj)
						gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
					}
				}
			}
		}
	}

	// D2 Layer 10: 武器光效
	if a.WeaponEffect > 0 && resources.Magic != nil {
		wpEffIdx := 3750 + a.Dir*10 + (a.EffectFrame % 5) // WPEFFECTBASE, MAXWPEFFECTFRAME=5
		if wpEffIdx >= 0 && wpEffIdx < resources.Magic.Count {
			if img := resources.Magic.GetImage(wpEffIdx); img != nil && img.RGBA != nil {
				if tex := resources.GetTexture(resources.Magic, wpEffIdx); tex != 0 {
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
					glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
						float32(img.Width), float32(img.Height), proj)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				}
			}
		}
	}

	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

func applyScale(y, h, scale float32) (float32, float32) {
	if scale >= 1.0 {
		return y, h
	}
	return y + h*(1-scale), h * scale
}

func (a *Actor) drawWeaponLayer(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, tintR, tintG, tintB float32, useTint bool, alpha, scale float32, proj [16]float32) {
	weaponIdx := HumanFrame*a.Weapon + a.CurrentFrame
	if resources.Weapon == nil || weaponIdx < 0 || weaponIdx >= resources.Weapon.Count {
		return
	}
	img := resources.Weapon.GetImage(weaponIdx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := resources.GetTexture(resources.Weapon, weaponIdx)
	if tex == 0 {
		return
	}
	w := float32(img.Width)
	h := float32(img.Height)
	wy := screenY + float32(img.HotY)
	wy, h = applyScale(wy, h, scale)
	drawTintedQuad(gl, tex, screenX+float32(img.HotX), wy, w, h, tintR, tintG, tintB, useTint, alpha, proj)
}

func (a *Actor) drawWingLayer(glState *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, tintR, tintG, tintB float32, useTint bool, alpha, scale float32, proj [16]float32) {
	var wilFile *wil.File
	var wingIdx int

	if a.Effect == 50 {
		if a.CurrentFrame > 536 {
			return
		}
		wilFile = resources.Effect
		wingIdx = 352 + a.WingFrame
	} else {
		wilFile = resources.HumEffect
		offset := (a.Effect - 1) * HumanFrame
		if a.CurrentFrame < 64 {
			wingIdx = offset + a.Dir*8 + a.WingFrame
		} else {
			wingIdx = offset + a.CurrentFrame
		}
	}

	if wilFile == nil || wingIdx < 0 || wingIdx >= wilFile.Count {
		return
	}
	img := wilFile.GetImage(wingIdx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := resources.GetTexture(wilFile, wingIdx)
	if tex == 0 {
		return
	}
	w := float32(img.Width)
	h := float32(img.Height)
	wy := screenY + float32(img.HotY)
	wy, h = applyScale(wy, h, scale)
	// Delphi Actor.pas:3767-3880: 翅膀始终使用加法混合（DrawBlend）
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	drawTintedQuad(glState, tex, screenX+float32(img.HotX), wy, w, h, tintR, tintG, tintB, useTint, alpha, proj)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
}

func getWilFile(resources *engine.ResourceManager, actorType ActorType, appr int) *wil.File {
	switch actorType {
	case ActorHuman:
		return resources.Hum
	case ActorMonster:
		nrace := appr / 10
		if nrace < len(resources.Mon) {
			return resources.Mon[nrace]
		}
		return resources.Mon[0]
	case ActorNPC:
		return resources.Npc
	}
	return nil
}

func (a *Actor) getTextureIndex() int {
	switch a.Type {
	case ActorHuman:
		return HumanFrame*a.Dress + a.CurrentFrame
	case ActorMonster:
		return GetMonOffset(a.Appearance) + a.CurrentFrame
	case ActorNPC:
		return GetNpcOffset(a.Appearance) + a.CurrentFrame
	}
	return 0
}

func GetMonOffset(appr int) int {
	nrace := appr / 10
	npos := appr % 10

	switch nrace {
	case 0:
		return npos * 280
	case 1:
		return npos * 230
	case 2, 3, 7, 8, 9, 10, 11, 12:
		return npos * 360
	case 4:
		if npos == 1 {
			return 600
		}
		return npos * 360
	case 5:
		return npos * 430
	case 6:
		return npos * 440
	case 13:
		switch npos {
		case 0:
			return 0
		case 1:
			return 360
		case 2:
			return 440
		case 3:
			return 550
		default:
			return npos * 360
		}
	case 14, 15, 16:
		return npos * 360
	case 17:
		if npos == 2 {
			return 920
		}
		return npos * 350
	case 18:
		offsets := []int{0, 520, 950}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 19:
		offsets := []int{0, 370, 810, 1250, 1630, 2010, 2390}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 20:
		offsets := []int{0, 360, 720, 1080, 1440, 1800, 2350, 3060}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 21:
		offsets := []int{0, 460, 820, 1180, 1540, 1900, 2440, 2570, 2700}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 22:
		offsets := []int{0, 430, 1290, 1810}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 23:
		offsets := []int{0, 440, 820, 1360, 1420, 1450, 1560, 1670, 2270, 2700}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 24:
		offsets := []int{0, 350, 700, 1050, 1650, 3100, 3450, 3880, 4230, 4580}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 25:
		offsets := []int{0, 350, 700, 1050, 1400, 1750, 2180, 2530, 3000, 3810}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 26:
		offsets := []int{0, 370, 720, 1080, 1430, 1780, 2290, 2720, 3150, 4000}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 27:
		offsets := []int{0, 350, 700, 1210, 1720, 2170, 2250, 2720}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 80:
		offsets := []int{0, 80, 300, 301, 302, 320, 321, 322, 321}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	case 90:
		offsets := []int{80, 168, 184, 200}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return 0
	default:
		return npos * 280
	}
}

func GetNpcOffset(appr int) int {
	if appr <= 22 {
		return appr * 60
	}
	switch appr {
	case 23:
		return 1380
	case 24, 25:
		return (appr-24)*60 + 1470
	case 26, 28, 29, 30, 31, 33, 34, 35, 36, 37, 38, 39, 40, 41:
		return (appr-26)*60 + 1620
	case 27, 32:
		return (appr-26)*60 + 1590
	case 42, 43:
		return 2580
	case 44, 45, 46, 47:
		return 2640
	case 48, 49, 50:
		return (appr-48)*60 + 2700
	case 51:
		return 2880
	case 52:
		return 2960
	case 53:
		return 3020
	default:
		if appr >= 54 && appr <= 57 {
			return (appr-54)*60 + 3070
		}
		switch appr {
		case 58:
			return 3270
		case 59:
			return 3290
		case 60:
			return 3330
		case 61, 62, 63, 64:
			return 3350
		case 65:
			return 3430
		case 66:
			return 3450
		case 67:
			return 3500
		case 68:
			return 3570
		case 75:
			return 3730
		case 76:
			return 3810
		case 81:
			return 4070
		case 82:
			return 4110
		case 83:
			return 4150
		}
		if appr >= 69 && appr <= 74 {
			return (appr-69)*20 + 3610
		}
		if appr >= 77 && appr <= 80 {
			return (appr-77)*20 + 3850
		}
		return 0
	}
}

func (a *Actor) Say(text string) {
	a.SayingArr = [5]string{}
	a.SayLineCount = 0
	a.SayTime = time.Now().UnixMilli()

	runes := []rune(text)
	line := 0
	start := 0
	for start < len(runes) && line < 5 {
		end := start + 20
		if end > len(runes) {
			end = len(runes)
		}
		a.SayingArr[line] = string(runes[start:end])
		line++
		start = end
	}
	a.SayLineCount = line
}

// updateScrollHide 推进传送动画帧（D4）。
// State 1=隐藏中（收缩），3=显示中（展开）。每 30ms 一帧，共 10 帧。
func (a *Actor) updateScrollHide() {
	now := time.Now().UnixMilli()
	elapsed := now - a.ScrollHideTick
	frame := int(elapsed / 30)
	if frame > 10 {
		frame = 10
	}
	switch a.ScrollHideState {
	case 1: // 隐藏中
		a.ScrollHideFrame = 10 - frame
		if frame >= 10 {
			a.ScrollHideState = 2 // 完全隐藏
		}
	case 3: // 显示中
		a.ScrollHideFrame = frame
		if frame >= 10 {
			a.ScrollHideState = 0 // 恢复正常
			a.ScrollHideFrame = 10
		}
	}
}

// ScrollHideScale 返回传送动画的 Y 缩放因子（0.0~1.0）。
func (a *Actor) ScrollHideScale() float32 {
	switch a.ScrollHideState {
	case 1:
		return float32(a.ScrollHideFrame) / 10.0
	case 2:
		return 0.0
	case 3:
		return float32(a.ScrollHideFrame) / 10.0
	default:
		return 1.0
	}
}

// --- 调试线框：图层边界框计算 ---

type LayerBounds struct {
	LayerName string
	WilName   string
	ImageIdx  int
	HotX      int16
	HotY      int16
	Width     int
	Height    int
	DrawX     float32
	DrawY     float32
	TexID     uint32
	Img       *wil.Image
}

func wilDisplayName(f *wil.File, resources *engine.ResourceManager) string {
	if f == nil {
		return "?"
	}
	switch f {
	case resources.Hum:
		return "Hum.wil"
	case resources.Hair:
		return "Hair.wil"
	case resources.Weapon:
		return "Weapon.wil"
	case resources.HumEffect:
		return "HumEffect.wil"
	case resources.Effect:
		return "Effect.wil"
	case resources.Npc:
		return "Npc.wil"
	case resources.Magic:
		return "Magic.wil"
	case resources.Magic2:
		return "Magic2.wil"
	}
	for i, m := range resources.Mon {
		if m == f {
			return fmt.Sprintf("Mon%d.wil", i)
		}
	}
	return "?.wil"
}

func ComputeAlphaStats(img *wil.Image) (transparent, opaqueBlack, opaqueOther int) {
	if img == nil || img.RGBA == nil {
		return
	}
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			off := (y*img.Width + x) * 4
			al := img.RGBA.Pix[off+3]
			if al == 0 {
				transparent++
			} else if img.RGBA.Pix[off+0] == 0 && img.RGBA.Pix[off+1] == 0 && img.RGBA.Pix[off+2] == 0 {
				opaqueBlack++
			} else {
				opaqueOther++
			}
		}
	}
	return
}

func (a *Actor) ComputeLayerBounds(resources *engine.ResourceManager, worldX, worldY float32) []LayerBounds {
	scale := a.ScrollHideScale()
	if scale <= 0 {
		return nil
	}

	var bounds []LayerBounds

	// body 层
	bodyImg := a.GetBodyImage(resources)
	if bodyImg != nil {
		wilFile := getWilFile(resources, a.Type, a.Appearance)
		texIdx := a.getTextureIndex()
		var tex uint32
		if wilFile != nil {
			tex = resources.GetTexture(wilFile, texIdx)
		}
		dy := worldY + float32(bodyImg.HotY)
		h := float32(bodyImg.Height)
		dy, h = applyScale(dy, h, scale)
		bounds = append(bounds, LayerBounds{
			LayerName: "body",
			WilName:   wilDisplayName(wilFile, resources),
			ImageIdx:  texIdx,
			HotX:      bodyImg.HotX,
			HotY:      bodyImg.HotY,
			Width:     bodyImg.Width,
			Height:    int(h),
			DrawX:     worldX + float32(bodyImg.HotX),
			DrawY:     dy,
			TexID:     tex,
			Img:       bodyImg,
		})
	}

	if a.Type != ActorHuman {
		return bounds
	}

	// weapon 层
	if a.Weapon >= 2 && resources.Weapon != nil {
		weaponIdx := HumanFrame*a.Weapon + a.CurrentFrame
		if weaponIdx >= 0 && weaponIdx < resources.Weapon.Count {
			img := resources.Weapon.GetImage(weaponIdx)
			if img != nil && img.RGBA != nil {
				tex := resources.GetTexture(resources.Weapon, weaponIdx)
				dy := worldY + float32(img.HotY)
				h := float32(img.Height)
				dy, h = applyScale(dy, h, scale)
				bounds = append(bounds, LayerBounds{
					LayerName: "weapon",
					WilName:   "Weapon.wil",
					ImageIdx:  weaponIdx,
					HotX:      img.HotX,
					HotY:      img.HotY,
					Width:     img.Width,
					Height:    int(h),
					DrawX:     worldX + float32(img.HotX),
					DrawY:     dy,
					TexID:     tex,
					Img:       img,
				})
			}
		}
	}

	// hair 层 — 重新计算 HairOffset（不依赖 a.HairOffset 可能过期的值）
	hairOffset := -1
	if a.Hair > 0 && resources.Hair != nil {
		haircount := resources.Hair.Count / HumanFrame / 2
		h := a.Hair
		if haircount > 0 && h > haircount-1 {
			h = haircount - 1
		}
		h = h * 2
		if h > 1 {
			hairOffset = HumanFrame * (h + a.Sex)
		}
	}
	if hairOffset >= 0 && resources.Hair != nil {
		hairIdx := hairOffset + a.CurrentFrame
		if hairIdx >= 0 && hairIdx < resources.Hair.Count {
			img := resources.Hair.GetImage(hairIdx)
			if img != nil && img.RGBA != nil {
				tex := resources.GetTexture(resources.Hair, hairIdx)
				dy := worldY + float32(img.HotY)
				h := float32(img.Height)
				dy, h = applyScale(dy, h, scale)
				bounds = append(bounds, LayerBounds{
					LayerName: "hair",
					WilName:   "Hair.wil",
					ImageIdx:  hairIdx,
					HotX:      img.HotX,
					HotY:      img.HotY,
					Width:     img.Width,
					Height:    int(h),
					DrawX:     worldX + float32(img.HotX),
					DrawY:     dy,
					TexID:     tex,
					Img:       img,
				})
			}
		}
	}

	// wing 层
	if a.Effect > 0 {
		var wilFile *wil.File
		var wingIdx int
		if a.Effect == 50 {
			if a.CurrentFrame <= 536 {
				wilFile = resources.Effect
				wingIdx = 352 + a.WingFrame
			}
		} else {
			wilFile = resources.HumEffect
			offset := (a.Effect - 1) * HumanFrame
			if a.CurrentFrame < 64 {
				wingIdx = offset + a.Dir*8 + a.WingFrame
			} else {
				wingIdx = offset + a.CurrentFrame
			}
		}
		if wilFile != nil && wingIdx >= 0 && wingIdx < wilFile.Count {
			img := wilFile.GetImage(wingIdx)
			if img != nil && img.RGBA != nil {
				tex := resources.GetTexture(wilFile, wingIdx)
				dy := worldY + float32(img.HotY)
				h := float32(img.Height)
				dy, h = applyScale(dy, h, scale)
				bounds = append(bounds, LayerBounds{
					LayerName: "wing",
					WilName:   wilDisplayName(wilFile, resources),
					ImageIdx:  wingIdx,
					HotX:      img.HotX,
					HotY:      img.HotY,
					Width:     img.Width,
					Height:    int(h),
					DrawX:     worldX + float32(img.HotX),
					DrawY:     dy,
					TexID:     tex,
					Img:       img,
				})
			}
		}
	}

	return bounds
}
