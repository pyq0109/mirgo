# Go 版完善路线图（以 Delphi 版为权威基线）

> 调查基准：Delphi 原版源码 `asset/delphi/`（lzxsz/MIR2 commit 98711da，客户端以 `Client/` 为权威，与 `MirClient/` 的差异处单独标注）；Go 实现 `cmd/` + `internal/`。
> 调查方式：3 轮全量代码对比（客户端 / 服务端 / 协议与架构），所有结论附 file:line 证据，关键 P0 项经二次人工核验。
> 约定：物品线格式、角色存档、单进程架构等**刻意不兼容项**按设计决策处理（见第五节），不列入待办。

---

## 修复进度（2026-08-05，物品系统第二轮审查 + 批次 1-7 全部完成）

> 本文档为活文档：批次 1-7 按"玩法优先"顺序实施完毕，每项附落点。
> 剩余长尾（特化怪物渲染类、UI 邮件/设置/二级密码、客户端反作弊、GME 副本、
> 攻城器械、付费体系持久化等）见文末 backlog，按需立项。

**已完成 ✅**

| 项 | 内容 | 落点 |
|----|------|------|
| 物品二轮-Bug | 交易改金币失败清零（回包携带当前值）+ OK u16 截断（Param/Tag 32 位拆分）；使用/丢弃失败补发全量背包防物品视觉丢失；仓库 BtValue 存取补齐（数据丢失 bug） | tradesystem.go / main.go(client) / itemsystem.go / attackmode.go / main.go |
| 物品二轮-高 | tooltip 全 StdMode 分支（含 (*) 前缀/需求红字/银行家舍入）；StdMode 31 解包（UnbindList）+@StdModeFunc；Reserved &2/&4 禁脱 + &8 死亡销毁 + &10 保护；TAKECHECKITEM/PARAM1-4；Need 全 22 分支 | uitooltip.go / itemsystem.go / itemdb.go / npcscript.go |
| 物品二轮-中 | 地面金币合并≤2000 + 每格≤5 件 + GetGoldShape；火把/蜡烛耐久 tick+655；SMAbility 扩展抗性/恢复五属性；交易校验（相邻/金币上限/1000ms 反连点）；仓库距离/负重/密码流程；动态价格对齐 Delphi（首次入表×1.1 不再涨）；死亡掉落对齐（PvP 不掉/槽 0..8/红名全掉背包）；CheckItemsNeed 自动脱下；丢弃节流/安全区/廉价管控/禁丢全金 | envir.go / playobject.go / tradesystem.go / storagesystem.go / npcobject.go / guildsystem.go / castle.go |
| 物品二轮-低 | 肉落地扣 2000 耐久 + 矿石随机外观；割肉 MeatQuality 联动；贩卖限制（Dura=0 不进货架）；饥饿度字段（食物 +=DuraMax/10，708 下发）；特效码 139/140/143/144/145/170/171/172；地面物品像素命中 + 同格聚合 + 随机闪光相位 + 自动捡物（Ctrl+P）；客户端失败文案族；获得金币提示；SMButch 坐下动画 | drops.go / butchsystem.go / merchantsystem.go / itemsystem.go / sceneplay.go / main.go(client) |

