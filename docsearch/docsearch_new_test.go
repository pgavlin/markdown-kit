//go:build sqlite_fts5

package docsearch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPaths(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	idx, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx.Close()

	ctx := context.Background()

	// Initially empty.
	paths, err := idx.ListPaths(ctx)
	require.NoError(t, err)
	assert.Empty(t, paths)

	// Add some documents.
	require.NoError(t, idx.Add(ctx, "/a.md", "A", "# A\nContent A"))
	require.NoError(t, idx.Add(ctx, "/b.md", "B", "# B\nContent B"))

	paths, err = idx.ListPaths(ctx)
	require.NoError(t, err)
	assert.Len(t, paths, 2)
	assert.Contains(t, paths, "/a.md")
	assert.Contains(t, paths, "/b.md")
}

func TestDocumentsNeedingEmbeddings(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open without embedder — documents will have no chunks.
	idx, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx.Close()

	ctx := context.Background()

	require.NoError(t, idx.Add(ctx, "/a.md", "A", "# A\nContent A"))
	require.NoError(t, idx.Add(ctx, "/b.md", "B", "# B\nContent B"))

	docs, err := idx.DocumentsNeedingEmbeddings(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	// With limit.
	docs, err = idx.DocumentsNeedingEmbeddings(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestAddFTSOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	idx, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx.Close()

	ctx := context.Background()

	// Add via FTS-only.
	err = idx.AddFTSOnly(ctx, "/test.md", "Test", "# Test\nHello world")
	require.NoError(t, err)

	// Should be searchable.
	results, err := idx.SearchKeyword(ctx, "Hello", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "/test.md", results[0].Path)

	// Adding same content again should be a no-op (hash match).
	err = idx.AddFTSOnly(ctx, "/test.md", "Test", "# Test\nHello world")
	require.NoError(t, err)

	// Changed content should update.
	err = idx.AddFTSOnly(ctx, "/test.md", "Test", "# Test\nUpdated content")
	require.NoError(t, err)

	results, err = idx.SearchKeyword(ctx, "Updated", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestBusyTimeout(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open two connections to the same database.
	idx1, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx1.Close()

	idx2, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx2.Close()

	ctx := context.Background()

	// Both should be able to write (busy_timeout handles contention).
	require.NoError(t, idx1.Add(ctx, "/a.md", "A", "Content A"))
	require.NoError(t, idx2.Add(ctx, "/b.md", "B", "Content B"))

	// Both documents should exist.
	paths, err := idx1.ListPaths(ctx)
	require.NoError(t, err)
	assert.Len(t, paths, 2)
}
