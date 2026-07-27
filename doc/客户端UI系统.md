# 客户端 UI 系统审查报告

> 基于 Delphi `Client/FState.pas` (6831行) + `DWinCtl.pas` 与 Go `cmd/client/sceneplay.go` (1428行) 的逐项对照。
> 审查时间: 2026-07-27。仅审查，不修复。

## 架构概览

### Delphi 参考

- **主 UI 窗体**: `TFrmDlg`（`Client/FState.pas`）
- **UI 框架**: 自建 `DWinCtl.pas` 控件库 — `TDWindow`, `TDButton`, `TDGrid`
- **Grid 默认值** (`DWinCtl.pas:699-705`): ColCount=8, RowCount=5, ColWidth=36, RowHeight=32
- **屏幕**: 800×600 (`Share.pas:23-24`)
- **资源 WIL** (`Share.pas:46-68`, `MShare.pas:155-165`):
  - `Prguse.wil` → `g_WMainImages`（所有 UI 背景/按钮）
  - `Items.wil` → `g_WBagItemImages`（背包物品图标）
  - `StateItem.wil` → `g_WStateItemImages`（装备槽图标）
  - `DnItems.wil` → `g_WDnItemImages`（地面物品图标）
  - `MagIcon.wil` → `g_WMagIconImages`（技能图标）

### Go 实现

- **主 UI**: `PlayScene.RenderUI()`（`cmd/client/sceneplay.go`）
- **UI 框架**: 原生 OpenGL quad 绘制 + `engine.TextRenderer`
- **屏幕**: 1024×768 硬编码 (`sceneplay.go:187,1061`)
- **资源 WIL** (`internal/engine/resourcemanager.go:28-40,128-164`):
  - `Prguse.wil`, `Prguse2.wil`, `Prguse3.wil`, `Items.wil`, `StateItem.wil`, `DnItems.wil`, `MagIcon.wil` — 全部已加载

---

## 一、底部 HUD 状态栏

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 背景图 | `Prguse.wil[1]`(800px)/`[2]`(1024px)，上半透明+下半不透明 (`FState.pas:3560-3594`) | `Prguse.wil[1]` 单 quad (`sceneplay.go:1054-1068`) | **差异** — 缺上半透明分割 |
| HP/MP 条 | `Prguse.wil[4]` 裁剪式；战士<28级用 `[5]+[6]` 单HP条 (`FState.pas:3606-3638`) | `Prguse.wil[4]` 暗色覆盖空余部分 (`sceneplay.go:1070-1101`) | **差异** — 缺战士特殊血条 |
| HP/MP 文字 | 不绘制在条上（仅 tooltip） | `"%d/%d"` 固定位置 (`sceneplay.go:1102-1103`) | 正确（简化） |
| 等级显示 | `PomiTextOut` 计算位置 (`FState.pas:3643`) | `"Lv.%d"` 文字 (`sceneplay.go:1104`) | 正确 |
| 经验条 | `Prguse.wil[7]` 裁剪 (`FState.pas:3646-3661`) | 无 | **缺失** |
| 重量条 | `Prguse.wil[7]` 裁剪 (`FState.pas:3663-3676`) | 无 | **缺失** |
| 昼夜图标 | `Prguse.wil[12-15]` 按 `g_nDayBright` 切换 (`FState.pas:3597-3604`) | 无 | **缺失** |
| 饥饿指示 | `Prguse.wil[16-19]` (`FState.pas:3682-3688`) | 无 | **缺失** |
| 聊天区 | 9行可滚动，点击可私聊 (`FState.pas:3692-3706,1914-1926`) | 10行色码，无交互 (`sceneplay.go:1172-1194`) | **差异** — 缺点击私聊/滚动 |
| 聊天输入 | `EdChat` TEdit，Enter 切换 (`FState.pas`) | `chatMode` bool，字符输入，Enter 切换 (`sceneplay.go:744-750,778-788`) | 正确（基础功能） |

