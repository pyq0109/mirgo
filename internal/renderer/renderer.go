package renderer

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

// GLRenderer renders the map using OpenGL.
type GLRenderer struct {
	Tiles   *wil.File
	SmTiles *wil.File
	Objects *wil.File // area 0 (Objects.wil)

	glState    *GLState
	dataDir    string
	texCache   map[int]uint32 // Tiles.wil image index -> GL texture
	smTexCache map[int]uint32 // SmTiles.wil image index -> GL texture

	// Area system: lazy-loaded Objects{N+1}.wil files and their texture caches.
	objectsLoaders map[int]*wil.File
	objectsCaches  map[int]map[int]uint32

	animCounter int

	// Tile highlight state (set from main loop).
	HighlightX, HighlightY int // hover tile (-1 = none)
	LockedX, LockedY       int // locked tile (-1 = none)
}

// NewGLRenderer creates a renderer with OpenGL state.
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

// getObjectsLoader returns the WIL loader for the given area, lazy-loading if needed.
// Area 0 = Objects.wil, Area N = Objects{N+1}.wil.
// Matches C++ MapRenderer::GetObjectsLoader.
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
	if idx < 0 || file == nil || idx >= len(file.Images) {
		return 0
	}
	if tex, ok := cache[idx]; ok {
		return tex
	}
	img := file.Images[idx]
	if img == nil || img.RGBA == nil {
		return 0
	}
	tex := UploadTexture(img.RGBA)
	cache[idx] = tex
	return tex
}

// Render draws the visible portion of the map using OpenGL.
// Render order matches C++ MapRenderer::Render:
// Back -> Middle -> Front(normal) -> Front(blend) -> MapBorder -> TileHighlight -> LockedHighlight -> Grid
func (r *GLRenderer) Render(m *mapformat.MapData, cam *Camera2D, showBack, showMid, showFront, showCollision, showGrid bool) {
	// Projection: orthographic Y-down
	left := float32(cam.X)
	top := float32(cam.Y)
	right := float32(cam.X + float64(cam.ViewW)/cam.Zoom)
	bottom := float32(cam.Y + float64(cam.ViewH)/cam.Zoom)
	proj := OrthoProj(left, right, bottom, top)

	gl.UseProgram(r.glState.Shader.ID)
	gl.Uniform1i(r.glState.Shader.TexLoc, 0)

	// Back/middle cull range
	startX, startY, endX, endY := cam.ViewportTiles(cullMargin, cullMargin)
	startX = clamp(startX, 0, m.Width-1)
	startY = clamp(startY, 0, m.Height-1)
	endX = clamp(endX, 0, m.Width-1)
	endY = clamp(endY, 0, m.Height-1)

	// Front cull range (wider margin for tall objects)
	fStartX, fStartY, fEndX, fEndY := cam.ViewportTiles(frontCullMargin, frontCullMargin)
	fStartX = clamp(fStartX, 0, m.Width-1)
	fStartY = clamp(fStartY, 0, m.Height-1)
	fEndX = clamp(fEndX, 0, m.Width-1)
	fEndY = clamp(fEndY, 0, m.Height-1)

	// Align to even for back layer stride-2 rendering.
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

	// 1. Back layer: even x, y (2x2 tile blocks)
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
				img := r.Tiles.Images[info.BackImage]
				wx := float32(x * TileWidth)
				wy := float32(y * TileHeight)
				r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, true, proj)
			}
		}
	}

	// 2. Middle layer: all cells
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
				img := r.SmTiles.Images[info.MiddleImage]
				wx := float32(x * TileWidth)
				wy := float32(y * TileHeight)
				r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, true, proj)
			}
		}
	}

	// 3. Front layer
	if showFront {
		// Normal (non-blend) objects
		for y := fStartY; y <= fEndY; y++ {
			for x := fStartX; x <= fEndX; x++ {
				info := m.InfoAt(x, y)
				r.drawFront(info, x, y, false, proj)
			}
		}
		// Blend objects (additive blending)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
		for y := fStartY; y <= fEndY; y++ {
			for x := fStartX; x <= fEndX; x++ {
				info := m.InfoAt(x, y)
				if info.FrontAniFrame&0x80 != 0 {
					r.drawFront(info, x, y, true, proj)
				}
			}
		}
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		r.animCounter++
	}

	// 4. Collision overlay
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

	// 5. Overlays (must render after all tile layers)
	r.drawMapBorder(cam, m, proj)
	r.drawTileHighlight(m, proj)
	r.drawLockedTileHighlight(m, proj)
	if showGrid {
		r.drawGrid(cam, startX, startY, endX, endY, proj)
	}
}

