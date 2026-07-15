// Server Config Converter converts Delphi server configuration files to JSONC format.
//
// Usage:
//
//	go run ./cmd/serverconfig [flags]
//
// Flags:
//
//	-input string   Input directory (default "asset/server")
//	-output string  Output directory (default "serverconfig")
//	-v              Verbose output
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	inputDir := flag.String("input", "asset/server", "Input directory containing Delphi server config")
	outputDir := flag.String("output", "serverconfig", "Output directory for JSONC files")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	start := time.Now()

	fmt.Println("=== Server Config Converter ===")
	fmt.Printf("输入目录: %s\n", *inputDir)
	fmt.Printf("输出目录: %s\n", *outputDir)
	fmt.Println()

	// Verify input directory exists
	if !DirExists(*inputDir) {
		fmt.Fprintf(os.Stderr, "错误: 输入目录不存在: %s\n", *inputDir)
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// Create subdirectories
	subdirs := []string{
		"maps", "items", "monsters", "magic", "npcs",
		"guards", "castle", "guild", "notice", "misc",
		"monsters/mon_items", "monsters/mon_use_items", "monsters/smart_monster",
		"magic/custom_magic",
		"npcs/npc_scripts", "npcs/merchant_scripts", "npcs/map_quest_scripts",
	}
	for _, subdir := range subdirs {
		dir := filepath.Join(*outputDir, subdir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 创建目录 %s 失败: %v\n", dir, err)
			os.Exit(1)
		}
	}

	stats := &ConversionStats{}

	// T2: Convert server config
	fmt.Println("[T2] 转换主配置...")
	if err := ConvertServer(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换主配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success++
		if *verbose {
			fmt.Println("  -> server.jsonc")
		}
	}

	// T3: Convert exp table and strings
	fmt.Println("[T3] 转换经验表和字符串...")
	if err := ConvertExpTable(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换经验表失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success++
		if *verbose {
			fmt.Println("  -> exp_table.jsonc")
		}
	}

	if err := ConvertStrings(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换字符串失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success++
		if *verbose {
			fmt.Println("  -> strings.jsonc")
		}
	}

	if err := ConvertGlobalVars(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换全局变量失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success++
		if *verbose {
			fmt.Println("  -> global_vars.jsonc")
		}
	}

	// T4: Convert database
	fmt.Println("[T4] 转换数据库...")
	if err := ConvertDatabase(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换数据库失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 3 // items, monsters, magic
		if *verbose {
			fmt.Println("  -> items/std_items.jsonc")
			fmt.Println("  -> monsters/monster_db.jsonc")
			fmt.Println("  -> magic/magic_db.jsonc")
		}
	}

	// T5: Convert maps
	fmt.Println("[T5] 转换地图配置...")
	if err := ConvertMaps(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换地图配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 3
		if *verbose {
			fmt.Println("  -> maps/map_info.jsonc")
			fmt.Println("  -> maps/mini_map.jsonc")
			fmt.Println("  -> maps/start_points.jsonc")
		}
	}

	// T6: Convert items
	fmt.Println("[T6] 转换物品配置...")
	if err := ConvertItems(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换物品配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 5
		if *verbose {
			fmt.Println("  -> items/filter_items.jsonc")
			fmt.Println("  -> items/item_rules.jsonc")
			fmt.Println("  -> items/group_items.jsonc")
			fmt.Println("  -> items/unbind_list.jsonc")
			fmt.Println("  -> items/make_items.jsonc")
		}
	}

	// T7: Convert monsters
	fmt.Println("[T7] 转换怪物配置...")
	if err := ConvertMonsters(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换怪物配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success++
		if *verbose {
			fmt.Println("  -> monsters/mon_gen.jsonc")
			fmt.Println("  -> monsters/mon_items/*.jsonc")
			fmt.Println("  -> monsters/smart_monster/*.ini")
		}
	}

	// T8: Convert NPCs
	fmt.Println("[T8] 转换NPC配置...")
	if err := ConvertNPCs(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换NPC配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 2
		if *verbose {
			fmt.Println("  -> npcs/npc_list.jsonc")
			fmt.Println("  -> npcs/merchant_list.jsonc")
			fmt.Println("  -> npcs/merchant_scripts/*.txt")
			fmt.Println("  -> npcs/npc_scripts/*.txt")
			fmt.Println("  -> npcs/map_quest_scripts/*.txt")
		}
	}

	// T9: Convert misc
	fmt.Println("[T9] 转换其他配置...")
	if err := ConvertMisc(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换其他配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 4
		if *verbose {
			fmt.Println("  -> guards/guard_list.jsonc")
			fmt.Println("  -> guards/admin_list.jsonc")
			fmt.Println("  -> castle/sabuk_wall.jsonc")
			fmt.Println("  -> notice/notice.jsonc")
			fmt.Println("  -> magic/custom_magic/*.ini")
		}
	}

	// Print summary
	elapsed := time.Since(start)
	fmt.Println()
	fmt.Println("=== 转换完成 ===")
	fmt.Printf("成功: %d 个文件\n", stats.Success)
	if stats.Errors > 0 {
		fmt.Printf("失败: %d 个文件\n", stats.Errors)
	}
	fmt.Printf("耗时: %v\n", elapsed.Round(time.Millisecond))
}

// ConversionStats tracks conversion statistics.
type ConversionStats struct {
	Success int
	Errors  int
	Skipped int
}
