# Delphi 版怪物 AI 系统实现

> 基于 `asset/delphi/M2Server/` 源码（commit `98711da`）的完整技术描述。
> 所有引用格式为 `文件:行号`。

## 一、架构总览

MIR2 服务端的怪物 AI 系统是**隐式状态机 + 继承多态 + Race 工厂**结构：没有显式的状态枚举，状态由布尔标志和目标指针隐式编码；不同 AI 行为通过类继承和方法覆写实现；怪物类由数据库 `btRace` 字段在刷怪时工厂选择。

```
┌──────────────────────────────────────────────────────────────────────────┐
│  引擎调度层 TUserEngine (UsrEngn.pas)                                     │
│    ProcessMonsters()    每 tick 轮询所有 Mongen 刷怪器 + 驱动怪物 Run      │
│    AddBaseObject()      Race ID → 类工厂（case 语句，~25 分支）            │
│    MonInitialize()      从 TMonInfo 数据库加载属性                         │
│    RegenMonsters()      在刷怪点范围内随机放置新怪物                        │
├──────────────────────────────────────────────────────────────────────────┤
│  对象基类层 (ObjBase.pas)                                                  │
│    TBaseObject          消息队列、HP/MP、战斗公式、死亡、状态效果           │
│    └── TAnimalObject    视野搜索、目标选取、移动、逃跑、闲逛               │
│         └── TMonster    AI 主循环 Run()、Think 防卡、攻击分发              │
├──────────────────────────────────────────────────────────────────────────┤
│  特化子类层 (ObjMon.pas / ObjMon2.pas / ObjMon3.pas / ObjAxeMon.pas)      │
│    TATMonster           近战追击        TSpitSpider      远程喷吐          │
│    TChickenDeer         逃跑            TStickMonster    潜地伏击          │
│    TExplosionSpider     自爆            TDualAxeMonster  远程飞斧          │
│    TRonObject           范围攻击        TElectronicScolpionMon 闪电吸血    │
│    TScultureMonster     石化 ambush     TBeeQueen        召唤蜂群          │
│    TFireMonster         火焰光环        TFireBallMonster 火球术            │
│    TElfMonster          变形            TGuardUnit       城堡守卫          │
│    ... 共 ~40 个类                                                        │
├──────────────────────────────────────────────────────────────────────────┤
│  数据驱动层                                                                │
│    TMonInfo (Grobal2.pas:1558)    怪物属性表（race/hp/dc/ac/speed/hit）   │
│    TMonGenInfo (Grobal2.pas:1540) 刷怪点定义（地图/坐标/范围/数量/间隔）   │
│    g_Config                       全局配置（刷怪速率/视野/毒 tick 等）     │
└──────────────────────────────────────────────────────────────────────────┘
```

### 核心设计原则

1. **隐式状态机**：无 FSM 枚举。怪物状态由 `m_boDeath`（死亡）、`m_boGhost`（销毁中）、`m_boFixedHideMode`（潜地）、`m_boRunAwayMode`（逃跑）、`m_boStoneMode`（石化）等布尔标志和 `m_TargetCret`（目标指针）的有无组合表达。
2. **继承多态**：AI 差异通过覆写 `Run()`、`AttackTarget()`、`Attack()`、`Struck()`、`Die()` 等虚方法实现，而非 switch/case 行为码。
3. **Race 工厂**：刷怪时由 `TUserEngine.AddBaseObject` 的 `case nMonRace of` 语句（`UsrEngn.pas:1819-1938`）选择实例化哪个子类。Race ID 来自怪物数据库 `TMonInfo.btRace`。
4. **引擎驱动**：怪物不自行调度。`TUserEngine.ProcessMonsters`（`UsrEngn.pas:1119-1287`）每 tick 遍历所有怪物，按 `m_nRunTime` 间隔调用 `Run()`，按 `m_dwSearchTime` 间隔调用 `SearchViewRange()`。
5. **时间片预算**：刷怪有 `g_dwZenLimit` 时间预算防止单 tick 卡顿；怪物处理按 round-robin 轮询刷怪器列表。

---

## 二、AI 相关字段与数据结构

### 2.1 TBaseObject 字段（ObjBase.pas:61-532）

| 字段 | 行号 | 类型 | 用途 |
|------|------|------|------|
| `m_nViewRange` | :117 | Integer | 视野/搜索半径（格数） |
| `m_btRaceServer` | :123 | Byte | Race 类型常量（决定阵营判定） |
| `m_btHitPoint` | :125 | Byte | 命中值（攻击方） |
| `m_btSpeedPoint` | :145 | Byte | 闪避值（防御方） |
| `m_btLifeAttrib` | :148 | Byte | 生命属性（LA_UNDEAD = 不死系） |
| `m_btCoolEye` | :149 | Byte | 反隐概率（0-100） |
| `m_Master` | :158 | TBaseObject | 主人指针（召唤/驯服关系） |
| `m_boAnimal` | :177 | Boolean | 动物标志（掉肉） |
| `m_boFixedHideMode` | :179 | Boolean | 永久隐藏（潜地状态） |
| `m_boStickMode` | :180 | Boolean | 固定模式（不可移动） |
| `m_boNoAttackMode` | :182 | Boolean | 被动模式（不主动攻击） |
| `m_boCrazyMode` | :190 | Boolean | 疯狂模式（攻击一切） |
| `m_boDeath` | :202 | Boolean | 已死亡 |
| `m_boGhost` | :200 | Boolean | 销毁中 |
| `m_boCoolEye` | :229 | Boolean | 能看穿隐身 |
| `m_dwSearchTime` | :267 | LongWord | SearchViewRange 调用间隔（ms） |
| `m_dwSearchTick` | :268 | LongWord | 上次 SearchViewRange 的 tick |
| `m_dwRunTick` | :269 | LongWord | 上次 Run() 执行的 tick |
| `m_nRunTime` | :270 | Integer | Run() 调用最小间隔（ms） |
| `m_TargetCret` | :273 | TBaseObject | 当前攻击目标 |
| `m_dwTargetFocusTick` | :274 | LongWord | 上次与目标交互的时间 |
| `m_LastHiter` | :275 | TBaseObject | 最后攻击自己的对象（仇恨） |
| `m_LastHiterTick` | :276 | LongWord | 最后被攻击时间戳 |
| `m_ExpHitter` | :277 | TBaseObject | 击杀经验归属者 |
| `m_dwHitTick` | :304 | LongWord | 上次攻击 tick（冷却用） |
| `m_dwWalkTick` | :305 | LongWord | 上次移动 tick（冷却用） |
| `m_dwSearchEnemyTick` | :306 | LongWord | 上次搜索敌人 tick |
| `m_VisibleActors` | :310 | TList | 视野内可见对象列表 |
| `m_nWalkSpeed` | :319 | Integer | 移动冷却（ms） |
| `m_nWalkStep` | :320 | Integer | 连续移动步数上限（之后休息） |
| `m_nWalkCount` | :321 | Integer | 当前连续步数计数 |
| `m_dwWalkWait` | :322 | LongWord | 休息持续时间（ms） |
| `m_dwWalkWaitTick` | :323 | LongWord | 休息开始 tick |
| `m_boWalkWaitLocked` | :324 | Boolean | 当前正在休息 |
| `m_nNextHitTime` | :325 | Integer | 攻击冷却（ms） |
| `m_boNastyMode` | :368 | Boolean | 厌恶模式（攻击所有非 NPC） |

