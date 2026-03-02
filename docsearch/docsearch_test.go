package docsearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbedder is a simple embedder for testing that returns fixed-dimension vectors.
type mockEmbedder struct {
	dims    int
	vectors map[string][]float32
}

func newMockEmbedder(dims int) *mockEmbedder {
	return &mockEmbedder{dims: dims, vectors: make(map[string][]float32)}
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if v, ok := m.vectors[text]; ok {
		return v, nil
	}
	// Return a deterministic vector based on text length.
	v := make([]float32, m.dims)
	for i := range v {
		v[i] = float32(len(text)+i) / 100.0
	}
	return v, nil
}

func testDBPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

func TestOpen(t *testing.T) {
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	assert.False(t, idx.HasEmbedder())
}

func TestOpenWithEmbedder(t *testing.T) {
	embedder := newMockEmbedder(128)
	idx, err := Open(testDBPath(t), embedder)
	require.NoError(t, err)
	defer idx.Close()

	assert.True(t, idx.HasEmbedder())
}

func TestAdd(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	// Insert a new document.
	err = idx.Add(ctx, "/tmp/test.md", "Test Doc", "# Hello World\n\nThis is a test document.")
	require.NoError(t, err)

	// Verify it's searchable.
	results, err := idx.SearchKeyword(ctx, "hello", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "/tmp/test.md", results[0].Path)
	assert.Equal(t, "Test Doc", results[0].Title)
}

