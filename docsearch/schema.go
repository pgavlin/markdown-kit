package docsearch

import (
	"database/sql"
	"fmt"
)

const schemaVersion = "1"

// createSchema creates the database tables and triggers if they don't exist.
func createSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS metadata (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS documents (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			path         TEXT UNIQUE NOT NULL,
			title        TEXT NOT NULL DEFAULT '',
			content      TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL,
			last_opened  INTEGER NOT NULL,
			indexed_at   INTEGER NOT NULL
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
			title,
			content,
			content='documents',
			content_rowid='id',
			tokenize='porter unicode61'
		)`,
		// Triggers to keep FTS in sync with the documents table.
		`CREATE TRIGGER IF NOT EXISTS documents_ai AFTER INSERT ON documents BEGIN
			INSERT INTO documents_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS documents_au AFTER UPDATE ON documents BEGIN
			INSERT INTO documents_fts(documents_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
			INSERT INTO documents_fts(rowid, title, content) VALUES (new.id, new.title, new.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS documents_ad AFTER DELETE ON documents BEGIN
			INSERT INTO documents_fts(documents_fts, rowid, title, content) VALUES('delete', old.id, old.title, old.content);
		END`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
	}

	// Set schema version if not present.
	_, err := db.Exec(
		`INSERT OR IGNORE INTO metadata (key, value) VALUES ('schema_version', ?)`,
		schemaVersion,
	)
	if err != nil {
		return fmt.Errorf("setting schema version: %w", err)
	}

	return nil
}

// ensureVecTable creates or recreates the vector table if the dimensions changed.
func ensureVecTable(db *sql.DB, dimensions int) error {
	var current string
	err := db.QueryRow(`SELECT value FROM metadata WHERE key = 'embedding_dimensions'`).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("checking embedding dimensions: %w", err)
	}

	wanted := fmt.Sprintf("%d", dimensions)
	if current == wanted {
		return nil
	}

	// Drop existing vec table if dimensions changed.
	if current != "" {
		if _, err := db.Exec(`DROP TABLE IF EXISTS documents_vec`); err != nil {
			return fmt.Errorf("dropping vec table: %w", err)
		}
	}

	// Create vec table with new dimensions.
	stmt := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS documents_vec USING vec0(document_id INTEGER PRIMARY KEY, embedding float[%d])`,
		dimensions,
	)
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("creating vec table: %w", err)
	}

	// Record the dimensions.
	_, err = db.Exec(
		`INSERT OR REPLACE INTO metadata (key, value) VALUES ('embedding_dimensions', ?)`,
		wanted,
	)
	if err != nil {
		return fmt.Errorf("storing embedding dimensions: %w", err)
	}

	return nil
}
