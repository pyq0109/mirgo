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

type PlayScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	mapDir    string
	cam       *engine.Camera2D
	mapData   *mapformat.MapData

	texCache       map[int]uint32
	smTexCache     map[int]uint32
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int

	State        *GameState
	sendMove     func(ident int, dir int)
	sendAttack   func(ident int, dir int)
	lastMoveTick int64
	text         *engine.TextRenderer

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
	}
}

func (s *PlayScene) SetSendMove(fn func(ident int, dir int)) {
	s.sendMove = fn
}

func (s *PlayScene) SetSendAttack(fn func(ident int, dir int)) {
	s.sendAttack = fn
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
	s.animCounter++
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

func (s *PlayScene) OnKey(key int, action int) {
	if action != 1 && action != 2 {
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
