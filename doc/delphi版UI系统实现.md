# Delphi 版 UI 系统实现

> 基于 `asset/delphi/Client/` 源码（commit `98711da`）的完整技术描述。
> 所有引用格式为 `文件:行号`。

## 一、架构总览

MIR2 客户端的 UI 是一套**自建的、基于 DirectX 表面的即时模式控件工具包**，构建在 DelphiX 库之上。不使用任何 VCL 可视控件（无 TButton/TPanel/TLabel），所有 UI 元素每帧直接 blit 到唯一的后缓冲区。

### 五层架构

```
┌──────────────────────────────────────────────────────────────┐
│  TDrawScreen (DrawScrn.pas:22)                                │
│    顶层场景调度器，持有 CurrentScene，驱动帧合成               │
├──────────────────────────────────────────────────────────────┤
│  TDWinManager + TDControl 控件树 (DWinCtl.pas)                │
│    TDControl → TDButton → TDWindow                            │
│    TDControl → TDGrid                                         │
│    TDWinManager = 根路由器（输入分发 + 绘制遍历）              │
├──────────────────────────────────────────────────────────────┤
│  TWMImages (WIL.pas:29)                                       │
│    .wil 图像库绑定，Images[index] → TDirectDrawSurface        │
├──────────────────────────────────────────────────────────────┤
│  TDirectDrawSurface (DXDraws.pas:133)                         │
│    可渲染表面（封装 IDirectDrawSurface）                       │
│    Draw / StretchDraw / DrawAlpha / Fill / Canvas             │
├──────────────────────────────────────────────────────────────┤
│  TDIB (DIB.pas:79)                                            │
│    Device Independent Bitmap，8 位调色板像素缓冲               │
└──────────────────────────────────────────────────────────────┘
```

### 核心设计原则

1. **即时模式渲染**：无缓存控件位图，每帧每个可见控件从 WIL 图像库 blit 到共享后缓冲区
2. **单后缓冲区**：所有 UI 绘制到同一个 `TDirectDrawSurface`（DXDraw.Surface），最后一次性 Flip 到主表面
3. **双树结构**：控件形成独立于 VCL Parent 的 `DParent/DControls` 树，由 `TDWinManager` 统一管理
4. **像素级命中检测**：控件形状由其 WIL 图像的透明度决定，无需矩形近似
5. **8 位调色板渲染**：全部在 256 色模式下运行，"alpha 混合"通过预计算 256×256 查找表实现

---

## 二、控件框架（DWinCtl.pas）

### 2.1 类层次

```
TCustomControl (VCL)
└── TDControl            DWinCtl.pas:26   — 基础控件
    ├── TDButton         DWinCtl.pas:98   — 可按压按钮（释放时在区域内才触发点击）
    │   └── TDWindow     DWinCtl.pas:146  — 可拖动/浮动/模态容器
    └── TDGrid           DWinCtl.pas:115  — 行列网格（背包/交易格）

TComponent
└── TDWinManager         DWinCtl.pas:166  — 顶层根/消息路由器（非 TDControl 子类）
```

注册到 IDE 组件面板 `'MirGame'`（`DWinCtl.pas:205-208`）。

### 2.2 TDControl 基类

#### 字段/属性

| 字段 | 位置 | 说明 |
|------|------|------|
| `FCaption` | `:28` | 文本标题 |
| `FDParent` | `:29` | D 控件树中的父控件（独立于 VCL Parent） |
| `FEnableFocus` | `:30` | 是否可获取键盘焦点 |
| `FVisible` | `:43` | 可见性标志 |
| `Background` | `:45` | 若为 true，控件为非交互背景；点击穿透到 `OnBackgroundClick`，子控件跳过键盘 |
| `DControls: TList` | `:46` | 子控件列表（容器机制） |
| `WLib: TWMImages` | `:48` | 绑定的图像库（纹理源） |
| `FaceIndex: integer` | `:49` | 在 `WLib.Images[]` 中的索引 |
| `WantReturn` | `:50` | 背景点击处理器消费点击的标志 |

默认尺寸：`Width:=80; Height:=24`（`:253-254`）。

#### Published 属性（`:76-95`）

`OnDirectPaint`, `OnKeyPress`, `OnKeyDown`, `OnMouseMove`, `OnMouseDown`, `OnMouseUp`, `OnDblClick`, `OnClick`, `OnInRealArea`, `OnBackgroundClick`, `Caption`, `DParent`, `Visible`, `EnableFocus`, 以及继承的 `Color`, `Font`, `Hint`, `ShowHint`, `Align`。

#### 事件委托类型（`:11-24`）

```pascal
TOnDirectPaint  = procedure(Sender; dsurface: TDirectDrawSurface)   // :13
TOnKeyPress     = procedure(Sender; var Key: Char)                   // :14
TOnKeyDown      = procedure(Sender; var Key: word; Shift)            // :15
TOnMouseMove    = procedure(Sender; Shift; X, Y)                     // :16
TOnMouseDown    = procedure(Sender; Button; Shift; X, Y)             // :17
TOnMouseUp      = procedure(Sender; Button; Shift; X, Y)             // :18
TOnClick        = procedure(Sender)                                  // :19
TOnClickEx      = procedure(Sender; X, Y)                            // :20
TOnInRealArea   = procedure(Sender; X, Y; var IsRealArea)            // :21  ← 自定义命中测试钩子
TOnGridSelect   = procedure(...)                                     // :22
TOnGridPaint    = procedure(...)                                     // :23
TOnClickSound   = procedure(Sender; ClickSound: TClickSound)         // :24
```

`TClickSound = (csNone, csStone, csGlass, csNorm)`（`:11`）— 按钮点击时播放对应音效。

### 2.3 生命周期

1. **Create**（`:235-260`）：分配 `DControls := TList.Create`，清空所有事件钩子，设默认值。注意 `inherited Visible := FALSE`（`:239`）禁用 VCL 窗口，而工具包自身的 `FVisible := TRUE`（`:256`）。
2. **Loaded**（`:306-322`）：运行时（非设计时）扫描 VCL Parent 的组件列表，将 `DParent = self` 的所有 `TDControl` 通过 `AddChild` 挂载。**这是 D 树从窗体扁平组件列表重建的方式。**
3. **注册到管理器**：`TDWinManager.AddDControl(dcon, visible)`（`:899-903`）将根控件加入 `DWinList` 并设置可见性。
4. **Destroy**（`:262-266`）：释放 `DControls` 列表（子控件本身由 VCL Owner 释放）。

### 2.4 坐标系与容器

#### 坐标转换

- **SurfaceX/SurfaceY**（`:325-349`）：沿 `DParent` 链向上**累加**每个父控件的 `Left/Top` → 将局部坐标转为绝对屏幕/表面坐标
- **LocalX/LocalY**（`:352-376`）：逆操作 — **减去**父控件偏移 → 将屏幕坐标转为控件局部空间。`TDWinManager` 用此转换全局鼠标坐标后再分发（`:984,991,1011,1018,...`）

#### 子控件管理

- **AddChild**（`:378-381`）：追加到 `DControls`
- **ChangeChildOrder**（`:383-397`）：对 `Floating=true` 的 `TDWindow`，将窗口移到列表**末尾**（提升 z-order / 置前）

#### Z-order

渲染顺序 = 列表顺序。`DirectPaint` 按 `0..Count-1` 遍历子控件（`:636-638`），**后绘制的在上层**。浮动窗口通过重新追加自身来到最前。

### 2.5 命中检测 — InRange（`:399-418`）

```
1. 矩形测试: x ∈ [Left, Left+Width), y ∈ [Top, Top+Height)
2. 若 OnInRealArea 已赋值 → 由调用者决定（自定义形状命中测试）
3. 否则若 WLib 已绑定 → 读取源图像像素:
      d := WLib.Images[FaceIndex];
      if d.Pixels[x-Left, y-Top] <= 0  →  不在区域内（透明像素）
```

**关键设计**：UI 控件的命中区域由其 WIL 图像的 alpha/透明度（像素值 `<= 0` = 透明）决定，不规则形状的按钮/窗口天然获得精确的命中检测。`Pixels` 对应 `TDirectDrawSurface.GetPixel`（`DXDraws.pas:157,259`）。

---

## 三、事件模型与输入分发

### 3.1 全局单例（`:192-196`）

```pascal
MouseCaptureControl: TDControl  — 当前鼠标捕获控件
FocusedControl:      TDControl  — 当前键盘焦点控件
MainWinHandle:       integer    — 用于 SetCapture 的 HWND
ModalDWindow:        TDControl  — 活动模态窗口（阻断所有其他输入）
```

辅助函数：`SetDFocus/ReleaseDFocus`（`:211-219`），`SetDCapture/ReleaseDCapture`（`:221-231`，封装 Win32 `SetCapture`/`ReleaseCapture`）。

### 3.2 分发算法（递归、深度优先、最顶层优先）

`TDControl` 上的每个输入方法（如 `MouseDown` `:493-522`）遵循相同模式：

```pascal
for i := DControls.Count-1 downto 0 do        // 子控件，最顶层优先
    if child.Visible and child.<Event>(...转换后坐标...) then exit(TRUE);
// 然后处理自身:
    - 若 MouseCaptureControl 已设 → 只有该控件收到事件
    - 若 Background → 触发 OnBackgroundClick（MouseDown）/ 跳过
    - 若 InRange(x,y) → 触发自身处理器
    - MouseDown + EnableFocus → SetDFocus(self)
```

子控件接收**转换后的坐标**（`X-Left, Y-Top`，如 `:471,500,531,568,595`）。

**Background 语义**：`Background` 控件永远不消费 mouse-move/up/click，但 `MouseDown` 时触发 `OnBackgroundClick` 并清除焦点（`:504-512`）— 用于点击空白区域关闭菜单。

### 3.3 TDWinManager 路由（`:916-1106`）

每个管理器方法（如 `MouseMove` `:976-1001`）按以下优先级：

1. **模态窗口优先**：若 `ModalDWindow` 可见 → 仅路由到它（坐标经 `LocalX/LocalY` 转换）并 `exit`
2. **鼠标捕获优先**：若 `MouseCaptureControl` 已设 → 仅路由到它
3. 否则遍历 `DWinList` **前到后**（`0..Count-1`），第一个返回 `TRUE` 的控件停止传播

键盘（`KeyPress`/`KeyDown` `:916-974`）先路由到 `ModalDWindow`，否则仅到 `FocusedControl`（无命中测试）。特殊行为：模态关闭后强制 `Key := #0`（`:928`）。

### 3.4 控件行为

#### TDButton（`:642-695`）

- `MouseDown`（`:665-675`）：设 `Downed:=TRUE` + `SetDCapture(self)`，即使鼠标移出边界也跟踪
- `MouseMove`（`:654-663`）：捕获期间根据 `InRange` 切换 `Downed`（悬停-按压视觉反馈）
- `MouseUp`（`:677-695`）：`ReleaseDCapture`；**仅当仍在 InRange 内**才触发 `OnClickSound` 然后 `OnClick`。经典的"按下-在内-释放在内"点击语义。

#### TDWindow 拖动（`:820-852`）

- `MouseDown` 记录抓取偏移 `SpotX/SpotY`，若 `Floating` 则重排到最前（`ChangeChildOrder`）
- `MouseMove`（`:820-839`）：若已捕获 + 浮动，计算增量并移动 `Left/Top`，**钳制**到屏幕区域 `[WINLEFT..WINRIGHT]` × `[WINTOP..BOTTOMEDGE]`
- `Show`/`ShowModal`（`:859-875`）：设 `Visible`，置前，设焦点；`ShowModal` 额外设 `ModalDWindow := self`

#### TDGrid（`:697-796`）

- `GetColRow`（`:711-719`）：将点映射到单元格 `(x-Left) div ColWidth`
- 选中要求 `MouseDown` 和 `MouseUp` 在**同一格**（`:752-769`）才触发 `OnGridSelect`
- 默认值：`ColCount=8, RowCount=5, ColWidth=36, RowHeight=32`（`:702-705`）

---

## 四、渲染管线

### 4.1 帧循环（ClMain.pas AppOnIdle `:1058`）

每帧执行顺序（**顺序决定 z-order**）：

1. `ProcessKeyMessages` `:1074`、`ProcessActionMessages` `:1075` — 处理消息队列
2. `DScreen.DrawScreen(DxDraw.Surface)` `:1076` — 场景世界 + Actor 覆盖层
3. `g_DWinMan.DirectPaint(DxDraw.Surface)` `:1077` — **所有对话框 UI 覆盖在场景之上**
4. `DScreen.DrawScreenTop` `:1078` — 系统消息
5. `DScreen.DrawHint` `:1079` — tooltip
6. 自定义光标 `:1085-1089`
7. 拖拽物品覆盖 `:1093-1113`
8. 淡入/淡出 `:1114-1130`
9. 连接中 Logo `:1147-1159`
10. `DxDraw.Primary.Draw` `:1161` — 最终 Flip

### 4.2 TDControl.DirectPaint（`:623-639`）

```pascal
if OnDirectPaint assigned  →  调用自定义处理器
else if WLib 已绑定:
    d := WLib.Images[FaceIndex];
    dsurface.Draw( SurfaceX(Left), SurfaceY(Top), d.ClientRect, d, TRUE );  // TRUE = 透明 blit
// 然后递归绘制可见子控件（前到后）
```

