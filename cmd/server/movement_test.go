package main

import "testing"

// 移动/占格回归测试：修复前 WalkTo 以裸 *BaseObject 写入网格，
// 导致移动过的对象不再阻挡、旧格残留幽灵。用例对齐 Delphi
// MoveToMovingObject 语义（Envir.pas:287）。

func newTestEnv(w, h int) *Environment {
	return &Environment{
		Name:     "test",
		Width:    w,
		Height:   h,
		Cells:    make([]MapCellInfo, w*h),
		objIndex: make(map[int32]interface{}),
	}
}

func placeMonster(env *Environment, id int32, x, y int) *MonsterObject {
	mon := NewMonsterObject("test", id, 51, 11, 160, 100, 1400, 2000, 0)
	mon.MapName = env.Name
	mon.CurrX, mon.CurrY = x, y
	mon.envir = env
	env.AddObject(x, y, OS_MOVINGOBJECT, mon)
	return mon
}

func cellMovingObjs(env *Environment, x, y int) []OSObject {
	var out []OSObject
	for _, o := range env.Cells[y*env.Width+x].ObjList {
		if o.Type == OS_MOVINGOBJECT {
			out = append(out, o)
		}
	}
	return out
}

// 核心回归：怪物走一步后，旧格清空、新格以具体类型存格且仍然阻挡。
func TestWalkToKeepsGridConsistent(t *testing.T) {
	env := newTestEnv(5, 5)
	mon := placeMonster(env, 1, 1, 1)

	if !mon.WalkTo(2) { // 右
		t.Fatal("WalkTo(right) failed on empty map")
	}
	if mon.CurrX != 2 || mon.CurrY != 1 {
		t.Fatalf("pos = (%d,%d), want (2,1)", mon.CurrX, mon.CurrY)
	}
	if objs := cellMovingObjs(env, 1, 1); len(objs) != 0 {
		t.Errorf("old cell still has %d moving objects (ghost entry)", len(objs))
	}
	objs := cellMovingObjs(env, 2, 1)
	if len(objs) != 1 {
		t.Fatalf("new cell has %d moving objects, want 1", len(objs))
	}
	if _, ok := objs[0].Obj.(*MonsterObject); !ok {
		t.Errorf("new cell stores %T, want *MonsterObject", objs[0].Obj)
	}
	if !env.hasBlockingObject(2, 1, nil) {
		t.Error("moved monster no longer blocks its own cell")
	}
	// 再走一步，连续移动不应残留
	if !mon.WalkTo(2) {
		t.Fatal("second WalkTo failed")
	}
	if objs := cellMovingObjs(env, 2, 1); len(objs) != 0 {
		t.Errorf("intermediate cell still has %d moving objects", len(objs))
	}
	if got := env.getObjectByID(1); got == nil {
		t.Error("getObjectByID lost the monster after it moved")
	}
}

// 两只怪相邻：互相不能走进对方格子（修复前移动过的怪不阻挡）。
func TestMonstersBlockEachOther(t *testing.T) {
	env := newTestEnv(5, 5)
	a := placeMonster(env, 1, 1, 1)
	placeMonster(env, 2, 2, 1)

	// a 先移动一次（修复前移动后即失去阻挡资格，此处同时验证可正常来回）
	if !a.WalkTo(4) { // 下 → (1,2)
		t.Fatal("setup WalkTo down failed")
	}
	if !a.WalkTo(0) { // 上 → (1,1)
		t.Fatal("WalkTo up failed")
	}
	if a.WalkTo(2) { // 右 → (2,1) 被 b 占
		t.Error("monster walked into another monster's cell")
	}
}

// Delphi 语义：存活玩家阻挡怪物；死亡/幽灵不阻挡。
func TestPlayerOccupancyBlocksMonster(t *testing.T) {
	env := newTestEnv(5, 5)
	mon := placeMonster(env, 1, 1, 1)
	p := NewPlayObject(nil, "P", 100)
	p.envir = env
	p.CurrX, p.CurrY = 2, 1
	env.AddObject(2, 1, OS_MOVINGOBJECT, p)

	if mon.WalkTo(2) {
		t.Error("monster walked into live player's cell")
	}
	p.Death = true
	if !mon.WalkTo(2) {
		t.Error("monster should walk onto a corpse")
	}
}

// Delphi 语义：隐身玩家仍然阻挡移动。
func TestHiddenPlayerStillBlocks(t *testing.T) {
	env := newTestEnv(5, 5)
	mon := placeMonster(env, 1, 1, 1)
	p := NewPlayObject(nil, "P", 100)
	p.envir = env
	p.CurrX, p.CurrY = 2, 1
	p.Hidden = true
	env.AddObject(2, 1, OS_MOVINGOBJECT, p)

	if mon.WalkTo(2) {
		t.Error("hidden player should still block movement")
	}
}

// Delphi 语义：石化（m_boStoneMode）不豁免阻挡，雕像占格。
func TestStoneModeBlocks(t *testing.T) {
	env := newTestEnv(5, 5)
	statue := placeMonster(env, 1, 2, 1)
	statue.StoneMode = true
	mover := placeMonster(env, 2, 1, 1)

	if mover.WalkTo(2) {
		t.Error("stone statue should block movement")
	}
}

// Delphi 语义：潜地（m_boFixedHideMode）不阻挡。
func TestFixedHideDoesNotBlock(t *testing.T) {
	env := newTestEnv(5, 5)
	buried := placeMonster(env, 1, 2, 1)
	buried.FixedHide = true
	mover := placeMonster(env, 2, 1, 1)

	if !mover.WalkTo(2) {
		t.Error("buried (FixedHide) monster should not block movement")
	}
}

// 地形墙不可进入。
func TestWallBlocks(t *testing.T) {
	env := newTestEnv(5, 5)
	env.Cells[1*env.Width+2].Flag = 1
	mon := placeMonster(env, 1, 1, 1)
	if mon.WalkTo(2) {
		t.Error("monster walked into a wall cell")
	}
}
