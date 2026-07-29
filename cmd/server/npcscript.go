package main

import (
	"bufio"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type ScriptSection struct {
	Conditions []string
	Actions    []string
	SayText    []string
	ElseSay    []string
	ElseAct    []string
}

type ScriptGoods struct {
	ItemName   string
	Count      int
	RefillTime int // 小时
}

type NpcScript struct {
	Labels map[string]*ScriptSection
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
		Labels:       make(map[string]*ScriptSection),
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
			// 处理 [@label] TRUE 格式
			if idx := strings.IndexByte(label, ' '); idx > 0 {
				label = label[:idx]
			}
			if strings.HasPrefix(label, "@") {
				label = label[1:]
			}
			currentLabel = label
			currentSection = &ScriptSection{}
			script.Labels[currentLabel] = currentSection
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

func (s *NpcScript) Execute(label string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	section, ok := s.Labels[label]
	if !ok {
		section, ok = s.Labels["main"]
		if !ok {
			return
		}
	}

	conditionsMet := true
	if len(section.Conditions) > 0 {
		conditionsMet = s.evalConditions(section.Conditions, p)
	}

	// 各行用 '\' 连接——这是 Delphi NPC 对话的行分隔符，
	// 客户端解析器（及其 <text/@label> 标签处理）按此分割。
	if conditionsMet {
		s.execActions(section.Actions, p, npc, server)
		if len(section.SayText) > 0 {
			text := strings.Join(section.SayText, "\\")
			resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
			server.Send(p.Session.ID, resp, protocol.EncodeString(text))
		}
	} else {
		s.execActions(section.ElseAct, p, npc, server)
		if len(section.ElseSay) > 0 {
			text := strings.Join(section.ElseSay, "\\")
			resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
			server.Send(p.Session.ID, resp, protocol.EncodeString(text))
		}
	}
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
		return len(p.ItemList) < MaxBagItems
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
		return p.hasEquipmentInSlot(slot)
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

func (s *NpcScript) execActions(actions []string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	for _, act := range actions {
		s.execOneAction(act, p, npc, server)
	}
}

func (s *NpcScript) execOneAction(act string, p *PlayObject, npc *NpcObject, server *netserver.TCPServer) {
	parts := strings.Fields(act)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToUpper(parts[0])
	switch cmd {
	case "GIVE":
		if len(parts) < 2 {
			return
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
			return
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
			return
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
			return
		}
		p.takeItem(itemName, count)
		p.SendBagItemsFull(server)
	case "SENDMSG", "MESSAGEBOX":
		if len(parts) < 2 {
			return
		}
		text := strings.Join(parts[1:], " ")
		text = s.replaceVars(text, p)
		msg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
		server.Send(p.Session.ID, msg, protocol.EncodeString(text))
	case "CHANGELEVEL":
		if len(parts) < 2 {
			return
		}
		lvl, _ := strconv.Atoi(parts[1])
		if lvl > 0 && lvl <= 500 {
			p.WAbil.Level = uint16(lvl)
			p.RecalcAbilitys()
			p.sendHealthSpell(server)
		}
	case "ADDGOLD", "GAMEGOLD":
		if len(parts) < 2 {
			return
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
			return
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
			return
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
			return
		}
		p.ScriptGotoCount++
		if p.ScriptGotoCount > 10 {
			return
		}
		label := strings.TrimPrefix(parts[1], "@")
		s.Execute(label, p, npc, server)
	case "STORAGE", "SAVEITEM":
		p.sendStorageMenu(server)
	case "CLOSE":
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	case "BREAK":
		return
	case "ADDSKILL":
		if len(parts) < 2 {
			return
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
			return
		}
		magicID, _ := strconv.Atoi(parts[1])
		p.removeMagic(magicID)
		p.SendMyMagicFull(server)
	case "CHANGEEXP":
		if len(parts) < 2 {
			return
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
			return
		}
		pk, _ := strconv.Atoi(parts[1])
		p.PkPoint += pk
		if p.PkPoint < 0 {
			p.PkPoint = 0
		}
	case "HUMANHP":
		if len(parts) < 3 {
			return
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
			return
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
			return
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
			return
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
		if len(parts) < 2 {
			return
		}
		text := strings.Join(parts[1:], " ")
		text = s.replaceVars(text, p)
		msg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
		server.Send(p.Session.ID, msg, protocol.EncodeString(text))
	case "MONGEN":
		if len(parts) < 2 {
			return
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
			return
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
			return
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
			return
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
			return
		}
		varName := parts[1]
		val, _ := strconv.Atoi(parts[2])
		p.setScriptVar(varName, val)
	case "MOVR":
		if len(parts) < 3 {
			return
		}
		varName := parts[1]
		n, _ := strconv.Atoi(parts[2])
		if n > 0 {
			p.setScriptVar(varName, rand.Intn(n))
		}
	case "SUM":
		if len(parts) < 3 {
			return
		}
		v1 := p.getScriptVar(parts[1])
		v2 := p.getScriptVar(parts[2])
		p.ScriptVars[9] = v1 + v2
	case "RESET":
		if len(parts) < 3 {
			return
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
			return
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
			return
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
			return
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
	case "BREAKTIMERECALL":
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
		if len(parts) < 2 || p.MapMgr == nil {
			return
		}
		mapName := parts[1]
		newEnvir := p.MapMgr.FindMap(mapName)
		if newEnvir != nil {
			p.EnterAnotherMap(server, newEnvir, newEnvir.Width/2, newEnvir.Height/2)
		}
	case "ADDNAMELIST":
		if len(parts) < 3 {
			return
		}
		p.addNameList(parts[1], parts[2])
	case "DELNAMELIST":
		if len(parts) < 3 {
			return
		}
		p.delNameList(parts[1], parts[2])
	case "OFFLINESENDMSG":
		if len(parts) < 3 {
			return
		}
		p.sysMsg(server, "["+parts[1]+"] "+strings.Join(parts[2:], " "))
	case "MOVEX":
		if len(parts) < 4 {
			return
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
			return
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
			return
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
			return
		}
		globalScriptVars.mu.Lock()
		globalScriptVars.StrVars[parts[1]] = parts[2]
		globalScriptVars.mu.Unlock()
	case "LOADVAR":
		if len(parts) < 2 {
			return
		}
		globalScriptVars.mu.RLock()
		val := globalScriptVars.StrVars[parts[1]]
		globalScriptVars.mu.RUnlock()
		p.StrScriptVars[parts[1]] = val
	case "CLEARNAMELIST":
		if len(parts) < 2 {
			return
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
	default:
		_ = parts
	}
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

	// 兼容无尖括号格式
	text = strings.ReplaceAll(text, "$USERNAME", p.Name)
	text = strings.ReplaceAll(text, "$LEVEL", strconv.Itoa(int(p.WAbil.Level)))
	text = strings.ReplaceAll(text, "$HP", strconv.Itoa(int(p.WAbil.HP)))
	text = strings.ReplaceAll(text, "$MAXHP", strconv.Itoa(int(p.WAbil.MaxHP)))
	text = strings.ReplaceAll(text, "$MP", strconv.Itoa(int(p.WAbil.MP)))
	text = strings.ReplaceAll(text, "$MAXMP", strconv.Itoa(int(p.WAbil.MaxMP)))
	text = strings.ReplaceAll(text, "$GOLDCOUNT", strconv.Itoa(p.Gold))
	text = strings.ReplaceAll(text, "$PKPOINT", strconv.Itoa(p.PkPoint))
	text = strings.ReplaceAll(text, "$GUILDNAME", p.GuildName)
	text = strings.ReplaceAll(text, "$SERVERNAME", "MirGo")

	return text
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
		return
	}

	npc, ok := p.envir.getNpcByID(int32(npcID))
	if !ok {
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
		npc.InitGoodsFromScript(script)
		p.ScriptGotoCount = 0
		p.ScriptGoBackLabel = ""
		p.ScriptCurrLabel = ""
		p.CurrentNpc = npc
		script.Execute("main", p, npc, server)
		return
	}

	dialog := npc.Name + ": 欢迎光临！"
	resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(dialog))
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
