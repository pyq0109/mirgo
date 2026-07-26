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
	"strings"
	"syscall"
	"time"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
	"github.com/pyq0109/mirgo/internal/storage"
)

func main() {
	configDir := flag.String("config", "serverconfig", "Path to serverconfig directory")
	mapDir := flag.String("maps", "asset/server/Map", "Path to map directory")
	flag.Parse()

	// Load configuration from serverconfig directory
	config, err := LoadConfig(*configDir)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to load config: %v", err)
		os.Exit(1)
	}

	listenAddr := config.GetListenAddr()
	dbPath := config.GetDatabasePath()

	log.Logf(log.LevelInfo, "Server", "Starting MIR2 Server...")
	log.Logf(log.LevelInfo, "Server", "Listen: %s", listenAddr)
	log.Logf(log.LevelInfo, "Server", "Database: %s", dbPath)
	log.Logf(log.LevelInfo, "Server", "Home: %s(%d,%d)", config.GetHomeMap(), config.GetHomeX(), config.GetHomeY())

	// Open single database file
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to open database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Logf(log.LevelInfo, "Server", "Database opened")

	mapMgr := NewMapManager(*mapDir)
	if err := mapMgr.LoadAllMaps(); err != nil {
		log.Logf(log.LevelError, "Server", "Failed to load maps: %v", err)
		os.Exit(1)
	}
	mapMgr.InitRoutes()

	var itemDB *ItemDB
	itemDBPath := filepath.Join(*configDir, "items", "std_items.jsonc")
	itemDB, err = LoadItemDB(itemDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "Failed to load ItemDB: %v (items disabled)", err)
		itemDB = nil
	}

	var magicDB *MagicDB
	magicDBPath := filepath.Join(*configDir, "magic", "magic_db.jsonc")
	magicDB, err = LoadMagicDB(magicDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "Failed to load MagicDB: %v (magic disabled)", err)
		magicDB = nil
	}

	var monsterDB *MonsterDB
	monsterDBPath := filepath.Join(*configDir, "monsters", "monster_db.jsonc")
	monsterDB, err = LoadMonsterDB(monsterDBPath)
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "Failed to load MonsterDB: %v (using defaults)", err)
		monsterDB = &MonsterDB{byName: make(map[string]*MonsterDef)}
	}

	dropTablesDir := filepath.Join(*configDir, "monsters", "mon_items")
	dropTables := LoadDropTables(dropTablesDir)

	safeZonesPath := filepath.Join(*configDir, "maps", "start_points.jsonc")
	LoadSafeZones(safeZonesPath)

	monGenPath := filepath.Join(*configDir, "monsters", "mon_gen.jsonc")

	sessionMgr := NewSessionManager()
	userEngine := NewUserEngine(db, mapMgr)
	userEngine.ItemDB = itemDB
	userEngine.MagicDB = magicDB
	userEngine.MonsterDB = monsterDB
	userEngine.DropTables = dropTables
	userEngine.monGenPath = monGenPath
	userEngine.InitWorld(mapMgr)
	server := netserver.NewTCPServer(listenAddr)

	server.SetConnectHandler(func(session *netserver.Session) {
		sessionMgr.Add(session)
		log.Logf(log.LevelInfo, "Server", "Session %d connected (total: %d)", session.ID, sessionMgr.Count())
	})

	server.SetDisconnectHandler(func(session *netserver.Session) {
		if session.State == netserver.StateInGame {
			player := userEngine.GetPlayer(int32(session.CharacterID))
			if player != nil {
				saveCharacterData(db, player)
				player.Ghost = true
				player.SendRefMsg(RM_DISAPPEAR, 0, 0, 0, "")
				if player.envir != nil {
					player.envir.RemoveObject(player.CurrX, player.CurrY, OS_MOVINGOBJECT, player)
				}
				userEngine.RemovePlayer(int32(session.CharacterID))
				log.Logf(log.LevelInfo, "Server", "Player %s saved and removed from world", player.Name)
			}
		}
		sessionMgr.Remove(session.ID)
		log.Logf(log.LevelInfo, "Server", "Session %d disconnected (total: %d)", session.ID, sessionMgr.Count())
	})

	// Fix 5: Handle **runlogin raw messages
	server.SetRawMessageHandler(func(session *netserver.Session, raw string) bool {
		// Format: **loginID/charName/cert/version/code
		if !strings.HasPrefix(raw, "**") {
			return false
		}
		loginInfo := raw[2:] // Strip **
		parts := strings.Split(loginInfo, "/")
		if len(parts) < 5 {
			log.Logf(log.LevelWarn, "Server", "Invalid run login format: %s", raw)
			return false
		}
		loginID := parts[0]
		charName := parts[1]
		var cert int32
		fmt.Sscanf(parts[2], "%d", &cert)

		log.Logf(log.LevelInfo, "Server", "[**RunLogin] user=%s char=%s cert=%d version=%s code=%s",
			loginID, charName, cert, parts[3], parts[4])
		log.Logf(log.LevelInfo, "Server", "[**RunLogin] session=%d sessionCert=%d accountID=%d",
			session.ID, session.Certification, session.CharacterID)

		// Validate certification matches session
		if session.Certification != 0 && session.Certification != cert {
			log.Logf(log.LevelWarn, "Server", "[**RunLogin] Cert mismatch: expected %d, got %d",
				session.Certification, cert)
		}

		// Load character and enter game
		log.Logf(log.LevelInfo, "Server", "[**RunLoading character %q for account %d...", charName, session.CharacterID)
		charData, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[**RunLogin] Character %q not found for account %d: %v",
				charName, session.CharacterID, err)
			return true
		}
		log.Logf(log.LevelInfo, "Server", "[**RunLogin] Character loaded: %s id=%d map=%s(%d,%d)",
			charData.Name, charData.ID, charData.Map, charData.X, charData.Y)

		// Create PlayObject
		player := NewPlayObject(session, charData.Name, int32(charData.ID))
		player.MapMgr = mapMgr
		player.ItemDB = itemDB
		player.MagicDB = magicDB
		player.Engine = userEngine
		player.MapName = charData.Map
		player.CurrX = charData.X
		player.CurrY = charData.Y
		player.Job = byte(charData.Job)
		player.Gender = byte(charData.Sex)
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

		// Find and set map environment
		envir := mapMgr.FindMap(charData.Map)
		if envir == nil {
			log.Logf(log.LevelError, "Server", "Map %s not found, using home map", charData.Map)
			envir = mapMgr.FindMap(config.GetHomeMap())
			if envir != nil {
				player.MapName = config.GetHomeMap()
				player.CurrX = config.GetHomeX()
				player.CurrY = config.GetHomeY()
			}
		}
		player.envir = envir

		// Add player to map
		if envir != nil {
			envir.AddObject(player.CurrX, player.CurrY, OS_MOVINGOBJECT, player)
		}

		// Update session state
		session.State = netserver.StateInGame
		session.CharacterID = charData.ID
		log.Logf(log.LevelInfo, "Server", "Session %d: Authenticated → InGame (char=%s id=%d)",
			session.ID, charData.Name, charData.ID)

		// Add to UserEngine
		userEngine.AddPlayer(player)
		player.ReadyToRun = true

		// Load or initialize items
		loadPlayerItems(db, player)

		if metaJSON, err := db.LoadCharacterMeta(charData.ID); err == nil && metaJSON != nil {
			var meta struct {
				PkPoint int           `json:"pkPoint"`
				Magics  []PlayerMagic `json:"magics"`
				Storage []int         `json:"storage"`
			}
			if json.Unmarshal(metaJSON, &meta) == nil {
				player.PkPoint = meta.PkPoint
				for i := range meta.Magics {
					player.LearnedMagics = append(player.LearnedMagics, &meta.Magics[i])
				}
				for _, idx := range meta.Storage {
					player.StorageItems = append(player.StorageItems, &protocol.UserItem{WIndex: uint16(idx)})
				}
			}
		}

		// Recalculate stats from equipment
		player.RecalcAbilitys()

		// Give starting spells based on job
		switch player.Job {
		case 0: // Warrior
			player.learnMagic(3, 0, 1) // 基本剑术
			player.learnMagic(4, 0, 2) // 攻杀剑术
		case 1: // Mage
			player.learnMagic(1, 0, 1) // 火球术
		case 2: // Taoist
			player.learnMagic(2, 0, 1) // 治愈术
		}

		// Send map info
		player.SendMapInfo(server)
		log.Logf(log.LevelInfo, "Server", "Sent map %s(%d,%d) to player %s",
			player.MapName, player.CurrX, player.CurrY, player.Name)

		// Send ability
		player.SendAbility(server)

		// Send bag and equipment
		player.SendBagItemsFull(server)
		player.SendUseItemsFull(server)

		// Send notice (SMLogon will be sent after CMLoginNoticeOK)
		noticeResp := protocol.MakeDefaultMsg(protocol.SMSendNotice, 0, 0, 0, 0)
		server.Send(session.ID, noticeResp, protocol.EncodeString("Welcome to MIR2 Go Server!"))

		log.Logf(log.LevelInfo, "Server", "Player %s entered game at %s(%d,%d)",
			player.Name, player.MapName, player.CurrX, player.CurrY)

		return true
	})

	server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body string) {
		stateNames := map[netserver.SessionState]string{
			netserver.StateConnected:     "Connected",
			netserver.StateAuthenticated: "Authenticated",
			netserver.StateInGame:        "InGame",
		}
		log.Logf(log.LevelInfo, "Server", "Session %d state=%s: dispatching %s",
			session.ID, stateNames[session.State], protocol.MsgName(msg.Ident))

		switch session.State {
		case netserver.StateConnected:
			handleConnectedMessage(server, session, msg, body, config, db)
		case netserver.StateAuthenticated:
			handleAuthenticatedMessage(server, session, msg, body, config, db, userEngine, mapMgr)
		case netserver.StateInGame:
			handleGameMessage(server, session, msg, body, userEngine)
		}
	})

	if err := server.Start(); err != nil {
		log.Logf(log.LevelError, "Server", "Failed to start server: %v", err)
		os.Exit(1)
	}

	ticker := time.NewTicker(time.Second / time.Duration(10))
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Logf(log.LevelInfo, "Server", "Server started. Press Ctrl+C to stop.")

	tickCount := int64(0)
	for {
		select {
		case <-ticker.C:
			tickCount++
			now := time.Now().UnixMilli()
			userEngine.ProcessHumans(server)
			userEngine.ProcessMonsters(server, tickCount*100)
			userEngine.ProcessDoors(tickCount * 100)
			userEngine.ProcessEvents(server, now)
			if tickCount%300 == 0 {
				userEngine.SaveAllPlayers(db)
			}
		case sig := <-sigChan:
			fmt.Println()
			log.Logf(log.LevelInfo, "Server", "Received signal: %v", sig)
			log.Logf(log.LevelInfo, "Server", "Shutting down...")
			server.Stop()
			log.Logf(log.LevelInfo, "Server", "Server stopped")
			return
		}
	}
}

