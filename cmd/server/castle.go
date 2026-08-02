package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type CastleWarState int

const (
	CastlePeace       CastleWarState = iota
	CastleWarDeclared
	CastleWarActive
)

type CastleConfig struct {
	Name           string `json:"name"`
	MapName        string `json:"map"`
	PalaceX        int    `json:"palaceX"`
	PalaceY        int    `json:"palaceY"`
	PalaceRadius   int    `json:"palaceRadius"`
	DoorMaxHP      int    `json:"doorMaxHP"`
	WallMaxHP      int    `json:"wallMaxHP"`
	DoorRepairCost int    `json:"doorRepairCost"`
	WallRepairCost int    `json:"wallRepairCost"`
	WarStartHour   int    `json:"warStartHour"`
	WarDurationMin int    `json:"warDurationMin"`
	GracePeriodSec int    `json:"gracePeriodSec"`
	TaxRate        int    `json:"taxRate"`
	MaxTaxRate     int    `json:"maxTaxRate"`
	ArcherCost     int    `json:"archerCost"`
	MaxGuards      int    `json:"maxGuards"`
}

func DefaultCastleConfig() *CastleConfig {
	return &CastleConfig{
		Name:           "沙巴克",
		MapName:        "3",
		PalaceX:        100,
		PalaceY:        100,
		PalaceRadius:   5,
		DoorMaxHP:      5000,
		WallMaxHP:      10000,
		DoorRepairCost: 100000,
		WallRepairCost: 200000,
		WarStartHour:   20,
		WarDurationMin: 180,
		GracePeriodSec: 300,
		TaxRate:        5,
		MaxTaxRate:     20,
		ArcherCost:     50000,
		MaxGuards:      8,
	}
}

