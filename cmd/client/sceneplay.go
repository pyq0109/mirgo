package main

import (
	"fmt"
	"path/filepath"
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
	gl        *engine.GLState
	resources *engine.ResourceManager
	mapDir    string
	cam       *engine.Camera2D
	mapData   *mapformat.MapData
	minimap      *Minimap
	minimapDirty bool
	lighting     *LightingSystem
	lightingDirty bool

	texCache       map[int]uint32
	smTexCache     map[int]uint32
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int

	State        *GameState
	sendMove     func(ident int, dir int)
	sendAttack   func(ident int, dir int)
	sendPickup   func()
	sendChat     func(text string)
	sendSpell    func(magID int, x, y int)
	sendNpcClick   func(npcID int)
	sendDealCancel func()
	sendUseItem    func(makeIndex int32) // CMEat，通过 MakeIndex 定位物品
	sendBuyItem    func(itemIdx int)
	sendSellItem   func(makeIndex int32) // CMUserSellItem，通过 MakeIndex 定位
	sendDropItem   func(makeIndex int32) // CMDropItem，通过 MakeIndex 定位
	sendDropGold   func(amount int)      // CMDropGold，数量放 Recog 字段
	sendDealTry    func()                // CMDealTry
	sendTakeOn     func(makeIndex int32, slot int) // CMTakeOnItem
	sendTakeOff       func(slot int)               // CMTakeOffItem
	sendMagicKey      func(magID, key int)         // CMMagicKeyChange
	sendMerchantSelect func(npcID int32, tag string) // CMMerchantDlgSelect
	sendQueryPrice    func(makeIndex int32)        // CMMerchantQuerySellPrice
	sendQueryRepair   func(makeIndex int32)        // CMMerchantQueryRepairCost
	sendRepairItem    func(makeIndex int32)        // CMUserRepairItem
	sendStorageItem   func(makeIndex int32)        // CMUserStorageItem
	sendTakeBackStorage func(makeIndex int32)      // CMUserTakeBackStorageItem
	sendDealAdd       func(makeIndex int32)        // CMDealAddItem
	sendDealDel       func(makeIndex int32)        // CMDealDelItem
	sendDealChgGold   func(amount int)             // CMDealChgGold
	sendDealEnd       func()                       // CMDealEnd
	sendOpenGuild        func()                    // CMOpenGuildDlg
	sendGuildMemberList  func()                    // CMGuildMemberList
	sendGuildAdd         func(name string)         // CMGuildAddMember
	sendGuildDel         func(name string)         // CMGuildDelMember
	sendGuildUpdateNotice func(text string)        // CMGuildUpdateNotice
	sendGuildUpdateRank  func(text string)         // CMGuildUpdateRankInfo
	sendGuildAlly        func(name string)         // CMGuildAlly
	sendGuildBreakAlly   func(name string)         // CMGuildBreakAlly
	sendGuildHome        func()                    // CMGuildHome
	sendGroupMode        func(allow int)           // CMGroupMode
	sendCreateGroup      func(name string)         // CMCreateGroup
	sendAddGroupMember   func(name string)         // CMAddGroupMember
	sendDelGroupMember   func(name string)         // CMDelGroupMember
	sendAdjustBonus      func(remaining int, deltas [9]int) // CMAdjustBonus
	sendQueryUserState   func(targetID int32)               // CMQueryUserState
	sendAttackMode func(mode int)
	lastMoveTick   int64
	lastAniTick    int64
	text         *engine.TextRenderer

	groundItems   map[int32]*GroundItemInfo
	floatingTexts []FloatingText
	chatMessages  []ChatMessage
	chatInput     string
	chatMode      bool

	ActionLock     bool
	ActionLockTime int64

	moveFailCount int

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
	npcClicks                []npcClickPoint
	npcSelectTag             string
	npcLastClickTick         int64
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
	lastPressTick        int64
	lastPressX, lastPressY float64

	targetX, targetY int

	autoPath    [][2]int
	autoPathIdx int

	showMinimap bool
	deathGray   bool
	focusActor  *Actor

	effects *EffectManager
	events  *EventManager
}

