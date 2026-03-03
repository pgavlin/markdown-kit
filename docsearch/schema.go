package docsearch

import (
	"database/sql"
	"fmt"
)

const schemaVersion = "2"

// createSchema creates the database tables and triggers if they don't exist,
// and runs any necessary migrations.
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
		// Chunks table for chunked embeddings.
		`CREATE TABLE IF NOT EXISTS chunks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			chunk_index INTEGER NOT NULL,
			heading     TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS chunks_document_id ON chunks(document_id)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
	}

	// Check current schema version and migrate if needed.
	var currentVersion string
	err := db.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		currentVersion = ""
	} else if err != nil {
		return fmt.Errorf("checking schema version: %w", err)
	}

	if currentVersion == "" {
		// Fresh database — set version.
		_, err := db.Exec(
			`INSERT OR REPLACE INTO metadata (key, value) VALUES ('schema_version', ?)`,
			schemaVersion,
		)
		if err != nil {
			return fmt.Errorf("setting schema version: %w", err)
		}
	} else if currentVersion != schemaVersion {
		if err := migrateSchema(db, currentVersion); err != nil {
			return err
		}
	}

	return nil
}

// migrateSchema handles schema upgrades.
func migrateSchema(db *sql.DB, fromVersion string) error {
	switch fromVersion {
	case "1":
		return migrateV1ToV2(db)
	default:
		return fmt.Errorf("unsupported schema version %q, cannot migrate", fromVersion)
	}
}

// migrateV1ToV2 migrates from schema v1 (document-level embeddings) to v2
// (chunk-level embeddings). Existing documents are preserved; embeddings will
// be regenerated on next use.
func migrateV1ToV2(db *sql.DB) error {
	statements := []string{
		// Drop the old document-keyed vec table — embeddings must be regenerated.
		`DROP TABLE IF EXISTS documents_vec`,
		// Clear the embedding dimensions so ensureVecTable recreates with chunk_id key.
		`DELETE FROM metadata WHERE key = 'embedding_dimensions'`,
		// Update schema version.
		`INSERT OR REPLACE INTO metadata (key, value) VALUES ('schema_version', '2')`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrating v1 to v2: %w", err)
		}
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

	// Create vec table keyed by chunk_id.
	stmt := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS documents_vec USING vec0(chunk_id INTEGER PRIMARY KEY, embedding float[%d])`,
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
