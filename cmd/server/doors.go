package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	doorCloseDelay = 5000
)

func ProcessDoors(envir *Environment, currentTick int64) {
	for i := range envir.Doors {
		door := &envir.Doors[i]
		if door.State == 1 && currentTick-door.OpenTick > doorCloseDelay {
			door.State = 0
			log.Logf(log.LevelDebug, "Doors", "Door %d auto-closed at (%d,%d)", door.ID, door.X, door.Y)
			if envir.rawMap != nil {
				info := envir.rawMap.InfoAt(door.X, door.Y)
				if info != nil {
					info.FrontDoorOffset &= 0x7F
				}
			}
		}
	}
}

func (p *PlayObject) HandleOpenDoor(msg SendMessage, server *netserver.TCPServer) {
	if p.envir == nil {
		return
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			x, y := p.CurrX+dx, p.CurrY+dy
			for i := range p.envir.Doors {
				door := &p.envir.Doors[i]
				if door.X == x && door.Y == y && door.State == 0 {
					door.State = 1
					door.OpenTick = time.Now().UnixMilli()
					if p.envir.rawMap != nil {
						info := p.envir.rawMap.InfoAt(door.X, door.Y)
						if info != nil {
							info.FrontDoorOffset |= 0x80
						}
					}
					resp := protocol.MakeDefaultMsg(protocol.SMOpenDoorOK, int32(door.ID), uint16(door.X), uint16(door.Y), 0)
					server.Send(p.Session.ID, resp, "")
					log.Logf(log.LevelInfo, "Doors", "%s opened door %d at (%d,%d)", p.Name, door.ID, door.X, door.Y)
					return
				}
			}
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMOpenDoorLock, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
