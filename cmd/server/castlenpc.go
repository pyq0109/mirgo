package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// 城堡 NPC 权限检查：仅城主行会掌门人或 GM（权限≥3）可访问
func (p *PlayObject) canAccessCastleNpc(npc *NpcObject) bool {
	if p.Permission >= 3 {
		return true
	}
	if p.Engine == nil || p.Engine.Castle == nil {
		return false
	}
	owner := p.Engine.Castle.GetOwnerGuild()
	if owner == "" || p.GuildName != owner {
		return false
	}
	guild := p.Engine.FindGuild(owner)
	if guild != nil && guild.Leader == p.Name {
		return true
	}
	return p.GuildRank == "master" || p.GuildRank == "掌门人"
}

func (p *PlayObject) HandleCastleNpcSelect(tag string, npc *NpcObject, server *netserver.TCPServer) {
	if !p.canAccessCastleNpc(npc) {
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}

	castle := p.Engine.Castle
	if castle == nil {
		p.sysMsg(server, "城堡系统未启用")
		return
	}

	switch {
	case tag == "@castlegold":
		gold := castle.GetGold()
		doorHP, doorMax := castle.GetDoorHP()
		wallHP, wallMax := castle.GetWallHP()
		taxRate := castle.GetTaxRate()
		owner := castle.GetOwnerGuild()
		text := fmt.Sprintf("城主: %s\n金库: %d 金币\n税率: %d%%\n城门: %d/%d\n城墙: %d/%d",
			owner, gold, taxRate, doorHP, doorMax, wallHP, wallMax)
		p.sysMsg(server, text)

	case tag == "@opendoor":
		castle.ToggleDoor(true)
		castle.broadcastSysMsg(server, p.Engine, fmt.Sprintf("%s 城门已开启", castle.Config.Name))
		p.sysMsg(server, "城门已开启")
		log.Logf(log.LevelInfo, "Castle", "%s opened castle door", p.Name)

	case tag == "@closedoor":
		castle.ToggleDoor(false)
		castle.broadcastSysMsg(server, p.Engine, fmt.Sprintf("%s 城门已关闭", castle.Config.Name))
		p.sysMsg(server, "城门已关闭")
		log.Logf(log.LevelInfo, "Castle", "%s closed castle door", p.Name)

	case tag == "@repairdoor":
		if castle.RepairDoor() {
			p.sysMsg(server, "城门已修复")
			castle.broadcastSysMsg(server, p.Engine, fmt.Sprintf("%s 城门已修复", castle.Config.Name))
		} else {
			doorHP, doorMax := castle.GetDoorHP()
			if doorHP >= doorMax {
				p.sysMsg(server, "城门无需修理")
			} else {
				p.sysMsg(server, fmt.Sprintf("金库不足，修理需要 %d 金币", castle.Config.DoorRepairCost))
			}
		}

	case tag == "@repairwall":
		if castle.RepairWall() {
			p.sysMsg(server, "城墙已修复")
			castle.broadcastSysMsg(server, p.Engine, fmt.Sprintf("%s 城墙已修复", castle.Config.Name))
		} else {
			wallHP, wallMax := castle.GetWallHP()
			if wallHP >= wallMax {
				p.sysMsg(server, "城墙无需修理")
			} else {
				p.sysMsg(server, fmt.Sprintf("金库不足，修理需要 %d 金币", castle.Config.WallRepairCost))
			}
		}

	case tag == "@hireguard":
		p.hireCastleGuard(castle, server, 112) // Race 112 = 弓箭守卫

	case tag == "@hirearcher":
		p.hireCastleGuard(castle, server, 112)

	case strings.HasPrefix(tag, "@withdraw"):
		p.handleCastleWithdraw(castle, tag, server)

	case tag == "@declarewar":
		p.handleCastleDeclareWar(castle, server)

	case strings.HasPrefix(tag, "@castletax"):
		p.handleCastleSetTax(castle, tag, server)

	default:
		p.sysMsg(server, "未知命令: "+tag)
	}
}

