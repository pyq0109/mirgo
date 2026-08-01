package main

import (
	"encoding/binary"
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// PendingMagic 延迟魔法（火球等弹道技能的飞行时间）。
type PendingMagic struct {
	MagID    int
	Power    int
	TargetX  int
	TargetY  int
	FireTick int64 // 预计命中时间
}

type PlayObject struct {
	*BaseObject

	Session     *netserver.Session
	AccountName string
	SessionID   int64
	ReadyToRun  bool
	MapMgr      *MapManager
	ItemDB      *ItemDB
	MagicDB     *MagicDB
	Engine      *UserEngine

	LearnedMagics []*PlayerMagic

	VisibleActors  map[int32]*VisibleEntry
	knownItems     map[int32]bool
	lastVisionTick int64
	visionInterval int64 // Delphi: Random(2000)+2000 = 2~4s
	lastRegenTick  int64
	deathTick      int64
	skeletonSent   bool

	WalkTick       int64
	RunTick        int64
	HorseRunTick   int64
	TurnTick       int64
	SpellTick      int64
	DigUpTick      int64
	WalkSpeed      int64
	RunSpeed       int64
	OverSpeedCount int
	LastSpeedViolationTick int64 // 上次超速违规时间（用于衰减）
	LastActionTick int64         // 上次动作时间（全局动作间隔）

	HitTick     int64
	FireHitTick int64
	TwinHitTick int64

	SkillPower int
	SkillLng   int
	SkillWid   int

	// Delphi m_nHitPlus / m_nHitDouble (ObjBase.pas:22125-22132)
	HitPlus   int // PowerHit 平坦加成（攻杀剑术）
	HitDouble int // FireHit/TwinHit 百分比加成（烈火/狂风）

	// Delphi PowerHit 自动蓄力 (ObjBase.pas:8862-8875)
	PowerHitCount int
	PowerHitReady bool

	// Delphi FireHit 激活窗口 (ObjBase.pas:9782, 6427)
	FireHitActive       bool
	FireHitActivateTick int64

	// Delphi 受击硬直 (ObjBase.pas:25234)
	StruckTick int64

	// Delphi m_btGreenPoisoningPoint (ObjBase.pas:22730)
	GreenPoisonDamage int

	PkPoint         int
	LastPkDecayTick int64
	PKFlag          bool  // 正当防卫标记（Delphi m_boPKFlag）
	PKFlagTick      int64 // PK 旗设置时间
	OnHorse         bool
	HorseType       byte
	Permission      byte
	ShutupTick      int64 // 禁言到期时间

	LastHiterID   int32 // 最后攻击者 ID（供奴隶目标选择）
	LastHiterTick int64
	EnterMapTick  int64 // 进入地图时间（切图保护）

	Deal         *DealState
	GuildName    string
	GuildRank    string
	StorageItems []*protocol.UserItem
	AttackMode   byte
	AllowGroup   bool // 是否接受组队邀请（CMGroupMode）

	MasterName       string
	ApprenticeNames  []string
	MasterRecallTick int64

	DearName       string // 配偶名字（Delphi m_sDearName）
	DearRecallTick int64  // 夫妻传送冷却

	Friends []string // 好友列表

	// 计算后的战斗属性（Delphi m_btHitPoint/m_btSpeedPoint，ObjBase.pas:1241-1242）。
	HitPoint   int
	SpeedPoint int
	BonusPoint int

	HasParalysis   bool
	HasRevival     bool
	HasTeleport    bool
	HasProbe       bool
	HasFlame       bool
	HasRecovery    bool
	HasAngry       bool
	HasMagicShield bool
	HasMuscle      bool
	HasRecallSuite bool
	HongMoSuite    int // 虹魔套装吸血百分比 (Delphi m_nHongMoSuite)

	// 临时 Buff（StdMode 3, Shape 12 神水/精酿）。
	BuffDC         int
	BuffMC         int
	BuffSC         int
	BuffHP         int
	BuffMP         int
	BuffHitSpeed   int
	BuffExpireTick int64

	SlaveIDs   []int32 // 当前宠物 ID 列表
	SlaveLevel int     // 宠物等级（1-7，Delphi m_btSlaveExpLevel）

	// Delphi: RM_DELAYMAGIC 延迟魔法队列 (ObjBase.pas:4565-4582)
	PendingMagics []PendingMagic

	// E2: 任务位标志（3×1024 bits，Delphi m_QuestUnitOpen/m_QuestUnit/m_QuestFlag）
	QuestUnitOpen [128]byte
	QuestUnit     [128]byte
	QuestFlag     [128]byte

	ScriptVars  [10]int
	ScriptVarsD [10]int  // 动态变量 D0-D9
	ScriptVarsM [100]int // 持久变量 M0-M99

	CreditPoint    int    // 声望点（Delphi m_nCreditPoint）
	ReNewLevel     int    // 转生等级（Delphi m_nReNewLevel）
	StoragePassword string // 仓库密码
	AutoGetExp     int    // 自动获取经验点数

	// Delphi: 自动获取经验 (ObjBase.pas:7100-7105)
	AutoGetExpTime     int64  // 间隔（毫秒）
	AutoGetExpPoint    int    // 每次经验值
	AutoGetExpMap      string // 限定地图（空=不限）
	AutoGetExpSafeZone bool   // 是否要求安全区
	autoGetExpTick     int64  // 上次给经验时间

	// Delphi: 反外挂 XOR 校验 (ObjBase.pas:7017-7048)
	RemoteXORKey int32 // 客户端上报密钥（-1=未设置）

	StrScriptVars map[string]string
	nameLists     map[string][]string

	// NPC 脚本导航状态（Delphi m_nScriptGotoCount/m_sScriptGoBackLable/m_sScriptCurrLable）
	ScriptGotoCount int
	ScriptGoBackLabel string
	ScriptCurrLabel   string
	CurrentNpc        *NpcObject

	// 标签安全白名单（Delphi m_CanJmpScriptLableList, ObjBase.pas:25372）
	CanJmpLabels []string

	// TIMERECALL 定时传送回调 (Delphi ObjNpc.pas:7800-7807)
	TimeRecall     bool
	TimeRecallTick int64
	RecallMap      string
	RecallX, RecallY int

	// Delphi: 经验/攻击倍率限时 (ObjBase.pas:6882-6903)
	KillMonExpRate     int   // 杀怪经验倍率（百分比，100=1x，0 视为 100）
	KillMonExpRateTick int64 // 到期时间（0=无限制）
	PowerRate          int   // 攻击力倍率（百分比，100=1x，0 视为 100）
	PowerRateTick      int64 // 到期时间

	// 渐进回复池（Delphi m_nIncHealth/m_nIncSpell/m_nIncHealing, ObjBase.pas:3782）
	IncHealth   int
	IncSpell    int
	IncHealing  int
	lastIncTick int64
}

type VisibleEntry struct {
	ID   int32
	Flag int
}

func NewPlayObject(session *netserver.Session, name string, id int32) *PlayObject {
	base := NewBaseObject(name, id)
	account := ""
	if session != nil {
		account = session.AccountName
	}
	p := &PlayObject{
		BaseObject:     base,
		Session:        session,
		AccountName:    account,
		VisibleActors:  make(map[int32]*VisibleEntry),
		knownItems:     make(map[int32]bool),
		WalkSpeed:      1400,
		RunSpeed:       1400,
		WalkTick:       time.Now().UnixMilli(),
		visionInterval: 2000 + rand.Int63n(2000),
		StrScriptVars:  make(map[string]string),
		nameLists:      make(map[string][]string),
	}
	base.outer = p
	return p
}

func (p *PlayObject) Operate(server *netserver.TCPServer) {
	for {
		msg, ok := p.GetMsg()
		if !ok {
			break
		}
		p.ProcessMessage(msg, server)
	}

	now := time.Now().UnixMilli()

	p.ProcessStatusEffects(server, now)
	p.DecayPkPoint(now)
	p.CheckPKStatus(now)
	p.processPendingMagics(server, now)

	if p.Death {
		if p.deathTick > 0 && now-p.deathTick > 10000 && !p.skeletonSent {
			p.skeletonSent = true
			// 非新鲜死亡：SM_DEATH 直接显示尸体/骨架。
			p.envir.broadcastDeathMsg(p.BaseObject, p.ID, p.CurrX, p.CurrY, p.Dir, false)
		}
		if now-p.deathTick > 180000 {
			p.resurrect(server)
		}
		return
	}

	p.Regenerate(server, now)
	p.processIncHealth(server, now)

	// Delphi: FireHit 20s 过期 (ObjBase.pas:6427)
	if p.FireHitActive && now-p.FireHitActivateTick > 20000 {
		p.FireHitActive = false
	}

	// Delphi: 经验/攻击倍率到期重置 (ObjBase.pas:6882-6903)
	if p.KillMonExpRateTick > 0 && now >= p.KillMonExpRateTick {
		p.KillMonExpRate = 0
		p.KillMonExpRateTick = 0
	}
	if p.PowerRateTick > 0 && now >= p.PowerRateTick {
		p.PowerRate = 0
		p.PowerRateTick = 0
	}

	// Delphi: 自动获取经验 (ObjBase.pas:7100-7105)
	if p.AutoGetExpPoint > 0 && p.AutoGetExpTime > 0 && now-p.autoGetExpTick >= p.AutoGetExpTime {
		mapOK := p.AutoGetExpMap == "" || p.MapName == p.AutoGetExpMap
		safeOK := !p.AutoGetExpSafeZone || CheckSafeZone(p.MapName, p.CurrX, p.CurrY)
		if mapOK && safeOK {
			p.autoGetExpTick = now
			p.addExp(server, p.AutoGetExpPoint)
		}
	}

	// TIMERECALL 到期检查
	if p.TimeRecall && now >= p.TimeRecallTick {
		p.TimeRecall = false
		if p.MapMgr != nil {
			if env := p.MapMgr.FindMap(p.RecallMap); env != nil {
				p.EnterAnotherMap(server, env, p.RecallX, p.RecallY)
			}
		}
	}

	if now-p.lastVisionTick >= p.visionInterval {
		p.lastVisionTick = now
		p.SearchViewRange(server)
	}
}

func (p *PlayObject) ProcessMessage(msg SendMessage, server *netserver.TCPServer) {
	if p.Death {
		switch msg.Ident {
		case protocol.CMSay, protocol.CMQueryBagItems, protocol.CMTakeOnItem, protocol.CMTakeOffItem:
		default:
			return
		}
	}
	switch msg.Ident {
	case protocol.CMTurn:
		p.HandleTurn(msg, server)
	case protocol.CMWalk:
		p.HandleWalk(msg, server)
	case protocol.CMRun:
		p.HandleRun(msg, server)
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit, protocol.CMPowerHit, protocol.CMLongHit, protocol.CMWideHit, protocol.CMFireHit, protocol.CMCrsHit, protocol.CMTwinHit:
		p.HandleHit(msg, server)
	case protocol.CMSpell:
		p.HandleSpellFull(msg, server)
	case protocol.CMPickup:
		p.HandlePickup(msg, server)
	case protocol.CMTakeOnItem:
		p.HandleTakeOnItem(msg, server)
	case protocol.CMTakeOffItem:
		p.HandleTakeOffItem(msg, server)
	case protocol.CMEat:
		p.HandleEatItem(msg, server)
	case protocol.CMMagicKeyChange:
		p.HandleMagicKeyChange(msg, server)
	case protocol.CMSay:
		p.HandleSay(msg, server)
	case protocol.CMWhisper:
		p.HandleWhisper(msg, server)
	case protocol.CMAddFriend:
		p.HandleAddFriend(msg, server)
	case protocol.CMDelFriend:
		p.HandleDelFriend(msg, server)
	case protocol.CMQueryFriends:
		p.HandleQueryFriends(server)
	case protocol.CMClickNPC:
		p.HandleNpcClick(msg, server)
	case protocol.CMCreateGroup:
		p.HandleCreateGroup(msg, server)
	case protocol.CMGroupMode:
		p.HandleGroupMode(msg, server)
	case protocol.CMAddGroupMember:
		p.HandleAddGroupMember(msg, server)
	case protocol.CMDelGroupMember:
		p.HandleDelGroupMember(msg, server)
	case protocol.CMOpenGuildDlg:
		p.HandleOpenGuildDlg(msg, server)
	case protocol.CMGuildMemberList:
		p.HandleGuildMemberListRequest(msg, server)
	case protocol.CMGuildUpdateRankInfo:
		p.HandleGuildUpdateRankInfo(msg, server)
	case protocol.CMGuildHome:
		p.HandleGuildHome(msg, server)
	case protocol.CMDealTry:
		p.HandleDealTry(msg, server)
	case protocol.CMDealAddItem:
		p.HandleDealAddItem(msg, server)
	case protocol.CMDealDelItem:
		p.HandleDealDelItem(msg, server)
	case protocol.CMDealCancel:
		p.HandleDealCancel(server)
	case protocol.CMDealChgGold:
		p.HandleDealChgGold(msg, server)
	case protocol.CMDealEnd:
		p.HandleDealEnd(server)
	case protocol.CMUserStorageItem:
		p.HandleStorageItem(msg, server)
	case protocol.CMUserTakeBackStorageItem:
		p.HandleTakeBackStorageItem(msg, server)
	case protocol.CMGuildAddMember:
		p.HandleGuildAddMember(msg, server)
	case protocol.CMGuildDelMember:
		p.HandleGuildDelMember(msg, server)
	case protocol.CMGuildAlly:
		p.HandleGuildAlly(msg, server)
	case protocol.CMGuildBreakAlly:
		p.HandleGuildBreakAlly(msg, server)
	case protocol.CMGuildUpdateNotice:
		p.HandleGuildUpdateNotice(msg, server)
	case protocol.CMGuildWar:
		p.HandleGuildWar(msg, server)
	case protocol.CMHorseRun:
		p.HandleHorseRun(msg, server)
	case protocol.CMOpenDoor:
		p.HandleOpenDoor(msg, server)
	case protocol.CMMineDig:
		p.HandleMineDig(server)
	case protocol.CMButch:
		p.HandleButch(msg, server)
	case protocol.CMUserBuyItem:
		p.HandleBuyItem(msg, server)
	case protocol.CMUserSellItem:
		p.HandleSellItem(msg, server)
	case protocol.CMUserRepairItem:
		p.HandleRepairItem(msg, server)
	case protocol.CMMerchantQuerySellPrice:
		p.HandleQuerySellPrice(msg, server)
	case protocol.CMMerchantQueryRepairCost:
		p.HandleQueryRepairCost(msg, server)
	case protocol.CMUserMakeDrugItem:
		p.HandleMakeDrugItem(msg, server)
	case protocol.CMMerchantDlgSelect:
		p.HandleMerchantDlgSelect(msg, server)
	case protocol.CMDropItem:
		p.HandleDropItem(msg, server)
	case protocol.CMDropGold:
		p.HandleDropGold(msg, server)
	case protocol.CMChangeAttackMode:
		p.HandleChangeAttackMode(msg, server)
	case protocol.CMAdjustBonus:
		p.HandleAdjustBonus(msg, server)
	case protocol.CMQueryUserState:
		p.HandleQueryUserState(msg, server)
	case RM_WALK:
		p.sendMovementToClient(server, protocol.SMWalk, msg)
	case RM_RUN:
		p.sendMovementToClient(server, protocol.SMRun, msg)
	case RM_HORSERUN:
		p.sendMovementToClient(server, protocol.SMHorseRun, msg)
	case RM_TURN:
		p.sendTurnToClient(server, msg)
	case RM_DISAPPEAR:
		p.sendDisappearToClient(server, msg)
	case RM_HIT:
		p.sendHitToClient(server, protocol.SMHit, msg)
	case RM_HEAVYHIT:
		p.sendHitToClient(server, protocol.SMHeavyHit, msg)
	case RM_BIGHIT:
		p.sendHitToClient(server, protocol.SMBigHit, msg)
	case RM_POWERHIT:
		p.sendHitToClient(server, protocol.SMPowerHit, msg)
	case RM_LONGHIT:
		p.sendHitToClient(server, protocol.SMLongHit, msg)
	case RM_WIDEHIT:
		p.sendHitToClient(server, protocol.SMWideHit, msg)
	case RM_FIREHIT:
		p.sendHitToClient(server, protocol.SMFireHit, msg)
	case RM_CRSHIT:
		p.sendHitToClient(server, protocol.SMCrsHit, msg)
	case RM_TWINHIT:
		p.sendHitToClient(server, protocol.SMTwinHit, msg)
	case RM_STRUCK:
		p.sendStruckToClient(server, msg)
	case RM_DEATH:
		p.sendDeathToClient(server, msg)
	case RM_SPELL:
		p.sendSpellToClient(server, msg)
	case RM_FEATURECHANGED:
		p.sendFeatureChangedToClient(server, msg)
	case RM_BREAKWEAPON:
		breakMsg := protocol.MakeDefaultMsg(protocol.SMBreakWeapon, msg.SourceID, 0, 0, 0)
		server.Send(p.Session.ID, breakMsg, "")
	case RM_CHANGENAMECOLOR:
		colorMsg := protocol.MakeDefaultMsg(protocol.SMChangeNameColor, msg.SourceID, uint16(msg.Param1), 0, 0)
		server.Send(p.Session.ID, colorMsg, "")
	}
}

func (p *PlayObject) HandleTurn(msg SendMessage, server *netserver.TCPServer) {
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	now := time.Now().UnixMilli()
	if !p.checkActionSpeed(now, p.Engine.Config.GetTurnInterval(), &p.TurnTick, server) {
		return
	}
	p.TurnTo(dir)
	server.SendRaw(p.Session.ID, "#+GOOD!")
}

func (p *PlayObject) HandleWalk(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	now := time.Now().UnixMilli()
	// Delphi: 受击硬直 (ObjBase.pas:25234)
	if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
		return
	}
	interval := p.Engine.Config.GetWalkInterval()
	if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
		interval *= 2
	}
	if !p.checkActionSpeed(now, interval, &p.WalkTick, server) {
		return
	}

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	// F11: GM 穿墙（仅检查地形）
	if p.Permission > 9 {
		dx, dy := dirToOffset(dir)
		nx, ny := p.CurrX+dx, p.CurrY+dy
		if p.envir != nil && p.envir.CanWalkAdmin(nx, ny) {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.CurrX, p.CurrY = nx, ny
			p.Dir = dir
			p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
			p.SendRefMsg(RM_WALK, dir, p.CurrX, p.CurrY, "")
			server.SendRaw(p.Session.ID, "#+GOOD!")
			p.CheckMapRoute(server)
			return
		}
	}
	if p.WalkTo(dir) {
		p.SendRefMsg(RM_WALK, dir, p.CurrX, p.CurrY, "")
		server.SendRaw(p.Session.ID, "#+GOOD!")
		p.CheckMapRoute(server)
	} else {
		p.sendMoveFail(server)
	}
}

