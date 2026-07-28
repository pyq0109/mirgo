# Delphi 版 NPC 系统实现

> 基于 `asset/delphi/` 源码（commit `98711da`）的完整技术描述。
> 所有引用格式为 `文件:行号`。

## 一、架构总览

MIR2 的 NPC 系统是一套**脚本驱动 + 继承多态 + Race 工厂**结构：NPC 的对话和行为完全由外部脚本文件（`.txt`）定义，运行时由脚本引擎解释执行；不同 NPC 类型通过类继承和方法覆写实现特化；NPC 类由数据库 `btRace` 字段在创建时工厂选择。

```
┌────────────────────────────────────────────────────────────────────────────┐
│  引擎调度层 TUserEngine (UsrEngn.pas)                                      │
│    AddBaseObject()     Race ID → 类工厂（case 语句）                        │
│    FindMerchant()      按对象引用查找商人                                   │
│    FindNPC()           按对象引用查找 NPC                                   │
├────────────────────────────────────────────────────────────────────────────┤
│  对象基类层 (ObjBase.pas)                                                   │
│    TBaseObject          消息队列、HP/MP、战斗公式、死亡                      │
│    └── TAnimalObject    视野搜索、移动基础                                  │
│         └── TNormNpc    NPC 基类：脚本引擎、对话、标签跳转 (ObjNpc.pas:78)   │
│              ├── TMerchant       商店：买卖/修理/仓库/升级 (ObjNpc.pas:280)  │
│              │    └── TCastleOfficial  城堡管理 (ObjNpc.pas:390)            │
│              ├── TGuildOfficial  行会管理 (ObjNpc.pas:360)                  │
│              └── TTrainer        训练师 (ObjNpc.pas:377)                    │
├────────────────────────────────────────────────────────────────────────────┤
│  脚本引擎层 (ObjNpc.pas)                                                    │
│    LoadNpcScript()     加载脚本文件（Npc_def/ 或 Market_def/）              │
│    GotoLable()         核心执行引擎：标签查找 → 条件求值 → 动作执行         │
│    QuestCheckCondition()  #IF 条件求值（~66 种命令）                        │
│    QuestActionProcess()   #ACT 动作执行（~73 种命令）                       │
│    GetVariableText()      <$VARIABLE> 文本替换                              │
├────────────────────────────────────────────────────────────────────────────┤
│  客户端渲染层 (Client/)                                                     │
│    TNpcActor (Actor.pas:760)    NPC 精灵加载、3 方向动画、特效层            │
│    TFrmDlg (FState.pas)         对话窗口、商品列表、出售/修理/仓库面板      │
│    TfrmMain (ClMain.pas)        消息分发、CM 消息发送、NPC 点击检测         │
└────────────────────────────────────────────────────────────────────────────┘
```

### 核心设计原则

1. **脚本驱动**：NPC 的对话内容、条件分支、动作效果全部由外部 `.txt` 脚本文件定义。服务端只负责解析和执行，不硬编码任何对话内容。脚本使用 `[@label]` 分段、`#IF/#ACT/#SAY` 结构化命令。
2. **继承多态**：NPC 类型差异通过覆写 `Click()`、`UserSelect()`、`Run()`、`GetVariableText()` 等虚方法实现。`TMerchant` 在 `TNormNpc` 基础上增加商品管理，`TCastleOfficial` 在 `TMerchant` 基础上增加城堡管理。
3. **Race 工厂**：NPC 创建时由 `TUserEngine.AddBaseObject` 的 `case` 语句根据 `btRace` 选择实例化哪个子类。`RC_NPC`（10）= 任务 NPC，`RCC_MERCHANT`（50）= 商人。
4. **三层商品架构**：商人商品分为配置层（`m_RefillGoodsList`，定义品种和补货周期）、库存层（`m_GoodsList`，当前实际库存）、价格层（`m_ItemPriceList`，动态价格），三层解耦。
5. **双端协议对称**：每个 NPC 交互都有明确的 CM（客户端→服务端）和 SM（服务端→客户端）消息对，内部通过 RM 消息桥接。

---

## 二、类层次与数据结构

### 2.1 类继承树

```
TBaseObject          (ObjBase.pas — 基础游戏对象)
  └─ TAnimalObject   (ObjBase.pas:533 — 添加 AI/行为字段)
       └─ TNormNpc   (ObjNpc.pas:78 — 任务/脚本 NPC)
            ├─ TMerchant       (ObjNpc.pas:280 — 商店 NPC)
            │    └─ TCastleOfficial  (ObjNpc.pas:390 — 城堡管理 NPC)
            ├─ TGuildOfficial  (ObjNpc.pas:360 — 行会管理 NPC)
            └─ TTrainer        (ObjNpc.pas:377 — 战斗训练 NPC)
```

**继承要点**：NPC 继承自 `TAnimalObject` 而非直接继承 `TBaseObject`，因此共享怪物的基础对象模型（位置、HP、方向、消息队列），但覆写 `Run`、`Operate`、`Click`、`UserSelect` 实现 NPC 特有行为。

### 2.2 TNormNpc 字段（ObjNpc.pas:78）

| 字段 | 类型 | 用途 |
|------|------|------|
| `m_nFlag` | Integer | NPC 有效性索引 |
| `m_ScriptList: TList` | TList | 脚本列表（pTScript 指针） |
| `m_sFilePath` | String | 脚本文件目录 |
| `m_boIsHide` | Boolean | 是否在地图上隐藏 |
| `m_boIsQuest` | Boolean | 是否为任务 NPC（影响脚本加载路径） |
| `m_sPath` | String | 任务脚本路径 |

**构造函数**（ObjNpc.pas:5646）设置的默认值：

| 属性 | 值 | 含义 |
|------|-----|------|
| `m_boSuperMan` | `True` | 无敌（不可被杀死） |
| `m_btRaceServer` | `RC_NPC`（10） | 服务端 Race 标识 |
| `m_nLight` | `2` | 光照等级 |
| `m_btAntiPoison` | `99` | 防毒（完全免疫） |
| `m_boStickMode` | `True` | 固定模式（不移动） |

### 2.3 TMerchant 附加字段（ObjNpc.pas:280）

| 字段 | 类型 | 用途 |
|------|------|------|
| `m_sScript` | String | 脚本名（`Script-MapName` 格式） |
| `m_nPriceRate` | Integer | 价格比率（默认 100，即 100%） |
| `m_boCastle` | Boolean | 是否属于城堡（影响税收） |
| `m_ItemTypeList: TList` | TList | 可交易物品类型列表 |
| `m_RefillGoodsList: TList` | TList | 配置层：补货商品定义（TGoods） |
| `m_GoodsList: TList` | TList | 库存层：当前商品库存（TList of TList of pTUserItem） |
| `m_ItemPriceList: TList` | TList | 价格层：动态价格（TItemPrice） |
| `m_UpgradeWeaponList: TList` | TList | 待取回升级武器列表（TUpgradeInfo） |

**能力标志**（由脚本 `[goods]` 段设置）：

| 标志 | 用途 |
|------|------|
| `m_boBuy` | 允许购买 |
| `m_boSell` | 允许出售 |
| `m_boMakeDrug` | 允许制药 |
| `m_boPrices` | 允许查询价格 |
| `m_boStorage` | 允许仓库存取 |
| `m_boGetback` | 允许取回仓库物品 |
| `m_boUpgradenow` | 允许武器升级 |
| `m_boGetBackupgnow` | 允许取回升级武器 |
| `m_boRepair` | 允许普通修理 |
| `m_boS_repair` | 允许特殊修理 |
| `m_boSendmsg` | 允许转发消息 |
| `m_boUseItemName` | 允许修改物品名称 |

**构造函数**（ObjNpc.pas:1707）设置的默认值：

| 属性 | 值 | 含义 |
|------|-----|------|
| `m_btRaceImg` | `RCC_MERCHANT`（50） | 客户端 Race 外观标识 |
| `m_nPriceRate` | `100` | 默认价格比率 100% |
| 所有能力标志 | `False` | 默认无能力，由脚本开启 |

### 2.4 TGuildOfficial（ObjNpc.pas:360）

| 方法 | 行号 | 用途 |
|------|------|------|
| `ReQuestBuildGuild` | :11317 | 创建行会 |
| `ReQuestGuildWar` | :11365 | 宣战行会 |
| `DoNate` | :11384 | 行会捐献 |
| `ReQuestCastleWar` | :11388 | 申请攻城战 |
| `Click` | :11214 | 覆写：调用 inherited |
| `UserSelect` | :11261 | 覆写：处理行会创建/战争/攻城标签 |

### 2.5 TTrainer（ObjNpc.pas:377）

| 方法 | 行号 | 用途 |
|------|------|------|
| `Operate` | :2642 | 处理训练消息 |
| `Run` | :2663 | 训练 tick 逻辑 |

### 2.6 TCastleOfficial（ObjNpc.pas:390，继承 TMerchant）

| 方法 | 行号 | 用途 |
|------|------|------|
| `HireArcher` | :701 | 雇佣弓箭手 |
| `HireGuard` | :619 | 雇佣守卫 |
| `RepairDoor` | :11434 | 修理城门 |
| `RepairWallNow` | :11469 | 修理城墙 |
| `Click` | :409 | 覆写：仅城主行会或 GM（权限 ≥ 3）可访问 |
| `UserSelect` | :485 | 覆写：城堡改名/存取金/开关门/修门修墙/雇守卫/雇弓手 |
| `GetVariableText` | :420 | 覆写：添加城堡变量（$CASTLEGOLD/$TODAYINCOME 等） |

### 2.7 脚本数据结构（ObjNpc.pas:1-77）

