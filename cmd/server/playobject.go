package main

import (
	"encoding/binary"
	"math/rand"
	"strconv"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// sendCtrl 发送控制消息（+GOOD/+FAIL），附带时间戳（Delphi ObjBase.pas:4730,4737）。
// 技能标记（+PWR/+LNG 等）不附时间戳，直接使用 SendRaw。
func sendCtrl(server *netserver.TCPServer, sessionID int64, tag string) {
	server.SendRaw(sessionID, "#+"+tag+"/"+strconv.FormatInt(time.Now().UnixMilli(), 10)+"!")
}

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

	// Delphi m_dwMoveTick/m_dwMoveCount（ObjBase.pas:617）：
	// walk/run/horserun 共享同一个移动节拍；MoveCount 是连续超速修正计数。
	MoveTick  int64
	MoveCount int
	TurnTick  int64
	SpellTick      int64
	DigUpTick      int64
	WalkSpeed      int64
	RunSpeed       int64
	OverSpeedCount int
	LastSpeedViolationTick int64 // 上次超速违规时间（用于衰减）
	LastActionTick int64         // Delphi m_dwActionTick：动作转换间隔锚点（ObjBase.pas:25246）
	OldIdent       int           // Delphi m_wOldIdent：上一动作（归一化后，ObjBase.pas:25352）
	OldDir         int           // Delphi m_btOldDir：上一动作方向（ObjBase.pas:25353）

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
	// lastDealTick 交易/丢弃共用节流（Delphi m_DealLastTick）：
	// 丢弃需间隔 3000ms（ObjBase.pas:16244）；DealEnd 双方最近
	// 交易动作 <1000ms 判连点作弊（ObjBase.pas:17844-17849）。
	lastDealTick int64
	tryDealTick  int64 // DealTry 冷却（dwTryDealTime=3000ms）
	lastLampTick int64 // 光源耐久 tick（500ms，Delphi dwDecLightItemDrugTime）
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
	// 特效码扩展（Delphi ObjBase.pas:2960-3060）。
	HasUnParalysis   bool // 139 麻痹免疫
	HasSuperman      bool // 140 超人（Delphi 设置后无消费点，仅标记）
	HasUnMagicShield bool // 143 攻击无视对方魔法盾
	HasUnRevival     bool // 144 攻击禁止对方复活戒指
	HasGuildMove     bool // 145 行会传送（待行会传送功能实装）
	HasNoDropItem    bool // 171 死亡不掉背包
	HasNoDropUseItem bool // 172 死亡不掉装备

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

	// CHECKITEM/CHECKDURA 等条件命中的物品实例（Delphi 脚本执行局部
	// 变量 UserItem，ObjNpc.pas:7074-7098），供 TAKECHECKITEM 收取。
	CheckedItemMakeIndex int32

	// PARAM1-4 暂存参数（Delphi nSC_PARAM1..4 局部变量，
	// ObjNpc.pas:7808-7827）。
	ScriptParamN [4]int
	ScriptParamS [4]string

	CreditPoint    int    // 声望点（Delphi m_nCreditPoint）
	ReNewLevel     int    // 转生等级（Delphi m_nReNewLevel）
	StoragePassword string // 仓库密码
	AutoGetExp     int    // 自动获取经验点数

	// 解锁药效果（StdMode 0 Shape 2，Delphi m_boUserUnLockDurg，
	// ObjBase.pas:23344-23348）：绕过 Reserved&2 与首饰封印的禁脱。
	UserUnLockDurg bool
	HungerStatus   int   // 饥饿度（Delphi m_nHungerStatus，上限 5000）
	StoragePwdLocked bool // 仓库密码锁定（错 >3 次，Delphi m_boPasswordLocked）
	storagePwdFail   int  // 连续输错次数（Delphi m_btPwdFailCount）

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
		MoveTick:       time.Now().UnixMilli(),
		visionInterval: 2000 + rand.Int63n(2000),
		StrScriptVars:  make(map[string]string),
		nameLists:      make(map[string][]string),
	}
	base.outer = p
	return p
}