// handleConnectedMessage handles messages in the Connected state (before authentication).
func handleConnectedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, config *ServerConfig, db *storage.Database) {
	switch msg.Ident {
	case protocol.CMProtocol:
		log.Logf(log.LevelInfo, "Server", "Protocol version: %d", msg.Recog)

	case protocol.CMAddNewUser:
		username, password := parseCredentials(body)
		log.Logf(log.LevelInfo, "Server", "[CMAddNewUser] Register attempt: %s", username)
		if username == "" || len(username) > 10 || password == "" || len(password) > 10 {
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, 2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		_, _, err := db.GetAccountByUsername(username)
		if err == nil {
			log.Logf(log.LevelWarn, "Server", "[CMAddNewUser] Account already exists: %s", username)
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, 1, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		hash := simpleHash(password)
		_, err = db.CreateAccount(username, hash)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMAddNewUser] Create failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMNewIDFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		log.Logf(log.LevelInfo, "Server", "[CMAddNewUser] Account created: %s", username)
		resp := protocol.MakeDefaultMsg(protocol.SMNewIDSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMIDPassword:
		// Parse username/password from body (format: "username/password")
		username, password := parseCredentials(body)
		log.Logf(log.LevelInfo, "Server", "Login attempt: %s", username)

		// Verify against database
		accountID, passwordHash, err := db.GetAccountByUsername(username)
		if err != nil {
			log.Logf(log.LevelWarn, "Server", "Account not found: %s", username)
			// Auto-create account for development (remove in production)
			hash := simpleHash(password)
			accountID, err = db.CreateAccount(username, hash)
			if err != nil {
				log.Logf(log.LevelError, "Server", "Failed to create account: %v", err)
				sendLoginFail(server, session)
				return
			}
			log.Logf(log.LevelInfo, "Server", "Auto-created account: %s (id=%d)", username, accountID)
		} else {
			// Verify password
			if !verifyPassword(password, passwordHash) {
				log.Logf(log.LevelWarn, "Server", "Invalid password for: %s", username)
				sendLoginFail(server, session)
				return
			}
		}

		// Authentication successful
		session.State = netserver.StateAuthenticated
		session.AccountName = username
		session.CharacterID = accountID
		log.Logf(log.LevelInfo, "Server", "Session %d: Connected → Authenticated (account=%s id=%d)",
			session.ID, username, accountID)

		// Fix 2: Send server list with body "serverName/status"
		resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
		serverName := config.Server.Name
		if serverName == "" {
			serverName = "Server"
		}
		server.Send(session.ID, resp, protocol.EncodeString(serverName+"/1"))
		log.Logf(log.LevelInfo, "Server", "Login successful for %s (account=%d)", username, accountID)

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Connected state", msg.Ident)
	}
}

// handleAuthenticatedMessage handles messages in the Authenticated state (character selection).
func handleAuthenticatedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, config *ServerConfig, db *storage.Database, userEngine *UserEngine, mapMgr *MapManager) {
	switch msg.Ident {
	case protocol.CMSelectServer:
		log.Logf(log.LevelInfo, "Server", "[CMSelectServer] body=%q", body)

		// Fix 2: Send SMSelectServerOK with "addr/port/certification"
		cert := rand.Int31()
		session.Certification = cert
		host, port := config.GetServerHostPort()
		addrBody := fmt.Sprintf("%s/%d/%d", host, port, cert)
		resp := protocol.MakeDefaultMsg(protocol.SMSelectServerOK, 0, 0, 0, 0)
		server.Send(session.ID, resp, protocol.EncodeString(addrBody))
		log.Logf(log.LevelInfo, "Server", "[CMSelectServer] Sent SMSelectServerOK: %s (cert=%d)", addrBody, cert)

	case protocol.CMQueryChr:
		log.Logf(log.LevelInfo, "Server", "[CMQueryChr] accountID=%d", session.CharacterID)
		sendCharacterList(server, session, db)

	case protocol.CMNewChr:
		// body: "loginID/charName/hair/job/sex"
		parts := strings.Split(body, "/")
		if len(parts) < 5 {
			log.Logf(log.LevelWarn, "Server", "[CMNewChr] Invalid body: %q", body)
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}
		charName := parts[1]
		var hair, job, sex int
		fmt.Sscanf(parts[2], "%d", &hair)
		fmt.Sscanf(parts[3], "%d", &job)
		fmt.Sscanf(parts[4], "%d", &sex)

		log.Logf(log.LevelInfo, "Server", "[CMNewChr] name=%q job=%d sex=%d hair=%d account=%d",
			charName, job, sex, hair, session.CharacterID)

		if charName == "" || len([]rune(charName)) > 14 {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		chars, err := db.GetCharactersByAccount(session.CharacterID)
		if err == nil && len(chars) >= 2 {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 3, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		_, err = db.GetCharacterByName(session.CharacterID, charName)
		if err == nil {
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 2, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		_, err = db.CreateCharacter(session.CharacterID, charName, job, sex)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMNewChr] Create failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMNewChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		log.Logf(log.LevelInfo, "Server", "[CMNewChr] Created character %q for account %d", charName, session.CharacterID)
		resp := protocol.MakeDefaultMsg(protocol.SMNewChrSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMDelChr:
		charName := body
		log.Logf(log.LevelInfo, "Server", "[CMDelChr] name=%q account=%d", charName, session.CharacterID)

		charData, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelWarn, "Server", "[CMDelChr] Character %q not found: %v", charName, err)
			resp := protocol.MakeDefaultMsg(protocol.SMDelChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		if err := db.DeleteCharacter(charData.ID); err != nil {
			log.Logf(log.LevelError, "Server", "[CMDelChr] Delete failed: %v", err)
			resp := protocol.MakeDefaultMsg(protocol.SMDelChrFail, 0, 0, 0, 0)
			server.Send(session.ID, resp, "")
			return
		}

		log.Logf(log.LevelInfo, "Server", "[CMDelChr] Deleted character %q (id=%d)", charName, charData.ID)
		resp := protocol.MakeDefaultMsg(protocol.SMDelChrSuccess, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMSelChr:
		// Fix 4: Parse character name from body instead of msg.Recog
		// Client sends: "loginID/charName"
		charName := body
		if idx := strings.Index(body, "/"); idx >= 0 {
			charName = body[idx+1:]
		}
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] body=%q charName=%q accountID=%d", body, charName, session.CharacterID)

		// Validate character exists for this account (don't overwrite session.CharacterID —
		// it still holds the account ID needed by the **runlogin handler)
		_, err := db.GetCharacterByName(session.CharacterID, charName)
		if err != nil {
			log.Logf(log.LevelError, "Server", "[CMSelChr] Character %q not found for account %d: %v",
				charName, session.CharacterID, err)
			return
		}
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] Character %q validated", charName)

		// Fix 6: Send SMStartPlay with "addr/port" (same server)
		// Don't create PlayObject here — wait for **runlogin
		host, port := config.GetServerHostPort()
		startBody := fmt.Sprintf("%s/%d", host, port)
		startResp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
		server.Send(session.ID, startResp, protocol.EncodeString(startBody))
		log.Logf(log.LevelInfo, "Server", "[CMSelChr] Sent SMStartPlay: %s", startBody)

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Authenticated state", msg.Ident)
	}
}

// handleGameMessage handles messages in the InGame state (gameplay).
func handleGameMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, userEngine *UserEngine) {
	player := userEngine.GetPlayer(int32(session.CharacterID))
	if player == nil {
		log.Logf(log.LevelError, "Server", "Player not found for session %d", session.ID)
		return
	}

	// Route message to player's message queue for processing in game tick
	switch msg.Ident {
	case protocol.CMTurn:
		player.SendMsg(protocol.CMTurn, int(msg.Param), 0, 0, "")
	case protocol.CMWalk:
		player.SendMsg(protocol.CMWalk, int(msg.Param), 0, 0, "")
	case protocol.CMRun:
		player.SendMsg(protocol.CMRun, int(msg.Param), 0, 0, "")
	case protocol.CMHit, protocol.CMHeavyHit, protocol.CMBigHit, protocol.CMPowerHit, protocol.CMLongHit, protocol.CMWideHit, protocol.CMFireHit, protocol.CMTwinHit:
		player.SendMsg(int(msg.Ident), int(msg.Param), int(msg.Tag), int(msg.Series), "")
	case protocol.CMSpell:
		player.SendMsg(protocol.CMSpell, int(msg.Param), int(msg.Tag), int(msg.Series), body)
	case protocol.CMSay:
		player.SendMsg(protocol.CMSay, 0, 0, 0, body)
	case protocol.CMClickNPC:
		player.SendMsg(protocol.CMClickNPC, int(msg.Recog), 0, 0, "")
	case protocol.CMCreateGroup:
		player.SendMsg(protocol.CMCreateGroup, int(msg.Recog), 0, 0, "")
	case protocol.CMPickup:
		player.SendMsg(protocol.CMPickup, 0, 0, 0, "")
	case protocol.CMTakeOnItem:
		player.SendMsg(protocol.CMTakeOnItem, int(msg.Param), int(msg.Tag), 0, "")
	case protocol.CMTakeOffItem:
		player.SendMsg(protocol.CMTakeOffItem, int(msg.Param), 0, 0, "")
	case protocol.CMEat:
		player.SendMsg(protocol.CMEat, int(msg.Param), 0, 0, "")
	case protocol.CMDealTry:
		player.SendMsg(protocol.CMDealTry, 0, 0, 0, body)
	case protocol.CMDealAddItem:
		player.SendMsg(protocol.CMDealAddItem, int(msg.Param), 0, 0, "")
	case protocol.CMDealDelItem:
		player.SendMsg(protocol.CMDealDelItem, int(msg.Param), 0, 0, "")
	case protocol.CMDealCancel:
		player.SendMsg(protocol.CMDealCancel, 0, 0, 0, "")
	case protocol.CMDealChgGold:
		player.SendMsg(protocol.CMDealChgGold, int(msg.Recog), 0, 0, "")
	case protocol.CMDealEnd:
		player.SendMsg(protocol.CMDealEnd, 0, 0, 0, "")
	case protocol.CMUserStorageItem:
		player.SendMsg(protocol.CMUserStorageItem, int(msg.Param), 0, 0, "")
	case protocol.CMUserTakeBackStorageItem:
		player.SendMsg(protocol.CMUserTakeBackStorageItem, int(msg.Param), 0, 0, "")
	case protocol.CMOpenGuildDlg:
		player.SendMsg(protocol.CMOpenGuildDlg, 0, 0, 0, body)
	case protocol.CMHorseRun:
		player.SendMsg(protocol.CMHorseRun, int(msg.Param), 0, 0, "")
	case protocol.CMOpenDoor:
		player.SendMsg(protocol.CMOpenDoor, 0, 0, 0, "")
	case protocol.CMLoginNoticeOK:
		log.Logf(log.LevelInfo, "Server", "Notice acknowledged by %s", player.Name)
		player.SendLogon(server)
		player.SendBagItemsFull(server)
		player.SendUseItemsFull(server)
		player.SendMyMagicFull(server)
		player.SendDayChanging(server)
		player.SendMapDescription(server)
		player.SendSubAbility(server)
		player.SendRefMsg(RM_TURN, player.Dir, player.CurrX, player.CurrY, player.Name)
	default:
		log.Logf(log.LevelDebug, "Server", "Unhandled game message: %d from %s", msg.Ident, player.Name)
	}
}

// sendCharacterList sends the character list to the client.
// Fix 3: Use text format "*name/job/hair/level/sex/..." instead of binary.
func sendCharacterList(server *netserver.TCPServer, session *netserver.Session, db *storage.Database) {
	log.Logf(log.LevelInfo, "Server", "[sendCharacterList] Loading characters for account %d...", session.CharacterID)
	chars, err := db.GetCharactersByAccount(session.CharacterID)
	if err != nil {
		log.Logf(log.LevelError, "Server", "[sendCharacterList] Failed: %v", err)
		resp := protocol.MakeDefaultMsg(protocol.SMQueryChrFail, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		return
	}

	// Encode as text: "*name1/job1/hair1/level1/sex1/name2/job2/hair2/level2/sex2"
	// '*' prefix marks the last selected character
	var sb strings.Builder
	for i, c := range chars {
		if i > 0 {
			sb.WriteByte('/')
		}
		if i == 0 {
			sb.WriteByte('*') // Mark first as selected
		}
		sb.WriteString(c.Name)
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Job))
		sb.WriteByte('/')
		sb.WriteString("0") // hair
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Level))
		sb.WriteByte('/')
		sb.WriteString(fmt.Sprintf("%d", c.Sex))
	}

	// msg.Param = character count
	resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, int32(len(chars)), 0, 0, 0)
	server.Send(session.ID, resp, protocol.EncodeString(sb.String()))

	log.Logf(log.LevelInfo, "Server", "Sent %d characters to session %d", len(chars), session.ID)
}


