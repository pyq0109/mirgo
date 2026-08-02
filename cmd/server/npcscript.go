package main

import (
	"bufio"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type ScriptSection struct {
	Conditions []string
	Actions    []string
	SayText    []string
	ElseSay    []string
	ElseAct    []string
	ExtJmp     bool // [@label] TRUE — 允许外部跳转访问 (Delphi boExtJmp)
}

type ScriptGoods struct {
	ItemName   string
	Count      int
	RefillTime int // 小时
}

type NpcScript struct {
	Labels map[string][]*ScriptSection
	Goods  []ScriptGoods

	// 商人脚本头
	PriceRate    int
	ItemTypes    []int
	Capabilities map[string]bool
}

func LoadNpcScript(path string) (*NpcScript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	script := &NpcScript{
		Labels:       make(map[string][]*ScriptSection),
		PriceRate:    100,
		Capabilities: make(map[string]bool),
	}
	scanner := bufio.NewScanner(f)

	var currentLabel string
	var currentSection *ScriptSection
	mode := ""
	inGoods := false
	inHeader := true

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}

		// [goods] 段
		if strings.EqualFold(line, "[goods]") {
			inGoods = true
			inHeader = false
			mode = ""
			currentSection = nil
			continue
		}

		if inGoods {
			if strings.HasPrefix(line, "[") {
				inGoods = false
			} else {
				parseGoodsLine(line, script)
				continue
			}
		}

		// [@label] 或 [label] 段头
		if strings.HasPrefix(line, "[") {
			inHeader = false
			label := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			extJmp := false
			// 处理 [@label] TRUE 格式 (Delphi boExtJmp, LocalDB.pas:3176-3180)
			if idx := strings.IndexByte(label, ' '); idx > 0 {
				if strings.EqualFold(strings.TrimSpace(label[idx+1:]), "TRUE") {
					extJmp = true
				}
				label = label[:idx]
			}
			if strings.HasPrefix(label, "@") {
				label = label[1:]
			}
			currentLabel = label
			currentSection = &ScriptSection{ExtJmp: extJmp}
			script.Labels[currentLabel] = append(script.Labels[currentLabel], currentSection)
			mode = ""
			continue
		}

		// 商人脚本头解析（在第一个 [ 之前的行）
		if inHeader {
			parseMerchantHeader(line, script)
			continue
		}

		if currentSection == nil {
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case upper == "#IF":
			// Delphi LocalDB.pas:3194-3203：若当前块已有条件或对话文本，新建过程块
			if len(currentSection.Conditions) > 0 || len(currentSection.SayText) > 0 {
				currentSection = &ScriptSection{}
				script.Labels[currentLabel] = append(script.Labels[currentLabel], currentSection)
			}
			mode = "IF"
			continue
		case upper == "#ACT":
			mode = "ACT"
			continue
		case upper == "#SAY":
			mode = "SAY"
			continue
		case upper == "#ELSESAY" || upper == "#ELSE":
			mode = "ELSESAY"
			continue
		case upper == "#ELSEACT" || upper == "#ELACT":
			mode = "ELSEACT"
			continue
		case strings.HasPrefix(upper, "#CALL"):
			// #CALL [file] @label — 外部脚本包含 (Delphi LocalDB.pas:1714-1734)
			if currentSection != nil {
				processCallDirective(line, path, script, currentSection)
			}
			continue
		}

		switch mode {
		case "IF":
			currentSection.Conditions = append(currentSection.Conditions, line)
		case "ACT":
			currentSection.Actions = append(currentSection.Actions, line)
		case "SAY":
			currentSection.SayText = append(currentSection.SayText, line)
		case "ELSESAY":
			currentSection.ElseSay = append(currentSection.ElseSay, line)
		case "ELSEACT":
			currentSection.ElseAct = append(currentSection.ElseAct, line)
		default:
			currentSection.SayText = append(currentSection.SayText, line)
		}
	}

	return script, nil
}

// processCallDirective 处理 #CALL [file] @label 指令。
// 将外部脚本段落合并到当前脚本，并在当前 section 添加 GOTO。
func processCallDirective(line, scriptPath string, script *NpcScript, current *ScriptSection) {
	// 格式: #CALL [QuestDiary/file.txt] @label
	rest := strings.TrimSpace(line[5:]) // 去掉 "#CALL"
	lb := strings.IndexByte(rest, '[')
	rb := strings.IndexByte(rest, ']')
	if lb < 0 || rb < 0 || rb <= lb {
		return
	}
	file := rest[lb+1 : rb]
	labelPart := strings.TrimSpace(rest[rb+1:])
	label := strings.TrimPrefix(labelPart, "@")
	if file == "" || label == "" {
		return
	}
	// 安全检查：禁止路径遍历
	if strings.Contains(file, "..") || strings.ContainsAny(file, "/\\") && !strings.Contains(file, "/") {
		return
	}
	dir := filepath.Dir(scriptPath)
	extPath := filepath.Join(dir, filepath.FromSlash(file))
	lines, err := loadCallSection(extPath, label)
	if err != nil {
		return
	}
	// 将外部段落解析为新的 ScriptSection
	extSection := parseCallLines(lines)
	if _, exists := script.Labels[label]; !exists {
		script.Labels[label] = []*ScriptSection{extSection}
	}
	// 当前 section 添加 GOTO 动作
	current.Actions = append(current.Actions, "GOTO @"+label)
}

// loadCallSection 从外部文件中提取 [@label] 到 } 之间的行。
func loadCallSection(path, label string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !found {
			// 查找 [@label] 或 [label]
			if strings.HasPrefix(line, "[") {
				l := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
				if idx := strings.IndexByte(l, ' '); idx > 0 {
					l = l[:idx]
				}
				l = strings.TrimPrefix(l, "@")
				if strings.EqualFold(l, label) {
					found = true
				}
			}
			continue
		}
		if line == "}" {
			break
		}
		lines = append(lines, line)
	}
	if !found {
		return nil, os.ErrNotExist
	}
	return lines, nil
}

// parseCallLines 将 #CALL 提取的行解析为 ScriptSection。
func parseCallLines(lines []string) *ScriptSection {
	sec := &ScriptSection{}
	mode := ""
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case upper == "#IF":
			mode = "IF"
			continue
		case upper == "#ACT":
			mode = "ACT"
			continue
		case upper == "#SAY":
			mode = "SAY"
			continue
		case upper == "#ELSESAY" || upper == "#ELSE":
			mode = "ELSESAY"
			continue
		case upper == "#ELSEACT" || upper == "#ELACT":
			mode = "ELSEACT"
			continue
		}
		switch mode {
		case "IF":
			sec.Conditions = append(sec.Conditions, line)
		case "ACT":
			sec.Actions = append(sec.Actions, line)
		case "SAY":
			sec.SayText = append(sec.SayText, line)
		case "ELSESAY":
			sec.ElseSay = append(sec.ElseSay, line)
		case "ELSEACT":
			sec.ElseAct = append(sec.ElseAct, line)
		default:
			sec.SayText = append(sec.SayText, line)
		}
	}
	return sec
}

func parseMerchantHeader(line string, script *NpcScript) {
	if strings.HasPrefix(line, "%") {
		rate, err := strconv.Atoi(line[1:])
		if err == nil && rate >= 55 {
			script.PriceRate = rate
		}
		return
	}

	if strings.HasPrefix(line, "+") {
		typeID, err := strconv.Atoi(line[1:])
		if err == nil {
			script.ItemTypes = append(script.ItemTypes, typeID)
		}
		return
	}

	if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
		inner := line[1 : len(line)-1]
		caps := strings.Fields(inner)
		for _, cap := range caps {
			cap = strings.ToLower(strings.TrimPrefix(cap, "@"))
			script.Capabilities[cap] = true
		}
	}
}

func parseGoodsLine(line string, script *NpcScript) {
	line = strings.Trim(line, "\"")
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}

	itemName := strings.Trim(fields[0], "\"")
	count, _ := strconv.Atoi(fields[1])
	refillTime := 1
	if len(fields) >= 3 {
		refillTime, _ = strconv.Atoi(fields[2])
	}
	if refillTime <= 0 {
		refillTime = 1
	}

	script.Goods = append(script.Goods, ScriptGoods{
		ItemName:   itemName,
		Count:      count,
		RefillTime: refillTime,
	})
}

// Execute 执行指定标签的所有过程块（Delphi ObjNpc.pas:8508-8540 GotoLable）。
// 多块按顺序执行，对话文本累积后一次性发送；BREAK 动作中止后续块。
func (s *NpcScript) Execute(label string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	sections, ok := s.Labels[label]
	if !ok {
		sections, ok = s.Labels["main"]
		if !ok {
			return
		}
	}

	var sayParts []string
	for _, section := range sections {
		conditionsMet := true
		if len(section.Conditions) > 0 {
			conditionsMet = s.evalConditions(section.Conditions, p)
		}

		if conditionsMet {
			if !s.execActions(section.Actions, p, npc, server) {
				break
			}
			sayParts = append(sayParts, section.SayText...)
		} else {
			if !s.execActions(section.ElseAct, p, npc, server) {
				break
			}
			sayParts = append(sayParts, section.ElseSay...)
		}
	}

	if len(sayParts) > 0 {
		text := strings.Join(sayParts, "\\")
		s.sendMerchantSay(text, p, npc, server)
	}
}

// sendMerchantSay 发送 NPC 对话文本（变量替换 + NPCName/text 格式 + 标签白名单提取）。
// Delphi: ObjNpc.pas:8428-8451 SendMerChantSayMsg + GetScriptLabel。
func (s *NpcScript) sendMerchantSay(text string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	text = s.replaceVars(text, p)
	p.CanJmpLabels = extractDialogLabels(text)
	body := npc.Name + "/" + text
	resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, uint16(npc.Face), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(body))
}

