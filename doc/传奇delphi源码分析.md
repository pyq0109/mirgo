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

### 2.3 角色动画系统

动画系统定义在 `Actor.pas` 中，是客户端最复杂的模块之一。

#### 2.3.1 核心数据结构 TActionInfo

每个动画动作由 5 个字段定义（Actor.pas:35）：

```pascal
TActionInfo = packed record
  start   : Word;  // 起始帧索引（在 WIL 文件中的位置）
  frame   : Word;  // 实际动画帧数
  skip    : Word;  // 每个方向之间的填充帧数
  ftime   : Word;  // 每帧显示时间（毫秒）
  usetick : Word;  // 速度控制（用于走/跑）
end;
```

**帧索引计算公式**（核心公式，贯穿整个动画系统）：

```
实际帧 = action.start + direction × (action.frame + action.skip) + 当前帧偏移
结束帧 = action.start + direction × (action.frame + action.skip) + action.frame - 1
```

- `direction`：0~7（8方向）
- `action.frame + action.skip`：每个方向消耗的总帧槽位
- WIL 文件中所有方向连续存储

**示例**：人类站立 HA.ActStand = {start:0, frame:4, skip:4}：
- 方向0(上)：帧 0~3（`0 + 0×(4+4) = 0`）
- 方向3(右下)：帧 24~27（`0 + 3×(4+4) = 24`）
- 方向7(左上)：帧 56~59（`0 + 7×(4+4) = 56`）

WIL 文件中的帧布局：`[dir0: f0 f1 f2 f3 _ _ _ _] [dir1: f0 f1 f2 f3 _ _ _ _] ...`

#### 2.3.2 人类角色动画（HA 模板）

人类使用 `HA` 模板（THumanAction，Actor.pas:75），包含 14 个动作：

| 动作 | start | frame | skip | ftime | usetick | 每方向帧数 |
|------|-------|-------|------|-------|---------|-----------|
| Stand（站立） | 0 | 4 | 4 | 200ms | 0 | 8 |
| Walk（行走） | 64 | 6 | 2 | 90ms | 2 | 8 |
| Run（跑步） | 128 | 6 | 2 | 120ms | 3 | 8 |
| RushLeft（左冲） | 128 | 3 | 5 | 120ms | 3 | 8 |
| RushRight（右冲） | 131 | 3 | 5 | 120ms | 3 | 8 |
| WarMode（战斗姿态） | 192 | 1 | 0 | 200ms | 0 | 1 |
| Hit（普通攻击） | 200 | 6 | 2 | 85ms | 0 | 8 |
| HeavyHit（重击） | 264 | 6 | 2 | 90ms | 0 | 8 |
| BigHit（大招） | 328 | 8 | 0 | 70ms | 0 | 8 |
| FireHitReady（烈火准备） | 192 | 6 | 4 | 70ms | 0 | 10 |
| Spell（施法） | 392 | 6 | 2 | 60ms | 0 | 8 |
| Sitdown（坐下） | 456 | 2 | 0 | 300ms | 0 | 2 |
| Struck（受击） | 472 | 3 | 5 | 70ms | 0 | 8 |
| Die（死亡） | 536 | 4 | 4 | 120ms | 0 | 8 |

**注意**：
- RushLeft 和 RushRight 复用 Run 的起始帧（128），但帧数不同（3帧 vs 6帧）
- WarMode 和 FireHitReady 共享起始帧 192
- usetick 字段用于控制移动速度消耗（Walk=2, Run=3）

**每套外观固定 600 帧**（`HUMANFRAME` 常量，Actor.pas:14）。

**人类三层同步渲染**（THumActor.LoadSurface，Actor.pas:3692）：

人类角色由 3~4 层叠加渲染，所有层共享同一个 `m_nCurrentFrame`，保证动画同步：

| 层 | WIL 文件 | 偏移计算 | 说明 |
|---|---|---|---|
| 身体 | Hum.wil | `600 × dress_id` | 按服装/盔甲 ID 偏移 |
| 头发 | Hair.wil | `600 × (hair×2 + sex)` | 按发型和性别偏移 |
| 武器 | Weapon.wil | `600 × weapon_id` | 按武器 ID 偏移 |
| 翅膀/特效 | HumEffect.wil | `600 × (effect-1)` | 按特效 ID 偏移 |

