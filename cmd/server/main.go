package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

type loginAttempt struct {
	count    int
	lockTime time.Time
}

var loginAttempts = struct {
	sync.Mutex
	m map[string]*loginAttempt
}{m: make(map[string]*loginAttempt)}

const defaultNoticeText = "Welcome to MIR2 Go Server!"

// loadNoticeText 加载登录公告（Delphi TNoticeManager，NoticeM.pas:52-72）。
// 文件缺失或为空时回退默认文案。
func loadNoticeText(configDir string) string {
	path := filepath.Join(configDir, "notice", "notice.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "notice file not found: %s, using default", path)
		return defaultNoticeText
	}
	var raw struct {
		Content string `json:"content"`
	}
	if err := parseJSONC(data, &raw); err != nil || strings.TrimSpace(raw.Content) == "" {
		log.Logf(log.LevelWarn, "Server", "failed to parse %s or empty content, using default", path)
		return defaultNoticeText
	}
	log.Logf(log.LevelInfo, "Server", "loaded login notice from %s", path)
	return raw.Content
}

func main() {
	configDir := flag.String("config", "serverconfig", "Path to serverconfig directory")
	mapDir := flag.String("maps", "asset/server/Map", "Path to map directory")
	flag.Parse()

	// 从 serverconfig 目录加载配置
	config, err := LoadConfig(*configDir)
	if err != nil {
		log.Logf(log.LevelError, "Server", "failed to load config: %v", err)
		os.Exit(1)
	}

	listenAddr := config.GetListenAddr()
	dbPath := config.GetDatabasePath()

	log.Logf(log.LevelInfo, "Server", "starting MIR2 server...")
	log.Logf(log.LevelInfo, "Server", "listen address: %s", listenAddr)
	log.Logf(log.LevelInfo, "Server", "database: %s", dbPath)
	log.Logf(log.LevelInfo, "Server", "home map: %s(%d,%d)", config.GetHomeMap(), config.GetHomeX(), config.GetHomeY())

	// 打开单个数据库文件
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Logf(log.LevelError, "Server", "failed to open database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Logf(log.LevelInfo, "Server", "database opened")

	mapMgr := NewMapManager(*mapDir)
	if err := mapMgr.LoadAllMaps(); err != nil {
		log.Logf(log.LevelError, "Server", "failed to load maps: %v", err)
		os.Exit(1)
	}
	mapMgr.InitRoutes(*configDir)
	mapMgr.InitMiniMaps(*configDir)
	mapMgr.InitMapFlags(*configDir)

	var itemDB *ItemDB
	itemDBPath := filepath.Join(*configDir, "items", "std_items.jsonc")
	itemDB, err = LoadItemDB(itemDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "failed to load ItemDB: %v (item system disabled)", err)
		itemDB = nil
	} else {
		itemDB.LoadUnbindList(filepath.Join(*configDir, "items", "unbind_list.jsonc"))
		itemDB.LoadDisableTakeOffList(filepath.Join(*configDir, "items", "disable_takeoff_list.jsonc"))
	}

	var magicDB *MagicDB
	magicDBPath := filepath.Join(*configDir, "magic", "magic_db.jsonc")
	magicDB, err = LoadMagicDB(magicDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "failed to load MagicDB: %v (magic system disabled)", err)
		magicDB = nil
	}

	var monsterDB *MonsterDB
	monsterDBPath := filepath.Join(*configDir, "monsters", "monster_db.jsonc")
	monsterDB, err = LoadMonsterDB(monsterDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "failed to load MonsterDB: %v (using defaults)", err)
		monsterDB = &MonsterDB{byName: make(map[string]*MonsterDef)}
	}

	dropTablesDir := filepath.Join(*configDir, "monsters", "mon_items")
	dropTables := LoadDropTables(dropTablesDir)

	safeZonesPath := filepath.Join(*configDir, "maps", "start_points.jsonc")
	LoadSafeZones(safeZonesPath)

	noticeText := loadNoticeText(*configDir)

	monGenPath := filepath.Join(*configDir, "monsters", "mon_gen.jsonc")
	npcConfigDir := filepath.Join(*configDir, "npcs")

	sessionMgr := NewSessionManager()
	userEngine := NewUserEngine(db, mapMgr)
	userEngine.Config = config
	userEngine.ItemDB = itemDB
	userEngine.MagicDB = magicDB
	userEngine.MonsterDB = monsterDB
	userEngine.DropTables = dropTables
	userEngine.monGenPath = monGenPath
	userEngine.npcConfigDir = npcConfigDir
	userEngine.InitWorld(mapMgr)
	userEngine.LoadGuilds()

	castleCfg, err := LoadCastleConfig(*configDir)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "failed to load castle config: %v (using defaults)", err)
		castleCfg = DefaultCastleConfig()
	}
	userEngine.Castle = NewCastleObject(*castleCfg)
	userEngine.Castle.LoadState(db)
	if castleEnv := mapMgr.FindMap(castleCfg.MapName); castleEnv != nil {
		castleEnv.Castle = userEngine.Castle
	}
	log.Logf(log.LevelInfo, "Server", "castle: %s (map=%s, owner=%s)", castleCfg.Name, castleCfg.MapName, userEngine.Castle.GetOwnerGuild())

	LoadDrugRecipes(*configDir)

	loadDenyChrNameList(*configDir)
	loadBlockLists(*configDir)
	loadWordFilter(*configDir)

	server := netserver.NewTCPServer(listenAddr)
	// 每连接入站消息限流（路线图 6.3 网关补偿层，默认 60 条/秒突发 40）
	server.SetMsgRateLimit(config.GetMsgRateLimit(), config.GetMsgBurst())

	server.SetConnectHandler(func(session *netserver.Session) {
		// IP 黑名单（Delphi BlockIPList 语义）：连接期直接拒绝
		if ip := sessionIP(session.Conn); isIPBlocked(ip) {
			log.Logf(log.LevelWarn, "Server", "blocked IP rejected: %s (session %d)", ip, session.ID)
			session.Conn.Close()
			return
		}
		sessionMgr.Add(session)
		log.Logf(log.LevelInfo, "Server", "session %d connected (total: %d)", session.ID, sessionMgr.Count())
	})

	server.SetDisconnectHandler(func(session *netserver.Session) {
		if session.State == netserver.StateInGame {
			player := userEngine.GetPlayer(int32(session.CharacterID))
			if player != nil {
				saveCharacterData(db, player)
				player.NotifyFriendsOffline(server)
				player.Ghost = true
				player.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
				if player.envir != nil {
					player.envir.RemoveObject(player.CurrX, player.CurrY, OS_MOVINGOBJECT, player)
				}
				userEngine.RemovePlayer(int32(session.CharacterID))
				log.Logf(log.LevelInfo, "Server", "player %s saved and removed from world", player.Name)
			}
		}
		sessionMgr.Remove(session.ID)
		clearSessionThrottles(session.ID)
		log.Logf(log.LevelInfo, "Server", "session %d disconnected (total: %d)", session.ID, sessionMgr.Count())
	})

	// Fix 5: 处理 **runlogin 原始消息
	server.SetRawMessageHandler(func(session *netserver.Session, raw string) bool {
		if strings.HasPrefix(raw, "+PWR/") || strings.HasPrefix(raw, "+LNG/") || strings.HasPrefix(raw, "+WID/") {
			if session.State != netserver.StateInGame {
				return true
			}
			player := userEngine.GetPlayer(int32(session.CharacterID))
			if player == nil {
				return true
			}
			parts := strings.SplitN(raw, "/", 2)
			if len(parts) != 2 {
				return true
			}
			var val int
			fmt.Sscanf(parts[1], "%d", &val)
			switch parts[0] {
			case "+PWR":
				player.SkillPower = val
			case "+LNG":
				player.SkillLng = val
			case "+WID":
				player.SkillWid = val
			}
			log.Logf(log.LevelDebug, "Server", "%s skill param %s=%d", player.Name, parts[0], val)
			return true
		}

		// 格式: **loginID/charName/cert/version/code
		if !strings.HasPrefix(raw, "**") {
			return false
		}
		loginInfo := raw[2:] // 去掉 **
		parts := strings.Split(loginInfo, "/")
		if len(parts) < 5 {
			log.Logf(log.LevelWarn, "Server", "invalid run login format: %s", raw)
			return false
		}
		loginID := parts[0]
		charName := parts[1]
		var cert int32
		fmt.Sscanf(parts[2], "%d", &cert)

		log.Logf(log.LevelInfo, "Server", "[**RunLogin] user=%s character=%s cert=%d version=%s code=%s",
			loginID, charName, cert, parts[3], parts[4])
		log.Logf(log.LevelInfo, "Server", "[**RunLogin] session=%d session cert=%d account ID=%d",
			session.ID, session.Certification, session.CharacterID)

		// 验证证书与会话匹配
		if session.Certification != 0 && session.Certification != cert {
			log.Logf(log.LevelWarn, "Server", "[**RunLogin] cert mismatch: expected %d, got %d — disconnecting",
				session.Certification, cert)
			server.CloseSession(session.ID)
			return true
		}

		// 加载角色并进入游戏
		log.Logf(log.LevelInfo, "Server", "[**RunLoading] loading character %q, account %d...", charName, session.CharacterID)
		charData, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[**RunLogin] character %q not found, account %d: %v",
				charName, session.CharacterID, err)
			return true
		}
		log.Logf(log.LevelInfo, "Server", "[**RunLogin] character loaded: %s id=%d map=%s(%d,%d)",
			charData.Name, charData.ID, charData.Map, charData.X, charData.Y)

		// 创建 PlayObject
		player := NewPlayObject(session, charData.Name, int32(charData.ID))
		player.MapMgr = mapMgr
		player.ItemDB = itemDB
		player.MagicDB = magicDB
		player.Engine = userEngine
		player.WalkSpeed = config.GetDefaultWalkSpeed()
		player.RunSpeed = config.GetDefaultWalkSpeed()
		player.visionInterval = config.GetVisionInterval() + rand.Int63n(config.GetVisionRand())
		player.ViewRange = config.GetViewRange()
		player.MapName = charData.Map
		player.CurrX = charData.X
		player.CurrY = charData.Y
		player.Job = byte(charData.Job)
		player.Gender = byte(charData.Sex)
		player.Hair = byte(charData.Hair)
		player.WAbil.Level = uint16(charData.Level)
		player.WAbil.HP = uint16(charData.HP)
		player.WAbil.MP = uint16(charData.MP)
		player.WAbil.MaxHP = uint16(charData.HP)
		player.WAbil.MaxMP = uint16(charData.MP)
		player.WAbil.Exp = uint32(charData.Exp)
		player.Gold = int(charData.Gold)
		player.SessionID = session.ID
		player.AccountName = session.AccountName
		player.LastPkDecayTick = time.Now().UnixMilli()
		if perm, ok := config.Game.GMAccounts[player.AccountName]; ok {
			player.Permission = byte(perm)
		}

		// 查找并设置地图环境
		envir := mapMgr.FindMap(charData.Map)
		if envir == nil {
			log.Logf(log.LevelError, "Server", "map %s not found, using home map", charData.Map)
			envir = mapMgr.FindMap(config.GetHomeMap())
			if envir != nil {
				player.MapName = config.GetHomeMap()
				player.CurrX = config.GetHomeX()
				player.CurrY = config.GetHomeY()
			}
		}
		player.envir = envir

		// 将玩家添加到地图
		if envir != nil {
			envir.AddObject(player.CurrX, player.CurrY, OS_MOVINGOBJECT, player)
		}

		// 更新会话状态
		session.State = netserver.StateInGame
		session.CharacterID = charData.ID
		log.Logf(log.LevelInfo, "Server", "session %d: authenticated -> in-game (character=%s id=%d)",
			session.ID, charData.Name, charData.ID)

		// 添加到 UserEngine
		userEngine.AddPlayer(player)
		player.ReadyToRun = true

		// 加载或初始化物品
		loadPlayerItems(db, player)

		if metaJSON, err := db.LoadCharacterMeta(charData.ID); err == nil && metaJSON != nil {
			var meta struct {
				PkPoint         int             `json:"pkPoint"`
				Dir             int             `json:"dir"`
				BonusPoint      int             `json:"bonusPoint"`
				CreditPoint     int             `json:"creditPoint"`
				ReNewLevel      int             `json:"reNewLevel"`
				AttackMode      int             `json:"attackMode"`
				AllowGroup      bool            `json:"allowGroup"`
				HungerStatus    int             `json:"hungerStatus"`
				StoragePassword string          `json:"storagePassword,omitempty"`
				StatusTimeArr   [12]int16       `json:"statusTimeArr"`
				Magics          []PlayerMagic   `json:"magics"`
				Storage         []savedUserItem `json:"storage"`
				DearName        string          `json:"dearName"`
				MasterName      string          `json:"masterName"`
				ApprenticeNames []string        `json:"apprenticeNames"`
				Friends         []string        `json:"friends,omitempty"`
				QuestUnitOpen   []byte          `json:"questUnitOpen,omitempty"`
				QuestUnit       []byte          `json:"questUnit,omitempty"`
				QuestFlag       []byte          `json:"questFlag,omitempty"`
			}
			if json.Unmarshal(metaJSON, &meta) == nil {
				player.PkPoint = meta.PkPoint
				if meta.Dir >= 0 && meta.Dir <= 7 {
					player.Dir = meta.Dir
				}
				player.BonusPoint = meta.BonusPoint
				player.CreditPoint = meta.CreditPoint
				player.ReNewLevel = meta.ReNewLevel
				player.AttackMode = byte(meta.AttackMode)
				player.AllowGroup = meta.AllowGroup
				player.HungerStatus = meta.HungerStatus
				player.StoragePassword = meta.StoragePassword
				// Delphi（UsrEngn.pas:2342-2344）：设过密码的仓库登录即上锁。
				if meta.StoragePassword != "" {
					player.StoragePwdLocked = true
				}
				// Delphi wStatusTimeArr 持久化：恢复毒/隐身等残留状态
				player.StatusTimeArr = meta.StatusTimeArr
				if meta.StatusTimeArr[STATE_TRANSPARENT] > 0 {
					player.Hidden = true
				}
				for _, v := range meta.StatusTimeArr {
					if v > 0 {
						player.BroadcastStatus()
						break
					}
				}
				player.DearName = meta.DearName
				player.MasterName = meta.MasterName
				player.ApprenticeNames = meta.ApprenticeNames
				player.Friends = meta.Friends
				if len(meta.QuestUnitOpen) == 128 {
					copy(player.QuestUnitOpen[:], meta.QuestUnitOpen)
				}
				if len(meta.QuestUnit) == 128 {
					copy(player.QuestUnit[:], meta.QuestUnit)
				}
				if len(meta.QuestFlag) == 128 {
					copy(player.QuestFlag[:], meta.QuestFlag)
				}
				for i := range meta.Magics {
					player.LearnedMagics = append(player.LearnedMagics, &meta.Magics[i])
				}
				for _, it := range meta.Storage {
					player.StorageItems = append(player.StorageItems, &protocol.UserItem{
						MakeIndex: it.MakeIndex,
						WIndex:    it.WIndex,
						Dura:      it.Dura,
						DuraMax:   it.DuraMax,
						BtValue:   it.BtValue,
					})
				}
			}
		}

		// 根据装备重新计算属性
		player.RecalcAbilitys()

		// 根据职业给予初始技能
		switch player.Job {
		case 0: // 战士
			player.learnMagic(3, 0, 1) // 基本剑术
			player.learnMagic(7, 0, 2) // 攻杀剑术
		case 1: // 法师
			player.learnMagic(1, 0, 1) // 火球术
		case 2: // 道士
			player.learnMagic(2, 0, 1) // 治愈术
		}

		// 发送地图信息
		player.SendMapInfo(server)
		log.Logf(log.LevelInfo, "Server", "sent map %s(%d,%d) to player %s",
			player.MapName, player.CurrX, player.CurrY, player.Name)

		// 发送能力属性
		player.SendAbility(server)

		// 物品定义数据库（客户端需要 Name/Looks/StdMode 用于图标、
		// 提示、拖拽规则）——每次登录发送一次。
		player.SendStdItems(server)

		// 发送背包和装备
		player.SendBagItemsFull(server)
		player.SendUseItemsFull(server)

		// 发送公告（SMLogon 将在 CMLoginNoticeOK 之后发送）
		noticeResp := protocol.MakeDefaultMsg(protocol.SMSendNotice, 0, 0, 0, 0)
		server.Send(session.ID, noticeResp, protocol.EncodeString(noticeText))

		player.NotifyFriendsOnline(server)

		log.Logf(log.LevelInfo, "Server", "player %s entered game at %s(%d,%d)",
			player.Name, player.MapName, player.CurrX, player.CurrY)

		return true
	})

	server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body, rawBody string) {
		stateNames := map[netserver.SessionState]string{
			netserver.StateConnected:     "connected",
			netserver.StateAuthenticated: "authenticated",
			netserver.StateInGame:        "in-game",
		}
		log.Logf(log.LevelInfo, "Server", "session %d state=%s: dispatching %s",
			session.ID, stateNames[session.State], protocol.MsgName(msg.Ident))

		switch session.State {
		case netserver.StateConnected:
			handleConnectedMessage(server, session, msg, body, rawBody, config, db, sessionMgr)
		case netserver.StateAuthenticated:
			handleAuthenticatedMessage(server, session, msg, body, rawBody, config, db, userEngine, mapMgr)
		case netserver.StateInGame:
			if msg.Ident == protocol.CMLogout || msg.Ident == protocol.CMExitGame || msg.Ident == protocol.CMSoftClose {
				player := userEngine.GetPlayer(int32(session.CharacterID))
				if player != nil {
					if msg.Ident == protocol.CMExitGame {
						userEngine.ExitPlayer(server, db, player)
					} else {
						// CM_SOFTCLOSE（Delphi ObjBase.pas:4751-4755）：软关闭回选角，
						// 单进程下与 CM_LOGOUT 同走 LogoutPlayer（保留连接回 Authenticated 态）。
						userEngine.LogoutPlayer(server, db, player)
					}
				}
				return
			}
			handleGameMessage(server, session, msg, body, userEngine)
		}
	})

	if err := server.Start(); err != nil {
		log.Logf(log.LevelError, "Server", "failed to start server: %v", err)
		os.Exit(1)
	}

	ticker := time.NewTicker(time.Duration(config.GetTickInterval()) * time.Millisecond)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Logf(log.LevelInfo, "Server", "server started. Press Ctrl+C to stop.")

	tickCount := int64(0)
	lastGuildRun := int64(0)
	lastOnlineMsg := int64(0)
	for {
		select {
		case <-ticker.C:
			tickCount++
			now := time.Now().UnixMilli()
			userEngine.ProcessHumans(server)
			userEngine.ProcessMonsters(server, now)
			userEngine.ProcessDoors(server, now)
			userEngine.ProcessEvents(server, now)
			userEngine.ProcessMineRegen(now)
			userEngine.ProcessNpcIdle()
			if userEngine.Castle != nil {
				userEngine.Castle.ProcessCastleTick(userEngine, server, now)
			}
			// Delphi g_GuildManager.Run 每 10 秒（Guild.pas:261）：行会战到期清理
			if now-lastGuildRun >= 10000 {
				lastGuildRun = now
				userEngine.ProcessGuilds(server, now)
			}
			// Delphi boSendOnlineCount（UsrEngn.pas:3054-3058）：周期系统广播在线人数
			if !config.Game.DisableOnlineCount && now-lastOnlineMsg >= config.GetSendOnlineTime() {
				lastOnlineMsg = now
				count := userEngine.GetPlayerCount() * config.GetSendOnlineCountRate() / 10
				text := strings.Replace(config.GetSendOnlineCountMsg(), "%c", strconv.Itoa(count), 1)
				onlineResp := protocol.MakeDefaultMsg(protocol.SMSysMessage, 0, 0, 0, 0)
				for _, p := range userEngine.allPlayers() {
					if !p.Ghost {
						server.Send(p.Session.ID, onlineResp, protocol.EncodeString(text))
					}
				}
			}
			if tickCount%int64(config.GetSaveInterval()) == 0 {
				userEngine.ProcessNpcs()
				userEngine.SaveAllPlayers(db)
			}
			if tickCount%int64(config.GetGuildSaveInterval()) == 0 {
				userEngine.SaveGuilds()
				if userEngine.Castle != nil {
					userEngine.Castle.SaveState(db)
				}
			}
		case sig := <-sigChan:
			fmt.Println()
			log.Logf(log.LevelInfo, "Server", "received signal: %v", sig)
			log.Logf(log.LevelInfo, "Server", "shutting down...")
			userEngine.SaveGuilds()
			if userEngine.Castle != nil {
				userEngine.Castle.SaveState(db)
			}
			server.Stop()
			log.Logf(log.LevelInfo, "Server", "server stopped")
			return
		}
	}
}