```pascal
TUpgradeInfo = record          // :7-18  武器升级跟踪
  sName: String;               // 玩家名
  Item: pTUserItem;            // 武器物品
  btDc, btSc, btMc: Byte;      // DC/SC/MC 加成
  btDura: Byte;                // 耐久加成
  dwTick: LongWord;            // 时间戳
end;

TItemPrice = record            // :20-23 动态价格
  nIndex: Integer;             // 物品索引
  nPrice: Integer;             // 当前价格
end;

TGoods = record                // :25-31 配置商品
  sName: String;               // 物品名
  nCount: Integer;             // 最大库存
  dwRefillTime: LongWord;      // 补货间隔（分钟）
  dwRefillTick: LongWord;      // 上次补货 tick
end;

TQuestActionInfo = record      // :32-47 解析后的动作
  nCmdCode: Integer;           // 命令码
  sParam: array[0..5] of String; // 6 个字符串参数
  nParam: array[0..5] of Integer; // 6 个整数参数
end;

TQuestConditionInfo = record   // :48-63 解析后的条件
  nCmdCode: Integer;           // 命令码
  sParam: array[0..5] of String; // 6 个字符串参数
  nParam: array[0..5] of Integer; // 6 个整数参数
end;

TSayingProcedure = record      // :64-71 一个"过程"块
  ConditionList: TList;        // #IF 条件列表
  ActionList: TList;           // #ACT 动作列表
  sSayMsg: String;             // #SAY 文本
  ElseActionList: TList;       // #ELACT 动作列表
  sElseSayMsg: String;         // #ELSE 文本
end;

TSayingRecord = record         // :72-77 一个 [@label] 块
  sLabel: String;              // 标签名
  ProcedureList: TList;        // 过程列表
  boExtJmp: Boolean;           // 外部跳转标志
end;
```

**脚本数据嵌套模型**：

```
TNormNpc.m_ScriptList: TList of pTScript
  └─ TScript.RecordList: TList of pTSayingRecord     （每个 [@label] 一个）
       └─ TSayingRecord.ProcedureList: TList of pTSayingProcedure
            ├─ ConditionList: TList of pTQuestConditionInfo  （#IF 条件）
            ├─ ActionList: TList of pTQuestActionInfo        （#ACT 动作）
            ├─ sSayMsg: String                               （#SAY 文本）
            ├─ ElseActionList: TList of pTQuestActionInfo    （#ELACT 动作）
            └─ sElseSayMsg: String                           （#ELSE 文本）
```

### 2.8 玩家侧 NPC 状态字段（ObjBase.pas，TPlayObject）

| 字段 | 行号 | 用途 |
|------|------|------|
| `m_sScriptLable` | :215 | 当前正在处理的脚本标签 |
| `m_Script` | :625 | 当前执行的 TScript 指针 |
| `m_NPC` | :626 | 当前交互的 NPC 指针 |
| `m_CanJmpScriptLableList` | :654 | 允许跳转的标签列表 |
| `m_nScriptGotoCount` | — | GOTO 递归计数器（上限 `nScriptGotoCountLimit` = 10） |
| `m_sScriptGoBackLable` | — | @BACK 导航的上一个标签 |
| `m_sScriptCurrLable` | — | @BACK 导航的当前标签 |

### 2.9 全局 NPC 引用（ObjBase.pas）

| 全局变量 | 用途 |
|----------|------|
| `g_FunctionNPC` | 全局功能 NPC：升级时触发 `@LevelUp`（:1975-1976），死亡时触发 `@OnDeath`（:5535-5536） |
| `g_ManageNPC` | 管理 NPC：成员管理触发 `@Member`（:13573-13575） |

---

## 三、网络协议消息

### 3.1 客户端→服务端（CM_*）

| 常量 | 值 | Grobal2.pas 行号 | 用途 |
|------|-----|-------------------|------|
| `CM_CLICKNPC` | 1010 | :132 | 玩家点击 NPC |
| `CM_MERCHANTDLGSELECT` | 1011 | :133 | 点击对话中的标签/链接 |
| `CM_MERCHANTQUERYSELLPRICE` | 1012 | :134 | 查询物品出售价格 |
| `CM_USERSELLITEM` | 1013 | :135 | 出售物品给 NPC |
| `CM_USERBUYITEM` | 1014 | :136 | 从 NPC 购买物品 |
| `CM_USERGETDETAILITEM` | 1015 | :137 | 获取详细物品列表（子菜单） |
| `CM_USERREPAIRITEM` | 1023 | :145 | 修理物品 |
| `CM_MERCHANTQUERYREPAIRCOST` | 1024 | :146 | 查询修理费用 |
| `CM_USERSTORAGEITEM` | 1031 | :153 | 存入仓库 |
| `CM_USERTAKEBACKSTORAGEITEM` | 1032 | :154 | 从仓库取回 |
| `CM_USERMAKEDRUGITEM` | 1034 | :156 | 制药 |

### 3.2 服务端→客户端（SM_*）

| 常量 | 值 | Grobal2.pas 行号 | 用途 |
|------|-----|-------------------|------|
| `SM_MERCHANTSAY` | 643 | :303 | NPC 对话文本 |
| `SM_MERCHANTDLGCLOSE` | 644 | :304 | 关闭 NPC 对话 |
| `SM_SENDGOODSLIST` | 645 | :305 | 商品列表（购买菜单） |
| `SM_SENDUSERSELL` | 646 | :306 | 打开出售面板 |
| `SM_SENDBUYPRICE` | 647 | :307 | 出售价格查询响应 |
| `SM_USERSELLITEM_OK` | 648 | :308 | 出售成功（Recog = 新金币数） |
| `SM_USERSELLITEM_FAIL` | 649 | :309 | 出售失败 |
| `SM_BUYITEM_SUCCESS` | 650 | :310 | 购买成功（Recog = 新金币数） |
| `SM_BUYITEM_FAIL` | 651 | :311 | 购买失败（Recog = 错误码 1/2/3） |
| `SM_SENDDETAILGOODSLIST` | 652 | :312 | 详细装备商品列表 |
| `SM_SENDUSERREPAIR` | 668 | :328 | 打开修理面板 |
| `SM_USERREPAIRITEM_OK` | 669 | :329 | 修理成功 |
| `SM_USERREPAIRITEM_FAIL` | 670 | :330 | 修理失败 |
| `SM_SENDREPAIRCOST` | 671 | :331 | 修理费用查询响应 |
| `SM_SENDUSERSTORAGEITEM` | 700 | :345 | 打开仓库面板 |
| `SM_STORAGE_OK` | 701 | :346 | 存储成功 |
| `SM_STORAGE_FULL` | 702 | :347 | 仓库已满 |
| `SM_STORAGE_FAIL` | 703 | :348 | 存储失败 |
| `SM_SAVEITEMLIST` | 704 | :349 | 仓库物品列表 |
| `SM_TAKEBACKSTORAGEITEM_OK` | 705 | :350 | 取回成功 |
| `SM_TAKEBACKSTORAGEITEM_FAIL` | 706 | :351 | 取回失败 |
| `SM_TAKEBACKSTORAGEITEM_FULLBAG` | 707 | :352 | 取回失败：背包已满 |
| `SM_SENDUSERMAKEDRUGITEMLIST` | 712 | :360 | 制药配方列表 |
| `SM_MAKEDRUG_SUCCESS` | 713 | :361 | 制药成功 |
| `SM_MAKEDRUG_FAIL` | 714 | :362 | 制药失败 |

### 3.3 内部消息（RM_*）

| 常量 | 值 | Grobal2.pas 行号 | 用途 |
|------|-----|-------------------|------|
| `RM_MERCHANTDLGCLOSE` | 10127 | :1141 | 内部：关闭商人对话 |
| `RM_MENU_OK` | 10309 | :1183 | 内部：菜单选择确认 |
| `RM_MERCHANTSAY` | 11009 | :1209 | 内部：NPC 说话（→ SM_MERCHANTSAY） |

### 3.4 Race 常量

| 常量 | 值 | Grobal2.pas 行号 | 含义 |
|------|-----|-------------------|------|
| `RC_NPC` | 10 | :1105 | 服务端 NPC Race |
| `RC_PEACENPC` | 15 | :1102 | 服务端和平 NPC Race |
| `RCC_MERCHANT` | 50 | :105 | 客户端 NPC 外观 Race |
| `RCC_GUARD` | 12 | :106 | 客户端守卫外观 |

### 3.5 完整交互流程

```
CLIENT                              SERVER
  │                                   │
  │── CM_CLICKNPC (1010) ──────────>  │  ObjBase.pas:4756 → ClientClickNPC
  │                                   │  → UserEngine.FindMerchant / FindNPC
  │                                   │  → 验证：同地图、距离 ≤ 15 格
  │                                   │  → NormNpc.Click(PlayObject)
  │                                   │  → 重置脚本导航状态
  │                                   │  → GotoLable('@main')
  │                                   │  → QuestCheckCondition + QuestActionProcess
  │                                   │  → GetVariableText 变量替换
  │                                   │  → SendMerChantSayMsg
  │<── SM_MERCHANTSAY (643) ────────  │  (RM_MERCHANTSAY → SM_MERCHANTSAY)
  │                                   │
  │── CM_MERCHANTDLGSELECT (1011) ──> │  → NormNpc.UserSelect(sData)
  │   "@buy\r"                        │  → TMerchant.UserSelect 分发内置命令
  │                                   │
  │<── SM_SENDGOODSLIST (645) ──────  │  (RM_SENDGOODSLIST)
  │                                   │
  │── CM_USERBUYITEM (1014) ────────> │  → TMerchant.ClientBuyItem
  │<── SM_BUYITEM_SUCCESS (650) ────  │  或 SM_BUYITEM_FAIL (651)
  │                                   │
  │── CM_USERSELLITEM (1013) ───────> │  → TMerchant.ClientSellItem
  │<── SM_USERSELLITEM_OK (648) ────  │  或 SM_USERSELLITEM_FAIL (649)
  │                                   │
  │── CM_USERREPAIRITEM (1023) ─────> │  → TMerchant.ClientRepairItem
  │<── SM_USERREPAIRITEM_OK (669) ──  │  或 SM_USERREPAIRITEM_FAIL (670)
  │                                   │
  │── CM_MERCHANTQUERYREPAIRCOST ───> │  → TMerchant.ClientQueryRepairCost
  │<── SM_SENDREPAIRCOST (671) ─────  │
  │                                   │
  │── CM_USERSTORAGEITEM (1031) ────> │  → TPlayObject.ClientStorageItem
  │<── SM_STORAGE_OK (701) ─────────  │  或 SM_STORAGE_FULL / SM_STORAGE_FAIL
  │                                   │
  │── CM_USERTAKEBACKSTORAGEITEM ───> │  → TPlayObject.ClientTakeBackStorageItem
  │<── SM_TAKEBACKSTORAGEITEM_OK ───  │  (705) 或 FAIL (706) 或 FULLBAG (707)
  │                                   │
  │── "@exit" via 1011 ─────────────> │  → RM_MERCHANTDLGCLOSE
  │<── SM_MERCHANTDLGCLOSE (644) ───  │
```

