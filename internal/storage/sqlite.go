// Package storage 为 MIR2 游戏服务端提供 SQLite 数据库访问。
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// guildsSchema 以行会名为主键存储行会数据，成员列表序列化为 JSON blob
//（成员名+职位合并存储，对应内存中的 Members + Ranks）。
const guildsSchema = `CREATE TABLE IF NOT EXISTS guilds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			master TEXT NOT NULL,
			notice TEXT NOT NULL DEFAULT '',
			members BLOB,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`

// Database 在 sql.DB 之上封装了游戏相关的操作。
type Database struct {
	db *sql.DB
}

// Open 在给定路径打开或创建一个 SQLite 数据库。
func Open(path string) (*Database, error) {
	// 确保目录存在
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
		guildsSchema,
		`CREATE TABLE IF NOT EXISTS character_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			slot_type INTEGER NOT NULL,
			slot_index INTEGER NOT NULL,
			item_data BLOB,
			UNIQUE(character_id, slot_type, slot_index),
			FOREIGN KEY (character_id) REFERENCES characters(id)
		)`,
		`CREATE TABLE IF NOT EXISTS castle (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			owner_guild TEXT NOT NULL DEFAULT '',
			gold INTEGER NOT NULL DEFAULT 0,
			door_hp INTEGER NOT NULL DEFAULT 5000,
			wall_hp INTEGER NOT NULL DEFAULT 10000,
			war_state INTEGER NOT NULL DEFAULT 0,
			tax_rate INTEGER NOT NULL DEFAULT 5
		)`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return fmt.Errorf("initialize: %w", err)
		}
	}

	return d.migrateGuilds()
}

// migrateGuilds 将早期以 leader_id（角色 ID 外键）定义、从未写入数据的
// guilds 表重建为按行会名存储的新结构。新库无此表时由 initialize 创建，
// 此处不做任何操作。
func (d *Database) migrateGuilds() error {
	rows, err := d.db.Query(`PRAGMA table_info(guilds)`)
	if err != nil {
		return fmt.Errorf("migrateGuilds: %w", err)
	}
	defer rows.Close()

	hasMaster := false
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("migrateGuilds: %w", err)
		}
		if name == "master" {
			hasMaster = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrateGuilds: %w", err)
	}
	if hasMaster {
		return nil
	}

	if _, err := d.db.Exec(`DROP TABLE guilds`); err != nil {
		return fmt.Errorf("migrateGuilds: drop legacy table: %w", err)
	}
	if _, err := d.db.Exec(guildsSchema); err != nil {
		return fmt.Errorf("migrateGuilds: recreate table: %w", err)
	}
	return nil
}

// Close 关闭数据库。
func (d *Database) Close() error {
	return d.db.Close()
}

// DB 返回底层的 sql.DB 以便进行高级操作。
func (d *Database) DB() *sql.DB {
	return d.db
}

// 账号操作

// CreateAccount 创建一个新账号。
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

// GetAccountByUsername 按用户名返回一个账号。
func (d *Database) GetAccountByUsername(username string) (id int64, passwordHash string, err error) {
	err = d.db.QueryRow(
		"SELECT id, password_hash FROM accounts WHERE username = ?",
		username,
	).Scan(&id, &passwordHash)
	return
}

// 角色操作

// UpdateAccountPassword 修改账号的密码哈希。
func (d *Database) UpdateAccountPassword(accountID int64, passwordHash string) error {
	_, err := d.db.Exec(
		"UPDATE accounts SET password_hash = ? WHERE id = ?",
		passwordHash, accountID,
	)
	return err
}

// CreateCharacter 创建一个新角色。
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

// GetCharactersByAccount 返回某个账号下的所有角色。
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

// GetCharacterByID 按 ID 返回一个角色。
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

// GetCharacterByName 在某个账号下按名字返回一个角色。
func (d *Database) GetCharacterByName(accountID int64, name string) (*Character, error) {
	var c Character
	err := d.db.QueryRow(
		"SELECT id, account_id, name, job, sex, level, map, x, y, hp, mp, exp, gold FROM characters WHERE account_id = ? AND name = ?",
		accountID, name,
	).Scan(&c.ID, &c.AccountID, &c.Name, &c.Job, &c.Sex, &c.Level, &c.Map, &c.X, &c.Y, &c.HP, &c.MP, &c.Exp, &c.Gold)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCharacter 更新角色数据。
func (d *Database) UpdateCharacter(c *Character) error {
	_, err := d.db.Exec(
		"UPDATE characters SET level=?, map=?, x=?, y=?, hp=?, mp=?, exp=?, gold=? WHERE id=?",
		c.Level, c.Map, c.X, c.Y, c.HP, c.MP, c.Exp, c.Gold, c.ID,
	)
	return err
}

// DeleteCharacter 按 ID 删除一个角色。
func (d *Database) DeleteCharacter(id int64) error {
	_, err := d.db.Exec("DELETE FROM characters WHERE id = ?", id)
	return err
}

func (d *Database) SaveCharacterItems(charID int64, bagJSON, equipJSON []byte) error {
	_, err := d.db.Exec(`INSERT OR REPLACE INTO character_items (character_id, slot_type, slot_index, item_data) VALUES (?, 0, 0, ?)`, charID, bagJSON)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`INSERT OR REPLACE INTO character_items (character_id, slot_type, slot_index, item_data) VALUES (?, 1, 0, ?)`, charID, equipJSON)
	return err
}

