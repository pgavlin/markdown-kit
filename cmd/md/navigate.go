package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/pgavlin/readability-go"
)

// pageLoadedMsg is sent when a page has been successfully loaded.
type pageLoadedMsg struct {
	name     string
	markdown string
	source   string
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
func loadFilePage(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return pageLoadErrorMsg{url: path, err: err}
		}
		return pageLoadedMsg{
			name:     filepath.Base(path),
			markdown: string(data),
			source:   path,
		}
	}
}

// fetchURLPage fetches a URL. If the content is markdown, it's used directly.
// If it's HTML, it's converted to markdown via readability.
func fetchURLPage(rawURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(rawURL)
		if err != nil {
			return pageLoadErrorMsg{url: rawURL, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return pageLoadErrorMsg{
				url: rawURL,
				err: fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status),
			}
		}

		// Use the final URL after redirects.
		finalURL := resp.Request.URL.String()
		ct := resp.Header.Get("Content-Type")

		if isMarkdownContentType(ct) {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return pageLoadErrorMsg{url: rawURL, err: err}
			}
			return pageLoadedMsg{
				name:     pageTitleFromURL(finalURL),
				markdown: string(body),
				source:   finalURL,
			}
		}

		// Try readability extraction from HTML.
		pageURL, _ := url.Parse(finalURL)
		article, err := readability.ParseReader(resp.Body, pageURL, nil)
		if err != nil {
			return pageLoadErrorMsg{url: rawURL, err: fmt.Errorf("failed to parse page: %w", err)}
		}
		if article == nil {
			return pageLoadErrorMsg{url: rawURL, err: fmt.Errorf("could not extract content from page")}
		}

		md := article.Markdown()
		name := article.Title
		if name == "" {
			name = pageTitleFromURL(finalURL)
		}

		return pageLoadedMsg{
			name:     name,
			markdown: md,
			source:   finalURL,
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