| 项 | 内容 | 落点 |
|----|------|------|
| P0-1 | 7 个 CM 路由补齐（屠宰/私聊/好友×3/行会宣战/挖矿） | cmd/server/main.go handleGameMessage |
| P0-2 | Race 134/214 AI 映射（AIBoneKing/AIFireAura） | monsterobject.go getAIBehavior |
| P0-3 | 挖矿链路：HandleHit 鹤嘴锄分支 + tryMineAt（Delphi PileStones 语义）+ `#=DIG!` | playobject.go / mining.go |
| P0-4 | MapFlag 全量解析并消费：SAFE（全图安全区）/DARK/EXPRATE/NODRUG/NORECALL/MINE（矿点仅在矿区生成）等 | mapmanager.go InitMapFlags / playobject.go / itemsystem.go |
| P0-5 | 登录公告加载 notice.jsonc（缺省回退） | main.go loadNoticeText |
| P0-6 | 昼夜按地图 DARK 标志（Delphi DayBright 语义） | playobject.go dayBright |
| P1-魔法 | 15 神圣战甲术、47 火龙气焰（含 DB 条目）、49 净化术、50 无极真气（DB ID，Delphi 36 语义）；清理死代码 case 36/战士37/34 分支；符咒表修正 | magicsystem.go / magic_db.jsonc |
| P1-怪物 | Race 206 拖拽（TKhazard）/208 绿毒触碰/209 红毒触碰/210 隐身伏击（含 RM_CHARSTATUSCHANGED→SMCharStatusChanged 广播）；训练师重写为 Race 55 无敌沙袋+伤害统计 | monsterai.go / monsterobject.go / trainernpc.go / mongen.go |
| P1-持久化 | 发型入库（建角不再丢弃）+ 迁移；方向/加点/声望/转生/攻击模式/组队开关入 meta；仓库 39→50 格 | sqlite.go / main.go / config.go |
| P1-客户端 | 公告模态确认（Delphi DMessageDlg 语义）、登出/退出守卫（死亡才可退+确认框，忠实 GEEM2 变体行为）、SM 补齐：640 魔法训练度/655 油灯耐久/685 交易改金失败/707 仓库取回满包/761 行会职务失败/764-765 捐献 | scenenotice.go / sceneplay.go / uidialog.go / main.go |
| 批次1-怪物 | Race 20 弓箭警察（复用 AIGuard，TArcherPolice 为 TArcherGuard 子类）；Race 120 足球（AISoccerBall：被踢沿击打者朝向滚动、动量累加上限 20 格、受阻按 Delphi 反弹表反向、m_boSuperMan 无敌） | monsterobject.go / monsterai.go / playobject.go applyDamage |
| 批次1-刷怪 | GetZenTime 在线人数加速（<30min 间隔、r=(在线−UserFull)/ZenFastStep≤6、每档 −10%）；config 新增 userFull(1000)/zenFastStep(300)；GetPlayerCount 接入 | mongen.go getZenTime / config.go |
| 批次1-CM | CM_QUERYUSERNAME(80) 3×3 近距校验→SM_USERNAME/SM_GHOST；CM_USERGETDETAILITEM(1015) 交易中禁止+同图距离≤15→SM_SENDDETAILGOODSLIST（≤10 实例含价格）；CM_SITDOWN(3012) 死亡/石化 FAIL+TurnInterval 限速+RM_POWERHIT；CM_SOFTCLOSE(1009) 复用 LogoutPlayer 回选角（保留连接，Delphi 语义）。CM_QUERYUSERSET(3040) 经查为 Delphi MD5 后门，CM_THROW(3005) Delphi 无处理器 → backlog | main.go / playobject.go / merchantsystem.go / message.go |
| 批次1-行会 | 行会周期 Run（10s，Delphi GuildManager.Run）：到期行会战移除+双方在线成员"战争结束"通知+变更落盘；guilds 表新增 wars/allies 列（ALTER 迁移）；启动加载丢弃过期战争 | guildsystem.go / sqlite.go / main.go |
| 批次1-速度 | ActionSpeed 四组合：RunLongHit/RunHit/WalkHit/RunMagic（默认 800/800/800/900）；checkActionTransition 复刻 Delphi CheckActionStatus（同动作不限、方向变化按组合取间隔）；HandleHit/HandleSpellFull 接入，Walk/Run/HorseRun/Turn 记录动作；ActionInterval 默认对齐 Delphi 400 | config.go / playobject.go / magicsystem.go |
| 批次1-持久化 | statusTimeArr[12] 入 charMeta：下线保存毒/隐身等剩余时间，上线恢复（含 Hidden 标志）并 BroadcastStatus | main.go / statuseffect.go |
| 批次2-城堡战 | WarDate 预约战：宣战登记日期（缺省明天，@declarewar [YYYY-MM-DD]）、到期日 WarStartHour 点自动开战、多预约队列、战后残留预约回 Declared；IsAttackAllyGuild/IsDefenseAllyGuild 联盟判定并接入攻城 PK（attackmode.go）；城堡状态持久化（castle 表 declarations 列+迁移+启动恢复） | castle.go / castlenpc.go / attackmode.go / sqlite.go |
| 批次2-引擎 | 在线人数周期系统广播（Delphi boSendOnlineCount 语义：5 分钟、%c 模板、rate/10 系数、可配置关闭）；SM_AREASTATE(766) 发送侧（登录+切图，FIGHT=1/SAFE=2/攻城自由PK=4） | main.go / config.go / playobject.go |
| 批次3-SM | 528 超速踢线前发 SM_OUTOFCONNECTION（Delphi KickFlag 路径）；652 商店明细列表（客户端解析+商店面板明细模式渲染/点击购买）；708 登录发送+客户端存储（饥饿系统未实装恒 0）；1103 协议常量+治疗系发送侧+客户端 2s 瞬时头顶血条；1104 客户端处理+WaitForRecogId 等待机制（服务端变身现走 FEATURECHANGED）；806/807 客户端合并处理（SHOW2 解析 body 换外观） | main.go(两端) / playobject.go / magicsystem.go / uinpc.go / sceneplay.go |
| 批次3-渲染 | 魔法特效数据化：服务端 magicEffType 扩展为 46 条 magID→effType/effNum 映射（对照 magic_db effectType=Delphi btEffectType），客户端 14 类特效参数查表（帧基址/帧数/音效）+爆炸特例 12 条；武器特效接线（Delphi DrawWeaponGlimmer 为空实现，真实触发为 SM_BREAKWEAPON→WeaponEffect 5帧×120ms）；小地图第三色（race 50/45/12→调色板 218）；Ctrl+F1-F8 第二套魔法键（键符 E-L，运行时+键位对话框第二排按钮） | magicsystem.go / magiceffect.go / actorbase.go / minimap.go / sceneplay.go / uistate.go |
| 批次4-协议覆盖 | 三个覆盖测试（go/ast 解析 message.go 常量 × 代码引用交叉断言 × 豁免表必须附原因）：①CM 路由覆盖（防 P0-1 复发，豁免 CMThrow）②SM 客户端处理覆盖（豁免 6 项：Spell2/CertificationSuccess/IDNotFound/Reconnect/TimeCheckMsg/ItemUpdate）③AI race 映射双向覆盖（Delphi 工厂 51 race 期望表+全 256 race 反向扫描，防 P0-2 复发）。决策：用"覆盖测试+豁免表"替代中心注册表重构，同等拦截能力、更低重构风险 | protocol_coverage_test.go（两端） |
| 批次5-NPC脚本 | 混合表驱动（存量 ~180 命令保留 switch 零语义风险，新命令走注册表 npcscript_ext.go init 登记）；未知条件/动作 default 分支告警日志（跑图实测驱动长尾收集）；新增 12 条命令：条件 CHECKITEMTYPE（Delphi ObjNpc.pas:5142）/CHECKHORSE/CHECKCASTLEWAR/CHECKMONAREA，动作 HAIRSTYLE（ObjNpc.pas:3010）/HAIRCOLOR/HORSECALL/KILLHORSE（Delphi 空实现，Go 闭环实装并注明）/INCFAME/DECFAME（CreditPoint 承载）/MAKEHEALZONE/MAKEDAMAGEZONE（新事件类型 ETHealZone/ETDamageZone，NPC 为中心 3×3）；6 个命令级单测 | npcscript.go / npcscript_ext.go / mapevent.go / types.go |
| 批次6-网关中间件 | 五层补偿（6.3 方案落地）：①封包序号校验（FrameScanner.OnCode 钩子+会话违规计数，重复 >10 断开、缓冲积压 >20000 记满，RunGate/Main.pas:363-413 语义）②每连接令牌桶限流（默认 60 条/秒突发 40，msgRateLimit/msgBurst 可配，超限计入违规累计）③敏感词过滤（serverconfig/WordFilter.txt 每行一词，聊天/私聊大小写不敏感替换等字符 '*'，RunGate FilterSayMsg 语义）④发送背压（客户端 '*' 回显重置计数，2048B 未确认暂停发送 3s，RunGate/Main.pas:501-553；SendChan 连续丢弃 32 次视为无响应断开）⑤黑名单（BlockIPList.txt 连接期 IP 拒绝支持前缀匹配、DenyAccountList.txt 登录期账号拒绝；转换器补 DenyIPAddrList/DenyAccountList/WordFilter 转换；机器码黑名单不做——客户端无机器码）；5 个中间件单测 | frame.go / server.go / wordfilter.go / blocklist.go / convert_misc.go |
| 批次7-工程化 | /status GM 命令（在线/会话/怪物/NPC/行会/堆内存/GC/goroutine/怪物按地图分布，6.7 可观测性）；WIL 纹理缓存 LRU（8192 上限，超限淘汰最久未访问纹理释放显存，6.5）；WAL 已启用（sqlite.go 连接串 _journal_mode=WAL，6.6 已满足）；迁移保持状态自检式（PRAGMA table_info 幂等自愈，替代版本号方案）；go vet 全量干净。CI 按用户要求不做（保持 AGENTS.md"无 CI"现状，验证命令：go build ./... + go vet ./... + go test ./... -count=1） | gmcommands.go / resourcemanager.go |

