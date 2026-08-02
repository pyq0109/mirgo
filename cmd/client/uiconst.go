package main

// 屏幕布局 — Delphi Share.pas:16-36, SWH800 分支 (固定 800×600).
const (
	ScreenWidth  = 800
	ScreenHeight = 600
	// 地图绘制面高度 (Delphi MAPSURFACEHEIGHT, Share.pas:31).
	MapSurfaceH = 445
	// 底栏图像 Prguse[1] 为 800×251; DBottom 底部锚定于
	// Top = 600-251 = 349 (FState:1184-1189, :3577 处
	// btop := SCREENHEIGHT-d.Height). 其上 120px 与地图绘制面重叠
	// (颜色键混合). buildHUD 在运行时按真实图像高度修正 Top.
	BottomBarImageH = 251
	BottomBarTop    = ScreenHeight - BottomBarImageH // 349

	// 窗口拖动边界钳制 (Share.pas:33-36).
	WinLeft = 60
	WinTop  = 60
)

// HUD / 聊天常量 (FState.pas:10-16).
const (
	ViewChatLine   = 9  // 可见聊天行数
	MaxStatePage   = 4  // 状态面板页数
	ListLineHeight = 13 // 商店/仓库列表行高
	MaxMenu        = 10 // 商店列表可见行数
)

// Prguse.wil UI 图像索引.
const (
	ImgBottomBar = 1 // BOTTOMBOARD800 (FState.pas:11)
	ImgBagBg     = 3 // DItemBag 背景 (FState:1167)
	ImgHPMPBar   = 4 // HP/MP 双球 (FState:3624)
	ImgWarHPBase = 5 // 28 级以下战士单血条底座 (FState:3610)
	ImgWarHPFill = 6 // 28 级以下战士单血条液体 (FState:3616)
	ImgStripBar  = 7 // 经验 + 负重条 (FState:3647)

	ImgBtnState  = 8 // 右下角状态按钮 (FState:1194-1203)
	ImgBtnBag    = 9
	ImgBtnMagic  = 10
	ImgBtnOption = 11

	ImgDayMorning = 12 // g_nDayBright 1 (FState:3599)
	ImgDayNoon    = 13 // g_nDayBright 2
	ImgDayDusk    = 14 // g_nDayBright 3
	ImgDayNight   = 15 // g_nDayBright 0 (FState:3598)
	ImgHunger1    = 16 // g_nMyHungryState 1..4 (FState:3683)

	ImgBeltSlot  = 0  // 隐形腰带格 (MShare:495-500)
	ImgDealGold  = 28 // 交易对话框金币按钮, 己方 + 对方 (FState:1475,1489)
	ImgGoldBtn   = 29 // 背包金币按钮 (MShare:501)
	ImgRepairBtn = 26 // (MShare:502)

	ImgCloseSmall = 371 // 小关闭按钮, 到处都用 (FState:1076...)
	ImgCloseMed   = 64  // 中关闭按钮 (MShare:505)
	ImgConfirm    = 362 // 确认/确定 (FState:1336,1352)
	ImgCancel     = 366 // 取消/关闭 (FState:1339,1355)

	ImgStateBg      = 370 // 状态/装备面板背景 (FState:983)
	ImgBodyMale     = 376 // 纸娃娃男性 (FState:2806)
	ImgBodyFemale   = 377 // 纸娃娃女性 (FState:2808)
	ImgStatePage2Bg = 382 // 详细属性页背景 (FState:2874)
	ImgStatePage3Bg = 383 // 魔法页背景 (FState:2934)
	ImgPageUp       = 387 // 上一页 (FState:1079)
	ImgPageDown     = 388 // 下一页 (FState:1080)
	ImgScrollUp     = 372 // 列表上滚 (FState:1540)
	ImgScrollDown   = 373 // 列表下滚 (FState:1543)

	ImgNpcDlg  = 384 // NPC 对话框背景 (FState:1301)
	ImgShopBg  = 385 // 购买列表背景 (FState:1328)
	ImgShopBuy = 386 // 购买按钮 (FState:1338)
	ImgSellBg  = 392 // 出售/修理/仓库背景 (FState:1350)
	ImgSellOk  = 393 // 出售确定 (MShare:515)
	ImgDealBg  = 389 // 交易己方面板 (FState:1463)
	ImgDealRem = 390 // 交易对方面板 (FState:1483)
	ImgDealOk  = 391 // 交易确认 (FState:1469)

	ImgModalSmall  = 381 // DMsgDlg 尺寸 (FState:2007)
	ImgModalNormal = 360 // (FState:2020)
	ImgModalTall   = 380 // (FState:2033)
	ImgModalOk     = 361 // (FState:745)
	ImgModalYes    = 363 // (FState:746)
	ImgModalCancel = 365 // (FState:747)
	ImgModalNo     = 367 // (FState:748)

	ImgHintBg = 394 // 提示框背景 (DrawScrn.pas:426)

	ImgGuildBg = 180 // 行会面板 (FState:1499)
	// 行会按钮 (FState:1504-1543), 图像 182-202 + 滚动 372/373.
	ImgGuildAddMem     = 182
	ImgGuildAlly       = 184
	ImgGuildBreakAll   = 186
	ImgGuildCancelWar  = 188
	ImgGuildChat       = 190
	ImgGuildDelMem     = 192
	ImgGuildEditGrade  = 194
	ImgGuildEditNotice = 196
	ImgGuildHome       = 198
	ImgGuildList       = 200
	ImgGuildWar        = 202
	ImgGuildNoticeBg   = 204 // 公告编辑器模态窗口 (FState:1546)

	ImgGroupBg     = 120 // 组队对话框 (FState:1439)
	ImgGroupCreate = 123 // (FState:1447)
	ImgGroupDel    = 124 // (FState:1453)
	ImgGroupAdd    = 125 // (FState:1450)
	ImgGroupAllow  = 347 // (FState:1444)

	ImgAdjustBg    = 226 // 属性调整面板 (FState:1557)
	ImgAdjustPlus  = 224 // (FState:1565-1591)
	ImgAdjustMinus = 225 // (FState:1593-1619)
	ImgAdjustOk    = 230 // (FState:1561)

	ImgKeyDlg    = 229 // 按键选择对话框 (FState:1367, 覆盖 DlgConf 620)
	ImgKeyF1     = 232 // F1..F8 = 232,234,236,238,240,242,244,246 (FState:1375-1398)
	ImgKeyNone   = 231 // (FState:1399)
	ImgKeyOk     = 230 // (FState:1402)
	ImgMagicLv   = 23  // 魔法列表 "lv" 标记 (FState:2970)
	ImgMagicExp  = 22  // 魔法列表 "exp" 标记 (FState:2973)
	ImgKeyDigit1 = 248 // 数字键 '1'..'8' = 248..255 (FState:2947-2954)

	ImgHairMale   = 440 // 男性发型基址: 440 + hair/2 (FState:2817)
	ImgHairFemale = 441 // 女性发型基址: 441 + hair/2 (FState:2818)
)

