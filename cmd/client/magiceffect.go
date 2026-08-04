package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/pyq0109/mirgo/internal/engine"
	"github.com/pyq0109/mirgo/internal/wil"
)

type MagicEffectType int

const (
	EffExplosion    MagicEffectType = 0
	EffFly          MagicEffectType = 1
	EffGround       MagicEffectType = 2
	EffFlyAxe       MagicEffectType = 3
	EffNone         MagicEffectType = 4 // 无特效（Delphi mtFireWind/mtKyulKai，PlayScn.pas:1573-1574, 1607-1609）
	EffFireGun      MagicEffectType = 5
	EffLightning    MagicEffectType = 6
	EffIce          MagicEffectType = 7
	EffBujaukExplo  MagicEffectType = 8
	EffBujaukGround MagicEffectType = 9
	EffFlyArrow     MagicEffectType = 11
	EffReady        MagicEffectType = 12
	EffThunder2     MagicEffectType = 13
	EffFlyBug       MagicEffectType = 14
)

// MagicEffectParams 每种特效类型的渲染参数，集中原散落在 main.go
// SMMagicFire 分发里的硬编码。数值出处：Delphi PlayScn.pas:1448-1647
// NewMagic 各 case 与 magiceff.pas 常量（FLYBASE=10、EXPLOSIONBASE=170、
// FLYOMAAXEBASE=447、ARCHERBASE=2607，magiceff.pas:16-22）。
// BaseIdx 为 -1 表示帧基址由 Add* 函数据 effNum 经 effectBase[effNum-1] 推导。
type MagicEffectParams struct {
	BaseIdx   int
	MaxFrame  int
	FrameTime int64
	Sound     int // 0 = 用默认 10000+magID*10+2
	Light     int // 光源级别（与 Add* 内部设置一致，集中备查）
	ImgLib    int // 0=Magic.wil 1=Magic2.wil（与 Add* 内部设置一致）
}

var magicEffectParams = map[MagicEffectType]MagicEffectParams{
	// 爆炸：基址 = effectBase[effNum-1]+170（magiceff.pas:17 EXPLOSIONBASE；
	// 特例 effNum 见 explosionByEffNum，PlayScn.pas:1459-1572）。
	EffExplosion: {BaseIdx: -1, MaxFrame: 10, FrameTime: 50, Light: 2},
	// 飞行：基址 = effectBase[effNum-1]+10（magiceff.pas:16 FLYBASE），
	// 飞行帧6（magiceff.pas:394）；effNum=39 寒冰掌特例见 flyByEffNum。
	EffFly: {BaseIdx: -1, MaxFrame: 6, FrameTime: 50, Light: 1},
	// 地面：基址 = effectBase[effNum-1]。
	EffGround: {BaseIdx: -1, MaxFrame: 10, FrameTime: 50, Light: 3},
	// 飞行斧：FLYOMAAXEBASE=447（magiceff.pas:20），3帧（magiceff.pas:531-538）。
	EffFlyAxe: {BaseIdx: 447, MaxFrame: 3, FrameTime: 50, Light: 1},
	// 无特效：仅播放音效，不创建特效。
	EffNone: {},
	// 火枪：TFireGunEffect(930)（PlayScn.pas:1575-1576），FIREGUNFRAME=6（magiceff.pas:26）。
	EffFireGun: {BaseIdx: 930, MaxFrame: 6, FrameTime: 50, Light: 2},
	// 疾光电影：TLightingThunder(970)（PlayScn.pas:1583-1584）。
	EffLightning: {BaseIdx: 970, MaxFrame: 10, FrameTime: 80, Light: 4},
	// 雷电术：TThuderEffect(10) + Magic2，6帧（PlayScn.pas:1577-1582）。
	EffIce: {BaseIdx: 10, MaxFrame: 6, FrameTime: 80, Light: 2, ImgLib: 1},
	// 符咒爆炸：TExploBujaukEffect(1160)（PlayScn.pas:1585-1596）。
	EffBujaukExplo: {BaseIdx: 1160, MaxFrame: 10, FrameTime: 80, Light: 2},
	// 符咒地面：TBujaukGroundEffect(1160)（PlayScn.pas:1598-1606）。
	EffBujaukGround: {BaseIdx: 1160, MaxFrame: 10, FrameTime: 80, Light: 3},
	// 飞行箭：ARCHERBASE=2607（magiceff.pas:22），1帧（magiceff.pas:539-544）。
	EffFlyArrow: {BaseIdx: 2607, MaxFrame: 1, FrameTime: 50, Light: 0},
	// 蓄力：基址 = effectBase[effNum-1]，MG_READY=10帧（magiceff.pas:11）。
	EffReady: {BaseIdx: -1, MaxFrame: 10, FrameTime: 50, Light: 1},
	// 雷电2（Delphi mt14）：TThuderEffect(140) + Magic2（PlayScn.pas:1634-1638）。
	EffThunder2: {BaseIdx: 140, MaxFrame: 6, FrameTime: 80, Light: 2, ImgLib: 1},
	// 飞行虫（Delphi mt15）：TFlyingBug，FlyImageBase=FLYOMAAXEBASE=447
	//（PlayScn.pas:1639-1643, magiceff.pas:1455-1459）。
	EffFlyBug: {BaseIdx: 447, MaxFrame: 3, FrameTime: 50, Light: 1},
}