func (p *PlayObject) HandleRun(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	now := time.Now().UnixMilli()
	if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
		return
	}
	// F10: 仅步行模式
	if p.Engine.Config.Game.WalkOnly && p.Permission < 10 {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	interval := p.Engine.Config.GetRunInterval()
	if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
		interval *= 2
	}
	if !p.checkActionSpeed(now, interval, &p.RunTick, server) {
		return
	}

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	dx, dy := dirToOffset(dir)
	x1, y1 := p.CurrX+dx, p.CurrY+dy
	x2, y2 := p.CurrX+dx*2, p.CurrY+dy*2
	// F11: GM 穿墙
	if p.Permission > 9 {
		if p.envir == nil || !p.envir.CanWalkAdmin(x2, y2) {
			p.sendMoveFail(server)
			return
		}
	} else {
		ignore := p.runIgnoreEntities()
		if p.envir == nil || !p.envir.CanWalkEx(x1, y1, ignore) || !p.envir.CanWalkEx(x2, y2, ignore) {
			p.sendMoveFail(server)
			return
		}
	}
	p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.CurrX, p.CurrY = x2, y2
	p.Dir = dir
	p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.SendRefMsg(RM_RUN, dir, p.CurrX, p.CurrY, "")
	server.SendRaw(p.Session.ID, "#+GOOD!")
	p.CheckMapRoute(server)
}

