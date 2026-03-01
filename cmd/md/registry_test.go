package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
)

func TestConverterRegistry_ForExtension(t *testing.T) {
	configs := []formatConverterConfig{
		{Extensions: []string{".rst", ".RST"}, Command: "pandoc-rst"},
		{Extensions: []string{".adoc"}, Command: "asciidoctor"},
	}
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, nil
		},
	})

	// Exact match.
	if c := r.forExtension(".rst"); c == nil {
		t.Error("expected converter for .rst")
	}

	// Case-insensitive lookup.
	if c := r.forExtension(".RST"); c == nil {
		t.Error("expected converter for .RST (case-insensitive)")
	}
	if c := r.forExtension(".Rst"); c == nil {
		t.Error("expected converter for .Rst (case-insensitive)")
	}

	// Unregistered extension.
	if c := r.forExtension(".txt"); c != nil {
		t.Error("expected nil for unregistered extension .txt")
	}
}

func TestConverterRegistry_ForMIMEType(t *testing.T) {
	configs := []formatConverterConfig{
		{MIMETypes: []string{"text/x-rst"}, Command: "pandoc-rst"},
	}
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, nil
		},
	})

	// Exact match.
	if c := r.forMIMEType("text/x-rst"); c == nil {
		t.Error("expected converter for text/x-rst")
	}

	// With parameters.
	if c := r.forMIMEType("text/x-rst; charset=utf-8"); c == nil {
		t.Error("expected converter for text/x-rst with charset parameter")
	}

	// Case-insensitive.
	if c := r.forMIMEType("Text/X-RST"); c == nil {
		t.Error("expected converter for Text/X-RST (case-insensitive)")
	}

	// Unregistered type.
	if c := r.forMIMEType("application/json"); c != nil {
		t.Error("expected nil for unregistered MIME type")
	}
}

func TestConverterRegistry_AllExtensions(t *testing.T) {
	configs := []formatConverterConfig{
		{Extensions: []string{".rst"}, Command: "pandoc-rst"},
		{Extensions: []string{".adoc", ".asciidoc"}, Command: "asciidoctor"},
	}
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, nil
		},
	})

	exts := r.allExtensions()
	sort.Strings(exts)
	want := []string{".adoc", ".asciidoc", ".rst"}
	if len(exts) != len(want) {
		t.Fatalf("allExtensions() = %v, want %v", exts, want)
	}
	for i := range want {
		if exts[i] != want[i] {
			t.Errorf("allExtensions()[%d] = %q, want %q", i, exts[i], want[i])
		}
	}
}

func TestConverterRegistry_FallbackOrder(t *testing.T) {
	// Two converters for .rst: first fails, second succeeds.
	configs := []formatConverterConfig{
		{Extensions: []string{".rst"}, Command: "bad-converter"},
		{Extensions: []string{".rst"}, Command: "good-converter"},
	}
	calls := 0
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			calls++
			if command == "bad-converter" {
				return nil, fmt.Errorf("bad converter failed")
			}
			return []byte("# Converted"), nil
		},
	})

	conv := r.forExtension(".rst")
	if conv == nil {
		t.Fatal("expected non-nil converter for .rst")
	}

	result, err := conv.convert([]byte("input"), nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# Converted" {
		t.Errorf("markdown = %q, want %q", result.markdown, "# Converted")
	}
	if calls != 2 {
		t.Errorf("expected 2 converter calls (first fails, second succeeds), got %d", calls)
	}
}

func TestConverterRegistry_FallbackAllFail(t *testing.T) {
	configs := []formatConverterConfig{
		{Extensions: []string{".rst"}, Command: "bad1"},
		{Extensions: []string{".rst"}, Command: "bad2"},
	}
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, fmt.Errorf("%s failed", command)
		},
	})

	conv := r.forExtension(".rst")
	_, err := conv.convert([]byte("input"), nil, discardLogger())
	if err == nil {
		t.Fatal("expected error when all converters fail")
	}
	// Last error should be from the last converter tried.
	if !strings.Contains(err.Error(), "bad2") {
		t.Errorf("error = %q, expected it to mention last converter", err)
	}
}

func TestConverterRegistry_FallbackMIMEType(t *testing.T) {
	configs := []formatConverterConfig{
		{MIMETypes: []string{"text/html"}, Command: "conv1"},
		{MIMETypes: []string{"text/html"}, Command: "conv2"},
	}
	calls := 0
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			calls++
			if command == "conv1" {
				return nil, fmt.Errorf("conv1 failed")
			}
			return []byte("# OK"), nil
		},
	})

	conv := r.forMIMEType("text/html")
	result, err := conv.convert([]byte("<html>"), nil, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.markdown != "# OK" {
		t.Errorf("markdown = %q", result.markdown)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestConverterRegistry_DuplicateExtNotRepeated(t *testing.T) {
	configs := []formatConverterConfig{
		{Extensions: []string{".rst"}, Command: "conv1"},
		{Extensions: []string{".rst"}, Command: "conv2"},
	}
	r := newConverterRegistry(configs, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, nil
		},
	})

	exts := r.allExtensions()
	count := 0
	for _, e := range exts {
		if e == ".rst" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("allExtensions() contains .rst %d times, want 1", count)
	}
}

func TestConverterRegistry_Nil(t *testing.T) {
	var r *converterRegistry

	if c := r.forExtension(".rst"); c != nil {
		t.Error("expected nil from nil registry forExtension")
	}
	if c := r.forMIMEType("text/x-rst"); c != nil {
		t.Error("expected nil from nil registry forMIMEType")
	}
	if exts := r.allExtensions(); exts != nil {
		t.Errorf("expected nil from nil registry allExtensions, got %v", exts)
	}
}

func TestConverterRegistry_EmptyConfigs(t *testing.T) {
	r := newConverterRegistry(nil, &fakeShellRunner{
		handler: func(command string, inputData []byte, inputEnvVar, outputEnvVar string, stderr io.Writer) ([]byte, error) {
			return nil, nil
		},
	})

	if r != nil {
		t.Error("expected nil registry for empty configs")
	}
}
