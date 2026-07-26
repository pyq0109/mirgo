package main

import (
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
			if p.StatusTimeArr[i] == 0 {
				if i == STATE_TRANSPARENT {
					p.Hidden = false
				}
			}
		}
	}

	if p.StatusTimeArr[POISON_DECHEALTH] > 0 {
		hp := int(p.WAbil.HP)
		hp -= 2
		if hp <= 0 {
			hp = 1
		}
		p.WAbil.HP = uint16(hp)
	}
}

func (p *PlayObject) MakePoison(effectIdx int, duration int16) {
	if effectIdx < 0 || effectIdx >= 12 {
		return
	}
	p.StatusTimeArr[effectIdx] = duration
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
