# WIL资源查看器开发方案

## 〇、实现状态

### 已完成

| 模块       | 文件                   | 状态 | 说明                                                         |
| ---------- | ---------------------- | ---- | ------------------------------------------------------------ |
| 基础框架   | `main.go`              | ✅   | GLFW + OpenGL + ImGui 主循环，接受目录路径                   |
| 目录树     | `ui/ui.go`             | ✅   | 左侧 250px 面板列出 .wil 文件，按类型四色着色                |
| WIL加载    | `main.go`              | ✅   | 复用 `internal/wil.Load()`，点击目录树时加载                 |
| 图像列表   | `ui/ui.go`             | ✅   | 右侧面板显示图像索引、尺寸、热点坐标                         |
| 图像导航   | `ui/ui.go`             | ✅   | 箭头键 + 按钮（`<<` `<` `>` `>>`）                           |
| 模式切换   | `ui/ui.go`             | ✅   | Browse / Animation 单选按钮                                  |
| GL渲染器   | `renderer/*.go`        | ✅   | 纹理上传、四边形绘制、着色器、`SetWILFile` 热切换            |
| 缩放/平移  | `renderer/renderer.go` | ✅   | 鼠标滚轮缩放（0.1x~20x），中键拖拽平移                       |
| 动作模板   | `action.go`            | ✅   | 人类7种、怪物2种（MA9/MA10）、NPC、`CalcFrames`              |
| 动画播放器 | `animation.go`         | ✅   | Play/Pause/Stop、单帧步进、方向切换、速度调节                |
| 动画模式UI | `ui/ui.go`             | ✅   | 动作选择（7种）、8方向切换、播放控制、速度滑块、帧信息       |
| 图像导出   | `renderer/renderer.go` | ✅   | 单张导出（ExportPNG）和批量导出（ExportAllPNG）为 PNG        |
| 文件分类   | `ui/ui.go`             | ✅   | `wilCategory()` 四色分类：动画=蓝、静态=绿、混合=黄、未知=白 |
| Debug日志  | 三个文件               | ✅   | 集成 `internal/log`，关键操作均有 DEBUG/INFO 级别日志        |

### 未完成

（无）

---

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

**核心原则：WIL 文件本身不存储动画/静态元数据，直接按文件名分类即可。**

WIL 文件只是"图片帧的有序集合"，每张图片就是一个像素数组。是否构成动画、帧与帧之间如何排列、播放速度如何，完全由外部代码（`TActionInfo` 常量表、`btAniFrame` 字段、`AniCount` 字段）来描述。

因此，**不需要自动检测**——根据文件名直接确定使用哪种模式。

#### WIL 文件完整分类表

根据 Delphi 源码 `Client/Share.pas` 中的常量定义：

**纯动画文件（使用动画模式）**：

| 文件名               | 常量名                | 内容          | 帧结构                        | 说明                       |
| -------------------- | --------------------- | ------------- | ----------------------------- | -------------------------- |
| Hum.wil              | HUMIMGIMAGESFILE      | 人类角色外观  | 每套装备 600 帧（8方向×75帧） | 按 TActionInfo.HA 模板组织 |
| HumEffect.wil        | HUMWINGIMAGESFILE     | 角色翅膀/特效 | 与 Hum.wil 对应               | 叠加在角色身上的特效       |
| Hair.wil             | HAIRIMGIMAGESFILE     | 角色发型      | 与 Hum.wil 对应               | 叠加在角色头上的发型       |
| Weapon.wil           | WEAPONIMAGESFILE      | 武器外观      | 与 Hum.wil 对应               | 叠加在角色手中的武器       |
| Mon1.wil ~ Mon18.wil | MONIMAGEFILE          | 怪物图像      | 每种怪物 280/360/440 帧       | 按 MA9~MA47 模板组织       |
| Npc.wil              | NPCIMAGESFILE         | NPC 图像      | 每个 NPC 60 帧                | 站立、说话等简单动画       |
| Magic.wil            | MAGICIMAGESFILE       | 技能特效      | 多帧序列                      | 飞行、爆炸等魔法效果       |
| Magic2.wil           | MAGIC2IMAGESFILE      | 技能特效2     | 多帧序列                      | 扩展技能特效               |
| Effect.wil           | EFFECTIMAGEFILE       | 通用特效      | 多帧序列                      | 爆炸、光效等               |
| Event.wil            | EVENTEFFECTIMAGESFILE | 事件特效      | 多帧序列                      | 地图事件特效               |
| Dragon.wil           | DRAGONIMAGEFILE       | 龙形怪物      | 特殊帧结构                    | 龙类怪物动画               |

