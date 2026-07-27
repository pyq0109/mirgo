# Delphi 版声音系统实现

> 基于 `asset/delphi/Client/` 源码（commit `98711da`）的完整技术描述。
> 所有引用格式为 `文件:行号`。

## 一、架构总览

MIR2 客户端的声音系统是**双通道、分层门面**结构：全部 WAV 音效与循环 BGM 走 DirectSound 通道，MP3 地图音乐走 DirectShow 通道，两条通道互不相通，停止逻辑各自独立。

```
┌──────────────────────────────────────────────────────────────────────┐
│  触发层（游戏逻辑）                                                    │
│    TActor.SetSound / RunSound / RunActSound        (Actor.pas)        │
│    TPlayScene / TLoginScene / TSelectChrScene  (PlayScn/IntroScn.pas) │
│    TFrmDlg UI 事件                                (FState.pas)        │
│    TfrmMain 消息分发 / DrawEffectHum              (ClMain.pas)        │
│    特化怪物子类                                   (AxeMon.pas)        │
│    魔法特效                                       (magiceff.pas)      │
├──────────────────────────────────────────────────────────────────────┤
│  门面层 SoundUtil.pas                                                  │
│    PlaySound(idx)      按索引播一次性音效                              │
│    PlayBGM(wav)        循环播 WAV 背景乐                               │
│    PlayMp3(name,flag)  MP3 播放/停止                                   │
│    PlayMapMusic(flag)  按地图编号播 .\Music\<n>.mp3                    │
│    SilenceSound        g_Sound.Clear 全停（停不掉 MP3）                │
│    ItemClickSound / ItemUseSound   按物品 StdMode 发声                 │
│    g_SoundList         索引→文件表（.\wav\sound.lst）                  │
├──────────────────────────────┬───────────────────────────────────────┤
│  通道 A：DirectSound          │  通道 B：DirectShow                    │
│  TSoundEngine (DXSounds.pas) │  TMPEG (Mpeg.pas)                      │
│   - WAV 一次性音效（可叠加）  │   - 地图音乐 .\Music\<n>.mp3           │
│   - 循环 WAV BGM              │   - COM FilterGraph + RenderFile       │
│   - 全内存、无缓存、无上限    │   - 异步、不可循环、音量不可调         │
│  TDirectSoundBuffer          │  IGraphBuilder / IMediaControl         │
│  TDirectSound → DSound.dll   │  → quartz.dll                          │
├──────────────────────────────┴───────────────────────────────────────┤
│  WAV 解析 Wave.pas：TWave / TWaveStream（RIFF 解析，全量进内存）       │
│  DShow.pas：DirectShow COM 接口声明（纯 SDK 头，无业务逻辑）           │
└──────────────────────────────────────────────────────────────────────┘
```

### 核心设计原则

1. **双通道分离**：WAV（音效 + 场景 BGM）与 MP3（地图音乐）走完全不同的技术栈。`SilenceSound` 只能停 DirectSound 通道；停 MP3 必须显式 `PlayMp3('',False)`（`SoundUtil.pas:215-218/271-274`）。
2. **全内存播放**：WAV 文件每次整段解析进堆内存再灌入次级缓冲（`Wave.pas:239-256`），无流式播放——DelphiX 自带的 `TAudioStream` 流式设施存在但游戏未使用。
3. **无缓存、无复音上限**：`PlaySound` 每次调用都重新读盘、新建缓冲；同名音效重复调用直接叠加，没有"停旧播新"逻辑，也没有复音数量限制，回收靠 500ms 定时器（`DXSounds.pas:1827-1881/1905-1915`）。
4. **失败静默降级**：DirectSound 初始化失败时 `g_Sound := nil; MP3 := nil`（`ClMain.pas:503-507`），所有发声函数经 nil 守卫与空 `except` 静默失效——无声卡机器上游戏照常运行。
5. **惰性 COM**：`TMPEG.Create` 不建图（`Mpeg.pas:55` 的 `Init` 被注释），每次 `Play` 重新初始化整个 FilterGraph（`Mpeg.pas:88`）。
6. **开关不持久化**：`g_boSound`/`g_boBGSound` 仅运行期变量，重启回到默认 `True`；且 `g_boBGSound` 没有任何运行期切换入口（见第八章）。

---

## 二、全局状态与初始化/销毁

### 2.1 全局变量

| 变量 | 类型 | 位置 | 说明 |
|------|------|------|------|
| `g_DXSound` | `TDXSound` | `MShare.pas:152` | DelphiX 声音组件（设备层，持有 `TDirectSound`） |
| `g_Sound` | `TSoundEngine` | `MShare.pas:153` | 音效引擎（通道 A） |
| `g_boSound` | `Boolean` | `MShare.pas:213` | 音效开关（注释「声音开关」） |
| `g_boBGSound` | `Boolean` | `MShare.pas:214` | 背景音乐开关（注释「背景音乐开关」） |
| `g_nMapMusic` | `Integer` | `MShare.pas:237` | 当前地图音乐编号（来自服务端） |
| `g_SoundList` | `TStringList` | `MShare.pas:247` | 音效索引表（注释「声音列表」） |
| `MP3` | `TMPEG` | `ClMain.pas:323` | MP3 播放器（通道 B，窗体字段） |
| `BGMusicList` | `TStringList` | `ClMain.pas:325` | 地图 BGM 映射表（已废弃，见 6.3） |

### 2.2 初始化链

启动顺序（`mir2.dpr:38-47`）：`Application.Initialize` → `CreateForm(TfrmMain)`（`:41`）→ `CreateForm(TFrmDlg)`（`:42`）→ `CreateForm(TfrmDlgConfig)`（`:43`）→ `InitObj`（`:44`）→ `Application.Run`（`:47`）。声音初始化全部发生在 `TfrmMain.FormCreate`（`ClMain.pas:435`）中：

| 步骤 | 代码 | 位置 |
|------|------|------|
| 创建组件 | `g_DXSound := TDXSound.Create(Self)` | `ClMain.pas:490` |
| 初始化 | `g_DXSound.Initialize` | `ClMain.pas:492` |
| 成功分支 | `if g_DXSound.Initialized then` `g_Sound := TSoundEngine.Create(g_DXSound.DSound)` | `ClMain.pas:498-500` |
| | `MP3 := TMPEG.Create(nil)`（纯音频模式，无视频宿主窗口） | `ClMain.pas:502` |
| 失败分支 | `g_Sound := nil; MP3 := nil`（注释「g_dxSound初始化失敗則置空」） | `ClMain.pas:503-507` |
| 默认开关 | `g_boSound := True; g_boBGSound := True` | `ClMain.pas:646-647` |
| 建表 | `g_SoundList := TStringList.Create; BGMusicList := TStringList.Create` | `ClMain.pas:511-513` |
| 载表 | `LoadSoundList('.\wav\sound.lst')` | `ClMain.pas:515-517` |
| | `LoadBGMusicList('.\wav\sound.lst')`（喂同一个文件，已废弃，见 6.3） | `ClMain.pas:518-519` |

要点：

- `g_DXSound` **不是 dfm 组件**——`ClMain.dfm` 中无任何 DXSound/Sound/MP3 相关条目，全部为代码创建。
- `TCustomDXSound` 的 `AutoInitialize` 在 `Loaded` 时也会自动初始化（`DXSounds.pas:2112-2125`），但 `ClMain.pas:492` 又手动调了一次 `Initialize`——组件层的自动初始化吞 `EDirectSoundError`（`DXSounds.pas:2121`），而这里的手动调用配合 `Initialized` 属性检查走失败置空分支。
- `LoadSoundList` 先于任何场景创建（`DScreen` 等在 `ClMain.pas:523+` 才建），保证登录场景 `PlayBGM` 时索引表已就绪。

### 2.3 销毁

`FormDestroy`（过程体 `ClMain.pas:716` 起）中的释放顺序：

| 代码 | 位置 |
|------|------|
| `g_Sound.Free` | `ClMain.pas:828` |
| `g_SoundList.Free` | `ClMain.pas:829` |
| `BGMusicList.Free` | `ClMain.pas:830` |
| `g_DXSound.Free` | `ClMain.pas:838` |

泄漏记录：`MP3`（`TMPEG`）在 `FormDestroy` 中**从未 Free**（`ClMain.pas:824-840` 区间无 `MP3.Free`）；`BGMusicList.Objects` 里的 `^String` 堆指针从不 `Dispose`（`SoundUtil.pas:241-243`）。两者随进程退出回收。

---

## 三、DirectSound 引擎（DXSounds.pas）

DelphiX 库的 DirectSound 封装，2599 行。游戏只使用其中 `TDirectSound` / `TDirectSoundBuffer` / `TSoundEngine` / `TCustomDXSound` 四个类，流式播放与波表集合设施完全未使用。

### 3.1 类清单

| 名称 | 位置 | 职责 | 游戏使用 |
|------|------|------|----------|
| `EDirectSoundError` | `DXSounds.pas:15` | DirectSound 通用异常 | ✓ |
| `EDirectSoundBufferError` | `DXSounds.pas:16` | 缓冲区异常 | ✓ |
| `TDirectSound` | `DXSounds.pas:22-44` | IDirectSound 设备封装，管理全部缓冲生命周期 | ✓ |
| `TDirectSoundBuffer` | `DXSounds.pas:48-107` | 次级缓冲封装 | ✓ |
| `TAudioStream` | `DXSounds.pas:115-172` | 流式播放（后台线程喂环形缓冲） | ✗ |
| `TAudioFileStream` | `DXSounds.pas:176-184` | 文件版流式播放 | ✗ |
| `TSoundCaptureStream` | `DXSounds.pas:214-245` | DirectSoundCapture 录音流 | ✗ |
| `TSoundEngine` | `DXSounds.pas:249-269` | 一次性音效池（**游戏音效引擎**） | ✓ |
| `TCustomDXSound` / `TDXSound` | `DXSounds.pas:285-348` | VCL 组件层：初始化/协作级/主缓冲/窗体挂钩 | ✓ |
| `TWaveCollectionItem` / `TWaveCollection` | `DXSounds.pas:358-420` | 命名波表项（带复音池），DFM 序列化 | ✗ |
| `TCustomDXWaveList` / `TDXWaveList` | `DXSounds.pas:424-446` | 波表组件 | ✗ |
| `TDXSoundDirectSound` | `DXSounds.pas:1919-1931` | `TDirectSound` 私有子类，接线缓冲恢复回调 | ✓（内部） |

「未使用」经全目录 `findstr` 验证：`TAudioStream`/`TAudioFileStream`/`TSoundCaptureStream`/`TWaveCollection`/`TDXWaveList` 只出现在 `DXSounds.pas` 自身。

### 3.2 TDirectSound — 设备层

**创建与销毁**

- `Create(GUID: PGUID)`（`DXSounds.pas:550-557`）：先建 `FBufferList := TList.Create`（`:553`），再 `DXDirectSoundCreate(GUID, FIDSound, nil)`（`:555`）；返回非 `DS_OK` 抛 `EDirectSoundError.CreateFmt(SCannotInitialized, [SDirectSound])`（`:555-556`）。
- `DirectSoundCreate` 从 `DSound.dll` **运行时动态加载**（`DXDirectSoundCreate`，`DXSounds.pas:452-460`，动态加载于 `:458`），非静态链接——无 DSound.dll 的机器上在异常路径降级。
- `Destroy`（`DXSounds.pas:559-567`）：`while BufferCount > 0 do Buffers[BufferCount-1].Free`（`:561-562`）——设备负责销毁挂在它上面的所有缓冲；最后 `FIDSound := nil`（`:565`）释放 COM 接口。

**缓冲注册表 FBufferList**

每个 `TDirectSoundBuffer.Create` 把自己 `Add` 进所属设备的 `FBufferList`（`DXSounds.pas:627`），`Destroy` 时 `Remove`（`:633`）。这是「设备 → 缓冲」的所有权登记，**不是音效缓存**。

**属性与异常守卫**

- `BufferCount` / `Buffers[]`：`DXSounds.pas:40-41`，getter `:596-604`。
- `IDSound` getter：`:606-612`，`Self = nil` 安全。
- `ISound` getter：`:614-619`——若接口为 nil **抛异常** `SNotMade 'IDirectSound'`（`:618`）。所有缓冲操作都走 `ISound`，「设备不可用还去用」以异常暴露，再由上层空 `except` 吞掉。

