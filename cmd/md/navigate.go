package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// pageLoadedMsg is sent when a page has been successfully loaded.
type pageLoadedMsg struct {
	name            string
	markdown        string
	source          string
	originalHTML    string // non-empty for pages fetched from HTML
	readabilityHTML string // non-empty for pages fetched from HTML
}

// pageLoadErrorMsg is sent when a page fails to load.
type pageLoadErrorMsg struct {
	url string
	err error
}

// resolveLink resolves a link relative to the current document source.
// For file sources, it uses filepath operations. For URL sources, it uses
// url.Parse + ResolveReference.
func resolveLink(link, currentSource string) string {
	if currentSource == "" {
		return link
	}

	// If the link is already an absolute URL, return as-is.
	if u, err := url.Parse(link); err == nil && u.IsAbs() {
		return link
	}

	// If the current source is a URL, resolve relative to it.
	if strings.HasPrefix(currentSource, "http://") || strings.HasPrefix(currentSource, "https://") {
		base, err := url.Parse(currentSource)
		if err != nil {
			return link
		}
		ref, err := url.Parse(link)
		if err != nil {
			return link
		}
		return base.ResolveReference(ref).String()
	}

	// File-based resolution.
	if filepath.IsAbs(link) {
		return link
	}
	return filepath.Join(filepath.Dir(currentSource), link)
}

var markdownExts = map[string]bool{
	".md":       true,
	".markdown": true,
	".mdown":    true,
	".mkdn":     true,
	".mkd":      true,
	".mdwn":     true,
}

// markdownExtsList returns the allowed markdown file extensions as a slice.
func markdownExtsList() []string {
	exts := make([]string, 0, len(markdownExts))
	for ext := range markdownExts {
		exts = append(exts, ext)
	}
	return exts
}

// isMarkdownFile checks if a path has a markdown file extension.
func isMarkdownFile(path string) bool {
	// Strip any fragment (e.g. "file.md#heading").
	if i := strings.IndexByte(path, '#'); i >= 0 {
		path = path[:i]
	}
	ext := strings.ToLower(filepath.Ext(path))
	return markdownExts[ext]
}

// isMarkdownContentType checks if a content-type header indicates markdown.
func isMarkdownContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/markdown") || strings.HasPrefix(ct, "text/x-markdown")
}

// loadFilePage reads a local markdown file and returns a pageLoadedMsg.
func loadFilePage(path string, fsys fileSystem, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		data, err := fsys.ReadFile(path)
		if err != nil {
			logger.Error("file_read_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}
		logger.Info("file_read", "path", path, "size", len(data))
		return pageLoadedMsg{
			name:     filepath.Base(path),
			markdown: string(data),
			source:   path,
		}
	}
}

// fetchResult holds the result of fetching a URL.
type fetchResult struct {
	name            string
	markdown        string
	source          string
	originalHTML    string // non-empty when the source was HTML
	readabilityHTML string // non-empty when the source was HTML
}

// fetchURL fetches a URL and returns a fetchResult. If the content is markdown,
// it's used directly. Otherwise, the provided converter is used to convert the
// content to markdown. Results are cached to disk when a cache is provided.
func fetchURL(rawURL string, conv converter, cache *conversionCache, client httpClient, logger *slog.Logger) (fetchResult, error) {
	// Check the cache for a fresh or stale entry.
	cached, fresh := cache.lookupHTTP(rawURL, logger)
	if cached != nil && fresh {
		logger.Info("cache_hit", "url", rawURL, "fresh", true)
		return fetchResult{
			name:            cached.Name,
			markdown:        cached.Markdown,
			source:          rawURL,
			originalHTML:    cached.OriginalHTML,
			readabilityHTML: cached.ReadabilityHTML,
		}, nil
	}
	if cached != nil {
		logger.Info("cache_hit", "url", rawURL, "fresh", false)
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return fetchResult{}, err
	}
	req.Header.Set("User-Agent", "markdown-kit/md")

	// Add conditional request headers if we have a stale cache entry.
	if cached != nil {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			req.Header.Set("If-Modified-Since", cached.LastModified)
		}
	}

	logger.Info("http_request", "method", "GET", "url", rawURL)
	start := time.Now()

	resp, err := client.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer resp.Body.Close()

	logger.Info("http_response", "url", rawURL, "status", resp.StatusCode, "duration", time.Since(start))

	// 304 Not Modified — use the cached entry.
	if resp.StatusCode == http.StatusNotModified && cached != nil {
		logger.Info("http_cache_revalidated", "url", rawURL)
		// Update caching headers from the new response.
		updated := *cached
		if v := resp.Header.Get("ETag"); v != "" {
			updated.ETag = v
		}
		if v := resp.Header.Get("Cache-Control"); v != "" {
			updated.CacheControl = v
		}
		if v := resp.Header.Get("Expires"); v != "" {
			updated.Expires = v
		}
		if v := resp.Header.Get("Date"); v != "" {
			updated.Date = v
		}
		cache.storeHTTP(rawURL, updated, logger)
		return fetchResult{
			name:            cached.Name,
			markdown:        cached.Markdown,
			source:          rawURL,
			originalHTML:    cached.OriginalHTML,
			readabilityHTML: cached.ReadabilityHTML,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return fetchResult{}, fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	// Use the final URL after redirects.
	finalURL := resp.Request.URL.String()
	ct := resp.Header.Get("Content-Type")

	if isMarkdownContentType(ct) {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fetchResult{}, err
		}
		entry := cacheEntryFromResponse(resp)
		entry.Name = pageTitleFromURL(finalURL)
		entry.Markdown = string(body)
		cache.storeHTTP(rawURL, entry, logger)
		logger.Info("cache_store", "url", rawURL)
		return fetchResult{
			name:     entry.Name,
			markdown: entry.Markdown,
			source:   finalURL,
		}, nil
	}

	// Read the full body for conversion.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fetchResult{}, err
	}

	logger.Info("content_converted", "url", finalURL, "content_type", ct)

	pageURL, _ := url.Parse(finalURL)
	cr, err := conv.convert(body, pageURL, logger)
	if err != nil {
		return fetchResult{}, err
	}

	entry := cacheEntryFromResponse(resp)
	entry.Name = cr.name
	entry.Markdown = cr.markdown
	entry.OriginalHTML = cr.originalHTML
	entry.ReadabilityHTML = cr.readabilityHTML
	cache.storeHTTP(rawURL, entry, logger)
	logger.Info("cache_store", "url", rawURL)

	return fetchResult{
		name:            cr.name,
		markdown:        cr.markdown,
		source:          finalURL,
		originalHTML:    cr.originalHTML,
		readabilityHTML: cr.readabilityHTML,
	}, nil
}

// fetchURLPage fetches a URL asynchronously as a tea.Cmd.
func fetchURLPage(rawURL string, conv converter, cache *conversionCache, client httpClient, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		result, err := fetchURL(rawURL, conv, cache, client, logger)
		if err != nil {
			return pageLoadErrorMsg{url: rawURL, err: err}
		}
		return pageLoadedMsg{
			name:            result.name,
			markdown:        result.markdown,
			source:          result.source,
			originalHTML:    result.originalHTML,
			readabilityHTML: result.readabilityHTML,
		}
	}
}

// pageTitleFromURL extracts host + path as a display name.
func pageTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	title := u.Host + u.Path
	if title == "" {
		return rawURL
	}
	return title
}