**纯静态文件（使用浏览模式）**：

| 文件名        | 常量名              | 内容         | 说明                             |
| ------------- | ------------------- | ------------ | -------------------------------- |
| Items.wil     | BAGITEMIMAGESFILE   | 背包物品图标 | 每个图像是独立的物品，无动画关系 |
| StateItem.wil | STATEITEMIMAGESFILE | 装备状态图标 | 角色面板中的装备图标             |
| DnItems.wil   | DNITEMIMAGESFILE    | 地面物品图标 | 掉落在地面上的物品静态图像       |
| Prguse.wil    | MAINIMAGEFILE       | UI 素材      | 按钮、图标、边框、血条等         |
| Prguse2.wil   | MAINIMAGEFILE2      | UI 素材2     | 扩展 UI 元素                     |
| Prguse3.wil   | MAINIMAGEFILE3      | UI 素材3     | 扩展 UI 元素                     |
| ChrSel.wil    | CHRSELIMAGEFILE     | 角色选择界面 | 登录时的角色选择画面素材         |
| mmap.wil      | MINMAPIMAGEFILE     | 小地图素材   | 小地图使用的图像                 |
| Tiles.wil     | TITLESIMAGEFILE     | 地图图块     | 48×32 像素的地面图块             |
| SmTiles.wil   | SMLTITLESIMAGEFILE  | 小地图图块   | 独立的地面纹理                   |
| MagIcon.wil   | MAGICONIMAGESFILE   | 技能图标     | 技能栏中的技能图标               |

**混合文件（根据上下文选择模式）**：

| 文件名                     | 常量名          | 内容     | 区分机制                               |
| -------------------------- | --------------- | -------- | -------------------------------------- |
| Objects.wil ~ ObjectsN.wil | OBJECTIMAGEFILE | 地图物体 | 通过地图文件中的 `btAniFrame` 字段区分 |

地图文件中每个格子的前方物体层有 `btAniFrame` 字段：

- `btAniFrame = 0` → 静态物体（树木、建筑等）
- `btAniFrame > 0` → 动画物体（火焰、灯光等），低 7 位为帧数
- `btAniFrame & 0x80 != 0` → 使用透明/混合绘制模式

#### 动画帧的三种区分机制

**机制一：TActionInfo 硬编码表（角色/怪物）**

`Actor.pas` 中硬编码了所有动作模板，定义了每个角色/怪物的动画帧布局：

```pascal
// 人类动作表 - 每 600 帧为一套角色动画
HA: THumanAction = (
    ActStand:  (start: 0;    frame: 4;  skip: 4;  ftime: 200);  // 站立
    ActWalk:   (start: 64;   frame: 6;  skip: 2;  ftime: 90);   // 行走
    ActRun:    (start: 128;  frame: 6;  skip: 2;  ftime: 120);  // 跑步
    ActHit:    (start: 200;  frame: 6;  skip: 2;  ftime: 85);   // 攻击
    ActSpell:  (start: 392;  frame: 6;  skip: 2;  ftime: 60);   // 施法
    ActDie:    (start: 536;  frame: 4;  skip: 4;  ftime: 120);  // 死亡
    ...
);
```

帧计算公式：`实际帧 = start + 方向 × (frame + skip) + 当前帧偏移`

怪物有数十种动作模板变体 (MA9~MA47)，每种定义了不同的帧布局。

**机制二：btAniFrame 字段（地图物体）**

地图文件中每个格子的前方物体层有 `btAniFrame` 字段：

- **= 0** → 静态图像
- **> 0** → 动画帧数（最高位 `$80` 表示透明/混合绘制）

**机制三：AniCount 字段（物品）**

物品结构 `TStdItem` 中有 `AniCount` 字段，表示动画帧数（通常为 0，表示静态）。

#### 工具实现建议