func (p *PlayObject) Operate(server *netserver.TCPServer) {
	// Delphi m_boGhost 守卫：下线/退出后不再消费消息（ObjBase.pas SendMsg/GetMessage
	// 均有此检查），避免残留的延迟移动消息把已移除对象插回地图格子。
	if p.Ghost {
		return
	}
	for {
		msg, ok := p.GetMsg()
		if !ok {
			break
		}
		p.ProcessMessage(msg, server)
		if p.Ghost {
			return
		}
	}

	now := time.Now().UnixMilli()

	p.ProcessStatusEffects(server, now)
	p.DecayPkPoint(now)
	p.CheckPKStatus(now)
	p.processPendingMagics(server, now)
	p.processLampDura(server, now)

	if p.Death {
		cfg := p.Engine.Config
		if p.deathTick > 0 && now-p.deathTick > cfg.GetDeathSkeletonDelay() && !p.skeletonSent {
			p.skeletonSent = true
			// 非新鲜死亡：SM_DEATH 直接显示尸体/骨架。
			p.envir.broadcastDeathMsg(p.BaseObject, p.ID, p.CurrX, p.CurrY, p.Dir, false)
		}
		if now-p.deathTick > cfg.GetAutoReviveDelay() {
			p.resurrect(server)
		}
		return
	}

	p.Regenerate(server, now)
	p.processIncHealth(server, now)

	// Delphi: FireHit 20s 过期 (ObjBase.pas:6427)
	if p.FireHitActive && now-p.FireHitActivateTick > p.Engine.Config.GetFireHitWindow() {
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
		// Delphi: NORECALL 地图禁止召回传送
		if recallAllowed(p.envir) && p.MapMgr != nil {
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
		case protocol.CMSay, protocol.CMQueryBagItems, protocol.CMTakeOnItem, protocol.CMTakeOffItem, protocol.CMWantMinimap, protocol.CMSitdown:
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
	case protocol.CMQueryUsername:
		p.HandleQueryUsername(msg, server)
	case protocol.CMSitdown:
		p.HandleSitDown(msg, server)
	case protocol.CMUserGetDetailItem:
		p.HandleGetDetailItem(msg, server)
	case protocol.CMWantMinimap:
		p.HandleWantMinimap(server)
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
	case RM_CHARSTATUSCHANGED:
		// 客户端重组 State = Param<<16 | Tag（main.go SMCharStatusChanged）
		statusMsg := protocol.MakeDefaultMsg(protocol.SMCharStatusChanged, msg.SourceID, uint16(msg.Param1), uint16(msg.Param2), 0)
		server.Send(p.Session.ID, statusMsg, "")
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
	p.recordAction(protocol.CMTurn, dir)
	sendCtrl(server, p.Session.ID, "GOOD")
}

// HandleSitDown — Delphi ClientSitDownHit（ObjBase.pas:17024-17042）：
// 死亡/石化禁止（+FAIL），TurnInterval 限速，广播 RM_POWERHIT 后 +GOOD。
// 客户端屠宰（Alt+左键）也随 CMButch 发送本消息播放下蹲动作。
func (p *PlayObject) HandleSitDown(msg SendMessage, server *netserver.TCPServer) {
	if p.Death || p.StatusTimeArr[POISON_STONE] > 0 {
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	now := time.Now().UnixMilli()
	if !p.checkActionSpeed(now, p.Engine.Config.GetTurnInterval(), &p.TurnTick, server) {
		return
	}
	if dir := msg.Param1; dir >= 0 && dir <= 7 {
		p.Dir = dir
	}
	p.SendRefMsg(RM_POWERHIT, p.Dir, p.CurrX, p.CurrY, "")
	sendCtrl(server, p.Session.ID, "GOOD")
}

func (p *PlayObject) HandleWalk(msg SendMessage, server *netserver.TCPServer) {
	// Delphi ClientWalkXY（ObjBase.pas:9593-9594）：麻痹/石化无论是否延迟消息都拦截。
	if !p.CanMoveCheck() {
		log.Logf(log.LevelDebug, "Move", "%s walk rejected: status effect (posion/stone) at (%d,%d)", p.Name, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	if !msg.LateDelivery {
		now := time.Now().UnixMilli()
		// Delphi: 受击硬直 → 延迟重投（CheckActionStatus, ObjBase.pas:25234-25240）
		if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
			left := p.Engine.Config.GetStruckTime() - (now - p.StruckTick)
			log.Logf(log.LevelDebug, "Move", "%s walk delayed: struck cooldown (%dms left) at (%d,%d)", p.Name, left, p.CurrX, p.CurrY)
			p.delayMoveOrFail(protocol.CMWalk, msg, server, left)
			return
		}
		interval := p.Engine.Config.GetWalkInterval()
		if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
			interval *= 2
		}
		// Delphi: 步频过快 → 延迟重投而非 +FAIL（ObjBase.pas:9604-9626）
		if !p.checkMoveInterval(now, interval, protocol.CMWalk, msg, server) {
			return
		}
	} else {
		// 延迟消息到期执行：跳过硬直/间隔复查（ObjBase.pas:9596），
		// 但入队后位置若已改变（传送等）则丢弃并纠偏。
		if msg.Param2 != p.CurrX || msg.Param3 != p.CurrY {
			log.Logf(log.LevelDebug, "Move", "%s delayed walk dropped: position changed to (%d,%d)", p.Name, p.CurrX, p.CurrY)
			p.sendMoveFail(server)
			return
		}
	}
	p.MoveTick = time.Now().UnixMilli() // Delphi ObjBase.pas:9637（含 WalkTo 失败也消耗间隔）

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		log.Logf(log.LevelDebug, "Move", "%s walk rejected: invalid dir=%d at (%d,%d)", p.Name, dir, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
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
			sendCtrl(server, p.Session.ID, "GOOD")
			p.recordAction(protocol.CMWalk, dir)
			p.CheckMapRoute(server)
			return
		}
	}
	if p.WalkTo(dir) {
		p.SendRefMsg(RM_WALK, dir, p.CurrX, p.CurrY, "")
		sendCtrl(server, p.Session.ID, "GOOD")
		p.recordAction(protocol.CMWalk, dir)
		p.CheckMapRoute(server)
	} else {
		p.MoveCount = 0 // Delphi ObjBase.pas:9696
		dx, dy := dirToOffset(dir)
		log.Logf(log.LevelDebug, "Move", "%s walk rejected: blocked dir=%d target=(%d,%d) from (%d,%d)", p.Name, dir, p.CurrX+dx, p.CurrY+dy, p.CurrX, p.CurrY)
		p.sendMoveFail(server)
	}
}

func (p *PlayObject) HandleRun(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		log.Logf(log.LevelDebug, "Move", "%s run rejected: status effect (poison/stone) at (%d,%d)", p.Name, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	// F10: 仅步行模式（延迟消息同样拦截）
	if p.Engine.Config.Game.WalkOnly && p.Permission < 10 {
		log.Logf(log.LevelDebug, "Move", "%s run rejected: walk-only mode at (%d,%d)", p.Name, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	if !msg.LateDelivery {
		now := time.Now().UnixMilli()
		// Delphi: 受击硬直 → 延迟重投（CheckActionStatus, ObjBase.pas:25234-25240）
		if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
			left := p.Engine.Config.GetStruckTime() - (now - p.StruckTick)
			log.Logf(log.LevelDebug, "Move", "%s run delayed: struck cooldown (%dms left) at (%d,%d)", p.Name, left, p.CurrX, p.CurrY)
			p.delayMoveOrFail(protocol.CMRun, msg, server, left)
			return
		}
		interval := p.Engine.Config.GetRunInterval()
		if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
			interval *= 2
		}
		// Delphi: 步频过快 → 延迟重投（ClientRunXY, ObjBase.pas:9521-9543）
		if !p.checkMoveInterval(now, interval, protocol.CMRun, msg, server) {
			return
		}
	} else {
		if msg.Param2 != p.CurrX || msg.Param3 != p.CurrY {
			log.Logf(log.LevelDebug, "Move", "%s delayed run dropped: position changed to (%d,%d)", p.Name, p.CurrX, p.CurrY)
			p.sendMoveFail(server)
			return
		}
	}
	p.MoveTick = time.Now().UnixMilli() // Delphi ObjBase.pas:9554

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		log.Logf(log.LevelDebug, "Move", "%s run rejected: invalid dir=%d at (%d,%d)", p.Name, dir, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
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
			log.Logf(log.LevelDebug, "Move", "%s run rejected: blocked dir=%d path=(%d,%d)->(%d,%d) from (%d,%d)", p.Name, dir, x1, y1, x2, y2, p.CurrX, p.CurrY)
			p.sendMoveFail(server)
			return
		}
	}
	p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.CurrX, p.CurrY = x2, y2
	p.Dir = dir
	p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.SendRefMsg(RM_RUN, dir, p.CurrX, p.CurrY, "")
	sendCtrl(server, p.Session.ID, "GOOD")
	p.recordAction(protocol.CMRun, dir)
	p.CheckMapRoute(server)
}

func (p *PlayObject) HandleHorseRun(msg SendMessage, server *netserver.TCPServer) {
	if !p.CanMoveCheck() {
		log.Logf(log.LevelDebug, "Move", "%s horserun rejected: status effect at (%d,%d)", p.Name, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	// F10: 仅步行模式（延迟消息同样拦截）
	if p.Engine.Config.Game.WalkOnly && p.Permission < 10 {
		log.Logf(log.LevelDebug, "Move", "%s horserun rejected: walk-only mode at (%d,%d)", p.Name, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	if !msg.LateDelivery {
		now := time.Now().UnixMilli()
		// Delphi: 受击硬直 → 延迟重投（CheckActionStatus, ObjBase.pas:25234-25240）
		if now-p.StruckTick < p.Engine.Config.GetStruckTime() {
			left := p.Engine.Config.GetStruckTime() - (now - p.StruckTick)
			log.Logf(log.LevelDebug, "Move", "%s horserun delayed: struck cooldown (%dms left) at (%d,%d)", p.Name, left, p.CurrX, p.CurrY)
			p.delayMoveOrFail(protocol.CMHorseRun, msg, server, left)
			return
		}
		interval := p.Engine.Config.GetRunInterval()
		if p.WAbil.Weight > p.WAbil.MaxWeight && p.WAbil.MaxWeight > 0 {
			interval *= 2
		}
		// Delphi: 步频过快 → 延迟重投（ClientHorseRunXY, ObjBase.pas:8914-8936）
		if !p.checkMoveInterval(now, interval, protocol.CMHorseRun, msg, server) {
			return
		}
	} else {
		if msg.Param2 != p.CurrX || msg.Param3 != p.CurrY {
			log.Logf(log.LevelDebug, "Move", "%s delayed horserun dropped: position changed to (%d,%d)", p.Name, p.CurrX, p.CurrY)
			p.sendMoveFail(server)
			return
		}
	}
	p.MoveTick = time.Now().UnixMilli() // Delphi ObjBase.pas:8939

	dir := msg.Param1
	if dir < 0 || dir > 7 {
		log.Logf(log.LevelDebug, "Move", "%s horserun rejected: invalid dir=%d at (%d,%d)", p.Name, dir, p.CurrX, p.CurrY)
		sendCtrl(server, p.Session.ID, "FAIL")
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
			log.Logf(log.LevelDebug, "Move", "%s horserun rejected: blocked dir=%d path=(%d,%d)->(%d,%d)->(%d,%d) from (%d,%d)", p.Name, dir, x1, y1, x2, y2, x3, y3, p.CurrX, p.CurrY)
			p.sendMoveFail(server)
			return
		}
	}
	p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.CurrX, p.CurrY = x3, y3
	p.Dir = dir
	p.envir.AddObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
	p.SendRefMsg(RM_HORSERUN, dir, p.CurrX, p.CurrY, "")
	sendCtrl(server, p.Session.ID, "GOOD")
	p.recordAction(protocol.CMRun, dir)
	p.CheckMapRoute(server)
}

func (p *PlayObject) runIgnoreEntities() bool {
	cfg := p.Engine.Config
	return cfg.Game.DisableRun || (p.Permission > 9 && cfg.Game.GMRunAll)
}

// checkMoveInterval Delphi 移动间隔检查（ObjBase.pas:9604-9626）：
// 步频过快时不拒绝，而是算出剩余延迟走延迟重投；返回 true 表示可立即执行。
func (p *PlayObject) checkMoveInterval(now, interval int64, cmIdent int, msg SendMessage, server *netserver.TCPServer) bool {
	elapsed := now - p.MoveTick
	if elapsed >= interval {
		return true
	}
	p.MoveCount++
	delay := interval - elapsed
	if delay > interval/3 {
		if p.MoveCount >= 4 {
			// 安全阀：连续小幅超速后重置节拍，把延迟钳到 interval/3
			// （ObjBase.pas:9611-9617）。
			p.MoveTick = now
			p.MoveCount = 0
			delay = interval / 3
		} else {
			p.MoveCount = 0
		}
	}
	log.Logf(log.LevelDebug, "Move", "%s move delayed: speed limit (%dms left, interval=%dms) at (%d,%d)", p.Name, delay, interval, p.CurrX, p.CurrY)
	p.delayMoveOrFail(cmIdent, msg, server, delay)
	return false
}

// delayMoveOrFail Delphi 移动消息延迟重投（ObjBase.pas:4967-5011）：
// 队列中同类消息未积压时重新入队延迟执行（不回复客户端，到期后执行并发 +GOOD）；
// 已积压则回 +FAIL 并计超速。Go 移动包只带方向，重投时附带当前坐标供到期校验。
func (p *PlayObject) delayMoveOrFail(cmIdent int, msg SendMessage, server *netserver.TCPServer, delay int64) {
	if delay <= 0 {
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	now := time.Now().UnixMilli()
	// 超速计数衰减（与 checkActionSpeed 一致）
	if p.OverSpeedCount > 0 && now-p.LastSpeedViolationTick > p.Engine.Config.GetSpeedDecayInterval() {
		p.OverSpeedCount--
		p.LastSpeedViolationTick = now
	}
	// Delphi nMaxWalkMsgCount/nMaxRunMsgCount 默认均为 1：
	// 队列里已有一条同类移动消息就不再积压（ObjBase.pas:4974-4992）。
	if p.QueuedMsgCount(cmIdent) >= 1 {
		p.OverSpeedCount++
		p.LastSpeedViolationTick = now
		cfg := p.Engine.Config
		if p.OverSpeedCount > cfg.GetSpeedHackMax() && cfg.Game.SpeedHackKick {
			log.Logf(log.LevelWarn, "Server", "speed-hack kick: %s (count=%d)", p.Name, p.OverSpeedCount)
			p.kickOutOfGame(server)
			return
		}
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}
	p.SendDelayMsg(cmIdent, msg.Param1, p.CurrX, p.CurrY, "", delay)
}

// kickOutOfGame — Delphi m_boKickFlag 路径（ObjBase.pas:6584-6587）：
// 先发 SM_OUTOFCONNECTION 通知客户端，再断开连接。
func (p *PlayObject) kickOutOfGame(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMOutOfConnection, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
	server.CloseSession(p.Session.ID)
}

// checkActionSpeed 校验动作间隔并记录超速违规。
// 返回 true 表示允许动作，false 表示拒绝（速度过快）。
func (p *PlayObject) checkActionSpeed(now, interval int64, tick *int64, server *netserver.TCPServer) bool {
	// E5: 超速计数衰减 — 10秒无违规则 -1
	if p.OverSpeedCount > 0 && now-p.LastSpeedViolationTick > p.Engine.Config.GetSpeedDecayInterval() {
		p.OverSpeedCount--
		p.LastSpeedViolationTick = now
	}
	if now-*tick < interval {
		p.OverSpeedCount++
		p.LastSpeedViolationTick = now
		cfg := p.Engine.Config
		if p.OverSpeedCount > cfg.GetSpeedHackMax() && cfg.Game.SpeedHackKick {
			log.Logf(log.LevelWarn, "Server", "speed-hack kick: %s (count=%d)", p.Name, p.OverSpeedCount)
			p.kickOutOfGame(server)
			return false
		}
		sendCtrl(server, p.Session.ID, "FAIL")
		return false
	}
	*tick = now
	return true
}

// normalizeActionIdent — Delphi ObjBase.pas:25330-25340：
// 普攻变体（重击/攻杀/半月/烈火等）统一按 CM_HIT 记录，供动作转换判定。
func normalizeActionIdent(ident int) int {
	switch ident {
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit,
		protocol.CMPowerHit, protocol.CMWideHit, protocol.CMFireHit:
		return protocol.CMHit
	}
	return ident
}

// checkActionTransition — Delphi CheckActionStatus（ObjBase.pas:25226-25365）：
// 不同动作之间的转换间隔。连续相同动作不受限（ObjBase.pas:25253-25258）；
// 动作不同且方向改变时按四组合取间隔：跑位刺杀 RunLongHit / 跑位普攻 RunHit /
// 走位普攻 WalkHit / 跑位魔法 RunMagic（ObjBase.pas:25266-25327），
// 否则用基础 ActionInterval。无论通过与否都记录本次动作（ObjBase.pas:25352-25353）。
func (p *PlayObject) checkActionTransition(now int64, ident, dir int, server *netserver.TCPServer) bool {
	ident = normalizeActionIdent(ident)
	if ident == p.OldIdent {
		return true
	}
	cfg := p.Engine.Config
	interval := cfg.GetActionInterval()
	if dir != p.OldDir {
		switch ident {
		case protocol.CMLongHit:
			if p.OldIdent == protocol.CMRun {
				interval = cfg.GetRunLongHitInterval()
			}
		case protocol.CMHit:
			switch p.OldIdent {
			case protocol.CMWalk:
				interval = cfg.GetWalkHitInterval()
			case protocol.CMRun:
				interval = cfg.GetRunHitInterval()
			}
		case protocol.CMSpell:
			if p.OldIdent == protocol.CMRun {
				interval = cfg.GetRunMagicInterval()
			}
		}
	}
	ok := p.checkActionSpeed(now, interval, &p.LastActionTick, server)
	p.OldIdent = ident
	p.OldDir = dir
	return ok
}

// recordAction — 移动/转身成功后记录动作（Delphi m_wOldIdent/m_btOldDir,
// ObjBase.pas:25352-25353），供后续攻击/施法的转换间隔判定。
func (p *PlayObject) recordAction(ident, dir int) {
	p.OldIdent = normalizeActionIdent(ident)
	p.OldDir = dir
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
		sendCtrl(server, p.Session.ID, "FAIL")
		return
	}

	// Delphi: 动作转换间隔（跑砍/走砍/跑位刺杀四组合，ObjBase.pas:25246-25327）
	if !p.checkActionTransition(now, msg.Ident, msg.Param1, server) {
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
		if now-p.FireHitTick < p.Engine.Config.GetFireHitCooldown() {
			sendCtrl(server, p.Session.ID, "FAIL")
			return
		}
		p.FireHitActive = true
		p.FireHitActivateTick = now
		p.FireHitTick = now
		p.SendSpecialAttackFlags(server)
		sendCtrl(server, p.Session.ID, "GOOD")
		return
	}
	// Delphi: TwinHit 同理 (ObjBase.pas:9797)
	if msg.Ident == protocol.CMTwinHit {
		if now-p.TwinHitTick < p.Engine.Config.GetTwinHitCooldown() {
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

	// Delphi: CM_HEAVYHIT + 鹤嘴锄(Shape=19) + 前方格不可通行 → 挖矿
	// (ObjBase.pas:8823-8856)；挖矿失败（无矿脉）则回退普通重击。
	if msg.Ident == protocol.CMHeavyHit {
		if weapon := p.UseItems[protocol.UWeapon]; weapon != nil && weapon.Dura > 0 && p.ItemDB != nil {
			if def := p.ItemDB.GetByIdx(int(weapon.WIndex)); def != nil && def.Shape == 19 {
				fdx, fdy := dirToOffset(dir)
				fx, fy := p.CurrX+fdx, p.CurrY+fdy
				if !p.envir.CanWalk(fx, fy) && p.tryMineAt(server, fx, fy) {
					server.SendRaw(p.Session.ID, "#=DIG!")
					sendCtrl(server, p.Session.ID, "GOOD")
					return
				}
			}
		}
	}

	// Delphi: PowerHit 自动蓄力 (ObjBase.pas:8862-8875)
	p.advancePowerHitCharge(msg.Ident, server)

	// Delphi: FireHit 激活窗口内自动触发 (ObjBase.pas:6427)
	fireHitTriggered := false
	if p.FireHitActive && now-p.FireHitActivateTick < p.Engine.Config.GetFireHitWindow() {
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
	sendCtrl(server, p.Session.ID, "GOOD")

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
	cfg := p.Engine.Config
	// 安全区守卫不可被玩家伤害
	if mon := p.envir.getMonsterByBase(target); mon != nil && mon.IsSafeZoneGuard {
		return
	}
	// Delphi TTrainer（ObjNpc.pas:2628-2676）：训练师为无敌沙袋，仅累计伤害统计
	if mon := p.envir.getMonsterByBase(target); mon != nil && isTrainer(mon) {
		mon.addTrainingDamage(server, p.ID, damage)
		return
	}
	// Delphi TSoccerBall（ObjMon2.pas:303-310）：m_boSuperMan 无敌，被击不扣血只触发踢滚
	if mon := p.envir.getMonsterByBase(target); mon != nil && mon.AIBehavior == AISoccerBall {
		mon.OnStruck(p.ID, time.Now().UnixMilli(), p.Engine)
		return
	}

	// Delphi: 魔法盾 1.5x MP 消耗完全吸收 (ObjBase.pas:2455-2469)。
	// 攻击者佩戴禁魔盾（143）时无视对方魔法盾（ObjBase.pas:2455
	// m_LastHiter.m_boUnMagicShield 判定）。
	if tp := p.envir.getPlayerByBase(target); tp != nil && !p.HasUnMagicShield && (tp.StatusTimeArr[STATE_BUBBLEDEFENCE] > 0 || tp.HasMagicShield) {
		mpCost := damage + damage*cfg.GetMagicShieldRatio()/100
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
		damage = damage + damage/cfg.GetRedPoisonBonus()
	}

	// Delphi: 不死系易伤 (ObjBase.pas:22428-22431)
	if target.UndeadBonus > 0 {
		damage += target.UndeadBonus
	}

	hp := int(target.WAbil.HP)
	hp -= damage

	// Delphi: 麻痹戒指 Random(target.AntiPoison + nAttackPosionRate) == 0 (ObjBase.pas:22265)。
	// 目标佩戴防麻痹（139）时免疫（Magic.pas:1341 m_boUnParalysis）。
	if p.HasParalysis && damage > 0 {
		targetUnPara := false
		if tp := p.envir.getPlayerByBase(target); tp != nil {
			targetUnPara = tp.HasUnParalysis
		}
		if !targetUnPara {
			antiPoison := target.AntiPoison
			if antiPoison < 0 {
				antiPoison = 0
			}
			if rand.Intn(antiPoison+cfg.GetParalysisDenom()) == 0 {
				paraDuration := int16(cfg.GetParalysisDuration())
				if tp := p.envir.getPlayerByBase(target); tp != nil {
					tp.MakePoison(POISON_STONE, paraDuration, 0)
				} else if mon := p.envir.getMonsterByBase(target); mon != nil {
					mon.StatusTimeArr[POISON_STONE] = paraDuration
				}
			}
		}
	}

	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	// Delphi 仅当伤害>0 才下发 SM_STRUCK（ObjBase.pas:5470）；
	// 魔法盾完全吸收（damage=0）时客户端不应播放受击动画。
	if damage > 0 {
		p.envir.broadcastRefMsg(target, RM_STRUCK, target.ID, damage, target.CurrY, dir)
	}

	// 攻击方武器磨损（Delphi ObjBase.pas:22255 + DoDamageWeapon:18967）：
	// 损量 = Random(5)+2 − 自身衣服 Source（btWeaponStrong），≤0 不磨损。
	if damage > 0 {
		if wp := p.UseItems[protocol.UWeapon]; wp != nil && wp.Dura > 0 {
			wear := rand.Intn(5) + 2
			if dress := p.UseItems[protocol.UDress]; dress != nil && p.ItemDB != nil {
				if def := p.ItemDB.GetByIdx(int(dress.WIndex)); def != nil && def.Source > 0 {
					wear -= int(def.Source)
				}
			}
			if wear > 0 {
				oldUnit := int(wp.Dura) / 1000
				if int(wp.Dura) <= wear {
					p.UseItems[protocol.UWeapon] = nil
					p.RecalcAbilitys()
					p.updateAppearance()
					p.SendUseItemsFull(server)
					log.Logf(log.LevelInfo, "Items", "%s weapon broke (Dura was %d)", p.Name, wp.Dura)
				} else {
					wp.Dura -= uint16(wear)
					// 显示粒度（Dura/1000）变化才下发（Delphi 18999-19007）。
					if int(wp.Dura)/1000 != oldUnit {
						p.sendDuraChange(server, wp)
					}
				}
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

		// 基础损量 5..14；红毒（POISON_DAMAGEARMOR）×1.2
		//（Delphi StruckDamage，ObjBase.pas:22471-22476，nPosionDamagarmor=12）。
		nDam := rand.Intn(10) + 5
		if tp.StatusTimeArr[POISON_DAMAGEARMOR] > 0 {
			nDam = nDam * 12 / 10
		}
		equipChanged := false
		// 衣服每击必掉（ObjBase.pas:22478-22516）。
		if tp.damageSlotDura(server, protocol.UDress, nDam) {
			equipChanged = true
		}
		// 全部槽位（含衣服）各 1/8 概率再判定一次
		//（ObjBase.pas:22517-22560：Low..High 无衣服豁免）。
		for i := 0; i < 13; i++ {
			if tp.UseItems[i] != nil && tp.UseItems[i].Dura > 0 && rand.Intn(8) == 0 {
				if tp.damageSlotDura(server, i, nDam) {
					equipChanged = true
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
		// 攻击者佩戴禁复活（144）时对方复活戒指不生效。
		if tp := p.envir.getPlayerByBase(target); tp != nil && tp.HasRevival && !p.HasUnRevival {
			tp.HasRevival = false
			tp.WAbil.HP = tp.WAbil.MaxHP / uint16(cfg.GetReviveHPRatio())
			tp.damageRevivalRings(server)
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
				// 被玩家击杀：默认不掉装备（Delphi boKillByHumanDropUseItem=False）。
				tp.DropDeathItems(server, false)
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
		exp = p.Engine.Config.GetFallbackExp()
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
	// Delphi: 地图 EXPRATE(n) 经验倍率（MapFlag.nEXPRATE）
	if p.envir != nil && p.envir.Flag.ExpRate > 1 {
		exp *= p.envir.Flag.ExpRate
	}
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

		p.BonusPoint += p.Engine.Config.GetBonusPerLevel()

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
	cfg := p.Engine.Config
	if now-p.lastRegenTick < cfg.GetHPRegenInterval() {
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
		if base := maxHP / cfg.GetHPRegenRatio(); base > regen {
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
		if base := maxMP / cfg.GetMPRegenRatio(); base > regen {
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

// processLampDura 右手光源耐久消耗（Delphi UseLamp，ObjBase.pas:15825-15871）：
// 每 500ms 检查一次；仅 StdItem.Source=0 的物品掉耐久（Delphi 中
// 火把类 Source≠0 跳过，实际只有蜡烛类消耗）；归零销毁并下发 655。
func (p *PlayObject) processLampDura(server *netserver.TCPServer, now int64) {
	if p.Death || p.ItemDB == nil {
		return
	}
	if now-p.lastLampTick < 500 {
		return
	}
	p.lastLampTick = now
	item := p.UseItems[protocol.URightHand]
	if item == nil || item.Dura == 0 {
		return
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil || def.Source != 0 {
		return
	}
	oldUnit := int(item.Dura) / 1000
	item.Dura--
	destroyed := item.Dura == 0
	if destroyed {
		p.UseItems[protocol.URightHand] = nil
		p.RecalcAbilitys()
		p.updateAppearance()
		p.SendUseItemsFull(server)
	} else if int(item.Dura)/1000 == oldUnit {
		return // 显示粒度（Dura/1000）未变化不下发
	}
	resp := protocol.MakeDefaultMsg(protocol.SMLampChangeDura, int32(item.Dura), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

// damageRevivalRings 复活戒指触发掉耐久（Delphi ItemDamageRevivalRing，
// ObjBase.pas:3625-3670）：Shape∈{114,160,161,162} 的物品（或武器/
// 右手 AniCount 命中）各扣 1000 耐久，归零销毁。
func (p *PlayObject) damageRevivalRings(server *netserver.TCPServer) {
	if p.ItemDB == nil {
		return
	}
	changed := false
	for i := 0; i < 13; i++ {
		it := p.UseItems[i]
		if it == nil || it.Dura == 0 {
			continue
		}
		def := p.ItemDB.GetByIdx(int(it.WIndex))
		if def == nil {
			continue
		}
		isRevival := false
		switch def.Shape {
		case 114, 160, 161, 162:
			isRevival = true
		}
		if !isRevival && (i == protocol.UWeapon || i == protocol.URightHand) {
			switch def.AniCount {
			case 114, 160, 161, 162:
				isRevival = true
			}
		}
		if !isRevival {
			continue
		}
		oldUnit := int(it.Dura) / 1000
		if int(it.Dura) <= 1000 {
			p.UseItems[i] = nil
			changed = true
		} else {
			it.Dura -= 1000
			if int(it.Dura)/1000 != oldUnit {
				p.sendDuraChange(server, it)
			}
		}
	}
	if changed {
		p.RecalcAbilitys()
		p.updateAppearance()
		p.SendUseItemsFull(server)
	}
}

// damageSlotDura 槽位装备扣耐久，返回物品是否破碎
//（Delphi StruckDamage 内循环，ObjBase.pas:22478-22560）。
func (p *PlayObject) damageSlotDura(server *netserver.TCPServer, slot, amount int) bool {
	it := p.UseItems[slot]
	if it == nil || it.Dura == 0 {
		return false
	}
	oldUnit := int(it.Dura) / 1000
	if int(it.Dura) <= amount {
		p.UseItems[slot] = nil
		return true
	}
	it.Dura -= uint16(amount)
	if int(it.Dura)/1000 != oldUnit {
		p.sendDuraChange(server, it)
	}
	return false
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

// DropDeathItems 死亡掉落（Delphi Die，ObjBase.pas:20983-21015）：
// killedByMonster=false（被玩家杀）默认不掉装备
//（boKillByHumanDropUseItem=False，M2Share.pas:2019-2026）；
// 被怪物杀才掉（boKillByMonstDropUseItem=True）。
// 背包：非红名每件 1/nDieScatterBagRate，红名全掉。金币不掉
//（boDieDropGold=False）。FIGHT/FIGHT3 格斗区死亡不掉。
func (p *PlayObject) DropDeathItems(server *netserver.TCPServer, killedByMonster bool) {
	if p.envir == nil || p.HasAngry {
		return
	}
	if p.envir.Flag.Fight || p.envir.Flag.Fight3 {
		return
	}
	now := time.Now().UnixMilli()
	cfg := p.Engine.Config

	// 特效码 171/172：死亡不掉背包/装备（Delphi m_boNoDropItem/
	// m_boNoDropUseItem，ObjBase.pas:20690/20611）。
	if killedByMonster && !p.HasNoDropUseItem {
		equipRate := cfg.GetDropEquipRate()
		if p.PKLevel() > 2 {
			equipRate = cfg.GetDropEquipRateRed()
		}
		equipChanged := false
		// Delphi DropUseItems（ObjBase.pas:20689-20763）：只扫槽 0..8，
		// U_BUJUK 与腰带不参与死亡掉落。
		for i := 0; i <= 8 && i < len(p.UseItems); i++ {
			item := p.UseItems[i]
			if item == nil {
				continue
			}
			var def *ItemDef
			if p.ItemDB != nil {
				def = p.ItemDB.GetByIdx(int(item.WIndex))
			}
			// Reserved&8：死亡直接销毁。
			if def != nil && def.Reserved&8 != 0 {
				p.UseItems[i] = nil
				equipChanged = true
				continue
			}
			// Reserved&10：死亡保护，落地前拦截（修正 Delphi 原版
			// "地上有副本+身上仍装备"的复制 bug）。
			if def != nil && def.Reserved&10 != 0 {
				continue
			}
			// 禁脱列表物品不参与死亡掉落（M2Share.pas:4642-4660）。
			if def != nil && p.ItemDB.InDisableTakeOffList(item.WIndex) {
				continue
			}
			if rand.Intn(equipRate) == 0 {
				p.dropItemToGround(item, server, now)
				p.UseItems[i] = nil
				equipChanged = true
			}
		}
		if equipChanged {
			p.RecalcAbilitys()
			p.updateAppearance()
			p.SendUseItemsFull(server)
		}
	}

	// 背包：Delphi ScatterBagItems（ObjBase.pas:26648-26698）——
	// 红名（PKLevel>=2 且 boDieRedScatterBagAll）全掉，否则每件 1/3；
	// 特效码 171（m_boNoDropItem）或地图 NODROPITEM 不掉背包。
	if p.HasNoDropItem || p.envir.Flag.NoDropItem {
		return
	}
	dropAll := p.PKLevel() >= 2
	var remaining []*protocol.UserItem
	for _, item := range p.ItemList {
		var def *ItemDef
		if p.ItemDB != nil {
			def = p.ItemDB.GetByIdx(int(item.WIndex))
		}
		if def != nil && def.Reserved&8 != 0 {
			continue // 死亡销毁
		}
		if def != nil && def.Reserved&10 != 0 {
			remaining = append(remaining, item)
			continue // 死亡保护
		}
		if dropAll || rand.Intn(cfg.GetDropBagRate()) == 0 {
			p.dropItemToGround(item, server, now)
		} else {
			remaining = append(remaining, item)
		}
	}
	p.ItemList = remaining
	if len(remaining) == 0 {
		p.ItemList = nil
	}
}

func (p *PlayObject) dropItemToGround(item *protocol.UserItem, server *netserver.TCPServer, now int64) {
	name := "Item"
	looks := 0
	if p.ItemDB != nil {
		if def := p.ItemDB.GetByIdx(int(item.WIndex)); def != nil {
			name = def.Name
			looks = int(def.Looks)
			// Delphi DropItemDown（ObjBase.pas:1597-1603）：肉落地扣 2000 耐久。
			if def.StdMode == 40 {
				if int(item.Dura) > 2000 {
					item.Dura -= 2000
				} else {
					item.Dura = 0
				}
			}
			// 矿石随机外观（ObjBase.pas:1608-1611 GetRandomLook；
			// Delphi 源码 StdMode=45，GEEM2 库矿石为 43，两者均适用）。
			if (def.StdMode == 43 || def.StdMode == 45) && def.Shape > 0 {
				looks += rand.Intn(int(def.Shape))
			}
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
	// 每格满 5 件拒绝落地（Delphi 同样丢失该物品）。
	if p.envir.AddGroundItem(gi) == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(protocol.SMItemShow, gi.ID, uint16(gi.X), uint16(gi.Y), uint16(gi.Looks))
	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, p.ViewRange)
	for _, obj := range objs {
		if other, ok := obj.(*PlayObject); ok && !other.Ghost {
			server.Send(other.Session.ID, resp, protocol.EncodeString(gi.Name))
		}
	}
}

func (p *PlayObject) resurrect(server *netserver.TCPServer) {
	p.Death = false
	p.skeletonSent = false
	p.WAbil.HP = p.WAbil.MaxHP / uint16(p.Engine.Config.GetReviveHPRatio())
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
	// Delphi: 地图级 SAFE 标志全图为安全区（ObjBase.pas:4473,21533 消费 boSAFE）
	if envir.Flag.Safe {
		return true
	}
	return CheckSafeZone(envir.Name, x, y)
}

// recallAllowed Delphi: NORECALL 地图禁止作为召回来源（MapFlag.boNORECALL）。
func recallAllowed(env *Environment) bool {
	return env == nil || !env.Flag.NoRecall
}

// incGold 增加金币（Delphi IncGold，ObjBase.pas:1978-1987）：
// 超过上限整体拒收（无部分收取），返回是否成功。
func (p *PlayObject) incGold(amount int) bool {
	if amount <= 0 {
		return true
	}
	maxGold := 10000000
	if p.Engine != nil {
		maxGold = p.Engine.Config.GetMaxGold()
	}
	if p.Gold+amount > maxGold {
		return false
	}
	p.Gold += amount
	return true
}

func (p *PlayObject) HandlePickup(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	// Delphi（ObjBase.pas:1695）：交易中禁止拾取。
	if p.Deal != nil {
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
		// 超上限整堆拒收不拆分（Delphi ObjBase.pas:1712/1727-1728）。
		if !p.incGold(item.Gold) {
			p.sysMsg(server, "金币已达到上限")
			return
		}
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		log.Logf(log.LevelInfo, "PlayObject", "%s picked up %d gold (total: %d)", p.Name, item.Gold, p.Gold)
	} else {
		added := false
		if item.UserItem != nil {
			if len(p.ItemList) < p.Engine.Config.GetMaxBagSlots() {
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
	objs := p.envir.GetRangeObjects(item.X, item.Y, p.ViewRange)
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

	objs := p.envir.GetRangeObjects(p.CurrX, p.CurrY, p.ViewRange)
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
		if dx <= p.ViewRange && dy <= p.ViewRange {
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

// HandleQueryUsername — Delphi ClientQueryUserName（ObjBase.pas:2638-2652）：
// Param1=目标ID Param2=x Param3=y。目标位于 (x,y) 3×3 范围内则回
// SM_USERNAME（名字+名字颜色），否则回 SM_GHOST（CretInNearXY，ObjBase.pas:16854）。
func (p *PlayObject) HandleQueryUsername(msg SendMessage, server *netserver.TCPServer) {
	targetID := int32(msg.Param1)
	x, y := msg.Param2, msg.Param3
	var target interface{}
	if p.envir != nil {
		for _, obj := range p.envir.GetRangeObjects(x, y, 1) {
			var base *BaseObject
			switch t := obj.(type) {
			case *PlayObject:
				base = t.BaseObject
			case *MonsterObject:
				base = t.BaseObject
			case *NpcObject:
				base = t.BaseObject
			default:
				continue
			}
			if base != nil && base.ID == targetID {
				target = obj
				break
			}
		}
	}
	if target == nil {
		resp := protocol.MakeDefaultMsg(protocol.SMGhost, targetID, uint16(x), uint16(y), 0)
		server.Send(p.Session.ID, resp, "")
		return
	}
	color := 255
	var name string
	switch t := target.(type) {
	case *PlayObject:
		color = t.NameColor()
		name = t.Name
	case *MonsterObject:
		name = t.Name
		// Delphi GetShowName（ObjBase.pas:2654-2663）：召唤物附带主人名
		if t.PlayerMasterID != 0 && p.Engine != nil {
			if master := p.Engine.GetPlayer(t.PlayerMasterID); master != nil {
				name += "(" + master.Name + ")"
			}
		}
	case *NpcObject:
		name = t.Name
	}
	resp := protocol.MakeDefaultMsg(protocol.SMUsername, targetID, uint16(color), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(name))
}

// HandleWantMinimap 响应 CM_WANTMINIMAP：下发当前地图的小地图图像号。
// Delphi: TPlayObject.ClientGetMinMap（ObjBase.pas:17985-17998）——
// 索引放在 Param 字段（SendDefMessage(SM_READMINIMAP_OK, 0, nMinMap, 0, 0, '')）。
func (p *PlayObject) HandleWantMinimap(server *netserver.TCPServer) {
	n := 0
	if p.envir != nil {
		n = p.envir.MinMap
	}
	if n > 0 {
		server.Send(p.Session.ID, protocol.MakeDefaultMsg(protocol.SMReadMinimapOK, 0, uint16(n), 0, 0), "")
	} else {
		server.Send(p.Session.ID, protocol.MakeDefaultMsg(protocol.SMReadMinimapFail, 0, 0, 0, 0), "")
	}
}

func (p *PlayObject) SendMapInfo(server *netserver.TCPServer) {
	mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, int32(p.CurrX), uint16(p.CurrY), uint16(p.dayBright()), 0)
	server.Send(p.Session.ID, mapResp, protocol.EncodeString(p.MapName))
}

// dayBright Delphi TPlayObject.DayBright（ObjBase.pas:4283-4296）：
// 返回 0=明亮 1=黑暗 2=中等。DARK 地图恒暗，DAYLIGHT 地图恒亮；
// Go 无全局昼夜（Delphi m_btBright 时间系统），非 DARK 地图默认白天。
func (p *PlayObject) dayBright() int {
	if p.envir != nil && p.envir.Flag.Dark {
		return 1
	}
	return 0
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
	// HitSpeed（Delphi m_nHitSpeed，装备攻速修正，可为负）追加在 offset 60。
	// 旧客户端只读前 60 字节，向后兼容；新客户端 ParseAbility 按需读取。
	putU16(uint16(int16(p.HitSpeed)))
	// offset 62 起：抗性/恢复五属性（Delphi SM_SUBABILITY 承载，
	// Go 闭环并入 SMAbility body；客户端镜像见 GameState.ParseAbility）。
	putU16(uint16(p.AntiMagic))
	putU16(uint16(p.AntiPoison))
	putU16(uint16(p.PoisonRecover))
	putU16(uint16(p.HealthRecover))
	putU16(uint16(p.SpellRecover))
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
// Source u16(int16 位模式), Reserved/Need/AniCount u8×3,
// NameLen u8 + Name (UTF-8)。客户端镜像见 GameState.ParseItemDefs。
func encodeStdItemsBody(items []ItemDef) string {
	buf := make([]byte, 2, 2+len(items)*45)
	binary.LittleEndian.PutUint16(buf, uint16(len(items)))
	var tmp [4]byte
	for i := range items {
		def := &items[i]
		binary.LittleEndian.PutUint16(tmp[:2], uint16(def.Idx))
		buf = append(buf, tmp[:2]...)
		binary.LittleEndian.PutUint16(tmp[:2], def.Looks)
		buf = append(buf, tmp[:2]...)
		// NeedLevel 线上仅传低字节（Go 物品库线格式；打包双条件分支
		// 10-13/40-44 等仅服务端穿装备校验使用，客户端只显示等级需求）。
		buf = append(buf, def.StdMode, def.Shape, def.Weight, byte(def.NeedLevel))
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
		// tooltip 需要 Source（强度/神圣/幸运诅咒显示）、Reserved
		//（(*) 前缀）与 Need（需求行）。
		binary.LittleEndian.PutUint16(tmp[:2], uint16(def.Source))
		buf = append(buf, tmp[:2]...)
		buf = append(buf, byte(def.Reserved), def.Need, def.AniCount)
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
	// 客户端 dayBrightToDarkness 用"亮度"语义（0=最暗, 3=最亮），
	// 由 Delphi DayBright（0=亮, 1=暗, 2=中）换算而来。
	bright := 3
	switch p.dayBright() {
	case 1:
		bright = 0
	case 2:
		bright = 1
	}
	resp := protocol.MakeDefaultMsg(protocol.SMDayChanging, int32(bright), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) SendMapDescription(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMapDescription, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(p.MapName))
}

// SendAreaState — Delphi RefUserState（ObjBase.pas:4467-4476）：
// bit1=FIGHT 格斗区、bit2=SAFE 安全图、bit4=自由 PK 区（攻城战期间的城堡图）。
func (p *PlayObject) SendAreaState(server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	state := 0
	if p.envir.Flag.Fight || p.envir.Flag.Fight3 {
		state |= 1
	}
	if p.envir.Flag.Safe {
		state |= 2
	}
	if p.Engine != nil && p.Engine.Castle != nil && p.Engine.Castle.IsAtWar() &&
		p.MapName == p.Engine.Castle.Config.MapName {
		state |= 4
	}
	resp := protocol.MakeDefaultMsg(protocol.SMAreaState, int32(state), 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
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

	// 丢弃旧地图上残留的（含延迟重投的）移动指令，避免在新地图多走一步。
	p.ClearQueuedMsgs(protocol.CMWalk, protocol.CMRun, protocol.CMHorseRun)

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

	changeMsg := protocol.MakeDefaultMsg(protocol.SMChangeMap, p.ID, uint16(p.CurrX), uint16(p.CurrY), uint16(p.dayBright()))
	server.Send(p.Session.ID, changeMsg, protocol.EncodeString(p.MapName))

	// Delphi: 切图后刷新区域状态位（ObjBase.pas:5617-5625 SM_NEWMAP 序列含 RefUserState）
	p.SendAreaState(server)

	p.VisibleActors = make(map[int32]*VisibleEntry)

	log.Logf(log.LevelInfo, "PlayObject", "%s entered map %s (position %d,%d)", p.Name, p.MapName, p.CurrX, p.CurrY)
	return true
}

const MaxSlaveCount = 2

func (p *PlayObject) addSlave(id int32) bool {
	p.cleanSlaveList()
	if len(p.SlaveIDs) >= p.Engine.Config.GetMaxSlaves() {
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
	if p.SlaveLevel < p.Engine.Config.GetMaxSlaveLevel() {
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
	dist := p.Engine.Config.GetNPCInteractDist()
	dx := p.CurrX - npc.CurrX
	dy := p.CurrY - npc.CurrY
	return dx >= -dist && dx <= dist && dy >= -dist && dy <= dist
}
