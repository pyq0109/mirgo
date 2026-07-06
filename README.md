# mirgo

# 准备

1. 新建 asset 目录
2. 下载 客户端美术资源 解压重命名为 client
3. 下载 服务端配置文件 重命名为 server

目录结构:

```
asset/
├── client/                 # 客户端美术资源
├── server/                 # 服务端配置文件
└── ...
```

# 编译运行 mapviewer

## 依赖

- Go 1.26
- GCC (MinGW-w64) — Fyne 框架需要 CGO

## 编译

```bash
$env:CGO_ENABLED=1
go build ./cmd/mapviewer
```

## 运行

```bash
# WIL 资源默认从 asset/client/Data/ 加载
.\mapviewer.exe ".\asset\client\Map\0.map"

# 指定 WIL 资源目录
.\mapviewer.exe ".\asset\server\Map\0102.map" ".\asset\client\Data"
```

操作：鼠标拖拽平移、滚轮缩放、左键查看格子、右侧面板切换图层

# 资源

- 游戏 delphi 源码参考: https://github.com/lzxsz/MIR2 (commit: 98711dad31567d9a7e272956f6c5a2487000848b)
- 服务端配置文件: https://github.com/cjlaaa/Mir2-GeeM2 (commit: 26b2881ae2e8aca0aac0ab58acbfca9c39dbfc9c)
- 客户端美术资源: [热血传奇十周年硬盘版.rar (提取码: ussz)](https://pan.baidu.com/s/1Fo4rnHku8EFRXDUcE-incw?pwd=ussz)
