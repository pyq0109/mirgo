package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func (p *PlayObject) HandleGMCommand(cmd string, server *netserver.TCPServer) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}

	if p.Permission < 10 {
		return false
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "make":
		if len(parts) < 2 {
			return true
		}
		if p.ItemDB != nil {
			def := p.ItemDB.GetByName(parts[1])
			if def != nil {
				p.GiveItem(def.Idx)
				p.SendBagItemsFull(server)
				p.sysMsg(server, "获得: "+def.Name)
			} else {
				p.sysMsg(server, "物品不存在: "+parts[1])
			}
		}
		return true
	case "level":
		if len(parts) < 2 {
			return true
		}
		lvl, _ := strconv.Atoi(parts[1])
		if lvl > 0 && lvl <= 500 {
			p.WAbil.Level = uint16(lvl)
			p.RecalcAbilitys()
			p.WAbil.HP = p.WAbil.MaxHP
			p.WAbil.MP = p.WAbil.MaxMP
			p.sendHealthSpell(server)
			p.sysMsg(server, "等级设置为: "+parts[1])
		}
		return true
	case "move":
		if len(parts) < 2 {
			return true
		}
		mapName := parts[1]
		newEnvir := p.MapMgr.FindMap(mapName)
		if newEnvir != nil {
			p.EnterAnotherMap(server, newEnvir, newEnvir.Width/2, newEnvir.Height/2)
			p.sysMsg(server, "传送到: "+mapName)
		} else {
			p.sysMsg(server, "地图不存在: "+mapName)
		}
		return true
	case "mob":
		if len(parts) < 2 {
			return true
		}
		dx, dy := dirToOffset(p.Dir)
		mx, my := p.CurrX+dx*2, p.CurrY+dy*2
		if mon := p.Engine.SpawnMonsterByName(p.MapName, mx, my, parts[1], time.Now().UnixMilli()); mon != nil {
			p.sysMsg(server, "召唤: "+parts[1])
		} else {
			p.sysMsg(server, "召唤失败: "+parts[1])
		}
		return true
	case "gold":
		if len(parts) < 2 {
			return true
		}
		gold, _ := strconv.Atoi(parts[1])
		p.Gold += gold
		p.sysMsg(server, "金币: "+strconv.Itoa(p.Gold))
		return true
	case "heal":
		p.WAbil.HP = p.WAbil.MaxHP
		p.WAbil.MP = p.WAbil.MaxMP
		p.sendHealthSpell(server)
		p.sysMsg(server, "已回复")
		return true
	case "takeonhorse":
		if !p.OnHorse {
			p.OnHorse = true
			p.broadcastFeatureChanged(server)
		}
		p.sysMsg(server, "已上马")
		return true
	case "takeoffhorse":
		if p.OnHorse {
			p.OnHorse = false
			p.broadcastFeatureChanged(server)
		}
		p.sysMsg(server, "已下马")
		return true
	case "pkpoint":
		if len(parts) < 2 {
			return true
		}
		pts, _ := strconv.Atoi(parts[1])
		p.PkPoint += pts
		if p.PkPoint < 0 {
			p.PkPoint = 0
		}
		p.sysMsg(server, "PK点数: "+strconv.Itoa(p.PkPoint))
		return true
	case "reloadnpc":
		if p.Engine != nil {
			p.Engine.InvalidateAllNpcScripts()
			p.sysMsg(server, "NPC脚本已重载")
		}
		return true
	default:
		return false
	}
}

func (p *PlayObject) sysMsg(server *netserver.TCPServer, text string) {
	msg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
	server.Send(p.Session.ID, msg, protocol.EncodeString(text))
}
