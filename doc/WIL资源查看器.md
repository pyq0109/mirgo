# WIL资源查看器开发方案

## 一、工具概述

### 1.1 用途

WIL资源查看器用于查看热血传奇游戏专用的 .wil/.wix 图像资源文件，支持静态图像浏览和动画播放两种模式。

### 1.2 必要性

- .wil 是游戏专用格式，无现成通用工具可查看
- 开发客户端时需要频繁查看和确认资源索引
- 角色/怪物动画需要按动作模板正确播放帧序列

---

## 二、游戏动画系统分析

### 2.1 角色动画结构

根据 Delphi 源码 `Client/Actor.pas`，人类角色动画结构如下：

**每套装备每个方向 600 帧**，按动作分组：

| 动作 | 起始帧 | 帧数 | 说明                  |
| ---- | ------ | ---- | --------------------- |
| 站立 | 0      | 4    | Stand                 |
| 走路 | 64     | 60   | Walk (8方向×6帧+余量) |
| 跑步 | 128    | 60   | Run                   |
| 攻击 | 192    | 60   | Attack                |
| 施法 | 256    | 60   | Spell                 |
| 被击 | 320    | 30   | Hit                   |
| 死亡 | 384    | 30   | Death                 |

**实际帧数计算**：

```
方向数 = 8
每方向帧数 = 约75帧
总帧数 = 8 × 75 = 600帧
```

### 2.2 怪物动画结构

怪物有14种动作模板（MA9~MA22），每种模板的帧数不同：

| 模板 | 动作数 | 每动作帧数 | 说明               |
| ---- | ------ | ---------- | ------------------ |
| MA9  | 6      | 10         | 基础怪物           |
| MA10 | 6      | 10         | 基础怪物（带方向） |
| MA11 | 6      | 10         | 基础怪物（带方向） |
| MA12 | 6      | 10         | 基础怪物（带方向） |
| MA13 | 6      | 10         | 基础怪物（带方向） |
| MA14 | 6      | 10         | 基础怪物（带方向） |
| MA15 | 6      | 10         | 基础怪物（带方向） |
| MA16 | 6      | 10         | 基础怪物（带方向） |
| MA17 | 6      | 10         | 基础怪物（带方向） |
| MA18 | 6      | 10         | 基础怪物（带方向） |
| MA19 | 6      | 10         | 基础怪物（带方向） |
| MA20 | 6      | 10         | 基础怪物（带方向） |
| MA21 | 6      | 10         | 基础怪物（带方向） |
| MA22 | 6      | 10         | 基础怪物（带方向） |

### 2.3 TActionInfo 结构定义

根据 Delphi 源码 `Client/Actor.pas`：

```pascal
TActionInfo = record
  start   : Word;    // 起始帧索引
  frame   : Word;    // 帧数
  skip    : Word;    // 跳帧数
  interval: Word;    // 帧间隔(ms)
end;
```

**人类角色动作定义示例**：

```pascal
// 站立
HumanActStand: TActionInfo = (start:0; frame:4; skip:0; interval:200);
// 走路
HumanActWalk: TActionInfo = (start:64; frame:60; skip:0; interval:100);
// 攻击
HumanActAttack: TActionInfo = (start:192; frame:60; skip:0; interval:80);
```

### 2.4 帧序列计算

**方向计算**：

```
方向索引 = 0~7 (上、右上、右、右下、下、左下、左、左上)
每方向起始帧 = 动作起始帧 + 方向索引 × 每方向帧数
```

**帧序列**：

```
帧序列 = [起始帧, 起始帧+1, 起始帧+2, ..., 起始帧+帧数-1]
跳帧处理：每隔 skip 帧跳过一帧
```

---

## 三、功能需求

### 3.1 两种工作模式

#### 浏览模式（Browse Mode）

**定义**：以缩略图列表形式展示WIL文件中的所有图像，支持单张查看和导出。

