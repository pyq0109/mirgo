package protocol

import "fmt"

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

// MsgName returns a human-readable name for a message ID.
func MsgName(ident uint16) string {
	switch ident {
	// CM - Character management
	case CMQueryUsername:
		return "CM_QUERYUSERNAME"
	case CMQueryBagItems:
		return "CM_QUERYBAGITEMS"
	case CMQueryUserState:
		return "CM_QUERYUSERSTATE"
	case CMQueryChr:
		return "CM_QUERYCHR"
	case CMNewChr:
		return "CM_NEWCHR"
	case CMDelChr:
		return "CM_DELCHR"
	case CMSelChr:
		return "CM_SELCHR"
	case CMSelectServer:
		return "CM_SELECTSERVER"
	// CM - Login/Account
	case CMProtocol:
		return "CM_PROTOCOL"
	case CMIDPassword:
		return "CM_IDPASSWORD"
	case CMAddNewUser:
		return "CM_ADDNEWUSER"
	case CMChangePassword:
		return "CM_CHANGEPASSWORD"
	case CMUpdateUser:
		return "CM_UPDATEUSER"
	// CM - Game operations
	case CMDropItem:
		return "CM_DROPITEM"
	case CMPickup:
		return "CM_PICKUP"
	case CMOpenDoor:
		return "CM_OPENDOOR"
	case CMTakeOnItem:
		return "CM_TAKEONITEM"
	case CMTakeOffItem:
		return "CM_TAKEOFFITEM"
	case CMEat:
		return "CM_EAT"
	case CMButch:
		return "CM_BUTCH"
	case CMClickNPC:
		return "CM_CLICKNPC"
	case CMLoginNoticeOK:
		return "CM_LOGINNOTICEOK"
	// CM - Combat/Movement
	case CMThrow:
		return "CM_THROW"
	case CMTurn:
		return "CM_TURN"
	case CMWalk:
		return "CM_WALK"
	case CMSitdown:
		return "CM_SITDOWN"
	case CMRun:
		return "CM_RUN"
	case CMHit:
		return "CM_HIT"
	case CMHeavyHit:
		return "CM_HEAVYHIT"
	case CMBigHit:
		return "CM_BIGHIT"
	case CMSpell:
		return "CM_SPELL"
	case CMPowerHit:
		return "CM_POWERHIT"
	case CMLongHit:
		return "CM_LONGHIT"
	case CMWideHit:
		return "CM_WIDEHIT"
	case CMFireHit:
		return "CM_FIREHIT"
	case CMSay:
		return "CM_SAY"
	// SM - Movement/Animation
	case SMThrow:
		return "SM_THROW"
	case SMRush:
		return "SM_RUSH"
	case SMRushKung:
		return "SM_RUSHKUNG"
	case SMFireHit:
		return "SM_FIREHIT"
	case SMBackStep:
		return "SM_BACKSTEP"
	case SMTurn:
		return "SM_TURN"
	case SMWalk:
		return "SM_WALK"
	case SMSitdown:
		return "SM_SITDOWN"
	case SMRun:
		return "SM_RUN"
	case SMHit:
		return "SM_HIT"
	case SMHeavyHit:
		return "SM_HEAVYHIT"
	case SMBigHit:
		return "SM_BIGHIT"
	case SMSpell:
		return "SM_SPELL"
	case SMPowerHit:
		return "SM_POWERHIT"
	case SMLongHit:
		return "SM_LONGHIT"
	case SMDigUp:
		return "SM_DIGUP"
	case SMDigDown:
		return "SM_DIGDOWN"
	case SMFlyAxe:
		return "SM_FLYAXE"
	case SMLighting:
		return "SM_LIGHTING"
	case SMWideHit:
		return "SM_WIDEHIT"
	case SMCrsHit:
		return "SM_CRSHIT"
	case SMTwinHit:
		return "SM_TWINHIT"
	case SMAlive:
		return "SM_ALIVE"
	case SMMoveFail:
		return "SM_MOVEFAIL"
	case SMHide:
		return "SM_HIDE"
	case SMDisappear:
		return "SM_DISAPPEAR"
	case SMStruck:
		return "SM_STRUCK"
	case SMDeath:
		return "SM_DEATH"
	case SMSkeleton:
		return "SM_SKELETON"
	case SMNowDeath:
		return "SM_NOWDEATH"
	// SM - State/Info
	case SMHear:
		return "SM_HEAR"
	case SMFeatureChanged:
		return "SM_FEATURECHANGED"
	case SMUsername:
		return "SM_USERNAME"
	case SMWinExp:
		return "SM_WINEXP"
	case SMLevelUp:
		return "SM_LEVELUP"
	case SMDayChanging:
		return "SM_DAYCHANGING"
	case SMLogon:
		return "SM_LOGON"
	case SMNewMap:
		return "SM_NEWMAP"
	case SMAbility:
		return "SM_ABILITY"
	case SMHealthSpellChanged:
		return "SM_HEALTHSPELLCHANGED"
	case SMMapDescription:
		return "SM_MAPDESCRIPTION"
	case SMSpell2:
		return "SM_SPELL2"
	// SM - System messages (100-104) share values with CM 100-104,
	// so they are handled by the CM cases above in a uint16 switch.
	// SM - Items
	case SMAddItem:
		return "SM_ADDITEM"
	case SMBagItems:
		return "SM_BAGITEMS"
	case SMDelItem:
		return "SM_DELITEM"
	case SMUpdateItem:
		return "SM_UPDATEITEM"
	case SMAddMagic:
		return "SM_ADDMAGIC"
	case SMSendMyMagic:
		return "SM_SENDMYMAGIC"
	case SMDelMagic:
		return "SM_DELMAGIC"
	// SM - Login flow
	case SMCertificationSuccess:
		return "SM_CERTIFICATIONSUCCESS"
	case SMCertificationFail:
		return "SM_CERTIFICATIONFAIL"
	case SMIDNotFound:
		return "SM_IDNOTFOUND"
	case SMPasswdFail:
		return "SM_PASSWDFAIL"
	case SMNewIDSuccess:
		return "SM_NEWIDSUCCESS"
	case SMNewIDFail:
		return "SM_NEWIDFAIL"
	case SMChgPasswdSuccess:
		return "SM_CHGPASSWDSUCCESS"
	case SMChgPasswdFail:
		return "SM_CHGPASSWDFAIL"
	case SMQueryChr:
		return "SM_QUERYCHR"
	case SMNewChrSuccess:
		return "SM_NEWCHRSUCCESS"
	case SMNewChrFail:
		return "SM_NEWCHRFAIL"
	case SMDelChrSuccess:
		return "SM_DELCHRSUCCESS"
	case SMDelChrFail:
		return "SM_DELCHRFAIL"
	case SMStartPlay:
		return "SM_STARTPLAY"
	case SMStartFail:
		return "SM_STARTFAIL"
	case SMQueryChrFail:
		return "SM_QUERYCHRFAIL"
	case SMOutOfConnection:
		return "SM_OUTOFCONNECTION"
	case SMPassOKSelectServer:
		return "SM_PASSOKSELECTSERVER"
	case SMSelectServerOK:
		return "SM_SELECTSERVEROK"
	case SMNeedUpdateAccount:
		return "SM_NEEDUPDATEACCOUNT"
	case SMUpdateIDSuccess:
		return "SM_UPDATEIDSUCCESS"
	case SMUpdateIDFail:
		return "SM_UPDATEIDFAIL"
	// SM - Gameplay
	case SMDropItemSuccess:
		return "SM_DROPITEMSUCCESS"
	case SMDropItemFail:
		return "SM_DROPITEMFAIL"
	case SMItemShow:
		return "SM_ITEMSHOW"
	case SMItemHide:
		return "SM_ITEMHIDE"
	case SMOpenDoorOK:
		return "SM_OPENDOOROK"
	case SMOpenDoorLock:
		return "SM_OPENDOORLOCK"
	case SMCloseDoor:
		return "SM_CLOSEDOOR"
	case SMTakeOnOK:
		return "SM_TAKEONOK"
	case SMTakeOnFail:
		return "SM_TAKEONFAIL"
	case SMTakeOffOK:
		return "SM_TAKEOFFOK"
	case SMTakeOffFail:
		return "SM_TAKEOFFFAIL"
	case SMSendUseItems:
		return "SM_SENDUSEITEMS"
	case SMWeightChanged:
		return "SM_WEIGHTCHANGED"
	case SMClearObjects:
		return "SM_CLEAROBJECTS"
	case SMChangeMap:
		return "SM_CHANGEMAP"
	case SMEatOK:
		return "SM_EATOK"
	case SMEatFail:
		return "SM_EATFAIL"
	case SMButch:
		return "SM_BUTCH"
	case SMMagicFire:
		return "SM_MAGICFIRE"
	case SMMagicFireFail:
		return "SM_MAGICFIREFAIL"
	case SMMagicLvExp:
		return "SM_MAGICLVEXP"
	case SMDuraChange:
		return "SM_DURACHANGE"
	case SMMerchantSay:
		return "SM_MERCHANTSAY"
	case SMMerchantDlgClose:
		return "SM_MERCHANTDLGCLOSE"
	case SMSendGoodsList:
		return "SM_SENDGOODSLIST"
	case SMSendUserSell:
		return "SM_SENDUSERSELL"
	case SMSendBuyPrice:
		return "SM_SENDBUYPRICE"
	case SMUserSellItemOK:
		return "SM_USERSELLITEMOK"
	case SMUserSellItemFail:
		return "SM_USERSELLITEMFAIL"
	case SMBuyItemSuccess:
		return "SM_BUYITEMSUCCESS"
	case SMBuyItemFail:
		return "SM_BUYITEMFAIL"
	case SMSendDetailGoodsList:
		return "SM_SENDDETAILGOODSLIST"
	case SMGoldChanged:
		return "SM_GOLDCHANGED"
	case SMChangeLight:
		return "SM_CHANGELIGHT"
	case SMLampChangeDura:
		return "SM_LAMPCHANGEDURA"
	case SMChangeNameColor:
		return "SM_CHANGENAMECOLOR"
	case SMCharStatusChanged:
		return "SM_CHARSTATUSCHANGED"
	case SMSendNotice:
		return "SM_SENDNOTICE"
	// SM - Group
	case SMGroupModeChanged:
		return "SM_GROUPMODECHANGED"
	case SMCreateGroupOK:
		return "SM_CREATEGROUPOK"
	case SMCreateGroupFail:
		return "SM_CREATEGROUPFAIL"
	case SMGroupAddMemOK:
		return "SM_GROUPADDMEMOK"
	case SMGroupDelMemOK:
		return "SM_GROUPDELMEMOK"
	case SMGroupAddMemFail:
		return "SM_GROUPADDMEMFAIL"
	case SMGroupDelMemFail:
		return "SM_GROUPDELMEMFAIL"
	case SMGroupCancel:
		return "SM_GROUPCANCEL"
	case SMGroupMembers:
		return "SM_GROUPMEMBERS"
	// SM - Repair
	case SMSendUserRepair:
		return "SM_SENDUSERREPAIR"
	case SMUserRepairItemOK:
		return "SM_USERREPAIRITEMOK"
	case SMUserRepairItemFail:
		return "SM_USERREPAIRITEMFAIL"
	case SMSendRepairCost:
		return "SM_SENDREPAIRCOST"
	// SM - Deal/Trade
	case SMDealMenu:
		return "SM_DEALMENU"
	case SMDealTryFail:
		return "SM_DEALTRYFAIL"
	case SMDealAddItemOK:
		return "SM_DEALADDITEMOK"
	case SMDealAddItemFail:
		return "SM_DEALADDITEMFAIL"
	case SMDealDelItemOK:
		return "SM_DEALDELITEMOK"
	case SMDealDelItemFail:
		return "SM_DEALDELITEMFAIL"
	case SMDealCancel:
		return "SM_DEALCANCEL"
	case SMDealRemoteAddItem:
		return "SM_DEALREMOTEADDITEM"
	case SMDealRemoteDelItem:
		return "SM_DEALREMOTEDELITEM"
	case SMDealChgGoldOK:
		return "SM_DEALCHGGOLDOK"
	case SMDealChgGoldFail:
		return "SM_DEALCHGGOLDFAIL"
	case SMDealRemoteChgGold:
		return "SM_DEALREMOTECHGGOLD"
	case SMDealSuccess:
		return "SM_DEALSUCCESS"
	// SM - Storage
	case SMSendUserStorageItem:
		return "SM_SENDUSERSTORAGEITEM"
	case SMStorageOK:
		return "SM_STORAGEOK"
	case SMStorageFull:
		return "SM_STORAGEFULL"
	case SMStorageFail:
		return "SM_STORAGEFAIL"
	case SMSaveItemList:
		return "SM_SAVEITEMLIST"
	case SMTakeBackStorageItemOK:
		return "SM_TAKEBACKSTORAGEITEMOK"
	case SMTakeBackStorageItemFail:
		return "SM_TAKEBACKSTORAGEITEMFAIL"
	case SMTakeBackStorageItemFullBag:
		return "SM_TAKEBACKSTORAGEITEMFULLBAG"
	// SM - Misc gameplay
	case SMAreaState:
		return "SM_AREASTATE"
	case SMMyStatus:
		return "SM_MYSTATUS"
	case SMDelItems:
		return "SM_DELITEMS"
	case SMReadMinimapOK:
		return "SM_READMINIMAPOK"
	case SMReadMinimapFail:
		return "SM_READMINIMAPFAIL"
	case SMSendUserMakeDrugItemList:
		return "SM_SENDUSERMAKEDRUGITEMLIST"
	case SMMakeDrugSuccess:
		return "SM_MAKEDRUGSUCCESS"
	case SMMakeDrugFail:
		return "SM_MAKEDRUGFAIL"
	// SM - Guild
	case SMChangeGuildName:
		return "SM_CHANGEGUILDNAME"
	case SMSendUserState:
		return "SM_SENDUSERSTATE"
	case SMSubAbility:
		return "SM_SUBABILITY"
	case SMOpenGuildDlg:
		return "SM_OPENGUILDDLG"
	case SMOpenGuildDlgFail:
		return "SM_OPENGUILDDLGFAIL"
	case SMSendGuildMemberList:
		return "SM_SENDGUILDMEMBERLIST"
	case SMGuildAddMemberOK:
		return "SM_GUILDADDMEBEROK"
	case SMGuildAddMemberFail:
		return "SM_GUILDADDMEBERFAIL"
	case SMGuildDelMemberOK:
		return "SM_GUILDDELMEBEROK"
	case SMGuildDelMemberFail:
		return "SM_GUILDDELMEBERFAIL"
	case SMGuildRankUpdateFail:
		return "SM_GUILDRANKUPDATEFAIL"
	case SMBuildGuildOK:
		return "SM_BUILDGUILDOK"
	case SMBuildGuildFail:
		return "SM_BUILDGUILDFAIL"
	case SMDonateOK:
		return "SM_DONATEOK"
	case SMDonateFail:
		return "SM_DONATEFAIL"
	case SMMenuOK:
		return "SM_MENUOK"
	case SMGuildMakeAllyOK:
		return "SM_GUILDMAKEALLYOK"
	case SMGuildMakeAllyFail:
		return "SM_GUILDMAKEALLYFAIL"
	case SMGuildBreakAllyOK:
		return "SM_GUILDBREAKALLYOK"
	case SMGuildBreakAllyFail:
		return "SM_GUILDBREAKALLYFAIL"
	case SMDlgMsg:
		return "SM_DLGMSG"
	// SM - Teleport/Events
	case SMSpaceMoveHide:
		return "SM_SPACEMOVEHIDE"
	case SMSpaceMoveShow:
		return "SM_SPACEMOVESHOW"
	case SMReconnect:
		return "SM_RECONNECT"
	case SMGhost:
		return "SM_GHOST"
	case SMShowEvent:
		return "SM_SHOWEVENT"
	case SMHideEvent:
		return "SM_HIDEEVENT"
	case SMSpaceMoveHide2:
		return "SM_SPACEMOVEHIDE2"
	case SMSpaceMoveShow2:
		return "SM_SPACEMOVESHOW2"
	case SMTimeCheckMsg:
		return "SM_TIMECHECKMSG"
	case SMAdjustBonus:
		return "SM_ADJUSTBONUS"
	// SM - Health/Status
	case SMOpenHealth:
		return "SM_OPENHEALTH"
	case SMCloseHealth:
		return "SM_CLOSEHEALTH"
	case SMBreakWeapon:
		return "SM_BREAKWEAPON"
	case SMChangeFace:
		return "SM_CHANGEFACE"
	case SMVersionFail:
		return "SM_VERSIONFAIL"
	// SM - Item/Monster updates
	case SMItemUpdate:
		return "SM_ITEMUPDATE"
	case SMMonsterSay:
		return "SM_MONSTERSAY"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", ident)
	}
}

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
