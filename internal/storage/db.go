package storage

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"zbor/internal/storage/sqlc"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// DB はデータベース接続を保持する
type DB struct {
	*sql.DB
	Queries *sqlc.Queries
}

// Open はデータベースに接続し、スキーマを初期化する
func Open(path string) (*DB, error) {
	// ディレクトリが存在しない場合は作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// SQLite接続
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 接続確認
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// SQLite設定
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	// スキーマ初期化
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &DB{DB: db, Queries: sqlc.New(db)}, nil
}

// initSchema はスキーマを初期化する
func initSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

// Close はデータベース接続を閉じる
func (db *DB) Close() error {
	return db.DB.Close()
}
