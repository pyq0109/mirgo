package main

import (
	"fmt"
	"path/filepath"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	cullMargin      = 3
	frontCullMargin = 20
)

// PlayScene renders the game world with 3-layer tiles.
type PlayScene struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	mapDir    string
	cam       *engine.Camera2D
	mapData   *mapformat.MapData

	// Texture caches (matching mapviewer)
	texCache       map[int]uint32 // Tiles.wil image index -> GL texture
	smTexCache     map[int]uint32 // SmTiles.wil image index -> GL texture
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int
}

// NewPlayScene creates a new play scene.
func NewPlayScene(gl *engine.GLState, resources *engine.ResourceManager, mapDir string) *PlayScene {
	return &PlayScene{
		gl:             gl,
		resources:      resources,
		mapDir:         mapDir,
		texCache:       make(map[int]uint32),
		smTexCache:     make(map[int]uint32),
		objectsLoaders: make(map[int]*wil.File),
		objectsCaches:  make(map[int]map[int]uint32),
	}
}

// LoadMap loads a map file by name from local directory.
func (s *PlayScene) LoadMap(mapName string) error {
	mapPath := filepath.Join(s.mapDir, mapName+".map")
	m, err := mapformat.Parse(mapPath)
	if err != nil {
		return fmt.Errorf("load map %s: %w", mapName, err)
	}
	s.mapData = m
	s.cam = engine.NewCamera(1024, 768)
	s.cam.CenterOn(float64(m.Width)*engine.TileWidth/2, float64(m.Height)*engine.TileHeight/2)

	// Initialize Objects.wil for area 0
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
	log.Logf(log.LevelInfo, "PlayScene", "Closed")
}

func (s *PlayScene) Update(dt float64) {
	// Animation counter is incremented in Render
}

// Render matches mapviewer's GLRenderer.Render exactly.
func (s *PlayScene) Render(glState *engine.GLState, proj [16]float32) {
	if s.mapData == nil || s.cam == nil {
		return
	}

	m := s.mapData
	cam := s.cam

	// Re-establish GL state
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Projection: orthographic Y-down (matching mapviewer)
	left := float32(cam.X)
	top := float32(cam.Y)
	right := float32(cam.X + float64(cam.ViewW)/cam.Zoom)
	bottom := float32(cam.Y + float64(cam.ViewH)/cam.Zoom)
	proj = engine.OrthoProj4(left, right, bottom, top)

	// Back/middle cull range
	startX, startY, endX, endY := cam.ViewportTiles(cullMargin, cullMargin)
	startX = clamp(startX, 0, m.Width-1)
	startY = clamp(startY, 0, m.Height-1)
	endX = clamp(endX, 0, m.Width-1)
	endY = clamp(endY, 0, m.Height-1)

	// Front cull range (wider margin for tall objects)
	fStartX, fStartY, fEndX, fEndY := cam.ViewportTiles(frontCullMargin, frontCullMargin)
	fStartX = clamp(fStartX, 0, m.Width-1)
	fStartY = clamp(fStartY, 0, m.Height-1)
	fEndX = clamp(fEndX, 0, m.Width-1)
	fEndY = clamp(fEndY, 0, m.Height-1)

	// Align to even for back layer stride-2 rendering
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

	// 1. Back layer: even x, y (2x2 tile blocks)
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
			img := s.resources.Tiles.Images[info.BackImage]
			wx := float32(x * engine.TileWidth)
			wy := float32(y * engine.TileHeight)
			s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		}
	}

	// 2. Middle layer: all cells
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
			img := s.resources.SmTiles.Images[info.MiddleImage]
			wx := float32(x * engine.TileWidth)
			wy := float32(y * engine.TileHeight)
			s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		}
	}

	// 3. Front layer — single pass with per-tile blend toggling
	for y := fStartY; y <= fEndY; y++ {
		for x := fStartX; x <= fEndX; x++ {
			info := m.InfoAt(x, y)
			s.drawFront(info, x, y, proj)
		}
	}
	s.animCounter++
}

// drawFront renders a single front-layer cell.
// Matches mapviewer's GLRenderer.drawFront exactly.
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

	// Animation
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

	// Door offset
	if info.FrontDoorOffset&0x80 != 0 {
		if info.FrontDoorIndex&0x7F != 0 {
			idx += int(info.FrontDoorOffset & 0x7F)
		}
	}

	if idx < 0 || idx >= len(loader.Images) {
		return
	}

	tex := s.getTex(cache, loader, idx)
	if tex == 0 {
		return
	}
	img := loader.Images[idx]

	cellWorldX := float32(x * engine.TileWidth)
	cellWorldY := float32(y * engine.TileHeight)

	if isBlend {
		// Blend objects (fire, light): hotspot-based positioning + additive blending
		// Delphi formula: (n + ax - 2, m + ay - 68)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		wx := cellWorldX + float32(img.HotX) - 2
		wy := cellWorldY + float32(img.HotY) - 68
		s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	} else {
		// Non-blend objects: bottom-aligned positioning
		wx := cellWorldX
		wy := cellWorldY - float32(img.Height) + engine.TileHeight
		s.gl.DrawQuad(tex, wx, wy, float32(img.Width), float32(img.Height), proj)
	}
}

// getObjectsLoader returns the WIL loader for the given area, lazy-loading if needed.
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

// getTex returns a cached texture, loading if needed.
func (s *PlayScene) getTex(cache map[int]uint32, file *wil.File, idx int) uint32 {
	if idx < 0 || file == nil || idx >= len(file.Images) {
		return 0
	}
	if tex, ok := cache[idx]; ok {
		return tex
	}
	img := file.Images[idx]
	if img == nil || img.RGBA == nil {
		return 0
	}
	tex := s.gl.UploadTexture(img.RGBA)
	cache[idx] = tex
	img.RGBA = nil // Free Go-side pixels; GPU has its own copy
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

func (s *PlayScene) OnKey(key int, action int) {}
func (s *PlayScene) OnMouse(x, y float64, button int, action int) {}
func (s *PlayScene) OnScroll(x, y float64) {
	if s.cam != nil {
		s.cam.ZoomAt(1.1, x, y)
	}
}