**缓冲丢失自动恢复**

- `CheckBuffer(Buffer)`（`DXSounds.pas:574-590`）：当某缓冲 `DXResult = DSERR_BUFFERLOST`（`:577`）且不在恢复过程中（`FInRestoreBuffer` 防重入，`:579-587`），调虚方法 `DoRestoreBuffer`（`:583`）。
- 基类 `DoRestoreBuffer` 为空（`:592-594`）；子类 `TDXSoundDirectSound.DoRestoreBuffer` 转发到组件 `FDXSound.Restore`（`:1927-1931`），触发 `dsntRestore` 通知链（`:2127-2134`）。
- 触发点在 `TDirectSoundBuffer.Check`（`:661-664`），由基类 `TDirectX` 在每次 API 调用后回调。

### 3.3 TDirectSoundBuffer — 次级缓冲

**创建参数（SetSize，`DXSounds.pas:895-916`）**

- `dwSize := SizeOf(TDSBufferDesc)`（`:904`）
- `dwFlags := DSBCAPS_CTRLDEFAULT`（`:905`）——默认带 CTRLVOLUME / CTRLPAN / CTRLFREQUENCY 控制权限
- 若 `DSound.FStickyFocus` 追加 `DSBCAPS_STICKYFOCUS`（`:906-907`）；否则若 `DSound.FGlobalFocus` 追加 `DSBCAPS_GLOBALFOCUS`（`:908-909`）。**游戏默认两者都不加**（组件 `Options := []`，见 3.5），意味着窗口失焦时声音自动静音（`DSSCL_NORMAL` 标准行为）。
- `dwBufferBytes := Size`（`:910`），`lpwfxFormat := @Format`（`:911`）
- `CreateBuffer` 失败抛 `EDirectSoundBufferError(SCannotMade)`（`:914-915`）
- `CreateBuffer`（`:666-677`）：先 `IDSBuffer := nil` 释放旧接口（`:670`），成功创建后才挂上（`:675-676`）

**加载数据**

- `LoadFromFile`（`:742-752`，`fmOpenRead`）→ `LoadFromStream`。
- `LoadFromStream`（`:781-792`）：new `TWave` → `Wave.LoadFromStream` → `LoadFromWave`。
- `LoadFromWave`（`:794-797`）→ `LoadFromMemory(Wave.Format^, Wave.Data, Wave.Size)`。
- `LoadFromMemory`（`:754-779`）：`SetSize` 建缓冲后 `Lock(0, Size, ...)`（`:764`），把整块 PCM `Move` 进去（支持环绕双段 Data1/Data2，`:767-769`），`finally UnLock`（`:771`）；Lock 失败清空接口并抛 `SCannotLock`（`:773-777`）。

**播放控制**

- `Play(Loop: Boolean)`（`:823-830`）：`Loop = True` → `IBuffer.Play(0, 0, DSBPLAY_LOOPING)`（`:826`），否则标志 0（`:828`）。循环 BGM 即靠这个标志位。
- `Stop`（`:923-926`）→ `IBuffer.Stop`。
- `Playing` getter（`:710-713`）：`(Status and (DSBSTATUS_PLAYING or DSBSTATUS_LOOPING)) <> 0`。
- `Position` getter/setter（`:720-725` / `:890-893`）：只操作播放游标。
- `Volume`（`:737-740` / `:918-921`）、`Pan`（`:715-718` / `:885-888`）、`Frequency`（`:690-693` / `:856-859`）——全部直通 `IBuffer`，无本地缓存。取值范围为 DirectSound 标准 `DSBVOLUME_MIN..DSBVOLUME_MAX`（定义在 `DirectX.pas`）。**游戏代码从不设置这三个属性**——音量跟随系统默认。
- `Restore`（`:832-836`）：只恢复缓冲内存，**不重填数据**；数据重填由上层负责（游戏靠「下次重放」自然恢复）。

**Lock 单层限制**

`FLockAudioPtr1/2` 声明为 `array[0..0]`（`DXSounds.pas:55-56`），`Lock` 内 `if FLockCount > High(FLockAudioPtr1) then Exit`（`:806`）——同一缓冲同时只允许一层 Lock，第二层直接返回 `False`。底层 `IBuffer.Lock(..., 0)` 不带 `DSBLOCK_ENTIREBUFFER`（`:808-810`）。`UnLock`（`:928-936`）`FLockCount = 0` 时直接退出。

**其他**

- `SetIDSBuffer`（`:861-883`）：挂接口时自动 `GetCaps`（`:874-875`）+ 两次 `GetFormat`（先取尺寸 `:877`，再取内容 `:879-881`），本地缓存 `FFormat/FFormatSize/FCaps`，`FLockCount` 清零（`:870`）。
- `Assign`（`:637-659`）：源是 `TWave` 则 `LoadFromWave`（`:643-644`）；源是另一缓冲则 `DuplicateSoundBuffer`（`:650-651`，`TWaveCollectionItem` 复音池机制，游戏不用）。

### 3.4 TSoundEngine — 游戏音效引擎

声明 `DXSounds.pas:249-269`：私有 `FDSound`/`FEffectList`/`FEnabled`/`FTimer`，公开 `Clear`/`EffectFile`/`EffectStream`/`EffectWave`，属性 `EffectCount`/`Effects[]`/`Enabled`。

**构造（`DXSounds.pas:1797-1808`）**

- `FDSound := ADSound`（`:1800`）——只持有引用**不拥有**，设备生命周期由 `TCustomDXSound` 管。
- `FEnabled := True`（`:1801`）。
- `FEffectList := TList.Create`（`:1804`）——活动缓冲列表。
- `FTimer := TTimer.Create(nil)`，`Interval := 500`，`OnTimer := TimerEvent`（`:1805-1807`）。`TTimer` 默认 `Enabled = True`，引擎一建立就开始 500ms 轮询回收。

**EffectFile 完整语义（`DXSounds.pas:1827-1837`）**

```pascal
procedure TSoundEngine.EffectFile(const Filename: string; Loop, Wait: Boolean);
```

- 每次调用 `TFileStream.Create(Filename, fmOpenRead)`（`:1831`）→ `EffectStream`（`:1833`）→ finally 关流。**无任何缓存：每次都读盘**。
- `EffectStream`（`:1839-1850`）：new `TWave` → `Wave.LoadFromStream`（整文件进内存，见第四章）→ `EffectWave`。
- `EffectWave(Wave; Loop, Wait)`（`:1852-1881`）是真正的分支点：
  - `if not FEnabled then Exit`（`:1856`）——引擎级静音开关。
  - **Wait = True（同步分支）**（`:1858-1868`）：建临时缓冲 → `LoadFromWave` → `Play(False)` → `while Buffer.Playing do Sleep(1)` 阻塞直到播完 → finally 释放。不进 `FEffectList`。**游戏从不使用这个分支**（SoundUtil 全部传 `Wait = FALSE`）。
  - **Wait = False（异步分支，游戏路径）**（`:1869-1880`）：new `TDirectSoundBuffer`（`:1871`）→ `LoadFromWave`（`:1873`）→ `Play(Loop)`（`:1874`）→ 成功后 `FEffectList.Add(Buffer)`（`:1879`）。异常时 `Buffer.Free; raise`（`:1875-1878`），由 SoundUtil 空 `except` 吞掉。

**重复播放时旧缓冲如何处理：不处理，直接叠加。** 每次 `EffectFile` 都创建全新缓冲（`:1871`），旧缓冲若仍在播放则继续播。没有「同名停旧」逻辑，没有复音上限。回收靠 `TimerEvent`（`DXSounds.pas:1905-1915`）：每 500ms 倒序遍历 `FEffectList`，`not Playing` 的 `Free` + `Delete`（`:1910-1914`）——播完的短音效最多滞留 500ms。密集调用（连续攻击音效）会使缓冲数瞬时膨胀，仅受 DirectSound 驱动本身限制。对比：DelphiX 自带的 `TWaveCollectionItem` 有 `MaxPlayingCount` 复音池（`:2275-2334`，达上限轮转复用最老缓冲 `:2315-2316`），游戏没有采用。

**Clear（`DXSounds.pas:1818-1825`）**：倒序 `Effects[i].Free` 后 `FEffectList.Clear`。Free 缓冲即停止发声，所以 `Clear` = **立即掐断所有音效与循环 WAV BGM**（`SilenceSound` 的实现基础）。析构 `Destroy` 先 `Clear` 再释放 Timer/列表（`:1810-1816`）。

**SetEnabled（`DXSounds.pas:1893-1903`）**：无论开/关都先 Clear 全部缓冲（`:1897-1899`），然后 `FEnabled := Value; FTimer.Enabled := Value`（`:1901-1902`）。关引擎 = 静音 + 停止回收轮询。游戏不直接操作 `Enabled`（静音走 `g_boSound` 守卫）。

### 3.5 TCustomDXSound / TDXSound — 组件层

**默认值**（`DXSounds.pas:1933-1939`）：`FAutoInitialize := True`（`:1937`），`Options := []`（`:1938`，即无 `soGlobalFocus`/`soStickyFocus`/`soExclusive`）。

**Initialize（`DXSounds.pas:2065-2110`）** 流程：

1. 先 `Finalize`（`:2073`）。
2. 沿 `Owner` 链向上找 `TCustomForm`（`:2075-2077`），找不到抛 `EDXSoundError(SNoForm)`（`:2078-2079`）。
3. `NotifyEventList(dsntInitializing)` + `DoInitializing`（`:2081-2082`）。
4. `FDSound := TDXSoundDirectSound.Create(Driver)`（`:2087`），回接 `FDXSound := Self`（`:2088`）。
5. `FDSound.FGlobalFocus := soGlobalFocus in FNowOptions`（`:2090`）。
6. 创建主缓冲 `FPrimary`（`:2093-2095`）——描述符 `dwFlags = DSBCAPS_PRIMARYBUFFER`（`:2066-2069`），失败抛 `EDXSoundError(SCannotMade, SDirectSoundPrimaryBuffer)`。
7. `SetForm(TCustomForm(Component))`（`:2099`）——协作级在此设置。
8. 任何异常 → `Finalize; raise`（`:2100-2103`）。
9. 成功后 `dsntInitialize` 通知 + `DoInitialize`（`:2105-2107`），最后 `Restore`（`:2109`）。

**协作级（SetForm，`DXSounds.pas:2146-2164`）**

- 先对窗体子类化 `TControlSubClass.Create(FForm, FormWndProc)`（`:2152-2153`），`WM_CREATE` 时重建挂钩（`:2001-2008`）。
- 级别：`soExclusive in FNowOptions` → `DSSCL_EXCLUSIVE`（`:2157-2158`），否则 **`DSSCL_NORMAL`**（`:2159-2160`，游戏走此分支）。
- `FDSound.ISound.SetCooperativeLevel(FForm.Handle, Level)`（`:2162`），结果存入 `DXResult`（不抛异常）。

**SetOptions（`DXSounds.pas:2166-2186`）**：`InitOptions = [soExclusive]`（`:2169`）——Exclusive 只能在初始化前设定，运行期改 Options 时 `NowOptions` 保留已生效的 Exclusive（`:2179-2180`）；`GlobalFocus`/`StickyFocus` 运行期可改，写入 `FDSound.FGlobalFocus/FStickyFocus`（`:2182-2183`），影响**之后**新建的缓冲。

**Loaded 的异常吞噬（`DXSounds.pas:2112-2125`）**：`AutoInitialize` 且非设计期时自动 `Initialize`，但 `on E: EDirectSoundError do ;`（`:2121`）**静默吞掉**，其他异常重抛。这是「无声卡机器上游戏照常启动」的第一道机制（第二道是 `ClMain.pas:498` 的 `Initialized` 检查）。

**Finalize（`DXSounds.pas:2037-2063`）**：释放子类化（`:2042`）→ `DoFinalize`（`:2048`）→ `dsntFinalize`（`:2051`）→ 释放 `FPrimary` 与 `FDSound`（`:2059-2060`）。

### 3.6 未使用的 DelphiX 流式/波表设施（备查）

