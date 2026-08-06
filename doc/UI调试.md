# UI 调试指南

本文档覆盖客户端 UI 调试的全部手段：调试控制台、可视化 overlay、一致性审计、自动化测试，以及按症状分类的调试流程。

**核心思路：UI 问题的判定尽量机器化，不靠人眼盯。**

| 层 | 回答的问题 | 手段 | 判定时机 |
|----|-----------|------|---------|
| 对照答案 | 数值抄对了吗？ | Delphi 真值对照测试（uidelphi_test.go） | `go test` |
| 对照行为 | 点得到/点得准吗？ | 命中断言测试 + `ui hit` / `click` 模拟 | `go test` / 控制台 |
| 对照自身 | 两套数据有矛盾吗？布局变了吗？ | `ui audit` 审计 + 布局快照测试 | 控制台 / `go test` |

人眼只用于最初对问题分类一次；验证与防回归全部由机器完成。

命令集刻意保持精简：只保留"观察 → 审计 → 模拟操作"工作流必需的命令。

---

## 一、调试控制台

按 `` ` ``（反引号键）打开/关闭。控制台跨场景存在，命令动态注册（PlayScene 进入时注册 `panel`/`key`，离开时注销）。

- 输入命令直接回车执行；`↑`/`↓` 浏览历史；`Tab` 补全命令名
- 输出区支持鼠标选择文本（Ctrl+C 复制）、拖拽调节面板高度
- `help` 列出当前场景下所有可用命令
- **UI 调试命令自动落日志**：`ui`/`click`/`panel`/`key` 的每次调用（命令行 `> ...`）与全部输出，除显示在控制台外，还按行写入日志（tag `UIDebug`，stderr），便于事后回溯与粘贴归档

| 命令 | 说明 |
|------|------|
| `help` | 列出所有命令 |
| `clear` | 清空输出 |
| `click <x> <y> [right]` | 按坐标模拟点击 |

---

## 二、UI 调试命令（`ui` 命令族）

UI 控件是一棵树：根节点 `DBackground`（全屏），下挂各面板（`DBottom` 底栏、`DItemBag` 背包、`DStateWin` 状态等）。以下命令全部作用于当前场景的控件树（`gActiveUI`）。

**所有场景统一用这一套命令**：登录 / 选角 / 公告 / 游戏四个场景都用 UIManager 构建控件树并在进入时挂到 `gActiveUI`，`ui`/`click` 命令与 `ui bounds` overlay 在哪个场景都同样工作（公告场景的 DMessageDlg 也是真控件树，可审计、可模拟点击）。

### 观察类

| 命令 | 说明 |
|------|------|
| `ui tree [深度]` | 控件树 dump：名字、类型、**绝对坐标**、尺寸、可见性（默认深度 3） |
| `ui inspect <名字>` | 单个控件完整属性 dump（含 grid 单元格、回调列表） |
| `ui hit` | **当前鼠标位置**的命中链（自底向上）+ 路由状态——排查"这个点被谁吃了" |
| `ui state` | 模态/捕获/焦点状态 + 可见控件统计 |
| `ui bounds` | 切换包围盒 overlay（见第三节） |
| `ui audit [名字\|err]` | 一致性审计清单（可过滤，见第四节） |

### 操作类

| 命令 | 说明 |
|------|------|
| `ui move <名字> <x> <y>` | 实时挪动控件，现场试位置 |
| `ui click <名字>` | 按名字模拟点击（命中控件中心） |
| `ui events` | 切换 UI 输入事件日志（每次 down/up/click/move 都记录——排查"事件被谁吃掉"） |

---

## 三、可视化 overlay：一眼看到错位

### `ui bounds` 包围盒

对所有可见控件画类型着色的命中矩形：

- **绿**=button，**蓝**=window，**黄**=grid，**灰**=普通 control
- 名字密度自动管理：默认仅悬停控件及其祖先显示名字（避免标签成团）

命中框与图片是否一致不由 overlay 判定，交给 `ui audit` 的 `size-override`/`img-offset` 规则机器化给出（见第四节）。

---

## 四、一致性审计 `ui audit`

命中矩形（Left/Top/Width/Height）与绘制图素（WLib/FaceIndex+图片尺寸/HotX/HotY）是两套独立数据，仅靠 `SetImgIndex` 弱同步——这是大多数 UI bug 的结构性根源。审计自动对照两套数据与布局常识列出问题清单（uiaudit.go）：

```
ui audit            # 全量清单（error 优先）
ui audit DMerchant  # 按路径子串过滤
ui audit err        # 只看 error 级
```

### 审计规则

| 规则 | 检查内容 | 级别 | 对应 bug 类型 |
|------|---------|------|--------------|
| `size-override` | 命中框尺寸 ≠ 图片宽高（SetImgIndex 后手改过） | warn | **按钮图与点击范围不符（主根源）** |
| `img-offset` | 图片 HotX/HotY ≠ 0（默认绘制忽略偏移、自定义绘制叠加，语义分裂） | warn | 同上 |
| `img-missing` | WLib 已设但取图为 nil 且无自绘 | error | 空气按钮 |
| `outside-parent` | 矩形越出父控件边界 | warn | **面板子控件错位** |
| `offscreen` | 矩形完全/部分出屏（800×600） | warn/info | 同上 |
| `sibling-overlap` | 同级两个可见按钮矩形相交 | warn | 点击歧义 |

### 构建期自动审计（UIAudit 日志）

各场景建树完成后自动跑一次审计，结果写入日志（tag `UIAudit`，error→Error 级、warn→Warn 级）——启动客户端即可在 stderr 看到，无需手动操作。

### 白名单（auditWhitelist）

已知**有意**偏离的条目登记进 `cmd/client/uiaudit.go` 的 `auditWhitelist`，key 为 `DebugPath|规则`，value 为原因。命中后降级为 info 并附原因，使 `ui audit` 输出收敛到真问题：

```go
var auditWhitelist = map[string]string{
    "DBackground>DItemBag>DItemGrid|size-override": "点击区有意裁剪为 286×162 (FState:1171-1174)",
}
```

---

## 五、按症状调试的流程

### 症状 A：按钮图片与可点击范围不符

1. `ui audit <控件名>` —— `size-override` / `img-offset` 直接指出是哪种失配
2. 定位代码：
   - `size-override` → 找 `SetImgIndex` 之后手工赋 Width/Height 的地方（`grep <控件名> cmd/client/`）
   - `img-offset` → 核对该控件的绘制路径：默认 BlitImage 忽略偏移，自定义 OnDirectPaint 是否叠加了 HotX/HotY，两边语义必须统一
3. 修复后：在 `uilayout_test.go` 补一条命中断言固化（见第七节）

### 症状 B：面板上的子控件位置不对

1. `ui tree 4` / `ui inspect <控件名>` —— 看**实际生效**的绝对坐标（注意运行时覆盖：不少控件建树后被代码重新定位，DlgConf 静态值不代表最终位置）
2. `ui audit <面板名>` —— `outside-parent` / `offscreen` 直接指出越界控件
3. 对照 Delphi 真值：静态布局值是否正确由 `TestDlgConfMatchesDelphi` 保证（`go test -run TestDlgConfMatchesDelphi ./cmd/client/`）；若测试绿而位置仍错，问题在运行时覆盖代码（对照注释里的 FState.pas 行号）
4. `ui move <控件名> <x> <y>` 现场试出正确值，再改代码

### 症状 C：点击没反应 / 点到了错误的控件

1. `ui events` 打开事件日志，再点一次——看事件进了哪个控件、在哪一步断掉
2. 鼠标移到目标位置，`ui hit` —— 命中链自底向上列出，`topmost` 就是实际吃到点击的控件
3. 常见原因：
   - 被兄弟控件遮挡（`ui audit` 的 `sibling-overlap`）
   - 模态窗口拦截（`ui state` 看 modal）
   - 命中矩形与视觉位置不符（走症状 A 流程）
4. `click <x> <y>` 模拟点击复现，无需反复手点

### 完整示例：调试 NPC 对话框

```text
`                    # 打开控制台
panel npc on         # 强制打开 NPC 对话框（不依赖服务端/NPC）
ui audit DMerchant   # 审计该面板子树
ui bounds            # 看命中框
ui hit               # 鼠标移到可疑位置查命中链
ui inspect DMerchantDlgClose   # 看关闭按钮运行时覆盖后的实际值
ui click DMerchantDlgClose     # 模拟点击验证
```

