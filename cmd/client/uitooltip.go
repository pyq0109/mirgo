package main

import (
	"strconv"
	"strings"
)

// Tooltip — port of DScreen.ShowHint/ClearHint/DrawHint (DrawScrn.pas:195-223,
// 417-447). Recomputed on mouse-move events, rendered every frame. Callers
// pass multi-line text split on '\' (Delphi convention).
type Tooltip struct {
	visible bool
	x, y    int
	drawUp  bool
	lines   []string
	color   [4]float32
}

const (
	hintPadX = 4
	hintPadY = 3
)

// Show sets the tooltip. text may contain '\' separators for multiple lines.
// drawUp places the box above the anchor point (Delphi bag cell hints).
func (t *Tooltip) Show(x, y int, text string, color [4]float32, drawUp bool) {
	t.lines = strings.Split(text, "\\")
	t.color = color
	t.x, t.y = x, y
	t.drawUp = drawUp
	t.visible = true
}

func (t *Tooltip) Clear() {
	t.visible = false
	t.lines = nil
}

// Render draws the tooltip on top of everything else.
func (t *Tooltip) Render(s *PlayScene, proj [16]float32) {
	if !t.visible || len(t.lines) == 0 || s.text == nil {
		return
	}
	lineH := s.text.LineHeight()
	w := 0
	for _, ln := range t.lines {
		if mw := s.text.MeasureText(ln); mw > w {
			w = mw
		}
	}
	w += hintPadX * 2
	h := len(t.lines)*lineH + hintPadY*2

	x := t.x
	if x+w > ScreenWidth {
		x = ScreenWidth - w
	}
	y := t.y
	if t.drawUp {
		y -= h
	}
	fx, fy := float32(x), float32(y)

	// Background panel Prguse[394] (DrawScrn.pas:426), stretched.
	drawn := false
	if s.resources.Prguse != nil {
		img := s.resources.Prguse.GetImage(ImgHintBg)
		tex := s.resources.GetTexture(s.resources.Prguse, ImgHintBg)
		if img != nil && img.RGBA != nil && tex != 0 {
			s.gl.DrawQuadTint(tex, fx, fy, float32(w), float32(h), 1, 1, 1, 0.9, proj)
			drawn = true
		}
	}
	if !drawn {
		s.gl.DrawQuadColor(fx, fy, float32(w), float32(h), 0.05, 0.05, 0.1, 0.9, proj)
	}

	for i, ln := range t.lines {
		s.text.DrawText(ln, fx+hintPadX, fy+hintPadY+float32(i*lineH),
			t.color[0], t.color[1], t.color[2], t.color[3], proj)
	}
}

// GetMouseItemInfo builds the hover text for an item (compact port of
// FState.pas:3935-4448): name \ weight \ dura \ stat ranges. useable is
// false when the level requirement is not met (red hint, :4400-4420).
func GetMouseItemInfo(gs *GameState, item *BagItem) (text string, useable bool) {
	if item == nil {
		return "", false
	}
	def := item.Def
	name := ""
	if def != nil {
		name = def.Name
	}
	if name == "" {
		name = "Item"
	}
	useable = true
	parts := []string{name}
	if def != nil && def.Weight > 0 {
		parts = append(parts, "Weight "+strconv.Itoa(int(def.Weight)))
	}
	if item.DuraMax > 0 {
		parts = append(parts, "Dura "+strconv.Itoa(int(item.Dura))+"/"+strconv.Itoa(int(item.DuraMax)))
	}
	if def != nil {
		if def.AC > 0 || def.ACMax > 0 {
			parts = append(parts, "AC "+strconv.Itoa(int(def.AC))+"-"+strconv.Itoa(int(def.ACMax)))
		}
		if def.MAC > 0 || def.MACMax > 0 {
			parts = append(parts, "MAC "+strconv.Itoa(int(def.MAC))+"-"+strconv.Itoa(int(def.MACMax)))
		}
		if def.DC > 0 || def.DCMax > 0 {
			parts = append(parts, "DC "+strconv.Itoa(int(def.DC))+"-"+strconv.Itoa(int(def.DCMax)))
		}
		if def.MC > 0 || def.MCMax > 0 {
			parts = append(parts, "MC "+strconv.Itoa(int(def.MC))+"-"+strconv.Itoa(int(def.MCMax)))
		}
		if def.SC > 0 || def.SCMax > 0 {
			parts = append(parts, "SC "+strconv.Itoa(int(def.SC))+"-"+strconv.Itoa(int(def.SCMax)))
		}
		if def.NeedLevel > 0 {
			parts = append(parts, "Need Lv "+strconv.Itoa(int(def.NeedLevel)))
			if gs.Level < int(def.NeedLevel) {
				useable = false
			}
		}
	}
	return strings.Join(parts, "\\"), useable
}