- **TAudioStream**（`DXSounds.pas:115-172`）：环形缓冲大小 `FBufferLength(ms) * nAvgBytesPerSec div 1000`（`:1257`），默认 `FBufferLength := 1000`ms（`:1017`）；缓冲标志比音效引擎多 `DSBCAPS_GETCURRENTPOSITION2`（`:1263`）；后台线程 `TAudioStreamNotify`（`:941-1009`）`WaitForSingleObject` 轮询，`FSleepTime := Min(BufferLength div 4, 1000 div 20)`（`:962`，≤50ms），醒来 `Synchronize(Update)` 回主线程填缓冲（`:988-989`）；尾部静音填充 8bit 用 `$80`、16bit 用 `0`（`:1414`）。
- **TWaveCollectionItem**（`:358-396`）：`MaxPlayingCount` 复音池，达上限 `FBufferList.Move(0, Count-1)` 轮转最老缓冲（`:2315-2316`）。

这两套设施是 Go 重制做流式 BGM / 复音上限时的现成设计参考，但 Delphi 原版并未使用。

### 3.7 异常路径总表

| 场景 | 行为 | 位置 |
|------|------|------|
| `DirectSoundCreate` 失败 | 抛 `EDirectSoundError` | `DXSounds.pas:555-556` |
| 组件 AutoInitialize 时上述异常 | **静默吞噬** | `DXSounds.pas:2121` |
| `Initialize` 失败 | `Finalize` 后重抛 | `DXSounds.pas:2100-2103` |
| 找不到宿主 Form | 抛 `EDXSoundError(SNoForm)` | `DXSounds.pas:2078-2079` |
| 主缓冲创建失败 | 抛 `EDXSoundError` | `DXSounds.pas:2094-2095` |
| 设备 nil 时使用 `ISound` | 抛 `EDirectSoundError(SNotMade)` | `DXSounds.pas:617-618` |
| 缓冲 nil 时使用 `IBuffer` | 抛 `EDirectSoundBufferError(SNotMade)` | `DXSounds.pas:706-707` |
| 次级缓冲创建失败 | 抛 `EDirectSoundBufferError(SCannotMade)` | `DXSounds.pas:914-915` |
| Lock 失败（LoadFromMemory） | 抛 `EDirectSoundBufferError(SCannotLock)` | `DXSounds.pas:775-776` |
| `DSERR_BUFFERLOST` | 自动 `DoRestoreBuffer` → 组件 `Restore` | `DXSounds.pas:574-590`, `:1927-1931` |
| 游戏层 PlaySound/PlayBGM/PlayMp3 | 外层 `try/except` 空块吞噬 | `SoundUtil.pas:186-189/257-261/278-282` |

### 3.8 关键常量

| 常量/值 | 位置 | 说明 |
|---------|------|------|
| 回收定时器间隔 **500ms** | `DXSounds.pas:1806` | `TSoundEngine` 缓冲回收轮询 |
| Wait 模式轮询 `Sleep(1)` | `DXSounds.pas:1865` | 同步分支（游戏未用） |
| `DSBCAPS_PRIMARYBUFFER` | `DXSounds.pas:2069` | 主缓冲标志 |
| `DSBCAPS_CTRLDEFAULT` | `DXSounds.pas:905` | 次级缓冲默认标志 |
| `DSSCL_NORMAL` | `DXSounds.pas:2160` | 协作级（Options 无 Exclusive） |
| 捕获格式枚举表（未用） | `DXSounds.pas:1596-1598` | 采样率 `{8000,11025,22050,33075,44100,48000,96000}`、位深 `{8,16,24,32}`、声道 `{1,2}` |

---

## 四、WAV 加载（Wave.pas）

726 行。核心是 `TWave`——整文件 PCM 内存容器；配套 `TWaveStream` 做 RIFF 解析。

### 4.1 类清单

| 名称 | 位置 | 职责 | 游戏使用 |
|------|------|------|----------|
| `EWaveError` | `Wave.pas:14` | 通用异常 | ✓ |
| `TWave` | `Wave.pas:18-43` | PCM 内存容器（`TPersistent`，可 DFM 序列化） | ✓ |
| `TCustomDXWave` / `TDXWave` | `Wave.pas:47-62` | 包装组件 | ✗ |
| `EWaveStreamError` | `Wave.pas:66` | 流解析异常 | ✓ |
| `TCustomWaveStream` | `Wave.pas:70-91` | 抽象波流（`TStream` 派生） | ✓（内部） |
| `TCustomWaveStream2` | `Wave.pas:95-105` | 自带格式内存的波流基类 | ✓（内部） |
| `TWaveObjectStream` | `Wave.pas:109-122` | 以 `TWave` 对象为后端的流适配器 | ✓（内部） |
| `TWaveStream` | `Wave.pas:126-146` | RIFF 解析/写入器（读写任意 `TStream`） | ✓ |
| `TWaveFileStream` | `Wave.pas:150-156` | 文件版（`TFileStream` 包装） | ✗ |
| `MakePCMWaveFormatEx` | `Wave.pas:165-178` | 构造 PCM 格式头 | ✗ |

### 4.2 TWave — 全量内存加载

- **LoadFromFile**（`Wave.pas:227-237`）：`TFileStream.Create(FileName, fmOpenRead or fmShareDenyWrite)`（`:231`）→ `LoadFromStream`。
- **LoadFromStream**（`Wave.pas:239-256`）：先 `Clear`（`:243`）；建 `TWaveStream` 并 `Open(False)`（读模式，`:245-247`）；拷格式（`FormatSize` + `Move`，`:249-250`）；`Size := WaveStream.Size` 分配 `FData` 后 `ReadBuffer(FData^, Size)`（`:251-252`）——**整段 PCM 一次读入堆内存**。
- **内存分配粒度**：`WavePoolSize = 8096`（`Wave.pas:182-183`），`SetSize` 按池对齐 `ReAllocMem`（`:306-317`，`(Value + WavePoolSize - 1) div WavePoolSize`，`:312`）。
- **Clear**（`:212-219`）：`FreeMem(FData, 0)` / `FreeMem(FFormat, 0)` 后清零。
- **Assign**（`:191-210`）：源为 nil 则 Clear；为 `TWave` 则深拷 Data/Format。
- **DFM 序列化**：`DefineProperties` 注册二进制属性 `'WAVE'`（`:221-225`）——游戏不使用。

### 4.3 TWaveStream — RIFF 解析

**OpenReadMode（`Wave.pas:574-626`）**

- FourCC 常量手写为小端整数：`ID_RIFF` / `ID_WAVE` / `ID_FMT` / `ID_FACT` / `ID_DATA`（`Wave.pas:497-501`），形如 `Ord('R') + Ord('I')*$100 + ...`。
- 文件头 `TWaveFileHeader = packed record FType, Size, RType`（`:504-508`，12 字节）；块头 `TWaveChunkHeader = packed record CType, Size`（`:510-513`，8 字节）。
- 校验：`FType <> ID_RIFF or RType <> ID_WAVE` → 抛 `EWaveStreamError(SInvalidWave)`（`:607-608`）。
- 块遍历循环（`:611-625`）：
  - `ID_FMT`：`FormatSize := WC.Size` 后原样 `ReadBuffer`（`:579-583`）——fmt 块按原始字节数照抄，**解析层不限 PCM**，理论可携带任何 `wFormatTag`。
  - `ID_DATA`：只记录 `FSize` 与 `FDataPosition` 然后 `Seek` 跳过（`:585-590`）——数据延迟到 `TWave.LoadFromStream` 的 `ReadBuffer` 阶段读取（`ReadWave` 定位 `FDataPosition + Position` 后从底层流读，`:542-549`）。
  - **未知块按 `WC.Size` 跳过**（`:618-621`）——LIST/INFO 等元数据块无害。
  - 循环终止于全零块（`while WC.CType <> 0`，`:613`，每轮末 `FillChar(WC, 0)` 后读下一块，`:623-624`）。
- 已打开再 `Open` 抛 `SStreamOpend`（`:593-594`）。

### 4.4 格式与写路径

- 配套工具 `MakePCMWaveFormatEx`（`Wave.pas:165-178`）只产生 `WAVE_FORMAT_PCM`（`:170`），`nBlockAlign := nChannels * (wBitsPerSample div 8)`（`:174`），`nAvgBytesPerSec := nBlockAlign * nSamplesPerSec`（`:175`），`cbSize := 0`（`:176`）——8/16bit、任意声道/采样率均可表达。实际游戏中 `sound.lst` 指向的 WAV 均为标准 PCM。
- 写路径 `OpenWriteMode`（`:628-678`）/ `CloseWriteMode`（`:680-710`）回填 RIFF/data 头——游戏不使用。

---

## 五、MP3 播放（Mpeg.pas + DShow.pas）

### 5.1 TMPEG — DirectShow FilterGraph 封装

`Mpeg.pas` 仅 114 行。`TMPEG`（`Mpeg.pas:7-29`）基于 **DirectShow（COM Filter Graph）**，非 MCI、非 ActiveX 控件。

**字段（`Mpeg.pas:9-17`）**

| 字段 | 行号 | 说明 |
|------|------|------|
| `g_pGraphBuilder: IGraphBuilder` | `:9` | 图构建器 |
| `g_pMediaControl: IMediaControl` | `:10` | 运行控制（注释「运行控制」） |
| `g_pMediaSeeking: IMediaSeeking` | `:11` | 定位（取得但从未使用） |
| `g_pAudioControl: IBasicAudio` | `:12` | 音量/平衡（取得但从未调用） |
| `g_pVideoWindow: IVideoWindow` | `:13` | 视频宿主窗口（游戏传 nil，纯音频） |
| `boInit` / `boPlay` | `:14-15` | 初始化/播放状态标志 |
| `MovieWindow: TWinControl` | `:17` | 视频宿主控件（游戏为 nil） |

**Create — 惰性初始化（`Mpeg.pas:47-57`）**

只置字段、五个接口全部 nil（`:50-54`），`Init` 调用被注释（`:55` `//boInit:=Init();`），`boInit := False`（`:56`）。构造不碰 COM。

**Init — 建图（`Mpeg.pas:65-75`，私有）**

1. `CoInitialize(nil)`（`:68`）
2. `CoCreateInstance(CLSID_FilterGraph, nil, CLSCTX_INPROC, IID_IGraphBuilder, g_pGraphBuilder)`（`:69`）——quartz.dll 的 FilterGraph 对象
3. 连续 `QueryInterface` 取 `IMediaControl`（`:70`）/ `IMediaSeeking`（`:71`）/ `IBasicAudio`（`:72`）/ `IVideoWindow`（`:73`）
4. 任一步 `failed` 短路返回 `False`——**已创建的接口不回滚**（Init 失败路径有小泄漏）；成功 `Result := true`（`:74`）

**Play — 每次重建图（`Mpeg.pas:82-102`）**

- **每次 Play 都重新 `Init`**（`boInit := Init()`，`:88`）——每次播放新建 COM 图；Play 前不自清理，依赖调用方先 `Stop`（`SoundUtil.PlayMp3` 正是这样做的，见 6.3）。
- 文件名 ANSI → 宽字符：`MultiByteToWideChar(CP_ACP, 0, ...)`（`:89`）。
- `g_pGraphBuilder.RenderFile(@wfile, nil)`（`:90`）——**由 DirectShow 按扩展名自动挑选源/解码/渲染滤镜**（对 `.mp3` 即 MPEG 音频解码器 + DirectSound 渲染器），失败 `exit`（`:91`）。
- 仅当 `MovieWindow <> nil` 才挂视频窗口：`put_Owner(Handle)`、`put_windowstyle(WS_CHILD or WS_Clipsiblings)`、`SetWindowposition(0, 0, W, H)`（`:92-96`）。游戏传入 `TMPEG.Create(nil)`（`ClMain.pas:502`），**纯音频模式**。
- 音量设置被注释：`:98` `//g_pAudioControl.put_Volume(VOLUME_FULL);`——**音量不可调**，随系统默认。
- `g_pMediaControl.run`（`:100`）后立即返回——**异步播放**（DirectShow 在自有线程推流）。
- **Bug**：`Result := False`（`:87`）后成功路径从未置 True（`:100-101` 只设 `boPlay`），函数永远返回 `False`。调用方（`SoundUtil.PlayMp3`）不检查返回值，无实际影响。
- **无循环**：没有 `IMediaEvent` / `EC_COMPLETE` 处理，播完即停。地图 MP3 BGM 因此只播一遍——切图时由新的 `PlayMapMusic` 再触发（`ClMain.pas:5223`）。