// drawFront renders a single front-layer cell.
// Matches C++ MapRenderer::RenderFrontLayer + DrawTile for layer 2.
func (r *GLRenderer) drawFront(info *mapformat.CellInfo, x, y int, blendOnly bool, proj [16]float32) {
	if info.FrontLib < 0 {
		return
	}
	isBlend := info.FrontAniFrame&0x80 != 0
	if blendOnly != isBlend {
		return
	}

	area := int(info.FrontArea)
	loader := r.getObjectsLoader(area)
	if loader == nil {
		return
	}
	cache := r.objectsCaches[area]

	idx := info.FrontImage

	// Animation
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

	// Door offset (C++ RenderFrontLayer lines 456-460)
	if info.FrontDoorOffset&0x80 != 0 {
		if info.FrontDoorIndex&0x7F != 0 {
			idx += int(info.FrontDoorOffset & 0x7F)
		}
	}

	if idx < 0 || idx >= len(loader.Images) {
		return
	}

	tex := r.getTex(cache, loader, idx)
	if tex == 0 {
		return
	}
	img := loader.Images[idx]

	cellWorldX := float32(x * TileWidth)
	cellWorldY := float32(y * TileHeight)

	var wx, wy float32
	if isBlend {
		// Blend objects (fire, light): hotspot-based positioning.
		// Delphi formula: (n + ax - 2, m + ay - 68)
		wx = cellWorldX + float32(img.HotX) - 2
		wy = cellWorldY + float32(img.HotY) - 68
	} else {
		// Non-blend objects: bottom-aligned positioning.
		wx = cellWorldX
		wy = cellWorldY - float32(img.Height) + TileHeight
	}
	r.glState.DrawQuad(wx, wy, float32(img.Width), float32(img.Height), tex, true, proj)
}

// drawGrid renders the tile grid overlay using batched line drawing.
// Matches C++ MapRenderer::RenderGrid.
func (r *GLRenderer) drawGrid(cam *Camera2D, startX, startY, endX, endY int, proj [16]float32) {
	gl.UseProgram(r.glState.GridShader.ID)
	gl.UniformMatrix4fv(r.glState.GridShader.ProjLoc, 1, false, &proj[0])
	gl.Uniform4f(r.glState.GridShader.ColorLoc, 0.5, 0.5, 0.5, 0.3)
	gl.BindVertexArray(r.glState.GridVAO)

	// Build all line vertices into one batch.
	lines := make([]float32, 0, ((endX-startX+2)+(endY-startY+2))*4)

	// Vertical lines
	for x := startX; x <= endX+1; x++ {
		wx := float32(x * TileWidth)
		wy0 := float32(startY * TileHeight)
		wy1 := float32((endY + 1) * TileHeight)
		lines = append(lines, wx, wy0, wx, wy1)
	}
	// Horizontal lines
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

// drawMapBorder draws a blue rectangle around the map boundary.
// Matches C++ MapRenderer::RenderMapBorder.
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

// drawTileHighlight draws a white rectangle around the tile under the cursor.
// Matches C++ MapRenderer::RenderTileHighlight.
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

// drawLockedTileHighlight draws a red rectangle around the locked tile.
// Matches C++ MapRenderer::RenderLockedTileHighlight.
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

// drawRect draws a rectangle outline using the grid shader.
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