**适用场景**：

- WIL文件中的图像是**独立的、无动画关系**的静态图像
- 需要查看**所有图像**的概览
- 需要**定位特定图像**的索引号
- 需要**批量导出**图像

**典型文件**：
| 文件 | 内容 | 为什么用浏览模式 |
|------|------|-----------------|
| Items.wil | 物品图标 | 每个图像是独立的物品，无动画关系 |
| Prguse.wil | UI素材 | 按钮、图标、边框等独立元素 |
| Tiles.wil | 地图图块 | 48x32像素的独立图块 |
| SmTiles.wil | 小地图图块 | 独立的地面纹理 |
| Objects.wil | 地图物体 | 树木、建筑等独立物体 |
| DnItems.wil | 地面物品 | 掉落物品的静态图像 |

#### 动画模式（Animation Mode）

**定义**：按动作模板播放帧序列，支持8方向切换和播放控制。

**适用场景**：

- WIL文件中的图像是**按动作组织的帧序列**
- 需要查看**角色/怪物的动画效果**
- 需要验证**帧序列是否正确**
- 需要检查**8方向是否正常**

**典型文件**：
| 文件 | 内容 | 为什么用动画模式 |
|------|------|-----------------|
| Hum.wil | 角色外观 | 600帧按动作分组（站立4帧、走路60帧、攻击60帧等） |
| Mon1~28.wil | 怪物图像 | 按MA9~MA22模板组织的动画帧 |
| Weapon.wil | 武器外观 | 与角色动画配合的武器帧序列 |
| Magic.wil | 技能特效 | 多帧动画效果 |
| Effect.wil | 特效 | 爆炸、光效等动画 |
| Hair.wil | 发型 | 角色发型动画 |

#### 如何判断使用哪种模式？

```
判断逻辑：
1. 打开WIL文件
2. 查看图像内容：
   - 如果图像是独立的图标/图块 → 浏览模式
   - 如果图像是连续的动画帧 → 动画模式
3. 如果不确定，先用浏览模式查看前几张图像：
   - 看起来像同一动作的不同帧 → 切换到动画模式
   - 看起来是独立的图像 → 保持浏览模式
```

**快速判断表**：
| 特征 | 浏览模式 | 动画模式 |
|------|---------|---------|
| 图像数量 | 通常较多（数百~数千） | 通常较少（几十~几百） |
| 图像内容 | 各不相同 | 相似但有细微变化 |
| 图像尺寸 | 多种尺寸 | 相对统一 |
| 用途 | 图标、素材、图块 | 角色、怪物、特效 |

### 3.2 浏览模式功能

| 功能        | 优先级 | 说明                          |
| ----------- | ------ | ----------------------------- |
| 打开WIL文件 | P0     | 支持打开 .wil/.wix 文件对     |
| 图像列表    | P0     | 显示所有图像的缩略图列表      |
| 图像查看    | P0     | 支持缩放、平移查看单张图像    |
| 元数据显示  | P0     | 显示宽高、偏移、调色板信息    |
| 图像导出    | P1     | 导出单张或批量导出为 PNG 格式 |

### 3.3 动画模式功能

| 功能       | 优先级 | 说明                           |
| ---------- | ------ | ------------------------------ |
| 8方向播放  | P0     | 支持切换8个方向                |
| 动作切换   | P0     | 站立、走路、跑步、攻击、施法等 |
| 播放控制   | P0     | 播放、暂停、停止、单帧步进     |
| 速度调节   | P1     | 调整播放速度                   |
| 帧信息显示 | P1     | 显示当前帧号、方向、动作       |

### 3.4 支持的WIL类型

