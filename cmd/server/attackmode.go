package main

import (
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	AttackModePeace  = 0
	AttackModeGroup  = 1
	AttackModeGuild  = 2
	AttackModeAll    = 3
	AttackModePK     = 4
)

func (p *PlayObject) HandleChangeAttackMode(msg SendMessage, server *netserver.TCPServer) {
	mode := msg.Param1
	if mode < AttackModePeace || mode > AttackModePK {
		return
	}
	p.AttackMode = byte(mode)
}

func (p *PlayObject) CanAttackTarget(target *BaseObject) bool {
	switch p.AttackMode {
	case AttackModePeace:
		if mon := p.envir.getMonsterByBase(target); mon != nil {
			return true
		}
		return false
	case AttackModeGroup:
		if mon := p.envir.getMonsterByBase(target); mon != nil {
			return true
		}
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			if p.isGroupMember(tp) {
				return false
			}
		}
		return true
	case AttackModeGuild:
		if mon := p.envir.getMonsterByBase(target); mon != nil {
			return true
		}
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			if p.GuildName != "" && tp.GuildName == p.GuildName {
				return false
			}
		}
		return true
	case AttackModePK:
		if mon := p.envir.getMonsterByBase(target); mon != nil {
			return true
		}
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			return tp.PkPoint >= 200
		}
		return true
	default:
		return true
	}
}

func (p *PlayObject) isGroupMember(other *PlayObject) bool {
	if p.Engine == nil {
		return false
	}
	p.Engine.mu.Lock()
	defer p.Engine.mu.Unlock()
	for _, party := range p.Engine.Parties {
		if party.Leader == p.ID || party.Leader == other.ID {
			for _, m := range party.Members {
				if m == p.ID {
					for _, m2 := range party.Members {
						if m2 == other.ID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (p *PlayObject) HandleDropItem(msg SendMessage, server *netserver.TCPServer) {
	// Param1 = MakeIndex（实例 ID；客户端布局由客户端维护）。
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		resp := protocol.MakeDefaultMsg(protocol.SMDropItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	item := p.ItemList[bagIdx]
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)

	name := "Item"
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
		}
	}

	p.Engine.mu.Lock()
	id := p.Engine.nextItemID
	p.Engine.nextItemID++
	p.Engine.mu.Unlock()

	gi := &GroundItem{
		ID:    id,
		Name:  name,
		Looks: 0,
		X:     p.CurrX,
		Y:     p.CurrY + 1,
	}
	if p.envir != nil {
		p.envir.AddGroundItem(gi)
	}

	resp := protocol.MakeDefaultMsg(protocol.SMDropItemSuccess, gi.ID, uint16(gi.X), uint16(gi.Y), 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(name))
	p.RecalcAbilitys()
	p.SendBagItemsFull(server)
	p.sendWeightChanged(server)

	if p.envir != nil {
		showResp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), uint16(gi.Looks))
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost {
				server.Send(other.Session.ID, showResp, protocol.EncodeString(name))
			}
		}
	}
}

func (p *PlayObject) HandleDropGold(msg SendMessage, server *netserver.TCPServer) {
	amount := msg.Param1
	if amount <= 0 || amount > p.Gold {
		return
	}
	p.Gold -= amount

	p.Engine.mu.Lock()
	id := p.Engine.nextItemID
	p.Engine.nextItemID++
	p.Engine.mu.Unlock()

	gi := &GroundItem{
		ID:   id,
		Name: "金币",
		X:    p.CurrX,
		Y:    p.CurrY + 1,
		Gold: amount,
	}
	if p.envir != nil {
		p.envir.AddGroundItem(gi)
	}

	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	if p.envir != nil {
		showResp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), 0)
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost {
				server.Send(other.Session.ID, showResp, protocol.EncodeString("金币"))
			}
		}
	}
}