### 功能按钮

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 状态/背包/魔法/设置 (4个) | `Prguse.wil[8-11]`，沿底部斜坡排列，可点击 (`FState.pas:1194-1205`) | `Prguse.wil[8-11]` 固定位置渲染 (`sceneplay.go:1106-1123`) | **差异** — 无鼠标命中检测 |
| 小地图/交易/行会/组队/加点/好友/备忘/退出/登出 (9个) | `Prguse.wil[128-140,530-532]` 沿底部排列 (`FState.pas:1210-1239`) | 无 | **缺失** |

---

## 二、腰带快捷栏

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 布局 | 6格 32×29px 43px间距，`g_ItemArr[0..5]` (`FState.pas:1245-1273`) | 6格 32×32px 36px间距 (`sceneplay.go:1145-1165`) | **正确** |
| 图标 | `Items.wil` by `Looks` (`FState.pas:3836-3853`) | `Items.wil` by `item.Idx` (`sceneplay.go:1150-1163`) | **正确** |
| 槽位编号 | `PomiTextOut` 1-6 (`FState.pas:3851`) | `DrawText` 1-6 (`sceneplay.go:1164`) | **正确** |
| 鼠标交互 | 点击拾取/放下，双击使用 (`FState.pas:3868-3920`) | 仅键盘 1-6 使用 (`sceneplay.go:829-836`) | **缺失** |
| 悬停 tooltip | `DBelt1MouseMove` 显示物品信息 (`FState.pas:3855-3866`) | 无 | **缺失** |

---

## 三、技能快捷栏

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 布局 | 8个 F 键槽，底部栏区域 | 8槽 34×34px 38px间距 y=720 (`sceneplay.go:1125-1143`) | **正确** |
| 图标 | `MagIcon.wil` 按魔法图标索引 (`FState.pas:3479-3497`) | `MagIcon.wil` by `MagID` (`sceneplay.go:1130-1141`) | **正确**（MagID 与图标索引可能不同） |
| F 键施法 | F1-F8 释放绑定魔法 (`FState.pas:3506-3545`) | F1-F8 释放 `Magics[slotIdx]` (`sceneplay.go:814-827`) | **正确** |
| 键位绑定对话框 | `DKeySelDlg` 模态框：图标+F1-F8+Ctrl+F1-F8+OK/None (`FState.pas:5277-5398`) | 无 | **缺失** |

---

## 四、背包面板

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 背景 | `Prguse.wil[3]` (`FState.pas:1167`) | `Prguse.wil[3]` + 回退色块 (`sceneplay.go:1231-1233`) | **正确** |
| 网格 | `TDGrid` DItemGrid: 8列×6行=48格，偏移+6(腰带)=slots[6..51]，36×32px (`FState.pas:1171-1174,4649-4661`) | 8列 46格 36px (`sceneplay.go:1237-1267`) | **正确** |
| 物品图标 | `Items.wil` by `Looks` (`FState.pas:4652`) | `Items.wil` by `item.Idx` (`sceneplay.go:1246-1266`) | **正确** |
| 金币显示 | `Prguse.wil[29]` 按钮 + 金币文字 (`FState.pas:1279-1281`) | 文字 "金币: %d" (`sceneplay.go:1269`) | **差异** — 无金币图标 |
| 关闭按钮 | `Prguse.wil[371]` (`FState.pas:1288-1292`) | 无（B 键切换） | **缺失** |
| 拖拽系统 | `g_boItemMoving` 全局状态：点击拾取→点击放下→交换 (`FState.pas:613-621,4557-4600`) | 无 | **缺失**（关键） |
| 双击使用 | `DItemGridDblClick`: StdMode<=4 or 31 → EatItem (`FState.pas:4602-4641`) | 无 | **缺失** |
| 右键拖拽 | 右键拖动移动物品 (`FState.pas:4536-4538`) | 无 | **缺失** |
| 悬停 tooltip | `GetMouseItemInfo`: 名称/耐久/属性，按可用性着色 (`FState.pas:4527-4554,3935-4135`) | 无 | **缺失** |
| 丢弃物品 | 拖到背景 → `SendDropItem` (`FState.pas:1842-1854,1865-1886`) | 无 | **缺失** |
| 丢弃金币 | 拖金币 → 数量对话框 → `SendDropGold` (`FState.pas:1870-1882`) | 无 | **缺失** |

---

