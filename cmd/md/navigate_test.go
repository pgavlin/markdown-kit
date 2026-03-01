package main

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResolveLink(t *testing.T) {
	tests := []struct {
		name    string
		link    string
		source  string
		want    string
	}{
		{"absolute_url", "https://other.com/page", "https://example.com/doc.md", "https://other.com/page"},
		{"relative_to_url", "page2.md", "https://example.com/docs/page1.md", "https://example.com/docs/page2.md"},
		{"relative_to_file", "other.md", "/home/user/docs/readme.md", "/home/user/docs/other.md"},
		{"absolute_path", "/abs/path.md", "/home/user/docs/readme.md", "/abs/path.md"},
		{"empty_source", "page.md", "", "page.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLink(tt.link, tt.source)
			if got != tt.want {
				t.Errorf("resolveLink(%q, %q) = %q, want %q", tt.link, tt.source, got, tt.want)
			}
		})
	}
}

func TestIsMarkdownFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"file.md", true},
		{"file.markdown", true},
		{"file.txt", false},
		{"file.html", false},
		{"file.md#heading", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isMarkdownFile(tt.path); got != tt.want {
				t.Errorf("isMarkdownFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsMarkdownContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/markdown", true},
		{"text/x-markdown", true},
		{"text/html", false},
		{"application/json", false},
	}
	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			if got := isMarkdownContentType(tt.ct); got != tt.want {
				t.Errorf("isMarkdownContentType(%q) = %v, want %v", tt.ct, got, tt.want)
			}
		})
	}
}

func TestPageTitleFromURL(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{"https://example.com/docs/page", "example.com/docs/page"},
		{"not-a-url", "not-a-url"},
	}
	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			if got := pageTitleFromURL(tt.rawURL); got != tt.want {
				t.Errorf("pageTitleFromURL(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestFetchURL_CacheFresh(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	now := time.Now().UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:         "cached page",
		Markdown:     "# Cached",
		CacheControl: "max-age=3600",
		Date:         now,
	}
	c.store("http://example.com", entry, logger)

	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			t.Fatal("HTTP client should not be called for fresh cache hit")
			return nil, nil
		},
	}

	result, err := fetchURL("http://example.com", &builtinConverter{}, nil, c, client, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Cached" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Cached")
	}
}

func TestFetchURL_CacheStale_304(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	pastDate := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:         "stale page",
		Markdown:     "# Stale",
		CacheControl: "max-age=60",
		Date:         pastDate,
		ETag:         "\"abc\"",
	}
	c.store("http://example.com", entry, logger)

	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     http.Header{"Date": {time.Now().UTC().Format(time.RFC1123)}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		},
	}

	result, err := fetchURL("http://example.com", &builtinConverter{}, nil, c, client, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Stale" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Stale")
	}
}

func TestFetchURL_MarkdownResponse(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# Hello")),
				Request:    req,
			}, nil
		},
	}

	result, err := fetchURL("http://example.com/doc.md", &builtinConverter{}, nil, nil, client, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Hello" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Hello")
	}
}

func TestFetchURL_HTMLResponse(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html><body>Hello</body></html>")),
				Request:    req,
			}, nil
		},
	}

	conv := &fakeConverter{
		result: convertResult{name: "Hello Page", markdown: "# Hello"},
	}

	result, err := fetchURL("http://example.com", conv, nil, nil, client, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Hello" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Hello")
	}
	if result.name != "Hello Page" {
		t.Errorf("name = %q, want %q", result.name, "Hello Page")
	}
}

func TestFetchURL_HTTPError(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		},
	}

	_, err := fetchURL("http://example.com", &builtinConverter{}, nil, nil, client, discardLogger())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchURL_NetworkError(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	}

	_, err := fetchURL("http://example.com", &builtinConverter{}, nil, nil, client, discardLogger())
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestFetchURL_NilCache(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# No Cache")),
				Request:    req,
			}, nil
		},
	}

	result, err := fetchURL("http://example.com", &builtinConverter{}, nil, nil, client, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# No Cache" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# No Cache")
	}
}

func TestFetchURL_ConditionalHeaders(t *testing.T) {
	fs := newMemFS()
	c := &conversionCache{dir: "/cache", fs: fs}
	logger := discardLogger()

	pastDate := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC1123)
	entry := cacheEntry{
		Name:         "page",
		Markdown:     "# Page",
		CacheControl: "max-age=60",
		Date:         pastDate,
		ETag:         "\"etag123\"",
		LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
	}
	c.store("http://example.com", entry, logger)

	var capturedReq *http.Request
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			capturedReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# Updated")),
				Request:    req,
			}, nil
		},
	}

	_, err := fetchURL("http://example.com", &builtinConverter{}, nil, c, client, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq == nil {
		t.Fatal("expected HTTP request to be made")
	}
	if got := capturedReq.Header.Get("If-None-Match"); got != "\"etag123\"" {
		t.Errorf("If-None-Match = %q, want %q", got, "\"etag123\"")
	}
	if got := capturedReq.Header.Get("If-Modified-Since"); got != "Mon, 01 Jan 2024 00:00:00 GMT" {
		t.Errorf("If-Modified-Since = %q, want %q", got, "Mon, 01 Jan 2024 00:00:00 GMT")
	}
}