**Pause（`Mpeg.pas:77-80`）**：`g_pMediaControl.Pause` **无 Assigned 检查**——未初始化调用会访问违例。游戏从未调用 `Pause`。

**Stop（`Mpeg.pas:106-111`）**：`boInit` 为 `False` 直接退出（`:108`）；否则 `g_pMediaControl.Stop` + `Close`（`:109-110`）。

**Close（`Mpeg.pas:35-45`）**：再 Stop 一次（`:37`），五个接口逐个置 nil（`:38-42`，COM 引用计数归零即拆图），**无条件** `CoUninitialize`（`:43`），`boInit := False`（`:44`）。`CoInitialize`/`CoUninitialize` 的配对依赖「Play 过才 Stop」的调用习惯（`Create` 不做 `CoInitialize`）。

**Destroy（`Mpeg.pas:59-63`）** → `Close`。但游戏的 `MP3` 对象从不 Free（`ClMain.pas:824-840`），析构路径实际不走。

### 5.2 DShow.pas — DirectShow SDK 头

5906 行的纯接口翻译单元，源自 Hiroyuki Hori 的 DirectX 头移植（对应 DirectX Media SDK 5.1 的 strmif.h 等），见文件头注释（`DShow.pas:1-17`，`"DShow.pas DirectShow (DirectX Media SDK 5.1)"` 于 `:14`）。`uses Windows, ActiveX, DirectX, MMSystem`（`:27`）。**无任何业务逻辑**，图构建全在 `Mpeg.pas` 的 `RenderFile` 一行里完成。

`Mpeg.pas` 引用的条目：

| 条目 | 位置 | 说明 |
|------|------|------|
| `IID_IMediaSeeking` | `DShow.pas:96` | 接口 GUID |
| `IID_IGraphBuilder` | `DShow.pas:108` | 接口 GUID |
| `IMediaSeeking` | `DShow.pas:418` | 定位接口 |
| `IGraphBuilder` | `DShow.pas:679-689` | `RenderFile` 于 `:683` |
| `IMediaControl` | `DShow.pas:2033-2045` | `Run`/`Pause`/`Stop` 于 `:2036-2038` |
| `IBasicAudio` | `DShow.pas:2088-2095` | `put_Volume` `:2091`、`put_Balance` `:2093` |
| `IVideoWindow` | `DShow.pas:2098-2140` | `put_Owner` `:2123`、`SetWindowPosition` `:2133` |
| `CLSID_FilterGraph` | `DShow.pas:5639` | FilterGraph 对象 GUID |

Go 重制若走 MP3 播放，等价物是任意媒体播放后端（DirectShow COM 在 Go 侧可用 `go-ole`，或换用其它解码方案）。需保留的语义是：异步、不可循环、与 WAV 通道停止逻辑分离。

---

## 六、门面层（SoundUtil.pas）

325 行。游戏逻辑层与两条音频通道之间的唯一接口。

### 6.1 函数清单

| 名称 | 位置 | 职责 |
|------|------|------|
| `CurVolume` (var) | `SoundUtil.pas:12` | **死变量**：全客户端无读写点 |
| `SoundInfo` (record) | `SoundUtil.pas:25-28` | **死类型**：`Idx` + `Name`，无引用 |
| `LoadSoundList` | `SoundUtil.pas:151-178` | 音效索引表加载（稀疏索引） |
| `LoadBGMusicList` | `SoundUtil.pas:222-248` | 地图 BGM 映射表加载（**已废弃**，见 6.3） |
| `PlaySound` | `SoundUtil.pas:180-192` | 按索引播一次性音效 |
| `PlayMapMusic` | `SoundUtil.pas:193-221` | 按地图编号播 `.\Music\<n>.mp3` |
| `PlayBGM` | `SoundUtil.pas:249-265` | 循环播 WAV（登录/选角/死亡 BGM） |
| `PlayMp3` | `SoundUtil.pas:266-285` | TMPEG 播放/停止门面 |
| `SilenceSound` | `SoundUtil.pas:286-291` | `g_Sound.Clear` 全停（停不掉 MP3） |
| `ItemClickSound` | `SoundUtil.pas:293-310` | 按物品 StdMode 播点击音 |
| `ItemUseSound` | `SoundUtil.pas:312-319` | 按 StdMode 播使用音 |

依赖：interface uses `DXDraws, DirectX, DXClass, Grobal2, ExtCtrls, HUtil32, EdCode, Actor, DXSounds`（`SoundUtil.pas:5-8`）；implementation uses `ClMain, MShare`（`:147-148`，取 `g_Sound`/`MP3`/`g_SoundList` 等全局量）。

### 6.2 LoadSoundList — 索引表文件格式（`SoundUtil.pas:151-178`）

文件 `.\wav\sound.lst`（由 `ClMain.pas:515-517` 传入），行格式 `索引: 相对路径`：

```
; 注释行（行首 ;）
1: wav\walk-ground-l.wav
2: wav\walk-ground-r.wav
50: wav\hit-short.wav
```

解析规则：

- 文件不存在整体跳过（`FileExists` 门控，`:158`）——缺文件不报错。
- 空行跳过（`:164`）；**注释符 `;`**：行首为 `;` 则 `continue`（`:165`）。
- **分隔符**：`GetValidStr3(str, data, [':', ' ', #9])`（`:166`）——冒号、空格、Tab 均可分隔；第一个 token 作索引号，剩余部分为文件名。`Str_ToInt(data, 0)` 非法数字回退 0（`HUtil32.pas:365-376`）。
- **稀疏索引处理**（`:168-173`）：仅当 `n > idx`（idx 为已见最大索引）才接受；先循环补空串 `for k := 0 to n - g_SoundList.Count - 1 do g_SoundList.Add('')`（`:169-170`），再 `Add(str)` 使文件名恰好落在下标 n（`:171`），`idx := n`（`:172`）。
- **隐含约束：索引必须严格递增出现**，乱序或重复索引的行被静默丢弃（`n > idx` 不成立）。缺口用 `''` 占位，`PlaySound` 在 `:184` 跳过空位。

### 6.3 六个播放函数的精确行为

**PlaySound(idx)（`SoundUtil.pas:180-192`）— 一次性音效**

五重门控后播放：

1. `(g_Sound <> nil) and g_boSound`（`:182`）
2. `(idx >= 0) and (idx < g_SoundList.Count)`（`:183`）
3. `g_SoundList[idx] <> ''`（`:184`，稀疏空位跳过）
4. `FileExists(g_SoundList[idx])`（`:185`，缺文件跳过）
5. `g_Sound.EffectFile(name, FALSE, FALSE)`（`:187`）= 非循环、异步

外层 `try/except` 空块吞异常（`:186-189`）。

**PlayBGM(wavname)（`SoundUtil.pas:249-265`）— WAV 循环背景乐**

- `Result := nil` 恒返回 nil（`:252`，注释 `//Jacky`）——签名虽返回 `TDirectSoundBuffer` 但从不赋值，**接口退化残留**。
- `g_boBGSound` 门控（`:253`）。
- **先 `SilenceSound`**（`:258`）——停掉一切现有音效/音乐，再 `g_Sound.EffectFile(wavname, TRUE, FALSE)`（`:259`，`Loop = TRUE`）。
- 实现含义：BGM 是一个**常驻内存的循环次级缓冲**（整个 WAV 解码驻留），不是流式；切 BGM 时靠 `Clear` 打断上一首。全库仅 3 个调用点：`IntroScn.pas:518`（`bmg_intro`）、`IntroScn.pas:1152`（`bmg_select`）、`Actor.pas:2374`（`bmg_gameover`）。

**PlayMp3(wavname, boFlag)（`SoundUtil.pas:266-285`）— MP3 门面**

- `MP3 = nil`（初始化失败时）则全函数无效（`:270`）。
- `boFlag = False` → `MP3.Stop` 后退出（`:271-274`）——**停止语义复用同一入口**。
- `g_boBGSound` 门控（`:275`）；空文件名/缺文件跳过（`:276-277`）。
- 播放前先 `MP3.Stop` 再 `MP3.Play(wavname)`（`:279-280`）——补偿 `TMPEG.Play` 不自清理（见 5.1）。异常吞掉（`:281-282`）。

**PlayMapMusic(boFlag)（`SoundUtil.pas:193-221`）— 地图音乐**

- `g_nMapMusic < 0` 或 `not boFlag` → `PlayMp3('', False)` 停止后退出（`:215-218`）。
- 否则拼 `sFileName := '.\Music\' + IntToStr(g_nMapMusic) + '.mp3'`（`:219`）→ `PlayMp3(sFileName, boFlag)`（`:220`）。
- `g_nMapMusic` 来自网络消息（见 9.1）。
- 函数内两段被注释的旧逻辑（`:201-214`）：早期按 `BGMusicList` 随机/按图名匹配——**已被编号方案取代**。

**LoadBGMusicList（`SoundUtil.pas:222-248`）— 事实上废弃**

- 格式为 `mapName: fileName`（两 token，`:235-236`），`New(pFileName)` 堆指针存入 `BGMusicList.AddObject(sMapName, TObject(pFileName))`（`:241-243`）——这些 `^String` 从不 `Dispose`（**泄漏**）。
- 废弃证据：① `PlayMapMusic` 中对 `BGMusicList` 的引用全被注释（`:201-214`）；② `ClMain.pas:518-519` 把**同一个文件 `.\wav\sound.lst`** 喂给 `LoadBGMusicList`，其数字索引行会被解析成 `mapName = '1'` 之类的垃圾项。

**SilenceSound（`SoundUtil.pas:286-291`）**

`g_Sound.Clear`（`:289`）——释放 `FEffectList` 全部缓冲 = 所有音效与 WAV BGM 立停；**不影响 MP3**（MP3 走 DirectShow 独立通道）。调用点：`IntroScn.pas:527`（登录场景关）、`IntroScn.pas:1145`（选角场景关）、`PlayScn.pas:492`（游戏场景关）。

**双通道停止语义（Go 重制须保留的边界）**：`CloseScene` 的 `SilenceSound` 只停 DirectSound 通道；地图 MP3 的停止靠切图时 `PlayMapMusic(True)` 内部先 `Stop` 再 `Play`（`SoundUtil.pas:279-280`），或 `g_nMapMusic < 0` 时显式 `PlayMp3('', False)`（`:215-218`）。

### 6.4 ItemClickSound / ItemUseSound — 物品音效映射

**ItemClickSound(std: TStdItem)（`SoundUtil.pas:293-310`）**，按 `std.StdMode` 分支：

| StdMode | 音效 | 行号 |
|---------|------|------|
| 0, 31 | `s_click_drug`（108） | `:296` |
| 5, 6 | `s_click_weapon`（111） | `:297` |
| 10, 11 | `s_click_armor`（112） | `:298` |
| 22, 23 | `s_click_ring`（113） | `:299` |
| 24, 26 | 按物品名分流（见下） | `:300-305` |
| 19, 20, 21 | `s_click_necklace`（115） | `:306` |
| 15 | `s_click_helmet`（116） | `:307` |
| 其余 | `s_itmclick`（118） | `:308` |

**StdMode 24/26 分支**（`SoundUtil.pas:300-305`，GBK 注释经 `Get-Content -Encoding Default` 还原）：

```pascal
if (pos('手镯', std.Name) > 0) or (pos('手套', std.Name) > 0) then
  PlaySound(s_click_grobes)   // 117
else
  PlaySound(s_click_armring); // 114
```

即物品名含**「手镯」或「手套」**→ 护腕/手套类音（117），否则 → 手镯类音（114）。典型的「按中文名子串分类」实现，与 GBK 编码和物品命名强耦合。

**ItemUseSound(stdmode)（`SoundUtil.pas:312-319`）**：`0 → s_click_drug`（108）；`1, 2 → s_eat_drug`（107，吃药声）；其余无声。调用点：`ClMain.pas:1971`（吃背包物品）、`ClMain.pas:1998`（吃手持物品 `g_EatingItem`）。

### 6.5 死代码一览