func (p *PlayObject) HandleHorseRun(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	// F10: 仅步行模式
	if p.Engine.Config.Game.WalkOnly && p.Permission < 10 {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	now := time.Now().UnixMilli()
	interval := p.Engine.Config.GetRunInterval()
	if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
		interval *= 2
	}
	if !p.checkActionSpeed(now, interval, &p.HorseRunTick, server) {
		return
	}
	dir := msg.Param1
	if dir < 0 || dir > 7 {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	if !p.OnHorse {
		p.OnHorse = true
		p.broadcastFeatureChanged(server)
	}
	dx, dy := dirToOffset(dir)
	x1, y1 := p.CurrX+dx, p.CurrY+dy
	x2, y2 := p.CurrX+dx*2, p.CurrY+dy*2
	x3, y3 := p.CurrX+dx*3, p.CurrY+dy*3
	// F11: GM 穿墙
	if p.Permission > 9 {
		if p.envir == nil || !p.envir.CanWalkAdmin(x3, y3) {
			p.sendMoveFail(server)
			return
		}
	} else {
		ignore := p.runIgnoreEntities()
		if p.envir == nil || !p.envir.CanWalkEx(x1, y1, ignore) || !p.envir.CanWalkEx(x2, y2, ignore) || !p.envir.CanWalkEx(x3, y3, ignore) {
			p.sendMoveFail(server)
			return
		}
	}
	p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.CurrX, p.CurrY = x3, y3
	p.Dir = dir
	p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.SendRefMsg(RM_HORSERUN, dir, p.CurrX, p.CurrY, "")
	server.SendRaw(p.Session.ID, "#+GOOD!")
	p.CheckMapRoute(server)
}

func (p *PlayObject) runIgnoreEntities() bool {
	cfg := p.Engine.Config
	return cfg.Game.DisableRun || (p.Permission > 9 && cfg.Game.GMRunAll)
}

// checkActionSpeed 校验动作间隔并记录超速违规。
// 返回 true 表示允许动作，false 表示拒绝（速度过快）。
func (p *PlayObject) checkActionSpeed(now, interval int64, tick *int64, server *netserver.TCPServer) bool {
	// E5: 超速计数衰减 — 10秒无违规则 -1
	if p.OverSpeedCount > 0 && now-p.LastSpeedViolationTick > 10000 {
		p.OverSpeedCount--
		p.LastSpeedViolationTick = now
	}
	if now-*tick < interval {
		p.OverSpeedCount++
		p.LastSpeedViolationTick = now
		cfg := p.Engine.Config
		if p.OverSpeedCount > cfg.GetSpeedHackMax() && cfg.Game.SpeedHackKick {
			log.Logf(log.LevelWarn, "Server", "speed-hack kick: %s (count=%d)", p.Name, p.OverSpeedCount)
			server.CloseSession(p.Session.ID)
			return false
		}
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return false
	}
	*tick = now
	return true
}

func hitSkillMagID(ident int) (int, bool) {
	switch ident {
	case protocol.CMPowerHit:
		return 7, true // 攻杀剑术
	case protocol.CMLongHit:
		return 12, true // 刺杀剑术
	case protocol.CMWideHit:
		return 25, true // 半月弯刀
	case protocol.CMFireHit:
		return 26, true // 烈火剑法
	case protocol.CMCrsHit:
		return 39, true // 十字斩
	case protocol.CMTwinHit:
		return 38, true // 狂风斩
	default:
		return 0, false
	}
}

func (p *PlayObject) HandleHit(msg SendMessage, server *netserver.TCPServer) {
	now := time.Now().UnixMilli()
	hitInterval := p.Engine.Config.GetHitIntervalTime()
	if now-p.HitTick < hitInterval {
		p.OverSpeedCount++
		p.LastSpeedViolationTick = now
		return
	}

	// Delphi: 受击硬直阻止攻击 (ObjBase.pas:25234)
	if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}

	if magID, ok := hitSkillMagID(msg.Ident); ok && p.findMagic(magID) == nil {
		msg.Ident = protocol.CMHit
	}

	p.CheckWeaponUpgradeStatus(server)
	if p.UseItems[protocol.UWeapon] == nil && msg.Ident != protocol.CMHit {
		return
	}

	// Delphi: FireHit 是激活模型，不是直接攻击 (ObjBase.pas:9782)
	if msg.Ident == protocol.CMFireHit {
		if now-p.FireHitTick < 10000 {
			server.SendRaw(p.Session.ID, "#+FAIL!")
			return
		}
		p.FireHitActive = true
		p.FireHitActivateTick = now
		p.FireHitTick = now
		p.SendSpecialAttackFlags(server)
		server.SendRaw(p.Session.ID, "#+GOOD!")
		return
	}
	// Delphi: TwinHit 同理 (ObjBase.pas:9797)
	if msg.Ident == protocol.CMTwinHit {
		if now-p.TwinHitTick < 60000 {
			return
		}
		p.TwinHitTick = now
		p.SendSpecialAttackFlags(server)
	}

	p.HitTick = now

	// Delphi: 攻击延迟回血 Dec(m_nHealthTick, 30) (ObjBase.pas:8876-8880)
	p.lastRegenTick = now

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.Dir = dir

	if p.envir == nil {
		return
	}
	if IsSafeZone(p.envir, p.CurrX, p.CurrY) {
		return
	}

	// Delphi: PowerHit 自动蓄力 (ObjBase.pas:8862-8875)
	p.advancePowerHitCharge(msg.Ident, server)

	// Delphi: FireHit 激活窗口内自动触发 (ObjBase.pas:6427)
	fireHitTriggered := false
	if p.FireHitActive && now-p.FireHitActivateTick < 20000 {
		fireHitTriggered = true
		p.FireHitActive = false
	}

	// 确定广播 RM_ 类型
	rmIdent := RM_HIT
	switch {
	case fireHitTriggered:
		rmIdent = RM_FIREHIT
		msg.Ident = protocol.CMFireHit
	case p.PowerHitReady && msg.Ident == protocol.CMHit:
		rmIdent = RM_POWERHIT
		msg.Ident = protocol.CMPowerHit
		p.PowerHitReady = false
	case msg.Ident == protocol.CMHeavyHit:
		rmIdent = RM_HEAVYHIT
	case msg.Ident == protocol.CMBigHit:
		rmIdent = RM_BIGHIT
	case msg.Ident == protocol.CMPowerHit:
		rmIdent = RM_POWERHIT
	case msg.Ident == protocol.CMLongHit:
		rmIdent = RM_LONGHIT
	case msg.Ident == protocol.CMWideHit:
		rmIdent = RM_WIDEHIT
	case msg.Ident == protocol.CMCrsHit:
		rmIdent = RM_CRSHIT
	case msg.Ident == protocol.CMTwinHit:
		rmIdent = RM_TWINHIT
	}
	p.SendRefMsg(rmIdent, dir, p.CurrX, p.CurrY, "")
	server.SendRaw(p.Session.ID, "#+GOOD!")

	// Delphi: WideHit/CRS 消耗 MP (ObjBase.pas:18788-18811)
	if msg.Ident == protocol.CMWideHit || msg.Ident == protocol.CMCrsHit {
		if !p.consumeSkillMP(msg.Ident, server) {
			msg.Ident = protocol.CMHit
		}
	}

	dx, dy := dirToOffset(dir)

	// 半月弯刀：3 方向 (Delphi g_Config.WideAttack[0..2])
	if msg.Ident == protocol.CMWideHit {
		p.doWideAttack(server, dir, 3)
		return
	}
	// CRS 十字斩：7 方向 (Delphi g_Config.CrsAttack[0..6])
	if msg.Ident == protocol.CMCrsHit {
		p.doWideAttack(server, dir, 7)
		return
	}

	target := p.findAttackTarget(p.CurrX+dx, p.CurrY+dy)

	// Delphi: 刺杀剑术穿透 2 格 (ObjBase.pas:22167-22185)
	if msg.Ident == protocol.CMLongHit {
		pm := p.findMagic(12)
		if pm != nil {
			baseDamage := p.calcDamage(nil)
			secPwr := p.calcSkillPower(baseDamage, pm, 2)
			x2, y2 := p.CurrX+dx*2, p.CurrY+dy*2
			if t2 := p.findAttackTarget(x2, y2); t2 != nil && p.CanAttackTarget(t2) {
				if !IsSafeZone(p.envir, t2.CurrX, t2.CurrY) {
					if p.hitCheck(t2) {
						p.applyDamage(server, t2, secPwr, dir)
					}
				}
			}
		}
		if target == nil {
			return
		}
	}

	if target == nil {
		if p.Engine != nil && p.Engine.Castle != nil && p.envir != nil && p.envir.Castle != nil {
			attackX, attackY := p.CurrX+dx, p.CurrY+dy
			damage := p.rollDC()
			if p.Engine.Castle.HandleStructureDamage(attackX, attackY, damage) {
				p.SendRefMsg(RM_HIT, dir, attackX, attackY, "")
			}
		}
		return
	}

	// Delphi: IsProperTarget 攻击模式检查 (ObjBase.pas:21495)
	if !p.CanAttackTarget(target) {
		return
	}
	if IsSafeZone(p.envir, target.CurrX, target.CurrY) {
		return
	}

	// Delphi: IsProtectTarget 等级保护 (ObjBase.pas:21258-21330)
	if tp := p.envir.getPlayerByBase(target); tp != nil && p.IsProtectTarget(tp) {
		return
	}

	// Delphi: 命中/闪避 (ObjBase.pas:22243)
	if !p.hitCheck(target) {
		return
	}

	damage := p.calcDamage(target)
	damage = p.applySkillBonus(damage, msg.Ident, fireHitTriggered)
	if damage < 1 {
		damage = 1
	}
	p.applyDamage(server, target, damage, dir)
}