**有意不做（本轮）⚠️**

- **解毒术（Delphi 40）**：GEEM2 DB 中 magId 40 = 双龙斩（战士被动切换技），byID 单键索引无法共存；双龙斩是现有可玩内容，不为 DB 未收录的解毒术破坏它。若需实装，需先决策 ID 体系迁移。
- **冰焰/MabMabe（Delphi 50）**：GEEM2 DB 无此魔法条目，原 case 36 死代码已删除。
- 客户端剩余长尾 SM（SM_RECONNECT 换服/SM_TIMECHECK 反作弊/SM_INSTANCEHEALGUAGE/SM_CHANGEFACE 变身）、特化怪物渲染类 44 个、UI 邮件/设置/二级密码：列入后续迭代（见第三、四节清单）。

---

## 一、总览

| 维度 | 覆盖度 | 结论 |
|------|--------|------|
| 核心线协议（6Bit/帧/TDefaultMessage/基础结构） | ~100% | 逐字节对齐，OLDMODE 语义与 Delphi 编译产物一致，有 KAT + round-trip 测试背书（edcode_test.go / frame_test.go / types_test.go 共 34 个测试） |
| 客户端主干（渲染/动画/UI/输入/声音） | ~85% | 三层地图+Y-sort、HA 14 动作/MA 38 模板逐值一致、UI 框架忠实移植、声音门面层完整；缺长尾（特化怪物特效、UI 邮件/设置类、反作弊） |
| 服务端核心玩法 | ~80% | 36 个系统中 27 个完整、4 个部分、3 个不可达（P0）、2 个缺失 |
| 网关层能力（原 RunGate/LoginGate） | ~20% | 序号校验/敏感词过滤/IP 黑名单/发送背压基本未移植，ratelimit.go 只覆盖登录期 |
| 配置转换（cmd/serverconfig） | 结构 100% / 语义 ~5% | 数据库表/刷新点/商店/脚本数量全对齐；但 !setup.txt 3544 键仅映射 ~18 个，mapinfo.txt 地图属性转换后无人消费 |
| 消息常量覆盖 | SM 205/216、CM 81/85、RM 25/126 | 已覆盖常量无数值冲突；RM 缺口属架构差异（Go 状态同步直发 SM，不走 RM 中继） |

**最危险的三类缺口**：
1. **已实现但不可达**（P0）：处理器写好了，路由/映射没接上 —— 屠宰、私聊、好友、行会宣战、挖矿、2 个怪物 AI 全部静默失效。
2. **数据转换了但无人消费**：地图属性（SAFE/DARK/EXPRATE…）、公告、部分 Envir 列表。
3. **网关层防刷能力真空**：单进程合并后，RunGate 的防攻击/过滤/背压没有补偿实现。

---

## 二、P0 缺陷（已实现但不可用，修复成本极低）

| # | 问题 | 证据 | 修复方向 |
|---|------|------|----------|
| P0-1 | **7 个 CM 消息未在 main.go 转发**：屠宰/私聊/好友(3)/行会宣战/挖矿全部失效。处理器存在但消息到不了 | 转发 switch `cmd/server/main.go:890-1007` 无 CMButch(1007)/CMGuildWar(1047)/CMMineDig(1048)/CMWhisper(1049)/CMAddFriend(1052)/CMDelFriend(1053)/CMQueryFriends(1054)，落入 default 仅打 debug 日志；处理器在 `playobject.go:346-407`（HandleButch→butchsystem.go:22、HandleWhisper→chatsystem.go:80、Handle*Friend→friendsystem.go:13/35/55、HandleGuildWar→guildsystem.go:451、HandleMineDig→mining.go:83） | main.go handleGameMessage 补 7 个 case 转发（注意 CMButch 需带 Recog=目标ID） |
| P0-2 | **Race 134/214 AI 映射遗漏**：骷髅王召唤、火焰光环 AI 已实现却退化为普通近战 | `monsterobject.go:128-218` getAIBehavior 无 case 134/214；实现存在于 `monsterai.go:80`（AIFireAura 分发）、`monsterai.go:86`（AIBoneKing 分发），常量 monsterai.go:36,39 | getAIBehavior 补 `case 134: return AIBoneKing`、`case 214: return AIFireAura` |
| P0-3 | **挖矿链路断裂**：客户端发 CMHit+1（重击），服务端 HandleHit 无鹤嘴锄分支；CMMineDig 客户端从不发 | 客户端 `cmd/client/sceneplay.go:1868-1888`（Shape==19 鹤嘴锄 → sendAttack(CMHit+1)，与 Delphi ClMain.pas:2252-2267 一致）；Delphi 服务端路径 CM_BIGHIT+shape=19→MakeMine（ObjBase.pas:21895-21910）；Go HandleHit 无此分支，=DIG 服务端也从不发（Delphi ObjBase.pas:8834） | HandleHit 按 Delphi 加"鹤嘴锄+重击→挖矿"分支；或客户端改发 CMMineDig 并补 main.go 路由 |
| P0-4 | **地图属性空壳**：map_info.jsonc 已含 props（SAFE/DARK/DAYLIGHT/FIGHT/MUSIC/EXPRATE/NORECONNECT/DECHP…）但无人消费，地图级安全区、黑暗、经验倍率、禁回城等全部失效 | `cmd/server/envir.go:31-37` MapFlag 仅 5 字段且**全仓库无赋值点**（grep 验证）；`mapmanager.go` 不读 props；转换工具 `cmd/serverconfig/convert_maps.go:17,119-121` 已把 props 存好；Delphi 消费点 LocalDB.pas:530-600 解析、ObjBase.pas:4473,21533 安全区判定 | mapmanager 加载时解析 props→MapFlag；扩充 MapFlag 字段对齐 Delphi TMapFlag（Grobal2.pas:1369-1400） |
| P0-5 | **登录公告硬编码**，serverconfig/notice/ 已转换但未加载 | `cmd/server/main.go:358` 硬编码 "Welcome to MIR2 Go Server!"；Delphi NoticeM.pas:13-72 支持文件+跑马灯+周期重载 | 启动加载 notice.jsonc，替换硬编码 |
| P0-6 | **昼夜硬编码**：登录固定发 SMDayChanging(3)，未读地图 DARK/DAYLIGHT 标志（依赖 P0-4） | `playobject.go:2178-2179`；Delphi 按地图标志 DayBright（ObjBase.pas:4283-4296,5726）；客户端消费侧 playobject.go:2054,2232 已有 Flag.Dark 判断（等待服务端赋值） | P0-4 落地后，SendDayChanging 改读 envir.Flag |