// extractDialogLabels 从对话文本中提取所有 <显示文本/@label> 的 label 部分。
// Delphi: ObjBase.pas:25372-25399 GetScriptLabel。
func extractDialogLabels(text string) []string {
	var labels []string
	for _, line := range strings.Split(text, "\\") {
		rest := line
		for {
			lt := strings.IndexByte(rest, '<')
			if lt < 0 {
				break
			}
			gt := strings.IndexByte(rest[lt:], '>')
			if gt < 0 {
				break
			}
			tag := rest[lt+1 : lt+gt]
			rest = rest[lt+gt+1:]
			// 跳过 <C>/</C> 居中控制符与空标签 (客户端 parseNpcDialog 同样特判,
			// 否则 </C> 会被误收成标签 "C")。
			if tag == "" || tag == "C" || tag == "/C" {
				continue
			}
			if slash := strings.IndexByte(tag, '/'); slash >= 0 {
				labels = append(labels, strings.TrimPrefix(tag[slash+1:], "@"))
			}
		}
	}
	return labels
}

func (s *NpcScript) evalConditions(conditions []string, p *PlayObject) bool {
	for _, cond := range conditions {
		if !s.evalOneCondition(cond, p) {
			return false
		}
	}
	return true
}

func (s *NpcScript) evalOneCondition(cond string, p *PlayObject) bool {
	parts := strings.Fields(cond)
	if len(parts) == 0 {
		return true
	}

	cmd := strings.ToUpper(parts[0])
	switch cmd {
	case "CHECK":
		// CHECK [N] value — 任务标志检查
		if len(parts) < 3 {
			return true
		}
		flagStr := strings.Trim(parts[1], "[]")
		idx, _ := strconv.Atoi(flagStr)
		val, _ := strconv.Atoi(parts[2])
		if idx >= 0 && idx < 10 {
			return p.ScriptVars[idx] == val
		}
		return true
	case "CHECKLEVEL":
		val, op := parseConditionValue(parts)
		level := int(p.WAbil.Level)
		return compareOp(level, op, val)
	case "CHECKGOLD":
		val, op := parseConditionValue(parts)
		return compareOp(p.Gold, op, val)
	case "CHECKJOB":
		if len(parts) < 2 {
			return true
		}
		jobName := parts[1]
		switch strings.ToLower(jobName) {
		case "warrior", "战士":
			return p.Job == 0
		case "wizard", "法师":
			return p.Job == 1
		case "taoist", "道士":
			return p.Job == 2
		}
		return false
	case "CHECKITEM":
		if len(parts) < 2 {
			return true
		}
		itemName := parts[1]
		// 特殊处理：金币
		if itemName == "金币" || strings.EqualFold(itemName, "gold") {
			count := 1
			if len(parts) >= 3 {
				count, _ = strconv.Atoi(parts[2])
			}
			return p.Gold >= count
		}
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		return p.countItem(itemName) >= count
	case "CHECKBAGGAGE":
		return len(p.ItemList) < p.Engine.Config.GetMaxBagSlots()
	case "CHECKHP":
		val, op := parseConditionValue(parts)
		return compareOp(int(p.WAbil.HP), op, val)
	case "CHECKMP":
		val, op := parseConditionValue(parts)
		return compareOp(int(p.WAbil.MP), op, val)
	case "CHECKPKPOINT":
		val, op := parseConditionValue(parts)
		return compareOp(p.PkPoint, op, val)
	case "CHECKDC":
		val, op := parseConditionValue(parts)
		hiDC := int(p.WAbil.DC >> 16)
		return compareOp(hiDC, op, val)
	case "CHECKMC":
		val, op := parseConditionValue(parts)
		hiMC := int(p.WAbil.MC >> 16)
		return compareOp(hiMC, op, val)
	case "CHECKSC":
		val, op := parseConditionValue(parts)
		hiSC := int(p.WAbil.SC >> 16)
		return compareOp(hiSC, op, val)
	case "CHECKEXP":
		val, op := parseConditionValue(parts)
		return compareOp(int(p.WAbil.Exp), op, val)
	case "CHECKGENDER", "GENDER":
		if len(parts) < 2 {
			return true
		}
		switch strings.ToLower(parts[1]) {
		case "man", "男":
			return p.Gender == 0
		case "woman", "女":
			return p.Gender == 1
		}
		return false
	case "CHECKMAP":
		if len(parts) < 2 {
			return true
		}
		return strings.EqualFold(p.MapName, parts[1])
	case "HASGUILD":
		return p.GuildName != ""
	case "CHECKMARRY":
		return p.IsMarried()
	case "ISGUILDMASTER":
		return p.GuildName != "" && p.GuildRank == "master"
	case "CHECKMASTER":
		return p.HasMaster()
	case "HAVEMASTER":
		return p.IsMaster()
	case "CHECKGROUPCOUNT":
		val, op := parseConditionValue(parts)
		count := 1
		if p.Engine != nil {
			p.Engine.mu.RLock()
			if party, ok := p.Engine.Parties[p.ID]; ok {
				count = len(party.Members)
			}
			p.Engine.mu.RUnlock()
		}
		return compareOp(count, op, val)
	case "RANDOM":
		if len(parts) < 2 {
			return true
		}
		n, _ := strconv.Atoi(parts[1])
		if n <= 0 {
			return true
		}
		return rand.Intn(n) == 0
	case "CHECKITEMW":
		if len(parts) < 2 {
			return true
		}
		slot := strings.ToUpper(strings.Trim(parts[1], "[]"))
		if !p.hasEquipmentInSlot(slot) {
			return false
		}
		// 可选第3参数：检查装备物品名 (Delphi ObjNpc.pas:7080-7085)
		if len(parts) >= 3 {
			name := strings.Trim(parts[2], "\"")
			slotIdx := p.slotNameToIndex(slot)
			if slotIdx >= 0 && slotIdx < len(p.UseItems) {
				item := p.UseItems[slotIdx]
				if item != nil && p.ItemDB != nil {
					def := p.ItemDB.GetByIdx(int(item.WIndex))
					return def != nil && def.Name == name
				}
				return false
			}
		}
		return true
	case "DAYTIME":
		if len(parts) < 2 {
			return true
		}
		hour := time.Now().Hour()
		switch strings.ToUpper(parts[1]) {
		case "SUNRAISE", "SUNRISE":
			return hour >= 5 && hour < 8
		case "DAY":
			return hour >= 8 && hour < 17
		case "SUNSET":
			return hour >= 17 && hour < 20
		case "NIGHT":
			return hour >= 20 || hour < 5
		}
		return true
	case "DAYOFWEEK":
		if len(parts) < 2 {
			return true
		}
		day := strings.ToUpper(parts[1])
		weekdays := []string{"SUN", "MON", "TUE", "WED", "THU", "FRI", "SAT"}
		today := weekdays[time.Now().Weekday()]
		return day == today
	case "HOUR":
		if len(parts) < 2 {
			return true
		}
		h, _ := strconv.Atoi(parts[1])
		hour := time.Now().Hour()
		if len(parts) >= 3 {
			h2, _ := strconv.Atoi(parts[2])
			return hour >= h && hour <= h2
		}
		return hour == h
	case "CHECKMONMAP":
		if len(parts) < 3 {
			return true
		}
		mapName := parts[1]
		count, _ := strconv.Atoi(parts[2])
		if p.Engine == nil {
			return true
		}
		n := 0
		p.Engine.mu.RLock()
		for _, m := range p.Engine.Monsters {
			if m.MapName == mapName && !m.Ghost && !m.Death {
				n++
			}
		}
		p.Engine.mu.RUnlock()
		return n >= count
	case "CHECKHUM":
		if len(parts) < 3 {
			return true
		}
		mapName := parts[1]
		count, _ := strconv.Atoi(parts[2])
		if p.Engine == nil {
			return true
		}
		n := 0
		p.Engine.mu.RLock()
		for _, pl := range p.Engine.PlayObjectList {
			if pl.MapName == mapName && !pl.Ghost {
				n++
			}
		}
		p.Engine.mu.RUnlock()
		return n >= count
	case "ISSYSOP":
		return p.Permission >= 4
	case "ISADMIN":
		return p.Permission >= 6
	case "EQUAL":
		return p.evalVarCondition(parts, "==")
	case "LARGE":
		return p.evalVarCondition(parts, ">")
	case "SMALL":
		return p.evalVarCondition(parts, "<")
	case "CHECKDURA":
		if len(parts) < 3 {
			return true
		}
		itemName := parts[1]
		val, _ := strconv.Atoi(parts[2])
		if p.ItemDB == nil {
			return false
		}
		def := p.ItemDB.GetByName(itemName)
		if def == nil {
			return false
		}
		for _, item := range p.ItemList {
			if item != nil && int(item.WIndex) == def.Idx {
				return int(item.Dura) >= val
			}
		}
		return false
	case "CHECKDURAMAX":
		if len(parts) < 3 {
			return true
		}
		itemName := parts[1]
		val, _ := strconv.Atoi(parts[2])
		if p.ItemDB == nil {
			return false
		}
		def := p.ItemDB.GetByName(itemName)
		if def == nil {
			return false
		}
		for _, item := range p.ItemList {
			if item != nil && int(item.WIndex) == def.Idx {
				return int(item.DuraMax) >= val
			}
		}
		return false
	case "CHECKGAMEGOLD":
		val, op := parseConditionValue(parts)
		return compareOp(p.Gold, op, val)
	case "CHECKGAMEPOINT":
		return true
	case "CHECKCREDITPOINT":
		return true
	case "CHECKSLAVECOUNT":
		val, op := parseConditionValue(parts)
		p.cleanSlaveList()
		return compareOp(len(p.SlaveIDs), op, val)
	case "CHECKSLAVELEVEL":
		val, op := parseConditionValue(parts)
		return compareOp(p.SlaveLevel, op, val)
	case "CHECKSLAVENAME":
		if len(parts) < 2 {
			return false
		}
		name := parts[1]
		for _, mon := range p.Engine.Monsters {
			if mon.PlayerMasterID == p.ID && !mon.Death && mon.Name == name {
				return true
			}
		}
		return false
	case "CHECKPOSELEVEL":
		val, op := parseConditionValue(parts)
		if pose := p.getFacingPlayer(); pose != nil {
			return compareOp(int(pose.WAbil.Level), op, val)
		}
		return false
	case "CHECKPOSEGENDER":
		if len(parts) < 2 || p.getFacingPlayer() == nil {
			return false
		}
		pose := p.getFacingPlayer()
		switch strings.ToLower(parts[1]) {
		case "man", "男", "0":
			return pose.Gender == 0
		case "woman", "女", "1":
			return pose.Gender == 1
		}
		return false
	case "CHECKPOSEDIR":
		if len(parts) < 2 || p.getFacingPlayer() == nil {
			return false
		}
		dir, _ := strconv.Atoi(parts[1])
		return p.getFacingPlayer().Dir == dir
	// E2: 任务条件
	case "CHECKOPENQUEST", "CHECKOPEN":
		if len(parts) < 3 {
			return false
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		return questGetBit(&p.QuestUnitOpen, idx) == (val == 1)
	case "CHECKQUEST", "CHECKUNIT":
		if len(parts) < 3 {
			return false
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		return questGetBit(&p.QuestUnit, idx) == (val == 1)
	case "CHECKFLAG":
		if len(parts) < 3 {
			return false
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		return questGetBit(&p.QuestFlag, idx) == (val == 1)
	case "MIN":
		if len(parts) < 2 {
			return false
		}
		minute, _ := strconv.Atoi(parts[1])
		return time.Now().Minute() == minute
	case "CHECKLUCKYPOINT":
		val, op := parseConditionValue(parts)
		return compareOp(p.Luck, op, val)
	case "CHECKNAMELIST":
		if len(parts) < 3 {
			return false
		}
		return p.inNameList(parts[1], parts[2])
	case "CHECKACCOUNTLIST":
		if len(parts) < 3 {
			return false
		}
		return p.inNameList(parts[1], p.AccountName)
	case "CHECKIPLIST":
		return true
	case "CHECKLEVELEX":
		if len(parts) < 3 {
			return false
		}
		lo, _ := strconv.Atoi(parts[1])
		hi, _ := strconv.Atoi(parts[2])
		lv := int(p.WAbil.Level)
		return lv >= lo && lv <= hi
	case "CHECKBONUSPOINT":
		val, op := parseConditionValue(parts)
		return compareOp(p.BonusPoint, op, val)
	case "CHECKCASTLEMASTER":
		// Delphi: 是否城主（城主行会会长）(ObjNpc.pas:7296)
		if p.Engine == nil || p.Engine.Castle == nil {
			return false
		}
		castle := p.Engine.Castle
		if castle.OwnerGuild == "" || p.GuildName == "" {
			return false
		}
		if castle.OwnerGuild != p.GuildName {
			return false
		}
		g := p.Engine.FindGuild(p.GuildName)
		return g != nil && g.Leader == p.Name
	case "ISCASTLEGUILD":
		// Delphi: 是否城主行会成员 (ObjNpc.pas:7297)
		if p.Engine == nil || p.Engine.Castle == nil {
			return false
		}
		return p.Engine.Castle.OwnerGuild != "" && p.Engine.Castle.OwnerGuild == p.GuildName
	case "ISATTACKGUILD":
		// Delphi: 是否攻击行会 (ObjNpc.pas:7298)
		if p.Engine == nil || p.Engine.Castle == nil {
			return false
		}
		return p.Engine.Castle.IsAttackingGuild(p.GuildName)
	case "ISDEFENSEGUILD":
		// Delphi: 是否防守行会 (ObjNpc.pas:7299)
		if p.Engine == nil || p.Engine.Castle == nil {
			return false
		}
		return p.Engine.Castle.IsDefendingGuild(p.GuildName)

	// Delphi ObjNpc.pas:5525-5536 CHECKPOS mapName x y
	case "CHECKPOS":
		if len(parts) < 4 {
			return true
		}
		x, _ := strconv.Atoi(parts[2])
		y, _ := strconv.Atoi(parts[3])
		return p.MapName == parts[1] && p.CurrX == x && p.CurrY == y

	// Delphi ObjNpc.pas:4907-4926 CHECKINMAPRANGE map x y range
	case "CHECKINMAPRANGE":
		if len(parts) < 5 {
			return true
		}
		x, _ := strconv.Atoi(parts[2])
		y, _ := strconv.Atoi(parts[3])
		r, _ := strconv.Atoi(parts[4])
		if p.MapName != parts[1] {
			return false
		}
		dx := p.CurrX - x
		dy := p.CurrY - y
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		return dx <= r && dy <= r

	// Delphi ObjNpc.pas:10080-10097 CHECKUSEITEM slotIndex
	case "CHECKUSEITEM":
		if len(parts) < 2 {
			return true
		}
		slot, _ := strconv.Atoi(parts[1])
		return slot >= 0 && slot < len(p.UseItems) && p.UseItems[slot] != nil && p.UseItems[slot].WIndex > 0

	// Delphi ObjNpc.pas:4207-4221 CHECKBAGSIZE count
	case "CHECKBAGSIZE":
		if len(parts) < 2 {
			return true
		}
		n, _ := strconv.Atoi(parts[1])
		return len(p.ItemList)+n <= p.Engine.Config.GetMaxBagSlots()

	// Delphi ObjNpc.pas:10384-10400 CHECKOFGUILD guildName
	case "CHECKOFGUILD":
		if len(parts) < 2 {
			return true
		}
		return strings.EqualFold(p.GuildName, parts[1])

	// Delphi ObjNpc.pas:5485-5492 CHECKSERVERNAME name
	case "CHECKSERVERNAME":
		if len(parts) < 2 {
			return true
		}
		if p.Engine == nil || p.Engine.Config == nil {
			return false
		}
		return strings.EqualFold(p.Engine.Config.Server.Name, parts[1])

	// Delphi ObjNpc.pas:5583-5600 CHECKMAGICLVL magicName level
	case "CHECKMAGICLVL":
		if len(parts) < 3 {
			return true
		}
		lvl, _ := strconv.Atoi(parts[2])
		for _, m := range p.MagicList {
			if m.MagicInfo != nil {
				name := string(m.MagicInfo.SMagicName[:])
				if i := strings.IndexByte(name, 0); i >= 0 {
					name = name[:i]
				}
				if strings.EqualFold(strings.TrimSpace(name), parts[1]) {
					return int(m.Level) == lvl
				}
			}
		}
		return false

	// Delphi ObjNpc.pas:11131-11153 CHECKMAPHUMANCOUNT map op count
	case "CHECKMAPHUMANCOUNT":
		if len(parts) < 4 {
			return true
		}
		val, op := parseConditionValue(parts[1:])
		if p.Engine == nil {
			return false
		}
		count := p.Engine.CountMapHumans(parts[1])
		return compareOp(count, op, val)

	// Delphi ObjNpc.pas:11157-11182 CHECKMAPMONCOUNT map op count
	case "CHECKMAPMONCOUNT":
		if len(parts) < 4 {
			return true
		}
		val, op := parseConditionValue(parts[1:])
		if p.Engine == nil {
			return false
		}
		count := p.Engine.CountMapMonsters(parts[1])
		return compareOp(count, op, val)

	default:
		return true
	}
}

// parseConditionValue 解析条件参数：支持 "CMD op val" 和 "CMD val"（默认 >=）两种格式。
func parseConditionValue(parts []string) (int, string) {
	if len(parts) >= 3 {
		val, err := strconv.Atoi(parts[2])
		if err == nil {
			return val, parts[1]
		}
		// parts[1] 可能是数值（无运算符格式）
		val, _ = strconv.Atoi(parts[1])
		return val, ">="
	}
	if len(parts) >= 2 {
		val, _ := strconv.Atoi(parts[1])
		return val, ">="
	}
	return 0, ">="
}

// execActions 执行动作列表，返回 false 表示遇到 BREAK 应中止后续过程块。
func (s *NpcScript) execActions(actions []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	for _, act := range actions {
		if !s.execOneAction(act, p, npc, server) {
			return false
		}
	}
	return true
}

// execOneAction 返回 false 表示 BREAK。
func (s *NpcScript) execOneAction(act string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) bool {
	parts := strings.Fields(act)
	if len(parts) == 0 {
		return true
	}

	cmd := strings.ToUpper(parts[0])
	switch cmd {
	case "GIVE":
		if len(parts) < 2 {
			return true
		}
		itemName := parts[1]
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		// 特殊处理：金币
		if itemName == "金币" || strings.EqualFold(itemName, "gold") {
			p.Gold += count
			resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			return true
		}
		if p.ItemDB != nil {
			def := p.ItemDB.GetByName(itemName)
			if def != nil {
				for i := 0; i < count; i++ {
					p.GiveItem(def.Idx)
				}
				p.SendBagItemsFull(server)
			}
		}
	case "TAKE":
		if len(parts) < 2 {
			return true
		}
		itemName := parts[1]
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		// 特殊处理：金币
		if itemName == "金币" || strings.EqualFold(itemName, "gold") {
			p.Gold -= count
			if p.Gold < 0 {
				p.Gold = 0
			}
			resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
			server.Send(p.Session.ID, resp, "")
			return true
		}
		p.takeItem(itemName, count)
		p.SendBagItemsFull(server)
	case "SENDMSG":
		// SENDMSG <type> <text> — 10种广播频道 (Delphi ObjNpc.pas:3364-3386)
		if len(parts) < 3 {
			return true
		}
		msgType, err := strconv.Atoi(parts[1])
		if err != nil {
			return true
		}
		text := strings.Join(parts[2:], " ")
		text = strings.ReplaceAll(text, "%s", p.Name)
		text = strings.ReplaceAll(text, "%d", npc.Name)
		text = s.replaceVars(text, p)
		s.sendMsgByType(msgType, text, p, npc, server)
	case "MESSAGEBOX":
		// Delphi: RM_MENU_OK → SM_MENU_OK(767) 模态弹窗 (ObjNpc.pas:3904-3908)
		if len(parts) < 2 {
			return true
		}
		text := strings.Join(parts[1:], " ")
		text = s.replaceVars(text, p)
		msg := protocol.MakeDefaultMsg(protocol.SMMenuOK, 0, 0, 0, 0)
		server.Send(p.Session.ID, msg, protocol.EncodeString(text))
	case "CHANGELEVEL":
		if len(parts) < 2 {
			return true
		}
		lvl, _ := strconv.Atoi(parts[1])
		if lvl > 0 && lvl <= 500 {
			p.WAbil.Level = uint16(lvl)
			p.RecalcAbilitys()
			p.sendHealthSpell(server)
		}
	case "ADDGOLD", "GAMEGOLD":
		if len(parts) < 2 {
			return true
		}
		gold, _ := strconv.Atoi(parts[1])
		p.Gold += gold
		if p.Gold < 0 {
			p.Gold = 0
		}
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	case "TAKEGOLD":
		if len(parts) < 2 {
			return true
		}
		gold, _ := strconv.Atoi(parts[1])
		p.Gold -= gold
		if p.Gold < 0 {
			p.Gold = 0
		}
		resp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	case "MAPMOVE", "MAP":
		if len(parts) < 2 {
			return true
		}
		mapName := parts[1]
		x, y := 100, 100
		if len(parts) >= 4 {
			x, _ = strconv.Atoi(parts[2])
			y, _ = strconv.Atoi(parts[3])
		}
		if p.MapMgr != nil {
			if env := p.MapMgr.FindMap(mapName); env != nil {
				p.EnterAnotherMap(server, env, x, y)
			}
		}
	case "GOTO":
		if len(parts) < 2 {
			return true
		}
		p.ScriptGotoCount++
		if p.ScriptGotoCount > 10 {
			return true
		}
		label := strings.TrimPrefix(parts[1], "@")
		s.Execute(label, p, npc, server)
	case "STORAGE", "SAVEITEM":
		p.sendStorageMenu(server)
	case "CLOSE":
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	case "BREAK":
		return false
	case "ADDSKILL":
		if len(parts) < 2 {
			return true
		}
		magicID, _ := strconv.Atoi(parts[1])
		level := 0
		if len(parts) >= 3 {
			level, _ = strconv.Atoi(parts[2])
		}
		p.learnMagic(magicID, level, 1)
		p.SendMyMagicFull(server)
	case "DELSKILL":
		if len(parts) < 2 {
			return true
		}
		magicID, _ := strconv.Atoi(parts[1])
		p.removeMagic(magicID)
		p.SendMyMagicFull(server)
	case "CHANGEEXP":
		if len(parts) < 2 {
			return true
		}
		exp, _ := strconv.Atoi(parts[1])
		p.WAbil.Exp += uint32(exp)
		maxExp := p.GetMaxExp()
		for p.WAbil.Exp >= maxExp {
			p.WAbil.Exp -= maxExp
			p.WAbil.Level++
			maxExp = p.GetMaxExp()
		}
		p.RecalcAbilitys()
		p.sendHealthSpell(server)
	case "CHANGEPKPOINT":
		if len(parts) < 2 {
			return true
		}
		pk, _ := strconv.Atoi(parts[1])
		p.PkPoint += pk
		if p.PkPoint < 0 {
			p.PkPoint = 0
		}
	case "HUMANHP":
		if len(parts) < 3 {
			return true
		}
		val, _ := strconv.Atoi(parts[2])
		switch strings.ToUpper(parts[1]) {
		case "+", "INC":
			p.WAbil.HP += uint16(val)
			if p.WAbil.HP > p.WAbil.MaxHP {
				p.WAbil.HP = p.WAbil.MaxHP
			}
		case "-", "DEC":
			if uint16(val) > p.WAbil.HP {
				p.WAbil.HP = 1
			} else {
				p.WAbil.HP -= uint16(val)
			}
		}
		p.sendHealthSpell(server)
	case "HUMANMP":
		if len(parts) < 3 {
			return true
		}
		val, _ := strconv.Atoi(parts[2])
		switch strings.ToUpper(parts[1]) {
		case "+", "INC":
			p.WAbil.MP += uint16(val)
			if p.WAbil.MP > p.WAbil.MaxMP {
				p.WAbil.MP = p.WAbil.MaxMP
			}
		case "-", "DEC":
			if uint16(val) > p.WAbil.MP {
				p.WAbil.MP = 0
			} else {
				p.WAbil.MP -= uint16(val)
			}
		}
		p.sendHealthSpell(server)
	case "KICK":
		p.Ghost = true
		if p.envir != nil {
			p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
		}
	case "KILL":
		p.WAbil.HP = 0
		p.Death = true
		p.deathTick = time.Now().UnixMilli()
		if p.envir != nil {
			p.envir.broadcastDeathMsg(p.BaseObject, p.ID, p.CurrX, p.CurrY, p.Dir, true)
		}
	case "CHANGEGENDER":
		if len(parts) < 2 {
			return true
		}
		switch strings.ToLower(parts[1]) {
		case "man", "男", "0":
			p.Gender = 0
		case "woman", "女", "1":
			p.Gender = 1
		}
		p.updateAppearance()
	case "CHANGEJOB":
		if len(parts) < 2 {
			return true
		}
		switch strings.ToLower(parts[1]) {
		case "warrior", "战士", "0":
			p.Job = 0
		case "wizard", "法师", "1":
			p.Job = 1
		case "taoist", "道士", "2":
			p.Job = 2
		}
	case "LINEMSG":
		// LINEMSG 与 SENDMSG 共享处理 (Delphi ActionOfLineMsg)
		if len(parts) < 3 {
			return true
		}
		msgType, err := strconv.Atoi(parts[1])
		if err != nil {
			return true
		}
		text := strings.Join(parts[2:], " ")
		text = strings.ReplaceAll(text, "%s", p.Name)
		text = strings.ReplaceAll(text, "%d", npc.Name)
		text = s.replaceVars(text, p)
		s.sendMsgByType(msgType, text, p, npc, server)
	case "MONGEN":
		if len(parts) < 2 {
			return true
		}
		monName := parts[1]
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		if p.Engine != nil && p.envir != nil {
			now := time.Now().UnixMilli()
			for i := 0; i < count; i++ {
				x := p.CurrX + rand.Intn(5) - 2
				y := p.CurrY + rand.Intn(5) - 2
				p.Engine.SpawnMonsterByName(p.MapName, x, y, monName, now)
			}
		}
	case "MONCLEAR":
		if p.envir != nil && p.Engine != nil {
			p.Engine.mu.Lock()
			var remaining []*MonsterObject
			for _, mon := range p.Engine.Monsters {
				if mon.MapName == p.MapName && abs(mon.CurrX-p.CurrX) <= 10 && abs(mon.CurrY-p.CurrY) <= 10 {
					mon.Ghost = true
					p.envir.RemoveObject(mon.CurrX, mon.CurrY, OS_MOVINGOBJECT, mon)
				} else {
					remaining = append(remaining, mon)
				}
			}
			p.Engine.Monsters = remaining
			p.Engine.mu.Unlock()
		}
	case "SET":
		if len(parts) < 3 {
			return true
		}
		// 支持 SET [N] val 和 SET P0 val 两种格式
		varName := strings.Trim(parts[1], "[]")
		val, _ := strconv.Atoi(parts[2])
		if idx, err := strconv.Atoi(varName); err == nil {
			// 纯数字：任务标志（存入 P 变量）
			if idx >= 0 && idx < 10 {
				p.ScriptVars[idx] = val
			}
		} else {
			p.setScriptVar(varName, val)
		}
	case "INC":
		if len(parts) < 2 {
			return true
		}
		varName := strings.Trim(parts[1], "[]")
		val := 1
		if len(parts) >= 3 {
			val, _ = strconv.Atoi(parts[2])
		}
		if idx, err := strconv.Atoi(varName); err == nil {
			if idx >= 0 && idx < 10 {
				p.ScriptVars[idx] += val
			}
		} else {
			cur := p.getScriptVar(varName)
			p.setScriptVar(varName, cur+val)
		}
	case "DEC":
		if len(parts) < 2 {
			return true
		}
		varName := strings.Trim(parts[1], "[]")
		val := 1
		if len(parts) >= 3 {
			val, _ = strconv.Atoi(parts[2])
		}
		if idx, err := strconv.Atoi(varName); err == nil {
			if idx >= 0 && idx < 10 {
				p.ScriptVars[idx] -= val
			}
		} else {
			cur := p.getScriptVar(varName)
			p.setScriptVar(varName, cur-val)
		}
	case "MOV":
		if len(parts) < 3 {
			return true
		}
		varName := parts[1]
		val, _ := strconv.Atoi(parts[2])
		p.setScriptVar(varName, val)
	case "MOVR":
		if len(parts) < 3 {
			return true
		}
		varName := parts[1]
		n, _ := strconv.Atoi(parts[2])
		if n > 0 {
			p.setScriptVar(varName, rand.Intn(n))
		}
	case "SUM":
		if len(parts) < 3 {
			return true
		}
		v1 := p.getScriptVar(parts[1])
		v2 := p.getScriptVar(parts[2])
		p.ScriptVars[9] = v1 + v2
	case "RESET":
		if len(parts) < 3 {
			return true
		}
		startStr := strings.Trim(parts[1], "[]")
		start, _ := strconv.Atoi(startStr)
		count, _ := strconv.Atoi(parts[2])
		for i := start; i < start+count && i < 10; i++ {
			if i >= 0 {
				p.ScriptVars[i] = 0
			}
		}
	case "TAKEW":
		if len(parts) < 2 {
			return true
		}
		slot := strings.ToUpper(strings.Trim(parts[1], "[]"))
		slotMap := map[string]int{
			"DRESS": protocol.UDress, "WEAPON": protocol.UWeapon,
			"NECKLACE": protocol.UNecklace, "HELMET": protocol.UHelmet,
			"RING": protocol.URingL, "ARMRING": protocol.UArmRingL,
			"BUJUK": protocol.UBujuk,
		}
		if idx, ok := slotMap[slot]; ok {
			if p.UseItems[idx] != nil {
				p.UseItems[idx] = nil
				p.RecalcAbilitys()
				p.SendUseItemsFull(server)
			}
		}
	case "RECALLMOB":
		if len(parts) < 2 {
			return true
		}
		monName := parts[1]
		level := 1
		if len(parts) >= 3 {
			level, _ = strconv.Atoi(parts[2])
		}
		if p.Engine != nil && p.envir != nil {
			now := time.Now().UnixMilli()
			x := p.CurrX + rand.Intn(3) - 1
			y := p.CurrY + rand.Intn(3) - 1
			mon := p.Engine.SpawnMonsterByName(p.MapName, x, y, monName, now)
			if mon != nil {
				mon.MasterID = p.ID
				mon.WAbil.Level = uint16(level)
			}
		}
	case "SKILLLEVEL":
		if len(parts) < 3 {
			return true
		}
		magicID, _ := strconv.Atoi(parts[1])
		level, _ := strconv.Atoi(parts[2])
		p.learnMagic(magicID, level, 0)
		p.SendMyMagicFull(server)
	case "CLEARSKILL":
		p.LearnedMagics = nil
		p.SendMyMagicFull(server)
	case "GAMEPOINT":
	case "CHANGENAMECOLOR":
	case "TIMERECALL":
		// TIMERECALL <minutes> — 定时传送回当前位置 (Delphi ObjNpc.pas:7800-7807)
		if len(parts) >= 2 && p.envir != nil {
			minutes, _ := strconv.Atoi(parts[1])
			if minutes > 0 {
				p.TimeRecall = true
				p.TimeRecallTick = time.Now().UnixMilli() + int64(minutes)*60000
				p.RecallMap = p.envir.Name
				p.RecallX, p.RecallY = p.CurrX, p.CurrY
			}
		}
	case "BREAKTIMERECALL":
		p.TimeRecall = false
	case "GROUPRECALL":
		if p.Engine != nil {
			for _, pl := range p.Engine.PlayObjectList {
				if pl.ID != p.ID && pl.AllowGroup && pl.envir != nil {
					p.summonPlayerTo(pl, server)
				}
			}
		}
	case "GUILDRECALL":
		if p.GuildName != "" && p.Engine != nil {
			for _, pl := range p.Engine.PlayObjectList {
				if pl.GuildName == p.GuildName && pl.ID != p.ID && pl.envir != nil {
					p.summonPlayerTo(pl, server)
				}
			}
		}
	case "GROUPMOVEMAP":
		// 移动全队到目标地图 (Delphi ObjNpc.pas:8275-8305)
		if len(parts) < 2 || p.MapMgr == nil {
			return true
		}
		mapName := parts[1]
		newEnvir := p.MapMgr.FindMap(mapName)
		if newEnvir == nil {
			return true
		}
		tx, ty := newEnvir.Width/2, newEnvir.Height/2
		p.EnterAnotherMap(server, newEnvir, tx, ty)
		if party := p.partyOf(); party != nil && p.Engine != nil {
			for _, id := range party.Members {
				if id == p.ID {
					continue
				}
				if pl := p.Engine.GetPlayer(id); pl != nil && !pl.Ghost {
					pl.EnterAnotherMap(server, newEnvir, tx, ty)
				}
			}
		}
	case "ADDNAMELIST":
		if len(parts) < 3 {
			return true
		}
		p.addNameList(parts[1], parts[2])
	case "DELNAMELIST":
		if len(parts) < 3 {
			return true
		}
		p.delNameList(parts[1], parts[2])
	case "OFFLINESENDMSG":
		if len(parts) < 3 {
			return true
		}
		p.sysMsg(server, "["+parts[1]+"] "+strings.Join(parts[2:], " "))
	case "MOVEX":
		if len(parts) < 4 {
			return true
		}
		mapName := parts[1]
		x, _ := strconv.Atoi(parts[2])
		y, _ := strconv.Atoi(parts[3])
		if p.MapMgr != nil {
			if newEnvir := p.MapMgr.FindMap(mapName); newEnvir != nil {
				p.EnterAnotherMap(server, newEnvir, x, y)
			}
		}
	case "EXCHGTAKEON":
		if len(parts) < 3 {
			return true
		}
		itemName := parts[1]
		slot, _ := strconv.Atoi(parts[2])
		if p.ItemDB != nil && slot >= 0 && slot <= 12 {
			def := p.ItemDB.GetByName(itemName)
			if def != nil {
				for i, item := range p.ItemList {
					if item != nil && int(item.WIndex) == def.Idx {
						old := p.UseItems[slot]
						p.ItemList = append(p.ItemList[:i], p.ItemList[i+1:]...)
						p.UseItems[slot] = item
						if old != nil {
							p.ItemList = append(p.ItemList, old)
						}
						p.RecalcAbilitys()
						p.SendBagItemsFull(server)
						p.SendUseItemsFull(server)
						break
					}
				}
			}
		}
	case "BREAKWEAPON":
		if p.UseItems[protocol.UWeapon] != nil {
			p.UseItems[protocol.UWeapon] = nil
			p.RecalcAbilitys()
			p.SendUseItemsFull(server)
			p.sysMsg(server, "武器已破碎")
		}
	case "DELAYCALL":
	case "NPCPAGE":
	case "CALC":
		if len(parts) < 4 {
			return true
		}
		v1 := p.getScriptVar(parts[1])
		v2, _ := strconv.Atoi(parts[3])
		op := parts[2]
		var result int
		switch op {
		case "+":
			result = v1 + v2
		case "-":
			result = v1 - v2
		case "*":
			result = v1 * v2
		case "/":
			if v2 != 0 {
				result = v1 / v2
			}
		}
		if len(parts) >= 5 {
			p.setScriptVar(parts[4], result)
		} else {
			p.ScriptVars[9] = result
		}
	case "SAVEVAR":
		if len(parts) < 3 {
			return true
		}
		globalScriptVars.mu.Lock()
		globalScriptVars.StrVars[parts[1]] = parts[2]
		globalScriptVars.mu.Unlock()
	case "LOADVAR":
		if len(parts) < 2 {
			return true
		}
		globalScriptVars.mu.RLock()
		val := globalScriptVars.StrVars[parts[1]]
		globalScriptVars.mu.RUnlock()
		p.StrScriptVars[parts[1]] = val
	case "CLEARNAMELIST":
		if len(parts) < 2 {
			return true
		}
		p.nameLists[parts[1]] = nil
	case "MARRY":
		if partner := p.getFacingPlayer(); partner != nil {
			p.Marry(server, partner.Name)
		} else {
			p.sysMsg(server, "面前没有玩家")
		}
	case "UNMARRY", "DELMARRY":
		p.Divorce(server)
	case "MASTER":
		if len(parts) >= 2 {
			p.TakeMaster(server, parts[1])
		} else if target := p.getFacingPlayer(); target != nil {
			p.TakeMaster(server, target.Name)
		} else {
			p.sysMsg(server, "面前没有玩家")
		}
	case "UNMASTER":
		p.LeaveMaster(server)
	// E2: 任务动作
	case "SETOPENQUEST", "SETOPEN":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			val, _ := strconv.Atoi(parts[2])
			if val == 1 {
				questSetBit(&p.QuestUnitOpen, idx)
			} else {
				questClearBit(&p.QuestUnitOpen, idx)
			}
		}
	case "RESETOPENQUEST", "RESETOPEN":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			count, _ := strconv.Atoi(parts[2])
			questClearRange(&p.QuestUnitOpen, idx, count)
		}
	case "SETQUEST", "SETUNIT":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			val, _ := strconv.Atoi(parts[2])
			if val == 1 {
				questSetBit(&p.QuestUnit, idx)
			} else {
				questClearBit(&p.QuestUnit, idx)
			}
		}
	case "RESETQUEST", "RESETUNIT":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			count, _ := strconv.Atoi(parts[2])
			questClearRange(&p.QuestUnit, idx, count)
		}
	case "SETFLAG":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			val, _ := strconv.Atoi(parts[2])
			if val == 1 {
				questSetBit(&p.QuestFlag, idx)
			} else {
				questClearBit(&p.QuestFlag, idx)
			}
		}
	case "RESETFLAG":
		if len(parts) >= 3 {
			idx, _ := strconv.Atoi(parts[1])
			count, _ := strconv.Atoi(parts[2])
			questClearRange(&p.QuestFlag, idx, count)
		}
	case "EXCHANGEMAP":
		if len(parts) < 3 {
			return true
		}
		p.exchangeMap(server, parts[1], parts[2])
	case "RECALLMAP":
		if len(parts) < 2 {
			return true
		}
		p.recallMap(server, parts[1])
	case "ADDGUILDLIST":
		if len(parts) < 3 {
			return true
		}
		p.addNameList("G_"+parts[1], parts[2])
	case "DELGUILDLIST":
		if len(parts) < 3 {
			return true
		}
		p.delNameList("G_"+parts[1], parts[2])
	case "ADDACCOUNTLIST":
		if len(parts) < 3 {
			return true
		}
		p.addNameList("A_"+parts[1], parts[2])
	case "DELACCOUNTLIST":
		if len(parts) < 3 {
			return true
		}
		p.delNameList("A_"+parts[1], parts[2])
	case "ADDIPLIST":
		if len(parts) < 3 {
			return true
		}
		p.addNameList("IP_"+parts[1], parts[2])
	case "DELIPLIST":
		if len(parts) < 3 {
			return true
		}
		p.delNameList("IP_"+parts[1], parts[2])
	case "GOQUEST":
		if len(parts) < 2 {
			return true
		}
		idx, _ := strconv.Atoi(parts[1])
		questSetBit(&p.QuestUnitOpen, idx)
	case "ENDQUEST":
		if len(parts) < 2 {
			return true
		}
		idx, _ := strconv.Atoi(parts[1])
		questSetBit(&p.QuestUnit, idx)
	case "MAPTING":
		if p.envir != nil {
			for i := 0; i < 20; i++ {
				nx := p.CurrX + rand.Intn(21) - 10
				ny := p.CurrY + rand.Intn(21) - 10
				if p.envir.CanWalk(nx, ny) {
					p.envir.RemoveObject(p.CurrX, p.CurrY, OS_MOVINGOBJECT, p)
					p.CurrX, p.CurrY = nx, ny
					p.envir.AddObject(nx, ny, OS_MOVINGOBJECT, p)
					p.SendRefMsg(RM_LOGON, p.Dir, nx, ny, "")
					break
				}
			}
		}
	case "DELNOJOBSKILL":
		job := int(p.Job)
		var kept []*PlayerMagic
		for _, pm := range p.LearnedMagics {
			def := p.MagicDB.GetByID(pm.MagID)
			if def != nil && def.Job != job {
				continue
			}
			kept = append(kept, pm)
		}
		p.LearnedMagics = kept
		p.SendMyMagicFull(server)
	case "MOBPLACE":
		if len(parts) < 5 {
			return true
		}
		mapName := parts[1]
		x, _ := strconv.Atoi(parts[2])
		y, _ := strconv.Atoi(parts[3])
		monName := parts[4]
		count := 1
		if len(parts) >= 6 {
			count, _ = strconv.Atoi(parts[5])
		}
		if p.Engine != nil {
			now := time.Now().UnixMilli()
			for i := 0; i < count; i++ {
				p.Engine.SpawnMonsterByName(mapName, x+i, y, monName, now)
			}
		}
	case "KILLSLAVE":
		for _, sid := range p.SlaveIDs {
			if mon := p.Engine.GetMonster(sid); mon != nil && !mon.Death {
				mon.Death = true
				mon.DeathTick = time.Now().UnixMilli()
				mon.WAbil.HP = 0
			}
		}
		p.SlaveIDs = nil
	case "KILLMONEXPRATE":
		if len(parts) < 3 {
			return true
		}
		rate, _ := strconv.Atoi(parts[1])
		duration, _ := strconv.Atoi(parts[2])
		p.KillMonExpRate = rate
		p.KillMonExpRateTick = time.Now().UnixMilli() + int64(duration)*1000
	case "POWERRATE":
		if len(parts) < 3 {
			return true
		}
		rate, _ := strconv.Atoi(parts[1])
		duration, _ := strconv.Atoi(parts[2])
		p.PowerRate = rate
		p.PowerRateTick = time.Now().UnixMilli() + int64(duration)*1000
	case "CHANGEMODE":
		if len(parts) < 2 {
			return true
		}
		mode, _ := strconv.Atoi(parts[1])
		switch mode {
		case 1:
			p.Permission = 10
		case 2:
			p.Permission = 0
		}
	case "CHANGEPERMISSION":
		if len(parts) < 2 {
			return true
		}
		perm, _ := strconv.Atoi(parts[1])
		p.Permission = byte(perm)
	case "BONUSPOINT":
		if len(parts) < 2 {
			return true
		}
		val, _ := strconv.Atoi(parts[1])
		p.BonusPoint += val
	case "RESTBONUSPOINT":
		p.BonusPoint = 0
	case "CREDITPOINT":
		if len(parts) < 2 {
			return true
		}
		val, _ := strconv.Atoi(parts[1])
		p.CreditPoint += val
	case "RENEWLEVEL":
		p.ReNewLevel++
		p.WAbil.Level = 1
		p.WAbil.Exp = 0
		p.RecalcAbilitys()
		p.WAbil.HP = p.WAbil.MaxHP
		p.WAbil.MP = p.WAbil.MaxMP
		p.SendAbility(server)
		p.sendHealthSpell(server)
	case "RESTRENEWLEVEL":
		p.ReNewLevel = 0
	case "CLEARPASSWORD":
		p.StoragePassword = ""
	case "MONGENEX":
		if len(parts) < 5 {
			return true
		}
		mapName := parts[1]
		x, _ := strconv.Atoi(parts[2])
		y, _ := strconv.Atoi(parts[3])
		monName := parts[4]
		count := 1
		if len(parts) >= 6 {
			count, _ = strconv.Atoi(parts[5])
		}
		if p.Engine != nil {
			now := time.Now().UnixMilli()
			for i := 0; i < count; i++ {
				p.Engine.SpawnMonsterByName(mapName, x, y, monName, now)
			}
		}
	case "CLEARMAPMON":
		if len(parts) < 2 {
			return true
		}
		if p.Engine != nil && p.MapMgr != nil {
			if env := p.MapMgr.FindMap(parts[1]); env != nil {
				p.Engine.clearMapMonsters(env)
			}
		}
	case "SETMAPMODE":
		// 地图模式设置（简化：仅记录日志）
	case "PKZONE":
		// PK 区域设置（简化：仅记录日志）
	case "TAKECASTLEGOLD":
		if len(parts) < 2 {
			return true
		}
		gold, _ := strconv.Atoi(parts[1])
		if p.Engine != nil && p.Engine.Castle != nil {
			if p.Engine.Castle.WithdrawGold(int64(gold)) {
				p.Gold += gold
				goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
				server.Send(p.Session.ID, goldResp, "")
			}
		}
	case "MOBFIREBURN":
		if len(parts) < 5 {
			return true
		}
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		damage, _ := strconv.Atoi(parts[3])
		duration, _ := strconv.Atoi(parts[4])
		if p.envir != nil {
			p.envir.AddFireEvent(server, x, y, damage, int64(duration)*1000, p.ID)
		}
	case "SETSCRIPTFLAG":
		if len(parts) < 3 {
			return true
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		if val == 1 {
			questSetBit(&p.QuestFlag, idx)
		} else {
			questClearBit(&p.QuestFlag, idx)
		}
	case "SETAUTOGETEXP":
		// Delphi: SETAUTOGETEXP <时间秒> <经验点> <安全区1/0> [地图名]
		if len(parts) < 3 {
			return true
		}
		nTime, _ := strconv.Atoi(parts[1])
		nPoint, _ := strconv.Atoi(parts[2])
		if nTime <= 0 || nPoint <= 0 {
			p.AutoGetExpPoint = 0
			p.AutoGetExpTime = 0
			return true
		}
		p.AutoGetExpTime = int64(nTime) * 1000
		p.AutoGetExpPoint = nPoint
		p.AutoGetExpSafeZone = len(parts) >= 4 && parts[3] == "1"
		if len(parts) >= 5 {
			p.AutoGetExpMap = parts[4]
		} else {
			p.AutoGetExpMap = ""
		}
		p.autoGetExpTick = time.Now().UnixMilli()
	case "VAR":
		if len(parts) < 4 {
			return true
		}
		v, _ := strconv.Atoi(parts[3])
		p.setScriptVar(parts[1], v)
	case "CALCVAR":
		if len(parts) < 5 {
			return true
		}
		a := p.getScriptVar(parts[2])
		b := p.getScriptVar(parts[4])
		var result int
		switch parts[3] {
		case "+":
			result = a + b
		case "-":
			result = a - b
		case "*":
			result = a * b
		case "/":
			if b != 0 {
				result = a / b
			}
		}
		p.setScriptVar(parts[1], result)
	case "GROUPADDLIST":
		if len(parts) < 3 {
			return true
		}
		p.addNameList("GRP_"+parts[1], parts[2])
	case "CLEARLIST":
		if len(parts) < 2 {
			return true
		}
		p.clearNameList(parts[1])
	case "TAKECHECKITEM":
		// 收取上次 CHECKITEM 检查的物品（简化：不做操作）
	case "PARAM1", "PARAM2", "PARAM3", "PARAM4":
		// 临时参数（简化：不做操作）
	case "EXEACTION":
		// 执行外部动作（简化：不做操作）
	case "UPGRADEITEMS", "UPGRADEITEMSEX":
		// Delphi: 随机强化当前装备武器 (ItmUnit.pas:179-226)
		weapon := p.UseItems[protocol.UWeapon]
		if weapon != nil && p.ItemDB != nil {
			if def := p.ItemDB.GetByIdx(int(weapon.WIndex)); def != nil {
				// 随机选择一个属性 +1
				switch rand.Intn(5) {
				case 0:
					weapon.BtValue[0]++
				case 1:
					weapon.BtValue[1]++
				case 2:
					weapon.BtValue[2]++
				case 3:
					weapon.BtValue[5]++
				case 4:
					weapon.BtValue[6]++
				}
				p.RecalcAbilitys()
				p.SendAbility(server)
				p.sendDuraChange(server, weapon)
			}
		}
	case "SETMEMBERTYPE", "SETMEMBERLEVEL":
		// Delphi: 设置行会成员职位 (ObjNpc.pas:8365-8366)
		if len(parts) >= 3 && p.Engine != nil {
			memberName := parts[1]
			rankName := parts[2]
			if g := p.Engine.PlayerGuild(p.Name); g != nil && g.Leader == p.Name {
				if g.IsMember(memberName) {
					g.Ranks[memberName] = rankName
				}
			}
		}
	case "AUTOADDGAMEGOLD", "AUTOSUBGAMEGOLD":
		if len(parts) >= 3 {
			gold, _ := strconv.Atoi(parts[2])
			if parts[0] == "AUTOADDGAMEGOLD" {
				p.Gold += gold
			} else {
				p.Gold -= gold
				if p.Gold < 0 {
					p.Gold = 0
				}
			}
			goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
			server.Send(p.Session.ID, goldResp, "")
		}
	case "SC_SETRANKLEVELNAME", "SETRANKLEVELNAME":
		// 设置称号（简化：不做操作）
	case "OPENMAGICBOX":
		// Delphi: 开宝箱 — 给指定物品
		if len(parts) >= 2 && p.ItemDB != nil {
			if def := p.ItemDB.GetByName(parts[1]); def != nil {
				p.GiveItem(def.Idx)
				p.SendBagItemsFull(server)
			}
		}
	default:
		_ = parts
	}
	return true
}

