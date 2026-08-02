package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	cullMargin      = 3
	frontCullMargin = 35 // Delphi LONGHEIGHT_IMAGE 常量（PlayScn.pas:17）
)

var debugRenderFrame int

type GroundItemInfo struct {
	ID    int32
	X, Y  int
	Looks int
	Name  string
}

type FloatingText struct {
	Text      string
	X, Y      float32
	Color     [4]float32
	StartTime int64
}

type ChatMessage struct {
	Text string
	Time int64
}

type PlayScene struct {
	gl            *engine.GLState
	resources     *engine.ResourceManager
	mapDir        string
	cam           *engine.Camera2D
	mapData       *mapformat.MapData
	minimap       *Minimap
	minimapDirty  bool
	lighting      *LightingSystem
	lightingDirty bool

	texCache       map[int]uint32
	smTexCache     map[int]uint32
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int
	renderFrame int

	State                 *GameState
	sendMove              func(ident int, dir int)
	sendOpenDoor          func(doorID, x, y int)
	sendAttack            func(ident int, dir int)
	sendPickup            func()
	sendButch             func(targetID int32)
	sendChat              func(text string)
	sendSpell             func(magID int, x, y int)
	sendNpcClick          func(npcID int)
	sendDealCancel        func()
	sendUseItem           func(makeIndex int32) // CMEat，通过 MakeIndex 定位物品
	sendBuyItem           func(itemIdx int)
	sendSellItem          func(makeIndex int32)              // CMUserSellItem，通过 MakeIndex 定位
	sendDropItem          func(makeIndex int32)              // CMDropItem，通过 MakeIndex 定位
	sendDropGold          func(amount int)                   // CMDropGold，数量放 Recog 字段
	sendDealTry           func()                             // CMDealTry
	sendTakeOn            func(makeIndex int32, slot int)    // CMTakeOnItem
	sendTakeOff           func(slot int)                     // CMTakeOffItem
	sendMagicKey          func(magID, key int)               // CMMagicKeyChange
	sendMerchantSelect    func(npcID int32, tag string)      // CMMerchantDlgSelect
	sendQueryPrice        func(makeIndex int32)              // CMMerchantQuerySellPrice
	sendQueryRepair       func(makeIndex int32)              // CMMerchantQueryRepairCost
	sendRepairItem        func(makeIndex int32)              // CMUserRepairItem
	sendStorageItem       func(makeIndex int32)              // CMUserStorageItem
	sendTakeBackStorage   func(makeIndex int32)              // CMUserTakeBackStorageItem
	sendDealAdd           func(makeIndex int32)              // CMDealAddItem
	sendDealDel           func(makeIndex int32)              // CMDealDelItem
	sendDealChgGold       func(amount int)                   // CMDealChgGold
	sendDealEnd           func()                             // CMDealEnd
	sendOpenGuild         func()                             // CMOpenGuildDlg
	sendGuildMemberList   func()                             // CMGuildMemberList
	sendGuildAdd          func(name string)                  // CMGuildAddMember
	sendGuildDel          func(name string)                  // CMGuildDelMember
	sendGuildUpdateNotice func(text string)                  // CMGuildUpdateNotice
	sendGuildUpdateRank   func(text string)                  // CMGuildUpdateRankInfo
	sendGuildAlly         func(name string)                  // CMGuildAlly
	sendGuildBreakAlly    func(name string)                  // CMGuildBreakAlly
	sendGuildHome         func()                             // CMGuildHome
	sendGroupMode         func(allow int)                    // CMGroupMode
	sendCreateGroup       func(name string)                  // CMCreateGroup
	sendAddGroupMember    func(name string)                  // CMAddGroupMember
	sendDelGroupMember    func(name string)                  // CMDelGroupMember
	sendAdjustBonus       func(remaining int, deltas [9]int) // CMAdjustBonus
	sendQueryUserState    func(targetID int32)               // CMQueryUserState
	sendAttackMode        func(mode int)
	sendLogout            func()
	sendExit              func()
	sendAddFriend         func(name string)
	sendDelFriend         func(name string)
	sendQueryFriends      func()
	lastMoveTick          int64
	lastAniTick           int64
	text                  *engine.TextRenderer

	groundItems   map[int32]*GroundItemInfo
	floatingTexts []FloatingText
	chatMessages  []ChatMessage
	chatInput     string
	chatMode      bool

	ActionLock     bool
	ActionLockTime int64

	actionFailLockUntil int64

	lastHitTick int64

	// UI 框架（DWinCtl 移植）：面板、按钮、网格、模态框。
	ui            *UIManager
	itemMove      ItemMoveState
	tooltip       Tooltip
	hudPlusAbil   *UIControl
	hudBag        *UIControl
	bagHoverItem  *BagItem // 背包窗口内信息区当前悬停的物品（FState:4465）
	hudState      *UIControl
	stateSlotBtns [13]*UIControl
	stateMagBtns  [5]*UIControl
	statePageUp   *UIControl
	statePageDown *UIControl
	magicPage     int
	chatScroll    int // 聊天板从最新消息向上回滚的行数

	// 交易窗口（uideal.go）。
	hudDealOwn, hudDealRemote *UIControl
	dealActionTick            int64

	// 行会 + 组队面板（uiguild.go）。
	hudGuild, hudGroup *UIControl
	hudFriend          *UIControl
	friendSelected     int
	guildAdminBtns     []*UIControl
	guildChatMode      bool
	guildChats         []string // 行会聊天缓冲，上限 500 / 裁剪至 100（FState:6465-6475）
	guildActionTick    int64    // 组队操作 5 秒共享间隔（FState:5514）
	guildQueryTick     int64    // 行会首页/成员列表 3 秒间隔（FState:6370）

	// 属性加点 + 查看窗口（uiabil.go）。
	hudAbil, hudInspect *UIControl
	abilDeltas          [9]int
	abilPointsLeft      int
	showInspect         bool
	inspectItems        [13]*protocol.UserItem
	inspectName         string
	inspectSex          int
	inspectHair         int
	ctrlDown            bool // Ctrl 按住状态（加点面板 ×10，FState:6638）

	// NPC 对话 + 商店状态（uinpc.go）。
	hudNpc, hudMenu, hudSell *UIControl
	npcLines                 [][]npcSegment
	npcLineCentered          []bool // 每行是否居中 (<C> 标签)
	npcClicks                []npcClickPoint
	npcSelectTag             string
	npcLastClickTick         int64
	npcScrollOffset          int // 对话文本滚动偏移
	menuTop                  int
	menuIndex                int
	lastBuyTick              int64
	sellItem                 *BagItem
	sellWait                 *BagItem // 等待服务器确认出售的物品
	sellPriceStr             string
	queryPrice               bool
	queryPriceTick           int64
	merchantWasOpen          bool

	// 逻辑 800×600 空间下的鼠标坐标（每次移动时更新）。
	mouseX, mouseY float64
	// 双击合成（GLFW 无原生双击事件）：Delphi 收 WM_LBUTTONDBLCLK；
	// 这里检测两次左键按下间隔 <400ms 且距离 <4px。
	lastPressTick          int64
	lastPressX, lastPressY float64

	targetX, targetY int

	autoPath    [][2]int
	autoPathIdx int

	showMinimap bool
	deathGray   bool
	focusActor  *Actor

	// Delphi 输入还原（ClMain.pas / MShare.pas）
	targetCret         *Actor // 锁定攻击目标（g_TargetCret）
	mouseDownTick      int64  // 鼠标按下时间（拖拽检测）
	leftHeld           bool
	rightHeld          bool
	dupSelection       int   // 重叠选择循环（g_nDupSelection）
	lastAttackTick     int64 // 攻击后 1 秒禁移（g_dwLastAttackTick）
	lastMoveActionTick int64 // NPC 对话 1.5 秒静止（g_dwLastMoveTick）
	autoDig            bool  // 自动挖矿（g_boAutoDig）
	shiftDown          bool
	altDown            bool
	runReadyCount      int   // 跑步预备计数（g_nRunReadyCount）
	lastSpellTick      int64 // 施法冷却（g_dwLatestSpellTick）
	canPowerHit        bool  // +PWR 攻杀剑术（一次性）
	canLongHit         bool  // +LNG 刺杀剑术
	canWideHit         bool  // +WID 半月弯刀
	canCrsHit          bool  // +CRS 十字斩
	canTwnHit          bool  // +TWN 双龙斩
	canFireHit         bool  // +FIR 烈火剑法（一次性）
	canStnHit          bool  // +STN 石化攻击
	attackSlow         bool  // 服务端强制攻击减速
	lastFireHitTick    int64 // 烈火 10 秒冷却
	hoverItemName      string
	showAllItemNames   bool // Ctrl+Z 切换

	effects *EffectManager
	events  *EventManager

	dbg *DebugConsole // 全局调试控制台 (由 main.go 注入)

	// PlayScene 专有的调试开关 (由控制台命令切换, 经 StatusExtra 汇报)
	ShowGrid     bool
	ShowLabel    bool
	ShowPath     bool
	DisableLight bool
	DisableHPBar bool
}

func NewPlayScene(gl *engine.GLState, resources *engine.ResourceManager, mapDir string, dbg *DebugConsole) *PlayScene {
	s := &PlayScene{
		gl:             gl,
		resources:      resources,
		mapDir:         mapDir,
		texCache:       make(map[int]uint32),
		smTexCache:     make(map[int]uint32),
		objectsLoaders: make(map[int]*wil.File),
		objectsCaches:  make(map[int]map[int]uint32),
		State:          NewGameState(),
		groundItems:    make(map[int32]*GroundItemInfo),
		targetX:        -1,
		targetY:        -1,
		showMinimap:    true,
		effects:        NewEffectManager(),
		events:         NewEventManager(),
		ui:             NewUIManager(gl, resources, nil),
		dbg:            dbg,
	}
	// 手持物品时点击空白处将物品丢到地面
	// （FState.pas:1865-1886 DBackgroundBackgroundClick）。
	// 未持物时 WantReturn 保持 false，点击仍传递给游戏世界。
	s.ui.Root.OnBackgroundClick = func(c *UIControl) {
		if s.backgroundClick() {
			c.WantReturn = true
		}
	}
	s.buildHUD()
	s.buildBag()
	s.buildState()
	s.buildNpcPanels()
	s.buildDealPanels()
	s.buildGuildPanels()
	s.buildAbilPanel()
	s.buildFriendPanel()
	return s
}

