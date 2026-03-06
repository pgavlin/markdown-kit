package main

import (
	"errors"
	"io"
	"net/url"
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

