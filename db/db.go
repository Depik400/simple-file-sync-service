package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type FileRecord struct {
	ID         int       `json:"id"`
	Path       string    `json:"path"`
	Hash       string    `json:"hash"`
	Size       int64     `json:"size"`
	Modified   time.Time `json:"modified"`
	ServerName string    `json:"server_name"`
	Action     string    `json:"action"` // "created", "modified", "deleted"
	SyncTime   time.Time `json:"sync_time"`
}

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &Database{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS file_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL,
		hash TEXT NOT NULL,
		size INTEGER NOT NULL,
		modified DATETIME NOT NULL,
		server_name TEXT NOT NULL,
		action TEXT NOT NULL,
		sync_time DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_path ON file_history(path);
	CREATE INDEX IF NOT EXISTS idx_server ON file_history(server_name);
	CREATE INDEX IF NOT EXISTS idx_sync_time ON file_history(sync_time);
	`

	_, err := db.Exec(schema)
	return err
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) RecordFileChange(path, hash string, size int64, modified time.Time, serverName, action string) error {
	_, err := d.db.Exec(
		"INSERT INTO file_history (path, hash, size, modified, server_name, action) VALUES (?, ?, ?, ?, ?, ?)",
		path, hash, size, modified, serverName, action,
	)
	return err
}

func (d *Database) GetFileHistory(path string, limit int) ([]FileRecord, error) {
	rows, err := d.db.Query(
		"SELECT id, path, hash, size, modified, server_name, action, sync_time FROM file_history WHERE path = ? ORDER BY sync_time DESC LIMIT ?",
		path, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var record FileRecord
		err := rows.Scan(&record.ID, &record.Path, &record.Hash, &record.Size, &record.Modified, &record.ServerName, &record.Action, &record.SyncTime)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

func (d *Database) GetRecentChanges(serverName string, limit int) ([]FileRecord, error) {
	rows, err := d.db.Query(
		"SELECT id, path, hash, size, modified, server_name, action, sync_time FROM file_history WHERE server_name = ? ORDER BY sync_time DESC LIMIT ?",
		serverName, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var record FileRecord
		err := rows.Scan(&record.ID, &record.Path, &record.Hash, &record.Size, &record.Modified, &record.ServerName, &record.Action, &record.SyncTime)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, rows.Err()
}

func (d *Database) GetAllServers() ([]string, error) {
	rows, err := d.db.Query("SELECT DISTINCT server_name FROM file_history")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []string
	for rows.Next() {
		var server string
		if err := rows.Scan(&server); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}

	return servers, rows.Err()
}
