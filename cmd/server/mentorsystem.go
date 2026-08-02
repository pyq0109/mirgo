package main

import (
	"strconv"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
)



func (p *PlayObject) HasMaster() bool {
	return p.MasterName != ""
}

func (p *PlayObject) IsMaster() bool {
	return len(p.ApprenticeNames) > 0
}

func (p *PlayObject) TakeMaster(server *netserver.TCPServer, masterName string) bool {
	if p.Engine == nil {
		return false
	}
	if p.MasterName != "" {
		p.sysMsg(server, "你已经有师傅了")
		return false
	}
	if masterName == p.Name {
		p.sysMsg(server, "不能拜自己为师")
		return false
	}
	master := p.Engine.GetPlayerByName(masterName)
	if master == nil {
		p.sysMsg(server, masterName+" 不在线")
		return false
	}
	if p.WAbil.Level >= master.WAbil.Level {
		p.sysMsg(server, "师傅的等级必须高于你")
		return false
	}
	if len(master.ApprenticeNames) >= p.Engine.Config.GetMaxApprentices() {
		p.sysMsg(server, masterName+" 的徒弟数量已满")
		return false
	}
	for _, name := range master.ApprenticeNames {
		if name == p.Name {
			p.sysMsg(server, "你已经是 "+masterName+" 的徒弟了")
			return false
		}
	}

	p.MasterName = masterName
	master.ApprenticeNames = append(master.ApprenticeNames, p.Name)

	p.sysMsg(server, "你拜 "+masterName+" 为师了")
	master.sysMsg(server, p.Name+" 拜你为师了")
	log.Logf(log.LevelInfo, "Mentor", "%s took %s as master", p.Name, masterName)
	return true
}

func (p *PlayObject) LeaveMaster(server *netserver.TCPServer) {
	if p.MasterName == "" {
		p.sysMsg(server, "你没有师傅")
		return
	}
	masterName := p.MasterName
	p.MasterName = ""
	if p.Engine != nil {
		if master := p.Engine.GetPlayerByName(masterName); master != nil {
			master.removeApprentice(p.Name)
			master.sysMsg(server, p.Name+" 与你解除了师徒关系")
		}
	}
	p.sysMsg(server, "你与 "+masterName+" 解除了师徒关系")
	log.Logf(log.LevelInfo, "Mentor", "%s left master %s", p.Name, masterName)
}

func (p *PlayObject) removeApprentice(name string) {
	for i, n := range p.ApprenticeNames {
		if n == name {
			p.ApprenticeNames = append(p.ApprenticeNames[:i], p.ApprenticeNames[i+1:]...)
			return
		}
	}
}

func (p *PlayObject) MasterRecall(server *netserver.TCPServer) {
	if !p.IsMaster() {
		p.sysMsg(server, "你没有徒弟")
		return
	}
	now := time.Now().UnixMilli()
	if now-p.MasterRecallTick < 60000 {
		p.sysMsg(server, "师徒召唤冷却中")
		return
	}
	if p.Engine == nil || p.envir == nil {
		return
	}
	p.MasterRecallTick = now

	count := 0
	for _, name := range p.ApprenticeNames {
		ap := p.Engine.GetPlayerByName(name)
		if ap == nil || ap.envir == nil || ap.MasterName != p.Name {
			continue
		}
		x, y := p.adjacentFreeTile(count)
		if x < 0 {
			continue
		}
		ap.EnterAnotherMap(server, p.envir, x, y)
		ap.sysMsg(server, "师傅把你召唤到了身边")
		count++
	}
	p.sysMsg(server, "召唤了 "+strconv.Itoa(count)+" 个徒弟")
}

func (p *PlayObject) adjacentFreeTile(index int) (int, int) {
	offsets := [8][2]int{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}
	start := index % len(offsets)
	for i := range offsets {
		o := offsets[(start+i)%len(offsets)]
		x, y := p.CurrX+o[0], p.CurrY+o[1]
		if p.envir.CanWalk(x, y) {
			return x, y
		}
	}
	return -1, -1
}