### 2.2 TAnimalObject 附加字段（ObjBase.pas:533-556）

| 字段 | 行号 | 类型 | 用途 |
|------|------|------|------|
| `m_nNotProcessCount` | :534 | Integer | 未处理消息计数 |
| `m_nTargetX` | :535 | Integer | 移动目标 X（-1 = 无） |
| `m_nTargetY` | :536 | Integer | 移动目标 Y |
| `m_boRunAwayMode` | :537 | Boolean | 逃跑模式 |
| `m_dwRunAwayStart` | :538 | LongWord | 逃跑开始 tick |
| `m_dwRunAwayTime` | :539 | LongWord | 逃跑持续时间（0 = 无限） |

### 2.3 TMonster 附加字段（ObjMon.pas:7-27）

| 字段 | 行号 | 类型 | 用途 |
|------|------|------|------|
| `m_dwThinkTick` | :9 | LongWord | 上次 Think() 的 tick |
| `m_boDupMode` | :11 | Boolean | 与其他对象重叠（卡住） |

### 2.4 TMonGenInfo 刷怪点结构（Grobal2.pas:1540-1552）

```pascal
TMonGenInfo = record
  sMapName: String;         // 地图名
  nX, nY: Integer;          // 刷怪中心坐标
  sMonName: String;         // 怪物名称
  nRange: Integer;          // 刷怪区域半径（格）
  nCount: Integer;          // 最大存活数量
  dwZenTime: dword;         // 重生间隔（ms）
  nMissionGenRate: integer; // 任务刷怪概率 %
  CertList: TList;          // 存活实例列表
  Envir: TObject;           // 地图环境引用
  nRace: integer;           // Race ID → 类选择
  dwStartTick: dword;       // 上次刷怪 tick
end;
```

### 2.5 TMonInfo 怪物属性结构（Grobal2.pas:1558）

包含字段：`sName`、`btRace`、`btRaceImg`、`wAppear`、`wLifeSpan`、`wHP`、`wMP`、`wAC`、`wMAC`、`wDC`、`wMaxDC`、`wMC`、`wSC`、`wSpeed`、`wWalkSpeed`、`wWalkStep`、`wWalkWait`、`wAttackSpeed`、`wExp`、`wUndead`、`btCoolEye`、`wHitPoint` 等。由 `MonInitialize`（`UsrEngn.pas:2578-2617`）加载到对象实例。

---

## 三、AI 主循环

### 3.1 引擎调度：ProcessMonsters（UsrEngn.pas:1119-1287）

```
引擎 tick（每帧）
  │
  ├── 刷怪维护（round-robin 轮询 m_MonGenList）
  │     条件：(GetTickCount - dwRegenMonstersTick) > g_Config.dwRegenMonstersTime
  │     对当前刷怪器：
  │       如果 dwStartTick=0 或超过 dwZenTime：
  │         统计存活数 → 不足则 RegenMonsters() 补充
  │         重置 dwStartTick
  │
  └── 怪物 AI 驱动（遍历所有存活怪物）
        ObjBase.pas:1209  if (dwCurrentTick - m_dwRunTick) > m_nRunTime then
        ObjBase.pas:1212    if (dwCurrentTick - m_dwSearchTick) > m_dwSearchTime then
        ObjBase.pas:1216      SearchViewRange();     // 更新视野列表
        ObjBase.pas:1221      Run();                 // 执行 AI
        ObjBase.pas:1237-1242 如果 m_boGhost 且超过 5 分钟 → Free()
```

- **m_nRunTime**：TMonster 默认 250ms（`ObjMon.pas:256`）。
- **m_dwSearchTime**：TMonster 默认 `3000 + Random(2000)` ms（`ObjMon.pas:257`），即 3-5 秒随机。
- **SearchViewRange**（`ObjBase.pas:19812`）：扫描以怪物为中心、边长 `2*m_nViewRange+1` 的正方形区域，将有效对象加入 `m_VisibleActors`。

### 3.2 TBaseObject.Run 基础处理（ObjBase.pas:3672-4281）

所有怪物通过 `inherited` 调用此方法，处理与 AI 无关但影响 AI 的基础逻辑：

| 步骤 | 行号 | 内容 |
|------|------|------|
| 消息处理 | :3696-3701 | 排空消息队列，逐条调用 `Operate()` |
| HP/MP 回复 | :3723-3766 | 按 `g_Config.nHealthFillTime` 定时回复 |
| 死亡检测 | :3753-3764 | HP=0 → 调用 `Die()` |
| 幽灵清理 | :3769-3771 | 死亡超过 `dwMakeGhostTime`（3 分钟）→ `MakeGhost()` |
| 目标过期 | :3884-3895 | 清除 `m_TargetCret`（条件见第四章） |
| LastHiter 过期 | :3898-3906 | 30 秒后清除 |
| ExpHitter 过期 | :3910-3918 | 6 秒后清除 |
| 主人死亡处理 | :3969-3984 | 奴隶叛变（变野怪）或跟随死亡 |
| 状态效果 tick | :4156-4241 | 毒/buff 持续时间递减 |
| 毒伤害 | :4255-4266 | 绿毒每 `dwPosionDecHealthTime`（2500ms）扣血 |