// backgroundClick 处理穿透所有控件的点击。返回是否消费了该点击
// （丢弃物品/金币），消费时设置 WantReturn 使世界层忽略同一次点击
// （FState:1865-1894）。
func (s *PlayScene) backgroundClick() bool {
	if !s.itemMove.Moving {
		return false
	}
	switch {
	case s.itemMove.Index >= 0:
		// 将物品丢到地面（FState.pas:1842-1854）。
		if s.sendDropItem != nil {
			s.sendDropItem(s.itemMove.Item.MakeIndex)
			if slot := s.State.FindBagItemByMakeIndex(s.itemMove.Item.MakeIndex); slot >= 0 {
				s.State.BagItems[slot] = nil
			}
			s.itemMove.End()
			return true
		}
	case s.itemMove.Index == moveIdxBagGold:
		// 丢金币：询问数量（FState.pas:1870-1882）。
		gold := s.State.Gold
		ShowInput(s, "Drop how much gold?", func(ok bool, text string) {
			if !ok {
				return
			}
			amount := atoiClamped(text, 0, gold)
			if amount > 0 && s.sendDropGold != nil {
				s.sendDropGold(amount)
			}
		})
		s.itemMove.End()
		return true
	default:
		s.itemMove.Cancel(s.State)
	}
	return false
}

func (s *PlayScene) SetSendMove(fn func(ident int, dir int)) {
	s.sendMove = fn
}

func (s *PlayScene) SetSendOpenDoor(fn func(doorID, x, y int)) {
	s.sendOpenDoor = fn
}

func (s *PlayScene) SetSendAttack(fn func(ident int, dir int)) {
	s.sendAttack = fn
}

func (s *PlayScene) SetSendPickup(fn func()) {
	s.sendPickup = fn
}

func (s *PlayScene) SetSendButch(fn func(int32)) {
	s.sendButch = fn
}

func (s *PlayScene) SetSendChat(fn func(string)) {
	s.sendChat = fn
}

func (s *PlayScene) SetSendSpell(fn func(int, int, int)) {
	s.sendSpell = fn
}

func (s *PlayScene) SetSendNpcClick(fn func(int)) {
	s.sendNpcClick = fn
}

func (s *PlayScene) SetSendDealCancel(fn func()) {
	s.sendDealCancel = fn
}

func (s *PlayScene) SetSendUseItem(fn func(makeIndex int32)) {
	s.sendUseItem = fn
}

func (s *PlayScene) SetSendBuyItem(fn func(itemIdx int)) {
	s.sendBuyItem = fn
}

func (s *PlayScene) SetSendSellItem(fn func(makeIndex int32)) {
	s.sendSellItem = fn
}

func (s *PlayScene) SetSendDropItem(fn func(makeIndex int32)) {
	s.sendDropItem = fn
}

func (s *PlayScene) SetSendDropGold(fn func(amount int)) {
	s.sendDropGold = fn
}

func (s *PlayScene) SetSendDealTry(fn func()) {
	s.sendDealTry = fn
}

func (s *PlayScene) SetSendTakeOn(fn func(makeIndex int32, slot int)) {
	s.sendTakeOn = fn
}

func (s *PlayScene) SetSendTakeOff(fn func(slot int)) {
	s.sendTakeOff = fn
}

func (s *PlayScene) SetSendMagicKey(fn func(magID, key int)) {
	s.sendMagicKey = fn
}

func (s *PlayScene) SetSendMerchantSelect(fn func(npcID int32, tag string)) {
	s.sendMerchantSelect = fn
}

func (s *PlayScene) SetSendQueryPrice(fn func(makeIndex int32)) {
	s.sendQueryPrice = fn
}

func (s *PlayScene) SetSendQueryRepair(fn func(makeIndex int32)) {
	s.sendQueryRepair = fn
}

func (s *PlayScene) SetSendRepairItem(fn func(makeIndex int32)) {
	s.sendRepairItem = fn
}

func (s *PlayScene) SetSendStorageItem(fn func(makeIndex int32)) {
	s.sendStorageItem = fn
}

func (s *PlayScene) SetSendTakeBackStorage(fn func(makeIndex int32)) {
	s.sendTakeBackStorage = fn
}

func (s *PlayScene) SetSendDealAdd(fn func(makeIndex int32)) {
	s.sendDealAdd = fn
}

func (s *PlayScene) SetSendDealDel(fn func(makeIndex int32)) {
	s.sendDealDel = fn
}

func (s *PlayScene) SetSendDealChgGold(fn func(amount int)) {
	s.sendDealChgGold = fn
}

func (s *PlayScene) SetSendDealEnd(fn func()) {
	s.sendDealEnd = fn
}

func (s *PlayScene) SetSendOpenGuild(fn func())                    { s.sendOpenGuild = fn }
func (s *PlayScene) SetSendGuildMemberList(fn func())              { s.sendGuildMemberList = fn }
func (s *PlayScene) SetSendGuildAdd(fn func(name string))          { s.sendGuildAdd = fn }
func (s *PlayScene) SetSendGuildDel(fn func(name string))          { s.sendGuildDel = fn }
func (s *PlayScene) SetSendGuildUpdateNotice(fn func(text string)) { s.sendGuildUpdateNotice = fn }
func (s *PlayScene) SetSendGuildUpdateRank(fn func(text string))   { s.sendGuildUpdateRank = fn }
func (s *PlayScene) SetSendGuildAlly(fn func(name string))         { s.sendGuildAlly = fn }
func (s *PlayScene) SetSendGuildBreakAlly(fn func(name string))    { s.sendGuildBreakAlly = fn }
func (s *PlayScene) SetSendGuildHome(fn func())                    { s.sendGuildHome = fn }
func (s *PlayScene) SetSendGroupMode(fn func(allow int))           { s.sendGroupMode = fn }
func (s *PlayScene) SetSendCreateGroup(fn func(name string))       { s.sendCreateGroup = fn }
func (s *PlayScene) SetSendAddGroupMember(fn func(name string))    { s.sendAddGroupMember = fn }
func (s *PlayScene) SetSendDelGroupMember(fn func(name string))    { s.sendDelGroupMember = fn }
func (s *PlayScene) SetSendAdjustBonus(fn func(remaining int, deltas [9]int)) {
	s.sendAdjustBonus = fn
}
func (s *PlayScene) SetSendQueryUserState(fn func(targetID int32)) { s.sendQueryUserState = fn }

func (s *PlayScene) SetSendAttackMode(fn func(mode int)) {
	s.sendAttackMode = fn
}

func (s *PlayScene) SetSendLogout(fn func()) {
	s.sendLogout = fn
}

func (s *PlayScene) SetSendExit(fn func()) {
	s.sendExit = fn
}

func (s *PlayScene) SetSendAddFriend(fn func(string)) {
	s.sendAddFriend = fn
}

func (s *PlayScene) SetSendDelFriend(fn func(string)) {
	s.sendDelFriend = fn
}

func (s *PlayScene) SetSendQueryFriends(fn func()) {
	s.sendQueryFriends = fn
}

func (s *PlayScene) AddGroundItem(id int32, x, y, looks int, name string) {
	s.groundItems[id] = &GroundItemInfo{ID: id, X: x, Y: y, Looks: looks, Name: name}
}

func (s *PlayScene) RemoveGroundItem(id int32) {
	delete(s.groundItems, id)
}

func (s *PlayScene) SetText(t *engine.TextRenderer) {
	s.text = t
	s.ui.SetText(t)
}

func (s *PlayScene) LoadMap(mapName string) error {
	mapPath := filepath.Join(s.mapDir, mapName+".map")
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		return fmt.Errorf("load map %s: %w", mapName, err)
	}
	s.mapData = m
	s.clearAutoPath()
	if s.cam == nil {
		s.cam = engine.NewCamera(ScreenWidth, ScreenHeight)
	}
	s.cam.CenterOn(float64(m.Width)*engine.TileWidth/2, float64(m.Height)*engine.TileHeight/2)
	s.State.MapName = mapName

	if s.minimap != nil {
		s.minimap.Destroy()
		s.minimap = nil
	}
	s.minimapDirty = true
	s.lightingDirty = true

	if s.resources.Objects[0] != nil {
		s.objectsLoaders[0] = s.resources.Objects[0]
		s.objectsCaches[0] = make(map[int]uint32)
	}

	log.Logf(log.LevelInfo, "PlayScene", "map loaded: %s (%dx%d)", mapName, m.Width, m.Height)
	return nil
}

func (s *PlayScene) OnResize() {}

func (s *PlayScene) Open() {
	gActiveUI = s.ui
	s.registerDebugCmds()
	log.Logf(log.LevelInfo, "PlayScene", "opened")
}

func (s *PlayScene) Close() {
	gActiveUI = nil
	s.unregisterDebugCmds()
	gSound.SilenceSound()
	s.State.Reset()
	log.Logf(log.LevelInfo, "PlayScene", "closed")
}

