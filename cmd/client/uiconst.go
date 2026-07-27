package main

// Screen layout — Delphi Share.pas:16-36, SWH800 branch (fixed 800×600).
const (
	ScreenWidth  = 800
	ScreenHeight = 600
	// Map drawing surface height (Delphi MAPSURFACEHEIGHT, Share.pas:31).
	MapSurfaceH = 445
	// Bottom bar image Prguse[1] is 800×251; DBottom is bottom-anchored at
	// Top = 600-251 = 349 (FState:1184-1189, btop := SCREENHEIGHT-d.Height
	// at :3577). Its upper 120px overlap the map surface (color-key blend).
	// buildHUD refines Top from the real image height at runtime.
	BottomBarImageH = 251
	BottomBarTop    = ScreenHeight - BottomBarImageH // 349

	// Window drag clamps (Share.pas:33-36).
	WinLeft   = 60
	WinTop    = 60
	WinRight  = ScreenWidth - 60
	WinBottom = ScreenHeight - 30
)

// HUD / chat constants (FState.pas:10-16).
const (
	ViewChatLine   = 9  // visible chat lines
	MaxStatePage   = 4  // state panel pages
	ListLineHeight = 13 // shop/storage list row height
	MaxMenu        = 10 // shop list visible rows
)

// Prguse.wil UI image indices.
const (
	ImgBottomBar = 1 // BOTTOMBOARD800 (FState.pas:11)
	ImgBagBg     = 3 // DItemBag background (FState:1167)
	ImgHPMPBar   = 4 // HP/MP double orb (FState:3624)
	ImgWarHPBase = 5 // warrior <28 single HP base (FState:3610)
	ImgWarHPFill = 6 // warrior <28 single HP liquid (FState:3616)
	ImgStripBar  = 7 // exp + weight strips (FState:3647)

	ImgBtnState  = 8 // bottom-right state buttons (FState:1194-1203)
	ImgBtnBag    = 9
	ImgBtnMagic  = 10
	ImgBtnOption = 11

	ImgDayMorning = 12 // g_nDayBright 1 (FState:3599)
	ImgDayNoon    = 13 // g_nDayBright 2
	ImgDayDusk    = 14 // g_nDayBright 3
	ImgDayNight   = 15 // g_nDayBright 0 (FState:3598)
	ImgHunger1    = 16 // g_nMyHungryState 1..4 (FState:3683)

	ImgBeltSlot  = 0  // invisible belt cell (MShare:495-500)
	ImgDealGold  = 28 // trade dialog gold button, own + remote (FState:1475,1489)
	ImgGoldBtn   = 29 // bag gold button (MShare:501)
	ImgRepairBtn = 26 // (MShare:502)

	ImgCloseSmall = 371 // small close, ubiquitous (FState:1076...)
	ImgCloseMed   = 64  // medium close (MShare:505)
	ImgConfirm    = 362 // confirm/OK (FState:1336,1352)
	ImgCancel     = 366 // cancel/close (FState:1339,1355)

	ImgStateBg      = 370 // state/equip panel bg (FState:983)
	ImgBodyMale     = 376 // paper doll male (FState:2806)
	ImgBodyFemale   = 377 // paper doll female (FState:2808)
	ImgStatePage2Bg = 382 // detailed stats page bg (FState:2874)
	ImgStatePage3Bg = 383 // magic page bg (FState:2934)
	ImgPageUp       = 387 // prev page (FState:1079)
	ImgPageDown     = 388 // next page (FState:1080)
	ImgScrollUp     = 372 // list scroll up (FState:1540)
	ImgScrollDown   = 373 // list scroll down (FState:1543)

	ImgNpcDlg  = 384 // NPC dialog bg (FState:1301)
	ImgShopBg  = 385 // buy list bg (FState:1328)
	ImgShopBuy = 386 // buy button (FState:1338)
	ImgSellBg  = 392 // sell/repair/storage bg (FState:1350)
	ImgSellOk  = 393 // sell OK (MShare:515)
	ImgDealBg  = 389 // trade own panel (FState:1463)
	ImgDealRem = 390 // trade remote panel (FState:1483)
	ImgDealOk  = 391 // trade confirm (FState:1469)

	ImgModalSmall  = 381 // DMsgDlg sizes (FState:2007)
	ImgModalNormal = 360 // (FState:2020)
	ImgModalTall   = 380 // (FState:2033)
	ImgModalOk     = 361 // (FState:745)
	ImgModalYes    = 363 // (FState:746)
	ImgModalCancel = 365 // (FState:747)
	ImgModalNo     = 367 // (FState:748)

	ImgHintBg = 394 // tooltip background (DrawScrn.pas:426)

	ImgGuildBg = 180 // guild panel (FState:1499)
	// Guild buttons (FState:1504-1543), images 182-202 + scroll 372/373.
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
	ImgGuildNoticeBg   = 204 // notice editor modal (FState:1546)

	ImgGroupBg     = 120 // group dialog (FState:1439)
	ImgGroupCreate = 123 // (FState:1447)
	ImgGroupDel    = 124 // (FState:1453)
	ImgGroupAdd    = 125 // (FState:1450)
	ImgGroupAllow  = 347 // (FState:1444)

	ImgAdjustBg    = 226 // adjust ability panel (FState:1557)
	ImgAdjustPlus  = 224 // (FState:1565-1591)
	ImgAdjustMinus = 225 // (FState:1593-1619)
	ImgAdjustOk    = 230 // (FState:1561)

	ImgKeyDlg    = 229 // key select dialog (FState:1367, overrides DlgConf 620)
	ImgKeyF1     = 232 // F1..F8 = 232,234,236,238,240,242,244,246 (FState:1375-1398)
	ImgKeyNone   = 231 // (FState:1399)
	ImgKeyOk     = 230 // (FState:1402)
	ImgMagicLv   = 23  // magic list "lv" mark (FState:2970)
	ImgMagicExp  = 22  // magic list "exp" mark (FState:2973)
	ImgKeyDigit1 = 248 // key digits '1'..'8' = 248..255 (FState:2947-2954)

	ImgHairMale   = 440 // hair base male: 440 + hair/2 (FState:2817)
	ImgHairFemale = 441 // hair base female: 441 + hair/2 (FState:2818)
)

