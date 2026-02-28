package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// cacheEntry holds a cached conversion result along with HTTP caching metadata
// or a content hash for local files.
type cacheEntry struct {
	// HTTP caching metadata (empty for local files).
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Expires      string `json:"expires,omitempty"`
	CacheControl string `json:"cache_control,omitempty"`
	Date         string `json:"date,omitempty"`

	// Content hash for local files.
	ContentHash string `json:"content_hash,omitempty"`

	// The cached conversion result.
	Name            string `json:"name"`
	Markdown        string `json:"markdown"`
	OriginalHTML    string `json:"original_html,omitempty"`
	ReadabilityHTML string `json:"readability_html,omitempty"`
}

// conversionCache stores cached conversion results on disk.
type conversionCache struct {
	dir string
}

// openCache creates a conversionCache using the user's cache directory.
// Returns nil if the cache directory cannot be determined or created (cache
// disabled gracefully).
func openCache() *conversionCache {
	userCache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(userCache, "md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	return &conversionCache{dir: dir}
}

// cacheKeyHash returns the SHA-256 hex digest of a cache key.
func cacheKeyHash(key string) string {
	h := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", h)
}

// entryPath returns the file path for a given cache key.
func (c *conversionCache) entryPath(key string) string {
	return filepath.Join(c.dir, cacheKeyHash(key)+".json")
}

// load reads and decodes a cache entry from disk. Returns nil if the entry
// does not exist or cannot be decoded.
func (c *conversionCache) load(key string) *cacheEntry {
	data, err := os.ReadFile(c.entryPath(key))
	if err != nil {
		return nil
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	return &entry
}

// store writes a cache entry to disk.
func (c *conversionCache) store(key string, entry cacheEntry, logger *slog.Logger) {
	data, err := json.Marshal(entry)
	if err != nil {
		logger.Error("cache_write_error", "key", key, "error", err)
		return
	}
	if err := os.WriteFile(c.entryPath(key), data, 0o644); err != nil {
		logger.Error("cache_write_error", "key", key, "error", err)
	}
}

// lookupHTTP returns a cached entry for the given URL. The second return value
// indicates whether the entry is fresh (can be used without revalidation).
// A stale entry is still returned so the caller can use ETag/Last-Modified for
// conditional requests. Returns nil, false if no entry exists or caching is
// disabled.
func (c *conversionCache) lookupHTTP(rawURL string, logger *slog.Logger) (*cacheEntry, bool) {
	if c == nil {
		return nil, false
	}

	logger.Debug("cache_load", "key", rawURL)
	entry := c.load(rawURL)
	if entry == nil {
		return nil, false
	}

	// Parse Cache-Control directives.
	directives := parseCacheControl(entry.CacheControl)

	// no-store: pretend the entry doesn't exist.
	if _, ok := directives["no-store"]; ok {
		return nil, false
	}

	// no-cache: return the entry but mark as stale for revalidation.
	if _, ok := directives["no-cache"]; ok {
		return entry, false
	}

	// Determine if the entry is fresh.
	if maxAgeStr, ok := directives["max-age"]; ok {
		maxAge, err := strconv.Atoi(maxAgeStr)
		if err == nil {
			dateTime := parseHTTPDate(entry.Date)
			if !dateTime.IsZero() {
				expiry := dateTime.Add(time.Duration(maxAge) * time.Second)
				if time.Now().Before(expiry) {
					return entry, true
				}
				return entry, false
			}
		}
	}

	// Fall back to Expires header.
	if entry.Expires != "" {
		expiresTime := parseHTTPDate(entry.Expires)
		if !expiresTime.IsZero() {
			if time.Now().Before(expiresTime) {
				return entry, true
			}
			return entry, false
		}
	}

	// No caching headers — return entry as stale.
	return entry, false
}

// storeHTTP writes an HTTP cache entry to disk.
func (c *conversionCache) storeHTTP(rawURL string, entry cacheEntry, logger *slog.Logger) {
	if c == nil {
		return
	}
	c.store(rawURL, entry, logger)
}

// lookupFile returns a cached entry for a local file if the content hash
// matches. Returns nil, false on cache miss.
func (c *conversionCache) lookupFile(absPath string, content []byte, logger *slog.Logger) (*cacheEntry, bool) {
	if c == nil {
		return nil, false
	}

	logger.Debug("cache_load", "key", absPath)
	entry := c.load(absPath)
	if entry == nil {
		return nil, false
	}

	hash := contentHash(content)
	if entry.ContentHash != hash {
		return nil, false
	}
	return entry, true
}

// storeFile writes a local file cache entry to disk, setting ContentHash from
// the provided content.
func (c *conversionCache) storeFile(absPath string, content []byte, entry cacheEntry, logger *slog.Logger) {
	if c == nil {
		return
	}
	entry.ContentHash = contentHash(content)
	c.store(absPath, entry, logger)
}

// contentHash returns the SHA-256 hex digest of content.
func contentHash(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h)
}

// cacheEntryFromResponse extracts HTTP caching headers from a response into a
// cacheEntry.
func cacheEntryFromResponse(resp *http.Response) cacheEntry {
	return cacheEntry{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		Expires:      resp.Header.Get("Expires"),
		CacheControl: resp.Header.Get("Cache-Control"),
		Date:         resp.Header.Get("Date"),
	}
}

// parseCacheControl parses a Cache-Control header value into a map of
// directive names to values. Directives without values (like "no-store")
// map to "true".
func parseCacheControl(header string) map[string]string {
	directives := make(map[string]string)
	if header == "" {
		return directives
	}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if eqIdx := strings.IndexByte(part, '='); eqIdx >= 0 {
			key := strings.TrimSpace(part[:eqIdx])
			val := strings.TrimSpace(part[eqIdx+1:])
			directives[strings.ToLower(key)] = val
		} else {
			directives[strings.ToLower(part)] = "true"
		}
	}
	return directives
}

// parseHTTPDate parses an HTTP date string (RFC 1123 or RFC 850 format).
func parseHTTPDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC 1123 first (most common).
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t
	}
	// Try RFC 850.
	if t, err := time.Parse(time.RFC850, s); err == nil {
		return t
	}
	// Try ANSI C asctime format.
	if t, err := time.Parse("Mon Jan _2 15:04:05 2006", s); err == nil {
		return t
	}
	return time.Time{}
}