| 文件        | 内容     | 推荐模式 |
| ----------- | -------- | -------- |
| Hum.wil     | 角色外观 | 动画模式 |
| Mon1~28.wil | 怪物图像 | 动画模式 |
| Weapon.wil  | 武器外观 | 动画模式 |
| Magic.wil   | 技能特效 | 动画模式 |
| Items.wil   | 物品图标 | 浏览模式 |
| Prguse.wil  | UI素材   | 浏览模式 |
| Tiles.wil   | 地图图块 | 浏览模式 |
| Objects.wil | 地图物体 | 浏览模式 |

---

## 四、技术方案

### 4.1 技术栈

| 组件    | 技术选型          | 说明                  |
| ------- | ----------------- | --------------------- |
| 语言    | Go                | 与项目一致            |
| 窗口    | GLFW              | 复用 mapviewer 方案   |
| 渲染    | OpenGL 3.3        | 复用 mapviewer 渲染器 |
| UI      | ImGui (cimgui-go) | 复用 mapviewer UI框架 |
| WIL加载 | internal/wil/     | 已实现的WIL加载器     |

### 4.2 目录结构

```
cmd/wilviewer/
├── main.go           ← 主程序、窗口创建、主循环
├── viewer.go         ← 图像浏览核心逻辑
├── animation.go      ← 动画帧播放控制
├── action.go         ← 动作模板定义
├── export.go         ← 图像导出功能
├── metadata.go       ← 元数据显示面板
└── ui.go             ← ImGui界面布局

复用模块：
├── internal/wil/     ← WIL/WIX文件加载
└── cmd/mapviewer/renderer/ ← OpenGL渲染
```

### 4.3 核心模块设计

#### 4.3.1 WIL文件加载

复用 `internal/wil/wil.go` 已有实现：

```go
type File struct {
    Header   Header
    Palette  [256]color.RGBA
    Entries  []Entry
    // ...
}

type Entry struct {
    Width  int
    Height int
    PX     int    // 热点X
    PY     int    // 热点Y
    RGBA   *image.RGBA
}
```

#### 4.3.2 动作模板定义

```go
// action.go

type ActionInfo struct {
    Start    int  // 起始帧索引
    Frame    int  // 帧数
    Skip     int  // 跳帧数
    Interval int  // 帧间隔(ms)
}

// 人类角色动作模板
var HumanActions = map[string]ActionInfo{
    "stand":  {Start: 0, Frame: 4, Skip: 0, Interval: 200},
    "walk":   {Start: 64, Frame: 60, Skip: 0, Interval: 100},
    "run":    {Start: 128, Frame: 60, Skip: 0, Interval: 80},
    "attack": {Start: 192, Frame: 60, Skip: 0, Interval: 80},
    "spell":  {Start: 256, Frame: 60, Skip: 0, Interval: 80},
    "hit":    {Start: 320, Frame: 30, Skip: 0, Interval: 100},
    "death":  {Start: 384, Frame: 30, Skip: 0, Interval: 150},
}

// 怪物动作模板（14种）
var MonsterActions = map[int]map[string]ActionInfo{
    9:  {"stand": {...}, "walk": {...}, "attack": {...}, ...},
    10: {"stand": {...}, "walk": {...}, "attack": {...}, ...},
    // ...
}
```

#### 4.3.3 动画播放器

```go
// animation.go

type AnimationPlayer struct {
    // 配置
    action     ActionInfo
    direction  int         // 方向(0-7)
    speed      float64     // 速度倍率

    // 状态
    playing    bool
    frameIdx   int         // 当前帧索引
    timer      *time.Timer
    lastUpdate time.Time

    // 帧序列
    frames     []int       // 计算后的帧序列
}

func NewAnimationPlayer(action ActionInfo, direction int) *AnimationPlayer

func (p *AnimationPlayer) Play()
func (p *AnimationPlayer) Pause()
func (p *AnimationPlayer) Stop()
func (p *AnimationPlayer) NextFrame()
func (p *AnimationPlayer) PrevFrame()
func (p *AnimationPlayer) SetDirection(dir int)
func (p *AnimationPlayer) SetSpeed(speed float64)
func (p *AnimationPlayer) GetCurrentFrame() int
func (p *AnimationPlayer) IsPlaying() bool
```