func (s *PlayScene) Update(dt float64) {
	if s.minimapDirty && s.mapData != nil {
		s.minimap = NewMinimap(s.gl, s.mapData)
		s.minimapDirty = false
	}

	if s.lightingDirty {
		if s.lighting != nil {
			s.lighting.Destroy()
		}
		s.lighting = NewLightingSystem(s.gl, s.resources.DataDir())
		s.lightingDirty = false
	}

	now := time.Now().UnixMilli()

	if len(s.floatingTexts) > 0 {
		alive := s.floatingTexts[:0]
		for _, ft := range s.floatingTexts {
			if now-ft.StartTime > 1000 {
				continue
			}
			ft.Y -= float32(dt * 30)
			alive = append(alive, ft)
		}
		s.floatingTexts = alive
	}

	moveTick := false
	if now-s.lastMoveTick >= 100 {
		s.lastMoveTick = now
		moveTick = true
	}

	// 地图动画计数器每 50ms 递增一次（Delphi m_nAniCount,
	// PlayScn.pas:892-896），与渲染帧率解耦。
	if now-s.lastAniTick >= 50 {
		s.lastAniTick = now
		s.animCounter++
	}

	s.State.Actors.Update(now, moveTick)
	s.effects.Update(now)
	s.events.Update(now)
	s.pumpSellQuery()

	if s.State.MySelf != nil && s.cam != nil && s.mapData != nil {
		my := s.State.MySelf
		wx := float64(my.Rx)*engine.TileWidth + my.ShiftX + engine.TileWidth/2
		wy := float64(my.Ry)*engine.TileHeight + my.ShiftY + engine.TileHeight/2
		// 玩家居中在无遮挡可视区中心 (800/2=400, (600-131)/2≈234→取174匹配底栏blend区)。
		s.cam.X = wx - 400/s.cam.Zoom
		s.cam.Y = wy - 174/s.cam.Zoom
		s.cam.ClampToBounds(s.mapData.Width, s.mapData.Height)
	}

	if len(s.autoPath) > 0 && s.State.MySelf != nil && moveTick && s.sendMove != nil && !s.State.MySelf.Death {
		s.stepAutoPath()
	}

	if s.targetX >= 0 && len(s.autoPath) == 0 && s.State.MySelf != nil && moveTick && s.sendMove != nil && !s.State.MySelf.Death {
		my := s.State.MySelf
		if my.CurrX == s.targetX && my.CurrY == s.targetY {
			s.targetX = -1
			s.targetY = -1
		} else if my.IsIdle() && s.ServerAcceptNextAction() {
			dir := dirToward(my.CurrX, my.CurrY, s.targetX, s.targetY)
			dx, dy := dirOffset(dir)
			nx, ny := my.CurrX+dx, my.CurrY+dy
			if s.CanWalk(nx, ny) {
				dist := absInt(my.CurrX-s.targetX) + absInt(my.CurrY-s.targetY)
				// 骑马奔跑前进 3 格；服务端要求前方三格全部可通行
				// （HandleHorseRun x1/x2/x3）。第一格已由上方 CanWalk(nx, ny) 验证。
				if my.OnHorse && dist >= 4 && s.CanWalk(my.CurrX+dx*2, my.CurrY+dy*2) && s.CanWalk(my.CurrX+dx*3, my.CurrY+dy*3) {
					my.UpdateMsg(protocol.CMHorseRun, my.CurrX+dx*3, my.CurrY+dy*3, dir, 0, 0)
					s.sendMove(protocol.CMHorseRun, dir)
				} else if dist >= 3 {
					my.UpdateMsg(protocol.CMRun, nx+dx, ny+dy, dir, 0, 0)
					s.sendMove(protocol.CMRun, dir)
				} else {
					my.UpdateMsg(protocol.CMWalk, nx, ny, dir, 0, 0)
					s.sendMove(protocol.CMWalk, dir)
				}
				s.ActionLock = true
				s.ActionLockTime = time.Now().UnixMilli()
				s.lastMoveActionTick = time.Now().UnixMilli()
			} else {
				if !s.tryOpenDoor(nx, ny) {
					s.targetX = -1
					s.targetY = -1
				}
			}
		}
	}

	// 持续攻击（Delphi MouseTimerTimer, ClMain:2399-2448）
	if s.targetCret != nil && moveTick && s.State.MySelf != nil && !s.State.MySelf.Death {
		if s.targetCret.Death || s.State.Actors.Get(s.targetCret.RecogID) == nil {
			s.targetCret = nil
		} else if s.shouldAttack(s.targetCret) {
			s.attackTarget(s.targetCret)
		}
	}

	// 自动挖矿（ClMain:2432-2438）
	if s.autoDig && moveTick && s.State.MySelf != nil && !s.State.MySelf.Death {
		my := s.State.MySelf
		if my.IsIdle() && s.ServerAcceptNextAction() && s.canNextHit() {
			s.lastHitTick = time.Now().UnixMilli()
			if s.sendAttack != nil {
				s.sendAttack(protocol.CMHit+1, my.Dir)
			}
		}
	}
}

func (s *PlayScene) startAutoPath(tx, ty int) {
	s.targetX, s.targetY = -1, -1
	s.autoPathIdx = 0
	if my := s.State.MySelf; my != nil && (my.CurrX != tx || my.CurrY != ty) {
		s.autoPath = findPath(s.CanWalk, my.CurrX, my.CurrY, tx, ty)
		return
	}
	s.autoPath = nil
}

func (s *PlayScene) clearAutoPath() {
	s.autoPath = nil
	s.autoPathIdx = 0
}

func (s *PlayScene) stepAutoPath() {
	my := s.State.MySelf
	dest := s.autoPath[len(s.autoPath)-1]
	for s.autoPathIdx < len(s.autoPath) &&
		my.CurrX == s.autoPath[s.autoPathIdx][0] && my.CurrY == s.autoPath[s.autoPathIdx][1] {
		s.autoPathIdx++
	}
	if s.autoPathIdx >= len(s.autoPath) {
		s.clearAutoPath()
		return
	}
	if !my.IsIdle() || !s.ServerAcceptNextAction() {
		return
	}
	s.autoPath = s.repairAutoPath(dest, my)
	if s.autoPath == nil {
		return
	}
	wp := s.autoPath[s.autoPathIdx]
	dir := dirToward(my.CurrX, my.CurrY, wp[0], wp[1])
	dx, dy := dirOffset(dir)

	run := 1
	remaining := len(s.autoPath) - s.autoPathIdx
	if remaining >= 3 && s.collinearAhead(s.autoPathIdx, dx, dy) && s.CanWalk(my.CurrX+dx*2, my.CurrY+dy*2) {
		run = 2
		if my.OnHorse && remaining >= 4 && s.collinearAhead(s.autoPathIdx+1, dx, dy) && s.CanWalk(my.CurrX+dx*3, my.CurrY+dy*3) {
			run = 3
		}
	}
	switch run {
	case 3:
		my.UpdateMsg(protocol.CMHorseRun, my.CurrX+dx*3, my.CurrY+dy*3, dir, 0, 0)
		s.sendMove(protocol.CMHorseRun, dir)
	case 2:
		my.UpdateMsg(protocol.CMRun, my.CurrX+dx*2, my.CurrY+dy*2, dir, 0, 0)
		s.sendMove(protocol.CMRun, dir)
	default:
		my.UpdateMsg(protocol.CMWalk, my.CurrX+dx, my.CurrY+dy, dir, 0, 0)
		s.sendMove(protocol.CMWalk, dir)
	}
	s.autoPathIdx += run
	s.ActionLock = true
	s.ActionLockTime = time.Now().UnixMilli()
	s.lastMoveActionTick = time.Now().UnixMilli()
}

// repairAutoPath 在偏离路径（+FAIL 回滚）或下一格被占（其他角色
// 走入）时从当前位置重新寻路；无可行路径返回 nil。
func (s *PlayScene) repairAutoPath(dest [2]int, my *Actor) [][2]int {
	wp := s.autoPath[s.autoPathIdx]
	if absInt(wp[0]-my.CurrX) <= 1 && absInt(wp[1]-my.CurrY) <= 1 && s.CanWalk(wp[0], wp[1]) {
		return s.autoPath
	}
	if absInt(wp[0]-my.CurrX) <= 1 && absInt(wp[1]-my.CurrY) <= 1 && s.tryOpenDoor(wp[0], wp[1]) {
		return s.autoPath
	}
	path := findPath(s.CanWalk, my.CurrX, my.CurrY, dest[0], dest[1])
	s.autoPathIdx = 0
	return path
}

func (s *PlayScene) collinearAhead(idx, dx, dy int) bool {
	if idx+1 >= len(s.autoPath) {
		return false
	}
	prev := s.autoPath[idx]
	next := s.autoPath[idx+1]
	return next[0]-prev[0] == dx && next[1]-prev[1] == dy
}

