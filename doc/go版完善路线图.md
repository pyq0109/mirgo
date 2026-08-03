# Go 版完善路线图（以 Delphi 版为权威基线）

> 调查基准：Delphi 原版源码 `asset/delphi/`（lzxsz/MIR2 commit 98711da，客户端以 `Client/` 为权威，与 `MirClient/` 的差异处单独标注）；Go 实现 `cmd/` + `internal/`。
> 调查方式：3 轮全量代码对比（客户端 / 服务端 / 协议与架构），所有结论附 file:line 证据，关键 P0 项经二次人工核验。
> 约定：物品线格式、角色存档、单进程架构等**刻意不兼容项**按设计决策处理（见第五节），不列入待办。

---

## 修复进度（2026-08-04）

**已完成 ✅**

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
| 缺失 | Delphi 证据 | 说明 |
|------|-------------|------|
| 15 神圣战甲术 | Magic.pas DoSpell 分支 | Go magicsystem.go:120-625 无 case 15 |
| 40 解毒术 | 同上 | 无 case 40 |
| 47 火龙气焰 | 同上 | 无 case 47 |
| 49 净化术 | 同上 | 无 case 49 |
| 36 无极真气语义错位 | Delphi 36 与 50 MabMabe 是不同魔法 | Go case 36 实现的是 Delphi 50 的语义（magicsystem.go:574） |

#### 怪物 AI
| 缺口 | 证据 |
|------|------|
| Race 20 弓箭警察（TArcherPolice） | ObjMon2.pas:96，Go 无 |
| Race 120 足球（TSoccerBall，可推动物体） | ObjMon2.pas:134，Go 无 |
| Race 206 TKhazard 特殊追击 | ObjMon3.pas:142，Go 退化 AIMelee |
| Race 208 绿毒相邻攻击 | ObjMon3.pas:126,1269-1300，Go 无 |
| Race 209 红毒同类变种 | ObjMon3.pas:134,1312，Go 无 |
| Race 210 霜虎无目标隐身伏击 | ObjMon3.pas:118,1220-1252，Go 无 |
| 训练师 Race 判定不一致 | Delphi Race=55（M2Share.pas:152），Go 判 Race==2（trainernpc.go:6-8） |
| 刷怪无在线人数加速 | Delphi GetZenTime（UsrEngn.pas:1097-1160），Go mongen.go 未实现 |

> 注：Go 现有 29 个 AI 行为常量（monsterai.go:11-39），已覆盖 Delphi 工厂的绝大多数 Race；AGENTS.md 中"12 种"的说法已过时。

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
| 缺失字段组 | Delphi 字段 | 后果 |
|-----------|-------------|------|
| 外观/朝向 | btDir/btHair | 重登后方向发型重置 |
| 状态效果 | wStatusTimeArr | 下线清毒/清隐身 |
| 回城点 | sHomeMap/wHomeX/wHomeY | Go 用全局安全区替代（playobject.go:1739） |
| 加点 | BonusAbil/nBonusPoint | 运行时有点数（playobject.go:122）但不落盘 |
| 声望/转生 | btCreditPoint/btReLevel | 运行时有字段不落盘 |
| 仓库密码 | sStoragePwd | 无设置/解锁流程 |
| 元宝/游戏点/充值点 | nGameGold/nGamePoint/nPayMentPoint | 只有 Gold |
| 行为偏好 | btAllowGroup/btAttatckMode/btIncHealth* | 重登重置 |
| 贡献/饥饿/身体幸运 | wContribution/nHungerStatus/dBodyLuck | 系统本身未实现 |

#### 其他服务端缺口
- **仓库格数**：Go 默认 39 格（config.go:506）vs Delphi 50 格（Grobal2.pas:817-818）
- **城堡战**：缺预约战日期 WarDate、联盟行会判定 IsAttackAllyGuild/IsDefenseAllyGuild、皇宫/密道地图管理、TechLevel 效果（castle.go:94 仅字段）；对照 Castle.pas:39-127
- **商人行为**：Delphi 商人移动/招揽（TMerchant.Run，UsrEngn.pas:1028-1092 ProcessMerchants）；Go 商人静止（usrengine.go:156-163 仅补货+存档）
- **行会 Run 语义**：Delphi g_GuildManager.Run 每 10s（Guild.pas:261）；Go 仅周期存档（main.go:430-432），行会战到期自动结束依赖 WarGuilds.EndTick（guildsystem.go:18-20）需补 tick 检查
- **在线人数广播**：Delphi boSendOnlineCount（UsrEngn.pas:3054-3060），Go 无
- **CM 缺口**：CM_SOFTCLOSE(1009)/CM_USERGETDETAILITEM(1015)/CM_QUERYUSERNAME(80)/CM_QUERYUSERSET(3040)/CM_SITDOWN(3012)/CM_THROW(3005) 未处理（对照 ObjBase.pas:4663-5287）

