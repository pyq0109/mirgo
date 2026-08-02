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

// HandleMineDig 处理玩家采矿动作。
func (p *PlayObject) HandleMineDig(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	now := time.Now().UnixMilli()
	if !p.checkActionSpeed(now, p.Engine.Config.GetDigUpInterval(), &p.DigUpTick, server) {
		return
	}

	// 检查是否装备了镐（简化：任何武器都可以采矿）
	weapon := p.UseItems[protocol.UWeapon]
	if weapon == nil {
		p.sysMsg(server, "你需要装备武器才能采矿")
		return
	}

	// 查找附近矿点
	me := p.envir.GetMineEventAt(p.CurrX, p.CurrY)
	if me == nil {
		p.sysMsg(server, "这里没有矿脉")
		return
	}
	if me.OreCount <= 0 {
		p.sysMsg(server, "矿石已经采完了")
		return
	}

	// 播放采矿动画
	p.SendRefMsg(RM_DIGUP, p.Dir, p.CurrX, p.CurrY, "")

	// 扣武器耐久
	cfg := p.Engine.Config
	duraLoss := uint16(cfg.GetMiningDuraLoss())
	if weapon.Dura > duraLoss {
		weapon.Dura -= duraLoss
	} else {
		weapon.Dura = 0
	}

	// 消耗矿点
	me.OreCount--
	if me.OreCount <= 0 {
		me.RegenTick = time.Now().UnixMilli() + mineRegenTime
	}

	// 产矿判定
	if rand.Intn(cfg.GetMiningOreRate()) == 0 {
		// 产出矿石
		ore := defaultOreTable[rand.Intn(len(defaultOreTable))]
		if p.ItemDB != nil && len(p.ItemList) < cfg.GetMaxBagSlots() {
			def := p.ItemDB.GetByName(ore.ItemName)
			if def != nil {
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
				p.envir.AddPileStonesEvent(server, p.CurrX, p.CurrY, cfg.GetPileStonesDuration())
				return
			}
		}
	} else if rand.Intn(cfg.GetMiningStoneRate()) == 0 {
		// 产出普通石头（简化：给一个碎石物品或仅视觉）
		p.envir.AddPileStonesEvent(server, p.CurrX, p.CurrY, cfg.GetPileStonesDuration())
	}
}
