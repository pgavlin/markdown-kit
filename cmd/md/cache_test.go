package main

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheKeyHash(t *testing.T) {
	h1 := cacheKeyHash("https://example.com")
	h2 := cacheKeyHash("https://example.com")
	h3 := cacheKeyHash("https://other.com")

	if h1 != h2 {
		t.Errorf("same input should produce same hash: %s != %s", h1, h2)
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(h1))
	}
}

func TestContentHash(t *testing.T) {
	h1 := contentHash([]byte("hello"))
	h2 := contentHash([]byte("hello"))
	h3 := contentHash([]byte("world"))

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hashes")
	}
}

func TestParseCacheControl(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"no-store", "no-store", map[string]string{"no-store": "true"}},
		{"max-age", "max-age=3600", map[string]string{"max-age": "3600"}},
		{"multiple", "no-cache, max-age=0", map[string]string{"no-cache": "true", "max-age": "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCacheControl(tt.header)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("directive %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseHTTPDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		zero  bool
	}{
		{"rfc1123", "Mon, 02 Jan 2006 15:04:05 GMT", false},
		{"rfc850", "Monday, 02-Jan-06 15:04:05 GMT", false},
		{"asctime", "Mon Jan  2 15:04:05 2006", false},
		{"empty", "", true},
		{"invalid", "not a date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHTTPDate(tt.input)
			if tt.zero && !got.IsZero() {
				t.Errorf("expected zero time, got %v", got)
			}
			if !tt.zero && got.IsZero() {
				t.Error("expected non-zero time, got zero")
			}
		})
	}
}

func TestCacheEntryFromResponse(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Etag":          {"\"abc\""},
			"Last-Modified": {"Mon, 02 Jan 2006 15:04:05 GMT"},
			"Cache-Control": {"max-age=3600"},
			"Date":          {"Mon, 02 Jan 2006 15:04:05 GMT"},
		},
	}
	entry := cacheEntryFromResponse(resp)

	if entry.ETag != "\"abc\"" {
		t.Errorf("ETag: got %q", entry.ETag)
	}
	if entry.CacheControl != "max-age=3600" {
		t.Errorf("CacheControl: got %q", entry.CacheControl)
	}
}

func TestCache_LoadStore_RoundTrip(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	entry := cacheEntry{
		Name:     "test",
		Markdown: "# Hello",
		ETag:     "\"123\"",
	}
	c.store("key1", entry, logger)

	loaded := c.load("key1")
	if loaded == nil {
		t.Fatal("expected non-nil entry")
	}
	if loaded.Name != "test" {
		t.Errorf("Name: got %q, want %q", loaded.Name, "test")
	}
	if loaded.Markdown != "# Hello" {
		t.Errorf("Markdown: got %q, want %q", loaded.Markdown, "# Hello")
	}
	if loaded.ETag != "\"123\"" {
		t.Errorf("ETag: got %q, want %q", loaded.ETag, "\"123\"")
	}
}

func TestCache_Load_Missing(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}

	if got := c.load("nonexistent"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCache_Load_InvalidJSON(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}

	// Write invalid JSON at the expected path.
	path := c.entryPath("key1")
	fs.files[path] = []byte("not json{{{")

	if got := c.load("key1"); got != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestCache_LookupHTTP_Fresh(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	now := time.Now().UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:         "page",
		Markdown:     "# Page",
		CacheControl: "max-age=3600",
		Date:         now,
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if !fresh {
		t.Error("expected fresh=true")
	}
	if got.Name != "page" {
		t.Errorf("Name: got %q", got.Name)
	}
}

func TestCache_LookupHTTP_Stale(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	pastDate := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:         "page",
		Markdown:     "# Page",
		CacheControl: "max-age=60",
		Date:         pastDate,
		ETag:         "\"stale\"",
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if fresh {
		t.Error("expected fresh=false for expired entry")
	}
}

func TestCache_LookupHTTP_NoStore(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	entry := cacheEntry{
		Name:         "page",
		Markdown:     "# Page",
		CacheControl: "no-store",
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got != nil {
		t.Errorf("expected nil for no-store, got %+v", got)
	}
	if fresh {
		t.Error("expected fresh=false")
	}
}

func TestCache_LookupHTTP_NoCache(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	entry := cacheEntry{
		Name:         "page",
		Markdown:     "# Page",
		CacheControl: "no-cache",
		ETag:         "\"nc\"",
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry for no-cache")
	}
	if fresh {
		t.Error("expected fresh=false for no-cache")
	}
}

func TestCache_LookupHTTP_NilCache(t *testing.T) {
	var c *conversionCache
	logger := discardLogger()

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got != nil {
		t.Error("expected nil from nil cache")
	}
	if fresh {
		t.Error("expected fresh=false from nil cache")
	}
}

func TestCache_StoreHTTP_NilCache(t *testing.T) {
	var c *conversionCache
	logger := discardLogger()

	// Should not panic.
	c.storeHTTP("http://example.com", cacheEntry{Name: "test"}, logger)
}

func TestCache_LookupHTTP_ExpiresFresh(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	// Use Expires header (no Cache-Control max-age).
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:     "page",
		Markdown: "# Expires Fresh",
		Expires:  future,
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if !fresh {
		t.Error("expected fresh=true for future Expires")
	}
}

func TestCache_LookupHTTP_ExpiresStale(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	// Expires in the past.
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:     "page",
		Markdown: "# Expires Stale",
		Expires:  past,
		ETag:     "\"stale-expires\"",
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if fresh {
		t.Error("expected fresh=false for past Expires")
	}
}

func TestCache_LookupHTTP_NoCachingHeaders(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	// No Cache-Control, no Expires — should return stale.
	entry := cacheEntry{
		Name:     "page",
		Markdown: "# No Headers",
	}
	c.store("http://example.com", entry, logger)

	got, fresh := c.lookupHTTP("http://example.com", logger)
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if fresh {
		t.Error("expected fresh=false when no caching headers")
	}
}

func TestCache_LookupFile_HashMatch(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	content := []byte("file content")
	entry := cacheEntry{
		Name:     "file.md",
		Markdown: "# Converted",
	}
	c.storeFile("/path/to/file.html", content, entry, logger)

	got, ok := c.lookupFile("/path/to/file.html", content, logger)
	if !ok {
		t.Error("expected ok=true for matching hash")
	}
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if got.Markdown != "# Converted" {
		t.Errorf("Markdown: got %q", got.Markdown)
	}
}

func TestCache_LookupFile_HashMismatch(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	content := []byte("original content")
	entry := cacheEntry{Name: "file.md", Markdown: "# Old"}
	c.storeFile("/path/to/file.html", content, entry, logger)

	got, ok := c.lookupFile("/path/to/file.html", []byte("different content"), logger)
	if ok {
		t.Error("expected ok=false for mismatched hash")
	}
	if got != nil {
		t.Errorf("expected nil entry, got %+v", got)
	}
}

func TestCache_StoreFile_SetsContentHash(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	content := []byte("test content")
	entry := cacheEntry{Name: "file.md", Markdown: "# Test"}
	c.storeFile("/path/to/file", content, entry, logger)

	loaded := c.load("/path/to/file")
	if loaded == nil {
		t.Fatal("expected non-nil entry")
	}
	expected := contentHash(content)
	if loaded.ContentHash != expected {
		t.Errorf("ContentHash: got %q, want %q", loaded.ContentHash, expected)
	}
}