### 3.3 TMonster.Run 核心状态机（ObjMon.pas:415-527）

```pascal
procedure TMonster.Run;
begin
  // 前置守卫：幽灵/死亡/潜地/石化 时跳过全部 AI
  if not m_boGhost and not m_boDeath and not m_boFixedHideMode
     and not m_boStoneMode and (m_wStatusTimeArr[POISON_STONE] = 0) then
  begin
    // ① Think：防卡 + 目标校验
    if Think then begin inherited; Exit; end;

    // ② 步频控制：突发-休息循环
    if m_boWalkWaitLocked then begin
      if (GetTickCount - m_dwWalkWaitTick) > m_dwWalkWait then
        m_boWalkWaitLocked := False;
    end;
    if not m_boWalkWaitLocked and
       (GetTickCount - m_dwWalkTick) > m_nWalkSpeed then
    begin
      m_dwWalkTick := GetTickCount();
      Inc(m_nWalkCount);
      if m_nWalkCount > m_nWalkStep then begin
        m_nWalkCount := 0;
        m_boWalkWaitLocked := True;   // 进入休息
      end;

      // ③ 逃跑模式
      if not m_boRunAwayMode then begin
        // ④ 战斗
        if not m_boNoAttackMode then begin
          if m_TargetCret <> nil then begin
            if AttackTarget then begin inherited; Exit; end;  // 虚方法
          end else begin
            m_nTargetX := -1;
            if m_boMission then begin  // 任务路径点
              m_nTargetX := m_nMissionX;
              m_nTargetY := m_nMissionY;
            end;
          end;
        end;

        // ⑤ 跟随主人（召唤/驯服怪）
        if m_Master <> nil then begin
          if m_TargetCret = nil then begin
            m_Master.GetBackPosition(nX, nY);
            m_nTargetX := nX; m_nTargetY := nY;
          end;
          // 距离 > 20 格或跨图 → 瞬移
          if (m_PEnvir <> m_Master.m_PEnvir) or
             (abs(m_nCurrX - m_Master.m_nCurrX) > 20) then
            SpaceMove(...);
        end;
      end else begin
        // 逃跑超时检查
        if (m_dwRunAwayTime > 0) and
           ((GetTickCount - m_dwRunAwayStart) > m_dwRunAwayTime) then
          m_boRunAwayMode := False;
      end;

      // ⑥ 移动执行
      if m_nTargetX <> -1 then
        GotoTargetXY()          // 向目标位置移动
      else if m_TargetCret = nil then
        Wondering();            // 无目标闲逛
    end;
  end;
  inherited;                    // TBaseObject.Run
end;
```

### 3.4 Think 防卡逻辑（ObjMon.pas:359-381）

```pascal
function TMonster.Think(): Boolean;
begin
  Result := False;
  if (GetTickCount - m_dwThinkTick) > 3000 then begin  // 每 3 秒
    m_dwThinkTick := GetTickCount();
    if m_PEnvir.GetXYObjCount(m_nCurrX, m_nCurrY) >= 2 then
      m_boDupMode := True;                             // 检测到重叠
    if not IsProperTarget(m_TargetCret) then
      m_TargetCret := nil;                             // 清除无效目标
  end;
  if m_boDupMode then begin
    WalkTo(Random(8), False);                          // 随机方向脱困
    if moved then begin m_boDupMode := False; Result := True; end;
  end;
end;
```

---

## 四、目标选择与仇恨系统

### 4.1 SearchViewRange 视野扫描（ObjBase.pas:19812）

以怪物当前位置为中心，扫描 `m_nViewRange` 格半径的正方形区域。对每个格子调用 `GetMapBaseObjects` 获取对象列表，过滤死亡/幽灵对象后加入 `m_VisibleActors`。每次调用前清空旧列表。

### 4.2 SearchTarget 最近目标选取（ObjBase.pas:22667-22692）

遍历 `m_VisibleActors`，用**曼哈顿距离**选取最近的合法目标：

```pascal
for i := 0 to m_VisibleActors.Count - 1 do begin
  BaseObject := m_VisibleActors.Items[i];
  if not BaseObject.m_boDeath then
    if IsProperTarget(BaseObject) and
       (not BaseObject.m_boHideMode or m_boCoolEye) then begin
      nC := abs(m_nCurrX - BaseObject.m_nCurrX)
          + abs(m_nCurrY - BaseObject.m_nCurrY);
      if nC < n10 then begin
        n10 := nC;
        BaseObject18 := BaseObject;
      end;
    end;
end;
if BaseObject18 <> nil then SetTargetCreat(BaseObject18);
```

### 4.3 IsProperTarget / IsAttackTarget 目标合法性（ObjBase.pas:21332-21519）

**野怪**（`m_btRaceServer >= RC_ANIMAL`，无主人）：

| 条件 | 行号 | 结果 |
|------|------|------|
| 目标是玩家 (`RC_PLAYOBJECT`) | :21376 | 攻击 |
| 目标是 NPC (`RC_PEACENPC..RC_ANIMAL`) | :21377-21378 | 攻击 |
| 目标有主人（召唤物） | :21379 | 攻击 |
| `m_boCrazyMode = True` | :21381 | 攻击一切 |
| `m_boNastyMode = True` | :21382 | 攻击所有非 NPC |
| 其他 | — | 不攻击 |

**召唤/驯服怪**（有主人，:21344-21373）：

| 条件 | 行号 | 结果 |
|------|------|------|
| 目标是主人的目标 | :21350 | 攻击 |
| 目标是主人的 LastHiter | :21355 | 攻击 |
| 目标正在攻击主人 | :21360 | 攻击 |
| 目标与主人是同一主人 | :21364 | 不攻击（友军） |
| 目标在安全区 | :21370-21371 | 不攻击 |

### 4.4 Struck 受击转目标（ObjBase.pas:2794-2815）

被攻击时的仇恨响应：

```pascal
if (m_TargetCret = nil) or                    // 无目标
   GetAttackDir(m_TargetCret, btDir) or       // 当前目标相邻（可兼顾）
   (Random(6) = 0) then                       // 1/6 随机概率
  if IsProperTarget(hiter) then
    SetTargetCreat(hiter);                    // 切换到攻击者
```

附加惩罚：`m_dwHitTick := m_dwHitTick + (150 - min(130, Level*4))`——被击中后攻击冷却增加，等级越高惩罚越小。