### 3.2 客户端

#### SM 消息未处理清单（Delphi 216 个，Go 实际处理 185，剔除死常量后有效缺口）
| 值 | 消息 | Delphi 用途 |
|----|------|-------------|
| 528 | SM_OUTOFCONNECTION | 断线清理（ClMain.pas:3795,4162） |
| 640 | SM_MAGIC_LVEXP | 魔法等级/训练度更新（ClMain.pas:4522） |
| 652 | SM_SENDDETAILGOODSLIST | 商店详细商品（ClMain.pas:4669） |
| 655 | SM_LAMPCHANGEDURA | 火把/油灯耐久（ClMain.pas:4042） |
| 685 | SM_DEALCHGGOLD_FAIL | 交易改金币失败（ClMain.pas:4824） |
| 707 | SM_TAKEBACKSTORAGEITEM_FULLBAG | 仓库取回背包满（ClMain.pas:4624） |
| 708 | SM_MYSTATUS | 饥饿状态图标（ClMain.pas:3901） |
| 761/764/765 | SM_GUILDRANKUPDATE_FAIL/SM_DONATE_OK/FAIL | 行会职务/捐献反馈（ClMain.pas:4877,4926） |
| 766 | SM_AREASTATE | 战斗区域图标（DrawScrn.pas:369-386） |
| 802 | SM_RECONNECT | 换服重连（ClMain.pas:5161-5213） |
| 806/807 | SM_SPACEMOVE_HIDE2/SHOW2 | 传送特效变体（ClMain.pas:3960） |
| 810 | SM_TIMECHECK_MSG | 速度作弊检测（ClMain.pas:3563-3600） |
| 1103 | SM_INSTANCEHEALGUAGE | 瞬时头顶血条（ClMain.pas:4303），Go 未定义 |
| 1104 | SM_CHANGEFACE | 变身换外观（ClMain.pas:4268） |
| 5007/5008 | SM_SERVERCONFIG/SM_GAMEGOLDNAME | 服务器配置/元宝名称（ClMain.pas:3864,3898），未定义 |
| 5009/8002/8003 | SM_PASSWORD 族 | 二级密码（ClMain.pas:4279,4944），未定义 |
| 8001 | SM_PLAYDICE | 骰子小游戏（FState.pas:1945-1994），未定义 |

#### 渲染/表现
| 缺口 | 证据 |
|------|------|
| **特化怪物渲染类全缺**：34 个怪物类（专属死亡/攻击特效常量 DEATHEFFECTBASE/COWMONFIREBASE 等）+ 10 个物体类（草药/城门/龙/足球） | AxeMon.pas:11-256、HerbActor.pas 全文；Delphi 工厂 PlayScn.pas:2028-2149 按 race 实例化；Go 仅泛型 Actor 三分（actormanager.go:97-128） |
| **魔法特效保真度低**：Delphi 客户端按 magid 本地查表生成特化特效（大 case 含爆炸参数/光效/帧数）；Go 服务端只把 12 个 magID 分为飞行/地面，其余默认爆炸 | PlayScn.pas:1448-1660 vs cmd/server/magicsystem.go:714-722 |
| 武器发光缺失 | Delphi DrawWeaponGlimmer（Actor.pas:1994-2016），Go 无 |
| 变身等待机制缺失 | Delphi m_nWaitForRecogId（PlayScn.pas:916-923），配合 SM_CHANGEFACE |
| 小地图第三色缺失 | Delphi race 50/45/12 用调色板 218（PlayScn.pas:831-835）；Go 仅两色（minimap.go:99-103 自注） |
| 像素级命中检测缺失 | Delphi CheckSelect 像素判定+包围盒回退（PlayScn.pas:1785-1818）；Go 仅包围盒（sceneplay.go:1975-1982） |
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
| =DIG 服务端发送 | 挖矿成功发 `=DIG`（ObjBase.pas:8834） | 客户端接收已实现（client/main.go:766,838），服务端从不发 |
| 客户端速度作弊检测 | SM_TIMECHECK_MSG + +GOOD 载荷（见 3.2） | 仅定义常量（message.go:871） |
| 自适应处理预算 | RunGate 接收/发送各 30-300ms 动态调节（RunGate/Main.pas:255-320） | goroutine 直接处理，无预算控制 |
| 动作速度配置剩余 | ActionSpeedConfig.pas:138-141 四组合间隔（RunLongHit/RunHit/WalkHit/RunMagic） | config.go:37-48 仅六项基础间隔 |
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
- **WIL LRU 淘汰**：解码结果永久驻留（wil.go:196-202），长时间跑图内存只增不减。Delphi 有 FreeOldMemorys+MaxMemorySize（WIL.pas:45），Go 补 LRU（引擎层 resourcemanager.go 是合适落点）。
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
- 补最小 CI：`go build ./... && go build -tags x11 ./cmd/client && go test ./... -count=1`。Go 侧已有 40+ 测试（协议 KAT/帧/类型/移动/战斗/物品/UI/地图）是很好的基线，Delphi 原版零测试。
- 文档同步机制：doc/技术参考.md 的勘误（3 处）说明文档与代码会漂移；建议把本次路线图作为活文档，每完成一项打勾。

