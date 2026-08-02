package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/mapformat"
)

// MapRoute 表示地图之间的传送路线。
type MapRoute struct {
	SrcMap     string
	SrcX, SrcY int
	DstMap     string
	DstX, DstY int
}

// MapManager 管理所有地图。
type MapManager struct {
	mapDir string
	maps   map[string]*Environment
	routes []MapRoute
	mu     sync.RWMutex
}

// NewMapManager 创建一个新的地图管理器。
func NewMapManager(mapDir string) *MapManager {
	return &MapManager{
		mapDir: mapDir,
		maps:   make(map[string]*Environment),
	}
}

// LoadAllMaps 从地图目录加载所有 .map 文件。
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
			log.Logf(log.LevelError, "MapManager", "failed to load map %s: %v", name, err)
			continue
		}

		env := NewEnvironment(mapName, mapData)
		env.InitMineEvents()
		m.maps[mapName] = env
		loaded++
	}

	log.Logf(log.LevelInfo, "MapManager", "loaded %d maps", loaded)
	return nil
}

// FindMap 按名称查找地图。
func (m *MapManager) FindMap(name string) *Environment {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maps[name]
}

// GetMapList 返回所有已加载的地图名称。
func (m *MapManager) GetMapList() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.maps))
	for name := range m.maps {
		names = append(names, name)
	}
	return names
}

// AddRoute 添加地图之间的传送路线。
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

// FindRoute 查找指定位置的传送路线。
func (m *MapManager) FindRoute(mapName string, x, y int) *MapRoute {
	for _, r := range m.routes {
		if r.SrcMap == mapName && r.SrcX == x && r.SrcY == y {
			return &r
		}
	}
	return nil
}

// GetLoadedCount 返回已加载的地图数量。
func (m *MapManager) GetLoadedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.maps)
}

// ProcessMineRegen 对所有地图执行矿石再生检查。
func (m *MapManager) ProcessMineRegen(now int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, env := range m.maps {
		env.ProcessMineRegen(now)
	}
}

// InitRoutes 从配置文件加载地图传送路线。
func (m *MapManager) InitRoutes(configDir string) {
	routesPath := filepath.Join(configDir, "maps", "map_routes.jsonc")
	if data, err := os.ReadFile(routesPath); err == nil {
		clean := stripJSONCComments(string(data))
		var raw struct {
			Routes []struct {
				SrcMap string `json:"srcMap"`
				SrcX   int    `json:"srcX"`
				SrcY   int    `json:"srcY"`
				DstMap string `json:"dstMap"`
				DstX   int    `json:"dstX"`
				DstY   int    `json:"dstY"`
			} `json:"routes"`
		}
		if err := json.Unmarshal([]byte(clean), &raw); err == nil && len(raw.Routes) > 0 {
			for _, r := range raw.Routes {
				m.AddRoute(r.SrcMap, r.SrcX, r.SrcY, r.DstMap, r.DstX, r.DstY)
			}
			log.Logf(log.LevelInfo, "MapManager", "loaded %d map routes from %s", len(raw.Routes), routesPath)
			return
		}
		log.Logf(log.LevelWarn, "MapManager", "failed to parse %s, no routes loaded", routesPath)
		return
	}
	log.Logf(log.LevelWarn, "MapManager", "map routes file not found: %s, no routes loaded", routesPath)
}