控件要么 (a) 委托给自定义 `OnDirectPaint` 处理器，要么 (b) 将绑定的 WIL 图像以色键透明方式 blit 到绝对表面位置。

### 4.3 TDirectDrawSurface 绘制（DXDraws.pas）

#### 基础 blit — Draw（`:1633-1761`）

- 从 `(X,Y)` + `SrcRect` 尺寸构建 `DestRect`，经 `ClipRect2` 裁剪
- **透明路径**使用 `DDBLTFAST_SRCCOLORKEY`（色键 blit）或带裁剪器时用 `DDBLT_KEYSRC`
- 支持**镜像**（翻转 `SrcRect`）通过 `DDBLTFX_MIRRORLEFTRIGHT/UPDOWN`（`:1659-1690`）
- `StretchDraw`（`:1764-1854`）处理缩放，安装临时裁剪器

#### 混合绘制（`:1856-1981`）

`DrawAdd`、`DrawAlpha`、`DrawSub` 锁定双方表面（`dxrDDSurfaceLock`）并调用 `dxrCopyRectBlend`：

| 方法 | 混合模式 | 用途 |
|------|----------|------|
| `DrawAdd` | `DXR_BLEND_ONE1_ADD_ONE2` / `SRCALPHA1_ADD_ONE2` | 加法发光（`:1878-1886`） |
| `DrawAlpha` | `DXR_BLEND_SRCALPHA1_ADD_INVSRCALPHA2` | 标准 src-alpha over（`:1929`） |
| `DrawSub` | `DXR_BLEND_ONE2_SUB_ONE1` / `..._SUB_SRCALPHA1` | 减法变暗（`:1970-1973`） |

索引色（调色板）目标回退到 `DXR_BLEND_ONE1`（不透明拷贝）。

### 4.4 8 位调色板混合（cliUtil.pas）

游戏运行在 **8 位调色板模式**，自定义混合通过预计算 256×256 查找表实现：

- **Color256Mix[i,j]**（`:55`，构建 `:94-120`）：颜色 i,j 的 **50/50 平均**的最近调色板索引
- **Color256Anti[i,j]**（`:56`，构建 `:122-153`）：**加法/滤色混合**（`src + (255-src)/255 * dst`）的最近索引，用于发光/光照效果

**DrawBlendEx**（`:908-1033`）是 MMX 加速的逐像素 blit：锁定双方表面，用 `movq` 将 src+dst 行拷入缓冲区，然后对每字节查找 `pmix[src*256+dst]` 并写回。`blendmode=0` → `Color256Mix`，否则 `Color256Anti`（`:956-959`）。跳过透明源像素（`src==0`，`:996-997`）。

**SpriteCopy**（`:1035+`）是 MMX 色键 blit，`TRANSPARENCY_VALUE=0`（`:1040`）。

### 4.5 文字渲染

#### BoldTextOut（ClFunc.pas:605-616）

绘制文字 5 次：4 个偏移副本用 `bcolor`（描边）在 (x±1,y)、(x,y±1)，然后中心用 `fcolor`。这是名字、HP、系统消息等所有"描边文字"的通用原语。使用 `surface.Canvas.TextOut`（GDI on surface DC）。

#### PomiTextOut

位图数字渲染，用于等级等数值显示。

### 4.6 屏幕合成（TDrawScreen, DrawScrn.pas）

三次绘制遍历覆盖后缓冲区 `MSurface`：

#### DrawScreen（`:234-390`）

1. `MSurface.Fill(0)` 清黑（`:260`）
2. `CurrentScene.PlayScene(MSurface)`（`:262`）— 场景绘制地图/角色/UI
3. FPS 统计（`:264-269`）
4. 仅 PlayScene：逐 Actor 覆盖层 —
   - HP 数字（`:280-285`）
   - 血条（黑底 `HEALTHBAR_BLACK=0` + 红色填充 `HEALTHBAR_RED=1`，按 `HP/MaxHP` 缩放，`:287-301`）
   - 焦点/自身名字文字（`:307-325`）
   - 语音气泡（4 秒过期，`:331-351`）
   - 区域状态图标右上角（`:377-386`）

#### DrawScreenTop（`:392-415`）

系统消息左上角（`m_SysMsgList`，绿色文字，每条 3000ms 后自动过期，`:409-410`）。

#### DrawHint（`:417-519`）

tooltip 框 — 背景图像 `g_WMainImages.Images[394]` 用 `DrawBlendEx(...,0)` 绘制（`:426-436`），上方绘制文字行；加上调试/白色/绿色 HUD 读数（金币、点数、坐标、地图标题、时间）。

**屏幕级 z-order**：场景世界 → Actor 覆盖 → 顶部系统消息 → 提示/HUD。提示和 HUD 永远在最上层。

---

## 五、图像/纹理系统

### 5.1 TWMImages（WIL.pas:29-86）

.wil 图像库加载器：

- 加载 WEMADE `.wil` 文件 + `.wix` 索引
- 字段：`FFileName`, `FImageCount`, `FLibType`, `m_IndexList`（索引表）, `m_FileStream`, `MainPalette: TRGBQuads`
- **LibType**（`WIL.pas:15`）：`ltLoadBmp | ltLoadMemory | ltLoadMunual | ltUseCache` — 控制缓存策略
- **Images[index]: TDirectDrawSurface**（`:76`）→ `FGetImageSurface`（`:348-359`）：
  - `ltUseCache` → `GetCachedSurface(index)`（按需解码 + LRU 缓存，按 `dwLatestTime`）
  - `ltLoadMemory` → 预加载的 `m_ImgArr[index].Surface`
- `LoadDxImage`（`:379+`）：读取 `TWMImageInfo` 头，通过 `TDIB`（`g_boUseDIBSurface`，`:393-407`）解码为 8 位 DIB（带 `MainPalette`），或直接填充 `TDirectDrawSurface`
- `GetImage(index, var px, py)`（`:484`）还返回图像的**枢轴/偏移**（`px,py`）用于居中绘制

### 5.2 TDIB（DIB.pas:79-177）

- `TGraphic` 子类；后端存储 `TDIBSharedImage`（`:41-77`）是**写时复制**的 `TSharedImage`，持有 `FHandle`(DDB)、`FPBits`(位指针)、`FBitmapInfo`、调色板、`FBitCount`、`FWidth/Height`、`FWidthBytes`、`FNextLine`
- `TDIBPixelFormat`（`:34-39`）：RGB 位掩码/移位/计数（支持 16/24/32 位）
- 关键公共 API：`Pixels[X,Y]: DWORD`（`:169`）、`ScanLine[Y]: Pointer`（`:170`）、`PBits`/`TopPBits`（原始缓冲区，`:167,173`）、`BitCount`（`:158`）、`SetSize(W,H,BitCount)`（`:150`）、`ColorTable: TRGBQuads`（`:135`）
- `LoadFromDIB`/`LoadFromDIBRect`（`DXDraws.pas:229-230`）将 `TDIB` 上传到 `TDirectDrawSurface`
- 效果：`Blur`, `Greyscale`, `Mirror`, `Negative`（`:153-156`）

### 5.3 WIL 数据结构（wmUtil.pas）

| 结构 | 位置 | 内容 |
|------|------|------|
| `TWMImageHeader` | `:11-17` | `Title[40], ImageCount, ColorCount, PaletteSize, VerFlag` |
| `TWMImageInfo` | `:29-35` | `nWidth, nHeight: SmallInt; px, py: SmallInt; bits: PByte` — .wil 中每图像头 |
| `TWMIndexHeader` | `:38-42` | `Title[40]('WEMADE Entertainment inc.'), IndexCount, VerFlag` |
| `TWMIndexInfo` | `:51-54` | `Position, Size` — 文件偏移表 |
| `TDXImage` | `:58-63` | `nPx, nPy: SmallInt; Surface: TDirectDrawSurface; dwLatestTime: LongWord` — 缓存运行时图像 |

---

## 六、场景状态机

### 6.1 TSceneType 与 TScene 基类

场景类型枚举（`IntroScn.pas:18-19`）：

```pascal
TSceneType = (stIntro, stLogin, stSelectCountry, stSelectChr,
              stNewChr, stLoading, stLoginNotice, stPlayGame);
```

`TScene` 基类（`IntroScn.pas:35-50`）定义虚方法：

| 方法 | 用途 |
|------|------|
| `Initialize` | 初始化场景资源 |
| `Finalize` | 释放资源 |
| `OpenScene` | 进入场景时调用（显示 UI、播放音乐） |
| `CloseScene` | 离开场景时调用（隐藏 UI） |
| `OpeningScene` | 场景打开中的过渡动画 |
| `PlayScene(dsurface)` | 每帧绘制场景内容 |
| `KeyPress/KeyDown` | 键盘输入 |
| `MouseMove/MouseDown` | 鼠标输入 |

具体场景类：`TIntroScene:52`, `TLoginScene:62`, `TSelectChrScene:122`, `TLoginNotice:156`。

### 6.2 切换逻辑（TDrawScreen.ChangeScene, DrawScrn.pas:123-139）

```pascal
procedure ChangeScene(SceneType: TSceneType);
begin
    CurrentScene.CloseScene;
    case SceneType of
        stIntro:       CurrentScene := IntroScene;
        stLogin:       CurrentScene := LoginScene;
        stSelectChr:   CurrentScene := SelectChrScene;
        stLoginNotice: CurrentScene := LoginNoticeScene;
        stPlayGame:    CurrentScene := PlayScene;
    end;
    CurrentScene.OpenScene;
end;
```

输入转发到活动场景：`KeyPress:99`, `KeyDown:105`, `MouseMove:111`, `MouseDown:117`。

### 6.3 网络驱动的状态转换（ClMain.pas）

| 触发 | 转换 | 位置 |
|------|------|------|
| `CSocketConnect` + `cnsLogin` | → `stLogin` | `:2736` |
| `CSocketConnect` + `cnsSelChr` | → `LoginScene.OpenLoginDoor` | `:2742` |
| `CSocketConnect` + `cnsPlay` | → `stLoginNotice` → `SendRunLogin` | `:2753-2757` |
| `SM_LOGON` | → `stPlayGame` | `:3859` |
| 登录门动画完成 | → `stSelectChr` | `IntroScn.pas:851` |
| 登出/重选 | → `stSelectChr` | `ClMain.pas:2549` |

### 6.4 各场景详细实现

以下四章分别深入描述每个场景的完整 UI 实现：

- **第七章** — 登录场景（TLoginScene）
- **第八章** — 角色选择场景（TSelectChrScene）
- **第九章** — 公告场景（TLoginNotice）
- **第十章** — 游戏场景（TPlayScene）

> 注：`TIntroScene`（`IntroScn.pas:52-60, 219-242`）是空壳 — Create/Destroy/OpenScene/CloseScene/PlayScene 全部为空方法体，不渲染任何内容，仅保留 `stIntro` 枚举位。实际入口流程在 socket 连接后直接跳转登录场景。

---

## 七、登录场景（TLoginScene）

> 源码：`IntroScn.pas:62-120`（类声明）、`:248-1097`（实现）
> 对话框控件：`FState.pas:756-892`（Initialize）、`FState.dfm`（事件绑定）

### 7.1 场景概述

`TLoginScene` 是玩家看到的第一个有 UI 的场景。它管理三组输入表单（登录/注册/改密码）、一个开门过渡动画、以及与 `FrmDlg` 中对话框控件的交互。

**字段**（`IntroScn.pas:62-120`）：
- 19 个 `TEdit` 控件（`:64-82`）
- 门动画状态：`m_nCurFrame`/`m_nMaxFrame`/`m_dwStartTime`/`m_boNowOpening`/`m_boOpenFirst`（`:83-87`）
- 注册重试缓存：`m_NewIdRetryUE: TUserEntry`、`m_NewIdRetryAdd: TUserEntryAdd`（`:88-89`）
- 公共：`m_sLoginId`、`m_sLoginPasswd`、`m_boUpdateAccountMode`（`:99-101`）

### 7.2 TEdit 控件清单

所有编辑框在构造函数中代码创建（`:248-481`），挂载到 `FrmMain`，全部 `BorderStyle=bsNone`、`Color=clBlack`、`Font.Color=clWhite`、`Visible=FALSE`。

#### 登录框（Tag = 10）

| 控件 | 位置 | 尺寸 | MaxLength | PasswordChar | 事件 |
|------|------|------|-----------|--------------|------|
| `m_EdId` | Left=255, Top=511 | 112×19 | 10 | 无 | `EdLoginIdKeyPress` |
| `m_EdPasswd` | Left=495, Top=511 | 112×19 | 10 | `*` | `EdLoginPasswdKeyPress` |

位置在 `OpenScene` 中设置（`:499-512`），字体 `Font.Size=10`。

#### 新账号（Tag = 11）

基准坐标：`nx = SCREENWIDTH div 2 - 320 = 80`，`ny = SCREENHEIGHT div 2 - 238 = 62`（`:276-277`）。

