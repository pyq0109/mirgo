package main

import (
	"github.com/pyq0109/mirgo/internal/log"
)

const (
	pkKillAddPoints   = 100
	pkDecayInterval   = int64(120000)
	pkDecayAmount     = 1
	pkLevel1Threshold = 100
	pkLevel2Threshold = 200
)

func (p *PlayObject) PKLevel() int {
	return p.PkPoint / 100
}

func (p *PlayObject) IncPkPoint(points int) {
	oldLevel := p.PKLevel()
	p.PkPoint += points
	if p.PKLevel() != oldLevel {
		log.Logf(log.LevelInfo, "PK", "%s PK level changed: %d -> %d", p.Name, oldLevel, p.PKLevel())
	}
}

func (p *PlayObject) DecayPkPoint(now int64) {
	if now-p.LastPkDecayTick < pkDecayInterval {
		return
	}
	p.LastPkDecayTick = now
	if p.PkPoint > 0 {
		p.PkPoint -= pkDecayAmount
		if p.PkPoint < 0 {
			p.PkPoint = 0
		}
	}
}

func (p *PlayObject) NameColor() int {
	level := p.PKLevel()
	if level >= 2 {
		return 249
	}
	if level >= 1 {
		return 251
	}
	return 255
}

func (p *PlayObject) OnPlayerKilled(victim *PlayObject) {
	if IsSafeZone(p.envir, victim.CurrX, victim.CurrY) {
		return
	}
	p.IncPkPoint(pkKillAddPoints)
	log.Logf(log.LevelInfo, "PK", "%s killed %s, PK points: %d (level %d)", p.Name, victim.Name, p.PkPoint, p.PKLevel())
}
