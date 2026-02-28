package main

import (
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
)

func TestExternalConverter_Success(t *testing.T) {
	runner := &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return []byte("# Converted Output"), nil
		},
	}
	conv := &externalConverter{command: "convert", shell: runner}
	sourceURL, _ := url.Parse("http://example.com/page")

	result, err := conv.convert([]byte("<html>test</html>"), sourceURL, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Converted Output" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Converted Output")
	}
	if result.name != "example.com/page" {
		t.Errorf("name = %q, want %q", result.name, "example.com/page")
	}
}

func TestExternalConverter_CommandFailure(t *testing.T) {
	runner := &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, errors.New("exit status 1")
		},
	}
	conv := &externalConverter{command: "badcmd", shell: runner}
	sourceURL, _ := url.Parse("http://example.com")

	_, err := conv.convert([]byte("input"), sourceURL, discardLogger())
	if err == nil {
		t.Fatal("expected error from failed command")
	}
}

func TestExternalConverter_VerifiesInput(t *testing.T) {
	var capturedInput []byte
	runner := &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			capturedInput = inputData
			return []byte("output"), nil
		},
	}
	conv := &externalConverter{command: "convert", shell: runner}
	sourceURL, _ := url.Parse("http://example.com")

	input := []byte("<p>Hello, world!</p>")
	_, err := conv.convert(input, sourceURL, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(capturedInput) != string(input) {
		t.Errorf("captured input = %q, want %q", capturedInput, input)
	}
}

func TestExternalConverter_EnvVarNames(t *testing.T) {
	var capturedInputEnv, capturedOutputEnv string
	runner := &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			capturedInputEnv = inputEnvVar
			capturedOutputEnv = outputEnvVar
			return []byte("output"), nil
		},
	}
	conv := &externalConverter{command: "convert", shell: runner}
	sourceURL, _ := url.Parse("http://example.com")

	_, err := conv.convert([]byte("input"), sourceURL, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInputEnv != "MD_INPUT" {
		t.Errorf("input env var = %q, want %q", capturedInputEnv, "MD_INPUT")
	}
	if capturedOutputEnv != "MD_OUTPUT" {
		t.Errorf("output env var = %q, want %q", capturedOutputEnv, "MD_OUTPUT")
	}
}

func TestBuiltinConverter_Success(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<article>
<h1>Hello World</h1>
<p>This is a paragraph with enough text to be considered main content by readability extraction algorithms.</p>
<p>Another paragraph to ensure the content is substantial enough for extraction to succeed properly.</p>
</article>
</body>
</html>`
	sourceURL, _ := url.Parse("http://example.com/test")
	conv := &builtinConverter{}

	result, err := conv.convert([]byte(html), sourceURL, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.name == "" {
		t.Error("expected non-empty name")
	}
	if result.markdown == "" {
		t.Error("expected non-empty markdown")
	}
	if result.originalHTML == "" {
		t.Error("expected originalHTML to be set")
	}
	if result.readabilityHTML == "" {
		t.Error("expected readabilityHTML to be set")
	}
}

func TestBuiltinConverter_TitleFromURL(t *testing.T) {
	// Minimal HTML with no title — should fall back to URL-based title.
	html := `<html><body>
<article>
<p>Content paragraph one with sufficient text for readability.</p>
<p>Content paragraph two with more text for readability extraction.</p>
</article>
</body></html>`
	sourceURL, _ := url.Parse("http://example.com/docs/page")
	conv := &builtinConverter{}

	result, err := conv.convert([]byte(html), sourceURL, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When no HTML title, should use URL-based title.
	if !strings.Contains(result.name, "example.com") {
		t.Errorf("name = %q, expected URL-based fallback", result.name)
	}
}

func TestBuiltinConverter_InvalidHTML(t *testing.T) {
	// Completely empty content — readability should fail.
	sourceURL, _ := url.Parse("http://example.com")
	conv := &builtinConverter{}

	_, err := conv.convert([]byte(""), sourceURL, discardLogger())
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}