| 控件 | Left | Top | Width | Height | MaxLength | PasswordChar |
|------|------|-----|-------|--------|-----------|--------------|
| `m_EdNewId` | nx+86=166 | ny+91=153 | 104 | 13 | 10 | 无 |
| `m_EdNewPasswd` | 166 | ny+118=180 | 104 | 13 | 10 | `*` |
| `m_EdConfirm` | 166 | ny+149=211 | 104 | 12 | 10 | `*` |
| `m_EdYourName` | 166 | ny+190=252 | 105 | 13 | 20 | 无 |
| `m_EdSSNo` | 166 | ny+207=269 | 105 | 13 | 14 | 无 |
| `m_EdBirthDay` | 166 | ny+217=279 | 105 | 13 | 10 | 无 |
| `m_EdQuiz1` | nx+263=343 | 180 | 124 | 13 | 20 | 无 |
| `m_EdAnswer1` | 343 | 211 | 124 | 12 | 12 | 无 |
| `m_EdQuiz2` | 343 | 252 | 124 | 13 | 20 | 无 |
| `m_EdAnswer2` | 343 | ny+218=280 | 124 | 13 | 12 | 无 |
| `m_EdPhone` | 343 | ny+285=347 | 124 | 13 | 14 | 无 |
| `m_EdMobPhone` | 343 | ny+315=377 | 124 | 12 | 13 | 无 |
| `m_EdEMail` | 343 | ny+368=430 | 124 | 13 | 40 | 无 |

所有 Tag=11 控件共享 `OnKeyPress=EdNewIdKeyPress`、`OnEnter=EdNewOnEnter`（`:279-410`）。

#### 改密码（Tag = 12）

基准坐标：`nx = SCREENWIDTH div 2 - 210 = 190`，`ny = SCREENHEIGHT div 2 - 150 = 150`（`:412-413`）。

| 控件 | Left | Top | PasswordChar |
|------|------|-----|--------------|
| `m_EdChgId` | nx+191=381 | ny+92=242 | 无 |
| `m_EdChgCurrentpw` | 381 | ny+119=269 | `*` |
| `m_EdChgNewPw` | 381 | ny+145=295 | `*` |
| `m_EdChgRepeat` | 381 | ny+172=322 | `*` |

全部 `Height=13, Width=104, MaxLength=10`（`:414-480`）。

### 7.3 OpenScene / CloseScene

**OpenScene**（`:488-520`）：
1. `m_nCurFrame := 0`，`m_nMaxFrame := 10`（门动画参数）
2. 清空 `m_sLoginId`/`m_sLoginPasswd`
3. 设置 `m_EdId`/`m_EdPasswd` 位置（255,511 / 495,511），`Visible:=FALSE`
4. `m_boOpenFirst := TRUE`（首帧标志）
5. 显示 `FrmDlg.DLogin`，隐藏 `FrmDlg.DNewAccount`
6. `m_boNowOpening := FALSE`
7. 播放 BGM：`PlayBGM(bmg_intro)` → `wav\log-in-long2.wav`（`SoundUtil.pas:31`）

**CloseScene**（`:522-528`）：隐藏 `m_EdId`/`m_EdPasswd`/`DLogin`，`SilenceSound`。

### 7.4 PlayScene 渲染（`:804-855`）

1. **首帧显示**（`:808-813`）：若 `m_boOpenFirst`，清除标志，显示 `m_EdId`+`m_EdPasswd`，聚焦 `m_EdId`
2. **背景**（`:815-822`）：`g_WChrSelImages.Images[102-80]` = 索引 **22**，居中绘制 `((SCREENWIDTH-800)/2, (SCREENHEIGHT-600)/2)`
3. **门动画**（`:823-854`），仅当 `m_boNowOpening`：
   - 每 **300ms** 推进一帧（`:826-828`）
   - 到达末帧（`m_nCurFrame >= m_nMaxFrame-1`）→ 启动淡出：`g_boDoFadeOut:=TRUE`, `g_boDoFadeIn:=TRUE`, `g_nFadeIndex:=29`（`:832-836`）
   - 门帧图像：`g_WChrSelImages.Images[103 + m_nCurFrame - 80]` = 索引 **23+frame**（`:841`）
   - 绘制位置：`((SCREENWIDTH-800)/2 + 252, (SCREENHEIGHT-600)/2 + 106)`（`:845`）
   - 淡出完成（`g_nFadeIndex <= 1`）→ 清 WIL 缓存 → `DScreen.ChangeScene(stSelectChr)`（`:847-851`）

**门触发**：`OpenLoginDoor`（`:796-802`）— 设 `m_boNowOpening:=TRUE`，隐藏登录框，播放 `s_rock_door_open`=音效 100。

### 7.5 子状态机 ChangeLoginState（`:857-927`）

| 状态 | Tag 过滤 | 对话框可见性 | 焦点 |
|------|----------|-------------|------|
| `lsLogin` | 10 | DLogin=TRUE, DNewAccount=FALSE, DChgPw=FALSE | `m_EdId` |
| `lsNewId` / `lsNewidRetry` | 11 | DNewAccount=TRUE, 其余 FALSE | `m_EdNewId`（或 `m_EdConfirm`） |
| `lsChgpw` | 12 | DChgPw=TRUE, 其余 FALSE | `m_EdChgId` |
| `lsCloseAll` | -1 | 全部 FALSE | 无 |

切换时遍历 `FrmMain.Controls` 中所有 `TEdit`（Tag 10-12），匹配的显示并**清空文本**，不匹配的隐藏并清空（`:869-883`）。

`lsNewidRetry` 额外行为：从 `m_NewIdRetryUE`/`m_NewIdRetryAdd` 回填 12 个编辑框（`:936-952`）。`m_boUpdateAccountMode` 为 TRUE 时 `m_EdNewId.Enabled:=FALSE`（`:898-901`）。

### 7.6 输入验证与焦点链

#### 登录焦点链

`EdLoginIdKeyPress`（`:530-539`）：Enter → 存 `m_sLoginId := LowerCase(text)` → 聚焦 `m_EdPasswd`。

`EdLoginPasswdKeyPress`（`:541-558`）：`~`/`'` 重映射为 `_`。Enter → 存 ID+密码 → 两者非空则 `FrmMain.SendLogin`（`:550`）+ 清空隐藏编辑框；否则聚焦 `m_EdId`。

#### 注册焦点链（`:683-699`）

```
m_EdNewId → m_EdNewPasswd → m_EdConfirm → m_EdYourName → m_EdSSNo →
m_EdBirthDay → m_EdQuiz1 → m_EdAnswer1 → m_EdQuiz2 → m_EdAnswer2 →
m_EdPhone → m_EdMobPhone → m_EdEMail → 回到 m_EdNewId（若启用）
```

#### 改密码焦点链（`:701-704`）

```
m_EdChgId → m_EdChgCurrentpw → m_EdChgNewPw → m_EdChgRepeat → 回到 m_EdChgId
```

#### 验证函数

| 函数 | 位置 | 规则 |
|------|------|------|
| `NewIdCheckNewId` | `:569-579` | 长度 ≥ 3 |
| `NewIdCheckSSno` | `:581-607` | 格式 `NNNNNN-NNNNNNN`，月 1-12，日 1-31，性别位 1-2 |
| `NewIdCheckBirthDay` | `:609-632` | 格式 `YYYY/MM/DD`，年 >1890 且 ≤2101 |
| `CheckUserEntrys` | `:976-1029` | 全字段顺序验证：ID≥3 → SSNo → 生日 → 密码≥3 → 确认一致 → 问题/答案≥1 → 姓名≥1 |

密码字段（`m_EdNewPasswd`/`m_EdChgNewPw`/`m_EdChgRepeat`）阻止 `~`、`'`、空格字符（`:640-641`）。

#### 上下文帮助（EdNewOnEnter, `:709-786`）

每个编辑框获焦时在 `FrmDlg.NAHelps` 中填入中文帮助文本（多行），由 `DNewAccountDirectPaint` 绘制在对话框右侧。

### 7.7 对话框控件（FState.pas Initialize）

#### DLogIn 及按钮（`:756-774`）

| 控件 | WIL 索引 | Left | Top | 说明 |
|------|----------|------|-----|------|
| `DLogIn` | [174]（注释掉） | 0 | 0 | 背景由 PlayScene 绘制 |
| `DLoginNew` | [61] | 447 | 558 | 新账号按钮 |
| `DLoginOk` | [62] | 90 | 558 | 登录按钮 |
| `DLoginChgPw` | [53] | 268 | 558 | 改密码按钮 |
| `DLoginClose` | [64] | 613 | 558 | 关闭按钮 |

**DirectPaint**：`DLoginNewDirectPaint`（`FState.pas:2342-2354`）— 仅按压态（`Downed`）时绘制 `WLib.Images[FaceIndex]`，未按压时不绘制（背景图已含按钮美术）。

**DFM 绑定**：4 个按钮均 `DParent=DLogIn`、`OnDirectPaint=DLoginNewDirectPaint`、`OnClickSound=DLoginNewClickSound`。`DLoginNew`/`DLoginOk`/`DLoginChgPw` 用 `csStone`，`DLoginClose` 用 `csNorm`。

#### DNewAccount 及按钮（`:861-876`）

| 控件 | WIL 索引 | Left | Top |
|------|----------|------|-----|
| `DNewAccount` | [63] | 居中 | 居中 |
| `DNewAccountOk` | [51] | 305 | 530 |
| `DNewAccountCancel` | [52] | 445 | 530 |
| `DNewAccountClose` | [83] | 587 | 33 |

**DirectPaint**：`DNewAccountDirectPaint`（`:2650-2672`）— 绘制背景 + `NAHelps` 帮助文本 + `NewAccountTitle` 标题。

#### DChgPw 及按钮（`:880-892`）

| 控件 | WIL 索引 | Left | Top |
|------|----------|------|-----|
| `DChgPw` | [50] | 居中 | 居中 |
| `DChgpwOk` | [361] | 81 | 141 |
| `DChgpwCancel` | [365] | 160 | 141 |

#### DSelServerDlg 及按钮（`:777-857`）

**中文版**：

| 控件 | WIL 索引 | Left | Top |
|------|----------|------|-----|
| `DSelServerDlg` | [160] | 居中 | 居中 |
| `DSSrvClose` | [64] | 448 | 33 |
| `DSServer1` | [161] | 134 | 102 |
| `DSServer2` | [162] | 236 | 101 |
| `DSServer3` | [163] | 87 | 190 |
| `DSServer4` | [164] | 280 | 190 |
| `DSServer5` | [165] | 134 | 280 |
| `DSServer6` | [166] | 236 | 280 |

**英文版**：`DSelServerDlg` 用 [256]，`DSSrvClose` 用 [83]，`DSServer1-6` 全部用 [79]，Left=65，Top=100/145/190/235/280/325。

**DirectPaint**：`DMsgDlgOkDirectPaint`（`:2209-2290`）— 未按压绘制 `[FaceIndex]`，按压绘制 `[FaceIndex+1]`。服务器按钮额外从 `g_ServerList` 读取名称和状态，按状态着色：0=维护(灰)、1=流畅(lime)、2=正常(绿)、3=繁忙(maroon)、4=满(红)。

**ShowSelectServerDlg**（`:2453-2517`）：根据 `g_ServerList.Count` 调整可见按钮数和位置。

### 7.8 网络发送

| 动作 | 方法 | 消息 |
|------|------|------|
| 登录 | `FrmMain.SendLogin(id, pw)` | `CM_IDPASSWORD`（`ClMain.pas:2827`） |
| 注册 | `FrmMain.SendNewAccount(ue, ua)` | `CM_ADDNEWUSER`（`:2838`） |
| 补充账号 | `FrmMain.SendUpdateAccount(ue, ua)` | `CM_UPDATEUSER`（`:2844`） |
| 改密码 | `FrmMain.SendChgPw(uid, pw, newpw)` | `CM_CHANGEPASSWORD`（`:2850`） |
| 选服 | `FrmMain.SendSelectServer(svname)` | `CM_SELECTSERVER`（`:2856`） |

---

## 八、角色选择场景（TSelectChrScene）

> 源码：`IntroScn.pas:122-154`（类声明）、`:1102-1548`（实现）
> 对话框控件：`FState.pas:899-965`（Initialize）

### 8.1 场景概述

管理 2 个角色槽的选择、创建、删除。每个角色槽有冻结（石化）/解冻动画状态。

### 8.2 TSelChar 记录（`:20-33`）

| 字段 | 类型 | 说明 |
|------|------|------|
| `Valid` | Boolean | 槽位是否有角色 |
| `UserChr` | TUserCharacterInfo | Name/Job/Hair/Level/Sex |
| `Selected` | Boolean | 是否被选中 |
| `FreezeState` | Boolean | TRUE = 冻结/石化中 |
| `Unfreezing` | Boolean | 正在播放解冻动画 |
| `Freezing` | Boolean | 正在播放冻结动画 |
| `AniIndex` | integer | 动画帧计数器 |
| `DarkLevel` | integer | 选中后渐亮级别（30→0） |
| `EffIndex` | integer | 石化特效帧计数器 |
| `StartTime/moretime/startefftime` | longword | 动画时间戳 |

### 8.3 EdChrName（`:1109-1121`）

`Parent=FrmMain`，`Height=21`，`Width=129`，`BorderStyle=bsNone`，`Color=clBlack`，`Font.Color=clWhite`，**`ImeMode=LocalLanguage`**（整个文件中唯一设置 IME 的控件），`MaxLength=14`，`Visible=FALSE`。