---

## 三、P1 重大功能缺口（Delphi 有、Go 应有）

### 3.1 服务端

#### 魔法系统（Go 46/49 个 ID）
| 缺失 | Delphi 证据 | 状态 |
|------|-------------|------|
| 15 神圣战甲术 | Magic.pas:456-460 | ✅ 已实现（7×7 友方物防 buff） |
| 47 火龙气焰 | Magic.pas:665-672 | ✅ 已实现（含 DB 条目） |
| 49 净化术 | Magic.pas:678-680 | ✅ 已实现（Delphi 原样空实现） |
| 36 无极真气语义错位 | Delphi 36 与 50 MabMabe 是不同魔法 | ✅ 已解决：无极真气按 DB ID 50 实装（Delphi 36 语义），死代码 case 36 删除 |
| 40 解毒术 | Magic.pas:586-610 | ⚠️ 有意不做：Delphi ID 40 与 GEEM2 DB 的双龙斩冲突（byID 单键索引），不破坏现有内容；冰焰/MabMabe（Delphi 50）DB 无条目亦未实装 |

#### 怪物 AI
| 缺口 | 证据 | 状态 |
|------|------|------|
| Race 206 TKhazard 特殊追击 | ObjMon3.pas:142 | ✅ 已修复（AIKhazard） |
| Race 208 绿毒相邻攻击 | ObjMon3.pas:126,1269-1300 | ✅ 已修复（AIGreenPoison） |
| Race 209 红毒同类变种 | ObjMon3.pas:134,1312 | ✅ 已修复（AIRedPoison） |
| Race 210 霜虎无目标隐身伏击 | ObjMon3.pas:118,1220-1252 | ✅ 已修复（AIFrostTiger） |
| 训练师 Race 判定不一致 | Delphi Race=55（M2Share.pas:152） | ✅ 已修复（重写为 Race 55 沙袋+统计） |
| Race 20 弓箭警察（TArcherPolice） | ObjMon2.pas:96 | ✅ 已修复（复用 AIGuard，TArcherPolice 为 TArcherGuard 子类） |
| Race 120 足球（TSoccerBall，可推动物体） | ObjMon2.pas:134 | ✅ 已修复（AISoccerBall：被踢滚动+反弹+无敌） |
| 刷怪无在线人数加速 | Delphi GetZenTime（UsrEngn.pas:1097-1160） | ✅ 已修复（getZenTime，<30min 间隔按在线数加速最多 60%） |

> 注：Go 现有 34 个 AI 行为常量（AIMelee=0…AITrainer=33，monsterai.go:10-45），
> 覆盖 Delphi 工厂 58 分支中的绝大多数；AGENTS.md 中"12 种"的说法已过时。

#### NPC 脚本（Delphi ~100 条件 + ~230 动作 vs Go ~60 + ~90）
缺失的重要命令类别（对照 M2Share.pas:190-1000 常量表）：

| 类别 | 代表命令 | 影响 |
|------|----------|------|
| GME 活动副本 | SETGMEMAP/STARTNEWGME/MOVETOGMEMAP/FINISHGME 等 12 个（M2Share.pas:802-830） | 活动玩法全缺 |
| SQL 变量 | READVALUESQL/WRITEVALUESQL/INCVALUESQL 等 6 个（:838-849） | Go 有 VAR/SAVEVAR 文件替代，但无跨服语义 |
| 攻城器械 | MAKESHOOTER/CHARGESHOOTER/CAPTURECASTLEFLAG 等 9 个（:750-762） | 攻城炮手缺 |
| 武器/首饰精炼 | REFINEWEAPON/REFINEACCESSORIES/CHECKWEAPONATOM 等 9 个（:666,846-856） | 仅有 NPC 升级（upgrade.go） |
| 路径/区域 | CLEARPATH/ADDPATH/MAKEHEALZONE/MAKEDAMAGEZONE（:826-840） | 治疗区/伤害区缺 |
| 马匹/外观 | HORSECALL/KILLHORSE/HAIRCOLOR/HAIRSTYLE/WEARCOLOR（:646-652,704） | 脚本给马、换发型染色缺 |
| 天气/地图法术 | MAPSPELL/CHANGEWEATHER（:824,696） | 缺 |
| 声望/贡献 | INCFAME/DECFAME/CHECKCONTRIBUTION（:740-744） | 缺 |
| 玩家摆摊 | OPENUSERMARKET 等（:790-796） | 缺 |
| 条件长尾 | CHECKITEMTYPE/CHECKACCESSORY/CHECKVAR/CHECKMONAREA/CHECKDURAEVA/ONERROR/ELARGE 等 ~25 个（:232-500） | 复杂脚本无法运行 |

#### GM 命令（Delphi 192 vs Go ~30）
Go 已有：move/search/recall/slave/dearrecall/masterrecall/nomob/make/level/mob/gold/heal/takeonhorse/takeoffhorse/pkpoint/reloadnpc/kick/info/mapinfo/monclear/revive/superman/observe/shutup/recallmob/moveuser/changemode（gmcommands.go:23-383）。
缺失类别（M2Share.pas:2680-2940）：
- **热重载族**：ReloadAdmin/ReloadManage/ReloadMonItems/ReloadItemDB/ReloadMagicDB/ReloadMonsterDB/ReloadLineNotice/ReloadAbuse（Go 仅 reloadnpc）
- **封禁族**：DenyIPLogon/DenyAccountLogon/DenyCharNameLogon/Del/ShowDeny*
- **属性修改族**：LuckyPoint/Hunger/Training/DeleteSkill/ChangeJob/ChangeGender/BonusPoint/AddGold/DelGold/CreditPoint/ReNewLevel/AdjustLevel/AdjustExp/ChangeWeaponDura
- **城堡族**：SabukWallGold/ForcedWallconquestWar/ChangeSabukLord/ReloadGuildAll/SbkDoor
- **权限/信息族**：PrvMsg/AllowMsg/LetShout/LetTrade/MemberFunc/MobLevel/MobCount/HumanCount/ServerStatus

