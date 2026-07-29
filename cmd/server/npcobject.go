package main

import (
	"math/rand"
	"sync"
	"time"

	"github.com/pyq0109/mirgo/internal/protocol"
)

type GoodsConfig struct {
	ItemName   string
	MaxCount   int
	RefillTime int   // 小时
	LastRefill int64 // unix ms
}

type GoodsStock struct {
	Items []*protocol.UserItem
}

type ItemPrice struct {
	ItemIdx int
	Price   int
}

type UpgradeInfo struct {
	PlayerName string
	Item       *protocol.UserItem
	BtDc       byte
	BtSc       byte
	BtMc       byte
	BtDura     byte
	Tick       int64
}

type NpcObject struct {
	*BaseObject
	Appr       uint16
	Script     string // 脚本文件路径
	IsMerchant bool
	MerchantID string // 商人脚本ID（如 "2Bwe"）
	Castle     bool
	Face       int
	Race       int

	// 商人能力标志
	CanBuy      bool
	CanSell     bool
	CanRepair   bool
	CanSRepair  bool
	CanMakeDrug bool
	CanStorage  bool
	CanGetback  bool
	CanUpgrade  bool
	CanGetBackup bool
	CanPrices   bool
	CanSendMsg  bool

	// 商人数据
	PriceRate int   // 价格比率（默认100）
	ItemTypes []int // 可交易物品类型

	// 三层商品架构
	RefillConfig []*GoodsConfig
	GoodsList    map[string]*GoodsStock
	PriceList    map[int]*ItemPrice

	// 武器升级
	UpgradeWeaponList []*UpgradeInfo

	// 脚本缓存
	mu         sync.RWMutex
	script     *NpcScript
	scriptTime int64
}

func NewNpcObject(name string, id int32, appr uint16) *NpcObject {
	base := NewBaseObject(name, id)
	return &NpcObject{
		BaseObject: base,
		Appr:       appr,
		PriceRate:  100,
		GoodsList:  make(map[string]*GoodsStock),
		PriceList:  make(map[int]*ItemPrice),
	}
}

func (o *NpcObject) Feature() int32 {
	raceImg := byte(10) // RC_NPC
	if o.IsMerchant {
		raceImg = 50 // RCC_MERCHANT
	}
	return protocol.MakeMonsterFeature(raceImg, 0, o.Appr)
}

func (o *NpcObject) GetScript() *NpcScript {
	o.mu.RLock()
	if o.script != nil {
		s := o.script
		o.mu.RUnlock()
		return s
	}
	o.mu.RUnlock()

	if o.Script == "" {
		return nil
	}

	script, err := LoadNpcScript(o.Script)
	if err != nil {
		return nil
	}

	o.mu.Lock()
	o.script = script
	o.scriptTime = time.Now().UnixMilli()
	o.mu.Unlock()

	return script
}

func (o *NpcObject) InvalidateScript() {
	o.mu.Lock()
	o.script = nil
	o.scriptTime = 0
	o.mu.Unlock()
}

func (o *NpcObject) InitGoodsFromScript(script *NpcScript) {
	if !o.IsMerchant || script == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	// 应用脚本头中的价格比率和能力标志
	if script.PriceRate > 0 {
		o.PriceRate = script.PriceRate
	}
	if len(script.ItemTypes) > 0 && len(o.ItemTypes) == 0 {
		o.ItemTypes = script.ItemTypes
	}
	if len(script.Capabilities) > 0 {
		o.CanBuy = script.Capabilities["buy"] || script.Capabilities["trading"]
		o.CanSell = script.Capabilities["sell"] || script.Capabilities["trading"]
		o.CanRepair = script.Capabilities["repair"]
		o.CanSRepair = script.Capabilities["s_repair"]
		o.CanMakeDrug = script.Capabilities["makedrug"]
		o.CanStorage = script.Capabilities["storage"]
		o.CanGetback = script.Capabilities["getback"]
		o.CanUpgrade = script.Capabilities["upgradenow"]
		o.CanGetBackup = script.Capabilities["getbackupgnow"]
		o.CanPrices = script.Capabilities["prices"]
		o.CanSendMsg = script.Capabilities["sendmsg"]
	}

	// 初始化商品配置
	if len(script.Goods) == 0 || len(o.RefillConfig) > 0 {
		return
	}

	for _, g := range script.Goods {
		o.RefillConfig = append(o.RefillConfig, &GoodsConfig{
			ItemName:   g.ItemName,
			MaxCount:   g.Count,
			RefillTime: g.RefillTime,
			LastRefill: time.Now().UnixMilli(),
		})
	}
}

func (o *NpcObject) RefillGoods(itemDB *ItemDB) {
	if itemDB == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().UnixMilli()
	for _, cfg := range o.RefillConfig {
		intervalMs := int64(cfg.RefillTime) * 3600 * 1000
		if intervalMs <= 0 {
			intervalMs = 3600 * 1000
		}
		if now-cfg.LastRefill < intervalMs {
			continue
		}
		cfg.LastRefill = now

		stock := o.GoodsList[cfg.ItemName]
		if stock == nil {
			stock = &GoodsStock{}
			o.GoodsList[cfg.ItemName] = stock
		}

		def := itemDB.GetByName(cfg.ItemName)
		if def == nil {
			continue
		}

		current := len(stock.Items)
		if current < cfg.MaxCount {
			for i := current; i < cfg.MaxCount; i++ {
				item := itemDB.CreateUserItem(def.Idx)
				if item != nil {
					stock.Items = append(stock.Items, item)
				}
			}
		} else if current > cfg.MaxCount {
			stock.Items = stock.Items[:cfg.MaxCount]
		}
	}
}

func (o *NpcObject) idleAnimate() {
	if rand.Intn(2) != 0 {
		return
	}
	if rand.Intn(2) == 0 {
		o.Dir = rand.Intn(8)
		o.SendRefMsg(RM_TURN, o.CurrX, o.CurrY, o.Dir, "")
	} else {
		o.SendRefMsg(RM_HIT, o.CurrX, o.CurrY, o.Dir, "")
	}
}
