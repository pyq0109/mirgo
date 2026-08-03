# AGENTS.md

## 项目

用 Go 语言重新实现热血传奇（MIR2）客户端和服务端。
Module: `github.com/pyq0109/mirgo`

## 架构

```
cmd/
├── client/          # 游戏客户端（OpenGL 3.3 + GLFW）
├── server/          # 游戏服务端（TCP，单端口统一监听）
├── serverconfig/    # 配置转换工具（Delphi → JSONC）
├── mapviewer/       # 地图查看器（OpenGL + ImGui）
└── wilviewer/       # WIL资源查看器（OpenGL + ImGui）
internal/
├── protocol/        # 共享协议层（6Bit编解码、消息常量、数据结构）
├── engine/          # 共享渲染引擎（窗口、GL状态、相机、文字、场景、资源管理）
├── netserver/       # TCP 服务端库
├── storage/         # SQLite 数据存储层
├── mapformat/       # .map 文件解析器
├── wil/             # .wil/.wix 图像加载器（懒加载）
└── log/             # 分级日志
asset/               # 已 gitignore — 游戏资源
serverconfig/        # 已 gitignore — 转换后的配置文件
```

## 约束

- `go.sum` 已 gitignore — 添加依赖后需运行 `go mod tidy`
- 无 CI、linter；测试用 `go test ./...`
- `asset/` 目录禁止提交（含二进制和大文件）
- 服务端单端口（默认7000）统一处理登录/选角/游戏消息
- 客户端不使用 ImGui（自建渲染），查看器工具使用 ImGui
- WIL 懒加载：`Load()` 只读索引，`GetImage(idx)` 按需解码，`Close()` 关闭句柄
- 服务端 `Send()` 不编码 body，调用方自行 `protocol.EncodeString()`/`EncodeBuffer()`

## 资源目录（已 gitignore — 需手动准备）

| 目录 | 来源 | 用途 |
|------|------|------|
| `asset/client/` | 热血传奇十周年硬盘版 | 客户端美术资源（WIL/地图） |
| `asset/server/` | `github.com/cjlaaa/Mir2-GeeM2` | 服务端配置 |
| `asset/delphi/` | `github.com/lzxsz/MIR2` (commit `98711da`) | 原始 Delphi 源码（主要参考） |

## 服务端文件结构

```
cmd/server/
├── main.go           # 入口、消息路由、登录流程、tick循环
├── config.go         # server.jsonc 配置加载
├── baseobject.go     # 基础对象、RM_*常量、SendRefMsg、WalkTo
├── playobject.go     # 玩家：移动/战斗/视野/地图切换/消息分发
├── monsterobject.go  # 怪物：AI(搜索/追击/攻击/游荡)
├── monsterai.go      # 怪物AI行为(34种:AIMelee=0..AITrainer=33)
├── monsterdb.go      # 怪物数据库加载(monster_db.jsonc)
├── npcobject.go      # NPC：固定位置、外观
├── mongen.go         # 刷怪系统、地面物品消失
├── drops.go          # 怪物掉落逻辑
├── droptable.go      # 掉落表加载(MonItems/*.jsonc)
├── envir.go          # 地图环境、碰撞、门、对象管理
├── mapevent.go       # 地图事件(火墙持续伤害/SM_SHOWEVENT广播)
├── mapmanager.go     # 地图加载、传送路由
├── usrengine.go      # 用户引擎、tick处理
├── session.go        # 会话管理
├── doors.go          # 门自动关闭
├── itemdb.go         # 物品数据库加载(std_items.jsonc)
├── itemsystem.go     # 背包/穿脱/RecalcAbilitys/使用物品
├── magicdb.go        # 魔法数据库加载(magic_db.jsonc)
├── magicsystem.go    # 施法/三职业技能/伤害
├── npcscript.go      # NPC脚本引擎([@label]解析)
├── chatsystem.go     # 聊天广播/组队
├── pksystem.go       # PK点数/红名/衰减
├── tradesystem.go    # 面对面交易
├── guildsystem.go    # 行会创建/聊天
├── merchantsystem.go # NPC商店买/卖/修理/价格查询
├── attackmode.go     # 攻击模式(全体/组队/和平等)
├── safezone.go       # 安全区配置(start_points.jsonc)
├── statuseffect.go   # 状态效果(毒/隐身/石化等12种)
├── storagesystem.go  # 仓库存取(39格)
└── gmcommands.go     # GM命令(@make/@level/@move/@mob等)
```

## 客户端文件结构

```
cmd/client/
├── main.go           # 入口、NetHandler、全部SM消息分发
├── gamestate.go      # 全局状态(MySelf/背包/装备/魔法/UI状态)
├── sceneplay.go      # 游戏场景：3层地图+Y-sort角色+UI+输入
├── actorbase.go      # Actor：消息队列/动画/Shift插值/多层渲染
├── actormanager.go   # Actor管理：注册/Y排序/Feature解析
├── actor.go          # 动画模板(HA 14动作/MA 39模板)/CalcFrame
├── worder.go         # 武器前后层规则
├── minimap.go        # 碰撞式小地图(FBO)
├── lighting.go       # 光照/迷雾(6级光罩+光源)
├── magiceffect.go    # 魔法特效管理(爆炸/飞行/地面)
├── eventman.go       # 地图事件渲染(SM_SHOWEVENT/火墙)
├── scenelogin.go     # 登录/注册场景(含选服对话框模式 modeServerSelect)
├── sceneselectchr.go # 选角/创角/删角场景
└── scenenotice.go    # 公告场景
```

## Delphi 源码参考

| 文件 | 内容 |
|------|------|
| `Common/Grobal2.pas` | 消息定义、数据结构（2739行） |
| `Common/EDcode.pas` | 6Bit 编解码算法 |
| `Client/Actor.pas` | 角色基类、动画模板、渲染（3944行） |
| `Client/PlayScn.pas` | 地图渲染、光照、对象管理（2366行） |
| `Client/ClMain.pas` | 主窗体、网络消息处理、本地预测 |
| `M2Server/ObjBase.pas` | 游戏对象基类（26821行） |
| `M2Server/ObjNpc.pas` | NPC脚本引擎（11556行） |
| `M2Server/Magic.pas` | 魔法系统（1560行） |
| `M2Server/Guild.pas` | 行会系统（1319行） |

## 核心协议

- 编码：6Bit（每3字节→4字符，偏移0x3C），OLDMODE（无XOR）
- 帧格式：客户端 `#<code><payload>!`，服务端 `#<payload>!`
- 控制消息：`#+GOOD!`（确认）、`#+FAIL!`（回滚）— 不经6Bit编码
- RunLogin：`**loginID/charName/cert/version/code`

## 编译运行

```bash
# 客户端（Linux 需 -tags x11 跳过 Wayland 编译；Windows 加上也无害）
go build -tags x11 -o client.exe ./cmd/client
./client.exe -data asset/client/Data -maps asset/client/Map -server localhost:7000

# 服务端（不依赖 GLFW，无需 tag）
go build -o server.exe ./cmd/server
./server.exe -config serverconfig -maps asset/server/Map

# 查看器工具（同客户端，Linux 需 -tags x11）
go build -tags x11 -o mapviewer.exe ./cmd/mapviewer
go build -tags x11 -o wilviewer.exe ./cmd/wilviewer

# 配置转换
go run ./cmd/serverconfig -v

# 测试
go test ./... -count=1
```