func LoadCastleConfig(configDir string) (*CastleConfig, error) {
	cfg := DefaultCastleConfig()

	data, err := os.ReadFile(filepath.Join(configDir, "castle", "sabuk_wall.jsonc"))
	if err != nil {
		return cfg, nil // 配置文件不存在时使用默认值
	}

	var cleanLines []string
	for _, line := range splitLines(string(data)) {
		trimmed := trimSpace(line)
		if len(trimmed) >= 2 && trimmed[:2] == "//" {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	if err := json.Unmarshal([]byte(joinLines(cleanLines)), cfg); err != nil {
		return nil, fmt.Errorf("parse castle config: %w", err)
	}
	return cfg, nil
}

type CastleObject struct {
	mu           sync.Mutex
	Config       CastleConfig
	OwnerGuild   string
	Gold         int64
	TaxRate      int
	DoorHP       int
	WallHP       int
	WarState     CastleWarState
	WarStartTick int64
	WarEndTick   int64
	AttackGuilds []string
	GuardIDs     []int32
	DoorOpen     bool

	// Delphi: 城堡经济 (Castle.pas:67-80)
	TechLevel   int   // 科技等级
	Power       int   // 能源
	TodayIncome int64 // 今日收入

	// Delphi: 修理冷却 (Castle.pas:1150-1219)
	DoorStruckTick int64 // 城门最后被攻击时间
	WallStruckTick int64 // 城墙最后被攻击时间
}

func NewCastleObject(cfg CastleConfig) *CastleObject {
	return &CastleObject{
		Config:  cfg,
		TaxRate: cfg.TaxRate,
		DoorHP:  cfg.DoorMaxHP,
		WallHP:  cfg.WallMaxHP,
	}
}

func (c *CastleObject) IsAtWar() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.WarState == CastleWarActive
}

func (c *CastleObject) IsAttackingGuild(guildName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, g := range c.AttackGuilds {
		if g == guildName {
			return true
		}
	}
	return false
}

func (c *CastleObject) IsDefendingGuild(guildName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.OwnerGuild != "" && c.OwnerGuild == guildName
}

func (c *CastleObject) DeclareWar(guildName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if guildName == "" || guildName == c.OwnerGuild {
		return false
	}
	if c.WarState == CastleWarActive {
		return false
	}
	for _, g := range c.AttackGuilds {
		if g == guildName {
			return false
		}
	}

	c.AttackGuilds = append(c.AttackGuilds, guildName)
	if c.WarState == CastlePeace {
		c.WarState = CastleWarDeclared
	}
	log.Logf(log.LevelInfo, "Castle", "%s declared war on castle %s", guildName, c.Config.Name)
	return true
}

func (c *CastleObject) StartWar(server *netserver.TCPServer, engine *UserEngine) {
	c.mu.Lock()
	if c.WarState != CastleWarDeclared || len(c.AttackGuilds) == 0 {
		c.mu.Unlock()
		return
	}
	now := time.Now().UnixMilli()
	c.WarState = CastleWarActive
	c.WarStartTick = now
	c.WarEndTick = now + int64(c.Config.WarDurationMin)*60*1000
	attackers := make([]string, len(c.AttackGuilds))
	copy(attackers, c.AttackGuilds)
	c.mu.Unlock()

	// Delphi: 攻城开始时关门 (Castle.pas:697)
	c.ToggleDoor(true) // close door

	// Delphi: 生成弓箭守卫 (Castle.pas:260-290)
	c.spawnCastleArchers(engine, now)

	text := fmt.Sprintf("[攻城战] %s 攻城战开始！进攻方: %s，防守方: %s",
		c.Config.Name, strings.Join(attackers, ","), c.OwnerGuild)
	c.broadcastSysMsg(server, engine, text)
	log.Logf(log.LevelInfo, "Castle", "castle war started: attackers=%v defender=%s", attackers, c.OwnerGuild)
}

// spawnCastleArchers — Delphi: 攻城战时在城堡地图生成弓箭手 (Castle.pas:260-290)
func (c *CastleObject) spawnCastleArchers(engine *UserEngine, now int64) {
	if engine == nil || engine.mapMgr == nil {
		return
	}
	env := engine.mapMgr.FindMap(c.Config.MapName)
	if env == nil {
		return
	}
	// 在皇宫周围生成弓箭手
	count := c.Config.MaxGuards
	if count <= 0 {
		count = 8
	}
	for i := 0; i < count; i++ {
		ax := c.Config.PalaceX + (i%4)*3 - 4
		ay := c.Config.PalaceY + (i/4)*3 - 2
		mon := engine.SpawnMonsterByName(c.Config.MapName, ax, ay, "弓箭守卫", now)
		if mon != nil {
			mon.StickMode = true
			mon.AIBehavior = AIGuard
			mon.ViewRange = 12
			c.mu.Lock()
			c.GuardIDs = append(c.GuardIDs, mon.ID)
			c.mu.Unlock()
		}
	}
}

func (c *CastleObject) EndWar(server *netserver.TCPServer, engine *UserEngine) {
	c.mu.Lock()
	if c.WarState != CastleWarActive {
		c.mu.Unlock()
		return
	}
	c.WarState = CastlePeace
	c.AttackGuilds = nil
	c.WarStartTick = 0
	c.WarEndTick = 0
	c.mu.Unlock()

	text := fmt.Sprintf("[攻城战] %s 攻城战结束", c.Config.Name)
	c.broadcastSysMsg(server, engine, text)
	log.Logf(log.LevelInfo, "Castle", "castle war ended")
}

func (c *CastleObject) DamageDoor(dmg int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DoorHP -= dmg
	c.DoorStruckTick = time.Now().UnixMilli()
	if c.DoorHP <= 0 {
		c.DoorHP = 0
		log.Logf(log.LevelInfo, "Castle", "castle door destroyed")
		return true
	}
	return false
}

func (c *CastleObject) DamageWall(dmg int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Delphi: 非攻城期间城墙无敌 (Castle.pas:720-726, m_boStoneMode=True)
	if c.WarState != CastleWarActive {
		return false
	}
	c.WallHP -= dmg
	c.WallStruckTick = time.Now().UnixMilli()
	if c.WallHP <= 0 {
		c.WallHP = 0
		log.Logf(log.LevelInfo, "Castle", "castle wall destroyed")
		return true
	}
	return false
}

// RepairDoor — Delphi (Castle.pas:1150-1182): 攻城期间不可修，被攻击60秒后才能修
func (c *CastleObject) RepairDoor() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.WarState == CastleWarActive {
		return false
	}
	if c.DoorHP >= c.Config.DoorMaxHP {
		return false
	}
	now := time.Now().UnixMilli()
	if c.DoorStruckTick > 0 && now-c.DoorStruckTick < 60000 {
		return false
	}
	if c.Gold < int64(c.Config.DoorRepairCost) {
		return false
	}
	c.Gold -= int64(c.Config.DoorRepairCost)
	c.DoorHP = c.Config.DoorMaxHP
	log.Logf(log.LevelInfo, "Castle", "castle door repaired (treasury: %d)", c.Gold)
	return true
}

