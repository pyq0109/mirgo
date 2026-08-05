package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// 可屠宰动物种族及对应产出
var butchTable = map[byte]struct {
	ItemName string
	MinCount int
	MaxCount int
}{
	51: {"鸡肉", 1, 2},
	52: {"鹿肉", 2, 4},
}

// initMeatQuality 生成时初始化肉质（Delphi UsrEngn.pas:1839-1862）：
// 鸡 rand(3500)+3000；鹿 1/30 概率高品质 rand(20000)+10000，
// 否则 rand(8000)+8000；其余可屠宰动物按鹿的普通档。
func initMeatQuality(mon *MonsterObject) {
	if _, ok := butchTable[mon.Race]; !ok {
		return
	}
	switch mon.Race {
	case 51:
		mon.MeatQuality = rand.Intn(3500) + 3000
	case 52:
		if rand.Intn(30) == 0 {
			mon.MeatQuality = rand.Intn(20000) + 10000
		} else {
			mon.MeatQuality = rand.Intn(8000) + 8000
		}
	default:
		mon.MeatQuality = rand.Intn(8000) + 8000
	}
}

// applyMeatQuality 把玩家背包内所有肉（StdMode 40）的耐久设为给定品质
//（Delphi ApplyMeatQuality，ObjBase.pas:20517-20535）。
func (p *PlayObject) applyMeatQuality(quality int) {
	if p.ItemDB == nil {
		return
	}
	for _, item := range p.ItemList {
		if item == nil {
			continue
		}
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil && def.StdMode == 40 {
			item.Dura = uint16(quality)
		}
	}
}

// HandleButch 处理屠宰动物尸体（CMButch=1007, Recog=目标怪物ID）。
func (p *PlayObject) HandleButch(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil || p.Death {
		return
	}
	targetID := msg.SourceID
	if targetID == 0 {
		targetID = int32(msg.Param1)
	}

	var target *MonsterObject
	for _, mon := range p.Engine.Monsters {
		if mon.ID == targetID {
			target = mon
			break
		}
	}
	if target == nil || !target.Death {
		return
	}

	info, ok := butchTable[target.Race]
	if !ok {
		p.sysMsg(server, "这个生物无法屠宰")
		return
	}
	if target.LootDropped {
		p.sysMsg(server, "已经被屠宰过了")
		return
	}

	// 距离检查
	dx := p.CurrX - target.CurrX
	dy := p.CurrY - target.CurrY
	if dx < -2 || dx > 2 || dy < -2 || dy > 2 {
		return
	}

	target.LootDropped = true

	count := info.MinCount + rand.Intn(info.MaxCount-info.MinCount+1)
	given := 0
	if p.ItemDB != nil {
		def := p.ItemDB.GetByName(info.ItemName)
		if def != nil {
			for i := 0; i < count; i++ {
				if p.GiveItem(def.Idx) {
					given++
				}
			}
		}
	}
	// 肉质联动（Delphi ApplyMeatQuality，ObjBase.pas:20517-20535）：
	// 背包内所有肉（含新割的）耐久设为该动物的 MeatQuality。
	if target.MeatQuality > 0 {
		p.applyMeatQuality(target.MeatQuality)
	}
	if given > 0 {
		p.SendBagItemsFull(server)
	}

	// Delphi RM_BUTCH 广播：Recog=屠宰者、Param/Tag/Series=x/y/dir，
	// 附近玩家客户端对屠宰者播放坐下动画（ClMain.pas:4059-4071）。
	butchMsg := protocol.MakeDefaultMsg(protocol.SMButch, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	if p.envir != nil {
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost {
				server.Send(other.Session.ID, butchMsg, "")
			}
		}
	}
	log.Logf(log.LevelInfo, "Butch", "%s butchered %s, got %d %s", p.Name, target.Name, given, info.ItemName)
}