func (s *PlayScene) Render(glState *engine.GLState, proj [16]float32) {
	if s.mapData == nil || s.cam == nil {
		return
	}
	s.renderFrame++
	debugRenderFrame = s.renderFrame
	verbose := s.renderFrame <= 2

	m := s.mapData
	cam := s.cam

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	if verbose {
		var srcRGB, dstRGB, srcA, dstA int32
		gl.GetIntegerv(gl.BLEND_SRC_RGB, &srcRGB)
		gl.GetIntegerv(gl.BLEND_DST_RGB, &dstRGB)
		gl.GetIntegerv(gl.BLEND_SRC_ALPHA, &srcA)
		gl.GetIntegerv(gl.BLEND_DST_ALPHA, &dstA)
		log.Logf(log.LevelInfo, "Render", "frame=%d BLEND factors: srcRGB=0x%04X dstRGB=0x%04X srcA=0x%04X dstA=0x%04X (expect src=0x0302 dst=0x0303)",
			s.renderFrame, srcRGB, dstRGB, srcA, dstA)
		log.Logf(log.LevelInfo, "Render", "frame=%d const check: SRC_ALPHA=0x%04X ONE_MINUS_SRC_ALPHA=0x%04X ONE=0x%04X ZERO=0x%04X",
			s.renderFrame, gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ZERO)
	}

	// 地图渲染到全窗口；底栏上浮覆盖底部。
	fbW, fbH := s.gl.ViewW, s.gl.ViewH
	s.gl.SetViewport(0, 0, fbW, fbH)
	if s.dbg.WireMode > 0 {
		s.gl.WireBounds = s.gl.WireBounds[:0]
		s.gl.WireRecording = true
		s.gl.WireRecord = false
	}
	if verbose {
		log.Logf(log.LevelInfo, "Render", "frame=%d fb=%dx%d viewport=(0,0,%d,%d) blend=SRC_ALPHA,ONE_MINUS_SRC_ALPHA",
			s.renderFrame, fbW, fbH, fbW, fbH)
	}

	left := float32(cam.X)
	top := float32(cam.Y)
	right := float32(cam.X + float64(cam.ViewW)/cam.Zoom)
	bottom := float32(cam.Y + float64(cam.ViewH)/cam.Zoom)
	proj = engine.OrthoProj4(left, right, bottom, top)

	startX, startY, endX, endY := cam.ViewportTiles(cullMargin, cullMargin)
	startX = clamp(startX, 0, m.Width-1)
	startY = clamp(startY, 0, m.Height-1)
	endX = clamp(endX, 0, m.Width-1)
	endY = clamp(endY, 0, m.Height-1)

	fStartX, fStartY, fEndX, fEndY := cam.ViewportTiles(frontCullMargin, frontCullMargin)
	fStartX = clamp(fStartX, 0, m.Width-1)
	fStartY = clamp(fStartY, 0, m.Height-1)
	fEndX = clamp(fEndX, 0, m.Width-1)
	fEndY = clamp(fEndY, 0, m.Height-1)

	bStartX, bStartY, bEndX, bEndY := startX, startY, endX, endY
	if bStartX%2 == 1 {
		bStartX--
	}
	if bStartY%2 == 1 {
		bStartY--
	}
	if bEndX%2 == 1 {
		bEndX++
	}
	if bEndY%2 == 1 {
		bEndY++
	}
	bStartX = clamp(bStartX, 0, m.Width-1)
	bStartY = clamp(bStartY, 0, m.Height-1)
	bEndX = clamp(bEndX, 0, m.Width-1)
	bEndY = clamp(bEndY, 0, m.Height-1)

	for y := bStartY; y <= bEndY; y += 2 {
		for x := bStartX; x <= bEndX; x += 2 {
			info := m.InfoAt(x, y)
			if info.BackLib < 0 || info.BackImage < 0 {
				continue
			}
			tex := s.getTex(s.texCache, s.resources.Tiles, info.BackImage)
			if tex == 0 {
				continue
			}
			img := s.resources.Tiles.GetImage(info.BackImage)
			wx := float32(x * engine.TileWidth)
			wy := float32(y * engine.TileHeight)
			s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		}
	}

	for y := startY; y <= endY; y++ {
		for x := startX; x <= endX; x++ {
			info := m.InfoAt(x, y)
			if info.MiddleLib < 0 || info.MiddleImage < 0 {
				continue
			}
			tex := s.getTex(s.smTexCache, s.resources.SmTiles, info.MiddleImage)
			if tex == 0 {
				continue
			}
			img := s.resources.SmTiles.GetImage(info.MiddleImage)
			wx := float32(x * engine.TileWidth)
			wy := float32(y * engine.TileHeight)
			s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		}
	}

	if s.dbg.WireMode > 0 {
		s.gl.WireRecord = true
	}
	s.renderFrontWithActors(fStartX, fStartY, fEndX, fEndY, proj)
	s.gl.WireCategory = 3
	s.effects.Render(s.gl, s.resources, proj)

	for _, ft := range s.floatingTexts {
		if s.text != nil {
			s.text.DrawText(ft.Text, ft.X, ft.Y, ft.Color[0], ft.Color[1], ft.Color[2], ft.Color[3], proj)
		}
	}

	if s.showMinimap && s.minimap != nil {
		s.minimap.Render(s.cam, s.mapData.Width, s.mapData.Height)
	}

	if s.deathGray {
		// F6: 死亡灰度效果 — 使用 multiply blend 实现更自然的去饱和
		gl.BlendFunc(gl.DST_COLOR, gl.ZERO)
		s.gl.DrawQuadColor(float32(s.cam.X), float32(s.cam.Y),
			float32(float64(s.cam.ViewW)/s.cam.Zoom), float32(float64(s.cam.ViewH)/s.cam.Zoom),
			0.45, 0.45, 0.45, 1.0, proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}

	if s.lighting != nil && !s.deathGray && !s.DisableLight {
		darkness := s.calcDarkness()
		if darkness > 0.01 {
			lights := s.collectLightSources()
			s.lighting.Render(proj, s.cam.X, s.cam.Y, s.cam.ViewW, s.cam.ViewH, s.cam.Zoom, darkness, lights)
		}
	}

	if s.dbg.WireMode > 0 {
		s.gl.WireRecording = false
		s.updateHover()
		s.renderWireframes(proj)
		s.renderHoverInfo(proj)
		s.dbg.wireHandled = true
	}
	if s.ShowGrid {
		s.renderDebugGrid(proj)
	}
	if s.ShowLabel {
		s.renderDebugInfo(proj)
	}
	if s.ShowPath {
		s.renderDebugPath(proj)
	}

	// UI 层使用完整窗口逻辑视口。
	s.gl.SetViewport(0, 0, fbW, fbH)
	if verbose {
		log.Logf(log.LevelInfo, "Render", "frame=%d UI viewport=(0,0,%d,%d) uiProj=OrthoProj(%d,%d)", s.renderFrame, fbW, fbH, ScreenWidth, ScreenHeight)
	}
	uiProj := engine.OrthoProj(ScreenWidth, ScreenHeight)
	if s.showMinimap {
		mmapDrawn := false
		mmIdx := s.State.MinimapIndex
		if s.resources.Mmap != nil && mmIdx >= 0 && mmIdx < s.resources.Mmap.Count {
			mmImg := s.resources.Mmap.GetImage(mmIdx)
			if mmImg != nil && mmImg.RGBA != nil {
				mmTex := s.resources.GetTexture(s.resources.Mmap, mmIdx)
				if mmTex != 0 {
					s.gl.DrawQuad(mmTex, ScreenWidth - 120, 0, 120, 120, uiProj)
					mmapDrawn = true
				}
			}
		}
		if !mmapDrawn && s.minimap != nil {
			glState.DrawQuad(s.minimap.GetTexture(), ScreenWidth - 120, 0, 120, 120, uiProj)
		}
		// 在小地图上叠加周边角色雷达点（以自身为中心）。
		if s.minimap != nil && s.State.MySelf != nil {
			s.minimap.DrawActorDots(s.gl, s.State.Actors.All(), s.State.MySelf.CurrX, s.State.MySelf.CurrY, uiProj)
		}
	}
	s.RenderUI(uiProj)
	if s.ui.ShowBounds {
		s.ui.RenderDebugBounds(uiProj)
	}
}

func (s *PlayScene) renderFrontWithActors(fStartX, fStartY, fEndX, fEndY int, proj [16]float32) {
	// 阶段 A：先绘制 48×32 地面层前景物件——它们始终在所有角色后面
	// （PlayScn.pas:1064-1108）。
	s.gl.WireCategory = 1
	for y := fStartY; y <= fEndY; y++ {
		for x := fStartX; x <= fEndX; x++ {
			info := s.mapData.InfoAt(x, y)
			s.drawFrontSmall(info, x, y, proj)
		}
	}

	// 阶段 B：逐行 Y 排序——大型/混合前景物件、地图事件、
	// 地面物品和角色按行交错绘制（PlayScn.pas:1124-1249）。
	actors := s.State.Actors.SortedByY()
	actorIdx := 0

	for y := fStartY; y <= fEndY; y++ {
		s.gl.WireCategory = 1
		for x := fStartX; x <= fEndX; x++ {
			info := s.mapData.InfoAt(x, y)
			s.drawFrontLarge(info, x, y, proj)
		}

		s.events.RenderRow(s.gl, s.resources, proj, y)

		s.gl.WireCategory = 4
		for _, gi := range s.groundItems {
			if gi.Y == y && gi.X >= fStartX && gi.X <= fEndX {
				s.drawGroundItemIcon(gi, proj)
			}
		}

		s.gl.WireCategory = 2
		for actorIdx < len(actors) && actors[actorIdx].Ry <= y {
			a := actors[actorIdx]
			worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
			worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
			a.Draw(s.gl, s.resources, worldX, worldY, proj)
			s.drawActorLabel(a, worldX, worldY, proj)
			s.drawChatBubble(a, worldX, worldY, proj)
			actorIdx++
		}
	}

	s.gl.WireCategory = 2
	for ; actorIdx < len(actors); actorIdx++ {
		a := actors[actorIdx]
		worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
		worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
		a.Draw(s.gl, s.resources, worldX, worldY, proj)
		s.drawActorLabel(a, worldX, worldY, proj)
		s.drawChatBubble(a, worldX, worldY, proj)
	}

	// NPC 特效覆盖层（Delphi PlayScn.pas:1321-1326 DrawEff pass）
	s.gl.WireCategory = 2
	for _, a := range actors {
		if a.Type == ActorNPC && a.NpcUseEffect {
			worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
			worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
			a.DrawNpcEffect(s.gl, s.resources, worldX, worldY, proj)
		}
	}

	// 阶段 C：覆盖层重绘和地面物品闪烁/名称
	// （PlayScn.pas:1290-1396）。
	if s.State.MySelf != nil && !s.State.MySelf.Death {
		my := s.State.MySelf
		wx := float32(float64(my.Rx*engine.TileWidth) + my.ShiftX)
		wy := float32(float64(my.Ry*engine.TileHeight) + my.ShiftY)
		log.Logf(log.LevelTrace, "Render", "play self redraw pos=(%.0f,%.0f) dir=%d", wx, wy, my.Dir)
		my.RedrawPass = true
		my.Draw(s.gl, s.resources, wx, wy, proj)
		my.RedrawPass = false
	}

	if s.focusActor != nil && s.focusActor != s.State.MySelf && !s.focusActor.Death {
		fa := s.focusActor
		fx := float32(float64(fa.Rx*engine.TileWidth) + fa.ShiftX)
		fy := float32(float64(fa.Ry*engine.TileHeight) + fa.ShiftY)
		fa.Draw(s.gl, s.resources, fx, fy, proj)
	}

	for _, gi := range s.groundItems {
		if gi.X < fStartX || gi.X > fEndX || gi.Y < fStartY || gi.Y > fEndY {
			continue
		}
		s.drawGroundItemFlashName(gi, proj)
	}
}

func (s *PlayScene) drawGroundItemIcon(gi *GroundItemInfo, proj [16]float32) {
	ix := float32(gi.X*engine.TileWidth) + 16
	iy := float32(gi.Y*engine.TileHeight) + 8
	if s.resources.DnItems != nil && gi.Looks >= 0 && gi.Looks < s.resources.DnItems.Count {
		img := s.resources.DnItems.GetImage(gi.Looks)
		if img != nil && img.RGBA != nil {
			tex := s.resources.GetTexture(s.resources.DnItems, gi.Looks)
			if tex != 0 {
				log.Logf(log.LevelTrace, "Render", "play ground item DnItems[%d] pos=(%.0f,%.0f) size=(%d,%d)", gi.Looks, ix, iy, img.Width, img.Height)
				s.gl.DrawQuad(tex, ix, iy, float32(img.Width), float32(img.Height), proj)
				return
			}
		}
	}
	s.gl.DrawQuadColor(ix, iy, 16, 16, 0.9, 0.8, 0.2, 0.8, proj)
}

func (s *PlayScene) drawGroundItemFlashName(gi *GroundItemInfo, proj [16]float32) {
	ix := float32(gi.X*engine.TileWidth) + 16
	iy := float32(gi.Y*engine.TileHeight) + 8
	if s.resources.Prguse != nil {
		now := time.Now().UnixMilli()
		cycle := now % 5000
		if cycle < 200 {
			step := int(cycle / 20)
			flashIdx := 410 + step
			if fimg := s.resources.Prguse.GetImage(flashIdx); fimg != nil && fimg.RGBA != nil {
				if ftex := s.resources.GetTexture(s.resources.Prguse, flashIdx); ftex != 0 {
					log.Logf(log.LevelTrace, "Render", "play item flash Prguse[%d] pos=(%.0f,%.0f)", flashIdx, ix, iy)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
					s.gl.DrawQuad(ftex, ix, iy, float32(fimg.Width), float32(fimg.Height), proj)
					gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				}
			}
		}
	}
	if s.text != nil && gi.Name != "" {
		nameW := float32(s.text.MeasureText(gi.Name))
		nameX := float32(gi.X*engine.TileWidth) + float32(engine.TileWidth)/2 - nameW/2
		log.Logf(log.LevelTrace, "Render", "play item name '%s' pos=(%.0f,%.0f)", gi.Name, nameX, iy-14)
		s.text.DrawText(gi.Name, nameX, iy-14, 1.0, 1.0, 0.8, 1.0, proj)
	}
}

func (s *PlayScene) drawActorLabel(a *Actor, worldX, worldY float32, proj [16]float32) {
	showName := false
	if s.State.MySelf != nil {
		if a.RecogID == s.State.MySelf.RecogID {
			showName = true
		} else if absInt(a.CurrX-s.State.MySelf.CurrX) <= 5 && absInt(a.CurrY-s.State.MySelf.CurrY) <= 5 {
			showName = true
		}
	}
	// Delphi SayY：存活 = tileRow + ShiftY - 47，死亡 = tileRow + ShiftY - 12
	// （PlayScn.pas:1232-1235）。世界坐标下 tileRow ≈ worldY。
	sayY := worldY - 47
	if a.Death {
		sayY = worldY - 12
	}
	if showName && a.UserName != "" && s.text != nil {
		nameW := float32(s.text.MeasureText(a.UserName))
		nameX := worldX + float32(engine.TileWidth)/2 - nameW/2
		log.Logf(log.LevelTrace, "Render", "play actor name '%s' pos=(%.0f,%.0f) death=%v", a.UserName, nameX, sayY, a.Death)
		nr, ng, nb := 1.0, 1.0, 1.0
		switch a.NameColor {
		case 249: // 红名
			nr, ng, nb = 1.0, 0.2, 0.2
		case 251: // 黄名
			nr, ng, nb = 1.0, 1.0, 0.2
		}
		s.text.DrawText(a.UserName, nameX-1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX+1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY-1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY+1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY, float32(nr), float32(ng), float32(nb), 1.0, proj)
	}

	// 所有可见角色的血条（DrawScrn.pas:280-301），位于 SayY - 10。
	// 注意：十周年版 Prguse2.wil 的 index 0/1 不是 HP 条图像（80x51 散布暗像素），
	// 因此直接使用彩色矩形绘制，不使用 Prguse2 纹理。
	if !a.Death && !s.DisableHPBar && a.Type != ActorNPC {
		hpBarY := sayY - 10
		s.gl.DrawQuadColor(worldX+4, hpBarY, 40, 4, 0.1, 0.0, 0.0, 0.8, proj)
		ratio := float32(1.0)
		if s.State.MySelf != nil && a.RecogID == s.State.MySelf.RecogID && s.State.MaxHP > 0 {
			ratio = float32(s.State.HP) / float32(s.State.MaxHP)
		} else if a.ShowHP && a.ShowMaxHPVal > 0 {
			ratio = float32(a.ShowHPVal) / float32(a.ShowMaxHPVal)
		}
		if ratio > 0 {
			s.gl.DrawQuadColor(worldX+4, hpBarY, 40*ratio, 4, 0.8, 0.0, 0.0, 0.8, proj)
		}
	}
}

func (s *PlayScene) drawChatBubble(a *Actor, worldX, worldY float32, proj [16]float32) {
	if s.text == nil || a.SayLineCount == 0 {
		return
	}
	if time.Now().UnixMilli()-a.SayTime > 4000 {
		return
	}
	sayY := worldY - 47
	if a.Death {
		sayY = worldY - 12
	}
	bubbleY := sayY - float32(a.SayLineCount)*16
	log.Logf(log.LevelTrace, "Render", "play chat bubble lines=%d pos=(%.0f,%.0f)", a.SayLineCount, worldX-20, bubbleY)
	for i := 0; i < a.SayLineCount && i < 5; i++ {
		if a.SayingArr[i] != "" {
			r, g, b := float32(1.0), float32(1.0), float32(1.0)
			if a.Death {
				r, g, b = 0.5, 0.5, 0.5
			}
			s.text.DrawText(a.SayingArr[i], worldX-20, bubbleY+float32(i)*14, r, g, b, 0.9, proj)
		}
	}
}

// frontImageData 解析前景层格子的 WIL 图像。格子无可绘制前景物件时
// 返回 nil img。
func (s *PlayScene) frontImageData(info *mapformat.CellInfo, x, y int) (loader *wil.File, cache map[int]uint32, idx int, isBlend bool, img *wil.Image, tex uint32) {
	if info.FrontLib < 0 {
		return
	}
	area := int(info.FrontArea)
	loader = s.getObjectsLoader(area)
	if loader == nil {
		return
	}
	cache = s.objectsCaches[area]

	idx = info.FrontImage
	isBlend = info.FrontAniFrame&0x80 != 0

	ani := int(info.FrontAniFrame & 0x7F)
	if ani > 0 {
		tick := int(info.FrontAniTick)
		if tick < 1 {
			tick = 1
		}
		cycleLen := ani + ani*tick
		if cycleLen > 0 {
			frame := (s.animCounter % cycleLen) / (1 + tick)
			idx += frame
		}
	}

	if info.FrontDoorOffset&0x80 != 0 {
		if info.FrontDoorIndex&0x7F != 0 {
			idx += int(info.FrontDoorOffset & 0x7F)
		}
	}

	if idx < 0 || idx >= loader.Count {
		return
	}
	tex = s.getTex(cache, loader, idx)
	if tex == 0 {
		return
	}
	img = loader.GetImage(idx)
	return
}

// drawFrontSmall 只绘制 48×32（单格）前景物件。这些是地面装饰，
// 必须始终渲染在所有角色后面（PlayScn.pas:1064-1108）。
func (s *PlayScene) drawFrontSmall(info *mapformat.CellInfo, x, y int, proj [16]float32) {
	_, _, _, isBlend, img, tex := s.frontImageData(info, x, y)
	if img == nil || isBlend {
		return
	}
	if img.Width != 48 || img.Height != 32 {
		return
	}
	wx := float32(x * engine.TileWidth)
	wy := float32(y*engine.TileHeight) - float32(img.Height) + engine.TileHeight
	s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
}

// drawFrontLarge 绘制非 48×32 的前景物件（高楼、树木等）以及
// alpha 混合物件。它们与角色一起参与逐行 Y 排序
// （PlayScn.pas:1124-1178）。
func (s *PlayScene) drawFrontLarge(info *mapformat.CellInfo, x, y int, proj [16]float32) {
	_, _, _, isBlend, img, tex := s.frontImageData(info, x, y)
	if img == nil {
		return
	}
	if !isBlend && img.Width == 48 && img.Height == 32 {
		return
	}
	cellWorldX := float32(x * engine.TileWidth)
	cellWorldY := float32(y * engine.TileHeight)
	if isBlend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		wx := cellWorldX + float32(img.HotX) - 2
		wy := cellWorldY + float32(img.HotY) - 68
		s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	} else {
		wx := cellWorldX
		wy := cellWorldY - float32(img.Height) + engine.TileHeight
		s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
	}
}

