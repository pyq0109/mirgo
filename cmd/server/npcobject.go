package main

import (
	"encoding/json"
	"math/rand"
	"sync"
	"time"

	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
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
	DBRecordID int64 // 数据库记录ID，用于持久化删除
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

	// Delphi TNormNpc 构造器字段 (ObjNpc.pas:5646-5658)
	SuperMan      bool // 无敌模式（HP/MP 每 tick 回满，Die() 跳过）
	AntiPoisonNpc int  // 抗毒（默认99）
	FixedHideMode bool // 城堡战隐藏状态

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
	npc := &NpcObject{
		BaseObject:    base,
		Appr:          appr,
		PriceRate:     100,
		GoodsList:     make(map[string]*GoodsStock),
		PriceList:     make(map[int]*ItemPrice),
		SuperMan:      true,  // Delphi ObjNpc.pas:5649
		AntiPoisonNpc: 99,    // Delphi ObjNpc.pas:5652
	}
	base.outer = npc
	return npc
}

func (o *NpcObject) Feature() int32 {
	raceImg := byte(10) // RC_NPC
	if o.IsMerchant {
		raceImg = 50 // RCC_MERCHANT
	} else if o.Race == 15 {
		raceImg = 15 // RC_PEACENPC
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
	defer o.mu.Unlock()
	// 双重检查：可能在等待锁期间已被其他goroutine加载
	if o.script != nil {
		return o.script
	}
	o.script = script
	o.scriptTime = time.Now().UnixMilli()

	return script
}

func (o *NpcObject) InvalidateScript() {
	o.mu.Lock()
	o.script = nil
	o.scriptTime = 0
	o.mu.Unlock()
}

func (o *NpcObject) InitGoodsFromScript(script *NpcScript, itemDB *ItemDB, engine *UserEngine) {
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

	// 立即填充初始库存，避免首次点击时库存为空
	if itemDB != nil {
		for _, cfg := range o.RefillConfig {
			def := itemDB.GetByName(cfg.ItemName)
			if def == nil {
				continue
			}
			stock := o.GoodsList[cfg.ItemName]
			if stock == nil {
				stock = &GoodsStock{}
				o.GoodsList[cfg.ItemName] = stock
			}
			for len(stock.Items) < cfg.MaxCount {
				item := itemDB.CreateUserItem(def.Idx)
				if item == nil {
					break
				}
				if engine != nil {
					item.MakeIndex = engine.allocItemID()
				}
				stock.Items = append(stock.Items, item)
			}
		}
	}
}

func (o *NpcObject) RefillGoods(itemDB *ItemDB, engine *UserEngine) {
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
					if engine != nil {
						item.MakeIndex = engine.allocItemID()
					}
					stock.Items = append(stock.Items, item)
				}
			}
		} else if current > cfg.MaxCount {
			stock.Items = stock.Items[:cfg.MaxCount]
		}

		// Delphi ObjNpc.pas:798-823 CheckItemPrice：补货时价格 +10%
		if ip, ok := o.PriceList[def.Idx]; ok {
			newPrice := ip.Price * 11 / 10
			if newPrice == ip.Price {
				newPrice++
			}
			ip.Price = newPrice
		} else {
			o.PriceList[def.Idx] = &ItemPrice{ItemIdx: def.Idx, Price: int(def.Price) * 11 / 10}
		}
	}
}

func (o *NpcObject) idleAnimate() {
	if rand.Intn(50) != 0 {
		return
	}
	if rand.Intn(2) == 0 {
		o.Dir = rand.Intn(8)
		o.SendRefMsg(RM_TURN, o.Dir, o.CurrX, o.CurrY, "")
	} else {
		o.SendRefMsg(RM_HIT, o.Dir, o.CurrX, o.CurrY, "")
	}
}

// NpcDataKey 返回 NPC 数据持久化键（Delphi: m_sScript + '-' + m_sMapName）。
func (o *NpcObject) NpcDataKey() string {
	if o.MerchantID != "" {
		return o.MerchantID + "-" + o.MapName
	}
	return o.Name + "-" + o.MapName
}

// SaveData 将商品库存和价格列表持久化到 SQLite。
func (o *NpcObject) SaveData(db *storage.Database) {
	if db == nil || !o.IsMerchant {
		return
	}
	key := o.NpcDataKey()
	o.mu.RLock()
	defer o.mu.RUnlock()

	if data, err := json.Marshal(o.GoodsList); err == nil {
		db.SaveNpcData(key, "goods", data)
	}
	if data, err := json.Marshal(o.PriceList); err == nil {
		db.SaveNpcData(key, "prices", data)
	}
}

// LoadData 从 SQLite 加载商品库存和价格列表。
// 恢复的库存实例若无 MakeIndex（旧存档），补分配唯一 ID
//（详细列表与按实例购买依赖它）。
func (o *NpcObject) LoadData(db *storage.Database, engine *UserEngine) {
	if db == nil || !o.IsMerchant {
		return
	}
	key := o.NpcDataKey()

	if data, err := db.LoadNpcData(key, "goods"); err == nil && data != nil {
		o.mu.Lock()
		json.Unmarshal(data, &o.GoodsList)
		if engine != nil {
			for _, stock := range o.GoodsList {
				for _, item := range stock.Items {
					if item.MakeIndex == 0 {
						item.MakeIndex = engine.allocItemID()
					}
				}
			}
		}
		o.mu.Unlock()
	}
	if data, err := db.LoadNpcData(key, "prices"); err == nil && data != nil {
		o.mu.Lock()
		json.Unmarshal(data, &o.PriceList)
		o.mu.Unlock()
	}
}
