# 热血传奇1.5 Delphi 源码深度架构分析

## 一、整体分布式架构

整个系统由8个独立进程组成，通过TCP/UDP网络通信协同工作。

### 1.1 核心通信链路

**玩家连接路径：**

```
客户端 → LoginGate(7000) → LoginSrv(5500) → 账号验证
客户端 → SelGate(7100) → DBServer(5100/6000) → 角色查询/选择
客户端 → RunGate(7200~7900) → M2Server(5000) → 游戏逻辑
```

**服务器间通信：**

- M2Server ↔ DBServer：角色存档读写
- M2Server ↔ LoginSrv：会话状态上报
- M2Server ↔ M2Server：跨服消息（私聊、行会战等）
- LogDataServer：UDP(10000) 接收全系统日志
- GameCenter：启动器/管理器，负责拉起和监控所有进程

### 1.2 配置文件体系

位于 `asset/server/`：

| 文件          | 用途                                       |
| ------------- | ------------------------------------------ |
| !setup.txt    | 全局配置，定义所有组件的IP、端口、窗口位置 |
| String.ini    | 服务端发给客户端的全部提示文本(210条)      |
| Exps.ini      | 经验表配置                                 |
| Command.ini   | 命令配置                                   |
| BaseAbil.ini  | 基础属性配置                               |
| GlobalVal.ini | 全局变量配置                               |
| SendSMS.ini   | 短信发送配置                               |
| 系统插件.ini  | 系统插件配置                               |

---

## 二、客户端核心模块

客户端主程序入口是 `mir2.dpr`，创建三个窗体：

- 主窗体 `TfrmMain` (ClMain.pas)
- UI窗体 `TFrmDlg` (FState.pas)
- 配置窗体 `TfrmDlgConfig` (DlgConfig.pas)

### 2.1 渲染管线

渲染基于 DirectX 7，通过 DelphiX 组件库封装。核心类是 `TDXDraw`，提供双缓冲 DirectDrawSurface。

**场景状态机** (IntroScn.pas)：

| 状态          | 说明     |
| ------------- | -------- |
| stIntro       | 启动画面 |
| stLogin       | 登录场景 |
| stSelectChr   | 选角场景 |
| stLoginNotice | 公告场景 |
| stPlayGame    | 游戏中   |

`TDrawScreen` (DrawScrn.pas) 管理场景切换和顶层绘制（聊天栏、系统消息、FPS）。

### 2.2 地图渲染

在 PlayScn.pas 中实现，地图采用三层结构：

| 层     | 数据字段 | 资源文件    |
| ------ | -------- | ----------- |
| 背景层 | wBkImg   | Tiles.wil   |
| 中间层 | wMidImg  | SmTiles.wil |
| 前景层 | wFrImg   | Objects.wil |

每层都是从 WIL 文件中索引的 48x32 像素图块。

**光照系统**：使用6级预计算光罩(LightMask0~5)，从 `Data/lig0a~f.dat` 加载。

### 2.3 角色动画

定义在 Actor.pas 中。`TActionInfo` 记录每个动作的起始帧、帧数、跳帧、帧间隔。

**帧数规格**：

- 人类角色：每套装备每个方向 600 帧（8方向 × 每方向约75帧）
- 怪物：根据类型有不同帧数（MA9~MA22 共14种动作模板）

### 2.4 图像资源系统 (WIL/WIX)

WIL 文件是热血传奇游戏专用的图像打包格式。

**文件头结构** (wmUtil.pas)：

```
TWMImageHeader (56字节):
  Title[40]       — "WEMADE Entertainment inc."
  ImageCount      — 图像总数
  ColorCount      — 颜色数
  PaletteSize     — 调色板大小
  VerFlag         — 版本标志(用于加密校验)
```

WIX 文件是对应的索引文件，每个条目记录图像在WIL中的偏移和大小。

`TWMImages` 类 (WIL.pas) 管理加载流程：

1. 读取WIL头
2. 加载调色板(256色)
3. 读取WIX索引
4. 按需加载图像到 DirectDrawSurface

**客户端加载的WIL资源清单** (Share.pas)：

