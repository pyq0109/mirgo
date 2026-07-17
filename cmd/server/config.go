package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServerConfig holds server configuration.
type ServerConfig struct {
	ListenAddr   string `json:"listen_addr"`
	DatabasePath string `json:"database_path"`
	MaxPlayers   int    `json:"max_players"`
	TickRate     int    `json:"tick_rate"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		ListenAddr:   ":7000",
		DatabasePath: "serverdata/mir2.db",
		MaxPlayers:   1000,
		TickRate:     10,
	}
}

// LoadConfig loads configuration from a JSON file.
func LoadConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return config, nil
}

// Save saves the configuration to a JSON file.
func (c *ServerConfig) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