---

## 四、NPC 交互流程

### 4.1 玩家点击 NPC

客户端右键点击 NPC 后发送 `CM_CLICKNPC`（1010），`nParam1` 携带 NPC 的对象引用。

**服务端路由**（ObjBase.pas:4756-4757）→ `TPlayObject.ClientClickNPC`（ObjBase.pas:9833-9855）：

1. 检查 `m_boCanDeal`（未在交易中）、未死亡/销毁
2. 先尝试 `UserEngine.FindMerchant(TObject(NPC))`，再尝试 `UserEngine.FindNPC(TObject(NPC))`
3. 验证：同一地图（`m_PEnvir`），X/Y 距离均 ≤ 15 格
4. 调用 `NormNpc.Click(Self)`

### 4.2 NPC Click 处理器（ObjNpc.pas:4157-4163）

```pascal
procedure TNormNpc.Click(PlayObject: TPlayObject);
begin
  PlayObject.m_nScriptGotoCount := 0;   // 重置递归计数
  PlayObject.m_sScriptGoBackLable := ''; // 清空返回标签
  PlayObject.m_sScriptCurrLable := '';   // 清空当前标签
  GotoLable(PlayObject, '@main', False); // 跳转到 @main
end;
```

### 4.3 脚本执行 — GotoLable（ObjNpc.pas:6806-8540）

这是 NPC 脚本引擎的核心。执行流程：

1. **查找脚本**：若标签为 `@main`，遍历 `m_ScriptList` 中所有脚本查找匹配记录；否则使用玩家当前脚本（`PlayObject.m_Script`）。回退到检查任务状态（`CheckQuestStatus`）。
2. **查找标签**：遍历 `Script.RecordList` 找到匹配 `sLabel` 的 `TSayingRecord`。
3. **遍历过程**：对 `SayingRecord.ProcedureList` 中的每个 `TSayingProcedure`：
   - 调用 `QuestCheckCondition(ConditionList)`（:6969）求值条件
   - 条件通过：追加 `sSayMsg` 到输出，执行 `QuestActionProcess(ActionList)`（:7740）
   - 条件不通过：追加 `sElseSayMsg`，执行 `QuestActionProcess(ElseActionList)`
4. **发送文本**：`SendMerChantSayMsg`（:8428）处理 `<$VARIABLE>` 替换（通过 `GetVariableText`），提取 `<link/@label>` 标签（通过 `PlayObject.GetScriptLabel`），然后发送 `RM_MERCHANTSAY` → `SM_MERCHANTSAY`（643）到客户端。

**递归保护**（:7373-7374）：`GotoLable` 递增 `m_nScriptGotoCount`；超过 `g_Config.nScriptGotoCountLimit`（10）时拒绝跳转。

### 4.4 玩家选择对话选项

客户端发送 `CM_MERCHANTDLGSELECT`（1011），body 携带标签字符串（如 `@buy`、`@sell`）。

**TNormNpc.UserSelect**（ObjNpc.pas:8639）：管理返回导航（`m_sScriptGoBackLable`、`m_sScriptCurrLable`），然后调用 `GotoLable` 跳转到对应标签。

**TMerchant.UserSelect**（ObjNpc.pas:1419）：调用 `inherited`，然后分发内置商人命令（见第七章 §7.10）。

### 4.5 对话关闭

- **服务端关闭**：`@EXIT` 标签触发 `RM_MERCHANTDLGCLOSE` → `SM_MERCHANTDLGCLOSE`（644）
- **客户端自动关闭**（ClMain.pas:1447-1452）：空闲循环中检测玩家位置，若距离对话打开时的位置 ≥ 8 格，自动调用 `FrmDlg.CloseMDlg`

---

## 五、NPC 脚本引擎

### 5.1 脚本文件格式

NPC 脚本文件使用以下结构：

```
[@main]
#IF
CHECKLEVEL 10
CHECKJOB Warrior
#ACT
GIVE Gold 100
#SAY
Hello <$USERNAME>, welcome to my shop!
<Buy/@buy> <Sell/@sell> <Repair/@repair>
#ELACT
MAPMOVE 0 330 330
#ELSE
You don't meet the requirements.

[@buy]
#IF
#ACT
#SAY
What would you like to buy?
```

**语法规则**：
- `[@label]` 或 `[label]`：标签段头，定义一个可跳转的脚本节点
- `#IF`：条件段，后续行为条件命令，全部通过才执行 #ACT
- `#ACT`：动作段，条件通过时执行
- `#SAY`：对话文本段，条件通过时显示
- `#ELACT`（#ELSEACT）：条件不通过时执行的动作
- `#ELSE`（#ELSESAY）：条件不通过时显示的文本
- `;` 或 `//` 开头的行为注释
- 一个标签下可有多个 IF/ACT/SAY 过程块，按顺序执行

### 5.2 脚本加载

**TNormNpc.LoadNpcScript**（ObjNpc.pas:8542-8556）：
- 若 `m_boIsQuest` = True：从 `sNpc_def` 目录加载，文件名 `CharName-MapName`
- 若 `m_boIsQuest` = False：从 `m_sFilePath` 目录加载，文件名 `CharName`
- 委托 `FrmDB.LoadNpcScript(Self, path, name)` 解析文件

**TMerchant.LoadNpcScript**（ObjNpc.pas:1789-1798）：
- 从 `sMarket_Def` 目录加载，文件名 `Script-MapName`
- 调用 `FrmDB.LoadScriptFile(Self, sMarket_Def, SC, True)` — `True` 标志表示商人模式（解析 `[goods]` 段）

### 5.3 条件命令（#IF）— QuestCheckCondition（ObjNpc.pas:6969-7368）

#### 基础检查

| 命令 | 常量 | 检查内容 |
|------|------|----------|
| `CHECK` | nCHECK | 任务标志状态（标志索引，期望值） |
| `RANDOM` | nRANDOM | 随机概率（1/N） |
| `GENDER` | nGENDER | 玩家性别（MAN/WOMAN） |
| `DAYTIME` | nDAYTIME | 游戏时段（SUNRAISE/DAY/SUNSET/NIGHT） |
| `CHECKLEVEL` | nCHECKLEVEL | 玩家等级 ≥ N |
| `CHECKJOB` | nCHECKJOB | 玩家职业（Warrior/Wizard/Taos） |
| `CHECKITEM` | nCHECKITEM | 持有物品且数量 ≥ N |
| `CHECKITEMW` | nCHECKITEMW | 穿戴位有装备（[NECKLACE]/[RING]/[WEAPON] 等） |
| `CHECKGOLD` | nCHECKGOLD | 金币 ≥ N |
| `CHECKDURA` | nCHECKDURA | 物品耐久 ≥ N |
| `CHECKDURAEVA` | nCHECKDURAEVA | 平均耐久 ≥ N |
| `CHECKPKPOINT` | nCHECKPKPOINT | PK 等级 ≥ N |
| `CHECKLUCKYPOINT` | nCHECKLUCKYPOINT | 身体幸运 ≥ N |
| `CHECKBBCOUNT` | nCHECKBBCOUNT | 宠物/奴隶数量 ≥ N |
| `CHECKBAGGAGE` | nCHECKBAGGAGE | 背包有空位（可指定物品重量） |
| `DAYOFWEEK` | nDAYOFWEEK | 星期几（SUN/MON/TUE/WED/THU/FRI/SAT） |
| `HOUR` | nHOUR | 当前小时在范围内 |
| `MIN` | nMIN | 当前分钟在范围内 |
| `CHECKMONMAP` | nCHECKMONMAP | 地图怪物数量 ≥ N |
| `CHECKHUM` | nCHECKHUM | 地图人数 ≥ N |
| `CHECKNAMELIST` | nCHECKNAMELIST | 玩家名在文件列表中 |
| `CHECKACCOUNTLIST` | nCHECKACCOUNTLIST | 账号在文件列表中 |
| `CHECKIPLIST` | nCHECKIPLIST | IP 在文件列表中 |
| `CHECKOPEN` | nCHECKOPEN | 任务单元开启状态 |
| `CHECKUNIT` | nCHECKUNIT | 任务单元状态 |

#### 变量比较

| 命令 | 常量 | 检查内容 |
|------|------|----------|
| `EQUAL` | nEQUAL | 变量 == 值（支持 P0-9/G0-19/D0-9/M0-99/A0-99） |
| `LARGE` | nLARGE | 变量 > 值 |
| `SMALL` | nSMALL | 变量 < 值 |

#### 高级检查（nSC_* 前缀，:7275-7365）