### 4.5 目标过期条件（ObjBase.pas:3884-3895）

`m_TargetCret` 在以下任一条件满足时被清除：

| 条件 | 说明 |
|------|------|
| `m_dwTargetFocusTick` 超过 30 秒 | 长时间未与目标交互 |
| 目标 `m_boDeath` 或 `m_boGhost` | 目标已死/已销毁 |
| 目标在不同地图 | 跨图不可追 |
| 曼哈顿距离 > 15 格 | 超出追击范围 |

### 4.6 TATMonster 周期性搜索（ObjMon.pas:614-630）

```pascal
if ((GetTickCount - m_dwSearchEnemyTick) > 8000) or
   (((GetTickCount - m_dwSearchEnemyTick) > 1000) and (m_TargetCret = nil)) then
begin
  m_dwSearchEnemyTick := GetTickCount();
  SearchTarget();
end;
```

- 有目标时：每 8 秒重新搜索（可能切换到更近的）。
- 无目标时：每 1 秒搜索一次（快速响应）。

### 4.7 CoolEye 反隐

`m_btCoolEye`（`ObjBase.pas:149`）为 0-100 的概率值。在 `SearchTarget` 中，隐身目标（`m_boHideMode = True`）只有当怪物 `m_boCoolEye = True` 时才能被选取。`m_boCoolEye` 在初始化时由 `m_btCoolEye` 概率掷骰决定。

---

## 五、AI 行为类型（Race 工厂）

### 5.0 Race ID → 类映射表（UsrEngn.pas:1819-1938）

| Race | 类 | 行为概述 |
|------|-----|----------|
| 51 | TMonster + m_boAnimal | 被动鸡（不主动攻击） |
| 52 (1/30) | TChickenDeer | 逃跑鹿 |
| 52 (29/30) | TMonster + m_boAnimal | 被动鹿 |
| 53 | TATMonster + m_boAnimal | 近战狼 |
| 80 | TMonster | 基础游荡（不主动搜索） |
| 81 | TATMonster | 标准近战追击 |
| 82 | TSpitSpider | 远程喷吐（2 格锥形） |
| 83 | TSlowATMonster | 慢速近战 |
| 84 | TScorpion | 近战（动物） |
| 85 | TStickMonster | 固定潜地伏击 |
| 87 | TDualAxeMonster | 远程飞斧（7 格） |
| 90 | TGasAttackMonster | 毒气近战 |
| 91 | TMagCowMonster | 魔法近战 |
| 92 | TCowKingMonster | Boss（牛魔王） |
| 93 | TThornDarkMonster | 远程（飞斧变种） |
| 94 | TLightingZombi | 远程闪电 |
| 95 | TDigOutZombi | 潜地僵尸 |
| 96 | TZilKinZombi | 死亡分裂 |
| 100 | TWhiteSkeleton | 升级骷髅（奴隶） |
| 101 | TScultureMonster | 石化伏击 |
| 102 | TScultureKingMonster | Boss + 召唤 |
| 104 | TArcherMonster | 远程弓箭 |
| 107 | TCentipedeKingMonster | 固定 Boss |
| 113 | TElfMonster | 变形精灵 |
| 114 | TElfWarriorMonster | 潜地战士 |
| 117 | TExplosionSpider | 自爆蜘蛛 |
| 118 | THighRiskSpider | 非毒喷吐 |
| 130 | TDoubleCriticalMonster | 远程双暴击 |
| 131 | TRonObject | 范围攻击 |
| 132 | TSandMobObject | 潜地沙怪 |
| 133 | TMagicMonObject | 魔法攻击 |
| 200 | TElectronicScolpionMon | 闪电 + 吸血 |
| 215 | TFireBallMonster | 远程火球 |

Race 常量定义于 `M2Share.pas:140-171`。

### 5.1 基础游荡（TMonster, Race 80）

最基础的怪物。不主动搜索敌人（无 `SearchTarget` 调用），仅在被攻击后通过 `Struck` 获得目标，然后追击/攻击。无目标时 `Wondering()` 闲逛。

### 5.2 近战追击（TATMonster, Race 81）

在 `TMonster.Run` 基础上增加周期性 `SearchTarget()`（见 4.6 节）。是大多数近战怪物的基类。

**AttackTarget**（`ObjMon.pas:383-413`）：
```pascal
if GetAttackDir(m_TargetCret, btDir) then begin  // 相邻（1 格）？
  if (GetTickCount - m_dwHitTick) > m_nNextHitTime then begin
    m_dwHitTick := GetTickCount();
    Attack(m_TargetCret, btDir);                  // 虚方法 → _Attack
  end;
  Result := True;                                 // 已在攻击范围
end else begin
  SetTargetXY(m_TargetCret.m_nCurrX, ...);       // 追击
end;
```

### 5.3 慢速近战（TSlowATMonster, Race 83）

继承 TATMonster，覆写 `Attack` 方法增加额外攻击延迟。行为与标准近战相同但攻击频率更低。

### 5.4 远程喷吐（TSpitSpider, Race 82）

**AttackTarget**（`ObjMon.pas:719-745`）：使用 `TargetInSpitRange` 替代 `GetAttackDir`。

**TargetInSpitRange**（`ObjBase.pas:18504-18530`）：范围为 **2 格**（非相邻）。使用 `g_Config.SpitMap[btDir, y, x]` 查找表验证 5×5 网格中的合法喷吐位置。

**SpitAttack**（`ObjMon.pas:674-718`）：
- 方向锥形范围伤害。
- 伤害基于 DC（攻击力），检查命中。
- 可选附加绿毒（`POISON_DECHEALTH`）。

### 5.5 远程飞斧（TDualAxeMonster, Race 87）

**AttackTarget**（`ObjAxeMon.pas:65-100`）：
- 范围：**7 格**。
- 连射机制：`m_nAttackMax`（默认 2）次快速连射，之后 1/5 概率重置计数器。
- 需要视线检查：`m_PEnvir.CanFly()`。

**FlyAxeAttack**（`ObjAxeMon.pas:41-64`）：
- 伤害延迟与距离成正比：`max(abs(dx), abs(dy)) * 50 + 600` ms。
- 发送 `RM_SPELL` 动画。

**Run**（`ObjAxeMon.pas:120-160`）：自带 5 秒周期目标搜索（不使用 TATMonster 的）。

