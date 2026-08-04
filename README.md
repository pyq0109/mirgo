# mirgo

用 Go 语言重新实现热血传奇客户端和服务端

目前处在非常早期的开发阶段, 功能很不完善存在大量 bug

讨论 QQ 群: 32309474

## 资源

- Delphi 源码参考: https://github.com/lzxsz/MIR2 (commit 98711dad31567d9a7e272956f6c5a2487000848b)
- 服务端配置: https://github.com/cjlaaa/Mir2-GeeM2 (commit 26b2881ae2e8aca0aac0ab58acbfca9c39dbfc9c)
- 客户端美术资源: [热血传奇十周年硬盘版.rar (提取码: ussz)](https://pan.baidu.com/s/1Fo4rnHku8EFRXDUcE-incw?pwd=ussz)

注意, 这个客户端美术资源也是我从网上收集到的, **会包含病毒程序, 请注意杀毒**. 本项目只使用美术资源部分

客户端美术资源解压放入 `asset/client/`，服务端配置放入 `asset/server/`。

## 编译运行

依赖 Go 1.26 + CGO（C 编译器）:

```bash
# Linux (Debian/Ubuntu) 安装系统依赖
sudo apt install gcc g++ pkg-config libasound2-dev libgl1-mesa-dev xorg-dev
# Windows 安装 MinGW-w64 后启用 CGO
$env:CGO_ENABLED=1
```

```bash
go mod tidy && go mod vendor

go run ./cmd/serverconfig -v                            # 转换服务端配置
go run ./cmd/server                                     # 服务端
go run ./cmd/client                                     # 客户端（Windows 省略 -tags x11）
go run ./cmd/mapviewer ./asset/client/Map/0.map         # 地图查看器
go run ./cmd/wilviewer ./asset/client/Data              # WIL 资源查看器
```

## 开源协议

MIT
