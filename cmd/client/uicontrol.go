package main

import (
	"github.com/pyq0109/mirgo/internal/wil"
)

// UI control framework — a Go port of the Delphi DWinCtl.pas semantics
// (TDControl/TDButton/TDWindow/TDGrid/TDWinManager). Controls form a tree
// rooted at a full-screen Background control; all event coordinates passed
// to handlers are in the control's PARENT coordinate space (matching the
// Delphi recursion, which forwards X-Left/Y-Top to children).

type UIKind int

const (
	KindControl UIKind = iota // plain TDControl
	KindButton                // TDButton: click fires on mouse-up-inside
	KindWindow                // TDWindow: optional floating drag + raise-to-top
	KindGrid                  // TDGrid: fixed cells, owner-painted
)

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
	Background  bool // canvas control: hit passes through, click clears focus
	EnableFocus bool
	Floating    bool // KindWindow: draggable + raises to top

	WLib      *wil.File
	FaceIndex int
	Tag       int

	// KindButton state.
	Downed bool
	// KindWindow drag state (parent-space grab point).
	SpotX, SpotY int
	// KindBackground click-consume protocol (handler may set WantReturn).
	WantReturn bool

	// KindGrid.
	ColCount, RowCount   int
	ColWidth, RowHeight  int
	SelectCol, SelectRow int // pressed cell (drives selection + highlight)
	Col, Row             int // last committed selection

	// Events. Nil handlers are simply not fired.
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
	// Grid events. Rects are absolute screen coords.
	OnGridPaint     func(c *UIControl, col, row int, x, y, w, h int, selected bool, proj [16]float32)
	OnGridSelect    func(c *UIControl, col, row int)
	OnGridMouseMove func(c *UIControl, col, row int)
}

// NewUIControl creates a control with Delphi constructor defaults
// (DWinCtl.pas:235-260): hidden, 80×24, focus disabled.
func NewUIControl(name string, kind UIKind) *UIControl {
	c := &UIControl{
		Name:    name,
		Kind:    kind,
		Width:   80,
		Height:  24,
		Visible: false,
	}
	switch kind {
	case KindButton, KindWindow:
		c.EnableFocus = true // TDButton/TDWindow default (DWinCtl:650,806)
	case KindGrid:
		c.ColCount, c.RowCount = 8, 5 // TDGrid defaults (DWinCtl:702-705)
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

// RaiseToTop moves a floating window to the end of its parent's child list
// (last = painted last = hit-tested first). TDWindow.ChangeChildOrder,
// DWinCtl.pas:383-397.
func (c *UIControl) RaiseToTop() {
	p := c.Parent
	if c.Kind != KindWindow || !c.Floating || p == nil {
		return
	}
	p.RemoveChild(c)
	p.AddChild(c)
}

// AbsX/AbsY convert the control's origin to absolute screen coords
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

// ParentSpaceX converts an absolute coord into this control's parent
// coordinate space (LocalX, DWinCtl.pas:352-376): the space in which the
// control's own Left/Top live and its handlers receive events.
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

// InRange hit-tests (x, y) given in parent space. Rectangle first, then
// per-pixel alpha for image-backed controls or the OnInRealArea override
// (TDControl.InRange, DWinCtl.pas:399-418).
// effectiveSize is Width/Height, except for grids whose bounds derive from
// the cell geometry (Delphi sizes TDGrid via ColCount×ColWidth in the DFM).
func (c *UIControl) effectiveSize() (int, int) {
	if c.Kind == KindGrid {
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

// SetImgIndex assigns the displayed image and auto-sizes the control from it
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

// ColRowAt maps parent-space coords to a grid cell (TDGrid.GetColRow,
// DWinCtl.pas:711-719).
func (c *UIControl) ColRowAt(x, y int) (col, row int, ok bool) {
	if !c.InRange(x, y) {
		return 0, 0, false
	}
	return (x - c.Left) / c.ColWidth, (y - c.Top) / c.RowHeight, true
}

// Show makes the control visible, raising floating windows and taking
// keyboard focus (TDWindow.Show, DWinCtl.pas:859-867). The manager is
// needed for focus; callers use UIManager.ShowWindow instead when focus
// matters.
func (c *UIControl) Show() {
	c.Visible = true
	if c.Kind == KindWindow && c.Floating {
		c.RaiseToTop()
	}
}

func (c *UIControl) Hide() {
	c.Visible = false
}