注意 NPC 面板有三处**树外手写命中**（`ui bounds` 画不出来，是 bug 高发区）：富文本链接命中（uinpc.go 链接注册/命中段）、商品行点击 menuRowClick、对话文本滚动区（sceneplay.go）。调试它们用 `click <x> <y>` + `ui events` 观察行为。

---

## 六、场景专有命令（PlayScene）

| 命令 | 说明 |
|------|------|
| `panel <名字> [on\|off]` | 强制开关面板：bag state guild group friend abil npc shop deal minimap（**调试面板 UI 的入口，不依赖服务端**） |
| `key <名字>` | 模拟快捷键：b c e g m n s v w enter esc f1-f12 1-6 |

---

## 七、自动化测试系统（headless，无需 GL/资源）

全部位于 `cmd/client/`，`go test ./cmd/client/ -count=1` 运行。

### 1. 布局快照（uilayout_test.go + testdata/*.snap）

PlayScene / LoginScene / SelectChrScene 三棵控件树以 nil 资源 headless 构建，dump `DebugTree`（名字+类型+绝对矩形+可见性）存入快照文件，防结构与坐标回归。

- 故意调整布局后：`go test ./cmd/client/ -update` 重生成基线（提交时说明原因）
- 快照记录的是**回退尺寸布局**（无资源时按钮走默认 80×24）；真实资源下的尺寸问题由 `ui audit` 覆盖，两者互补