**绘制顺序**：`WORDER[sex, frame]` 查找表（600条/性别，Actor.pas:461）决定武器在身体前（1）还是后（0）绘制，解决 8 方向精灵中武器与身体的遮挡关系。

#### 2.3.3 怪物动画系统

怪物动画的选择分三步：**Race → 动画模板**，**Appr → WIL文件 + 帧偏移**。

**第一步：服务端发送 feature 整数**

服务端通过 `MakeMonsterFeature(RaceImg, Weapon, Appr)` 编码外观信息（Grobal2.pas:2671）：

```
feature = MakeLong(MakeWord(RaceImg, Weapon), Appr)

RACEfeature = LoByte(LoWord(feature))  → m_btRace（种族 ID）
APPRfeature = HiWord(feature)          → m_wAppearance（外观值）
```

**第二步：Race → 动画模板（GetRaceByPM）**

`GetRaceByPM(race, Appr)` 函数（Actor.pas:818）将种族 ID 映射到 `TMonsterAction` 模板。每个模板定义 7 个动作：站立/行走/攻击/暴击/受击/死亡/尸体。

| Race | 模板 | 说明 |
|------|------|------|
| 9 | MA9 | 攻击复用行走帧 |
| 10 | MA10 | 标准 8 帧怪物 |
| 11 | MA11 | 10 帧怪物 |
| 13,14,17,18,23 | MA14 | 扩展死亡动画 |
| 15,22 | MA15 | |
| 19,20,21,37,40,45,52,53,64-69,73,74,79 | **MA19** | **最常用模板** |
| 32 | MA24 | 有暴击攻击 |
| 33 | MA25 | 蜈蚣王 |
| 43 | MA21 | 蜂后（不移动） |
| 47 | MA22 | 石像类 |
| 50 | 按 Appr 分派 | NPC，见下文 |
| 60-62,70-72 | MA33 | |
| 75,77 | MA39 | 石像怪物 |
| 84-89 | MA45 | 龙形雕像 |
| 98 | MA27 | 城墙 |
| 99 | MA26 | 城门 |

**第三步：Appr → WIL 文件 + 帧偏移**

`aGetMonImg(appr)` 函数（Actor.pas:958）选择 WIL 文件：

```pascal
case (appr div 10) of
  0:  Result := WMonImg;      // Mon1.wil
  1:  Result := WMon2Img;     // Mon2.wil
  2:  Result := WMon3Img;     // Mon3.wil
  ...
  17: Result := WMon18Img;    // Mon18.wil
  80: Result := WDragonImg;   // Dragon.wil
  90: Result := WEffectImg;   // Effect.wil
end;
```

`GetOffset(appr)` 函数（Actor.pas:1003）计算帧偏移：

```
nrace = appr div 10   // 选哪个 WIL 文件
npos  = appr mod 10   // 该 WIL 文件内的第几种怪物

| nrace | 每种怪物帧数 | 计算方式 |
|-------|-------------|---------|
| 0     | 280         | npos × 280 |
| 1     | 230         | npos × 230 |
| 2,3,7~12 | 360     | npos × 360（默认） |
| 5     | 430         | npos × 430 |
| 6     | 440         | npos × 440 |
| 13~27 | 不规则      | 硬编码偏移表 |
```

**怪物纹理加载**（TActor.LoadSurface，Actor.pas:1918）：

```pascal
mimg := GetMonImg(m_wAppearance);  // 按 Appr 选 WIL 文件
m_BodySurface := mimg.GetCachedImage(
  GetOffset(m_wAppearance) + m_nCurrentFrame,  // 基础偏移 + 当前帧
  m_nPx, m_nPy);
```

最终纹理索引 = `GetOffset(appr) + m_nCurrentFrame`

#### 2.3.4 怪物动画模板表（MA9~MA47）

每个 TMonsterAction 模板定义 7 个动作的帧布局：