## 五、装备/状态面板

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 背景 | `Prguse.wil[370]`，右对齐 (`FState.pas:981-986`) | `Prguse.wil[370]` x=780 (`sceneplay.go:1276`) | **正确** |
| 4页切换 | Page0=装备纸娃娃, Page1=属性, Page2=详细属性, Page3=魔法 (`FState.pas:2804-2998`) | 单页文字列表 (`sceneplay.go:1282-1290`) | **缺失** — 缺3页 |
| 纸娃娃 | 性别体型 `Prguse.wil[376/377]` + 头发 `[440+]` + 衣服/武器/头盔 from `StateItem.wil` (`FState.pas:2805-2853`) | 无 | **缺失** |
| 13装备槽 | 环绕纸娃娃定位: 项链/头盔/照明/左右手镯/左右戒指/武器/衣服/护身符/腰带/鞋子 (`FState.pas:987-1042`) | 文字列表，无位置布局 (`sceneplay.go:1282-1290`) | **缺失** |
| 装备槽图标 | `StateItem.wil` by `Looks`，34×31 槽居中 (`FState.pas:3041-3185`) | 显示 `#WIndex` 文字 (`sceneplay.go:1288`) | **缺失** |
| 穿脱交互 | 点击空槽+持物 → `SendTakeOnItem`；点击已装备 → `SendTakeOffItem` (`FState.pas:3257-3400`) | 无 | **缺失** |
| Page1: 属性 | AC/MAC/DC/MC/SC/HP/MP 文字 (`FState.pas:2855-2870`) | 无 | **缺失** |
| Page2: 详细属性 | 经验%/重量/穿戴重量/手持重量/命中/攻速/抗魔/抗毒/恢复 (`FState.pas:2871-2930`) | 无 | **缺失** |
| Page3: 魔法列表 | 每页5个，MagIcon.wil 图标+名称/等级/训练进度/快捷键 (`FState.pas:2931-2998`) | 无（仅底部技能栏） | **缺失** |
| 页导航按钮 | `Prguse.wil[387/388]` 上一页/下一页 (`FState.pas:1079-1084,3241-3255`) | 无 | **缺失** |
| 关闭按钮 | `Prguse.wil[371]` (`FState.pas:1076-1078`) | 无（N 键切换） | **缺失** |
| 悬停 tooltip | 鼠标移到装备上显示信息 (`FState.pas:3000-3023`) | 无 | **缺失** |

---

## 六、NPC 对话

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 背景 | `Prguse.wil[384]` (`FState.pas:1297-1302`) | `Prguse.wil[384]` + 回退 (`sceneplay.go:1297-1299`) | **正确** |
| 脚本文本 | 富文本解析: `<>` 标签→可点击链接，`<C>` 居中，颜色/下划线 (`FState.pas:4806-4880`) | 纯文本 `\n` 分割 (`sceneplay.go:1301-1307`) | **差异** — 无标签解析 |
| 可点击链接 | `MDlgPoints` TClickPoint 矩形列表 → `SendMerchantDlgSelect` (`FState.pas:5116-5135`) | 无 | **缺失** |
| 关闭按钮 | `Prguse.wil[371]` (`FState.pas:1303-1305`) | 无（任意键关闭） | **缺失** |
| NPC 头像 | MerchantFace 图片 | 无 | **缺失** |

---

## 七、NPC 商店

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 购买面板 | `DMenuDlg` `Prguse.wil[385]`，列表含名称/价格/库存，滚动上/下，购买按钮 (`FState.pas:1323-1341,4887-4967`) | 基础文字列表，名称+价格，ESC 关闭 (`sceneplay.go:1352-1395`) | **差异** — 无 WIL 背景/滚动/按钮 |
| 出售面板 | `DSellDlg` `Prguse.wil[392]`，拖拽物品到出售位，价格显示，OK/Close (`FState.pas:1345-1361,5164-5264`) | 背包文字列表 ShopMode==1 (`sceneplay.go:1380-1391`) | **差异** — 无拖拽出售 |
| 修理模式 | `SpotDlgMode=dmRepair` (`FState.pas:5179-5182,5257`) | ShopMode==2 仅标题 (`sceneplay.go:1362`) | **差异** — 无修理交互 |
| 仓库模式 | `BoStorageMenu` 复用 DMenuDlg "存入/取出" (`FState.pas:4943-4961,5041-5043`) | 无 | **缺失** |
| 物品详情 | `SendGetDetailItem` 子菜单查询 (`FState.pas:5036-5039`) | 无 | **缺失** |
| 购买交互 | 点击物品→点击购买→`SendBuyItem` (`FState.pas:5028-5051`) | 点击物品→`sendBuyItem`（已接线但无 UI 点击区域） | **差异** — 部分接线 |