#### 持久化字段缺失（Delphi THumData 3164B vs Go SQLite 列+JSON）
| 缺失字段组 | Delphi 字段 | 状态 |
|-----------|-------------|------|
| 外观/朝向 | btDir/btHair | ✅ 已修复（hair 入库 + dir 入 meta） |
| 加点 | BonusAbil/nBonusPoint | ✅ 已修复（入 meta） |
| 声望/转生 | btCreditPoint/btReLevel | ✅ 已修复（入 meta） |
| 行为偏好 | btAllowGroup/btAttatckMode | ✅ 已修复（入 meta） |
| 状态效果 | wStatusTimeArr | ✅ 已修复（入 charMeta，上线恢复含隐身标志+状态广播） |
| 回城点 | sHomeMap/wHomeX/wHomeY | 设计决策：用全局安全区替代（playobject.go:1739） |
| 仓库密码 | sStoragePwd | ✅ 已实现（@setstoragepwd/@chgstoragepwd/@unlockstorage，4-7 位、错 >3 锁定、登录自动上锁、入 charMeta；storagesystem.go） |
| 元宝/游戏点/充值点 | nGameGold/nGamePoint/nPayMentPoint | 待实现（只有 Gold） |
| 贡献/饥饿/身体幸运 | wContribution/nHungerStatus/dBodyLuck | 饥饿度字段已实现（食物 +=DuraMax/10 上限 5000，入 charMeta，708 下发）；贡献/身体幸运未实现 |

#### 其他服务端缺口
- **仓库格数**：✅ 已修复并对齐 Delphi 真值。初版按"Delphi 50 格"理解有误——Delphi 运行时上限为 **39**（ObjBase.pas:24720 `Count<39`），存档数组 `TStorageItems[0..49]` 只是序列化容量。现 GetMaxStorageSlots 默认 **39**（config.go）。
- **城堡战**：~~缺预约战日期 WarDate、联盟行会判定~~ ✅ 已修复（WarDate 预约制+IsAttackAllyGuild/IsDefenseAllyGuild+状态持久化）；余 TechLevel 效果（castle.go 仅字段，需先调研 Delphi 语义）与皇宫/密道独立地图管理（backlog）
- **商人行为**：Delphi 商人移动/招揽（TMerchant.Run，UsrEngn.pas:1028-1092 ProcessMerchants）——经查 m_boCanMove 所有赋值点在 Delphi 中被注释（ConfigMerchant.pas:975/LocalDB.pas:1140,3418,3447），属死功能 → backlog（保留开关位）；攻城隐藏城堡商人已实现（usrengine.go ProcessNpcIdle）
- **行会 Run 语义**：~~Delphi g_GuildManager.Run 每 10s；Go 仅周期存档~~ ✅ 已修复（ProcessGuilds 每 10s：到期行会战移除+成员通知+落盘；wars/allies 持久化）
- **在线人数广播**：~~Delphi boSendOnlineCount（UsrEngn.pas:3054-3060），Go 无~~ ✅ 已修复（周期系统消息，配置 sendOnlineTime/sendOnlineCountRate/sendOnlineCountMsg）
- **CM 缺口**：~~CM_SOFTCLOSE(1009)/CM_USERGETDETAILITEM(1015)/CM_QUERYUSERNAME(80)/CM_SITDOWN(3012)~~ ✅ 已修复；CM_QUERYUSERSET(3040) 经查为 Delphi MD5 后门、CM_THROW(3005) Delphi 无处理器 → backlog（对照 ObjBase.pas:4663-5287）

### 3.2 客户端

#### SM 消息未处理清单（Delphi 216 个，Go 实际处理 185，剔除死常量后有效缺口）
| 值 | 消息 | Delphi 用途 |
|----|------|-------------|
| 528 | SM_OUTOFCONNECTION | 断线清理（ClMain.pas:3795,4162） |
| 640 | SM_MAGIC_LVEXP | 魔法等级/训练度更新（ClMain.pas:4522） |
| 652 | SM_SENDDETAILGOODSLIST | ✅ 已修复（服务端 HandleGetDetailItem + 客户端明细列表 UI） |
| 655 | SM_LAMPCHANGEDURA | 火把/油灯耐久（ClMain.pas:4042） |
| 685 | SM_DEALCHGGOLD_FAIL | 交易改金币失败（ClMain.pas:4824） |
| 707 | SM_TAKEBACKSTORAGEITEM_FULLBAG | 仓库取回背包满（ClMain.pas:4624） |
| 708 | SM_MYSTATUS | ✅ 已修复（登录发送+客户端存储；饥饿系统未实装恒 0） |
| 761/764/765 | SM_GUILDRANKUPDATE_FAIL/SM_DONATE_OK/FAIL | 行会职务/捐献反馈（ClMain.pas:4877,4926） |
| 766 | SM_AREASTATE | ✅ 发送侧已实现（登录+切图）；客户端图标渲染待做（DrawScrn.pas:369-386） |
| 802 | SM_RECONNECT | 换服重连（ClMain.pas:5161-5213）——P3 决策转服不做 → backlog |
| 806/807 | SM_SPACEMOVE_HIDE2/SHOW2 | ✅ 客户端已处理（与 800/801 合并，SHOW2 解析 body 换外观）；服务端暂无发送场景 |
| 810 | SM_TIMECHECK_MSG | 速度作弊检测（ClMain.pas:3563-3600）——随客户端反作弊 backlog |
| 1103 | SM_INSTANCEHEALGUAGE | ✅ 已修复（常量+治疗系发送侧+客户端 2s 头顶血条） |
| 1104 | SM_CHANGEFACE | ✅ 客户端已处理（含 WaitForRecogId 等待机制）；服务端变身现走 FEATURECHANGED |
| 5007/5008 | SM_SERVERCONFIG/SM_GAMEGOLDNAME | 服务器配置/元宝名称（ClMain.pas:3864,3898），未定义 |
| 5009/8002/8003 | SM_PASSWORD 族 | 二级密码（ClMain.pas:4279,4944），未定义 |
| 8001 | SM_PLAYDICE | 骰子小游戏（FState.pas:1945-1994），未定义 |

