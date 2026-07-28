package main

import (
	"github.com/pyq0109/mirgo/internal/wil"
)

// UI 控件框架 — Delphi DWinCtl.pas 语义的 Go 移植
// (TDControl/TDButton/TDWindow/TDGrid/TDWinManager). 控件组成一棵树,
// 根节点是全屏 Background 控件; 传给处理函数的所有事件坐标都处于
// 控件的父级坐标空间 (与 Delphi 递归一致, 即向子控件传递
// X-Left/Y-Top).

type UIKind int

const (
	KindControl UIKind = iota // 普通 TDControl
	KindButton                // TDButton: 在区域内抬起鼠标时触发点击
	KindWindow                // TDWindow: 可选浮动拖动 + 置顶
	KindGrid                  // TDGrid: 固定单元格, 属主自绘
)

func (k UIKind) String() string {
	switch k {
	case KindControl:
		return "control"
	case KindButton:
		return "button"
	case KindWindow:
		return "window"
	case KindGrid:
		return "grid"
	default:
		return "unknown"
	}
}

type UIControl struct {
	Name   string
	Kind   UIKind
	Left   int
	Top    int
	Width  int
	Height int

	Parent   *UIControl
	Children []*UIControl

	Visible     bool
	Background  bool // 画布控件: 命中穿透, 点击清除焦点
	EnableFocus bool
	Floating    bool // KindWindow: 可拖动 + 置顶

	WLib      *wil.File
	FaceIndex int
	Tag       int

	// KindButton 状态.
	Downed bool
	// KindWindow 拖动状态 (父空间抓取点).
	SpotX, SpotY int
	// KindBackground 点击消费协议 (处理函数可设置 WantReturn).
	WantReturn bool

	// KindGrid.
	ColCount, RowCount   int
	ColWidth, RowHeight  int
	SelectCol, SelectRow int // 按下的单元格 (驱动选择 + 高亮)
	Col, Row             int // 上次确认的选择

	// ClickSound 是按钮点击时播放的音效索引（-1=无声）。
	// 对应 Delphi TClickSound: csNorm=103, csStone=104, csGlass=105。
	ClickSound int

	// 事件. nil 处理函数直接不触发.
	OnDirectPaint     func(c *UIControl, proj [16]float32)
	OnClick           func(c *UIControl, x, y int)
	OnDblClick        func(c *UIControl, x, y int)
	OnMouseDown       func(c *UIControl, button, x, y int)
	OnMouseUp         func(c *UIControl, button, x, y int)
	OnMouseMove       func(c *UIControl, x, y int)
	OnBackgroundClick func(c *UIControl)
	OnInRealArea      func(c *UIControl, x, y int) bool
	OnChar            func(c *UIControl, ch rune)
	OnKeyDown         func(c *UIControl, key int)
	// 网格事件. 矩形为绝对屏幕坐标.
	OnGridPaint     func(c *UIControl, col, row int, x, y, w, h int, selected bool, proj [16]float32)
	OnGridSelect    func(c *UIControl, col, row int)
	OnGridMouseMove func(c *UIControl, col, row int)
}

// NewUIControl 按 Delphi 构造函数默认值创建控件
// (DWinCtl.pas:235-260): 隐藏、80×24、禁用焦点.
func NewUIControl(name string, kind UIKind) *UIControl {
	c := &UIControl{
		Name:       name,
		Kind:       kind,
		Width:      80,
		Height:     24,
		Visible:    false,
		ClickSound: -1,
	}
	switch kind {
	case KindButton, KindWindow:
		c.EnableFocus = true // TDButton/TDWindow 默认值 (DWinCtl:650,806)
	case KindGrid:
		// Width/Height 保持 0: 边界由单元格几何推导, 除非构建方显式
		// 设置 (Delphi 通过 DFM 的 Width/Height 设定 TDGrid 尺寸,
		// 可能裁剪单元格区域 — FState:1173-1174 对 8×6 背包网格设置了
		// 286×162).
		c.Width, c.Height = 0, 0
		c.ColCount, c.RowCount = 8, 5 // TDGrid 默认值 (DWinCtl:702-705)
		c.ColWidth, c.RowHeight = 36, 32
	}
	return c
}

func (c *UIControl) AddChild(ch *UIControl) {
	ch.Parent = c
	c.Children = append(c.Children, ch)
}