---

## 八、交易窗口

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 本方面板 | `DDealDlg` `Prguse.wil[389]`，右侧 (`FState.pas:1459-1477`) | 纯色矩形 + "等待双方确认" 占位文字 (`sceneplay.go:1310-1317`) | **差异** — 无 WIL 背景 |
| 对方面板 | `DDealRemoteDlg` `Prguse.wil[390]`，本方左侧 (`FState.pas:1479-1491`) | 无 | **缺失** |
| 本地物品格 | `DDGrid` TDGrid 5×2=10格 36×33px，Items.wil 图标 (`FState.pas:1465-1468,5758-5776`) | 无 | **缺失** |
| 对方物品格 | `DDRGrid` TDGrid 5×4=20格 (`FState.pas:1485-1488,5789-5807`) | 无 | **缺失** |
| 金币输入 | `DDGold` 按钮→数量对话框→`SendChangeDealGold` (`FState.pas:5828-5865`) | 无 | **缺失** |
| 确认/关闭 | `DDealOk` `Prguse.wil[391]` / `DDealClose` `Prguse.wil[64]` (`FState.pas:1469-1474`) | ESC 取消 (`sceneplay.go:761-767`) | **缺失** |
| 物品拖入交易 | 从背包拖到交易格→`SendAddDealItem`；拖回→`SendDelDealItem` (`FState.pas:5722-5756`) | 无 | **缺失** |

---

## 九、行会面板

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 背景 | `Prguse.wil[180]`，全屏左侧 (`FState.pas:1495-1499`) | 纯色矩形 250×200 (`sceneplay.go:1319-1330`) | **差异** — 无 WIL 背景 |
| 功能按钮 (11个) | 主页/列表/聊天/加人/删人/编辑公告/编辑职位/联盟/解盟/宣战/停战 (`FState.pas:1504-1536`) | 无 | **缺失** |
| 成员列表 | 滚动色码列表 `GuildStrs` (`FState.pas:6318-6351`) | 名称+职位文字 (`sceneplay.go:1324-1329`) | **差异** |
| 公告编辑器 | `DGuildEditNotice` 模态框 `Prguse.wil[204]` + TMemo (`FState.pas:6217-6263`) | 无 | **缺失** |
| 职位编辑器 | 模态框 TMemo 成员列表 (`FState.pas:6265-6316`) | 无 | **缺失** |
| 行会聊天 | `BoGuildChat` 模式，`GuildChats` 列表 (`FState.pas:565-566`) | 无 | **缺失** |
| 权限控制 | 按职位显示/隐藏管理按钮 (`FState.pas:6193-6211`) | 无 | **缺失** |

---

## 十、组队面板

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 对话框 | `DGroupDlg` `Prguse.wil[120]`，居中 (`FState.pas:1435-1455`) | 无 | **缺失** |
| 按钮 (5个) | 允许组队/创建/加人/删人/关闭 (`FState.pas:1441-1455`) | 无 | **缺失** |
| 成员列表 | `DGroupDlgDirectPaint` 含 HP 显示 (`FState.pas:404`) | 无 | **缺失** |

---

## 十一、仓库面板

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 网格 | 复用 NPC 商店 DMenuDlg `BoStorageMenu` 模式 (`FState.pas:4943-4961`) | 39格 8列 Items.wil 图标 (`sceneplay.go:1397-1428`) | **正确**（Go 实现更好） |
| 存入交互 | `DSellDlg` dmStorage 模式：拖物品到出售位→OK→`SendStorageItem` (`FState.pas:5255-5258`) | 无 | **缺失** |
| 取出交互 | 点击物品→`SendTakeBackStorageItem` (`FState.pas:5041-5043,5094-5109`) | 网格已渲染但不可点击 | **缺失** |