| 命令 | 检查内容 |
|------|----------|
| `ISSYSOP` / `ISADMIN` | 权限等级检查 |
| `CHECKGROUPCOUNT` | 队伍人数 |
| `CHECKPOSEDIR` / `CHECKPOSELEVEL` / `CHECKPOSEGENDER` | 朝向/等级/性别检查 |
| `CHECKLEVELEX` / `CHECKBONUSPOINT` | 等级扩展/奖励点 |
| `CHECKSLAVECOUNT` / `CHECKSLAVELEVEL` / `CHECKSLAVENAME` | 宠物数量/等级/名称 |
| `HASGUILD` / `ISGUILDMASTER` | 有行会/是行会会长 |
| `CHECKCASTLEMASTER` / `ISCASTLEGUILD` | 城堡城主/城堡行会 |
| `ISATTACKGUILD` / `ISDEFENSEGUILD` | 攻城方/守城方行会 |
| `CHECKCASTLEDOOR` | 城门状态 |
| `ISATTACKALLYGUILD` / `ISDEFENSEALLYGUILD` | 攻城/守城联盟行会 |
| `CHECKMEMBERTYPE` / `CHECKMEMBERLEVEL` | 成员类型/等级 |
| `CHECKGAMEGOLD` / `CHECKGAMEPOINT` | 游戏金币/游戏点数 |
| `CHECKNAMELISTPOSITION` / `CHECKGUILDLIST` | 名单位置/行会列表 |
| `CHECKRENEWLEVEL` | 转生等级 |
| `CHECKCREDITPOINT` | 信誉点 |
| `CHECKOFGUILD` | 属于指定行会 |
| `CHECKPAYMENT` / `CHECKUSEITEM` / `CHECKBAGSIZE` | 付费/使用物品/背包大小 |
| `CHECKLISTCOUNT` | 列表计数 |
| `CHECKDC` / `CHECKMC` / `CHECKSC` | 攻击/魔法/道术值 |
| `CHECKHP` / `CHECKMP` | 生命值/魔法值 |
| `CHECKITEMTYPE` / `CHECKEXP` | 物品类型/经验值 |
| `CHECKCASTLEGOLD` | 城堡金币 |
| `PASSWORDERRORCOUNT` / `ISLOCKPASSWORD` / `ISLOCKSTORAGE` | 密码错误次数/密码锁/仓库锁 |
| `CHECKBUILDPOINT` / `CHECKAURAEPOINT` / `CHECKSTABILITYPOINT` / `CHECKFLOURISHPOINT` | 建设/灵气/稳定/繁荣度 |
| `CHECKCONTRIBUTION` | 贡献度 |
| `CHECKRANGEMONCOUNT` / `CHECKITEMADDVALUE` / `CHECKINMAPRANGE` | 范围怪物数/物品附加值/地图范围 |
| `CASTLECHANGEDAY` / `CASTLEWARDAY` | 城堡变更日/攻城战日 |
| `ONLINELONGMIN` | 在线时长（分钟） |
| `CHECKGUILDCHIEFITEMCOUNT` | 行会首席物品数 |
| `CHECKNAMEDATELIST` / `CHECKMAPHUMANCOUNT` / `CHECKMAPMONCOUNT` | 日期名单/地图人数/地图怪物数 |
| `CHECKVAR` / `CHECKSERVERNAME` | 变量值/服务器名 |
| `CHECKMAP` / `CHECKPOS` | 地图名/坐标 |
| `REVIVESLAVE` | 复活宠物 |
| `CHECKMAGICLVL` / `CHECKGROUPCLASS` | 魔法等级/队伍职业 |

### 5.4 动作命令（#ACT）— QuestActionProcess（ObjNpc.pas:7740-8425）

#### 核心脚本流程

| 命令 | 常量 | 功能 |
|------|------|------|
| `SET` | nSET | 设置任务标志（索引，值） |
| `RESET` | nRESET | 重置一段范围的任务标志为 0 |
| `SETOPEN` | nSETOPEN | 设置任务单元开启状态 |
| `SETUNIT` | nSETUNIT | 设置任务单元状态 |
| `RESETUNIT` | nRESETUNIT | 重置任务单元状态范围 |
| `BREAK` | nBREAK | 停止处理后续动作 |
| `GOTO` | nGOTO | 跳转到另一个 [@label]（递归上限 10） |
| `CLOSE` | nCLOSE | 关闭 NPC 对话（发送 RM_MERCHANTDLGCLOSE） |
| `GOQUEST` | nGOQUEST | 按任务编号跳转任务脚本 |
| `ENDQUEST` | nENDQUEST | 清除当前脚本引用 |

#### 物品操作

| 命令 | 常量 | 功能 |
|------|------|------|
| `TAKE` | nTAKE | 从玩家背包移除物品（特殊："Gold" 移除金币） |
| `GIVE` | nSC_GIVE | 给予玩家物品（单次上限 50；背包满时掉落在脚下） |
| `TAKEW` | nTAKEW | 移除穿戴位装备（[NECKLACE]/[RING]/[WEAPON]/[HELMET]/[DRESS]/[BUJUK]） |
| `TAKECHECKITEM` | nTAKECHECKITEM | 移除 CHECKITEM 条件找到的物品 |

#### 变量操作

| 命令 | 常量 | 功能 |
|------|------|------|
| `MOV` | nMOV | 设置变量值（P0-9=个人/G0-19=全局/D0-9=动态/M0-99=任务/A0-99=全局动态） |
| `INC` | nINC | 变量递增 |
| `DEC` | nDEC | 变量递减 |
| `SUM` | nSUM | 两个变量相加，存入索引 9 |
| `MOVR` | nMOVR | 设置变量为随机值（0..N-1） |
| `VAR` | nSC_VAR | 变量操作（ActionOfVar） |
| `LOADVAR` | nSC_LOADVAR | 从文件加载变量 |
| `SAVEVAR` | nSC_SAVEVAR | 保存变量到文件 |
| `CALCVAR` | nSC_CALCVAR | 计算变量 |

#### 移动/召唤

| 命令 | 常量 | 功能 |
|------|------|------|
| `MAPMOVE` | nMAPMOVE | 传送到指定地图+坐标 |
| `MAP` | nMAP | 随机传送到地图 |
| `EXCHANGEMAP` | nEXCHANGEMAP | 与目标地图上的某人交换位置 |
| `RECALLMAP` | nRECALLMAP | 召回地图上所有玩家（上限 20） |
| `MONGEN` | nMONGEN | 在坐标处刷怪 |
| `MONCLEAR` | nMONCLEAR | 清除地图上所有怪物 |
| `RECALLMOB` | nSC_RECALLMOB | 召唤宠物/奴隶怪物 |
| `TIMERECALL` | nTIMERECALL | N 分钟后自动召回玩家 |
| `BREAKTIMERECALL` | nBREAKTIMERECALL | 取消定时召回 |

#### 玩家修改（nSC_* 动作，:8340-8424）

| 命令 | 功能 |
|------|------|
| `CHANGELEVEL` / `CHANGEEXP` / `CHANGEJOB` | 修改等级/经验/职业 |
| `CHANGEPKPOINT` / `CHANGENAMECOLOR` | 修改 PK 点/名字颜色 |
| `ADDSKILL` / `DELSKILL` / `DELNOJOBSKILL` / `CLEARSKILL` / `SKILLLEVEL` | 技能增删改 |
| `GAMEGOLD` / `GAMEPOINT` / `AUTOADDGAMEGOLD` / `AUTOSUBGAMEGOLD` | 游戏金币/点数 |
| `RENEWLEVEL` / `RESTRENEWLEVEL` / `BONUSPOINT` / `RESTBONUSPOINT` | 转生/奖励点 |
| `CREDITPOINT` | 信誉点 |
| `KILLSLAVE` / `CHANGEGENDER` | 杀宠物/改性别 |
| `KILLMONEXPRATE` / `POWERRATE` | 杀怪经验倍率/能力倍率 |
| `CHANGEMODE` / `CHANGEPERMISSION` | 修改模式/权限 |
| `KILL` / `KICK` | 杀死/踢出玩家 |
| `SETMEMBERTYPE` / `SETMEMBERLEVEL` | 设置成员类型/等级 |
| `SETMAPMODE` / `PKZONE` | 设置地图模式/PK 区域 |
| `CLEARNEEDITEMS` / `CLEARMAEKITEMS` | 清除需求/制造物品 |
| `UPGRADEITEMS` / `UPGRADEITEMSEX` | 升级物品 |
| `MONGENEX` / `CLEARMAPMON` / `MOBPLACE` / `MOBFIREBURN` | 高级刷怪/清怪/放置/火墙 |
| `HUMANHP` / `HUMANMP` / `HAIRSTYLE` / `MISSION` | 修改 HP/MP/发型/任务 |
| `RECALLGROUPMEMBERS` / `CLEARNAMELIST` / `MAPTING` | 召回队员/清名单/地图传送 |
| `BUILDPOINT` / `AURAEPOINT` / `STABILITYPOINT` / `FLOURISHPOINT` | 城堡四维属性 |
| `OPENMAGICBOX` / `SETRANKLEVELNAME` / `GMEXECUTE` | 开宝箱/设排名/GM 执行 |
| `GUILDCHIEFITEMCOUNT` / `ADDNAMEDATELIST` / `DELNAMEDATELIST` | 行会首席物品/日期名单 |
| `MESSAGEBOX` / `SETSCRIPTFLAG` / `SETAUTOGETEXP` | 消息框/脚本标志/自动获经验 |
| `GUILDRECALL` / `GROUPADDLIST` / `CLEARLIST` / `GROUPRECALL` / `GROUPMOVEMAP` | 行会/队伍操作 |
| `LINEMSG` / `SENDMSG` | 服务器全服广播消息 |

#### 列表操作

| 命令 | 功能 |
|------|------|
| `ADDNAMELIST` / `DELNAMELIST` | 添加/移除玩家名到文件 |
| `ADDGUILDLIST` / `DELGUILDLIST` | 添加/移除行会名到文件 |
| `ADDACCOUNTLIST` / `DELACCOUNTLIST` | 添加/移除账号到文件 |
| `ADDIPLIST` / `DELIPLIST` | 添加/移除 IP 到文件 |

### 5.5 EXEACTION 二级分发器（ObjNpc.pas:5674-5844）

用于复杂操作的二级动作分发：

| 操作 | 参数 | 功能 |
|------|------|------|
| `CHANGEEXP` | 0=设置, 1=增加, 2=减少 | 修改经验 |
| `CHANGELEVEL` | 0=设置, 1=增加, 2=减少 | 修改等级 |
| `KILL` | 0=普通, 1=不掉物, 2=NPC 为杀手, 3=NPC+不掉物 | 杀死玩家 |
| `KICK` | — | 断开玩家连接 |

### 5.6 变量替换 — GetVariableText（ObjNpc.pas:5864-~6800）

替换 NPC 对话文本中的 `<$VARIABLE>` 标记：

**服务器信息**：`$SERVERNAME`、`$SERVERIP`、`$WEBSITE`、`$BBSSITE`、`$CLIENTDOWNLOAD`、`$QQ`、`$PHONE`、`$BANKACCOUNT0`-`$BANKACCOUNT9`、`$GAMEGOLDNAME`、`$GAMEPOINTNAME`、`$USERCOUNT`、`$MACRUNTIME`、`$SERVERRUNTIME`、`$DATETIME`

**排行榜**：`$HIGHLEVELINFO`、`$HIGHPKINFO`、`$HIGHDCINFO`、`$HIGHMCINFO`、`$HIGHSCINFO`、`$HIGHONLINEINFO`

