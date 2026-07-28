package main

import (
	"strconv"
	"strings"
)

// Tooltip — 移植自 DScreen.ShowHint/ClearHint/DrawHint (DrawScrn.pas:195-223,
// 417-447)。鼠标移动时重新计算, 每帧渲染。调用方传入以 '\' 分隔的
// 多行文本 (Delphi 惯例)。
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

// Show 设置提示框。text 可含 '\' 分隔多行。
// drawUp 将提示框放在锚点上方 (Delphi 背包格提示)。
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

// Render 在最顶层绘制提示框。
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

	// 背景面板 Prguse[394]。Delphi 从图片左上角 1:1 绘制
	// (DrawScrn.pas:426-436), 将框体限制在图片尺寸内
	// (:428-429), 而非拉伸纹理。
	var hintTex uint32
	var imgW, imgH int
	if s.resources.Prguse != nil {
		img := s.resources.Prguse.GetImage(ImgHintBg)
		tex := s.resources.GetTexture(s.resources.Prguse, ImgHintBg)
		if img != nil && img.RGBA != nil && tex != 0 {
			hintTex = tex
			imgW, imgH = img.Width, img.Height
			if w > imgW {
				w = imgW
			}
			if h > imgH {
				h = imgH
			}
		}
	}

	// 定位提示框, 限制在屏幕边缘内且坐标非负
	// (DrawScrn.pas:430-434; drawUp 可能使 y 为负)。
	x := t.x
	if x+w > ScreenWidth {
		x = ScreenWidth - w
	}
	if x < 0 {
		x = 0
	}
	y := t.y
	if t.drawUp {
		y -= h
	}
	if y < 0 {
		y = 0
	}
	fx, fy := float32(x), float32(y)

	if hintTex != 0 {
		// 从图片左上角取 1:1 源子矩形, alpha=1 (:436)。
		s.gl.DrawQuadSub(hintTex, float32(imgW), float32(imgH),
			0, 0, float32(w), float32(h), fx, fy, float32(w), float32(h),
			1, 1, 1, 1, proj)
	} else {
		s.gl.DrawQuadColor(fx, fy, float32(w), float32(h), 0.05, 0.05, 0.1, 0.9, proj)
	}

	for i, ln := range t.lines {
		s.text.DrawText(ln, fx+hintPadX, fy+hintPadY+float32(i*lineH),
			t.color[0], t.color[1], t.color[2], t.color[3], proj)
	}
}

// GetMouseItemInfo 构建物品悬浮文本 (精简移植自
// FState.pas:3935-4448): 名称 \ 重量 \ 持久 \ 属性范围。等级不足时
// useable 为 false (红色提示, :4400-4420)。
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
	// Delphi 显示 Round(dura/1000) (FState.pas:3936-3942); +500 实现四舍五入。
	if item.DuraMax > 0 {
		parts = append(parts, "Dura "+strconv.Itoa((int(item.Dura)+500)/1000)+"/"+strconv.Itoa((int(item.DuraMax)+500)/1000))
	} else if item.Dura > 0 {
		parts = append(parts, "Dura "+strconv.Itoa((int(item.Dura)+500)/1000))
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
