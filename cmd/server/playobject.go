package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

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
	lastRegenTick  int64
	deathTick      int64
	skeletonSent   bool

	WalkTick       int64
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

	PkPoint         int
	LastPkDecayTick int64
	OnHorse         bool
	HorseType       byte
	Permission      byte
	ShutupTick      int64 // 禁言到期时间

	LastHiterID   int32 // 最后攻击者 ID（供奴隶目标选择）
	LastHiterTick int64

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

	SlaveIDs   []int32 // 当前宠物 ID 列表
	SlaveLevel int     // 宠物等级（1-7，Delphi m_btSlaveExpLevel）

	// E2: 任务位标志（3×1024 bits，Delphi m_QuestUnitOpen/m_QuestUnit/m_QuestFlag）
	QuestUnitOpen [128]byte
	QuestUnit     [128]byte
	QuestFlag     [128]byte

	ScriptVars  [10]int
	ScriptVarsD [10]int  // 动态变量 D0-D9
	ScriptVarsM [100]int // 持久变量 M0-M99

	StrScriptVars map[string]string
	nameLists     map[string][]string

	// NPC 脚本导航状态（Delphi m_nScriptGotoCount/m_sScriptGoBackLable/m_sScriptCurrLable）
	ScriptGotoCount int
	ScriptGoBackLabel string
	ScriptCurrLabel   string
	CurrentNpc        *NpcObject
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
	return &PlayObject{
		BaseObject:    base,
		Session:       session,
		AccountName:   account,
		VisibleActors: make(map[int32]*VisibleEntry),
		knownItems:    make(map[int32]bool),
		WalkSpeed:     1400,
		RunSpeed:      1400,
		WalkTick:      time.Now().UnixMilli(),
		StrScriptVars: make(map[string]string),
		nameLists:     make(map[string][]string),
	}
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

	if now-p.lastVisionTick >= 1000 {
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
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit, protocol.CMPowerHit, protocol.CMLongHit, protocol.CMWideHit, protocol.CMFireHit, protocol.CMTwinHit:
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
	p.TurnTo(dir)
	p.SendRefMsg(RM_TURN, dir, p.CurrX, p.CurrY, p.Name)
	server.SendRaw(p.Session.ID, "#+GOOD!")
}

