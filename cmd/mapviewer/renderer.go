package main

import (
	"fmt"
	"path/filepath"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/pyq0109/mirgo/internal/mapformat"
	"github.com/pyq0109/mirgo/internal/wil"
)

const (
	cullMargin      = 3
	frontCullMargin = 20
)

// GLRenderer 使用 OpenGL 渲染地图。
type GLRenderer struct {
	Tiles   *wil.File
	SmTiles *wil.File
	Objects *wil.File // area 0 (Objects.wil)

	glState    *GLState
	dataDir    string
	texCache   map[int]uint32 // Tiles.wil 图片索引 -> GL 纹理
	smTexCache map[int]uint32 // SmTiles.wil 图片索引 -> GL 纹理

	// Area 系统：懒加载 Objects{N+1}.wil 及其纹理缓存
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int

	// 格子高亮状态（由主循环设置）
	HighlightX, HighlightY int // 悬停格子（-1 = 无）
	LockedX, LockedY       int // 锁定格子（-1 = 无）
}

// NewGLRenderer 创建带 OpenGL 状态的渲染器。
func NewGLRenderer(tiles, smTiles, objects *wil.File, dataDir string, glState *GLState) *GLRenderer {
	r := &GLRenderer{
		Tiles:          tiles,
		SmTiles:        smTiles,
		Objects:        objects,
		glState:        glState,
		dataDir:        dataDir,
		texCache:       make(map[int]uint32),
		smTexCache:     make(map[int]uint32),
		objectsLoaders: make(map[int]*wil.File),
		objectsCaches:  make(map[int]map[int]uint32),
		HighlightX:     -1,
		HighlightY:     -1,
		LockedX:        -1,
		LockedY:        -1,
	}
	if objects != nil {
		r.objectsLoaders[0] = objects
		r.objectsCaches[0] = make(map[int]uint32)
	}
	return r
}

// getObjectsLoader 返回指定 area 的 WIL 加载器，需要时懒加载。
// Area 0 = Objects.wil, Area N = Objects{N+1}.wil。
// 对应 C++ MapRenderer::GetObjectsLoader。
func (r *GLRenderer) getObjectsLoader(area int) *wil.File {
	if f, ok := r.objectsLoaders[area]; ok {
		return f
	}
	if area == 0 {
		return r.Objects
	}
	filename := fmt.Sprintf("Objects%d.wil", area+1)
	wilPath := filepath.Join(r.dataDir, filename)
	f, err := wil.Load(wilPath)
	if err != nil {
		r.objectsLoaders[area] = nil
		return nil
	}
	r.objectsLoaders[area] = f
	r.objectsCaches[area] = make(map[int]uint32)
	return f
}

func (r *GLRenderer) getTex(cache map[int]uint32, file *wil.File, idx int) uint32 {
	if idx < 0 || file == nil || idx >= file.Count {
		return 0
	}
	if tex, ok := cache[idx]; ok {
		return tex
	}
	img := file.GetImage(idx)
	if img == nil || img.RGBA == nil {
		return 0
	}
	tex := UploadTexture(img.RGBA)
	cache[idx] = tex
	file.ReleasePixels(idx) // 释放 Go 侧像素；GPU 已有自己的副本
	return tex
}