// doWideAttack — Delphi SwordWideAttack/CrsWideAttack (ObjBase.pas:22028-22075)
// count=3 为半月弯刀（dir-1, dir, dir+1），count=7 为 CRS（除正后方外全部）。
func (p *PlayObject) doWideAttack(server *netserver.TCPServer, dir, count int) {
	pm := p.findMagic(25) // 半月弯刀
	if msg := p.findMagic(39); msg != nil && count == 7 {
		pm = msg // CRS
	}
	baseDamage := p.calcDamage(nil)
	secPwr := baseDamage
	if pm != nil {
		secPwr = p.calcSkillPower(baseDamage, pm, 10)
	}
	if secPwr < 1 {
		secPwr = 1
	}

	var offsets []int
	if count == 3 {
		offsets = []int{-1, 0, 1}
	} else {
		offsets = []int{-3, -2, -1, 0, 1, 2, 3}
	}

	for _, off := range offsets {
		d := ((dir + off) % 8 + 8) % 8
		ddx, ddy := dirToOffset(d)
		t := p.findAttackTarget(p.CurrX+ddx, p.CurrY+ddy)
		if t == nil {
			continue
		}
		if !p.CanAttackTarget(t) || IsSafeZone(p.envir, t.CurrX, t.CurrY) {
			continue
		}
		if p.hitCheck(t) {
			p.applyDamage(server, t, secPwr, d)
		}
	}
}

// hitCheck — Delphi 命中判定 (ObjBase.pas:22243):
// m_btHitPoint < Random(target.m_btSpeedPoint) → MISS
func (p *PlayObject) hitCheck(target *BaseObject) bool {
	spd := target.SpeedPoint
	if spd < 1 {
		spd = 1
	}
	return !(p.HitPoint < rand.Intn(spd))
}

// rollDC — 仅掷 DC 骰（不含防御减伤），用于攻城等场景。
func (p *PlayObject) rollDC() int {
	loDC := int(p.WAbil.DC & 0xFFFF)
	hiDC := int(p.WAbil.DC >> 16)
	if hiDC > loDC {
		return loDC + rand.Intn(hiDC-loDC+1)
	}
	if loDC < 1 {
		return 1
	}
	return loDC
}

// calcSkillPower — Delphi 技能伤害缩放:
// Ergum: nPower/(btTrainLv+2)*(btLevel+2) (ObjBase.pas:22175)
// Banwol/CRS: nPower/(btTrainLv+10)*(btLevel+2) (ObjBase.pas:22194)
func (p *PlayObject) calcSkillPower(baseDamage int, pm *PlayerMagic, divisor int) int {
	if pm == nil || baseDamage <= 0 {
		return baseDamage
	}
	trainLv := 3 // MagicDef.TrainLv 固定为 3（maxLevel）
	return baseDamage / (trainLv + divisor) * (pm.Level + 2)
}

// applySkillBonus — 根据 wHitMode 附加技能伤害 (ObjBase.pas:22119-22163)
func (p *PlayObject) applySkillBonus(damage int, ident int, fireHit bool) int {
	switch {
	case fireHit || ident == protocol.CMFireHit:
		// Delphi: nPower += Round(nPower / 100 * (m_nHitDouble * 10))
		damage += damage * p.HitDouble * 10 / 100
	case ident == protocol.CMTwinHit:
		damage += damage * p.HitDouble * 10 / 100
	case ident == protocol.CMPowerHit:
		// Delphi: nPower += m_nHitPlus
		damage += p.HitPlus
	}
	// HeavyHit/BigHit: 无伤害变化，仅动画不同
	return damage
}

// advancePowerHitCharge — Delphi PowerHit 自动蓄力 (ObjBase.pas:8862-8875)
func (p *PlayObject) advancePowerHitCharge(ident int, server *netserver.TCPServer) {
	pm := p.findMagic(7) // 攻杀剑术
	if pm == nil {
		return
	}
	if ident == protocol.CMPowerHit {
		if p.PowerHitReady {
			p.PowerHitReady = false
		}
		return
	}
	p.PowerHitCount--
	if p.PowerHitCount <= 0 {
		p.PowerHitReady = true
		maxCycle := 7 - pm.Level
		if maxCycle < 1 {
			maxCycle = 1
		}
		p.PowerHitCount = rand.Intn(maxCycle) + 1
		if server != nil {
			p.SendSpecialAttackFlags(server)
		}
	}
}

// consumeSkillMP — Delphi 半月/CRS 消耗 MP (ObjBase.pas:18788-18811)
func (p *PlayObject) consumeSkillMP(ident int, server *netserver.TCPServer) bool {
	var magID int
	switch ident {
	case protocol.CMWideHit:
		magID = 25
	case protocol.CMCrsHit:
		magID = 39
	default:
		return true
	}
	pm := p.findMagic(magID)
	if pm == nil {
		return false
	}
	var def *MagicDef
	if p.MagicDB != nil {
		def = p.MagicDB.GetByID(magID)
	}
	cost := 0
	if def != nil {
		cost = def.Spell / (3 + 1) * (pm.Level + 1)
	}
	if int(p.WAbil.MP) < cost {
		return false
	}
	p.WAbil.MP -= uint16(cost)
	p.sendHealthSpell(server)
	return true
}

func (p *PlayObject) findAttackTarget(x, y int) *BaseObject {
	if p.envir == nil {
		return nil
	}
	if x < 0 || x >= p.envir.Width || y < 0 || y >= p.envir.Height {
		return nil
	}
	idx := y*p.envir.Width + x
	for _, o := range p.envir.Cells[idx].ObjList {
		if o.Type != OS_MOVINGOBJECT {
			continue
		}
		switch obj := o.Obj.(type) {
		case *MonsterObject:
			if !obj.Death && !obj.Ghost {
				return obj.BaseObject
			}
		case *PlayObject:
			if obj.ID != p.ID && !obj.Death && !obj.Ghost {
				return obj.BaseObject
			}
		}
	}
	return nil
}