func TestFetchURL_Redirect(t *testing.T) {
	// Simulate a redirect: the response's Request.URL differs from the original.
	redirectedURL, _ := url.Parse("https://example.com/final-page")
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# Redirected")),
				Request:    &http.Request{URL: redirectedURL},
			}, nil
		},
	}

	result, err := fetchURL("http://example.com/old-page", &builtinConverter{}, nil, nil, client, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.source != "https://example.com/final-page" {
		t.Errorf("source = %q, want final redirect URL", result.source)
	}
	if result.markdown != "# Redirected" {
		t.Errorf("markdown = %q", result.markdown)
	}
}

func TestFetchURL_ConverterError(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html>test</html>")),
				Request:    req,
			}, nil
		},
	}
	conv := &fakeConverter{err: errors.New("conversion failed")}

	_, err := fetchURL("http://example.com", conv, nil, nil, client, discardLogger())
	if err == nil {
		t.Fatal("expected error from failed converter")
	}
}

func TestFetchURLPage_Success(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/markdown"}},
				Body:       io.NopCloser(strings.NewReader("# Page")),
				Request:    req,
			}, nil
		},
	}

	cmd := fetchURLPage("http://example.com/doc.md", false, &builtinConverter{}, nil, nil, client, discardLogger())
	msg := cmd()

	loaded, ok := msg.(pageLoadedMsg)
	if !ok {
		t.Fatalf("expected pageLoadedMsg, got %T", msg)
	}
	if loaded.markdown != "# Page" {
		t.Errorf("markdown = %q", loaded.markdown)
	}
	if loaded.source != "http://example.com/doc.md" {
		t.Errorf("source = %q", loaded.source)
	}
	if loaded.newTab {
		t.Error("expected newTab=false")
	}
}

func TestFetchURLPage_Error(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network error")
		},
	}

	cmd := fetchURLPage("http://example.com", false, &builtinConverter{}, nil, nil, client, discardLogger())
	msg := cmd()

	errMsg, ok := msg.(pageLoadErrorMsg)
	if !ok {
		t.Fatalf("expected pageLoadErrorMsg, got %T", msg)
	}
	if errMsg.url != "http://example.com" {
		t.Errorf("url = %q", errMsg.url)
	}
}

func TestFetchURLPage_HTMLContent(t *testing.T) {
	client := &fakeHTTPClient{
		handler: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html>body</html>")),
				Request:    req,
			}, nil
		},
	}
	conv := &fakeConverter{
		result: convertResult{
			name:            "Converted",
			markdown:        "# Converted",
			originalHTML:    "<html>body</html>",
			readabilityHTML: "<article>body</article>",
		},
	}

	cmd := fetchURLPage("http://example.com", false, conv, nil, nil, client, discardLogger())
	msg := cmd()

	loaded, ok := msg.(pageLoadedMsg)
	if !ok {
		t.Fatalf("expected pageLoadedMsg, got %T", msg)
	}
	if loaded.originalHTML != "<html>body</html>" {
		t.Errorf("originalHTML = %q", loaded.originalHTML)
	}
	if loaded.readabilityHTML != "<article>body</article>" {
		t.Errorf("readabilityHTML = %q", loaded.readabilityHTML)
	}
}

func TestLoadFilePage_Success(t *testing.T) {
	fs := newMemFS()
	fs.files["/docs/readme.md"] = []byte("# Hello World")

	cmd := loadFilePage("/docs/readme.md", false, fs, discardLogger())
	msg := cmd()

	loaded, ok := msg.(pageLoadedMsg)
	if !ok {
		t.Fatalf("expected pageLoadedMsg, got %T", msg)
	}
	if loaded.markdown != "# Hello World" {
		t.Errorf("markdown = %q, want %q", loaded.markdown, "# Hello World")
	}
	if loaded.name != "" {
		t.Errorf("name = %q, want empty (inferred from heading by SetText)", loaded.name)
	}
	if loaded.source != "/docs/readme.md" {
		t.Errorf("source = %q, want %q", loaded.source, "/docs/readme.md")
	}
}

func TestLoadFilePage_Error(t *testing.T) {
	fs := newMemFS()

	cmd := loadFilePage("/docs/missing.md", false, fs, discardLogger())
	msg := cmd()

	errMsg, ok := msg.(pageLoadErrorMsg)
	if !ok {
		t.Fatalf("expected pageLoadErrorMsg, got %T", msg)
	}
	if errMsg.url != "/docs/missing.md" {
		t.Errorf("url = %q, want %q", errMsg.url, "/docs/missing.md")
	}
	if errMsg.err == nil {
		t.Error("expected non-nil error")
	}
}