func (s *NpcScript) replaceVars(text string, p *PlayObject) string {
	text = strings.ReplaceAll(text, "<$USERNAME>", p.Name)
	text = strings.ReplaceAll(text, "<$LEVEL>", strconv.Itoa(int(p.WAbil.Level)))
	text = strings.ReplaceAll(text, "<$HP>", strconv.Itoa(int(p.WAbil.HP)))
	text = strings.ReplaceAll(text, "<$MAXHP>", strconv.Itoa(int(p.WAbil.MaxHP)))
	text = strings.ReplaceAll(text, "<$MP>", strconv.Itoa(int(p.WAbil.MP)))
	text = strings.ReplaceAll(text, "<$MAXMP>", strconv.Itoa(int(p.WAbil.MaxMP)))
	text = strings.ReplaceAll(text, "<$GOLDCOUNT>", strconv.Itoa(p.Gold))
	text = strings.ReplaceAll(text, "<$PKPOINT>", strconv.Itoa(p.PkPoint))
	text = strings.ReplaceAll(text, "<$GUILDNAME>", p.GuildName)
	text = strings.ReplaceAll(text, "<$SERVERNAME>", "MirGo")
	text = strings.ReplaceAll(text, "<$JOB>", jobName(p.Job))
	text = strings.ReplaceAll(text, "<$DC>", strconv.Itoa(int(p.WAbil.DC>>16)))
	text = strings.ReplaceAll(text, "<$MAXDC>", strconv.Itoa(int(p.WAbil.DC>>16)))
	text = strings.ReplaceAll(text, "<$MC>", strconv.Itoa(int(p.WAbil.MC>>16)))
	text = strings.ReplaceAll(text, "<$MAXMC>", strconv.Itoa(int(p.WAbil.MC>>16)))
	text = strings.ReplaceAll(text, "<$SC>", strconv.Itoa(int(p.WAbil.SC>>16)))
	text = strings.ReplaceAll(text, "<$MAXSC>", strconv.Itoa(int(p.WAbil.SC>>16)))
	text = strings.ReplaceAll(text, "<$AC>", strconv.Itoa(int(p.WAbil.AC&0xFFFF)))
	text = strings.ReplaceAll(text, "<$MAXAC>", strconv.Itoa(int(p.WAbil.AC>>16)))
	text = strings.ReplaceAll(text, "<$MAC>", strconv.Itoa(int(p.WAbil.MAC&0xFFFF)))
	text = strings.ReplaceAll(text, "<$MAXMAC>", strconv.Itoa(int(p.WAbil.MAC>>16)))
	text = strings.ReplaceAll(text, "<$EXP>", strconv.Itoa(int(p.WAbil.Exp)))
	text = strings.ReplaceAll(text, "<$MAXEXP>", strconv.Itoa(int(p.GetMaxExp())))
	text = strings.ReplaceAll(text, "<$DATETIME>", time.Now().Format("2006-01-02 15:04:05"))

	// 装备名称
	text = strings.ReplaceAll(text, "<$WEAPON>", p.getEquipName(protocol.UWeapon))
	text = strings.ReplaceAll(text, "<$DRESS>", p.getEquipName(protocol.UDress))
	text = strings.ReplaceAll(text, "<$HELMET>", p.getEquipName(protocol.UHelmet))
	text = strings.ReplaceAll(text, "<$NECKLACE>", p.getEquipName(protocol.UNecklace))
	text = strings.ReplaceAll(text, "<$RING_R>", p.getEquipName(protocol.URingR))
	text = strings.ReplaceAll(text, "<$RING_L>", p.getEquipName(protocol.URingL))
	text = strings.ReplaceAll(text, "<$ARMRING_R>", p.getEquipName(protocol.UArmRingR))
	text = strings.ReplaceAll(text, "<$ARMRING_L>", p.getEquipName(protocol.UArmRingL))

	// 补充变量 (Delphi ObjNpc.pas:6060-6480)
	text = strings.ReplaceAll(text, "<$GAMEGOLD>", strconv.Itoa(p.Gold))
	text = strings.ReplaceAll(text, "<$GAMEPOINT>", "0")
	text = strings.ReplaceAll(text, "<$CREDITPOINT>", "0")
	text = strings.ReplaceAll(text, "<$HUNGER>", "100")
	text = strings.ReplaceAll(text, "<$HW>", strconv.Itoa(p.getEquipWeight(protocol.UWeapon)))
	text = strings.ReplaceAll(text, "<$BW>", strconv.Itoa(p.getEquipWeight(protocol.UDress)))
	text = strings.ReplaceAll(text, "<$WW>", strconv.Itoa(p.getTotalEquipWeight()))
	text = strings.ReplaceAll(text, "<$JOB>", jobName(p.Job))

	// 兼容无尖括号格式
	text = strings.ReplaceAll(text, "$USERNAME", p.Name)
	text = strings.ReplaceAll(text, "$LEVEL", strconv.Itoa(int(p.WAbil.Level)))
	text = strings.ReplaceAll(text, "$HP", strconv.Itoa(int(p.WAbil.HP)))
	text = strings.ReplaceAll(text, "$MAXHP", strconv.Itoa(int(p.WAbil.MaxHP)))
	text = strings.ReplaceAll(text, "$MP", strconv.Itoa(int(p.WAbil.MP)))
	text = strings.ReplaceAll(text, "$MAXMP", strconv.Itoa(int(p.WAbil.MaxMP)))
	text = strings.ReplaceAll(text, "$GOLDCOUNT", strconv.Itoa(p.Gold))
	text = strings.ReplaceAll(text, "$GAMEGOLD", strconv.Itoa(p.Gold))
	text = strings.ReplaceAll(text, "$PKPOINT", strconv.Itoa(p.PkPoint))
	text = strings.ReplaceAll(text, "$GUILDNAME", p.GuildName)
	text = strings.ReplaceAll(text, "$SERVERNAME", "MirGo")

	return text
}

