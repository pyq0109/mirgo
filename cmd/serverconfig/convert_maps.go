package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// MapInfo represents a map definition.
type MapInfo struct {
	Index  int      `json:"index"`
	Name   string   `json:"name"`
	Flags  int      `json:"flags"`
	Props  []string `json:"props,omitempty"`
}

// MapRoute represents a teleport route between maps.
type MapRoute struct {
	SrcMap string `json:"srcMap"`
	SrcX   int    `json:"srcX"`
	SrcY   int    `json:"srcY"`
	DstMap string `json:"dstMap"`
	DstX   int    `json:"dstX"`
	DstY   int    `json:"dstY"`
}

// MapsConfig represents the maps configuration.
type MapsConfig struct {
	Source  string     `json:"source"`
	Maps    []MapInfo  `json:"maps"`
	Routes  []MapRoute `json:"routes"`
}

// MiniMap represents a minimap mapping.
type MiniMap struct {
	MapName  string `json:"mapName"`
	MiniMapID int   `json:"miniMapId"`
}

// StartPoint represents a safe zone / respawn point.
type StartPoint struct {
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Range   int    `json:"range"`
}

// ConvertMaps converts map configuration files.
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

	// Convert MiniMap.txt
	if err := convertMiniMap(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting MiniMap.txt: %w", err)
	}

	// Convert StartPoint.txt
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

	config := MapsConfig{
		Source: "asset/server/Envir/mapinfo.txt",
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || line[0] == ';' {
			continue
		}

		// Map definition: [index name flags]
		if line[0] == '[' && line[len(line)-1] == ']' {
			content := line[1 : len(line)-1]
			parts := strings.Fields(content)
			if len(parts) >= 3 {
				var index int
				var name string
				var flags int
				fmt.Sscanf(parts[0], "%d", &index)
				name = parts[1]
				fmt.Sscanf(parts[2], "%d", &flags)

				// Parse additional properties
				var props []string
				if len(parts) > 3 {
					props = parts[3:]
				}

				config.Maps = append(config.Maps, MapInfo{
					Index: index,
					Name:  name,
					Flags: flags,
					Props: props,
				})
			}
			continue
		}

		// Route definition: srcMap x,y -> dstMap x,y
		if strings.Contains(line, "->") {
			parts := strings.Split(line, "->")
			if len(parts) == 2 {
				srcParts := strings.Fields(strings.TrimSpace(parts[0]))
				dstParts := strings.Fields(strings.TrimSpace(parts[1]))

				if len(srcParts) >= 2 && len(dstParts) >= 2 {
					var route MapRoute
					route.SrcMap = srcParts[0]
					fmt.Sscanf(srcParts[1], "%d,%d", &route.SrcX, &route.SrcY)
					route.DstMap = dstParts[0]
					fmt.Sscanf(dstParts[1], "%d,%d", &route.DstX, &route.DstY)
					config.Routes = append(config.Routes, route)
				}
			}
		}
	}

	result := map[string]interface{}{
		"_source":     config.Source,
		"_description": "地图信息定义，包含地图属性和传送点",
		"maps":        config.Maps,
		"routes":      config.Routes,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "maps", "map_info.jsonc")
	comment := fmt.Sprintf("地图信息\n来源: asset/server/Envir/mapinfo.txt\n数量: %d 个地图, %d 个传送点", len(config.Maps), len(config.Routes))

	return WriteJSONC(outputFile, string(jsonData), comment)
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
