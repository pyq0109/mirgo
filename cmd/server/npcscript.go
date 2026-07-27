package main

import (
	"bufio"
	"math/rand"
	"os"
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
}

type NpcScript struct {
	Labels map[string]*ScriptSection
}

func LoadNpcScript(path string) (*NpcScript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	script := &NpcScript{Labels: make(map[string]*ScriptSection)}
	scanner := bufio.NewScanner(f)

	var currentLabel string
	var currentSection *ScriptSection
	mode := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}

		if strings.HasPrefix(line, "[@") || strings.HasPrefix(line, "[") {
			label := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if strings.HasPrefix(label, "@") {
				label = label[1:]
			}
			currentLabel = label
			currentSection = &ScriptSection{}
			script.Labels[currentLabel] = currentSection
			mode = ""
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
		case upper == "#ELSESAY":
			mode = "ELSESAY"
			continue
		case upper == "#ELSEACT":
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

	// Lines join with '\' — the Delphi NPC dialog line separator, which the
	// client parser (and its <text/@label> tag handling) splits on.
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
	case "CHECKLEVEL":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		level := int(p.WAbil.Level)
		return compareOp(level, op, val)
	case "CHECKGOLD":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
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
		count := 1
		if len(parts) >= 3 {
			count, _ = strconv.Atoi(parts[2])
		}
		return p.countItem(itemName) >= count
	case "CHECKBAGGAGE":
		return len(p.ItemList) < MaxBagItems
	case "CHECKHP":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		return compareOp(int(p.WAbil.HP), op, val)
	case "CHECKMP":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		return compareOp(int(p.WAbil.MP), op, val)
	case "CHECKPKPOINT":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		return compareOp(p.PkPoint, op, val)
	case "CHECKDC":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		hiDC := int(p.WAbil.DC >> 16)
		return compareOp(hiDC, op, val)
	case "CHECKMC":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		hiMC := int(p.WAbil.MC >> 16)
		return compareOp(hiMC, op, val)
	case "CHECKSC":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		hiSC := int(p.WAbil.SC >> 16)
		return compareOp(hiSC, op, val)
	case "CHECKEXP":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		return compareOp(int(p.WAbil.Exp), op, val)
	case "CHECKGENDER":
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
	case "ISGUILDMASTER":
		return p.GuildName != "" && p.GuildRank == "master"
	case "CHECKGROUPCOUNT":
		if len(parts) < 3 {
			return true
		}
		op := parts[1]
		val, _ := strconv.Atoi(parts[2])
		count := 1
		if p.Engine != nil {
			p.Engine.mu.RLock()
			if party, ok := p.Engine.Parties[p.ID]; ok {
				count = len(party.Members)
			}
			p.Engine.mu.RUnlock()
		}
		return compareOp(count, op, val)
	default:
		return true
	}
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
		label := parts[1]
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
			p.envir.broadcastRefMsg(p.BaseObject, RM_DEATH, p.ID, p.CurrX, p.CurrY, 0)
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
			p.Engine.mu.Lock()
			for i := 0; i < count; i++ {
				id := p.Engine.nextMonsterID
				p.Engine.nextMonsterID++
				mon := NewMonsterObject(monName, id, 0, 0, 0, 100, 2000, 2000, 50)
				mon.CurrX = p.CurrX + rand.Intn(5) - 2
				mon.CurrY = p.CurrY + rand.Intn(5) - 2
				mon.MapName = p.MapName
				p.envir.AddObject(mon.CurrX, mon.CurrY, OS_MOVINGOBJECT, mon)
				p.Engine.Monsters = append(p.Engine.Monsters, mon)
			}
			p.Engine.mu.Unlock()
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
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		if idx >= 0 && idx < len(p.ScriptVars) {
			p.ScriptVars[idx] = val
		}
	case "INC":
		if len(parts) < 3 {
			return
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		if idx >= 0 && idx < len(p.ScriptVars) {
			p.ScriptVars[idx] += val
		}
	case "DEC":
		if len(parts) < 3 {
			return
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		if idx >= 0 && idx < len(p.ScriptVars) {
			p.ScriptVars[idx] -= val
		}
	case "MOV":
		if len(parts) < 3 {
			return
		}
		idx, _ := strconv.Atoi(parts[1])
		val, _ := strconv.Atoi(parts[2])
		if idx >= 0 && idx < len(p.ScriptVars) {
			p.ScriptVars[idx] = val
		}
	case "TIMERECALL":
	case "GROUPRECALL":
	case "GUILDRECALL":
	default:
		_ = parts
	}
}

func (s *NpcScript) replaceVars(text string, p *PlayObject) string {
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

	if npc.Script != "" {
		script, err := LoadNpcScript(npc.Script)
		if err == nil {
			script.Execute("main", p, npc, server)
			log.Logf(log.LevelInfo, "NPC", "%s executed script for %s", p.Name, npc.Name)
			return
		}
	}

	dialog := npc.Name + ": 欢迎光临！"
	resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(dialog))

	log.Logf(log.LevelInfo, "NPC", "%s clicked NPC %s", p.Name, npc.Name)
}
