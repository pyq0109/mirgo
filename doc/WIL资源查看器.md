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

| 动作 | 起始帧 | 帧数 | 每方向帧数 | 说明                  |
| ---- | ------ | ---- | ---------- | --------------------- |
| 站立 | 0      | 4    | 4          | Stand（不分方向）     |
| 走路 | 64     | 60   | 7~8        | Walk (8方向×7帧+余量) |
| 跑步 | 128    | 60   | 7~8        | Run                   |
| 攻击 | 192    | 60   | 7~8        | Attack                |
| 施法 | 256    | 60   | 7~8        | Spell                 |
| 被击 | 320    | 30   | 3~4        | Hit                   |
| 死亡 | 384    | 30   | 3~4        | Death                 |

**实际帧数计算**：

```
方向数 = 8
每方向帧数 = 帧数 / 8（站立除外，站立 Frame=4 < 8，不分方向）
总帧数 = 8 × 75 = 600帧
```

**注意**：站立动作 Frame=4 < 8，不按方向分配。查看器中 `calcAnimFrames` 对 Frame < 8 的动作直接返回全部帧，不除以 8。

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

**定义**：以 64×64 缩略图网格展示WIL文件中的所有图像，支持单张查看和导出。

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

| 功能        | 优先级 | 说明                           |
| ----------- | ------ | ------------------------------ |
| 打开WIL文件 | P0     | 支持打开 .wil/.wix 文件对      |
| 纹理网格    | P0     | 64×64 缩略图网格展示所有图像   |
| 图像查看    | P0     | 右下面板自适应窗口显示选中图像 |
| 元数据显示  | P0     | 右上面板显示宽高、热点信息     |
| 图像导出    | P1     | 导出单张或批量导出为 PNG 格式  |

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
├── main.go           ← 主程序、窗口创建、主循环、输入处理
├── action.go         ← 动作模板定义（ActionInfo、HumanActions、MonsterActions、NpcActions、CalcFrames）
├── renderer/
│   ├── renderer.go   ← WIL图像渲染（纹理缓存、SetWILFile、GetOrCreateTexture、UploadTexture）
│   ├── gl.go         ← OpenGL状态管理（GLState、DrawQuad、DrawQuadColor）
│   └── shader.go     ← 着色器程序（ShaderProgram，含 flipV uniform）
└── ui/
    └── ui.go         ← ImGui界面（目录树、纹理网格、信息面板、预览面板、动画控制、calcAnimFrames）

复用模块：
└── internal/wil/     ← WIL/WIX文件加载
```

### 4.3 核心模块设计

#### 4.3.1 WIL文件加载

复用 `internal/wil/wil.go` 已有实现：

```go
type File struct {
    Title      string
    Count      int
    Images     []*Image
    Palette    [256]color.RGBA
    BtVersion  int    // 版本标志（0=12字节图像头, 1=8字节图像头）
    ColorCount int    // 颜色数: 256→8-bit调色板, 65536→16-bit RGB565
}

type Image struct {
    Width  int
    Height int
    HotX   int16
    HotY   int16
    RGBA   *image.RGBA
}
```

#### 4.3.1b WIL 文件格式

**ILib 文件头**（56字节）：

| 偏移 | 大小 | 字段 | 说明 |
|------|------|------|------|
| 0-39 | 40字节 | Title | 文件标识，如 `#ILIB v1.0-WEMADE Entertainment inc.` |
| 40-43 | 4字节 | VerFlag | 版本标志（!=0 时图像头为12字节） |
| 44-47 | 4字节 | ImageCount | 图像总数 |
| 48-51 | 4字节 | ColorCount | 颜色数（256=8-bit调色板，65536=16-bit RGB565） |
| 52-55 | 4字节 | PaletteSize | 调色板大小（通常1024=256×4） |
| 56-1079 | 1024字节 | Palette | 256色 BGRA 调色板（仅8-bit模式使用） |

**INDX 索引文件头**（48字节）：

| 偏移 | 大小 | 字段 | 说明 |
|------|------|------|------|
| 0-39 | 40字节 | Title | 文件标识，如 `#INDX v1.0-WEMADE Entertainment inc.` |
| 40-43 | 4字节 | Unknown | 未知字段 |
| 44-47 | 4字节 | IndexCount | 索引数量（与 WIL 的 ImageCount 一致） |
| 48+ | 4×N字节 | Offsets | 每个图像在 WIL 文件中的绝对偏移 |

**图像头**（btVersion=0 时12字节，btVersion=1 时8字节）：

| 偏移 | 大小 | 字段 | 说明 |
|------|------|------|------|
| 0-1 | 2字节 | Width | 图像宽度（int16） |
| 2-3 | 2字节 | Height | 图像高度（int16） |
| 4-5 | 2字节 | HotX | 热点X（int16） |
| 6-7 | 2字节 | HotY | 热点Y（int16） |
| 8-11 | 4字节 | Bits | 仅 btVersion=0 时存在（通常为0，可忽略） |

**像素数据**：

