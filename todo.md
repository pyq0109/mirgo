# client

- [ ] 游戏声音
- [x] 攻击特效 (攻杀剑术) 的动画播放问题
- [x] WIL 解析问题, DnItems.wil 解析错误
- [x] 人物男女角色渲染问题
- [x] NPC 不能点
- [x] 支持更大更多的分辨率 ALT + 回车 切换, 或者 res 1/2/3/4 切换
- [ ] 小地图显示错误, 怪物红点和玩家白点不停闪烁
- [x] 被攻击后的动画朝向问题
- [x] 走路卡脚问题 (跑步会卡 1 秒左右?)
- [ ] SceneSelectChr 场景的 UI 微调, 新建角色的名字输入框
- [ ] NPC 购买物品功能
- [ ] ScenePlay 场景的 UI 问题: NPC 对话框
- [ ] ScenePlay 场景的 UI 问题: 聊天框
- [ ] ScenePlay 场景的 UI 问题: 右下角四个按钮

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

- [ ] serverconfig 转换出来的配置文件要带上本项目的 go 服务端额外的一些配置, 而不仅仅是 176 服务端原本的配置
- [ ] serverconfig 转换出来的配置文件要带上中文注释

# 综合

- [x] WIL 解码缓存加 LRU 淘汰（当前解码结果永久驻留，内存只增不减）
- [x] 封包拆包逻辑是否和 Delphi 完全一致
- [x] doc 目录文档整理
- [x] 恢复提交 go.sum（当前被 gitignore）