// RepairWall — Delphi (Castle.pas:1184-1219): 攻城期间不可修，被攻击60秒后才能修
func (c *CastleObject) RepairWall() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.WarState == CastleWarActive {
		return false
	}
	if c.WallHP >= c.Config.WallMaxHP {
		return false
	}
	now := time.Now().UnixMilli()
	if c.WallStruckTick > 0 && now-c.WallStruckTick < 60000 {
		return false
	}
	if c.Gold < int64(c.Config.WallRepairCost) {
		return false
	}
	c.Gold -= int64(c.Config.WallRepairCost)
	c.WallHP = c.Config.WallMaxHP
	log.Logf(log.LevelInfo, "Castle", "castle wall repaired (treasury: %d)", c.Gold)
	return true
}

func (c *CastleObject) CheckCapture(engine *UserEngine) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.WarState != CastleWarActive {
		return ""
	}

	radius := c.Config.PalaceRadius
	if radius <= 0 {
		radius = 5
	}

	guildCounts := make(map[string]int)
	for _, p := range engine.allPlayers() {
		if p.Ghost || p.Death || p.MapName != c.Config.MapName {
			continue
		}
		dx := abs(p.CurrX - c.Config.PalaceX)
		dy := abs(p.CurrY - c.Config.PalaceY)
		if dx > radius || dy > radius {
			continue
		}
		if p.GuildName == "" {
			continue
		}
		if !c.isAttackingGuildLocked(p.GuildName) {
			continue
		}
		guildCounts[p.GuildName]++
	}

	for guildName, count := range guildCounts {
		if count >= 3 {
			c.OwnerGuild = guildName
			c.WarState = CastlePeace
			c.AttackGuilds = nil
			log.Logf(log.LevelInfo, "Castle", "castle captured by %s (%d members in palace)", guildName, count)
			return guildName
		}
	}
	return ""
}

func (c *CastleObject) CollectTax(amount int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Gold += amount
	c.TodayIncome += amount
}

func (c *CastleObject) WithdrawGold(amount int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if amount <= 0 || amount > c.Gold {
		return false
	}
	c.Gold -= amount
	return true
}

// DepositGold — Delphi: 城主存款 (Castle.pas:120)
func (c *CastleObject) DepositGold(amount int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if amount <= 0 {
		return false
	}
	c.Gold += amount
	return true
}

func (c *CastleObject) SetTaxRate(rate int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rate < 0 || rate > c.Config.MaxTaxRate {
		return false
	}
	c.TaxRate = rate
	return true
}

func (c *CastleObject) AddGuard(id int32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.GuardIDs) >= c.Config.MaxGuards {
		return false
	}
	c.GuardIDs = append(c.GuardIDs, id)
	return true
}

func (c *CastleObject) RemoveGuard(id int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, gid := range c.GuardIDs {
		if gid == id {
			c.GuardIDs = append(c.GuardIDs[:i], c.GuardIDs[i+1:]...)
			return
		}
	}
}

func (c *CastleObject) GuardCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.GuardIDs)
}

func (c *CastleObject) GetGold() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Gold
}

func (c *CastleObject) GetDoorHP() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.DoorHP, c.Config.DoorMaxHP
}

func (c *CastleObject) GetWallHP() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.WallHP, c.Config.WallMaxHP
}

func (c *CastleObject) GetWarState() CastleWarState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.WarState
}

func (c *CastleObject) GetOwnerGuild() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.OwnerGuild
}

func (c *CastleObject) GetTaxRate() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.TaxRate
}

func (c *CastleObject) ToggleDoor(open bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DoorOpen = open
}

func (c *CastleObject) IsDoorOpen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.DoorOpen
}

// DoorPosition 返回城门的地图坐标（皇宫入口前方）。
func (c *CastleObject) DoorPosition() (int, int) {
	return c.Config.PalaceX, c.Config.PalaceY + c.Config.PalaceRadius + 1
}

