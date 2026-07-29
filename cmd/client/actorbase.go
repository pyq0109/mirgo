package main

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
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

	RealActionMsg ActorMsg
	HasRealAction bool

	Type ActorType

	MonAction *MonsterAction
	NpcAppr   int

	HitEffectNumber int

	UseMagic       bool
	SpellFrame     int
	CurEffFrame    int
	EffectFrame    int  // 特效动画帧计数器（魔法盾/武器光效循环）
	SpellConfirmed bool // F4: 服务端确认施法（SMMagicFire 到达后置 true）

	Effect  int
	OnHorse bool
	State   int32
	IsSelf  bool

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
	NameColor       int   // SMChangeNameColor: 名字颜色（0=默认, 1=红, 2=黄）
	ShowHP          bool  // SMOpenHealth: 显示头顶HP条
	ShowHPVal       int   // 当前HP
	ShowMaxHPVal    int   // 最大HP
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
	if len(a.MsgList) == 0 {
		return ActorMsg{}, false
	}
	msg := a.MsgList[0]
	a.MsgList = a.MsgList[1:]
	return msg, true
}

func (a *Actor) ProcMsg() {
	for {
		if len(a.MsgList) == 0 {
			break
		}
		next := a.MsgList[0]
		isStruck := next.Ident == protocol.SMStruck
		if a.CurrentAction != 0 && !isStruck {
			break
		}
		if a.LockEndFrame && !isStruck {
			break
		}
		msg, _ := a.GetMessage()
		a.ReadyAction(msg)
		if !isStruck {
			break
		}
	}
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
		if msg.Ident == protocol.CMHorseRun {
			msg.Ident = protocol.SMHorseRun
		} else {
			msg.Ident = msg.Ident - 3000
		}
	}

	a.CurrX = msg.X
	a.CurrY = msg.Y
	a.Dir = msg.Dir

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
	return a.CurrentAction == 0 && len(a.MsgList) == 0
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
	a.SendMsg(ident, x, y, dir, feature, state)
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

func (a *Actor) calcNPCFrame() {
	npcDir := a.Dir % 3
	a.StartFrame = GetNpcOffset(a.Appearance) + npcDir*8
	a.EndFrame = a.StartFrame + 7
	a.CurrentFrame = a.StartFrame
	a.FrameTime = 200
	a.LastFrameTick = 0
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
					a.RunActSound(a.CurrentFrame - a.StartFrame)
				}
			} else {
				a.CurrentAction = 0
				a.UseMagic = false
				a.SpellConfirmed = false
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
}