### 8.4 OpenScene / CloseScene

**OpenScene**（`:1136-1141`）：显示 `FrmDlg.DSelectChr`，启用 `SoundTimer`（Interval=1ms → 首次触发播放 `bmg_select`=`wav\sellect-loop2.wav`，然后禁用自身）。

**CloseScene**（`:1143-1148`）：`SilenceSound`，隐藏 `DSelectChr`，禁用 `SoundTimer`。

### 8.5 PlayScene 渲染（`:1363-1548`）

#### 背景

`g_WMainImages.Images[65]`，居中绘制（`:1375-1383`）。

#### 角色精灵帧库（g_WChrSelImages）

| 动画 | 基址公式 | 帧数 | 帧率 | 方向 |
|------|----------|------|------|------|
| 冻结/解冻（石化过渡） | `60 + Job*40 + Sex*120` | 13（`FREEZEFRAME`） | 50ms | 解冻正序，冻结倒序 |
| 选中待机循环 | `40 + Job*40 + Sex*120` | 16（`SELECTEDFRAME`） | 300ms | 循环 |
| 石化特效叠加 | `[4 + effIndex]` | 14（`EFFECTFRAME`） | 50ms | DrawBlend mode=1 |

冻结态静态帧：`Images[60 + Job*40 + Sex*120]`（frame 0）。

#### 逐 Job×Sex 精灵锚点（相对 800×600 居中偏移）

| Job | Sex=0 (bx,by) | Sex=0 (fx,fy) | Sex=1 (bx,by) | Sex=1 (fx,fy) |
|-----|---------------|---------------|---------------|---------------|
| 0 战士 | (71, 52) | (71, 52) | (65, 55) | (65, 55) |
| 1 法师 | (77, 46) | (77, 46) | (171, 97) | (141, 83) |
| 2 道士 | (85, 63) | (85, 63) | (164, 103) | (141, 83) |

（`:1390-1430`）

**槽位 1 偏移**：`bx += 340`，`by += 2`，`fx += 340`，`fy += 2`，`ex += 430`（`:1431-1438`）。

#### 解冻动画（`:1439-1459`）

- 身体：`Images[60 + Job*40 + Sex*120 + aniIndex]`，绘制于 (bx,by)
- 特效：`Images[4 + effIndex]`，`DrawBlend` 于 (ex,ey)
- 身体每 50ms 推进，特效每 50ms 推进
- 结束条件：`aniIndex > FREEZEFRAME-1`（13 帧后）→ `FreezeState:=FALSE`

#### 冻结动画（`:1466-1479`）

- 倒序播放：`Images[60 + Job*40 + Sex*120 + FREEZEFRAME - aniIndex - 1]`
- 结束：`FreezeState:=TRUE`

#### 待机动画（`:1480-1516`）

- 未冻结：`Images[40 + Job*40 + Sex*120 + aniIndex]`，300ms/帧，16 帧循环
- `DarkLevel > 0` 时：创建临时表面 → `MakeDark(dd, 30-DarkLevel)` → 绘制（渐亮效果）
- `DarkLevel` 每 25ms 递减 1 直到 0（`:1510-1514`）
- 冻结：静态帧 `Images[60 + Job*40 + Sex*120]`

#### 文字（`:1517-1538`）

| 内容 | 槽位 0 坐标 | 槽位 1 坐标 |
|------|------------|------------|
| 名字 | (136, 476) | (586, 476) |
| 等级 | (136, 513) | (666, 513) |
| 职业 | (136, 548) | (638, 548) |

`BoldTextOut` 白色文字 + 黑色描边，`TRANSPARENT` 背景模式。

**服务器名**：水平居中，`y = (SCREENHEIGHT-600)/2 + 8`（`:1539-1545`）。

### 8.6 按钮交互

| 按钮 | 处理器 | 行为 |
|------|--------|------|
| `DscSelect1` | `SelChrSelect1Click`（`:1157-1172`） | 槽 0 有效且未选中 → `FrmMain.SelectChr(name)` + 解冻动画 + `s_meltstone`(101) |
| `DscSelect2` | `SelChrSelect2Click`（`:1174-1189`） | 同上，槽 1 |
| `DscStart` | `SelChrStartClick`（`:1191-1206`） | 取选中角色名 → 快速淡出 `g_boDoFastFadeOut:=TRUE, g_nFadeIndex:=29` → `FrmMain.SendSelChr(chrname)` |
| `DscNewChr` | `SelChrNewChrClick`（`:1208-1215`） | 有空槽 → `MakeNewChar(index)`；两槽满 → DMessageDlg 提示 |
| `DscEraseChr` | `SelChrEraseChrClick`（`:1217-1231`） | 选中+已解冻+有名 → DMessageDlg [mbYes,mbNo,mbCancel] 确认 → mrYes → `FrmMain.SendDelChr(name)` |
| `DscCredits` | `SelChrCreditsClick`（`:1233-1235`） | 空方法 |
| `DscExit` | `SelChrExitClick`（`:1237-1240`） | `FrmMain.Close` |

### 8.7 创角对话框控件（FState.pas:929-965）

| 控件 | WIL 索引 | Left | Top | Tag |
|------|----------|------|-----|-----|
| `DCreateChr` | [73] | 居中 | 居中 | — |
| `DccWarrior` | [74] | 36 | 139 | 55 |
| `DccWizzard` | [75] | 103 | 139 | 56 |
| `DccMonk` | [76] | 168 | 139 | 57 |
| `DccMale` | [77] | 70 | 211 | 58 |
| `DccFemale` | [78] | 137 | 211 | 59 |
| `DccLeftHair` | [79]（注释） | 76 | 308 | — |
| `DccRightHair` | [80]（注释） | 170 | 308 | — |
| `DccOk` | [51] | 46 | 273 | — |
| `DccClose` | [52] | 138 | 273 | — |

**选中指示器**：`DccCloseDirectPaint`（`:2725-2761`）非按压态时，若当前 Job/Sex 匹配则绘制 `Images[55-59]` 叠加（战士[55]/法师[56]/道士[57]/男[58]/女[59]）。

**MakeNewChar**（`:1268-1288`）定位：
- 槽 0：`DCreateChr.Left:=469, Top:=63`
- 槽 1：`DCreateChr.Left:=87, Top:=63`
- `EdChrName.Left := DCreateChr.Left + 63, Top := DCreateChr.Top + 79`

**SelChrNewOk**（`:1318-1337`）：发型 = `1 + Random(5)`（随机，非预览值），发送 `FrmMain.SendNewChr(LoginId, chrname, shair, sjob, ssex)`。

### 8.8 网络发送

| 动作 | 方法 | 消息 |
|------|------|------|
| 选择角色 | `FrmMain.SelectChr(name)` | 仅本地（设 `EdChrNamet.Text`） |
| 进入游戏 | `FrmMain.SendSelChr(name)` | `CM_SELCHR`（`ClMain.pas:2896`） |
| 删除角色 | `FrmMain.SendDelChr(name)` | `CM_DELCHR`（`:2888`） |
| 创建角色 | `FrmMain.SendNewChr(uid, name, hair, job, sex)` | `CM_NEWCHR`（`:2872`） |

---

## 九、公告场景（TLoginNotice）

> 场景类：`IntroScn.pas:156-161`（空壳）
> 公告流程：`ClMain.pas:2749-2758, 5732-5748`（消息处理）
> 公告渲染：`FState.pas:1938-2128`（DMessageDlg 模态对话框）
> 服务端门控：`M2Server/ObjBase.pas:1788-1827`（RunNotice）

### 9.1 TLoginNotice 场景类（空壳）

`TLoginNotice` **不重写任何虚方法**：

- 仅声明 `Create` 和 `Destroy`（`:156-161`）
- `Create`（`:1553-1556`）：仅调用 `inherited Create(stLoginNotice)`
- `Destroy`（`:1558-1561`）：仅调用 `inherited Destroy`
- 继承 `TScene` 的空方法体（`:183-191, 213-216`）
- 在渲染循环中，`TDrawScreen.DrawScreen` 先 `MSurface.Fill(0)` 清黑（`DrawScrn.pas:260`），再调用 `CurrentScene.PlayScene` — 空方法 → **纯黑屏**

该场景的唯一作用是**遮盖选角场景**，在等待服务端公告期间提供黑色背景。

### 9.2 完整流程：选角 → 公告 → 进入游戏

```
玩家点击"开始"
  │  IntroScn.pas:1191  SelChrStartClick
  │  → 快速淡出 g_boDoFastFadeOut:=TRUE, g_nFadeIndex:=29
  │  → FrmMain.SendSelChr(chrname)  发送 CM_SELCHR(103)
  ▼
服务端返回 SM_STARTPLAY(525)
  │  ClMain.pas:5125  ClientGetStartPlay
  │  → 解码游戏服务器地址/端口 (g_sRunServerAddr/g_nRunServerPort)
  │  → g_ConnectionStep := cnsPlay
  │  → 断开登录服务器，连接游戏服务器
  ▼
Socket 连接成功（cnsPlay 分支, ClMain.pas:2749-2758）
  │  → ClearBag（清空背包）
  │  → DScreen.ClearChatBoard（清空聊天）
  │  → DScreen.ChangeScene(stLoginNotice)  ← 切换到黑屏占位场景
  │  → SendRunLogin("**loginID/charName/cert/version/code")
  ▼
服务端发送 SM_SENDNOTICE(658)
  │  服务端: ObjBase.pas:16326  SendNotice
  │  body = EncodeString(公告各行用 #32#27 即空格+ESC 分隔)
  │  Recog = 2000（客户端忽略）
  ▼
客户端消息分发
  │  CSocketRead(:2785) → SocStr 缓冲
  │  Timer1Timer(:3478) → 帧提取 #...! → DecodeMessagePacket(:3606)
  │  SM_SENDNOTICE 在 g_MySelf=nil 前置门中通过显式 fall-through 列表
  │  （:3795-3799）进入主 case → ClMain.pas:4677 → ClientGetSendNotice
  ▼
ClientGetSendNotice（ClMain.pas:5732-5748）
  │  1. g_boDoFastFadeOut := FALSE  （停止选角淡出）
  │  2. DecodeString(body) → 按 #27 拆行 → 用 '\' 拼接
  │  3. FrmDlg.DialogSize := 2  （大尺寸对话框）
  │  4. FrmDlg.DMessageDlg(msgstr, [mbOk])  ← 阻塞式模态
  ▼
DMessageDlg 渲染（FState.pas:1938-2128）
  │  背景: Prguse.wil[380]（高对话框），居中
  │  文字: BoldTextOut 白色/黑色描边，14px 行距，按 '\' 分行
  │  按钮: 仅 DMsgDlgOk [361]，位于 (105, 305)
  │  模态循环: while DMsgDlg.Visible do
  │    FrmMain.ProcOnIdle（渲染黑屏+对话框）
  │    Application.ProcessMessages（泵输入）
  │  注: Timer1Timer 的 busy 锁阻止新消息处理，后续消息排队
  ▼
玩家点击 OK / 按 Enter
  │  FState.pas:2130  DialogResult := mrOk → 退出模态循环
  │  ClMain.pas:5746  SendClientMessage(CM_LOGINNOTICEOK=1018)
  ▼
服务端门控（ObjBase.pas:1788-1827）
  │  RunNotice 检查 CM_LOGINNOTICEOK:
  │    收到 → m_boLoginNoticeOK := True (:1817)
  │    10 秒超时 → m_boEmergencyClose := True（踢人, :1808-1811）
  │  下一 tick → UserLogon → 发送 SM_LOGON(50)
  ▼
SM_LOGON 处理器（ClMain.pas:3847-3876）
  │  PlayScene.SendMsg(SM_LOGON,...) → 创建 g_MySelf
  │  DScreen.ChangeScene(stPlayGame)  ← 进入游戏场景
  │  SendClientMessage(CM_QUERYBAGITEMS) → 请求背包数据
```

### 9.3 公告消息体格式

**服务端发送**（`M2Server/ObjBase.pas:16326-16342`）：

```
SM_SENDNOTICE, Recog=2000, Param=0, Tag=0, Series=0
body = EncodeString(行1 + #32#27 + 行2 + #32#27 + ... + 行N)
```

公告文本来源：`NoticeManager.GetNoticeMsg('Notice', ...)`，每行用空格+ESC（`#$20#$1B`）连接。

**客户端解析**（`ClMain.pas:5738-5742`）：

```pascal
body := DecodeString(body);
while body <> '' do begin
    body := GetValidStr3(body, data, [#27]);  // 按 ESC 拆分
    msgstr := msgstr + data + '\';             // 转为 '\' 分行
end;
```

### 9.4 模态对话框渲染细节

`DMessageDlg` 在 `DialogSize=2` 时（`FState.pas:2029-2041`）：

| 项目 | 值 |
|------|-----|
| 背景图像 | `Prguse.wil[380]`（高尺寸） |
| 文字起点 | `msglx=23, msgly=20`（相对对话框） |
| 行距 | 14px |
| OK 按钮 | `DMsgDlgOk`，`Prguse.wil[361]`，位于 `(105, 305)` |
| 文字颜色 | 白色（`clWhite`）+ 黑色描边（`clBlack`） |
| 键盘 | Enter = OK（`:2142`）；ESC 无效（仅 Cancel 可见时才响应，`:2152-2157`） |

