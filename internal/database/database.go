package database

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", q, err)
		}
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS lists (id TEXT PRIMARY KEY, name TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS todos (id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT, list_id TEXT NOT NULL REFERENCES lists(id), priority TEXT NOT NULL CHECK(priority IN ('low','medium','high')), due_at TEXT, parent_todo_id TEXT REFERENCES todos(id), completed INTEGER NOT NULL DEFAULT 0, version INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS tasks (task_id TEXT PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL, status_message TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, expires_at TEXT NOT NULL, cancel_requested INTEGER NOT NULL DEFAULT 0, result_json TEXT, error_json TEXT);
 CREATE TABLE IF NOT EXISTS import_jobs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(task_id), items_json TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'queued', claimed_at TEXT);
 `)
	return err
}