### 5.6 逃跑（TChickenDeer, Race 52, 1/30 概率）

**Run**（`ObjMon.pas:542-598`）：反转正常 AI 逻辑：
1. 搜索最近可见敌人。
2. 如果发现：`m_boRunAwayMode := True`，`m_TargetCret` 设为威胁源。
3. 计算逃跑位置：`GetNextPosition(target.X, target.Y, dir_away, 5)`——向远离威胁方向跑 5 格。
4. 如果无敌：`m_boRunAwayMode := False`，正常闲逛。

### 5.7 范围攻击（TRonObject, Race 131）

**Run**（`ObjMon3.pas:241-264`）：目标在 6 格内且攻击冷却已过 → 调用 `AroundAttack`。

**AroundAttack**（`ObjMon3.pas:209-239`）：
- `GetMapBaseObjects(envir, x, y, 1, list)` 获取 1 格半径内所有对象。
- 对列表中**所有**合法目标执行 `_Attack`。

### 5.8 潜地（TStickMonster, Race 85 / TDigOutZombi, Race 95）

**TStickMonster.Run**（`ObjMon2.pas:252-297`）：双状态机：

| 状态 | 条件 | 行为 |
|------|------|------|
| 隐藏 | `m_boFixedHideMode = True` | `CheckComeOut()`：检查 `nComeOutValue` 格内有无合法目标 |
| 出现 | 目标进入范围 | `ComeOut()`：设 `m_boFixedHideMode := False`，发 `RM_DIGUP` |
| 可见 | 正常 | 搜索目标 → 攻击或追击 |
| 回潜 | 目标超出 `nAttackRange` 或无目标 | `ComeDown()`：发 `RM_DIGDOWN`，清空视野，设 `m_boFixedHideMode := True` |

**TDigOutZombi**（`ObjMon.pas:138`）：类似但为移动型潜地，可在地下移动到目标附近再出现。

### 5.9 自爆（TExplosionSpider, Race 117）

**AttackTarget**（`ObjMon2.pas:774-802`）：相邻时调用 `sub_4A65C4()`。

**sub_4A65C4**（`ObjMon2.pas:745-773`）：
- 设 `m_WAbil.HP := 0`（自杀）。
- 对 1 格内所有合法目标造成伤害：50% 物理（`GetHitStruckDamage`）+ 50% 魔法（`GetMagStruckDamage`）。

**Run**（`ObjMon2.pas:804-814`）：额外 60 秒自毁计时器——存活超过 60 秒无论是否有目标都会自爆。

### 5.10 闪电吸血（TElectronicScolpionMon, Race 200）

**Run**（`ObjMon.pas:1842-1881`）：
- HP < 50% 时切换魔法模式（`m_boUseMagic`）。
- 目标在 2 格内且（魔法模式或距离恰好 2 格）→ 发射 `LightingAttack`。

**LightingAttack**（`ObjMon.pas:1820-1840`）：
- 使用 MC（魔法力）计算伤害。
- **吸血**：`Inc(m_WAbil.HP, nDamage div btGetBackHP)`。

### 5.11 毒气近战（TGasAttackMonster, Race 90）

继承 TATMonster，覆写 `Attack`：近战命中时附加绿毒效果（`POISON_DECHEALTH`）。

子类：
- **TGasMothMonster**（`ObjMon.pas:193`）：毒蛾。
- **TGasDungMonster**（`ObjMon.pas:201`）：毒粪怪。

### 5.12 石化伏击（TScultureMonster, Race 101）

初始 `m_boStoneMode = True`（石化不可见）。玩家靠近时解除石化并攻击。石化状态下完全跳过 AI（`TMonster.Run` 前置守卫）。

### 5.13 召唤类

| 类 | 行号 | 行为 |
|-----|------|------|
| TBeeQueen | ObjMon2.pas:24 | 蜂后：周期召唤蜜蜂 |
| TSpiderHouseMonster | ObjMon2.pas:56 | 蜘蛛巢：召唤蜘蛛 |
| TScultureKingMonster | ObjMon.pas:180 | 雕像王 Boss：召唤石像 |
| TBoneKingMonster | ObjMon3.pas:54 | 骨王：召唤骷髅 |

召唤逻辑：检查当前存活 minion 数量 < 上限 → 在自身周围找空位 → `AddBaseObject` 创建新怪物 → 设 `m_Master := Self`。

### 5.14 死亡分裂（TZilKinZombi, Race 96）

**Die**（`ObjMon.pas:147-158`）：死亡时生成额外僵尸。通过 `nZilKillCount` 控制分裂数量。

### 5.15 火焰光环（TFireMonster）

**Run**（`ObjMon3.pas:1064-1143`）：
- 创建 `TFireBurnEvent` 地图事件，十字形分布（中心 + 四个方向各 2 格 = 9 格）。
- 事件持续 20 秒，每 tick 对范围内对象造成 10 点伤害。
- 是**持续区域封锁**，非定向攻击。

### 5.16 火球术（TFireBallMonster, Race 215）

**Run**（`ObjMon3.pas:984-1046`）：
- 使用 MC 计算魔法伤害。
- 范围：8 格。
- 视线检查：`MagCanHitTarget`。
- 发送 `RM_SPELL` + `RM_MAGICFIRE` 特效。

### 5.17 变形（TElfMonster, Race 113）

**ObjMon.pas:207**：可在两种形态间切换，切换时改变外观（`m_wAppearance`）和属性。

### 5.18 守卫（TGuardUnit / TArcherGuard）

**ObjMon2.pas:78-121**：
- TGuardUnit：城堡守卫基类。
- TArcherGuard（:87）：远程弓箭守卫。
- TArcherPolice（:96）：警察弓箭手。
- TCastleDoor（:102）：城门（不可移动）。
- TWallStructure（:121）：城墙结构。

守卫只攻击敌对阵营（攻沙时对方行会成员）。

### 5.19 远程双暴击（TDoubleCriticalMonster, Race 130）

**ObjMon.pas:233**：远程攻击，有概率触发双倍伤害。

### 5.20 升级骷髅（TWhiteSkeleton, Race 100）

**ObjMon.pas:159**：道士召唤的骷髅战士。通过战斗获得经验升级，升级时属性增长。有 `m_Master` 跟随逻辑。

---

## 六、移动系统