| 模板 | 每方向帧数 | Stand | Walk | Attack | Critical | Struck | Die | Death | 特点 |
|------|-----------|-------|------|--------|----------|--------|-----|-------|------|
| MA9 | 8 | 0/1/7 | 64/6/2 | 64/6/2 | — | 64/6/2 | 0/1/7 | 0/1/7 | 攻击复用行走帧 |
| MA10 | 8 | 0/4/4 | 64/6/2 | 128/4/4 | — | 192/2/0 | 208/4/4 | 272/1/0 | 标准 8 帧 |
| MA11 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 340/1/0 | 10 帧 |
| MA12 | 8 | 0/4/4 | 64/6/2 | 128/6/2 | — | 192/2/0 | 208/4/4 | 272/1/0 | |
| MA14 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 340/10/0 | 扩展死亡 |
| MA15 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 1/1/0 | |
| MA16 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/4/6 | 0/1/0 | 慢攻击(160ms) |
| MA17 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 340/1/0 | 快站立(60ms) |
| MA19 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 340/1/0 | **最常用** |
| MA20 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | — | 240/2/0 | 260/10/0 | 340/10/0 | 长死亡 |
| MA21 | 10 | 0/4/6 | 0/0/0 | 10/6/4 | — | 20/2/0 | 30/10/0 | 0/0/0 | 不移动 |
| MA22 | 10 | 80/4/6 | 160/6/4 | 240/6/4 | — | 320/2/0 | 340/10/0 | 0/6/4 | 石像类 |
| MA23 | 10 | 20/4/6 | 100/6/4 | 180/6/4 | — | 260/2/0 | 280/10/0 | 0/20/0 | |
| MA24 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | **240/6/4** | 320/2/0 | 340/10/0 | 420/1/0 | **有暴击** |
| MA25 | 10 | 0/4/6 | 70/10/0 | 20/6/4 | 10/6/4 | 50/2/0 | 60/10/0 | 80/10/0 | |
| MA26 | 8 | 0/1/7 | 0/0/0 | 56/6/2 | 64/6/2 | 0/4/4 | 24/10/0 | 0/0/0 | 城门，双攻击 |
| MA27 | 8 | 0/1/7 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/10/0 | 0/0/0 | 城墙 |
| MA28 | 10 | 80/4/6 | 160/6/4 | 0/6/4 | — | 240/2/0 | 260/10/0 | 0/10/0 | |
| MA29 | 10 | 80/4/6 | 160/6/4 | 240/6/4 | 0/10/0 | 320/2/0 | 340/10/0 | 0/10/0 | |
| MA30 | 10 | 0/4/6 | 0/10/0 | 10/6/4 | 10/6/4 | 20/2/0 | 30/20/0 | 0/10/0 | |
| MA31 | 10 | 0/4/6 | 0/10/0 | 10/6/4 | 0/6/4 | 0/2/8 | 20/10/0 | 0/10/0 | |
| MA32 | 10 | 0/1/9 | 0/6/4 | 0/6/4 | 0/6/4 | 0/2/8 | 80/10/0 | 80/10/0 | |
| MA33 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | 340/6/4 | 240/2/0 | 260/10/0 | 260/10/0 | |
| MA34 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | 320/6/4 | 400/2/0 | 420/20/0 | 420/20/0 | |
| MA35 | 10 | 0/4/6 | 0/0/0 | 30/10/0 | — | 0/1/9 | 0/0/0 | 0/0/0 | NPC 用 |
| MA36 | 10 | 0/4/6 | 0/0/0 | 30/20/0 | — | 0/1/9 | 0/0/0 | 0/0/0 | |
| MA37 | 10 | 30/4/6 | 0/0/0 | 30/4/6 | — | 0/1/9 | 0/0/0 | 0/0/0 | |
| MA38 | 10 | 0/4/6 | 0/0/0 | 80/6/4 | — | 0/0/0 | 0/0/0 | 0/0/0 | |
| MA39 | 10 | 0/4/6 | 0/0/0 | 10/6/4 | — | 20/2/0 | 30/10/0 | 0/0/0 | 石像 |
| MA40 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | 580/20/0 | 240/2/0 | 260/20/0 | 260/20/0 | 大暴击 |
| MA41 | 10 | 0/4/6 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 纯装饰 NPC |
| MA42 | 10 | 0/4/6 | 10/8/2 | 0/0/0 | — | 0/0/0 | 30/10/0 | 30/10/0 | |
| MA43 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | 160/6/4 | 240/2/0 | 260/10/0 | 340/10/0 | 攻击=暴击 |
| MA44 | 10 | 0/10/0 | 10/6/4 | 20/6/4 | 40/10/0 | 40/2/8 | 30/6/4 | 0/0/0 | |
| MA45 | 10 | 0/10/0 | 0/10/0 | 10/10/0 | 10/10/0 | 0/1/9 | 0/1/9 | 0/1/9 | 龙形雕像 |
| MA46 | 10 | 0/20/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 0/0/0 | 20帧站立 |
| MA47 | 10 | 0/4/6 | 80/6/4 | 160/6/4 | 260/6/4 | 240/2/0 | 524/6/0 | 524/6/0 | |

