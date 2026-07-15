# AGENTS.md

## 项目

用 Go 语言重新实现热血传奇（MIR2）客户端和服务端。
Module: `github.com/pyq0109/mirgo`

## 架构

```
main.go              # 入口（占位）
cmd/
├── mapviewer/       # 地图查看器（OpenGL + ImGui）
├── wilviewer/       # WIL资源查看器（OpenGL + ImGui）
├── client/          # 游戏客户端（开发中）
└── server/          # 游戏服务端（开发中）
internal/
├── mapformat/       # .map 文件解析器
├── wil/             # .wil/.wix 图像加载器
├── log/             # 分级日志
├── protocol/        # 共享协议层（✅ Phase 1 完成）
│   ├── edcode.go    # 6Bit 编解码
│   ├── message.go   # 消息类型和常量
│   └── types.go     # 共享数据结构
├── engine/          # 共享渲染引擎（待实现）
├── netclient/       # TCP 客户端库（待实现）
├── netserver/       # TCP 服务端库（待实现）
├── game/            # 共享游戏逻辑 ECS（待实现）
└── storage/         # 数据存储层（待实现）
asset/               # 已 gitignore — 游戏资源，非 Go 代码
```

## 开发计划

采用**客户端与服务端同步开发**方式，详见 `doc/客户端服务端开发计划.md`。

| 阶段 | 状态 | 内容 |
|------|------|------|
| Phase 1 | ✅ 完成 | `internal/protocol/` — 共享数据 & 协议 |
| Phase 2A | 待开始 | 客户端窗口 & 场景框架 |
| Phase 2B | 待开始 | 服务端核心基础设施 |
| Phase 3A | 待开始 | 客户端游戏场景 & 地图渲染 |
| Phase 3B | 待开始 | 服务端地图 & 世界管理 |
| Phase 4A | 待开始 | 客户端角色系统 |
| Phase 4B | 待开始 | 服务端游戏对象 & 玩家逻辑 |
| Phase 5A | 待开始 | 客户端网络层 & 输入 |
| Phase 5B | 待开始 | 服务端消息处理 & 游戏循环 |
| Phase 6A | 待开始 | 客户端战斗 & 魔法视觉 |
| Phase 6B | 待开始 | 服务端怪物AI & 战斗系统 |
| Phase 7A | 待开始 | 客户端UI系统 |
| Phase 7B | 待开始 | 服务端NPC & 脚本系统 |
| Phase 8A | 待开始 | 客户端打磨 & 音效 |
| Phase 8B | 待开始 | 服务端进阶系统 & 优化 |

## 现代架构设计原则

1. **接口驱动设计** — 核心系统通过接口交互，便于测试和替换
2. **消息总线架构** — 客户端和服务端内部使用 channel-based 消息总线
3. **组件化实体系统 (ECS)** — 使用组合模式而非深度继承
4. **依赖注入** — 核心服务通过构造函数注入，便于单元测试
5. **优雅关闭** — 所有 goroutine 通过 context 和 channel 协调关闭

## 地图查看器

编译运行参见 README.md 的"编译运行 mapviewer"章节。

## WIL资源查看器

用于查看热血传奇游戏专用的 .wil/.wix 图像资源文件，支持静态图像浏览和动画播放两种模式。

**编译运行**：
```bash
go build -o cmd/wilviewer/wilviewer.exe ./cmd/wilviewer
./cmd/wilviewer/wilviewer.exe asset/client/Data
```

**功能**：
- 左侧目录树：列出目录中的所有 .wil 文件，点击切换查看，按类型四色着色（蓝=动画、绿=静态、黄=混合、白=未知）
- 中间纹理网格：以 64×64 缩略图网格展示 WIL 文件中的所有纹理，点击选中，悬停显示尺寸信息
- 右上文件信息：显示文件标题、图像数量、当前索引/尺寸/热点、导航按钮（`<<`到首/`<`前一/`>`后一/`>>`到末）、导出按钮
- 右下预览/动画：浏览模式下显示选中图像（自适应窗口大小，保持比例）；动画模式下显示动作/方向/播放控制和动画预览
- 图像导航：使用箭头键或按钮浏览图像
- 图像导出：单张或批量导出为 PNG 格式
- Debug日志：集成 `internal/log`，关键操作均有日志输出