func (d *Database) LoadCharacterItems(charID int64) (bagJSON, equipJSON []byte, err error) {
	err = d.db.QueryRow(`SELECT item_data FROM character_items WHERE character_id=? AND slot_type=0`, charID).Scan(&bagJSON)
	if err != nil {
		return nil, nil, nil
	}
	d.db.QueryRow(`SELECT item_data FROM character_items WHERE character_id=? AND slot_type=1`, charID).Scan(&equipJSON)
	return bagJSON, equipJSON, nil
}

func (d *Database) SaveCharacterMeta(charID int64, metaJSON []byte) error {
	_, err := d.db.Exec(`INSERT OR REPLACE INTO character_items (character_id, slot_type, slot_index, item_data) VALUES (?, 2, 0, ?)`, charID, metaJSON)
	return err
}

func (d *Database) LoadCharacterMeta(charID int64) ([]byte, error) {
	var data []byte
	err := d.db.QueryRow(`SELECT item_data FROM character_items WHERE character_id=? AND slot_type=2`, charID).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// 行会操作

// GuildMember 是行会成员及其职位。
type GuildMember struct {
	Name string `json:"name"`
	Rank string `json:"rank"`
}

// GuildRecord 是从数据库加载的行会数据。
type GuildRecord struct {
	Name    string
	Master  string
	Notice  string
	Members []GuildMember
}

// SaveGuild 按行会名 upsert 一条行会记录，成员列表序列化为 JSON。
func (d *Database) SaveGuild(name, master, notice string, members []GuildMember) error {
	membersJSON, err := json.Marshal(members)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(
		`INSERT OR REPLACE INTO guilds (name, master, notice, members) VALUES (?, ?, ?, ?)`,
		name, master, notice, membersJSON,
	)
	return err
}

// LoadGuilds 返回数据库中全部行会。
func (d *Database) LoadGuilds() ([]GuildRecord, error) {
	rows, err := d.db.Query(`SELECT name, master, notice, members FROM guilds`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guilds []GuildRecord
	for rows.Next() {
		var g GuildRecord
		var membersJSON []byte
		if err := rows.Scan(&g.Name, &g.Master, &g.Notice, &membersJSON); err != nil {
			return nil, err
		}
		if len(membersJSON) > 0 {
			if err := json.Unmarshal(membersJSON, &g.Members); err != nil {
				return nil, err
			}
		}
		guilds = append(guilds, g)
	}
	return guilds, rows.Err()
}

// DeleteGuild 按名字删除一个行会。
func (d *Database) DeleteGuild(name string) error {
	_, err := d.db.Exec(`DELETE FROM guilds WHERE name = ?`, name)
	return err
}

// CharacterInfo 是角色数据的摘要。
type CharacterInfo struct {
	ID    int64
	Name  string
	Job   int
	Sex   int
	Level int
}

// Character 是完整的角色数据。
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

// CastleRecord 是城堡持久化数据。
type CastleRecord struct {
	OwnerGuild string
	Gold       int64
	DoorHP     int
	WallHP     int
	WarState   int
	TaxRate    int
}

// SaveCastle 保存城堡状态（单行表，id=1）。
func (d *Database) SaveCastle(r CastleRecord) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO castle (id, owner_guild, gold, door_hp, wall_hp, war_state, tax_rate) VALUES (1, ?, ?, ?, ?, ?, ?)`,
		r.OwnerGuild, r.Gold, r.DoorHP, r.WallHP, r.WarState, r.TaxRate,
	)
	return err
}

// LoadCastle 加载城堡状态。无数据时返回零值。
func (d *Database) LoadCastle() (CastleRecord, error) {
	var r CastleRecord
	err := d.db.QueryRow(`SELECT owner_guild, gold, door_hp, wall_hp, war_state, tax_rate FROM castle WHERE id=1`).
		Scan(&r.OwnerGuild, &r.Gold, &r.DoorHP, &r.WallHP, &r.WarState, &r.TaxRate)
	if err == sql.ErrNoRows {
		return CastleRecord{}, nil
	}
	return r, err
}
