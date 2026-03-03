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
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
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
		if _, err := idx.db.ExecContext(ctx,
			`UPDATE documents SET last_opened = ?, title = ? WHERE id = ?`,
			now, title, existingID,
		); err != nil {
			return err
		}

		// If embedder is configured but no chunks exist (e.g. after migration),
		// generate them now.
		if idx.embedder != nil {
			var chunkCount int
			if err := idx.db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM chunks WHERE document_id = ?`, existingID,
			).Scan(&chunkCount); err != nil {
				return fmt.Errorf("checking chunks: %w", err)
			}
			if chunkCount == 0 {
				if err := idx.updateEmbeddings(ctx, existingID, markdown); err != nil {
					return err
				}
			}
		}
		return nil
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

		// Update chunked embeddings if embedder is configured.
		if idx.embedder != nil {
			if err := idx.updateEmbeddings(ctx, existingID, markdown); err != nil {
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

	// Update chunked embeddings if embedder is configured.
	if idx.embedder != nil {
		docID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("getting document id: %w", err)
		}
		if err := idx.updateEmbeddings(ctx, docID, markdown); err != nil {
			return err
		}
	}

	return nil
}

// updateEmbeddings generates and stores chunked vector embeddings for the given document.
func (idx *Index) updateEmbeddings(ctx context.Context, docID int64, markdown string) error {
	// Delete existing chunks and their vec entries.
	if _, err := idx.db.ExecContext(ctx, `DELETE FROM documents_vec WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id = ?)`, docID); err != nil {
		return fmt.Errorf("deleting old vec entries: %w", err)
	}
	if _, err := idx.db.ExecContext(ctx, `DELETE FROM chunks WHERE document_id = ?`, docID); err != nil {
		return fmt.Errorf("deleting old chunks: %w", err)
	}

	// Chunk the markdown.
	chunks := chunkMarkdown(markdown, 2000)
	if len(chunks) == 0 {
		// Nothing to embed (e.g. empty document).
		return nil
	}

	for i, c := range chunks {
		// Insert chunk.
		result, err := idx.db.ExecContext(ctx,
			`INSERT INTO chunks (document_id, chunk_index, heading, content) VALUES (?, ?, ?, ?)`,
			docID, i, c.heading, c.text,
		)
		if err != nil {
			return fmt.Errorf("inserting chunk: %w", err)
		}

		chunkID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("getting chunk id: %w", err)
		}

		// Embed and store.
		embedding, err := idx.embedder.Embed(ctx, c.text)
		if err != nil {
			return fmt.Errorf("generating embedding for chunk %d: %w", i, err)
		}

		_, err = idx.db.ExecContext(ctx,
			`INSERT INTO documents_vec (chunk_id, embedding) VALUES (?, ?)`,
			chunkID, serializeFloat32s(embedding),
		)
		if err != nil {
			return fmt.Errorf("storing embedding for chunk %d: %w", i, err)
		}
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
// Results are grouped by document, using the best (minimum distance) chunk match.
func (idx *Index) SearchSemantic(ctx context.Context, query string, limit int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, fmt.Errorf("semantic search requires an embedder")
	}

	embedding, err := idx.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Fetch more chunk matches than needed, then group by document.
	k := limit * 5
	if k < 20 {
		k = 20
	}

	rows, err := idx.db.QueryContext(ctx, `
		SELECT d.path, d.title, d.last_opened, MIN(v.distance) AS distance
		FROM documents_vec v
		JOIN chunks c ON c.id = v.chunk_id
		JOIN documents d ON d.id = c.document_id
		WHERE v.embedding MATCH ?
		AND k = ?
		GROUP BY d.id
		ORDER BY distance
		LIMIT ?
	`, serializeFloat32s(embedding), k, limit)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	defer rows.Close()

	return scanResults(rows)
}

// FindSimilar returns documents most similar to the given content,
// using vector embedding cosine similarity. Requires an embedder.
// The document at excludePath (if non-empty) is excluded from results.
//
// All chunks of the query content are embedded and searched; results are
// aggregated by taking the best (minimum) distance per target document.
func (idx *Index) FindSimilar(ctx context.Context, content, excludePath string, limit int) ([]Result, error) {
	if idx.embedder == nil {
		return nil, fmt.Errorf("find similar requires an embedder")
	}

	// Look up the document ID to exclude (if it exists in the index).
	var excludeID int64 = -1
	if excludePath != "" {
		_ = idx.db.QueryRowContext(ctx,
			`SELECT id FROM documents WHERE path = ?`, excludePath,
		).Scan(&excludeID)
	}

	// Chunk and embed the query content.
	queryChunks := chunkMarkdown(content, 2000)
	if len(queryChunks) == 0 {
		// Nothing to search with — embed the raw content as fallback.
		stripped := stripMarkdown(content)
		if stripped == "" {
			return nil, nil
		}
		queryChunks = []chunk{{text: stripped}}
	}

	// Search with each query chunk and merge results.
	type docResult struct {
		path       string
		title      string
		lastOpened int64
		distance   float64
	}
	bestByPath := make(map[string]*docResult)

	k := limit * 5
	if k < 20 {
		k = 20
	}

	for _, qc := range queryChunks {
		embedding, err := idx.embedder.Embed(ctx, qc.text)
		if err != nil {
			return nil, fmt.Errorf("embedding content chunk: %w", err)
		}

		rows, err := idx.db.QueryContext(ctx, `
			SELECT d.path, d.title, d.last_opened, MIN(v.distance) AS distance
			FROM documents_vec v
			JOIN chunks c ON c.id = v.chunk_id
			JOIN documents d ON d.id = c.document_id
			WHERE v.embedding MATCH ?
			AND k = ?
			AND c.document_id != ?
			GROUP BY d.id
			ORDER BY distance
		`, serializeFloat32s(embedding), k, excludeID)
		if err != nil {
			return nil, fmt.Errorf("finding similar: %w", err)
		}

		for rows.Next() {
			var r docResult
			if err := rows.Scan(&r.path, &r.title, &r.lastOpened, &r.distance); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning result: %w", err)
			}

			if existing, ok := bestByPath[r.path]; ok {
				if r.distance < existing.distance {
					existing.distance = r.distance
				}
			} else {
				bestByPath[r.path] = &r
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterating results: %w", err)
		}
	}

	// Convert to sorted results.
	results := make([]Result, 0, len(bestByPath))
	for _, r := range bestByPath {
		results = append(results, Result{
			Path:       r.path,
			Title:      r.title,
			LastOpened: time.Unix(r.lastOpened, 0),
			Score:      r.distance,
		})
	}

	// Sort by distance (ascending).
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score < results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

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

	// Delete vec entries for this document's chunks.
	_, _ = idx.db.Exec(`DELETE FROM documents_vec WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id = ?)`, docID)

	// Delete chunks.
	_, _ = idx.db.Exec(`DELETE FROM chunks WHERE document_id = ?`, docID)

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