func (c *UIControl) RemoveChild(ch *UIControl) {
	for i, x := range c.Children {
		if x == ch {
			c.Children = append(c.Children[:i], c.Children[i+1:]...)
			ch.Parent = nil
			return
		}
	}
}

// RaiseToTop 将浮动窗口移到父控件子列表末尾
// (末尾 = 最后绘制 = 最先命中检测). TDWindow.ChangeChildOrder,
// DWinCtl.pas:383-397.
func (c *UIControl) RaiseToTop() {
	p := c.Parent
	if c.Kind != KindWindow || !c.Floating || p == nil {
		return
	}
	p.RemoveChild(c)
	p.AddChild(c)
}

// AbsX/AbsY 将控件原点换算为绝对屏幕坐标
// (SurfaceX(Left)/SurfaceY(Top), DWinCtl.pas:325-349,634).
func (c *UIControl) AbsX() int {
	x := c.Left
	for p := c.Parent; p != nil; p = p.Parent {
		x += p.Left
	}
	return x
}

func (c *UIControl) AbsY() int {
	y := c.Top
	for p := c.Parent; p != nil; p = p.Parent {
		y += p.Top
	}
	return y
}

// ParentSpaceX 将绝对坐标换算到本控件的父级坐标空间
// (LocalX, DWinCtl.pas:352-376): 控件自身的 Left/Top 以及处理函数
// 接收的事件都处于该空间.
func (c *UIControl) ParentSpaceX(absX int) int {
	x := absX
	for p := c.Parent; p != nil; p = p.Parent {
		x -= p.Left
	}
	return x
}

func (c *UIControl) ParentSpaceY(absY int) int {
	y := absY
	for p := c.Parent; p != nil; p = p.Parent {
		y -= p.Top
	}
	return y
}

// InRange 对父空间下的 (x, y) 做命中检测. 先判矩形, 再对带图像的
// 控件做逐像素 alpha 检测, 或使用 OnInRealArea 覆写
// (TDControl.InRange, DWinCtl.pas:399-418).
// effectiveSize 即 Width/Height; 网格的边界由单元格几何推导,
// 除非显式设置了 Width/Height (Delphi DFM 惯例, 例如背包网格
// 运行时裁剪为 286×162, FState:1173-1174).
func (c *UIControl) effectiveSize() (int, int) {
	if c.Kind == KindGrid && (c.Width <= 0 || c.Height <= 0) {
		return c.ColCount * c.ColWidth, c.RowCount * c.RowHeight
	}
	return c.Width, c.Height
}

func (c *UIControl) InRange(x, y int) bool {
	w, h := c.effectiveSize()
	if x < c.Left || x >= c.Left+w || y < c.Top || y >= c.Top+h {
		return false
	}
	if c.OnInRealArea != nil {
		return c.OnInRealArea(c, x-c.Left, y-c.Top)
	}
	if c.WLib != nil {
		img := c.WLib.GetImage(c.FaceIndex)
		if img != nil && img.RGBA != nil {
			px, py := x-c.Left, y-c.Top
			if px >= 0 && px < img.Width && py >= 0 && py < img.Height {
				if img.RGBA.Pix[py*img.RGBA.Stride+px*4+3] == 0 {
					return false
				}
			}
		}
	}
	return true
}

// SetImgIndex 设置显示图像并据此自动设定控件尺寸
// (TDControl.SetImgIndex, DWinCtl.pas:607-621).
func (c *UIControl) SetImgIndex(f *wil.File, idx int) {
	if f == nil {
		return
	}
	c.WLib = f
	c.FaceIndex = idx
	if img := f.GetImage(idx); img != nil {
		c.Width = img.Width
		c.Height = img.Height
	}
}

// ColRowAt 将父空间坐标映射到网格单元格 (TDGrid.GetColRow,
// DWinCtl.pas:711-719).
func (c *UIControl) ColRowAt(x, y int) (col, row int, ok bool) {
	if !c.InRange(x, y) {
		return 0, 0, false
	}
	return (x - c.Left) / c.ColWidth, (y - c.Top) / c.RowHeight, true
}

// Show 使控件可见, 置顶浮动窗口并获取键盘焦点
// (TDWindow.Show, DWinCtl.pas:859-867). 焦点需要管理器参与;
// 需要焦点时调用方应改用 UIManager.ShowWindow.
func (c *UIControl) Show() {
	c.Visible = true
	if c.Kind == KindWindow && c.Floating {
		c.RaiseToTop()
	}
}

func (c *UIControl) Hide() {
	c.Visible = false
}
