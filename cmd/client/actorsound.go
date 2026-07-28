package main

import (
	"math/rand/v2"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// initSoundDefaults 设置声音字段默认值（Delphi Actor.pas:1258-1266）。
func (a *Actor) initSoundDefaults() {
	a.FootStepSound = -1
	a.StruckSound = sStruckBodyLongstick // Delphi 默认 s_struck_body_longstick
	a.StruckWeaponSound = -1
	a.AppearSound = -1
	a.NormalSound = -1
	a.AttackSound = -1
	a.WeaponSound = -1
	a.ScreamSound = -1
	a.DieSound = -1
	a.Die2Sound = -1
	a.MagicStartSound = -1
	a.MagicFireSound = -1
	a.MagicExplosionSound = -1
	a.MagicStruckSound = -1
}

// SetSound 按当前动作和角色属性计算声音字段（Delphi Actor.pas:2129-2332）。
func (a *Actor) SetSound(mapData *mapformat.MapData) {
	if a.Type == ActorHuman {
		a.setSoundHuman(mapData)
	} else if a.Type == ActorMonster {
		a.setSoundMonster()
	}
}

func (a *Actor) setSoundHuman(mapData *mapformat.MapData) {
	// 脚步声：仅本机玩家、移动动作（Actor.pas:2134-2142）
	if a.IsSelf && mapData != nil {
		switch a.CurrentAction {
		case protocol.SMWalk, protocol.SMBackStep, protocol.SMRun,
			protocol.SMHorseRun, protocol.SMRush, protocol.SMRushKung:
			a.FootStepSound = a.calcFootstep(mapData)
		}
	}

	// 惨叫/死亡按性别（Actor.pas:2242-2248）
	if a.Sex == 0 {
		a.ScreamSound = sManStruck
		a.DieSound = sManDie
	} else {
		a.ScreamSound = sWomStruck
		a.DieSound = sWomDie
	}

	// 挥击声按武器（Actor.pas:2250-2262）
	switch a.CurrentAction {
	case protocol.SMThrow, protocol.SMHit, protocol.SMHeavyHit, protocol.SMBigHit,
		protocol.SMPowerHit, protocol.SMLongHit, protocol.SMWideHit,
		protocol.SMFireHit, protocol.SMCrsHit, protocol.SMTwinHit:
		a.WeaponSound = weaponSoundIdx(a.Weapon)
	}

	// 受击声（Actor.pas:2264-2294）
	if a.CurrentAction == protocol.SMStruck {
		if a.MagicStruckSound >= 1 {
			// 魔法受击声优先，跳过普通受击声
		} else {
			a.calcStruckSound()
		}
	}

	// 魔法三段声（Actor.pas:2297-2302）
	if a.UseMagic && a.MagicSerial > 0 {
		a.MagicStartSound = 10000 + a.MagicSerial*10
		a.MagicFireSound = 10000 + a.MagicSerial*10 + 1
		a.MagicExplosionSound = 10000 + a.MagicSerial*10 + 2
	}
}

func (a *Actor) calcFootstep(mapData *mapformat.MapData) int {
	// 坐标取样（Actor.pas:2144-2150）
	cx := a.CurrX
	cy := a.CurrY
	cx = cx / 2 * 2
	cy = cy / 2 * 2

	cell := mapData.At(cx, cy)
	if cell == nil {
		return sWalkGroundL
	}

	base := terrainFootstepSound(cell.BkImg, cell.Area, cell.MidImg, cell.FrImg)

	// 跑步偏移（Actor.pas:2237-2238）
	if a.CurrentAction == protocol.SMRun || a.CurrentAction == protocol.SMHorseRun {
		base += 2
	}

	return base
}

func (a *Actor) calcStruckSound() {
	// 需要找攻击者（Actor.pas:2269-2294）
	// HiterCode 由 SM_STRUCK 消息的 State 字段携带
	if a.HiterCode == 0 {
		return
	}
	// 攻击者信息需要从 ActorManager 查找，但 Actor 不持有 manager 引用。
	// 简化处理：使用默认受击声（不区分武器/护甲）。
	// 完整实现需要在 SM_STRUCK 消息中携带攻击者武器信息。
}

func (a *Actor) setSoundMonster() {
	// 外观公式（Actor.pas:2323-2332）
	if a.Race == 50 { // NPC 类跳过
		return
	}
	base := 200 + a.Appearance*10
	a.AppearSound = base     // +0 出现
	a.NormalSound = base + 1  // +1 平时
	a.AttackSound = base + 2  // +2 攻击
	a.WeaponSound = base + 3  // +3 武器
	a.ScreamSound = base + 4  // +4 惨叫
	a.DieSound = base + 5     // +5 死亡
	a.Die2Sound = base + 6    // +6 死亡2

	// 怪物受击声（Actor.pas:2305-2321）
	if a.CurrentAction == protocol.SMStruck {
		a.StruckSound = sStruckBodyFist // 怪物默认肉体受击
	}
}

// RunSound 在动作起始时播放一次性声音（Delphi Actor.pas:2357-2390）。
func (a *Actor) RunSound() {
	a.BoRunSound = true
	a.SetSound(a.MapRef)

	switch a.CurrentAction {
	case protocol.SMStruck:
		if a.StruckWeaponSound >= 0 {
			gSound.PlaySound(a.StruckWeaponSound)
		}
		if a.StruckSound >= 0 {
			gSound.PlaySound(a.StruckSound)
		}
		if a.ScreamSound >= 0 {
			gSound.PlaySound(a.ScreamSound)
		}

	case protocol.SMNowDeath:
		if a.DieSound >= 0 {
			gSound.PlaySound(a.DieSound)
		}
		if a.IsSelf {
			gSound.PlayBGM(bmgGameover)
		}

	case protocol.SMThrow, protocol.SMHit, protocol.SMFlyAxe,
		protocol.SMLighting, protocol.SMDigDown:
		if a.AttackSound >= 0 {
			gSound.PlaySound(a.AttackSound)
		}

	case protocol.SMAlive, protocol.SMDigUp:
		if a.AppearSound >= 0 {
			gSound.PlaySound(a.AppearSound)
		}

	case protocol.SMSpell:
		if a.MagicStartSound >= 0 {
			gSound.PlaySound(a.MagicStartSound)
		}
	}
}

// RunActSound 在动画帧推进时播放帧驱动声音（Delphi Actor.pas:2392-2472）。
func (a *Actor) RunActSound(frameOffset int) {
	if !a.BoRunSound {
		return
	}

	if a.Type == ActorHuman {
		a.runActSoundHuman(frameOffset)
	} else if a.Type == ActorMonster && a.Race != 50 {
		a.runActSoundMonster(frameOffset)
	}
}

func (a *Actor) runActSoundHuman(frame int) {
	switch a.CurrentAction {
	case protocol.SMThrow, protocol.SMHit, protocol.SMHeavyHit, protocol.SMBigHit:
		if frame == 2 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			a.BoRunSound = false
		}

	case protocol.SMPowerHit:
		if frame == 2 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			if a.Sex == 0 {
				gSound.PlaySound(sYedoMan)
			} else {
				gSound.PlaySound(sYedoWoman)
			}
			a.BoRunSound = false
		}

	case protocol.SMLongHit:
		if frame == 2 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			gSound.PlaySound(sLongHit)
			a.BoRunSound = false
		}

	case protocol.SMWideHit:
		if frame == 2 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			gSound.PlaySound(sWideHit)
			a.BoRunSound = false
		}

	case protocol.SMFireHit, protocol.SMCrsHit, protocol.SMTwinHit:
		if frame == 2 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			gSound.PlaySound(sFirehit)
			a.BoRunSound = false
		}
	}
}