模态期间（`:2098-2117`）：
- `FrmMain.ProcOnIdle` → `AppOnIdle` 持续渲染帧（黑屏背景 + `g_DWinMan.DirectPaint` 绘制对话框）
- `Application.ProcessMessages` 泵送输入和定时器
- `Timer1Timer` 的 `busy` 重入锁（`:3483`）阻止新 socket 消息被处理 — 后续消息在 `BufferStr` 中排队，等对话框关闭后才继续分发

### 9.5 服务端门控机制

`TPlayObject.RunNotice`（`M2Server/ObjBase.pas:1788-1827`）：

1. 首次调用：发送 `SendNotice()`，设 `m_boSendNotice := True`，记录时间戳
2. 后续调用：扫描消息队列寻找 `CM_LOGINNOTICEOK`
3. 收到 OK → `m_boLoginNoticeOK := True`（`:1817`）
4. 超过 10 秒未收到 → `m_boEmergencyClose := True`（`:1808-1811`），断开连接
5. `m_boLoginNoticeOK` 为 True 后，`UsrEngn.pas:905-911` 的 tick 循环调用 `UserLogon` → 发送 `SM_LOGON`

**重连/换服跳过公告**：`ClientGetReconnect`（`ClMain.pas:5161-5213`）复用 `cnsPlay` + `stLoginNotice` + `SendRunLogin` 路径，但 `g_boServerChanging=TRUE` 使连接处理器跳过场景切换（`:2750-2756`）。服务端在重连时预设 `m_boLoginNoticeOK := True`（`UsrEngn.pas:717`），不发送公告。

---

## 十、游戏场景（TPlayScene）

> 源码：`PlayScn.pas:145-243`（类声明）、`:252-2366`（实现）

### 10.1 类结构

#### 私有字段（`:146-175`）

| 字段 | 类型 | 说明 |
|------|------|------|
| `m_MapSurface` | TDirectDrawSurface | 瓦片地图离屏表面 |
| `m_ObjSurface` | TDirectDrawSurface | 对象/角色/特效离屏表面 |
| `m_FogScreen` | array[0..445, 0..800] of byte | 逐像素迷雾/黑暗缓冲 |
| `m_Lights[0..5]` | TLightEffect | 6 级光罩（lig0a-f.dat） |
| `m_dwMoveTime` | LongWord | 移动 tick 计时器 |
| `m_nMoveStepCount` | Integer | 移动子步（0/1 交替） |
| `m_dwAniTime` | LongWord | 动画帧计时器 |
| `m_nAniCount` | Integer | 全局动画计数器 |
| `m_nDefXX/m_nDefYY` | Integer | 缓存的渲染偏移 defx/defy |
| `m_LightMap[0..30, 0..26]` | TLightMapInfo | 逐格光照累积网格 |

#### 公共字段（`:176-194`）

| 字段 | 类型 | 说明 |
|------|------|------|
| `EdChat` | TEdit | 聊天输入框 |
| `MemoLog` | TMemo | 调试日志（隐藏） |
| `EdAccountt/EdChrNamet` | TEdit | 账号/角色名编辑框 |
| `m_ActorList` | TList | 所有可见 Actor |
| `m_GroundEffectList` | TList | 地面魔法特效（火墙等） |
| `m_EffectList` | TList | 爆炸/通用魔法特效 |
| `m_FlyList` | TList | 飞行弹道（箭/斧/火球） |
| `m_dwBlinkTime/m_boViewBlink` | — | 小地图闪烁计时 |

### 10.2 初始化

#### EdChat 创建（`:267-280`）

| 属性 | 值 |
|------|-----|
| Parent | FrmMain |
| BorderStyle | bsNone |
| Left | 208 |
| Top | SCREENHEIGHT-19 = 581 |
| Height | 12 |
| Width | (SCREENWIDTH div 2 - 207) * 2 = 386 |
| Color | clSilver |
| MaxLength | 70 |
| Visible | FALSE |

#### 离屏表面（Initialize, `:442-466`）

| 表面 | 尺寸 | 说明 |
|------|------|------|
| `m_MapSurface` | 1022×573 | `(MAPSURFACEWIDTH + UNITX*4 + 30) × (MAPSURFACEHEIGHT + UNITY*4)`，系统内存 |
| `m_ObjSurface` | 800×445 | `MAPSURFACEWIDTH × MAPSURFACEHEIGHT`，系统内存 |

光罩加载（`LoadFog`, `:603-631`）：读取 `Data\lig0a.dat` ~ `lig0f.dat`，每个文件 4 字节宽 + 4 字节高 + w×h 字节 alpha 数据。

### 10.3 OpenScene / CloseScene

**OpenScene**（`:478-487`）：
1. `g_WMainImages.ClearCache` — 清主 UI 图像缓存
2. `FrmDlg.ViewBottomBox(TRUE)` — 显示底部 HUD 栏
3. `SetImeMode(FrmMain.Handle, LocalLanguage)` — 启用 IME

**CloseScene**（`:489-496`）：
1. `SilenceSound` — 停止所有音频
2. `EdChat.Visible := FALSE`
3. `FrmDlg.ViewBottomBox(FALSE)` — 隐藏底部 HUD 栏

### 10.4 PlayScene 12 步渲染管线（`:848-1419`）

#### 前置：计时 tick（`:885-896`）

- 移动 tick：每 100ms，`m_nMoveStepCount` 在 0/1 间交替
- 动画 tick：每 50ms，`m_nAniCount` 递增

#### 完整渲染顺序

| 步骤 | 目标 | 内容 | 位置 |
|------|------|------|------|
| 1 | m_MapSurface | 背景瓦片（隔格绘制，`g_WTilesImages[wBkImg-1]`，不透明） | `:550-573` |
| 2 | m_MapSurface | 中层瓦片（`g_WSmTilesImages[wMidImg-1]`，透明） | `:576-594` |
| 3 | m_ObjSurface | 视口裁剪 blit（SrcRect 含 ShiftX/Y 平滑滚动偏移） | `:1047-1053` |
| 4 | m_ObjSurface | 48×32 小前景物（仅尺寸恰为 48×32 的 `wFrImg`） | `:1064-1108` |
| 5 | m_ObjSurface | 地面魔法特效（`m_GroundEffectList`） | `:1110-1118` |
| 6 | m_ObjSurface | **逐行 Y-sort 大遍历**：大前景物 → 地图事件 → 地面物品 → 全部 Actor → 飞行特效 | `:1124-1249` |
| 7 | （光照） | 光源收集（地图格 `btLight` + Actor 光照） | `:1255-1288` |
| 8 | m_ObjSurface | 自身 + 焦点目标 + 魔法目标 **重绘到最上层** | `:1290-1316` |
| 9 | m_ObjSurface | Actor 武器特效 + 爆炸特效（`m_EffectList`） | `:1318-1345` |
| 10 | m_ObjSurface | 地面物品闪烁（`[410+FlashStep]`，5s 周期）+ 名称标签 | `:1347-1396` |
| 11 | MSurface | 迷雾/死亡灰度后处理 → blit m_ObjSurface → MSurface | `:1397-1412` |
| 12 | MSurface | 小地图（若 `g_boViewMiniMap`） | `:1414-1416` |

#### 步骤 3 视口裁剪细节

```
SrcRect = (UNITX*3 + ShiftX, UNITY*2 + ShiftY,
           UNITX*3 + ShiftX + 800, UNITY*2 + ShiftY + 445)
        = (144 + ShiftX, 64 + ShiftY, ...)
```

#### 步骤 6 Y-sort 遍历

对每行 `j`（从 `Top-BlockTop` 到 `Bottom-BlockTop+35`）：
- **大前景物**：非 48×32 的 `wFrImg`，混合对象用 `DrawBlend`（`:1129-1179`）
- **地图事件**：`EventMan.EventList` 中 `evn.m_nY == j` 的事件（`:1188-1195`）
- **地面物品**：`g_WDnItemImages[DropItem.Looks]`，焦点物品用 `g_ImgMixSurface` + `ceBright` 增亮（`:1197-1226`）
- **Actor**：所有 Actor 按 `m_nRy == j` 绘制，计算 `SayX/SayY` 气泡位置（`:1229-1240`）
- **飞行特效**：`m_FlyList` 中 `Ry == j` 的弹道（`:1241-1245`）

#### 步骤 8 自身重绘

自身 Actor 在 Y-sort 后**再次绘制**确保在最上层。同时重绘 `g_FocusCret`（鼠标悬停目标）和 `g_MagicTarget`（魔法目标）。

#### 步骤 10 物品闪烁

每 `g_dwDropItemFlashTime`（默认 5000ms）触发闪烁，持续 10 帧 × 20ms，图像 `g_WMainImages[FLASHBASE(410) + FlashStep]`，`DrawBlend` alpha=1。

名称标签：`BoldTextOut`，颜色从 `TShowItem.nFColor/nBColor` 取（默认白/黑），位置 `iy + HALFY - TextHeight*2`。

#### 步骤 11 后处理

- 死亡：`g_DeathColorEffect`（默认 `ceGrayScale`）应用到 `m_ObjSurface`
- 迷雾：`ApplyLightMap` → `DrawFog` → blit
- 正常：直接 blit `m_ObjSurface` → `MSurface` 于 `(0,0)`

### 10.5 小地图（DrawMiniMap, `:791-842`）

| 项目 | 值 |
|------|-----|
| 图像源 | `g_WMMapImages.Images[g_nMiniMapIndex]`（mmap.wil） |
| 视口 | 120×120 像素，以玩家为中心 |
| 屏幕位置 | `(SCREENWIDTH-120, 0)` = (680, 0) |
| 缩放 | 每地图格 = 1.5px 宽 × 1px 高（预缩放） |
| 玩家点 | 白色(255)，每 300ms 闪烁 |
| NPC/弓箭手 | 颜色 218（绿），2×2 像素块 |
| 玩家 | 颜色 255（白） |
| 怪物 | 颜色 249（红） |
| 扫描范围 | 玩家周围 21×21 格 |

**模式**（`g_nViewMinMapLv`）：0=不透明，1=半透明（`DrawBlendEx` mode=0），2=隐藏。

地图切换时 `g_nMiniMapIndex := -1` + 发送 `SendWantMiniMap` 请求新索引（`:2279-2283`）。

### 10.6 EdChat 聊天输入

**EdChatKeyPress**（`:427-440`）：
- Enter → `FrmMain.SendSay(EdChat.Text)` → 清空 → 隐藏
- Escape → 清空 → 隐藏

前缀解析（`!` 喊话、`@` GM、`/` 私聊等）在 `FrmMain.SendSay`（ClMain.pas）中处理，EdChat 仅传递原始文本。

显示/隐藏由 ClMain.pas 键盘处理器控制（Space/Enter/`!`/`@`/`/` 打开）。

### 10.7 坐标系统

#### Map → Screen（ScreenXYfromMCXY, `:1729-1739`）

```
sx = (cx - MySelf.Rx) * 48 + 364 + 24 - ShiftX
sy = (cy - MySelf.Ry) * 32 + 192 + 16 - ShiftY
```

#### Screen → Map（CXYfromMouseXY, `:1742-1752`）

```
ccx = Round((mx - 364 + ShiftX - UNITX) / UNITX) + Rx
ccy = Round((my - 192 + ShiftY - UNITY) / UNITY) + Ry
```

#### 内部渲染偏移（defx/defy, `:1059-1062`）

```
defx = -UNITX*2 - ShiftX + AAX + 14 = -66 - ShiftX
defy = -UNITY*2 - ShiftY = -64 - ShiftY
```

Actor 屏幕位置：`((Rx-Left)*UNITX + defx, (Ry-Top-1)*UNITY + defy)`（Y 方向 -1 因为精灵锚点在脚部）。

### 10.8 命中检测函数

