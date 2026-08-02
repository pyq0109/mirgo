package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
)



func (p *PlayObject) IsMarried() bool {
	return p.DearName != ""
}

func (p *PlayObject) Marry(server *netserver.TCPServer, partnerName string) bool {
	if p.IsMarried() {
		p.sysMsg(server, "你已经结婚了")
		return false
	}
	if p.Engine == nil {
		return false
	}
	partner := p.Engine.GetPlayerByName(partnerName)
	if partner == nil || partner == p {
		p.sysMsg(server, partnerName+" 不在线")
		return false
	}
	if partner.IsMarried() {
		p.sysMsg(server, partner.Name+" 已经结婚了")
		return false
	}
	if partner.MapName != p.MapName {
		p.sysMsg(server, "双方必须在同一张地图")
		return false
	}

	p.DearName = partner.Name
	partner.DearName = p.Name
	p.sysMsg(server, "你与 "+partner.Name+" 结为夫妻")
	partner.sysMsg(server, "你与 "+p.Name+" 结为夫妻")
	return true
}

func (p *PlayObject) Divorce(server *netserver.TCPServer) {
	if !p.IsMarried() {
		p.sysMsg(server, "你还没有结婚")
		return
	}
	exName := p.DearName
	p.DearName = ""
	if p.Engine != nil {
		if ex := p.Engine.GetPlayerByName(exName); ex != nil {
			ex.DearName = ""
			ex.sysMsg(server, "你与 "+p.Name+" 离婚了")
		}
	}
	p.sysMsg(server, "你与 "+exName+" 离婚了")
}

func (p *PlayObject) DearRecall(server *netserver.TCPServer) {
	if !p.IsMarried() {
		p.sysMsg(server, "你还没有结婚")
		return
	}
	now := time.Now().UnixMilli()
	if now-p.DearRecallTick < p.Engine.Config.GetSpouseRecallCooldown() {
		p.sysMsg(server, "夫妻传送冷却中")
		return
	}
	if p.Engine == nil {
		return
	}
	spouse := p.Engine.GetPlayerByName(p.DearName)
	if spouse == nil {
		p.sysMsg(server, p.DearName+" 不在线")
		return
	}
	tx, ty, ok := p.adjacentTile()
	if !ok {
		p.sysMsg(server, "身边没有可用的位置")
		return
	}

	p.DearRecallTick = now
	if spouse.MapName == p.MapName && spouse.envir != nil {
		spouse.envir.RemoveObject(spouse.CurrX, spouse.CurrY, OS_MOVINGOBJECT, spouse)
		spouse.envir.broadcastRefMsg(spouse.BaseObject, RM_DISAPPEAR, spouse.ID, spouse.CurrX, spouse.CurrY, spouse.Dir)
		spouse.CurrX, spouse.CurrY = tx, ty
		spouse.envir.AddObject(tx, ty, OS_MOVINGOBJECT, spouse)
		spouse.envir.broadcastRefMsg(spouse.BaseObject, RM_LOGON, spouse.ID, tx, ty, spouse.Dir)
	} else {
		spouse.EnterAnotherMap(server, p.envir, tx, ty)
	}
	p.sysMsg(server, spouse.Name+" 已传送到你身边")
	spouse.sysMsg(server, p.Name+" 将你传送到了身边")
}

func (p *PlayObject) adjacentTile() (int, int, bool) {
	if p.envir == nil {
		return 0, 0, false
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := p.CurrX+dx, p.CurrY+dy
			if p.envir.CanWalk(x, y) {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}