// explosionByEffNum 爆炸特效按动画号的特例参数（Delphi mtExplosion 分支中
// 覆盖 MagExplosionBase 的 case，PlayScn.pas:1459-1572）。imgLib: 1=Magic2
// （PlayScn 中 meff.ImgLib := wimg 的分支，GetEffectBase 返回 Magic2）。
var explosionByEffNum = map[int]struct {
	base, frames  int
	frameTime     int64
	light, imgLib int
}{
	18: {1570, 10, 80, 2, 0},  // 诱惑之光（PlayScn.pas:1461-1466）
	21: {1660, 20, 80, 3, 0},  // 爆裂火焰（PlayScn.pas:1467-1474）
	26: {3990, 10, 80, 2, 0},  // 心灵启示（PlayScn.pas:1475-1482）
	27: {1800, 10, 80, 3, 0},  // 群体治愈术（PlayScn.pas:1483-1490）
	30: {3930, 16, 80, 3, 0},  // 圣言术（PlayScn.pas:1491-1498）
	31: {3850, 20, 80, 3, 0},  // 冰咆哮（PlayScn.pas:1499-1506）
	34: {140, 20, 80, 3, 1},   // Magic2（PlayScn.pas:1507-1516）
	40: {620, 20, 100, 3, 1},  // 净化术，Magic2（PlayScn.pas:1517-1526）
	45: {920, 20, 100, 3, 1},  // Magic2（PlayScn.pas:1527-1536）
	47: {1010, 20, 100, 3, 1}, // 火龙气焰，Magic2（PlayScn.pas:1537-1546）
	48: {1060, 40, 50, 3, 1},  // Magic2（PlayScn.pas:1547-1556）
	49: {1110, 10, 100, 3, 1}, // Magic2（PlayScn.pas:1557-1566）
}

// flyByEffNum 飞行特效按动画号的特例参数：寒冰掌 effNum=39 飞行4帧、
// 爆炸8帧、使用 Magic2（PlayScn.pas:1452-1456, magiceff.pas:400-403）。
var flyByEffNum = map[int]struct {
	frames, explosionFrames, imgLib int
}{
	39: {4, 8, 1},
}

type LightSource struct {
	X, Y  float64
	Level int
}

type MagicEffect struct {
	Type             MagicEffectType
	X, Y             float64
	StartX, StartY   float64
	TargetX, TargetY float64
	Frame            int
	MaxFrame         int
	FrameTime        int64
	LastTick         int64
	BaseIdx          int
	ImgLib           int
	Done             bool
	Light            int
	Dir16            int
	ExplosionBase    int
	ExplosionFrames  int // 0 = 默认10帧（Delphi ExplosionFrame 可被特例覆盖）
	Exploding        bool
}

func flyDir16(sx, sy, tx, ty float64) int {
	fx := tx - sx
	fy := ty - sy
	if fx == 0 {
		if fy < 0 {
			return 0
		}
		return 8
	}
	if fy == 0 {
		if fx < 0 {
			return 12
		}
		return 4
	}
	switch {
	case fx > 0 && fy < 0:
		r := 4
		if -fy > fx/4 {
			r = 3
		}
		if -fy > fx/1.9 {
			r = 2
		}
		if -fy > fx*1.4 {
			r = 1
		}
		if -fy > fx*4 {
			r = 0
		}
		return r
	case fx > 0 && fy > 0:
		r := 4
		if fy > fx/4 {
			r = 5
		}
		if fy > fx/1.9 {
			r = 6
		}
		if fy > fx*1.4 {
			r = 7
		}
		if fy > fx*4 {
			r = 8
		}
		return r
	case fx < 0 && fy > 0:
		r := 12
		if fy > -fx/4 {
			r = 11
		}
		if fy > -fx/1.9 {
			r = 10
		}
		if fy > -fx*1.4 {
			r = 9
		}
		if fy > -fx*4 {
			r = 8
		}
		return r
	default:
		r := 12
		if -fy > -fx/4 {
			r = 13
		}
		if -fy > -fx/1.9 {
			r = 14
		}
		if -fy > -fx*1.4 {
			r = 15
		}
		if -fy > -fx*4 {
			r = 0
		}
		return r
	}
}

