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
	return db.createTables()
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
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			time INTEGER NOT NULL,
			method TEXT,
			path TEXT,
			cli_type TEXT,
			provider TEXT,
			model TEXT,
			status_code INTEGER,
			duration_ms INTEGER,
			error TEXT,
			response_body TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_time ON request_logs(time)`,
		`CREATE INDEX IF NOT EXISTS idx_providers_enabled ON providers(enabled)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("创建表失败: %w", err)
		}
	}

	return nil
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) Close() error {
	return db.conn.Close()
}