// handleConnectedMessage 处理 Connected 状态（认证前）的消息。
func handleConnectedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body, rawBody string, config *ServerConfig, db *storage.Database, sessionMgr *SessionManager) {
	switch msg.Ident {
	case protocol.CMProtocol:
		log.Logf(log.LevelInfo, "Server", "protocol version: %d", msg.Recog)

	case protocol.CMAddNewUser:
		// Delphi 同一连接两次注册请求间隔必须 > 5000ms，否则静默丢弃并记日志
		//（LoginSrv/LMain.pas:979-984）。
		if !throttleOK(session.ID, "addnewuser", 5*time.Second) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 新建帐号 (session %d)", session.ID)
			return
		}
		// Body = EncodeBuffer(TUserEntry) + EncodeBuffer(TUserEntryAdd): 两个
		// 独立的 6Bit 段（ClMain.pas:2844）。在解码前按固定编码长度
		// 分割原始载荷；一次性解码整个字符串会导致第二段错位。
		if len(rawBody) < protocol.UserEntryEncodedSize+protocol.UserEntryAddEncodedSize {
			log.Logf(log.LevelWarn, "Server", "[CMAddNewUser] body too short (%d chars)", len(rawBody))
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, -2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		ueBuf := make([]byte, protocol.UserEntrySize)
		protocol.DecodeBuffer(rawBody[:protocol.UserEntryEncodedSize], ueBuf)
		uaBuf := make([]byte, protocol.UserEntryAddSize)
		protocol.DecodeBuffer(rawBody[protocol.UserEntryEncodedSize:protocol.UserEntryEncodedSize+protocol.UserEntryAddEncodedSize], uaBuf)
		ue := protocol.UserEntryFromBytes(ueBuf)
		ua := protocol.UserEntryAddFromBytes(uaBuf)
		username := strings.ToLower(ue.Account())
		password := ue.Password()
		log.Logf(log.LevelInfo, "Server", "[CMAddNewUser] registration attempt: %s", username)
		// Delphi SM_NEWID_FAIL Recog: 0=已存在, -2=系统繁忙, 其他=非法 (ClMain.pas:3694-3702)。
		// 账号名字符集校验移植自 CheckAccountName (LoginSrv/LSShare.pas:169-192)。
		if !checkAccountName(username) || len(username) < config.GetMinAcctLen() || len(username) > config.GetMaxAcctLen() || len(password) < config.GetMinPassLen() || len(password) > config.GetMaxPassLen() {
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, -1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		_, _, err := db.GetAccountByUsername(username)
		if err == nil {
			log.Logf(log.LevelWarn, "Server", "[CMAddNewUser] account already exists: %s", username)
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		hash := simpleHash(password)
		info := accountInfoFromEntry(&ue, &ua)
		_, err = db.CreateAccountWithEntry(username, hash, info)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMAddNewUser] creation failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, -2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		log.Logf(log.LevelInfo, "Server", "[CMAddNewUser] account created: %s", username)
		resp := protocol.MakeDefaultMsg(protocol.SMNewIDSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMChangePassword:
		// Delphi 同一连接两次改密请求间隔必须 > 5000ms，否则静默丢弃
		//（LoginSrv/LMain.pas:987-996）。
		if !throttleOK(session.ID, "changepw", 5*time.Second) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 修改密码 (session %d)", session.ID)
			return
		}
		// Body = id + #9 + passwd + #9 + newpasswd（ClMain.pas:2864-2870）。
		parts := strings.Split(body, "\t")
		if len(parts) != 3 {
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, -3, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		id, oldpw, newpw := parts[0], parts[1], parts[2]
		log.Logf(log.LevelInfo, "Server", "[CMChangePassword] change attempt: %s", id)
		// Delphi 顺序（LMain.pas:1098-1123）：新密码 >=3 → 查账号 → 错误计数锁
		//（5 次/180 秒，与登录共享 nErrorCount）→ 旧密码比对。
		// 新密码过短与账号不存在都回 Recog=0。
		if len(newpw) < config.GetMinPassLen() {
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		accountID, hash, err := db.GetAccountByUsername(id)
		if err != nil {
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		loginAttempts.Lock()
		attempt := loginAttempts.m[id]
		if attempt != nil && attempt.count >= config.GetLoginMaxAttempts() && time.Since(attempt.lockTime) < 180*time.Second {
			loginAttempts.Unlock()
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, -2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		loginAttempts.Unlock()
		if !verifyPassword(oldpw, hash) {
			loginAttempts.Lock()
			if loginAttempts.m[id] == nil {
				loginAttempts.m[id] = &loginAttempt{}
			}
			loginAttempts.m[id].count++
			if loginAttempts.m[id].count >= config.GetLoginMaxAttempts() {
				loginAttempts.m[id].lockTime = time.Now()
			}
			loginAttempts.Unlock()
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, -1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		if err := db.UpdateAccountPassword(accountID, simpleHash(newpw)); err != nil {
			log.Logf(log.LevelError, "Server", "[CMChangePassword] update failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdFail, -3, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		loginAttempts.Lock()
		delete(loginAttempts.m, id)
		loginAttempts.Unlock()
		log.Logf(log.LevelInfo, "Server", "[CMChangePassword] password changed: %s", id)
		resp := protocol.MakeDefaultMsg(protocol.SMChgPasswdSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMIDPassword:
		// 从 body 解析用户名/密码（格式: "username/password"）
		username, password := parseCredentials(body)
		log.Logf(log.LevelInfo, "Server", "login attempt: %s", username)

		// 账号黑名单（Delphi DenyAccountList 语义）：登录期拒绝
		if isAccountBlocked(username) {
			log.Logf(log.LevelWarn, "Server", "blocked account rejected: %s", username)
			resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		// 登录失败锁定检查（Delphi: 5次后锁定60秒，LoginSrv/LMain.pas:1218-1230）
		loginAttempts.Lock()
		attempt := loginAttempts.m[username]
		if attempt != nil && attempt.count >= config.GetLoginMaxAttempts() && time.Since(attempt.lockTime) < time.Duration(config.GetLoginLockoutSecs())*time.Second {
			loginAttempts.Unlock()
			log.Logf(log.LevelWarn, "Server", "account locked (too many attempts): %s", username)
			resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		loginAttempts.Unlock()

		// 对照数据库验证
		accountID, passwordHash, err := db.GetAccountByUsername(username)
		if err != nil {
			log.Logf(log.LevelWarn, "Server", "account not found: %s", username)
			resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		// 验证密码
		if !verifyPassword(password, passwordHash) {
			log.Logf(log.LevelWarn, "Server", "invalid password: %s", username)
			loginAttempts.Lock()
			if loginAttempts.m[username] == nil {
				loginAttempts.m[username] = &loginAttempt{}
			}
			loginAttempts.m[username].count++
			if loginAttempts.m[username].count >= config.GetLoginMaxAttempts() {
				loginAttempts.m[username].lockTime = time.Now()
			}
			loginAttempts.Unlock()
			sendLoginFail(server, session)
			return
		}

		// 认证成功，清除失败记录
		loginAttempts.Lock()
		delete(loginAttempts.m, username)
		loginAttempts.Unlock()

		// 资料完整性检查（Delphi: 密码正确但 sUserName='' 或 sQuiz2='' 时
		// 要求补全资料，LoginSrv/LMain.pas:1213-1216）。
		needUpdate := false
		if info, ierr := db.GetAccountInfo(username); ierr == nil && (info.UserName == "" || info.Quiz2 == "") {
			needUpdate = true
		}

		// 重复登录检测（Delphi: LoginSrv/LMain.pas:1235-1238）。
		// Delphi 语义：踢掉旧会话的游戏角色，但新连接登录失败并回 -3
		//「这个账号已经登录」，而不是让新连接顶替成功。
		if old := sessionMgr.GetByAccount(username); old != nil && old.ID != session.ID {
			log.Logf(log.LevelInfo, "Server", "account %s already logged in (session %d), rejecting session %d with -3",
				username, old.ID, session.ID)
			server.CloseSession(old.ID)
			if needUpdate {
				// Delphi 在发送 SM_PASSWD_FAIL 之前先发 SM_NEEDUPDATE_ACCOUNT
				//（LMain.pas:1239-1244 在 nCode 分支之前执行）。
				sendNeedUpdateAccount(server, session, db, username)
			}
			resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -3, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		if needUpdate {
			sendNeedUpdateAccount(server, session, db, username)
		}

		session.State = netserver.StateAuthenticated
		session.AccountName = username
		session.CharacterID = accountID
		log.Logf(log.LevelInfo, "Server", "session %d: connected -> authenticated (account=%s ID=%d)",
			session.ID, username, accountID)

		resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
		serverName := config.Server.Name
		if serverName == "" {
			serverName = "Server"
		}
		server.Send(session.ID, resp, protocol.EncodeString(serverName+"/1"))
		log.Logf(log.LevelInfo, "Server", "login successful: %s (account=%d)", username, accountID)

	default:
		log.Logf(log.LevelWarn, "Server", "unexpected message %d in Connected state", msg.Ident)
	}
}

// handleAuthenticatedMessage 处理 Authenticated 状态（选角）的消息。
func handleAuthenticatedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body, rawBody string, config *ServerConfig, db *storage.Database, userEngine *UserEngine, mapMgr *MapManager) {
	switch msg.Ident {
	case protocol.CMSelectServer:
		log.Logf(log.LevelInfo, "Server", "[CMSelectServer] body=%q", body)

		// Fix 2: 发送 SMSelectServerOK，内容为 "addr/port/certification"
		cert := rand.Int31()
		session.Certification = cert
		host, port := config.GetServerHostPort()
		addrBody := fmt.Sprintf("%s/%d/%d", host, port, cert)
		resp := protocol.MakeDefaultMsg(protocol.SMSelectServerOK, 0, 0, 0, 0)
		server.Send(session.ID, resp, protocol.EncodeString(addrBody))
		log.Logf(log.LevelInfo, "Server", "[CMSelectServer] sent SMSelectServerOK: %s (cert=%d)", addrBody, cert)

	case protocol.CMUpdateUser:
		// 补全账号资料（Delphi CM_UPDATEUSER，LoginSrv/LMain.pas:1422-1470）。
		// Delphi 限流：同一连接两次请求间隔 > 5000ms（LMain.pas:997-1004）。
		if !throttleOK(session.ID, "updateuser", 5*time.Second) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 补全资料 (session %d)", session.ID)
			return
		}
		// body 与注册同构：EncodeBuffer(TUserEntry)+EncodeBuffer(TUserEntryAdd)。
		if len(rawBody) < protocol.UserEntryEncodedSize+protocol.UserEntryAddEncodedSize {
			resp := protocol.MakeDefaultMsg(protocol.SMUpdateIDFail, -1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		ueBuf := make([]byte, protocol.UserEntrySize)
		protocol.DecodeBuffer(rawBody[:protocol.UserEntryEncodedSize], ueBuf)
		uaBuf := make([]byte, protocol.UserEntryAddSize)
		protocol.DecodeBuffer(rawBody[protocol.UserEntryEncodedSize:protocol.UserEntryEncodedSize+protocol.UserEntryAddEncodedSize], uaBuf)
		ue := protocol.UserEntryFromBytes(ueBuf)
		ua := protocol.UserEntryAddFromBytes(uaBuf)

		// Delphi 要求会话账号与提交的账号一致且账号名合法（LMain.pas:1443）。
		// 账号不匹配或非法 → Recog=-1；账号不存在 → Recog=0。
		if !strings.EqualFold(session.AccountName, ue.Account()) || !checkAccountName(ue.Account()) {
			resp := protocol.MakeDefaultMsg(protocol.SMUpdateIDFail, -1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		if _, _, err := db.GetAccountByUsername(session.AccountName); err != nil {
			resp := protocol.MakeDefaultMsg(protocol.SMUpdateIDFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		// 整条覆盖（含密码），Delphi LMain.pas:1449-1451。
		info := accountInfoFromEntry(&ue, &ua)
		if err := db.UpdateAccountInfo(session.AccountName, simpleHash(ue.Password()), info); err != nil {
			log.Logf(log.LevelError, "Server", "[CMUpdateUser] update failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMUpdateIDFail, -1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		log.Logf(log.LevelInfo, "Server", "[CMUpdateUser] account info updated: %s", session.AccountName)
		resp := protocol.MakeDefaultMsg(protocol.SMUpdateIDSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMQueryChr:
		// Delphi 限流：未查询过则放行；已查询过则要求距上次 chr 操作 >200ms
		//（DBServer/UsrSoc.pas:536-540）。
		if getChrQueried(session.ID) && !chrThrottleOK(session.ID, 200*time.Millisecond) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 查询角色 (session %d)", session.ID)
			return
		}
		chrThrottleOK(session.ID, 0) // 刷新共享 dwChrTick
		log.Logf(log.LevelInfo, "Server", "[CMQueryChr] accountID=%d", session.CharacterID)
		if sendCharacterList(server, session, db) {
			setChrQueried(session.ID, true) // Delphi: boChrQueryed:=True
		}

	case protocol.CMNewChr:
		// Delphi 限流：距上次 chr 操作须 >1000ms，否则静默丢弃
		//（DBServer/UsrSoc.pas:546-559）。
		if !chrThrottleOK(session.ID, 1000*time.Millisecond) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 创建角色 (session %d)", session.ID)
			return
		}
		// body: "loginID/charName/hair/job/sex"
		parts := strings.Split(body, "/")
		if len(parts) < 5 {
			log.Logf(log.LevelWarn, "Server", "[CMNewChr] invalid body: %q", body)
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		charName := strings.TrimSpace(parts[1]) // Delphi: sChrName:=Trim(sChrName) (UsrSoc.pas:750)
		var hair, job, sex int
		fmt.Sscanf(parts[2], "%d", &hair)
		fmt.Sscanf(parts[3], "%d", &job)
		fmt.Sscanf(parts[4], "%d", &sex)

		log.Logf(log.LevelInfo, "Server", "[CMNewChr] name=%q job=%d sex=%d hair=%d account=%d",
			charName, job, sex, hair, session.CharacterID)

		// Delphi 校验顺序（UsrSoc.pas:750-796）：
		// 1) 最小长度——原版按字节 length(sChrName) < 3 拒绝（UsrSoc.pas:751）。
		//    UTF-8 语义移植：单汉字为 3 字节，需补 rune 数 >= 2 才能等价拒绝
		//    （GBK 单汉字 2 字节 <3 被拒、双汉字 4 字节省放行）。
		// 2) 字符集——checkChrName 仅允许数字/字母/汉字（DBShare.pas:265-304 +
		//    UsrSoc.pas:757-789 禁止符号表）。
		// 3) 禁止字黑名单——整名大小写不敏感匹配（UsrSoc.pas:981-996），命中 Recog=2。
		tooShort := len(charName) < config.GetMinNameLen() || len([]rune(charName)) < 2
		if tooShort || len([]rune(charName)) > config.GetMaxNameLen() || !checkChrName(charName) {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		if !checkDenyChrName(charName, denyChrNameList) {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		chars, err := db.GetCharactersByAccount(session.CharacterID)
		if err == nil && len(chars) >= config.GetMaxCharsPerAcct() {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 3, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		// Delphi 做全局重名检查（UsrSoc.pas:794-796），而非仅同账号范围。
		exists, err := db.CharacterNameExists(charName)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMNewChr] name lookup failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 4, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		if exists {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		_, err = db.CreateCharacter(session.CharacterID, charName, job, sex, hair)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMNewChr] creation failed: %v", err)
			// Delphi 写库失败 Recog=4（UsrSoc.pas:833）。
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 4, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		log.Logf(log.LevelInfo, "Server", "[CMNewChr] created character %q, account %d", charName, session.CharacterID)
		setChrQueried(session.ID, false) // Delphi: 建角后须重新查角 (UsrSoc.pas:552)
		resp := protocol.MakeDefaultMsg(protocol.SMNewChrSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMDelChr:
		// Delphi 限流：距上次 chr 操作须 >1000ms（DBServer/UsrSoc.pas:561-574）。
		if !chrThrottleOK(session.ID, 1000*time.Millisecond) {
			log.Logf(log.LevelWarn, "Server", "[非法操作] 删除角色 (session %d)", session.ID)
			return
		}
		charName := body
		log.Logf(log.LevelInfo, "Server", "[CMDelChr] name=%q account=%d", charName, session.CharacterID)

		charData, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelWarn, "Server", "[CMDelChr] character %q not found: %v", charName, err)
			resp := protocol.MakeDefaultMsg(protocol.SMDelChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		if err := db.DeleteCharacter(charData.ID); err != nil {
			log.Logf(log.LevelError, "Server", "[CMDelChr] deletion failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMDelChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		log.Logf(log.LevelInfo, "Server", "[CMDelChr] deleted character %q (id=%d)", charName, charData.ID)
		setChrQueried(session.ID, false) // Delphi: 删角后须重新查角 (UsrSoc.pas:567)
		resp := protocol.MakeDefaultMsg(protocol.SMDelChrSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMSelChr:
		// Delphi 要求选角前必须先查角（boChrQueryed），防止重复提交
		//（DBServer/UsrSoc.pas:576-590，原码条件写反，这里按合理语义实现）。
		if !getChrQueried(session.ID) {
			log.Logf(log.LevelWarn, "Server", "Double send _SELCHR (session %d)", session.ID)
			return
		}
		// Fix 4: 从 body 解析角色名，而非 msg.Recog
		// 客户端发送: "loginID/charName"
		charName := body
		if idx := strings.Index(body, "/"); idx >= 0 {
			charName = body[idx+1:]
		}
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] body=%q character=%q account ID=%d", body, charName, session.CharacterID)

		// 验证该账号下角色存在（不要覆盖 session.CharacterID ——
		// 它仍保存着 **runlogin 处理器所需的账号 ID）
		_, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMSelChr] character %q not found, account %d: %v",
				charName, session.CharacterID, err)
			return
		}
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] character %q validated", charName)
		setChrQueried(session.ID, false) // 选角完成，防止重复选角

		// Fix 6: 发送 SMStartPlay，内容为 "addr/port"（同一服务器）
		// 这里不创建 PlayObject —— 等待 **runlogin
		host, port := config.GetServerHostPort()
		startBody := fmt.Sprintf("%s/%d", host, port)
		startResp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
		server.Send(session.ID, startResp, protocol.EncodeString(startBody))
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] sent SMStartPlay: %s", startBody)

	default:
		log.Logf(log.LevelWarn, "Server", "unexpected message %d in Authenticated state", msg.Ident)
	}
}

// handleGameMessage 处理 InGame 状态（游戏中）的消息。
func handleGameMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, userEngine *UserEngine) {
	player := userEngine.GetPlayer(int32(session.CharacterID))
	if player == nil {
		log.Logf(log.LevelError, "Server", "player not found for session %d", session.ID)
		return
	}

	// 将消息路由到玩家消息队列，在游戏 tick 中处理
	switch msg.Ident {
	case protocol.CMTurn:
		player.SendMsg(protocol.CMTurn, int(msg.Param), 0, 0, "")
	case protocol.CMWalk:
		player.SendMsg(protocol.CMWalk, int(msg.Param), 0, 0, "")
	case protocol.CMRun:
		player.SendMsg(protocol.CMRun, int(msg.Param), 0, 0, "")
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit, protocol.CMPowerHit, protocol.CMLongHit, protocol.CMWideHit, protocol.CMFireHit, protocol.CMCrsHit, protocol.CMTwinHit:
		player.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series), "")
	case protocol.CMSpell:
		player.SendMsg(protocol.CMSpell, int(msg.Param), int(msg.Tag), int(msg.Series), body)
	case protocol.CMSay:
		player.SendMsg(protocol.CMSay, 0, 0, 0, body)
	case protocol.CMClickNPC:
		player.SendMsg(protocol.CMClickNPC, int(msg.Recog), 0, 0, "")
	case protocol.CMCreateGroup:
		// Body = 要邀请的目标玩家名。
		player.SendMsg(protocol.CMCreateGroup, 0, 0, 0, body)
	case protocol.CMPickup:
		player.SendMsg(protocol.CMPickup, 0, 0, 0, "")
	case protocol.CMTakeOnItem:
		// Recog=MakeIndex, Param=目标槽位。
		player.SendMsg(protocol.CMTakeOnItem, int(msg.Recog), int(msg.Param), 0, "")
	case protocol.CMTakeOffItem:
		player.SendMsg(protocol.CMTakeOffItem, int(msg.Param), 0, 0, "")
	case protocol.CMEat:
		// 物品 CM 消息在 Recog 中携带 32 位 MakeIndex（Delphi 约定）。
		player.SendMsg(protocol.CMEat, int(msg.Recog), 0, 0, "")
	case protocol.CMMagicKeyChange:
		player.SendMsg(protocol.CMMagicKeyChange, int(msg.Recog), int(msg.Param), 0, "")
	case protocol.CMDealTry:
		player.SendMsg(protocol.CMDealTry, 0, 0, 0, body)
	case protocol.CMDealAddItem:
		player.SendMsg(protocol.CMDealAddItem, int(msg.Recog), 0, 0, "")
	case protocol.CMDealDelItem:
		// Recog = 要取回的已提供物品的 MakeIndex。
		player.SendMsg(protocol.CMDealDelItem, int(msg.Recog), 0, 0, "")
	case protocol.CMDealCancel:
		player.SendMsg(protocol.CMDealCancel, 0, 0, 0, "")
	case protocol.CMDealChgGold:
		player.SendMsg(protocol.CMDealChgGold, int(msg.Recog), 0, 0, "")
	case protocol.CMDealEnd:
		player.SendMsg(protocol.CMDealEnd, 0, 0, 0, "")
	case protocol.CMUserStorageItem:
		player.SendMsg(protocol.CMUserStorageItem, int(msg.Recog), 0, 0, "")
	case protocol.CMUserTakeBackStorageItem:
		player.SendMsg(protocol.CMUserTakeBackStorageItem, int(msg.Param), 0, 0, "")
	case protocol.CMOpenGuildDlg:
		player.SendMsg(protocol.CMOpenGuildDlg, 0, 0, 0, body)
	case protocol.CMGuildMemberList:
		player.SendMsg(protocol.CMGuildMemberList, 0, 0, 0, "")
	case protocol.CMGuildUpdateRankInfo:
		player.SendMsg(protocol.CMGuildUpdateRankInfo, 0, 0, 0, body)
	case protocol.CMGuildHome:
		player.SendMsg(protocol.CMGuildHome, 0, 0, 0, "")
	case protocol.CMGroupMode:
		player.SendMsg(protocol.CMGroupMode, int(msg.Param), 0, 0, "")
	case protocol.CMAddGroupMember:
		player.SendMsg(protocol.CMAddGroupMember, 0, 0, 0, body)
	case protocol.CMDelGroupMember:
		player.SendMsg(protocol.CMDelGroupMember, 0, 0, 0, body)
	case protocol.CMGuildAddMember:
		player.SendMsg(protocol.CMGuildAddMember, 0, 0, 0, body)
	case protocol.CMGuildDelMember:
		player.SendMsg(protocol.CMGuildDelMember, 0, 0, 0, body)
	case protocol.CMGuildAlly:
		player.SendMsg(protocol.CMGuildAlly, 0, 0, 0, body)
	case protocol.CMGuildBreakAlly:
		player.SendMsg(protocol.CMGuildBreakAlly, 0, 0, 0, body)
	case protocol.CMGuildUpdateNotice:
		player.SendMsg(protocol.CMGuildUpdateNotice, 0, 0, 0, body)
	case protocol.CMGuildWar:
		// Body = 目标行会名。
		player.SendMsg(protocol.CMGuildWar, 0, 0, 0, body)
	case protocol.CMButch:
		// Recog = 目标怪物ID。
		player.SendMsg(protocol.CMButch, int(msg.Recog), 0, 0, "")
	case protocol.CMMineDig:
		player.SendMsg(protocol.CMMineDig, 0, 0, 0, "")
	case protocol.CMWhisper:
		// Body = "目标名/消息内容"。
		player.SendMsg(protocol.CMWhisper, 0, 0, 0, body)
	case protocol.CMAddFriend:
		player.SendMsg(protocol.CMAddFriend, 0, 0, 0, body)
	case protocol.CMDelFriend:
		player.SendMsg(protocol.CMDelFriend, 0, 0, 0, body)
	case protocol.CMQueryFriends:
		player.SendMsg(protocol.CMQueryFriends, 0, 0, 0, "")
	case protocol.CMHorseRun:
		player.SendMsg(protocol.CMHorseRun, int(msg.Param), 0, 0, "")
	case protocol.CMOpenDoor:
		player.SendMsg(protocol.CMOpenDoor, 0, 0, 0, "")
	case protocol.CMUserBuyItem:
		// Delphi SendBuyItem（ClMain.pas:3160-3166）：Recog=商人，
		// Param/Tag=Lo/Hi(MakeIndex)，body=物品名。
		player.SendMsg(protocol.CMUserBuyItem, int(msg.Recog), int(uint32(msg.Tag)<<16|uint32(msg.Param)), 0, body)
	case protocol.CMUserSellItem:
		player.SendMsg(protocol.CMUserSellItem, int(msg.Recog), 0, 0, "")
	case protocol.CMUserRepairItem:
		player.SendMsg(protocol.CMUserRepairItem, int(msg.Recog), 0, 0, "")
	case protocol.CMMerchantQuerySellPrice:
		player.SendMsg(protocol.CMMerchantQuerySellPrice, int(msg.Recog), 0, 0, "")
	case protocol.CMMerchantQueryRepairCost:
		player.SendMsg(protocol.CMMerchantQueryRepairCost, int(msg.Recog), 0, 0, "")
	case protocol.CMUserMakeDrugItem:
		// Delphi SendMakeDrugItem（FState.pas:5045-5047）：body=物品名。
		player.SendMsg(protocol.CMUserMakeDrugItem, int(msg.Recog), 0, 0, body)
	case protocol.CMMerchantDlgSelect:
		// Body 携带点击的链接标签。
		player.SendMsg(protocol.CMMerchantDlgSelect, int(msg.Recog), int(msg.Param), 0, body)
	case protocol.CMDropItem:
		player.SendMsg(protocol.CMDropItem, int(msg.Recog), 0, 0, "")
	case protocol.CMDropGold:
		player.SendMsg(protocol.CMDropGold, int(msg.Recog), 0, 0, "")
	case protocol.CMChangeAttackMode:
		player.SendMsg(protocol.CMChangeAttackMode, int(msg.Param), 0, 0, "")
	case protocol.CMAdjustBonus:
		player.SendMsg(protocol.CMAdjustBonus, int(msg.Recog), 0, 0, body)
	case protocol.CMQueryUserState:
		player.SendMsg(protocol.CMQueryUserState, int(msg.Recog), 0, 0, "")
	case protocol.CMQueryUsername:
		// Delphi: Param1=目标ID Param2=x Param3=y（ObjBase.pas:4663-4666）。
		player.SendMsg(protocol.CMQueryUsername, int(msg.Recog), int(msg.Param), int(msg.Tag), "")
	case protocol.CMUserGetDetailItem:
		// Delphi: Param1=商人 Param2=偏移 sMsg=物品名（ObjBase.pas:4768-4769）。
		player.SendMsg(protocol.CMUserGetDetailItem, int(msg.Recog), int(msg.Param), 0, body)
	case protocol.CMSitdown:
		// Param = 朝向（屠宰动作共用此消息，客户端 Alt+左键）。
		player.SendMsg(protocol.CMSitdown, int(msg.Param), 0, 0, "")
	case protocol.CMWantMinimap:
		player.SendMsg(protocol.CMWantMinimap, 0, 0, 0, "")
	case protocol.CMLoginNoticeOK:
		log.Logf(log.LevelInfo, "Server", "%s confirmed notice", player.Name)
		player.SendLogon(server)
		player.SendBagItemsFull(server)
		player.SendUseItemsFull(server)
		player.SendMyMagicFull(server)
		player.SendSpecialAttackFlags(server)
		player.SendDayChanging(server)
		player.SendMapDescription(server)
		player.SendAreaState(server)
		// Delphi RefMyStatus（ObjBase.pas:6193-6196）：饥饿状态。
		player.sendMyStatus(server)
		player.SendSubAbility(server)
		player.SendRefMsg(RM_TURN, player.Dir, player.CurrX, player.CurrY, player.Name)
	case protocol.CMQueryBagItems:
		player.SendBagItemsFull(server)
	default:
		log.Logf(log.LevelDebug, "Server", "unhandled game message: %d from %s", msg.Ident, player.Name)
	}
}

// sendCharacterList 向客户端发送角色列表。
// Fix 3: 使用文本格式 "*name/job/hair/level/sex/..." 代替二进制。
func sendCharacterList(server *netserver.TCPServer, session *netserver.Session, db *storage.Database) bool {
	log.Logf(log.LevelInfo, "Server", "[sendCharacterList] loading characters for account %d...", session.CharacterID)
	chars, err := db.GetCharactersByAccount(session.CharacterID)
	if err != nil {
		log.Logf(log.LevelError, "Server", "[sendCharacterList] failed: %v", err)
		resp := protocol.MakeDefaultMsg(protocol.SMQueryChrFail, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		return false
	}

	// 编码为文本: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
	// '*' 前缀标记上次选择的角色
	var sb strings.Builder
	for i, c := range chars {
		if i > 0 {
			sb.WriteByte('/')
		}
		if i == 0 {
			sb.WriteByte('*') // 标记第一个为已选择
		}
		sb.WriteString(c.Name)
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Job))
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Hair))
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Level))
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Sex))
	}

	// msg.Param = 角色数量
	resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, int32(len(chars)), 0, 0, 0)
	server.Send(session.ID, resp, protocol.EncodeString(sb.String()))

	log.Logf(log.LevelInfo, "Server", "sent %d characters to session %d", len(chars), session.ID)
	return true
}


// sendLoginFail 发送登录失败响应。
// Fix 1: 使用 SMPasswdFail (503) 代替 SMQueryChrFail (527)。
func sendLoginFail(server *netserver.TCPServer, session *netserver.Session) {
	log.Logf(log.LevelWarn, "Server", "[sendLoginFail] session=%d", session.ID)
	resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -1, 0, 0, 0)
	server.Send(session.ID, resp, "")
}

// saveCharacterData 将玩家数据保存到数据库。
func saveCharacterData(db *storage.Database, player *PlayObject) {
	c := &storage.Character{
		ID:    int64(player.ID),
		Map:   player.MapName,
		X:     player.CurrX,
		Y:     player.CurrY,
		Level: int(player.WAbil.Level),
		HP:    int(player.WAbil.HP),
		MP:    int(player.WAbil.MP),
		Exp:   int64(player.WAbil.Exp),
		Gold:  int64(player.Gold),
	}
	if err := db.UpdateCharacter(c); err != nil {
		log.Logf(log.LevelError, "Server", "failed to save character %s: %v", player.Name, err)
	} else {
		log.Logf(log.LevelDebug, "Server", "saved character %s at %s(%d,%d)", player.Name, player.MapName, player.CurrX, player.CurrY)
	}

	savePlayerItems(db, player)

	type charMeta struct {
		PkPoint         int             `json:"pkPoint"`
		Dir             int             `json:"dir"`
		BonusPoint      int             `json:"bonusPoint"`
		CreditPoint     int             `json:"creditPoint"`
		ReNewLevel      int             `json:"reNewLevel"`
		AttackMode      int             `json:"attackMode"`
		AllowGroup      bool            `json:"allowGroup"`
		HungerStatus    int             `json:"hungerStatus"`
		StoragePassword string          `json:"storagePassword,omitempty"`
		StatusTimeArr   [12]int16       `json:"statusTimeArr"`
		Magics          []PlayerMagic   `json:"magics"`
		Storage         []savedUserItem `json:"storage"`
		DearName        string          `json:"dearName"`
		MasterName      string          `json:"masterName"`
		ApprenticeNames []string        `json:"apprenticeNames"`
		Friends         []string        `json:"friends,omitempty"`
		QuestUnitOpen   []byte          `json:"questUnitOpen,omitempty"`
		QuestUnit       []byte          `json:"questUnit,omitempty"`
		QuestFlag       []byte          `json:"questFlag,omitempty"`
	}
	meta := charMeta{
		PkPoint:         player.PkPoint,
		Dir:             player.Dir,
		BonusPoint:      player.BonusPoint,
		CreditPoint:     player.CreditPoint,
		ReNewLevel:      player.ReNewLevel,
		AttackMode:      int(player.AttackMode),
		AllowGroup:      player.AllowGroup,
		HungerStatus:    player.HungerStatus,
		StoragePassword: player.StoragePassword,
		StatusTimeArr:   player.StatusTimeArr,
		Magics:          make([]PlayerMagic, 0, len(player.LearnedMagics)),
		DearName:        player.DearName,
		MasterName:      player.MasterName,
		ApprenticeNames: player.ApprenticeNames,
		Friends:         player.Friends,
		QuestUnitOpen:   player.QuestUnitOpen[:],
		QuestUnit:       player.QuestUnit[:],
		QuestFlag:       player.QuestFlag[:],
	}
	for _, pm := range player.LearnedMagics {
		if pm != nil {
			meta.Magics = append(meta.Magics, *pm)
		}
	}
	for _, item := range player.StorageItems {
		if item != nil {
			meta.Storage = append(meta.Storage, savedUserItem{
				MakeIndex: item.MakeIndex,
				WIndex:    item.WIndex,
				Dura:      item.Dura,
				DuraMax:   item.DuraMax,
				BtValue:   item.BtValue,
			})
		}
	}
	metaJSON, err := json.Marshal(meta)
	if err == nil {
		db.SaveCharacterMeta(int64(player.ID), metaJSON)
	}
}

type savedUserItem struct {
	MakeIndex int32    `json:"makeIndex"`
	WIndex    uint16   `json:"wIndex"`
	Dura      uint16   `json:"dura"`
	DuraMax   uint16   `json:"duraMax"`
	BtValue   [14]byte `json:"btValue,omitempty"` // 武器升级附加属性
}

func savePlayerItems(db *storage.Database, player *PlayObject) {
	bag := make([]savedUserItem, 0, len(player.ItemList))
	for _, item := range player.ItemList {
		bag = append(bag, savedUserItem{
			MakeIndex: item.MakeIndex,
			WIndex:    item.WIndex,
			Dura:      item.Dura,
			DuraMax:   item.DuraMax,
			BtValue:   item.BtValue,
		})
	}
	bagJSON, err := json.Marshal(bag)
	if err != nil {
		log.Logf(log.LevelError, "Server", "failed to serialize bag items, %s: %v", player.Name, err)
		return
	}

	equip := make([]*savedUserItem, 13)
	for i := 0; i < 13; i++ {
		if player.UseItems[i] != nil {
			equip[i] = &savedUserItem{
				MakeIndex: player.UseItems[i].MakeIndex,
				WIndex:    player.UseItems[i].WIndex,
				Dura:      player.UseItems[i].Dura,
				DuraMax:   player.UseItems[i].DuraMax,
				BtValue:   player.UseItems[i].BtValue,
			}
		}
	}
	equipJSON, err := json.Marshal(equip)
	if err != nil {
		log.Logf(log.LevelError, "Server", "failed to serialize equipment items, %s: %v", player.Name, err)
		return
	}

	if err := db.SaveCharacterItems(int64(player.ID), bagJSON, equipJSON); err != nil {
		log.Logf(log.LevelError, "Server", "failed to save items, %s: %v", player.Name, err)
	}
}

func loadPlayerItems(db *storage.Database, player *PlayObject) {
	bagJSON, equipJSON, err := db.LoadCharacterItems(int64(player.ID))
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "failed to load items, %s: %v", player.Name, err)
	}

	if bagJSON == nil && equipJSON == nil {
		player.GiveItem(1)
		player.GiveItem(1)
		player.GiveItem(2)
		log.Logf(log.LevelInfo, "Server", "gave initial items to %s", player.Name)
		return
	}

	if bagJSON != nil {
		var bag []savedUserItem
		if err := json.Unmarshal(bagJSON, &bag); err == nil {
			for _, si := range bag {
				player.ItemList = append(player.ItemList, &protocol.UserItem{
					MakeIndex: si.MakeIndex,
					WIndex:    si.WIndex,
					Dura:      si.Dura,
					DuraMax:   si.DuraMax,
					BtValue:   si.BtValue,
				})
			}
		}
	}

	if equipJSON != nil {
		var equip []*savedUserItem
		if err := json.Unmarshal(equipJSON, &equip); err == nil {
			for i := 0; i < 13 && i < len(equip); i++ {
				if equip[i] != nil {
					player.UseItems[i] = &protocol.UserItem{
						MakeIndex: equip[i].MakeIndex,
						WIndex:    equip[i].WIndex,
						Dura:      equip[i].Dura,
						DuraMax:   equip[i].DuraMax,
						BtValue:   equip[i].BtValue,
					}
				}
			}
		}
	}

	player.updateAppearance()
}

// parseCredentials 解析 "username/password" 格式。
func parseCredentials(body string) (username, password string) {
	for i, c := range body {
		if c == '/' {
			return body[:i], body[i+1:]
		}
	}
	return body, ""
}

// simpleHash 创建用于开发的简单哈希（不可用于生产）。
func simpleHash(password string) string {
	// 基于 XOR 的简单哈希，仅用于开发
	// 生产环境请使用 bcrypt 或 argon2
	hash := make([]byte, len(password))
	for i, c := range password {
		hash[i] = byte(c) ^ 0x5A
	}
	return fmt.Sprintf("%x", hash)
}

// verifyPassword 验证密码与哈希是否匹配。
func verifyPassword(password, hash string) bool {
	return simpleHash(password) == hash
}

// accountInfoFromEntry 把客户端注册结构转为存储层资料。
func accountInfoFromEntry(ue *protocol.UserEntry, ua *protocol.UserEntryAdd) *storage.AccountInfo {
	return &storage.AccountInfo{
		UserName:    ue.UserName(),
		SSNo:        ue.SSNo(),
		Phone:       ue.Phone(),
		Quiz:        ue.Quiz(),
		Answer:      ue.Answer(),
		Email:       ue.EMail(),
		Quiz2:       ua.Quiz2(),
		Answer2:     ua.Answer2(),
		BirthDay:    ua.BirthDay(),
		MobilePhone: ua.MobilePhone(),
		Memo:        ua.Memo(),
		Memo2:       ua.Memo2(),
	}
}

// userEntryFromInfo 用已存资料构建回发客户端的 TUserEntry（SM_NEEDUPDATE_ACCOUNT，
// LoginSrv/LMain.pas:1239-1244）。密码列不下发——Go 只存哈希，客户端补全表单
// 需重新输入密码。
func userEntryFromInfo(username string, info *storage.AccountInfo) protocol.UserEntry {
	var ue protocol.UserEntry
	ue.SetAccount(username)
	ue.SetUserName(info.UserName)
	ue.SetSSNo(info.SSNo)
	ue.SetPhone(info.Phone)
	ue.SetQuiz(info.Quiz)
	ue.SetAnswer(info.Answer)
	ue.SetEMail(info.Email)
	return ue
}

// sendNeedUpdateAccount 发送 SM_NEEDUPDATE_ACCOUNT + EncodeBuffer(TUserEntry)，
// 要求客户端补全账号资料（LoginSrv/LMain.pas:1239-1244）。发送后登录流程照常
//（LMain.pas:1245-1282 仍发 SM_PASSOK_SELECTSERVER）。
func sendNeedUpdateAccount(server *netserver.TCPServer, session *netserver.Session, db *storage.Database, username string) {
	info, err := db.GetAccountInfo(username)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "[SMNeedUpdateAccount] load info failed for %s: %v", username, err)
		return
	}
	ue := userEntryFromInfo(username, info)
	resp := protocol.MakeDefaultMsg(protocol.SMNeedUpdateAccount, 0, 0, 0, 0)
	server.Send(session.ID, resp, protocol.EncodeBuffer(ue.Bytes()))
	log.Logf(log.LevelInfo, "Server", "[SMNeedUpdateAccount] sent to session %d (account %s)", session.ID, username)
}

// encodeCharacterInfo 编码角色信息用于网络传输。
func encodeCharacterInfo(c storage.CharacterInfo) []byte {
	buf := make([]byte, 24) // name(20) + job(1) + hair(1) + level(1) + sex(1)
	copy(buf[:20], c.Name)
	buf[20] = byte(c.Job)
	buf[21] = 0 // 发型
	buf[22] = byte(c.Level)
	buf[23] = byte(c.Sex)
	return buf
}

// encodeUint16 将 uint16 编码为字节（小端序）。
func encodeUint16(v uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, v)
	return buf
}
