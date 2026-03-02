package docsearch

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	vec.Auto()
}

// Result represents a search result.
type Result struct {
	Path       string
	Title      string
	LastOpened time.Time
	Score      float64
}

// Index is a SQLite-backed document index supporting full-text and vector search.
type Index struct {
	db       *sql.DB
	embedder Embedder
}

// Open opens (or creates) the index database at the given path.
// If embedder is nil, only keyword search will be available.
func Open(path string, embedder Embedder) (*Index, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	if embedder != nil {
		if err := ensureVecTable(db, embedder.Dimensions()); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &Index{db: db, embedder: embedder}, nil
}

// Close closes the database.
func (idx *Index) Close() error {
	return idx.db.Close()
}

// HasEmbedder reports whether an embedder is configured.
func (idx *Index) HasEmbedder() bool {
	return idx.embedder != nil
}

// contentHash returns the SHA-256 hash of the given content.
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// Add indexes a document. If the document already exists with the same
// content hash, only last_opened is updated. Otherwise the FTS and
// vector indexes are refreshed.
func (idx *Index) Add(ctx context.Context, path, title, markdown string) error {
	now := time.Now().Unix()
	hash := contentHash(markdown)

	// Check if the document exists with the same content hash.
	var existingID int64
	var existingHash string
	err := idx.db.QueryRowContext(ctx,
		`SELECT id, content_hash FROM documents WHERE path = ?`, path,
	).Scan(&existingID, &existingHash)

	if err == nil && existingHash == hash {
		// Content unchanged — just update last_opened.
		_, err := idx.db.ExecContext(ctx,
			`UPDATE documents SET last_opened = ?, title = ? WHERE id = ?`,
			now, title, existingID,
		)
		return err
	}

	// Insert or replace the document. The triggers handle FTS updates.
	if err == nil {
		// Existing document with changed content — update it.
		_, err := idx.db.ExecContext(ctx,
			`UPDATE documents SET title = ?, content = ?, content_hash = ?, last_opened = ?, indexed_at = ? WHERE id = ?`,
			title, markdown, hash, now, now, existingID,
		)
		if err != nil {
			return fmt.Errorf("updating document: %w", err)
		}

		// Update vector embedding if embedder is configured.
		if idx.embedder != nil {
			if err := idx.updateEmbedding(ctx, existingID, markdown); err != nil {
				return err
			}
		}
		return nil
	}

	if err != sql.ErrNoRows {
		return fmt.Errorf("checking existing document: %w", err)
	}

	// New document — insert it. Triggers handle FTS.
	result, err := idx.db.ExecContext(ctx,
		`INSERT INTO documents (path, title, content, content_hash, last_opened, indexed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		path, title, markdown, hash, now, now,
	)
	if err != nil {
		return fmt.Errorf("inserting document: %w", err)
	}

	// Update vector embedding if embedder is configured.
	if idx.embedder != nil {
		docID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("getting document id: %w", err)
		}
		if err := idx.updateEmbedding(ctx, docID, markdown); err != nil {
			return err
		}
	}

	return nil
}

// updateEmbedding generates and stores a vector embedding for the given document.
func (idx *Index) updateEmbedding(ctx context.Context, docID int64, text string) error {
	embedding, err := idx.embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	_, err = idx.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO documents_vec (document_id, embedding) VALUES (?, ?)`,
		docID, serializeFloat32s(embedding),
	)
	if err != nil {
		return fmt.Errorf("storing embedding: %w", err)
	}

	return nil
}

// SearchKeyword returns documents matching the query using FTS5 keyword search.
// Works without an embedder.
func (idx *Index) SearchKeyword(ctx context.Context, query string, limit int) ([]Result, error) {
	if query == "" {
		return idx.recentDocuments(ctx, limit)
	}

	rows, err := idx.db.QueryContext(ctx, `
		SELECT d.path, d.title, d.last_opened, rank
		FROM documents_fts fts
		JOIN documents d ON d.id = fts.rowid
		WHERE documents_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	return scanResults(rows)
}

// recentDocuments returns the most recently opened documents.
func (idx *Index) recentDocuments(ctx context.Context, limit int) ([]Result, error) {
	rows, err := idx.db.QueryContext(ctx, `
		SELECT path, title, last_opened, 0.0
		FROM documents
		ORDER BY last_opened DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("recent documents: %w", err)
	}
	defer rows.Close()

	return scanResults(rows)
}

// SearchSemantic returns documents matching the query using vector similarity.
// Requires an embedder to be configured; returns an error otherwise.
func (idx *Index) SearchSemantic(ctx context.Context, query string, limit int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, fmt.Errorf("semantic search requires an embedder")
	}

	embedding, err := idx.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	rows, err := idx.db.QueryContext(ctx, `
		SELECT d.path, d.title, d.last_opened, v.distance
		FROM documents_vec v
		JOIN documents d ON d.id = v.document_id
		WHERE v.embedding MATCH ?
		AND k = ?
		ORDER BY v.distance
	`, serializeFloat32s(embedding), limit)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	defer rows.Close()

	return scanResults(rows)
}

