package main

// 框架分发语义测试（无需 GL——不覆盖 Paint）。
// 验证 DWinCtl 移植：按钮抬起且在内部才触发点击、网格按下==抬起选中、
// 窗口拖拽带边界限制、模态窗口拦截、背景点击+焦点释放、键盘焦点路由。

import "testing"

func newTestManager() *UIManager {
	return NewUIManager(nil, nil, nil)
}

func TestButtonClickOnUpInside(t *testing.T) {
	m := newTestManager()
	btn := NewUIControl("btn", KindButton)
	btn.Left, btn.Top, btn.Width, btn.Height = 100, 100, 40, 30
	clicks := 0
	btn.OnClick = func(c *UIControl, x, y int) { clicks++ }
	btn.Visible = true
	m.Root.AddChild(btn)

	// 内部按下，内部抬起 → 一次点击。
	if !m.RouteMouseDown(110, 110, 0) {
		t.Fatal("mouse down inside button should be consumed")
	}
	if !btn.Downed {
		t.Fatal("button should be Downed after press")
	}
	if m.Capture != btn {
		t.Fatal("button should own the capture after press")
	}
	if !m.RouteMouseUp(120, 115, 0) {
		t.Fatal("mouse up inside button should be consumed")
	}
	if clicks != 1 {
		t.Fatalf("expected 1 click, got %d", clicks)
	}
	if btn.Downed {
		t.Fatal("button should reset Downed after release")
	}

	// 内部按下，拖出范围，外部抬起 → 不触发点击。
	m.RouteMouseDown(110, 110, 0)
	m.RouteMouseMove(300, 300)
	if btn.Downed {
		t.Fatal("button Downed should track cursor out of range while captured")
	}
	m.RouteMouseUp(300, 300, 0)
	if clicks != 1 {
		t.Fatalf("expected still 1 click after drag-out, got %d", clicks)
	}
}

func TestGridSelectDownEqualsUp(t *testing.T) {
	m := newTestManager()
	grid := NewUIControl("grid", KindGrid)
	grid.Left, grid.Top = 33, 43
	grid.ColCount, grid.RowCount = 8, 6
	grid.ColWidth, grid.RowHeight = 36, 32
	var selCol, selRow int
	selects := 0
	grid.OnGridSelect = func(c *UIControl, col, row int) {
		selects++
		selCol, selRow = col, row
	}
	grid.Visible = true
	m.Root.AddChild(grid)

	// 格子 (2,1): x = 33 + 2*36 + 5 = 110, y = 43 + 1*32 + 5 = 80。
	m.RouteMouseDown(110, 80, 0)
	if m.Capture != grid {
		t.Fatal("grid should own capture after cell press")
	}
	// 同一格子抬起 → 选中。
	m.RouteMouseUp(115, 85, 0)
	if selects != 1 || selCol != 2 || selRow != 1 {
		t.Fatalf("expected select (2,1) once, got %d selects col=%d row=%d", selects, selCol, selRow)
	}

	// 按下格子 (0,0)，抬起格子 (3,2) → 不选中。
	m.RouteMouseDown(40, 50, 0)
	m.RouteMouseUp(33+3*36+5, 43+2*32+5, 0)
	if selects != 1 {
		t.Fatalf("drag across cells must not select, got %d selects", selects)
	}
}

func TestWindowDragAndClamp(t *testing.T) {
	m := newTestManager()
	win := NewUIControl("win", KindWindow)
	win.Floating = true
	win.Left, win.Top, win.Width, win.Height = 200, 100, 120, 120
	win.Visible = true
	m.Root.AddChild(win)

	// 按下窗口主体 → 记录抓取点。
	if !m.RouteMouseDown(210, 110, 0) {
		t.Fatal("window should consume press")
	}
	if win.SpotX != 210 || win.SpotY != 110 {
		t.Fatalf("grab spot not recorded: %d,%d", win.SpotX, win.SpotY)
	}
	// 拖拽 +50,+30 → 窗口移动。
	m.RouteMouseMove(260, 140)
	if win.Left != 250 || win.Top != 130 {
		t.Fatalf("window did not follow drag: %d,%d", win.Left, win.Top)
	}
	// 拖到远超右边界限制处。
	m.RouteMouseMove(2000, 140)
	maxLeft := m.Root.Width - 60
	if win.Left > maxLeft {
		t.Fatalf("window Left %d exceeds max %d", win.Left, maxLeft)
	}
}

