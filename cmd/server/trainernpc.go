package main

import (
	"fmt"
	"time"

	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

// raceTrainer Delphi TRAINER = 55（M2Share.pas:152）。TTrainer 虽是
// TNormNpc 子类（ObjNpc.pas:377），但由怪物工厂创建（UsrEngn.pas:1865-1868），
// Go 中对应 Race=55 的 MonsterObject。NPC 列表的 race 语义不同
//（0=店主/1=国王/2=城堡官员，Npcs.txt 表头），不可混用。
const raceTrainer = 55

// isTrainer 判断是否为训练师（无敌沙袋+伤害统计）。
func isTrainer(mon *MonsterObject) bool {
	return mon != nil && mon.Race == raceTrainer
}

// addTrainingDamage 累计受到的伤害（不扣血）。
// Delphi TTrainer.Operate（ObjNpc.pas:2642-2661）：累加伤害值与命中次数。
func (o *MonsterObject) addTrainingDamage(server *netserver.TCPServer, attackerID int32, damage int) {
	now := time.Now().UnixMilli()
	// Delphi TTrainer.Run（ObjNpc.pas:2663-2676）：3 秒无新命中先汇报旧统计
	if o.trainLastTick > 0 && now-o.trainLastTick > 3000 {
		o.reportTraining(server)
	}
	o.trainDmgTotal += damage
	o.trainHitCount++
	o.trainLastTick = now
	o.trainLastHiterID = attackerID
}

// checkTrainingReport 由 Run 周期调用：3 秒无新命中则汇报并清零。
func (o *MonsterObject) checkTrainingReport(server *netserver.TCPServer, now int64) {
	if o.trainLastTick > 0 && now-o.trainLastTick > 3000 {
		o.reportTraining(server)
	}
}

func (o *MonsterObject) reportTraining(server *netserver.TCPServer) {
	if o.trainHitCount == 0 {
		return
	}
	avg := o.trainDmgTotal / o.trainHitCount
	text := fmt.Sprintf("破坏力 %d / 平均值 %d", o.trainDmgTotal, avg)
	if o.engine != nil {
		if hitter := o.engine.GetPlayer(o.trainLastHiterID); hitter != nil {
			msg := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
			server.Send(hitter.Session.ID, msg, protocol.EncodeString(text))
		}
	}
	o.trainDmgTotal, o.trainHitCount, o.trainLastTick, o.trainLastHiterID = 0, 0, 0, 0
}