**玩家信息**：`$USERNAME`（及等级、职业、金币、行会等大量字段）

**商人专属**（TMerchant.GetVariableText，:1806）：`$PRICERATE`、`$UPGRADEWEAPONFEE`、`$USERWEAPON`

**城堡专属**（TCastleOfficial.GetVariableText，:420）：`$CASTLEGOLD`、`$TODAYINCOME`、`$CASTLEDOORSTATE`、`$REPAIRDOORGOLD`、`$REPAIRWALLGOLD`、`$GUARDFEE`、`$ARCHERFEE`、`$GUARDRULE`

---

## 六、NPC 类型详解

### 6.1 TNormNpc — 任务 NPC（ObjNpc.pas:78）

- **构造**（:5646）：`m_boSuperMan := True`（无敌）、`m_btRaceServer := RC_NPC`（10）、`m_nLight := 2`、`m_btAntiPoison := 99`（防毒）、`m_boStickMode := True`（不移动）
- **Run**（:8562）：清除主人引用（NPC 不可被召唤），调用 inherited
- **Click**（:4157）：重置脚本导航，跳转 `@main`
- **GotoLable**（:6806-8540）：核心脚本执行引擎
- **用途**：任务发布者、传送 NPC、对话 NPC、事件触发器

### 6.2 TMerchant — 商店 NPC（ObjNpc.pas:280）

- **构造**（:1707）：`m_btRaceImg := RCC_MERCHANT`（50）、`m_nPriceRate := 100`、所有能力标志 False
- **Run**（:1609）：
  - 每 30 秒调用 `RefillGoods()`（商品补货）
  - 每 10 分钟清除过期升级武器
  - 随机转向（`TurnTo(Random(8))`）或播放攻击动画（`SendRefMsg(RM_HIT, ...)`），概率 1/50
  - 攻城战期间隐藏（发送 `RM_DISAPPEAR`，设 `m_boFixedHideMode := True`）
  - 可选移动：`m_boCanMove` + `m_dwMoveTime` 允许定时随机传送（`MapRandomMove`）
- **UserSelect**（:1419-1607）：分发内置标签（见 §7.10）
- **用途**：物品商店、武器修理、仓库、武器升级、制药

### 6.3 TGuildOfficial — 行会 NPC（ObjNpc.pas:360）

- **Click**（:11214）：调用 inherited
- **UserSelect**（:11261）：处理行会创建（`@@buildguildnow`）、行会战争、攻城战申请
- **ReQuestBuildGuild**（:11317）：验证条件（等级、金币、无行会），创建行会
- **ReQuestGuildWar**（:11365）：宣战行会
- **ReQuestCastleWar**（:11388）：申请攻城战
- **用途**：行会创建、行会战争、攻城战管理

### 6.4 TTrainer — 训练 NPC（ObjNpc.pas:377）

- **Operate**（:2642）：处理训练消息
- **Run**（:2663）：训练 tick 逻辑
- **用途**：玩家战斗训练/对练

### 6.5 TCastleOfficial — 城堡管理 NPC（ObjNpc.pas:390，继承 TMerchant）

- **Click**（:409）：仅城主行会成员或 GM（权限 ≥ 3）可访问
- **UserSelect**（:485）：处理城堡改名、存取金、开关门、修门修墙、雇守卫、雇弓手、退出/返回
- **HireGuard**（:619）：雇佣守卫（收费 `nGuardFee`）
- **HireArcher**（:701）：雇佣弓箭手（收费 `nArcherFee`）
- **RepairDoor**（:11434）：修理城门（收费 `nRepairDoorPrice`）
- **RepairWallNow**（:11469）：修理城墙（收费 `nRepairWallPrice`）
- **用途**：沙巴克城墙管理

---

## 七、商店系统

### 7.1 三层商品架构

```
┌──────────────────────────────────────────────────────────────┐
│  配置层 m_RefillGoodsList (TGoods 列表)                       │
│    来源：脚本 [goods] 段                                      │
│    内容：物品名、最大库存、补货间隔（分钟）                     │
├──────────────────────────────────────────────────────────────┤
│  库存层 m_GoodsList (TList of TList of pTUserItem)            │
│    内容：当前实际库存，每个内层 List 存放同名物品的多个实例     │
│    上限：非补货物品 1000，配置补货物品 5000                    │
├──────────────────────────────────────────────────────────────┤
│  价格层 m_ItemPriceList (TItemPrice 列表)                     │
│    内容：物品索引 + 当前价格                                   │
│    动态通胀：每次 CheckItemPrice 价格上涨 10%                  │
└──────────────────────────────────────────────────────────────┘
```

### 7.2 商品补货 — RefillGoods（ObjNpc.pas:850-983）

由 `TMerchant.Run` 每 30 秒调用一次：

1. 遍历 `m_RefillGoodsList` 中的每个配置商品
2. 若已过时间 > 补货间隔：
   - 当前库存 < 配置数量：通过 `UserEngine.CopyToUserItemFromName` 创建新物品补充
   - 当前库存 > 配置数量：移除多余物品
3. 非补货物品上限 1000，配置补货物品上限 5000
4. 通过 `FrmDB.SaveGoodRecord` / `FrmDB.SaveGoodPriceRecord` 持久化到数据库

### 7.3 价格计算

**GetItemPrice**（:999）：从 `m_ItemPriceList` 查找动态价格；若物品类型在 `m_ItemTypeList` 中，回退到 `StdItem.Price`。

**GetUserItemPrice**（:1838）：计算实际物品价值：
- 基础价格来自 `GetItemPrice`
- 耐久比率（StdMode 40 = 药水，43 = 石头有特殊处理）
- 附加值（DC/MC/SC/AC/MAC/HP/MP 加成）— 每个附加值根据物品类型贡献额外价格

**GetUserPrice**（:1385）：应用商人 `m_nPriceRate` 百分比。城堡成员享受折扣：`max(60, priceRate * nCastleMemberPriceRate/100)`。

**GetSellItemPrice**（:2127）：出售价格 = 购买价格 / 2。

### 7.4 动态定价

`CheckItemPrice`（:798-814）：每次查询价格时，物品价格上涨 10%，形成供需模拟。价格通过 `FrmDB.SaveGoodPriceRecord` / `LoadGoodPriceRecord` 持久化，以 `Script-MapName` 为键。

### 7.5 购买流程 — ClientBuyItem（ObjNpc.pas:1922-2028）

1. 在 `m_GoodsList` 中搜索匹配物品名
2. 消耗品（StdMode ≤ 4, 42, 31）：任意实例即可；装备：必须匹配 `MakeIndex`
3. 通过 `GetUserPrice(PlayObject, GetUserItemPrice(UserItem))` 计算价格
4. 检查：玩家金币足够且价格 > 0
5. 添加物品到玩家背包（`AddItemToBag`）
6. 扣除金币，应用城堡税（`IncRateGold`）
7. 发送 `RM_BUYITEM_SUCCESS`（→ `SM_BUYITEM_SUCCESS` 650）或 `RM_BUYITEM_FAIL`（→ `SM_BUYITEM_FAIL` 651）
8. 从商品列表移除物品；若列表为空，移除列表条目

### 7.6 出售流程 — ClientSellItem（ObjNpc.pas:2133-2226）

1. 计算出售价格：`GetSellItemPrice(GetUserItemPrice(UserItem))` = 基础价格 / 2
2. 特殊检查：药水（StdMode 25）和卷轴（StdMode 30）必须 Dura ≥ 4000
3. 增加玩家金币（`IncGold`）
4. 应用城堡税
5. 发送 `RM_USERSELLITEM_OK`（→ `SM_USERSELLITEM_OK` 648）
6. 通过 `AddItemToGoodsList`（:2233）添加到商人商品列表 — 创建新的 `pTUserItem` 副本

**Bug 修复注记**（:2233）：原始代码直接添加指针（`ItemList.add(UserItem)`），导致客户端从背包移除物品时数据损坏。修复后创建新的 `pTUserItem` 副本。

### 7.7 修理系统 — ClientRepairItem（ObjNpc.pas:2422-2493）

**普通修理**（`m_boRepair`）：
- 费用公式：`price / 3 / DuraMax * (DuraMax - Dura)`
- **永久降低 DuraMax**：`DuraMax -= (DuraMax - Dura) / nRepairItemDecDura`（默认 30）
- 然后 `Dura = DuraMax`
- 即：每次普通修理都会永久损失最大耐久

**特殊修理**（`m_boS_repair`）：
- 费用 = 普通修理费用 × `nSuperRepairPriceRate`（默认 3 倍）
- **不降低 DuraMax**：直接 `Dura = DuraMax`

修理成功后跳转 `@repair_ok` 或 `@s_repair_ok` 标签。

**ClientQueryRepairCost**（:2392）：同样费用公式，发送 `RM_SENDREPAIRCOST`（→ `SM_SENDREPAIRCOST` 671）。

### 7.8 武器升级 — UpgradeWapon（ObjNpc.pas:1034-1229）

**升级流程**：
1. 检查：玩家已装备武器、金币足够（`nUpgradeWeaponPrice`）、持有黑铁矿
2. 扣除金币，从玩家身上移除武器
3. 分析玩家背包中的升级材料：
   - 黑铁矿：贡献耐久升级（按耐久排序，取前 5 个平均）
   - 饰品（StdMode 19-26）：根据属性贡献 DC/SC/MC 点数
4. 计算升级加成：`btDc = minDC/5 + maxDC/3`（SC/MC 同理）
5. 存入 `m_UpgradeWeaponList` 带时间戳
6. 跳转 `@upgrade_ok` 或 `@upgrade_fail`

**取回流程 — GetBackupgWeapon**（:1230-1384）：
1. 检查：背包有空位、有待取回升级
2. 等待时间：`dwUPgradeWeaponGetBackTime`（GM 可跳过）
3. 根据 `btDura` 分数应用耐久变化（0-8：大幅降低，9-15：可能降低，18+：升级）
4. 应用属性升级：根据最高材料分数决定升级哪种属性（DC/MC/SC）
5. 成功概率：`min(85, score*7 + 10 + duraBonus - duraPenalty + bodyLuck)`
6. 成功时：设 `btValue[10]` 为 10/11/12（DC）、20/21/22（MC）或 30/31/32（SC），对应 +1/+2/+3 点
7. 返回武器到玩家背包

