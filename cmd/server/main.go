package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
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

	sessionMgr := NewSessionManager()
	userEngine := NewUserEngine(db, mapMgr)
	server := netserver.NewTCPServer(listenAddr)

	server.SetConnectHandler(func(session *netserver.Session) {
		sessionMgr.Add(session)
		log.Logf(log.LevelInfo, "Server", "Session %d connected (total: %d)", session.ID, sessionMgr.Count())
	})

	server.SetDisconnectHandler(func(session *netserver.Session) {
		// Clean up PlayObject if player was in game
		if session.State == netserver.StateInGame {
			player := userEngine.GetPlayer(int32(session.CharacterID))
			if player != nil {
				// Save character data before removing
				saveCharacterData(db, player)
				// Remove from map
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

	server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body string) {
		log.Logf(log.LevelDebug, "Server", "Session %d: msg=%d body=%q", session.ID, msg.Ident, body)

		switch session.State {
		case netserver.StateConnected:
			handleConnectedMessage(server, session, msg, body, db)
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

	for {
		select {
		case <-ticker.C:
			// Game tick: process all player actions
			userEngine.ProcessHumans()
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
func handleConnectedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, db *storage.Database) {
	switch msg.Ident {
	case protocol.CMProtocol:
		log.Logf(log.LevelDebug, "Server", "Protocol version: %d", msg.Recog)

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

		// Store account ID in session for later use
		// We'll use CharacterID temporarily to store account ID until character is selected
		session.CharacterID = accountID

		resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		log.Logf(log.LevelInfo, "Server", "Login successful for %s (account=%d)", username, accountID)

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Connected state", msg.Ident)
	}
}

// handleAuthenticatedMessage handles messages in the Authenticated state (character selection).
func handleAuthenticatedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, config *ServerConfig, db *storage.Database, userEngine *UserEngine, mapMgr *MapManager) {
	switch msg.Ident {
	case protocol.CMSelectServer:
		log.Logf(log.LevelInfo, "Server", "Server selected: %s", body)
		resp := protocol.MakeDefaultMsg(protocol.SMSelectServerOK, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMQueryChr:
		log.Logf(log.LevelInfo, "Server", "Query characters for account %d", session.CharacterID)
		sendCharacterList(server, session, db)

	case protocol.CMSelChr:
		charName := body
		log.Logf(log.LevelInfo, "Server", "Select character: %s (id=%d)", charName, msg.Recog)

		// Load character from database
		charID := int64(msg.Recog)
		charData, err := db.GetCharacterByID(charID)
		if err != nil {
			log.Logf(log.LevelError, "Server", "Failed to load character %d: %v", charID, err)
			return
		}

		// Create PlayObject
		player := NewPlayObject(session, charData.Name, int32(charData.ID))
		player.MapName = charData.Map
		player.CurrX = charData.X
		player.CurrY = charData.Y
		player.Job = byte(charData.Job)
		player.Gender = byte(charData.Sex)
		player.WAbil.Level = uint16(charData.Level)
		player.WAbil.HP = uint16(charData.HP)
		player.WAbil.MP = uint16(charData.MP)
		player.WAbil.MaxHP = uint16(charData.HP) // TODO: calculate from stats
		player.WAbil.MaxMP = uint16(charData.MP) // TODO: calculate from stats
		player.WAbil.Exp = uint32(charData.Exp)
		player.SessionID = session.ID
		player.AccountName = session.AccountName

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

		// Add to UserEngine
		userEngine.AddPlayer(player)
		player.ReadyToRun = true

		// Send map info
		player.SendMapInfo(server)
		log.Logf(log.LevelInfo, "Server", "Sent map %s(%d,%d) to player %s", player.MapName, player.CurrX, player.CurrY, player.Name)

		// Send start play
		startResp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
		server.Send(session.ID, startResp, "")

		// Send notice
		noticeResp := protocol.MakeDefaultMsg(protocol.SMSendNotice, 0, 0, 0, 0)
		server.Send(session.ID, noticeResp, "Welcome to MIR2 Go Server!")

		// Send logon
		player.SendLogon(server)

		// Send ability
		player.SendAbility(server)

		// Send bag items (empty for now)
		sendBagItems(server, session)

		log.Logf(log.LevelInfo, "Server", "Player %s entered game at %s(%d,%d)", player.Name, player.MapName, player.CurrX, player.CurrY)

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
	case protocol.CMHit:
		player.SendMsg(protocol.CMHit, int(msg.Param), int(msg.Tag), int(msg.Series), "")
	case protocol.CMSpell:
		player.SendMsg(protocol.CMSpell, int(msg.Param), int(msg.Tag), int(msg.Series), body)
	default:
		log.Logf(log.LevelDebug, "Server", "Unhandled game message: %d from %s", msg.Ident, player.Name)
	}
}

// sendCharacterList sends the character list to the client.
func sendCharacterList(server *netserver.TCPServer, session *netserver.Session, db *storage.Database) {
	chars, err := db.GetCharactersByAccount(session.CharacterID)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to load characters: %v", err)
		resp := protocol.MakeDefaultMsg(protocol.SMQueryChrFail, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		return
	}

	// Encode character list as binary data
	// Format: count(1) + per char: name(20) + job(1) + hair(1) + level(1) + sex(1)
	var buf []byte
	buf = append(buf, byte(len(chars)))
	for _, c := range chars {
		// Name (20 bytes, null-terminated)
		var nameBuf [20]byte
		copy(nameBuf[:], c.Name)
		buf = append(buf, nameBuf[:]...)
		// Job (1 byte)
		buf = append(buf, byte(c.Job))
		// Hair (1 byte) - default 0
		buf = append(buf, 0)
		// Level (1 byte)
		buf = append(buf, byte(c.Level))
		// Sex (1 byte)
		buf = append(buf, byte(c.Sex))
	}

	// Send as SMQueryChr with encoded body
	// msg.Param = character count
	resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, int32(len(chars)), 0, 0, 0)
	encodedBody := protocol.EncodeBuffer(buf)
	server.Send(session.ID, resp, encodedBody)

	log.Logf(log.LevelInfo, "Server", "Sent %d characters to session %d", len(chars), session.ID)
}

// sendBagItems sends the bag items to the client (empty for now).
func sendBagItems(server *netserver.TCPServer, session *netserver.Session) {
	resp := protocol.MakeDefaultMsg(protocol.SMBagItems, 0, 0, 0, 0)
	server.Send(session.ID, resp, "")
}

// sendLoginFail sends a login failure response.
func sendLoginFail(server *netserver.TCPServer, session *netserver.Session) {
	resp := protocol.MakeDefaultMsg(protocol.SMQueryChrFail, 0, 0, 0, 0)
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
	}
	if err := db.UpdateCharacter(c); err != nil {
		log.Logf(log.LevelError, "Server", "Failed to save character %s: %v", player.Name, err)
	} else {
		log.Logf(log.LevelDebug, "Server", "Saved character %s at %s(%d,%d)", player.Name, player.MapName, player.CurrX, player.CurrY)
	}
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