type EffectManager struct {
	effects []*MagicEffect
}

func NewEffectManager() *EffectManager {
	return &EffectManager{}
}

// AddExplosion 在目标点播放爆炸动画。effNum 为魔法 EffectNumber，
// 帧基址 = EffectBase[effNum-1] + EXPLOSIONBASE(170)；特例动画号
// （explosionByEffNum）改用 Delphi MagExplosionBase 覆盖值。
func (em *EffectManager) AddExplosion(x, y float64, effNum, maxFrame int, frameTime int64) {
	base := effectBaseIdx(effNum-1) + 170
	light := magicEffectParams[EffExplosion].Light
	imgLib := 0
	if ov, ok := explosionByEffNum[effNum]; ok {
		base, maxFrame, frameTime = ov.base, ov.frames, ov.frameTime
		light, imgLib = ov.light, ov.imgLib
	}
	em.effects = append(em.effects, &MagicEffect{
		Type: EffExplosion, X: x, Y: y,
		BaseIdx: base, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: light, ImgLib: imgLib,
	})
}

// AddFly 发射飞行弹道。effNum 为魔法 EffectNumber，
// 飞行帧基址 = EffectBase[effNum-1] + FLYBASE(10)，到达后切换爆炸；
// 特例动画号（flyByEffNum，如寒冰掌39）覆盖帧数/图库（PlayScn.pas:1452-1456）。
func (em *EffectManager) AddFly(sx, sy, tx, ty float64, effNum, maxFrame int, frameTime int64) {
	base := effectBaseIdx(effNum - 1)
	imgLib := 0
	explFrames := 0
	if ov, ok := flyByEffNum[effNum]; ok {
		maxFrame, imgLib, explFrames = ov.frames, ov.imgLib, ov.explosionFrames
	}
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFly, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: base + 10, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1, ImgLib: imgLib,
		Dir16: flyDir16(sx, sy, tx, ty), ExplosionBase: base + 170, ExplosionFrames: explFrames,
	})
}

// AddGround 在目标点播放地面效果。帧基址 = EffectBase[effNum-1]。
func (em *EffectManager) AddGround(x, y float64, effNum, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffGround, X: x, Y: y,
		BaseIdx: effectBaseIdx(effNum - 1), MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 3,
	})
}

func (em *EffectManager) AddLightning(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffLightning, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 4,
	})
}

func (em *EffectManager) AddFireGun(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFireGun, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) AddIce(x, y float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffIce, X: x, Y: y,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2, ImgLib: 1,
	})
}

func (em *EffectManager) AddFlyAxe(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyAxe, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) AddFlyArrow(sx, sy, tx, ty float64, baseIdx, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyArrow, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: baseIdx, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 0,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

// AddReady 在施法者位置播放蓄力动画。帧基址 = EffectBase[effNum-1]。
func (em *EffectManager) AddReady(x, y float64, effNum, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffReady, X: x, Y: y,
		BaseIdx: effectBaseIdx(effNum - 1), MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
	})
}

func (em *EffectManager) AddBujaukExplo(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffBujaukExplo, X: x, Y: y,
		BaseIdx: 1160, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2,
	})
}

func (em *EffectManager) AddBujaukGround(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffBujaukGround, X: x, Y: y,
		BaseIdx: 1160, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 3,
	})
}

func (em *EffectManager) AddThunder2(x, y float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffThunder2, X: x, Y: y,
		BaseIdx: 140, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 2, ImgLib: 1,
	})
}

func (em *EffectManager) AddFlyBug(sx, sy, tx, ty float64, maxFrame int, frameTime int64) {
	em.effects = append(em.effects, &MagicEffect{
		Type: EffFlyBug, X: sx, Y: sy, StartX: sx, StartY: sy, TargetX: tx, TargetY: ty,
		BaseIdx: 447, MaxFrame: maxFrame, FrameTime: frameTime,
		LastTick: time.Now().UnixMilli(), Light: 1,
		Dir16: flyDir16(sx, sy, tx, ty),
	})
}

