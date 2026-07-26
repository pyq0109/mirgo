package main

import (
	"bufio"
	"os"
	"strings"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type NpcScript struct {
	Labels map[string][]string
}

func LoadNpcScript(path string) (*NpcScript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	script := &NpcScript{Labels: make(map[string][]string)}
	scanner := bufio.NewScanner(f)
	currentLabel := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "[@") {
			label := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			currentLabel = label
			script.Labels[currentLabel] = nil
		} else if currentLabel != "" {
			script.Labels[currentLabel] = append(script.Labels[currentLabel], line)
		}
	}
	return script, nil
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

	dialog := npc.Name + ": 欢迎光临！\n"
	if npc.Script != "" {
		dialog += "(脚本: " + npc.Script + ")"
	}

	resp := protocol.MakeDefaultMsg(protocol.SMMerchantSay, npc.ID, 0, 0, 0)
	server.Send(p.Session.ID, resp, protocol.EncodeString(dialog))

	log.Logf(log.LevelInfo, "NPC", "%s clicked NPC %s", p.Name, npc.Name)
}