// calcDamage — Delphi GetAttackPower + GetHitStruckDamage (ObjBase.pas:2416, 22414)
// target 为 nil 时仅掷攻击骰（用于技能次级伤害计算）。
func (p *PlayObject) calcDamage(target *BaseObject) int {
	loDC := int(p.WAbil.DC & 0xFFFF)
	hiDC := int(p.WAbil.DC >> 16)

	attack := loDC
	if hiDC > loDC {
		attack = loDC + rand.Intn(hiDC-loDC+1)
	}

	// Delphi: Random(10 - MIN(9, luck)) == 0 → 最大伤害
	luck := p.Luck
	if luck > 9 {
		luck = 9
	}
	if luck > 0 {
		if rand.Intn(10-luck) == 0 {
			attack = hiDC
		}
	} else if luck < 0 {
		// Delphi: 负幸运 → Random(10 - MAX(0, -luck)) == 0 → 最小伤害
		unluck := -luck
		if unluck > 9 {
			unluck = 9
		}
		if rand.Intn(10-unluck) == 0 {
			attack = loDC
		}
	}

	// Delphi: 攻击力倍率 (ObjBase.pas:6882-6903, m_nPowerRate)
	if rate := p.PowerRate; rate > 0 && rate != 100 {
		attack = attack * rate / 100
		if attack < 1 {
			attack = 1
		}
	}

	if target == nil {
		if attack < 1 {
			attack = 1
		}
		return attack
	}

	// Delphi: nArmor = LoAC + Random(HiAC - LoAC + 1)
	loAC := int(target.WAbil.AC & 0xFFFF)
	hiAC := int(target.WAbil.AC >> 16)
	armor := loAC
	if hiAC > loAC {
		armor = loAC + rand.Intn(hiAC-loAC+1)
	}

	damage := attack - armor
	if damage < 1 {
		damage = 1
	}
	return damage
}

func (p *PlayObject) applyDamage(server *netserver.TCPServer, target *BaseObject, damage int, dir int) {
	// 安全区守卫不可被玩家伤害
	if mon := p.envir.getMonsterByBase(target); mon != nil && mon.IsSafeZoneGuard {
		return
	}

	// Delphi: 魔法盾 1.5x MP 消耗完全吸收 (ObjBase.pas:2455-2469)
	if tp := p.envir.getPlayerByBase(target); tp != nil && (tp.StatusTimeArr[STATE_BUBBLEDEFENCE] > 0 || tp.HasMagicShield) {
		mpCost := damage + damage/2 // 1.5x
		mp := int(tp.WAbil.MP)
		if mp >= mpCost {
			tp.WAbil.MP -= uint16(mpCost)
			damage = 0
		} else {
			absorbed := mp * 2 / 3 // 反推可吸收伤害
			tp.WAbil.MP = 0
			damage -= absorbed
			tp.StatusTimeArr[STATE_BUBBLEDEFENCE] = 0
		}
		if damage < 0 {
			damage = 0
		}
		tp.sendHealthSpell(server)
	}

	// Delphi: 红毒增幅最终伤害 (ObjBase.pas:22472): damage * (nPosionDamagarmor/10)
	if target.StatusTimeArr[POISON_DAMAGEARMOR] > 0 {
		damage = damage + damage/5 // 默认 +20%
	}

	// Delphi: 不死系易伤 (ObjBase.pas:22428-22431)
	if target.UndeadBonus > 0 {
		damage += target.UndeadBonus
	}

	hp := int(target.WAbil.HP)
	hp -= damage

	// Delphi: 麻痹戒指 Random(target.AntiPoison + nAttackPosionRate) == 0 (ObjBase.pas:22265)
	if p.HasParalysis && damage > 0 {
		antiPoison := target.AntiPoison
		if antiPoison < 0 {
			antiPoison = 0
		}
		if rand.Intn(antiPoison+5) == 0 {
			if tp := p.envir.getPlayerByBase(target); tp != nil {
				tp.MakePoison(POISON_STONE, 50, 0)
			} else if mon := p.envir.getMonsterByBase(target); mon != nil {
				mon.StatusTimeArr[POISON_STONE] = 50
			}
		}
	}

	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	p.envir.broadcastRefMsg(target, RM_STRUCK, target.ID, damage, target.CurrY, dir)

	// 攻击方武器磨损（Delphi: DoDamageWeapon, ObjBase.pas:18967）
	if damage > 0 {
		if wp := p.UseItems[protocol.UWeapon]; wp != nil && wp.Dura > 0 {
			wear := damage / 5
			if wear < 1 {
				wear = 1
			}
			if int(wp.Dura) <= wear {
				p.UseItems[protocol.UWeapon] = nil
				p.RecalcAbilitys()
				p.updateAppearance()
				p.SendUseItemsFull(server)
				log.Logf(log.LevelInfo, "Items", "%s weapon broke (Dura was %d)", p.Name, wp.Dura)
			} else {
				wp.Dura -= uint16(wear)
				p.sendDuraChange(server, wp)
			}
		}
	}

	if mon := p.envir.getMonsterByBase(target); mon != nil {
		mon.OnStruck(p.ID, time.Now().UnixMilli(), p.Engine)
	}

	// 被击方装备磨损与破碎（Delphi: StruckDamage, ObjBase.pas:22461）
	if tp := p.envir.getPlayerByBase(target); tp != nil {
		tp.LastHiterID = p.ID
		tp.LastHiterTick = time.Now().UnixMilli()
		tp.StruckTick = time.Now().UnixMilli()

		// Delphi: 被玩家击中标记正当防卫旗 (ObjBase.pas:21220-21236)
		tp.SetPKFlag(p)

		equipChanged := false
		if it := tp.UseItems[protocol.UDress]; it != nil && it.Dura > 0 {
			it.Dura--
			if it.Dura == 0 {
				tp.UseItems[protocol.UDress] = nil
				equipChanged = true
			} else {
				tp.sendDuraChange(server, it)
			}
		}
		for i := 1; i < 13; i++ {
			if it := tp.UseItems[i]; it != nil && it.Dura > 0 {
				if rand.Intn(8) == 0 {
					it.Dura--
					if it.Dura == 0 {
						tp.UseItems[i] = nil
						equipChanged = true
					} else {
						tp.sendDuraChange(server, it)
					}
				}
			}
		}
		if equipChanged {
			tp.RecalcAbilitys()
			tp.updateAppearance()
			tp.SendUseItemsFull(server)
		}
	}

	if hp <= 0 {
		if tp := p.envir.getPlayerByBase(target); tp != nil && tp.HasRevival {
			tp.HasRevival = false
			tp.WAbil.HP = tp.WAbil.MaxHP / 2
			tp.sendHealthSpell(server)
		} else {
			target.Death = true
			p.envir.broadcastDeathMsg(target, target.ID, target.CurrX, target.CurrY, target.Dir, true)

		if mon := p.envir.getMonsterByBase(target); mon != nil {
			mon.DeathTick = time.Now().UnixMilli()
			p.awardExp(server, mon)
		}
			if tp := p.envir.getPlayerByBase(target); tp != nil {
				tp.deathTick = time.Now().UnixMilli()
				tp.Death = true
				p.OnPlayerKilled(server, tp)
				tp.DropDeathItems(server)
			}
		}
	} else {
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			tp.sendHealthSpell(server)
		}
	}

	// Delphi: 虹魔套装吸血 (ObjBase.pas:22272-22281)
	if p.HongMoSuite > 0 && damage > 0 {
		heal := damage * p.HongMoSuite / 100
		if heal > 0 {
			hp := int(p.WAbil.HP) + heal
			if hp > int(p.WAbil.MaxHP) {
				hp = int(p.WAbil.MaxHP)
			}
			p.WAbil.HP = uint16(hp)
			p.sendHealthSpell(server)
		}
	}

	log.Logf(log.LevelInfo, "Combat", "%s attacked %s dealing %d damage (HP: %d/%d)",
		p.Name, target.Name, damage, hp, target.WAbil.MaxHP)
}

func (p *PlayObject) sendHitToClient(server *netserver.TCPServer, smIdent uint16, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendStruckToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMStruck, msg.SourceID, uint16(msg.Param1), uint16(msg.Param2), uint16(msg.Param3))
	// body[0:4] = 攻击者 ID, body[4:6] = 伤害值
	var hiterID int32
	if p.envir != nil {
		if obj := p.envir.getObjectByID(msg.SourceID); obj != nil {
			switch t := obj.(type) {
			case *PlayObject:
				hiterID = t.LastHiterID
			case *MonsterObject:
				hiterID = t.LastHiterID
			}
		}
	}
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(hiterID))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(msg.Param1))
	body := protocol.EncodeBuffer(buf)
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendDeathToClient(server *netserver.TCPServer, msg SendMessage) {
	// Param3==1 表示新鲜击杀 → SM_NOWDEATH（客户端播放死亡
	// 动画）；否则 SM_DEATH（直接显示尸体/骨架）。
	ident := uint16(protocol.SMDeath)
	if msg.Param3 == 1 {
		ident = protocol.SMNowDeath
	}
	resp := protocol.MakeDefaultMsg(ident, msg.SourceID, uint16(msg.Param1), uint16(msg.Param2), uint16(msg.Dir))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) awardExp(server *netserver.TCPServer, mon *MonsterObject) {
	exp := mon.Exp
	if exp <= 0 {
		exp = 10
	}

	// Delphi: 经验倍率 (ObjBase.pas:1828-1842)
	if rate := p.KillMonExpRate; rate > 0 && rate != 100 {
		exp = exp * rate / 100
		if exp < 1 {
			exp = 1
		}
	}

	if party := p.partyOf(); party != nil && len(party.Members) > 1 {
		var recipients []*PlayObject
		for _, id := range party.Members {
			member := p.Engine.GetPlayer(id)
			if member != nil && !member.Ghost && !member.Death && member.MapName == p.MapName {
				recipients = append(recipients, member)
			}
		}
		if len(recipients) > 1 {
			share := exp / len(recipients)
			if share < 1 {
				share = 1
			}
			for _, member := range recipients {
				member.addExp(server, share)
			}
		} else {
			p.addExp(server, exp)
		}
	} else {
		p.addExp(server, exp)
	}

	if len(p.SlaveIDs) > 0 {
		p.gainSlaveExp()
	}
}