- `ColorCount <= 256`：每像素1字节，为调色板索引，通过 Palette 查找 RGBA
- `ColorCount > 256`（通常65536）：每像素2字节，RGB565 格式（R5 G6 B5），无需调色板

**RGB565 解码**（`internal/wil/wil.go` 中实现）：
```
R = (pixel >> 11) << 3
G = ((pixel >> 5) & 0x3F) << 2
B = (pixel & 0x1F) << 3
pixel == 0x0000 → 透明（Alpha=0）
```

**io.ReadFull**：读取像素数据时使用 `io.ReadFull` 替代 `f.Read`，防止底层 `Read` 返回短读导致图像损坏。

**WIX 文件名兼容**：部分 WIX 文件名存在异常（如 `Deco..wix` 对应 `Deco.wil`），
加载器在标准路径找不到 WIX 时会遍历同目录下所有 `.wix` 文件，按 base name（不区分大小写、去尾部点）匹配。

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

#### 4.3.3 动画播放

动画播放逻辑直接集成在 `ui/ui.go` 的 `UIState` 中，没有独立的 `AnimationPlayer` 类型。

**动画状态**（`UIState` 字段）：

```go
AnimPlaying   bool       // 是否播放中
AnimDirection int        // 方向(0-7)
AnimAction    string     // "stand", "walk", "run", 等
AnimSpeed     float64    // 速度倍率
animFrameIdx  int        // 当前帧序列中的索引
animLastTick  float64    // glfw 时间戳（用于帧间计时）
```

**帧序列计算**（`calcAnimFrames` 函数）：

```go
func calcAnimFrames(action string, direction int, maxCount int) []int
```

- 根据 action 名查找 HumanActions/对应模板
- Frame < 8 的动作（如站立 Frame=4）不按方向分配，返回全部帧
- Frame >= 8 的动作按 8 方向分配，每方向 `Frame/8` 帧
- 生成的帧索引受 `maxCount` 约束（不超过 WIL 文件图像总数）

**播放控制**：在 UI 面板中直接处理 Play/Pause/Stop 按钮，通过 `glfw.GetTime()` 计时驱动帧切换。

#### 4.3.4 图像查看器

图像查看逻辑分布在 `renderer/renderer.go` 和 `ui/ui.go` 中：

**`renderer.WILRenderer`**：
```go
type WILRenderer struct {
    GL          *GLState
    Prog        *ShaderProgram
    textures    map[int]uint32  // 纹理缓存（index → GL texture ID）
    currentFile *wil.File
}
func (r *WILRenderer) SetWILFile(f *wil.File)  // 切换 WIL 文件，清空缓存
func (r *WILRenderer) GetOrCreateTexture(idx int) (uint32, float32, float32)
func (r *WILRenderer) ExportPNG(idx int, dir string) error
```

**`ui.UIState`**：维护当前选中图像索引、模式（browse/animation）、动画状态。

**纹理 UV 翻转**：ImGui 纹理坐标需要手动翻转——OpenGL 纹理 row 0 在底部，但 `image.RGBA` row 0 在顶部。所有 `ig.ImageButtonV` 调用使用 `uv0=(0,1), uv1=(1,0)` 而非默认的 `(0,0),(1,1)`。

#### 4.3.5 图像导出

导出功能集成在 `renderer/renderer.go` 中：

```go
func (r *WILRenderer) ExportPNG(idx int, dir string) error
```

将指定索引的图像导出为 PNG 文件到指定目录。UI 面板提供单张导出按钮。

### 4.4 界面布局（当前实现）

窗口大小：1600×1000，字体 20px，UI 缩放 1.5x。

```
┌──────────────────────────────────────────────────────────────────┐
│ (无菜单栏)                                                        │
├──────────┬───────────────────────────────┬───────────────────────┤
│          │                               │ WIL Info (右上)       │
│ Files    │   纹理网格 (中间)              │  Title: xxx           │
│ (左)     │                               │  Images: 600          │
│          │  ┌─────┐ ┌─────┐ ┌─────┐     │  Mode: ○Browse ○Anim  │
│ Data/    │  │ img0│ │ img1│ │ img2│ ... │  Index: 42            │
│ ├ Hum    │  └─────┘ └─────┘ └─────┘     │  Size: 96 x 68       │
│ ├ Items  │  ┌─────┐ ┌─────┐ ┌─────┐     │  HotX: 48 HotY: 34   │
│ ├ Mon1   │  │ img3│ │ img4│ │ img5│     │  Nav: << < 42/599 > >>│
│ └ ...    │  └─────┘ └─────┘ └─────┘     │  [Export PNG] [Export All]│
│          │       ... 滚动 ...            ├───────────────────────┤
│ 蓝=动画  │                               │ Preview (右下)        │
│ 绿=静态  │  每格 64×64 缩略图            │                       │
│ 黄=混合  │  点击选中，悬停显示尺寸        │  ┌───────────────┐    │
│ 白=未知  │                               │  │               │    │
│          │                               │  │  选中图像预览  │    │
│          │                               │  │  (自适应窗口)  │    │
│          │                               │  └───────────────┘    │
└──────────┴───────────────────────────────┴───────────────────────┘
```

**目录树颜色分类**（`wilCategory()` 函数）：

