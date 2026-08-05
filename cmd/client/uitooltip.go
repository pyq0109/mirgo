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

// delphiRound 复刻 Delphi Round 的银行家舍入（半取偶）：
// 计算 Round(v/scale)。
func delphiRound(v, scale int) int {
	if scale <= 0 {
		return v
	}
	q := v / scale
	r := v % scale
	if r < 0 {
		r = -r
	}
	half := scale / 2
	if scale%2 == 0 {
		if r > half || (r == half && q%2 != 0) {
			q++
		}
	} else if r > half {
		q++
	}
	return q
}

// duraStr 持久显示 Round(dura/1000)/Round(duraMax/1000)
//（Delphi GetDuraStr，FState.pas:3936-3942）。
func duraStr(dura, duraMax uint16) string {
	return strconv.Itoa(delphiRound(int(dura), 1000)) + "/" + strconv.Itoa(delphiRound(int(duraMax), 1000))
}

// needLine 装备需求行与 useable 判定（Delphi 武器/衣服/首饰三处
// 完全相同的需求 case，FState.pas:4068-4153）。
func needLine(gs *GameState, def *ClientItemDef) (string, bool) {
	nl := int(def.NeedLevel)
	switch def.Need {
	case 0:
		return "需要等级: " + strconv.Itoa(nl), gs.Level >= nl
	case 1:
		return "需要攻击力: " + strconv.Itoa(nl), int(gs.DC>>16) >= nl
	case 2:
		return "需要魔法力: " + strconv.Itoa(nl), int(gs.MC>>16) >= nl
	case 3:
		return "需要精神力: " + strconv.Itoa(nl), int(gs.SC>>16) >= nl
	case 4:
		return "所需转生等级" + strconv.Itoa(nl), true
	case 5:
		return "所需声望点" + strconv.Itoa(nl), true
	case 6:
		return "行会成员专用", true
	case 7:
		return "沙城成员专用", true
	case 8:
		return "会员专用", true
	case 40:
		return "所需转生&等级" + strconv.Itoa(nl), true
	case 41:
		return "所需转生&攻击力" + strconv.Itoa(nl), true
	case 42:
		return "所需转生&魔法力" + strconv.Itoa(nl), true
	case 43:
		return "所需转生&道术" + strconv.Itoa(nl), true
	case 44:
		return "所需转生&声望点" + strconv.Itoa(nl), true
	case 60:
		return "行会掌门专用", true
	case 70:
		return "沙城城主专用", true
	case 81:
		return "会员类型 =" + strconv.Itoa(nl&0xFFFF) + "会员等级 >=" + strconv.Itoa(nl>>16), true
	case 82:
		return "会员类型 >=" + strconv.Itoa(nl&0xFFFF) + "会员等级 >=" + strconv.Itoa(nl>>16), true
	}
	// 未列出的 Need：不输出需求行，保持不可用（红字）。
	return "", false
}

