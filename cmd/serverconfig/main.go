// 服务端配置转换工具，将 Delphi 服务端配置文件转换为 JSONC 格式。
//
// 用法:
//
//	go run ./cmd/serverconfig [flags]
//
// 参数:
//
//	-input string   输入目录（默认 "asset/server"）
//	-output string  输出目录（默认 "serverconfig"）
//	-v              详细输出
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	inputDir := flag.String("input", "asset/server", "Delphi 服务端配置输入目录")
	outputDir := flag.String("output", "serverconfig", "JSONC 文件输出目录")
	verbose := flag.Bool("v", false, "详细输出")
	flag.Parse()

	start := time.Now()

	fmt.Println("=== 服务端配置转换工具 ===")
	fmt.Printf("输入目录: %s\n", *inputDir)
	fmt.Printf("输出目录: %s\n", *outputDir)
	fmt.Println()

	// 验证输入目录是否存在
	if !DirExists(*inputDir) {
		fmt.Fprintf(os.Stderr, "错误: 输入目录不存在: %s\n", *inputDir)
		os.Exit(1)
	}

	// 创建输出目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	// 创建子目录
	subdirs := []string{
		"maps", "items", "monsters", "magic", "npcs",
		"guards", "castle", "notice",
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

	// T2: 转换主配置
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

	// T3: 转换经验表和字符串
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

	// T4: 转换数据库
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

	// T5: 转换地图配置
	fmt.Println("[T5] 转换地图配置...")
	if err := ConvertMaps(*inputDir, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 转换地图配置失败: %v\n", err)
		stats.Errors++
	} else {
		stats.Success += 3
		if *verbose {
			fmt.Println("  -> maps/map_info.jsonc")
			fmt.Println("  -> maps/map_routes.jsonc")
			fmt.Println("  -> maps/mini_map.jsonc")
			fmt.Println("  -> maps/start_points.jsonc")
		}
	}

	// T6: 转换物品配置
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

	// T7: 转换怪物配置
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
			fmt.Println("  -> monsters/mon_use_items/*.txt")
		}
	}

	// T8: 转换 NPC 配置
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

	// T9: 转换其他配置
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

	// 输出汇总
	elapsed := time.Since(start)
	fmt.Println()
	fmt.Println("=== 转换完成 ===")
	fmt.Printf("成功: %d 个文件\n", stats.Success)
	if stats.Errors > 0 {
		fmt.Printf("失败: %d 个文件\n", stats.Errors)
	}
	fmt.Printf("耗时: %v\n", elapsed.Round(time.Millisecond))
}

// ConversionStats 跟踪转换统计信息。
type ConversionStats struct {
	Success int
	Errors  int
	Skipped int
}