表中格式：`start/frame/skip`，"—" 表示该模板无此动作。

#### 2.3.5 NPC 动画

NPC 固定使用 `Npc.wil`，每种 NPC 占 60 帧（`MERCHANTFRAME`）。

`GetNpcOffset(nAppr)` 函数（Actor.pas:1156）计算帧偏移：

```pascal
case nAppr of
  0..22:   Result := nAppr * 60;           // 每种 60 帧
  23:      Result := 1380;
  24,25:   Result := (nAppr-24)*60 + 1470;
  26,28..31,33..41: Result := (nAppr-26)*60 + 1620;
  // ...更多条目，部分 NPC 共享偏移或使用 20 帧
end;
```

NPC 只有 3 个方向（`m_btDir := m_btDir mod 3`），使用 MA10 模板。

#### 2.3.6 地图物体动画（Objects.wil）

地图前景物体（wFrImg）通过地图数据中的 `btAniFrame` 字段控制动画（PlayScn.pas:1072）：

- `btAniFrame = 0` → 静态图像
- `btAniFrame > 0` → 动画帧数（最高位 0x80 表示 Alpha 混合绘制）
- `btAniTick` → 动画速度（值越大越慢）
- `btArea` → 选择 Objects1~Objects15.wil 文件

动画帧计算公式：

```pascal
ani := btAniFrame and $7F;  // 去掉标志位，获取帧数
blend := (btAniFrame and $80) > 0;  // 是否 Alpha 混合
currentFrame := fridx + (globalAniCount mod (ani + ani*anitick)) div (1 + anitick);
```

#### 2.3.7 自定义怪物模板

客户端还支持从外部文件加载自定义动画模板（MShare.pas:858）：

```pascal
// 文件路径：Graphics\Monster\%d.pm（%d = appr）
function GetMonAction(nAppr: Integer): pTMonsterAction;
begin
  sFileName := format(MONPMFILE, [nAppr]);
  if FileExists(sFileName) then begin
    // 从二进制文件读取 TMonsterAction
  end;
end;
```

在创建角色时调用（PlayScn.pas:2138）：`m_Action := GetMonAction(m_wAppearance);`

#### 2.3.8 完整数据流总结

**怪物动画完整流程**：

1. 服务端发送 `feature` 整数
2. `RACEfeature` → `m_btRace`，`APPRfeature` → `m_wAppearance`
3. `GetRaceByPM(race, appr)` → 返回 `TMonsterAction` 模板（如 `@MA19`）
4. `GetOffset(appr)` → 计算 WIL 文件内的基础帧偏移
5. `aGetMonImg(appr)` → 按 `appr div 10` 选择 MonX.wil 文件
6. 收到动作消息（SM_TURN、SM_HIT 等）时，`CalcActorFrame` 计算：
   - `m_nStartFrame = template.ActXxx.start + dir × (template.ActXxx.frame + template.ActXxx.skip)`
   - `m_nEndFrame = m_nStartFrame + template.ActXxx.frame - 1`
7. 渲染时 `LoadSurface` 加载：`WIL.GetCachedImage(GetOffset(appr) + m_nCurrentFrame)`
8. `m_nCurrentFrame` 从 `m_nStartFrame` 递增到 `m_nEndFrame`，间隔 `ftime` 毫秒

**人类动画完整流程**：

1. 服务端发送 `feature` 整数
2. `DRESSfeature` → `m_btDress`，`HAIRfeature` → `m_btHair`，`WEAPONfeature` → `m_btWeapon`
3. 固定使用 `HA` 模板（不需查找）
4. `m_nBodyOffset = 600 × m_btDress`（Hum.wil）
5. `m_nHairOffset = 600 × (m_btHair×2 + m_btSex)`（Hair.wil）
6. `m_nWeaponOffset = 600 × m_btWeapon`（Weapon.wil）
7. 同样的帧计算：`m_nStartFrame = HA.ActXxx.start + dir × (HA.ActXxx.frame + HA.ActXxx.skip)`
8. 三层纹理使用相同的 `m_nCurrentFrame`，保证同步
9. `WORDER[sex, frame]` 决定武器绘制顺序

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

