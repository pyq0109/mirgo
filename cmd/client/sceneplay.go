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
	frontCullMargin = 35 // Delphi LONGHEIGHT_IMAGE (PlayScn.pas:17)
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
	sendUseItem    func(makeIndex int32) // CMEat, item addressed by MakeIndex
	sendBuyItem    func(itemIdx int)
	sendSellItem   func(makeIndex int32) // CMUserSellItem, addressed by MakeIndex
	sendDropItem   func(makeIndex int32) // CMDropItem, addressed by MakeIndex
	sendDropGold   func(amount int)      // CMDropGold, amount in Recog
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
	text         *engine.TextRenderer

	groundItems   map[int32]*GroundItemInfo
	floatingTexts []FloatingText
	chatMessages  []ChatMessage
	chatInput     string
	chatMode      bool

	ActionLock     bool
	ActionLockTime int64

	actionFailLock     bool
	actionFailLockTime int64

	lastHitTick int64

	// UI framework (DWinCtl port): panels, buttons, grids, modals.
	ui            *UIManager
	itemMove      ItemMoveState
	tooltip       Tooltip
	hudPlusAbil   *UIControl
	hudBag        *UIControl
	bagHoverItem  *BagItem // hovered bag item for the in-window info area (FState:4465)
	hudState      *UIControl
	stateSlotBtns [13]*UIControl
	stateMagBtns  [5]*UIControl
	statePageUp   *UIControl
	statePageDown *UIControl
	magicPage     int
	chatScroll    int // lines the chat board is scrolled back from newest

	// Trade windows (uideal.go).
	hudDealOwn, hudDealRemote *UIControl
	dealActionTick            int64

	// Guild + group panels (uiguild.go).
	hudGuild, hudGroup *UIControl
	guildAdminBtns     []*UIControl
	guildChatMode      bool
	guildChats         []string // guild chat buffer, 500 cap / trim 100 (FState:6465-6475)
	guildActionTick    int64    // party ops 5s shared gate (FState:5514)
	guildQueryTick     int64    // guild Home/List 3s gate (FState:6370)

	// Adjust-ability + inspect windows (uiabil.go).
	hudAbil, hudInspect *UIControl
	abilDeltas          [9]int
	abilPointsLeft      int
	showInspect         bool
	inspectItems        [13]*protocol.UserItem
	inspectName         string
	inspectSex          int
	inspectHair         int
	ctrlDown            bool // Ctrl held (adjust panel ×10, FState:6638)

	// NPC dialog + shop state (uinpc.go).
	hudNpc, hudMenu, hudSell *UIControl
	npcLines                 [][]npcSegment
	npcClicks                []npcClickPoint
	npcSelectTag             string
	npcLastClickTick         int64
	menuTop                  int
	menuIndex                int
	lastBuyTick              int64
	sellItem                 *BagItem
	sellWait                 *BagItem // item pending server sell confirmation
	sellPriceStr             string
	queryPrice               bool
	queryPriceTick           int64
	merchantWasOpen          bool

	// Cursor position in logical 800×600 space (updated on every move).
	mouseX, mouseY float64
	// Double-click synthesis (GLFW has no native event): Delphi gets
	// WM_LBUTTONDBLCLK; we detect two left presses <400ms and <4px apart.
	lastPressTick        int64
	lastPressX, lastPressY float64

	targetX, targetY int

	showMinimap bool
	deathGray   bool

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
	// Clicking empty space while holding an item drops it on the ground
	// (FState.pas:1865-1886 DBackgroundBackgroundClick). WantReturn stays
	// false so the click still reaches the game world when nothing is held.
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