// 选角/创角场景 (IntroScn.pas + FState.pas:904-965).
const (
	ImgSelBg      = 65 // 选角界面背景 (IntroScn:1375)
	ImgSelSelect1 = 66 // DscSelect1 (FState:904)
	ImgSelSelect2 = 67 // DscSelect2
	ImgSelStart   = 68 // DscStart
	ImgSelNewChr  = 69 // DscNewChr
	ImgSelErase   = 70 // DscEraseChr
	ImgSelExit    = 72 // DscExit

	ImgCreateBg     = 73 // 创角窗口背景 (FState:930)
	ImgCreateJob1   = 74 // DccWarrior (FState:936)
	ImgCreateJob2   = 75 // DccWizzard
	ImgCreateJob3   = 76 // DccMonk
	ImgCreateMale   = 77 // DccMale (FState:940)
	ImgCreateFemale = 78 // DccFemale
	ImgCreateOk     = 51 // DccOk (FState:944)
	ImgCreateCancel = 52 // DccClose (FState:945)

	// 职业/性别选择高亮, 叠画在烘焙好的窗口面上
	// (DccCloseDirectPaint, FState:2737-2756).
	ImgClassHi1  = 55 // 选中战士
	ImgClassHi2  = 56 // 选中法师
	ImgClassHi3  = 57 // 选中道士
	ImgGenderHiM = 58 // 选中男性
	ImgGenderHiF = 59 // 选中女性
)

// 底栏功能按钮 (DlgConf, MShare.pas:474-494; 应用见 FState:1210-1239).
const (
	ImgBotGroup    = 128
	ImgBotMinimap  = 130
	ImgBotTrade    = 132
	ImgBotGuild    = 134
	ImgBotLogout   = 136
	ImgBotExit     = 138
	ImgBotPlusAbil = 140
	ImgBotFriend   = 530
	ImgBotMemo     = 532
)

// Items.wil 常量.
const (
	ItemImgGold = 115 // 金币图素 (ClMain.pas:1096)
)