**客户端加载的WIL资源清单** (Share.pas, ClMain.pas:48-77)：

| 资源文件 | 用途 | 变量名 |
|---------|------|--------|
| Prguse.wil / Prguse2.wil / Prguse3.wil | 主界面UI素材 | g_WMainImages / g_WMain2Images / g_WMain3Images |
| ChrSel.wil | 选角界面 | g_WChrSelImages |
| Hum.wil | 人类角色身体 | g_WHumImgImages |
| Hair.wil | 角色发型 | g_WHairImgImages |
| Weapon.wil | 武器外观 | g_WWeaponImages |
| HumEffect.wil | 翅膀/特效 | g_WHumEffectImages |
| Mon1~Mon28.wil | 怪物图像(28个文件) | WMonImg ~ WMon28Img |
| Dragon.wil | 龙形怪物 | WDragonImg |
| Effect.wil | 通用特效 | WEffectImg |
| Items.wil | 背包物品图标 | g_WBagItemImages |
| DnItems.wil | 地面物品图标 | g_WDnItemImages |
| StateItem.wil | 装备状态图标 | g_WStateItemImages |
| Magic.wil / Magic2.wil | 魔法特效 | g_WMagicImages / g_WMagic2Images |
| MagIcon.wil | 技能图标 | g_WMagIconImages |
| Npc.wil | NPC外观 | g_WNpcImages |
| Event.wil | 事件特效 | g_WEventEffectImages |
| Tiles.wil | 地图背景图块 | g_WTilesImages |
| SmTiles.wil | 地图中间层图块 | g_WSmTilesImages |
| Objects.wil ~ ObjectsN.wil | 地图前景物体 | g_WObjectImages |
| mmap.wil | 小地图 | g_WMMapImages |

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

**消息头格式**（TMsgHeader，16字节，定义在 Grobal2.pas:1036）：

```
TMsgHeader:
  dwCode        : DWord   — 魔数 0xAA55AA55 (RUNGATECODE)
  nSocket       : Integer — 客户端 Socket 标识
  wGSocketIdx   : Word    — 网关 Socket 索引
  wIdent        : Word    — 消息类型 (GM_OPEN/GM_CLOSE/GM_DATA)
  wUserListIndex: Word    — 用户列表索引
  nLength       : Integer — 后续数据长度
```

**网关消息类型** (GM_*):
- `GM_OPEN = 1` — 新客户端连接
- `GM_CLOSE = 2` — 客户端断开
- `GM_DATA = 5` — 游戏数据
- `GM_RECEIVE_OK = 7` — 流控确认

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

**注意**：
- 默认配置启动 8 个 RunGate 实例（端口 7200, 7300, 7400, 7500, 7600, 7700, 7800, 7900），每个间隔 100
- M2Server 最多支持 20 个网关连接（`g_GateArr: array[0..19]`，RunSock.pas:43）
- 每个 RunGate 最多管理 1000 个会话（`GATEMAXSESSION = 1000`，GateShare.pas:7）

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

**装备槽位** (1.50版10个槽位，Grobal2.pas:28)：

```
U_DRESS(0)      — 衣服
U_WEAPON(1)     — 武器
U_RIGHTHAND(2)  — 盾牌/右手
U_NECKLACE(3)   — 项链
U_HELMET(4)     — 头盔
U_ARMRINGL(5)   — 左手镯
U_ARMRINGR(6)   — 右手镯
U_RINGL(7)      — 左戒指
U_RINGR(8)      — 右戒指
U_BUJUK(9)      — 符咒位
```

