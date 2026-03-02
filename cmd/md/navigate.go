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
	newTab          bool   // when true, open in a new tab instead of current tab
	reload          bool   // when true, replace current content without pushing to back stack
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

// isConvertibleFile checks if a path has an extension handled by the registry.
func isConvertibleFile(path string, registry *converterRegistry) bool {
	if registry == nil {
		return false
	}
	if i := strings.IndexByte(path, '#'); i >= 0 {
		path = path[:i]
	}
	ext := strings.ToLower(filepath.Ext(path))
	return registry.forExtension(ext) != nil
}

// viewableExtsList returns markdown extensions plus converter extensions.
func viewableExtsList(registry *converterRegistry) []string {
	exts := markdownExtsList()
	exts = append(exts, registry.allExtensions()...)
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
func loadFilePage(path string, newTab bool, fsys fileSystem, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		data, err := fsys.ReadFile(path)
		if err != nil {
			logger.Error("file_read_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}
		logger.Info("file_read", "path", path, "size", len(data))
		return pageLoadedMsg{
			markdown: string(data),
			source:   path,
			newTab:   newTab,
		}
	}
}

// loadConvertFilePage reads a local file, converts it via the registry, and
// returns a pageLoadedMsg. Results are cached by content hash.
func loadConvertFilePage(path string, newTab bool, registry *converterRegistry, cache *conversionCache, fsys fileSystem, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		data, err := fsys.ReadFile(path)
		if err != nil {
			logger.Error("file_read_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}

		ext := strings.ToLower(filepath.Ext(path))
		conv := registry.forExtension(ext)
		if conv == nil {
			return pageLoadErrorMsg{url: path, err: fmt.Errorf("no converter for extension %q", ext)}
		}

		// Check cache.
		if cached, ok := cache.lookupFile(path, data, logger); ok {
			logger.Info("cache_hit", "path", path)
			return pageLoadedMsg{
				name:     cached.Name,
				markdown: cached.Markdown,
				source:   path,
				newTab:   newTab,
			}
		}

		logger.Info("converting_file", "path", path, "ext", ext)
		cr, err := conv.convert(data, nil, logger)
		if err != nil {
			logger.Error("file_convert_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}

		entry := cacheEntry{
			Name:     cr.name,
			Markdown: cr.markdown,
		}
		cache.storeFile(path, data, entry, logger)
		logger.Info("cache_store", "path", path)

		return pageLoadedMsg{
			name:     cr.name,
			markdown: cr.markdown,
			source:   path,
			newTab:   newTab,
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
func fetchURL(rawURL string, conv converter, registry *converterRegistry, cache *conversionCache, client httpClient, logger *slog.Logger) (fetchResult, error) {
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
	req.Header.Set("Accept", "text/markdown")

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

	// Check if the registry has a MIME-type-specific converter.
	activeConv := conv
	if mc := registry.forMIMEType(ct); mc != nil {
		activeConv = mc
	}

	pageURL, _ := url.Parse(finalURL)
	cr, err := activeConv.convert(body, pageURL, logger)
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
func fetchURLPage(rawURL string, newTab bool, conv converter, registry *converterRegistry, cache *conversionCache, client httpClient, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		result, err := fetchURL(rawURL, conv, registry, cache, client, logger)
		if err != nil {
			return pageLoadErrorMsg{url: rawURL, err: err}
		}
		return pageLoadedMsg{
			name:            result.name,
			markdown:        result.markdown,
			source:          result.source,
			originalHTML:    result.originalHTML,
			readabilityHTML: result.readabilityHTML,
			newTab:          newTab,
		}
	}
}

// reloadFilePage re-reads a local markdown file and returns a reload pageLoadedMsg.
func reloadFilePage(path string, fsys fileSystem, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		data, err := fsys.ReadFile(path)
		if err != nil {
			logger.Error("file_read_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}
		logger.Info("file_reload", "path", path, "size", len(data))
		return pageLoadedMsg{
			markdown: string(data),
			source:   path,
			reload:   true,
		}
	}
}

// reloadConvertFilePage re-reads and re-converts a local file, returning a reload pageLoadedMsg.
func reloadConvertFilePage(path string, registry *converterRegistry, cache *conversionCache, fsys fileSystem, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		data, err := fsys.ReadFile(path)
		if err != nil {
			logger.Error("file_read_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}

		ext := strings.ToLower(filepath.Ext(path))
		conv := registry.forExtension(ext)
		if conv == nil {
			return pageLoadErrorMsg{url: path, err: fmt.Errorf("no converter for extension %q", ext)}
		}

		// Check cache — content hash ensures stale conversions are skipped.
		if cached, ok := cache.lookupFile(path, data, logger); ok {
			logger.Info("cache_hit", "path", path)
			return pageLoadedMsg{
				name:     cached.Name,
				markdown: cached.Markdown,
				source:   path,
				reload:   true,
			}
		}

		logger.Info("converting_file", "path", path, "ext", ext)
		cr, err := conv.convert(data, nil, logger)
		if err != nil {
			logger.Error("file_convert_error", "path", path, "error", err)
			return pageLoadErrorMsg{url: path, err: err}
		}

		entry := cacheEntry{
			Name:     cr.name,
			Markdown: cr.markdown,
		}
		cache.storeFile(path, data, entry, logger)
		logger.Info("cache_store", "path", path)

		return pageLoadedMsg{
			name:     cr.name,
			markdown: cr.markdown,
			source:   path,
			reload:   true,
		}
	}
}

// reloadURLPage fetches a URL and returns a reload pageLoadedMsg.
func reloadURLPage(rawURL string, conv converter, registry *converterRegistry, cache *conversionCache, client httpClient, logger *slog.Logger) tea.Cmd {
	return func() tea.Msg {
		result, err := fetchURL(rawURL, conv, registry, cache, client, logger)
		if err != nil {
			return pageLoadErrorMsg{url: rawURL, err: err}
		}
		return pageLoadedMsg{
			name:            result.name,
			markdown:        result.markdown,
			source:          result.source,
			originalHTML:    result.originalHTML,
			readabilityHTML: result.readabilityHTML,
			reload:          true,
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