| 函数 | 位置 | 算法 |
|------|------|------|
| `GetCharacter` | `:1755-1782` | 逐行倒序（ccy+8→ccy-1，优先近处），像素级 `actor.CheckSelect(x-dx, y-dy)` |
| `GetAttackFocusCharacter` | `:1840-1870` | 先 GetCharacter；无命中则回退包围盒（宽>40 裁剪两侧，高>70 裁剪上下） |
| `IsSelectMyself` | `:1820-1837` | 仅测试 g_MySelf，行范围 ccy+2→ccy-1 |
| `GetDropItems` | `:1840-1870` | 像素级非透明检测（`s.Pixels[dx,dy] <> 0`），构建 `\` 分隔的名称串 |
| `CanWalk` | `:1943-1948` | `Map.CanMove AND NOT CrashMan` |
| `CrashMan` | `:1950-1963` | 目标格有可见存活 Actor 则阻挡 |
| `CanRun` | `:1899-1915` | 检查中间步和终点 |

### 10.9 Actor 工厂（NewActor, `:2028-2149`）

根据 `RACEfeature(cfeature)` 创建对应 `TActor` 子类（约 50 种映射）：

| Race | 类 | 说明 |
|------|-----|------|
| 0 | THumActor | 人类玩家/NPC |
| 9 | TSoccerBall | 足球 |
| 13 | TKillingHerb | 杀人植物 |
| 14 | TSkeletonOma | 骷髅 |
| 15 | TDualAxeOma | 双斧战士 |
| 16 | TGasKuDeGi | 气体生物 |
| 17,19,30,31,74 | TCatMon | 猫型怪物 |
| 45 | TArcherMon | 弓箭手怪物 |
| 50 | TNpcActor | NPC |
| 81 | TAngel | 天使 |
| 83 | TFireDragon | 火龙 |
| 98 | TWallStructure | 城墙 |
| 99 | TCastleDoor | 城门 |
| 其他 | TActor | 通用 |

---

## 十一、主窗体结构（ClMain）

### 11.1 DFM 声明（ClMain.dfm）

`TfrmMain` / `frmMain`：`BorderStyle=bsNone`, `Caption='legend of mir'`, `Position=poDesktopCenter`, `KeyPreview=True`。

| 组件 | 类型 | 用途 |
|------|------|------|
| `DXDraw` | `TDXDraw` | `Align=alClient`，全屏渲染面 + 鼠标事件源。选项：`doAllowReboot, doWaitVBlank, doAllowPalette256, doSystemMemory, doCenter` |
| `CSocket` | `TClientSocket` | `ctNonBlocking`，网络连接 |
| `Timer1` | `TTimer` | 主定时器 |
| `MouseTimer` | `TTimer` | 鼠标轮询 + 自动攻击 |
| `WaitMsgTimer` | `TTimer` | 等待消息 |
| `SelChrWaitTimer` | `TTimer` | 选角等待 |
| `CmdTimer` | `TTimer` | 连接步骤状态机 |
| `MinTimer` | `TTimer` | 最小化 |
| `SpeedHackTimer` | `TTimer` | 加速检测 |
| `WMonImg..WMon28Img` | `TWMImages` ×30 | 怪物图像库 `Mon1.wil..Mon28.wil`，`LibType=ltUseCache` |
| `WEffectImg` | `TWMImages` | `Effect.wil` |
| `WDragonImg` | `TWMImages` | `Dragon.wil` |

**DFM 中无按钮/编辑框/面板** — 所有 UI 要么自建控件，要么代码创建。

### 11.2 FormCreate 初始化（ClMain.pas:435）

1. 读取 `Lmir.ini`（服务器地址/端口、全屏、字体）`:454-467`
2. 设 `DXDraw.Display.Width/Height := SCREENWIDTH/SCREENHEIGHT` `:495-496`
3. 创建 `DScreen` `:523`
4. 创建五个场景对象 `:525-533`
5. 创建 `Map` `:535`
6. 初始化全局物品数组 `:548-553`
7. 设 socket 地址 + `CSocket.Active:=True` `:651-684`
8. 挂 `Application.OnIdle := AppOnIdle` `:692`

### 11.3 DXDrawInitialize（ClMain.pas:853）

1. 设表面尺寸
2. 分配字体到 `PlayScene.EdChat` `:873`
3. 初始化所有 WIL 库
4. **`DScreen.Initialize` `:1007`**
5. **`PlayScene.Initialize` `:1008`**
6. **`FrmDlg.Initialize` `:1009`**（所有对话框定位）
7. 创建 `g_ImgMixSurface`（300×350）和 `g_MiniMapSurface`（540×360）`:1025-1030`

### 11.4 输入路由

所有输入事件**先过 `g_DWinMan`**，若返回 `True` 则 UI 已处理，跳过游戏逻辑：

| 事件 | 处理器 | UI 门 | 游戏逻辑 |
|------|--------|-------|----------|
| KeyPress | `FormKeyPress` `:1725` | `g_DWinMan.KeyPress` `:1727` | 聊天热键 Space/Enter/`!`/`@`/`/` `:1741-1783`；`1..6` 腰带 `:1734` |
| KeyDown | `FormKeyDown` `:1456` | `g_DWinMan.KeyDown` `:1471` | F9=背包, F10=状态, F11=魔法, F12=设置, F1-F8=施法 |
| MouseMove | `DXDrawMouseMove` `:2076` | `g_DWinMan.MouseMove` `:2083` | 焦点/目标/拖拽提示 `:2085-2116` |
| MouseDown | `DXDrawMouseDown` `:2120` | `g_DWinMan.MouseDown` `:2197` | 右键=转向/跑, 左键=攻击/NPC/拾取/走 |
| MouseUp | `DXDrawMouseUp` `:2384` | `g_DWinMan.MouseUp` `:2387` | 清除移动目标 |

### 11.5 全局场景对象（ClMain.pas:314-319）

```pascal
DScreen:           TDrawScreen;
IntroScene:        TIntroScene;
LoginScene:        TLoginScene;
SelectChrScene:    TSelectChrScene;
PlayScene:         TPlayScene;
LoginNoticeScene:  TLoginNotice;
```

---

## 十二、对话框系统（FState.pas / TFrmDlg）

### 12.1 控件清单

`TFrmDlg = class(TForm)` 声明约 **230 个自建控件**（`FState.pas:49-285`），全部使用 `D` 前缀，按功能分组。类型为 `TDWindow`（可拖动对话框容器）、`TDButton`（图像按钮）、`TDGrid`（物品网格）。

| 功能组 | 主要控件 |
|--------|----------|
| 状态/装备 | `DStateWin` + `DSW{Necklace,Light,ArmRingR/L,RingR/L,Weapon,Dress,Helmet,Bujuk,Belt,Boots,Charm}` + `DStMag1-5` |
| 背包 | `DItemBag`, `DItemGrid`, `DGold`, `DRepairItem`, `DCloseBag` |
| 底部 HUD | `DBottom`, `DMyState/Bag/Magic/Option`, `DBot{Group,Trade,MiniMap,Friend,Logout,Exit,Guild,PlusAbil,Memo}`, `DBelt1-6`, `DButtonHP/MP` |
| 登录流程 | `DLogIn` + `DLoginNew/Ok/Close/ChgPw`, `DNewAccount` + `DNewAccountOk/Cancel/Close`, `DChgPw` + `DChgpwOk/Cancel` |
| 选角 | `DSelectChr` + `Dsc{Select1,Select2,Start,NewChr,EraseChr,Credits,Exit}`, `DCreateChr` + `Dcc{Warrior,Wizzard,Monk,Male,Female,LeftHair,RightHair,Ok,Close}` |
| 选服 | `DSelServerDlg` + `DSServer1-6` + `DSSrvClose` + `DEngServer1` |
| 消息对话框 | `DMsgDlg` + `DMsgDlg{Ok,Yes,No,Cancel}` |
| NPC/商店 | `DMerchantDlg`, `DMenuDlg` + `DMenu{Prev,Next,Buy,Close}`, `DSellDlg` + `DSellDlg{Ok,Close,Spot}` |
| 魔法键位 | `DKeySelDlg` + `DKs{Icon,F1-F8,None,Ok}` + `DKsConF1-F8` |
| 组队 | `DGroupDlg` + `DGrp{AllowGroup,DlgClose,Create,AddMem,DelMem}` |
| 交易 | `DDealDlg`/`DDealRemoteDlg` + `DDGrid/DDRGrid` + `DDeal{Ok,Close}` + `DDGold/DDRGold` |
| 行会 | `DGuildDlg` + `DGD{Home,List,Chat,AddMem,DelMem,EditNotice,EditGrade,Ally,BreakAlly,War,CancelWar,Up,Down,Close}`, `DGuildEditNotice` + `DGE{Close,Ok}` |
| 加点 | `DAdjustAbility` + `DPlus{DC,MC,SC,AC,MAC,HP,MP,Hit,Speed}` + `DMinus{...}` + `DAdjustAbil{Close,Ok}` |
| 社交 | `DFriendDlg`, `DMailListDlg`, `DMailDlg`, `DBlockListDlg`, `DMemo`, `DConfigDlg`, `DChgGamePwd` |

### 12.2 DlgConf 布局表（MShare.pas:468-541）

`TControlInfo` 记录（`MShare.pas:35-42`）：

```pascal
TControlInfo = record
    Image, Left, Top, Width, Height: Integer;
    Obj: TDControl;