func (p *PlayObject) addExp(server *netserver.TCPServer, exp int) {
	p.WAbil.Exp += uint32(exp)

	expMsg := protocol.MakeDefaultMsg(protocol.SMWinExp, int32(exp), 0, 0, 0)
	server.Send(p.Session.ID, expMsg, "")

	maxExp := p.GetMaxExp()
	leveledUp := false
	if p.WAbil.Exp >= maxExp {
		p.WAbil.Exp -= maxExp
		p.WAbil.Level++
		p.RecalcAbilitys()
		p.WAbil.HP = p.WAbil.MaxHP
		p.WAbil.MP = p.WAbil.MaxMP

		levelMsg := protocol.MakeDefaultMsg(protocol.SMLevelUp, int32(p.WAbil.Level), 0, 0, 0)
		server.Send(p.Session.ID, levelMsg, "")

		p.BonusPoint += 3

		log.Logf(log.LevelInfo, "Combat", "%s leveled up to %d", p.Name, p.WAbil.Level)
		leveledUp = true
	}
	p.SendAbility(server)
	p.sendHealthSpell(server)
	if leveledUp {
		p.sendWeightChanged(server)
	}
}

func (p *PlayObject) GetMaxExp() uint32 {
	level := int(p.WAbil.Level)
	if level <= 0 {
		level = 1
	}
	return uint32(level * level * 100)
}

func (p *PlayObject) sendHealthSpell(server *netserver.TCPServer) {
	// Recog=HP, Param=MaxHP, Tag=MP, Series=MaxMP（双端同步修改；
	// 旧的 HP<<16|MP 打包方式导致客户端把 MP 读成 HP）。
	resp := protocol.MakeDefaultMsg(protocol.SMHealthSpellChanged,
		int32(p.WAbil.HP), uint16(p.WAbil.MaxHP), uint16(p.WAbil.MP), uint16(p.WAbil.MaxMP))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) Regenerate(server *netserver.TCPServer, now int64) {
	if p.Death {
		return
	}
	if now-p.lastRegenTick < 10000 {
		return
	}
	p.lastRegenTick = now

	changed := false
	maxHP := int(p.WAbil.MaxHP)
	maxMP := int(p.WAbil.MaxMP)

	if int(p.WAbil.HP) < maxHP {
		regen := p.HealthRecover
		if regen < 1 {
			regen = 1
		}
		if base := maxHP / 20; base > regen {
			regen = base
		}
		hp := int(p.WAbil.HP) + regen
		if hp > maxHP {
			hp = maxHP
		}
		p.WAbil.HP = uint16(hp)
		changed = true
	}

	if int(p.WAbil.MP) < maxMP {
		regen := p.SpellRecover
		if regen < 1 {
			regen = 1
		}
		if base := maxMP / 15; base > regen {
			regen = base
		}
		mp := int(p.WAbil.MP) + regen
		if mp > maxMP {
			mp = maxMP
		}
		p.WAbil.MP = uint16(mp)
		changed = true
	}

	if changed {
		p.sendHealthSpell(server)
	}
}

// processIncHealth 处理渐进回复池（Delphi: Run 循环, ObjBase.pas:3782-3855）。
// 间隔 = 600 - min(400, Level*10) ms，每次回复 Level/10+5 点。
func (p *PlayObject) processIncHealth(server *netserver.TCPServer, now int64) {
	if p.IncHealth <= 0 && p.IncSpell <= 0 && p.IncHealing <= 0 {
		return
	}
	level := int(p.WAbil.Level)
	interval := int64(600 - level*10)
	if interval < 200 {
		interval = 200
	}
	if now-p.lastIncTick < interval {
		return
	}
	p.lastIncTick = now

	perTick := level/10 + 5
	changed := false

	if p.IncHealth > 0 && int(p.WAbil.HP) < int(p.WAbil.MaxHP) {
		heal := perTick
		if heal > p.IncHealth {
			heal = p.IncHealth
		}
		p.IncHealth -= heal
		hp := int(p.WAbil.HP) + heal
		if hp > int(p.WAbil.MaxHP) {
			hp = int(p.WAbil.MaxHP)
		}
		p.WAbil.HP = uint16(hp)
		changed = true
	} else if int(p.WAbil.HP) >= int(p.WAbil.MaxHP) {
		p.IncHealth = 0
	}

	if p.IncSpell > 0 && int(p.WAbil.MP) < int(p.WAbil.MaxMP) {
		heal := perTick
		if heal > p.IncSpell {
			heal = p.IncSpell
		}
		p.IncSpell -= heal
		mp := int(p.WAbil.MP) + heal
		if mp > int(p.WAbil.MaxMP) {
			mp = int(p.WAbil.MaxMP)
		}
		p.WAbil.MP = uint16(mp)
		changed = true
	} else if int(p.WAbil.MP) >= int(p.WAbil.MaxMP) {
		p.IncSpell = 0
	}

	if p.IncHealing > 0 && int(p.WAbil.HP) < int(p.WAbil.MaxHP) {
		heal := 5
		if heal > p.IncHealing {
			heal = p.IncHealing
		}
		p.IncHealing -= heal
		hp := int(p.WAbil.HP) + heal
		if hp > int(p.WAbil.MaxHP) {
			hp = int(p.WAbil.MaxHP)
		}
		p.WAbil.HP = uint16(hp)
		changed = true
	} else if int(p.WAbil.HP) >= int(p.WAbil.MaxHP) {
		p.IncHealing = 0
	}

	if changed {
		p.sendHealthSpell(server)
	}
}

func (p *PlayObject) DropDeathItems(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	if p.HasAngry {
		return
	}
	now := time.Now().UnixMilli()

	// Delphi DropUseItems (ObjBase.pas:15487): 红名 1/15，普通 1/30。
	equipRate := 30
	if p.PkPoint >= 200 {
		equipRate = 15
	}
	equipChanged := false
	for i := 0; i < 13; i++ {
		if p.UseItems[i] == nil {
			continue
		}
		if rand.Intn(equipRate) == 0 {
			p.dropItemToGround(p.UseItems[i], server, now)
			p.UseItems[i] = nil
			equipChanged = true
		}
	}

	var remaining []*protocol.UserItem
	for _, item := range p.ItemList {
		if rand.Intn(10) == 0 {
			p.dropItemToGround(item, server, now)
		} else {
			remaining = append(remaining, item)
		}
	}
	p.ItemList = remaining

	if equipChanged {
		p.RecalcAbilitys()
		p.updateAppearance()
		p.SendUseItemsFull(server)
	}
}

func (p *PlayObject) dropItemToGround(item *protocol.UserItem, server *netserver.TCPServer, now int64) {
	name := "Item"
	looks := 0
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
			looks = int(def.Looks)
		}
	}
	dropX := p.CurrX + rand.Intn(3) - 1
	dropY := p.CurrY + rand.Intn(3) - 1
	p.Engine.mu.Lock()
	id := p.Engine.nextItemID
	p.Engine.nextItemID++
	p.Engine.mu.Unlock()
	gi := &GroundItem{
		ID:       id,
		Name:     name,
		Looks:    looks,
		X:        dropX,
		Y:        dropY,
		DropTick: now,
		UserItem: item,
	}
	p.envir.AddGroundItem(gi)
	resp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), uint16(gi.Looks))
	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	for _, obj := range objs {
		if other, ok := obj.(*PlayObject); ok && !other.Ghost {
			server.Send(other.Session.ID, resp, protocol.EncodeString(gi.Name))
		}
	}
}

func (p *PlayObject) resurrect(server *netserver.TCPServer) {
	p.Death = false
	p.skeletonSent = false
	p.WAbil.HP = p.WAbil.MaxHP / 2
	p.WAbil.MP = p.WAbil.MaxMP / 2

	if p.envir != nil {
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	}
	safeMap, safeX, safeY := GetSafeZonePoint()
	if p.MapMgr != nil {
		if env := p.MapMgr.FindMap(safeMap); env != nil {
			p.envir = env
			p.MapName = safeMap
			p.CurrX = safeX
			p.CurrY = safeY
			env.AddObject(safeX, safeY, OS_MOVINGOBJECT, p)
		}
	}

	resp := protocol.MakeDefaultMsg(protocol.SMAlive, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, resp, "")

	p.SendRefMsg(RM_TURN, p.Dir, p.CurrX, p.CurrY, p.Name)
	p.sendHealthSpell(server)

	log.Logf(log.LevelInfo, "Combat", "%s resurrected at %s(%d,%d)", p.Name, p.MapName, p.CurrX, p.CurrY)
}

func IsSafeZone(envir *Environment, x, y int) bool {
	if envir == nil {
		return false
	}
	return CheckSafeZone(envir.Name, x, y)
}