| 资源文件                                | 用途               |
| --------------------------------------- | ------------------ |
| Prguse.wil / Prguse2.wil / Prguse3.wil  | 主界面UI素材       |
| ChrSel.wil                              | 选角界面           |
| Hum.wil / Hair.wil / Weapon.wil         | 角色外观           |
| Mon1~28.wil                             | 怪物图像(28个文件) |
| Items.wil / DnItems.wil / StateItem.wil | 物品图标           |
| Magic.wil / Magic2.wil / Effect.wil     | 魔法特效           |
| Npc.wil                                 | NPC外观            |
| Tiles.wil / SmTiles.wil / Objects.wil   | 地图图块           |
| mmap.wil                                | 小地图             |

### 2.5 地图文件格式

地图格式在 MapUnit.pas 和 Envir.pas 中均有定义：

**文件头** (52字节)：

```
TMapHeader:
  wWidth      : Word        — 地图宽度(格子数)
  wHeight     : Word        — 地图高度(格子数)
  sTitle[16]  : String      — 地图标题
  UpdateDate  : TDateTime   — 更新日期
  Reserved[22]: Char        — 保留
```

**每格数据** (10字节)：

```
TMapInfo:
  wBkImg       : Word   — 背景图索引(最高位$8000标志不可移动)
  wMidImg      : Word   — 中间层图索引
  wFrImg       : Word   — 前景层图索引
  btDoorIndex  : Byte   — 门索引($80标志暗门)
  btDoorOffset : Byte   — 门偏移
  btAniFrame   : Byte   — 动画帧数($80标志Alpha绘制)
  btAniTick    : Byte   — 动画间隔
  btArea       : Byte   — 区域标识
  btLight      : Byte   — 光照等级(0~4)
```

**存储方式**：列优先（先存第1列所有行，再存第2列...）

### 2.6 网络通信

客户端通过 `JSocket`（自定义Socket组件）与 RunGate 通信。

**消息结构** (16字节)：

```
TDefaultMessage:
  Recog  : Integer  (4字节) — 认证/识别码
  Ident  : Word     (2字节) — 消息ID
  Param  : Word     (2字节) — 参数1
  Tag    : Word     (2字节) — 参数2
  Series : Word     (2字节) — 参数3
```

**编码方式**：6Bit编码（Common/EDcode.pas）

- 将每3个字节编码为4个6Bit字符
- 使用查表法 EncodeBitMasks/DecodeBitMasks
- 附加消息体（字符串用同样方式编码）

**消息前缀约定**：

- `CM_` 前缀：客户端发往服务端
- `SM_` 前缀：服务端发往客户端

**关键消息分类** (Grobal2.pas，约300个消息ID)：

| 消息ID范围 | 用途                                               |
| ---------- | -------------------------------------------------- |
| 100~104    | 角色查询/创建/删除/选择/选服                       |
| 1000~1034  | 游戏内操作（拾取/丢弃/穿戴/使用/交易/组队/行会等） |
| 3010~3030  | 战斗动作（走/跑/攻击/施法/说话）                   |
| 500~533    | 登录认证响应                                       |
| 600~772    | 物品/地图/UI状态同步                               |

---

## 三、服务端核心模块

### 3.1 M2Server — 游戏主服务器

这是最庞大的组件，由97个源文件组成。

#### 核心引擎架构（三层）

**前端引擎 `TFrontEngine`** (FrnEngn.pas)

- 独立线程运行
- 负责异步读写角色存档
- 维护 `m_LoadRcdList`（待加载）和 `m_SaveRcdList`（待保存）两个队列
- 通过 DBSocket 与 DBServer 交互

**用户引擎 `TUserEngine`** (UsrEngn.pas)

- 游戏世界的核心驱动
- 管理：`StdItemList`(物品数据库)、`MonsterList`(怪物数据库)、`m_MagicList`(技能数据库)、`m_MerchantList`(商人NPC)、`QuestNPCList`(任务NPC)、`m_MonGenList`(刷怪点)
- 每帧依次处理：ProcessHumans → ProcessMonsters → ProcessMerchants → ProcessNpcs → ProcessEvents

**网络引擎 `TRunSocket`** (RunSock.pas)

