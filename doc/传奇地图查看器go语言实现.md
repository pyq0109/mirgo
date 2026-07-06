# 传奇地图查看器 — Go 语言实现方案

## 1. 概述

用 Go + Fyne 实现传奇地图查看器，纯软件渲染，不引入 go-gl。

参考：`asset/map_viewer` C++ 实现、`doc/传奇地图查看器.md` 文档。

## 2. 技术选型

| 库 | 用途 |
|---|---|
| `fyne.io/fyne/v2` | UI 框架（窗口、菜单、面板、图像显示、输入事件） |
| `golang.org/x/text` | GBK → UTF-8 编码转换 |

**不引入 go-gl 的理由**：
- Fyne 内置 OpenGL 渲染 UI，额外引入 go-gl 需管理两个 GL 上下文，复杂度高
- 地图查看器渲染量有限（视口内几百个 tile），纯 CPU 合成完全够用
- Fyne 的 `canvas.NewImageFromImage()` 可直接显示 `*image.RGBA`
- 动画用 goroutine 定时重绘，Blend 用 `draw.DrawMask` 实现

## 3. 架构

```
┌─────────────────────────────────────────────────┐
│  Fyne Window                                    │
│  ┌──────────────────────────┬──────────────────┐│
│  │  MapCanvas               │  Info Panel      ││
│  │  canvas.NewImageFromImage│  - 格子属性      ││
│  │  ← *image.RGBA           │  - 坐标显示      ││
│  │                          │  - 地图信息      ││
│  │  ┌────────────────────┐  │                  ││
│  │  │ Minimap (200x200)  │  │                  ││
│  │  └────────────────────┘  │                  ││
│  └──────────────────────────┴──────────────────┘│
└─────────────────────────────────────────────────┘

渲染流程:
  mapformat.Parse() → MapData
  wil.Load() → 图像缓存
  renderer.Render() → 合成可见区域到 *image.RGBA
  canvas.Refresh() → Fyne 显示
```

## 4. 目录结构

```
cmd/mapviewer/
└── main.go              # 入口、Fyne UI 布局、事件处理

internal/pkg/
├── mapformat/
│   ├── map.go           # .map 文件解析器
│   └── map_test.go      # 解析测试
├── wil/
│   ├── wil.go           # .wil/.wix 图像加载器
│   └── wil_test.go      # 加载测试
└── renderer/
    ├── camera.go         # 2D 相机（平移、缩放、边界约束）
    ├── renderer.go       # 三层软件渲染器 + 动画 + Blend
    └── minimap.go        # 小地图（碰撞纹理）
```

## 5. 核心数据结构

### 5.1 mapformat — .map 解析

```go
type Header struct {
    Width, Height uint16
    TitleLen      uint8
    Title         [16]byte
    UpdateDate    float64
    Reserved      [23]byte
}

type Cell struct {
    BkImg      uint16  // bit15=碰撞
    MidImg     uint16
    FrImg      uint16
    DoorIndex  uint8   // bit7=有门
    DoorOffset uint8   // bit7=门开启
    AniFrame   uint8   // bit7=Alpha混合, bits6-0=帧数
    AniTick    uint8
    Area       uint8   // 选择 Objects{N+1}.wil
    Light      uint8
}

type MapData struct {
    Header Header
    Cells  []Cell
    Width  int
    Height int
}
```

- 文件头 52 字节 + `Width*Height*12` 字节格子数据
- **列优先**存储 → 解析后转**行优先**
- 自动检测 12/14/20 字节格式
- 图像索引 1-based，0 表示空

### 5.2 wil — WIL/WIX 加载

```go
type Image struct {
    Width, Height int
    HotX, HotY    int16
    Pixels        []byte  // RGBA
}

type File struct {
    Title  string
    Images []*Image
}
```

- 读 WIL 文件头 → 256 色调色板(BGRA) → 从 WIX 读偏移索引 → 按偏移读图像
- 调色板索引 0 = 透明
- 图像缓存为 `*image.RGBA`，供 `draw.Draw` 合成

### 5.3 renderer — 软件渲染器

```go
type Renderer struct {
    tiles      *wil.File     // Tiles.wil
    smTiles    *wil.File     // SmTiles.wil
    objects    []*wil.File   // Objects.wil, Objects2.wil, ...
    camera     *Camera
    animStates []AnimState
}

type AnimState struct {
    FrameIdx int
    TickAcc  int
}

type Camera struct {
    X, Y         float64
    Zoom         float64
    ViewW, ViewH int
}
```

## 6. 渲染方案

### 6.1 瓦片规格
- 48×32 像素/格子
- `tile_x = floor(world_x / 48)`, `tile_y = floor(world_y / 32)`

### 6.2 软件渲染流程

