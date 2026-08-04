package main

import (
	"image/color"
)

// renderMinimap 绘制 mmap.wil 预渲染小地图与角色标记。
// Delphi TPlayScene.DrawMiniMap 移植（PlayScn.pas:791-842）：
// 以玩家为中心裁剪 120×120 视口，1:1 贴到屏幕右上角 (680,0)；
// 小地图图上 X 方向每格 1.5 像素、Y 方向每格 1 像素；
// 视口钳制到图像边界（玩家贴边时偏离中心），不做越界填充。
func (s *PlayScene) renderMinimap(proj [16]float32) {
	if s.minimapLv == 0 || s.State.MySelf == nil {
		return
	}
	idx := s.State.MinimapIndex
	if idx < 0 || s.resources.Mmap == nil {
		return
	}
	img := s.resources.Mmap.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return
	}
	tex := s.resources.GetTexture(s.resources.Mmap, idx)
	if tex == 0 {
		return
	}

	// 玩家在小地图图上的像素位置（PlayScn.pas:808-809）。
	mx := s.State.MySelf.CurrX * 48 / 32
	my := s.State.MySelf.CurrY

	// 视口矩形，钳制到图像边界（PlayScn.pas:810-813）。
	left := mx - 60
	if left < 0 {
		left = 0
	}
	top := my - 60
	if top < 0 {
		top = 0
	}
	right := left + 120
	if right > img.Width {
		right = img.Width
	}
	bottom := top + 120
	if bottom > img.Height {
		bottom = img.Height
	}
	w := right - left
	h := bottom - top
	if w <= 0 || h <= 0 {
		return
	}

	alpha := float32(1)
	if s.minimapLv == 1 {
		// Lv=1 半透明：近似 DrawBlendEx 的 50% 调色板混合（PlayScn.pas:815-816）。
		alpha = 0.5
	}
	s.gl.DrawQuadSub(tex, float32(img.Width), float32(img.Height),
		float32(left), float32(top), float32(w), float32(h),
		ScreenWidth-120, 0, float32(w), float32(h),
		1, 1, 1, alpha, proj)

	// 标记与图像同步闪烁：灭帧只画底图（PlayScn.pas:819）。
	if !s.mmBlinkOn {
		return
	}

	selfColor, monsterColor, animalColor := s.minimapMarkerColors()

	// 自己：1×1 白点，调色板 255（PlayScn.pas:820-822）。
	s.gl.DrawQuadColor(ScreenWidth-120+float32(mx-left), float32(my-top),
		1, 1, selfColor[0], selfColor[1], selfColor[2], 1, proj)

	// 其他角色：仅玩家周围 ±10 格，2×2 色块（PlayScn.pas:824-838）。
	selfX := s.State.MySelf.CurrX
	selfY := s.State.MySelf.CurrY
	for _, a := range s.State.Actors.All() {
		if a == nil || a.IsSelf || a.Death {
			continue
		}
		dx := a.CurrX - selfX
		dy := a.CurrY - selfY
		if dx < -10 || dx > 10 || dy < -10 || dy > 10 {
			continue
		}
		c := monsterColor
		if a.Type == ActorHuman {
			// race 0（其他玩家）→ 调色板 255。
			c = selfColor
		} else if a.Race == 50 || a.Race == 45 || a.Race == 12 {
			// Delphi PlayScn.pas:831-835：race 50/45/12（动物/守卫）→ 调色板 218
			c = animalColor
		}
		s.gl.DrawQuadColor(ScreenWidth-120+float32(a.CurrX*48/32-left), float32(a.CurrY-top),
			2, 2, c[0], c[1], c[2], 1, proj)
	}
}

// minimapMarkerColors 从 Prguse.wil 屏幕调色板取标记色（ClMain.pas:996
// 将 Prguse 的 MainPalette 设为全屏调色板）：
// 255 = 自己/其他玩家（白），249 = 怪物/NPC，218 = race 50/45/12 动物/守卫。
func (s *PlayScene) minimapMarkerColors() (self, monster, animal [3]float32) {
	self = [3]float32{1, 1, 1}
	monster = [3]float32{1, 0, 0}
	animal = [3]float32{0, 1, 0}
	if s.resources.Prguse != nil {
		self = paletteRGB(s.resources.Prguse.Palette[255])
		monster = paletteRGB(s.resources.Prguse.Palette[249])
		animal = paletteRGB(s.resources.Prguse.Palette[218])
	}
	return
}

func paletteRGB(c color.RGBA) [3]float32 {
	return [3]float32{float32(c.R) / 255, float32(c.G) / 255, float32(c.B) / 255}
}
