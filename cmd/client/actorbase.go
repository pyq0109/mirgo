package main

import (
	"encoding/binary"
	"math"

	"github.com/pyq0109/mirgo/internal/engine"
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

	Sex        int
	Race       int
	Hair       int
	Dress      int
	Weapon     int
	Job        int
	Appearance int

	Death    bool
	Skeleton bool
	WarMode  bool

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

	MsgList []ActorMsg

	Type ActorType

	MonAction *MonsterAction
	NpcAppr   int
}

func NewActor(recogID int32, x, y, dir int) *Actor {
	return &Actor{
		RecogID: recogID,
		CurrX:   x,
		CurrY:   y,
		Rx:      x,
		Ry:      y,
		Dir:     dir,
	}
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
	for a.CurrentAction == 0 && !a.LockEndFrame {
		msg, ok := a.GetMessage()
		if !ok {
			break
		}
		a.ReadyAction(msg)
	}
}

func (a *Actor) ReadyAction(msg ActorMsg) {
	switch msg.Ident {
	case protocol.SMWalk, protocol.SMRun, protocol.SMTurn, protocol.SMRush, protocol.SMRushKung:
		a.updateFeature(msg.Feature)
	}

	a.CurrX = msg.X
	a.CurrY = msg.Y
	a.Dir = msg.Dir

	a.CurrentAction = msg.Ident
	a.CalcActorFrame()

	if msg.Ident == protocol.SMWalk || msg.Ident == protocol.SMRun {
		a.Shift(a.Dir, a.MoveStep, 0, a.EndFrame-a.StartFrame+1)
	}
}

func (a *Actor) updateFeature(feature int) {
	if feature == 0 {
		return
	}
	_, dress, weapon, hair := protocol.ParseHumanFeature(int32(feature))
	a.Dress = int(dress)
	a.Weapon = int(weapon)
	a.Hair = int(hair)
}

func (a *Actor) updateFeatureFromLogon(body string) {
	buf := make([]byte, 16)
	protocol.DecodeBuffer(body, buf)
	if len(buf) >= 4 {
		feature := int32(binary.LittleEndian.Uint32(buf[0:4]))
		a.updateFeature(int(feature))
	}
}

func (a *Actor) updateFeatureFromBody(body string) {
	if body == "" {
		return
	}
	buf := make([]byte, 8)
	protocol.DecodeBuffer(body, buf)
	if len(buf) >= 4 {
		feature := int32(binary.LittleEndian.Uint32(buf[0:4]))
		a.updateFeature(int(feature))
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
	case protocol.SMRush, protocol.SMRushKung:
		action = HA.ActRun
		a.MoveStep = 1
	case protocol.SMHit, protocol.SMHeavyHit, protocol.SMBigHit,
		protocol.SMPowerHit, protocol.SMLongHit, protocol.SMWideHit,
		protocol.SMCrsHit, protocol.SMTwinHit, protocol.SMFireHit:
		action = HA.ActHit
	case protocol.SMSpell:
		action = HA.ActSpell
	case protocol.SMStruck:
		action = HA.ActStruck
	case protocol.SMDeath, protocol.SMNowDeath:
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
	case protocol.SMHit, protocol.SMStruck:
		action = a.MonAction.ActAttack
	case protocol.SMDeath, protocol.SMNowDeath:
		action = a.MonAction.ActDie
	default:
		action = a.MonAction.ActStand
	}
	a.StartFrame, a.EndFrame = CalcFrame(action, a.Dir)
	a.CurrentFrame = a.StartFrame
	a.FrameTime = action.FTime
	a.LastFrameTick = 0
}

func (a *Actor) calcNPCFrame() {
	a.StartFrame = GetNpcOffset(a.Appearance) + a.Dir*8
	a.EndFrame = a.StartFrame + 7
	a.CurrentFrame = a.StartFrame
	a.FrameTime = 200
	a.LastFrameTick = 0
}