// GetMouseItemInfo 构建物品悬浮文本（逐分支移植自 Delphi
// GetMouseItemInfo，FState.pas:3935-4448）。返回 '\' 分隔的多行
// 文本与 useable（false 时调用方以红色显示）。
// 注：Go 客户端物品定义为服务端基础值（未按实例 btValue 折叠），
// 升级/幸运等实例加成的折叠显示依赖 652 详细列表的 TClientItem。
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
	line1, line2, line3 := "", "", ""

	if def == nil {
		return name, true
	}

	it := func(v int) string { return strconv.Itoa(v) }

	switch {
	case def.StdMode == 0: // 药水（:3973-4002）
		line1 = " 重量:" + it(int(def.Weight))
		ac, mac := int(def.AC), int(def.MAC)
		switch def.Shape {
		case 0:
			if ac > 0 && mac == 0 {
				line2 = "恢复 " + it(ac) + "生命值"
			} else if mac > 0 && ac == 0 {
				line2 = "恢复 " + it(mac) + "魔法值"
			} else {
				line2 = "恢复 " + it(ac) + "生命值 和 " + it(mac) + "魔法值"
			}
		case 1:
			if ac > 0 && mac == 0 {
				line2 = "立即恢复 " + it(ac) + "生命值"
			} else if mac > 0 && ac == 0 {
				line2 = "立即恢复" + it(mac) + "魔法值" // 原文无空格（:3989）
			} else {
				line2 = "立即恢复 " + it(ac) + "生命值 和 " + it(mac) + "魔法值"
			}
		case 3:
			if ac > 0 && mac == 0 {
				line2 = "立即恢复 " + it(ac) + "%生命值"
			} else if mac > 0 && ac == 0 {
				line2 = "立即恢复 " + it(mac) + "%魔法值"
			} else {
				line2 = "立即恢复 " + it(ac) + "%生命值 和 " + it(mac) + "%魔法值"
			}
		}
	case def.StdMode >= 1 && def.StdMode <= 3: // 食物/杂项/特殊消耗品（:4003-4012）
		line1 = "重量" + it(int(def.Weight))
		if def.StdMode == 3 && def.Shape == 13 {
			line2 = "授予用户 " + it(int(item.DuraMax)) + " 点经验值"
		}
	case def.StdMode == 4: // 技能书（:4013-4035）
		line1 = " 重量:" + it(int(def.Weight))
		line3 = "需要等级: " + it(int(def.NeedLevel))
		useable = false
		switch def.Shape {
		case 0:
			line2 = "武士秘籍"
			useable = gs.Job == 0 && gs.Level >= int(def.NeedLevel)
		case 1:
			line2 = "法师秘籍"
			useable = gs.Job == 1 && gs.Level >= int(def.NeedLevel)
		case 2:
			line2 = "道士秘籍"
			useable = gs.Job == 2 && gs.Level >= int(def.NeedLevel)
		}
	case def.StdMode == 5 || def.StdMode == 6: // 武器（:4036-4154）
		useable = false
		if def.Reserved&1 != 0 {
			name = "(*)" + name
		}
		line1 = " 重量:" + it(int(def.Weight)) + " 持久力:" + duraStr(item.Dura, item.DuraMax)
		if def.DC > 0 || def.DCMax > 0 {
			line2 += "攻击:" + it(int(def.DC)) + "-" + it(int(def.DCMax)) + " "
		}
		if def.MC > 0 || def.MCMax > 0 {
			line2 += "魔法:" + it(int(def.MC)) + "-" + it(int(def.MCMax)) + " "
		}
		if def.SC > 0 || def.SCMax > 0 {
			line2 += "道术:" + it(int(def.SC)) + "-" + it(int(def.SCMax)) + " "
		}
		src := int(def.Source)
		if src >= -50 && src <= -1 {
			line2 += "强度:+" + it(-src) + " "
		} else if src >= -100 && src <= -51 {
			line2 += "神圣:-" + it(-src-50) + " "
		}
		if def.ACMax > 0 {
			line3 += "准确:+" + it(int(def.ACMax)) + " "
		}
		hiMAC := int(def.MACMax)
		if hiMAC > 10 {
			line3 += "攻击速度:+" + it(hiMAC-10) + " "
		} else if hiMAC >= 1 {
			line3 += "攻击速度:-" + it(hiMAC) + " "
		}
		if def.AC > 0 {
			line3 += "幸运:+" + it(int(def.AC)) + " "
		}
		if def.MAC > 0 {
			line3 += "诅咒:+" + it(int(def.MAC)) + " "
		}
		if nl, ok := needLine(gs, def); nl != "" {
			line3 += nl
			useable = ok
		}
	case def.StdMode == 10 || def.StdMode == 11: // 衣服（:4155-4256）
		useable = false
		if def.Reserved&1 != 0 {
			name = "(*)" + name
		}
		line1 = " 重量:" + it(int(def.Weight)) + " 持久力:" + duraStr(item.Dura, item.DuraMax)
		if def.AC > 0 || def.ACMax > 0 {
			line2 += "防御:" + it(int(def.AC)) + "-" + it(int(def.ACMax)) + " "
		}
		if def.MAC > 0 || def.MACMax > 0 {
			line2 += "魔御:" + it(int(def.MAC)) + "-" + it(int(def.MACMax)) + " "
		}
		if def.DC > 0 || def.DCMax > 0 {
			line2 += "攻击:" + it(int(def.DC)) + "-" + it(int(def.DCMax)) + " "
		}
		if def.MC > 0 || def.MCMax > 0 {
			line2 += "魔法:" + it(int(def.MC)) + "-" + it(int(def.MCMax)) + " "
		}
		if def.SC > 0 || def.SCMax > 0 {
			line2 += "道术:" + it(int(def.SC)) + "-" + it(int(def.SCMax))
		}
		src := uint16(def.Source) // 按字节解释（:4173-4176）
		if lo := int(src & 0xFF); lo > 0 {
			line3 += "幸运:+" + it(lo) + " "
		}
		if hi := int(src >> 8); hi > 0 {
			line3 += "诅咒:+" + it(hi) + " "
		}
		if nl, ok := needLine(gs, def); nl != "" {
			line3 += nl
			useable = ok
		}
	case def.StdMode == 15 || (def.StdMode >= 19 && def.StdMode <= 24) ||
		def.StdMode == 26 || def.StdMode == 51 || def.StdMode == 52 ||
		def.StdMode == 53 || def.StdMode == 54 || def.StdMode == 62 ||
		def.StdMode == 63 || def.StdMode == 64: // 首饰族（:4257-4421）
		useable = false
		if def.Reserved&1 != 0 {
			name = "(*)" + name
		}
		line1 = " 重量:" + it(int(def.Weight)) + " 持久:" + duraStr(item.Dura, item.DuraMax)
		switch def.StdMode {
		case 19, 53: // 项链
			if def.ACMax > 0 {
				line2 += "魔法躲避:+" + it(int(def.ACMax)) + "0% "
			}
			if def.MAC > 0 {
				line2 += "诅咒:+" + it(int(def.MAC)) + " "
			}
			if def.MACMax > 0 {
				line2 += "幸运:+" + it(int(def.MACMax)) + " "
			}
		case 20, 24, 52:
			if def.ACMax > 0 {
				line2 += "准确:+" + it(int(def.ACMax)) + " "
			}
			if def.MACMax > 0 {
				line2 += "敏捷:+" + it(int(def.MACMax)) + " "
			}
		case 21, 54: // 体力/魔法恢复项链
			if def.ACMax > 0 {
				line2 += "体力恢复:+" + it(int(def.ACMax)) + "0% "
			}
			if def.MACMax > 0 {
				line2 += "魔法恢复:+" + it(int(def.MACMax)) + "0% "
			}
			if def.AC > 0 {
				line2 += "攻击速度:+" + it(int(def.AC)) + " "
			}
			if def.MAC > 0 {
				line2 += "攻击速度:-" + it(int(def.MAC)) + " "
			}
		case 23: // 中毒戒指
			if def.ACMax > 0 {
				line2 += "毒物躲避:+" + it(int(def.ACMax)) + "0% "
			}
			if def.MACMax > 0 {
				line2 += "中毒恢复:+" + it(int(def.MACMax)) + "0% "
			}
			if def.AC > 0 {
				line2 += "攻击速度:+" + it(int(def.AC)) + " "
			}
			if def.MAC > 0 {
				line2 += "攻击速度:-" + it(int(def.MAC)) + " "
			}
		case 62: // 鞋（负重）
			if def.ACMax > 0 {
				line2 += "手执负重:+" + it(int(def.ACMax)) + " "
			}
			if def.MACMax > 0 {
				line2 += "装备负重:+" + it(int(def.MACMax)) + " "
			}
			if def.MAC > 0 {
				line2 += "背包负重:+" + it(int(def.MAC)) + " "
			}
		case 63: // 宝石
			if def.AC > 0 {
				line2 += "体力恢复:+" + it(int(def.AC)) + " "
			}
			if def.ACMax > 0 {
				line2 += "魔法恢复:+" + it(int(def.ACMax)) + " "
			}
			if def.MAC > 0 {
				line2 += "诅咒:+" + it(int(def.MAC)) + " "
			}
			if def.MACMax > 0 {
				line2 += "运气:+" + it(int(def.MACMax)) + " "
			}
		default: // 15/22/26/51/64：防御/魔御
			if def.AC > 0 || def.ACMax > 0 {
				line2 += "防御:" + it(int(def.AC)) + "-" + it(int(def.ACMax)) + " "
			}
			if def.MAC > 0 || def.MACMax > 0 {
				line2 += "魔御:" + it(int(def.MAC)) + "-" + it(int(def.MACMax)) + " "
			}
		}
		if def.DC > 0 || def.DCMax > 0 {
			line2 += "攻击:" + it(int(def.DC)) + "-" + it(int(def.DCMax)) + " "
		}
		if def.MC > 0 || def.MCMax > 0 {
			line2 += "魔法:" + it(int(def.MC)) + "-" + it(int(def.MCMax)) + " "
		}
		if def.SC > 0 || def.SCMax > 0 {
			line2 += "道术:" + it(int(def.SC)) + "-" + it(int(def.SCMax)) + " "
		}
		src := int(def.Source)
		if src >= -50 && src <= -1 {
			line2 += "幸运:+" + it(-src)
		} else if src >= -100 && src <= -51 {
			line2 += "诅咒1:-" + it(-src-50) // 原文即"诅咒1"（:4341-4342）
		}
		if nl, ok := needLine(gs, def); nl != "" {
			line3 += nl
			useable = ok
		}
	case def.StdMode == 25: // 护身符/毒药（:4422-4426）
		line1 = " 重量:" + it(int(def.Weight))
		line2 = "数量:" + it(delphiRound(int(item.Dura), 100)) + "/" + it(delphiRound(int(item.DuraMax), 100))
	case def.StdMode == 30: // 照明物（:4427-4430）
		line1 = " 重量:" + it(int(def.Weight)) + " 持久:" + duraStr(item.Dura, item.DuraMax)
	case def.StdMode == 40: // 肉（:4431-4434）
		line1 = " 重量:" + it(int(def.Weight)) + " 品质:" + duraStr(item.Dura, item.DuraMax)
	case def.StdMode == 42: // 药材（:4435-4438）
		line1 = " 重量:" + it(int(def.Weight)) + " 合成物品"
	case def.StdMode == 43: // 矿石（:4439-4442）
		line1 = " 重量:" + it(int(def.Weight)) + " 纯度:" + it(delphiRound(int(item.Dura), 1000))
	default: // 其余（含 StdMode 31 打包物品）：仅重量行（:4443-4445）
		line1 = " 重量:" + it(int(def.Weight))
	}

	parts := []string{name + line1}
	if line2 != "" {
		parts = append(parts, line2)
	}
	if line3 != "" {
		parts = append(parts, line3)
	}
	return strings.Join(parts, "\\"), useable
}