```go
func (r *Renderer) Render(m *mapformat.MapData) *image.RGBA {
    dst := image.NewRGBA(image.Rect(0, 0, viewW, viewH))
    // 1. 背景层: 偶数 x,y 位置，覆盖 2x2 格子
    for y := startY; y <= endY; y += 2 {
        for x := startX; x <= endX; x += 2 {
            img := r.getBackImage(m, x, y)
            if img != nil {
                draw.Draw(dst, screenRect, img, imgBounds.Min, draw.Over)
            }
        }
    }
    // 2. 中间层: 所有位置
    for y := startY; y <= endY; y++ {
        for x := startX; x <= endX; x++ {
            img := r.getMidImage(m, x, y)
            if img != nil {
                draw.Draw(dst, screenRect, img, imgBounds.Min, draw.Over)
            }
        }
    }
    // 3. 前景层: 底部对齐
    for y := startY; y <= endY; y++ {
        for x := startX; x <= endX; x++ {
            img := r.getFrontImage(m, x, y)
            if img != nil {
                draw.Draw(dst, screenRect, img, imgBounds.Min, draw.Over)
            }
        }
    }
    // 4. Blend 对象: draw.Over 加法混合
    for _, blend := range blendObjects {
        draw.Draw(dst, screenRect, blend.img, blend.pos, draw.Over)
    }
    // 5. 碰撞覆盖: 红色半透明
    for y := startY; y <= endY; y++ {
        for x := startX; x <= endX; x++ {
            if m.IsCollision(x, y) {
                draw.Draw(dst, tileRect, collisionImg, image.Point{}, draw.Over)
            }
        }
    }
    return dst
}
```

### 6.3 动画支持
- `AniFrame & 0x7F` = 帧数（0=静态，>0=动画）
- `AniTick` = 每帧持续 tick 数
- goroutine 定时递增 TickAcc，达到 AniTick 时切换下一帧
- 动画帧图像索引连续：`baseIdx + frameIdx`
- 帧切换后调用 `canvas.Image.Refresh()` 重绘

### 6.4 Blend 混合（火焰/灯光）
- 检测: `AniFrame & 0x80 != 0`
- Go 实现: `draw.Draw()` 的 `draw.Over` 操作符（SRC over DST alpha 合成）
- 定位: 使用 hot_x/hot_y 热点偏移
- 绘制顺序: Blend 对象在普通前景之后

### 6.5 Fyne UI
```go
// 主布局
mapImage := canvas.NewImageFromImage(renderedMap)
mapImage.FillMode = canvas.ImageFillStretch

infoLabel := widget.NewLabel("格子信息...")

split := container.NewHSplit(
    container.NewMax(mapImage, minimapWidget),
    infoLabel,
)
split.SetOffset(0.8)

w := fyne.NewWindow("地图查看器")
w.SetContent(split)
w.Resize(fyne.NewSize(1200, 800))
```

### 6.6 交互
- 鼠标拖拽平移（Fyne `desktop.MouseEvent`）
- 滚轮缩放（Fyne `desktop.ScrollEvent`）
- 左键点击显示格子属性（Fyne `desktop.MouseEvent`）
- 小地图: 200×200 碰撞纹理 + 视口矩形

## 7. 关键实现细节

### 7.1 列优先转行优先
```go
for col := 0; col < width; col++ {
    for row := 0; row < height; row++ {
        offset := 52 + (col*height+row)*12
        cells[row*width+col] = parseCell(data[offset:])
    }
}
```

### 7.2 位域解析
```go
collision := (cell.BkImg & 0x8000) != 0
bkIndex := int(cell.BkImg&0x7FFF) - 1
isBlend := (cell.AniFrame & 0x80) != 0
aniFrames := cell.AniFrame & 0x7F
```

### 7.3 GBK 编码
```go
title, _ = simplifiedchinese.GBK.NewDecoder().Bytes(header.Title[:header.TitleLen])
```

### 7.4 WIL 调色板解码
```go
for i, idx := range pixelData {
    if idx == 0 {
        rgba[i*4+3] = 0  // 透明
    } else {
        rgba[i*4+0] = palette[idx].Red
        rgba[i*4+1] = palette[idx].Green
        rgba[i*4+2] = palette[idx].Blue
        rgba[i*4+3] = 255
    }
}
```

## 8. 文件清单

| 文件 | 说明 |
|---|---|
| `go.mod` | 添加 fyne、x/text 依赖 |
| `internal/pkg/mapformat/map.go` | .map 文件解析器 |
| `internal/pkg/mapformat/map_test.go` | 解析测试 |
| `internal/pkg/wil/wil.go` | WIL/WIX 图像加载器 |
| `internal/pkg/wil/wil_test.go` | 加载测试 |
| `internal/pkg/renderer/camera.go` | 2D 相机 |
| `internal/pkg/renderer/renderer.go` | 软件三层渲染器 |
| `internal/pkg/renderer/minimap.go` | 小地图 |
| `cmd/mapviewer/main.go` | Fyne UI 入口 |

## 9. 验证方式

1. `go build ./cmd/mapviewer` — 编译通过
2. 运行加载 `asset/server/Map/` 下的 .map 文件
3. 验证三层渲染正确（背景、中间层、前景）
4. 验证动画帧切换和 Blend 混合效果
5. 验证碰撞可视化（红色覆盖）
6. 验证平移缩放流畅