- **蓝色**：动画文件 — Hum, HumEffect, Hair, Weapon, Mon\*, Npc, Magic, Magic2, Effect, Event, Dragon
- **绿色**：静态文件 — Items, StateItem, DnItems, Prguse\*, ChrSel, mmap, Tiles, SmTiles, MagIcon
- **黄色**：混合文件 — Objects\*
- **白色**：未分类 .wil 文件

**动画模式面板**（切换到 Animation 模式时显示在右下面板内）：

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
│ ┌──────────────────────┐                                │
│ │  动画帧预览           │                                │
│ └──────────────────────┘                                │
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
func Load(wilPath string) (*File, error)

type File struct {
    Title     string         // 文件标题（ILib 标识）
    Count     int            // 图像总数
    Images    []*Image       // 图像数组
    Palette   [256]color.RGBA // 256色调色板（仅8-bit模式使用）
    BtVersion  int           // 版本标志（0=12字节图像头, 1=8字节图像头）
    ColorCount int           // 颜色数: 256→8-bit调色板, 65536→16-bit RGB565
}

type Image struct {
    Width  int
    Height int
    HotX   int16
    HotY   int16
    RGBA   *image.RGBA
}
```

**实际验证结果**：所有游戏客户端 WIL 文件均为 ILib 格式（`#ILIB`/`#INDX` 签名），图像头均为 12 字节（`BtVersion != 0`）。8-bit 文件（如 Hum.wil、Mon1.WIL）`ColorCount=256`，16-bit 文件（如 Items.wil、DnItems.wil）`ColorCount=65536`。

### 7.3 帧序列计算示例

```go
// CalcFrames 计算某动作某方向的帧序列
func CalcFrames(action ActionInfo, direction int) []int {
    dirFrames := action.Frame
    start := action.Start

    // 帧数 >= 8 时按 8 方向分配；否则所有方向共享帧序列
    if action.Frame >= 8 && direction >= 0 && direction < 8 {
        dirFrames = action.Frame / 8
        start = action.Start + direction*dirFrames
    }

    frames := make([]int, 0, dirFrames)
    for i := 0; i < dirFrames; i++ {
        if action.Skip > 0 && i%(action.Skip+1) == action.Skip {
            continue // 跳帧
        }
        frames = append(frames, start+i)
    }
    return frames
}
```

UI 中还有 `calcAnimFrames(action string, direction int, maxCount int) []int`，逻辑相同但额外接收 `maxCount` 参数限制帧索引不超过 WIL 文件图像总数。

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

| 测试项   | 测试文件 | 预期结果                |
| -------- | -------- | ----------------------- |
| 加载角色 | Hum.wil  | 正确加载600帧（8-bit）  |
| 站立动画 | Hum.wil  | 4帧循环播放（不分方向） |
| 走路动画 | Hum.wil  | 8方向各7~8帧            |
| 攻击动画 | Hum.wil  | 8方向各7~8帧            |
| 加载怪物 | Mon1.wil | 正确加载怪物图像（8-bit）|
| 怪物动画 | Mon1.wil | 按模板播放              |
| 方向切换 | 任意     | 8方向正确切换           |
| 速度调节 | 任意     | 播放速度变化            |

**已验证的文件**：

| 文件 | 格式 | ColorCount | 像素格式 | 状态 |
| ---- | ---- | ---------- | -------- | ---- |
| Hum.wil | ILib/INDX | 256 | 8-bit 调色板 | 正常 |
| Mon1.wil | ILib/INDX | 256 | 8-bit 调色板 | 正常 |
| Items.wil | ILib/INDX | 65536 | 16-bit RGB565 | 正常 |
| DnItems.wil | ILib/INDX | 65536 | 16-bit RGB565 | 正常 |
| Deco.wil | ILib/INDX | 65536 | 16-bit RGB565 | 正常（WIX 文件名为 `Deco..wix`，需模糊匹配）|

### 8.3 验证步骤

1. 编译运行：
   ```bash
   go build -o cmd/wilviewer/wilviewer.exe ./cmd/wilviewer
   ./cmd/wilviewer/wilviewer.exe asset/client/Data
   ```
2. **浏览模式测试**：
   - 左侧目录树点击 `Items.wil`（16-bit RGB565）
   - 验证缩略图网格正确显示物品图标
   - 点击任意图像，验证右下预览区显示正确（UV 翻转正常，无上下颠倒）
   - 验证右上面板显示正确的 Width/Height/HotX/HotY
3. **动画模式测试**：
   - 左侧目录树点击 `Hum.wil`（8-bit 调色板）
   - 切换右上面板 Mode 为 Animation
   - 选择 Action: stand，验证 4 帧循环播放
   - 切换 Direction，验证方向正确切换
   - 选择 Action: walk/run/attack，验证动画流畅
   - 调整 Speed 滑块，验证播放速度变化
   - 测试 `Mon1.wil`（怪物，8-bit）
4. **特殊文件测试**：
   - 点击 `Deco.wil`，验证能正确加载（WIX 文件名为 `Deco..wix`）
5. **日志验证**：
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