func NewPlayScene(gl *engine.GLState, resources *engine.ResourceManager, mapDir string) *PlayScene {
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

func (s *PlayScene) SetSendAttack(fn func(ident int, dir int)) {
	s.sendAttack = fn
}

func (s *PlayScene) SetSendPickup(fn func()) {
	s.sendPickup = fn
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

func (s *PlayScene) SetSendOpenGuild(fn func())                          { s.sendOpenGuild = fn }
func (s *PlayScene) SetSendGuildMemberList(fn func())                    { s.sendGuildMemberList = fn }
func (s *PlayScene) SetSendGuildAdd(fn func(name string))                { s.sendGuildAdd = fn }
func (s *PlayScene) SetSendGuildDel(fn func(name string))                { s.sendGuildDel = fn }
func (s *PlayScene) SetSendGuildUpdateNotice(fn func(text string))       { s.sendGuildUpdateNotice = fn }
func (s *PlayScene) SetSendGuildUpdateRank(fn func(text string))         { s.sendGuildUpdateRank = fn }
func (s *PlayScene) SetSendGuildAlly(fn func(name string))               { s.sendGuildAlly = fn }
func (s *PlayScene) SetSendGuildBreakAlly(fn func(name string))          { s.sendGuildBreakAlly = fn }
func (s *PlayScene) SetSendGuildHome(fn func())                          { s.sendGuildHome = fn }
func (s *PlayScene) SetSendGroupMode(fn func(allow int))                 { s.sendGroupMode = fn }
func (s *PlayScene) SetSendCreateGroup(fn func(name string))             { s.sendCreateGroup = fn }
func (s *PlayScene) SetSendAddGroupMember(fn func(name string))          { s.sendAddGroupMember = fn }
func (s *PlayScene) SetSendDelGroupMember(fn func(name string))          { s.sendDelGroupMember = fn }
func (s *PlayScene) SetSendAdjustBonus(fn func(remaining int, deltas [9]int)) {
	s.sendAdjustBonus = fn
}
func (s *PlayScene) SetSendQueryUserState(fn func(targetID int32)) { s.sendQueryUserState = fn }

func (s *PlayScene) SetSendAttackMode(fn func(mode int)) {
	s.sendAttackMode = fn
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
		// 地图渲染区域为 800×445（Delphi MAPSURFACEHEIGHT, Share.pas:31）；
		// 底部 155px 属于 HUD 栏。
		s.cam = engine.NewCamera(ScreenWidth, MapSurfaceH)
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

func (s *PlayScene) Open() {
	log.Logf(log.LevelInfo, "PlayScene", "opened")
}

func (s *PlayScene) Close() {
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
		// Delphi 将玩家固定在屏幕 (388, 208)（800×445 视口内），
		// 而非几何中心 (400, 222.5)。常量来自 PlayScn.pas:1741-1748
		// 反推公式：
		//   sx = (cx-Rx)*48 + 364 + 24 - ShiftX  →  cx==Rx 时为 388
		//   sy = (cy-Ry)*32 + 192 + 16 - ShiftY  →  cy==Ry 时为 208
		// (388,208) 锚点是屏幕像素；ScreenToWorld 会除以 Zoom
		// （camera.go:28-30），所以世界偏移也要缩放——
		// 否则 Zoom != 1 时玩家会偏离锚点。
		s.cam.X = wx - 388/s.cam.Zoom
		s.cam.Y = wy - 208/s.cam.Zoom
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
			} else {
				s.targetX = -1
				s.targetY = -1
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
}

// repairAutoPath 在偏离路径（+FAIL 回滚）或下一格被占（其他角色
// 走入）时从当前位置重新寻路；无可行路径返回 nil。
func (s *PlayScene) repairAutoPath(dest [2]int, my *Actor) [][2]int {
	wp := s.autoPath[s.autoPathIdx]
	if absInt(wp[0]-my.CurrX) <= 1 && absInt(wp[1]-my.CurrY) <= 1 && s.CanWalk(wp[0], wp[1]) {
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

	m := s.mapData
	cam := s.cam

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// 世界渲染到上方 445 逻辑行；底部 155px 是 HUD。
	fbW, fbH := s.gl.ViewW, s.gl.ViewH
	worldH := int32(float64(MapSurfaceH) * float64(fbH) / float64(ScreenHeight))
	s.gl.SetViewport(0, fbH-worldH, fbW, worldH)

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

	s.renderFrontWithActors(fStartX, fStartY, fEndX, fEndY, proj)
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

	if s.lighting != nil && !s.deathGray {
		darkness := s.calcDarkness()
		if darkness > 0.01 {
			lights := s.collectLightSources()
			s.lighting.Render(proj, s.cam.X, s.cam.Y, s.cam.ViewW, s.cam.ViewH, s.cam.Zoom, darkness, lights)
		}
	}

	// UI 层使用完整的 800×600 逻辑视口。
	s.gl.SetViewport(0, 0, fbW, fbH)
	uiProj := engine.OrthoProj(ScreenWidth, ScreenHeight)
	if s.showMinimap {
		mmapDrawn := false
		mmIdx := s.State.MinimapIndex
		if s.resources.Mmap != nil && mmIdx >= 0 && mmIdx < s.resources.Mmap.Count {
			mmImg := s.resources.Mmap.GetImage(mmIdx)
			if mmImg != nil && mmImg.RGBA != nil {
				mmTex := s.resources.GetTexture(s.resources.Mmap, mmIdx)
				if mmTex != 0 {
					// Delphi: (SCREENWIDTH-120, 0), 120×120（PlayScn.pas:791-842）。
					log.Logf(log.LevelTrace, "Render", "play minimap Mmap[%d] pos=(%d,0) size=(120,120)", mmIdx, ScreenWidth-120)
					s.gl.DrawQuad(mmTex, ScreenWidth-120, 0, 120, 120, uiProj)
					mmapDrawn = true
				}
			}
		}
		if !mmapDrawn && s.minimap != nil {
			glState.DrawQuad(s.minimap.GetTexture(), ScreenWidth-120, 0, 120, 120, uiProj)
		}
		// 在小地图上叠加周边角色雷达点（以自身为中心）。
		if s.minimap != nil && s.State.MySelf != nil {
			s.minimap.DrawActorDots(s.gl, s.State.Actors.All(), s.State.MySelf.CurrX, s.State.MySelf.CurrY, uiProj)
		}
	}
	s.RenderUI(uiProj)
}

func (s *PlayScene) renderFrontWithActors(fStartX, fStartY, fEndX, fEndY int, proj [16]float32) {
	// 阶段 A：先绘制 48×32 地面层前景物件——它们始终在所有角色后面
	// （PlayScn.pas:1064-1108）。
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
		for x := fStartX; x <= fEndX; x++ {
			info := s.mapData.InfoAt(x, y)
			s.drawFrontLarge(info, x, y, proj)
		}

		s.events.RenderRow(s.gl, s.resources, proj, y)

		for _, gi := range s.groundItems {
			if gi.Y == y && gi.X >= fStartX && gi.X <= fEndX {
				s.drawGroundItemIcon(gi, proj)
			}
		}

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

	for ; actorIdx < len(actors); actorIdx++ {
		a := actors[actorIdx]
		worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
		worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
		a.Draw(s.gl, s.resources, worldX, worldY, proj)
		s.drawActorLabel(a, worldX, worldY, proj)
		s.drawChatBubble(a, worldX, worldY, proj)
	}

	// 阶段 C：覆盖层重绘和地面物品闪烁/名称
	// （PlayScn.pas:1290-1396）。
	if s.State.MySelf != nil && !s.State.MySelf.Death {
		my := s.State.MySelf
		wx := float32(float64(my.Rx*engine.TileWidth) + my.ShiftX)
		wy := float32(float64(my.Ry*engine.TileHeight) + my.ShiftY)
		log.Logf(log.LevelTrace, "Render", "play self redraw pos=(%.0f,%.0f) dir=%d", wx, wy, my.Dir)
		my.Draw(s.gl, s.resources, wx, wy, proj)
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
		s.text.DrawText(a.UserName, nameX-1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX+1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY-1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY+1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY, 1.0, 1.0, 1.0, 1.0, proj)
	}

	// 所有可见角色的血条（DrawScrn.pas:280-301），位于 SayY - 10。
	if !a.Death && s.resources.Prguse2 != nil {
		bgImg := s.resources.Prguse2.GetImage(0)
		fillImg := s.resources.Prguse2.GetImage(1)
		if bgImg != nil && bgImg.RGBA != nil && fillImg != nil {
			bgTex := s.resources.GetTexture(s.resources.Prguse2, 0)
			fillTex := s.resources.GetTexture(s.resources.Prguse2, 1)
			hpBarW := float32(bgImg.Width)
			hpBarH := float32(bgImg.Height)
			hpBarX := worldX + float32(engine.TileWidth)/2 - hpBarW/2
			hpBarY := sayY - 10
			if bgTex != 0 {
				log.Logf(log.LevelTrace, "Render", "play hp bar bg Prguse2[0] pos=(%.0f,%.0f) size=(%.0f,%.0f)", hpBarX, hpBarY, hpBarW, hpBarH)
				s.gl.DrawQuad(bgTex, hpBarX, hpBarY, hpBarW, hpBarH, proj)
			}
			ratio := float32(1.0)
			if s.State.MySelf != nil && a.RecogID == s.State.MySelf.RecogID && s.State.MaxHP > 0 {
				ratio = float32(s.State.HP) / float32(s.State.MaxHP)
			}
			if fillTex != 0 && ratio > 0 {
				fillW := hpBarW * ratio
				s.gl.DrawQuad(fillTex, hpBarX, hpBarY, fillW, hpBarH, proj)
			}
		} else {
			hpBarY := sayY - 10
			s.gl.DrawQuadColor(worldX+4, hpBarY, 40, 4, 0.1, 0.0, 0.0, 0.8, proj)
			s.gl.DrawQuadColor(worldX+4, hpBarY, 40, 4, 0.8, 0.0, 0.0, 0.8, proj)
		}
	} else if !a.Death {
		hpBarY := sayY - 10
		s.gl.DrawQuadColor(worldX+4, hpBarY, 40, 4, 0.1, 0.0, 0.0, 0.8, proj)
		s.gl.DrawQuadColor(worldX+4, hpBarY, 40, 4, 0.8, 0.0, 0.0, 0.8, proj)
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

func (s *PlayScene) ServerAcceptNextAction() bool {
	if !s.ActionLock {
		return true
	}
	if time.Now().UnixMilli()-s.ActionLockTime > 10000 {
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
		case 71: // G — 行会（ClMain:1637-1661）
			s.toggleGuild()
			return
		case 72: // H — 切换攻击模式（ClMain:1517-1525）
			s.State.AttackMode = (s.State.AttackMode + 1) % 5
			if s.sendAttackMode != nil {
				s.sendAttackMode(s.State.AttackMode)
			}
			modes := []string{"和平", "组队", "行会", "全体", "PK"}
			s.AddChatMessage("[系统] 攻击模式: " + modes[s.State.AttackMode])
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
		case 86: // V — 好友对话框（ClMain:1687-1690；服务端未实现）
			s.AddChatMessage("好友: 尚未实现")
			return
		case 87: // W — 交易（ClMain:1663-1666）
			s.tryDeal()
			return
		case 90: // Z — 拾取物品（ClMain:1564-1573）
			if s.sendPickup != nil {
				s.sendPickup()
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

		// F1..F8 释放绑定在该键上的魔法（FState:3506-3545）。
		if !s.chatMode && key >= 290 && key <= 297 {
			k := byte('1' + (key - 290))
			for i := range s.State.Magics {
				if s.State.Magics[i].Key != k {
					continue
				}
				if s.sendSpell != nil {
					if my := s.State.MySelf; my != nil {
						dx, dy := dirOffset(my.Dir)
						s.sendSpell(int(s.State.Magics[i].MagID), my.CurrX+dx, my.CurrY+dy)
					}
				}
				break
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
	if y >= MapSurfaceH || s.cam == nil || s.State.MySelf == nil {
		return
	}
	wx, wy := s.cam.ScreenToWorld(x, y)
	tx, ty := s.cam.WorldToTile(wx, wy)
	for _, a := range s.State.Actors.All() {
		if a.RecogID == s.State.MySelf.RecogID || a.Death {
			continue
		}
		if a.CurrX == tx && a.CurrY == ty {
			s.focusActor = a
			return
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

	if action == 0 { // 松开
		s.ui.RouteMouseUp(ix, iy, button)
		return
	}
	if action != 1 {
		return
	}

	const (
		modShift = 0x0001
		modCtrl  = 0x0002
		modAlt   = 0x0004
	)
	_ = modAlt // 占位，暂未使用

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
		log.Logf(log.LevelDebug, "PlayScene", "mouse event consumed by UI pos=(%d,%d)", ix, iy)
		return
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
	// 左键（ClMain.pas:2246-2275）：攻击 / NPC 对话 / 拾取。
	if button == 0 {
		if s.cam == nil || s.mapData == nil {
			return
		}
		wx, wy := s.cam.ScreenToWorld(x, y)
		tx, ty := s.cam.WorldToTile(wx, wy)
		log.Logf(log.LevelDebug, "Mouse", "play left-click world=(%.1f,%.1f) tile=(%d,%d)", wx, wy, tx, ty)

		my := s.State.MySelf
		for _, a := range s.State.Actors.All() {
			if a.RecogID == my.RecogID {
				continue
			}
			if a.CurrX == tx && a.CurrY == ty && !a.Death {
				s.clearAutoPath()
				if a.Type == ActorNPC {
					if s.sendNpcClick != nil {
						s.sendNpcClick(int(a.RecogID))
					}
					return
				}
				dir := dirToward(my.CurrX, my.CurrY, a.CurrX, a.CurrY)
				dx := a.CurrX - my.CurrX
				dy := a.CurrY - my.CurrY
				if dx >= -1 && dx <= 1 && dy >= -1 && dy <= 1 {
					now := time.Now().UnixMilli()
					cooldown := int64(1400 - s.State.Speed*100)
					if cooldown < 500 {
						cooldown = 500
					} else if cooldown > 2800 {
						cooldown = 2800
					}
					if s.State.Weight > s.State.MaxWeight {
						cooldown *= 2
					}
					if now-s.lastHitTick < cooldown {
						return
					}
					s.lastHitTick = now
					if s.sendAttack != nil {
						s.sendAttack(protocol.CMHit, dir)
					}
				} else {
					my.UpdateMsg(protocol.CMTurn, my.CurrX, my.CurrY, dir, 0, 0)
					s.sendMove(protocol.CMTurn, dir)
				}
				return
			}
		}

		if s.sendPickup != nil {
			for _, gi := range s.groundItems {
				if gi.X == tx && gi.Y == ty {
					s.clearAutoPath()
					s.sendPickup()
					return
				}
			}
		}

		s.startAutoPath(tx, ty)
	}
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
	// 滚轮在聊天板上滚动聊天记录；其他区域缩放视角
	// （Delphi 从 PlayScn 滚动 ChatBoardTop）。
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
	if s.cam != nil {
		if offY > 0 {
			s.cam.ZoomAt(1.1, s.mouseX, s.mouseY)
		} else {
			s.cam.ZoomAt(1/1.1, s.mouseX, s.mouseY)
		}
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


