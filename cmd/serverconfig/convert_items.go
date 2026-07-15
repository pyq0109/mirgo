package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// FilterItem represents an item in the filter list.
type FilterItem struct {
	Type  int    `json:"type"`
	Name  string `json:"name"`
	Props []int  `json:"props"`
}

// ItemRule represents an item rule.
type ItemRule struct {
	Name  string `json:"name"`
	Rules string `json:"rules"`
}

// GroupItem represents an item set/group.
type GroupItem struct {
	ID       int      `json:"id"`
	Count    int      `json:"count"`
	Name     string   `json:"name"`
	Items    []string `json:"items"`
	Bonus    string   `json:"bonus"`
	Desc     string   `json:"desc"`
}

// MakeItem represents a crafting recipe.
type MakeItem struct {
	Name  string         `json:"name"`
	Items []MakeItemPart `json:"items"`
}

// MakeItemPart represents an ingredient in a crafting recipe.
type MakeItemPart struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ConvertItems converts item configuration files.
func ConvertItems(inputDir, outputDir string) error {
	envirDir := filepath.Join(inputDir, "Envir")

	// Convert FilterItemList.txt
	if err := convertFilterItems(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting FilterItemList.txt: %w", err)
	}

	// Convert ItemRuleList.txt
	if err := convertItemRules(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting ItemRuleList.txt: %w", err)
	}

	// Convert GroupItemList.txt
	if err := convertGroupItems(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting GroupItemList.txt: %w", err)
	}

	// Convert UnbindList.txt
	if err := convertUnbindList(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting UnbindList.txt: %w", err)
	}

	// Convert MakeItem.txt
	if err := convertMakeItems(envirDir, outputDir); err != nil {
		return fmt.Errorf("converting MakeItem.txt: %w", err)
	}

	return nil
}

func convertFilterItems(envirDir, outputDir string) error {
	filterFile := filepath.Join(envirDir, "FilterItemList.txt")
	data, err := ReadGBKFile(filterFile)
	if err != nil {
		return err
	}

	var items []FilterItem
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			var item FilterItem
			fmt.Sscanf(parts[0], "%d", &item.Type)
			item.Name = parts[1]

			// Parse properties
			for i := 2; i < len(parts); i++ {
				var prop int
				fmt.Sscanf(parts[i], "%d", &prop)
				item.Props = append(item.Props, prop)
			}

			items = append(items, item)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/FilterItemList.txt",
		"_description": "物品过滤列表",
		"items":       items,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "filter_items.jsonc")
	comment := fmt.Sprintf("物品过滤列表\n来源: asset/server/Envir/FilterItemList.txt\n数量: %d 个物品", len(items))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertItemRules(envirDir, outputDir string) error {
	rulesFile := filepath.Join(envirDir, "ItemRuleList.txt")
	data, err := ReadGBKFile(rulesFile)
	if err != nil {
		return err
	}

	var rules []ItemRule
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.SplitN(line, "\t", 2)
		if len(parts) >= 2 {
			rules = append(rules, ItemRule{
				Name:  strings.TrimSpace(parts[0]),
				Rules: strings.TrimSpace(parts[1]),
			})
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/ItemRuleList.txt",
		"_description": "物品规则列表",
		"rules":       rules,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "item_rules.jsonc")
	comment := fmt.Sprintf("物品规则列表\n来源: asset/server/Envir/ItemRuleList.txt\n数量: %d 条规则", len(rules))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertGroupItems(envirDir, outputDir string) error {
	groupFile := filepath.Join(envirDir, "GroupItemList.txt")
	data, err := ReadGBKFile(groupFile)
	if err != nil {
		return err
	}

	var groups []GroupItem
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) >= 5 {
			var group GroupItem
			fmt.Sscanf(parts[0], "%d", &group.ID)
			fmt.Sscanf(parts[1], "%d", &group.Count)
			group.Name = parts[2]
			group.Items = strings.Split(parts[3], "|")
			group.Bonus = parts[4]
			if len(parts) > 5 {
				group.Desc = parts[5]
			}
			groups = append(groups, group)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/GroupItemList.txt",
		"_description": "套装定义",
		"groups":      groups,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "group_items.jsonc")
	comment := fmt.Sprintf("套装定义\n来源: asset/server/Envir/GroupItemList.txt\n数量: %d 套", len(groups))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertUnbindList(envirDir, outputDir string) error {
	unbindFile := filepath.Join(envirDir, "UnbindList.txt")
	data, err := ReadGBKFile(unbindFile)
	if err != nil {
		return err
	}

	type UnbindItem struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	var items []UnbindItem
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			var item UnbindItem
			fmt.Sscanf(parts[0], "%d", &item.ID)
			item.Name = parts[1]
			items = append(items, item)
		}
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/UnbindList.txt",
		"_description": "解绑物品列表",
		"items":       items,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "unbind_list.jsonc")
	comment := fmt.Sprintf("解绑物品列表\n来源: asset/server/Envir/UnbindList.txt\n数量: %d 个物品", len(items))

	return WriteJSONC(outputFile, string(jsonData), comment)
}

func convertMakeItems(envirDir, outputDir string) error {
	makeFile := filepath.Join(envirDir, "MakeItem.txt")
	data, err := ReadGBKFile(makeFile)
	if err != nil {
		return err
	}

	var recipes []MakeItem
	var currentRecipe *MakeItem

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ';' {
			continue
		}

		// Recipe header: [item name]
		if line[0] == '[' && line[len(line)-1] == ']' {
			if currentRecipe != nil {
				recipes = append(recipes, *currentRecipe)
			}
			name := line[1 : len(line)-1]
			currentRecipe = &MakeItem{Name: name}
			continue
		}

		// Ingredient: name count
		if currentRecipe != nil {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var part MakeItemPart
				part.Name = parts[0]
				fmt.Sscanf(parts[1], "%d", &part.Count)
				currentRecipe.Items = append(currentRecipe.Items, part)
			}
		}
	}

	if currentRecipe != nil {
		recipes = append(recipes, *currentRecipe)
	}

	result := map[string]interface{}{
		"_source":     "asset/server/Envir/MakeItem.txt",
		"_description": "合成配方",
		"recipes":     recipes,
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	outputFile := filepath.Join(outputDir, "items", "make_items.jsonc")
	comment := fmt.Sprintf("合成配方\n来源: asset/server/Envir/MakeItem.txt\n数量: %d 个配方", len(recipes))

	return WriteJSONC(outputFile, string(jsonData), comment)
}
