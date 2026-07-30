package main

import (
	"fmt"
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

	command := strings.ToLower(parts[0])

	// 戒指/套装命令（不需要GM权限）
	switch command {
	case "move":
		if p.HasTeleport && len(parts) >= 3 {
			return p.ringTeleport(server, parts[1], parts[2])
		}
	case "search":
		if p.HasProbe && len(parts) >= 2 {
			return p.ringSearch(server, parts[1])
		}
	case "recall":
		if p.HasRecallSuite {
			return p.ringRecall(server)
		}
	case "slave":
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "relax":
				p.toggleSlaveRelax(server)
			case "recall":
				p.recallSlaves(server)
			default:
				p.sysMsg(server, "用法: @slave relax|recall")
			}
		} else {
			p.cleanSlaveList()
			p.sysMsg(server, "宠物: "+strconv.Itoa(len(p.SlaveIDs))+"/"+strconv.Itoa(MaxSlaveCount)+" 等级: "+strconv.Itoa(p.SlaveLevel))
		}
		return true
	case "dearrecall":
		p.DearRecall(server)
		return true
	case "masterrecall":
		p.MasterRecall(server)
		return true
	case "nomob":
		now := time.Now().UnixMilli()
		p.Engine.NoMonGen = !p.Engine.NoMonGen
		if p.Engine.NoMonGen {
			killed := 0
			for _, m := range p.Engine.Monsters {
				if !m.Death && !m.Ghost {
					m.Death = true
					m.DeathTick = now
					if m.envir != nil {
						m.envir.broadcastDeathMsg(m.BaseObject, m.ID, m.CurrX, m.CurrY, m.Dir, true)
					}
					killed++
				}
			}
			for i := range p.Engine.MonGenList {
				p.Engine.MonGenList[i].LiveList = nil
			}
			p.sysMsg(server, fmt.Sprintf("怪物生成已停止，已清除 %d 只", killed))
		} else {
			p.sysMsg(server, "怪物生成已恢复")
		}
		return true
	}

	if p.Permission < 10 {
		return false
	}

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
	case "kick":
		if len(parts) < 2 {
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil && target.Session != nil {
			server.CloseSession(target.Session.ID)
			p.sysMsg(server, "已踢出: "+parts[1])
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "info":
		if len(parts) < 2 {
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil {
			p.sysMsg(server, fmt.Sprintf("[%s] Lv%d %s HP:%d/%d MP:%d/%d Gold:%d Map:%s(%d,%d) PK:%d",
				target.Name, target.WAbil.Level, jobName(target.Job),
				target.WAbil.HP, target.WAbil.MaxHP, target.WAbil.MP, target.WAbil.MaxMP,
				target.Gold, target.MapName, target.CurrX, target.CurrY, target.PkPoint))
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "mapinfo":
		if p.envir != nil {
			monCount := 0
			playerCount := 0
			for i := range p.envir.Cells {
				for _, o := range p.envir.Cells[i].ObjList {
					if o.Type == OS_MOVINGOBJECT {
						switch o.Obj.(type) {
						case *MonsterObject:
							monCount++
						case *PlayObject:
							playerCount++
						}
					}
				}
			}
			p.sysMsg(server, fmt.Sprintf("地图:%s 大小:%dx%d 玩家:%d 怪物:%d 事件:%d",
				p.envir.Name, p.envir.Width, p.envir.Height, playerCount, monCount, len(p.envir.Events)))
		}
		return true
	case "monclear":
		if p.envir != nil {
			count := 0
			for i := range p.envir.Cells {
				for _, o := range p.envir.Cells[i].ObjList {
					if o.Type == OS_MOVINGOBJECT {
						if mon, ok := o.Obj.(*MonsterObject); ok && !mon.Death {
							mon.Death = true
							mon.WAbil.HP = 0
							count++
						}
					}
				}
			}
			p.sysMsg(server, fmt.Sprintf("已清除 %d 个怪物", count))
		}
		return true
	case "revive":
		if len(parts) < 2 {
			// 复活自己
			p.WAbil.HP = p.WAbil.MaxHP
			p.WAbil.MP = p.WAbil.MaxMP
			p.Death = false
			p.sendHealthSpell(server)
			p.sysMsg(server, "已复活")
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil {
			target.WAbil.HP = target.WAbil.MaxHP
			target.WAbil.MP = target.WAbil.MaxMP
			target.Death = false
			target.sendHealthSpell(server)
			p.sysMsg(server, "已复活: "+parts[1])
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "superman":
		if p.WAbil.MaxHP == 65535 {
			p.RecalcAbilitys()
			p.sysMsg(server, "无敌模式关闭")
		} else {
			p.WAbil.MaxHP = 65535
			p.WAbil.HP = 65535
			p.sysMsg(server, "无敌模式开启")
		}
		p.sendHealthSpell(server)
		return true
	case "observe":
		if len(parts) < 2 {
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil && target.envir != nil {
			p.EnterAnotherMap(server, target.envir, target.CurrX, target.CurrY)
			p.sysMsg(server, "已传送到 "+parts[1]+" 身边")
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "shutup":
		if len(parts) < 2 {
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil {
			minutes := 10
			if len(parts) >= 3 {
				minutes, _ = strconv.Atoi(parts[2])
			}
			target.ShutupTick = time.Now().UnixMilli() + int64(minutes)*60000
			p.sysMsg(server, fmt.Sprintf("已禁言 %s %d分钟", parts[1], minutes))
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "recallmob":
		if len(parts) < 2 {
			return true
		}
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		if count > 10 {
			count = 10
		}
		dx, dy := dirToOffset(p.Dir)
		for i := 0; i < count; i++ {
			mx := p.CurrX + dx*(2+i%3) + (i/3)*1
			my := p.CurrY + dy*(2+i%3) + (i/3)*1
			if mon := p.Engine.SpawnMonsterByName(p.MapName, mx, my, parts[1], time.Now().UnixMilli()); mon != nil {
				mon.PlayerMasterID = p.ID
				p.SlaveIDs = append(p.SlaveIDs, mon.ID)
			}
		}
		p.sysMsg(server, fmt.Sprintf("召唤 %d 个 %s", count, parts[1]))
		return true
	case "moveuser":
		if len(parts) < 3 {
			return true
		}
		target := p.Engine.GetPlayerByName(parts[1])
		if target != nil {
			newEnvir := p.MapMgr.FindMap(parts[2])
			if newEnvir != nil {
				tx, ty := newEnvir.Width/2, newEnvir.Height/2
				if len(parts) >= 5 {
					tx, _ = strconv.Atoi(parts[3])
					ty, _ = strconv.Atoi(parts[4])
				}
				target.EnterAnotherMap(server, newEnvir, tx, ty)
				p.sysMsg(server, "已传送 "+parts[1]+" 到 "+parts[2])
			} else {
				p.sysMsg(server, "地图不存在: "+parts[2])
			}
		} else {
			p.sysMsg(server, "玩家不在线: "+parts[1])
		}
		return true
	case "changemode":
		if len(parts) < 2 {
			return true
		}
		mode, _ := strconv.Atoi(parts[1])
		if mode >= 0 && mode <= 5 {
			p.AttackMode = byte(mode)
			p.sysMsg(server, fmt.Sprintf("攻击模式: %d", mode))
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

var lastTeleportTick int64

func (p *PlayObject) ringTeleport(server *netserver.TCPServer, sx, sy string) bool {
	now := time.Now().UnixMilli()
	if now-lastTeleportTick < 10000 {
		p.sysMsg(server, "传送戒指冷却中")
		return true
	}
	x, err1 := strconv.Atoi(sx)
	y, err2 := strconv.Atoi(sy)
	if err1 != nil || err2 != nil || x < 0 || y < 0 {
		p.sysMsg(server, "格式: @move X Y")
		return true
	}
	if p.envir != nil && p.envir.CanWalk(x, y) {
		lastTeleportTick = now
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
		p.envir.broadcastRefMsg(p.BaseObject, RM_DISAPPEAR, p.ID, p.CurrX, p.CurrY, p.Dir)
		p.CurrX, p.CurrY = x, y
		p.envir.AddObject(x, y, OS_MOVINGOBJECT, p)
		p.envir.broadcastRefMsg(p.BaseObject, RM_LOGON, p.ID, x, y, p.Dir)
		p.sysMsg(server, "已传送")
	} else {
		p.sysMsg(server, "目标位置不可达")
	}
	return true
}

func (p *PlayObject) ringSearch(server *netserver.TCPServer, name string) bool {
	if p.Engine == nil {
		return true
	}
	p.Engine.mu.Lock()
	var found *PlayObject
	for _, pl := range p.Engine.PlayObjectList {
		if pl.Name == name {
			found = pl
			break
		}
	}
	p.Engine.mu.Unlock()
	if found != nil {
		p.sysMsg(server, found.Name+" 位于 "+found.MapName+" "+strconv.Itoa(found.CurrX)+":"+strconv.Itoa(found.CurrY))
	} else {
		p.sysMsg(server, name+" 不在线")
	}
	return true
}

func (p *PlayObject) ringRecall(server *netserver.TCPServer) bool {
	if p.GuildName == "" || p.Engine == nil {
		p.sysMsg(server, "需要行会")
		return true
	}
	p.Engine.mu.Lock()
	count := 0
	for _, pl := range p.Engine.PlayObjectList {
		if pl.GuildName == p.GuildName && pl.Name != p.Name && pl.envir != nil {
			if p.envir != nil && p.envir.CanWalk(p.CurrX+count%3-1, p.CurrY+count/3-1) {
				pl.envir.RemoveObject(pl.CurrX, pl.CurrY, OS_MOVINGOBJECT, pl)
				pl.envir.broadcastRefMsg(pl.BaseObject, RM_DISAPPEAR, pl.ID, pl.CurrX, pl.CurrY, pl.Dir)
				pl.CurrX = p.CurrX + count%3 - 1
				pl.CurrY = p.CurrY + count/3 - 1
				pl.MapName = p.MapName
				pl.envir = p.envir
				pl.envir.AddObject(pl.CurrX, pl.CurrY, OS_MOVINGOBJECT, pl)
				pl.envir.broadcastRefMsg(pl.BaseObject, RM_LOGON, pl.ID, pl.CurrX, pl.CurrY, pl.Dir)
				count++
			}
		}
	}
	p.Engine.mu.Unlock()
	p.sysMsg(server, "召集了 "+strconv.Itoa(count)+" 名行会成员")
	return true
}