### 2. 命中断言（uilayout_test.go）

机器判定"点得到/点得准"：

- 每个无重叠按钮的中心点必须命中自己（逐面板验证，与实际"一次开一个面板"一致）
- 腰带相邻格间隙中点不得命中任何控件
- 背包网格裁剪区（286×162）外不得命中
- 空白区不得命中

修复点击类 bug 时的标准动作：**先加一条失败断言，再修代码到绿**——同一个 bug 第二次出现会被测试直接拦住。

### 3. Delphi 真值对照（uidelphi_test.go）

解析 `asset/delphi/Client/MShare.pas` 的 `DlgConf:TConfig=(...)` 原版真值（含 `219+30*2`、`SCREENWIDTH div 2 + (...)` 表达式求值），与 Go 的 `DlgConf`（uiconst.go）逐条比对 Image/Left/Top/W/H。

- asset/delphi 不存在时自动 skip（无资源环境可跑）
- 不一致 → 测试失败并输出 go/delphi 双方数值
- 沿用 protocol_coverage_test.go 的"对照+豁免表"模式

---

## 八、故意不按 Delphi 值调整 UI 怎么办

机器检查不是"必须和 Delphi 一样"，而是一道**确认门**：改动触发测试红 → 确认是故意的 → 登记一行原因 → 绿。每次有意偏离都留痕，下次看到异常能立刻分清"故意的"还是"改坏了"。

| 机制 | 故意偏离时怎么做 |
|------|----------------|
| Delphi 对照测试 | 在 `uidelphi_test.go` 的 `delphiExempt` 加一行 `"控件名": "原因"` |
| 布局快照 | 改完跑 `go test ./cmd/client/ -update`，新布局成为新基线 |
| `ui audit` | 有意项登记进 `uiaudit.go` 的 `auditWhitelist` |

---

## 九、扩展调试功能

### 注册新的控制台命令

场景内调用 `dc.Register(name, help, fn)`（参考 debugplay.go registerDebugCmds），离开场景时 Unregister。全局命令在 NewDebugConsole 注册。

### 新增审计规则

在 `uiaudit.go` 的 `auditControl` 中用 `add(规则名, 级别, 消息)` 追加判断即可，白名单机制自动生效。

### 相关文件索引

| 文件 | 内容 |
|------|------|
| cmd/client/debugconsole.go | 控制台本体、通用命令（help/clear/ui/click） |
| cmd/client/debugplay.go | PlayScene 专有命令（panel/key） |
| cmd/client/uiaudit.go | 一致性审计 + 白名单 |
| cmd/client/uimanager.go | 控件树路由/绘制、Debug* 函数、bounds overlay |
| cmd/client/uicontrol.go | 控件定义、InRange 命中、SetImgIndex |
| cmd/client/uidelphi_test.go | Delphi 真值对照 + 豁免表 |
| cmd/client/uilayout_test.go | 布局快照 + 命中断言 |
| cmd/client/testdata/*.snap | 三场景布局基线 |