func (s *PlayScene) getObjectsLoader(area int) *wil.File {
	if f, ok := s.objectsLoaders[area]; ok {
		return f
	}
	if area == 0 {
		return s.resources.Objects[0]
	}
	filename := fmt.Sprintf("Objects%d.wil", area+1)
	wilPath := filepath.Join(s.resources.DataDir(), filename)
	f, err := wil.Load(wilPath)
	if err != nil {
		s.objectsLoaders[area] = nil
		return nil
	}
	s.objectsLoaders[area] = f
	s.objectsCaches[area] = make(map[int]uint32)
	return f
}

func (s *PlayScene) getTex(cache map[int]uint32, file *wil.File, idx int) uint32 {
	if idx < 0 || file == nil || idx >= file.Count {
		return 0
	}
	if tex, ok := cache[idx]; ok {
		return tex
	}
	img := file.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return 0
	}
	tex := s.gl.UploadTexture(img.RGBA)
	cache[idx] = tex
	img.RGBA = nil
	return tex
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (s *PlayScene) CanWalk(x, y int) bool {
	if s.mapData == nil {
		return false
	}
	if x < 0 || x >= s.mapData.Width || y < 0 || y >= s.mapData.Height {
		return false
	}
	info := s.mapData.InfoAt(x, y)
	if info == nil {
		return false
	}
	if info.Collision {
		return false
	}
	if info.FrontDoorIndex&0x80 != 0 && info.FrontDoorOffset&0x80 == 0 {
		return false
	}
	for _, a := range s.State.Actors.All() {
		if a.CurrX == x && a.CurrY == y && !a.Death {
			return false
		}
	}
	return true
}

func (s *PlayScene) tryOpenDoor(x, y int) bool {
	if s.mapData == nil || s.sendOpenDoor == nil {
		return false
	}
	door := s.mapData.GetDoor(x, y)
	if door > 0 && !s.mapData.IsDoorOpen(x, y) {
		s.sendOpenDoor(door, x, y)
		return true
	}
	return false
}

func (s *PlayScene) ServerAcceptNextAction() bool {
	now := time.Now().UnixMilli()
	if now < s.actionFailLockUntil {
		return false
	}
	if !s.ActionLock {
		return true
	}
	if now-s.ActionLockTime > 10000 {
		s.ActionLock = false
		return true
	}
	return false
}

func (s *PlayScene) calcDarkness() float32 {
	dayVal := dayBrightToDarkness(s.State.DayBright)
	mapVal := darkLevelToDarkness(s.State.MapDarkness)
	return max(dayVal, mapVal)
}

// dayBrightToDarkness 将 SM_DAYCHANGING Param（0=最暗 … 3+=最亮）
// 映射为 0..1 的遮罩透明度。
func dayBrightToDarkness(bright int) float32 {
	switch {
	case bright <= 0:
		return 0.7
	case bright == 1:
		return 0.45
	case bright == 2:
		return 0.2
	default:
		return 0
	}
}

// darkLevelToDarkness 将 Delphi DarkLevel 映射为暗度值
// （0=白天, 1=深夜, 2=中等, 3=黄昏）→ 0..1 遮罩透明度
// （PlayScn.pas:2275, cliUtil.pas:594-599）。
func darkLevelToDarkness(level int) float32 {
	switch {
	case level <= 0:
		return 0
	case level == 1:
		return 0.7
	case level == 2:
		return 0.45
	default:
		return 0.2
	}
}

