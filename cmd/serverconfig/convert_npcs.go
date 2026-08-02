package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// NpcDef 表示 NPC 定义。
type NpcDef struct {
	Name    string `json:"name"`
	Race    int    `json:"race"`
	MapName string `json:"mapName"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Face    int    `json:"face"`
	Body    int    `json:"body"`
}

// MerchantDef 表示商人/NPC 位置。
type MerchantDef struct {
	ID       string `json:"id"`
	MapName  string `json:"mapName"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Name     string `json:"name"`
	Face     int    `json:"face"`
	Body     int    `json:"body"`
	Castle   int    `json:"castle"`
}

// NpcScript 表示带元数据的 NPC 脚本。
type NpcScript struct {
	Source     string `json:"source"`
	Type       string `json:"type"`
	MapName    string `json:"mapName,omitempty"`
	NpcName    string `json:"npcName,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Script     string `json:"script"`
}

// ConvertNPCs 转换 NPC 配置文件。
func ConvertNPCs(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// 转换 Npcs.txt
	if err := convertNpcList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting Npcs.txt: %w", err)
	}

	// 转换 merchant.txt
	if err := convertMerchantList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting merchant.txt: %w", err)
	}

	// Copy market_def/*.txt
	if err := copyNpcScripts(envirDir, outputDir, "market_def", "merchant_scripts"); err != nil {
		return fmt.Errorf("copying market_def: %w", err)
	}

	// Copy Npc_def/*.txt
	if err := copyNpcScripts(envirDir, outputDir, "Npc_def", "npc_scripts"); err != nil {
		return fmt.Errorf("copying Npc_def: %w", err)
	}

	// Copy MapQuest_def/*.txt
	if err := copyNpcScripts(envirDir, outputDir, "MapQuest_def", "map_quest_scripts"); err != nil {
		return fmt.Errorf("copying MapQuest_def: %w", err)
	}

	return nil
}

func convertNpcList(envirDir, outputDir string) error {
	npcsFile := filepath.Join(envirDir, "Npcs.txt")
	data, err := ReadGBKFile(npcsFile)
	if err != nil {
		return err
	}

	var npcs []NpcDef
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 6 {
			var npc NpcDef
			npc.Name = parts[0]
			fmt.Sscanf(parts[1], "%d", &npc.Race)
			npc.MapName = parts[2]
			fmt.Sscanf(parts[3], "%d", &npc.X)
			fmt.Sscanf(parts[4], "%d", &npc.Y)
			fmt.Sscanf(parts[5], "%d", &npc.Face)
			if len(parts) > 6 {
				fmt.Sscanf(parts[6], "%d", &npc.Body)
			}
			npcs = append(npcs, npc)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/Npcs.txt",
		"_description": "NPC定义",
		"npcs":        npcs,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "npcs", "npc_list.jsonc")
	comment := fmt.Sprintf("NPC定义\n来源: asset/server/Envir/Npcs.txt\n数量: %d 个NPC", len(npcs))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertMerchantList(envirDir, outputDir string) error {
	merchantFile := filepath.Join(envirDir, "merchant.txt")
	data, err := ReadGBKFile(merchantFile)
	if err != nil {
		return err
	}

	var merchants []MerchantDef
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 7 {
			var m MerchantDef
			m.ID = parts[0]
			m.MapName = parts[1]
			fmt.Sscanf(parts[2], "%d", &m.X)
			fmt.Sscanf(parts[3], "%d", &m.Y)
			m.Name = parts[4]
			fmt.Sscanf(parts[5], "%d", &m.Face)
			fmt.Sscanf(parts[6], "%d", &m.Body)
			if len(parts) > 7 {
				fmt.Sscanf(parts[7], "%d", &m.Castle)
			}
			merchants = append(merchants, m)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/merchant.txt",
		"_description": "商人/NPC位置定义",
		"merchants":   merchants,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "npcs", "merchant_list.jsonc")
	comment := fmt.Sprintf("商人/NPC位置\n来源: asset/server/Envir/merchant.txt\n数量: %d 个商人", len(merchants))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func copyNpcScripts(envirDir, outputDir, srcSubdir, dstSubdir string) error {
	srcDir := filepath.Join(envirDir, srcSubdir)
	dstDir := filepath.Join(outputDir, "npcs", dstSubdir)

	if !DirExists(srcDir) {
		fmt.Printf("  跳过 %s (目录不存在)\n", srcSubdir)
		return nil
	}

	// 递归复制以保留子目录（如 market_def/魔龙城、market_def/backup）。
	count, err := CopyDirRecursive(srcDir, dstDir, "*.txt")
	if err != nil {
		return err
	}

	fmt.Printf("  复制了 %d 个 %s 脚本\n", count, srcSubdir)
	return nil
}