end;
```

`TConfig` 记录包含约 80 个 `TControlInfo` 字段，覆盖每个对话框/按钮的默认几何。关键布局值见第十一章速查表。

### 12.3 Initialize 运行时布局（FState.pas:722-1764）

所有几何在运行时赋值，`.dfm` 中不存储有意义的坐标：

1. `g_DWinMan.ClearAll`
2. `DBackground` 设为全屏 + `AddDControl(DBackground, TRUE)`（`:727-734`）
3. 逐控件调用 `SetImgIndex(g_WMainImages, <idx>)` + 设 `Left/Top/Width/Height`

关键锚点：

| 控件 | WIL 索引 | 位置 |
|------|----------|------|
| `DMsgDlg` | 360 | 居中（`:739-752`） |
| 登录按钮 | 61/62/53/64 | `y=558`（`:763-774`） |
| `DSelServerDlg` | 160 | + 6 个服务器按钮（`:779-857`） |
| `DNewAccount` | 63 | （`:862-876`） |
| `DChgPw` | 50 | （`:881-892`） |
| `DSelectChr` | 全屏 | + `Dsc*`（`:900-925`） |
| `DCreateChr` | 73 | + `Dcc*`（`:930-965`） |
| `DStateWin` | 370 | 右对齐 + `DSW*` 槽（`:981-1084`） |
| `DItemBag` | 3 | + `DItemGrid(33,43,286×162)`（`:1167-1174`） |
| `DBottom` | 1 | （`:1179-1189`） |
| `DMenuDlg` | 385 | （`:1324-1341`） |
| `DSellDlg` | 392 | （`:1346-1361`） |
| `DKeySelDlg` | 229 | + `DKsF1-F8`（`:1367-1404`） |
| `DGroupDlg` | 120 | （`:1435-1455`） |
| `DDealDlg` / `DDealRemoteDlg` | 389 / 390 | （`:1459-1491`） |
| `DGuildDlg` | 180 | （`:1495-1543`） |
| `DAdjustAbility` | 226 | （`:1557-1619`） |
| `DFriendDlg` / `DMailListDlg` / `DBlockListDlg` / `DMemo` | 456/457/458/459 | （`:1621-1725`） |

### 12.4 模态对话框（DMessageDlg, FState.pas:1938-2128）

三种尺寸通过 `DialogSize` 选择：

| 尺寸 | WIL 索引 | 用途 |
|------|----------|------|
| 0=小 | 381 | 简单确认 |
| 1=宽 | 360 | 标准对话框 |
| 2=高 | 380 | 长文本 |

- 按钮 `Ok/Yes/No/Cancel` 从 `lx=324(XBase), ly=126` 起右到左排列，间距 110px（`:2060-2083`）
- `HideAllControls`（`:695-710`）隐藏已打开的编辑框，然后 `DMsgDlg.ShowModal` + 手动消息泵（`FrmMain.ProcOnIdle; Application.ProcessMessages` `:2098-2117`）保持模态期间渲染
- `mbAbort` 添加输入编辑框（`EdDlgEdit`）用于金币数量等（`:2086-2095`）
- 键盘：Enter=Ok/Yes, Esc=Cancel（`DMsgDlgKeyDown :2139-2158`）
- 包含骰子动画小游戏（`:1945-1994, 2104-2113`）

### 12.5 FState.dfm 结构

- 根控件 `TDWindow DBackground`（`Align=alClient`，`OnBackgroundClick=DBackgroundBackgroundClick`）
- 每个对话框是 `TDWindow`，`DParent=DBackground`，`Floating=True`
- 每个按钮 `DParent=<所属对话框>`
- 事件绑定：`OnDirectPaint`（自定义绘制）、`OnClick`、`OnClickSound`（选择点击音效）、`ClickCount`
- 许多处理器是**共享的**（如 `DMsgDlgOkDirectPaint` 绘制所有服务器按钮；`DccCloseDirectPaint` 绘制大多数通用按钮），通过 `Sender` 分发
- `FormCreate`（`FState.pas:623-680`）还在 `FrmMain` 上创建两个代码挂载控件：`EdDlgEdit:TEdit`（`:656-667`，模态文本输入）和 `Memo:TMemo`（`:669-678`）

---

## 十三、HUD 布局（游戏内）

### 13.1 底部面板（DBottom）

`DBottomDirectPaint`（`FState.pas:3560-3708`）：

| 元素 | WIL 索引 | 位置/行为 |
|------|----------|-----------|
| 背景 | `BOTTOMBOARD800=1` / `BOTTOMBOARD1024=2`（`:11-12`） | `Top := SCREENHEIGHT - height`；上 120px 透明绘制，下半不透明（`:3578-3594`） |
| HP/MP 球（战士<28级） | `[5]` 底 + `[6]` 填充 | `x=38, btop+90`，单球（`:3606-3620`） |
| HP/MP 球（其他） | `[4]` | HP 左半 `x=40`，MP 右半，`btop+91`，按 `MaxHP/HP` 计算填充（`:3622-3639`） |
| 等级 | `PomiTextOut` 位图数字 | 右侧集群（`:3643`） |
| 经验条 | `[7]` | `SCREENHEIGHT-73`，按 `Exp/MaxExp` 裁剪（`:3646-3661`） |
| 重量条 | `[7]` | `SCREENHEIGHT-40`，按 `Weight/MaxWeight` 裁剪（`:3663-3676`） |
| 昼夜图标 | `[12-15]` | 按 `g_nDayBright` 切换太阳/月亮（`:3597-3604`） |
| 饥饿指示 | `[16-19]` | 右侧（`:3682-3688`） |
| 聊天区 | — | 9 行（`VIEWCHATLINE=9`，`:13`），起点 `sx=208, sy=SCREENHEIGHT-130`，12px 行高，逐行前景/背景色（`:3692-3705`） |

HP/MP 可点击覆盖：`DButtonHP` 在 `(40,91,45×90)`，`DButtonMP` 在 `(87,91,45×90)`（`:1727-1735`）。

聊天存储在 `TDrawScreen.ChatStrs`（`DrawScrn.pas:30,147-188`），上限 200 行。点击聊天行打开私聊 — `DBottomMouseDown`（`FState.pas:1896-1927`）。

### 13.2 腰带（DBelt1-6）

`FState.pas:110-115`，初始化 `:1245-1273`：

- 每格 `32×29px`，`Top=59`
- 起始 `Left=SCREENWIDTH/2-115`，间距 43px
- 热键 `1..6` 消费腰带物品（`ClMain.pas:1734-1737`）
- 悬停 tooltip：`DBelt1MouseMove`（`FState.pas:3855-3866`）
- 双击使用：`DBelt1DblClick`（`:3902-3920`）

### 13.3 功能按钮

#### 右侧斜坡（4 个状态按钮）

| 按钮 | WIL 索引 | 位置 |
|------|----------|------|
| `DMyState`（状态） | 8 | `Left:643, Top:61` |
| `DMyBag`（背包） | 9 | `Left:682, Top:41` |
| `DMyMagic`（魔法） | 10 | `Left:722, Top:21` |
| `DOption`（设置） | 11 | `Left:764, Top:11` |

（`FState.pas:1194-1205`，`MShare.pas:470-473`）

#### 底部工具栏（9 个按钮，Top:104，30px 间距）

| 按钮 | WIL 索引 | Left |
|------|----------|------|
| `DBotMiniMap` | 130 | 219 |
| `DBotTrade` | 132 | 249 |
| `DBotGuild` | 134 | 279 |
| `DBotGroup` | 128 | 309 |
| `DBotPlusAbil` | 140 | 339 |
| `DBotFriend` | 530 | 369 |
| `DBotMemo` | 532 | 计算值 |
| `DBotExit` | 138 | 计算值 |
| `DBotLogout` | 136 | 计算值 |

（`FState.pas:1210-1239`，`MShare.pas:474-494`）

### 13.4 角色状态窗口（DStateWin）

图像 370，`Left:=SCREENWIDTH-width, Top:=0`（`FState.pas:981-986`）。

**4 页**（`StatePage`，`MAXSTATEPAGE=4`），`DStateWinDirectPaint`（`:2788-3039`）：

| 页 | 内容 | 位置 |
|----|------|------|
| 0 | 纸娃娃/装备（头发/衣服/武器/头盔层） | `:2805-2854` |
| 1 | 基础属性（AC/MAC/DC/MC/SC/HP/MP） | `:2855-2870` |
| 2 | 详细属性（经验%/重量/命中/攻速/抗性） | `:2871-2930` |
| 3 | 魔法列表（图标+名称/等级/训练进度/快捷键） | `:2931-2998` |

装备槽 `DSW*` 有精确位置/尺寸（`:987-1084`）；页导航 `DStPageUp/Down`，关闭 `DCloseState`，上/下一个 `DPrevState/DNextState`（`:1069-1084`）。

`DUserState1`（`:1089-1162`）是"查看其他玩家"变体，停靠在左侧一个窗口宽度处。

### 13.5 世界覆盖层（TDrawScreen 绘制）

| 覆盖 | WIL 源 | 行为 |
|------|--------|------|
| Actor 血条 | `g_WMain2Images` `[0]` 黑底 / `[1]` 红条 | 头顶，按 HP/MaxHP 缩放（`DrawScrn.pas:280-301`） |
| 焦点/自身名字 | — | 描边文字（`:307-325`） |
| 语音气泡 | — | 4 秒过期，死亡灰色（`:330-352`） |
| 区域状态图标 | `[150+]` | 右上角（`:377-386`） |
| 系统消息 | — | 左上 `(30,40)`，绿色，3s 过期（`:392-415`） |
| Tooltip | `[394]` | 鼠标跟随，`DrawBlendEx` 背景（`:417-447`） |

### 13.6 小地图

`TPlayScene.DrawMiniMap`（`PlayScn.pas:791`），当 `g_boViewMiniMap` 时调用（`:1414-1416`）；渲染到 `g_MiniMapSurface` 540×360（`ClMain.pas:1028-1030`）。

---

## 十四、全局状态与常量

### 14.1 屏幕/视口常量（Share.pas）

| 常量 | 值 | 位置 |
|------|-----|------|
| `SWH800` / `SWH1024` / `SWH` | `0 / 1 / SWH800` | `:16-18` |
| `SCREENWIDTH` | `800`（或 1024） | `:23-28` |
| `SCREENHEIGHT` | `600`（或 768） | `:24,27` |
| `MAPSURFACEWIDTH` | `= SCREENWIDTH`（800） | `:30` |
| `MAPSURFACEHEIGHT` | `= SCREENHEIGHT - 155` → **445** | `:31` |
| `WINLEFT` / `WINTOP` | `60 / 60` | `:33-34` |
| `WINRIGHT` | `SCREENWIDTH - 60`（740） | `:35` |
| `BOTTOMEDGE` | `SCREENHEIGHT - 30`（570） | `:36` |
| `MAXX` / `MAXY` | `SCREENWIDTH div 20` = **40** | `:81-82` |
| `MAXBAGITEMCL` | `52` | `:89` |
| `MAXFONT` | `8` | `:90` |
| `ENEMYCOLOR` | `69`（调色板索引） | `:91` |

瓦片几何（`Grobal2.pas`）：`UNITX=48`, `UNITY=32`（`:45-46`）；`HALFX=24`, `HALFY=16`（`:48-49`）。

WIL 资源路径（`Share.pas:42-75`）：

| 常量 | 文件 | 用途 |
|------|------|------|
| `MAINIMAGEFILE` | `Data\Prguse.wil` | 主 UI 精灵 |
| `MAINIMAGEFILE2/3` | `Prguse2/3.wil` | 辅助 UI |
| `CHRSELIMAGEFILE` | `Data\ChrSel.wil` | 选角场景 |
| `MINMAPIMAGEFILE` | `Data\mmap.wil` | 小地图 |
| `BAGITEMIMAGESFILE` | `Data\Items.wil` | 背包物品图标 |
| `STATEITEMIMAGESFILE` | `Data\StateItem.wil` | 装备槽图标 |
| `MAGICONIMAGESFILE` | `Data\MagIcon.wil` | 技能图标 |

### 14.2 UI 全局变量（MShare.pas）

#### 渲染/窗口管理对象

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_DXDraw` | `:150` | DirectX 绘制组件 |
| `g_DWinMan` | `:151` | 窗口/控件管理器 |
| `g_ImgMixSurface` | `:232` | 图像合成离屏表面 |
| `g_MiniMapSurface` | `:233` | 小地图离屏表面 |

#### WIL 图像库（`:155-175`）

`g_WMainImages / g_WMain2Images / g_WMain3Images`（主 UI），`g_WChrSelImages`（选角），`g_WMMapImages`（小地图），`g_WTilesImages / g_WSmTilesImages`（地图瓦片），`g_WHumWingImages`（翅膀），`g_WBagItemImages`（背包图标），`g_WStateItemImages`（装备图标），`g_WDnItemImages`（地面物品），`g_WHumImgImages / g_WHairImgImages / g_WWeaponImages`（角色外观），`g_WMagIconImages`（技能图标），`g_WNpcImgImages`（NPC），`g_WMagicImages / g_WMagic2Images`（魔法特效），`g_WEventEffectImages`（事件特效），`g_WObjectArr[0..14]`（对象），`g_WMonImagesArr[0..9999]`（怪物，懒加载）。

#### 连接/场景状态

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_ConnectionStep` | `:211` | `cnsLogin/cnsSelChr/cnsReSelChr/cnsPlay` |
| `g_ChrAction` | `:210` | `caWalk/caRun/caHorseRun/caHit/caSpell/caSitdown` |
| `g_SoftClosed` | `:209` | 软关闭/最小化 |
| `g_boServerConnected` | `:208` | 服务器已连接 |
| `g_boBagLoaded` | `:327` | 背包已加载 |

#### 显示/渲染开关

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_boFullScreen` | `:229` | 全屏模式（默认 True） |
| `g_boViewFog` | `:370` | 显示迷雾/黑暗 |
| `g_boNoDarkness` | `:375` | 无黑暗 |
| `g_nDayBright` | `:372` | 昼夜亮度 |
| `g_boDrawTileMap` | `:463` | 绘制地图瓦片（默认 True） |
| `g_boDrawDropItem` | `:464` | 绘制地面物品（默认 True） |
| `g_DeathColorEffect` | `:425` | 死亡屏幕色调（默认灰度） |
| `g_boDoFadeOut/In` | `:388-391` | 屏幕淡入淡出 |
| `g_boViewMiniMap` | `:291` | 显示小地图 |
| `g_nViewMinMapLv` | `:292` | 小地图模式（0=普通,1=透明,2=不透明） |

#### 提示/信息显示标志

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_boShowHumanInfo` | `:444` | 显示人物名字（默认 True） |
| `g_boShowMonsterInfo` | `:445` | 显示怪物名字（默认 False） |
| `g_boShowRedHPLable` | `:437` | 显示血条（默认 True） |
| `g_boShowHPNumber` | `:438` | 显示 HP 数字（默认 True） |
| `g_boShowJobLevel` | `:439` | 显示职业+等级（默认 True） |
| `g_boShowAllItem` | `:461` | 显示所有地面物品名（默认 False） |

#### 鼠标/目标状态

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_nMouseCurrX/Y` | `:271-272` | 鼠标地图格坐标 |
| `g_nMouseX/Y` | `:273-274` | 鼠标屏幕像素坐标 |
| `g_TargetCret` | `:278` | 目标 Actor |
| `g_FocusCret` | `:279` | 焦点 Actor |
| `g_MagicTarget` | `:280` | 魔法目标 |

