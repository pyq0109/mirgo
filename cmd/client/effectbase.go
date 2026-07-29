package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

// effectBase 魔法身特效基址表（对应 Delphi magiceff.pas:65-115 EffectBase[0..48]）。
// 下标 = EffectNumber-1。
var effectBase = [49]int{
	0, 200, 400, 600, 0, 900, 920, 940, 20, 940,
	940, 940, 0, 1380, 1500, 1520, 940, 1560, 1590, 1620,
	1650, 1680, 0, 0, 0, 3960, 1790, 0, 3880, 3920,
	3840, 0, 40, 130, 160, 190, 0, 210, 400, 600,
	1500, 650, 710, 740, 910, 940, 990, 1040, 1110,
}

// hitEffectBase 击打特效基址表（对应 Delphi magiceff.pas:127-136 HitEffectBase[0..7]）。
var hitEffectBase = [8]int{800, 1410, 1700, 3480, 3390, 40, 220, 740}

// getEffectBase 返回魔法特效所在的 WIL 文件与基址帧（对应 Delphi magiceff.pas:319-383 GetEffectBase）。
// mag = EffectNumber-1（身特效 mtype=0）或 HitEffectNumber-1（击打特效 mtype=1）。
func getEffectBase(resources *engine.ResourceManager, mag, mtype int) (*wil.File, int) {
	if mtype == 1 {
		idx := 0
		if mag >= 0 && mag < len(hitEffectBase) {
			idx = hitEffectBase[mag]
		}
		if mag >= 5 {
			return resources.Magic2, idx
		}
		return resources.Magic, idx
	}

	switch {
	case mag == 8 || mag == 27 ||
		(mag >= 33 && mag <= 35) || (mag >= 37 && mag <= 39) || (mag >= 41 && mag <= 48):
		return resources.Magic2, effectBaseIdx(mag)
	case mag == 31:
		return monEffectFile(resources, 21), effectBaseIdx(mag)
	case mag == 36:
		return monEffectFile(resources, 22), effectBaseIdx(mag)
	case mag >= 80 && mag <= 82:
		return resources.Dragon, dragonEffectIdx(mag)
	case mag == 89:
		return resources.Dragon, 350
	default:
		return resources.Magic, effectBaseIdx(mag)
	}
}

func effectBaseIdx(mag int) int {
	if mag >= 0 && mag < len(effectBase) {
		return effectBase[mag]
	}
	return 0
}

// monEffectFile 取 Mon<nrace>.wil（Delphi WMon21Img/WMon22Img），缺失时回退 Magic。
func monEffectFile(resources *engine.ResourceManager, nrace int) *wil.File {
	if nrace >= 0 && nrace < len(resources.Mon) && resources.Mon[nrace] != nil {
		return resources.Mon[nrace]
	}
	return resources.Magic
}

// dragonEffectIdx 龙地图特效基址（Delphi magiceff.pas:343-361 依玩家坐标分支，此处取默认分支）。
func dragonEffectIdx(mag int) int {
	switch mag {
	case 80:
		return 140
	case 81:
		return 160
	case 82:
		return 180
	}
	return 0
}
