# AGENTS.md

## 项目

用 Go 语言重新实现热血传奇（MIR2）客户端和服务端。
Module: `github.com/pyq0109/mirgo`

## 架构

```
main.go              # 入口（占位）
cmd/
└── mapviewer/       # 地图查看器（Fyne UI）
internal/
├── mapformat/       # .map 文件解析器
├── wil/             # .wil/.wix 图像加载器
└── renderer/        # 软件渲染器（三层渲染 + 相机 + 小地图）
asset/               # 已 gitignore — 游戏资源，非 Go 代码
```

## 地图查看器

编译运行参见 README.md 的"编译运行 mapviewer"章节。

## 资源目录（已 gitignore — 需手动准备）

`asset/` 不纳入版本管理，需手动下载填充：

1. `asset/client/` — 客户端美术资源，来自热血传奇十周年硬盘版
2. `asset/server/` — 服务端配置，来自 `github.com/cjlaaa/Mir2-GeeM2`
3. `asset/delphi/` — 原始 Delphi 源码，来自 `github.com/lzxsz/MIR2`（commit `98711da`）

Delphi 源码是 Go 重写的主要参考。
服务端关键组件：`M2Server/`、`DBServer/`、`LoginSrv/`、`LoginGate/`、`RunGate/`、`SelGate/`。
客户端关键组件：`Client/`、`MirClient/`。

## 约束

- `go.sum` 已 gitignore — 添加依赖后需运行 `go mod tidy`
- 尚无 CI、linter 或测试基础设施
- 无 Makefile、Dockerfile 或构建脚本
- `asset/` 目录禁止提交（含二进制和大文件）
