package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	pkKillAddPoints   = 100
	pkDecayInterval   = int64(120000)
	pkDecayAmount     = 1
	pkLevel1Threshold = 100
	pkLevel2Threshold = 200
	pkFlagDuration    = int64(60000) // 60 秒正当防卫窗口
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

// SetPKFlag 被玩家击中标记正当防卫旗（Delphi SetPKFlag, ObjBase.pas:21220-21236）。
// 双方 PKLevel<2 且非格斗区时，给被击者 60 秒黄旗。
func (p *PlayObject) SetPKFlag(attacker *PlayObject) {
	if p.PKLevel() >= 2 || attacker.PKLevel() >= 2 {
		return
	}
	p.PKFlag = true
	p.PKFlagTick = time.Now().UnixMilli()
}

// IsGoodKilling 正当防卫判定：死者带 PK 旗则击杀者无罪（Delphi ObjBase.pas:21251-21255）。
func (p *PlayObject) IsGoodKilling(victim *PlayObject) bool {
	return victim.PKFlag && time.Now().UnixMilli()-victim.PKFlagTick < pkFlagDuration
}

// CheckPKStatus 清除过期 PK 旗（Delphi ObjBase.pas:18868-18875）。
func (p *PlayObject) CheckPKStatus(now int64) {
	if p.PKFlag && now-p.PKFlagTick >= pkFlagDuration {
		p.PKFlag = false
	}
}

// BroadcastNameColor 向视野内玩家广播名字颜色变化。
func (p *PlayObject) BroadcastNameColor(server *netserver.TCPServer) {
	color := p.NameColor()
	p.SendRefMsg(RM_CHANGENAMECOLOR, color, p.CurrX, p.CurrY, "")
	msg := protocol.MakeDefaultMsg(protocol.SMChangeNameColor, p.ID, uint16(color), 0, 0)
	server.Send(p.Session.ID, msg, "")
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

func (p *PlayObject) OnPlayerKilled(server *netserver.TCPServer, victim *PlayObject) {
	if IsSafeZone(p.envir, victim.CurrX, victim.CurrY) {
		return
	}

	// Delphi: 正当防卫 — 死者带 PK 旗则击杀者无罪 (ObjBase.pas:21251-21255)
	if p.IsGoodKilling(victim) {
		log.Logf(log.LevelInfo, "PK", "%s killed %s (self-defense, no PK points)", p.Name, victim.Name)
	} else {
		p.IncPkPoint(pkKillAddPoints)
		p.BroadcastNameColor(server)
	}

	if victim.WAbil.Exp > 0 {
		penalty := victim.WAbil.Exp / 20
		victim.WAbil.Exp -= penalty
	}

	if p.PKLevel() >= 1 && rand.Intn(5) == 0 {
		if it := p.UseItems[protocol.UWeapon]; it != nil && it.Dura > 0 {
			it.Dura -= 100
			if it.Dura > it.DuraMax {
				it.Dura = 0
			}
			p.sendDuraChange(server, it)
		}
	}

	log.Logf(log.LevelInfo, "PK", "%s killed %s, PK points: %d (level %d)", p.Name, victim.Name, p.PkPoint, p.PKLevel())
}