// Render 绘制地图可见部分。
// 渲染顺序对应 C++ MapRenderer::Render:
// 背景 -> 中间 -> 前景(普通) -> 前景(混合) -> 地图边界 -> 格子高亮 -> 锁定高亮 -> 网格
func (r *GLRenderer) Render(m *mapformat.MapData, cam *Camera2D, showBack, showMid, showFront, showCollision, showGrid bool) {
	// 重新设置 ImGui 可能修改过的 GL 状态
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// 投影：Y 轴向下的正交投影
	left := float32(cam.X)
	top := float32(cam.Y)
	right := float32(cam.X + float64(cam.ViewW)/cam.Zoom)
	bottom := float32(cam.Y + float64(cam.ViewH)/cam.Zoom)
	proj := OrthoProj(left, right, bottom, top)

	gl.UseProgram(r.glState.Shader.ID)
	gl.Uniform1i(r.glState.Shader.TexLoc, 0)

	// 背景/中间层裁剪范围
	startX, startY, endX, endY := cam.ViewportTiles(cullMargin, cullMargin)
	startX = clamp(startX, 0, m.Width-1)
	startY = clamp(startY, 0, m.Height-1)
	endX = clamp(endX, 0, m.Width-1)
	endY = clamp(endY, 0, m.Height-1)

	// 前景层裁剪范围（更大边距以容纳高物体）
	fStartX, fStartY, fEndX, fEndY := cam.ViewportTiles(frontCullMargin, frontCullMargin)
	fStartX = clamp(fStartX, 0, m.Width-1)
	fStartY = clamp(fStartY, 0, m.Height-1)
	fEndX = clamp(fEndX, 0, m.Width-1)
	fEndY = clamp(fEndY, 0, m.Height-1)

	// 对齐到偶数，用于背景层步长 2 渲染
	bStartX, bStartY, bEndX, bEndY := startX, startY, endX, endY
	if bStartX%2 == 1 {
		bStartX--
	}
	if bStartY%2 == 1 {
		bStartY--
	}
	if bEndX%2 == 1 {
		bEndX++
	}
	if bEndY%2 == 1 {
		bEndY++
	}
	bStartX = clamp(bStartX, 0, m.Width-1)
	bStartY = clamp(bStartY, 0, m.Height-1)
	bEndX = clamp(bEndX, 0, m.Width-1)
	bEndY = clamp(bEndY, 0, m.Height-1)

	// 1. 背景层：偶数 x, y（2x2 格子块）
	if showBack {
		for y := bStartY; y <= bEndY; y += 2 {
			for x := bStartX; x <= bEndX; x += 2 {
				info := m.InfoAt(x, y)
				if info.BackLib < 0 || info.BackImage < 0 {
					continue
				}
				tex := r.getTex(r.texCache, r.Tiles, info.BackImage)
				if tex == 0 {
					continue
				}
				img := r.Tiles.GetImage(info.BackImage)
				wx := float32(x * TileWidth)
				wy := float32(y * TileHeight)
				r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, false, proj)
			}
		}
	}

	// 2. 中间层：所有格子
	if showMid {
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				info := m.InfoAt(x, y)
				if info.MiddleLib < 0 || info.MiddleImage < 0 {
					continue
				}
				tex := r.getTex(r.smTexCache, r.SmTiles, info.MiddleImage)
				if tex == 0 {
					continue
				}
				img := r.SmTiles.GetImage(info.MiddleImage)
				wx := float32(x * TileWidth)
				wy := float32(y * TileHeight)
				r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, false, proj)
			}
		}
	}

	// 3. 前景层 — 单遍渲染，逐格子切换混合模式（对应 C++）
	if showFront {
		for y := fStartY; y <= fEndY; y++ {
			for x := fStartX; x <= fEndX; x++ {
				info := m.InfoAt(x, y)
				r.drawFront(info, x, y, proj)
			}
		}
		r.animCounter++
	}

	// 4. 碰撞叠加层
	if showCollision {
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				if m.IsCollision(x, y) {
					wx := float32(x * TileWidth)
					wy := float32(y * TileHeight)
					r.glState.DrawQuadColor(wx, wy, TileWidth, TileHeight, 1, 0, 0, 0.3, proj)
				}
			}
		}
	}

	// 5. 叠加层（必须在所有格子层之后渲染）
	r.drawMapBorder(cam, m, proj)
	r.drawTileHighlight(m, proj)
	r.drawLockedTileHighlight(m, proj)
	if showGrid {
		r.drawGrid(cam, startX, startY, endX, endY, proj)
	}
}

// drawFront 渲染单个前景层格子。
// 对应 C++ MapRenderer::RenderFrontLayer + DrawTile（layer 2）。
func (r *GLRenderer) drawFront(info *mapformat.CellInfo, x, y int, proj [16]float32) {
	if info.FrontLib < 0 {
		return
	}

	area := int(info.FrontArea)
	loader := r.getObjectsLoader(area)
	if loader == nil {
		return
	}
	cache := r.objectsCaches[area]

	idx := info.FrontImage
	isBlend := info.FrontAniFrame&0x80 != 0

	// 动画
	ani := int(info.FrontAniFrame & 0x7F)
	if ani > 0 {
		tick := int(info.FrontAniTick)
		if tick < 1 {
			tick = 1
		}
		cycleLen := ani + ani*tick
		if cycleLen > 0 {
			frame := (r.animCounter % cycleLen) / (1 + tick)
			idx += frame
		}
	}

	// 门偏移（C++ RenderFrontLayer 456-460 行）
	if info.FrontDoorOffset&0x80 != 0 {
		if info.FrontDoorIndex&0x7F != 0 {
			idx += int(info.FrontDoorOffset & 0x7F)
		}
	}

	if idx < 0 || idx >= loader.Count {
		return
	}

	tex := r.getTex(cache, loader, idx)
	if tex == 0 {
		return
	}
	img := loader.GetImage(idx)

	cellWorldX := float32(x * TileWidth)
	cellWorldY := float32(y * TileHeight)

	if isBlend {
		// 混合物体（火焰、光效）：基于热点定位 + 加法混合
		// Delphi 公式: (n + ax - 2, m + ay - 68)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		wx := cellWorldX + float32(img.HotX) - 2
		wy := cellWorldY + float32(img.HotY) - 68
		r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, false, proj)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	} else {
		// 非混合物体：底部对齐定位
		wx := cellWorldX
		wy := cellWorldY - float32(img.Height) + TileHeight
		r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, false, proj)
	}
}