func (em *EffectManager) Update(now int64) {
	alive := em.effects[:0]
	for _, eff := range em.effects {
		if now-eff.LastTick >= eff.FrameTime {
			eff.LastTick = now
			eff.Frame++
			switch eff.Type {
			case EffFly, EffFlyAxe, EffFlyArrow, EffFireGun, EffFlyBug:
				if !eff.Exploding && eff.MaxFrame > 0 {
					t := float64(eff.Frame) / float64(eff.MaxFrame)
					if t >= 1.0 {
						t = 1.0
						if eff.ExplosionBase > 0 {
							eff.Exploding = true
							eff.Frame = 0
							eff.MaxFrame = 10
							if eff.ExplosionFrames > 0 {
								eff.MaxFrame = eff.ExplosionFrames
							}
							eff.BaseIdx = eff.ExplosionBase
							break
						}
					}
					eff.X = eff.StartX + (eff.TargetX-eff.StartX)*t
					eff.Y = eff.StartY + (eff.TargetY-eff.StartY)*t
				}
			}
		}
		if eff.Frame >= eff.MaxFrame {
			eff.Done = true
		}
		if !eff.Done {
			alive = append(alive, eff)
		}
	}
	em.effects = alive
}

// isGroundEffect 判断地面特效（Delphi m_GroundEffectList，画在高前景物件
// 之前，PlayScn.pas:1110-1118）。
func isGroundEffect(eff *MagicEffect) bool {
	return eff.Type == EffGround || eff.Type == EffBujaukGround
}

// isFlyEffect 判断飞行物（Delphi m_FlyList，参与逐行 Y 排序，
// PlayScn.pas:1241-1245）。FireGun 属 m_EffectList 晚层，不在此列。
func isFlyEffect(eff *MagicEffect) bool {
	switch eff.Type {
	case EffFly, EffFlyAxe, EffFlyArrow, EffFlyBug:
		return true
	}
	return false
}

// drawEffect 绘制单个特效（帧索引计算 + 取图 + 混合绘制）。
func (em *EffectManager) drawEffect(eff *MagicEffect, glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32) {
	var idx int
	switch {
	case eff.Exploding:
		idx = eff.BaseIdx + eff.Frame
	case eff.Type == EffFlyAxe || eff.Type == EffFlyBug:
		idx = eff.BaseIdx + eff.Dir16*10 + eff.Frame
	case eff.Type == EffFly && !eff.Exploding:
		idx = eff.BaseIdx + eff.Dir16*10 + eff.Frame
	case eff.Type == EffFlyArrow:
		idx = eff.BaseIdx + eff.Dir16 + eff.Frame
	default:
		idx = eff.BaseIdx + eff.Frame
	}

	var wilFile *wil.File
	if eff.ImgLib == 0 {
		wilFile = resources.Magic
	} else {
		wilFile = resources.Magic2
	}
	if wilFile == nil || idx < 0 || idx >= wilFile.Count {
		return
	}

	img := wilFile.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return
	}

	tex := resources.GetTexture(wilFile, idx)
	if tex == 0 {
		return
	}

	w := float32(img.Width)
	h := float32(img.Height)
	drawX := float32(eff.X) - w/2
	drawY := float32(eff.Y) - h/2

	switch eff.Type {
	case EffFlyAxe, EffFlyArrow, EffFlyBug:
		if !eff.Exploding {
			glState.DrawQuad(tex, drawX, drawY, w, h, proj)
		} else {
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
			glState.DrawQuad(tex, drawX, drawY, w, h, proj)
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		}
	default:
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		glState.DrawQuad(tex, drawX, drawY, w, h, proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}
}

// RenderGround 绘制地面特效（画在小前景之后、大前景/角色之前，
// PlayScn.pas:1110-1118）。
func (em *EffectManager) RenderGround(glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32) {
	for _, eff := range em.effects {
		if isGroundEffect(eff) {
			em.drawEffect(eff, glState, resources, proj)
		}
	}
}

// RenderFlyRow 仅绘制瓦片行等于 row 的飞行物特效（参与 Y 排序，
// PlayScn.pas:1241-1245）。飞行物坐标为像素，行号 = floor(Y/TileHeight)。
func (em *EffectManager) RenderFlyRow(glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32, row int) {
	for _, eff := range em.effects {
		if isFlyEffect(eff) && int(eff.Y/engine.TileHeight) == row {
			em.drawEffect(eff, glState, resources, proj)
		}
	}
}

// Render 绘制晚层特效（爆炸类等，画在角色之后，Delphi m_EffectList.DrawEff，
// PlayScn.pas:1328-1335）。地面特效与飞行物已在前面的层绘制，此处跳过。
func (em *EffectManager) Render(glState *engine.GLState, resources *engine.ResourceManager, proj [16]float32) {
	for _, eff := range em.effects {
		if isGroundEffect(eff) || isFlyEffect(eff) {
			continue
		}
		em.drawEffect(eff, glState, resources, proj)
	}
}

func (em *EffectManager) LightSources() []LightSource {
	var lights []LightSource
	for _, eff := range em.effects {
		if eff.Light > 0 {
			lights = append(lights, LightSource{X: eff.X, Y: eff.Y, Level: eff.Light})
		}
	}
	return lights
}
