package main

import (
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// MineEvent 表示地图上的一个矿石节点。
type MineEvent struct {
	X, Y      int
	OreCount  int   // 剩余可采次数
	RegenTick int64 // 再生时间
	MaxCount  int   // 最大可采次数
}

// MineOre 表示一种可产出的矿石。
type MineOre struct {
	ItemName string
	Chance   int // 1/N 概率
	MinDura  int
	MaxDura  int
}

// 默认矿石表
var defaultOreTable = []MineOre{
	{ItemName: "黑铁矿", Chance: 3, MinDura: 3000, MaxDura: 15000},
	{ItemName: "金矿", Chance: 8, MinDura: 2000, MaxDura: 10000},
	{ItemName: "银矿", Chance: 10, MinDura: 2000, MaxDura: 8000},
	{ItemName: "铁矿", Chance: 2, MinDura: 1000, MaxDura: 6000},
}

const (
	mineRegenTime = 600000 // 10分钟再生
	mineMaxCount  = 200    // 初始可采次数
)

// InitMineEvents 在采矿地图上初始化矿石节点。
func (e *Environment) InitMineEvents() {
	if e.rawMap == nil {
		return
	}
	// 检查地图是否有采矿标志（通过地图名或配置判断）
	// 简化：所有非安全区地图的随机位置生成矿点
	for y := 0; y < e.Height; y += 10 {
		for x := 0; x < e.Width; x += 10 {
			idx := y*e.Width + x
			if idx < len(e.Cells) && e.Cells[idx].Flag == 0 {
				e.MineEvents = append(e.MineEvents, &MineEvent{
					X:        x + rand.Intn(5),
					Y:        y + rand.Intn(5),
					OreCount: rand.Intn(mineMaxCount) + 50,
					MaxCount: mineMaxCount,
				})
			}
		}
	}
}

// GetMineEventAt 获取指定位置的矿石事件。
func (e *Environment) GetMineEventAt(x, y int) *MineEvent {
	for _, me := range e.MineEvents {
		if abs(me.X-x) <= 1 && abs(me.Y-y) <= 1 {
			return me
		}
	}
	return nil
}

// ProcessMineRegen 处理矿石再生（在 tick 循环中调用）。
func (e *Environment) ProcessMineRegen(now int64) {
	for _, me := range e.MineEvents {
		if me.OreCount <= 0 && me.RegenTick > 0 && now >= me.RegenTick {
			me.OreCount = rand.Intn(80) + 20
			me.RegenTick = 0
		}
	}
}

// mineHitRate Delphi g_Config.nMakeMineHitRate 默认 4：每镐 1/4 概率松动矿石
//（ObjBase.pas:21895 Random(g_Config.nMakeMineHitRate)=0）。
const mineHitRate = 4

// tryMineAt Delphi PileStones（ObjBase.pas:21881-21929）：对目标格矿脉挥镐。
// 返回 true 表示发生了有效挖掘（命中矿脉且松动成功），调用方应发送 =DIG；
// 返回 false 表示目标格无矿脉，调用方应回退为普通重击。
func (p *PlayObject) tryMineAt(server *netserver.TCPServer, fx, fy int) bool {
	if p.envir == nil {
		return false
	}
	now := time.Now().UnixMilli()
	if !p.checkActionSpeed(now, p.Engine.Config.GetDigUpInterval(), &p.DigUpTick, server) {
		return true // 限流：本次挥镐被消费，不回退普通攻击
	}
	me := p.envir.GetMineEventAt(fx, fy)
	if me == nil {
		return false
	}

	mined := false
	if me.OreCount > 0 {
		if rand.Intn(mineHitRate) == 0 {
			me.OreCount--
			if me.OreCount <= 0 {
				me.RegenTick = now + mineRegenTime
			}
			mined = true
			cfg := p.Engine.Config
			p.envir.AddPileStonesEvent(server, p.CurrX, p.CurrY, cfg.GetPileStonesDuration())
			// Delphi: 1/nMakeMineRate 概率产矿（Go 用 GetMiningOreRate）
			if rand.Intn(cfg.GetMiningOreRate()) == 0 {
				p.giveOre(server)
			} else if rand.Intn(cfg.GetMiningStoneRate()) == 0 {
				// 只出碎石（仅视觉事件，已在上方添加）
			}
		}
	} else if me.RegenTick == 0 {
		// Delphi: 采空后 10 分钟补充（TStoneMineEvent.AddStoneMine，ObjBase.pas:21922-21925）
		me.RegenTick = now + mineRegenTime
	}

	if !mined {
		return false
	}

	// Delphi: 挖掘成功广播重击动画（ObjBase.pas:21929 SendRefMsg RM_HEAVYHIT）
	p.SendRefMsg(RM_HEAVYHIT, p.Dir, p.CurrX, p.CurrY, "")

	// 武器耐久损耗（Delphi DoDamageWeapon(Random(15)+5)，Go 用配置值）
	if weapon := p.UseItems[protocol.UWeapon]; weapon != nil {
		duraLoss := uint16(p.Engine.Config.GetMiningDuraLoss())
		if weapon.Dura > duraLoss {
			weapon.Dura -= duraLoss
		} else {
			weapon.Dura = 0
		}
	}
	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		p.sysMsg(server, "背包已满！无法携带更多的东西")
	}
	return true
}

// giveOre Delphi MakeMine（ObjBase.pas:21912）：产出一块矿石入包。
func (p *PlayObject) giveOre(server *netserver.TCPServer) {
	cfg := p.Engine.Config
	if p.ItemDB == nil || len(p.ItemList) >= cfg.GetMaxBagSlots() {
		return
	}
	ore := defaultOreTable[rand.Intn(len(defaultOreTable))]
	def := p.ItemDB.GetByName(ore.ItemName)
	if def == nil {
		return
	}
	makeIndex := int32(def.Idx)
	if p.Engine != nil {
		p.Engine.mu.Lock()
		makeIndex = int32(p.Engine.nextItemID)
		p.Engine.nextItemID++
		p.Engine.mu.Unlock()
	}
	dura := uint16(ore.MinDura + rand.Intn(ore.MaxDura-ore.MinDura))
	item := &protocol.UserItem{
		MakeIndex: makeIndex,
		WIndex:    uint16(def.Idx),
		Dura:      dura,
		DuraMax:   dura,
	}
	p.ItemList = append(p.ItemList, item)
	p.SendBagItemsFull(server)
	p.sysMsg(server, "你挖到了"+ore.ItemName+"！")
}

// HandleMineDig 处理 CMMineDig 备用路径（客户端实际走 CMHeavyHit+鹤嘴锄，见 HandleHit）。
func (p *PlayObject) HandleMineDig(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	fdx, fdy := dirToOffset(p.Dir)
	if p.tryMineAt(server, p.CurrX+fdx, p.CurrY+fdy) {
		server.SendRaw(p.Session.ID, "#=DIG!")
	}
}