#### 4.3.4 图像查看器

```go
// viewer.go

type ImageViewer struct {
    file       *wil.File
    textures   map[int]uint32  // 纹理缓存
    currentIdx int             // 当前选中图像
    zoom       float32         // 缩放比例
    offset     Vec2            // 平移偏移

    // 动画相关
    player     *AnimationPlayer
    mode       string          // "browse" / "animation"
}

func (v *ImageViewer) LoadWIL(path string) error
func (v *ImageViewer) GetTexture(idx int) uint32
func (v *ImageViewer) Render()
func (v *ImageViewer) HandleInput()
func (v *ImageViewer) SetMode(mode string)
func (v *ImageViewer) SetAnimation(action string, direction int)
```

#### 4.3.5 图像导出

```go
// export.go

type Exporter struct {
    outputDir string
}

func (e *Exporter) ExportSingle(img *image.RGBA, filename string) error
func (e *Exporter) ExportBatch(file *wil.File, indices []int) error
func (e *Exporter) ExportAll(file *wil.File) error
```

### 4.4 界面布局

```
┌─────────────────────────────────────────────────────────┐
│ 菜单栏: 文件 | 模式 | 视图 | 工具                        │
├──────────┬──────────────────────────────────────────────┤
│          │                                              │
│ 图像列表 │              图像/动画预览区                   │
│ (缩略图) │         (支持缩放、平移)                      │
│          │                                              │
│          │         ┌──────────────┐                     │
│          │         │              │                     │
│          │         │   图像/动画   │                     │
│          │         │              │                     │
│          │         └──────────────┘                     │
│          │                                              │
├──────────┴──────────────────────────────────────────────┤
│ 模式切换: [浏览模式] [动画模式]                           │
├─────────────────────────────────────────────────────────┤
│ 属性面板: 图像索引 | 宽高 | 偏移 | 调色板信息            │
├─────────────────────────────────────────────────────────┤
│ 动画控制（仅动画模式）:                                   │
│ 动作: [站立] [走路] [跑步] [攻击] [施法] [被击] [死亡]  │
│ 方向: ↑  ↗  →  ↘  ↓  ↙  ←  ↖                          │
│ 播放: ◀◀  ◀  ▶/⏸  ▶  ▶▶  |  速度: [========]         │
│ 帧信息: 帧号: 42/60 | 方向: 右下 | 动作: 攻击           │
└─────────────────────────────────────────────────────────┘
```

---

## 五、开发步骤

### Step 1: 基础框架搭建（2天）

1. 创建 `cmd/wilviewer/` 目录
2. 复制 `cmd/mapviewer/main.go` 作为起点
3. 修改窗口标题和大小
4. 验证 GLFW + ImGui 基础框架能运行

### Step 2: WIL文件加载（2天）

1. 实现文件打开对话框
2. 调用 `internal/wil.Load()` 加载文件
3. 显示文件头信息（ImageCount、ColorCount等）
4. 验证能正确加载不同类型的 .wil 文件

### Step 3: 浏览模式实现（3天）

1. 实现缩略图生成（缩放到固定尺寸）
2. 实现 ImGui 列表控件
3. 实现图像查看区（缩放、平移）
4. 实现元数据显示面板
5. 实现图像导出功能

### Step 4: 动作模板定义（2天）

1. 实现 `action.go`，定义所有动作模板
2. 参考 Delphi 源码中的 TActionInfo 定义
3. 实现人类角色动作模板（7种）
4. 实现怪物动作模板（14种）

### Step 5: 动画播放器（3天）

1. 实现 `animation.go`，核心播放逻辑
2. 实现帧序列计算（考虑方向、跳帧）
3. 实现播放/暂停/停止控制
4. 实现单帧步进
5. 实现速度调节

### Step 6: 动画模式实现（3天）