// drawGrid 用批量线段绘制渲染格子网格叠加层。
// 对应 C++ MapRenderer::RenderGrid。
func (r *GLRenderer) drawGrid(cam *Camera2D, startX, startY, endX, endY int, proj [16]float32) {
	gl.UseProgram(r.glState.GridShader.ID)
	gl.UniformMatrix4fv(r.glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(r.glState.GridShader.ColorLoc, 0.5, 0.5, 0.5, 0.3)
	gl.BindVertexArray(r.glState.GridVAO)

	// 将所有线段顶点构建为一个批次
	lines := make([]float32, 0, ((endX-startX+2)+(endY-startY+2))*4)

	// 竖线
	for x := startX; x <= endX+1; x++ {
		wx := float32(x * TileWidth)
		wy0 := float32(startY * TileHeight)
		wy1 := float32((endY + 1) * TileHeight)
		lines = append(lines, wx, wy0, wx, wy1)
	}
	// 横线
	for y := startY; y <= endY+1; y++ {
		wy := float32(y * TileHeight)
		wx0 := float32(startX * TileWidth)
		wx1 := float32((endX + 1) * TileWidth)
		lines = append(lines, wx0, wy, wx1, wy)
	}

	vertexCount := int32(len(lines) / 2)
	if vertexCount == 0 {
		gl.BindVertexArray(0)
		return
	}

	gl.BindBuffer(gl.ARRAY_BUFFER, r.glState.GridVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(lines)*4, unsafe.Pointer(&lines[0]), gl.STREAM_DRAW)
	gl.DrawArrays(gl.LINES, 0, vertexCount)
	gl.BindVertexArray(0)
}

// drawMapBorder 在地图边界绘制蓝色矩形。
// 对应 C++ MapRenderer::RenderMapBorder。
func (r *GLRenderer) drawMapBorder(cam *Camera2D, m *mapformat.MapData, proj [16]float32) {
	mapW := float32(m.Width * TileWidth)
	mapH := float32(m.Height * TileHeight)

	lines := []float32{
		0, 0, mapW, 0,
		mapW, 0, mapW, mapH,
		mapW, mapH, 0, mapH,
		0, mapH, 0, 0,
	}

	gl.UseProgram(r.glState.GridShader.ID)
	gl.UniformMatrix4fv(r.glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(r.glState.GridShader.ColorLoc, 0.2, 0.5, 1.0, 1.0)
	gl.BindVertexArray(r.glState.GridVAO)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.glState.GridVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(lines)*4, unsafe.Pointer(&lines[0]), gl.STREAM_DRAW)
	gl.DrawArrays(gl.LINES, 0, 8)
	gl.BindVertexArray(0)
}

// drawTileHighlight 在鼠标悬停的格子周围绘制白色矩形。
// 对应 C++ MapRenderer::RenderTileHighlight。
func (r *GLRenderer) drawTileHighlight(m *mapformat.MapData, proj [16]float32) {
	if r.HighlightX < 0 || r.HighlightY < 0 {
		return
	}
	if r.HighlightX >= m.Width || r.HighlightY >= m.Height {
		return
	}
	r.drawRect(float32(r.HighlightX*TileWidth), float32(r.HighlightY*TileHeight),
		TileWidth, TileHeight, 1, 1, 1, 0.8, proj)
}

// drawLockedTileHighlight 在锁定格子周围绘制红色矩形。
// 对应 C++ MapRenderer::RenderLockedTileHighlight。
func (r *GLRenderer) drawLockedTileHighlight(m *mapformat.MapData, proj [16]float32) {
	if r.LockedX < 0 || r.LockedY < 0 {
		return
	}
	if r.LockedX >= m.Width || r.LockedY >= m.Height {
		return
	}
	r.drawRect(float32(r.LockedX*TileWidth), float32(r.LockedY*TileHeight),
		TileWidth, TileHeight, 1, 0.3, 0.3, 1.0, proj)
}

// drawRect 用网格着色器绘制矩形边框。
func (r *GLRenderer) drawRect(x, y, w, h float32, red, green, blue, alpha float32, proj [16]float32) {
	lines := []float32{
		x, y, x + w, y,
		x + w, y, x + w, y + h,
		x + w, y + h, x, y + h,
		x, y + h, x, y,
	}

	gl.UseProgram(r.glState.GridShader.ID)
	gl.UniformMatrix4fv(r.glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(r.glState.GridShader.ColorLoc, red, green, blue, alpha)
	gl.BindVertexArray(r.glState.GridVAO)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.glState.GridVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(lines)*4, unsafe.Pointer(&lines[0]), gl.STREAM_DRAW)
	gl.DrawArrays(gl.LINES, 0, 8)
	gl.BindVertexArray(0)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// Destroy 释放渲染器持有的所有 GL 资源。
func (r *GLRenderer) Destroy() {
	for _, tex := range r.texCache {
		gl.DeleteTextures(1, &tex)
	}
	for _, tex := range r.smTexCache {
		gl.DeleteTextures(1, &tex)
	}
	for _, cache := range r.objectsCaches {
		for _, tex := range cache {
			gl.DeleteTextures(1, &tex)
		}
	}
}