---

## 十二、完全缺失的面板

| 面板 | Delphi 参考 | 严重度 |
|------|------------|--------|
| 通用对话框 | `DMsgDlg` 3种尺寸: 小 `[381]`/中 `[360]`/大 `[380]`；OK/Yes/No/Cancel 按钮；模态循环；骰子游戏 (`FState.pas:1938-2128`) | **高** |
| 加点面板 | `DAdjustAbility` `Prguse.wil[226]`，9组 +/- 按钮 (DC/MC/SC/AC/MAC/HP/MP/Hit/Speed)，OK/Close (`FState.pas:1557-1619`)；`DBotPlusAbil` 有可加点时闪烁 (`FState.pas:3770-3795`) | **中** |
| 设置/配置 | `DConfigDlg` `Prguse.wil[182]`，声音开关等，OK/Close (`FState.pas:1308-1319`) | 低 |
| 好友/黑名单 | `DFriendDlg` `Prguse.wil[456]`，好友/黑名单标签页，加/删/备忘/邮件/私聊 (`FState.pas:1621-1656`) | 低 |
| 邮件列表 | `DMailListDlg` `Prguse.wil[457]`，回复/阅读/删除/锁定/屏蔽 (`FState.pas:1658-1687`) | 低 |
| 屏蔽列表 | `DBlockListDlg` `Prguse.wil[458]` (`FState.pas:1689-1709`) | 低 |
| 备忘录 | `DMemo` `Prguse.wil[459]` (`FState.pas:1711-1725`) | 低 |
| 查看他人信息 | `DUserState1` `Prguse.wil[370]`，纸娃娃+装备槽（查看其他玩家）(`FState.pas:1088-1163,5872-5899`) | 低 |
| 金币输入框 | `EdDlgEdit` TEdit 用于金币数量输入 (`FState.pas:2086-2095`) | 中 |

---

## 十三、关键交互系统

| 系统 | Delphi 实现 | Go 现状 | 严重度 |
|------|------------|---------|--------|
| 物品拖拽 (`g_boItemMoving`) | 全局状态 + `g_MovingItem`，跨背包/装备/腰带/交易/出售 (`FState.pas:613-621,1811-1854`) | 完全缺失 | **阻断** |
| UI 鼠标命中检测 | `TDWinManager` 路由鼠标到 `TDControl.MouseDown/Click` (`DWinCtl.pas:721-783`) | `OnMouse` 仅处理世界坐标点击 (`sceneplay.go:926-987`) | **阻断** |
| 物品悬停 tooltip | `GetMouseItemInfo` + `DScreen.ShowHint`，500+行属性格式化 (`FState.pas:3935-4135`) | 完全缺失 | 高 |
| 双击使用物品 | `DItemGridDblClick` / `DBelt1DblClick` → `EatItem` (`FState.pas:4602-4641,3902-3920`) | 仅腰带键盘 1-6 (`sceneplay.go:829-836`) | 高 |
| 穿脱装备 | 点击装备槽 → `SendTakeOnItem`/`SendTakeOffItem` (`FState.pas:3257-3400`) | 完全缺失 | 高 |
| 键盘快捷键 | F1-F8 魔法, 1-8 腰带, B 背包等 | B/G/N/M/H/P + F1-F8 + 1-6 (`sceneplay.go:758-836`) | 部分 |

---

## 十四、WIL 资源使用

