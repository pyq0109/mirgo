package main

// 布局快照 + 命中断言 — headless (无 GLFW/GL/资源依赖):
//
//  1. 快照: 各场景建树后 dump DebugTree 与 testdata/*.snap 比对,
//     防结构与坐标回归。故意调整布局后跑 -update 重生成基线
//     (在提交说明里解释原因)。
//  2. 命中断言: 按钮中心必须命中自己、间隙不得命中、网格裁剪区外
//     不得命中 — 机器判定 "点得到/点得准", 不靠人眼。
//
// 注意: nil 资源走回退尺寸, 快照记录的是回退布局; 真实资源下的
// 尺寸/偏移问题由 ui audit (size-override/img-offset) 与 Delphi
// 真值对照 (uidelphi_test.go) 覆盖, 三者互补。

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pyq0109/mirgo/internal/engine"
)

var updateSnaps = flag.Bool("update", false, "重新生成 UI 布局快照 (go test ./cmd/client/ -update)")

func snapCheck(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateSnaps {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("快照已更新: %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("快照 %s 缺失: 请先运行 go test ./cmd/client/ -update (%v)", path, err)
	}
	if string(want) == got {
		return
	}
	wl := strings.Split(string(want), "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) > n {
		n = len(gl)
	}
	var sb strings.Builder
	shown := 0
	for i := 0; i < n && shown < 20; i++ {
		w, g := "", ""
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			fmt.Fprintf(&sb, "  行%d:\n    基线: %s\n    实际: %s\n", i+1, w, g)
			shown++
		}
	}
	t.Errorf("布局快照 %s 变化 (故意调整请用 -update 重生成并说明原因), want %d 行 got %d 行:\n%s",
		name, len(wl), len(gl), sb.String())
}

// newTestText 构造 headless TextRenderer (gl=nil 只存指针不解引用;
// 需要系统字体, 缺失时跳过依赖它的场景快照)。
func newTestText(t *testing.T) *engine.TextRenderer {
	t.Helper()
	tr, err := engine.NewTextRenderer(nil, "", 9)
	if err != nil {
		t.Skipf("无法 headless 构造 TextRenderer (缺系统字体?): %v", err)
	}
	return tr
}

func TestPlaySceneLayoutSnapshot(t *testing.T) {
	s := NewPlayScene(nil, &engine.ResourceManager{}, "", nil)
	snapCheck(t, "play_layout.snap", s.ui.DebugTree(99))
}

func TestLoginSceneLayoutSnapshot(t *testing.T) {
	tr := newTestText(t)
	s := NewLoginScene(nil, &engine.ResourceManager{}, tr)
	s.ui = NewUIManager(nil, s.resources, s.textSmall)
	s.buildLoginUI()
	snapCheck(t, "login_layout.snap", s.ui.DebugTree(99))
}

func TestSelectChrSceneLayoutSnapshot(t *testing.T) {
	tr := newTestText(t)
	s := NewSelectChrScene(nil, &engine.ResourceManager{}, tr)
	s.ui = NewUIManager(nil, s.resources, s.text)
	s.buildUI()
	snapCheck(t, "selectchr_layout.snap", s.ui.DebugTree(99))
}

// --- 命中断言 ---

func collectVisibleButtons(c *UIControl, out *[]*UIControl) {
	if c.Visible && c.Kind == KindButton && !c.Background {
		*out = append(*out, c)
	}
	for _, ch := range c.Children {
		collectVisibleButtons(ch, out)
	}
}

// overlapsSibling 判断按钮是否与同级可见按钮矩形相交。
// 回退尺寸下部分按钮人为重叠 (真实图素更窄), 这类重叠由
// ui audit 的 sibling-overlap 规则负责, 命中断言跳过。
func overlapsSibling(b *UIControl) bool {
	if b.Parent == nil {
		return false
	}
	w, h := b.effectiveSize()
	for _, s := range b.Parent.Children {
		if s == b || s.Kind != KindButton || !s.Visible || s.Background {
			continue
		}
		sw, sh := s.effectiveSize()
		if b.Left < s.Left+sw && s.Left < b.Left+w && b.Top < s.Top+sh && s.Top < b.Top+h {
			return true
		}
	}
	return false
}