func (p *PlayObject) HandlePickup(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	var item *GroundItem
	for dy := -1; dy <= 1 && item == nil; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if gi := p.envir.GetGroundItemAt(p.CurrX+dx, p.CurrY+dy); gi != nil {
				item = gi
				break
			}
		}
	}
	if item == nil {
		return
	}

	// 拾取保护：2 分钟内仅归属者/队友可拾取（Delphi: ClientPickUpItem, ObjBase.pas:1699）
	now := time.Now().UnixMilli()
	if item.OwnerID != 0 && item.OwnerID != p.ID && now-item.OwnerTick < pickupProtectMs {
		allowed := false
		if party := p.partyOf(); party != nil {
			for _, id := range party.Members {
				if id == item.OwnerID {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return
		}
	}

	if item.Gold > 0 {
		p.Gold += item.Gold
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		log.Logf(log.LevelInfo, "PlayObject", "%s picked up %d gold (total: %d)", p.Name, item.Gold, p.Gold)
	} else {
		added := false
		if item.UserItem != nil {
			if len(p.ItemList) < MaxBagItems {
				// 负重检查（Delphi: IsAddWeightAvailable, ObjBase.pas:2085）
				if p.ItemDB != nil {
					if def := p.ItemDB.GetByIdx(int(item.UserItem.WIndex)); def != nil {
						if int(p.WAbil.Weight)+int(def.Weight) > int(p.WAbil.MaxWeight) {
							p.sysMsg(server, "物品太重，无法携带更多")
							return
						}
					}
				}
				p.ItemList = append(p.ItemList, item.UserItem)
				added = true
			} else {
				p.sysMsg(server, "背包已满")
				return
			}
		} else if p.ItemDB != nil {
			if def := p.ItemDB.GetByName(item.Name); def != nil {
				added = p.GiveItem(def.Idx)
			}
		}
		if added {
			resp := protocol.MakeDefaultMsg(protocol.SMDropItemSuccess, 1, 0, 0, 0)
			server.Send(p.Session.ID, resp, protocol.EncodeString(item.Name))
			p.RecalcAbilitys()
			p.SendBagItemsFull(server)
			p.sendWeightChanged(server)
			log.Logf(log.LevelInfo, "PlayObject", "%s picked up %s", p.Name, item.Name)
		}
	}

	p.envir.RemoveGroundItem(item.ID)

	hideResp := protocol.MakeDefaultMsg(protocol.SMItemHide, item.ID, 0, 0, 0)
	objs := p.envir.GetRangeObjects(item.X, item.Y, viewRange)
	for _, obj := range objs {
		op, ok := obj.(*PlayObject)
		if !ok || op.Ghost {
			continue
		}
		server.Send(op.Session.ID, hideResp, "")
	}
}

func (p *PlayObject) sendMoveFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMoveFail, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMovementToClient(server *netserver.TCPServer, smIdent uint16, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	if obj == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, msg.SourceID, uint16(msg.Param2), uint16(msg.Param3), uint16(msg.Param1))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendTurnToClient(server *netserver.TCPServer, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMTurn, msg.SourceID, uint16(msg.Param2), uint16(msg.Param3), uint16(msg.Param1))
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
	if src.Name != "" {
		body += protocol.EncodeString(src.Name)
	}
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) sendDisappearToClient(server *netserver.TCPServer, msg SendMessage) {
	resp := protocol.MakeDefaultMsg(protocol.SMDisappear, msg.SourceID, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendFeatureChangedToClient(server *netserver.TCPServer, msg SendMessage) {
	if p.envir == nil {
		return
	}
	obj := p.envir.getObjectByID(msg.SourceID)
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMFeatureChanged, src.ID, 0, 0, 0)
	body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
	server.Send(p.Session.ID, resp, body)
}

func (p *PlayObject) encodeCharDesc(feature, featureEx int32) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(feature))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(featureEx))
	return buf
}

// FeatureEx 编码扩展外观：低字节=马类型，高字节=衣服特效。
func (p *PlayObject) FeatureEx() int32 {
	if p.OnHorse {
		return 1
	}
	return 0
}

// broadcastFeatureChanged 通知视野内玩家和自身客户端外观发生变化。
func (p *PlayObject) broadcastFeatureChanged(server *netserver.TCPServer) {
	p.SendRefMsg(RM_FEATURECHANGED, 0, 0, 0, "")
	resp := protocol.MakeDefaultMsg(protocol.SMFeatureChanged, p.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(p.encodeCharDesc(p.Feature(), p.FeatureEx())))
}

func (p *PlayObject) SearchViewRange(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	for _, entry := range p.VisibleActors {
		entry.Flag = 0
	}

	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
	for _, obj := range objs {
		var id int32
		var skip bool
		switch o := obj.(type) {
		case *PlayObject:
			if o.ID == p.ID || o.Ghost || o.Death || o.Hidden {
				skip = true
			} else {
				id = o.ID
			}
		case *MonsterObject:
			if o.Ghost || o.Death || o.Hidden {
				skip = true
			} else {
				id = o.ID
			}
		case *NpcObject:
			id = o.ID
		default:
			skip = true
		}
		if skip {
			continue
		}
		if entry, exists := p.VisibleActors[id]; exists {
			entry.Flag = 1
		} else {
			p.VisibleActors[id] = &VisibleEntry{ID: id, Flag: 2}
		}
	}

	for id, entry := range p.VisibleActors {
		switch entry.Flag {
		case 0:
			resp := protocol.MakeDefaultMsg(protocol.SMDisappear, id, 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			delete(p.VisibleActors, id)
		case 2:
			obj := p.envir.getObjectByID(id)
			base := objectBase(obj)
			if base == nil {
				delete(p.VisibleActors, id)
				continue
			}
			resp := protocol.MakeDefaultMsg(protocol.SMTurn, base.ID, uint16(base.CurrX), uint16(base.CurrY), uint16(base.Dir))
			body := protocol.EncodeBuffer(p.encodeCharDesc(objectFeature(obj), objectFeatureEx(obj)))
			if base.Name != "" {
				body += protocol.EncodeString(base.Name)
			}
			server.Send(p.Session.ID, resp, body)
		}
	}

	for _, item := range p.envir.GroundItems {
		dx := abs(item.X - p.CurrX)
		dy := abs(item.Y - p.CurrY)
		if dx <= viewRange && dy <= viewRange {
			if !p.knownItems[item.ID] {
				p.knownItems[item.ID] = true
				resp := protocol.MakeDefaultMsg(protocol.SMItemShow, item.ID, uint16(item.X), uint16(item.Y), uint16(item.Looks))
				server.Send(p.Session.ID, resp, protocol.EncodeString(item.Name))
			}
		} else {
			if p.knownItems[item.ID] {
				delete(p.knownItems, item.ID)
				resp := protocol.MakeDefaultMsg(protocol.SMItemHide, item.ID, 0, 0, 0)
				server.Send(p.Session.ID, resp, "")
			}
		}
	}
}

// HandleQueryUserState 回应查看请求（CMQueryUserState：
// Recog = 目标玩家 ID）：130 字节装备数据（13 × WIndex u16,
// Dura u16, DuraMax u16, MakeIndex u32），后跟玩家名。
func (p *PlayObject) HandleQueryUserState(msg SendMessage, server *netserver.TCPServer) {
	if p.Engine == nil {
		return
	}
	target := p.Engine.GetPlayer(int32(msg.Param1))
	if target == nil {
		return
	}
	buf := make([]byte, 130, 130+len(target.Name))
	for i := 0; i < 13; i++ {
		it := target.UseItems[i]
		if it == nil {
			continue
		}
		off := i * 10
		binary.LittleEndian.PutUint16(buf[off:off+2], it.WIndex)
		binary.LittleEndian.PutUint16(buf[off+2:off+4], it.Dura)
		binary.LittleEndian.PutUint16(buf[off+4:off+6], it.DuraMax)
		binary.LittleEndian.PutUint32(buf[off+6:off+10], uint32(it.MakeIndex))
	}
	buf = append(buf, []byte(target.Name)...)
	resp := protocol.MakeDefaultMsg(protocol.SMSendUserState, target.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeBuffer(buf))
}

func (p *PlayObject) SendMapInfo(server *netserver.TCPServer) {
	darkness := uint16(0)
	if p.envir != nil && p.envir.Flag.Dark {
		darkness = 1
	}
	mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, int32(p.CurrX), uint16(p.CurrY), darkness, 0)
	server.Send(p.Session.ID, mapResp, protocol.EncodeString(p.MapName))
}

func (p *PlayObject) SendLogon(server *netserver.TCPServer) {
	logonResp := protocol.MakeDefaultMsg(protocol.SMLogon, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.Dir))
	body := p.encodeLogonBody()
	server.Send(p.Session.ID, logonResp, body)
}

func (p *PlayObject) encodeLogonBody() string {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(p.Feature()))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(p.FeatureEx()))
	// 第三个槽位存放职业（0 战士 / 1 法师 / 2 道士）；feature
	// 不包含职业信息，客户端 HUD/职业界面需要它。
	binary.LittleEndian.PutUint32(buf[8:12], uint32(p.Job))
	binary.LittleEndian.PutUint32(buf[12:16], 0)
	return protocol.EncodeBuffer(buf)
}

