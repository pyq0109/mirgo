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
// wars/allies 分别为行会战列表与联盟行会列表的 JSON blob。
const guildsSchema = `CREATE TABLE IF NOT EXISTS guilds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			master TEXT NOT NULL,
			notice TEXT NOT NULL DEFAULT '',
			members BLOB,
			wars BLOB,
			allies BLOB,
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
			user_name TEXT NOT NULL DEFAULT '',
			ss_no TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			quiz TEXT NOT NULL DEFAULT '',
			answer TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			quiz2 TEXT NOT NULL DEFAULT '',
			answer2 TEXT NOT NULL DEFAULT '',
			birthday TEXT NOT NULL DEFAULT '',
			mobile_phone TEXT NOT NULL DEFAULT '',
			memo TEXT NOT NULL DEFAULT '',
			memo2 TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS characters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			name TEXT UNIQUE NOT NULL,
			job INTEGER NOT NULL DEFAULT 0,
			sex INTEGER NOT NULL DEFAULT 0,
			hair INTEGER NOT NULL DEFAULT 0,
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
			tax_rate INTEGER NOT NULL DEFAULT 5,
			declarations BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS npc_upgrades (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			npc_id INTEGER NOT NULL,
			player_name TEXT NOT NULL,
			item_data BLOB NOT NULL,
			bt_dc INTEGER NOT NULL DEFAULT 0,
			bt_sc INTEGER NOT NULL DEFAULT 0,
			bt_mc INTEGER NOT NULL DEFAULT 0,
			bt_dura INTEGER NOT NULL DEFAULT 0,
			submitted_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS npc_data (
			npc_key TEXT NOT NULL,
			data_type TEXT NOT NULL,
			json_data BLOB,
			PRIMARY KEY (npc_key, data_type)
		)`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return fmt.Errorf("initialize: %w", err)
		}
	}

	if err := d.migrateAccounts(); err != nil {
		return err
	}
	if err := d.migrateGuilds(); err != nil {
		return err
	}
	if err := d.migrateCastle(); err != nil {
		return err
	}
	return d.migrateCharacters()
}

// migrateCastle 为早期创建的 castle 表补齐 declarations 列（预约攻城列表）。
func (d *Database) migrateCastle() error {
	rows, err := d.db.Query(`PRAGMA table_info(castle)`)
	if err != nil {
		return fmt.Errorf("migrateCastle: %w", err)
	}
	hasDeclarations := false
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
			rows.Close()
			return fmt.Errorf("migrateCastle: %w", err)
		}
		if name == "declarations" {
			hasDeclarations = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("migrateCastle: %w", err)
	}
	rows.Close()
	if hasDeclarations {
		return nil
	}
	if _, err := d.db.Exec(`ALTER TABLE castle ADD COLUMN declarations BLOB`); err != nil {
		return fmt.Errorf("migrateCastle: add declarations column: %w", err)
	}
	return nil
}