| WIL 文件 | Delphi 变量 | Go 字段 | Go UI 使用 | 备注 |
|----------|------------|---------|-----------|------|
| `Prguse.wil` | `g_WMainImages` | `resources.Prguse` | ✅ 部分 | 已用: 底栏[1], HP条[4], 按钮[8-11], 背包[3], 装备[370], NPC[384]；缺大量面板背景/按钮 |
| `Prguse2.wil` | `g_WMain2Images` | `resources.Prguse2` | ✅ 仅血条 | Actor 头顶血条背景 (`sceneplay.go:499-504`) |
| `Prguse3.wil` | `g_WMain3Images` | `resources.Prguse3` | ❌ 未使用 | 已加载但 UI 中无引用 |
| `Items.wil` | `g_WBagItemImages` | `resources.Items` | ✅ | 背包/腰带/仓库/地面物品图标 |
| `StateItem.wil` | `g_WStateItemImages` | `resources.StateItem` | ❌ 未使用 | 已加载；装备槽图标需要此文件 |
| `DnItems.wil` | `g_WDnItemImages` | `resources.DnItems` | ✅ | 地面物品渲染 (`sceneplay.go:458-461`) |
| `MagIcon.wil` | `g_WMagIconImages` | `resources.MagIcon` | ✅ | 技能栏图标 (`sceneplay.go:1130-1141`) |

---

## 十五、登录/选角场景（已实现）

| 场景 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 登录 | `DLogIn` + 按钮 `[61-64]` (`FState.pas:757-774`) | `scenelogin.go` Prguse 按钮 | **已实现** |
| 选服 | `DSelServerDlg` `[160]`/`[256]`(英文)，6个服务器按钮 (`FState.pas:778-857`) | `sceneserverselect.go` `[256]+[79]` 按钮 | **已实现** |
| 选角 | `DSelectChr` 全屏，`[66-72]` 按钮 (`FState.pas:900-925`) | `sceneselectchr.go` `[65]` 背景 + 按钮 | **已实现** |
| 创角 | `DCreateChr` `[73]`，职业/性别按钮 (`FState.pas:930-965`) | `sceneselectchr.go` `[73]` + 职业/性别区域 | **已实现** |

---

## 十六、小地图

| 项目 | Delphi | Go | 状态 |
|------|--------|-----|------|
| 渲染 | `mmap.wil` 服务端图，独立窗体 | FBO 碰撞式小地图 (`cmd/client/minimap.go`) | **正确**（Go 自建实现） |
| 切换 | `DBotMiniMap` 按钮 (`FState.pas:1210-1212`) | M 键切换 (`sceneplay.go:798-800`) | **差异** — 无按钮，仅键盘 |

---

## 修复优先级建议

### 阻断级（无此功能则 UI 交互不可用）

1. **UI 鼠标命中检测系统** — 所有面板按钮/格子的点击响应前提
2. **物品拖拽系统** (`g_boItemMoving`) — 背包/装备/腰带/交易/出售的物品移动基础设施

### 高优先级（核心游戏体验）

3. **装备面板完善** — StateItem.wil 图标 + 4页切换 + 穿脱交互
4. **物品 tooltip** — 悬停显示名称/耐久/属性
5. **NPC 脚本标签解析** — `<>` 可点击链接（NPC 功能入口）
6. **通用对话框** — 确认/输入框（丢弃金币、交易确认等依赖）
7. **双击使用物品** — 背包内直接使用药品等

### 中优先级（功能完善）

8. **NPC 商店 UI 完善** — WIL 背景 + 按钮 + 修理交互
9. **交易窗口** — 双方格子 + 金币 + 确认
10. **行会面板** — 成员列表 + 功能按钮
11. **底部 HUD 补全** — 经验条/重量条/9个底部按钮
12. **仓库取出交互** — 点击取出物品

### 低优先级（锦上添花）

13. 组队面板、加点面板、好友/邮件、设置、查看他人

---

## 实施进度（2026-07-27，计划 1785107348995-playful-falcon.md）

固定 800×600（Delphi SWH800 分支），UI 全部迁入 DWinCtl 语义控件树
（`cmd/client/ui*.go`：uicontrol/uimanager 框架 + uihud/uibag/uistate/uinpc/
uideal/uiguild/uiabil 面板），旧手绘面板已删除。

