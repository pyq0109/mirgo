package main

import (
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

// UIManager routes input and paint across the control tree. Port of
// TDWinManager (DWinCtl.pas:878-1122): modal short-circuits everything,
// then mouse capture, then the tree rooted at Root (the full-screen
// DBackground control). Paint order = tree order, modal last (on top).
type UIManager struct {
	gl        *engine.GLState
	resources *engine.ResourceManager
	text      *engine.TextRenderer

	Root    *UIControl
	Modal   *UIControl
	Capture *UIControl
	Focused *UIControl
}

func NewUIManager(gl *engine.GLState, resources *engine.ResourceManager, text *engine.TextRenderer) *UIManager {
	root := NewUIControl("DBackground", KindControl)
	root.Background = true
	root.Visible = true
	root.Left, root.Top = 0, 0
	root.Width, root.Height = ScreenWidth, ScreenHeight
	return &UIManager{
		gl:        gl,
		resources: resources,
		text:      text,
		Root:      root,
	}
}

func (m *UIManager) SetText(t *engine.TextRenderer) { m.text = t }

// ---------------------------------------------------------------------------
// Focus / capture (DWinCtl.pas:211-231)
// ---------------------------------------------------------------------------

func (m *UIManager) SetFocus(c *UIControl)   { m.Focused = c }
func (m *UIManager) ReleaseFocus()           { m.Focused = nil }
func (m *UIManager) SetCapture(c *UIControl) { m.Capture = c }
func (m *UIManager) ReleaseCapture()         { m.Capture = nil }

// canFocusMsg: a control receives mouse messages only when no capture is
// active, or it (or its parent) owns the capture (DWinCtl.pas:456-462).
func (m *UIManager) canFocusMsg(c *UIControl) bool {
	return m.Capture == nil || m.Capture == c || m.Capture == c.Parent
}

// ShowWindow shows the window, raises floating ones and takes focus
// (TDWindow.Show, DWinCtl.pas:859-867).
func (m *UIManager) ShowWindow(c *UIControl) {
	c.Show()
	if c.EnableFocus {
		m.SetFocus(c)
	}
}

// ShowModal shows the window as modal: all input routes to it and it paints
// on top (TDWindow.ShowModal, DWinCtl.pas:869-875; manager routing
// :1008-1015). Non-blocking — results are delivered via callbacks.
func (m *UIManager) ShowModal(c *UIControl) {
	c.Visible = true
	m.Modal = c
	if c.EnableFocus {
		m.SetFocus(c)
	}
}

func (m *UIManager) CloseModal(c *UIControl) {
	if m.Modal == c {
		m.Modal = nil
	}
	c.Hide()
}

// ---------------------------------------------------------------------------
// Input routing
// ---------------------------------------------------------------------------

// RouteMouseDown returns true if the UI consumed the event (the game world
// must ignore it — mirrors ClMain.pas `if g_DWinMan.MouseDown then exit`).
func (m *UIManager) RouteMouseDown(absX, absY int, button int) bool {
	if m.Modal != nil && m.Modal.Visible {
		return m.dispatchMouseDown(m.Modal, button, m.Modal.ParentSpaceX(absX), m.Modal.ParentSpaceY(absY))
	}
	if m.Capture != nil {
		return m.dispatchMouseDown(m.Capture, button, m.Capture.ParentSpaceX(absX), m.Capture.ParentSpaceY(absY))
	}
	return m.dispatchMouseDown(m.Root, button, absX-m.Root.Left, absY-m.Root.Top)
}

func (m *UIManager) RouteMouseUp(absX, absY int, button int) bool {
	if m.Modal != nil && m.Modal.Visible {
		return m.dispatchMouseUp(m.Modal, button, m.Modal.ParentSpaceX(absX), m.Modal.ParentSpaceY(absY))
	}
	if m.Capture != nil {
		return m.dispatchMouseUp(m.Capture, button, m.Capture.ParentSpaceX(absX), m.Capture.ParentSpaceY(absY))
	}
	return m.dispatchMouseUp(m.Root, button, absX-m.Root.Left, absY-m.Root.Top)
}

func (m *UIManager) RouteMouseMove(absX, absY int) bool {
	if m.Modal != nil && m.Modal.Visible {
		return m.dispatchMouseMove(m.Modal, m.Modal.ParentSpaceX(absX), m.Modal.ParentSpaceY(absY))
	}
	if m.Capture != nil {
		return m.dispatchMouseMove(m.Capture, m.Capture.ParentSpaceX(absX), m.Capture.ParentSpaceY(absY))
	}
	return m.dispatchMouseMove(m.Root, absX-m.Root.Left, absY-m.Root.Top)
}

// RouteDblClick routes a synthesized double-click (capture-first semantics,
// TDControl.DblClick DWinCtl.pas:553-578).
func (m *UIManager) RouteDblClick(absX, absY int) bool {
	if m.Modal != nil && m.Modal.Visible {
		return m.dispatchDblClick(m.Modal, m.Modal.ParentSpaceX(absX), m.Modal.ParentSpaceY(absY))
	}
	if m.Capture != nil {
		return m.dispatchDblClick(m.Capture, m.Capture.ParentSpaceX(absX), m.Capture.ParentSpaceY(absY))
	}
	return m.dispatchDblClick(m.Root, absX-m.Root.Left, absY-m.Root.Top)
}

// RouteChar / RouteKeyDown: modal subtree first, else the focused control
// (TDWinManager.KeyPress/KeyDown, DWinCtl.pas:916-974).
func (m *UIManager) RouteChar(ch rune) bool {
	target := m.Focused
	if m.Modal != nil && m.Modal.Visible {
		target = m.Modal
	}
	if target == nil {
		return false
	}
	return m.dispatchChar(target, ch)
}

func (m *UIManager) RouteKeyDown(key int) bool {
	target := m.Focused
	if m.Modal != nil && m.Modal.Visible {
		target = m.Modal
	}
	if target == nil {
		return false
	}
	return m.dispatchKeyDown(target, key)
}

// ---------------------------------------------------------------------------
// Recursive dispatch (TDControl methods, DWinCtl.pas:464-605,665-852)
// ---------------------------------------------------------------------------

func (m *UIManager) dispatchMouseDown(c *UIControl, button, x, y int) bool {
	// Children back-to-front (topmost painted last = tested first).
	// TDGrid does not recurse (its MouseDown override skips inherited).
	if c.Kind != KindGrid {
		for i := len(c.Children) - 1; i >= 0; i-- {
			ch := c.Children[i]
			if ch.Visible && m.dispatchMouseDown(ch, button, x-c.Left, y-c.Top) {
				return true
			}
		}
	}

	switch c.Kind {
	case KindGrid:
		// TDGrid.MouseDown (:721-736): left button, cell hit → capture.
		if button == 0 {
			if col, row, ok := c.ColRowAt(x, y); ok {
				c.SelectCol, c.SelectRow = col, row
				m.SetCapture(c)
				return true
			}
		}
		return false

	default:
		if c.Background {
			// TDControl.MouseDown Background branch (:504-511).
			if c.OnBackgroundClick != nil {
				c.WantReturn = false
				c.OnBackgroundClick(c)
				consumed := c.WantReturn
				m.ReleaseFocus()
				return consumed
			}
			m.ReleaseFocus()
			return false
		}
		if m.canFocusMsg(c) && (c.InRange(x, y) || m.Capture == c) {
			if c.OnMouseDown != nil {
				c.OnMouseDown(c, button, x, y)
			}
			if c.EnableFocus {
				m.SetFocus(c)
			}
			if c.Kind == KindButton || c.Kind == KindWindow {
				// TDButton.MouseDown (:665-675): press visual + capture.
				// TDWindow subclasses TDButton, so windows capture too —
				// that is what makes floating drag work.
				c.Downed = true
				m.SetCapture(c)
			}
			if c.Kind == KindWindow && c.Floating {
				// TDWindow.MouseDown (:841-852): raise + grab spot.
				c.RaiseToTop()
				c.SpotX, c.SpotY = x, y
			}
			return true
		}
		return false
	}
}

func (m *UIManager) dispatchMouseUp(c *UIControl, button, x, y int) bool {
	if c.Kind != KindGrid {
		for i := len(c.Children) - 1; i >= 0; i-- {
			ch := c.Children[i]
			if ch.Visible && m.dispatchMouseUp(ch, button, x-c.Left, y-c.Top) {
				return true
			}
		}
	}

	switch c.Kind {
	case KindGrid:
		// TDGrid.MouseUp (:752-769): select only when up-cell == down-cell.
		consumed := false
		if button == 0 {
			if col, row, ok := c.ColRowAt(x, y); ok {
				if c.SelectCol == col && c.SelectRow == row {
					c.Col, c.Row = col, row
					if c.OnGridSelect != nil {
						c.OnGridSelect(c, col, row)
					}
				}
				consumed = true
			}
			m.ReleaseCapture()
		}
		return consumed

	default:
		if m.Capture != nil {
			if m.Capture == c {
				if c.Kind == KindButton || c.Kind == KindWindow {
					// TDButton.MouseUp (:677-695): click fires only if the
					// release is still inside the button (TDWindow inherits
					// this — TDWindow.MouseUp just calls inherited).
					m.ReleaseCapture()
					if !c.Background && c.InRange(x, y) {
						if c.OnMouseUp != nil {
							c.OnMouseUp(c, button, x, y)
						}
						if c.OnClick != nil {
							c.OnClick(c, x, y)
						}
					}
					c.Downed = false
					return true
				}
				if c.OnMouseUp != nil {
					c.OnMouseUp(c, button, x, y)
				}
				return true
			}
			return false // capture belongs to another control
		}
		if c.Background {
			return false
		}
		if c.InRange(x, y) {
			if c.OnMouseUp != nil {
				c.OnMouseUp(c, button, x, y)
			}
			// In Delphi a WM_LBUTTONCLK follows the up; controls with
			// OnClick (e.g. DStateWin magic-page hit test) get it here.
			if c.OnClick != nil {
				c.OnClick(c, x, y)
			}
			if c.Kind == KindButton || c.Kind == KindWindow {
				c.Downed = false
			}
			return true
		}
		return false
	}
}

func (m *UIManager) dispatchMouseMove(c *UIControl, x, y int) bool {
	if c.Kind != KindGrid {
		for i := len(c.Children) - 1; i >= 0; i-- {
			ch := c.Children[i]
			if ch.Visible && m.dispatchMouseMove(ch, x-c.Left, y-c.Top) {
				return true
			}
		}
	}

	switch c.Kind {
	case KindGrid:
		// TDGrid.MouseMove (:738-750): hover cell notification.
		if col, row, ok := c.ColRowAt(x, y); ok {
			if c.OnGridMouseMove != nil {
				c.OnGridMouseMove(c, col, row)
			}
			return true
		}
		return false

	default:
		if m.Capture != nil {
			if m.Capture == c {
				if c.Kind == KindButton {
					// TDButton press visual follows the cursor (:654-663).
					c.Downed = c.InRange(x, y)
				}
				if c.Kind == KindWindow && c.Floating {
					// TDWindow.MouseMove drag with clamp (:820-839).
					if c.SpotX != x || c.SpotY != y {
						al := c.Left + (x - c.SpotX)
						at := c.Top + (y - c.SpotY)
						if al+c.Width < WinLeft {
							al = WinLeft - c.Width
						}
						if al > WinRight {
							al = WinRight
						}
						if at+c.Height < WinTop {
							at = WinTop - c.Height
						}
						if at+c.Height > WinBottom {
							at = WinBottom - c.Height
						}
						c.Left, c.Top = al, at
						c.SpotX, c.SpotY = x, y
					}
				}
				if c.OnMouseMove != nil {
					c.OnMouseMove(c, x, y)
				}
				return true
			}
			return false
		}
		if c.Background {
			return false
		}
		if c.InRange(x, y) {
			if c.OnMouseMove != nil {
				c.OnMouseMove(c, x, y)
			}
			return true
		}
		return false
	}
}

func (m *UIManager) dispatchDblClick(c *UIControl, x, y int) bool {
	// Capture takes priority over children (DWinCtl.pas:558-565).
	if m.Capture != nil {
		if m.Capture == c && c.OnDblClick != nil {
			c.OnDblClick(c, x, y)
			return true
		}
		return false
	}
	for i := len(c.Children) - 1; i >= 0; i-- {
		ch := c.Children[i]
		if ch.Visible && m.dispatchDblClick(ch, x-c.Left, y-c.Top) {
			return true
		}
	}
	if c.Background {
		return false
	}
	if c.InRange(x, y) && c.OnDblClick != nil {
		c.OnDblClick(c, x, y)
		return true
	}
	return false
}

func (m *UIManager) dispatchChar(c *UIControl, ch rune) bool {
	if c.Background {
		return false
	}
	for i := len(c.Children) - 1; i >= 0; i-- {
		child := c.Children[i]
		if child.Visible && m.dispatchChar(child, ch) {
			return true
		}
	}
	if m.Focused == c {
		if c.OnChar != nil {
			c.OnChar(c, ch)
		}
		return true
	}
	return false
}

func (m *UIManager) dispatchKeyDown(c *UIControl, key int) bool {
	if c.Background {
		return false
	}
	for i := len(c.Children) - 1; i >= 0; i-- {
		child := c.Children[i]
		if child.Visible && m.dispatchKeyDown(child, key) {
			return true
		}
	}
	if m.Focused == c {
		if c.OnKeyDown != nil {
			c.OnKeyDown(c, key)
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Painting (TDControl.DirectPaint, DWinCtl.pas:623-639; TDGrid :783-796;
// TDWinManager :1108-1122)
// ---------------------------------------------------------------------------

func (m *UIManager) Paint(proj [16]float32) {
	m.paintControl(m.Root, proj)
	if m.Modal != nil && m.Modal.Visible {
		m.paintControl(m.Modal, proj)
	}
}

func (m *UIManager) paintControl(c *UIControl, proj [16]float32) {
	if !c.Visible {
		return
	}

	if c.Kind == KindGrid {
		// Grid is owner-painted; it draws no image of its own and has no
		// painted children (TDGrid.DirectPaint does not call inherited).
		if c.OnGridPaint != nil {
			ax, ay := c.AbsX(), c.AbsY()
			for row := 0; row < c.RowCount; row++ {
				for col := 0; col < c.ColCount; col++ {
					sel := c.SelectCol == col && c.SelectRow == row
					c.OnGridPaint(c, col, row, ax+col*c.ColWidth, ay+row*c.RowHeight, c.ColWidth, c.RowHeight, sel, proj)
				}
			}
		}
		return
	}

	if c.OnDirectPaint != nil {
		c.OnDirectPaint(c, proj)
	} else if c.WLib != nil {
		m.BlitImage(c.WLib, c.FaceIndex, c.AbsX(), c.AbsY(), proj)
	}

	for _, ch := range c.Children {
		if ch.Visible {
			m.paintControl(ch, proj)
		}
	}
}

// BlitImage draws a WIL image at absolute screen coords (the default
// control paint, DWinCtl.pas:631-635). Returns false when unavailable.
func (m *UIManager) BlitImage(f *wil.File, idx, x, y int, proj [16]float32) bool {
	if f == nil || m.resources == nil {
		return false
	}
	img := f.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return false
	}
	tex := m.resources.GetTexture(f, idx)
	if tex == 0 {
		return false
	}
	m.gl.DrawQuad(tex, float32(x), float32(y), float32(img.Width), float32(img.Height), proj)
	return true
}
