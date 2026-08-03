package main

import (
	"fmt"
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
		m.maps[mapName] = env
		loaded++
	}

	log.Logf(log.LevelInfo, "MapManager", "loaded %d maps", loaded)
	return nil
}

// InitMapFlags 从 map_info.jsonc 加载地图属性（Delphi LoadSubMapInfo 解析
// mapinfo.txt props，LocalDB.pas:560-850），写入对应 Environment.Flag；
// 并按 MINE 标志生成矿石节点。
func (m *MapManager) InitMapFlags(configDir string) {
	path := filepath.Join(configDir, "maps", "map_info.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "MapManager", "map info file not found: %s, no map flags loaded", path)
		return
	}
	var raw struct {
		Maps []struct {
			Index string   `json:"index"`
			Name  string   `json:"name"`
			Props []string `json:"props"`
		} `json:"maps"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		log.Logf(log.LevelWarn, "MapManager", "failed to parse %s: %v", path, err)
		return
	}

	matched := 0
	mined := 0
	for _, mi := range raw.Maps {
		env := m.FindMap(mi.Index)
		if env == nil {
			continue
		}
		env.Flag = parseMapFlag(mi.Props)
		if env.Flag.Mine || env.Flag.Mine2 {
			env.InitMineEvents()
			mined++
		}
		matched++
	}
	log.Logf(log.LevelInfo, "MapManager", "applied map flags to %d/%d maps (%d mining) from %s",
		matched, len(raw.Maps), mined, path)
}

// parseMapFlag 解析 props token（Delphi LocalDB.pas:573-850 的 token→flag 映射）。
// 带参 token 形如 NORECONNECT(0)/EXPRATE(3)/DECHP(5/1000)。
func parseMapFlag(props []string) MapFlag {
	var f MapFlag
	f.MusicID = -1
	for _, tok := range props {
		name, arg := tok, ""
		if i := strings.IndexByte(tok, '('); i >= 0 {
			name = tok[:i]
			arg = strings.TrimSuffix(tok[i+1:], ")")
		}
		switch strings.ToUpper(name) {
		case "SAFE":
			f.Safe = true
		case "DARK":
			f.Dark = true
		case "DAY":
			f.DayLight = true
		case "FIGHT":
			f.Fight = true
		case "FIGHT3":
			f.Fight3 = true
		case "QUIZ":
			f.Quiz = true
		case "MINE":
			f.Mine = true
		case "MINE2":
			f.Mine2 = true
		case "NORECONNECT":
			f.NoReconnect = true
			f.ReconnectMap = arg
		case "NORECALL":
			f.NoRecall = true
		case "NORANDOMMOVE":
			f.NoRandomMove = true
		case "NOPOSITIONMOVE", "NOHORSE":
			// Delphi NOHORSE 分支同样置 boNOPOSITIONMOVE（LocalDB.pas:808-811 原样行为）
			f.NoPositionMove = true
		case "NODRUG":
			f.NoDrug = true
		case "NEEDHOLE":
			f.NeedHole = true
		case "NODROPITEM":
			f.NoDropItem = true
		case "NOTHROWITEM":
			f.NoThrowItem = true
		case "NOCHAT":
			f.NoChat = true
		case "MUSIC":
			f.MusicID = atoiOr(arg, -1)
		case "EXPRATE":
			f.ExpRate = atoiOr(arg, -1)
		case "DECHP":
			f.DecHPPoint, f.DecHPTime = parsePair(arg)
		}
	}
	return f
}

// parsePair 解析 "point/time" 形式（Delphi DECHP/INCHP 参数，LocalDB.pas:678-684）。
func parsePair(s string) (int, int) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return atoiOr(s[:i], -1), atoiOr(s[i+1:], -1)
	}
	return -1, atoiOr(s, -1)
}

func atoiOr(s string, def int) int {
	n := 0
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
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

// InitMiniMaps 从 mini_map.jsonc 加载地图 → 小地图图像号映射，
// 写入对应 Environment.MinMap。Delphi: LocalDB.LoadMinMap（LocalDB.pas:1058-1085）。
func (m *MapManager) InitMiniMaps(configDir string) {
	path := filepath.Join(configDir, "maps", "mini_map.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "MapManager", "mini map file not found: %s, no minimaps loaded", path)
		return
	}
	var raw struct {
		MiniMaps []struct {
			MapName   string `json:"mapName"`
			MiniMapID int    `json:"miniMapId"`
		} `json:"miniMaps"`
	}
	if err := parseJSONC(data, &raw); err != nil {
		log.Logf(log.LevelWarn, "MapManager", "failed to parse %s: %v, no minimaps loaded", path, err)
		return
	}
	matched := 0
	for _, mm := range raw.MiniMaps {
		if env := m.FindMap(mm.MapName); env != nil {
			env.MinMap = mm.MiniMapID
			matched++
		} else {
			log.Logf(log.LevelDebug, "MapManager", "minimap entry for unknown map %q skipped", mm.MapName)
		}
	}
	log.Logf(log.LevelInfo, "MapManager", "loaded %d/%d minimap mappings from %s", matched, len(raw.MiniMaps), path)
}

// InitRoutes 从配置文件加载地图传送路线。
func (m *MapManager) InitRoutes(configDir string) {
	routesPath := filepath.Join(configDir, "maps", "map_routes.jsonc")
	if data, err := os.ReadFile(routesPath); err == nil {
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
		if err := parseJSONC(data, &raw); err == nil && len(raw.Routes) > 0 {
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
