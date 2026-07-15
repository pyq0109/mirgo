package protocol

// DefaultMessage is the core message structure for all client-server communication.
// Size: 12 bytes (not 16 as some docs suggest - the 16 is the encoded size)
type DefaultMessage struct {
	Recog  int32  // Recognition/identification code
	Ident  uint16 // Message ID (CM_* or SM_*)
	Param  uint16 // Parameter 1
	Tag    uint16 // Parameter 2
	Series uint16 // Parameter 3
}

// MsgHeader is the binary frame header for RunGate <-> M2Server communication.
// Size: 16 bytes
type MsgHeader struct {
	DwCode         uint32 // Magic code: 0xAA55AA55
	NSocket        int32  // Client socket identifier
	WGSocketIdx    uint16 // Gate socket index
	WIdent         uint16 // Message type (GM_*)
	WUserListIndex uint16 // User list index
	NLength        int32  // Payload length
}

// RunGateCode is the magic number for RunGate protocol
const RunGateCode = 0xAA55AA55

// GM_* - Gate Message types (RunGate <-> M2Server)
const (
	GMOpen           = 1  // New client connection
	GMClose          = 2  // Client disconnected
	GMCheckServer    = 3  // Keep-alive from server
	GMCheckClient    = 4  // Keep-alive from client
	GMData           = 5  // Game data message
	GMServerUserIndex = 6 // Assign server-side user index
	GMReceiveOK      = 7  // Flow control acknowledgment
	GMTest           = 20 // Test/benchmark message
)

// CharDesc describes a character's appearance for movement/action messages.
// Size: 8 bytes
type CharDesc struct {
	Feature int32 // Appearance encoding
	Status  int32 // Status flags/buffs
}

// MessageBodyW is a Word-sized extended message body.
// Size: 8 bytes
type MessageBodyW struct {
	Param1 uint16
	Param2 uint16
	Tag1   uint16
	Tag2   uint16
}

// MessageBodyWL is a DWord-sized extended message body.
// Size: 16 bytes
type MessageBodyWL struct {
	LParam1 int32
	LParam2 int32
	LTag1   int32
	LTag2   int32
}

// ShortMessage is a compact message format.
// Size: 4 bytes
type ShortMessage struct {
	Ident uint16
	WMsg  uint16
}

// ============================================================================
// CM_* - Client to Server Message IDs
// ============================================================================

// Character management
const (
	CMQueryUsername = 80
	CMQueryBagItems = 81
	CMQueryUserState = 82

	CMQueryChr     = 100
	CMNewChr       = 101
	CMDelChr       = 102
	CMSelChr       = 103
	CMSelectServer = 104
)

// Game operations
const (
	CMDropItem           = 1000
	CMPickup             = 1001
	CMOpenDoor           = 1002
	CMTakeOnItem         = 1003
	CMTakeOffItem        = 1004
	CMEat                = 1006
	CMButch              = 1007
	CMMagicKeyChange     = 1008
	CMClickNPC           = 1010
	CMMerchantDlgSelect  = 1011
	CMMerchantQuerySellPrice = 1012
	CMUserSellItem       = 1013
	CMUserBuyItem        = 1014
	CMUserGetDetailItem  = 1015
	CMDropGold           = 1016
	CMLoginNoticeOK      = 1018
	CMGroupMode          = 1019
	CMCreateGroup        = 1020
	CMAddGroupMember     = 1021
	CMDelGroupMember     = 1022
	CMUserRepairItem     = 1023
	CMMerchantQueryRepairCost = 1024
	CMDealTry            = 1025
	CMDealAddItem        = 1026
	CMDealDelItem        = 1027
	CMDealCancel         = 1028
	CMDealChgGold        = 1029
	CMDealEnd            = 1030
	CMUserStorageItem    = 1031
	CMUserTakeBackStorageItem = 1032
	CMWantMinimap        = 1033
	CMUserMakeDrugItem   = 1034
	CMOpenGuildDlg       = 1035
	CMGuildHome          = 1036
	CMGuildMemberList    = 1037
	CMGuildAddMember     = 1038
	CMGuildDelMember     = 1039
	CMGuildUpdateNotice  = 1040
	CMGuildUpdateRankInfo = 1041
	CMAdjustBonus        = 1043
	CMGuildAlly          = 1044
	CMGuildBreakAlly     = 1045
)

// Login/Account
const (
	CMProtocol       = 2000
	CMIDPassword     = 2001
	CMAddNewUser     = 2002
	CMChangePassword = 2003
	CMUpdateUser     = 2004
)