| 条目 | 位置 | 状态 |
|------|------|------|
| `CurVolume` | `SoundUtil.pas:12` | 全库无读写点 |
| `SoundInfo` record | `SoundUtil.pas:25-28` | 全库无引用 |
| `bmg_field = 'wav\Field2.wav'` | `SoundUtil.pas:33` | 定义但全库无调用点 |
| `s_intro_theme = 102` | `SoundUtil.pas:109` | 无调用点，且与 `s_main_theme = 102`（`:111`）同值冲突 |
| `LoadBGMusicList` | `SoundUtil.pas:222-248` | 废弃但仍被调用（见 6.3） |
| `OpenSoundOption` | `FState.pas:1774-1782`（声明 `:516`） | 与 `DOptionClick` 逻辑重复，无调用方 |

---

## 七、音效索引常量全表

### 7.1 BGM 路径常量（`SoundUtil.pas:31-34`）

| 常量 | 值 | 用途 |
|------|-----|------|
| `bmg_intro` | `wav\log-in-long2.wav` | 登录场景 BGM |
| `bmg_select` | `wav\sellect-loop2.wav`（原文拼写如此） | 选角场景 BGM |
| `bmg_field` | `wav\Field2.wav` | **无调用点**（死常量） |
| `bmg_gameover` | `wav\game over2.wav`（路径含空格） | 玩家死亡 BGM |

### 7.2 音效索引常量（`SoundUtil.pas:36-142`）

**脚步声 1-32**（8 种地表 × 走/跑 × 左/右脚，`:36-67`）：

| 地表 | 走左/走右 | 跑左/跑右 |
|------|-----------|-----------|
| ground（土） | 1 / 2 | 3 / 4 |
| stone（石） | 5 / 6 | 7 / 8 |
| lawn（草） | 9 / 10 | 11 / 12 |
| rough（粗糙） | 13 / 14 | 15 / 16 |
| wood（木） | 17 / 18 | 19 / 20 |
| cave（洞） | 21 / 22 | 23 / 24 |
| room（房） | 25 / 26 | 27 / 28 |
| water（水） | 29 / 30 | 31 / 32 |

常量布局刻意成对/成组：`+1` = 右脚，`+2` = 跑（同侧），`SetSound` 只赋「走左」基值，运行时推导其余三态（见 9.2）。

**击打声 50-57（`s_hit_*`，`:70-77`）**：short 50、wooden 51、sword 52、do 53、axe 54、club 55、long 56、fist 57。

**受击武器声 60-65（`s_struck_*`，`:79-84`）**：short 60、wooden 61、sword 62、do 63、axe 64、club 65。

**受击肉体声 70-73（`s_struck_body_*`，`:86-89`）**：sword 70、axe 71、longstick 72、fist 73。

**受击护甲声 80-83（`s_struck_armor_*`，`:91-94`）**：sword 80、axe 81、longstick 82、fist 83。

**石矿声（`:105-106`）**：`s_strike_stone` 91、`s_drop_stonepiece` 92（调用点已注释，`Actor.pas:3499`）。

**系统/UI 声 100-110（`:108-119`）**：

| 常量 | 值 | 说明 |
|------|-----|------|
| `s_rock_door_open` | 100 | 登录石门 |
| `s_meltstone` | 101 | 选角解冻 |
| `s_intro_theme` | 102 | **死常量，与下冲突** |
| `s_main_theme` | 102 | 游戏主题曲（定时器已停用，见 9.1） |
| `s_norm_button_click` | 103 | 普通按钮 |
| `s_rock_button_click` | 104 | 石质按钮 |
| `s_glass_button_click` | 105 | 玻璃按钮 |
| `s_money` | 106 | 金币变化 |
| `s_eat_drug` | 107 | 吃药 |
| `s_click_drug` | 108 | 点击药品 |
| `s_spacemove_out` | 109 | 瞬移离场 |
| `s_spacemove_in` | 110 | 瞬移入场 |

**物品点击声 111-118（`:121-128`）**：weapon 111、armor 112、ring 113、armring 114、necklace 115、helmet 116、grobes 117、itmclick 118。

**特殊攻击声 130-137（`:130-137`）**：`s_yedo_man` 130、`s_yedo_woman` 131（攻杀配音）、`s_longhit` 132（攻杀剑术）、`s_widehit` 133（半月）、`s_rush_l`/`s_rush_r` 134/135、`s_firehit_ready` 136、`s_firehit` 137（烈火/逐日/双龙）。

**人声 138-145（`:139-142`）**：`s_man_struck` 138、`s_wom_struck` 139、`s_man_die` 144、`s_wom_die` 145（140-143 未定义）。

### 7.3 游戏代码硬编码索引（无常量定义）

| 索引 | 位置 | 对应事件 |
|------|------|----------|
| 48 | `ClMain.pas:6096` | SM_716 人形特效 nType=3 |
| 146-152 | `Actor.pas:2949` | NPC（appearance=52）出土，`Random(7) + 146` 随机 |
| 2276 | `AxeMon.pas:1542` | TBanyaGuardMon race=72 mt13 魔法 |
| 2396 | `AxeMon.pas:1546` | TBanyaGuardMon race=78 mt13（目标=自身） |
| 8201 | `AxeMon.pas:2778` | TFireDragon 出场/启动 |
| 8202 | `AxeMon.pas:2815` | TFireDragon 近战 |
| 8203 | `AxeMon.pas:2821` | TFireDragon 远程火球 |
| 8206 | `AxeMon.pas:2682` | TFireDragon 攻击特效 |
| 8207 | `ClMain.pas:6109` | SM_716 nType=6（NewMagic 73 mtThunder） |
| 8208 | `magiceff.pas:630` | 火龙吐息类特效创建（bt80=1） |
| 8222 | `AxeMon.pas:2496` | TDragonStatue 龙柱雷电 |
| 8226 | `ClMain.pas:6113` | SM_716 nType=7（NewMagic 74 mtThunder） |
| 8301 | `ClMain.pas:6100` | SM_716 nType=4（NewMagic 70 mtThunder） |
| 8302 | `ClMain.pas:6105` | SM_716 nType=5（NewMagic 71+72 mtThunder） |
| 10012 | `AxeMon.pas:1538` | TBanyaGuardMon race=71 mtFly 魔法 |
| 10112 | `AxeMon.pas:1534` | TBanyaGuardMon race=70/81 mtThunder 魔法 |
| `10000 + Serial*10 + {0,1,2}` | `Actor.pas:2299-2301` | 魔法起始/飞行/爆炸声（按 MagicSerial，见 9.3） |
| `200 + Appearance*10 + {0..6}` | `Actor.pas:2325-2331` | 怪物出现/普通/攻击/武器/惨叫/死亡/死亡2（见 9.3） |

这些索引全部指向 `sound.lst` 中的条目——索引表是稀疏的，缺口处 `PlaySound` 静默跳过（`SoundUtil.pas:183-184`），因此缺失资源不会报错。

---

## 八、配置与开关

### 8.1 开关变量与持久化

`g_boSound` / `g_boBGSound` 默认均为 `True`（`ClMain.pas:646-647`），且**均不持久化**：

- `FormCreate` 读取的 `Lmir.ini`（`ClMain.pas:454-467`）只含 `FullScreen`（`ReadBool('Setup','FullScreen')`，`:462`）、字体、服务器地址等键，**无任何声音键**。
- 全库无声音开关的 `GetBoolean` / `WriteBool` 调用——两个开关仅运行期生效，重启回到默认 `True`。

守卫位置：

| 开关 | 守卫 | 位置 |
|------|------|------|
| `g_boSound` | `PlaySound` 内 `if (g_Sound <> nil) and g_boSound` | `SoundUtil.pas:182` |
| `g_boBGSound` | `PlayBGM` 内 `if not g_boBGSound then exit` | `SoundUtil.pas:253` |
| `g_boBGSound` | `PlayMp3` 内 `if not g_boBGSound then exit` | `SoundUtil.pas:275` |

### 8.2 切换入口

**g_boSound 的唯一实际切换入口是 `TFrmDlg.DOptionClick`（`FState.pas:3813-3821`）**：

```pascal
procedure TFrmDlg.DOptionClick();
begin
  g_boSound := not g_boSound;          // :3815
  if g_boSound then
    DScreen.AddChatBoardString('[音乐打开]', clWhite, clBlack)  // :3817
  else
    DScreen.AddChatBoardString('[音乐关闭]', clWhite, clBlack); // :3819
end;
```

两个触发路径：

| 路径 | 位置 | 说明 |
|------|------|------|
| 底栏选项按钮 | `FState.pas:3810` | `DMyStateClick` 分发：`if Sender = DOption then DOptionClick` |
| F12 快捷键 | `ClMain.pas:1499-1508` | `Ctrl+Alt+F12` 打开调试对话框（`:1500-1503`）；否则 `FrmDlg.DOptionClick`（`:1506`） |

另有 `FState.pas:1996-1998`：关闭选项面板时若 `DConfigDlg.Visible` 也调 `DOptionClick()`。

**g_boBGSound 除默认赋值（`ClMain.pas:647`）与两处读取（`SoundUtil.pas:253/275`）外，全库无任何写入点**——背景音乐开关恒为 `True`，没有 UI 可关闭它。注意 `DOptionClick` 的聊天提示文字是「音乐打开/关闭」，实际控制的却是音效开关 `g_boSound`。

### 8.3 DlgConfig.pas 与声音无关（考证）

`TfrmDlgConfig`（`DlgConfig.pas:10-246`）是通过 `Ctrl+Alt+F12` 打开的开发期调试对话框，只含窗口位置 SpinEdit、`CheckBoxDrawTileMap`（`:36/:236`）、`CheckBoxDrawDropItem`（`:37/:241`）、测试坐标、SpellTime/HitTime 等渲染调试项；`Open()`（`:81-98`）与 `FormCreate`（`:100-155`）均不涉及 `g_boSound` / `g_boBGSound`。声音选项 UI 实际在游戏内底栏的 `DOption` 按钮上。

---

## 九、触发点全景

### 9.1 场景 BGM 与场景音效

| 场景/事件 | 位置 | 行为 |
|-----------|------|------|
| Logo 场景 | `IntroScn.pas:232-242` | `OpenScene`/`CloseScene`/`PlayScene` 均为空过程——首屏静音 |
| 登录场景开 | `IntroScn.pas:518` | `TLoginScene.OpenScene`（`:488`）末尾：`PlayBGM(bmg_intro)` |
| 登录场景关 | `IntroScn.pas:527` | `CloseScene`：`SilenceSound` |
| 登录成功石门 | `IntroScn.pas:801` | `OpenLoginDoor`（`:796`）：`m_boNowOpening := TRUE` 后 `PlaySound(s_rock_door_open)` |
| 选角场景开 | `IntroScn.pas:1139-1140` | `OpenScene`：`SoundTimer.Enabled := TRUE; Interval := 1`——1ms 延迟一次性触发 |
| 选角 BGM | `IntroScn.pas:1152-1153` | `SoundOnTimer`：`PlayBGM(bmg_select)` 后立即禁用定时器（`:1154` 的 38 秒循环方案被注释） |
| 选角场景关 | `IntroScn.pas:1145` | `CloseScene`：`SilenceSound` |
| 选角色槽 1/2 | `IntroScn.pas:1170 / :1187` | 选中有效角色解冻动画：`PlaySound(s_meltstone)` |
| 玩家死亡 BGM | `Actor.pas:2370-2374` | `RunSound` 的 `SM_NOWDEATH` 分支：先 `PlaySound(m_nDieSound)`，再 `if Self = g_MySelf then PlayBGM(bmg_gameover)`（`:2372` 注释显示原条件为 `m_btRace = RC_USERHUMAN`，已改为只判本机玩家） |
| 地图音乐 | `ClMain.pas:5222-5223` | `SM_MAPDESCRIPTION`（= 54，`Grobal2.pas:227`，注释「地图描述,地图音乐」；分发于 `ClMain.pas:3892-3894`）→ `ClientGetMapDescription`：`g_nMapMusic := Msg.Recog` → `PlayMapMusic(True)` |
| 游戏场景关 | `PlayScn.pas:492` | `CloseScene`：`SilenceSound`（有效；停不掉 MP3） |
| 主题曲定时器 | `PlayScn.pas:421-425` | `SoundOnTimer`：`PlaySound(s_main_theme)` 并设 `Interval := 46*1000`——**死逻辑**：`OpenScene` 中的启用代码 `:485-486` 两行均被注释（`//MainSoundTimer.Interval := 1000; //MainSoundTimer.Enabled := TRUE;`），`CloseScene` 的停用 `:491` 同样被注释。46 秒循环主题曲运行时永不触发，游戏内 BGM 完全依赖 MP3 通道 |