// encodeAbilityBody 构建 SMAbility 消息体：固定 60 字节布局，
// 双端统一定义（Go 闭环）。客户端镜像见 GameState.ParseAbility。
func (p *PlayObject) encodeAbilityBody() string {
	buf := make([]byte, 0, 60)
	var tmp [4]byte
	putU16 := func(v uint16) {
		binary.LittleEndian.PutUint16(tmp[:2], v)
		buf = append(buf, tmp[:2]...)
	}
	putU32 := func(v uint32) {
		binary.LittleEndian.PutUint32(tmp[:4], v)
		buf = append(buf, tmp[:4]...)
	}
	putU16(p.WAbil.Level)
	putU32(p.WAbil.AC)
	putU32(p.WAbil.MAC)
	putU32(p.WAbil.DC)
	putU32(p.WAbil.MC)
	putU32(p.WAbil.SC)
	putU16(p.WAbil.HP)
	putU16(p.WAbil.MaxHP)
	putU16(p.WAbil.MP)
	putU16(p.WAbil.MaxMP)
	putU32(p.WAbil.Exp)
	putU32(p.GetMaxExp())
	putU16(p.WAbil.Weight)
	putU16(p.WAbil.MaxWeight)
	putU16(p.WAbil.WearWeight)
	putU16(p.WAbil.MaxWearWeight)
	putU16(p.WAbil.HandWeight)
	putU16(p.WAbil.MaxHandWeight)
	putU16(uint16(p.HitPoint))
	putU16(uint16(p.SpeedPoint))
	putU16(uint16(p.BonusPoint))
	putU32(uint32(p.Gold))
	return protocol.EncodeBuffer(buf)
}

// SendAbility 发送完整的属性数据块。消息头 Recog 保留等级供
// 旧客户端使用；完整数据在消息体中。
func (p *PlayObject) SendAbility(server *netserver.TCPServer) {
	abilResp := protocol.MakeDefaultMsg(protocol.SMAbility, int32(p.WAbil.Level), 0, 0, 0)
	server.Send(p.Session.ID, abilResp, p.encodeAbilityBody())
}

func (p *PlayObject) sendWeightChanged(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMWeightChanged,
		int32(p.WAbil.Weight), uint16(p.WAbil.MaxWeight), 0, 0)
	server.Send(p.Session.ID, resp, "")
}

// encodeStdItemsBody 序列化物品定义数据库。布局：count u16，
// 每个物品：Idx u16, Looks u16, StdMode/Shape/Weight/NeedLevel u8×4,
// AC/ACMax/MAC/MACMax/DC/DCMax/MC/MCMax/SC/SCMax u16×10, Price u32,
// NameLen u8 + Name (UTF-8)。客户端镜像见 GameState.ParseItemDefs。
func encodeStdItemsBody(items []ItemDef) string {
	buf := make([]byte, 2, 2+len(items)*40)
	binary.LittleEndian.PutUint16(buf, uint16(len(items)))
	var tmp [4]byte
	for i := range items {
		def := &items[i]
		binary.LittleEndian.PutUint16(tmp[:2], uint16(def.Idx))
		buf = append(buf, tmp[:2]...)
		binary.LittleEndian.PutUint16(tmp[:2], def.Looks)
		buf = append(buf, tmp[:2]...)
		buf = append(buf, def.StdMode, def.Shape, def.Weight, def.NeedLevel)
		for _, v := range []uint16{
			uint16(def.AC), uint16(def.ACMax), uint16(def.MAC), uint16(def.MACMax),
			uint16(def.DC), uint16(def.DCMax), uint16(def.MC), uint16(def.MCMax),
			uint16(def.SC), uint16(def.SCMax),
		} {
			binary.LittleEndian.PutUint16(tmp[:2], v)
			buf = append(buf, tmp[:2]...)
		}
		binary.LittleEndian.PutUint32(tmp[:4], def.Price)
		buf = append(buf, tmp[:4]...)
		name := []byte(def.Name)
		if len(name) > 255 {
			name = name[:255]
		}
		buf = append(buf, byte(len(name)))
		buf = append(buf, name...)
	}
	return protocol.EncodeBuffer(buf)
}

// SendStdItems 在登录时一次性下发完整的物品定义数据库。
func (p *PlayObject) SendStdItems(server *netserver.TCPServer) {
	if p.ItemDB == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMStdItems, int32(len(p.ItemDB.Items)), 0, 0, 0)
	server.Send(p.Session.ID, resp, encodeStdItemsBody(p.ItemDB.Items))
}

func (p *PlayObject) SendUseItems(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSendUseItems, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendDayChanging(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMDayChanging, 3, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendMapDescription(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMapDescription, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(p.MapName))
}

func (p *PlayObject) SendSubAbility(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMSubAbility, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) CheckMapRoute(server *netserver.TCPServer) {
	if p.MapMgr == nil {
		return
	}
	route := p.MapMgr.FindRoute(p.MapName, p.CurrX, p.CurrY)
	if route == nil {
		return
	}
	newEnvir := p.MapMgr.FindMap(route.DstMap)
	if newEnvir == nil {
		return
	}
	p.EnterAnotherMap(server, newEnvir, route.DstX, route.DstY)
}

func (p *PlayObject) EnterAnotherMap(server *netserver.TCPServer, newEnvir *Environment, newX, newY int) bool {
	p.Ghost = true
	p.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
	p.Ghost = false

	clearMsg := protocol.MakeDefaultMsg(protocol.SMClearObjects, 0, 0, 0, 0)
	server.Send(p.Session.ID, clearMsg, "")

	if p.envir != nil {
		p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	}

	p.envir = newEnvir
	p.MapName = newEnvir.Name
	p.CurrX = newX
	p.CurrY = newY
	p.EnterMapTick = time.Now().UnixMilli()

	newEnvir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)

	darkness := uint16(0)
	if newEnvir.Flag.Dark {
		darkness = 1
	}
	changeMsg := protocol.MakeDefaultMsg(protocol.SMChangeMap, p.ID, uint16(p.CurrX), uint16(p.CurrY), darkness)
	server.Send(p.Session.ID, changeMsg, protocol.EncodeString(p.MapName))

	p.VisibleActors = make(map[int32]*VisibleEntry)

	log.Logf(log.LevelInfo, "PlayObject", "%s entered map %s (position %d,%d)", p.Name, p.MapName, p.CurrX, p.CurrY)
	return true
}

const MaxSlaveCount = 2

func (p *PlayObject) addSlave(id int32) bool {
	p.cleanSlaveList()
	if len(p.SlaveIDs) >= MaxSlaveCount {
		return false
	}
	p.SlaveIDs = append(p.SlaveIDs, id)
	return true
}

func (p *PlayObject) removeSlave(id int32) {
	for i, sid := range p.SlaveIDs {
		if sid == id {
			p.SlaveIDs = append(p.SlaveIDs[:i], p.SlaveIDs[i+1:]...)
			return
		}
	}
}

func (p *PlayObject) cleanSlaveList() {
	if p.Engine == nil {
		return
	}
	var alive []int32
	for _, sid := range p.SlaveIDs {
		for _, mon := range p.Engine.Monsters {
			if mon.ID == sid && !mon.Death && mon.PlayerMasterID == p.ID {
				alive = append(alive, sid)
				break
			}
		}
	}
	p.SlaveIDs = alive
}

func (p *PlayObject) gainSlaveExp() {
	if p.SlaveLevel < 7 {
		p.SlaveLevel++
		for _, mon := range p.Engine.Monsters {
			if mon.PlayerMasterID == p.ID && !mon.Death {
				mon.WAbil.Level = p.WAbil.Level + uint16(p.SlaveLevel)
				bonus := p.SlaveLevel * 3
				mon.WAbil.DC += uint32(bonus)
				mon.WAbil.MaxHP += uint16(bonus * 10)
				if mon.WAbil.HP < mon.WAbil.MaxHP {
					mon.WAbil.HP = mon.WAbil.MaxHP
				}
			}
		}
	}
}

func (p *PlayObject) toggleSlaveRelax(server *netserver.TCPServer) {
	relax := true
	for _, mon := range p.Engine.Monsters {
		if mon.PlayerMasterID == p.ID && !mon.Death {
			relax = !mon.SlaveRelax
			break
		}
	}
	for _, mon := range p.Engine.Monsters {
		if mon.PlayerMasterID == p.ID && !mon.Death {
			mon.SlaveRelax = relax
		}
	}
	if relax {
		p.sysMsg(server, "宠物休息")
	} else {
		p.sysMsg(server, "宠物攻击")
	}
}

func (p *PlayObject) recallSlaves(server *netserver.TCPServer) {
	count := 0
	for _, mon := range p.Engine.Monsters {
		if mon.PlayerMasterID == p.ID && !mon.Death {
			mon.WAbil.HP = 0
			mon.Death = true
			mon.DeathTick = time.Now().UnixMilli()
			if mon.envir != nil {
				mon.envir.broadcastRefMsg(mon.BaseObject, RM_DEATH, mon.ID, mon.CurrX, mon.CurrY, mon.Dir)
			}
			count++
		}
	}
	if count > 0 {
		p.sysMsg(server, "宠物已召回")
	}
}

// isNearNpc 检查玩家是否在NPC附近（15格范围内）且在同一地图
// Delphi ObjBase.pas:9833 使用 15 格距离。
func (p *PlayObject) isNearNpc(npc *NpcObject) bool {
	if npc == nil || p.MapName != npc.MapName {
		return false
	}
	dx := p.CurrX - npc.CurrX
	dy := p.CurrY - npc.CurrY
	return dx >= -15 && dx <= 15 && dy >= -15 && dy <= 15
}
