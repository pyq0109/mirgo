package main

import (
	"math/rand"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

type MonsterObject struct {
	*BaseObject
	Race        byte
	RaceImg     byte
	Appr        uint16
	MaxHP       int
	WalkSpeed   int64
	AttackSpeed int64
	Exp         int

	TargetID     int32
	HomeX, HomeY int
	WalkTick     int64
	SearchTick   int64
	HitTick      int64
	DeathTick    int64
	LootDropped  bool
}

func NewMonsterObject(name string, id int32, race, raceImg byte, appr uint16, hp int, walkSpeed, attackSpeed int64, exp int) *MonsterObject {
	base := NewBaseObject(name, id)
	return &MonsterObject{
		BaseObject:  base,
		Race:        race,
		RaceImg:     raceImg,
		Appr:        appr,
		MaxHP:       hp,
		WalkSpeed:   walkSpeed,
		AttackSpeed: attackSpeed,
		Exp:         exp,
	}
}

func (o *MonsterObject) Feature() int32 {
	return protocol.MakeMonsterFeature(o.RaceImg, 0, o.Appr)
}

func (o *MonsterObject) Run(server *netserver.TCPServer, now int64, userEngine *UserEngine) {
	if o.Ghost || o.Death {
		return
	}

	o.searchTarget(now, userEngine)
	o.validateTarget(userEngine)

	if o.TargetID != 0 {
		target := userEngine.GetPlayer(o.TargetID)
		dx := abs(target.CurrX - o.CurrX)
		dy := abs(target.CurrY - o.CurrY)

		if dx <= 1 && dy <= 1 {
			if now-o.HitTick > o.AttackSpeed {
				o.HitTick = now
				dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
				o.TurnTo(dir)
				o.SendRefMsg(RM_TURN, dir, o.CurrX, o.CurrY, o.Name)

				if !IsSafeZone(o.envir, target.CurrX, target.CurrY) {
					o.attackPlayer(server, target, dir, now)
				}
			}
		} else if now-o.WalkTick > o.WalkSpeed {
			o.WalkTick = now
			dir := dirToward(o.CurrX, o.CurrY, target.CurrX, target.CurrY)
			if o.WalkTo(dir) {
				o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
			}
		}
	} else {
		if now-o.WalkTick > o.WalkSpeed*3 {
			o.WalkTick = now
			if rand.Intn(20) == 0 {
				if rand.Intn(2) == 0 {
					dir := rand.Intn(8)
					if o.WalkTo(dir) {
						o.SendRefMsg(RM_WALK, dir, o.CurrX, o.CurrY, "")
					}
				} else {
					dir := rand.Intn(8)
					o.TurnTo(dir)
					o.SendRefMsg(RM_TURN, dir, o.CurrX, o.CurrY, o.Name)
				}
			}
		}
	}
}

func (o *MonsterObject) attackPlayer(server *netserver.TCPServer, target *PlayObject, dir int, now int64) {
	baseDmg := o.MaxHP / 10
	if baseDmg < 1 {
		baseDmg = 1
	}
	ac := int(target.WAbil.AC & 0xFFFF)
	damage := baseDmg - ac
	if damage < 1 {
		damage = 1
	}

	hp := int(target.WAbil.HP)
	hp -= damage
	if hp < 0 {
		hp = 0
	}
	target.WAbil.HP = uint16(hp)

	if o.envir != nil {
		o.envir.broadcastRefMsg(target.BaseObject, RM_STRUCK, o.ID, target.CurrX, target.CurrY, dir)
	}

	if hp <= 0 {
		target.Death = true
		target.deathTick = now
		if o.envir != nil {
			o.envir.broadcastRefMsg(target.BaseObject, RM_DEATH, target.ID, target.CurrX, target.CurrY, dir)
		}
		log.Logf(log.LevelInfo, "Combat", "%s killed %s", o.Name, target.Name)
	} else {
		target.sendHealthSpell(server)
	}

	log.Logf(log.LevelInfo, "Combat", "%s hit %s for %d damage (HP: %d/%d)",
		o.Name, target.Name, damage, hp, target.WAbil.MaxHP)
}

func (o *MonsterObject) searchTarget(now int64, userEngine *UserEngine) {
	hasTarget := o.TargetID != 0
	interval := int64(1000)
	if hasTarget {
		interval = 8000
	}
	if now-o.SearchTick <= interval {
		return
	}
	o.SearchTick = now

	if o.envir == nil {
		return
	}

	objs := o.envir.GetRangeObjects(o.CurrX, o.CurrY, 12)
	var best *PlayObject
	bestDist := 999999
	for _, obj := range objs {
		p, ok := obj.(*PlayObject)
		if !ok || p.Ghost || p.Death || p.Hidden {
			continue
		}
		d := abs(p.CurrX-o.CurrX) + abs(p.CurrY-o.CurrY)
		if d < bestDist {
			bestDist = d
			best = p
		}
	}
	if best != nil {
		o.TargetID = best.ID
	}
}

func (o *MonsterObject) validateTarget(userEngine *UserEngine) {
	if o.TargetID == 0 {
		return
	}
	target := userEngine.GetPlayer(o.TargetID)
	if target == nil || target.Ghost || target.Death {
		o.TargetID = 0
		return
	}
	if target.MapName != o.MapName {
		o.TargetID = 0
		return
	}
	dx := abs(target.CurrX - o.CurrX)
	dy := abs(target.CurrY - o.CurrY)
	if dx > 15 || dy > 15 {
		o.TargetID = 0
	}
}

func dirToward(fromX, fromY, toX, toY int) int {
	dx := toX - fromX
	dy := toY - fromY

	sx, sy := 0, 0
	if dx > 0 {
		sx = 1
	} else if dx < 0 {
		sx = -1
	}
	if dy > 0 {
		sy = 1
	} else if dy < 0 {
		sy = -1
	}

	switch {
	case sx == 0 && sy == -1:
		return 0
	case sx == 1 && sy == -1:
		return 1
	case sx == 1 && sy == 0:
		return 2
	case sx == 1 && sy == 1:
		return 3
	case sx == 0 && sy == 1:
		return 4
	case sx == -1 && sy == 1:
		return 5
	case sx == -1 && sy == 0:
		return 6
	case sx == -1 && sy == -1:
		return 7
	}
	return 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