// migrateCharacters 为早期创建的 characters 表补齐 hair 列。
func (d *Database) migrateCharacters() error {
	rows, err := d.db.Query(`PRAGMA table_info(characters)`)
	if err != nil {
		return fmt.Errorf("migrateCharacters: %w", err)
	}
	hasHair := false
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
			rows.Close()
			return fmt.Errorf("migrateCharacters: %w", err)
		}
		if name == "hair" {
			hasHair = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrateCharacters: %w", err)
	}
	if hasHair {
		return nil
	}
	if _, err := d.db.Exec(`ALTER TABLE characters ADD COLUMN hair INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("migrateCharacters: add hair: %w", err)
	}
	return nil
}

// migrateAccounts 为早期创建的 accounts 表补齐注册资料列（Delphi
// TUserEntry+TUserEntryAdd，Grobal2.pas:592-609）。新建库已由 initialize
// 建好全部列，此处仅对缺列的旧库执行 ALTER TABLE。
func (d *Database) migrateAccounts() error {
	rows, err := d.db.Query(`PRAGMA table_info(accounts)`)
	if err != nil {
		return fmt.Errorf("migrateAccounts: %w", err)
	}
	have := make(map[string]bool)
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
			rows.Close()
			return fmt.Errorf("migrateAccounts: %w", err)
		}
		have[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrateAccounts: %w", err)
	}

	want := []string{"user_name", "ss_no", "phone", "quiz", "answer", "email",
		"quiz2", "answer2", "birthday", "mobile_phone", "memo", "memo2"}
	for _, col := range want {
		if have[col] {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE accounts ADD COLUMN %s TEXT NOT NULL DEFAULT ''", col)
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrateAccounts: add %s: %w", col, err)
		}
	}
	return nil
}

// migrateGuilds 将早期以 leader_id（角色 ID 外键）定义、从未写入数据的
// guilds 表重建为按行会名存储的新结构。新库无此表时由 initialize 创建，
// 旧结构库补齐 wars/allies 列（行会战与联盟持久化）。
func (d *Database) migrateGuilds() error {
	rows, err := d.db.Query(`PRAGMA table_info(guilds)`)
	if err != nil {
		return fmt.Errorf("migrateGuilds: %w", err)
	}

	hasMaster := false
	hasWars := false
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
			rows.Close()
			return fmt.Errorf("migrateGuilds: %w", err)
		}
		if name == "master" {
			hasMaster = true
		}
		if name == "wars" {
			hasWars = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("migrateGuilds: %w", err)
	}
	rows.Close()

	if !hasMaster {
		if _, err := d.db.Exec(`DROP TABLE guilds`); err != nil {
			return fmt.Errorf("migrateGuilds: drop legacy table: %w", err)
		}
		if _, err := d.db.Exec(guildsSchema); err != nil {
			return fmt.Errorf("migrateGuilds: recreate table: %w", err)
		}
		return nil
	}
	if !hasWars {
		if _, err := d.db.Exec(`ALTER TABLE guilds ADD COLUMN wars BLOB`); err != nil {
			return fmt.Errorf("migrateGuilds: add wars column: %w", err)
		}
		if _, err := d.db.Exec(`ALTER TABLE guilds ADD COLUMN allies BLOB`); err != nil {
			return fmt.Errorf("migrateGuilds: add allies column: %w", err)
		}
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

// AccountInfo 是账号注册资料，对应 Delphi TUserEntry+TUserEntryAdd
//（Grobal2.pas:592-609）中除账号名/密码外的字段。
type AccountInfo struct {
	UserName    string
	SSNo        string
	Phone       string
	Quiz        string
	Answer      string
	Email       string
	Quiz2       string
	Answer2     string
	BirthDay    string
	MobilePhone string
	Memo        string
	Memo2       string
}

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

// CreateAccountWithEntry 创建账号并持久化完整注册资料（Delphi 注册时把整个
// TUserEntry+TUserEntryAdd 写入账号库）。
func (d *Database) CreateAccountWithEntry(username, passwordHash string, info *AccountInfo) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO accounts (username, password_hash, user_name, ss_no, phone, quiz, answer, email,
			quiz2, answer2, birthday, mobile_phone, memo, memo2)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		username, passwordHash,
		info.UserName, info.SSNo, info.Phone, info.Quiz, info.Answer, info.Email,
		info.Quiz2, info.Answer2, info.BirthDay, info.MobilePhone, info.Memo, info.Memo2,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetAccountInfo 返回账号的注册资料。
func (d *Database) GetAccountInfo(username string) (*AccountInfo, error) {
	var info AccountInfo
	err := d.db.QueryRow(
		`SELECT user_name, ss_no, phone, quiz, answer, email, quiz2, answer2, birthday, mobile_phone, memo, memo2
			FROM accounts WHERE username = ?`,
		username,
	).Scan(&info.UserName, &info.SSNo, &info.Phone, &info.Quiz, &info.Answer, &info.Email,
		&info.Quiz2, &info.Answer2, &info.BirthDay, &info.MobilePhone, &info.Memo, &info.Memo2)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// UpdateAccountInfo 整条更新账号的密码与注册资料（Delphi CM_UPDATEUSER 用客户端
// 发来的整个 TUserEntry+TUserEntryAdd 覆盖原记录，LoginSrv/LMain.pas:1449-1451）。
func (d *Database) UpdateAccountInfo(username, passwordHash string, info *AccountInfo) error {
	_, err := d.db.Exec(
		`UPDATE accounts SET password_hash=?, user_name=?, ss_no=?, phone=?, quiz=?, answer=?, email=?,
			quiz2=?, answer2=?, birthday=?, mobile_phone=?, memo=?, memo2=? WHERE username=?`,
		passwordHash, info.UserName, info.SSNo, info.Phone, info.Quiz, info.Answer, info.Email,
		info.Quiz2, info.Answer2, info.BirthDay, info.MobilePhone, info.Memo, info.Memo2, username,
	)
	return err
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
func (d *Database) CreateCharacter(accountID int64, name string, job, sex, hair int) (int64, error) {
	result, err := d.db.Exec(
		"INSERT INTO characters (account_id, name, job, sex, hair) VALUES (?, ?, ?, ?, ?)",
		accountID, name, job, sex, hair,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetCharactersByAccount 返回某个账号下的所有角色。
func (d *Database) GetCharactersByAccount(accountID int64) ([]CharacterInfo, error) {
	rows, err := d.db.Query(
		"SELECT id, name, job, sex, hair, level FROM characters WHERE account_id = ?",
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chars []CharacterInfo
	for rows.Next() {
		var c CharacterInfo
		if err := rows.Scan(&c.ID, &c.Name, &c.Job, &c.Sex, &c.Hair, &c.Level); err != nil {
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
		"SELECT id, account_id, name, job, sex, hair, level, map, x, y, hp, mp, exp, gold FROM characters WHERE id = ?",
		id,
	).Scan(&c.ID, &c.AccountID, &c.Name, &c.Job, &c.Sex, &c.Hair, &c.Level, &c.Map, &c.X, &c.Y, &c.HP, &c.MP, &c.Exp, &c.Gold)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CharacterNameExists 检查角色名是否已被全局占用（跨所有账号）。
// Delphi 创建角色时做全局重名检查（DBServer/UsrSoc.pas:794-796）。
func (d *Database) CharacterNameExists(name string) (bool, error) {
	var one int
	err := d.db.QueryRow(
		"SELECT 1 FROM characters WHERE name = ? LIMIT 1",
		name,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetCharacterByName 在某个账号下按名字返回一个角色。
func (d *Database) GetCharacterByName(accountID int64, name string) (*Character, error) {
	var c Character
	err := d.db.QueryRow(
		"SELECT id, account_id, name, job, sex, hair, level, map, x, y, hp, mp, exp, gold FROM characters WHERE account_id = ? AND name = ?",
		accountID, name,
	).Scan(&c.ID, &c.AccountID, &c.Name, &c.Job, &c.Sex, &c.Hair, &c.Level, &c.Map, &c.X, &c.Y, &c.HP, &c.MP, &c.Exp, &c.Gold)
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
// Wars/Allies 为 JSON blob（行会战列表/联盟行会名列表），可为空。
type GuildRecord struct {
	Name    string
	Master  string
	Notice  string
	Members []GuildMember
	Wars    []byte
	Allies  []byte
}

// SaveGuild 按行会名 upsert 一条行会记录，成员列表序列化为 JSON。
func (d *Database) SaveGuild(name, master, notice string, members []GuildMember, wars, allies []byte) error {
	membersJSON, err := json.Marshal(members)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(
		`INSERT OR REPLACE INTO guilds (name, master, notice, members, wars, allies) VALUES (?, ?, ?, ?, ?, ?)`,
		name, master, notice, membersJSON, wars, allies,
	)
	return err
}

// LoadGuilds 返回数据库中全部行会。
func (d *Database) LoadGuilds() ([]GuildRecord, error) {
	rows, err := d.db.Query(`SELECT name, master, notice, members, wars, allies FROM guilds`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guilds []GuildRecord
	for rows.Next() {
		var g GuildRecord
		var membersJSON []byte
		if err := rows.Scan(&g.Name, &g.Master, &g.Notice, &membersJSON, &g.Wars, &g.Allies); err != nil {
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
	Hair  int
	Level int
}

// Character 是完整的角色数据。
type Character struct {
	ID        int64
	AccountID int64
	Name      string
	Job       int
	Sex       int
	Hair      int
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
// Declarations 为预约攻城列表 JSON blob（行会名+日期），可为空。
type CastleRecord struct {
	OwnerGuild   string
	Gold         int64
	DoorHP       int
	WallHP       int
	WarState     int
	TaxRate      int
	Declarations []byte
}

// SaveCastle 保存城堡状态（单行表，id=1）。
func (d *Database) SaveCastle(r CastleRecord) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO castle (id, owner_guild, gold, door_hp, wall_hp, war_state, tax_rate, declarations) VALUES (1, ?, ?, ?, ?, ?, ?, ?)`,
		r.OwnerGuild, r.Gold, r.DoorHP, r.WallHP, r.WarState, r.TaxRate, r.Declarations,
	)
	return err
}