全库 `PlayBGM` 调用点仅 3 处：`Actor.pas:2374`、`IntroScn.pas:518`、`IntroScn.pas:1152`。

**场景 BGM 切换时序**：登录场景 `OpenScene` 播 `bmg_intro` 循环 WAV → 登录成功石门音效 → `CloseScene` `SilenceSound` 全停 → 选角场景 1ms 定时器播 `bmg_select` → 进入游戏 `CloseScene` 再停 → 服务端下发 `SM_MAPDESCRIPTION` 触发 `.\Music\<n>.mp3` → 玩家死亡叠播 `bmg_gameover`（`PlayBGM` 内部先 `SilenceSound`，会打断一切音效但停不掉 MP3）。

### 9.2 脚步声 — SetSound 地表映射

脚步声不在 PlayScn 层计算，而在 `TActor.SetSound`（`Actor.pas:2129`）中按地图格数据动态赋值。

**触发条件（`Actor.pas:2134-2142`）**：`m_btRace = 0`（人形）且 `self = g_MySelf`（仅本机玩家有脚步声）且当前动作为 `SM_WALK` / `SM_BACKSTEP` / `SM_RUN` / `SM_HORSERUN` / `SM_RUSH` / `SM_RUSHKUNG` 之一。

**坐标取样（`Actor.pas:2144-2150`）**：

```pascal
cx := g_MySelf.m_nCurrX - Map.m_nBlockLeft;   // 转地图局部坐标
cy := g_MySelf.m_nCurrY - Map.m_nBlockTop;
cx := cx div 2 * 2;                            // 偶数对齐（逻辑格→地砖格）
cy := cy div 2 * 2;
bidx := Map.m_MArr[cx, cy].wBkImg and $7FFF;   // 背景图索引（去翻转标志位）
wunit := Map.m_MArr[cx, cy].btArea;            // 贴图单元号
bidx := wunit * 10000 + bidx - 1;              // 合成全局地砖索引
```

**地表映射表（case bidx，`Actor.pas:2151-2197`）**：

| bidx 区间 | 地表 | 赋值 | 位置 |
|-----------|------|------|------|
| 330..349, 450..454, 550..554, 750..754, 950..954, 1250..1254, 1400..1424, 1455..1474, 1500..1524, 1550..1574 | 草地 | `s_walk_lawn_l`（9） | `:2153-2156` |
| 250..254, 1005..1009, 1050..1054, 1060..1064, 1450..1454, 1650..1654 | 草丛地面 | `s_walk_rough_l`（13） | `:2160-2162` |
| 605..609, 650..654, 660..664, 2000..2049, 3025..3049, 2400..2424, 4625..4649, 4675..4678 | 石头 | `s_walk_stone_l`（5） | `:2165-2167` |
| 1825..1924, 2150..2174, 3075..3099, 3325..3349, 3375..3399 | 洞穴 | `s_walk_cave_l`（21） | `:2170-2172` |
| 3230, 3231, 3246, 3277 | 木头 | `s_walk_wood_l`（17） | `:2175-2176` |
| 3780..3799 | 桥 | `s_walk_wood_l`（17） | `:2179-2180` |
| 3825..4434 | 木地板特例 | `(bidx-3825) mod 25 = 0` → 木（17），否则土（1） | `:2182-2184` |
| 2075..2099, 2125..2149 | 房间 | `s_walk_room_l`（25） | `:2188-2189` |
| 1800..1824 | 水 | `s_walk_water_l`（29） | `:2192-2193` |
| 其余 | 土 | `s_walk_ground_l`（1） | `:2195-2196` |

**覆盖规则（case 之后依次执行，后写覆盖先写）**：

| 条件 | 覆盖为 | 位置 |
|------|--------|------|
| `bidx ∈ 825..1349` 且 `((bidx-825) div 25) mod 2 = 0` | 石头（5） | `:2199-2202` |
| `bidx ∈ 1375..1799` 且 `((bidx-1375) div 25) mod 2 = 0` | 洞穴（21） | `:2204-2207` |
| `bidx ∈ {1385, 1386, 1391, 1392}` | 木头（17） | `:2208-2211` |

**中层图与前景图修正（`Actor.pas:2213-2236`）**：

- 中层图 `wMidImg and $7FFF - 1`（`:2213-2214`）：`0..115` → 土（`:2216-2217`），`120..124` → 草（`:2218-2219`）。
- 前景图 `wFrImg and $7FFF - 1`（`:2222-2223`）：
  - `221..289, 583..658, 1183..1206, 7163..7295, 7404..7414` → 石头（`:2226-2228`）
  - `3125..3267, 3757..3948, 6030..6999` → 木头（`:2230-2232`）
  - `3316..3589` → 房间（`:2234-2235`）

**跑步偏移（`Actor.pas:2237-2238`）**：`SM_RUN` 或 `SM_HORSERUN` 时 `m_nFootStepSound := m_nFootStepSound + 2`（walk→run 常量偏移）。

**播放时机（帧驱动，`Actor.pas:2650-2661`）**：走/跑动作期间按动画帧偏移发声：

```pascal
case (m_nCurrentFrame - m_nStartFrame) of
  1: PlaySound(m_nFootStepSound);      // 左脚（_l 常量）
  4: PlaySound(m_nFootStepSound + 1);  // 右脚（_r 常量）
end;
```

即 `SetSound` 只赋「左脚 walk」基值，`+1` 推导右脚、`+2` 推导跑步，四个变体共用一个字段。此播放点对所有 Actor 生效（不限 `g_MySelf`），但赋值仅限本机玩家（`:2135`），其他角色 `m_nFootStepSound` 保持 -1，`PlaySound(-1)` 被范围检查（`SoundUtil.pas:183`）拦下。

### 9.3 战斗声音 — SetSound 计算 + RunSound/RunActSound 播放

**声音字段声明（`Actor.pas:662-679`）**：

| 字段 | 行号 | 注释含义 |
|------|------|----------|
| `m_nMagicStruckSound` | `:663` | 被魔法打中的声音（服务端下发，1 以上生效） |
| `m_boRunSound` | `:664` | 是否需要播放音效（一次性门控） |
| `m_nFootStepSound` | `:665` | 脚步声（CM_WALK/CM_RUN） |
| `m_nStruckSound` | `:666` | 被打声音（SM_STRUCK） |
| `m_nStruckWeaponSound` | `:667` | 按攻击者武器分的被打声 |
| `m_nAppearSound` | `:669` | 出场声（偏移 0） |
| `m_nNormalSound` | `:670` | 平时声（偏移 1） |
| `m_nAttackSound` | `:671` | 攻击声（偏移 2） |
| `m_nWeaponSound` | `:672` | 武器声（偏移 3） |
| `m_nScreamSound` | `:673` | 惨叫（偏移 4） |
| `m_nDieSound` | `:674` | 死亡（偏移 5，SM_DEATHNOW） |
| `m_nDie2Sound` | `:675` | 死亡 2（偏移 6） |
| `m_nMagicStartSound` | `:677` | 魔法起始 |
| `m_nMagicFireSound` | `:678` | 魔法飞行 |
| `m_nMagicExplosionSound` | `:679` | 魔法爆炸 |

**构造默认值（`TActor.Create`，`Actor.pas:1258-1266`）**：`Normal`/`FootStep`/`Attack`/`Weapon`/`StruckWeapon`/`Scream`/`Die`/`Die2` 全部 `-1`；唯独 `m_nStruckSound := s_struck_body_longstick`（`:1262`，默认受击声）。`THumActor.Create`（`:3129`）与 `TNpcActor.Create`（`:2962`）均不设置声音字段——人/怪差异不在构造期硬编码，而在 `SetSound` 运行期按 `m_btRace` 分支计算。

#### 人形分支（m_btRace = 0）

**惨叫/死亡按性别（`Actor.pas:2242-2248`）**：`m_btSex = 0`（男）→ `s_man_struck`（138）/ `s_man_die`（144）；否则（女）→ `s_wom_struck`（139）/ `s_wom_die`（145）。

**挥击声按武器（`Actor.pas:2250-2262`）**：动作为 `SM_THROW`/`SM_HIT`/`SM_HIT+1`/`SM_HIT+2`/`SM_POWERHIT`/`SM_LONGHIT`/`SM_WIDEHIT`/`SM_FIREHIT`/`SM_CRSHIT`/`SM_TWINHIT` 时（`:2251`），按 `m_btWeapon div 2`：

| `m_btWeapon div 2` | 赋值 | 行号 |
|--------------------|------|------|
| 6, 20 | `s_hit_short`（50） | `:2254` |
| 1 | `s_hit_wooden`（51） | `:2255` |
| 2, 13, 9, 5, 14, 22 | `s_hit_sword`（52） | `:2256` |
| 4, 17, 10, 15, 16, 23 | `s_hit_do`（53） | `:2257` |
| 3, 7, 11 | `s_hit_axe`（54） | `:2258` |
| 24 | `s_hit_club`（55） | `:2259` |
| 8, 12, 18, 21 | `s_hit_long`（56） | `:2260` |
| 其余 | `s_hit_fist`（57） | `:2261` |

**受击声按攻击者武器 + 自身护甲（`Actor.pas:2264-2294`）**：`SM_STRUCK` 时，若 `m_nMagicStruckSound >= 1`（服务端下发的魔法受击声）则跳过普通受击声（`:2266-2267`）；否则经 `PlayScene.FindActor(m_nHiterCode)` 找攻击者（`:2269`），`attackweapon := hiter.m_btWeapon div 2`（`:2272`），**仅当攻击者是人形**（`hiter.m_btRace = 0`，`:2273`）时按自身 `m_btDress div 2` 分支：

- `= 3`（穿护甲，`:2275-2282`）：武器 6/1/2/4/5/9/10/13/14/15/16/17 → `s_struck_armor_sword`（80）；3/7/11 → `s_struck_armor_axe`（81）；8/12/18 → `s_struck_armor_longstick`（82）；其余 → `s_struck_armor_fist`（83）。
- 其余（肉体，`:2283-2290`）：同武器分组 → `s_struck_body_*`（70-73）。

**受击武器声（`Actor.pas:2335-2353`）**：`SM_STRUCK` 且攻击者是人形时，`attackweapon := hiter.m_btWeapon div 2`（`:2340`）后再 `case attackweapon div 2`（`:2342`，**双重 div**）：6/20→`s_struck_short`（60）、1→`s_struck_wooden`（61）、2/13/9/5/14/22→`s_struck_sword`（62）、4/17/10/15/16/23→`s_struck_do`（63）、3/7/11→`s_struck_axe`（64）、24→`s_struck_club`（65）、8/12/18/21→`s_struck_wooden`（`:2349`，原为 `s_struck_long` 已改，行内注释 `//long`）；**else 分支被注释**（`:2350` `//else struckweaponsound := s_struck_fist;`），其他武器保持 -1 不发声。

**魔法三段声（`Actor.pas:2297-2302`）**：`m_boUseMagic` 且 `m_CurMagic.MagicSerial > 0` 时：

```pascal
m_nMagicStartSound     := 10000 + m_CurMagic.MagicSerial * 10;      // 起始
m_nMagicFireSound      := 10000 + m_CurMagic.MagicSerial * 10 + 1;  // 飞行
m_nMagicExplosionSound := 10000 + m_CurMagic.MagicSerial * 10 + 2;  // 爆炸
```

#### 怪物分支（m_btRace <> 0）

**受击声（`Actor.pas:2305-2321`）**：`SM_STRUCK` 时按攻击者武器 → `s_struck_body_*` 肉体声（`:2312-2317`，怪物无护甲概念；分组与人形略有差异：3/11 → axe，无 7）。

**外观公式（`Actor.pas:2323-2332`）**：`m_btRace = 50`（NPC 类）跳过（`:2323`）；否则按 `m_wAppearance`：

