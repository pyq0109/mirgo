package main

// NPC 脚本扩展命令注册表（路线图 6.2 表驱动）。
//
// 存量命令保留在 npcscript.go 的两个 switch 中；新增命令一律在此以
// registerCondition/registerAction 登记（init 注册），未知命令由
// evalOneCondition/execOneAction 的 default 分支打告警日志，跑图实测
// 收集真实脚本缺口后增量补齐。
//
// Delphi 参照：ObjNpc.pas 命令分派大 case（nSC_/nSS_ 常量见 M2Share.pas）。

import (
	"strconv"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type scriptCondFunc func(parts []string, p *PlayObject) bool
type scriptActionFunc func(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool

var scriptCondRegistry = map[string]scriptCondFunc{}
var scriptActionRegistry = map[string]scriptActionFunc{}

func registerCondition(name string, fn scriptCondFunc) {
	scriptCondRegistry[strings.ToUpper(name)] = fn
}

func registerAction(name string, fn scriptActionFunc) {
	scriptActionRegistry[strings.ToUpper(name)] = fn
}

// safeBroadcastFeatureChanged 外观变化广播（单测可能无 server/session）。
func safeBroadcastFeatureChanged(p *PlayObject, server *netserver.TCPServer) {
	if server == nil || p.Session == nil {
		return
	}
	p.broadcastFeatureChanged(server)
}

func init() {
	registerCondition("CHECKITEMTYPE", condCheckItemType)
	registerCondition("CHECKHORSE", condCheckHorse)
	registerCondition("CHECKCASTLEWAR", condCheckCastleWar)
	registerCondition("CHECKMONAREA", condCheckMonArea)

	registerAction("HAIRSTYLE", actHairStyle)
	registerAction("HAIRCOLOR", actHairColor)
	registerAction("HORSECALL", actHorseCall)
	registerAction("KILLHORSE", actKillHorse)
	registerAction("INCFAME", actIncFame)
	registerAction("DECFAME", actDecFame)
	registerAction("MAKEHEALZONE", actMakeHealZone)
	registerAction("MAKEDAMAGEZONE", actMakeDamageZone)
}

// condCheckItemType — Delphi ConditionOfCheckItemType（ObjNpc.pas:5142-5162）：
// CHECKITEMTYPE <装备位0-12> <StdMode>，该位置装备的类型相符为真。
func condCheckItemType(parts []string, p *PlayObject) bool {
	if len(parts) < 3 {
		return false
	}
	where, err1 := strconv.Atoi(parts[1])
	stdMode, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || where < 0 || where > 12 {
		return false
	}
	item := p.UseItems[where]
	if item == nil || item.WIndex == 0 {
		return false
	}
	if p.ItemDB == nil {
		return false
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	return def != nil && int(def.StdMode) == stdMode
}

// condCheckHorse 骑乘状态检查（Go 闭环扩展，配合 GM takeonhorse/坐骑系统）。
func condCheckHorse(parts []string, p *PlayObject) bool {
	return p.OnHorse
}

// condCheckCastleWar 攻城战进行中检查（配合城堡脚本）。
func condCheckCastleWar(parts []string, p *PlayObject) bool {
	return p.Engine != nil && p.Engine.Castle != nil && p.Engine.Castle.IsAtWar()
}

// condCheckMonArea — CHECKMONAREA <怪物名> [数量]：玩家周围 viewRange 格内
// 指定名称的存活怪物数量达标（默认 1）。
func condCheckMonArea(parts []string, p *PlayObject) bool {
	if len(parts) < 2 || p.Engine == nil {
		return false
	}
	name := parts[1]
	count := 1
	if len(parts) >= 3 {
		if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
			count = n
		}
	}
	n := 0
	for _, mon := range p.Engine.Monsters {
		if mon.Ghost || mon.Death || mon.Name != name {
			continue
		}
		if mon.MapName != p.MapName {
			continue
		}
		if abs(mon.CurrX-p.CurrX) <= viewRange && abs(mon.CurrY-p.CurrY) <= viewRange {
			n++
			if n >= count {
				return true
			}
		}
	}
	return false
}

// actHairStyle — Delphi ActionOfChangeHairStyle（ObjNpc.pas:3010-3024）：
// HAIRSTYLE <n> 设置发型并广播外观变化。
func actHairStyle(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if len(parts) < 2 {
		return true
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return true
	}
	p.Hair = byte(n)
	safeBroadcastFeatureChanged(p, server)
	return true
}

// actHairColor — Delphi nSC_HAIRCOLOR 为空实现（ObjNpc.pas:8331）；
// Go 无独立的发色编码位，按发型字节整体设置兜底，避免脚本报错。
func actHairColor(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if len(parts) < 2 {
		return true
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return true
	}
	p.Hair = byte(n)
	safeBroadcastFeatureChanged(p, server)
	return true
}

// actHorseCall — Delphi nSC_HORSECALL 为空实现（ObjNpc.pas:8335）；
// Go 闭环有坐骑系统，实装为立即上马。
func actHorseCall(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if !p.OnHorse {
		p.OnHorse = true
		safeBroadcastFeatureChanged(p, server)
	}
	return true
}

// actKillHorse — Delphi nSC_KILLHORSE 为空实现（ObjNpc.pas:8337）；
// Go 闭环实装为立即下马。
func actKillHorse(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if p.OnHorse {
		p.OnHorse = false
		safeBroadcastFeatureChanged(p, server)
	}
	return true
}

// actIncFame / actDecFame — INCFAME <n> / DECFAME <n>：声望点增减
//（Delphi 声望体系；Go 以 CreditPoint 承载，已持久化）。
func actIncFame(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if len(parts) < 2 {
		return true
	}
	n, _ := strconv.Atoi(parts[1])
	p.CreditPoint += n
	if p.CreditPoint < 0 {
		p.CreditPoint = 0
	}
	return true
}

func actDecFame(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	if len(parts) < 2 {
		return true
	}
	n, _ := strconv.Atoi(parts[1])
	p.CreditPoint -= n
	if p.CreditPoint < 0 {
		p.CreditPoint = 0
	}
	return true
}

// actMakeHealZone — MAKEHEALZONE <每秒治疗量> [持续秒数=30]：
// 以 NPC 为中心 3×3 铺设治疗区事件（Delphi 无现成实现，Go 自定义语义）。
func actMakeHealZone(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	amount, seconds := 0, 30
	if len(parts) >= 2 {
		amount, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		if s, err := strconv.Atoi(parts[2]); err == nil && s > 0 {
			seconds = s
		}
	}
	if amount <= 0 || npc == nil || npc.envir == nil {
		return true
	}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			npc.envir.AddZoneEvent(server, protocol.ETHealZone, npc.CurrX+dx, npc.CurrY+dy, amount, int64(seconds)*1000)
		}
	}
	log.Logf(log.LevelInfo, "NpcScript", "%s created heal zone at %s(%d,%d) for %ds",
		p.Name, npc.MapName, npc.CurrX, npc.CurrY, seconds)
	return true
}

// actMakeDamageZone — MAKEDAMAGEZONE <每秒伤害> [持续秒数=30]：
// 以 NPC 为中心 3×3 铺设伤害区事件（玩家钳制在 1 HP，与火墙一致）。
func actMakeDamageZone(parts []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	amount, seconds := 0, 30
	if len(parts) >= 2 {
		amount, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		if s, err := strconv.Atoi(parts[2]); err == nil && s > 0 {
			seconds = s
		}
	}
	if amount <= 0 || npc == nil || npc.envir == nil {
		return true
	}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			npc.envir.AddZoneEvent(server, protocol.ETDamageZone, npc.CurrX+dx, npc.CurrY+dy, amount, int64(seconds)*1000)
		}
	}
	log.Logf(log.LevelInfo, "NpcScript", "%s created damage zone at %s(%d,%d) for %ds",
		p.Name, npc.MapName, npc.CurrX, npc.CurrY, seconds)
	return true
}
