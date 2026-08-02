package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MonGen 表示怪物刷怪点。
type MonGen struct {
	MapName  string `json:"mapName"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Name     string `json:"name"`
	Range    int    `json:"range"`
	Count    int    `json:"count"`
	Interval int    `json:"interval"`
}

// MonItem 表示怪物掉落物品。
type MonItem struct {
	Prob string `json:"prob"`
	Name string `json:"name"`
	Count int   `json:"count,omitempty"`
}

// ConvertMonsters 转换怪物配置文件。
func ConvertMonsters(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// Convert mongen.txt
	if err := convertMonGen(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting mongen.txt: %w", err)
	}

	// 转换 MonItems/*.txt
	if err := convertMonItems(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting MonItems: %w", err)
	}

	// Copy SmartMonster/*.ini
	if err := copySmartMonster(envirDir, outputDir); err != nil {
		return fmt.Errorf("copying SmartMonster: %w", err)
	}

	// Copy MonUseItems/*.txt（人形怪装备配置）
	if err := copyMonUseItems(envirDir, outputDir); err != nil {
		return fmt.Errorf("copying MonUseItems: %w", err)
	}

	return nil
}

func convertMonGen(envirDir, outputDir string) error {
	mongenFile := filepath.Join(envirDir, "mongen.txt")
	data, err := ReadGBKFile(mongenFile)
	if err != nil {
		return err
	}

	var gens []MonGen
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 6 {
			var gen MonGen
			gen.MapName = parts[0]
			fmt.Sscanf(parts[1], "%d", &gen.X)
			fmt.Sscanf(parts[2], "%d", &gen.Y)
			gen.Name = parts[3]
			fmt.Sscanf(parts[4], "%d", &gen.Range)
			fmt.Sscanf(parts[5], "%d", &gen.Count)
			if len(parts) > 6 {
				fmt.Sscanf(parts[6], "%d", &gen.Interval)
			}
			gens = append(gens, gen)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/mongen.txt",
		"_description": "怪物刷新点定义",
		"spawns":      gens,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "monsters", "mon_gen.jsonc")
	comment := fmt.Sprintf("怪物刷新点\n来源: asset/server/Envir/mongen.txt\n数量: %d 个刷新点", len(gens))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertMonItems(envirDir, outputDir string) error {
	monItemsDir := filepath.Join(envirDir, "MonItems")
	dstDir := filepath.Join(outputDir, "monsters", "mon_items")

	// 读取目录并按扩展名不区分大小写过滤（.txt/.TXT 均命中）。
	entries, err := os.ReadDir(monItemsDir)
	if err != nil {
		return err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		base := entry.Name()
		ext := filepath.Ext(base)
		if !strings.EqualFold(ext, ".txt") {
			continue
		}
		srcFile := filepath.Join(monItemsDir, base)

		data, err := ReadGBKFile(srcFile)
		if err != nil {
			fmt.Printf("  警告: 读取 %s 失败: %v\n", srcFile, err)
			continue
		}

		monsterName := strings.TrimSuffix(base, ext)
		var items []MonItem

		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == ';' {
				continue
			}

			// Format: 1/N itemname [count]
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var item MonItem
				item.Prob = parts[0]
				item.Name = parts[1]
				if len(parts) > 2 {
					fmt.Sscanf(parts[2], "%d", &item.Count)
				}
				items = append(items, item)
			}
		}

		result := map[string]interface{}{
			"_source":      fmt.Sprintf("asset/server/Envir/MonItems/%s", base),
			"_description": fmt.Sprintf("%s 的掉落表", monsterName),
			"monster":      monsterName,
			"items":        items,
		}

		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			continue
		}

		outputFile := filepath.Join(dstDir, monsterName+".jsonc")
		comment := fmt.Sprintf("怪物掉落表\n来源: asset/server/Envir/MonItems/%s\n怪物: %s", base, monsterName)

		if err := WriteJSONC(outputFile, string(jsonData), comment); err != nil {
			fmt.Printf("  警告: 写入 %s 失败: %v\n", outputFile, err)
			continue
		}
		count++
	}

	fmt.Printf("  转换了 %d 个怪物掉落表\n", count)
	return nil
}

func copySmartMonster(envirDir, outputDir string) error {
	srcDir := filepath.Join(envirDir, "SmartMonster")
	dstDir := filepath.Join(outputDir, "monsters", "smart_monster")

	count, err := CopyDir(srcDir, dstDir, "*.ini")
	if err != nil {
		return err
	}

	fmt.Printf("  复制了 %d 个智能怪物配置\n", count)
	return nil
}

func copyMonUseItems(envirDir, outputDir string) error {
	srcDir := filepath.Join(envirDir, "MonUseItems")
	dstDir := filepath.Join(outputDir, "monsters", "mon_use_items")

	if !DirExists(srcDir) {
		fmt.Println("  跳过 MonUseItems (目录不存在)")
		return nil
	}

	count, err := CopyDir(srcDir, dstDir, "*.txt")
	if err != nil {
		return err
	}

	fmt.Printf("  复制了 %d 个怪物装备配置\n", count)
	return nil
}