func (p *PlayObject) HandleWalk(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		server.SendRaw(p.Session.ID, "#+FAIL!")
		return
	}
	now := time.Now().UnixMilli()
	interval := p.Engine.Config.GetWalkInterval()
	if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
		interval *= 2
	}
	if !p.checkMoveSpeed(now, interval, server) {
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
	if !p.checkMoveSpeed(now, interval, server) {
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
	if !p.checkMoveSpeed(now, interval, server) {
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

// checkMoveSpeed 校验移动间隔并记录超速违规。
// 返回 true 表示允许移动，false 表示拒绝（速度过快）。
func (p *PlayObject) checkMoveSpeed(now, interval int64, server *netserver.TCPServer) bool {
	// E5: 超速计数衰减 — 10秒无违规则 -1
	if p.OverSpeedCount > 0 && now-p.LastSpeedViolationTick > 10000 {
		p.OverSpeedCount--
		p.LastSpeedViolationTick = now
	}
	if now-p.WalkTick < interval {
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
	p.WalkTick = now
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
	case protocol.CMTwinHit:
		return 38, true // 狂风斩
	default:
		return 0, false
	}
}

func (p *PlayObject) HandleHit(msg SendMessage, server *netserver.TCPServer) {
	now := time.Now().UnixMilli()
	// E5: 攻击速度验证（配置驱动）
	hitInterval := p.Engine.Config.GetHitIntervalTime()
	if now-p.HitTick < hitInterval {
		p.OverSpeedCount++
		p.LastSpeedViolationTick = now
		return
	}

	if magID, ok := hitSkillMagID(msg.Ident); ok && p.findMagic(magID) == nil {
		msg.Ident = protocol.CMHit
	}

	// E3: 攻击时处理武器升级结果
	p.CheckWeaponUpgradeStatus(server)
	// 武器破碎后无法攻击
	if p.UseItems[protocol.UWeapon] == nil && msg.Ident != protocol.CMHit {
		return
	}

	switch msg.Ident {
	case protocol.CMFireHit:
		if now-p.FireHitTick < 10000 {
			return
		}
		p.FireHitTick = now
	case protocol.CMTwinHit:
		if now-p.TwinHitTick < 60000 {
			return
		}
		p.TwinHitTick = now
	}
	p.HitTick = now

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		return
	}
	p.Dir = dir

	rmIdent := RM_HIT
	switch msg.Ident {
	case protocol.CMHeavyHit:
		rmIdent = RM_HEAVYHIT
	case protocol.CMBigHit:
		rmIdent = RM_BIGHIT
	case protocol.CMPowerHit:
		rmIdent = RM_POWERHIT
	case protocol.CMLongHit:
		rmIdent = RM_LONGHIT
	case protocol.CMWideHit:
		rmIdent = RM_WIDEHIT
	case protocol.CMFireHit:
		rmIdent = RM_FIREHIT
	}
	p.SendRefMsg(rmIdent, dir, p.CurrX, p.CurrY, "")

	if p.envir == nil {
		return
	}

	if IsSafeZone(p.envir, p.CurrX, p.CurrY) {
		return
	}

	var multiplier float64
	switch msg.Ident {
	case protocol.CMHeavyHit:
		multiplier = 1.5
	case protocol.CMBigHit:
		multiplier = 2.0
	case protocol.CMPowerHit:
		multiplier = 1.3
	case protocol.CMLongHit:
		multiplier = 1.2
	case protocol.CMWideHit:
		multiplier = 1.1
	case protocol.CMFireHit:
		multiplier = 2.5
		if p.SkillPower > 0 {
			multiplier += float64(p.SkillPower) / 100.0
			p.SkillPower = 0
		}
	default:
		multiplier = 1.0
	}

	dx, dy := dirToOffset(dir)

	if msg.Ident == protocol.CMWideHit {
		targets := p.findWideTargets()
		for _, target := range targets {
			damage := p.calcDamage(target)
			damage = int(float64(damage) * multiplier)
			if damage < 1 {
				damage = 1
			}
			p.applyDamage(server, target, damage, dir)
		}
		return
	}

	target := p.findAttackTarget(p.CurrX+dx, p.CurrY+dy)
	if target == nil && msg.Ident == protocol.CMLongHit {
		target = p.findAttackTarget(p.CurrX+dx*2, p.CurrY+dy*2)
	}
	if target == nil {
		// 攻城战：攻击城门/城墙
		if p.Engine != nil && p.Engine.Castle != nil && p.envir != nil && p.envir.Castle != nil {
			attackX, attackY := p.CurrX+dx, p.CurrY+dy
			loDC := int(p.WAbil.DC & 0xFFFF)
			hiDC := int(p.WAbil.DC >> 16)
			damage := loDC
			if hiDC > loDC {
				damage = loDC + rand.Intn(hiDC-loDC+1)
			}
			damage = int(float64(damage) * multiplier)
			if damage < 1 {
				damage = 1
			}
			if p.Engine.Castle.HandleStructureDamage(attackX, attackY, damage) {
				p.SendRefMsg(RM_HIT, dir, attackX, attackY, "")
				return
			}
		}
		return
	}

	if IsSafeZone(p.envir, target.CurrX, target.CurrY) {
		return
	}

	damage := p.calcDamage(target)
	damage = int(float64(damage) * multiplier)
	if damage < 1 {
		damage = 1
	}
	p.applyDamage(server, target, damage, dir)
}

func (p *PlayObject) findWideTargets() []*BaseObject {
	var targets []*BaseObject
	if p.envir == nil {
		return targets
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			x, y := p.CurrX+dx, p.CurrY+dy
			t := p.findAttackTarget(x, y)
			if t != nil && !IsSafeZone(p.envir, t.CurrX, t.CurrY) {
				targets = append(targets, t)
			}
		}
	}
	return targets
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

func (p *PlayObject) calcDamage(target *BaseObject) int {
	loDC := int(p.WAbil.DC & 0xFFFF)
	hiDC := int(p.WAbil.DC >> 16)
	loAC := int(target.WAbil.AC & 0xFFFF)
	hiAC := int(target.WAbil.AC >> 16)

	attack := loDC
	if hiDC > loDC {
		attack = loDC + rand.Intn(hiDC-loDC+1)
	}
	if p.Luck > 0 && rand.Intn(100) < p.Luck {
		attack = hiDC
	}

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
	if tp := p.envir.getPlayerByBase(target); tp != nil && (tp.StatusTimeArr[STATE_BUBBLEDEFENCE] > 0 || tp.HasMagicShield) {
		absorbed := damage / 2
		mp := int(tp.WAbil.MP)
		if mp < absorbed {
			absorbed = mp
			tp.StatusTimeArr[STATE_BUBBLEDEFENCE] = 0
		}
		tp.WAbil.MP -= uint16(absorbed)
		damage -= absorbed
		if damage < 1 {
			damage = 1
		}
	}

	hp := int(target.WAbil.HP)
	hp -= damage

	if p.HasParalysis && rand.Intn(20) == 0 {
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			tp.MakePoison(POISON_STONE, 50)
		}
	}

	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	p.envir.broadcastRefMsg(target, RM_STRUCK, target.ID, target.CurrX, target.CurrY, dir)

	if mon := p.envir.getMonsterByBase(target); mon != nil {
		mon.OnStruck(p.ID, time.Now().UnixMilli(), p.Engine)
	}

	if tp := p.envir.getPlayerByBase(target); tp != nil {
		tp.LastHiterID = p.ID
		tp.LastHiterTick = time.Now().UnixMilli()
		if it := tp.UseItems[protocol.UDress]; it != nil && it.Dura > 0 {
			it.Dura--
			tp.sendDuraChange(server, it)
		}
		for i := 1; i < 13; i++ {
			if it := tp.UseItems[i]; it != nil && it.Dura > 0 {
				if rand.Intn(8) == 0 {
					it.Dura--
					tp.sendDuraChange(server, it)
				}
			}
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
	// body[0:4] = 攻击者 ID（客户端据此查找武器/种族以区分受击声）
	var body string
	if p.envir != nil {
		if obj := p.envir.getObjectByID(msg.SourceID); obj != nil {
			var hiterID int32
			switch t := obj.(type) {
			case *PlayObject:
				hiterID = t.LastHiterID
			case *MonsterObject:
				hiterID = t.LastHiterID
			}
			if hiterID != 0 {
				buf := make([]byte, 4)
				binary.LittleEndian.PutUint32(buf, uint32(hiterID))
				body = protocol.EncodeBuffer(buf)
			}
		}
	}
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
	p.WAbil.Exp += uint32(exp)

	if len(p.SlaveIDs) > 0 {
		p.gainSlaveExp()
	}

	expMsg := protocol.MakeDefaultMsg(protocol.SMWinExp, int32(exp), 0, 0, 0)
	server.Send(p.Session.ID, expMsg, "")

	maxExp := p.GetMaxExp()
	leveledUp := false
	if p.WAbil.Exp >= maxExp {
		p.WAbil.Exp -= maxExp
		p.WAbil.Level++
		// 属性增长现在由 RecalcAbilitys 中的职业公式计算。
		p.RecalcAbilitys()
		p.WAbil.HP = p.WAbil.MaxHP
		p.WAbil.MP = p.WAbil.MaxMP

		levelMsg := protocol.MakeDefaultMsg(protocol.SMLevelUp, int32(p.WAbil.Level), 0, 0, 0)
		server.Send(p.Session.ID, levelMsg, "")

		// 可分配的属性点（Delphi 中每级调用 GetBonusPoint）。
		p.BonusPoint += 3

		log.Logf(log.LevelInfo, "Combat", "%s leveled up to %d", p.Name, p.WAbil.Level)
		leveledUp = true
	}
	// 同步客户端的经验/负重/等级信息。
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
		regen := maxHP / 20
		if regen < 1 {
			regen = 1
		}
		hp := int(p.WAbil.HP) + regen
		if hp > maxHP {
			hp = maxHP
		}
		p.WAbil.HP = uint16(hp)
		changed = true
	}

	if int(p.WAbil.MP) < maxMP {
		regen := maxMP / 15
		if regen < 1 {
			regen = 1
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

func (p *PlayObject) DropDeathItems(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	if p.HasAngry {
		return
	}
	var remaining []*protocol.UserItem
	for _, item := range p.ItemList {
		if rand.Intn(10) == 0 {
			dropX := p.CurrX + rand.Intn(3) - 1
			dropY := p.CurrY + rand.Intn(3) - 1
			p.Engine.mu.Lock()
			id := p.Engine.nextItemID
			p.Engine.nextItemID++
			p.Engine.mu.Unlock()
			gi := &GroundItem{
				ID:       id,
				Name:     fmt.Sprintf("Item#%d", item.WIndex),
				Looks:    0,
				X:        dropX,
				Y:        dropY,
				DropTick: time.Now().UnixMilli(),
			}
			p.envir.AddGroundItem(gi)
			resp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), uint16(gi.Looks))
			objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, viewRange)
			for _, obj := range objs {
				if other, ok := obj.(*PlayObject); ok && !other.Ghost {
					server.Send(other.Session.ID, resp, protocol.EncodeString(gi.Name))
				}
			}
		} else {
			remaining = append(remaining, item)
		}
	}
	p.ItemList = remaining
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

	if item.Gold > 0 {
		p.Gold += item.Gold
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		log.Logf(log.LevelInfo, "PlayObject", "%s picked up %d gold (total: %d)", p.Name, item.Gold, p.Gold)
	} else {
		added := false
		if p.ItemDB != nil {
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
	src := objectBase(obj)
	if src == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
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
	resp := protocol.MakeDefaultMsg(protocol.SMTurn, src.ID, uint16(src.CurrX), uint16(src.CurrY), uint16(src.Dir))
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
	p.SlaveIDs = nil
	p.sysMsg(server, "召回了 "+strconv.Itoa(count)+" 个宠物")
}