**架构**：
- 复用 mapviewer 的 OpenGL + ImGui 架构
- 复用 `internal/wil/` 包加载 WIL 文件
- `cmd/wilviewer/main.go` — 主程序、窗口创建、主循环
- `cmd/wilviewer/action.go` — 动作模板定义（人类7种、怪物、NPC）
- `cmd/wilviewer/animation.go` — 动画播放器（Play/Pause/Stop、方向、速度）
- `cmd/wilviewer/renderer/` — OpenGL 渲染（纹理缓存、SetWILFile 热切换、PNG 导出）
- `cmd/wilviewer/ui/` — ImGui 界面（左侧目录树、中间纹理网格、右上信息面板、右下预览/动画面板）
- 支持两种模式切换（浏览/动画）

## 资源目录（已 gitignore — 需手动准备）

`asset/` 不纳入版本管理，需手动下载填充：

1. `asset/client/` — 客户端美术资源，来自热血传奇十周年硬盘版
2. `asset/server/` — 服务端配置，来自 `github.com/cjlaaa/Mir2-GeeM2`
3. `asset/delphi/` — 原始 Delphi 源码，来自 `github.com/lzxsz/MIR2`（commit `98711da`）

Delphi 源码是 Go 重写的主要参考。
服务端关键组件：`M2Server/`、`DBServer/`、`LoginSrv/`、`LoginGate/`、`RunGate/`、`SelGate/`。
客户端关键组件：`Client/`、`MirClient/`。

## 约束

- `go.sum` 已 gitignore — 添加依赖后需运行 `go mod tidy`
- 尚无 CI、linter 或测试基础设施
- 无 Makefile、Dockerfile 或构建脚本
- `asset/` 目录禁止提交（含二进制和大文件）

## Delphi 源码关键发现

### 客户端架构

**渲染管线**：基于 DirectX 7，通过 DelphiX 组件库封装，核心类 `TDXDraw` 提供双缓冲。

**场景状态机** (IntroScn.pas)：
- stIntro → stLogin → stSelectChr → stLoginNotice → stPlayGame

**地图三层渲染** (PlayScn.pas)：
| 层 | 数据字段 | 资源文件 |
|---|---|---|
| 背景层 | wBkImg | Tiles.wil |
| 中间层 | wMidImg | SmTiles.wil |
| 前景层 | wFrImg | Objects.wil |

**光照系统**：6级预计算光罩 (LightMask0~5)，从 `Data/lig0a~f.dat` 加载。

### WIL 文件分类（动画 vs 静态）

**核心原则：WIL 文件本身不存储动画/静态元数据，直接按文件名分类即可。**

WIL 文件只是"图片帧的有序集合"，是否构成动画由外部代码决定。

**动画文件**（11类）：
- Hum.wil — 人类角色外观（每套装备 600 帧，8方向×75帧）
- HumEffect.wil — 角色翅膀/特效
- Hair.wil — 角色发型
- Weapon.wil — 武器外观
- Mon1.wil ~ Mon18.wil — 怪物图像（每种 280/360/440 帧）
- Npc.wil — NPC 图像（每个 60 帧）
- Magic.wil, Magic2.wil — 技能特效
- Effect.wil — 通用特效
- Event.wil — 事件特效
- Dragon.wil — 龙形怪物

**静态文件**（11类）：
- Items.wil — 背包物品图标
- StateItem.wil — 装备状态图标
- DnItems.wil — 地面物品图标
- Prguse.wil, Prguse2.wil, Prguse3.wil — UI 素材
- ChrSel.wil — 角色选择界面
- mmap.wil — 小地图素材
- Tiles.wil — 地图图块（48×32）
- SmTiles.wil — 小地图图块
- MagIcon.wil — 技能图标

**混合文件**（1类）：
- Objects.wil ~ ObjectsN.wil — 地图物体（通过 btAniFrame 字段区分）

### 动画帧区分机制

1. **TActionInfo 硬编码表**（角色/怪物）：
   - 定义在 `Client/Actor.pas`
   - 帧计算公式：`实际帧 = start + 方向 × (frame + skip) + 当前帧偏移`
   - 人类动作模板 HA（站立、行走、跑步、攻击、施法、死亡等）
   - 怪物动作模板 MA9~MA47（数十种变体）

2. **btAniFrame 字段**（地图物体）：
   - 存储在地图文件中
   - `= 0` → 静态图像
   - `> 0` → 动画帧数（最高位 0x80 表示透明/混合绘制）