**1.70版扩展到13个槽位**（注释在 Grobal2.pas:41）：
```
U_BELT(10)      — 腰带
U_BOOTS(11)     — 鞋子
U_CHARM(12)     — 宝石
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

Common/EDcode.pas（360行）实现 6Bit 编码算法，是网络通信的核心。

**编码算法**（Encode6BitBuf，EDcode.pas:94）：

1. 每字节从 bit 2 开始，每次取 6 位
2. 每个 6-bit 块加上 `$3C` (60) 产生可打印 ASCII 字符
3. `nRestCount` 累积到 6 时，同时输出已构造字节和剩余字节
4. NEWMODE（未启用）会先用 XOR 掩码处理

**解码算法**（Decode6BitBuf，EDcode.pas:150）：

1. 每个编码字符减去 `$3C` 恢复 6-bit 值
2. 使用位位置跟踪器 (`nBitPos`) 在 2, 4, 6 间循环
3. 使用掩码 `$FC, $F8, $F0, $E0, $C0` 提取位
4. 累积足够位（>= 8）时重建完整字节

**关键函数**：
- `EncodeMessage(msg: TDefaultMessage): string` — 编码消息结构
- `DecodeMessage(str: string): TDefaultMessage` — 解码消息结构
- `EncodeString/DecodeString` — 编解码字符串
- `EncodeBuffer/DecodeBuffer` — 编解码任意缓冲区

**查表法**：EncodeBitMasks/DecodeBitMasks（各256字节）仅在 NEWMODE 下使用（当前为 OLDMODE）

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
完整消息结构（M2Server ↔ RunGate）：
[TMsgHeader] + [TDefaultMessage] + [消息体(可选)]

TMsgHeader (16字节，Grobal2.pas:1036):
  dwCode         : DWord   — 魔数 0xAA55AA55 (RUNGATECODE)
  nSocket        : Integer — 客户端 Socket 标识
  wGSocketIdx    : Word    — 网关 Socket 索引
  wIdent         : Word    — 消息类型 (GM_OPEN/GM_CLOSE/GM_DATA)
  wUserListIndex : Word    — 用户列表索引
  nLength        : Integer — 后续数据长度

TDefaultMessage (16字节，Grobal2.pas:498，经6Bit编码后约22字符):
  Recog:Integer + Ident:Word + Param:Word + Tag:Word + Series:Word

M2Server ↔ DBSocket 文本协议：
  发送格式: #<queryID>/<encodedMsg><checkCode>!
  响应格式: #<queryID>/<checkFlag><encodedResponse>!
```

### 5.2 角色存档结构

`THumDataInfo` (SIZEOFTHUMAN = 3628字节，M2Share.pas:136)：

- 角色基础信息：名称/职业/性别/等级/HP/MP/经验/金币/坐标
- 装备数组[0..9]：TUserItem（1.50版10个槽位）
- 背包数组[0..45]：TUserItem（MAXBAGITEM=46）
- 技能数组[0..19]：TUserMagic（HOWMANYMAGICS=20）
- 仓库数组[0..49]：TUserItem（50格）
- 任务标志、行会信息

**TAbility 结构**（50字节，Grobal2.pas:695）：

```
Level         : Word     等级
AC/MAC        : DWord    物防/魔防 (Lo=基础, Hi=最大)
DC/MC/SC      : DWord    物攻/魔攻/道攻
HP/MaxHP      : Word     当前/最大生命
MP/MaxMP      : Word     当前/最大魔法
Exp/MaxExp    : DWord    当前/升级经验
Weight/MaxWeight : Word  当前/最大负重
WearWeight/MaxWearWeight : Byte 穿戴负重
HandWeight/MaxHandWeight : Byte 手持负重
```

**TUserItem 结构**（24字节，Grobal2.pas 定义）：

```
MakeIndex : Integer      唯一实例 ID
wIndex    : Word         物品定义索引（StdItemList 中的 1-based）
Dura      : Word         当前耐久
DuraMax   : Word         最大耐久
btValue   : [0..13] Byte 自定义值（升级属性等）
```

**btValue 按物品类型解释**：
- 武器: [0]=DC加, [1]=MC加, [2]=SC加, [3]=AC加, [4]=MAC加, [5]=准确, [6]=速度, [7]=神圣
- 衣服: [0]=AC加, [1]=MAC加, [2]=DC加, [3]=MC加, [4]=SC加
- 饰品: [0]=AC加, [1]=MAC加, [2]=DC加, [3]=MC加, [4]=SC加

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

### 5.6 Feature 编码（外观）

32 位整数编码外观信息（Grobal2.pas 定义）：

**人类 (Race=0)**：
```
Bits [31..24] = Dress (衣服外观)
Bits [23..16] = Hair (发型)
Bits [15..8]  = Weapon (武器外观)
Bits [7..0]   = RaceImg (人类=0)
性别 = Dress mod 2 (奇=男, 偶=女)
```