// TestPlaySceneHitAssertions 逐面板验证命中: 每次只显示一个顶层面板
// (与实际游玩时一次只开一个面板一致), 其内每个无重叠按钮的中心点
// 必须命中自己。
func TestPlaySceneHitAssertions(t *testing.T) {
	s := NewPlayScene(nil, &engine.ResourceManager{}, "", nil)
	ui := s.ui

	tops := append([]*UIControl{}, ui.Root.Children...)
	if len(tops) == 0 {
		t.Fatal("Root 下无顶层面板")
	}
	checked := 0
	for _, p := range tops {
		for _, q := range tops {
			q.Visible = q == p
		}
		var btns []*UIControl
		collectVisibleButtons(p, &btns)
		for _, b := range btns {
			if overlapsSibling(b) {
				continue
			}
			w, h := b.effectiveSize()
			if w <= 0 || h <= 0 {
				continue
			}
			cx, cy := b.AbsX()+w/2, b.AbsY()+h/2
			if got := ui.HoveredControl(cx, cy); got != b {
				t.Errorf("面板 %s: 按钮 %s 中心 (%d,%d) 命中 %s, 期望命中自己 (矩形 [%d,%d %dx%d])",
					p.Name, b.DebugPath(), cx, cy, debugCtlName(got), b.AbsX(), b.AbsY(), w, h)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("没有执行任何按钮命中断言")
	}
	t.Logf("按钮中心命中断言: %d 个", checked)

	// showOnly 只保留指定名字的顶层面板可见 (还原真实游玩时
	// 一次只开一个面板的状态), 供后续点位断言使用。
	showOnly := func(name string) {
		for _, q := range tops {
			q.Visible = q.Name == name
		}
	}

	// 腰带相邻格间隙中点不得命中任何控件 (防格子命中区外溢)。
	showOnly("DBottom")
	var belts []*UIControl
	var walk func(c *UIControl)
	walk = func(c *UIControl) {
		if c.Name == "DBelt" && c.Kind == KindButton {
			belts = append(belts, c)
		}
		for _, ch := range c.Children {
			walk(ch)
		}
	}
	walk(ui.Root)
	sort.Slice(belts, func(i, j int) bool { return belts[i].AbsX() < belts[j].AbsX() })
	if len(belts) != 6 {
		t.Fatalf("腰带格数量 = %d, want 6", len(belts))
	}
	for i := 0; i+1 < len(belts); i++ {
		a, b := belts[i], belts[i+1]
		aw, ah := a.effectiveSize()
		gap := b.AbsX() - (a.AbsX() + aw)
		if gap < 2 {
			continue
		}
		px := a.AbsX() + aw + gap/2
		py := a.AbsY() + ah/2
		if got := ui.HoveredControl(px, py); got != nil {
			t.Errorf("腰带格 %d/%d 间隙点 (%d,%d) 命中 %s, 期望无命中", i, i+1, px, py, got.Name)
		}
	}

	// 背包网格裁剪: 点击区 286×162 (第 6 行几乎不可点, FState:1171-1174)。
	showOnly("DItemBag")
	var grid *UIControl
	var findGrid func(c *UIControl)
	findGrid = func(c *UIControl) {
		if c.Name == "DItemGrid" && c.Kind == KindGrid {
			grid = c
			return
		}
		for _, ch := range c.Children {
			findGrid(ch)
		}
	}
	findGrid(ui.Root)
	if grid == nil {
		t.Fatal("未找到 DItemGrid")
	}
	gw, gh := grid.effectiveSize()
	if gw != 286 || gh != 162 {
		t.Fatalf("DItemGrid 点击区 = %dx%d, want 286x162", gw, gh)
	}
	inX, inY := grid.AbsX()+2*grid.ColWidth+5, grid.AbsY()+1*grid.RowHeight+5
	if got := ui.HoveredControl(inX, inY); got != grid {
		t.Errorf("网格内点 (%d,%d) 命中 %s, 期望 DItemGrid", inX, inY, debugCtlName(got))
	}
	outX, outY := grid.AbsX()+5, grid.AbsY()+gh+5
	if got := ui.HoveredControl(outX, outY); got == grid {
		t.Errorf("网格裁剪区外点 (%d,%d) 仍命中 DItemGrid (第6行应不可点)", outX, outY)
	}

	// 左上角空白区不得命中任何控件 (仅底栏的真实初始状态)。
	showOnly("DBottom")
	if got := ui.HoveredControl(10, 10); got != nil {
		t.Errorf("空白点 (10,10) 命中 %s, 期望无命中", got.Name)
	}
}
