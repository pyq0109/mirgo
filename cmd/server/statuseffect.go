package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/netserver"
)

const (
	POISON_DECHEALTH    = 0
	POISON_DAMAGEARMOR  = 1
	POISON_LOCKSPELL    = 2
	POISON_DONTMOVE     = 4
	POISON_STONE        = 5
	STATE_TRANSPARENT   = 8
	STATE_DEFENCEUP     = 9
	STATE_MAGDEFENCEUP  = 10
	STATE_BUBBLEDEFENCE = 11
)

func (p *PlayObject) ProcessStatusEffects(server *netserver.TCPServer, now int64) {
	for i := 0; i < 12; i++ {
		if p.StatusTimeArr[i] > 0 {
			p.StatusTimeArr[i]--
			// Delphi: PoisonRecover 加速恢复
			if p.PoisonRecover > 0 && rand.Intn(p.PoisonRecover+1) == 0 {
				p.StatusTimeArr[i]--
			}
			if p.StatusTimeArr[i] <= 0 {
				p.StatusTimeArr[i] = 0
				if i == STATE_TRANSPARENT {
					p.Hidden = false
				}
			}
		}
	}

	// Delphi: 绿毒 DamageHealth(m_btGreenPoisoningPoint + 1) (ObjBase.pas:4261)
	if p.StatusTimeArr[POISON_DECHEALTH] > 0 {
		dmg := p.GreenPoisonDamage
		if dmg < 1 {
			dmg = 2
		}
		hp := int(p.WAbil.HP) - dmg
		if hp <= 0 {
			hp = 0
			p.WAbil.HP = 0
			if !p.Death {
				p.Death = true
				p.deathTick = now
				if p.envir != nil {
					p.envir.broadcastDeathMsg(p.BaseObject, p.ID, p.CurrX, p.CurrY, p.Dir, true)
				}
			}
			return
		}
		p.WAbil.HP = uint16(hp)
		p.sendHealthSpell(server)
	}
}

// MakePoison — Delphi MakePosion (ObjBase.pas:22730)
// power: 绿毒每次 tick 伤害（m_btGreenPoisoningPoint），其他效果传 0。
func (p *PlayObject) MakePoison(effectIdx int, duration int16, power int) {
	if effectIdx < 0 || effectIdx >= 12 {
		return
	}
	// Delphi: 已有更长时间则不覆盖
	if p.StatusTimeArr[effectIdx] > duration {
		return
	}
	p.StatusTimeArr[effectIdx] = duration
	if effectIdx == POISON_DECHEALTH && power > 0 {
		p.GreenPoisonDamage = power
	}
	if effectIdx == STATE_TRANSPARENT {
		p.Hidden = true
	}
}

func (p *PlayObject) IsStone() bool {
	return p.StatusTimeArr[POISON_STONE] > 0
}

func (p *PlayObject) CanMoveCheck() bool {
	return p.StatusTimeArr[POISON_DONTMOVE] <= 0 && p.StatusTimeArr[POISON_STONE] <= 0
}

func (p *PlayObject) CanCast() bool {
	return p.StatusTimeArr[POISON_LOCKSPELL] <= 0 && p.StatusTimeArr[POISON_STONE] <= 0
}
