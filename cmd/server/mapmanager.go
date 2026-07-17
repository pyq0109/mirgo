package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

// MapRoute represents a teleport route between maps.
type MapRoute struct {
	SrcMap     string
	SrcX, SrcY int
	DstMap     string
	DstX, DstY int
}

// MapManager manages all maps.
type MapManager struct {
	mapDir string
	maps   map[string]*Environment
	routes []MapRoute
	mu     sync.RWMutex
}

// NewMapManager creates a new map manager.
func NewMapManager(mapDir string) *MapManager {
	return &MapManager{
		mapDir: mapDir,
		maps:   make(map[string]*Environment),
	}
}

// LoadAllMaps loads all .map files from the map directory.
func (m *MapManager) LoadAllMaps() error {
	entries, err := os.ReadDir(m.mapDir)
	if err != nil {
		return err
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".map") {
			continue
		}

		mapName := strings.TrimSuffix(name, ".map")
		mapPath := filepath.Join(m.mapDir, name)

		mapData, err := mapformat.Parse(mapPath)
		if err != nil {
			log.Logf(log.LevelError, "MapManager", "Failed to load map %s: %v", name, err)
			continue
		}

		env := NewEnvironment(mapName, mapData)
		m.maps[mapName] = env
		loaded++
	}

	log.Logf(log.LevelInfo, "MapManager", "Loaded %d maps", loaded)
	return nil
}

// FindMap finds a map by name.
func (m *MapManager) FindMap(name string) *Environment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maps[name]
}

// GetMapList returns all loaded map names.
func (m *MapManager) GetMapList() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.maps))
	for name := range m.maps {
		names = append(names, name)
	}
	return names
}

// AddRoute adds a teleport route between maps.
func (m *MapManager) AddRoute(srcMap string, srcX, srcY int, dstMap string, dstX, dstY int) {
	m.routes = append(m.routes, MapRoute{
		SrcMap: srcMap,
		SrcX:   srcX,
		SrcY:   srcY,
		DstMap: dstMap,
		DstX:   dstX,
		DstY:   dstY,
	})
}

// FindRoute finds a route at the given position.
func (m *MapManager) FindRoute(mapName string, x, y int) *MapRoute {
	for _, r := range m.routes {
		if r.SrcMap == mapName && r.SrcX == x && r.SrcY == y {
			return &r
		}
	}
	return nil
}

// GetLoadedCount returns the number of loaded maps.
func (m *MapManager) GetLoadedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.maps)
}
