// Package db provides SQLite-backed storage for agents, provider keys, and usage logs.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	// journal_mode(wal): enables WAL mode for concurrent readers without blocking writers.
	// busy_timeout(5000): waits up to 5 s when the database is locked instead of failing immediately.
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Restrict DB file permissions to owner-only read/write.
	os.Chmod(path, 0600)
	// Allow concurrent readers in WAL mode, but SQLite still serializes writes
	conn.SetMaxOpenConns(4)
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("starting migration tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS provider_keys (
			id TEXT PRIMARY KEY,
			vendor TEXT NOT NULL,
			secret_encrypted BLOB NOT NULL,
			label TEXT,
			created_at INTEGER DEFAULT (unixepoch())
		);

		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			hostname TEXT UNIQUE NOT NULL,
			token_hash TEXT UNIQUE NOT NULL,
			allowed_vendors TEXT,
			created_at INTEGER DEFAULT (unixepoch())
		);

		CREATE TABLE IF NOT EXISTS usage_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_id TEXT,
			vendor TEXT,
			key_id TEXT,
			status_code INTEGER,
			created_at INTEGER DEFAULT (unixepoch())
		);

		CREATE INDEX IF NOT EXISTS idx_usage_agent ON usage_log(agent_id);
		CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_log(created_at);
	`)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return tx.Commit()
}