1. 实现模式切换（浏览/动画）
2. 实现动作选择面板
3. 实现方向选择面板
4. 实现播放控制面板
5. 实现帧信息显示

### Step 7: 测试和优化（3天）

1. 测试浏览模式（Items.wil、Tiles.wil等）
2. 测试动画模式（Hum.wil、Mon1.wil等）
3. 修复兼容性问题
4. 优化性能（纹理缓存、懒加载）

---

## 六、关键代码参考

### 6.1 从 mapviewer 复用的代码

```go
// 纹理上传
func UploadTexture(img *image.RGBA) uint32

// 绘制四边形
func DrawQuad(tex uint32, x, y, w, h float32)

// ImGui 集成
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow
```

### 6.2 WIL加载器接口

```go
// internal/wil/wil.go
func Load(path string) (*File, error)
func (f *File) GetImage(idx int) (*Entry, error)

type Entry struct {
    Width  int
    Height int
    PX     int    // 热点X
    PY     int    // 热点Y
    RGBA   *image.RGBA
}
```

### 6.3 帧序列计算示例

```go
// 计算某动作某方向的帧序列
func calcFrames(action ActionInfo, direction int) []int {
    dirFrames := action.Frame / 8  // 每方向帧数
    start := action.Start + direction * dirFrames

    frames := make([]int, 0, dirFrames)
    for i := 0; i < dirFrames; i++ {
        if action.Skip > 0 && i % (action.Skip + 1) == action.Skip {
            continue  // 跳帧
        }
        frames = append(frames, start + i)
    }
    return frames
}
```

---

## 七、测试验证

### 7.1 浏览模式测试

| 测试项       | 测试文件   | 预期结果         |
| ------------ | ---------- | ---------------- |
| 打开物品WIL  | Items.wil  | 显示物品图标列表 |
| 打开UI素材   | Prguse.wil | 显示UI元素列表   |
| 打开地图图块 | Tiles.wil  | 显示48x32图块    |
| 图像查看     | 任意       | 缩放、平移正常   |
| 图像导出     | 任意       | 导出PNG文件正确  |

### 7.2 动画模式测试

| 测试项   | 测试文件 | 预期结果         |
| -------- | -------- | ---------------- |
| 加载角色 | Hum.wil  | 正确加载600帧    |
| 站立动画 | Hum.wil  | 4帧循环播放      |
| 走路动画 | Hum.wil  | 8方向各6帧       |
| 攻击动画 | Hum.wil  | 8方向各8帧       |
| 加载怪物 | Mon1.wil | 正确加载怪物图像 |
| 怪物动画 | Mon1.wil | 按模板播放       |
| 方向切换 | 任意     | 8方向正确切换    |
| 速度调节 | 任意     | 播放速度变化     |

### 7.3 验证步骤

1. 运行 `go run cmd/wilviewer/main.go`
2. **浏览模式测试**：
   - 打开 `asset/client/Data/Items.wil`
   - 验证缩略图列表正确显示
   - 点击任意图像，验证查看区显示正确
   - 测试导出功能
3. **动画模式测试**：
   - 切换到动画模式
   - 打开 `asset/client/Data/Hum.wil`
   - 选择"站立"动作，验证4帧循环
   - 切换方向，验证8方向正确
   - 切换"走路"动作，验证动画流畅
   - 调整速度，验证播放速度变化
   - 测试怪物WIL（Mon1.wil）

---

## 八、后续扩展

### 8.1 可选功能

- **批量查看**：同时打开多个WIL文件
- **图像对比**：对比不同WIL中的图像
- **资源搜索**：按尺寸、类型搜索图像
- **调色板编辑**：修改调色板并预览效果
- **装备叠加**：同时显示角色和武器动画
- **动画录制**：录制动画为GIF或视频
- **批量预览**：同时预览多个方向
- **帧序列编辑**：自定义帧序列