### 7.9 制药 — ClientMakeDrugItem（ObjNpc.pas:2276）

1. 通过 `GetMakeItemInfo(sItemName)` 查找配方 — 返回所需材料
2. 验证玩家背包中有全部材料
3. 移除材料，创建药品
4. 收费 `g_Config.nMakeDurgPrice` 金币

### 7.10 UserSelect 内置标签分发（TMerchant，:1419-1607）

| 标签 | 能力标志 | 动作 |
|------|----------|------|
| `@SENDMSG` | m_boSendmsg | 转发消息到 SendCustemMsg |
| `@SUPERREPAIR` | m_boS_repair | 打开特殊修理（发送 RM_SENDUSERSREPAIR） |
| `@BUY` | m_boBuy | 发送商品列表（RM_SENDGOODSLIST → SM_SENDGOODSLIST 645） |
| `@SELL` | m_boSell | 打开出售面板（RM_SENDUSERSELL → SM_SENDUSERSELL 646） |
| `@REPAIR` | m_boRepair | 打开修理面板（RM_SENDUSERREPAIR → SM_SENDUSERREPAIR 668） |
| `@MAKEDURG` | m_boMakeDrug | 发送制药配方列表（RM_USERMAKEDRUGITEMLIST） |
| `@PRICES` | m_boPrices | （空实现） |
| `@STORAGE` | m_boStorage | 打开仓库（RM_USERSTORAGEITEM → SM_SENDUSERSTORAGEITEM 700） |
| `@GETBACK` | m_boGetback | 打开取回（RM_USERGETBACKITEM） |
| `@UPGRADENOW` | m_boUpgradenow | 开始武器升级 |
| `@GETBACKUPGNOW` | m_boGetBackupgnow | 取回升级武器 |
| `@USEITEMNAME*` | m_boUseItemName | 修改物品名称 |
| `@EXIT` | — | 关闭对话（RM_MERCHANTDLGCLOSE → SM_MERCHANTDLGCLOSE 644） |
| `@BACK` | — | 跳转到上一个标签（或 @main） |

---

## 八、仓库系统

### 8.1 存储/取回消息流

仓库通过 NPC 对话系统触发：

- `@STORAGE` 标签 → `RM_USERSTORAGEITEM` → 客户端显示仓库 UI
- 客户端发送 `CM_USERSTORAGEITEM`（1031）→ `TPlayObject.ClientStorageItem`（ObjBase.pas:821）
- 客户端发送 `CM_USERTAKEBACKSTORAGEITEM`（1032）→ `TPlayObject.ClientTakeBackStorageItem`（ObjBase.pas:822）

**服务端响应**：

| 操作 | 成功 | 失败 |
|------|------|------|
| 存入 | `SM_STORAGE_OK`（701） | `SM_STORAGE_FULL`（702）/ `SM_STORAGE_FAIL`（703） |
| 取回 | `SM_TAKEBACKSTORAGEITEM_OK`（705） | `SM_TAKEBACKSTORAGEITEM_FAIL`（706）/ `SM_TAKEBACKSTORAGEITEM_FULLBAG`（707） |

仓库物品列表通过 `SM_SAVEITEMLIST`（704）发送。

### 8.2 仓库密码锁

- `m_sStoragePwd`：仓库密码
- `m_boPasswordLocked`：是否锁定
- 条件命令 `ISLOCKPASSWORD` / `ISLOCKSTORAGE`：检查密码/仓库锁状态
- 动作命令 `CLEARPASSWORD`：清除密码

### 8.3 容量限制

仓库容量为 `STORAGEITEMCOUNT`（39 格），与 Go 实现的 `MaxStorageItems = 39` 一致。

---

## 九、NPC 外观与放置

### 9.1 Race/Appearance 映射

- **服务端 Race**：`m_btRaceServer = RC_NPC`（10）— 任务 NPC
- **客户端外观 Race**：`m_btRaceImg = RCC_MERCHANT`（50）— 商人 NPC
- 客户端通过 `RCC_MERCHANT`（50）选择 NPC 精灵表（`Npc.wil`）
- NPC 的 `m_wAppearance` 决定在 `Npc.wil` 中的帧偏移（见第十一章 §11.1）

### 9.2 NPC 默认属性

| 属性 | 值 | 含义 |
|------|-----|------|
| `m_boSuperMan` | `True` | 无敌（不可被杀死） |
| `m_boStickMode` | `True` | 固定模式（不游荡） |
| `m_btAntiPoison` | `99` | 完全免疫毒 |
| `m_nLight` | `2` | 光照等级 |
| `m_boIsHide` | `False` | 可见（可设为 True 隐藏） |

### 9.3 商人可选移动

`TMerchant` 支持可选的随机移动：
- `m_boCanMove`：是否允许移动
- `m_dwMoveTime`：移动间隔
- 触发时调用 `MapRandomMove` 随机传送到地图上另一个位置

### 9.4 攻城战隐藏

攻城战期间（`m_boUnderWar`），城堡商人：
1. 发送 `RM_DISAPPEAR` 从客户端视野中消失
2. 设置 `m_boFixedHideMode := True`
3. 攻城战结束后重新出现

### 9.5 空闲动画

商人在 `Run`（:1609）中以 1/50 概率每 tick：
- 随机转向：`TurnTo(Random(8))`
- 或播放攻击动画：`SendRefMsg(RM_HIT, ...)`

这给 NPC 增加了"活着"的视觉效果。

---

## 十、GM 命令

| 命令 | ObjBase.pas 行号 | 功能 |
|------|-------------------|------|
| `CmdMobNpc` | :1115 | 在当前位置生成一个 NPC |
| `CmdNpcScript` | :1116 | 执行 NPC 脚本 |
| `CmdDelNpc` | :1117（实现 :12368） | 删除一个 NPC |
| `CmdReloadNpc` | :1032 | 重新加载所有 NPC 脚本 |

---

## 十一、客户端 NPC 系统

### 11.1 NPC Actor 渲染 — TNpcActor（Actor.pas:760-775）

```pascal
TNpcActor = class (TActor)
private
  m_nEffX, m_nEffY        : Integer;      // 特效偏移
  m_bo248                  : Boolean;      // 定时特效标志
  m_dwUseEffectTick        : LongWord;     // 特效过期 tick
  m_EffSurface             : TDirectDrawSurface; // 特效精灵
public
  constructor Create; override;
  procedure Run; override;
  procedure CalcActorFrame; override;
  function  GetDefaultFrame(wmode: Boolean): integer; override;
  procedure LoadSurface; override;
  procedure DrawChr(...); override;
  procedure DrawEff(...); override;
end;
```

**精灵加载**：
- 主 WIL 文件：`Data\Npc.wil`（常量 `NPCIMAGESFILE`，Share.pas:61）
- 全局图像对象：`g_WNpcImgImages: TWMImages`（MShare.pas:170），初始化于 :688-692
- 独立 WIL 文件：`m_nBodyOffset >= 1000` 的 NPC 从 `.\Graphics\Npc\<appr>.wil` 加载（ClMain.pas:6153-6177），缓存于 `NpcImageList: TList`（:210）

**帧偏移计算 — GetNpcOffset()**（Actor.pas:1156-1204）：
- 常量 `MERCHANTFRAME = 60`（:19）：每个 NPC 60 帧（3 方向 × 20 帧/动作组）
- 每个 `m_wAppearance` 映射到 `Npc.wil` 中的帧偏移，是一个复杂查找表：
  - 外观 0-22：`nAppr * 60`
  - 外观 24-25：`(nAppr - 24) * 60 + 1470`
  - 外观 42-43：`2580`
  - 外观 44-47：`2640`
  - 外观 54-57：`(nAppr - 54) * 60 + 3070`
  - ... 直到外观 83

**LoadSurface**（Actor.pas:3028-3083）：
- `m_btRace = 50`（RCC_MERCHANT）：从 `g_WNpcImgImages` 获取 `m_nBodyOffset + m_nCurrentFrame`
- 外观 42-47：无身体（隐形 NPC，仅特效）
- 特效表面：外观 33/34/42-47/51/52 有特殊特效帧

**CalcActorFrame**（Actor.pas:2866-2960）：
- NPC 仅使用 3 个方向：`m_btDir := m_btDir mod 3`
- 动画状态：
  - **SM_TURN**（站立）：使用 `pm.ActStand` 帧。外观 33/34 有循环特效，42-47 有特殊帧范围，51 在第 60 帧有特效
  - **SM_HIT**（攻击）：使用 `pm.ActAttack` 帧。外观 33/34/52 回退到站立动画
  - **SM_DIGUP**：仅外观 52 — 播放声音，触发 23 秒定时特效

**DrawChr / DrawEff**（Actor.pas:2970-3010）：
- `DrawChr`：在 `(dx + m_nPx + m_nShiftX, dy + m_nPy + m_nShiftY)` 绘制 `m_BodySurface`。外观 51 始终使用混合模式。超过 60 秒重新加载表面
- `DrawEff`：在特效偏移位置混合绘制 `m_EffSurface`

**场景渲染**（PlayScn.pas:1229-1240）：NPC 与所有其他 Actor 一起按 Y 排序绘制。

**NPC 创建**（PlayScn.pas:2028-2149，`NewActor()`）：
- 当服务端为未知 Actor ID 发送 SM_TURN/SM_WALK 等消息时调用
- :2085：`50: actor := TNpcActor.Create;`（race 50 = RCC_MERCHANT）
- 从服务端消息设置属性（:2123-2146）：`m_btRace`、`m_wAppearance`、`m_sUserName`

### 11.2 NPC 点击检测（ClMain.pas:2190-2356）

1. 右键点击游戏场景调用 `PlayScene.GetAttackFocusCharacter(X, Y, ...)`（:2202）
2. `GetAttackFocusCharacter`（PlayScn.pas:1785-1818）对所有可见 Actor 做像素级命中测试，检查鼠标位置是否在 Actor 精灵边界框（`CharWidth` × `CharHeight`）内
3. 若目标 `m_btRace = RCC_MERCHANT`（50），发送 `CM_CLICKNPC`（:2294-2296）：
   ```pascal
   if target.m_btRace = RCC_MERCHANT then begin
     SendClientMessage(CM_CLICKNPC, target.m_nRecogId, 0, 0, 0);
     exit;
   end;
   ```
