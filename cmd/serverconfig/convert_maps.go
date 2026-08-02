package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// MapInfo 表示地图定义。
// Index 为字符串：地图编号可以是数字（"0"）或字母（"D001"、"dygw"）。
type MapInfo struct {
	Index string   `json:"index"`
	Name  string   `json:"name"`
	Flags int      `json:"flags"`
	Props []string `json:"props,omitempty"`
}

// MapRoute 表示地图间的传送路线。
type MapRoute struct {
	SrcMap string `json:"srcMap"`
	SrcX   int    `json:"srcX"`
	SrcY   int    `json:"srcY"`
	DstMap string `json:"dstMap"`
	DstX   int    `json:"dstX"`
	DstY   int    `json:"dstY"`
}

// MiniMap 表示小地图映射。
type MiniMap struct {
	MapName  string `json:"mapName"`
	MiniMapID int   `json:"miniMapId"`
}

// StartPoint 表示安全区/重生点。
type StartPoint struct {
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Range   int    `json:"range"`
}

// ConvertMaps 转换地图配置文件。
func ConvertMaps(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// Convert mapinfo.txt
	if err := convertMapInfo(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting mapinfo.txt: %w", err)
	}

	// Copy .map files
	if err := copyMapFiles(inputDir, outputDir); err != nil {
		return fmt.Errorf("copying map files: %w", err)
	}

	// 转换 MiniMap.txt
	if err := convertMiniMap(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting MiniMap.txt: %w", err)
	}

	// 转换 StartPoint.txt
	if err := convertStartPoint(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting StartPoint.txt: %w", err)
	}

	return nil
}

func convertMapInfo(envirDir, outputDir string) error {
	mapInfoFile := filepath.Join(envirDir, "mapinfo.txt")
	data, err := ReadGBKFile(mapInfoFile)
	if err != nil {
		return err
	}

	var maps []MapInfo
	var routes []MapRoute

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		// 路线定义: srcMap x,y -> dstMap x,y
		if strings.Contains(line, "->") {
			parts := strings.SplitN(line, "->", 2)
			srcParts := strings.Fields(strings.TrimSpace(parts[0]))
			dstParts := strings.Fields(strings.TrimSpace(parts[1]))
			if len(srcParts) >= 2 && len(dstParts) >= 2 {
				var route MapRoute
				route.SrcMap = srcParts[0]
				fmt.Sscanf(srcParts[1], "%d,%d", &route.SrcX, &route.SrcY)
				route.DstMap = dstParts[0]
				fmt.Sscanf(dstParts[1], "%d,%d", &route.DstX, &route.DstY)
				routes = append(routes, route)
			}
			continue
		}

		// 地图定义: [index name flags] [PROPS...]
		// 属性后缀出现在右括号之后（如 [D001 兽人古墓一层 0] DARK NORECALL），
		// flags 可省略（如 [6 魔龙城]）。
		openIdx := strings.Index(line, "[")
		closeIdx := strings.Index(line, "]")
		if openIdx >= 0 && closeIdx > openIdx {
			content := line[openIdx+1 : closeIdx]
			propsStr := strings.TrimSpace(line[closeIdx+1:])
			parts := strings.Fields(content)
			if len(parts) >= 2 {
				mi := MapInfo{Index: parts[0], Name: parts[1]}
				if len(parts) >= 3 {
					fmt.Sscanf(parts[2], "%d", &mi.Flags)
				}
				if propsStr != "" {
					mi.Props = strings.Fields(propsStr)
				}
				maps = append(maps, mi)
			}
		}
	}

	// 地图定义 → maps/map_info.jsonc
	mapResult := map[string]interface{}{
		"_source":      "asset/server/Envir/mapinfo.txt",
		"_description": "地图信息定义（地图属性）",
		"maps":         maps,
	}
	mapJSON, err := json.MarshalIndent(mapResult, "", "  ")
	if err != nil {
		return err
	}
	mapComment := fmt.Sprintf("地图信息\n来源: asset/server/Envir/mapinfo.txt\n数量: %d 个地图", len(maps))
	if err := WriteJSONC(filepath.Join(outputDir, "maps", "map_info.jsonc"), string(mapJSON), mapComment); err != nil {
		return err
	}

	// 传送路线 → maps/map_routes.jsonc（与 server MapManager.InitRoutes 约定一致）
	routeResult := map[string]interface{}{
		"_source":      "asset/server/Envir/mapinfo.txt",
		"_description": "地图间传送路线",
		"routes":       routes,
	}
	routeJSON, err := json.MarshalIndent(routeResult, "", "  ")
	if err != nil {
		return err
	}
	routeComment := fmt.Sprintf("传送路线\n来源: asset/server/Envir/mapinfo.txt\n数量: %d 条路线", len(routes))
	return WriteJSONC(filepath.Join(outputDir, "maps", "map_routes.jsonc"), string(routeJSON), routeComment)
}

func copyMapFiles(inputDir, outputDir string) error {
	srcDir := filepath.Join(inputDir, "Map")
	dstDir := filepath.Join(outputDir, "maps")

	count, err := CopyDir(srcDir, dstDir, "*.map")
	if err != nil {
		return err
	}

	fmt.Printf("  复制了 %d 个地图文件\n", count)
	return nil
}

func convertMiniMap(envirDir, outputDir string) error {
	miniMapFile := filepath.Join(envirDir, "MiniMap.txt")
	data, err := ReadGBKFile(miniMapFile)
	if err != nil {
		return err
	}

	var miniMaps []MiniMap
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			var mm MiniMap
			mm.MapName = parts[0]
			fmt.Sscanf(parts[1], "%d", &mm.MiniMapID)
			miniMaps = append(miniMaps, mm)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/MiniMap.txt",
		"_description": "小地图映射关系",
		"miniMaps":    miniMaps,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "maps", "mini_map.jsonc")
	comment := fmt.Sprintf("小地图映射\n来源: asset/server/Envir/MiniMap.txt\n数量: %d 个映射", len(miniMaps))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertStartPoint(envirDir, outputDir string) error {
	startPointFile := filepath.Join(envirDir, "StartPoint.txt")
	data, err := ReadGBKFile(startPointFile)
	if err != nil {
		return err
	}

	var startPoints []StartPoint
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 4 {
			var sp StartPoint
			sp.MapName = parts[0]
			fmt.Sscanf(parts[1], "%d", &sp.X)
			fmt.Sscanf(parts[2], "%d", &sp.Y)
			fmt.Sscanf(parts[3], "%d", &sp.Range)
			startPoints = append(startPoints, sp)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/StartPoint.txt",
		"_description": "安全区和复活点",
		"startPoints": startPoints,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "maps", "start_points.jsonc")
	comment := fmt.Sprintf("安全区/复活点\n来源: asset/server/Envir/StartPoint.txt\n数量: %d 个点", len(startPoints))

	return WriteJSONC(outputFile, string(jsonData), comment)
}