func TestAddUpdatesLastOpened(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	content := "# Test\n\nSame content"

	// Insert document.
	err = idx.Add(ctx, "/tmp/test.md", "Test", content)
	require.NoError(t, err)

	results, err := idx.SearchKeyword(ctx, "test", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	firstOpened := results[0].LastOpened

	// Re-add with same content — should only update last_opened.
	err = idx.Add(ctx, "/tmp/test.md", "Test", content)
	require.NoError(t, err)

	results, err = idx.SearchKeyword(ctx, "test", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, !results[0].LastOpened.Before(firstOpened))
}

func TestAddContentChange(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	// Insert document.
	err = idx.Add(ctx, "/tmp/test.md", "Test", "# Old content")
	require.NoError(t, err)

	// Update with new content.
	err = idx.Add(ctx, "/tmp/test.md", "Test Updated", "# New content with different words")
	require.NoError(t, err)

	// Old content should not be found.
	results, err := idx.SearchKeyword(ctx, "old", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)

	// New content should be found.
	results, err = idx.SearchKeyword(ctx, "different", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Test Updated", results[0].Title)
}

func TestSearchKeywordEmpty(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	err = idx.Add(ctx, "/tmp/a.md", "Doc A", "Content A")
	require.NoError(t, err)
	err = idx.Add(ctx, "/tmp/b.md", "Doc B", "Content B")
	require.NoError(t, err)

	// Empty query returns recent documents.
	results, err := idx.SearchKeyword(ctx, "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSearchSemanticWithoutEmbedder(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	_, err = idx.SearchSemantic(ctx, "test", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires an embedder")
}

func TestSearchSemanticWithEmbedder(t *testing.T) {
	ctx := context.Background()
	embedder := newMockEmbedder(4)
	dbPath := testDBPath(t)
	idx, err := Open(dbPath, embedder)
	require.NoError(t, err)
	defer idx.Close()

	err = idx.Add(ctx, "/tmp/a.md", "Doc A", "Alpha content")
	require.NoError(t, err)
	err = idx.Add(ctx, "/tmp/b.md", "Doc B", "Beta content")
	require.NoError(t, err)

	results, err := idx.SearchSemantic(ctx, "Alpha", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestFindSimilarWithEmbedder(t *testing.T) {
	ctx := context.Background()
	embedder := newMockEmbedder(4)
	// Give similar vectors to related docs.
	embedder.vectors["Similar content A"] = []float32{0.1, 0.2, 0.3, 0.4}
	embedder.vectors["Similar content B"] = []float32{0.11, 0.21, 0.31, 0.41}
	embedder.vectors["Very different content"] = []float32{0.9, 0.8, 0.7, 0.6}

	idx, err := Open(testDBPath(t), embedder)
	require.NoError(t, err)
	defer idx.Close()

	err = idx.Add(ctx, "/tmp/a.md", "Doc A", "Similar content A")
	require.NoError(t, err)
	err = idx.Add(ctx, "/tmp/b.md", "Doc B", "Similar content B")
	require.NoError(t, err)
	err = idx.Add(ctx, "/tmp/c.md", "Doc C", "Very different content")
	require.NoError(t, err)

	// Find similar to Doc A's content, excluding Doc A itself.
	results, err := idx.FindSimilar(ctx, "Similar content A", "/tmp/a.md", 2)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	// First result should be the most similar doc (Doc B).
	if len(results) > 0 {
		assert.Equal(t, "/tmp/b.md", results[0].Path)
	}
}

func TestFindSimilarNoIndexedDocs(t *testing.T) {
	ctx := context.Background()
	embedder := newMockEmbedder(4)
	idx, err := Open(testDBPath(t), embedder)
	require.NoError(t, err)
	defer idx.Close()

	// No documents indexed — should return empty results (not error).
	results, err := idx.FindSimilar(ctx, "some content", "", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRemove(t *testing.T) {
	ctx := context.Background()
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	err = idx.Add(ctx, "/tmp/test.md", "Test", "# Hello World")
	require.NoError(t, err)

	// Verify it exists.
	results, err := idx.SearchKeyword(ctx, "hello", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Remove it.
	err = idx.Remove("/tmp/test.md")
	require.NoError(t, err)

	// Verify it's gone.
	results, err = idx.SearchKeyword(ctx, "hello", 10)
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestRemoveNonexistent(t *testing.T) {
	idx, err := Open(testDBPath(t), nil)
	require.NoError(t, err)
	defer idx.Close()

	err = idx.Remove("/tmp/nonexistent.md")
	assert.NoError(t, err)
}

func TestDimensionMismatchRecreatesVecTable(t *testing.T) {
	dbPath := testDBPath(t)

	// Open with 4 dimensions.
	embedder4 := newMockEmbedder(4)
	idx, err := Open(dbPath, embedder4)
	require.NoError(t, err)

	ctx := context.Background()
	err = idx.Add(ctx, "/tmp/test.md", "Test", "Content")
	require.NoError(t, err)

	idx.Close()

	// Reopen with 8 dimensions — vec table should be recreated.
	embedder8 := newMockEmbedder(8)
	idx, err = Open(dbPath, embedder8)
	require.NoError(t, err)
	defer idx.Close()

	// Should be able to add and search with new dimensions.
	err = idx.Add(ctx, "/tmp/test2.md", "Test 2", "Content 2")
	require.NoError(t, err)

	results, err := idx.SearchSemantic(ctx, "Content", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}

func TestPersistence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "persist.db")

	// Open, add a document, close.
	idx, err := Open(dbPath, nil)
	require.NoError(t, err)
	err = idx.Add(ctx, "/tmp/test.md", "Test", "# Persistent content")
	require.NoError(t, err)
	idx.Close()

	// Reopen and verify the document is still there.
	idx, err = Open(dbPath, nil)
	require.NoError(t, err)
	defer idx.Close()

	results, err := idx.SearchKeyword(ctx, "persistent", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "/tmp/test.md", results[0].Path)
}

func TestDataDirectory(t *testing.T) {
	// Verify Open creates parent directories.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "dir", "test.db")
	err := os.MkdirAll(filepath.Dir(dbPath), 0o755)
	require.NoError(t, err)

	idx, err := Open(dbPath, nil)
	require.NoError(t, err)
	defer idx.Close()
}