// sendLoginFail sends a login failure response.
// Fix 1: Use SMPasswdFail (503) instead of SMQueryChrFail (527).
func sendLoginFail(server *netserver.TCPServer, session *netserver.Session) {
	log.Logf(log.LevelWarn, "Server", "[sendLoginFail] session=%d", session.ID)
	resp := protocol.MakeDefaultMsg(protocol.SMPasswdFail, -1, 0, 0, 0)
	server.Send(session.ID, resp, "")
}

// saveCharacterData saves player data to the database.
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
		log.Logf(log.LevelError, "Server", "Failed to save character %s: %v", player.Name, err)
	} else {
		log.Logf(log.LevelDebug, "Server", "Saved character %s at %s(%d,%d)", player.Name, player.MapName, player.CurrX, player.CurrY)
	}

	savePlayerItems(db, player)

	type charMeta struct {
		PkPoint int           `json:"pkPoint"`
		Magics  []PlayerMagic `json:"magics"`
		Storage []int         `json:"storage"`
	}
	meta := charMeta{
		PkPoint: player.PkPoint,
		Magics:  make([]PlayerMagic, 0, len(player.LearnedMagics)),
	}
	for _, pm := range player.LearnedMagics {
		if pm != nil {
			meta.Magics = append(meta.Magics, *pm)
		}
	}
	for _, item := range player.StorageItems {
		if item != nil {
			meta.Storage = append(meta.Storage, int(item.WIndex))
		}
	}
	metaJSON, err := json.Marshal(meta)
	if err == nil {
		db.SaveCharacterMeta(int64(player.ID), metaJSON)
	}
}

