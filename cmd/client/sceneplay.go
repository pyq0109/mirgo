package main

import (
	"fmt"
	"path/filepath"
	"strings"
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
	sendUseItem    func(bagIdx int)
	sendBuyItem    func(itemIdx int)
	sendSellItem   func(bagIdx int)
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

	targetX, targetY int

	showMinimap bool
	deathGray   bool

	effects *EffectManager
}

func NewPlayScene(gl *engine.GLState, resources *engine.ResourceManager, mapDir string) *PlayScene {
	return &PlayScene{
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
}

func (s *PlayScene) LoadMap(mapName string) error {
	mapPath := filepath.Join(s.mapDir, mapName+".map")
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		return fmt.Errorf("load map %s: %w", mapName, err)
	}
	s.mapData = m
	if s.cam == nil {
		s.cam = engine.NewCamera(1024, 768)
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

	uiProj := engine.OrthoProj(1024, 768)
	if s.showMinimap {
		mmapDrawn := false
		if s.resources.Mmap != nil && s.resources.Mmap.Count > 0 {
			mmImg := s.resources.Mmap.GetImage(0)
			if mmImg != nil && mmImg.RGBA != nil {
				mmTex := s.resources.GetTexture(s.resources.Mmap, 0)
				if mmTex != 0 {
					s.gl.DrawQuad(mmTex, 904, 10, 120, 120, uiProj)
					mmapDrawn = true
				}
			}
		}
		if !mmapDrawn && s.minimap != nil {
			glState.DrawQuad(s.minimap.GetTexture(), 814, 10, minimapSize, minimapSize, uiProj)
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
	if s.chatMode && char >= 32 && char <= 126 {
		if len(s.chatInput) < 80 {
			s.chatInput += string(char)
		}
	}
}

func (s *PlayScene) OnKey(key int, action int) {
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
				if s.sendDealCancel != nil {
					s.sendDealCancel()
				}
				return
			}
			if s.State.ShowShop {
				s.State.ShowShop = false
				return
			}
			if s.State.ShowGuild || s.State.ShowStorage {
				s.State.ShowGuild = false
				s.State.ShowStorage = false
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
			s.State.ShowGuild = !s.State.ShowGuild
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
		case 80: // P
			s.State.ShowChar = !s.State.ShowChar
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
			beltIdx := key - 49
			bagIdx := s.State.BeltItems[beltIdx]
			if bagIdx >= 0 && bagIdx < len(s.State.BagItems) && s.sendUseItem != nil {
				s.sendUseItem(bagIdx)
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

func (s *PlayScene) OnMouse(x, y float64, button int, action int) {
	if action != 1 {
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

func (s *PlayScene) OnScroll(x, y float64) {
	if s.cam != nil {
		s.cam.ZoomAt(1.1, x, y)
	}
}

func (s *PlayScene) addChatMessage(text string) {
	s.chatMessages = append(s.chatMessages, ChatMessage{Text: text, Time: time.Now().UnixMilli()})
	if len(s.chatMessages) > 10 {
		s.chatMessages = s.chatMessages[len(s.chatMessages)-10:]
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
	st := s.State

	if s.resources.Prguse != nil {
		barImg := s.resources.Prguse.GetImage(1)
		if barImg != nil && barImg.RGBA != nil {
			barTex := s.resources.GetTexture(s.resources.Prguse, 1)
			if barTex != 0 {
				barW := float32(barImg.Width)
				barH := float32(barImg.Height)
				barX := (1024 - barW) / 2
				barY := 768 - barH
				s.gl.DrawQuad(barTex, barX, barY, barW, barH, proj)
			}
		}
	} else {
		s.gl.DrawQuadColor(0, 628, 1024, 140, 0.08, 0.08, 0.12, 0.95, proj)
	}

	if s.resources.Prguse != nil {
		barImg := s.resources.Prguse.GetImage(4)
		if barImg != nil && barImg.RGBA != nil {
			barTex := s.resources.GetTexture(s.resources.Prguse, 4)
			if barTex != 0 {
				barX := float32(40)
				barY := float32(640)
				barW := float32(barImg.Width)
				barH := float32(barImg.Height)
				s.gl.DrawQuad(barTex, barX, barY, barW, barH, proj)
				hpRatio := float32(0)
				if st.MaxHP > 0 {
					hpRatio = float32(st.HP) / float32(st.MaxHP)
				}
				halfW := barW / 2
				if hpRatio < 1.0 {
					emptyX := barX + halfW*hpRatio
					emptyW := halfW * (1.0 - hpRatio)
					s.gl.DrawQuadColor(emptyX, barY, emptyW, barH, 0, 0, 0, 0.6, proj)
				}
				mpRatio := float32(0)
				if st.MaxMP > 0 {
					mpRatio = float32(st.MP) / float32(st.MaxMP)
				}
				if mpRatio < 1.0 {
					emptyX := barX + halfW + halfW*mpRatio
					emptyW := halfW * (1.0 - mpRatio)
					s.gl.DrawQuadColor(emptyX, barY, emptyW, barH, 0, 0, 0, 0.6, proj)
				}
			}
		}
	}
	s.text.DrawText(fmt.Sprintf("%d/%d", st.HP, st.MaxHP), 42, 642, 1.0, 1.0, 1.0, 1.0, proj)
	s.text.DrawText(fmt.Sprintf("%d/%d", st.MP, st.MaxMP), 90, 642, 1.0, 1.0, 1.0, 1.0, proj)
	s.text.DrawText(fmt.Sprintf("Lv.%d", st.Level), 50, 684, 1.0, 1.0, 0.5, 1.0, proj)

	if s.resources.Prguse != nil {
		buttons := []struct {
			idx   int
			x, y  float32
			label string
		}{
			{8, 843, 680, "状态"},
			{9, 882, 660, "背包"},
			{10, 922, 640, "魔法"},
			{11, 964, 620, "设置"},
		}
		for _, btn := range buttons {
			if !s.drawWilImage(s.resources.Prguse, btn.idx, btn.x, btn.y, proj) {
				s.gl.DrawQuadColor(btn.x, btn.y, 28, 28, 0.15, 0.15, 0.2, 0.9, proj)
				s.text.DrawText(btn.label, btn.x+2, btn.y+8, 0.7, 0.7, 0.7, 1.0, proj)
			}
		}
	}

	skillX := float32(380)
	skillY := float32(720)
	for i := 0; i < 8; i++ {
		sx := skillX + float32(i)*38
		s.gl.DrawQuadColor(sx, skillY, 34, 34, 0.1, 0.1, 0.15, 0.7, proj)
		if i < len(st.Magics) && s.resources.MagIcon != nil {
			magIdx := int(st.Magics[i].MagID)
			if magIdx >= 0 && magIdx < s.resources.MagIcon.Count {
				iconImg := s.resources.MagIcon.GetImage(magIdx)
				if iconImg != nil && iconImg.RGBA != nil {
					iconTex := s.resources.GetTexture(s.resources.MagIcon, magIdx)
					if iconTex != 0 {
						s.gl.DrawQuad(iconTex, sx+1, skillY+1, 32, 32, proj)
					}
				}
			}
		}
		s.text.DrawText(fmt.Sprintf("F%d", i+1), sx+2, skillY+24, 0.7, 0.7, 0.7, 1.0, proj)
	}

	beltX := float32(160)
	beltY := float32(720)
	for i := 0; i < 6; i++ {
		bx := beltX + float32(i)*36
		s.gl.DrawQuadColor(bx, beltY, 32, 32, 0.12, 0.12, 0.18, 0.8, proj)
		bagIdx := st.BeltItems[i]
		if bagIdx >= 0 && bagIdx < len(st.BagItems) && s.resources.Items != nil {
			item := st.BagItems[bagIdx]
			looks := int(item.Idx)
			if looks >= 0 && looks < s.resources.Items.Count {
				itemImg := s.resources.Items.GetImage(looks)
				if itemImg != nil && itemImg.RGBA != nil {
					itemTex := s.resources.GetTexture(s.resources.Items, looks)
					if itemTex != 0 {
						s.gl.DrawQuad(itemTex, bx+2, beltY+2, 28, 28, proj)
					}
				}
			}
		}
		s.text.DrawText(fmt.Sprintf("%d", i+1), bx+2, beltY+22, 0.6, 0.6, 0.6, 1.0, proj)
	}

	amNames := []string{"和平", "组队", "行会", "全体", "PK"}
	amColors := [][4]float32{{0.5, 1.0, 0.5, 1}, {0.5, 0.5, 1.0, 1}, {0.5, 1.0, 1.0, 1}, {1.0, 1.0, 1.0, 1}, {1.0, 0.3, 0.3, 1}}
	ac := amColors[st.AttackMode]
	s.text.DrawText("["+amNames[st.AttackMode]+"]", 10, 620, ac[0], ac[1], ac[2], 1.0, proj)

	chatX := float32(208)
	chatY := float32(620)
	for i := len(s.chatMessages) - 1; i >= 0; i-- {
		msg := s.chatMessages[i]
		r, g, b := float32(1.0), float32(1.0), float32(0.8)
		if strings.HasPrefix(msg.Text, "[系统]") {
			r, g, b = 1.0, 0.5, 0.5
		}
		if strings.HasPrefix(msg.Text, "[行会]") {
			r, g, b = 0.5, 1.0, 0.5
		}
		if strings.HasPrefix(msg.Text, "[组队]") {
			r, g, b = 0.5, 0.5, 1.0
		}
		if strings.HasPrefix(msg.Text, "[私聊]") {
			r, g, b = 1.0, 0.5, 1.0
		}
		if strings.HasPrefix(msg.Text, "[喊话]") {
			r, g, b = 1.0, 1.0, 0.0
		}
		s.text.DrawText(msg.Text, chatX, chatY, r, g, b, 1.0, proj)
		chatY -= 14
	}

	if s.chatMode {
		s.gl.DrawQuadColor(208, 750, 386, 16, 0.7, 0.7, 0.7, 0.9, proj)
		s.text.DrawText(s.chatInput+"|", 212, 752, 0.0, 0.0, 0.0, 1.0, proj)
	}

	if st.ShowNpcDialog {
		s.renderNpcDialog(proj)
	}
	if st.ShowBag {
		s.renderBagPanel(proj)
	}
	if st.ShowEquip {
		s.renderEquipPanel(proj)
	}
	if st.ShowChar {
		s.renderCharPanel(proj)
	}
	if st.InDeal {
		s.renderTradePanel(proj)
	}
	if st.ShowGuild {
		s.renderGuildPanel(proj)
	}
	if st.ShowShop {
		s.renderShopPanel(proj)
	}
	if st.ShowStorage {
		s.renderStoragePanel(proj)
	}
}

func (s *PlayScene) renderBagPanel(proj [16]float32) {
	st := s.State
	px, py := float32(0), float32(0)

	if !s.drawWilImage(s.resources.Prguse, 3, px, py, proj) {
		s.gl.DrawQuadColor(px, py, 320, 380, 0.08, 0.08, 0.12, 0.92, proj)
	}

	s.text.DrawText("背包", px+140, py+8, 1.0, 1.0, 0.8, 1.0, proj)

	cellSize := float32(36)
	startX := px + 16
	startY := py + 36
	for i := 0; i < 46; i++ {
		col := i % 8
		row := i / 8
		cx := startX + float32(col)*(cellSize+2)
		cy := startY + float32(row)*(cellSize+2)
		s.gl.DrawQuadColor(cx, cy, cellSize, cellSize, 0.15, 0.15, 0.2, 0.8, proj)
		if i < len(st.BagItems) && s.resources.Items != nil {
			item := st.BagItems[i]
			looks := int(item.Idx)
			if looks >= 0 && looks < s.resources.Items.Count {
				itemImg := s.resources.Items.GetImage(looks)
				if itemImg != nil && itemImg.RGBA != nil {
					itemTex := s.resources.GetTexture(s.resources.Items, looks)
					if itemTex != 0 {
						iw := float32(itemImg.Width)
						ih := float32(itemImg.Height)
						if iw > cellSize {
							iw = cellSize
						}
						if ih > cellSize {
							ih = cellSize
						}
						s.gl.DrawQuad(itemTex, cx+(cellSize-iw)/2, cy+(cellSize-ih)/2, iw, ih, proj)
					}
				}
			}
		}
	}

	s.text.DrawText(fmt.Sprintf("金币: %d", st.Gold), px+16, py+360, 1.0, 0.9, 0.3, 1.0, proj)
}

func (s *PlayScene) renderEquipPanel(proj [16]float32) {
	st := s.State
	px, py := float32(780), float32(0)

	if !s.drawWilImage(s.resources.Prguse, 370, px, py, proj) {
		s.gl.DrawQuadColor(px, py, 240, 350, 0.08, 0.08, 0.12, 0.92, proj)
	}

	s.text.DrawText("装备", px+100, py+8, 1.0, 1.0, 0.8, 1.0, proj)

	slotNames := []string{"衣服", "武器", "右手", "项链", "头盔", "左手镯", "右手镯", "左戒指", "右戒指", "护身符", "腰带", "鞋子", "宝石"}
	for i := 0; i < 13; i++ {
		sy := py + 35 + float32(i)*24
		s.gl.DrawQuadColor(px+10, sy, 220, 20, 0.12, 0.12, 0.18, 0.7, proj)
		s.text.DrawText(slotNames[i], px+14, sy+3, 0.7, 0.7, 0.7, 1.0, proj)
		if st.UseItems[i] != nil {
			s.text.DrawText(fmt.Sprintf("#%d", st.UseItems[i].WIndex), px+120, sy+3, 1.0, 1.0, 0.5, 1.0, proj)
		}
	}
}

func (s *PlayScene) renderNpcDialog(proj [16]float32) {
	st := s.State
	px, py := float32(250), float32(150)

	if !s.drawWilImage(s.resources.Prguse, 384, px, py, proj) {
		s.gl.DrawQuadColor(px, py, 400, 250, 0.05, 0.05, 0.1, 0.95, proj)
	}

	lines := strings.Split(st.NpcDialog, "\n")
	for i, line := range lines {
		if i > 10 {
			break
		}
		s.text.DrawText(line, px+15, py+15+float32(i)*18, 0.9, 0.9, 0.9, 1.0, proj)
	}
}

func (s *PlayScene) renderTradePanel(proj [16]float32) {
	px, py := float32(300), float32(250)
	s.gl.DrawQuadColor(px, py, 300, 200, 0.08, 0.08, 0.12, 0.95, proj)
	s.gl.DrawQuadColor(px+2, py+2, 296, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("交易 - "+s.State.DealPartner, px+80, py+5, 1.0, 1.0, 0.8, 1.0, proj)
	s.text.DrawText("等待双方确认...", px+80, py+100, 0.8, 0.8, 0.8, 1.0, proj)
	s.text.DrawText("[ESC取消交易]", px+100, py+175, 0.6, 0.6, 0.6, 1.0, proj)
}

func (s *PlayScene) renderGuildPanel(proj [16]float32) {
	px, py := float32(350), float32(200)
	s.gl.DrawQuadColor(px, py, 250, 200, 0.08, 0.08, 0.12, 0.92, proj)
	s.gl.DrawQuadColor(px+2, py+2, 246, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("行会", px+105, py+5, 1.0, 1.0, 0.8, 1.0, proj)
	if s.State.GuildName != "" {
		s.text.DrawText("名称: "+s.State.GuildName, px+10, py+35, 0.9, 0.9, 0.9, 1.0, proj)
		s.text.DrawText("职位: "+s.State.GuildRank, px+10, py+55, 0.9, 0.9, 0.9, 1.0, proj)
	} else {
		s.text.DrawText("未加入行会", px+70, py+80, 0.7, 0.7, 0.7, 1.0, proj)
	}
}

func (s *PlayScene) renderCharPanel(proj [16]float32) {
	px, py := float32(100), float32(150)
	s.gl.DrawQuadColor(px, py, 220, 280, 0.08, 0.08, 0.12, 0.92, proj)
	s.gl.DrawQuadColor(px+2, py+2, 216, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("角色信息", px+80, py+5, 1.0, 1.0, 0.8, 1.0, proj)
	st := s.State
	y := py + 35
	s.text.DrawText(fmt.Sprintf("等级: %d", st.Level), px+10, y, 0.9, 0.9, 0.9, 1.0, proj)
	y += 20
	s.text.DrawText(fmt.Sprintf("HP: %d/%d", st.HP, st.MaxHP), px+10, y, 0.9, 0.3, 0.3, 1.0, proj)
	y += 20
	s.text.DrawText(fmt.Sprintf("MP: %d/%d", st.MP, st.MaxMP), px+10, y, 0.3, 0.3, 0.9, 1.0, proj)
	y += 20
	s.text.DrawText(fmt.Sprintf("金币: %d", st.Gold), px+10, y, 1.0, 0.9, 0.3, 1.0, proj)
	y += 20
	if st.GuildName != "" {
		s.text.DrawText("行会: "+st.GuildName, px+10, y, 0.8, 0.9, 0.8, 1.0, proj)
	}
}

func (s *PlayScene) renderShopPanel(proj [16]float32) {
	st := s.State
	px, py := float32(550), float32(100)
	s.gl.DrawQuadColor(px, py, 280, 400, 0.08, 0.08, 0.12, 0.95, proj)
	s.gl.DrawQuadColor(px+2, py+2, 276, 24, 0.15, 0.15, 0.25, 1.0, proj)

	title := "商店"
	if st.ShopMode == 1 {
		title = "出售"
	} else if st.ShopMode == 2 {
		title = "修理"
	}
	s.text.DrawText(title, px+120, py+5, 1.0, 1.0, 0.8, 1.0, proj)

	if st.ShopMode == 0 {
		for i, item := range st.ShopGoods {
			if i > 14 {
				break
			}
			iy := py + 32 + float32(i)*24
			s.gl.DrawQuadColor(px+6, iy, 268, 22, 0.12, 0.12, 0.18, 0.7, proj)
			name := item.Name
			if name == "" {
				name = fmt.Sprintf("物品#%d", item.ItemIdx)
			}
			s.text.DrawText(name, px+10, iy+4, 0.9, 0.9, 0.9, 1.0, proj)
			s.text.DrawText(fmt.Sprintf("%d金", item.Price), px+200, iy+4, 1.0, 0.9, 0.3, 1.0, proj)
		}
	} else {
		for i, item := range st.BagItems {
			if i > 14 {
				break
			}
			iy := py + 32 + float32(i)*24
			s.gl.DrawQuadColor(px+6, iy, 268, 22, 0.12, 0.12, 0.18, 0.7, proj)
			s.text.DrawText(fmt.Sprintf("物品#%d", item.Idx), px+10, iy+4, 0.9, 0.9, 0.9, 1.0, proj)
			if item.DuraMax > 0 {
				s.text.DrawText(fmt.Sprintf("%d/%d", item.Dura, item.DuraMax), px+200, iy+4, 0.7, 0.7, 0.7, 1.0, proj)
			}
		}
	}

	s.text.DrawText("[ESC关闭]", px+100, py+380, 0.6, 0.6, 0.6, 1.0, proj)
}

func (s *PlayScene) renderStoragePanel(proj [16]float32) {
	st := s.State
	px, py := float32(350), float32(150)
	s.gl.DrawQuadColor(px, py, 300, 350, 0.08, 0.08, 0.12, 0.95, proj)
	s.gl.DrawQuadColor(px+2, py+2, 296, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("仓库", px+130, py+5, 1.0, 1.0, 0.8, 1.0, proj)

	cellSize := float32(36)
	startX := px + 12
	startY := py + 32
	for i := 0; i < 39; i++ {
		col := i % 8
		row := i / 8
		cx := startX + float32(col)*(cellSize+2)
		cy := startY + float32(row)*(cellSize+2)
		s.gl.DrawQuadColor(cx, cy, cellSize, cellSize, 0.15, 0.15, 0.2, 0.8, proj)
		if i < len(st.StorageItems) && s.resources.Items != nil {
			item := st.StorageItems[i]
			looks := int(item.Idx)
			if looks >= 0 && looks < s.resources.Items.Count {
				itemImg := s.resources.Items.GetImage(looks)
				if itemImg != nil && itemImg.RGBA != nil {
					itemTex := s.resources.GetTexture(s.resources.Items, looks)
					if itemTex != 0 {
						s.gl.DrawQuad(itemTex, cx+2, cy+2, cellSize-4, cellSize-4, proj)
					}
				}
			}
		}
	}
	s.text.DrawText("[ESC关闭]", px+110, py+330, 0.6, 0.6, 0.6, 1.0, proj)
}