```pascal
m_nAppearSound := 200 + m_wAppearance * 10;      // +0 出现
m_nNormalSound := 200 + m_wAppearance * 10 + 1;  // +1 平时
m_nAttackSound := 200 + m_wAppearance * 10 + 2;  // +2 攻击
m_nWeaponSound := 200 + m_wAppearance * 10 + 3;  // +3 武器
m_nScreamSound := 200 + m_wAppearance * 10 + 4;  // +4 惨叫
m_nDieSound    := 200 + m_wAppearance * 10 + 5;  // +5 死亡
m_nDie2Sound   := 200 + m_wAppearance * 10 + 6;  // +6 死亡2
```

即每种怪物外观占用 10 个连续音效索引（实际用 7 个），`sound.lst` 按此约定排布；缺口由 `PlaySound` 静默跳过。

#### 播放时机

**RunSound（`Actor.pas:2357-2390`）— 动作起始一次性**：置 `m_boRunSound := TRUE` 并调 `SetSound`（`:2359-2360`）后按动作分发：

| 动作 | 播放 | 行号 |
|------|------|------|
| `SM_STRUCK` | `m_nStruckWeaponSound` + `m_nStruckSound` + `m_nScreamSound`（各 `>= 0` 门控） | `:2362-2367` |
| `SM_NOWDEATH` | `m_nDieSound`；若 `Self = g_MySelf` 追加 `PlayBGM(bmg_gameover)` | `:2368-2376` |
| `SM_THROW`/`SM_HIT`/`SM_FLYAXE`/`SM_LIGHTING`/`SM_DIGDOWN` | `m_nAttackSound`（`>= 0` 门控） | `:2377-2380` |
| `SM_ALIVE`/`SM_DIGUP` | `m_nAppearSound` | `:2381-2384` |
| `SM_SPELL` | `m_nMagicStartSound` | `:2385-2388` |

**RunActSound(frame)（`Actor.pas:2392-2472`）— 帧驱动**：以 `m_boRunSound` 为门控，播放后立即清 `FALSE`（一次动作只发一轮声）。

人形（`m_btRace = 0`）：

| 动作 | 帧 | 播放 | 行号 |
|------|-----|------|------|
| `SM_THROW`/`SM_HIT`/`SM_HIT+1`/`SM_HIT+2` | frame = 2 | `m_nWeaponSound` | `:2397-2401` |
| `SM_POWERHIT`（攻杀） | frame = 2 | `m_nWeaponSound` + 按性别 `s_yedo_man`/`s_yedo_woman` | `:2402-2409` |
| `SM_LONGHIT`（攻杀剑术） | frame = 2 | `m_nWeaponSound` + `s_longhit`（132） | `:2410-2415` |
| `SM_WIDEHIT`（半月） | frame = 2 | `m_nWeaponSound` + `s_widehit`（133） | `:2416-2421` |
| `SM_FIREHIT`（烈火） | frame = 2 | `m_nWeaponSound` + `s_firehit`（137） | `:2422-2427` |
| `SM_CRSHIT`（逐日，注释 Damian） | frame = 2 | `m_nWeaponSound` + `s_firehit` | `:2428-2433` |
| `SM_TWINHIT`（双龙，注释 Damian） | frame = 2 | `m_nWeaponSound` + `s_firehit` | `:2434-2439` |

怪物（`m_btRace <> 0` 且 ≠ 50）：

| 条件 | 播放 | 行号 |
|------|------|------|
| `SM_WALK`/`SM_TURN`，frame = 1 且 `Random(8) = 1` | `m_nNormalSound`（1/8 概率平时声） | `:2445-2449` |
| `SM_HIT`，frame = 3 且 `m_nAttackSound >= 0` | `m_nWeaponSound` | `:2451-2455` |
| `m_wAppearance = 80`（石头怪类）`SM_NOWDEATH`，frame = 2 | `m_nDie2Sound` | `:2457-2466` |

**魔法飞行/爆炸**：`TActor.Run` 中施法飞行帧播 `m_nMagicFireSound`、爆炸帧播 `m_nMagicExplosionSound`（`Actor.pas:2586/2588`）；`THumActor.Run` 有一组重复实现（`Actor.pas:3647/3649`）。爆炸声另由魔法特效回取施法者字段播放（`magiceff.pas:797/1344`，见 9.4）。

### 9.4 怪物/魔法硬编码音效

**AxeMon.pas（特化怪物子类）**：

| 位置 | 索引 | 上下文 |
|------|------|--------|
| `AxeMon.pas:1534` | 10112 | `TBanyaGuardMon.Run`（`:1487`）`SM_LIGHTING` 帧偏移 4，`m_btRace = 70` 或 81，`NewMagic mtThunder` |
| `AxeMon.pas:1538` | 10012 | 同过程，`m_btRace = 71`，`NewMagic mtFly` |
| `AxeMon.pas:1542` | 2276 | 同过程，`m_btRace = 72`，`NewMagic mt13` |
| `AxeMon.pas:1546` | 2396 | 同过程，`m_btRace = 78`，`NewMagic mt13`（目标=自身） |
| `AxeMon.pas:2496` | 8222 | `TDragonStatue.Run`（`:2449`）`SM_LIGHTING` 且 frame = 4，`NewMagic 74 mtThunder` |
| `AxeMon.pas:2682` | 8206 | `TFireDragon.AttackEff`（`:2653`）生成 `mtThunder` 后 |
| `AxeMon.pas:2778` | 8201 | `TFireDragon.Run`（`:2767`）`m_boRunSound` 为真时一次性出场声（`:2777-2779`） |
| `AxeMon.pas:2815` | 8202 | `TFireDragon.Run` `SM_HIT` 时 `AttackEff` 后 |
| `AxeMon.pas:2821` | 8203 | `TFireDragon.Run` 动作 81/82/83 帧偏移 4，`NewMagic mtFly` |

**magiceff.pas（魔法特效）**：

| 位置 | 索引 | 上下文 |
|------|------|--------|
| `magiceff.pas:630` | 8208 | `TMagicEff.Create`（`:385`）`bt80 = 1`（火龙吐息类标志，`:617`）；注意 `:618` 与 `:622` 有两个相同的 `if id = 81`（第二个应为 id = 82，是 bug，导致 82 的起点坐标调整失效） |
| `magiceff.pas:797` | `TActor(MagOwner).m_nMagicExplosionSound` | `TMagicEff.Shift`（`:678`）飞行命中（`crash` 且 `TargetActor <> nil`，`:789`）转固定爆炸帧时，回取施法者的爆炸声字段 |
| `magiceff.pas:1344` | 同上 | `TBujaukGroundEffect.Run`（`:1332`）到达目标距离阈值（`:1336-1337`）转固定效果时 |

`m_nMagicExplosionSound` 由施法者 `SetSound` 按当前魔法计算为 `10000 + Serial*10 + 2`（`Actor.pas:2301`）——魔法爆炸声的最终来源仍是魔法序号公式，magiceff 只是经 `MagOwner` 回取。

**ClMain.pas DrawEffectHum（SM_716 人形特效）**：消息 `SM_716`（`Grobal2.pas:355`）分发于 `ClMain.pas:4666-4668`，调 `DrawEffectHum(Msg.Series, Msg.Param, Msg.Tag)`（`ClMain.pas:6081`）：

| nType | 索引 | 伴随效果 | 行号 |
|-------|------|----------|------|
| 3 | 48 | `g_WMagic2Images` 690 特效 | `:6094-6096` |
| 4 | 8301 | `NewMagic 70 mtThunder` | `:6098-6100` |
| 5 | 8302 | `NewMagic 71 + 72 mtThunder` | `:6102-6105` |
| 6 | 8207 | `NewMagic 73 mtThunder` | `:6107-6109` |
| 7 | 8226 | `NewMagic 74 mtThunder` | `:6111-6113` |

**其他硬编码点**：

| 位置 | 索引 | 上下文 |
|------|------|--------|
| `Actor.pas:2949` | `Random(7) + 146`（146-152 随机） | `TNpcActor.CalcActorFrame` `SM_DIGUP` 分支（`:2943`）且 `m_wAppearance = 52`（`:2945`），NPC 出土 |
| `Actor.pas:3498` | `s_strike_stone`（91） | `THumActor.RunFrameAction` `SM_HEAVYHIT` 且 frame = 5 且 `m_boDigFragment`（`:3492-3494`），挖矿碎石；`:3499` 的 `s_drop_stonepiece`（92）被注释 |

### 9.5 UI 音效

**按钮点击声机制（TClickSound）**：

1. 枚举 `TClickSound = (csNone, csStone, csGlass, csNorm)`（`DWinCtl.pas:11`）；事件类型 `TOnClickSound = procedure(Sender; ClickSound: TClickSound) of object`（`:24`）。
2. `TDButton` 持有 `FClickSound`（`:100`）与 `FOnClickSound`（`:102`），构造默认 `csNone`（`:651`）。
3. `TDButton.MouseUp`（`DWinCtl.pas:677`）：鼠标在控件区域内抬起（`InRange`）时，**先**触发 `FOnClickSound(self, FClickSound)`（`:684`），**再**触发 `FOnClick`（`:685`）——声音先于点击逻辑。
4. 处理函数 `TFrmDlg.DLoginNewClickSound`（`FState.pas:2376-2384`）：`csNorm` → `s_norm_button_click`（103）、`csStone` → `s_rock_button_click`（104）、`csGlass` → `s_glass_button_click`（105）、`csNone` 无声。
5. 绑定在 **FState.dfm** 而非代码——各 `TDButton`/`TDWindow` 在 dfm 中声明 `ClickCount = csXxx` 与 `OnClickSound = DLoginNewClickSound`（如 `DMyBag`/`DOption` 为 csGlass，`DLoginOk`/`DscStart` 为 csStone，`DCloseBag` 为 csNorm）。

**ItemClickSound 调用点（FState.pas）**：

| 位置 | 所属 UI 操作 |
|------|--------------|
| `FState.pas:3358` | `DSWWeaponClick`（`:3257`）：移动物品放入装备槽 `g_UseItems[n]` |
| `FState.pas:3365` | `DSWWeaponClick`：穿戴（`SendTakeOnItem`，`:3369`） |
| `FState.pas:3394` | `DSWWeaponClick`：点装备槽脱下 |
| `FState.pas:3877` | `DBelt1Click`（`:3868`）：腰带格拾取（`:3886` 的手持物品版本已注释） |
| `FState.pas:4571` | `DItemGridGridSelect`（`:4557`）：背包格拾取（`:4574` 已注释） |
| `FState.pas:5202 / :5211` | `DSellDlgSpotClick`（`:5195`）：卖出窗拾取/放入 |
| `FState.pas:5737 / :5742` | `DDGridGridSelect`（`:5722`）：交易窗格取/放 |

**UI 层 PlaySound 调用点**：

| 位置 | 声音 | 场景 |
|------|------|------|
| `FState.pas:4668` | `s_money`（106） | `DGoldClick`：点金币显示 |
| `FState.pas:4981` | `s_glass_button_click`（105） | `DMenuDlgClick`：NPC 商店菜单 |
| `FState.pas:5129` | `s_glass_button_click`（105） | `DMerchantDlgClick`：NPC 对话窗 |
| `FState.pas:5837` | `s_money`（106） | `DDGoldClick`：交易窗金币调整 |

### 9.6 其他触发点

| 位置 | 声音 | 场景 |
|------|------|------|
| `ClMain.pas:1971` | `ItemUseSound(StdMode)` | `EatItem`（`:1944`）吃背包物品（`SendEat` 后） |
| `ClMain.pas:1998` | `ItemUseSound(StdMode)` | `EatItem` 吃手持物品 `g_EatingItem` |
| `ClMain.pas:4481` | `s_money`（106） | `SM_GOLDCHANGED`（`:4479`）金币增减 |
| `ClMain.pas:4831` | `s_money`（106） | `SM_DEALREMOTECHGGOLD`（`:4829`，注释「对方金币改变时播放声音」） |
| `Actor.pas:1638 / :1643` | `s_spacemove_out`（109） | `SM_SPACEMOVE_HIDE` / `SM_SPACEMOVE_HIDE2`：瞬移离场（配 `TScrollHideEffect`） |
| `Actor.pas:1650 / :1657` | `s_spacemove_in`（110） | `SM_SPACEMOVE_SHOW` / `SM_SPACEMOVE_SHOW2`：瞬移入场（配 `TCharEffect`） |