- 管理与 RunGate 的连接
- 支持最多20个网关连接(g_GateArr)
- 使用 `TMsgHeader` 格式打包消息

**消息头格式**：

```
TMsgHeader:
  nLength   : Integer  — 后续数据长度
  nGateIdx  : Integer  — 网关索引
  nSocket   : Integer  — Socket标识
  nSessionID: Integer  — 会话ID
```

#### 对象继承体系 (ObjBase.pas，26821行，最大文件)

```
TBaseObject — 所有游戏对象基类
  ├── TAnimalObject — 有生命的对象
  │     ├── TPlayObject — 玩家角色
  │     ├── TMonster — 基础怪物 (ObjMon.pas)
  │     │     ├── TChickenDeer, TATMonster, TSpitSpider...
  │     │     └── (ObjMon2.pas, ObjMon3.pas 更多怪物变体)
  │     ├── TGuard — 卫兵 (ObjGuard.pas)
  │     ├── TArcherGuard — 弓箭守卫 (ObjAxeMon.pas)
  │     └── TNormNpc → TMerchant — NPC/商人 (ObjNpc.pas, 11556行)
  └── TRobotObject — 机器人NPC (ObjRobot.pas)
```

**TBaseObject 约200个属性字段**：

- 坐标、方向、性别、职业、等级
- HP/MP、攻击/防御/魔法属性
- 装备栏(13个槽位)、背包(最多46格)
- 技能列表(最多20个)、状态效果数组
- 组队/行会/城堡关联

#### 地图管理 `TMapManager` (Envir.pas)

`TEnvirnoment` 加载 .map 文件，维护 `MapCellArray`（二维格子数组）：

- 每个格子有 `ObjList` 存放该格上的所有对象
- 支持传送门(AddMapRoute)、门系统(m_DoorList)、区域标志(TMapFlag)

#### 魔法系统 `TMagicManager` (Magic.pas, 1560行)

`DoSpell` 方法根据魔法ID分发到具体实现：

- 火球术、雷电术、治愈术、施毒术
- 召唤术、圣言术、隐身术
- 火墙、爆裂火焰等

伤害公式使用 `GetPower`/`GetPower13` 函数，基于魔法等级和训练等级计算。

#### 物品系统 (ItmUnit.pas)

`TItem` 类定义物品基础属性：

- 名称、类型、重量、外观、耐久
- 攻击/防御/魔法属性（AC/MAC/DC/MC/SC）
- 需求类型和等级（Need/NeedLevel）
- 价格

支持随机升级 `RandomUpgradeItem`，根据物品类型（武器/防具/饰品）有不同的属性随机规则。

`TUserItem` 记录物品实例的具体数值：

- `btValue[0..13]` 用于存放随机附加属性

#### 行会系统 `TGuild` (Guild.pas)

行会数据以文件方式存储（非数据库）：

- 成员列表、等级制度（军衔）
- 公告、战争列表、同盟列表
- 支持行会战(TeamFight)和城堡争夺

#### 城堡系统 `TUserCastle` (Castle.pas)

沙巴克攻城战逻辑：

- 管理城门、城墙（左/中/右）
- 弓箭手（最多12个）、守卫（最多4个）
- 支持攻城预约、攻城战计时、税收系统

#### NPC脚本引擎 (ObjNpc.pas)

NPC对话采用条件-动作脚本系统：

- `TSayingRecord` 包含标签和程序列表
- 每个 `TSayingProcedure` 有：
  - `ConditionList`（条件链）
  - `ActionList`（动作链）
  - `sSayMsg`（对话文本）
- 支持标签跳转和外部跳转(boExtJmp)

### 3.2 DBServer — 数据库服务器

负责角色数据的持久化存储。

**核心文件** HumDB.pas 定义了两套数据库：

| 数据库 | 管理类     | 存储内容                         |
| ------ | ---------- | -------------------------------- |
| Hum.DB | TFileHumDB | 角色-账号映射（THumInfo记录）    |
| Mir.DB | TFileDB    | 角色详细数据（THumDataInfo记录） |

**文件头结构** (128字节)：

```
TDBHeader:
  sDesc[39]      — 文件描述标识
  nLastIndex     — 最后操作索引
  dLastDate      — 最后操作时间
  nHumCount      — 角色总数
  dUpdateDate    — 更新日期
```