// Combat/Movement
const (
	CMThrow    = 3005
	CMTurn     = 3010
	CMWalk     = 3011
	CMSitdown  = 3012
	CMRun      = 3013
	CMHit      = 3014
	CMHeavyHit = 3015
	CMBigHit   = 3016
	CMSpell    = 3017
	CMPowerHit = 3018
	CMLongHit  = 3019
	CMWideHit  = 3024
	CMFireHit  = 3025
	CMSay      = 3030
	CMHorseRun = 3035
	CMCrsHit   = 3036
	CMTwinHit  = 3038
)

// ============================================================================
// SM_* - Server to Client Message IDs
// ============================================================================

// Movement/Animation (0-34)
const (
	SMThrow     = 5
	SMRush      = 6
	SMRushKung  = 7
	SMFireHit   = 8
	SMBackStep  = 9
	SMTurn      = 10
	SMWalk      = 11
	SMSitdown   = 12
	SMRun       = 13
	SMHit       = 14
	SMHeavyHit  = 15
	SMBigHit    = 16
	SMSpell     = 17
	SMPowerHit  = 18
	SMLongHit   = 19
	SMDigUp     = 20
	SMDigDown   = 21
	SMFlyAxe    = 22
	SMLighting  = 23
	SMWideHit   = 24
	SMCrsHit    = 25
	SMTwinHit   = 26
	SMAlive     = 27
	SMMoveFail  = 28
	SMHide      = 29
	SMDisappear = 30
	SMStruck    = 31
	SMDeath     = 32
	SMSkeleton  = 33
	SMNowDeath  = 34
)

// State/Info (40-54)
const (
	SMHear             = 40
	SMFeatureChanged   = 41
	SMUsername         = 42
	SMWinExp           = 44
	SMLevelUp          = 45
	SMDayChanging      = 46
	SMLogon            = 50
	SMNewMap           = 51
	SMAbility          = 52
	SMHealthSpellChanged = 53
	SMMapDescription   = 54
	SMSpell2           = 117
)

// System messages (100-104)
const (
	SMSysMessage   = 100
	SMGroupMessage = 101
	SMCry          = 102
	SMWhisper      = 103
	SMGuildMessage = 104
)

// Items (200-212)
const (
	SMAddItem     = 200
	SMBagItems    = 201
	SMDelItem     = 202
	SMUpdateItem  = 203
	SMAddMagic    = 210
	SMSendMyMagic = 211
	SMDelMagic    = 212
)

// Login flow (500-533)
const (
	SMCertificationSuccess = 500
	SMCertificationFail    = 501
	SMIDNotFound           = 502
	SMPasswdFail           = 503
	SMNewIDSuccess         = 504
	SMNewIDFail            = 505
	SMChgPasswdSuccess     = 506
	SMChgPasswdFail        = 507
	SMQueryChr             = 520
	SMNewChrSuccess        = 521
	SMNewChrFail           = 522
	SMDelChrSuccess        = 523
	SMDelChrFail           = 524
	SMStartPlay            = 525
	SMStartFail            = 526
	SMQueryChrFail         = 527
	SMOutOfConnection      = 528
	SMPassOKSelectServer   = 529
	SMSelectServerOK       = 530
	SMNeedUpdateAccount    = 531
	SMUpdateIDSuccess      = 532
	SMUpdateIDFail         = 533
)

