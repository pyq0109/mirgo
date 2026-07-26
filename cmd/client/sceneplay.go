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
	minimap   *Minimap

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
	lastMoveTick   int64
	text         *engine.TextRenderer

	groundItems   map[int32]*GroundItemInfo
	floatingTexts []FloatingText
	chatMessages  []ChatMessage
	chatInput     string
	chatMode      bool

	ActionLock     bool
	ActionLockTime int64
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
	}
	s.minimap = NewMinimap(s.gl, m)

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

	if s.State.MySelf != nil && s.cam != nil && s.mapData != nil {
		my := s.State.MySelf
		wx := float64(my.Rx)*engine.TileWidth + my.ShiftX + engine.TileWidth/2
		wy := float64(my.Ry)*engine.TileHeight + my.ShiftY + engine.TileHeight/2
		s.cam.CenterOn(wx, wy)
		s.cam.ClampToBounds(s.mapData.Width, s.mapData.Height)
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

	for _, ft := range s.floatingTexts {
		if s.text != nil {
			s.text.DrawText(ft.Text, ft.X, ft.Y, ft.Color[0], ft.Color[1], ft.Color[2], ft.Color[3], proj)
		}
	}

	if s.minimap != nil {
		s.minimap.Render(s.cam, s.mapData.Width, s.mapData.Height)
	}

	s.animCounter++

	uiProj := engine.OrthoProj(1024, 768)
	if s.minimap != nil {
		glState.DrawQuad(s.minimap.GetTexture(), 814, 10, minimapSize, minimapSize, uiProj)
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
			actorIdx++
		}
	}

	for ; actorIdx < len(actors); actorIdx++ {
		a := actors[actorIdx]
		worldX := float32(float64(a.Rx*engine.TileWidth) + a.ShiftX)
		worldY := float32(float64(a.Ry*engine.TileHeight) + a.ShiftY)
		a.Draw(s.gl, s.resources, worldX, worldY, proj)
		s.drawActorLabel(a, worldX, worldY, proj)
	}

	for _, gi := range s.groundItems {
		if gi.X < fStartX || gi.X > fEndX || gi.Y < fStartY || gi.Y > fEndY {
			continue
		}
		worldX := float32(gi.X*engine.TileWidth) + float32(engine.TileWidth)/2 - 8
		worldY := float32(gi.Y*engine.TileHeight) + float32(engine.TileHeight)/2 - 8
		var r, g, b float32
		if gi.Looks == 112 {
			r, g, b = 1.0, 0.84, 0.0
		} else {
			r, g, b = 1.0, 1.0, 1.0
		}
		s.gl.DrawQuadColor(worldX, worldY, 16, 16, r, g, b, 0.9, proj)
		if s.text != nil && gi.Name != "" {
			nameW := float32(s.text.MeasureText(gi.Name))
			nameX := float32(gi.X*engine.TileWidth) + float32(engine.TileWidth)/2 - nameW/2
			s.text.DrawText(gi.Name, nameX, worldY-16, r, g, b, 1.0, proj)
		}
	}
}

