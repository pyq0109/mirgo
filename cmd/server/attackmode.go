package main

import (
	"time"

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

// IsProtectTarget 玩家保护判定（Delphi ObjBase.pas:21258-21330）。
func (p *PlayObject) IsProtectTarget(target *PlayObject) bool {
	now := time.Now().UnixMilli()
	cfg := p.Engine.Config

	// 切图保护
	if target.EnterMapTick > 0 && now-target.EnterMapTick < cfg.GetMapEnterProtect() {
		return true
	}

	targetLevel := int(target.WAbil.Level)
	attackerLevel := int(p.WAbil.Level)

	// 红名保护：红名玩家不可攻击低等级玩家
	if p.PKLevel() >= 2 && targetLevel <= cfg.GetPKRedProtectLevel() {
		return true
	}

	// 等级保护：低等级玩家受保护
	if targetLevel <= cfg.GetPKProtectLevel() && attackerLevel-targetLevel > cfg.GetPKProtectDiff() {
		return true
	}

	return false
}

func (p *PlayObject) CanAttackTarget(target *BaseObject) bool {
	// 攻城战期间：攻守双方（含各自联盟行会，Castle.pas:768-783/896-902）可自由攻击
	if p.Engine != nil && p.Engine.Castle != nil && p.Engine.Castle.IsAtWar() {
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			castle := p.Engine.Castle
			pAttack := castle.IsAttackingGuild(p.GuildName) || castle.IsAttackAllyGuild(p.Engine, p.GuildName)
			pDefend := castle.IsDefendingGuild(p.GuildName) || castle.IsDefenseAllyGuild(p.Engine, p.GuildName)
			tAttack := castle.IsAttackingGuild(tp.GuildName) || castle.IsAttackAllyGuild(p.Engine, tp.GuildName)
			tDefend := castle.IsDefendingGuild(tp.GuildName) || castle.IsDefenseAllyGuild(p.Engine, tp.GuildName)
			if pAttack && tDefend {
				return true
			}
			if pDefend && tAttack {
				return true
			}
			if pAttack && tAttack && p.GuildName != tp.GuildName {
				return true
			}
		}
	}

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
	// Delphi ClientDropItem（ObjBase.pas:16213-16277）。
	dropFail := func() {
		resp := protocol.MakeDefaultMsg(protocol.SMDropItemFail, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		// 客户端手持丢弃时物品已离手，失败补发全量背包恢复显示。
		p.SendBagItemsFull(server)
	}
	// Param1 = MakeIndex（实例 ID；客户端布局由客户端维护）。
	bagIdx := p.findBagItem(int32(msg.Param1))
	if bagIdx < 0 {
		dropFail()
		return
	}
	// 安全区/NOTHROWITEM 地图禁丢。
	if p.envir != nil && (p.envir.Flag.NoThrowItem || IsSafeZone(p.envir, p.CurrX, p.CurrY)) {
		p.sysMsg(server, "这里不能丢弃物品")
		dropFail()
		return
	}
	// 3000ms 节流（与交易共用 m_DealLastTick，ObjBase.pas:16244）。
	now := time.Now().UnixMilli()
	if now-p.lastDealTick < 3000 {
		dropFail()
		return
	}
	item := p.ItemList[bagIdx]

	name := "Item"
	looks := 0
	price := uint32(0)
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
			looks = int(def.Looks)
			price = def.Price
		}
	}
	p.lastDealTick = now
	p.ItemList = append(p.ItemList[:bagIdx], p.ItemList[bagIdx+1:]...)

	// 廉价物品管控（Delphi boControlDropItem，ObjBase.pas:16256-16262）：
	// Price<500 直接删除不落地。
	if p.Engine.Config.GetControlDropItem() && price < 500 {
		resp := protocol.MakeDefaultMsg(protocol.SMDropItemSuccess, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, protocol.EncodeString(name))
		p.RecalcAbilitys()
		p.SendBagItemsFull(server)
		p.sendWeightChanged(server)
		return
	}

	p.Engine.mu.Lock()
	id := p.Engine.nextItemID
	p.Engine.nextItemID++
	p.Engine.mu.Unlock()

	gi := &GroundItem{
		ID:       id,
		Name:     name,
		Looks:    looks,
		X:        p.CurrX,
		Y:        p.CurrY + 1,
		DropTick: now,
		UserItem: item,
	}
	if p.envir != nil {
		if p.envir.AddGroundItem(gi) == nil {
			// 格子已满无法落地：物品放回背包（Delphi 会直接丢失，
			// Go 闭环选择对玩家更安全的归还）。
			p.ItemList = append(p.ItemList, item)
			resp := protocol.MakeDefaultMsg(protocol.SMDropItemFail, 0, 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			p.SendBagItemsFull(server)
			return
		}
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
	// Delphi（ObjBase.pas:16207）：禁止把金币全部丢光。
	if amount >= p.Gold {
		p.sysMsg(server, "不能丢弃全部金币")
		return
	}
	// Delphi ClientDropGold（ObjBase.pas:16187-16212）：安全区禁丢。
	if p.envir != nil && IsSafeZone(p.envir, p.CurrX, p.CurrY) {
		p.sysMsg(server, "这里不能丢弃金币")
		return
	}
	// 廉价金币管控：<1000 禁丢（boControlDropItem）。
	if p.Engine.Config.GetControlDropItem() && amount < 1000 {
		return
	}
	// 3000ms 节流（与丢弃物品共用）。
	now := time.Now().UnixMilli()
	if now-p.lastDealTick < 3000 {
		return
	}
	p.lastDealTick = now

	p.Engine.mu.Lock()
	id := p.Engine.nextItemID
	p.Engine.nextItemID++
	p.Engine.mu.Unlock()

	// 金币散开范围 3（Delphi DropGoldDown，ObjBase.pas:2316）。
	x, y := p.CurrX, p.CurrY+1
	if p.envir != nil {
		x, y = getDropPosition(p.envir, p.CurrX, p.CurrY, 3)
	}
	gi := &GroundItem{
		ID:       id,
		Name:     "金币",
		X:        x,
		Y:        y,
		Gold:     amount,
		DropTick: now,
	}
	if p.envir != nil {
		placed := p.envir.AddGroundItem(gi)
		if placed == nil {
			return // 落地失败（格满）：金币不扣除
		}
		if placed != gi {
			gi = placed // 与已有金堆合并：广播合并后的堆
		}
	}
	p.Gold -= amount

	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	if p.envir != nil {
		showResp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), uint16(gi.Looks))
		objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
		for _, obj := range objs {
			if other, ok := obj.(*PlayObject); ok && !other.Ghost {
				server.Send(other.Session.ID, showResp, protocol.EncodeString("金币"))
			}
		}
	}
}