func (a *Actor) runActSoundMonster(frame int) {
	switch a.CurrentAction {
	case protocol.SMWalk, protocol.SMTurn:
		// 1/8 概率平时声（Actor.pas:2445-2449）
		if frame == 1 && rand.IntN(8) == 1 {
			if a.NormalSound >= 0 {
				gSound.PlaySound(a.NormalSound)
			}
		}

	case protocol.SMHit:
		if frame == 3 && a.AttackSound >= 0 {
			if a.WeaponSound >= 0 {
				gSound.PlaySound(a.WeaponSound)
			}
			a.BoRunSound = false
		}

	case protocol.SMNowDeath:
		// appearance=80 石头怪类死亡2（Actor.pas:2457-2466）
		if a.Appearance == 80 && frame == 2 {
			if a.Die2Sound >= 0 {
				gSound.PlaySound(a.Die2Sound)
			}
			a.BoRunSound = false
		}
	}
}

// PlayFootstep 在移动动画帧推进时播放脚步声（Delphi Actor.pas:2650-2661）。
func (a *Actor) PlayFootstep(frameOffset int) {
	if a.FootStepSound < 0 {
		return
	}
	switch frameOffset {
	case 1:
		gSound.PlaySound(a.FootStepSound) // 左脚
	case 4:
		gSound.PlaySound(a.FootStepSound + 1) // 右脚
	}
}