4. NPC 被排除在自动攻击目标之外（:2304-2306, :2419-2421）

### 11.3 NPC 碰撞（PlayScn.pas:1923-1941）

NPC 默认阻挡移动。`g_boCanRunNpc` 标志（来自服务端配置，ClMain.pas:6356/6368）控制玩家是否可以穿过 NPC：
```pascal
if (Actor.m_btRace = RCC_MERCHANT) and g_boCanRunNpc then Continue;
```

### 11.4 对话窗口 — DMerchantDlg（FState.pas）

**消息流**：
1. 服务端发送 `SM_MERCHANTSAY`，`Recog=merchantID`，`Param=face`，`body=编码后脚本`
2. ClMain.pas:4531-4534 分发到 `ClientGetMerchantSay()`
3. `ClientGetMerchantSay()`（:5537-5551）：
   - 保存玩家位置到 `g_nMDlgX/g_nMDlgY`（走远自动关闭用）
   - 若商人更换，调用 `FrmDlg.ResetMenuDlg` 和 `FrmDlg.CloseMDlg`
   - 解析 NPC 名：`saying := GetValidStr3(saying, npcname, ['/'])`
   - 调用 `FrmDlg.ShowMDlg(face, npcname, saying)`

**ShowMDlg**（FState.pas:4697-4714）：
- `DMerchantDlg` 置于 (0, 0)
- 保存 `MerchantFace`、`MerchantName`、`MDlgStr`（原始脚本文本）
- 清空旧点击点，设 `RequireAddPoints := TRUE`（下次绘制重建可点击区域）
- 5 秒防刷：`LastestClickTime := GetTickCount`

**脚本渲染 — DMerchantDlgDirectPaint**（FState.pas:4806-4880）：

核心 NPC 脚本渲染引擎，解析脚本文本并渲染可点击链接：

1. 按 `\` 分割文本为行（:4826）
2. 每行内用 `ArrestStringEx(data, '<', '>', cmdstr)` 查找 `<...>` 标签（:4834）
3. 特殊标签：
   - `<C>` — 启用居中对齐（:4838-4840）
   - `</C>` — 禁用居中对齐（:4842-4844）
   - `<tagtext/param>` — 可点击链接；`tagtext` 为显示文本，`param` 为发送到服务端的脚本命令（:4846）
4. 普通文本：白色+黑色描边（`BoldTextOut`，:4853）
5. 链接文本：黄色+下划线；当前选中（鼠标按下）时红色（:4863-4868）
6. **点击区域注册**：`RequireAddPoints` 为 true 时，每个链接创建 `TClickPoint` 记录（边界矩形 + 命令字符串）（:4857-4861）
7. 行高：16 像素（:4874）
8. 文本起始偏移：(30, 30)（:4820-4821）

**链接点击 — DMerchantDlgClick**（FState.pas:5116-5135）：
- 5 秒防刷检查
- 遍历 `MDlgPoints` 命中测试
- 播放 `s_glass_button_click` 音效
- 调用 `FrmMain.SendMerchantDlgSelect(g_nCurMerchant, p.RStr)`
- 设置 `LastestClickTime := GetTickCount + 5000`

**发送选择 — SendMerchantDlgSelect**（ClMain.pas:3094-3110）：
- `@@` 前缀命令（如 `@@buildguildnow`）触发文本输入对话框（行会名等）
- 用户输入以 `#13` 分隔符追加到命令后
- 发送 `CM_MERCHANTDLGSELECT`（1011）

**窗口控件**（FState.dfm:195-198）：
- `DMerchantDlg`：`TDWindow`（自建 DirectX 绘制窗口，非 VCL 窗体）
- 背景图：`g_WMainImages` 索引 384（`Data\Prguse.wil`）
- 关闭按钮：`DMerchantDlgClose` 位于 (372, 20)，图像索引 371

**CloseMDlg**（FState.pas:4779-4793）：隐藏 `DMerchantDlg`、`DMenuDlg`、`DSellDlg`，清空 `MDlgPoints`，将 `DItemBag` 复位到 (0,0)。

### 11.5 商品列表窗口 — DMenuDlg（FState.pas）

**打开购买菜单**：
- 服务端消息 `SM_SENDGOODSLIST`（645），`Recog=merchantID`，`Param=count`，`body=编码商品列表`
- `ClientGetSendGoodsList()`（ClMain.pas:5553-5584）：解析 `/` 分隔记录 `name/submenu/price/stock/...`
- 每条记录创建 `TClientGoods`（Grobal2.pas:723-730）：

```pascal
TClientGoods = record
  Name    : String;
  SubMenu : Integer;  // >0 表示有子项
  Price   : Integer;
  Stock   : Integer;
  Grade   : Integer;  // -1 = 不显示等级
end;
```

**ShowShopMenuDlg**（FState.pas:4741-4761）：同时显示三个窗口：
- `DMerchantDlg` 于 (0, 0) — NPC 对话背景
- `DMenuDlg` 于 (0, 170) — 商品列表面板
- `DItemBag` 于 (475, 0) — 玩家背包

**菜单渲染 — DMenuDlgDirectPaint**（FState.pas:4887-4967）：
- **普通模式**（购买）："物品列表" | "价格" | "耐久" 表头
  - 物品名 x=38，价格 x=170，等级 x=265
  - 选中项红色箭头（字符 7）x=25
  - 子菜单指示：`#31`（下箭头）x=137
- **仓库模式**（`BoStorageMenu`）："仓库物品" | "耐久" 表头
- 行高：`LISTLINEHEIGHT`，最大可见行数：`MAXMENU`

**购买点击 — DMenuBuyClick**（FState.pas:5028-5052）：
- `SubMenu > 0`：发送 `SendGetDetailItem`（打开子菜单）
- `BoStorageMenu`：发送 `SendTakeBackStorageItem`（取回仓库物品）
- `BoMakeDrugMenu`：发送 `SendMakeDrugItem`（制药）
- 否则：发送 `SendBuyItem`（购买）

**分页**（DMenuPrevClick / DMenuNextClick，:5054-5075）：
- 普通模式：`MenuTop` 滚动 `MAXMENU-1` 行
- 详细模式（`BoDetailMenu`）：发送 `SendGetDetailItem` 带新偏移

### 11.6 出售/修理/仓库窗口 — DSellDlg（FState.pas）

三种模式复用同一个 `DSellDlg` 窗口，由 `SpotDlgMode` 区分：

| SM_ 消息 | 处理器 | SpotDlgMode | ClMain.pas 行号 |
|----------|--------|-------------|-----------------|
| `SM_SENDUSERSELL`（646） | `ClientGetSendUserSell` | `dmSell` | :5622-5628 |
| `SM_SENDUSERREPAIR`（668） | `ClientGetSendUserRepair` | `dmRepair` | :5630-5636 |
| `SM_SENDUSERSTORAGEITEM`（700） | `ClientGetSendUserStorage` | `dmStorage` | :5638-5644 |

**ShowShopSellDlg**（FState.pas:4763-4777）：`DSellDlg` 于 (260, 170)，隐藏 `DMenuDlg`，`DItemBag` 于 (475, 0)。

**DSellDlg 布局**（FState.dfm:212-221）：
- 背景：`g_WMainImages` 索引 392
- 确认按钮 (28, 135)，图像 362
- 关闭按钮 (81, 135)，图像 366
- 物品放置区：`DSellDlgSpot` 于 (27, 67)，尺寸 61×52

**渲染 — DSellDlgDirectPaint**（FState.pas:5164-5188）：
- `dmSell`："价格: " + `g_sSellPriceStr`
- `dmRepair`："修理: " + `g_sSellPriceStr`
- `dmStorage`："存储物品:"

**物品放置 — DSellDlgSpotClick**（FState.pas:5195-5227）：
- 使用全局物品移动系统（`g_boItemMoving`、`g_MovingItem`）
- 放置后设 `g_boQueryPrice := TRUE` + `g_dwQueryPriceTime := GetTickCount`
- 空闲循环（ClMain.pas:3513-3521）等 500ms 后发送价格查询：
  - `dmSell`：`SendQueryPrice`（CM_MERCHANTQUERYSELLPRICE 1012）
  - `dmRepair`：`SendQueryRepairCost`（CM_MERCHANTQUERYREPAIRCOST 1024）

**确认操作 — DSellDlgOkClick**（FState.pas:5251-5264）：
- `dmSell`：`SendSellItem`（CM_USERSELLITEM 1013）
- `dmRepair`：`SendRepairItem`（CM_USERREPAIRITEM 1023）
- `dmStorage`：`SendStorageItem`（CM_USERSTORAGEITEM 1031）
- 保存 `g_SellDlgItemSellWait` 用于失败回滚
- 5 秒防刷

**服务端响应处理**（ClMain.pas）：

| 响应 | 处理 | 行号 |
|------|------|------|
| `SM_SENDBUYPRICE`（647） | 设 `g_sSellPriceStr` 为价格或 "????" | :4555-4562 |
| `SM_USERSELLITEM_OK`（648） | 更新金币，清除等待物品 | :4563-4568 |
| `SM_USERSELLITEM_FAIL`（649） | 物品归还背包，显示错误 | :4570-4576 |
| `SM_SENDREPAIRCOST`（671） | 设 `g_sSellPriceStr` | :4578-4585 |
| `SM_USERREPAIRITEM_OK`（669） | 更新金币，恢复耐久，归还物品 | :4586-4596 |
| `SM_USERREPAIRITEM_FAIL`（670） | 归还物品，显示错误 | :4597-4603 |
| `SM_STORAGE_OK/FULL/FAIL`（701-703） | 非 OK 显示错误，失败归还物品 | :4604-4617 |
| `SM_BUYITEM_SUCCESS`（650） | 更新金币，`SoldOutGoods` 更新库存 | :4636-4641 |
| `SM_BUYITEM_FAIL`（651） | 错误消息：1=金币不足，2=太重，3=太大 | :4642-4650 |

### 11.7 仓库 UI