#### 渲染/表现
| 缺口 | 证据 |
|------|------|
| **特化怪物渲染类全缺**：34 个怪物类（专属死亡/攻击特效常量 DEATHEFFECTBASE/COWMONFIREBASE 等）+ 10 个物体类（草药/城门/龙/足球） | AxeMon.pas:11-256、HerbActor.pas 全文；Delphi 工厂 PlayScn.pas:2028-2149 按 race 实例化；Go 仅泛型 Actor 三分（actormanager.go:97-128） |
| **魔法特效保真度低** | ✅ 已修复：服务端 46 条 magID→effType/effNum 映射 + 客户端 14 类特效参数查表（数据驱动方案） |
| 武器发光缺失 | ✅ 已修复：Delphi DrawWeaponGlimmer 实为空实现，真实特效为 SM_BREAKWEAPON 武器破碎光效（Go 已接线 WeaponEffect） |
| 变身等待机制缺失 | ✅ 已修复（pendingFaces 队列，等 actor 空闲后应用，5s 超时强制） |
| 小地图第三色缺失 | ✅ 已修复（race 50/45/12 → 调色板 218） |
| 像素级命中检测缺失 | ✅ 已实现（sceneplay.go actorPixelHit，本表原说法已过时） |
| 公告模态确认缺失 | Delphi 公告以模态对话框显示并等待用户点 Ok 才发 CM_LOGINNOTICEOK（ClMain.pas:5732-5749）；Go 静默自动确认（main.go:1063-1065） |
| 字体切换缺失 | Delphi Ctrl+F 循环 8 种字体（ClMain.pas:1547-1563）；Go 单一 TTF |
| 全屏模式缺失 | Delphi DirectX 全屏；Go 仅窗口化 4 档分辨率（main.go:80-81） |

#### UI 缺口（Delphi TFrmDlg 233 个控件 vs Go 16 个 ui*.go）
| 缺失 | Delphi 位置 |
|------|-------------|
| 邮件系统 DMailListDlg/DMailDlg | FState.pas |
| 黑名单 DBlockListDlg | FState.pas |
| 游戏设置 DConfigDlg（Ctrl+Alt+F12） | FState.pas:3813-3822 |
| 二级密码输入 DChgGamePwd（Ctrl+D） | ClMain.pas:1527-1537 |
| 骰子小游戏 | FState.pas:1945-1994,2104-2113 |
| 好友面板完整功能：私聊/邮件/黑名单/翻页 | Go uifriend.go:7 自注"简化版" |
| Ctrl+F1..F8 第二套魔法键绑定 | Delphi DKsConF1-F8（ClMain.pas:1480-1481,1278-1283） |

#### 输入行为差异
| 项 | Delphi | Go |
|----|--------|-----|
| Alt+X 登出 / Alt+Q 退出 | 战斗中 10 秒禁止 + 确认对话框（ClMain.pas:1167-1195） | 直接发 CMLogout/CMExitGame，无守卫无确认（sceneplay.go:1550-1559） |
| VK_PAUSE 截图 | PrintScreenNow（ClMain.pas:1197-1266） | 无 |
| 移动模型 | 逐步贪心+侧滑绕障，遇障即停（ClMain.pas:1295-1454） | BFS 寻路半径 25（pathfind.go:13-73）——Go 能绕路，属增强而非缺失 |

#### 反作弊缺失
Delphi 客户端三定时器 CheckHackTimer/SpeedHackTimer/SendTimeTimer（ClMain.pas:108-110）、SM_TIMECHECK_MSG 1 小时窗口检测（ClMain.pas:3563-3600）、+GOOD 载荷速度比对（ClMain.pas:3651-3654）——Go 客户端全部未实现（服务端侧有超速检测替代，playobject.go:755-789）。

#### 其他客户端差异
- **背包本地持久化**：Delphi 登出写 `Data/<服名>.<角色>.itm`（ClMain.pas:5170,5189）；Go 无本地存档，服务端权威全量重同步——闭环下属合理设计，不需补。
- **受击音效简化**：不查攻击者武器/护甲（actorsound.go:103-112 自注）。

---

## 四、P2 网关层 / 安全 / 运维

| 缺口 | Delphi 证据 | Go 现状 |
|------|-------------|---------|
| 封包序列号校验 | RunGate 重复序号 nPacketErrCount++，>10 拒绝服务；缓冲 >20000 直接记 99（RunGate/Main.pas:363-413） | 仅静默剥离 code 数字（frame.go:56-60），不校验重复/乱序 |
| 聊天敏感词过滤 | WordFilter.txt 网关级重写 CM_SAY（RunGate/Main.pas:430-455） | 无 |
| 发言间隔限制 | dwSayMsgTime=1000ms（GateShare.pas:110） | 无 |
| IP/账号/机器码黑名单 | BlockIPList.txt/TempBlockIPList（GateShare.pas:93-94）；DenyIPAddrList/DenyAccountList/DenyMachineIDList（Envir 目录） | 仅 DenyChrNameList 建角黑名单（validation.go:19-42）；三类列表未转换未实现 |
| 发送背压/心跳 | 每 512B 插 `*`，2048B 未回显暂停发送 3 秒（RunGate/Main.pas:501-553）；客户端回显 `*`（ClMain.pas:2795-2799） | 客户端回显已实现（cmd/client/main.go:750-753）；服务端 FrameScanner keepalive 传 nil（server.go:189），无背压（SendChan 256 满则丢包 server.go:299-305） |
| =DIG 服务端发送 | 挖矿成功发 `=DIG`（ObjBase.pas:8834） | ✅ 已实现（playobject.go:885-886、mining.go:182-183 两处；本表原"服务端从不发"说法有误） |
| 客户端速度作弊检测 | SM_TIMECHECK_MSG + +GOOD 载荷（见 3.2） | 仅定义常量（message.go:871） |
| 自适应处理预算 | RunGate 接收/发送各 30-300ms 动态调节（RunGate/Main.pas:255-320） | goroutine 直接处理，无预算控制 |
| 动作速度配置剩余 | ActionSpeedConfig.pas:138-141 四组合间隔（RunLongHit/RunHit/WalkHit/RunMagic） | ✅ 已修复（config 四组合+checkActionTransition 复刻 CheckActionStatus） |
| Envir 配置长尾 | MonHPProgress/MonSpAbilList/MonDropLimitList/NameFilterList/ItemBind*/Highest&LowestSellingPrice/DisableMoveMap/Robot_def/SmartMonster | 未转换未实现（SmartMonster 仅建目录） |
| 在线人数广播 | boSendOnlineCount（UsrEngn.pas:3054-3060） | 无 |
| 登录期限流覆盖 | LoginSrv/DBServer 全链路 | ratelimit.go:24-88 已移植注册/改密/查角/建角限流；游戏期无每连接频率限制 |

