package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
)

type DB struct {
	conn *sql.DB
}

func New() (*DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %w", err)
	}

	dir := filepath.Join(home, ".relayai")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	dbPath := filepath.Join(dir, "relayai.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.init(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) init() error {
	// 启用 WAL 模式
	if _, err := db.conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("设置 WAL 模式失败: %w", err)
	}

	// 创建表
	if err := db.createTables(); err != nil {
		return err
	}
	return db.migrateTables()
}

func (db *DB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			default_model TEXT,
			model_mappings TEXT,
			cli_types TEXT,
			enabled INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			usage_updated_at INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			time INTEGER NOT NULL,
			method TEXT,
			path TEXT,
			cli_type TEXT,
			provider_id TEXT,
			provider TEXT,
			model TEXT,
			status_code INTEGER,
			duration_ms INTEGER,
			error TEXT,
			response_body TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS provider_usage_points (
			provider_id TEXT NOT NULL,
			bucket_start INTEGER NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0,
			PRIMARY KEY (provider_id, bucket_start)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_time ON request_logs(time)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_provider ON request_logs(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_usage_points_provider_time ON provider_usage_points(provider_id, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("创建表失败: %w", err)
		}
	}

	return nil
}

func (db *DB) migrateTables() error {
	columns := map[string]string{
		"providers.prompt_tokens":     "ALTER TABLE providers ADD COLUMN prompt_tokens INTEGER DEFAULT 0",
		"providers.completion_tokens": "ALTER TABLE providers ADD COLUMN completion_tokens INTEGER DEFAULT 0",
		"providers.total_tokens":      "ALTER TABLE providers ADD COLUMN total_tokens INTEGER DEFAULT 0",
		"providers.usage_updated_at":  "ALTER TABLE providers ADD COLUMN usage_updated_at INTEGER DEFAULT 0",
		"request_logs.provider_id":       "ALTER TABLE request_logs ADD COLUMN provider_id TEXT",
		"request_logs.upstream_url":      "ALTER TABLE request_logs ADD COLUMN upstream_url TEXT",
		"providers.auth_token":           "ALTER TABLE providers ADD COLUMN auth_token TEXT",
		"providers.chat_compat_mode":     "ALTER TABLE providers ADD COLUMN chat_compat_mode INTEGER DEFAULT 0",
		"request_logs.prompt_tokens":     "ALTER TABLE request_logs ADD COLUMN prompt_tokens INTEGER DEFAULT 0",
		"request_logs.completion_tokens": "ALTER TABLE request_logs ADD COLUMN completion_tokens INTEGER DEFAULT 0",
		"request_logs.total_tokens":      "ALTER TABLE request_logs ADD COLUMN total_tokens INTEGER DEFAULT 0",
	}

	for key, statement := range columns {
		table, column := splitTableColumn(key)
		exists, err := db.columnExists(table, column)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := db.conn.Exec(statement); err != nil {
				return fmt.Errorf("迁移字段失败 %s.%s: %w", table, column, err)
			}
		}
	}
	return nil
}

func splitTableColumn(key string) (string, string) {
	for i, r := range key {
		if r == '.' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

func (db *DB) columnExists(table, column string) (bool, error) {
	rows, err := db.conn.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("查询表结构失败: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) Close() error {
	return db.conn.Close()
}