func (s *PlayScene) collectLightSources() []LightSource {
	var lights []LightSource
	if s.State.MySelf != nil {
		my := s.State.MySelf
		lights = append(lights, LightSource{
			X:     float64(my.Rx)*engine.TileWidth + engine.TileWidth/2,
			Y:     float64(my.Ry)*engine.TileHeight + engine.TileHeight/2,
			Level: max(2, s.State.LightLevel),
		})
	}
	lights = append(lights, s.effects.LightSources()...)
	if s.mapData != nil {
		startX, startY, endX, endY := s.cam.ViewportTiles(2, 2)
		startX = clamp(startX, 0, s.mapData.Width-1)
		startY = clamp(startY, 0, s.mapData.Height-1)
		endX = clamp(endX, 0, s.mapData.Width-1)
		endY = clamp(endY, 0, s.mapData.Height-1)
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				info := s.mapData.InfoAt(x, y)
				if info.Light > 0 {
					lights = append(lights, LightSource{
						X:     float64(x)*engine.TileWidth + engine.TileWidth/2,
						Y:     float64(y)*engine.TileHeight + engine.TileHeight/2,
						Level: int(info.Light) - 1,
					})
				}
			}
		}
	}
	return lights
}

func (s *PlayScene) OnChar(char rune) {
	if s.ui.RouteChar(char) {
		return
	}
	// 聊天前缀（ClMain:1765-1783）：@ ! / 打开聊天并预设前缀
	if !s.chatMode && (char == '@' || char == '!' || char == '/') {
		s.chatMode = true
		s.chatInput = string(char)
		return
	}
	// 聊天输入接受任何可打印字符，包括中文（游戏为中文环境）；
	// 排除 127（DEL）。最大长度 70（PlayScn.pas:273）。
	if s.chatMode && char >= 32 && char != 127 {
		if utf8.RuneCountInString(s.chatInput) < 70 {
			s.chatInput += string(char)
		}
	}
}

func (s *PlayScene) OnKey(key int, action int) {
	// UI（模态框 / 聚焦的编辑控件）优先接收按键。
	if s.ui.RouteKeyDown(key) {
		return
	}
	// 跟踪 Ctrl 键用于加点面板 ×10 加速。
	if key == 341 || key == 345 {
		s.ctrlDown = action == 1
		return
	}
	// 跟踪 Shift/Alt 键（Delphi 输入判定依赖）
	if key == 340 || key == 344 { // Left/Right Shift
		s.shiftDown = action == 1
		return
	}
	if key == 342 || key == 346 { // Left/Right Alt
		s.altDown = action == 1
		return
	}
	if key == 256 && s.itemMove.Moving { // Esc 取消手持物品
		s.itemMove.Cancel(s.State)
		return
	}

	if action == 1 {
		switch key {
		case 257: // Enter
			if s.chatMode {
				if s.chatInput != "" && s.sendChat != nil {
					s.sendChat(s.chatInput)
				}
				s.chatInput = ""
				s.chatMode = false
			} else {
				s.chatMode = true
			}
			return
		case keyBackspace:
			if s.chatMode && s.chatInput != "" {
				runes := []rune(s.chatInput)
				s.chatInput = string(runes[:len(runes)-1])
			}
			return
		case 32: // 空格 — 打开聊天（ClMain:1741-1745）
			s.chatMode = true
			return
		case 66: // B — 背包（ClMain:1675-1678）
			s.State.ShowBag = !s.State.ShowBag
			return
		case 67: // C — 状态面板第 0 页（ClMain:1668-1674）
			s.State.StatePage = 0
			s.State.ShowEquip = true
			return
		case 69: // E — 状态面板第 3 页（ClMain:1676-1685）
			s.State.StatePage = 3
			s.State.ShowEquip = true
			return
		case 71: // G（ClMain:1637-1661）
			if s.ctrlDown {
				// Ctrl+G — 与 focusActor 组队（ClMain:1638-1643）
				if s.focusActor != nil && s.focusActor.Type == ActorHuman && s.sendCreateGroup != nil {
					s.sendCreateGroup(s.focusActor.UserName)
				}
			} else if s.altDown {
				// Alt+G — 从组队移除 focusActor（ClMain:1645-1648）
				if s.focusActor != nil && s.focusActor.Type == ActorHuman && s.sendDelGroupMember != nil {
					s.sendDelGroupMember(s.focusActor.UserName)
				}
			} else {
				s.toggleGuild()
			}
			return
		case 72: // Ctrl+H — 切换攻击模式（ClMain:1517-1525）
			if s.ctrlDown {
				s.State.AttackMode = (s.State.AttackMode + 1) % 5
				if s.sendAttackMode != nil {
					s.sendAttackMode(s.State.AttackMode)
				}
				modes := []string{"和平", "组队", "行会", "全体", "PK"}
				s.AddChatMessage("[系统] 攻击模式: " + modes[s.State.AttackMode])
			}
			return
		case 77: // M — 小地图（ClMain:1613-1628；三态循环见 B5）
			s.showMinimap = !s.showMinimap
			return
		case 78: // N — 属性加点（ClMain:1692-1695）
			s.State.ShowPlusAbil = !s.State.ShowPlusAbil
			return
		case 83: // S — 组队对话框（ClMain:1629-1636）
			s.State.ShowGroupDlg = !s.State.ShowGroupDlg
			return
		case 86: // V — 好友对话框
			s.State.ShowFriend = !s.State.ShowFriend
			if s.State.ShowFriend && s.sendQueryFriends != nil {
				s.sendQueryFriends()
			}
			return
		case 87: // W — 交易（ClMain:1663-1666）
			s.tryDeal()
			return
		case 90: // Z（ClMain:1564-1573）
			if s.ctrlDown {
				// Ctrl+Z — 切换显示所有地面物品名（ClMain:1564-1567）
				s.showAllItemNames = !s.showAllItemNames
			} else {
				// Z — 拾取物品
				if s.sendPickup != nil {
					s.sendPickup()
				}
			}
			return
		case 65: // Ctrl+A — 休息（ClMain:1522-1526）
			if s.ctrlDown && s.sendChat != nil {
				s.sendChat("@Rest")
			}
			return
		case 88: // Alt+X — 登出（ClMain:1575-1593）
			if s.altDown && s.sendLogout != nil {
				s.sendLogout()
			}
			return
		case 81: // Alt+Q — 退出游戏（ClMain:1594-1612）
			if s.altDown && s.sendExit != nil {
				s.sendExit()
			}
			return
		case 265: // 上箭头 — 聊天向上翻一行（ClMain:1699-1706）
			s.scrollChat(-1)
			return
		case 264: // 下箭头 — 聊天向下翻一行（ClMain:1707-1714）
			s.scrollChat(1)
			return
		case 266: // PageUp — 聊天向上翻页（ClMain:1715-1718）
			s.scrollChat(-ViewChatLine)
			return
		case 267: // PageDown — 聊天向下翻页（ClMain:1719-1722）
			s.scrollChat(ViewChatLine)
			return
		case 298: // F9 — 背包（ClMain:1488-1494）
			s.State.ShowBag = !s.State.ShowBag
			return
		case 299: // F10 — 状态面板第 0 页（ClMain:1495-1502）
			s.State.StatePage = 0
			s.State.ShowEquip = true
			return
		case 300: // F11 — 状态面板第 3 页（ClMain:1503-1508）
			s.State.StatePage = 3
			s.State.ShowEquip = true
			return
		case 301: // F12 — 选项/声音（ClMain:1509+）
			if gSound != nil {
				if gSound.ToggleSFX() {
					s.AddChatMessage("[音乐打开]")
				} else {
					s.AddChatMessage("[音乐关闭]")
				}
			}
			return
		}

		// F1..F8 释放绑定在该键上的魔法，朝鼠标位置施法（ClMain:1268-1285）。
		if !s.chatMode && key >= 290 && key <= 297 {
			now := time.Now().UnixMilli()
			if now-s.lastSpellTick >= 500 {
				k := byte('1' + (key - 290))
				for i := range s.State.Magics {
					if s.State.Magics[i].Key != k {
						continue
					}
					if s.sendSpell != nil && s.cam != nil {
						wx, wy := s.cam.ScreenToWorld(s.mouseX, s.mouseY)
						tx, ty := s.cam.WorldToTile(wx, wy)
						s.sendSpell(int(s.State.Magics[i].MagID), tx, ty)
						s.lastSpellTick = now
					}
					break
				}
			}
			return
		}

		if !s.chatMode && key >= 49 && key <= 54 {
			if item := s.State.BeltItems[key-49]; item != nil && s.sendUseItem != nil {
				if item.Def != nil {
					if idx := itemUseSoundIdx(item.Def.StdMode); idx >= 0 {
						gSound.PlaySound(idx)
					}
				}
				s.sendUseItem(item.MakeIndex)
			}
			return
		}
	}
}

// scrollChat 按 delta 行滚动聊天板，限制在缓冲区内
// （ClMain:1699-1722）。
func (s *PlayScene) scrollChat(delta int) {
	s.chatScroll += delta
	max := len(s.chatMessages) - ViewChatLine
	if max < 0 {
		max = 0
	}
	if s.chatScroll < 0 {
		s.chatScroll = 0
	}
	if s.chatScroll > max {
		s.chatScroll = max
	}
}

func dirOffset(dir int) (dx, dy int) {
	switch dir {
	case 0:
		return 0, -1
	case 1:
		return 1, -1
	case 2:
		return 1, 0
	case 3:
		return 1, 1
	case 4:
		return 0, 1
	case 5:
		return -1, 1
	case 6:
		return -1, 0
	case 7:
		return -1, -1
	}
	return 0, 0
}