#### NPC/交易/拖拽状态

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_nCurMerchant` | `:296` | 商人对话框类型 |
| `g_SellDlgItem` | `:347` | 出售位物品 |
| `g_DealItems[0..9]` | `:355` | 交易本方物品 |
| `g_DealRemoteItems[0..19]` | `:356` | 交易对方物品 |
| `g_MouseItem` | `:361` | 光标下物品 |
| `g_boItemMoving` | `:365` | 物品拖拽中 |
| `g_MovingItem` | `:366` | 拖拽物品数据 |

#### 玩家数据（UI 面板显示）

| 变量 | 位置 | 说明 |
|------|------|------|
| `g_MySelf` | `:322` | 自身角色数据 |
| `g_UseItems[0..12]` | `:324` | 已装备物品 |
| `g_ItemArr[0..51]` | `:325` | 背包物品 |
| `g_nBonusPoint` | `:249` | 可分配点数 |
| `g_BonusAbil` | `:251` | 加点属性 |
| `g_sGuildName` | `:256` | 行会名 |
| `g_MagicList` | `:240` | 魔法列表 |
| `g_GroupMembers` | `:241` | 组队成员 |

### 14.3 协议常量（Grobal2.pas，UI 相关子集）

#### 客户端 → 服务端（CM_）

| 常量 | 值 | 用途 |
|------|-----|------|
| `CM_QUERYCHR` | 100 | 查询角色 |
| `CM_NEWCHR` | 101 | 创建角色 |
| `CM_DELCHR` | 102 | 删除角色 |
| `CM_SELCHR` | 103 | 选择角色 |
| `CM_DROPITEM` | 1000 | 丢弃物品 |
| `CM_PICKUP` | 1001 | 拾取物品 |
| `CM_TAKEONITEM` | 1003 | 穿戴装备 |
| `CM_TAKEOFFITEM` | 1004 | 脱下装备 |
| `CM_EAT` | 1006 | 使用物品 |
| `CM_MAGICKEYCHANGE` | 1008 | 修改魔法键位 |
| `CM_CLICKNPC` | 1010 | 点击 NPC |
| `CM_MERCHANTDLGSELECT` | 1011 | NPC 对话框选择 |
| `CM_USERSELLITEM` | 1013 | 出售物品 |
| `CM_USERBUYITEM` | 1014 | 购买物品 |
| `CM_USERGETDETAILITEM` | 1015 | 查询物品详情 |
| `CM_GROUPMODE` | 1019 | 组队模式 |
| `CM_CREATEGROUP` | 1020 | 创建组队 |
| `CM_ADDGROUPMEMBER` | 1021 | 添加队员 |
| `CM_DELGROUPMEMBER` | 1022 | 删除队员 |
| `CM_USERREPAIRITEM` | 1023 | 修理物品 |
| `CM_DEALTRY..CM_DEALEND` | 1025-1030 | 交易流程 |
| `CM_USERSTORAGEITEM` | 1031 | 仓库存入 |
| `CM_USERTAKEBACKSTORAGEITEM` | 1032 | 仓库取出 |
| `CM_WANTMINIMAP` | 1033 | 请求小地图 |
| `CM_OPENGUILDDLG` | 1035 | 打开行会对话框 |
| `CM_IDPASSWORD` | 2001 | 登录 |
| `CM_ADDNEWUSER` | 2002 | 注册 |
| `CM_CHANGEPASSWORD` | 2003 | 改密码 |

#### 服务端 → 客户端（SM_）

| 常量 | 值 | 用途 |
|------|-----|------|
| `SM_SYSMESSAGE` | 100 | 系统消息 |
| `SM_WHISPER` | 103 | 私聊 |
| `SM_ADDITEM` | 200 | 添加物品 |
| `SM_BAGITEMS` | 201 | 背包列表 |
| `SM_DELITEM` | 202 | 删除物品 |
| `SM_UPDATEITEM` | 203 | 更新物品 |
| `SM_ADDMAGIC` | 210 | 添加魔法 |
| `SM_SENDMYMAGIC` | 211 | 魔法列表 |
| `SM_CERTIFICATION_SUCCESS` | 500 | 认证成功 |
| `SM_QUERYCHR` | 520 | 角色查询结果 |
| `SM_STARTPLAY` | 525 | 开始游戏 |
| `SM_ABILITY` | 52 | 属性更新 |
| `SM_HEALTHSPELLCHANGED` | 53 | HP/MP 变化 |
| `SM_MAPDESCRIPTION` | 54 | 地图描述 |
| `SM_ITEMSHOW` | 610 | 地面物品显示 |
| `SM_ITEMHIDE` | 611 | 地面物品隐藏 |
| `SM_TAKEON_OK` | 615 | 穿戴成功 |
| `SM_TAKEOFF_OK` | 619 | 脱下成功 |
| `SM_SENDUSEITEMS` | 621 | 装备列表 |
| `SM_WEIGHTCHANGED` | 622 | 重量变化 |
| `SM_CHANGEMAP` | 634 | 切换地图 |
| `SM_MERCHANTSAY` | 643 | NPC 对话 |
| `SM_MERCHANTDLGCLOSE` | 644 | NPC 关闭 |
| `SM_SENDGOODSLIST` | 645 | 商品列表 |
| `SM_GOLDCHANGED` | 653 | 金币变化 |
| `SM_CHANGELIGHT` | 654 | 光照变化 |
| `SM_SENDNOTICE` | 658 | 公告 |
| `SM_GROUPMODECHANGED` | 659 | 组队模式变化 |
| `SM_GROUPMEMBERS` | 667 | 队员列表 |
| `SM_DEALMENU` | 673 | 交易菜单 |
| `SM_DEALSUCCESS` | 687 | 交易成功 |
| `SM_SENDUSERSTORAGEITEM` | 700 | 仓库物品 |
| `SM_SAVEITEMLIST` | 704 | 寄存列表 |
| `SM_READMINIMAP_OK` | 710 | 小地图数据 |
| `SM_OPENGUILDDLG` | 753 | 行会对话框 |
| `SM_DLGMSG` | 772 | 对话框消息 |
| `SM_SHOWEVENT` | 804 | 显示事件 |
| `SM_HIDEEVENT` | 805 | 隐藏事件 |
| `SM_OPENHEALTH` | 1100 | 打开血条 |
| `SM_CLOSEHEALTH` | 1101 | 关闭血条 |

### 14.4 UI 显示数据结构（Grobal2.pas）

| 结构 | 位置 | 内容 |
|------|------|------|
| `TStdItem` | `:536-555` | 物品定义（60 字节）：Name, StdMode, Shape, Weight, AniCount, **Looks**(Items.wil 索引), DuraMax, AC/MAC/DC/MC/SC, Need, NeedLevel, Price |
| `TClientItem` | `:558-563` | 物品实例：S:TStdItem + MakeIndex + Dura + DuraMax |
| `TUserStateInfo` | `:572-579` | 查看面板：Feature, UserName, GuildName, GuildRankName, NameColor, UseItems[0..12] |
| `TUserCharacterInfo` | `:580-586` | 选角行：Name, Job, Hair, Level, Sex |
| `TUserEntry` / `TUserEntryAdd` | `:587-604` | 注册表单字段 |
| `TDropItem` | `:615-625` | 地面物品：X, Y, Id, Looks, Name, FlashTime... |
| `TClientMagic` | `:629-652` | 魔法面板：Key, Level, CurTrain, Def{name, spell, power, TrainLevel[], dwDelayTime} |
| `TAbility` | `:731-750` | 状态面板：Level, AC, MAC, DC, MC, SC, HP, MP, MaxHP, MaxMP, Exp, MaxExp, Weight, MaxWeight... |
| `TNakedAbility` | `:657-668` | 加点面板：DC, MC, SC, AC, MAC, HP, MP, Hit, Speed |
| `TClientGoods` | `:718-724` | 商品列表：Name, SubMenu, Price, Stock, Grade |
| `TMsgColor` | `:1425` | `c_Red, c_Green, c_Blue, c_White` |
| `TMsgType` | `:1426` | `t_System, t_Notice, t_Hint, t_Say, t_Castle, t_Cust, t_GM, t_Mon` |

装备槽索引（`Grobal2.pas:26-38`）：

```
U_DRESS=0, U_WEAPON=1, U_RIGHTHAND=2, U_NECKLACE=3, U_HELMET=4,
U_ARMRINGR=5, U_ARMRINGL=6, U_RINGR=7, U_RINGL=8, U_BUJUK=9,
U_BELT=10, U_BOOTS=11, U_CHARM=12
```

---

## 十五、关键常量速查表

### 15.1 Prguse.wil 常用索引

| 索引 | 用途 | 引用 |
|------|------|------|
| 1 | 底部面板背景（800px） | `FState.pas:11` |
| 2 | 底部面板背景（1024px） | `FState.pas:12` |
| 3 | 背包背景 | `FState.pas:1167` |
| 4 | HP/MP 双球 | `FState.pas:3622` |
| 5-6 | 战士 HP 单球（底/填充） | `FState.pas:3606` |
| 7 | 经验条/重量条 | `FState.pas:3646` |
| 8-11 | 状态/背包/魔法/设置按钮 | `MShare.pas:470-473` |
| 12-15 | 昼夜图标（太阳/月亮） | `FState.pas:3597` |
| 16-19 | 饥饿指示 | `FState.pas:3682` |
| 26 | 修理按钮 | `MShare.pas:502` |
| 29 | 金币按钮 | `MShare.pas:501` |
| 50 | 改密码对话框 | `FState.pas:881` |
| 53 | 登录按钮 | `FState.pas:763` |
| 61-64 | 登录流程按钮 | `FState.pas:763-774` |
| 63 | 新账号对话框 | `FState.pas:862` |
| 64 | 通用关闭按钮（复用） | 多处 |
| 65 | 选角背景 | `IntroScn.pas:1363` |
| 73 | 创角对话框 | `FState.pas:930` |
| 120 | 组队对话框 | `FState.pas:1435` |
| 128-140 | 底部工具栏按钮 | `MShare.pas:474-494` |
| 160 | 选服对话框 | `FState.pas:779` |
| 180 | 行会对话框 | `FState.pas:1495` |
| 204 | 配置对话框 | `MShare.pas:506` |
| 226 | 加点对话框 | `FState.pas:1557` |
| 229 | 键位选择对话框 | `FState.pas:1367` |
| 232-246 | F1-F8 键位按钮（偶数） | `MShare.pas:520-527` |
| 256 | 英文选服 | `FState.pas:779` |
| 360 | 消息对话框（宽） | `FState.pas:2002` |
| 370 | 角色状态窗口 | `FState.pas:981` |
| 371 | 关闭按钮（背包/状态） | `FState.pas:1076` |
| 376/377 | 纸娃娃体型（男/女） | `FState.pas:2805` |
| 380 | 消息对话框（高） | `FState.pas:2002` |
| 381 | 消息对话框（小） | `FState.pas:2002` |
| 384 | NPC 对话背景 | `FState.pas:1297` |
| 385 | 商店购买面板 | `FState.pas:1324` |
| 386-388 | 购买/上一页/下一页按钮 | `MShare.pas:510-512` |
| 389/390 | 交易本方/对方面板 | `FState.pas:1459-1491` |
| 391 | 交易确认按钮 | `FState.pas:1469` |
| 392 | 出售面板 | `FState.pas:1346` |
| 393 | 出售确认按钮 | `MShare.pas:515` |
| 394 | Tooltip 背景 | `DrawScrn.pas:426` |
| 440+ | 纸娃娃发型 | `FState.pas:2805` |
| 456-459 | 好友/邮件/屏蔽/备忘 | `FState.pas:1621-1725` |
| 530-532 | 好友/备忘按钮 | `MShare.pas:479-480` |
| 620-640 | 键位对话框按钮 | `MShare.pas:518-537` |

### 15.2 其他 WIL 文件

| 文件 | 变量 | 用途 |
|------|------|------|
| `Prguse2.wil` | `g_WMain2Images` | 血条 [0] 黑底 / [1] 红条 |
| `Prguse3.wil` | `g_WMain3Images` | 辅助 UI（未广泛使用） |
| `ChrSel.wil` | `g_WChrSelImages` | 选角场景 [65] 背景、[80+] 登录背景、[103+] 门动画 |
| `Items.wil` | `g_WBagItemImages` | 背包/腰带/仓库物品图标（by `Looks`） |
| `StateItem.wil` | `g_WStateItemImages` | 装备槽图标（by `Looks`） |
| `DnItems.wil` | `g_WDnItemImages` | 地面物品图标 |
| `MagIcon.wil` | `g_WMagIconImages` | 技能栏/魔法页图标 |
| `mmap.wil` | `g_WMMapImages` | 小地图 |

### 15.3 屏幕/网格/槽位常量

| 常量 | 值 | 位置 |
|------|-----|------|
| 屏幕 | 800×600 | `Share.pas:23-24` |
| 游戏视口高度 | 445（600-155） | `Share.pas:31` |
| 窗口拖动钳制 | [60..740]×[60..570] | `Share.pas:33-36` |
| 背包格数 | 52（`MAXBAGITEMCL`） | `Share.pas:89` |
| 装备槽数 | 13（0..12） | `Grobal2.pas:26-38` |
| 仓库格数 | 50 | `Grobal2.pas:796` |
| 组队上限 | 11（`GROUPMAX`） | `Grobal2.pas:1036` |
| 魔法上限 | 20（`HOWMANYMAGICS`） | `Grobal2.pas:52` |
| 技能等级上限 | 3（`MaxSkillLevel`） | `Grobal2.pas:54` |
| Grid 默认 | 8列×5行, 36×32px | `DWinCtl.pas:702-705` |
| 腰带格 | 6 格, 32×29px, 43px 间距 | `FState.pas:1245-1273` |
| 聊天行 | 9 行（`VIEWCHATLINE`）, 12px 行高 | `FState.pas:13` |
| 系统消息上限 | 8 行（`MAXSYSLINE`） | `DrawScrn.pas:12` |
| 血条精灵 | 黑底=0, 红条=1 | `DrawScrn.pas:17-18` |
| 状态图标基址 | 150（`AREASTATEICONBASE`） | `DrawScrn.pas:16` |
| 聊天上限 | 200 行 | `DrawScrn.pas:147` |
| 气泡超时 | 4 秒 | `DrawScrn.pas:331` |
| 系统消息超时 | 3000ms | `DrawScrn.pas:409` |
| 色键透明值 | 0（`TRANSPARENCY_VALUE`） | `cliUtil.pas:1040` |

### 15.4 字体配置（MShare.pas:216-228）

8 种字体：宋体、黑体、楷体、仿宋、Courier New、Arial、MS Sans Serif、Microsoft Sans Serif。默认 `g_nCurFont=0`（宋体）。

---

## 附录：文件索引

| 文件 | 行数 | 内容 |
|------|------|------|
| `DWinCtl.pas` | ~1130 | 控件框架（TDControl/TDButton/TDWindow/TDGrid/TDWinManager） |
| `DrawScrn.pas` | ~520 | 场景调度、屏幕合成、HUD 覆盖 |
| `ClMain.pas` | ~6500 | 主窗体、渲染循环、网络消息分发、输入路由 |
| `ClMain.dfm` | 二进制 | 主窗体 DFM（DXDraw/Socket/Timer/WIL 库） |
| `FState.pas` | ~6831 | 对话框系统（~230 控件、布局、交互） |
| `FState.dfm` | 二进制 | 对话框 DFM（控件树、事件绑定） |
| `IntroScn.pas` | ~1550 | 场景状态机、登录/选角场景 |
| `PlayScn.pas` | ~2366 | 游戏场景渲染、小地图 |
| `MShare.pas` | ~540+ | 客户端全局状态、DlgConf 布局表 |
| `Share.pas` | ~95 | 屏幕/视口常量、WIL 路径 |
| `Grobal2.pas` | ~2739 | 协议常量、数据结构 |
| `WIL.pas` | ~500+ | WIL 图像库加载器 |
| `DIB.pas` | ~240 | Device Independent Bitmap |
| `DXDraws.pas` | ~2000+ | DirectX 表面、混合绘制 |
| `cliUtil.pas` | ~1040+ | 调色板混合 LUT、MMX blit |
| `wmUtil.pas` | ~65 | WIL 数据结构 |
| `ClFunc.pas` | ~620 | BoldTextOut 等绘制辅助 |