func (a *Actor) Move(now int64) bool {
	switch a.CurrentAction {
	case protocol.SMWalk, protocol.SMRun, protocol.SMRush, protocol.SMRushKung:
	default:
		return false
	}

	if now-a.LastFrameTick < int64(a.FrameTime) {
		return true
	}
	a.LastFrameTick = now

	if a.CurrentFrame < a.EndFrame {
		a.CurrentFrame++
		cur := a.CurrentFrame - a.StartFrame + 1
		max := a.EndFrame - a.StartFrame + 1
		a.Shift(a.Dir, a.MoveStep, cur, max)
	}

	if a.CurrentFrame >= a.EndFrame {
		a.CurrentAction = 0
		a.LockEndFrame = true
		a.SmoothMoveTime = now
	}
	return true
}

func (a *Actor) Run(now int64) {
	if a.CurrentAction == protocol.SMWalk || a.CurrentAction == protocol.SMRun ||
		a.CurrentAction == protocol.SMRush || a.CurrentAction == protocol.SMRushKung {
		return
	}

	if a.CurrentAction != 0 {
		if now-a.LastFrameTick >= int64(a.FrameTime) {
			a.LastFrameTick = now
			if a.CurrentFrame < a.EndFrame {
				a.CurrentFrame++
			} else {
				a.CurrentAction = 0
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
	if a.Death {
		a.CurrentFrame = a.getEndFrame()
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
	case protocol.DRUp:
		dx, dy = 0, -1
	case protocol.DRUpRight:
		dx, dy = 1, -1
	case protocol.DRRight:
		dx, dy = 1, 0
	case protocol.DRDownRight:
		dx, dy = 1, 1
	case protocol.DRDown:
		dx, dy = 0, 1
	case protocol.DRDownLeft:
		dx, dy = -1, 1
	case protocol.DRLeft:
		dx, dy = -1, 0
	case protocol.DRUpLeft:
		dx, dy = -1, -1
	}

	fCur := float64(cur)
	fMax := float64(max)

	a.Rx = a.CurrX - dx*step + int(math.Round((fMax-fCur)/fMax))*dx*step
	a.Ry = a.CurrY - dy*step + int(math.Round((fMax-fCur)/fMax))*dy*step

	remainX := unx * (1.0 - fCur/fMax)
	remainY := uny * (1.0 - fCur/fMax)

	a.ShiftX = -float64(dx) * remainX
	a.ShiftY = -float64(dy) * remainY
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

func (a *Actor) drawBody(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
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
	gl.DrawQuad(tex, screenX, screenY-h+engine.TileHeight, w, h, proj)
}

func (a *Actor) drawHuman(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	bodyIdx := HumanFrame*a.Dress + a.CurrentFrame
	if resources.Hum != nil && bodyIdx >= 0 && bodyIdx < resources.Hum.Count {
		img := resources.Hum.GetImage(bodyIdx)
		if img != nil && img.RGBA != nil {
			tex := resources.GetTexture(resources.Hum, bodyIdx)
			if tex != 0 {
				w := float32(img.Width)
				h := float32(img.Height)
				gl.DrawQuad(tex, screenX, screenY-h+engine.TileHeight, w, h, proj)
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
				gl.DrawQuad(tex, screenX, screenY-h+engine.TileHeight, w, h, proj)
			}
		}
	}

	weaponIdx := HumanFrame*a.Weapon + a.CurrentFrame
	if a.Weapon > 0 && resources.Weapon != nil && weaponIdx >= 0 && weaponIdx < resources.Weapon.Count {
		img := resources.Weapon.GetImage(weaponIdx)
		if img != nil && img.RGBA != nil {
			tex := resources.GetTexture(resources.Weapon, weaponIdx)
			if tex != 0 {
				w := float32(img.Width)
				h := float32(img.Height)
				gl.DrawQuad(tex, screenX, screenY-h+engine.TileHeight, w, h, proj)
			}
		}
	}
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