func (s *PlayScene) OnMouseMove(x, y float64) {
	s.mouseX, s.mouseY = x, y
	s.ui.RouteMouseMove(int(x), int(y))

	// 基于格子的焦点检测（Delphi g_FocusCret, ClMain.pas:2085-2096）。
	s.focusActor = nil
	s.hoverItemName = ""
	if y >= MapSurfaceH || s.cam == nil || s.State.MySelf == nil {
		return
	}
	wx, wy := s.cam.ScreenToWorld(x, y)
	tx, ty := s.cam.WorldToTile(wx, wy)

	// 地面物品悬停提示（ClMain:2098-2107）
	hoverItem := ""
	for _, gi := range s.groundItems {
		if gi.X == tx && gi.Y == ty {
			hoverItem = gi.Name
			s.tooltip.Show(int(x), int(y), gi.Name, [4]float32{1, 1, 0.8, 1}, true)
			break
		}
	}
	// 仅当之前有悬停物品且现在没有时清除（避免覆盖 UI 提示）
	if s.hoverItemName != "" && hoverItem == "" {
		s.tooltip.Clear()
	}
	s.hoverItemName = hoverItem

	for _, a := range s.State.Actors.All() {
		if a.RecogID == s.State.MySelf.RecogID || a.Death {
			continue
		}
		if a.CurrX == tx && a.CurrY == ty {
			s.focusActor = a
			break
		}
	}

	// 拖拽移动（ClMain:2115-2116）：按住 >300ms 重新触发移动
	nowMs := time.Now().UnixMilli()
	if nowMs >= s.actionFailLockUntil && s.leftHeld && nowMs-s.mouseDownTick > 300 {
		if s.State.MySelf != nil && !s.State.MySelf.Death && s.sendMove != nil {
			my := s.State.MySelf
			if tx != my.CurrX || ty != my.CurrY {
				s.targetCret = nil
				s.startAutoPath(tx, ty)
			}
		}
	}
	if nowMs >= s.actionFailLockUntil && s.rightHeld && nowMs-s.mouseDownTick > 300 {
		if s.State.MySelf != nil && !s.State.MySelf.Death && s.sendMove != nil {
			my := s.State.MySelf
			if absInt(my.CurrX-tx) > 2 || absInt(my.CurrY-ty) > 2 {
				s.clearAutoPath()
				s.targetX = tx
				s.targetY = ty
			}
		}
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (s *PlayScene) OnMouse(x, y float64, button int, action int, mods int) {
	s.mouseX, s.mouseY = x, y
	ix, iy := int(x), int(y)

	if action == 0 { // 松开（ClMain:2384-2389）
		s.ui.RouteMouseUp(ix, iy, button)
		if button == 0 {
			s.leftHeld = false
		}
		if button == 1 {
			s.rightHeld = false
		}
		// Delphi: 松开鼠标取消移动目标
		s.targetX, s.targetY = -1, -1
		s.clearAutoPath()
		return
	}
	if action != 1 {
		return
	}

	// 记录按下状态（ClMain:2120-2126）
	s.mouseDownTick = time.Now().UnixMilli()
	s.runReadyCount = 0
	if button == 0 {
		s.leftHeld = true
	}
	if button == 1 {
		s.rightHeld = true
	}

	const (
		modShift = 0x0001
		modCtrl  = 0x0002
		modAlt   = 0x0004
	)

	// 右键（ClMain.pas:2200-2229）：Ctrl+右键 = 查看，否则移动。
	if button == 1 {
		if s.itemMove.Moving {
			s.itemMove.Cancel(s.State)
			return
		}
		if mods&modCtrl != 0 {
			s.tryInspect(x, y)
			return
		}
		// 无修饰键右键：循环重叠选择（ClMain:2201）
		if mods == 0 {
			s.dupSelection++
		}
		if y >= MapSurfaceH {
			return
		}
		if s.State.MySelf == nil || s.sendMove == nil || s.State.MySelf.Death {
			return
		}
		if s.cam == nil || s.mapData == nil {
			return
		}
		wx, wy := s.cam.ScreenToWorld(x, y)
		tx, ty := s.cam.WorldToTile(wx, wy)
		log.Logf(log.LevelDebug, "Mouse", "play right-click world=(%.1f,%.1f) tile=(%d,%d)", wx, wy, tx, ty)
		s.clearAutoPath()
		my := s.State.MySelf
		if absInt(my.CurrX-tx) <= 2 && absInt(my.CurrY-ty) <= 2 {
			dir := dirToward(my.CurrX, my.CurrY, tx, ty)
			my.UpdateMsg(protocol.CMTurn, my.CurrX, my.CurrY, dir, 0, 0)
			s.sendMove(protocol.CMTurn, dir)
			s.lastMoveActionTick = time.Now().UnixMilli()
		} else {
			s.targetX = tx
			s.targetY = ty
		}
		return
	}

	// 左键按下：双击合成（Delphi WM_LBUTTONDBLCLK）。
	dbl := false
	if button == 0 {
		now := time.Now().UnixMilli()
		if now-s.lastPressTick < 400 && absF(x-s.lastPressX) < 4 && absF(y-s.lastPressY) < 4 {
			dbl = true
			s.lastPressTick = 0
		} else {
			s.lastPressTick = now
			s.lastPressX, s.lastPressY = x, y
		}
	}
	if dbl {
		s.ui.RouteDblClick(ix, iy)
		return
	}
	if s.ui.RouteMouseDown(ix, iy, button) {
		if debugClickLog {
			if hit := s.ui.DebugHitTest(ix, iy); hit != "" {
				clickLogf("[click] UI consumed (%d,%d): %s", ix, iy, strings.Split(hit, "\n")[0])
			}
		}
		return
	}
	if button == 0 && s.dbg.WireMode > 0 {
		if s.clickInspect(x, y) {
			return
		}
	}
	if y >= MapSurfaceH {
		return
	}
	if s.State.MySelf == nil || s.sendMove == nil {
		return
	}
	if s.State.MySelf.Death {
		return
	}
	// 左键（ClMain.pas:2246-2355）：完整决策树。
	if button == 0 {
		s.autoDig = false // 任何左键操作停止自动挖矿
		if s.cam == nil || s.mapData == nil {
			return
		}
		wx, wy := s.cam.ScreenToWorld(x, y)
		tx, ty := s.cam.WorldToTile(wx, wy)
		log.Logf(log.LevelDebug, "Mouse", "play left-click world=(%.1f,%.1f) tile=(%d,%d)", wx, wy, tx, ty)

		my := s.State.MySelf
		now := time.Now().UnixMilli()

		// 1. 鹤嘴锄挖矿（ClMain:2252-2267）
		if w := s.State.UseItems[1]; w != nil && !my.OnHorse {
			if def := s.State.ItemDefs[int(w.WIndex)]; def != nil && def.Shape == 19 {
				target := s.findTargetActor(wx, wy, true)
				if target == nil {
					tdir := dirToward(my.CurrX, my.CurrY, tx, ty)
					fdx, fdy := dirOffset(tdir)
					nx, ny := my.CurrX+fdx, my.CurrY+fdy
					if !s.CanWalk(nx, ny) || s.shiftDown {
						if my.IsIdle() && s.ServerAcceptNextAction() && s.canNextHit() {
							s.lastHitTick = now
							if s.sendAttack != nil {
								s.sendAttack(protocol.CMHit+1, tdir)
							}
						}
						s.autoDig = true
						return
					}
				}
			}
		}

		// 2. Alt+左键 → 屠宰（ClMain:2269-2283）
		if s.altDown && !my.OnHorse {
			tdir := dirToward(my.CurrX, my.CurrY, tx, ty)
			if my.IsIdle() && s.ServerAcceptNextAction() {
				var corpse *Actor
				for _, a := range s.State.Actors.All() {
					if a.Death && a.Type == ActorMonster && a.Race != 0 &&
						absInt(a.CurrX-tx) <= 1 && absInt(a.CurrY-ty) <= 1 {
						corpse = a
						break
					}
				}
				if corpse != nil && s.sendButch != nil {
					s.sendButch(corpse.RecogID)
				}
				my.UpdateMsg(protocol.CMSitdown, my.CurrX, my.CurrY, tdir, 0, 0)
				s.sendMove(protocol.CMSitdown, tdir)
			}
			return
		}

		// 3. 查找目标 Actor（ClMain:2248, liveonly=TRUE）
		target := s.findTargetActor(wx, wy, true)

		if target != nil || s.shiftDown {
			// 攻击/交互路径
			s.targetX, s.targetY = -1, -1
			s.clearAutoPath()

			if target != nil {
				// NPC 对话（ClMain:2292-2298）：商人 + 静止 1.5 秒
				if target.Type == ActorNPC || target.Race == 50 {
					clickLogf("[click] NPC #%d %q tile=(%d,%d)", target.RecogID, target.UserName, tx, ty)
					if now-s.lastMoveActionTick > 1500 && s.sendNpcClick != nil {
						s.sendNpcClick(int(target.RecogID))
					}
					return
				}

				if !target.Death && !my.OnHorse {
					clickLogf("[click] attack type=%d #%d %q tile=(%d,%d)", target.Type, target.RecogID, target.UserName, tx, ty)
					s.targetCret = target
					if s.shouldAttack(target) {
						s.attackTarget(target)
					}
				}
				return
			}

			// Shift+无目标 → 空砍（ClMain:2316-2334）
			clickLogf("[click] swing (no target) tile=(%d,%d)", tx, ty)
			tdir := dirToward(my.CurrX, my.CurrY, tx, ty)
			if my.IsIdle() && s.ServerAcceptNextAction() && s.canNextHit() {
				s.lastHitTick = now
				s.lastAttackTick = now
				hitMsg := s.selectHitType()
				if s.sendAttack != nil {
					s.sendAttack(hitMsg, tdir)
				}
			}
			return
		}

		// 4. 无目标无 Shift：拾取或移动（ClMain:2336-2353）
		if tx == my.CurrX && ty == my.CurrY {
			// 点击自己格子 → 拾取
			clickLogf("[click] pickup tile=(%d,%d)", tx, ty)
			if s.sendPickup != nil {
				s.clearAutoPath()
				s.sendPickup()
			}
			return
		}

		// 攻击后 1 秒内禁止移动（ClMain:2344）
		if now-s.lastAttackTick < 1000 {
			return
		}

		clickLogf("[click] move to tile=(%d,%d)", tx, ty)
		s.targetCret = nil
		s.startAutoPath(tx, ty)
	}
}

// findTargetActor 用精灵包围盒检测查找点击命中的目标 Actor（PlayScn:1785-1818）。
// wx, wy 为点击处的世界坐标；liveOnly=true 时仅返回存活 Actor（左键用）。
// dupSelection 用于重叠 Actor 时循环选择。
//
// Delphi 原版先做像素级 CheckSelect，失败后回退到 CharWidth×CharHeight 包围盒
// （PlayScn.pas:1798-1812）。NPC/怪物精灵底部锚定、向上延伸，精确格子匹配会漏掉
// 点在身体/头部的点击，故此处采用世界空间包围盒检测。
func (s *PlayScene) findTargetActor(wx, wy float64, liveOnly bool) *Actor {
	my := s.State.MySelf
	// SortedByY 按 Ry 升序（渲染由后往前）；命中检测反序遍历，
	// 让绘制在最上层（Y 最大）的 Actor 优先被选中。
	actors := s.State.Actors.SortedByY()
	var candidates []*Actor
	for i := len(actors) - 1; i >= 0; i-- {
		a := actors[i]
		if a.RecogID == my.RecogID {
			continue
		}
		if liveOnly && a.Death {
			continue
		}
		if s.actorHitTest(a, wx, wy) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	idx := s.dupSelection % len(candidates)
	return candidates[idx]
}

// actorHitTest 判断世界坐标 (wx, wy) 是否落在 Actor 精灵的包围盒内。
// 包围盒由当前帧 body 图的宽高与热点推算，大精灵按 Delphi 规则裁去透明边距。
func (s *PlayScene) actorHitTest(a *Actor, wx, wy float64) bool {
	worldX := float64(a.Rx*engine.TileWidth) + a.ShiftX
	worldY := float64(a.Ry*engine.TileHeight) + a.ShiftY

	var w, h, hotX, hotY float64
	if img := a.GetBodyImage(s.resources); img != nil {
		w = float64(img.Width)
		h = float64(img.Height)
		hotX = float64(img.HotX)
		hotY = float64(img.HotY)
	} else {
		// 取图失败时的兜底盒子（约一格宽、成人身高）。
		w, h = engine.TileWidth, 70
		hotX, hotY = -engine.TileWidth/2, -70
	}

	left := worldX + hotX
	top := worldY + hotY
	right := left + w
	bottom := top + h

	// Delphi PlayScn.pas:1803-1806：超出 40×70 的精灵裁去居中边距，
	// 避免点中大片透明区域。
	var centx, centy float64
	if w > 40 {
		centx = (w - 40) / 2
	}
	if h > 70 {
		centy = (h - 70) / 2
	}

	return wx >= left+centx && wx <= right-centx && wy >= top+centy && wy <= bottom-centy
}

// shouldAttack 判断是否应攻击目标（ClMain:2300-2314）。
func (s *PlayScene) shouldAttack(a *Actor) bool {
	if a.Type == ActorMonster {
		return true
	}
	if a.Type == ActorNPC {
		return false
	}
	// 人类玩家
	if a.Race == 0 { // RCC_USERHUMAN
		if s.shiftDown {
			return true
		}
		if a.NameColor == 1 { // 红名 = 敌人
			return true
		}
		return false
	}
	if a.Race == 12 { // RCC_GUARD
		return false
	}
	if a.Race == 50 { // RCC_MERCHANT
		return false
	}
	// 召唤宠物：名字包含 "("
	if len(a.UserName) > 0 && a.UserName[0] == '(' {
		return false
	}
	return true
}

// attackTarget 攻击目标（ClMain:2128-2180）。
// 相邻 → 发送攻击消息；不相邻 → 走向目标背后。
func (s *PlayScene) attackTarget(a *Actor) {
	my := s.State.MySelf
	dir := dirToward(my.CurrX, my.CurrY, a.CurrX, a.CurrY)
	dx := a.CurrX - my.CurrX
	dy := a.CurrY - my.CurrY

	if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 && !a.Death {
		if !my.IsIdle() || !s.ServerAcceptNextAction() || !s.canNextHit() {
			return
		}
		now := time.Now().UnixMilli()
		s.lastHitTick = now
		s.lastAttackTick = now
		hitMsg := s.selectHitType()
		if s.sendAttack != nil {
			s.sendAttack(hitMsg, dir)
		}
	} else {
		// 不相邻 → 走向目标背后（ClMain:2166-2179）
		s.targetCret = a
		backDir := (a.Dir + 4) % 8
		bdx, bdy := dirOffset(backDir)
		s.targetX = a.CurrX + bdx
		s.targetY = a.CurrY + bdy
		s.clearAutoPath()
	}
}

// selectHitType 特殊攻击优先级链（ClMain:2136-2163）。
func (s *PlayScene) selectHitType() int {
	// 烈火剑法（一次性，10 秒冷却）
	if s.canFireHit && s.State.MP >= 7 {
		s.canFireHit = false
		return protocol.CMFireHit
	}
	// 攻杀剑术（一次性）
	if s.canPowerHit {
		s.canPowerHit = false
		return protocol.CMPowerHit
	}
	// 双龙斩
	if s.canTwnHit && s.State.MP >= 10 {
		s.canTwnHit = false
		return protocol.CMTwinHit
	}
	// 半月弯刀
	if s.canWideHit && s.State.MP >= 3 {
		return protocol.CMWideHit
	}
	// 十字斩
	if s.canCrsHit && s.State.MP >= 6 {
		return protocol.CMCrsHit
	}
	// 刺杀剑术
	if s.canLongHit {
		return protocol.CMLongHit
	}
	// 普通攻击：StdMode=6 的武器用重击
	if w := s.State.UseItems[1]; w != nil {
		if def := s.State.ItemDefs[int(w.WIndex)]; def != nil && def.StdMode == 6 {
			return protocol.CMHeavyHit
		}
	}
	return protocol.CMHit
}

// canNextHit 攻击速度限制（ClMain:3415-3432）。
func (s *PlayScene) canNextHit() bool {
	now := time.Now().UnixMilli()
	levelFast := s.State.Level * 14
	if levelFast > 370 {
		levelFast = 370
	}
	levelFast += s.State.Hit * 60
	if levelFast > 800 {
		levelFast = 800
	}
	nextHitTime := int64(1400 - levelFast)
	if s.attackSlow {
		nextHitTime += 1500
	}
	if s.State.Weight > s.State.MaxWeight {
		nextHitTime *= 2
	}
	return now-s.lastHitTick >= nextHitTime
}

func dirToward(fromX, fromY, toX, toY int) int {
	dx := toX - fromX
	dy := toY - fromY
	if dx == 0 && dy == 0 {
		return 0
	}
	if dy < 0 {
		if dx < 0 {
			return 7
		}
		if dx > 0 {
			return 1
		}
		return 0
	}
	if dy > 0 {
		if dx < 0 {
			return 5
		}
		if dx > 0 {
			return 3
		}
		return 4
	}
	if dx < 0 {
		return 6
	}
	return 2
}

func (s *PlayScene) OnScroll(offX, offY float64) {
	// NPC 对话框滚动
	if s.State.ShowNpcDialog && s.hudNpc != nil {
		ax, ay := s.hudNpc.AbsX(), s.hudNpc.AbsY()
		if int(s.mouseX) >= ax && int(s.mouseX) <= ax+s.hudNpc.Width &&
			int(s.mouseY) >= ay && int(s.mouseY) <= ay+s.hudNpc.Height {
			maxVisible := (s.hudNpc.Height - npcTextY - 10) / npcLineH
			maxScroll := len(s.npcLines) - maxVisible
			if maxScroll < 0 {
				maxScroll = 0
			}
			s.npcScrollOffset -= int(offY)
			if s.npcScrollOffset < 0 {
				s.npcScrollOffset = 0
			}
			if s.npcScrollOffset > maxScroll {
				s.npcScrollOffset = maxScroll
			}
			return
		}
	}
	// 滚轮在聊天板上滚动聊天记录。
	if s.mouseX >= chatBoardX && s.mouseX <= chatBoardX+474 &&
		s.mouseY >= float64(chatBoardTop) && s.mouseY < float64(chatBoardTop+chatLineH*ViewChatLine) {
		maxScroll := len(s.chatMessages) - ViewChatLine
		if maxScroll < 0 {
			maxScroll = 0
		}
		s.chatScroll -= int(offY) // 向上滚（offY>0）回看历史
		if s.chatScroll < 0 {
			s.chatScroll = 0
		}
		if s.chatScroll > maxScroll {
			s.chatScroll = maxScroll
		}
		return
	}
}

func (s *PlayScene) AddChatMessage(text string) {
	s.chatMessages = append(s.chatMessages, ChatMessage{Text: text, Time: time.Now().UnixMilli()})
	if len(s.chatMessages) > 100 {
		s.chatMessages = s.chatMessages[len(s.chatMessages)-100:]
	}
}

func (s *PlayScene) drawWilImage(f *wil.File, idx int, x, y float32, proj [16]float32) bool {
	if f == nil || idx < 0 || idx >= f.Count {
		return false
	}
	img := f.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return false
	}
	tex := s.resources.GetTexture(f, idx)
	if tex == 0 {
		return false
	}
	s.gl.DrawQuad(tex, x, y, float32(img.Width), float32(img.Height), proj)
	return true
}

func (s *PlayScene) RenderUI(proj [16]float32) {
	if s.text == nil {
		return
	}

	if s.State.MapTitle != "" && s.State.MySelf != nil {
		title := fmt.Sprintf("%s %d:%d", s.State.MapTitle, s.State.MySelf.CurrX, s.State.MySelf.CurrY)
		s.text.DrawText(title, 8, 580, 1, 1, 1, 1, proj)
	}

	// 底栏、HP/MP 球、按钮、腰带、聊天、背包、状态面板——全部由
	// 控件树（uihud.go / uibag.go / uistate.go）通过下方 s.ui.Paint 绘制。
	s.syncBagWindow()
	s.syncStateWindow()
	s.syncMerchantWindows()
	s.syncDealWindows()
	s.syncGuildWindows()
	s.syncAbilWindows()

	// UI 控件树（DWinCtl 移植）绘制在旧版手绘面板之上；
	// 旧版面板正逐步迁移到控件树中。
	s.ui.Paint(proj)

	// 先画提示面板，再画光标上的手持物品——Delphi 绘制顺序
	// （ClMain.pas: DrawHint :1079, 手持物品 :1093）。
	s.tooltip.Render(s, proj)
	s.renderHeldItem(proj)
}

// renderHeldItem 在光标中心绘制拖拽的物品并显示名称标签
// （ClMain.pas:1093-1113）。
func (s *PlayScene) renderHeldItem(proj [16]float32) {
	if !s.itemMove.Moving {
		return
	}
	var looks int
	var name string
	if s.itemMove.Index == moveIdxBagGold || s.itemMove.Index == moveIdxDealGold {
		looks = ItemImgGold
		name = "Gold"
	} else {
		looks = int(s.itemMove.Item.Looks())
		if s.itemMove.Item.Def != nil {
			name = s.itemMove.Item.Def.Name
		}
	}
	if s.resources.Items != nil && looks >= 0 && looks < s.resources.Items.Count {
		img := s.resources.Items.GetImage(looks)
		tex := s.resources.GetTexture(s.resources.Items, looks)
		if img != nil && img.RGBA != nil && tex != 0 {
			s.gl.DrawQuad(tex, float32(s.mouseX)-float32(img.Width)/2,
				float32(s.mouseY)-float32(img.Height)/2,
				float32(img.Width), float32(img.Height), proj)
		}
	}
	if name != "" && s.text != nil {
		s.text.DrawText(name, float32(s.mouseX)+9, float32(s.mouseY)+3, 1, 1, 0, 1, proj)
	}
}

func (s *PlayScene) addFloatingText(tileX, tileY int, text string, r, g, b float32) {
	s.floatingTexts = append(s.floatingTexts, FloatingText{
		Text:      text,
		X:         float32(tileX*48 + 24),
		Y:         float32(tileY * 32),
		Color:     [4]float32{r, g, b, 1.0},
		StartTime: time.Now().UnixMilli(),
	})
}