**仓库物品列表**（SM_SAVEITEMLIST 704）：
- `ClientGetSaveItemList()`（ClMain.pas:5651-5690）
- 清空 `g_SaveItemList`（PTClientItem 列表）
- 解析 body：每个 `/` 分隔块解码为 `TClientItem`
- 转换为 `TClientGoods`：`Name` = 物品名，`Price` = MakeIndex（服务端物品索引），`Stock` = Dura/1000，`Grade` = DuraMax/1000
- 设 `BoStorageMenu := TRUE` 显示商店菜单

**取回**：通过 `DMenuBuyClick`（:5041-5043），`BoStorageMenu` 模式下发送 `SendTakeBackStorageItem`。

**移除**（FState.pas:5094-5109，`DelStorageItem()`）：成功后从 `MenuList` 和 `g_SaveItemList` 中匹配 `pg.Price = itemserverindex` 移除。

### 11.8 制药 UI

**SM_SENDUSERMAKEDRUGITEMLIST**（712）：
- `ClientGetSendMakeDrugList()`（ClMain.pas:5586-5619）
- 解析与商品列表相同，设 `BoMakeDrugMenu := TRUE`
- 购买按钮发送 `CM_USERMAKEDRUGITEM`（1034）
- 响应：`SM_MAKEDRUG_SUCCESS`（713）/ `SM_MAKEDRUG_FAIL`（714），:4651-4665

### 11.9 客户端全局状态

**MShare.pas**：

| 变量 | 类型 | 行号 | 用途 |
|------|------|------|------|
| `g_nCurMerchant` | Integer | :296 | 当前 NPC 的服务端 ID |
| `g_nMDlgX` / `g_nMDlgY` | Integer | :297-298 | 对话打开时玩家位置（走远自动关闭） |
| `g_WNpcImgImages` | TWMImages | :170 | NPC 精灵 WIL（`Data\Npc.wil`） |
| `g_SaveItemList` | TList | :242 | 仓库物品（PTClientItem） |
| `g_MenuItemList` | TList | :243 | 详细菜单物品（PTClientItem） |
| `g_SellDlgItem` | TClientItem | :347 | 出售/修理/仓库放置区物品 |
| `g_SellDlgItemSellWait` | TClientItem | :348 | 待确认物品（失败回滚用） |
| `g_boQueryPrice` | Boolean | :350 | 价格查询等待中 |
| `g_sSellPriceStr` | String | :352 | 显示的价格/费用字符串 |
| `g_boCanRunNpc` | Boolean | :429 | 允许穿过 NPC |

**FState.pas（TFrmDlg 字段）**：

| 字段 | 类型 | 行号 | 用途 |
|------|------|------|------|
| `MerchantName` | string | :533 | NPC 显示名 |
| `MerchantFace` | integer | :534 | NPC 头像索引（SM_MERCHANTSAY Param） |
| `MDlgStr` | string | :535 | 原始 NPC 脚本文本 |
| `MDlgPoints` | TList | :536 | 可点击链接区域（pTClickPoint） |
| `RequireAddPoints` | Boolean | :537 | 下次绘制重建点击区域 |
| `SelectMenuStr` | string | :538 | 当前悬停/选中链接 |
| `LastestClickTime` | longword | :539 | 防刷时间戳 |
| `SpotDlgMode` | TSpotDlgMode | :540 | dmSell / dmRepair / dmStorage |
| `MenuList` | TList | :542 | 商品/仓库物品（PTClientGoods） |
| `MenuIndex` | integer | :543 | 选中菜单项索引 |
| `MenuTopLine` | integer | :545 | 详细菜单滚动偏移 |
| `BoDetailMenu` | Boolean | :546 | 显示子菜单 |
| `BoStorageMenu` | Boolean | :547 | 显示仓库物品 |
| `BoMakeDrugMenu` | Boolean | :549 | 显示制药菜单 |

### 11.10 @@ 前缀输入对话框

`SendMerchantDlgSelect`（ClMain.pas:3094-3110）中，`@@` 前缀命令触发文本输入：
- `@@buildguildnow`：输入行会名称
- 其他 `@@` 命令：通用输入对话框
- 用户输入以 `#13` 分隔符追加到命令字符串后

### 11.11 UI 框架（DWinCtl.pas）

NPC 对话系统使用自建 DirectX UI 框架（非 VCL 窗体）：

- **TDControl**（:26）：基类。`WLib: TWMImages`（精灵库）、`FaceIndex`（精灵索引）、`DControls`（子控件列表）、鼠标/键盘事件、`DirectPaint()` 渲染
- **TDButton**（:98）：添加点击音效、`Downed` 状态、鼠标事件覆写
- **TDWindow**（:146）：添加 `Floating`（可拖动）、`DialogResult`、`ShowModal`。继承 TDButton
- **TDWinManager**（:166）：管理所有 DControl，分发输入，调用 `DirectPaint`

`DMerchantDlg`、`DMenuDlg`、`DSellDlg` 均为 `TDWindow` 实例。子按钮（`DMerchantDlgClose`、`DMenuBuy`、`DSellDlgOk` 等）为 `TDButton` 实例。所有渲染通过 `OnDirectPaint` 回调绘制到 `TDirectDrawSurface`。

---

## 十二、关键实现细节与陷阱

### 12.1 脚本递归保护

`GotoLable` 递增 `m_nScriptGotoCount`；超过 `g_Config.nScriptGotoCountLimit`（10）时拒绝跳转（ObjNpc.pas:7373-7374）。防止脚本 `GOTO` 死循环。

### 12.2 城堡税收系统

所有商人交易（买/卖/修理/升级）可应用城堡税：
- 若 `m_boCastle` = True：通过 `TUserCastle.IncRateGold` 收税
- 若 `g_Config.boGetAllNpcTax` = True：通过 `g_CastleManager.IncRateGold` 全局收税

### 12.3 动态定价通胀

`CheckItemPrice`（:811-814）：每次查询价格上涨 10%。这创建了一个供需模拟——频繁查询的物品会越来越贵。

### 12.4 商品持久化

商人库存和价格通过 `FrmDB.SaveGoodRecord` / `LoadGoodRecord` 和 `FrmDB.SaveGoodPriceRecord` / `LoadGoodPriceRecord` 保存到数据库/从数据库加载，以 `Script-MapName` 为键。

### 12.5 出售回购 Bug 修复

`AddItemToGoodsList`（:2233）有文档化的 bug 修复——原始代码直接添加指针（`ItemList.add(UserItem)`），导致客户端从背包移除物品时数据损坏。修复后创建新的 `pTUserItem` 副本。

### 12.6 普通 vs 特殊修理耐久衰减

- **普通修理**：永久降低 `DuraMax`，公式 `DuraMax -= (DuraMax - Dura) / 30`。这意味着物品最终会报废。
- **特殊修理**：不降低 `DuraMax`，但费用为普通的 3 倍。

### 12.7 脚本加载路径差异

- **任务 NPC**（`m_boIsQuest = True`）：从 `Npc_def/` 目录加载，文件名 `CharName-MapName.txt`
- **商人 NPC**：从 `Market_def/` 目录加载，文件名 `Script-MapName.txt`

### 12.8 婚姻/师徒系统

代码中大量注释掉的婚姻/师徒功能（:94-102, :191-200, :310-312 等）——此服务端版本已移除该功能。

---

## 十三、Go 实现现状对照

> 基于 `cmd/server/` 和 `cmd/client/` 当前代码的简要对照。详细完成度参见 `doc/客户端服务端开发计划.md` 第十章。

| 子系统 | Delphi | Go 现状 | 完成度 |
|--------|--------|---------|--------|
| NPC 对象 | 5 层类继承（TNormNpc/TMerchant/TGuildOfficial/TTrainer/TCastleOfficial） | `NpcObject` 仅 21 行，无子类 | ~5% |
| NPC 生成 | 数据库驱动，数百 NPC 定义 | 硬编码 1 个 NPC | ~5% |
| 脚本解析器 | 完整（[@label]/#IF/#ACT/#SAY/#ELACT/#ELSE） | 核心功能可用 | ~80% |
| 脚本条件 | ~66 种命令 | 17 种 | ~26% |
| 脚本动作 | ~73 种命令 | ~30 种（含 3 个空桩） | ~41% |
| 变量系统 | P/G/D/M/A 五类变量 + 字符串 + 列表 + 持久化 | `[10]int` 数组 | ~20% |
| 商店买卖 | 完整（库存/补货/动态定价/城堡税） | 核心买卖可用，无库存/补货/动态定价 | ~60% |
| 修理 | 普通 + 特殊修理（耐久衰减） | 仅普通修理 | ~50% |
| 仓库 | 39 格 + 密码锁 + 持久化 | 39 格，无密码/持久化 | ~30% |
| 武器升级 | 完整（材料分析/概率/属性加成） | 未实现 | 0% |
| 制药 | 完整（配方/材料/收费） | 未实现 | 0% |
| 传送 NPC | 脚本 MAPMOVE/MAP | 完整 | 100% |
| 任务脚本 | 完整任务系统集成 | 未实现 | 0% |
| 行会 NPC | 创建/战争/攻城 | 基础创建 | ~30% |
| 城堡 NPC | 完整城堡管理 | 未实现 | 0% |
| 训练 NPC | 战斗训练 | 未实现 | 0% |
| 婚姻/师徒 | 已注释移除 | 未实现 | 0% |
| 协议消息 | 全部 CM/SM 定义 | 全部已定义并路由 | 100% |
| 客户端对话 UI | 完整（脚本渲染/链接/居中） | 完整（559 行 uinpc.go） | ~90% |
| 客户端商店 UI | 完整（买/卖/修/仓/制药） | 买/卖/修/仓可用 | ~70% |
| 客户端 NPC 渲染 | 3 方向 + 特效 + Npc.wil | 3 方向 + Npc.wil | ~80% |

**总体评估**：Go 实现形成了一个**可工作的垂直切片**——可以点击 NPC、查看脚本对话、导航到买/卖/修/仓、执行交易。但广度约为 Delphi 参考的 **15-25%**。最大差距在于脚本命令库（315 vs ~47）、变量系统、NPC 定义/生成基础设施、以及商人特有功能（库存、回购、费率、移动）。