// Gameplay (600-772)
const (
	SMDropItemSuccess = 600
	SMDropItemFail    = 601

	SMItemShow = 610
	SMItemHide = 611

	SMOpenDoorOK   = 612
	SMOpenDoorLock = 613
	SMCloseDoor    = 614

	SMTakeOnOK      = 615
	SMTakeOnFail    = 616
	SMTakeOffOK     = 619
	SMTakeOffFail   = 620
	SMSendUseItems  = 621
	SMWeightChanged = 622

	SMClearObjects = 633
	SMChangeMap    = 634
	SMEatOK        = 635
	SMEatFail      = 636
	SMButch        = 637
	SMMagicFire    = 638
	SMMagicFireFail = 639
	SMMagicLvExp   = 640
	SMDuraChange   = 642
	SMMerchantSay  = 643
	SMMerchantDlgClose = 644
	SMSendGoodsList    = 645
	SMSendUserSell     = 646
	SMSendBuyPrice     = 647
	SMUserSellItemOK   = 648
	SMUserSellItemFail = 649
	SMBuyItemSuccess   = 650
	SMBuyItemFail      = 651
	SMSendDetailGoodsList = 652
	SMGoldChanged      = 653
	SMChangeLight      = 654
	SMLampChangeDura   = 655
	SMChangeNameColor  = 656
	SMCharStatusChanged = 657
	SMSendNotice       = 658

	// Group operations (659-667)
	SMGroupModeChanged = 659
	SMCreateGroupOK    = 660
	SMCreateGroupFail  = 661
	SMGroupAddMemOK    = 662
	SMGroupDelMemOK    = 663
	SMGroupAddMemFail  = 664
	SMGroupDelMemFail  = 665
	SMGroupCancel      = 666
	SMGroupMembers     = 667

	// Repair operations (668-671)
	SMSendUserRepair     = 668
	SMUserRepairItemOK   = 669
	SMUserRepairItemFail = 670
	SMSendRepairCost     = 671

	// Deal/Trade operations (673-687)
	SMDealMenu          = 673
	SMDealTryFail       = 674
	SMDealAddItemOK     = 675
	SMDealAddItemFail   = 676
	SMDealDelItemOK     = 677
	SMDealDelItemFail   = 678
	SMDealCancel        = 681
	SMDealRemoteAddItem = 682
	SMDealRemoteDelItem = 683
	SMDealChgGoldOK     = 684
	SMDealChgGoldFail   = 685
	SMDealRemoteChgGold = 686
	SMDealSuccess       = 687

	// Storage operations (700-707)
	SMSendUserStorageItem      = 700
	SMStorageOK                = 701
	SMStorageFull              = 702
	SMStorageFail              = 703
	SMSaveItemList             = 704
	SMTakeBackStorageItemOK    = 705
	SMTakeBackStorageItemFail  = 706
	SMTakeBackStorageItemFullBag = 707

	SMAreaState = 766
	SMMyStatus  = 708

	SMDelItems           = 709
	SMReadMinimapOK      = 710
	SMReadMinimapFail    = 711
	SMSendUserMakeDrugItemList = 712
	SMMakeDrugSuccess    = 713
	SMMakeDrugFail       = 714

	// Guild operations (750-772)
	SMChangeGuildName     = 750
	SMSendUserState       = 751
	SMSubAbility          = 752
	SMOpenGuildDlg        = 753
	SMOpenGuildDlgFail    = 754
	SMSendGuildMemberList = 756
	SMGuildAddMemberOK    = 757
	SMGuildAddMemberFail  = 758
	SMGuildDelMemberOK    = 759
	SMGuildDelMemberFail  = 760
	SMGuildRankUpdateFail = 761
	SMBuildGuildOK        = 762
	SMBuildGuildFail      = 763
	SMDonateOK            = 764
	SMDonateFail          = 765
	SMMenuOK              = 767
	SMGuildMakeAllyOK     = 768
	SMGuildMakeAllyFail   = 769
	SMGuildBreakAllyOK    = 770
	SMGuildBreakAllyFail  = 771
	SMDlgMsg              = 772
)

// Teleport/Events (800-811)
const (
	SMSpaceMoveHide  = 800
	SMSpaceMoveShow  = 801
	SMReconnect      = 802
	SMGhost          = 803
	SMShowEvent      = 804
	SMHideEvent      = 805
	SMSpaceMoveHide2 = 806
	SMSpaceMoveShow2 = 807
	SMTimeCheckMsg   = 810
	SMAdjustBonus    = 811
)

// Health/Status (1100+)
const (
	SMOpenHealth     = 1100
	SMCloseHealth    = 1101
	SMBreakWeapon    = 1102
	SMChangeFace     = 1104
	SMVersionFail    = 1106
)

// Item/Monster updates (1500+)
const (
	SMItemUpdate  = 1500
	SMMonsterSay  = 1501
)

// ============================================================================
// SS_* - Server to Server (Inter-server) Message IDs
// ============================================================================
const (
	SSOpenSession    = 100
	SSCloseSession   = 101
	SSSoftOutSession = 102
	SSServerInfo     = 103
	SSKeepAlive      = 104
	SSKickUser       = 111
	SSServerLoad     = 113
)

// ============================================================================
// DB_* - Database Message IDs
// ============================================================================
const (
	DBRFail         = 2000
	DBLoadHumanRcd  = 100
	DBSaveHumanRcd  = 101
	DBSaveHumanRcdEx = 102
	DBRLoadHumanRcd = 1100
	DBRSaveHumanRcd = 1102
)

// ============================================================================
// Control message prefixes (not 6Bit encoded)
// ============================================================================
const (
	CtrlGood  = "+GOOD"  // Action confirmation
	CtrlFail  = "+FAIL"  // Action failure
	CtrlPwr   = "+PWR"   // Enable PowerHit
	CtrlLng   = "+LNG"   // Enable LongHit
	CtrlULng  = "+ULNG"  // Disable LongHit
	CtrlWid   = "+WID"   // Enable WideHit
	CtrlUWid  = "+UWID"  // Disable WideHit
	CtrlCrs   = "+CRS"   // Enable CrsHit
	CtrlUCrs  = "+UCRS"  // Disable CrsHit
	CtrlTwn   = "+TWN"   // Enable TwnHit
	CtrlUTwn  = "+UTWN"  // Disable TwnHit
	CtrlFir   = "+FIR"   // Enable FireHit
	CtrlUFir  = "+UFIR"  // Disable FireHit
	CtrlDig   = "=DIG"   // Set dig flag
)
