package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

// ActorType represents the type of actor.
type ActorType int

const (
	ActorHuman   ActorType = 0
	ActorMonster ActorType = 1
	ActorNPC     ActorType = 2
)

// Actor represents a game entity (player, monster, NPC).
type Actor struct {
	// Identity
	RecogID  int32
	UserName string

	// Position
	CurrX, CurrY int
	Dir          int
	ShiftX, ShiftY float64

	// Appearance
	Sex        int
	Race       int
	Hair       int
	Dress      int
	Weapon     int
	Job        int
	Appearance int

	// State
	Death    bool
	Skeleton bool
	WarMode  bool

	// Rendering
	BodySurface   *wil.Image
	BodyOffset    int
	HairSurface   *wil.Image
	HairOffset    int
	WeaponSurface *wil.Image
	WeaponOffset  int

	// Animation
	StartFrame   int
	EndFrame     int
	CurrentFrame int
	FrameTime    int
	LastFrameTick int64

	// Action
	Action     *ActionInfo
	ActionName string

	// Type
	Type ActorType

	// Monster specific
	MonAction *MonsterAction

	// NPC specific
	NpcAppr int
}

// NewActor creates a new actor.
func NewActor(recogID int32, x, y, dir int) *Actor {
	return &Actor{
		RecogID: recogID,
		CurrX:   x,
		CurrY:   y,
		Dir:     dir,
	}
}

// SetAction sets the current action for the actor.
func (a *Actor) SetAction(action ActionInfo, dir int) {
	a.Action = &action
	a.Dir = dir
	a.StartFrame, a.EndFrame = CalcFrame(action, dir)
	a.CurrentFrame = a.StartFrame
	a.FrameTime = action.FTime
	a.LastFrameTick = 0
}

// UpdateAnimation updates the animation frame.
func (a *Actor) UpdateAnimation(now int64) bool {
	if a.Action == nil {
		return false
	}
	if now-a.LastFrameTick < int64(a.FrameTime) {
		return false
	}
	a.LastFrameTick = now

	if a.CurrentFrame < a.EndFrame {
		a.CurrentFrame++
		return true
	}

	// Loop back to start
	a.CurrentFrame = a.StartFrame
	return true
}

// GetBodyImage returns the body image for the actor.
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
	if idx < 0 || idx >= len(resources.Hum.Images) {
		return nil
	}
	return resources.Hum.Images[idx]
}

func (a *Actor) getMonsterBodyImage(resources *engine.ResourceManager) *wil.Image {
	// Get the correct Mon*.wil file based on appearance
	monFile := a.getMonFile(resources)
	if monFile == nil {
		return nil
	}

	offset := GetMonOffset(a.Appearance)
	idx := offset + a.CurrentFrame
	if idx < 0 || idx >= len(monFile.Images) {
		return nil
	}
	return monFile.Images[idx]
}

func (a *Actor) getMonFile(resources *engine.ResourceManager) *wil.File {
	nrace := a.Appearance / 10
	switch nrace {
	case 0:
		return resources.Mon[0]
	case 1:
		return resources.Mon[1]
	case 2:
		return resources.Mon[2]
	case 3:
		return resources.Mon[3]
	case 4:
		return resources.Mon[4]
	case 5:
		return resources.Mon[5]
	case 6:
		return resources.Mon[6]
	case 7:
		return resources.Mon[7]
	case 8:
		return resources.Mon[8]
	case 9:
		return resources.Mon[9]
	case 10:
		return resources.Mon[10]
	case 11:
		return resources.Mon[11]
	case 12:
		return resources.Mon[12]
	case 13:
		return resources.Mon[13]
	case 14:
		return resources.Mon[14]
	case 15:
		return resources.Mon[15]
	case 16:
		return resources.Mon[16]
	case 17:
		return resources.Mon[17]
	default:
		return resources.Mon[0]
	}
}

func (a *Actor) getNPCBodyImage(resources *engine.ResourceManager) *wil.Image {
	if resources.Npc == nil {
		return nil
	}
	offset := GetNpcOffset(a.Appearance)
	idx := offset + a.CurrentFrame
	if idx < 0 || idx >= len(resources.Npc.Images) {
		return nil
	}
	return resources.Npc.Images[idx]
}

// GetMonOffset returns the base frame offset for a monster appearance.
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

// GetNpcOffset returns the base frame offset for an NPC appearance.
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

// Draw renders the actor at the specified screen position.
func (a *Actor) Draw(gl *engine.GLState, resources *engine.ResourceManager, screenX, screenY float32, proj [16]float32) {
	img := a.GetBodyImage(resources)
	if img == nil || img.RGBA == nil {
		return
	}

	tex := resources.GetTexture(getWilFile(resources, a.Type, a.Appearance), a.getTextureIndex(resources))
	if tex == 0 {
		return
	}

	// Draw at bottom-aligned position
	w := float32(img.Width)
	h := float32(img.Height)
	gl.DrawQuad(tex, screenX, screenY-h+engine.TileHeight, w, h, proj)
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

func (a *Actor) getTextureIndex(resources *engine.ResourceManager) int {
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
