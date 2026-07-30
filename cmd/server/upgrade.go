package main

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)

const (
	upgradeWeaponPrice    = 10000
	upgradeGetBackTime    = 3600000 // 1小时（ms）
	upgradeBlackIronName  = "黑铁矿"
	upgradeMaxPoints      = 7 // 最大升级总点数
)

func (p *PlayObject) HandleUpgradeWeapon(npc *NpcObject, server *netserver.TCPServer) {
	if npc == nil || !npc.CanUpgrade {
		return
	}

	// 距离验证
	if !p.isNearNpc(npc) {
		return
	}

	// 检查已装备武器
	weapon := p.UseItems[protocol.UWeapon]
	if weapon == nil {
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	// 检查金币
	if p.Gold < upgradeWeaponPrice {
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	// 检查黑铁矿
	ironIdx := -1
	if p.ItemDB != nil {
		def := p.ItemDB.GetByName(upgradeBlackIronName)
		if def != nil {
			ironIdx = p.findBagItemByWIndex(uint16(def.Idx))
		}
	}
	if ironIdx < 0 {
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	// 扣金币
	p.Gold -= upgradeWeaponPrice
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")

	// 移除武器
	p.UseItems[protocol.UWeapon] = nil
	p.RecalcAbilitys()
	p.SendUseItemsFull(server)

	// 分析升级材料
	info := &UpgradeInfo{
		PlayerName: p.Name,
		Item:       weapon,
		Tick:       time.Now().UnixMilli(),
	}

	// 黑铁矿贡献耐久
	ironItem := p.ItemList[ironIdx]
	info.BtDura = byte(ironItem.Dura / 1000)
	p.ItemList = append(p.ItemList[:ironIdx], p.ItemList[ironIdx+1:]...)
	p.SendBagItemsFull(server)

	// 分析背包中的饰品（StdMode 19-26）
	var dcScore, mcScore, scScore int
	if p.ItemDB != nil {
		var remaining []*protocol.UserItem
		for _, item := range p.ItemList {
			def := p.ItemDB.GetByIdx(int(item.WIndex))
			if def != nil && def.StdMode >= 19 && def.StdMode <= 26 {
				dcScore += int(def.DC + def.DCMax)
				mcScore += int(def.MC + def.MCMax)
				scScore += int(def.SC + def.SCMax)
			} else {
				remaining = append(remaining, item)
			}
		}
		p.ItemList = remaining
		p.SendBagItemsFull(server)
	}

	info.BtDc = byte(dcScore/5 + dcScore/3)
	info.BtMc = byte(mcScore/5 + mcScore/3)
	info.BtSc = byte(scScore/5 + scScore/3)

	// 存入升级列表
	npc.mu.Lock()
	npc.UpgradeWeaponList = append(npc.UpgradeWeaponList, info)
	npc.mu.Unlock()

	// 持久化到数据库
	if p.Engine != nil {
		itemData, err := json.Marshal(savedUserItem{
			MakeIndex: weapon.MakeIndex,
			WIndex:    weapon.WIndex,
			Dura:      weapon.Dura,
			DuraMax:   weapon.DuraMax,
			BtValue:   weapon.BtValue,
		})
		if err == nil {
			recordID, err := p.Engine.db.SaveNpcUpgrade(npc.ID, p.Name, itemData, info.BtDc, info.BtSc, info.BtMc, info.BtDura, info.Tick)
			if err != nil {
				log.Logf(log.LevelError, "Server", "failed to save upgrade weapon for %s: %v", p.Name, err)
			} else {
				info.DBRecordID = recordID
			}
		}
	}

	p.execScriptLabel(npc, "upgrade_ok", server)
}

func (p *PlayObject) HandleGetBackupWeapon(npc *NpcObject, server *netserver.TCPServer) {
	if npc == nil || !npc.CanGetBackup {
		return
	}

	// 距离验证
	if !p.isNearNpc(npc) {
		return
	}

	if len(p.ItemList) >= MaxBagItems {
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	npc.mu.Lock()
	// 查找属于该玩家的升级武器
	idx := -1
	for i, info := range npc.UpgradeWeaponList {
		if info.PlayerName == p.Name {
			idx = i
			break
		}
	}

	if idx < 0 {
		npc.mu.Unlock()
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	info := npc.UpgradeWeaponList[idx]
	now := time.Now().UnixMilli()

	// 等待时间检查（GM 可跳过）
	if p.Permission < 10 && now-info.Tick < upgradeGetBackTime {
		npc.mu.Unlock()
		p.execScriptLabel(npc, "upgrade_fail", server)
		return
	}

	// 从列表移除
	npc.UpgradeWeaponList = append(npc.UpgradeWeaponList[:idx], npc.UpgradeWeaponList[idx+1:]...)
	npc.mu.Unlock()

	// 从数据库删除
	if p.Engine != nil && info.DBRecordID > 0 {
		if err := p.Engine.db.DeleteNpcUpgrade(info.DBRecordID); err != nil {
			log.Logf(log.LevelError, "Server", "failed to delete upgrade record %d: %v", info.DBRecordID, err)
		}
	}

	// 计算升级结果
	weapon := info.Item
	score := int(info.BtDc) + int(info.BtMc) + int(info.BtSc)
	duraBonus := 0
	if info.BtDura >= 18 {
		duraBonus = 5
	} else if info.BtDura >= 9 {
		duraBonus = 2
	}

	// 成功概率: min(85, score*7 + 10 + duraBonus + bodyLuck)
	bodyLuck := 0
	chance := score*7 + 10 + duraBonus + bodyLuck
	if chance > 85 {
		chance = 85
	}

	success := rand.Intn(100) < chance
	if success {
		// 决定升级哪种属性
		maxVal := int(info.BtDc)
		attrIdx := byte(10) // DC
		if int(info.BtMc) > maxVal {
			maxVal = int(info.BtMc)
			attrIdx = 20 // MC
		}
		if int(info.BtSc) > maxVal {
			maxVal = int(info.BtSc)
			attrIdx = 30 // SC
		}

		// 升级点数 (1-3)
		points := byte(1)
		if maxVal > 10 {
			points = 2
		}
		if maxVal > 20 {
			points = 3
		}
		weapon.BtValue[10] = attrIdx + points - 1

		// 耐久升级
		if duraBonus > 0 {
			weapon.DuraMax += uint16(duraBonus * 1000)
			weapon.Dura = weapon.DuraMax
		}
	} else {
		// 失败：设置破碎编码（攻击时触发 CheckWeaponUpgradeStatus）
		weapon.BtValue[10] = 1
		// 诅咒概率 30%
		if rand.Intn(100) < 30 && weapon.BtValue[12] < 7 {
			weapon.BtValue[12]++
		}
	}

	// 分配新 MakeIndex 并返回武器
	if p.Engine != nil {
		p.Engine.mu.Lock()
		weapon.MakeIndex = int32(p.Engine.nextItemID)
		p.Engine.nextItemID++
		p.Engine.mu.Unlock()
	}
	p.ItemList = append(p.ItemList, weapon)
	p.SendBagItemsFull(server)
	p.RecalcAbilitys()

	if success {
		p.execScriptLabel(npc, "upgrade_ok", server)
	} else {
		p.execScriptLabel(npc, "upgrade_fail", server)
	}
}

func (p *PlayObject) execScriptLabel(npc *NpcObject, label string, server *netserver.TCPServer) {
	script := npc.GetScript()
	if script != nil {
		if _, exists := script.Labels[label]; exists {
			script.Execute(label, p, npc, server)
			return
		}
	}
}

func (p *PlayObject) findBagItemByWIndex(wIdx uint16) int {
	for i, item := range p.ItemList {
		if item.WIndex == wIdx {
			return i
		}
	}
	return -1
}

// CheckWeaponUpgradeStatus 攻击时处理武器升级结果（Delphi ObjBase.pas:18704-18772）。
// BtValue[10] 编码：1=破碎, 10-13=DC+1..4, 20-23=MC+1..4, 30-33=SC+1..4
func (p *PlayObject) CheckWeaponUpgradeStatus(server *netserver.TCPServer) {
	weapon := p.UseItems[protocol.UWeapon]
	if weapon == nil {
		return
	}
	bt := weapon.BtValue[10]
	if bt == 0 {
		return
	}
	weapon.BtValue[10] = 0 // 消耗结果码

	if bt == 1 {
		// 武器破碎
		p.UseItems[protocol.UWeapon] = nil
		p.RecalcAbilitys()
		p.SendUseItemsFull(server)
		p.SendRefMsg(RM_BREAKWEAPON, 0, p.CurrX, p.CurrY, "")
		p.sysMsg(server, "你的武器破碎了！")
		return
	}

	// 解析属性和点数
	var attrIdx int
	var points byte
	switch {
	case bt >= 30 && bt <= 33:
		attrIdx = 2 // SC
		points = bt - 29
	case bt >= 20 && bt <= 23:
		attrIdx = 1 // MC
		points = bt - 19
	case bt >= 10 && bt <= 13:
		attrIdx = 0 // DC
		points = bt - 9
	default:
		return
	}

	// 检查总升级点数上限
	totalUpgrades := int(weapon.BtValue[0]) + int(weapon.BtValue[1]) + int(weapon.BtValue[2])
	if totalUpgrades+int(points) > upgradeMaxPoints {
		// 超限：武器破碎
		p.UseItems[protocol.UWeapon] = nil
		p.RecalcAbilitys()
		p.SendUseItemsFull(server)
		p.SendRefMsg(RM_BREAKWEAPON, 0, p.CurrX, p.CurrY, "")
		p.sysMsg(server, "升级过度，武器破碎了！")
		return
	}

	// 应用升级加成
	weapon.BtValue[attrIdx] += points
	weapon.BtValue[11]++ // 升级等级计数器
	p.RecalcAbilitys()
	p.SendUseItemsFull(server)
	p.sysMsg(server, "武器升级成功！")
}