func (a *Actor) DefaultMotion(now int64) {
	// D2: 特效动画帧递增（魔法盾/武器光效/施法循环）
	if now-a.DefFrameTime > 100 {
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

	if dx != 0 && dy != 0 {
		a.ShiftX *= 0.7071
		a.ShiftY *= 0.7071
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
	return resources.Hum.GetImage(idx)
}

func (a *Actor) getHumanHairImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Hair == nil || a.Hair == 0 {
		return nil
	}
	idx := HumanFrame*a.Hair + a.CurrentFrame
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
	img := a.GetBodyImage(resources)
	if img == nil || img.RGBA == nil {
		return
	}
	wilFile := getWilFile(resources, a.Type, a.Appearance)
	if wilFile == nil {
		return
	}
	tex := resources.GetTexture(wilFile, a.getTextureIndex())
	if tex == 0 {
		return
	}
	w := float32(img.Width)
	h := float32(img.Height)
	drawX := screenX + float32(img.HotX)
	drawY := screenY + float32(img.HotY)

	blend := a.State&0x00800000 != 0
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	}
	tr, tg, tb, tinted := getStateTint(a.State)
	drawTintedQuad(glState, tex, drawX, drawY, w, h, tr, tg, tb, tinted, stateAlpha(a.State), proj)
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

func (a *Actor) wingBehind() bool {
	return a.Dir >= 3 && a.Dir <= 5
}

func (a *Actor) drawHuman(glState *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	wpord := getWordOrder(a.Sex, a.CurrentFrame)

	blend := a.State&0x00800000 != 0
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	}

	// 状态着色与隐身透明度对全部图层（身体/头发/武器/翅膀）一致生效。
	tintR, tintG, tintB, useTint := getStateTint(a.State)
	alpha := stateAlpha(a.State)

	if a.Effect > 0 && a.wingBehind() {
		a.drawWingLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, proj)
	}

	if wpord == 0 && a.Weapon >= 2 {
		a.drawWeaponLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, proj)
	}

	bodyIdx := HumanFrame*a.Dress + a.CurrentFrame
	if resources.Hum != nil && bodyIdx >= 0 && bodyIdx < resources.Hum.Count {
		img := resources.Hum.GetImage(bodyIdx)
		if img != nil && img.RGBA != nil {
			tex := resources.GetTexture(resources.Hum, bodyIdx)
			if tex != 0 {
				w := float32(img.Width)
				h := float32(img.Height)
				bx := screenX + float32(img.HotX)
				by := screenY + float32(img.HotY)
				drawTintedQuad(glState, tex, bx, by, w, h, tintR, tintG, tintB, useTint, alpha, proj)
			}
		}
	}

	hairIdx := HumanFrame*a.Hair + a.CurrentFrame
	if a.Hair > 0 && resources.Hair != nil && hairIdx >= 0 && hairIdx < resources.Hair.Count {
		img := resources.Hair.GetImage(hairIdx)
		if img != nil && img.RGBA != nil {
			tex := resources.GetTexture(resources.Hair, hairIdx)
			if tex != 0 {
				w := float32(img.Width)
				h := float32(img.Height)
				hx := screenX + float32(img.HotX)
				hy := screenY + float32(img.HotY)
				drawTintedQuad(glState, tex, hx, hy, w, h, tintR, tintG, tintB, useTint, alpha, proj)
			}
		}
	}

	if wpord == 1 && a.Weapon >= 2 {
		a.drawWeaponLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, proj)
	}

	if a.Effect > 0 && !a.wingBehind() {
		a.drawWingLayer(glState, resources, screenX, screenY, tintR, tintG, tintB, useTint, alpha, proj)
	}

	// D2 Layer 7: 魔法盾泡泡（STATE_BUBBLEDEFENCEUP = 0x00020000）
	if a.State&0x00020000 != 0 && resources.Magic != nil {
		bubbleBase := 1570 // Delphi MAGICBASE + bubble offset
		frame := bubbleBase + (a.EffectFrame % 10)
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

	// D2 Layer 8: 施法特效
	if a.EffectNumber > 0 && resources.Magic != nil {
		spellIdx := a.EffectNumber*10 + (a.EffectFrame % 10)
		if spellIdx >= 0 && spellIdx < resources.Magic.Count {
			if img := resources.Magic.GetImage(spellIdx); img != nil && img.RGBA != nil {
				if tex := resources.GetTexture(resources.Magic, spellIdx); tex != 0 {
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
					glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
						float32(img.Width), float32(img.Height), proj)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				}
			}
		}
	}

	// D2 Layer 9: 攻击特效（方向性火花）
	if a.HitEffectNumber > 0 && resources.Magic != nil {
		hitBase := 1450 + a.HitEffectNumber*10 // Delphi HITEFFECTBASE
		hitIdx := hitBase + a.Dir
		if hitIdx >= 0 && hitIdx < resources.Magic.Count {
			if img := resources.Magic.GetImage(hitIdx); img != nil && img.RGBA != nil {
				if tex := resources.GetTexture(resources.Magic, hitIdx); tex != 0 {
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
					glState.DrawQuad(tex, screenX+float32(img.HotX), screenY+float32(img.HotY),
						float32(img.Width), float32(img.Height), proj)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				}
			}
		}
	}

	// D2 Layer 10: 武器光效
	if a.WeaponEffect > 0 && resources.Magic != nil {
		wpEffIdx := 1700 + a.Dir*10 + (a.EffectFrame % 10) // Delphi WPEFFECTBASE
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

	// D4: 传送动画（垂直缩放）
	if a.ScrollHideState == 1 || a.ScrollHideState == 3 {
		a.updateScrollHide()
	}

	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

func (a *Actor) drawWeaponLayer(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, tintR, tintG, tintB float32, useTint bool, alpha float32, proj [16]float32) {
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
	drawTintedQuad(gl, tex, screenX+float32(img.HotX), screenY+float32(img.HotY), w, h, tintR, tintG, tintB, useTint, alpha, proj)
}

func (a *Actor) drawWingLayer(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, tintR, tintG, tintB float32, useTint bool, alpha float32, proj [16]float32) {
	if resources.HumEffect == nil {
		return
	}
	wingIdx := (a.Effect-1)*HumanFrame + a.CurrentFrame
	if wingIdx < 0 || wingIdx >= resources.HumEffect.Count {
		return
	}
	img := resources.HumEffect.GetImage(wingIdx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := resources.GetTexture(resources.HumEffect, wingIdx)
	if tex == 0 {
		return
	}
	w := float32(img.Width)
	h := float32(img.Height)
	drawTintedQuad(gl, tex, screenX+float32(img.HotX), screenY+float32(img.HotY), w, h, tintR, tintG, tintB, useTint, alpha, proj)
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
		offsets := []int{0, 360, 440, 550, 700, 830, 950, 1060, 1170}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return npos * 360
	case 14, 15, 16:
		return npos * 360
	case 17:
		offsets := []int{0, 360, 920}
		if npos < len(offsets) {
			return offsets[npos]
		}
		return npos * 360
	case 18, 19, 20, 21, 22, 23, 24, 25, 26, 27:
		return npos * 360
	case 80:
		return npos * 600
	case 90:
		return npos * 360
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