**快速索引**：

- `TQuickList`：带线程安全的排序字符串列表，支持二分查找
- `TQuickIDList`：按账号索引角色的快速查找结构

数据库操作使用临界区(CriticalSection)保证线程安全，支持记录删除回收(m_DeletedList)。

### 3.3 LoginSrv — 登录服务器

负责账号验证和会话管理。

**核心文件**：

- `IDDB.pas` — 账号数据库格式定义
- `LMain.pas` — 主窗体，处理网关连接和账号验证请求
- `MasSock.pas` — 与 DBServer/M2Server 的通信
- `MonSoc.pas` — 监控连接
- `LSShare.pas` — 共享变量和配置

账号数据库与 Hum.DB 结构类似（128字节文件头 + 索引文件），存储 `TAccountDBRecord` 包含账号、密码、密保、注册信息等。

### 3.4 网关组件 (Gate)

三个网关结构高度相似，都是透明转发代理：

| 网关      | 端口      | 功能                                 |
| --------- | --------- | ------------------------------------ |
| LoginGate | 7000      | 客户端 ↔ LoginSrv 之间的转发         |
| SelGate   | 7100      | 客户端 ↔ DBServer 之间的选角请求转发 |
| RunGate   | 7200~7900 | 客户端 ↔ M2Server 之间的游戏数据转发 |

**RunGate 特有功能**：

- 消息过滤 (MessageFilterConfig.pas)
- 带宽控制 (PrefConfig.pas)

每个网关都包含：

- `GateShare.pas` — 共享配置
- `GeneralConfig.pas` — 通用配置UI
- `IPaddrFilter.pas` — IP过滤
- `JSocket.pas` — Socket组件

---

## 四、公共模块 (Common/ 和 SDK/)

### 4.1 共享数据结构 (Grobal2.pas)

Common/Grobal2.pas 是**客户端和服务端共用的核心文件**（2739行）。

**方向常量**：

```
DR_UP(0), DR_UPRIGHT(1), DR_RIGHT(2), DR_DOWNRIGHT(3)
DR_DOWN(4), DR_DOWNLEFT(5), DR_LEFT(6), DR_UPLEFT(7)
```

**装备槽位** (1.50版10个槽位)：

```
U_DRESS(0), U_WEAPON(1), U_RIGHTHAND(2)
U_HELMET(3), U_NECKLACE(4), U_ARMRINGL(5)
U_ARMRINGR(6), U_RINGL(7), U_RINGR(8), U_BUJUK(9)
```

**网格常量**：

```
UNITX = 48          // 等距网格像素宽度
UNITY = 32          // 等距网格像素高度
LOGICALMAPUNIT = 40 // 逻辑地图单元大小
```

**物品类型** TStdItem (60字节)：

```
Name[20]      — 名称
StdMode       — 类型
Shape         — 外形
Weight        — 重量
Looks         — 外观图ID
DuraMax       — 最大耐久
AC/MAC/DC/MC/SC — 防御/魔防/攻击/魔法/道术（各为双字，低字基础值高字附加值）
Need/NeedLevel — 需求类型和等级
Price         — 价格
```

**角色能力** TAbility：

- 等级、HP/MP/最大HP/最大MP
- 经验、攻击/魔法/道术上下限
- 防御/魔防上下限、负重、腕力

**客户端物品** TClientItem：

- TStdItem + MakeIndex(制造序号) + Dura/DuraMax(当前/最大耐久)

**用户状态** TUserStateInfo：

- 用于查看其他玩家信息
- 包含外观、名称、行会、装备数组[0..12]

**数据库协议常量**：

```
DB_LOADHUMANRCD  = 100
DB_SAVEHUMANRCD  = 101
DBR_LOADHUMANRCD = 1100
DBR_SAVEHUMANRCD = 1102
```

### 4.2 编解码 (EDcode.pas)

Common/EDcode.pas 实现 6Bit 编码算法，是网络通信的核心。

**编码方式**：

- 将每3个字节编码为4个6Bit字符
- 使用查表法 EncodeBitMasks/DecodeBitMasks（各256字节）
- 用于消息传输（TDefaultMessage + 消息体）