// Character select / creation scene (IntroScn.pas + FState.pas:904-965).
const (
	ImgSelBg      = 65 // select screen background (IntroScn:1375)
	ImgSelSelect1 = 66 // DscSelect1 (FState:904)
	ImgSelSelect2 = 67 // DscSelect2
	ImgSelStart   = 68 // DscStart
	ImgSelNewChr  = 69 // DscNewChr
	ImgSelErase   = 70 // DscEraseChr
	ImgSelExit    = 72 // DscExit

	ImgCreateBg     = 73 // creation window background (FState:930)
	ImgCreateJob1   = 74 // DccWarrior (FState:936)
	ImgCreateJob2   = 75 // DccWizzard
	ImgCreateJob3   = 76 // DccMonk
	ImgCreateMale   = 77 // DccMale (FState:940)
	ImgCreateFemale = 78 // DccFemale
	ImgCreateOk     = 51 // DccOk (FState:944)
	ImgCreateCancel = 52 // DccClose (FState:945)

	// Class/gender selection highlights drawn over the baked window faces
	// (DccCloseDirectPaint, FState:2737-2756).
	ImgClassHi1  = 55 // warrior selected
	ImgClassHi2  = 56 // mage selected
	ImgClassHi3  = 57 // taoist selected
	ImgGenderHiM = 58 // male selected
	ImgGenderHiF = 59 // female selected
)

// Bottom-bar function buttons (DlgConf, MShare.pas:474-494; applied FState:1210-1239).
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

// Items.wil constants.
const (
	ItemImgGold = 115 // gold coin sprite (ClMain.pas:1096)
)

// DlgEntry mirrors Delphi TConfig record (MShare.pas:468-541).
// Left/Top are relative to the owning window: HUD controls own parent is
// DBottom (screen y = BottomBarTop + Top), panel controls are relative to
// their panel. Width/Height 0 means auto-sized from the image.
// NOTE: Delphi applies this table selectively per control (FState:1210-1239);
// some entries are dead data (e.g. DItemGrid — FState:1171-1174 hardcodes
// 33,43 instead). Verify each control's real assignment before use.
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
	"DBotMemo":     {ImgBotMemo, 753, 204, 0, 0}, // y out of screen — suspect, verify dfm
	"DBotExit":     {ImgBotExit, 560, 104, 0, 0},
	"DBotLogout":   {ImgBotLogout, 530, 104, 0, 0},
	"DBelt1":       {ImgBeltSlot, 285, 59, 32, 29},
	"DBelt2":       {ImgBeltSlot, 328, 59, 32, 29},
	"DBelt3":       {ImgBeltSlot, 371, 59, 32, 29},
	"DBelt4":       {ImgBeltSlot, 415, 59, 32, 29},
	"DBelt5":       {ImgBeltSlot, 459, 59, 32, 29},
	"DBelt6":       {ImgBeltSlot, 503, 59, 32, 29},
	"DGold":        {ImgGoldBtn, 10, 190, 0, 0},       // parent DItemBag
	"DRepairItem":  {ImgRepairBtn, 254, 183, 48, 22},  // parent DItemBag
	"DClosebag":    {ImgCloseSmall, 309, 203, 14, 20}, // parent DItemBag

	"DMerchantDlg":      {ImgNpcDlg, 0, 0, 0, 0},
	"DMerchantDlgClose": {ImgCloseMed, 399, 1, 0, 0},

	"DMenuDlg":   {ImgShopBg, 138, 163, 0, 0}, // runtime ShowShopMenuDlg → (0,170)
	"DMenuPrev":  {ImgPageDown, 43, 175, 0, 0},
	"DMenuNext":  {ImgPageUp, 90, 175, 0, 0},
	"DMenuBuy":   {ImgShopBuy, 215, 171, 0, 0},
	"DMenuClose": {ImgCloseMed, 291, 0, 0, 0},

	"DSellDlg":      {ImgSellBg, 328, 163, 0, 0}, // runtime ShowShopSellDlg → (260,170)
	"DSellDlgOk":    {ImgSellOk, 85, 150, 0, 0},
	"DSellDlgClose": {ImgCloseMed, 115, 0, 0, 0},
	"DSellDlgSpot":  {0, 27, 67, 61, 52}, // FState:1358-1361

	"DItemGrid": {0, 33, 43, 286, 162}, // FState:1171-1174 (DlgConf's 29,41 is dead)
}

// Key select dialog layout (FState:1375-1404; the FState block overrides DlgConf).
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