func (p *PlayObject) getEquipWeight(slot int) int {
	item := p.UseItems[slot]
	if item == nil || p.ItemDB == nil {
		return 0
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		return 0
	}
	return int(def.Weight)
}

func (p *PlayObject) getTotalEquipWeight() int {
	total := 0
	for i := 0; i < len(p.UseItems); i++ {
		total += p.getEquipWeight(i)
	}
	return total
}

// sendMsgByType 按频道类型发送消息 (Delphi ObjNpc.pas:3364-3386)。
func (s *NpcScript) sendMsgByType(msgType int, text string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	sysMsg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
	body := protocol.EncodeString(text)
	switch msgType {
	case 0: // 全服广播
		if p.Engine != nil {
			for _, pl := range p.Engine.allPlayers() {
				if !pl.Ghost {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		}
	case 1: // 全服广播 (*) 前缀
		text = "(*) " + text
		body = protocol.EncodeString(text)
		if p.Engine != nil {
			for _, pl := range p.Engine.allPlayers() {
				if !pl.Ghost {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		}
	case 2: // 全服广播 [NPC名] 前缀
		text = "[" + npc.Name + "] " + text
		body = protocol.EncodeString(text)
		if p.Engine != nil {
			for _, pl := range p.Engine.allPlayers() {
				if !pl.Ghost {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		}
	case 3: // 全服广播 [玩家名] 前缀
		text = "[" + p.Name + "] " + text
		body = protocol.EncodeString(text)
		if p.Engine != nil {
			for _, pl := range p.Engine.allPlayers() {
				if !pl.Ghost {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		}
	case 4: // 地图本地消息
		if p.Engine != nil && p.envir != nil {
			for _, pl := range p.Engine.allPlayers() {
				if !pl.Ghost && pl.envir != nil && pl.envir.Name == p.envir.Name {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		}
	case 5, 6, 7: // 个人消息（红/绿/蓝 — 当前统一为系统消息）
		server.Send(p.Session.ID, sysMsg, body)
	case 8: // 组队消息
		if party := p.partyOf(); party != nil && p.Engine != nil {
			for _, id := range party.Members {
				if pl := p.Engine.GetPlayer(id); pl != nil && !pl.Ghost {
					server.Send(pl.Session.ID, sysMsg, body)
				}
			}
		} else {
			server.Send(p.Session.ID, sysMsg, body)
		}
	case 9: // 行会消息
		if p.Engine != nil && p.GuildName != "" {
			guild := p.Engine.FindGuild(p.GuildName)
			if guild != nil {
				for _, name := range guild.Members {
					if pl := p.Engine.GetPlayerByName(name); pl != nil && !pl.Ghost {
						server.Send(pl.Session.ID, sysMsg, body)
					}
				}
			}
		}
	default:
		server.Send(p.Session.ID, sysMsg, body)
	}
}

func jobName(job byte) string {
	switch job {
	case 0:
		return "战士"
	case 1:
		return "法师"
	case 2:
		return "道士"
	}
	return "未知"
}

func (p *PlayObject) getEquipName(slot int) string {
	item := p.UseItems[slot]
	if item == nil || p.ItemDB == nil {
		return "无"
	}
	def := p.ItemDB.GetByIdx(int(item.WIndex))
	if def == nil {
		return "无"
	}
	return def.Name
}

func compareOp(a int, op string, b int) bool {
	switch op {
	case ">":
		return a > b
	case "<":
		return a < b
	case ">=":
		return a >= b
	case "<=":
		return a <= b
	case "=", "==":
		return a == b
	case "!=":
		return a != b
	default:
		return a >= b
	}
}

func (p *PlayObject) HandleNpcClick(msg SendMessage, server *netserver.TCPServer) {
	npcID := msg.Param1
	if p.envir == nil {
		log.Logf(log.LevelDebug, "NPC", "%s click NPC #%d rejected: no envir", p.Name, npcID)
		return
	}

	npc, ok := p.envir.getNpcByID(int32(npcID))
	if !ok {
		log.Logf(log.LevelDebug, "NPC", "%s click NPC #%d rejected: NPC not found on map %s", p.Name, npcID, p.MapName)
		return
	}

	// 距离验证：玩家必须在NPC附近才能交互
	if !p.isNearNpc(npc) {
		log.Logf(log.LevelDebug, "NPC", "%s click NPC #%d rejected: too far player=(%d,%d) npc=%s(%d,%d)",
			p.Name, npcID, p.CurrX, p.CurrY, npc.Name, npc.CurrX, npc.CurrY)
		return
	}

	// 城堡 NPC 权限检查
	if npc.Castle && !p.canAccessCastleNpc(npc) {
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
		return
	}

	script := npc.GetScript()
	if script != nil {
		npc.InitGoodsFromScript(script, p.ItemDB)
		p.ScriptGotoCount = 0
		p.ScriptGoBackLabel = ""
		p.ScriptCurrLabel = ""
		p.CurrentNpc = npc
		script.Execute("main", p, npc, server)
		return
	}

	body := npc.Name + "/" + "欢迎光临！"
	resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, uint16(npc.Face), 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(body))
}

// labelIsCanJmp 检查标签是否在玩家可见白名单中。
// Delphi: ObjBase.pas:25401-25430 LableIsCanJmp。
func (p *PlayObject) labelIsCanJmp(label string) bool {
	if strings.EqualFold(label, "main") {
		return true
	}
	for _, l := range p.CanJmpLabels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

func (p *PlayObject) hasEquipmentInSlot(slot string) bool {
	slotMap := map[string]int{
		"DRESS":     protocol.UDress,
		"WEAPON":    protocol.UWeapon,
		"RIGHTHAND": protocol.URightHand,
		"NECKLACE":  protocol.UNecklace,
		"HELMET":    protocol.UHelmet,
		"ARMRINGL":  protocol.UArmRingL,
		"ARMRINGR":  protocol.UArmRingR,
		"ARMRING":   protocol.UArmRingL,
		"RINGL":     protocol.URingL,
		"RINGR":     protocol.URingR,
		"RING":      protocol.URingL,
		"BUJUK":     protocol.UBujuk,
		"BELT":      protocol.UBelt,
		"BOOTS":     protocol.UBoots,
	}
	idx, ok := slotMap[slot]
	if !ok {
		return false
	}
	return p.UseItems[idx] != nil
}

func (p *PlayObject) slotNameToIndex(slot string) int {
	slotMap := map[string]int{
		"DRESS":     protocol.UDress,
		"WEAPON":    protocol.UWeapon,
		"RIGHTHAND": protocol.URightHand,
		"NECKLACE":  protocol.UNecklace,
		"HELMET":    protocol.UHelmet,
		"ARMRINGL":  protocol.UArmRingL,
		"ARMRINGR":  protocol.UArmRingR,
		"ARMRING":   protocol.UArmRingL,
		"RINGL":     protocol.URingL,
		"RINGR":     protocol.URingR,
		"RING":      protocol.URingL,
		"BUJUK":     protocol.UBujuk,
		"BELT":      protocol.UBelt,
		"BOOTS":     protocol.UBoots,
	}
	idx, ok := slotMap[slot]
	if !ok {
		return -1
	}
	return idx
}

func (p *PlayObject) evalVarCondition(parts []string, defaultOp string) bool {
	if len(parts) < 3 {
		return true
	}
	varName := parts[1]
	val, _ := strconv.Atoi(parts[2])

	currentVal := p.getScriptVar(varName)
	return compareOp(currentVal, defaultOp, val)
}

func (p *PlayObject) getScriptVar(name string) int {
	name = strings.ToUpper(name)
	if len(name) < 2 {
		return 0
	}
	prefix := name[0]
	idx, _ := strconv.Atoi(name[1:])

	switch prefix {
	case 'P':
		if idx >= 0 && idx < 10 {
			return p.ScriptVars[idx]
		}
	case 'G':
		if idx >= 0 && idx < 20 {
			return globalScriptVars.G[idx]
		}
	case 'D':
		if idx >= 0 && idx < 10 {
			return p.ScriptVarsD[idx]
		}
	case 'M':
		if idx >= 0 && idx < 100 {
			return p.ScriptVarsM[idx]
		}
	case 'I':
		if idx >= 0 && idx < 100 {
			globalScriptVars.mu.RLock()
			v := globalScriptVars.I[idx]
			globalScriptVars.mu.RUnlock()
			return v
		}
	}
	return 0
}

func (p *PlayObject) setScriptVar(name string, val int) {
	name = strings.ToUpper(name)
	if len(name) < 2 {
		return
	}
	prefix := name[0]
	idx, _ := strconv.Atoi(name[1:])

	switch prefix {
	case 'P':
		if idx >= 0 && idx < 10 {
			p.ScriptVars[idx] = val
		}
	case 'G':
		if idx >= 0 && idx < 20 {
			globalScriptVars.G[idx] = val
		}
	case 'D':
		if idx >= 0 && idx < 10 {
			p.ScriptVarsD[idx] = val
		}
	case 'M':
		if idx >= 0 && idx < 100 {
			p.ScriptVarsM[idx] = val
		}
	case 'I':
		if idx >= 0 && idx < 100 {
			globalScriptVars.mu.Lock()
			globalScriptVars.I[idx] = val
			globalScriptVars.mu.Unlock()
		}
	}
}

func (p *PlayObject) getFacingPlayer() *PlayObject {
	if p.envir == nil {
		return nil
	}
	dx, dy := dirToOffset(p.Dir)
	obj := p.envir.GetMovingObject(p.CurrX+dx, p.CurrY+dy)
	if obj == nil {
		return nil
	}
	if pl, ok := obj.(*PlayObject); ok && !pl.Ghost {
		return pl
	}
	return nil
}

func (p *PlayObject) summonPlayerTo(target *PlayObject, server *netserver.TCPServer) {
	if p.envir == nil || target.envir == nil {
		return
	}
	dx, dy := dirToOffset(p.Dir)
	tx, ty := p.CurrX+dx, p.CurrY+dy
	if !p.envir.CanWalk(tx, ty) {
		tx, ty = p.CurrX, p.CurrY
	}
	target.envir.RemoveObject(target.CurrX, target.CurrY, OS_MOVINGOBJECT, target)
	target.envir.broadcastRefMsg(target.BaseObject, RM_DISAPPEAR, target.ID, target.CurrX, target.CurrY, target.Dir)
	target.MapName = p.MapName
	target.CurrX, target.CurrY = tx, ty
	target.envir = p.envir
	target.envir.AddObject(tx, ty, OS_MOVINGOBJECT, target)
	target.envir.broadcastRefMsg(target.BaseObject, RM_LOGON, target.ID, tx, ty, target.Dir)
}

func (p *PlayObject) addNameList(listName, name string) {
	if p.nameLists == nil {
		p.nameLists = make(map[string][]string)
	}
	for _, n := range p.nameLists[listName] {
		if n == name {
			return
		}
	}
	p.nameLists[listName] = append(p.nameLists[listName], name)
}

func (p *PlayObject) delNameList(listName, name string) {
	list := p.nameLists[listName]
	for i, n := range list {
		if n == name {
			p.nameLists[listName] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func (p *PlayObject) inNameList(listName, name string) bool {
	for _, n := range p.nameLists[listName] {
		if n == name {
			return true
		}
	}
	return false
}

func (p *PlayObject) clearNameList(listName string) {
	p.nameLists[listName] = nil
}

func (p *PlayObject) exchangeMap(server *netserver.TCPServer, map1, map2 string) {
	if p.Engine == nil || p.MapMgr == nil {
		return
	}
	env1 := p.MapMgr.FindMap(map1)
	env2 := p.MapMgr.FindMap(map2)
	if env1 == nil || env2 == nil {
		return
	}
	for _, pl := range p.Engine.allPlayers() {
		if pl.Ghost || pl.Death {
			continue
		}
		if pl.MapName == map1 {
			pl.EnterAnotherMap(server, env2, pl.CurrX, pl.CurrY)
		} else if pl.MapName == map2 {
			pl.EnterAnotherMap(server, env1, pl.CurrX, pl.CurrY)
		}
	}
}

func (p *PlayObject) recallMap(server *netserver.TCPServer, mapName string) {
	if p.Engine == nil {
		return
	}
	for _, pl := range p.Engine.allPlayers() {
		if pl.Ghost || pl.Death || pl.MapName != mapName {
			continue
		}
		tx, ty := p.CurrX, p.CurrY
		if p.envir != nil && !p.envir.CanWalk(tx, ty) {
			tx, ty = p.CurrX+1, p.CurrY
		}
		pl.EnterAnotherMap(server, p.envir, tx, ty)
	}
}