### 4.3 工具库 (HUtil32.pas)

Common/HUtil32.pas（2100行）是通用工具函数库：

- 字符串处理：ArrestString/CompareLStr/CaptureString
- 文件操作
- 内存操作：memset/memcpy
- 位图处理、颜色转换

### 4.4 快速索引 (MudUtil.pas)

Common/MudUtil.pas 定义了：

- `TQuickList`：带线程安全的排序字符串列表，支持二分查找
- `TQuickIDList`：按账号索引角色的快速查找结构

这两个类是数据库查询的核心数据结构。

### 4.5 SDK.pas

SDK/SDK.pas（86行）定义：

- `TGList` — 带临界区锁的线程安全列表类
- `TGStringList` — 带临界区锁的线程安全字符串列表类

这是服务端多线程安全的基础组件。

---

## 五、关键算法和数据格式总结

### 5.1 网络协议格式

```
完整消息结构：
[TMsgHeader] + [TDefaultMessage] + [消息体(可选)]

TMsgHeader (16字节):
  nLength   : Integer  — 后续数据长度
  nGateIdx  : Integer  — 网关索引
  nSocket   : Integer  — Socket标识
  nSessionID: Integer  — 会话ID

TDefaultMessage (16字节，经6Bit编码后约22字符):
  Recog:Integer + Ident:Word + Param:Word + Tag:Word + Series:Word
```

### 5.2 角色存档结构

`THumDataInfo` (SIZEOFTHUMAN = 3628字节)：

- 角色基础信息：名称/职业/性别/等级/HP/MP/经验/金币/坐标
- 装备数组[0..9]：TUserItem
- 背包数组[0..45]：TUserItem
- 技能数组[0..19]：TUserMagic
- 任务标志、行会信息

### 5.3 WIL图像格式

```
[WEMADE头 56字节]
[调色板 256*3字节 RGB]
[图像数据区]
  每张图像：nWidth*2 + nHeight*2 + px*2 + py*2 + 像素数据(8bit索引色)

WIX索引文件：每个条目8字节(Position:Integer + Size:Integer)
```

### 5.4 地图格式

- 列优先存储
- 每格10字节
- 最高位标志位用于控制移动/动画/门等

### 5.5 光照系统

6级预计算光罩(LightMask0~5)，尺寸从3x3到17x17不等。值0~4表示光照强度，运行时与调色板混合计算最终颜色。

---

## 六、附录：关键文件索引

### 客户端关键文件

| 文件         | 行数     | 职责                 |
| ------------ | -------- | -------------------- |
| ClMain.pas   | 主窗体   | 游戏主循环、输入处理 |
| DrawScrn.pas | 绘屏管理 | 场景切换、顶层绘制   |
| PlayScn.pas  | 游戏场景 | 地图渲染、对象管理   |
| Actor.pas    | 角色基类 | 动画、移动、状态     |
| FState.pas   | UI窗体   | 背包、装备、技能界面 |
| WIL.pas      | WIL管理  | 图像资源加载         |
| EDcode.pas   | 编解码   | 6Bit编码算法         |
| Grobal2.pas  | 共享定义 | 消息ID、数据结构     |

### 服务端关键文件

| 文件        | 行数       | 职责                     |
| ----------- | ---------- | ------------------------ |
| ObjBase.pas | 26821      | 游戏对象基类（最大文件） |
| UsrEngn.pas | 用户引擎   | 游戏世界核心驱动         |
| FrnEngn.pas | 前端引擎   | 异步存档                 |
| RunSock.pas | 网络引擎   | RunGate连接管理          |
| Envir.pas   | 地图管理   | 地图实例、格子数组       |
| Magic.pas   | 1560       | 魔法系统                 |
| ItmUnit.pas | 物品系统   | 物品定义、随机升级       |
| Guild.pas   | 行会系统   | 行会数据、战争           |
| Castle.pas  | 城堡系统   | 沙巴克攻城战             |
| ObjNpc.pas  | 11556      | NPC脚本引擎              |
| HumDB.pas   | 角色数据库 | Hum.DB、Mir.DB管理       |
