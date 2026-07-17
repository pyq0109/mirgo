// Package storage provides SQLite database access for the MIR2 game server.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Database wraps sql.DB with game-specific operations.
type Database struct {
	db *sql.DB
}

// Open opens or creates a SQLite database at the given path.
func Open(path string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	d := &Database{db: db}
	if err := d.initialize(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func (d *Database) initialize() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS characters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			name TEXT UNIQUE NOT NULL,
			job INTEGER NOT NULL DEFAULT 0,
			sex INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			map TEXT NOT NULL DEFAULT '0',
			x INTEGER NOT NULL DEFAULT 289,
			y INTEGER NOT NULL DEFAULT 618,
			hp INTEGER NOT NULL DEFAULT 100,
			mp INTEGER NOT NULL DEFAULT 100,
			exp INTEGER NOT NULL DEFAULT 0,
			gold INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (account_id) REFERENCES accounts(id)
		)`,
		`CREATE TABLE IF NOT EXISTS guilds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			leader_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (leader_id) REFERENCES characters(id)
		)`,
		`CREATE TABLE IF NOT EXISTS character_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			slot_type INTEGER NOT NULL,
			slot_index INTEGER NOT NULL,
			item_data BLOB,
			UNIQUE(character_id, slot_type, slot_index),
			FOREIGN KEY (character_id) REFERENCES characters(id)
		)`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return fmt.Errorf("initialize: %w", err)
		}
	}

	return nil
}

// Close closes the database.
func (d *Database) Close() error {
	return d.db.Close()
}

// DB returns the underlying sql.DB for advanced operations.
func (d *Database) DB() *sql.DB {
	return d.db
}

// Account operations

// CreateAccount creates a new account.
func (d *Database) CreateAccount(username, passwordHash string) (int64, error) {
	result, err := d.db.Exec(
		"INSERT INTO accounts (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetAccountByUsername returns an account by username.
func (d *Database) GetAccountByUsername(username string) (id int64, passwordHash string, err error) {
	err = d.db.QueryRow(
		"SELECT id, password_hash FROM accounts WHERE username = ?",
		username,
	).Scan(&id, &passwordHash)
	return
}

// Character operations

// CreateCharacter creates a new character.
func (d *Database) CreateCharacter(accountID int64, name string, job, sex int) (int64, error) {
	result, err := d.db.Exec(
		"INSERT INTO characters (account_id, name, job, sex) VALUES (?, ?, ?, ?)",
		accountID, name, job, sex,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetCharactersByAccount returns all characters for an account.
func (d *Database) GetCharactersByAccount(accountID int64) ([]CharacterInfo, error) {
	rows, err := d.db.Query(
		"SELECT id, name, job, sex, level FROM characters WHERE account_id = ?",
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []CharacterInfo
	for rows.Next() {
		var c CharacterInfo
		if err := rows.Scan(&c.ID, &c.Name, &c.Job, &c.Sex, &c.Level); err != nil {
			return nil, err
		}
		chars = append(chars, c)
	}
	return chars, rows.Err()
}

// GetCharacterByID returns a character by ID.
func (d *Database) GetCharacterByID(id int64) (*Character, error) {
	var c Character
	err := d.db.QueryRow(
		"SELECT id, account_id, name, job, sex, level, map, x, y, hp, mp, exp, gold FROM characters WHERE id = ?",
		id,
	).Scan(&c.ID, &c.AccountID, &c.Name, &c.Job, &c.Sex, &c.Level, &c.Map, &c.X, &c.Y, &c.HP, &c.MP, &c.Exp, &c.Gold)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCharacter updates character data.
func (d *Database) UpdateCharacter(c *Character) error {
	_, err := d.db.Exec(
		"UPDATE characters SET level=?, map=?, x=?, y=?, hp=?, mp=?, exp=?, gold=? WHERE id=?",
		c.Level, c.Map, c.X, c.Y, c.HP, c.MP, c.Exp, c.Gold, c.ID,
	)
	return err
}

// DeleteCharacter deletes a character by ID.
func (d *Database) DeleteCharacter(id int64) error {
	_, err := d.db.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

// CharacterInfo is a summary of character data.
type CharacterInfo struct {
	ID    int64
	Name  string
	Job   int
	Sex   int
	Level int
}

// Character is the full character data.
type Character struct {
	ID        int64
	AccountID int64
	Name      string
	Job       int
	Sex       int
	Level     int
	Map       string
	X         int
	Y         int
	HP        int
	MP        int
	Exp       int64
	Gold      int64
}
