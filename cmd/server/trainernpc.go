package main

// 训练师 NPC：玩家攻击训练师时不造成伤害（SuperMan 模式）
// 训练师可用于玩家练习技能

func isTrainerNpc(npc *NpcObject) bool {
	return npc.Race == 2 // Delphi: race=2 为训练师/国王类型
}
