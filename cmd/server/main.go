package main

import (
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
	server := netserver.NewTCPServer(listenAddr)

	server.SetConnectHandler(func(session *netserver.Session) {
		sessionMgr.Add(session)
		log.Logf(log.LevelInfo, "Server", "Session %d connected (total: %d)", session.ID, sessionMgr.Count())
	})

	server.SetDisconnectHandler(func(session *netserver.Session) {
		sessionMgr.Remove(session.ID)
		log.Logf(log.LevelInfo, "Server", "Session %d disconnected (total: %d)", session.ID, sessionMgr.Count())
	})

	server.SetMessageHandler(func(session *netserver.Session, msg protocol.DefaultMessage, body string) {
		log.Logf(log.LevelDebug, "Server", "Session %d: msg=%d body=%q", session.ID, msg.Ident, body)

		switch session.State {
		case netserver.StateConnected:
			handleConnectedMessage(server, session, msg, body)
		case netserver.StateAuthenticated:
			handleAuthenticatedMessage(server, session, msg, body, config)
		case netserver.StateInGame:
			handleGameMessage(server, session, msg, body, mapMgr)
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
			// Game tick
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

func handleConnectedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string) {
	switch msg.Ident {
	case protocol.CMIDPassword:
		log.Logf(log.LevelInfo, "Server", "Login attempt: %s", body)
		session.State = netserver.StateAuthenticated
		session.AccountName = body

		resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		log.Logf(log.LevelInfo, "Server", "Login successful for session %d", session.ID)

	case protocol.CMProtocol:
		log.Logf(log.LevelDebug, "Server", "Protocol version: %d", msg.Recog)

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Connected state", msg.Ident)
	}
}

func handleAuthenticatedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, config *ServerConfig) {
	switch msg.Ident {
	case protocol.CMQueryChr:
		log.Logf(log.LevelInfo, "Server", "Query characters for session %d", session.ID)
		resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMSelChr:
		log.Logf(log.LevelInfo, "Server", "Select character: %s", body)
		session.State = netserver.StateInGame
		session.CharacterID = int64(msg.Recog)

		// Use home map from config
		mapName := config.GetHomeMap()
		x := int32(config.GetHomeX())
		y := uint16(config.GetHomeY())

		mapResp := protocol.MakeDefaultMsg(protocol.SMNewMap, x, y, 0, 0)
		server.Send(session.ID, mapResp, mapName)
		log.Logf(log.LevelInfo, "Server", "Sent map %s(%d,%d) to session %d", mapName, x, y, session.ID)

		startResp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
		server.Send(session.ID, startResp, "")

		logonResp := protocol.MakeDefaultMsg(protocol.SMLogon, 0, 0, 0, 0)
		server.Send(session.ID, logonResp, "")

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Authenticated state", msg.Ident)
	}
}

func handleGameMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string, mapMgr *MapManager) {
	switch msg.Ident {
	case protocol.CMTurn:
		log.Logf(log.LevelDebug, "Server", "Player turn: dir=%d", msg.Param)
	case protocol.CMWalk:
		log.Logf(log.LevelDebug, "Server", "Player walk: dir=%d", msg.Param)
	case protocol.CMRun:
		log.Logf(log.LevelDebug, "Server", "Player run: dir=%d", msg.Param)
	default:
		log.Logf(log.LevelDebug, "Server", "Game message: %d", msg.Ident)
	}
}
