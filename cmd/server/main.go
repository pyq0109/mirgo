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
	configPath := flag.String("config", "", "Path to config file")
	addr := flag.String("addr", ":7000", "Listen address")
	flag.Parse()

	// Load config
	config := DefaultConfig()
	if *configPath != "" {
		var err error
		config, err = LoadConfig(*configPath)
		if err != nil {
			log.Logf(log.LevelError, "Server", "Failed to load config: %v", err)
			os.Exit(1)
		}
	}
	if *addr != ":7000" {
		config.ListenAddr = *addr
	}

	log.Logf(log.LevelInfo, "Server", "Starting MIR2 Server...")
	log.Logf(log.LevelInfo, "Server", "Listen: %s", config.ListenAddr)
	log.Logf(log.LevelInfo, "Server", "Database: %s", config.DatabasePath)

	// Open database
	db, err := storage.Open(config.DatabasePath)
	if err != nil {
		log.Logf(log.LevelError, "Server", "Failed to open database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Logf(log.LevelInfo, "Server", "Database opened")

	// Create session manager
	sessionMgr := NewSessionManager()

	// Create TCP server
	server := netserver.NewTCPServer(config.ListenAddr)

	// Set handlers
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
			handleAuthenticatedMessage(server, session, msg, body)
		case netserver.StateInGame:
			// TODO: Forward to game logic
			log.Logf(log.LevelDebug, "Server", "Game message: %d", msg.Ident)
		}
	})

	// Start server
	if err := server.Start(); err != nil {
		log.Logf(log.LevelError, "Server", "Failed to start server: %v", err)
		os.Exit(1)
	}

	// Main tick loop
	ticker := time.NewTicker(time.Second / time.Duration(config.TickRate))
	defer ticker.Stop()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Logf(log.LevelInfo, "Server", "Server started. Press Ctrl+C to stop.")

	for {
		select {
		case <-ticker.C:
			// Game tick - will be expanded in Phase 5B
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
		// TODO: Validate credentials against database
		log.Logf(log.LevelInfo, "Server", "Login attempt: %s", body)
		session.State = netserver.StateAuthenticated
		session.AccountName = body // TODO: Parse account/password

		// Send success response
		resp := protocol.MakeDefaultMsg(protocol.SMPassOKSelectServer, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")
		log.Logf(log.LevelInfo, "Server", "Login successful for session %d", session.ID)

	case protocol.CMProtocol:
		// Protocol version check
		log.Logf(log.LevelDebug, "Server", "Protocol version: %d", msg.Recog)

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Connected state", msg.Ident)
	}
}

func handleAuthenticatedMessage(server *netserver.TCPServer, session *netserver.Session, msg protocol.DefaultMessage, body string) {
	switch msg.Ident {
	case protocol.CMQueryChr:
		// TODO: Load characters from database
		log.Logf(log.LevelInfo, "Server", "Query characters for session %d", session.ID)
		resp := protocol.MakeDefaultMsg(protocol.SMQueryChr, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	case protocol.CMSelChr:
		// TODO: Load selected character
		log.Logf(log.LevelInfo, "Server", "Select character: %s", body)
		session.State = netserver.StateInGame
		session.CharacterID = int64(msg.Recog)

		// Send game start
		resp := protocol.MakeDefaultMsg(protocol.SMStartPlay, 0, 0, 0, 0)
		server.Send(session.ID, resp, "")

	default:
		log.Logf(log.LevelWarn, "Server", "Unexpected message %d in Authenticated state", msg.Ident)
	}
}