**怪物 (Race>0)**：
```
Bits [31..16] = Appr (外观 ID)
Bits [15..8]  = Weapon
Bits [7..0]   = RaceImg
```

**方向常量**：
```
DR_UP=0, DR_UPRIGHT=1, DR_RIGHT=2, DR_DOWNRIGHT=3
DR_DOWN=4, DR_DOWNLEFT=5, DR_LEFT=6, DR_UPLEFT=7
```

**关键常量**：
```
UNITX = 48              瓦片宽度（像素）
UNITY = 32              瓦片高度（像素）
LOGICALMAPUNIT = 40     逻辑地图单元
MAXBAGITEM = 46         最大背包物品数
HOWMANYMAGICS = 20      最大学习魔法数
MAX_STATUS_ATTRIBUTE = 12 最大状态效果数
HUMANFRAME = 600        人类精灵帧数
MONFRAME = 280          怪物精灵帧数
MERCHANTFRAME = 60      商人精灵帧数
```

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

---

## 七、完整消息 ID 参考（Grobal2.pas）

### 7.1 客户端→服务端 (CM_*)

**角色管理**：
| ID | 名称 | 说明 |
|----|------|------|
| 80 | CM_QUERYUSERNAME | 查询用户名 |
| 81 | CM_QUERYBAGITEMS | 查询背包 |
| 82 | CM_QUERYUSERSTATE | 查询用户状态 |
| 100 | CM_QUERYCHR | 查询角色列表 |
| 101 | CM_NEWCHR | 创建角色 |
| 102 | CM_DELCHR | 删除角色 |
| 103 | CM_SELCHR | 选择角色 |
| 104 | CM_SELECTSERVER | 选择服务器 |

**游戏操作**：
| ID | 名称 | 说明 |
|----|------|------|
| 1000 | CM_DROPITEM | 丢弃物品 |
| 1001 | CM_PICKUP | 拾取物品 |
| 1002 | CM_OPENDOOR | 开门 |
| 1003 | CM_TAKEONITEM | 穿戴装备 |
| 1004 | CM_TAKEOFFITEM | 脱下装备 |
| 1006 | CM_EAT | 使用物品 |
| 1007 | CM_BUTCH | 挖肉 |
| 1010 | CM_CLICKNPC | 点击 NPC |
| 1011 | CM_MERCHANTDLGSELECT | NPC 对话选择 |
| 1013 | CM_USERSELLITEM | 卖物品 |
| 1014 | CM_USERBUYITEM | 买物品 |
| 1019-1022 | CM_GROUP* | 组队操作 |
| 1025-1030 | CM_DEAL* | 交易操作 |
| 1031-1032 | CM_*STORAGE* | 仓库操作 |
| 1035-1045 | CM_GUILD* | 行会操作 |

**战斗动作**：
| ID | 名称 | 说明 |
|----|------|------|
| 3010 | CM_TURN | 转身 |
| 3011 | CM_WALK | 行走 |
| 3013 | CM_RUN | 跑步 |
| 3014 | CM_HIT | 普通攻击 |
| 3015 | CM_HEAVYHIT | 重击 |
| 3016 | CM_BIGHIT | 大招 |
| 3017 | CM_SPELL | 施法 |
| 3018 | CM_POWERHIT | 刺杀 |
| 3019 | CM_LONGHIT | 攻杀 |
| 3024 | CM_WIDEHIT | 半月弯刀 |
| 3025 | CM_FIREHIT | 烈火剑法 |
| 3030 | CM_SAY | 说话 |
| 3035 | CM_HORSERUN | 骑马跑步 |
| 3036 | CM_CRSHIT | 十字攻击 |
| 3038 | CM_TWINHIT | 双重攻击 |

**登录/账号**：
| ID | 名称 | 说明 |
|----|------|------|
| 2000 | CM_PROTOCOL | 协议版本 |
| 2001 | CM_IDPASSWORD | 发送账号密码 |
| 2002 | CM_ADDNEWUSER | 注册新账号 |
| 2003 | CM_CHANGEPASSWORD | 修改密码 |

### 7.2 服务端→客户端 (SM_*)

