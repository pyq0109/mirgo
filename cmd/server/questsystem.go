package main

// 任务系统框架 — 3072 位标志（3×1024），NPC 脚本驱动。
// Delphi 参考：ObjBase.pas lines 10015-10127, ObjNpc.pas lines 7030-7055, 7762-7798

// questGetBit 获取位标志（1-indexed，Delphi 约定）。
func questGetBit(arr *[128]byte, idx int) bool {
	if idx < 1 || idx > 1024 {
		return false
	}
	byteIdx := (idx - 1) / 8
	bitIdx := uint((idx - 1) % 8)
	return arr[byteIdx]&(1<<bitIdx) != 0
}

// questSetBit 设置位标志。
func questSetBit(arr *[128]byte, idx int) {
	if idx < 1 || idx > 1024 {
		return
	}
	byteIdx := (idx - 1) / 8
	bitIdx := uint((idx - 1) % 8)
	arr[byteIdx] |= 1 << bitIdx
}

// questClearBit 清除位标志。
func questClearBit(arr *[128]byte, idx int) {
	if idx < 1 || idx > 1024 {
		return
	}
	byteIdx := (idx - 1) / 8
	bitIdx := uint((idx - 1) % 8)
	arr[byteIdx] &^= 1 << bitIdx
}

// questClearRange 清除从 start 开始的 count 个位标志。
func questClearRange(arr *[128]byte, start, count int) {
	for i := start; i < start+count; i++ {
		questClearBit(arr, i)
	}
}
