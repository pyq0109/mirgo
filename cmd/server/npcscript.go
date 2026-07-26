package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"

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

	if conditionsMet {
		s.execActions(section.Actions, p, npc, server)
		if len(section.SayText) > 0 {
			text := strings.Join(section.SayText, "\n")
			resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
			server.Send(p.Session.ID, resp, protocol.EncodeString(text))
		}
	} else {
		s.execActions(section.ElseAct, p, npc, server)
		if len(section.ElseSay) > 0 {
			text := strings.Join(section.ElseSay, "\n")
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
	case "SENDMSG", "MESSAGEBOX":
		if len(parts) < 2 {
			return
		}
		text := strings.Join(parts[1:], " ")
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
		}
	case "ADDGOLD":
		if len(parts) < 2 {
			return
		}
		gold, _ := strconv.Atoi(parts[1])
		p.Gold += gold
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
	case "CLOSE":
		resp := protocol.MakeDefaultMsg(protocol.SMMerchantDlgClose, 0, 0, 0, 0)
		server.Send(p.Session.ID, resp, "")
	}
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