**移动/动画** (0-34)：
| ID | 名称 | 说明 |
|----|------|------|
| 10 | SM_TURN | 转身 |
| 11 | SM_WALK | 行走 |
| 13 | SM_RUN | 跑步 |
| 14-26 | SM_HIT~SM_TWINHIT | 各种攻击动画 |
| 31 | SM_STRUCK | 被击中 |
| 32 | SM_DEATH | 死亡 |
| 34 | SM_NOWDEATH | 立即死亡 |

**状态/信息** (40-54)：
| ID | 名称 | 说明 |
|----|------|------|
| 40 | SM_HEAR | 聊天消息 |
| 41 | SM_FEATURECHANGED | 外观变化 |
| 42 | SM_USERNAME | 用户名响应 |
| 44 | SM_WINEXP | 获得经验 |
| 45 | SM_LEVELUP | 升级 |
| 50 | SM_LOGON | 登录确认 |
| 51 | SM_NEWMAP | 新地图 |
| 52 | SM_ABILITY | 角色属性 |
| 53 | SM_HEALTHSPELLCHANGED | HP/MP 变化 |

**系统消息** (100-104)：
| ID | 名称 | 说明 |
|----|------|------|
| 100 | SM_SYSMESSAGE | 系统消息 |
| 101 | SM_GROUPMESSAGE | 组队消息 |
| 102 | SM_CRY | 喊话 |
| 103 | SM_WHISPER | 私聊 |
| 104 | SM_GUILDMESSAGE | 行会消息 |

**物品** (200-212)：
| ID | 名称 | 说明 |
|----|------|------|
| 200 | SM_ADDITEM | 添加物品 |
| 201 | SM_BAGITEMS | 背包内容 |
| 202 | SM_DELITEM | 删除物品 |
| 210 | SM_ADDMAGIC | 添加魔法 |
| 211 | SM_SENDMYMAGIC | 发送魔法列表 |

**登录流程** (500-533)：
| ID | 名称 | 说明 |
|----|------|------|
| 500 | SM_CERTIFICATION_SUCCESS | 认证成功 |
| 501 | SM_CERTIFICATION_FAIL | 认证失败 |
| 520 | SM_QUERYCHR | 角色列表 |
| 525 | SM_STARTPLAY | 开始游戏 |
| 529 | SM_PASSOK_SELECTSERVER | 密码验证通过 |
| 530 | SM_SELECTSERVER_OK | 服务器选择成功 |

**游戏操作** (600-772)：
| ID | 名称 | 说明 |
|----|------|------|
| 612-614 | SM_*DOOR* | 门操作 |
| 615-620 | SM_TAKEON/OFF* | 穿脱装备 |
| 634 | SM_CHANGEMAP | 地图切换 |
| 638 | SM_MAGICFIRE | 魔法效果 |
| 643 | SM_MERCHANTSAY | NPC 说话 |
| 645-652 | SM_*GOODS* | 商店操作 |
| 660-667 | SM_GROUP* | 组队操作 |
| 673-687 | SM_DEAL* | 交易操作 |
| 700-707 | SM_STORAGE* | 仓库操作 |
| 750-772 | SM_GUILD* | 行会操作 |

**传送/事件** (800-811)：
| ID | 名称 | 说明 |
|----|------|------|
| 800-801 | SM_SPACEMOVE_* | 传送效果 |
| 802 | SM_RECONNECT | 重连 |
| 804-805 | SM_SHOW/HIDEEVENT | 事件显示/隐藏 |
| 1100-1101 | SM_OPEN/CLOSEHEALTH | 显示/隐藏 HP 条 |

**跨服消息** (SS_*)：
| ID | 名称 | 说明 |
|----|------|------|
| 100 | SS_OPENSESSION | 打开会话 |
| 101 | SS_CLOSESESSION | 关闭会话 |
| 102 | SS_SOFTOUTSESSION | 软断开 |
| 103 | SS_SERVERINFO | 服务器信息 |
| 104 | SS_KEEPALIVE | 保活 |
| 111 | SS_KICKUSER | 踢出用户 |

**数据库消息** (DB_*)：
| ID | 名称 | 说明 |
|----|------|------|
| 100 | DB_LOADHUMANRCD | 加载角色记录 |
| 101 | DB_SAVEHUMANRCD | 保存角色记录 |
| 1100 | DBR_LOADHUMANRCD | 加载响应 |
| 1102 | DBR_SAVEHUMANRCD | 保存响应 |
| 2000 | DBR_FAIL | 失败响应 |