---

## 五、P3 设计决策（明确不做，仅存档说明）

以下差异是单进程闭环架构的**刻意取舍**，不列为待办：

| 决策 | 内容 | 说明 |
|------|------|------|
| 单进程架构 | 8 进程（LoginGate/SelGate/RunGate/LoginSrv/DBServer/M2Server/LogDataServer/GameCenter）合并为单端口 | 进程间协议 GM_/SS_/DB_/DBR_/SG_（共 47 个常量）与 TMsgHeader 二进制协议自然消失，由函数调用替代；丢失的网关能力在 P2 中以"连接中间件链"方式补偿（见第六节） |
| 物品线格式 | 登录时 SMStdItems=715 批量下发物品库（message.go:353）+ 实例 10B 紧凑格式（itemsystem.go:107-129） | 与原版客户端/服务端不互通；Go 客户端专用闭环 |
| 角色存档 | SQLite 列+JSON meta 替代 THumData 3164B 二进制档 | 不做原版 FDB 兼容；如需迁移可另做一次性导入器 |
| cert 语义 | 会话内随机数（main.go:669-676）替代跨进程会话 ID | 单进程无需跨进程准入列表 |
| 重连流程 | 断开→重连同一地址重新认证 | 模拟三段式但只有一条物理连接 |
| 图形栈 | OpenGL 3.3 + GLFW 替代 DelphiX/DirectX | 渲染管线等价重写，附加分辨率切换/调试控制台增强 |
| 声音栈 | gopxl/beep 替代 DirectSound/DirectShow | 增加解码缓存+32 复音上限（sound.go:23,265-292），属改进 |
| 登录器协议 | SM_SENDGAMELIST(5002)/SM_GETREGINFO(8004)/CM_SERVERREGINFO(65074) 等 | GameCenter/登录器生态不适用 |
| Robot 假人/跨服转服 | ObjRobot.pas、SendChangeServer（UsrEngn.pas:2238） | 按需评估，默认不做 |
| Delphi GUI 管理窗体 | ViewKernelInfo/ViewOnlineHuman 等 15+ 窗体 | 由 JSONC 配置+GM 命令替代；如需可视化运维另做 Web 控制台 |
| 源码基准 | Client/ 与 MirClient/ 双份源码以 Client/ 为权威 | 已知差异：MAXBAGITEM 52 vs 46（Go 取 46，与服务端协议一致）、ClMain.pas 1284 行 diff |

---

## 六、现代游戏项目架构建议

### 6.1 消息注册表 + 协议覆盖测试（最高优先）
P0-1/P0-2/P0-3 全是同一类 bug：**处理器写了，路由没接上**。靠 code review 防不住，要靠机制：
- 建立单一事实源：把 CM/SM 常量、方向、body 格式、处理器函数集中登记（Go 可用 map 注册表或 init 注册，替代 main.go 与 playobject.go 两处手写 switch）。
- 增加"协议覆盖测试"：反射/扫描 `internal/protocol` 中所有 CM_/SM_ 常量，断言每个常量要么出现在路由测试的已登记集合中，要么显式标记 `unimplemented`。本次发现的 7 个死代码 + 2 个 AI 映射遗漏，一个测试就能全部捕获。
- 顺手收益：注册表可自动生成协议文档，避免 doc/技术参考.md 与代码漂移（本次已发现 3 处文档错误：sceneserverselect.go 不存在、MA 模板 39→38 种、TMsgHeader 16→20 字节）。

### 6.2 数据驱动管线
- 配置 → 启动校验 → 热重载三级管线：P0-4/P0-5 暴露的是"转换了但没人消费"，根因是加载链路没有闭环校验。建议启动时校验每个 jsonc 的必填字段与消费方存在性；把 Delphi 的 19 个 Reload* 命令收敛为统一 `/reload <模块>`。
- NPC 脚本命令表驱动化：当前 npcscript.go 2478 行巨型 switch，~150 个 case。改为 `map[string]ConditionFunc` / `map[string]ActionFunc` 注册表后，长尾命令（3.1 节 ~240 个）可增量补齐且可单测。
- 魔法特效数据化：Go 服务端 effType 三分类（magicsystem.go:714-722）+ 客户端特效表，比移植 Delphi 44 个硬编码特化类（AxeMon/HerbActor）更符合现代做法——把"哪个 magid 用什么特效参数"放进 magiceff.jsonc。

### 6.3 连接中间件链（补偿网关层）
单进程不代表放弃网关能力。在 `internal/netserver` 内建分层管道：

```
原始字节 → 帧解析 → 序列号校验(重复/乱序计数封禁) → 频率限制(每连接令牌桶) → 敏感词过滤(仅 CMSay) → dispatch
```

每层独立可测、可配置开关。这比把逻辑散进业务代码（现状：限流在 ratelimit.go、超速在 playobject.go、过滤不存在）更可持续，也是 P2 全部缺口的统一落点。

### 6.4 服务端对象拆分，防"ObjBase 化"
ObjBase.pas 26821 行是 Delphi 版的维护噩梦。Go 侧 playobject.go 已 2345 行，且还在吸收新系统（marriage/mentorsystem/friendsystem/questsystem 已独立成文件，是好趋势）。建议明确约定：
- 每个玩法系统 = `CM 处理器 + tick 钩子 + 持久化钩子` 三件套，注册进 UserEngine 而不是塞进 PlayObject 方法集。
- PlayObject 只保留移动/战斗/视野/消息分发主干。