```
打开 WIL 文件时的处理逻辑：

1. 根据文件名确定模式：
   - 文件名匹配 Hum/HumEffect/Hair/Weapon/Mon*/Npc/Magic*/Effect/Event/Dragon → 动画模式
   - 文件名匹配 Items/StateItem/DnItems/Prguse*/ChrSel/mmap/Tiles/SmTiles/MagIcon → 浏览模式
   - 文件名匹配 Objects* → 默认浏览模式，可手动切换

2. 动画模式下，根据文件名选择动作模板：
   - Hum.wil → 使用 HA (THumanAction) 模板
   - Mon1~18.wil → 根据怪物类型选择 MA9~MA47 模板
   - Npc.wil → 使用 MERCHANTFRAME (60帧) 模板
   - 其他动画文件 → 提供通用帧序列控制

3. 日志记录：
   mlog.Logf(mlog.LevelDebug, "WIL", "文件: %s, 模式: %s, 图像数: %d", filename, mode, count)
```

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

详见 3.1 节的"WIL 文件完整分类表"，此处为简要汇总：

**动画模式**（11类）：Hum, HumEffect, Hair, Weapon, Mon1~18, Npc, Magic, Magic2, Effect, Event, Dragon

**浏览模式**（11类）：Items, StateItem, DnItems, Prguse, Prguse2, Prguse3, ChrSel, mmap, Tiles, SmTiles, MagIcon

**混合模式**（1类）：Objects ~ ObjectsN（通过 btAniFrame 字段区分）

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
├── main.go           ← 主程序、窗口创建、主循环、缩放/平移处理
├── action.go         ← 动作模板定义（ActionInfo、HumanActions、MonsterActions、CalcFrames）
├── animation.go      ← 动画帧播放控制（AnimationPlayer）
├── renderer/
│   ├── renderer.go   ← WIL图像渲染（纹理缓存、SetWILFile、ExportPNG、ExportAllPNG）
│   ├── gl.go         ← OpenGL状态管理（GLState、DrawQuad、DrawQuadColor）
│   └── shader.go     ← 着色器程序（ShaderProgram）
└── ui/
    └── ui.go         ← ImGui界面（目录树、信息面板、动画控制、文件分类）

复用模块：
└── internal/wil/     ← WIL/WIX文件加载
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

### 4.4 界面布局（当前实现）

```
┌──────────────────────────────────────────────────────────┐
│ 菜单栏: File → Exit                                      │
├────────┬────────────────────────┬────────────────────────┤
│        │                        │                        │
│ Files  │     图像预览区          │      WIL Info          │
│ (250px)│   (OpenGL 渲染)        │       (380px)          │
│        │                        │                        │
│ Data/  │                        │  Title: xxx            │
│ │Hum  │ │  ┌──────────────┐    │  Images: 600           │
│ │Items│ │  │              │    │  Mode: ○Browse ○Anim   │
│ │Mon1 │ │  │   当前图像    │    │  Index: 42             │
│ │...   │ │  │              │    │  Size: 96 x 68         │
│        │  └──────────────┘    │  Navigation: << < 42 > >>│
│ 蓝=动画 │                        │  Image List:           │
│ 绿=静态 │                        │   0: 96x68             │
│ 黄=混合 │                        │   1: 96x68             │
│ 白=未知 │                        │   ...                  │
├────────┴────────────────────────┴────────────────────────┤
│ 操作: ESC退出, 左右箭头键切换图像                         │
└──────────────────────────────────────────────────────────┘
```

**目录树颜色分类**（`wilCategory()` 函数）：

- **蓝色**：动画文件 — Hum, HumEffect, Hair, Weapon, Mon\*, Npc, Magic, Magic2, Effect, Event, Dragon
- **绿色**：静态文件 — Items, StateItem, DnItems, Prguse\*, ChrSel, mmap, Tiles, SmTiles, MagIcon
- **黄色**：混合文件 — Objects\*
- **白色**：未分类 .wil 文件

**动画模式面板**（切换到 Animation 模式时显示在右侧面板内）：

```
├─────────────────────────────────────────────────────────┤
│ Animation Controls                                      │
│ Action: ○stand ○walk ○run ○attack ○spell ○hit ○death  │
│ Direction: ○↑ ○↗ ○→ ○↘ ○↓ ○↙ ○← ○↖                 │
│ Playback: |< < Play > >|                               │
│ Speed: [========] 1.0x                                  │
│ Frame: 1/6 (image 0)                                    │
│ Direction: Up (0)                                       │
│ Action: stand                                           │
└─────────────────────────────────────────────────────────┘
```

---

## 五、开发原则

### 5.1 多写 Debug 日志

**核心原则：验证结果以日志输出为准**