// FindSimilar returns documents most similar to the given content,
// using vector embedding cosine similarity. Requires an embedder.
// The document at excludePath (if non-empty) is excluded from results.
func (idx *Index) FindSimilar(ctx context.Context, content, excludePath string, limit int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, fmt.Errorf("find similar requires an embedder")
	}

	// Embed the current document's content directly.
	embedding, err := idx.embedder.Embed(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("embedding content: %w", err)
	}

	// Look up the document ID to exclude (if it exists in the index).
	var excludeID int64 = -1
	if excludePath != "" {
		_ = idx.db.QueryRowContext(ctx,
			`SELECT id FROM documents WHERE path = ?`, excludePath,
		).Scan(&excludeID)
	}

	// Search for similar documents, excluding the source document.
	rows, err := idx.db.QueryContext(ctx, `
		SELECT d.path, d.title, d.last_opened, v.distance
		FROM documents_vec v
		JOIN documents d ON d.id = v.document_id
		WHERE v.embedding MATCH ?
		AND k = ?
		AND v.document_id != ?
		ORDER BY v.distance
	`, serializeFloat32s(embedding), limit+1, excludeID)
	if err != nil {
		return nil, fmt.Errorf("finding similar: %w", err)
	}
	defer rows.Close()

	results, err := scanResults(rows)
	if err != nil {
		return nil, err
	}

	// Trim to requested limit.
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Remove removes a document from the index.
func (idx *Index) Remove(path string) error {
	var docID int64
	err := idx.db.QueryRow(`SELECT id FROM documents WHERE path = ?`, path).Scan(&docID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("looking up document: %w", err)
	}

	// Delete from vec table first (if it exists).
	_, _ = idx.db.Exec(`DELETE FROM documents_vec WHERE document_id = ?`, docID)

	// Delete from documents table (triggers handle FTS cleanup).
	_, err = idx.db.Exec(`DELETE FROM documents WHERE id = ?`, docID)
	if err != nil {
		return fmt.Errorf("removing document: %w", err)
	}

	return nil
}

// scanResults reads search result rows into a slice.
func scanResults(rows *sql.Rows) ([]Result, error) {
	var results []Result
	for rows.Next() {
		var r Result
		var lastOpened int64
		if err := rows.Scan(&r.Path, &r.Title, &lastOpened, &r.Score); err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}
		r.LastOpened = time.Unix(lastOpened, 0)
		results = append(results, r)
	}
	return results, rows.Err()
}

// serializeFloat32s converts a float32 slice to a byte slice for sqlite-vec.
func serializeFloat32s(v []float32) []byte {
	b, _ := vec.SerializeFloat32(v)
	return b
}