// LoadCastle 加载城堡状态。无数据时返回零值。
func (d *Database) LoadCastle() (CastleRecord, error) {
	var r CastleRecord
	err := d.db.QueryRow(`SELECT owner_guild, gold, door_hp, wall_hp, war_state, tax_rate, declarations FROM castle WHERE id=1`).
		Scan(&r.OwnerGuild, &r.Gold, &r.DoorHP, &r.WallHP, &r.WarState, &r.TaxRate, &r.Declarations)
	if err == sql.ErrNoRows {
		return CastleRecord{}, nil
	}
	return r, err
}

// UpgradeRecord 是NPC武器升级队列的持久化记录。
type UpgradeRecord struct {
	ID          int64
	NpcID       int32
	PlayerName  string
	ItemData    []byte // JSON序列化的savedUserItem
	BtDc        byte
	BtSc        byte
	BtMc        byte
	BtDura      byte
	SubmittedAt int64
}

// SaveNpcUpgrade 保存一条武器升级记录，返回数据库记录ID。
func (d *Database) SaveNpcUpgrade(npcID int32, playerName string, itemData []byte, btDc, btSc, btMc, btDura byte, submittedAt int64) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO npc_upgrades (npc_id, player_name, item_data, bt_dc, bt_sc, bt_mc, bt_dura, submitted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		npcID, playerName, itemData, btDc, btSc, btMc, btDura, submittedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteNpcUpgrade 删除一条武器升级记录。
func (d *Database) DeleteNpcUpgrade(id int64) error {
	_, err := d.db.Exec(`DELETE FROM npc_upgrades WHERE id = ?`, id)
	return err
}

// LoadAllNpcUpgrades 加载所有武器升级记录，按NPC ID分组。
func (d *Database) LoadAllNpcUpgrades() (map[int32][]UpgradeRecord, error) {
	rows, err := d.db.Query(`SELECT id, npc_id, player_name, item_data, bt_dc, bt_sc, bt_mc, bt_dura, submitted_at FROM npc_upgrades`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int32][]UpgradeRecord)
	for rows.Next() {
		var r UpgradeRecord
		if err := rows.Scan(&r.ID, &r.NpcID, &r.PlayerName, &r.ItemData, &r.BtDc, &r.BtSc, &r.BtMc, &r.BtDura, &r.SubmittedAt); err != nil {
			return nil, err
		}
		result[r.NpcID] = append(result[r.NpcID], r)
	}
	return result, rows.Err()
}

// SaveNpcData 保存 NPC 数据（商品库存/价格），以 JSON blob 存储。
func (d *Database) SaveNpcData(npcKey, dataType string, jsonData []byte) error {
	_, err := d.db.Exec(`INSERT OR REPLACE INTO npc_data (npc_key, data_type, json_data) VALUES (?, ?, ?)`,
		npcKey, dataType, jsonData)
	return err
}

// LoadNpcData 加载 NPC 数据。
func (d *Database) LoadNpcData(npcKey, dataType string) ([]byte, error) {
	var data []byte
	err := d.db.QueryRow(`SELECT json_data FROM npc_data WHERE npc_key = ? AND data_type = ?`,
		npcKey, dataType).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return data, err
}
