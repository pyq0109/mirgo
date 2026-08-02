# client

- [ ] ScenePlay 场景的 UI 问题
- [ ] SceneSelectChr 场景的 UI 微调
- [x] 攻击特效 (攻杀剑术) 的动画播放问题
- [ ] 声音
- [x] WIL 解析问题, DnItems.wil 解析错误
- [x] 人物男女角色渲染问题
- [x] NPC 不能点
- [x] 支持更大更多的分辨率
- [ ] 小地图显示错误
- [ ] 把客户端的所有可调的配置, 抽离到共同的地方, 容易修改调整. 代码中不要有魔法数字

# server

- [ ] 所有服务端的设置都通过读取 serverconfig 目录下的 jsonc 配置文件来设置
- [x] 登录注册逻辑是否正确
- [ ] 玩家账号密码检查, 是否合规
- [ ] 玩家名字检查, 是否合规

# wilviewer

- [x] 左侧 tree 文件列表高亮显示当前打开的是哪个 wil 文件
- [ ] 右下角 Preview 窗口支持鼠标滚轮上下滚能缩放图片大小, 并且按住滚轮可以左右拖动图片, 方便观察
- [ ] 三个列表可以随意拖动大小高宽
- [x] 动画播放模式是否应该去掉?
- [ ] wilviewer 目录扁平化

# mapviewer

- [ ] mapviewer 目录扁平化

# serverconfig

- [ ] 检查 serverconfig 转换出来的配置文件与 asset/server 是否完全一致
- [ ] serverdata 目录取消, 让 mir2.db 与 server 同级
- [ ] 压缩优化 serverconfig 目录结构, 让配置更密集, 更合理
- [ ] serverconfig 文件名去掉 "\_"

# 综合

- [x] 封包拆包逻辑是否和 Delphi 完全一致
- [ ] doc 目录文档整理