---

## 七、建议实施顺序

| 阶段 | 内容 | 预估 | 验收 |
|------|------|------|------|
| 1 | P0 六项全修 | 1-2 天 | 屠宰/私聊/好友/行会宣战/挖矿可用；Race134/214 怪物行为正确；黑暗地图生效；公告显示转换后文本 |
| 2 | 协议覆盖测试 + 消息注册表（6.1） | 2-3 天 | 测试能捕获本批 P0；新增 CM/SM 必须登记 |
| 3 | P1 服务端核心：魔法补齐、6 类特殊怪、持久化字段、仓库 50 格、昼夜/地图属性消费完善 | 1-2 周 | 逐项对照 3.1 清单 |
| 4 | P1 客户端长尾：未处理 SM 清单、公告模态、登出守卫、Ctrl+F1-F8、小地图三色、魔法特效数据化 | 1-2 周 | 逐项对照 3.2 清单 |
| 5 | NPC 脚本/GM 命令长尾（表驱动化先行，6.2） | 持续 | 脚本命令单测 |
| 6 | P2 连接中间件链（6.3） | 1 周 | 序号校验/限流/过滤分层可测 |
| 7 | 工程化：CI、/status、WIL LRU | 穿插 | CI 绿 |

## 附：调查方法与证据索引

- 客户端对比：模块映射（25 单元）、场景流程（5 状态）、SM 覆盖 185/216、渲染 10 维、动画逐值核对（HA 14 / MA 38 / GetOffset / WORDER）、UI 233 控件对照、输入键位表、声音 12 项。
- 服务端对比：对象模型映射（12 类）、玩法系统 36 项、怪物 AI 逐 Race 表（35 Race）、NPC 脚本规模（100+230 vs 60+90）、引擎 tick 12 环节、持久化字段组、CM 覆盖 66/85。
- 协议/架构对比：常量族 9 组统计、结构布局 8 个逐字节核对、三份 EDcode.pas diff 一致、帧/流控 7 机制、8 进程职责表、地图/WIL 格式逐项、配置转换抽查（StdItems 686/Monster 378/Magic 104/mapinfo 357/routes 2460/mongen 3402/merchant 168 全对齐，2 行源脏数据合理跳过）、登录时序三段式对照。
- 主要 Delphi 参考：ObjBase.pas(26821行)/ObjNpc.pas(11556)/M2Share.pas(11087)/UsrEngn.pas(3176)/ClMain.pas(6500)/FState.pas(6831)/Actor.pas(3944)/PlayScn.pas(2366)/Grobal2.pas(2739)。
- 已知文档勘误（本次发现）：AGENTS.md 的 sceneserverselect.go 不存在；doc/技术参考.md "MA 39 种"实为 38 种（无 MA18）、TMsgHeader 实为 20 字节（非 packed）；AGENTS.md "怪物 AI 12 种"实为 29 种行为。