其余文件经 `findstr PlaySound PlayBGM PlayMp3 PlayMapMusic SilenceSound ItemClickSound ItemUseSound EffectFile` 全量核查：`clEvent.pas`、`ClFunc.pas`、`DrawScrn.pas`、`wmUtil.pas`、`HerbActor.pas`、`cliUtil.pas`、`MapUnit.pas` **均无发声调用**——第九/十章清单即全集。

---

## 十、资源文件与目录

| 资源 | 路径 | 格式/约定 | 加载点 |
|------|------|-----------|--------|
| 音效索引表 | `.\wav\sound.lst` | 文本；`索引: 相对路径`；`;` 注释；索引严格递增 | `ClMain.pas:515-519` |
| WAV 音效/BGM | `wav\` 目录（索引表内相对路径） | 标准 PCM WAV（RIFF/fmt/data）；整段进内存 | `SoundUtil.pas:187/259` |
| 场景 BGM | `wav\log-in-long2.wav`、`wav\sellect-loop2.wav`、`wav\game over2.wav` | 循环 WAV（`DSBPLAY_LOOPING`）；注意 `game over2.wav` 含空格、`sellect` 为原文拼写 | `SoundUtil.pas:31-34` |
| 地图音乐 | `.\Music\<编号>.mp3` | 编号来自 `SM_MAPDESCRIPTION` 的 `Msg.Recog` | `SoundUtil.pas:219` |

所有路径相对客户端工作目录；文件缺失经 `FileExists` 门控静默跳过（`SoundUtil.pas:158/185/256/277`），无声卡经 nil 守卫静默降级（`:182/270`）。

---

## 十一、审查发现的实现瑕疵

| # | 瑕疵 | 位置 | 影响 |
|---|------|------|------|
| 1 | `TMPEG.Play` 返回值恒 `False` | `Mpeg.pas:87-102` | 调用方无法判断播放成功（当前调用方不检查，无实害） |
| 2 | `TMPEG.Pause` 无 Assigned 防护 | `Mpeg.pas:77-80` | 未初始化调用将访问违例（游戏未调 Pause） |
| 3 | `TMPEG.Init` 失败不回滚已建接口 | `Mpeg.pas:65-75` | 失败路径 COM 接口小泄漏 |
| 4 | `TMPEG.Close` 无条件 `CoUninitialize` | `Mpeg.pas:43` | Init/Uninit 配对依赖「Play 过才 Stop」的调用习惯 |
| 5 | `MP3` 对象从不释放 | `ClMain.pas:824-840` | 进程退出前 DirectShow 图资源不回收 |
| 6 | `PlayBGM` 返回值恒 nil（签名残留） | `SoundUtil.pas:252` | 返回 `TDirectSoundBuffer` 但从不赋值，误导性接口 |
| 7 | `s_intro_theme` 与 `s_main_theme` 同为 102 | `SoundUtil.pas:109/111` | 常量语义冲突（前者无调用点，无实害） |
| 8 | `LoadBGMusicList` 废弃但仍被调用且泄漏 | `SoundUtil.pas:222-248`、`ClMain.pas:518-519` | 喂入 `sound.lst` 产生垃圾表项；`^String` 堆指针从不 Dispose |
| 9 | 音效无缓存、无复音上限、每次读盘 | `DXSounds.pas:1852-1881` | 密集发声时磁盘 I/O 与缓冲数瞬时膨胀 |
| 10 | 单缓冲仅支持一层 Lock | `DXSounds.pas:55-56/806` | 限制流式扩展（游戏未触及） |
| 11 | `sound.lst` 索引必须升序书写 | `SoundUtil.pas:168` | 乱序/重复行静默丢弃，配置错误难发现 |
| 12 | `g_boBGSound` 无切换入口、开关不持久化 | `MShare.pas:213-214`、`ClMain.pas:646-647` | 玩家无法关闭 BGM；音效设置重启即失 |
| 13 | `magiceff` 重复 `if id = 81` | `magiceff.pas:618/622` | id = 82 的起点坐标调整失效（应为 82 误写 81） |
| 14 | 死代码：`CurVolume`/`SoundInfo`/`bmg_field`/`OpenSoundOption` | `SoundUtil.pas:12/25-28/33`、`FState.pas:1774-1782` | — |
| 15 | 游戏主题曲定时器死逻辑 | `PlayScn.pas:421-425/485-486` | 46 秒循环 `s_main_theme` 永不触发，游戏内 BGM 全靠 MP3 通道 |
| 16 | `ItemClickSound` 按中文物品名子串判断类别 | `SoundUtil.pas:300-305` | 与 GBK 编码及物品命名强耦合（「手镯」「手套」） |

---

## 十二、附录：发声调用点完整清单

**ClMain.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:1971` | `ItemUseSound(StdMode)` | EatItem 吃背包物品 |
| `:1998` | `ItemUseSound(StdMode)` | EatItem 吃手持物品 |
| `:4481` | `PlaySound(s_money)` | SM_GOLDCHANGED |
| `:4831` | `PlaySound(s_money)` | SM_DEALREMOTECHGGOLD |
| `:5223` | `PlayMapMusic(True)` | SM_MAPDESCRIPTION / ClientGetMapDescription |
| `:6096` | `PlaySound(48)` | DrawEffectHum nType=3 |
| `:6100` | `PlaySound(8301)` | DrawEffectHum nType=4 |
| `:6105` | `PlaySound(8302)` | DrawEffectHum nType=5 |
| `:6109` | `PlaySound(8207)` | DrawEffectHum nType=6 |
| `:6113` | `PlaySound(8226)` | DrawEffectHum nType=7 |

**IntroScn.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:518` | `PlayBGM(bmg_intro)` | TLoginScene.OpenScene |
| `:527` | `SilenceSound` | TLoginScene.CloseScene |
| `:801` | `PlaySound(s_rock_door_open)` | OpenLoginDoor 石门动画 |
| `:1145` | `SilenceSound` | TSelectChrScene.CloseScene |
| `:1152` | `PlayBGM(bmg_select)` | TSelectChrScene.SoundOnTimer |
| `:1170` | `PlaySound(s_meltstone)` | SelChrSelect1Click |
| `:1187` | `PlaySound(s_meltstone)` | SelChrSelect2Click |

**FState.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:2380` | `PlaySound(s_norm_button_click)` | DLoginNewClickSound / csNorm |
| `:2381` | `PlaySound(s_rock_button_click)` | csStone |
| `:2382` | `PlaySound(s_glass_button_click)` | csGlass |
| `:3358` | `ItemClickSound` | 装备入槽 |
| `:3365` | `ItemClickSound` | 穿戴 |
| `:3394` | `ItemClickSound` | 脱下 |
| `:3877` | `ItemClickSound` | 腰带拾取 |
| `:4571` | `ItemClickSound` | 背包拾取 |
| `:4668` | `PlaySound(s_money)` | DGoldClick |
| `:4981` | `PlaySound(s_glass_button_click)` | NPC 商店菜单 |
| `:5129` | `PlaySound(s_glass_button_click)` | NPC 对话窗 |
| `:5202 / :5211` | `ItemClickSound` | 卖出窗取/放 |
| `:5737 / :5742` | `ItemClickSound` | 交易窗放/取 |
| `:5837` | `PlaySound(s_money)` | 交易金币调整 |

**Actor.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:1638 / :1643` | `PlaySound(s_spacemove_out)` | SM_SPACEMOVE_HIDE / HIDE2 |
| `:1650 / :1657` | `PlaySound(s_spacemove_in)` | SM_SPACEMOVE_SHOW / SHOW2 |
| `:2364-2366` | `PlaySound(StruckWeapon/Struck/Scream)` | RunSound / SM_STRUCK |
| `:2371` | `PlaySound(m_nDieSound)` | RunSound / SM_NOWDEATH |
| `:2374` | `PlayBGM(bmg_gameover)` | SM_NOWDEATH 且 Self = g_MySelf |
| `:2379` | `PlaySound(m_nAttackSound)` | RunSound / SM_THROW,HIT,FLYAXE,LIGHTING,DIGDOWN |
| `:2383` | `PlaySound(m_nAppearSound)` | RunSound / SM_ALIVE,DIGUP |
| `:2387` | `PlaySound(m_nMagicStartSound)` | RunSound / SM_SPELL |
| `:2399 / :2404` | `PlaySound(m_nWeaponSound)` | RunActSound 人形命中帧 frame=2 |
| `:2406-2407` | `PlaySound(s_yedo_man / s_yedo_woman)` | SM_POWERHIT 按性别配音 |
| `:2412-2413` | `WeaponSound + s_longhit` | SM_LONGHIT |
| `:2418-2419` | `WeaponSound + s_widehit` | SM_WIDEHIT |
| `:2424-2425 / :2430-2431 / :2436-2437` | `WeaponSound + s_firehit` | SM_FIREHIT / SM_CRSHIT / SM_TWINHIT |
| `:2447` | `PlaySound(m_nNormalSound)` | 怪物 SM_WALK/TURN frame=1 且 Random(8)=1 |
| `:2453` | `PlaySound(m_nWeaponSound)` | 怪物 SM_HIT frame=3 |
| `:2462` | `PlaySound(m_nDie2Sound)` | appearance=80 SM_NOWDEATH frame=2 |
| `:2586 / :2588` | `MagicFire / MagicExplosionSound` | TActor.Run 施法飞行/爆炸帧 |
| `:2659-2660` | `PlaySound(FootStep / FootStep+1)` | 走/跑 frame 1/4 左右脚 |
| `:2949` | `PlaySound(Random(7) + 146)` | TNpcActor SM_DIGUP appearance=52 |
| `:3498` | `PlaySound(s_strike_stone)` | SM_HEAVYHIT 挖矿碎石 |
| `:3647 / :3649` | `MagicFire / MagicExplosionSound` | THumActor.Run 施法（与 :2586/:2588 重复） |

**AxeMon.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:1534` | `PlaySound(10112)` | TBanyaGuardMon SM_LIGHTING race 70/81 |
| `:1538` | `PlaySound(10012)` | race 71 |
| `:1542` | `PlaySound(2276)` | race 72 |
| `:1546` | `PlaySound(2396)` | race 78 |
| `:2496` | `PlaySound(8222)` | TDragonStatue SM_LIGHTING |
| `:2682` | `PlaySound(8206)` | TFireDragon.AttackEff |
| `:2778` | `PlaySound(8201)` | TFireDragon 出场 |
| `:2815` | `PlaySound(8202)` | TFireDragon SM_HIT |
| `:2821` | `PlaySound(8203)` | TFireDragon 动作 81/82/83 |

**magiceff.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:630` | `PlaySound(8208)` | TMagicEff.Create bt80=1 |
| `:797` | `PlaySound(MagOwner.m_nMagicExplosionSound)` | TMagicEff.Shift 命中爆炸 |
| `:1344` | `PlaySound(MagOwner.m_nMagicExplosionSound)` | TBujaukGroundEffect.Run 爆炸 |

**PlayScn.pas**

| 行号 | 调用 | 场景 |
|------|------|------|
| `:423` | `PlaySound(s_main_theme)` | SoundOnTimer（定时器已在 `:485-486` 注释停用，运行时不触发） |
| `:492` | `SilenceSound` | TPlayScene.CloseScene |

**SoundUtil.pas（门面层内部，非业务触发点）**

| 行号 | 调用 | 说明 |
|------|------|------|
| `:187` | `g_Sound.EffectFile(file, FALSE, FALSE)` | PlaySound 实现 |
| `:216` | `PlayMp3('', False)` | PlayMapMusic 停止分支 |
| `:220` | `PlayMp3('.\Music\' + g_nMapMusic + '.mp3')` | PlayMapMusic 播放分支 |
| `:258-259` | `SilenceSound` + `EffectFile(wav, TRUE, FALSE)` | PlayBGM（循环） |
| `:272 / :279-280` | `MP3.Stop` / `MP3.Stop + MP3.Play` | PlayMp3 |
| `:289` | `g_Sound.Clear` | SilenceSound 实现 |
