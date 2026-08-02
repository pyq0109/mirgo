package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Guard 表示守卫定义。
type Guard struct {
	Name    string `json:"name"`
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Dir     int    `json:"dir"`
}

// ConvertMisc 转换其他配置文件。
func ConvertMisc(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// 转换 GuardList.txt
	if err := convertGuardList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting GuardList.txt: %w", err)
	}

	// 转换 AdminList.txt
	if err := convertAdminList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting AdminList.txt: %w", err)
	}

	// 转换 Castle/0/SabukW.txt
	if err := convertCastle(inputDir, outputDir); err != nil {
		return fmt.Errorf("converting castle: %w", err)
	}

	// 转换 Notice/Notice.txt
	if err := convertNotice(inputDir, outputDir); err != nil {
		return fmt.Errorf("converting notice: %w", err)
	}

	// 转换 CustomMagic/*.ini
	if err := convertCustomMagic(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting custom magic: %w", err)
	}

	return nil
}

func convertGuardList(envirDir, outputDir string) error {
	guardFile := filepath.Join(envirDir, "GuardList.txt")
	data, err := ReadGBKFile(guardFile)
	if err != nil {
		return err
	}

	var guards []Guard
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		// Format: name map [x,y] : dir
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			var g Guard
			g.Name = parts[0]
			g.MapName = parts[1]

			// Parse [x,y]
			coord := parts[2]
			coord = strings.Trim(coord, "[]:,")
			fmt.Sscanf(coord, "%d,%d", &g.X, &g.Y)

			// Parse direction
			if len(parts) > 3 {
				dirStr := strings.Trim(parts[3], ":")
				fmt.Sscanf(dirStr, "%d", &g.Dir)
			}

			guards = append(guards, g)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/GuardList.txt",
		"_description": "卫兵列表",
		"guards":      guards,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "guards", "guard_list.jsonc")
	comment := fmt.Sprintf("卫兵列表\n来源: asset/server/Envir/GuardList.txt\n数量: %d 个卫兵", len(guards))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertAdminList(envirDir, outputDir string) error {
	adminFile := filepath.Join(envirDir, "AdminList.txt")
	data, err := ReadGBKFile(adminFile)
	if err != nil {
		return err
	}

	var admins []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		// Format: * name
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == "*" {
			admins = append(admins, parts[1])
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/AdminList.txt",
		"_description": "管理员列表",
		"admins":      admins,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "guards", "admin_list.jsonc")
	comment := fmt.Sprintf("管理员列表\n来源: asset/server/Envir/AdminList.txt\n数量: %d 个管理员", len(admins))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertCastle(inputDir, outputDir string) error {
	castleFile := filepath.Join(inputDir, "Castle", "0", "SabukW.txt")

	if !FileExists(castleFile) {
		fmt.Println("  跳过城堡配置 (文件不存在)")
		return nil
	}

	castleINI, err := ParseINI(castleFile)
	if err != nil {
		return err
	}

	// 输出 schema 与服务端 cmd/server/castle.go 的 CastleConfig 对齐，
	// 顶层即配置字段（无包裹）。SabukW.txt 中不存在的字段（修理费、
	// 攻城战参数、税率等）交由服务端 DefaultCastleConfig 提供默认值。
	config := map[string]interface{}{
		"name":      getINIValue(castleINI, "setup", "CastleName", "沙巴克"),
		"map":       getINIValue(castleINI, "defense", "CastleMap", "3"),
		"palaceX":   getINIInt(castleINI, "defense", "CastlePalaceDoorX", 631),
		"palaceY":   getINIInt(castleINI, "defense", "CastlePalaceDoorY", 274),
		"doorMaxHP": getINIInt(castleINI, "defense", "MainDoorHP", 5000),
		"wallMaxHP": getINIInt(castleINI, "defense", "LeftWallHP", 5000),
	}

	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "castle", "sabuk_wall.jsonc")
	comment := "沙巴克城堡配置\n来源: asset/server/Castle/0/SabukW.txt\n说明: schema 对齐服务端 CastleConfig，源文件未提供的字段使用服务端默认值"

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertNotice(inputDir, outputDir string) error {
	noticeFile := filepath.Join(inputDir, "Notice", "Notice.txt")

	if !FileExists(noticeFile) {
		fmt.Println("  跳过公告 (文件不存在)")
		return nil
	}

	data, err := ReadGBKFile(noticeFile)
	if err != nil {
		return err
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Notice/Notice.txt",
		"_description": "登录公告",
		"content":     string(data),
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "notice", "notice.jsonc")
	comment := "登录公告\n来源: asset/server/Notice/Notice.txt"

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertCustomMagic(envirDir, outputDir string) error {
	customMagicDir := filepath.Join(envirDir, "CustomMagic")
	dstDir := filepath.Join(outputDir, "magic", "custom_magic")

	if !DirExists(customMagicDir) {
		fmt.Println("  跳过自定义魔法 (目录不存在)")
		return nil
	}

	count, err := CopyDir(customMagicDir, dstDir, "*.ini")
	if err != nil {
		return err
	}

	fmt.Printf("  复制了 %d 个自定义魔法配置\n", count)
	return nil
}