// WallPositions 返回城墙段的坐标列表（皇宫周围）。
func (c *CastleObject) WallPositions() [][2]int {
	r := c.Config.PalaceRadius
	px, py := c.Config.PalaceX, c.Config.PalaceY
	var walls [][2]int
	for dx := -r; dx <= r; dx++ {
		walls = append(walls, [2]int{px + dx, py - r})
		walls = append(walls, [2]int{px + dx, py + r})
	}
	for dy := -r + 1; dy < r; dy++ {
		walls = append(walls, [2]int{px - r, py + dy})
		walls = append(walls, [2]int{px + r, py + dy})
	}
	return walls
}

// HandleStructureDamage 攻城战期间玩家攻击城门/城墙时调用。
// 返回是否命中了建筑结构。
func (c *CastleObject) HandleStructureDamage(x, y, damage int) bool {
	if !c.IsAtWar() {
		return false
	}
	doorX, doorY := c.DoorPosition()
	if abs(x-doorX) <= 1 && abs(y-doorY) <= 1 {
		destroyed := c.DamageDoor(damage)
		if destroyed {
			c.ToggleDoor(true)
		}
		return true
	}
	for _, w := range c.WallPositions() {
		if abs(x-w[0]) <= 0 && abs(y-w[1]) <= 0 {
			c.DamageWall(damage)
			return true
		}
	}
	return false
}

// IsBlockedByDoor 检查指定位置是否被关闭的城门阻挡。
func (c *CastleObject) IsBlockedByDoor(x, y int) bool {
	if c.IsDoorOpen() || c.DoorHP <= 0 {
		return false
	}
	doorX, doorY := c.DoorPosition()
	return abs(x-doorX) <= 1 && abs(y-doorY) <= 0
}

func (c *CastleObject) isAttackingGuildLocked(guildName string) bool {
	for _, g := range c.AttackGuilds {
		if g == guildName {
			return true
		}
	}
	return false
}

func (c *CastleObject) ProcessCastleTick(engine *UserEngine, server *netserver.TCPServer, now int64) {
	state := c.GetWarState()

	switch state {
	case CastleWarDeclared:
		hour := time.Now().Hour()
		if hour >= c.Config.WarStartHour {
			c.StartWar(server, engine)
		}
	case CastleWarActive:
		c.mu.Lock()
		endTick := c.WarEndTick
		c.mu.Unlock()
		if now >= endTick {
			c.EndWar(server, engine)
		} else {
			captured := c.CheckCapture(engine)
			if captured != "" {
				text := fmt.Sprintf("[攻城战] %s 被 %s 占领！", c.Config.Name, captured)
				c.broadcastSysMsg(server, engine, text)
			}
		}
	}

	// 税收：每 60 秒向城主行会金库收取税金
	if state == CastlePeace && c.OwnerGuild != "" {
		c.collectPeriodicTax(engine, now)
	}
}

var lastTaxTick int64

func (c *CastleObject) collectPeriodicTax(engine *UserEngine, now int64) {
	if now-lastTaxTick < 60000 {
		return
	}
	lastTaxTick = now

	rate := c.GetTaxRate()
	if rate <= 0 {
		return
	}

	var totalTax int64
	for _, p := range engine.allPlayers() {
		if p.Ghost || p.Death {
			continue
		}
		tax := int64(p.Gold) * int64(rate) / 100
		if tax > 0 && tax <= int64(p.Gold) {
			p.Gold -= int(tax)
			totalTax += tax
		}
	}
	if totalTax > 0 {
		c.CollectTax(totalTax)
	}
}

func (c *CastleObject) broadcastSysMsg(server *netserver.TCPServer, engine *UserEngine, text string) {
	msg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
	for _, p := range engine.allPlayers() {
		if !p.Ghost {
			server.Send(p.Session.ID, msg, protocol.EncodeString(text))
		}
	}
}

func (e *UserEngine) allPlayers() []*PlayObject {
	e.mu.RLock()
	defer e.mu.RUnlock()
	players := make([]*PlayObject, 0, len(e.PlayObjectList))
	for _, p := range e.PlayObjectList {
		players = append(players, p)
	}
	return players
}