func TestModalShortCircuit(t *testing.T) {
	m := newTestManager()
	behind := NewUIControl("behind", KindButton)
	behind.Left, behind.Top, behind.Width, behind.Height = 100, 100, 40, 30
	behindClicks := 0
	behind.OnClick = func(c *UIControl, x, y int) { behindClicks++ }
	behind.Visible = true
	m.Root.AddChild(behind)

	modal := NewUIControl("modal", KindWindow)
	modal.Left, modal.Top, modal.Width, modal.Height = 90, 90, 200, 150
	modalDown := 0
	modal.OnMouseDown = func(c *UIControl, button, x, y int) { modalDown++ }
	m.ShowModal(modal)

	// 点击 "behind" 所在位置——模态窗口必须拦截。
	if !m.RouteMouseDown(110, 110, 0) {
		t.Fatal("modal should consume all mouse input")
	}
	m.RouteMouseUp(110, 110, 0)
	if behindClicks != 0 {
		t.Fatalf("control behind modal was clicked (%d)", behindClicks)
	}
	if modalDown != 1 {
		t.Fatalf("modal got %d downs, want 1", modalDown)
	}
	m.CloseModal(modal)
}

func TestBackgroundClickAndFocusRelease(t *testing.T) {
	m := newTestManager()
	bgClicks := 0
	m.Root.OnBackgroundClick = func(c *UIControl) { bgClicks++ }

	btn := NewUIControl("btn", KindButton)
	btn.Left, btn.Top, btn.Width, btn.Height = 10, 10, 30, 30
	btn.Visible = true
	m.Root.AddChild(btn)

	// 点击按钮 → 获得焦点。
	m.RouteMouseDown(15, 15, 0)
	m.RouteMouseUp(15, 15, 0)
	if m.Focused != btn {
		t.Fatal("pressing EnableFocus button should focus it")
	}
	// 点击空白区域 → 触发背景点击并清除焦点。
	m.RouteMouseDown(500, 500, 0)
	if bgClicks != 1 {
		t.Fatalf("expected 1 background click, got %d", bgClicks)
	}
	if m.Focused != nil {
		t.Fatal("background click should release focus")
	}
	// WantReturn=true → 背景点击消费事件。
	m.Root.OnBackgroundClick = func(c *UIControl) { c.WantReturn = true }
	if !m.RouteMouseDown(500, 500, 0) {
		t.Fatal("background click with WantReturn should be consumed")
	}
}

func TestKeyboardFocusRouting(t *testing.T) {
	m := newTestManager()
	btn := NewUIControl("btn", KindButton)
	btn.Left, btn.Top, btn.Width, btn.Height = 10, 10, 30, 30
	got := ""
	btn.OnChar = func(c *UIControl, ch rune) { got += string(ch) }
	btn.Visible = true
	m.Root.AddChild(btn)

	// 无焦点 → 字符不投递。
	if m.RouteChar('a') {
		t.Fatal("char should not route without focus")
	}
	// 通过点击使按钮获得焦点 → 字符投递成功。
	m.RouteMouseDown(15, 15, 0)
	m.RouteMouseUp(15, 15, 0)
	if !m.RouteChar('a') || !m.RouteChar('b') {
		t.Fatal("focused control should receive chars")
	}
	if got != "ab" {
		t.Fatalf("expected 'ab', got %q", got)
	}
}

func TestRaiseToTopChangesHitOrder(t *testing.T) {
	m := newTestManager()
	w1 := NewUIControl("w1", KindWindow)
	w1.Floating = true
	w1.Left, w1.Top, w1.Width, w1.Height = 100, 100, 100, 100
	w2 := NewUIControl("w2", KindWindow)
	w2.Floating = true
	w2.Left, w2.Top, w2.Width, w2.Height = 150, 150, 100, 100
	who := ""
	w1.OnMouseDown = func(c *UIControl, b, x, y int) { who = "w1" }
	w2.OnMouseDown = func(c *UIControl, b, x, y int) { who = "w2" }
	w1.Visible = true
	w2.Visible = true
	m.Root.AddChild(w1)
	m.Root.AddChild(w2)

	// 重叠区域 (160,160): w2 后添加 → 在最上层 → 先命中。
	m.RouteMouseDown(160, 160, 0)
	if who != "w2" {
		t.Fatalf("expected w2 on top, got %q", who)
	}
	// 点击 w1 非重叠区域 → 提升到 w2 之上。
	m.RouteMouseUp(160, 160, 0)
	m.RouteMouseDown(110, 110, 0)
	m.RouteMouseUp(110, 110, 0)
	if who != "w1" {
		t.Fatalf("expected w1 hit, got %q", who)
	}
	// 此时重叠区域应命中 w1（已提升）。
	m.RouteMouseDown(160, 160, 0)
	if who != "w1" {
		t.Fatalf("expected w1 raised on top, got %q", who)
	}
}