type savedUserItem struct {
	MakeIndex int32  `json:"makeIndex"`
	WIndex    uint16 `json:"wIndex"`
	Dura      uint16 `json:"dura"`
	DuraMax   uint16 `json:"duraMax"`
}

func savePlayerItems(db *storage.Database, player *PlayObject) {
	bag := make([]savedUserItem, 0, len(player.ItemList))
	for _, item := range player.ItemList {
		bag = append(bag, savedUserItem{
			MakeIndex: item.MakeIndex,
			WIndex:    item.WIndex,
			Dura:      item.Dura,
			DuraMax:   item.DuraMax,
		})
	}
	bagJSON, err := json.Marshal(bag)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to marshal bag items for %s: %v", player.Name, err)
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
			}
		}
	}
	equipJSON, err := json.Marshal(equip)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to marshal equip items for %s: %v", player.Name, err)
		return
	}

	if err := db.SaveCharacterItems(int64(player.ID), bagJSON, equipJSON); err != nil {
		log.Logf(log.LevelError, "Server", "Failed to save items for %s: %v", player.Name, err)
	}
}

func loadPlayerItems(db *storage.Database, player *PlayObject) {
	bagJSON, equipJSON, err := db.LoadCharacterItems(int64(player.ID))
	if err != nil {
		log.Logf(log.LevelWarn, "Server", "Failed to load items for %s: %v", player.Name, err)
	}

	if bagJSON == nil && equipJSON == nil {
		player.GiveItem(1)
		player.GiveItem(1)
		player.GiveItem(2)
		log.Logf(log.LevelInfo, "Server", "Gave starting items to %s", player.Name)
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
					}
				}
			}
		}
	}

	player.updateAppearance()
}

// parseCredentials parses "username/password" format.
func parseCredentials(body string) (username, password string) {
	for i, c := range body {
		if c == '/' {
			return body[:i], body[i+1:]
		}
	}
	return body, ""
}

// simpleHash creates a simple hash for development (NOT for production).
func simpleHash(password string) string {
	// Simple XOR-based hash for development
	// In production, use bcrypt or argon2
	hash := make([]byte, len(password))
	for i, c := range password {
		hash[i] = byte(c) ^ 0x5A
	}
	return fmt.Sprintf("%x", hash)
}

// verifyPassword verifies a password against a hash.
func verifyPassword(password, hash string) bool {
	return simpleHash(password) == hash
}

// encodeCharacterInfo encodes character info for network transmission.
func encodeCharacterInfo(c storage.CharacterInfo) []byte {
	buf := make([]byte, 24) // name(20) + job(1) + hair(1) + level(1) + sex(1)
	copy(buf[:20], c.Name)
	buf[20] = byte(c.Job)
	buf[21] = 0 // hair
	buf[22] = byte(c.Level)
	buf[23] = byte(c.Sex)
	return buf
}

// encodeUint16 encodes a uint16 to bytes (little-endian).
func encodeUint16(v uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, v)
	return buf
}