### 6.1 WalkTo 单步移动（ObjBase.pas:1995-2074）

```pascal
function TBaseObject.WalkTo(nDir: Integer; boRun: Boolean): Boolean;
begin
  // 1. 计算目标格 (nX, nY) = 当前位置 + 方向偏移
  GetFrontPosition(nDir, nX, nY);
  // 2. 边界检查
  if (nX < 0) or (nX > m_PEnvir.m_nWidth-1) ... then Exit;
  // 3. 危险地形检查（lava 等）
  if bo2BA then CanSafeWalk(nX, nY);
  // 4. 不走到主人正前方
  if m_Master <> nil then ...
  // 5. 碰撞检测
  m_PEnvir.MoveToMovingObject(m_nCurrX, m_nCurrY, nX, nY, Self);
  // 6. 成功 → 广播 RM_WALK
  SendRefMsg(RM_WALK, ...);
end;
```

方向常量：`DR_UP=0, DR_UPRIGHT=1, DR_RIGHT=2, DR_DOWNRIGHT=3, DR_DOWN=4, DR_DOWNLEFT=5, DR_LEFT=6, DR_UPLEFT=7`。

### 6.2 GotoTargetXY 追踪（ObjBase.pas:2709-2759）

**非 A\* 的贪心追踪**：
1. 计算朝 `(m_nTargetX, m_nTargetY)` 的方向（sign 比较）。
2. 调用 `WalkTo(nDir)`。
3. 如果被阻挡（位置未变），尝试最多 **8 个备选方向**（螺旋搜索）：
```pascal
n20 := Random(3);
for i := DR_UP to DR_UPLEFT do begin
  if not moved then begin
    if n20 <> 0 then Inc(nDir) else Dec(nDir);  // 顺/逆时针绕行
    WalkTo(nDir, False);
  end;
end;
```

这是简单的**贴墙绕行**，不具备真正路径规划能力。复杂地形中怪物可能永久卡住。

### 6.3 Wondering 闲逛（ObjBase.pas:22723-22728）

```pascal
if (Random(20) = 0) then           // 5% 概率/tick
  if (Random(4) = 1) then          // 其中 25% → 转向
    TurnTo(Random(8))
  else
    WalkTo(m_btDirection, False);  // 75% → 向前走
```

有效概率：每 tick ~1.25% 概率移动，~0.42% 概率转向。

### 6.4 步频控制（ObjMon.pas:430-446）

突发-休息循环：
- 每 `m_nWalkSpeed` ms 允许一步（来自数据库 `wWalkSpeed`）。
- 连续走 `m_nWalkStep` 步后，休息 `m_dwWalkWait` ms。
- 由 `m_boWalkWaitLocked` 控制休息状态。

### 6.5 主人跟随与瞬移（ObjMon.pas:468-497）

召唤/驯服怪物的跟随逻辑：
- 目标位置 = `m_Master.GetBackPosition()`（主人背后一格）。
- 距离 > 20 格或不同地图 → `SpaceMove`（瞬移到主人身边）。
- 主人 `m_boSlaveRelax = True` 时不瞬移（休息模式）。

### 6.6 逃跑移动（TChickenDeer, ObjMon.pas:542-598）

- 计算远离方向：从威胁到自身的方向。
- `GetNextPosition(target.X, target.Y, dir_away, 5)`：沿远离方向跑 5 格。
- 设置 `m_nTargetX/Y` 后由 `GotoTargetXY` 执行移动。

---

## 七、战斗系统

### 7.1 物理伤害公式（ObjBase.pas:21967-22000）

```pascal
// 攻击方：DC 随机
nDamage := Random(HiWord(WAbil.DC) - LoWord(WAbil.DC) + 1) + LoWord(WAbil.DC);

// 命中判定（ObjBase.pas:21978）
if Random(BaseObject.m_btSpeedPoint) < m_btHitPoint then  // 命中
else  // 闪避（0 伤害）

// 防御方：AC 减伤（ObjBase.pas:22414-22439 GetHitStruckDamage）
nArmor := LoWord(m_WAbil.AC) + Random(HiWord(m_WAbil.AC) - LoWord(m_WAbil.AC) + 1);
nDamage := MAX(0, nDamage - nArmor);

// 不死系加成
if (m_btLifeAttrib = LA_UNDEAD) then
  Inc(nDamage, Target.m_AddAbil.btUndead);
```

### 7.2 魔法伤害公式（ObjBase.pas:22441-22458）

```pascal
// GetMagStruckDamage
n14 := LoWord(m_WAbil.MAC) + Random(HiWord(m_WAbil.MAC) - LoWord(m_WAbil.MAC) + 1);
nDamage := MAX(0, nDamage - n14);
```

### 7.3 攻击冷却

所有攻击检查：`if Integer(GetTickCount - m_dwHitTick) > m_nNextHitTime then`。
`m_nNextHitTime` 来自数据库 `wAttackSpeed`。

### 7.4 受击反馈（ObjBase.pas:2794-2815）

- 更新 `m_dwStruckTick`。
- 可能切换目标到攻击者（见 4.4 节）。
- 动物被击中降低肉品质。
- 攻击延迟惩罚：`m_dwHitTick += 150 - min(130, Level*4)`。

### 7.5 安全区保护

在安全区内，怪物不能攻击玩家，玩家也不能攻击其他玩家（PvP 保护）。由 `IsProperTarget` 中的安全区检查实现。

---

## 八、刷怪系统（Mongen）

### 8.1 引擎调度（UsrEngn.pas:1140-1177）

```pascal
if ((GetTickCount - dwRegenMonstersTick) > g_Config.dwRegenMonstersTime) then begin
  dwRegenMonstersTick := GetTickCount();
  MonGen := m_MonGenList.Items[m_nCurrMonGen];  // round-robin
  // 推进到下一个刷怪器
  Inc(m_nCurrMonGen);
  if m_nCurrMonGen >= m_MonGenList.Count then m_nCurrMonGen := 0;

  if (MonGen.dwStartTick = 0) or
     ((GetTickCount - MonGen.dwStartTick) > GetZenTime(MonGen.dwZenTime)) then
  begin
    nGenCount := GetGenMonCount(MonGen);
    nGenModCount := MAX(1, Round(MAX(1, MonGen.nCount) / (g_Config.nMonGenRate / 10)));
    if nGenModCount > nGenCount then
      RegenMonsters(MonGen, nGenModCount - nGenCount);
    MonGen.dwStartTick := GetTickCount();
  end;
end;
```