3. **AniCount 字段**（物品）：
   - 存储在物品结构 TStdItem 中
   - 表示动画帧数（通常为 0，表示静态）

### 地图文件格式

**文件头** (52字节)：Width, Height, Title[16], UpdateDate, Reserved[22]

**每格数据** (10字节)：
- wBkImg (Word) — 背景图索引（最高位 0x8000 标志不可移动）
- wMidImg (Word) — 中间层图索引
- wFrImg (Word) — 前景层图索引
- btDoorIndex (Byte) — 门索引（0x80 标志暗门）
- btDoorOffset (Byte) — 门偏移
- btAniFrame (Byte) — 动画帧数（0x80 标志 Alpha 绘制）
- btAniTick (Byte) — 动画间隔
- btArea (Byte) — 区域标识（选择 Objects{N+1}.wil）
- btLight (Byte) — 光照等级（0~4）

**存储方式**：列优先（先存第1列所有行，再存第2列...）

### 网络协议

**消息结构**：
```
[TMsgHeader 16字节] + [TDefaultMessage 12字节] + [消息体(可选)]

TMsgHeader (Grobal2.pas:1036):
  dwCode        : DWord   — 魔数 0xAA55AA55 (RUNGATECODE)
  nSocket       : Integer — 客户端 Socket 标识
  wGSocketIdx   : Word    — 网关 Socket 索引
  wIdent        : Word    — 消息类型 (GM_OPEN/GM_CLOSE/GM_DATA)
  wUserListIndex: Word    — 用户列表索引
  nLength       : Integer — 后续数据长度

TDefaultMessage (经6Bit编码后16字符):
  Recog : Integer (4字节) — 认证/识别码
  Ident : Word    (2字节) — 消息ID
  Param : Word    (2字节) — 参数1
  Tag   : Word    (2字节) — 参数2
  Series: Word    (2字节) — 参数3
```

**编码方式**：6Bit 编码（Common/EDcode.pas），每3字节→4字符，偏移 0x3C。

**消息前缀**：`CM_` 客户端→服务端，`SM_` 服务端→客户端。

**消息 ID 范围**（约300个，定义在 Grobal2.pas）：
- 100~104：角色查询/创建/删除/选择
- 1000~1034：游戏内操作（拾取/丢弃/穿戴/交易/组队/行会）
- 3010~3030：战斗动作（走/跑/攻击/施法）
- 500~533：登录认证响应
- 600~772：物品/地图/UI状态同步

**Go 实现**：`internal/protocol/` 包已完成所有消息类型和编解码函数。

### 服务端架构

服务端由 8 个分布式进程组成，通过 TCP/UDP 通信：

**玩家连接路径**：
```
客户端 → LoginGate(7000) → LoginSrv(5500) → 账号验证
客户端 → SelGate(7100) → DBServer(5100/6000) → 角色查询/选择
客户端 → RunGate(7200~7900) → M2Server(5000) → 游戏逻辑
```

**M2Server 三层引擎**：
1. **TFrontEngine** (FrnEngn.pas) — 前端引擎，异步读写角色存档
2. **TUserEngine** (UsrEngn.pas) — 用户引擎，游戏世界核心驱动
3. **TRunSocket** (RunSock.pas) — 网络引擎，管理 RunGate 连接

**对象继承体系** (ObjBase.pas，26821行)：
```
TBaseObject — 所有游戏对象基类
  ├── TAnimalObject — 有生命的对象
  │     ├── TPlayObject — 玩家角色
  │     ├── TMonster — 基础怪物 (ObjMon.pas)
  │     ├── TGuard — 卫兵
  │     └── TNormNpc → TMerchant — NPC/商人
  └── TRobotObject — 机器人NPC
```

**关键系统**：
- 地图管理：TMapManager (Envir.pas)，TEnvirnoment 加载 .map 文件
- 魔法系统：TMagicManager (Magic.pas)，DoSpell 方法分发魔法
- 物品系统：TItem (ItmUnit.pas)，支持随机升级
- 行会系统：TGuild (Guild.pas)，文件存储
- 城堡系统：TUserCastle (Castle.pas)，沙巴克攻城战
- NPC脚本引擎：TSayingRecord (ObjNpc.pas)，条件-动作脚本

### 关键数据结构

**方向常量**：DR_UP(0), DR_UPRIGHT(1), DR_RIGHT(2), DR_DOWNRIGHT(3), DR_DOWN(4), DR_DOWNLEFT(5), DR_LEFT(6), DR_UPLEFT(7)