func (s *PlayScene) drawActorLabel(a *Actor, worldX, worldY float32, proj [16]float32) {
	if a.UserName != "" && s.text != nil {
		nameW := float32(s.text.MeasureText(a.UserName))
		nameX := worldX + float32(engine.TileWidth)/2 - nameW/2
		nameY := worldY - 60
		s.text.DrawText(a.UserName, nameX, nameY, 1.0, 1.0, 1.0, 1.0, proj)
	}

	if a.Type == ActorMonster && !a.Death {
		barW := float32(40)
		barH := float32(4)
		barX := worldX + float32(engine.TileWidth)/2 - barW/2
		barY := worldY - 52
		s.gl.DrawQuadColor(barX, barY, barW, barH, 0.2, 0.0, 0.0, 0.8, proj)
		s.gl.DrawQuadColor(barX, barY, barW, barH, 0.0, 0.8, 0.0, 0.8, proj)
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

func (s *PlayScene) OnScroll(x, y float64) {
	if s.cam != nil {
		s.cam.ZoomAt(1.1, x, y)
	}
}

func (s *PlayScene) RenderUI(proj [16]float32) {
	if s.text == nil {
		return
	}
	st := s.State

	s.gl.DrawQuadColor(10, 730, 200, 12, 0.1, 0.1, 0.1, 0.8, proj)
	if st.MaxHP > 0 {
		hpRatio := float32(st.HP) / float32(st.MaxHP)
		s.gl.DrawQuadColor(10, 730, 200*hpRatio, 12, 0.8, 0.1, 0.1, 0.9, proj)
	}
	s.text.DrawText(fmt.Sprintf("HP %d/%d", st.HP, st.MaxHP), 14, 731, 1.0, 1.0, 1.0, 1.0, proj)

	s.gl.DrawQuadColor(10, 746, 200, 12, 0.1, 0.1, 0.1, 0.8, proj)
	if st.MaxMP > 0 {
		mpRatio := float32(st.MP) / float32(st.MaxMP)
		s.gl.DrawQuadColor(10, 746, 200*mpRatio, 12, 0.1, 0.1, 0.8, 0.9, proj)
	}
	s.text.DrawText(fmt.Sprintf("MP %d/%d", st.MP, st.MaxMP), 14, 747, 1.0, 1.0, 1.0, 1.0, proj)

	s.text.DrawText(fmt.Sprintf("Lv.%d", st.Level), 10, 714, 1.0, 1.0, 0.5, 1.0, proj)

	chatY := float32(680)
	for i := len(s.chatMessages) - 1; i >= 0; i-- {
		msg := s.chatMessages[i]
		alpha := float32(1.0)
		age := time.Now().UnixMilli() - msg.Time
		if age > 10000 {
			alpha = 0.5
		}
		if age > 20000 {
			continue
		}
		s.text.DrawText(msg.Text, 10, chatY, 1.0, 1.0, 0.8, alpha, proj)
		chatY -= 16
	}

	if s.chatMode {
		s.gl.DrawQuadColor(10, 750, 400, 18, 0.0, 0.0, 0.0, 0.8, proj)
		s.text.DrawText("> "+s.chatInput+"|", 14, 752, 1.0, 1.0, 1.0, 1.0, proj)
	}

	skillX := float32(350)
	skillY := float32(740)
	for i := 0; i < 8; i++ {
		sx := skillX + float32(i)*40
		s.gl.DrawQuadColor(sx, skillY, 36, 24, 0.1, 0.1, 0.15, 0.9, proj)
		s.text.DrawText(fmt.Sprintf("F%d", i+1), sx+2, skillY+2, 0.6, 0.6, 0.6, 1.0, proj)
		if i < len(st.Magics) {
			s.text.DrawText(fmt.Sprintf("%d", st.Magics[i].MagID), sx+4, skillY+10, 1.0, 1.0, 0.5, 1.0, proj)
		}
	}

	if st.ShowNpcDialog {
		px, py := float32(250), float32(150)
		s.gl.DrawQuadColor(px, py, 400, 200, 0.05, 0.05, 0.1, 0.95, proj)
		s.gl.DrawQuadColor(px+2, py+2, 396, 24, 0.15, 0.15, 0.25, 1.0, proj)
		s.text.DrawText("NPC对话", px+170, py+5, 1.0, 1.0, 0.8, 1.0, proj)
		lines := strings.Split(st.NpcDialog, "\n")
		for i, line := range lines {
			if i > 8 {
				break
			}
			s.text.DrawText(line, px+10, py+32+float32(i)*18, 0.9, 0.9, 0.9, 1.0, proj)
		}
		s.text.DrawText("[任意键关闭]", px+160, py+180, 0.6, 0.6, 0.6, 1.0, proj)
	}

	if st.ShowBag {
		s.renderBagPanel(proj)
	}
	if st.ShowEquip {
		s.renderEquipPanel(proj)
	}
	if st.InDeal {
		s.renderTradePanel(proj)
	}
	if st.ShowGuild {
		s.renderGuildPanel(proj)
	}

	s.text.DrawText(fmt.Sprintf("金币: %d", st.Gold), 10, 700, 1.0, 0.9, 0.3, 1.0, proj)
}

func (s *PlayScene) renderBagPanel(proj [16]float32) {
	st := s.State
	px, py := float32(600), float32(200)
	s.gl.DrawQuadColor(px, py, 320, 350, 0.08, 0.08, 0.12, 0.92, proj)
	s.gl.DrawQuadColor(px+2, py+2, 316, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("背包", px+140, py+5, 1.0, 1.0, 0.8, 1.0, proj)

	cellSize := float32(36)
	startX := px + 10
	startY := py + 32
	for i := 0; i < 46; i++ {
		col := i % 8
		row := i / 8
		cx := startX + float32(col)*cellSize + float32(col)*2
		cy := startY + float32(row)*cellSize + float32(row)*2

		s.gl.DrawQuadColor(cx, cy, cellSize, cellSize, 0.12, 0.12, 0.18, 1.0, proj)

		if i < len(st.BagItems) {
			item := st.BagItems[i]
			s.gl.DrawQuadColor(cx+2, cy+2, cellSize-4, cellSize-4, 0.3, 0.25, 0.15, 1.0, proj)
			s.text.DrawText(fmt.Sprintf("%d", item.Idx), cx+6, cy+10, 0.8, 0.8, 0.6, 1.0, proj)
		}
	}
}

func (s *PlayScene) renderEquipPanel(proj [16]float32) {
	st := s.State
	px, py := float32(100), float32(200)
	s.gl.DrawQuadColor(px, py, 200, 350, 0.08, 0.08, 0.12, 0.92, proj)
	s.gl.DrawQuadColor(px+2, py+2, 196, 24, 0.15, 0.15, 0.25, 1.0, proj)
	s.text.DrawText("装备", px+80, py+5, 1.0, 1.0, 0.8, 1.0, proj)

	slotNames := []string{"衣服", "武器", "右手", "项链", "头盔", "左手镯", "右手镯", "左戒指", "右戒指", "护身符", "腰带", "鞋子", "宝石"}
	for i := 0; i < 13; i++ {
		sy := py + 32 + float32(i)*24
		s.gl.DrawQuadColor(px+10, sy, 180, 20, 0.12, 0.12, 0.18, 1.0, proj)
		s.text.DrawText(slotNames[i], px+14, sy+3, 0.7, 0.7, 0.7, 1.0, proj)
		if st.UseItems[i] != nil {
			s.text.DrawText(fmt.Sprintf("#%d", st.UseItems[i].WIndex), px+100, sy+3, 1.0, 1.0, 0.5, 1.0, proj)
		}
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