- 每 tick 只处理**一个**刷怪器（round-robin）。
- 尊重 `dwZenTime` 重生间隔。
- 按 `nMonGenRate` 配置分批刷怪。

### 8.2 RegenMonsters 随机位置生成（UsrEngn.pas:2006-2055）

```pascal
nX := (MonGen.nX - MonGen.nRange) + Random(MonGen.nRange * 2 + 1);
nY := (MonGen.nY - MonGen.nRange) + Random(MonGen.nRange * 2 + 1);
Cert := AddBaseObject(MonGen.sMapName, nX, nY, MonGen.nRace, MonGen.sMonName);
```

- 在 `[center-range, center+range]` 正方形内随机取点。
- 有 `g_dwZenLimit` 时间预算防止单 tick 刷怪过多导致卡顿。

### 8.3 AddBaseObject 工厂（UsrEngn.pas:1819-2001）

1. `case nMonRace of` 选择类（见 5.0 节映射表）。
2. 调用 `MonInitialize` 加载属性。
3. 放置到地图；如果出生点被占，最多尝试 **31 次**随机偏移找空位。

### 8.4 MonInitialize 属性加载（UsrEngn.pas:2578-2617）

```pascal
BaseObject.m_btRaceServer := Monster.btRace;
BaseObject.m_Abil.Level   := Monster.wLevel;
BaseObject.m_dwFightExp   := Monster.dwExp;
BaseObject.m_Abil.HP      := Monster.wHP;
BaseObject.m_Abil.AC      := MakeLong(Monster.wAC, Monster.wAC);
BaseObject.m_Abil.DC      := MakeLong(Monster.wDC, Monster.wMaxDC);
BaseObject.m_btSpeedPoint := Monster.wSpeed;
BaseObject.m_btHitPoint   := Monster.wHitPoint;
BaseObject.m_nWalkSpeed   := Monster.wWalkSpeed;
BaseObject.m_nWalkStep    := Monster.wWalkStep;
BaseObject.m_dwWalkWait   := Monster.wWalkWait;
BaseObject.m_nNextHitTime := Monster.wAttackSpeed;
```

---

## 九、死亡与掉落

### 9.1 Die 流程（ObjBase.pas:20765-21075）

```
Die()
  │
  ├── ① 设死亡状态：m_boDeath := True; m_dwDeathTick := GetTickCount()
  │
  ├── ② 经验分配（:20798-20869）
  │     优先级：m_ExpHitter > m_LastHiter
  │     如果 ExpHitter 是召唤物 → 经验也给其主人
  │     触发 @OnKillMob NPC 脚本
  │
  ├── ③ PK 处理（:20885-20977）
  │     玩家杀玩家 → PK 点数、行会战争检查
  │
  ├── ④ 物品掉落（:20982-21016）
  │     DropUseItems()     掉落装备栏物品
  │     ScatterBagItems()  掉落背包物品（3 格半径散布）
  │     ScatterGolds()     掉落金币（每堆 nMonOneDropGoldCount，最多 17 堆）
  │     g_MonDropLimitLIst 掉落上限检查
  │
  └── ⑤ 广播死亡：SendRefMsg(RM_DEATH, ...)
```

### 9.2 经验分配规则

| 条件 | 经验归属 |
|------|----------|
| `m_ExpHitter` 存在 | 给 ExpHitter（最后造成显著伤害者） |
| ExpHitter 是召唤物 | 经验同时给其主人 |
| 无 ExpHitter | 给 `m_LastHiter` |
| 都没有 | 无经验 |

### 9.3 MakeGhost 清理（ObjBase.pas:20510-20515）

```pascal
m_boGhost := True;
m_dwGhostTick := GetTickCount();
DisappearA();
```

- 死亡后 3 分钟（`dwMakeGhostTime`）→ 变为幽灵。
- 幽灵状态再经过 5 分钟 → `ProcessMonsters` 中 `Free()`（`UsrEngn.pas:1237-1242`）。

### 9.4 特殊死亡

| 怪物 | 行为 |
|------|------|
| TZilKinZombi | 死亡时分裂生成额外僵尸 |
| TExplosionSpider | 自爆即死亡，无正常 Die 流程 |
| TWhiteSkeleton | 死亡时通知主人（道士） |

---

## 十、状态效果与特殊机制

### 10.1 毒效果

| 类型 | 常量 | 效果 |
|------|------|------|
| 绿毒 | `POISON_DECHEALTH` | 每 2500ms（`dwPosionDecHealthTime`）扣固定 HP |
| 红毒 | `POISON_DAMAGEARMOR` | 降低防御力 |
| 定身 | `POISON_DONTMOVE` | 禁止移动 |
| 石化 | `POISON_STONE` | 完全冻结（跳过 Run） |
| 封魔 | `POISON_LOCKSPELL` | 禁止施法 |

毒处理在 `TBaseObject.Run` 中（:4156-4266），每 tick 递减持续时间，到期清除。

### 10.2 隐身与反隐

- `m_boHideMode`：隐身状态（道士隐身术）。
- `m_boCoolEye`：怪物能看穿隐身。
- 在 `SearchTarget` 中：`not BaseObject.m_boHideMode or m_boCoolEye`。

### 10.3 疯狂模式与厌恶模式

- `m_boCrazyMode`（`ObjBase.pas:190`）：攻击所有对象（包括其他怪物）。
- `m_boNastyMode`（`ObjBase.pas:368`）：攻击所有非 NPC 对象。
- 两者都通过 `IsAttackTarget` 中的条件分支生效。

### 10.4 主人-奴隶关系

| 字段 | 用途 |
|------|------|
| `m_Master` | 主人指针 |
| `m_boSlaveRelax` | 休息模式（不瞬移跟随） |
| `m_SlaveList` | 主人的奴隶列表 |

- 召唤/驯服怪物通过 `m_Master` 关联主人。
- 主人死亡时（`ObjBase.pas:3969-3984`）：奴隶叛变（清除 Master，变为野怪）或跟随死亡。
- 奴隶攻击目标受主人约束（见 4.3 节）。

### 10.5 固定隐藏与潜地