| 审查报告条目 | 状态 | 落点 |
|---|---|---|
| UI 鼠标命中检测系统（阻断） | ✅ 完成 | uicontrol.go/uimanager.go（模态/捕获短路、像素级 alpha 命中、按钮"释放时在区域内才点击"、网格 down==up 选中、窗口拖动钳制） |
| 物品拖拽系统（阻断） | ✅ 完成 | itemmove.go（Delphi 符号 Index 编码、光标持物渲染、取消归位、背景落地/丢金币） |
| 底部 HUD（差异+缺失） | ✅ 完成 | uihud.go：底栏两段混合、HP/MP 裁剪球（战士<28 级 [5]+[6]）、经验/重量条 [7]、昼夜图标、9 功能按钮+悬停提示、4 状态钮（按压态绘制）、腰带回填（拾放/双击使用/tooltip）、聊天 9×12 滚动+点击私聊 |
| 背包（缺失交互） | ✅ 完成 | uibag.go：固定 46 格指针槽、拾放/交换、双击使用/自动穿戴、tooltip、关闭钮；物品图标改走 def.Looks（登录 SMStdItems 同步物品库） |
| 装备面板 4 页（缺失） | ✅ 完成 | uistate.go：纸娃娃（376/377+发型+StateItem 层，HotX/HotY 偏移，St<N>.wil 懒加载）、13 槽精确坐标+穿脱+tooltip、属性/详细属性/魔法页、键位对话框→CMMagicKeyChange |
| NPC 对话富文本（差异） | ✅ 完成 | uinpc.go：`<文本/链接>` 标签解析、黄色下划线链接（按下变红）、5s 防连点、`@@` 输入提示、CMMerchantDlgSelect 按标签；服务端按标签路由（@buy/@sell/@repair/脚本 label 跳转），#SAY 行以 `\` 连接 |
| 商店/出售/修理（差异） | ✅ 完成 | uinpc.go：DMenuDlg [385] 列表/滚动/购买、DSellDlg [392] 出售位拖放+500ms 询价泵+修理/寄存模式 |
| 交易窗口（差异+缺失） | ✅ 完成 | uideal.go + 服务端 tradesystem.go 重写：双方面板 [389]/[390]、本方 5×2/对端 5×4 格（物品 body）、金币输入、确认锁、4s 节流；修复 CMDealDelItem 误路由 |
| 行会面板（差异+缺失） | ✅ 完成 | uiguild.go：[180]+12 按钮（8 个按权限门控）、成员列表（name/rank/online）、公告/职位/结盟输入；服务端 CMOpenGuildDlg 修复（原误路由建会）、按需成员列表、@@buildguildnow 建会 |
| 组队面板（缺失） | ✅ 完成 | uiguild.go：[120]+5 按钮、允许组队开关、双列成员列表；服务端 CMGroupMode/Add/Del + 全员名单广播 |
| 仓库（缺失交互） | ✅ 完成 | 脚本 STORAGE 动作→SMSendUserStorageItem+SMSaveItemList；客户端复用商店面板仓库模式（列表取出+出售位寄存，MakeIndex 寻址） |
| 通用对话框（缺失） | ✅ 完成 | uidialog.go：DMsgDlg 三尺寸、Ok/Yes/No/Cancel 右起排布、Enter/Esc 语义、输入模式（非阻塞回调版）；uiedit.go EditBox |
| tooltip（缺失） | ✅ 完成 | uitooltip.go：ShowHint/DrawHint（[394] 背景、`\` 多行）、GetMouseItemInfo |
| 加点面板（缺失） | ✅ 完成 | uiabil.go：[226] 9 组 +/-、Ok→CMAdjustBonus（9×u16）；服务端升级 +3 点、HandleAdjustBonus |
| 查看他人（缺失） | ✅ 完成 | uiabil.go：右键查询→CMQueryUserState→DUserState1 [370] 只读装备视图 |
| 数据基础 | ✅ 完成 | SMAbility 全字段 body（职业公式 MaxHP/MaxWeight 等）、HealthSpell 打包修复（原 MP 被读成 HP）、SMWeightChanged、魔法列表扩展（图标/训练值/名称） |

### 仍未做（登记）

- 好友/邮件/黑名单/备忘录（服务端子系统缺失）
- 声音系统（DOption 仅聊天行占位）
- 饥饿指示（服务端无状态，控件预留）、动态昼夜（固定发 3）
- 商品详情子菜单（CMUserGetDetailItem）、行会宣战/停战、行会主页传送
- 多行 Memo 控件（公告/职位编辑暂用单行输入）、Ctrl×10 加点步进
- DBotMemo 坐标越界（待查 DFM 父归属）