**网格常量**：UNITX=48, UNITY=32, LOGICALMAPUNIT=40

**装备槽位** (10个)：U_DRESS(0), U_WEAPON(1), U_RIGHTHAND(2), U_HELMET(3), U_NECKLACE(4), U_ARMRINGL(5), U_ARMRINGR(6), U_RINGL(7), U_RINGR(8), U_BUJUK(9)

**物品类型** TStdItem (60字节)：Name[20], StdMode, Shape, Weight, Looks, DuraMax, AC/MAC/DC/MC/SC, Need/NeedLevel, Price

**角色存档** THumDataInfo (3628字节)：基础信息 + 装备[0..9] + 背包[0..45] + 技能[0..19] + 任务标志 + 行会信息

### 配置文件体系

位于 `asset/server/`：
| 文件 | 用途 |
|---|---|
| !setup.txt | 全局配置（IP、端口、窗口位置） |
| String.ini | 服务端提示文本（210条） |
| Exps.ini | 经验表配置 |
| Command.ini | 命令配置 |
| BaseAbil.ini | 基础属性配置 |

### 关键参考文件

**客户端**：
- `Client/ClMain.pas` — 主窗体、游戏主循环
- `Client/DrawScrn.pas` — 场景切换、顶层绘制
- `Client/PlayScn.pas` — 地图渲染、对象管理
- `Client/Actor.pas` — 角色基类、动画结构
- `Client/WIL.pas` — WIL 文件加载器
- `Client/Share.pas` — WIL 文件路径常量

**服务端**：
- `M2Server/ObjBase.pas` — 游戏对象基类（26821行，最大文件）
- `M2Server/UsrEngn.pas` — 用户引擎
- `M2Server/FrnEngn.pas` — 前端引擎
- `M2Server/RunSock.pas` — 网络引擎
- `M2Server/Envir.pas` — 地图管理
- `M2Server/Magic.pas` — 魔法系统
- `M2Server/ObjNpc.pas` — NPC脚本引擎（11556行）

**公共模块**：
- `Common/Grobal2.pas` — 消息定义、数据结构（2739行）
- `Common/EDcode.pas` — 6Bit 编解码
- `Common/HUtil32.pas` — 通用工具函数（2100行）
- `Common/MudUtil.pas` — 快速索引结构
- `SDK/SDK.pas` — 线程安全列表类

## 已实现的 Go 模块

### internal/protocol（Phase 1 ✅）

共享协议层，客户端和服务端共同使用：

- **edcode.go**：6Bit 编解码算法
  - `Encode6BitBuf` / `Decode6BitBuf` — 核心算法
  - `EncodeMessage` / `DecodeMessage` — 消息结构体编解码
  - `EncodeString` / `DecodeString` — 字符串编解码
  - `EncodeBuffer` / `DecodeBuffer` — 缓冲区编解码
  - `FormatClientFrame` / `FormatServerFrame` — 帧格式化

- **message.go**：消息类型和常量
  - `DefaultMessage` — 核心消息结构体 (12字节)
  - `MsgHeader` — RunGate 帧头 (16字节)
  - 所有 `CM_*` / `SM_*` / `SS_*` / `DB_*` 常量
  - 控制消息前缀 (`+GOOD`, `+FAIL` 等)

- **types.go**：共享数据结构
  - `StdItem` (60字节) — 物品定义
  - `UserItem` (24字节) — 物品实例
  - `Ability` (50字节) — 角色属性
  - `Magic` — 魔法定义
  - Feature 编码/解码函数

### internal/mapformat（已完成）

.map 文件解析器：
- 支持 12/14/20 字节单元格格式
- 列优先到行优先转换
- 三层数据：背景、中间、前景
- 碰撞检测（wBkImg bit 15）
- 有单元测试

### internal/wil（已完成）

.wil/.wix 图像加载器：
- 支持标准和 ILib (#ILIB/#INDX) 格式
- 两种图像头版本：12字节 (btVersion=0) 和 8字节 (btVersion=1)
- 两种像素格式：8位调色板 和 16位 RGB565
- 返回 decoded `*image.RGBA`

### internal/log（已完成）

分级日志：
- 5个级别：TRACE, DEBUG, INFO, WARN, ERROR
- 基于 Tag 的日志记录
- 时间戳输出到 stderr