func (p *PlayObject) hireCastleGuard(castle *CastleObject, server *netserver.TCPServer, race byte) {
	if castle.GuardCount() >= castle.Config.MaxGuards {
		p.sysMsg(server, fmt.Sprintf("守卫数量已达上限 (%d)", castle.Config.MaxGuards))
		return
	}

	cost := int64(castle.Config.ArcherCost)
	if castle.GetGold() < cost {
		p.sysMsg(server, fmt.Sprintf("金库不足，雇佣需要 %d 金币", cost))
		return
	}

	if !castle.WithdrawGold(cost) {
		p.sysMsg(server, "金库不足")
		return
	}

	now := time.Now().UnixMilli()
	mon := p.Engine.SpawnMonsterByName(castle.Config.MapName, p.CurrX+1, p.CurrY, "弓箭守卫", now)
	if mon == nil {
		castle.CollectTax(cost) // 退还金币
		p.sysMsg(server, "无法在此位置召唤守卫")
		return
	}

	castle.AddGuard(mon.ID)
	p.sysMsg(server, "已雇佣弓箭守卫")
	log.Logf(log.LevelInfo, "Castle", "%s hired guard (id=%d) at %s(%d,%d)", p.Name, mon.ID, mon.MapName, mon.CurrX, mon.CurrY)
}

func (p *PlayObject) handleCastleWithdraw(castle *CastleObject, tag string, server *netserver.TCPServer) {
	// @withdraw 或 @withdraw <amount>
	amountStr := strings.TrimSpace(strings.TrimPrefix(tag, "@withdraw"))
	var amount int64
	if amountStr == "" {
		amount = castle.GetGold()
	} else {
		var err error
		amount, err = strconv.ParseInt(amountStr, 10, 64)
		if err != nil || amount <= 0 {
			p.sysMsg(server, "无效金额")
			return
		}
	}

	if !castle.WithdrawGold(amount) {
		p.sysMsg(server, "金库余额不足")
		return
	}

	p.Gold += int(amount)
	resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	p.sysMsg(server, fmt.Sprintf("提取了 %d 金币", amount))
	log.Logf(log.LevelInfo, "Castle", "%s withdrew %d gold from castle treasury", p.Name, amount)
}

func (p *PlayObject) handleCastleDeclareWar(castle *CastleObject, server *netserver.TCPServer) {
	if p.GuildName == "" {
		p.sysMsg(server, "你需要加入行会才能宣战")
		return
	}
	if p.GuildRank != "master" && p.GuildRank != "掌门人" {
		p.sysMsg(server, "只有行会掌门人才能宣战")
		return
	}

	if castle.DeclareWar(p.GuildName) {
		castle.broadcastSysMsg(server, p.Engine,
			fmt.Sprintf("[攻城战] %s 向 %s 宣战！", p.GuildName, castle.Config.Name))
		p.sysMsg(server, "宣战成功")
	} else {
		state := castle.GetWarState()
		if state == CastleWarActive {
			p.sysMsg(server, "攻城战正在进行中")
		} else if castle.IsAttackingGuild(p.GuildName) {
			p.sysMsg(server, "你的行会已经宣战过了")
		} else {
			p.sysMsg(server, "无法宣战")
		}
	}
}

func (p *PlayObject) handleCastleSetTax(castle *CastleObject, tag string, server *netserver.TCPServer) {
	rateStr := strings.TrimSpace(strings.TrimPrefix(tag, "@castletax"))
	if rateStr == "" {
		p.sysMsg(server, fmt.Sprintf("当前税率: %d%% (上限: %d%%)", castle.GetTaxRate(), castle.Config.MaxTaxRate))
		return
	}

	rate, err := strconv.Atoi(rateStr)
	if err != nil {
		p.sysMsg(server, "无效税率")
		return
	}

	if castle.SetTaxRate(rate) {
		p.sysMsg(server, fmt.Sprintf("税率已设为 %d%%", rate))
		log.Logf(log.LevelInfo, "Castle", "%s set castle tax rate to %d%%", p.Name, rate)
	} else {
		p.sysMsg(server, fmt.Sprintf("税率必须在 0-%d 之间", castle.Config.MaxTaxRate))
	}
}
