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
├── wil/             # .wil/.wix 图像加载器
└── log/             # 分级日志
asset/               # 已 gitignore — 游戏资源
serverconfig/        # 已 gitignore — 转换后的配置文件
```

## 当前状态

端到端登录流程可跑通：登录→选服→门动画→选角（含创建/删除）→公告→进游戏。
P0 基础正确性修复已完成（WIL/协议/地图碰撞/门系统）。下一步：Phase 6（角色可动）。
详见 **`doc/客户端服务端开发计划.md`**。

## 约束

- `go.sum` 已 gitignore — 添加依赖后需运行 `go mod tidy`
- 无 CI、linter；测试用 `go test ./...`
- `asset/` 目录禁止提交（含二进制和大文件）
- 服务端单端口（默认7000）统一处理登录/选角/游戏消息
- 客户端不使用 ImGui（自建渲染），查看器工具使用 ImGui

## 资源目录（已 gitignore — 需手动准备）

| 目录 | 来源 | 用途 |
|------|------|------|
| `asset/client/` | 热血传奇十周年硬盘版 | 客户端美术资源（WIL/地图） |
| `asset/server/` | `github.com/cjlaaa/Mir2-GeeM2` | 服务端配置 |
| `asset/delphi/` | `github.com/lzxsz/MIR2` (commit `98711da`) | 原始 Delphi 源码（主要参考） |

## Delphi 源码参考

### 关键文件

| 文件 | 内容 |
|------|------|
| `Common/Grobal2.pas` | 消息定义、数据结构（2739行） |
| `Common/EDcode.pas` | 6Bit 编解码算法 |
| `Client/Actor.pas` | 角色基类、动画模板、渲染（3944行） |
| `Client/PlayScn.pas` | 地图渲染、光照、对象管理（2366行） |
| `Client/ClMain.pas` | 主窗体、网络消息处理 |
| `Client/IntroScn.pas` | 场景状态机、登录/选角场景 |
| `Client/MapUnit.pas` | 地图加载、碰撞、门逻辑 |
| `Client/WIL.pas` + `wmUtil.pas` | WIL 文件加载器 |
| `Client/MShare.pas` | 客户端全局变量 |
| `Client/ClFunc.pas` | 背包/方向/装备工具函数 |
| `M2Server/ObjBase.pas` | 游戏对象基类（26821行） |
| `M2Server/Envir.pas` | 地图管理、碰撞、视野 |
| `M2Server/UsrEngn.pas` | 用户引擎、游戏循环 |
| `M2Server/ObjNpc.pas` | NPC脚本引擎（11556行） |
| `M2Server/Magic.pas` | 魔法系统 |

### 核心数据结构

- **TDefaultMessage** (12字节): Recog(int32) + Ident/Param/Tag/Series(uint16×4)
- **TStdItem** (60字节): 物品定义
- **TUserItem** (24字节): 物品实例
- **TAbility** (50字节): 角色属性（注意：WearWeight 等为 Word 非 Byte）
- **TMapInfo** (12字节): 地图格子（wBkImg/wMidImg/wFrImg + 门/动画/区域/光照）

### 网络协议

- 编码：6Bit（每3字节→4字符，偏移0x3C），OLDMODE（无XOR）
- 帧格式：客户端 `#<code><payload>!`，服务端 `#<payload>!`
- RunLogin：`**loginID/charName/cert/version/code`（原始字符串编码）

### WIL 文件格式

- 头部：Title(41字节 String[40]) + ImageCount + ColorCount + PaletteSize + VerFlag
- VerFlag=0 → 旧格式（8字节图像头），VerFlag≠0 → 新格式（12字节图像头）
- 像素：8位调色板或 16位 RGB565（ILib 格式无行填充）
- 扫描线：底到顶存储，加载时翻转
- 索引：.wix 文件，头部同为 String[40] + IndexCount + VerFlag

### 动画系统

- 帧公式：`实际帧 = start + 方向 × (frame + skip) + 当前帧偏移`
- 人类 HA 模板：14 种动作，每套装备 600 帧
- 怪物 MA9~MA47：39 种模板，280/360/440 帧
- WORDER 表：600×2 字节，控制武器在身体前/后绘制
- 只有 Stand 循环，其他动作播放一次后回到 DefaultMotion

### 地图渲染管线（Delphi PlayScn.pas）

1. 背景层（Tiles.wil，stride-2）
2. 中间层（SmTiles.wil，逐格）
3. 前景小物体 + 角色 + 地面物品 **按 Y 坐标交替渲染**（深度排序）
4. 前景大物体（高于格子的）
5. 光照/迷雾叠加（lig0a~f.dat，6级光罩）
6. 小地图

### 服务端架构（Delphi 8进程 → Go 单进程）

Delphi 原版：LoginGate → LoginSrv → SelGate → DBServer → RunGate → M2Server
Go 简化：单端口统一监听，内部路由登录/选角/游戏消息

## 编译运行

```bash
# 客户端
go build -o client.exe ./cmd/client
./client.exe -data asset/client/Data -maps asset/client/Map -server localhost:7000

# 服务端
go build -o server.exe ./cmd/server
./server.exe -config serverconfig -maps asset/server/Map

# 配置转换
go run ./cmd/serverconfig -v

# 测试
go test ./internal/protocol/ ./cmd/client/ -count=1
```