// DlgEntry 对应 Delphi TConfig 记录 (MShare.pas:468-541).
// Left/Top 相对于所属窗口: HUD 控件的父级是 DBottom
// (屏幕 y = BottomBarTop + Top), 面板控件相对于各自面板.
// Width/Height 为 0 表示按图像自动取尺寸.
// 注意: Delphi 按控件选择性地应用此表 (FState:1210-1239);
// 部分条目是死数据 (如 DItemGrid — FState:1171-1174 硬编码为
// 33,43). 使用前先核实各控件的实际赋值.
type DlgEntry struct {
	Image int
	Left  int
	Top   int
	W     int
	H     int
}

var DlgConf = map[string]DlgEntry{
	"DBottom":      {ImgBottomBar, 0, 0, 0, 0},
	"DMyState":     {ImgBtnState, 643, 61, 0, 0},
	"DMyBag":       {ImgBtnBag, 682, 41, 0, 0},
	"DMyMagic":     {ImgBtnMagic, 722, 21, 0, 0},
	"DOption":      {ImgBtnOption, 764, 11, 0, 0},
	"DBotMiniMap":  {ImgBotMinimap, 219, 104, 0, 0},
	"DBotTrade":    {ImgBotTrade, 249, 104, 0, 0},
	"DBotGuild":    {ImgBotGuild, 279, 104, 0, 0},
	"DBotGroup":    {ImgBotGroup, 309, 104, 0, 0},
	"DBotPlusAbil": {ImgBotPlusAbil, 339, 104, 0, 0},
	"DBotFriend":   {ImgBotFriend, 369, 104, 0, 0},
	"DBotMemo":     {ImgBotMemo, 753, 204, 0, 0}, // y 超出屏幕 — 可疑, 需核对 dfm
	"DBotExit":     {ImgBotExit, 560, 104, 0, 0},
	"DBotLogout":   {ImgBotLogout, 530, 104, 0, 0},
	"DBelt1":       {ImgBeltSlot, 285, 59, 32, 29},
	"DBelt2":       {ImgBeltSlot, 328, 59, 32, 29},
	"DBelt3":       {ImgBeltSlot, 371, 59, 32, 29},
	"DBelt4":       {ImgBeltSlot, 415, 59, 32, 29},
	"DBelt5":       {ImgBeltSlot, 459, 59, 32, 29},
	"DBelt6":       {ImgBeltSlot, 503, 59, 32, 29},
	"DGold":        {ImgGoldBtn, 10, 190, 0, 0},       // 父级 DItemBag
	"DRepairItem":  {ImgRepairBtn, 254, 183, 48, 22},  // 父级 DItemBag
	"DClosebag":    {ImgCloseSmall, 309, 203, 14, 20}, // 父级 DItemBag

	"DMerchantDlg":      {ImgNpcDlg, 0, 0, 0, 0},
	"DMerchantDlgClose": {ImgCloseMed, 399, 1, 0, 0},

	"DMenuDlg":   {ImgShopBg, 138, 163, 0, 0}, // 运行时 ShowShopMenuDlg → (0,170)
	"DMenuPrev":  {ImgPageDown, 43, 175, 0, 0},
	"DMenuNext":  {ImgPageUp, 90, 175, 0, 0},
	"DMenuBuy":   {ImgShopBuy, 215, 171, 0, 0},
	"DMenuClose": {ImgCloseMed, 291, 0, 0, 0},

	"DSellDlg":      {ImgSellBg, 328, 163, 0, 0}, // 运行时 ShowShopSellDlg → (260,170)
	"DSellDlgOk":    {ImgSellOk, 85, 150, 0, 0},
	"DSellDlgClose": {ImgCloseMed, 115, 0, 0, 0},
	"DSellDlgSpot":  {0, 27, 67, 61, 52}, // FState:1358-1361

	"DItemGrid": {0, 33, 43, 286, 162}, // FState:1171-1174 (DlgConf 中的 29,41 是死数据)
}

// 按键选择对话框布局 (FState:1375-1404; 该 FState 段覆盖 DlgConf).
var keySelButtons = []DlgEntry{
	{ImgKeyF1 + 0, 25, 78, 0, 0},   // F1
	{ImgKeyF1 + 2, 57, 78, 0, 0},   // F2
	{ImgKeyF1 + 4, 89, 78, 0, 0},   // F3
	{ImgKeyF1 + 6, 121, 78, 0, 0},  // F4
	{ImgKeyF1 + 8, 160, 78, 0, 0},  // F5
	{ImgKeyF1 + 10, 192, 78, 0, 0}, // F6
	{ImgKeyF1 + 12, 224, 78, 0, 0}, // F7
	{ImgKeyF1 + 14, 256, 78, 0, 0}, // F8
}
