package main

import (
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

func ProcessDoors(envir *Environment, currentTick int64, cfg *ServerConfig, server *netserver.TCPServer) {
	for i := range envir.Doors {
		door := &envir.Doors[i]
		if door.State == 1 && currentTick-door.OpenTick > cfg.GetDoorCloseDelay() {
			door.State = 0
			log.Logf(log.LevelDebug, "Doors", "door %d auto-closed at (%d,%d)", door.ID, door.X, door.Y)
			if envir.rawMap != nil {
				info := envir.rawMap.InfoAt(door.X, door.Y)
				if info != nil {
					info.FrontDoorOffset &= 0x7F
				}
			}
			// 通知周围玩家关门，否则客户端以为门仍开着，
			// 走向门口会被服务端拒绝并回拉冻结（SMMoveFail）。
			broadcastDoorStatus(envir, server, protocol.SMCloseDoor, door.X, door.Y)
		}
	}
}

// broadcastDoorStatus 向门坐标 ±12 格内所有玩家广播门开关消息。
// Delphi: SendDoorStatus 广播 RM_DOOROPEN/RM_DOORCLOSE（UsrEngn.pas:2645-2682），
// 各玩家转发 SM_OPENDOOR_OK/SM_CLOSEDOOR（ObjBase.pas:5747-5764）。
func broadcastDoorStatus(envir *Environment, server *netserver.TCPServer, smIdent uint16, doorX, doorY int) {
	if envir == nil {
		return
	}
	resp := protocol.MakeDefaultMsg(smIdent, 0, uint16(doorX), uint16(doorY), 0)
	x0, x1 := doorX-12, doorX+12
	y0, y1 := doorY-12, doorY+12
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= envir.Width {
		x1 = envir.Width - 1
	}
	if y1 >= envir.Height {
		y1 = envir.Height - 1
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			cell := &envir.Cells[y*envir.Width+x]
			for i := range cell.ObjList {
				o := &cell.ObjList[i]
				if o.Type != OS_MOVINGOBJECT {
					continue
				}
				p, ok := o.Obj.(*PlayObject)
				if !ok || p.Ghost || p.Session == nil {
					continue
				}
				server.Send(p.Session.ID, resp, "")
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
					// Delphi 开门同样广播给 ±12 格内所有玩家（UsrEngn.pas:2617-2632）
					broadcastDoorStatus(p.envir, server, protocol.SMOpenDoorOK, door.X, door.Y)
					log.Logf(log.LevelInfo, "Doors", "%s opened door %d at (%d,%d)", p.Name, door.ID, door.X, door.Y)
					return
				}
			}
		}
	}
	resp := protocol.MakeDefaultMsg(protocol.SMOpenDoorLock, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