- `m_boFixedHideMode`：完全从 AI 循环中跳过（`TMonster.Run` 前置守卫）。
- 用于 TStickMonster 的潜伏状态。
- 通过 `ComeOut()`/`ComeDown()` 切换，伴随 `RM_DIGUP`/`RM_DIGDOWN` 动画。

---

## 十一、关键常量与配置

### 11.1 Race 阵营常量（Grobal2.pas:1100-1106）

| 常量 | 值 | 含义 |
|------|-----|------|
| `RC_PLAYOBJECT` | 0 | 玩家 |
| `RC_NPC` | 10 | NPC |
| `RC_GUARD` | 11 | 守卫 |
| `RC_PEACENPC` | 15 | 和平 NPC |
| `RC_ANIMAL` | 50 | 动物 |
| `RC_MONSTER` | 80 | 怪物 |
| `RC_ARCHERGUARD` | 112 | 弓箭守卫 |

### 11.2 方向常量

| 常量 | 值 | 方向 |
|------|-----|------|
| `DR_UP` | 0 | 上 |
| `DR_UPRIGHT` | 1 | 右上 |
| `DR_RIGHT` | 2 | 右 |
| `DR_DOWNRIGHT` | 3 | 右下 |
| `DR_DOWN` | 4 | 下 |
| `DR_DOWNLEFT` | 5 | 左下 |
| `DR_LEFT` | 6 | 左 |
| `DR_UPLEFT` | 7 | 左上 |

### 11.3 时间常量

| 参数 | 默认值 | 来源 | 用途 |
|------|--------|------|------|
| `m_nRunTime` | 250ms | ObjMon.pas:256 | AI tick 间隔 |
| `m_dwSearchTime` | 3000-5000ms | ObjMon.pas:257 | 视野刷新间隔 |
| 目标过期 | 30s | ObjBase.pas:3884 | FocusTick 超时 |
| 追击距离上限 | 15 格 | ObjBase.pas:3895 | 超出则丢失目标 |
| LastHiter 过期 | 30s | ObjBase.pas:3898 | 仇恨衰减 |
| ExpHitter 过期 | 6s | ObjBase.pas:3910 | 经验归属窗口 |
| 死亡→幽灵 | 3 min | ObjBase.pas:3769 | dwMakeGhostTime |
| 幽灵→销毁 | 5 min | UsrEngn.pas:1237 | Free 延迟 |
| 绿毒 tick | 2500ms | ObjBase.pas:4255 | dwPosionDecHealthTime |
| Think 间隔 | 3000ms | ObjMon.pas:360 | 防卡检测 |
| 搜索间隔（有目标） | 8000ms | ObjMon.pas:614 | TATMonster |
| 搜索间隔（无目标） | 1000ms | ObjMon.pas:615 | TATMonster |
| 自爆计时 | 60s | ObjMon2.pas:804 | TExplosionSpider |
| 瞬移距离 | 20 格 | ObjMon.pas:494 | 主人跟随 |

### 11.4 g_Config 相关配置项

| 配置项 | 用途 |
|--------|------|
| `dwRegenMonstersTime` | 刷怪 tick 间隔 |
| `nMonGenRate` | 刷怪批次比率 |
| `dwZenLimit` | 单 tick 刷怪时间预算 |
| `nHealthFillTime` | HP 回复间隔 |
| `dwPosionDecHealthTime` | 绿毒伤害间隔 |
| `SpitMap[8,5,5]` | 喷吐范围查找表 |
| `nMonOneDropGoldCount` | 金币每堆数量 |

---

## 十二、AI 决策流程图

```
ProcessMonsters (引擎 tick)
  │
  ├── 每 m_dwSearchTime: SearchViewRange() → 填充 m_VisibleActors
  │
  └── 每 m_nRunTime: Monster.Run()
       │
       ├── TBaseObject.Run: 消息处理 / HP 回复 / 目标过期 / 状态效果
       │
       └── TMonster.Run (子类覆写):
            │
            ├── [守卫] 幽灵/死亡/潜地/石化 → 跳过
            │
            ├── Think(): 防卡(3s) + 目标校验
            │
            ├── 步频控制: m_nWalkSpeed / m_nWalkStep / m_dwWalkWait
            │
            ├── IF m_boRunAwayMode:
            │    └── 检查逃跑超时 → 解除
            │
            ├── ELSE IF m_TargetCret != nil:
            │    └── AttackTarget() [虚方法]
            │         ├── 近战: GetAttackDir(相邻) → Attack()
            │         ├── 喷吐: TargetInSpitRange(2格) → SpitAttack()
            │         ├── 飞斧: dist≤7 + CanFly → FlyAxeAttack()
            │         ├── 自爆: 相邻 → sub_4A65C4() (自杀+范围伤害)
            │         ├── 范围: dist<6 → AroundAttack() (1格半径全打)
            │         ├── 闪电: dist≤2 → LightingAttack() (吸血)
            │         ├── 火球: dist≤8 + MagCanHit → 魔法伤害
            │         └── 不在范围: SetTargetXY → 追击
            │
            ├── ELSE IF m_Master != nil:
            │    ├── 跟随主人背后位置
            │    └── dist>20 或跨图 → SpaceMove 瞬移
            │
            ├── ELSE IF m_nTargetX != -1:
            │    └── GotoTargetXY() → 贪心移动 + 绕行
            │
            └── ELSE:
                 └── Wondering() → 1.25% 走 / 0.42% 转向
```

---

## 附录：源文件索引

| 文件 | 行数 | 内容 |
|------|------|------|
| `M2Server/ObjBase.pas` | 25,370 | TBaseObject / TAnimalObject 基类 |
| `M2Server/ObjMon.pas` | 1,692 | TMonster + 第一批特化子类 |
| `M2Server/ObjMon2.pas` | 1,092 | 第二批特化（潜地/召唤/自爆/守卫） |
| `M2Server/ObjMon3.pas` | 1,535 | 第三批特化（范围/魔法/火焰/克隆） |
| `M2Server/ObjAxeMon.pas` | 186 | 远程飞斧 |
| `M2Server/UsrEngn.pas` | ~3,000 | 引擎调度 / 工厂 / 属性加载 |
| `M2Server/M2Share.pas` | 大 | Race 常量 / 全局配置 |
| `Common/Grobal2.pas` | 2,739 | 消息定义 / 数据结构 / 阵营常量 |