- 每个关键步骤都要输出 DEBUG 级别日志
- 文件加载时记录：文件路径、图像数量、调色板信息
- 动画播放时记录：当前动作、方向、帧号、帧序列
- 用户操作时记录：点击的图像索引、切换的模式、选择的动作
- 错误发生时记录：完整错误信息、上下文参数

**日志示例**：

```go
mlog.Logf(mlog.LevelDebug, "WIL", "加载文件: %s, 图像数量: %d", path, count)
mlog.Logf(mlog.LevelDebug, "Anim", "播放动作: %s, 方向: %d, 帧序列: %v", action, dir, frames)
mlog.Logf(mlog.LevelDebug, "Export", "导出图像: %d, 尺寸: %dx%d", idx, w, h)
```

### 5.2 验证方式

- 运行程序后检查日志输出，确认每个步骤的执行结果
- 对比日志中的数值与预期值，确保逻辑正确
- 错误时通过日志快速定位问题原因

---

## 六、开发步骤

### Step 1: 基础框架搭建

1. 创建 `cmd/wilviewer/` 目录
2. 复制 `cmd/mapviewer/main.go` 作为起点
3. 修改窗口标题和大小
4. 验证 GLFW + ImGui 基础框架能运行

### Step 2: WIL文件加载

1. 实现文件打开对话框
2. 调用 `internal/wil.Load()` 加载文件
3. 显示文件头信息（ImageCount、ColorCount等）
4. 验证能正确加载不同类型的 .wil 文件

### Step 3: 浏览模式实现

1. 实现缩略图生成（缩放到固定尺寸）
2. 实现 ImGui 列表控件
3. 实现图像查看区（缩放、平移）
4. 实现元数据显示面板
5. 实现图像导出功能

### Step 4: 动作模板定义

1. 实现 `action.go`，定义所有动作模板
2. 参考 Delphi 源码中的 TActionInfo 定义
3. 实现人类角色动作模板（7种）
4. 实现怪物动作模板（14种）

### Step 5: 动画播放器

1. 实现 `animation.go`，核心播放逻辑
2. 实现帧序列计算（考虑方向、跳帧）
3. 实现播放/暂停/停止控制
4. 实现单帧步进
5. 实现速度调节

### Step 6: 动画模式实现

1. 实现模式切换（浏览/动画）
2. 实现动作选择面板
3. 实现方向选择面板
4. 实现播放控制面板
5. 实现帧信息显示

### Step 7: 测试和优化

1. 测试浏览模式（Items.wil、Tiles.wil等）
2. 测试动画模式（Hum.wil、Mon1.wil等）
3. 修复兼容性问题
4. 优化性能（纹理缓存、懒加载）

---

## 七、关键代码参考

### 7.1 从 mapviewer 复用的代码

```go
// 纹理上传
func UploadTexture(img *image.RGBA) uint32

// 绘制四边形
func DrawQuad(tex uint32, x, y, w, h float32)

// ImGui 集成
func toImGuiWindow(w *glfw.Window) *igglfw.GLFWwindow
```

### 7.2 WIL加载器接口

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

### 7.3 帧序列计算示例

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

## 八、测试验证

### 8.1 浏览模式测试

| 测试项       | 测试文件   | 预期结果         |
| ------------ | ---------- | ---------------- |
| 打开物品WIL  | Items.wil  | 显示物品图标列表 |
| 打开UI素材   | Prguse.wil | 显示UI元素列表   |
| 打开地图图块 | Tiles.wil  | 显示48x32图块    |
| 图像查看     | 任意       | 缩放、平移正常   |
| 图像导出     | 任意       | 导出PNG文件正确  |

### 8.2 动画模式测试

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

### 8.3 验证步骤

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
4. **日志验证**：
   - 检查控制台输出，确认每个关键步骤都有 DEBUG 日志
   - 验证日志中的数值（图像数量、帧号、方向等）与预期一致
   - 错误时通过日志快速定位问题

---

## 九、后续扩展

### 9.1 可选功能

- **批量查看**：同时打开多个WIL文件
- **图像对比**：对比不同WIL中的图像
- **资源搜索**：按尺寸、类型搜索图像
- **调色板编辑**：修改调色板并预览效果
- **装备叠加**：同时显示角色和武器动画
- **动画录制**：录制动画为GIF或视频
- **批量预览**：同时预览多个方向
- **帧序列编辑**：自定义帧序列
