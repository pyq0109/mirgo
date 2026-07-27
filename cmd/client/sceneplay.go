package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	cullMargin      = 3
	frontCullMargin = 20
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

	// Adjust-ability + inspect windows (uiabil.go).
	hudAbil, hudInspect *UIControl
	abilDeltas          [9]int
	abilPointsLeft      int
	showInspect         bool
	inspectItems        [13]*protocol.UserItem
	inspectName         string

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
		s.backgroundClick()
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

// backgroundClick handles clicks that fell through every control.
func (s *PlayScene) backgroundClick() {
	if !s.itemMove.Moving {
		return
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
	default:
		s.itemMove.Cancel(s.State)
	}
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
		s.cam.CenterOn(wx, wy)
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
				if dist >= 3 {
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
					s.gl.DrawQuad(mmTex, ScreenWidth-120-10, 10, 120, 120, uiProj)
					mmapDrawn = true
				}
			}
		}
		if !mmapDrawn && s.minimap != nil {
			glState.DrawQuad(s.minimap.GetTexture(), ScreenWidth-minimapSize-10, 10, minimapSize, minimapSize, uiProj)
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
					s.gl.DrawQuad(tex, ix, iy, float32(img.Width), float32(img.Height), proj)
				}
			} else {
				s.gl.DrawQuadColor(ix, iy, 16, 16, 0.9, 0.8, 0.2, 0.8, proj)
			}
		} else {
			s.gl.DrawQuadColor(ix, iy, 16, 16, 0.9, 0.8, 0.2, 0.8, proj)
		}
		if s.text != nil && gi.Name != "" {
			nameW := float32(s.text.MeasureText(gi.Name))
			nameX := float32(gi.X*engine.TileWidth) + float32(engine.TileWidth)/2 - nameW/2
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
	if showName && a.UserName != "" && s.text != nil {
		nameW := float32(s.text.MeasureText(a.UserName))
		nameX := worldX + float32(engine.TileWidth)/2 - nameW/2
		nameY := worldY - 75
		s.text.DrawText(a.UserName, nameX-1, nameY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX+1, nameY, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, nameY-1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, nameY+1, 0, 0, 0, 1.0, proj)
		s.text.DrawText(a.UserName, nameX, nameY, 1.0, 1.0, 1.0, 1.0, proj)
	}

	if !a.Death && s.resources.Prguse2 != nil {
		bgImg := s.resources.Prguse2.GetImage(0)
		fillImg := s.resources.Prguse2.GetImage(1)
		if bgImg != nil && bgImg.RGBA != nil && fillImg != nil {
			bgTex := s.resources.GetTexture(s.resources.Prguse2, 0)
			fillTex := s.resources.GetTexture(s.resources.Prguse2, 1)
			hpBarW := float32(bgImg.Width)
			hpBarH := float32(bgImg.Height)
			hpBarX := worldX + float32(engine.TileWidth)/2 - hpBarW/2
			hpBarY := worldY - 70
			if bgTex != 0 {
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
			s.gl.DrawQuadColor(worldX+4, worldY-70, 40, 4, 0.1, 0.0, 0.0, 0.8, proj)
			s.gl.DrawQuadColor(worldX+4, worldY-70, 40, 4, 0.8, 0.0, 0.0, 0.8, proj)
		}
	} else if !a.Death {
		s.gl.DrawQuadColor(worldX+4, worldY-70, 40, 4, 0.1, 0.0, 0.0, 0.8, proj)
		s.gl.DrawQuadColor(worldX+4, worldY-70, 40, 4, 0.8, 0.0, 0.0, 0.8, proj)
	}
}

func (s *PlayScene) drawChatBubble(a *Actor, worldX, worldY float32, proj [16]float32) {
	if s.text == nil || a.SayLineCount == 0 {
		return
	}
	if time.Now().UnixMilli()-a.SayTime > 5000 {
		return
	}
	bubbleY := worldY - 70
	for i := 0; i < a.SayLineCount && i < 5; i++ {
		if a.SayingArr[i] != "" {
			s.text.DrawText(a.SayingArr[i], worldX-20, bubbleY+float32(i)*14, 1.0, 1.0, 1.0, 0.9, proj)
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
	if s.chatMode && char >= 32 && char <= 126 {
		if len(s.chatInput) < 80 {
			s.chatInput += string(char)
		}
	}
}

func (s *PlayScene) OnKey(key int, action int) {
	// UI (modals / focused edit controls) gets keys first.
	if s.ui.RouteKeyDown(key) {
		return
	}
	if key == 256 && s.itemMove.Moving { // Esc releases the held item
		s.itemMove.Cancel(s.State)
		return
	}
	if s.State.ShowNpcDialog {
		s.State.ShowNpcDialog = false
		return
	}

	if action == 1 {
		switch key {
		case 256: // Escape
			if s.State.InDeal {
				s.State.InDeal = false
				s.State.DealPartner = ""
				s.resetDeal()
				if s.sendDealCancel != nil {
					s.sendDealCancel()
				}
				return
			}
			if s.State.ShowShop {
				s.State.ShowShop = false
				return
			}
			if s.State.ShowGuild {
				s.State.ShowGuild = false
				return
			}
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
		case 66: // B
			s.State.ShowBag = !s.State.ShowBag
			return
		case 71: // G
			s.toggleGuild()
			return
		case 78: // N
			s.State.ShowEquip = !s.State.ShowEquip
			return
		case 77: // M
			s.showMinimap = !s.showMinimap
			return
		case 72: // H
			s.State.AttackMode = (s.State.AttackMode + 1) % 5
			if s.sendAttackMode != nil {
				s.sendAttackMode(s.State.AttackMode)
			}
			modes := []string{"和平", "组队", "行会", "全体", "PK"}
			s.addChatMessage("[系统] 攻击模式: " + modes[s.State.AttackMode])
			return
		case 80: // P — character stats (state panel page 1)
			s.State.ShowEquip = !s.State.ShowEquip
			if s.State.ShowEquip {
				s.State.StatePage = 1
			}
			return
		}

		if !s.chatMode && key >= 290 && key <= 297 {
			slotIdx := key - 290
			if slotIdx < len(s.State.Magics) && s.sendSpell != nil {
				mag := s.State.Magics[slotIdx]
				my := s.State.MySelf
				if my != nil {
					dx, dy := dirOffset(my.Dir)
					tx := my.CurrX + dx
					ty := my.CurrY + dy
					s.sendSpell(int(mag.MagID), tx, ty)
				}
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
	if action != 1 && action != 2 {
		return
	}
	if s.chatMode {
		return
	}
	if s.State.MySelf == nil || s.sendMove == nil {
		return
	}
	if s.State.MySelf.Death {
		return
	}
	if s.actionFailLock {
		if time.Now().UnixMilli()-s.actionFailLockTime > 1000 {
			s.actionFailLock = false
		} else {
			return
		}
	}
	if !s.State.MySelf.IsIdle() || !s.ServerAcceptNextAction() {
		return
	}

	dir := -1
	switch key {
	case 87, 265: // W, Up
		dir = 0
	case 69: // E
		dir = 1
	case 68, 262: // D, Right
		dir = 2
	case 67: // C
		dir = 3
	case 83, 264: // S, Down
		dir = 4
	case 90: // Z
		dir = 5
	case 65, 263: // A, Left
		dir = 6
	case 81: // Q
		dir = 7
	}

	if dir < 0 {
		return
	}

	s.targetX = -1
	s.targetY = -1

	dx, dy := dirOffset(dir)
	newX := s.State.MySelf.CurrX + dx
	newY := s.State.MySelf.CurrY + dy

	if !s.CanWalk(newX, newY) {
		s.State.MySelf.UpdateMsg(protocol.CMTurn, s.State.MySelf.CurrX, s.State.MySelf.CurrY, dir, 0, 0)
		s.sendMove(protocol.CMTurn, dir)
		return
	}

	s.State.MySelf.UpdateMsg(protocol.CMWalk, newX, newY, dir, 0, 0)
	s.sendMove(protocol.CMWalk, dir)
	s.ActionLock = true
	s.ActionLockTime = time.Now().UnixMilli()
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

func (s *PlayScene) OnMouse(x, y float64, button int, action int) {
	s.mouseX, s.mouseY = x, y
	ix, iy := int(x), int(y)

	if action == 0 { // release
		s.ui.RouteMouseUp(ix, iy, button)
		return
	}
	if action != 1 {
		return
	}

	// Right-click cancels an item drag (ClMain.pas:2193); otherwise it
	// inspects the clicked player.
	if button == 1 {
		if s.itemMove.Moving {
			s.itemMove.Cancel(s.State)
			return
		}
		s.tryInspect(x, y)
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
		// The second press of a double-click is not a standalone down
		// (Windows sends DOWN, UP, DBLCLK, UP).
		s.ui.RouteDblClick(ix, iy)
		return
	}
	if s.ui.RouteMouseDown(ix, iy, button) {
		return
	}
	// Below the map surface is HUD territory (bottom bar, y>=445).
	if y >= MapSurfaceH {
		return
	}
	if s.State.MySelf == nil || s.sendMove == nil {
		return
	}
	if s.State.MySelf.Death {
		return
	}
	if button == 0 {
		if s.cam == nil || s.mapData == nil {
			return
		}
		wx, wy := s.cam.ScreenToWorld(x, y)
		tx, ty := s.cam.WorldToTile(wx, wy)

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

		s.targetX = tx
		s.targetY = ty
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


