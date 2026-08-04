# client

- [ ] ScenePlay 场景的 UI 问题
- [ ] SceneSelectChr 场景的 UI 微调
- [x] 攻击特效 (攻杀剑术) 的动画播放问题
- [ ] 声音
- [x] WIL 解析问题, DnItems.wil 解析错误
- [x] 人物男女角色渲染问题
- [x] NPC 不能点
- [x] 支持更大更多的分辨率 ALT + 回车 切换, 或者 res 1/2/3/4 切换
- [x] 小地图显示错误
- [x] 被攻击后的动画朝向问题
- [x] 走路卡脚问题

# server

- [x] 所有服务端的设置都通过读取 serverconfig 目录下的 jsonc 配置文件来设置
- [x] config.go 使用 jsonc 解析器 (支持注释), 而不是 json 解析器
- [x] 登录注册逻辑是否正确
- [x] 玩家账号密码检查, 玩家名字检查, 是否合规

# wilviewer

- [x] 左侧 tree 文件列表高亮显示当前打开的是哪个 wil 文件
- [x] 右下角 Preview 窗口支持鼠标滚轮上下滚能缩放图片大小, 并且按住滚轮可以左右拖动图片, 方便观察
- [ ] 三个列表可以随意拖动大小高宽
- [x] 动画播放模式是否应该去掉?
- [x] wilviewer 目录扁平化

# mapviewer

- [x] mapviewer 目录扁平化

# serverconfig

- [ ] 检查 serverconfig 转换出来的配置文件与 asset/server 是否完全一致
- [ ] serverdata 目录取消, 让 mir2.db 与 server 同级
- [ ] 压缩优化 serverconfig 目录结构, 让配置更密集, 更合理
- [ ] serverconfig 文件名去掉 "\_"

# 综合

- [x] 封包拆包逻辑是否和 Delphi 完全一致
- [x] doc 目录文档整理

# 架构优化（按建议顺序实施）

安全网先行：

- [ ] 协议覆盖测试：扫描 internal/protocol 全部 CM\_/SM\_ 常量，断言每个已注册或显式标记 unimplemented（防止再出现 7 个 CM 路由死代码这类问题）
- [ ] 引入 golangci-lint（govet/unused/staticcheck）+ CI（go build -tags x11 && go test -tags x11 ./...）
- [ ] 恢复提交 go.sum（当前被 gitignore）

消息注册制：

- [ ] 服务端 MessageRouter 注册表：各系统注册 CM 处理器，main.go handleGameMessage 只查表分发，消除 main.go 与 playobject.go ProcessMessage 两处人肉同步
- [ ] 客户端 main.go（2640 行）拆分：NetHandler（连接/帧）与 SM 分发分离，SM handler 按域分文件（战斗/物品/社交/系统）注册进分发表

持久化与配置：

- [ ] main.go 中抽出 CharacterStore（repository）：保存/加载共用单一 CharacterSaveData 结构（当前加载 inline struct 与保存 charMeta 两份定义，会漂移）
- [ ] 数据库迁移版本化：schema\_migrations 编号表，替换 migrateAccounts/migrateGuilds/migrateCharacters 的手写 PRAGMA 检查
- [ ] config 默认值改 struct tag：加载期一次性应用默认值，删掉 config.go 约 180 个 GetXxx 回退函数

系统化：

- [ ] 统一 System 接口（消息注册 + Tick 钩子 + Save/Load 钩子）注册进 UserEngine；主循环遍历注册的 tickables（interval+phase），替代手工排列 Process\* 调用
- [ ] PlayObject 瘦身（2395 行/72 方法）：只保留移动/战斗/视野/消息队列主干，玩法域逐个迁出（防 Delphi ObjBase.pas 26821 行覆辙）

数据驱动：

- [ ] NPC 脚本命令注册表：map\[string\]CondFunc/ActionFunc，替换 npcscript.go 2478 行巨型 switch，长尾约 240 命令变为增量补齐+可单测
- [ ] 魔法注册表：替换 magicEffType/amuletSkills 硬编码表；DB 为 ID 唯一权威，Delphi 语义以注释映射
- [ ] WIL 解码缓存加 LRU 淘汰（当前解码结果永久驻留，内存只增不减）

测试补强：

- [ ] NPC 脚本命令表驱动测试
- [ ] 登录→建角→进游戏 golden test（内存 netserver 跑通全链路）