// backgroundClick handles clicks that fell through every control. It
// reports whether the click was consumed (an item/gold drop), which sets
// WantReturn so the world layer ignores the same click (FState:1865-1894).
func (s *PlayScene) backgroundClick() bool {
	if !s.itemMove.Moving {
		return false
	}
	switch {
	case s.itemMove.Index >= 0:
		// Drop the item on the ground (FState.pas:1842-1854).
		if s.sendDropItem != nil {
			s.sendDropItem(s.itemMove.Item.MakeIndex)
			if slot := s.State.FindBagItemByMakeIndex(s.itemMove.Item.MakeIndex); slot >= 0 {
				s.State.BagItems[slot] = nil
			}
			s.itemMove.End()
			return true
		}
	case s.itemMove.Index == moveIdxBagGold:
		// Drop gold: ask the amount (FState.pas:1870-1882).
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

func (s *PlayScene) AddChatMessage(text string) {
	s.chatMessages = append(s.chatMessages, ChatMessage{Text: text, Time: time.Now().UnixMilli()})
	if len(s.chatMessages) > 10 {
		s.chatMessages = s.chatMessages[1:]
	}
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
	if s.cam == nil {
		// Map surface is 800×445 (Delphi MAPSURFACEHEIGHT, Share.pas:31);
		// the bottom 155px belong to the HUD bar.
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

	log.Logf(log.LevelInfo, "PlayScene", "Loaded map: %s (%dx%d)", mapName, m.Width, m.Height)
	return nil
}

func (s *PlayScene) Open() {
	log.Logf(log.LevelInfo, "PlayScene", "Opened")
}

func (s *PlayScene) Close() {
	s.State.Reset()
	log.Logf(log.LevelInfo, "PlayScene", "Closed")
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

	s.State.Actors.Update(now, moveTick)
	s.effects.Update(now)
	s.events.Update(now)
	s.pumpSellQuery()

	if s.State.MySelf != nil && s.cam != nil && s.mapData != nil {
		my := s.State.MySelf
		wx := float64(my.Rx)*engine.TileWidth + my.ShiftX + engine.TileWidth/2
		wy := float64(my.Ry)*engine.TileHeight + my.ShiftY + engine.TileHeight/2
		// Delphi places the player at screen (388, 208) in the 800×445
		// viewport, not the geometric center (400, 222.5). The constants
		// come from PlayScn.pas:1741-1748 reverse formula:
		//   sx = (cx-Rx)*48 + 364 + 24 - ShiftX  →  388 when cx==Rx
		//   sy = (cy-Ry)*32 + 192 + 16 - ShiftY  →  208 when cy==Ry
		// The (388,208) anchor is in screen pixels; ScreenToWorld divides by
		// Zoom (camera.go:28-30), so the world offset must be scaled too —
		// otherwise the player drifts off-anchor whenever Zoom != 1.
		s.cam.X = wx - 388/s.cam.Zoom
		s.cam.Y = wy - 208/s.cam.Zoom
		s.cam.ClampToBounds(s.mapData.Width, s.mapData.Height)
	}

	if s.targetX >= 0 && s.State.MySelf != nil && moveTick && s.sendMove != nil && !s.State.MySelf.Death {
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
				// Mounted run advances 3 tiles; the server requires all three
				// tiles ahead to be walkable (HandleHorseRun x1/x2/x3). The
				// first tile is already validated by CanWalk(nx, ny) above.
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

func (s *PlayScene) Render(glState *engine.GLState, proj [16]float32) {
	if s.mapData == nil || s.cam == nil {
		return
	}

	m := s.mapData
	cam := s.cam

	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// World renders into the top 445 logical rows; the bottom 155px is HUD.
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

	s.events.Render(s.gl, s.resources, proj)
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
		log.Logf(log.LevelTrace, "Render", "play death-gray viewport=(%.0f,%.0f,%.0f,%.0f)",
			s.cam.X, s.cam.Y, float64(s.cam.ViewW)/s.cam.Zoom, float64(s.cam.ViewH)/s.cam.Zoom)
		s.gl.DrawQuadColor(float32(s.cam.X), float32(s.cam.Y),
			float32(float64(s.cam.ViewW)/s.cam.Zoom), float32(float64(s.cam.ViewH)/s.cam.Zoom),
			0.3, 0.3, 0.3, 0.4, proj)
	}

	if s.lighting != nil && !s.deathGray {
		darkness := s.calcDarkness()
		if darkness > 0.01 {
			lights := s.collectLightSources()
			s.lighting.Render(proj, s.cam.X, s.cam.Y, s.cam.ViewW, s.cam.ViewH, s.cam.Zoom, darkness, lights)
		}
	}

	s.animCounter++

	// UI layers use the full viewport in 800×600 logical space.
	s.gl.SetViewport(0, 0, fbW, fbH)
	uiProj := engine.OrthoProj(ScreenWidth, ScreenHeight)
	if s.showMinimap {
		mmapDrawn := false
		if s.resources.Mmap != nil && s.resources.Mmap.Count > 0 {
			mmImg := s.resources.Mmap.GetImage(0)
			if mmImg != nil && mmImg.RGBA != nil {
				mmTex := s.resources.GetTexture(s.resources.Mmap, 0)
				if mmTex != 0 {
					// Delphi: (SCREENWIDTH-120, 0), 120×120 (PlayScn.pas:791-842).
					log.Logf(log.LevelTrace, "Render", "play minimap Mmap[0] pos=(%d,0) size=(120,120)", ScreenWidth-120)
					s.gl.DrawQuad(mmTex, ScreenWidth-120, 0, 120, 120, uiProj)
					mmapDrawn = true
				}
			}
		}
		if !mmapDrawn && s.minimap != nil {
			glState.DrawQuad(s.minimap.GetTexture(), ScreenWidth-120, 0, 120, 120, uiProj)
		}
	}
	s.RenderUI(uiProj)
}

func (s *PlayScene) renderFrontWithActors(fStartX, fStartY, fEndX, fEndY int, proj [16]float32) {
	actors := s.State.Actors.SortedByY()
	actorIdx := 0

	for y := fStartY; y <= fEndY; y++ {
		for x := fStartX; x <= fEndX; x++ {
			info := s.mapData.InfoAt(x, y)
			s.drawFront(info, x, y, proj)
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

	// Re-draw self on top (PlayScn.pas:1290-1295) so the player is never
	// occluded by other actors walking past.
	if s.State.MySelf != nil && !s.State.MySelf.Death {
		my := s.State.MySelf
		wx := float32(float64(my.Rx*engine.TileWidth) + my.ShiftX)
		wy := float32(float64(my.Ry*engine.TileHeight) + my.ShiftY)
		log.Logf(log.LevelTrace, "Render", "play self-redraw pos=(%.0f,%.0f) dir=%d", wx, wy, my.Dir)
		my.Draw(s.gl, s.resources, wx, wy, proj)
	}

	for _, gi := range s.groundItems {
		if gi.X < fStartX || gi.X > fEndX || gi.Y < fStartY || gi.Y > fEndY {
			continue
		}
		ix := float32(gi.X*engine.TileWidth) + 16
		iy := float32(gi.Y*engine.TileHeight) + 8
		if s.resources.DnItems != nil && gi.Looks >= 0 && gi.Looks < s.resources.DnItems.Count {
			img := s.resources.DnItems.GetImage(gi.Looks)
			if img != nil && img.RGBA != nil {
				tex := s.resources.GetTexture(s.resources.DnItems, gi.Looks)
				if tex != 0 {
					log.Logf(log.LevelTrace, "Render", "play item DnItems[%d] pos=(%.0f,%.0f) size=(%d,%d)", gi.Looks, ix, iy, img.Width, img.Height)
					s.gl.DrawQuad(tex, ix, iy, float32(img.Width), float32(img.Height), proj)
				}
			} else {
				s.gl.DrawQuadColor(ix, iy, 16, 16, 0.9, 0.8, 0.2, 0.8, proj)
			}
		} else {
			s.gl.DrawQuadColor(ix, iy, 16, 16, 0.9, 0.8, 0.2, 0.8, proj)
		}
		// Flash effect: Prguse[410..419], 10 frames × 20ms, 5000ms cycle
		// (PlayScn.pas:1349-1368, FLASHBASE=410).
		if s.resources.Prguse != nil {
			now := time.Now().UnixMilli()
			cycle := now % 5000
			if cycle < 200 { // 10 frames × 20ms = 200ms active window
				step := int(cycle / 20)
				flashIdx := 410 + step
				if fimg := s.resources.Prguse.GetImage(flashIdx); fimg != nil && fimg.RGBA != nil {
					if ftex := s.resources.GetTexture(s.resources.Prguse, flashIdx); ftex != 0 {
						log.Logf(log.LevelTrace, "Render", "play item-flash Prguse[%d] pos=(%.0f,%.0f)", flashIdx, ix, iy)
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
			log.Logf(log.LevelTrace, "Render", "play item-name '%s' pos=(%.0f,%.0f)", gi.Name, nameX, iy-14)
			s.text.DrawText(gi.Name, nameX, iy-14, 1.0, 1.0, 0.8, 1.0, proj)
		}
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
	// Delphi SayY: alive = tileRow + ShiftY - 47, dead = tileRow + ShiftY - 12
	// (PlayScn.pas:1232-1235). In world coords tileRow ≈ worldY.
	sayY := worldY - 47
	if a.Death {
		sayY = worldY - 12
	}
	if showName && a.UserName != "" && s.text != nil {
		nameW := float32(s.text.MeasureText(a.UserName))
		nameX := worldX + float32(engine.TileWidth)/2 - nameW/2
		log.Logf(log.LevelTrace, "Render", "play actor-name '%s' pos=(%.0f,%.0f) death=%v", a.UserName, nameX, sayY, a.Death)
		s.text.DrawText(a.UserName, nameX-1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX+1, sayY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY-1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY+1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, sayY, 1.0, 1.0, 1.0, 1.0, proj)
	}

	// HP bar for all visible actors (DrawScrn.pas:280-301), at SayY - 10.
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
				log.Logf(log.LevelTrace, "Render", "play hpbar-bg Prguse2[0] pos=(%.0f,%.0f) size=(%.0f,%.0f)", hpBarX, hpBarY, hpBarW, hpBarH)
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
	log.Logf(log.LevelTrace, "Render", "play chat-bubble lines=%d pos=(%.0f,%.0f)", a.SayLineCount, worldX-20, bubbleY)
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

func (s *PlayScene) drawFront(info *mapformat.CellInfo, x, y int, proj [16]float32) {
	if info.FrontLib < 0 {
		return
	}

	area := int(info.FrontArea)
	loader := s.getObjectsLoader(area)
	if loader == nil {
		return
	}
	cache := s.objectsCaches[area]

	idx := info.FrontImage
	isBlend := info.FrontAniFrame&0x80 != 0

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

	tex := s.getTex(cache, loader, idx)
	if tex == 0 {
		return
	}
	img := loader.GetImage(idx)

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
	bright := s.State.DayBright
	if bright >= 3 {
		return 0
	}
	switch bright {
	case 0:
		return 0.7
	case 1:
		return 0.45
	case 2:
		return 0.2
	default:
		return 0
	}
}

func (s *PlayScene) collectLightSources() []LightSource {
	var lights []LightSource
	if s.State.MySelf != nil {
		my := s.State.MySelf
		lights = append(lights, LightSource{
			X:     float64(my.Rx)*engine.TileWidth + engine.TileWidth/2,
			Y:     float64(my.Ry)*engine.TileHeight + engine.TileHeight/2,
			Level: 2,
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
	// Chat input accepts any printable rune, including CJK (the game is a
	// Chinese environment); 127 (DEL) is excluded. MaxLength 70
	// (PlayScn.pas:273).
	if s.chatMode && char >= 32 && char != 127 {
		if utf8.RuneCountInString(s.chatInput) < 70 {
			s.chatInput += string(char)
		}
	}
}

func (s *PlayScene) OnKey(key int, action int) {
	// UI (modals / focused edit controls) gets keys first.
	if s.ui.RouteKeyDown(key) {
		return
	}
	// Track Ctrl for the adjust-panel ×10 accelerator.
	if key == 341 || key == 345 {
		s.ctrlDown = action == 1
		return
	}
	if key == 256 && s.itemMove.Moving { // Esc releases the held item
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
		case 32: // Space — open chat (ClMain:1741-1745)
			s.chatMode = true
			return
		case 66: // B — bag (ClMain:1675-1678)
			s.State.ShowBag = !s.State.ShowBag
			return
		case 67: // C — status panel page 0 (ClMain:1668-1674)
			s.State.StatePage = 0
			s.State.ShowEquip = true
			return
		case 69: // E — status panel page 3 (ClMain:1676-1685)
			s.State.StatePage = 3
			s.State.ShowEquip = true
			return
		case 71: // G — guild (ClMain:1637-1661)
			s.toggleGuild()
			return
		case 72: // H — cycle attack mode (ClMain:1517-1525)
			s.State.AttackMode = (s.State.AttackMode + 1) % 5
			if s.sendAttackMode != nil {
				s.sendAttackMode(s.State.AttackMode)
			}
			modes := []string{"和平", "组队", "行会", "全体", "PK"}
			s.addChatMessage("[系统] 攻击模式: " + modes[s.State.AttackMode])
			return
		case 77: // M — minimap (ClMain:1613-1628; three-state cycle is B5)
			s.showMinimap = !s.showMinimap
			return
		case 78: // N — adjust ability (ClMain:1692-1695)
			s.State.ShowPlusAbil = !s.State.ShowPlusAbil
			return
		case 83: // S — group dialog (ClMain:1629-1636)
			s.State.ShowGroupDlg = !s.State.ShowGroupDlg
			return
		case 86: // V — friend dialog (ClMain:1687-1690; server side missing)
			s.addChatMessage("好友: 尚未实现")
			return
		case 87: // W — trade (ClMain:1663-1666)
			s.tryDeal()
			return
		case 90: // Z — pick up item (ClMain:1564-1573)
			if s.sendPickup != nil {
				s.sendPickup()
			}
			return
		case 265: // Up — scroll chat back one line (ClMain:1699-1706)
			s.scrollChat(-1)
			return
		case 264: // Down — scroll chat forward one line (ClMain:1707-1714)
			s.scrollChat(1)
			return
		case 266: // PageUp — page chat back (ClMain:1715-1718)
			s.scrollChat(-ViewChatLine)
			return
		case 267: // PageDown — page chat forward (ClMain:1719-1722)
			s.scrollChat(ViewChatLine)
			return
		case 298: // F9 — bag (ClMain:1488-1494)
			s.State.ShowBag = !s.State.ShowBag
			return
		case 299: // F10 — status page 0 (ClMain:1495-1502)
			s.State.StatePage = 0
			s.State.ShowEquip = true
			return
		case 300: // F11 — status page 3 (ClMain:1503-1508)
			s.State.StatePage = 3
			s.State.ShowEquip = true
			return
		case 301: // F12 — options/sound (ClMain:1509+; audio not implemented)
			s.addChatMessage("[声音] 切换(音频未实现)")
			return
		}

		// F1..F8 cast the magic bound to that key (FState:3506-3545).
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
				s.sendUseItem(item.MakeIndex)
			}
			return
		}
	}
}

// scrollChat scrolls the chat board by delta lines, clamped to the buffer
// (ClMain:1699-1722).
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

	if action == 0 { // release
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
	_ = modAlt // butcher placeholder

	// Right-click (ClMain.pas:2200-2229): Ctrl+right = inspect, otherwise move.
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

	// Left press: double-click synthesis (Delphi WM_LBUTTONDBLCLK).
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
		log.Logf(log.LevelDebug, "PlayScene", "mouse consumed by UI pos=(%d,%d)", ix, iy)
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
	// Left-click (ClMain.pas:2246-2275): attack / NPC dialog / pickup.
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
					if now-s.lastHitTick < 1400 {
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
					s.sendPickup()
					return
				}
			}
		}
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
	// Wheel over the chat board scrolls chat history; elsewhere zooms
	// (Delphi scrolls ChatBoardTop from PlayScn).
	if s.mouseX >= chatBoardX && s.mouseX <= chatBoardX+474 &&
		s.mouseY >= float64(chatBoardTop) && s.mouseY < float64(chatBoardTop+chatLineH*ViewChatLine) {
		maxScroll := len(s.chatMessages) - ViewChatLine
		if maxScroll < 0 {
			maxScroll = 0
		}
		s.chatScroll -= int(offY) // wheel up (offY>0) goes back in history
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

func (s *PlayScene) addChatMessage(text string) {
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

	// Bottom bar, HP/MP orbs, buttons, belt, chat, bag, state panel — all
	// drawn by the control tree (uihud.go / uibag.go / uistate.go) via
	// s.ui.Paint below.
	s.syncBagWindow()
	s.syncStateWindow()
	s.syncMerchantWindows()
	s.syncDealWindows()
	s.syncGuildWindows()
	s.syncAbilWindows()

	// UI control tree (DWinCtl port) paints on top of the legacy hand-drawn
	// panels; legacy panels are migrated into the tree phase by phase.
	s.ui.Paint(proj)

	// Hint panel, then the item held on the cursor — Delphi paint order
	// (ClMain.pas: DrawHint :1079, held item :1093).
	s.tooltip.Render(s, proj)
	s.renderHeldItem(proj)
}

// renderHeldItem draws the dragged item centered on the cursor with a name
// label (ClMain.pas:1093-1113).
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


