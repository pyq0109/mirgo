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
	if given > 0 {
		p.SendBagItemsFull(server)
	}

	butchMsg := protocol.MakeDefaultMsg(protocol.SMButch, target.ID, 0, 0, 0)
	server.Send(p.Session.ID, butchMsg, "")
	log.Logf(log.LevelInfo, "Butch", "%s butchered %s, got %d %s", p.Name, target.Name, given, info.ItemName)
}