### 6.5 客户端资源与平台层
- ~~WIL LRU 淘汰~~ 已完成：落点改在 wil 包内（GetImage 直连调用方多，引擎层覆盖不全）——全局字节预算（默认 128MB，wil.SetCacheLimit 可调）+ 每文件 LRU，GPU 上传后 ReleasePixels 归还像素保留元数据壳。Delphi 原版（WIL.pas:546）实为 5 分钟 TTL、字节配额被注释，Go 版做了真 LRU。
- **IME**：GLFW 无原生 IME，中文输入目前依赖 char 回调（无候选框）。如需完善，走平台层（X11 ibus / Win32 IMM32）——工作量不小，建议放 P2 之后。
- **像素级命中检测**：可用 WIL 图像 alpha 做精灵精确点选（Delphi CheckSelect，PlayScn.pas:1785-1818），提升点选体验，属低成本增强。

### 6.6 存储演进
SQLite 方向正确。补齐 3.1 节持久化字段时建议：
- schema 迁移版本化（sqlite.go:141-220 已有 migrateAccounts/migrateGuilds 雏形，扩展为通用 migration 表）。
- 开启 WAL 模式减少 tick 内写阻塞；角色落盘集中在登出+周期存档两个时机（对齐 Delphi TFrontEngine 异步写语义，Go 用同步 SQLite 目前可接受）。

### 6.7 可观测性
Delphi 用 5 个监控 GUI（ViewKernelInfo/ViewLevel/ViewList/ViewOnlineHuman/ViewSession），Go 应替代为：
- `/status` GM 命令：在线数、各地图对象数、tick 耗时、内存。
- internal/log 增加分级+模块标签约定（现状已有 LevelX+模块名，统一即可）。
- 在线人数周期广播（对齐 Delphi boSendOnlineCount）。

### 6.8 工程化基线
- ~~补最小 CI~~ 用户决策：不加 CI，保持本地验证（`go build ./... && go vet ./... && go test ./... -count=1`）。Go 侧已有 70+ 测试（协议 KAT/帧/类型/移动/战斗/物品/UI/地图/协议覆盖）是很好的基线，Delphi 原版零测试。
- 文档同步机制：doc/技术参考.md 的勘误（3 处）说明文档与代码会漂移；建议把本次路线图作为活文档，每完成一项打勾。

---

## 七、建议实施顺序（已全部执行完毕）

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | P0 六项全修 | ✅（2026-08-04 上午） |
| 2 | 协议覆盖测试 + 消息注册表（6.1） | ✅ 批次4（覆盖测试+豁免表方案，等效拦截） |
| 3 | P1 服务端核心 | ✅ 批次1-2 |
| 4 | P1 客户端长尾 | ✅ 批次3 |
| 5 | NPC 脚本/GM 命令长尾 | ✅ 批次5（混合表驱动+12 条玩法命令，余长尾由未知命令告警驱动） |
| 6 | P2 连接中间件链（6.3） | ✅ 批次6（五层补偿） |
| 7 | 工程化：CI、/status、WIL LRU | ✅ 批次7 |

## 八、Backlog（记录不排期，按需立项）

**渲染/表现**
- 特化怪物渲染 34 类 + 物体类 10 个：走数据化方案（参照魔法特效 magiceff 模式），不逐类移植 AxeMon/HerbActor。
- 符咒飞行段→爆炸段切换、符咒地面 16/24 帧、雷电2 10 帧（批次3 保留 Go 现值的已知偏差）。
- SM_AREASTATE(766) 客户端区域图标渲染（发送侧已实现）。
- Ctrl+F 字体切换、全屏模式、IME 候选框（平台层）。

**UI/系统**
- 邮件系统、黑名单面板、游戏设置面板、二级密码（SM_PASSWORD 族 5009/8002/8003）、骰子小游戏（8001）、好友面板完整功能（私聊/邮件/翻页）。
- 饥饿系统（SM_MYSTATUS 管道已铺，服务端恒 0）。

**服务端长尾**
- 解毒术（Delphi 40）：ID 体系迁移决策后再做（与双龙斩冲突）。
- 城堡战 TechLevel 效果逻辑、皇宫/密道独立地图管理。
- 商人巡逻（Delphi 死代码，保留 m_boCanMove 开关位）。
- GM 命令长尾（封禁族/属性修改族/城堡族，Delphi 192 vs Go ~31）。
- NPC 脚本长尾（GME 副本/SQL 变量/攻城器械/天气法术/玩家摆摊等 ~220 条，由未知命令告警日志实测驱动）。
- CM_QUERYUSERSET(3040)（Delphi MD5 后门）、CM_THROW(3005)（Delphi 无处理器）。

**协议/安全**
- 客户端反作弊（810 TIMECHECK 响应、CheckHackTimer/SpeedHackTimer、+GOOD 速度载荷）——服务端已有超速检测替代。
- SM_RECONNECT(802) 换服重连——P3 决策转服不做。
- SMSpell2/SMCertificationSuccess/SMIDNotFound/SMItemUpdate——旧协议变体，Go 闭环有替代通道。

**持久化**
- 元宝/游戏点/充值点/贡献/身体幸运（付费体系）；仓库密码 StoragePwd（无设置/解锁流程）。

## 附：调查方法与证据索引

- 客户端对比：模块映射（25 单元）、场景流程（5 状态）、SM 覆盖 185/216、渲染 10 维、动画逐值核对（HA 14 / MA 38 / GetOffset / WORDER）、UI 233 控件对照、输入键位表、声音 12 项。
- 服务端对比：对象模型映射（12 类）、玩法系统 36 项、怪物 AI 逐 Race 表（35 Race）、NPC 脚本规模（100+230 vs 60+90）、引擎 tick 12 环节、持久化字段组、CM 覆盖 66/85。
- 协议/架构对比：常量族 9 组统计、结构布局 8 个逐字节核对、三份 EDcode.pas diff 一致、帧/流控 7 机制、8 进程职责表、地图/WIL 格式逐项、配置转换抽查（StdItems 686/Monster 378/Magic 104/mapinfo 357/routes 2460/mongen 3402/merchant 168 全对齐，2 行源脏数据合理跳过）、登录时序三段式对照。
- 主要 Delphi 参考：ObjBase.pas(26821行)/ObjNpc.pas(11556)/M2Share.pas(11087)/UsrEngn.pas(3176)/ClMain.pas(6500)/FState.pas(6831)/Actor.pas(3944)/PlayScn.pas(2366)/Grobal2.pas(2739)。
- 已知文档勘误（本次发现，均已修正）：AGENTS.md 的 sceneserverselect.go 不存在；doc/技术参考.md "MA 39 种"实为 38 种（无 MA18）、TMsgHeader 实为 20 字节（非 packed）；AGENTS.md "怪物 AI 12 种"实为 34 种行为。
