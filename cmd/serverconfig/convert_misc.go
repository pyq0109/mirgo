package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Guard represents a guard definition.
type Guard struct {
	Name    string `json:"name"`
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Dir     int    `json:"dir"`
}

// CastleConfig represents the Sabuk castle configuration.
type CastleConfig struct {
	CastleName   string `json:"castleName"`
	OwnGuild     string `json:"ownGuild"`
	TotalGold    int    `json:"totalGold"`
	CastleMap    string `json:"castleMap"`
	MainDoorName string `json:"mainDoorName"`
	MainDoorX    int    `json:"mainDoorX"`
	MainDoorY    int    `json:"mainDoorY"`
	MainDoorHP   int    `json:"mainDoorHP"`
	LeftWallHP   int    `json:"leftWallHP"`
	CenterWallHP int    `json:"centerWallHP"`
	RightWallHP  int    `json:"rightWallHP"`
}

// ConvertMisc converts miscellaneous configuration files.
func ConvertMisc(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// Convert GuardList.txt
	if err := convertGuardList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting GuardList.txt: %w", err)
	}

	// Convert AdminList.txt
	if err := convertAdminList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting AdminList.txt: %w", err)
	}

	// Convert Castle/0/SabukW.txt
	if err := convertCastle(inputDir, outputDir); err != nil {
		return fmt.Errorf("converting castle: %w", err)
	}

	// Convert Notice/Notice.txt
	if err := convertNotice(inputDir, outputDir); err != nil {
		return fmt.Errorf("converting notice: %w", err)
	}

	// Convert CustomMagic/*.ini
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

	config := CastleConfig{
		CastleName:   getINIValue(castleINI, "setup", "CastleName", "沙巴克"),
		OwnGuild:     getINIValue(castleINI, "setup", "OwnGuild", ""),
		TotalGold:    getINIInt(castleINI, "setup", "TotalGold", 0),
		CastleMap:    getINIValue(castleINI, "defense", "CastleMap", "3"),
		MainDoorName: getINIValue(castleINI, "defense", "MainDoorName", "SabukDoor"),
		MainDoorX:    getINIInt(castleINI, "defense", "MainDoorX", 672),
		MainDoorY:    getINIInt(castleINI, "defense", "MainDoorY", 330),
		MainDoorHP:   getINIInt(castleINI, "defense", "MainDoorHP", 10000),
		LeftWallHP:   getINIInt(castleINI, "defense", "LeftWallHP", 5000),
		CenterWallHP: getINIInt(castleINI, "defense", "CenterWallHP", 5000),
		RightWallHP:  getINIInt(castleINI, "defense", "RightWallHP", 5000),
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Castle/0/SabukW.txt",
		"_description": "沙巴克城堡配置",
		"castle":      config,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "castle", "sabuk_wall.jsonc")
	comment := "沙巴克城堡配置\n来源: asset/server/Castle/0/SabukW.txt"

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
