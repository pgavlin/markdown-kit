package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/pgavlin/readability-go"
)

// converter converts raw content (typically HTML) into markdown.
type converter interface {
	convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error)
}

// convertResult holds the output of a content conversion.
type convertResult struct {
	name            string
	markdown        string
	originalHTML    string
	readabilityHTML string
}

// builtinConverter uses readability extraction to convert HTML to markdown.
type builtinConverter struct{}

func (c *builtinConverter) convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error) {
	article, err := readability.ParseReader(strings.NewReader(string(content)), sourceURL, nil)
	if err != nil {
		logger.Error("readability_convert_error", "url", sourceURL.String(), "error", err)
		return convertResult{}, fmt.Errorf("failed to parse page: %w", err)
	}
	if article == nil {
		logger.Error("readability_convert_error", "url", sourceURL.String(), "error", "could not extract content")
		return convertResult{}, fmt.Errorf("could not extract content from page")
	}

	name := article.Title
	if name == "" {
		name = pageTitleFromURL(sourceURL.String())
	}

	logger.Info("readability_convert", "url", sourceURL.String(), "title", name)

	return convertResult{
		name:            name,
		markdown:        article.Markdown(),
		originalHTML:    string(content),
		readabilityHTML: article.Content,
	}, nil
}

// externalConverter runs an external command via the system shell to convert
// content to markdown. The command receives the input via the MD_INPUT env var
// (path to a temp file) and writes output to the path in MD_OUTPUT.
type externalConverter struct {
	command string
}

func (c *externalConverter) convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error) {
	// Write input to a temp file.
	inputFile, err := os.CreateTemp("", "md-input-*")
	if err != nil {
		logger.Error("temp_file_error", "op", "create", "error", err)
		return convertResult{}, fmt.Errorf("creating temp input file: %w", err)
	}
	defer os.Remove(inputFile.Name())

	if _, err := inputFile.Write(content); err != nil {
		inputFile.Close()
		logger.Error("temp_file_error", "op", "write", "error", err)
		return convertResult{}, fmt.Errorf("writing temp input file: %w", err)
	}
	inputFile.Close()

	// Create a temp file path for output.
	outputFile, err := os.CreateTemp("", "md-output-*")
	if err != nil {
		logger.Error("temp_file_error", "op", "create", "error", err)
		return convertResult{}, fmt.Errorf("creating temp output file: %w", err)
	}
	outputFile.Close()
	defer os.Remove(outputFile.Name())

	// Run the command via the system shell.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", c.command)
	} else {
		cmd = exec.Command("sh", "-c", c.command)
	}
	cmd.Env = append(os.Environ(),
		"MD_INPUT="+inputFile.Name(),
		"MD_OUTPUT="+outputFile.Name(),
	)
	cmd.Stderr = os.Stderr

	logger.Info("external_converter_start", "command", c.command)
	start := time.Now()

	if err := cmd.Run(); err != nil {
		logger.Error("external_converter_error", "command", c.command, "error", err)
		return convertResult{}, fmt.Errorf("running converter command: %w", err)
	}

	logger.Info("external_converter_done", "command", c.command, "duration", time.Since(start))

	// Read the output.
	output, err := os.ReadFile(outputFile.Name())
	if err != nil {
		return convertResult{}, fmt.Errorf("reading converter output: %w", err)
	}

	return convertResult{
		name:     pageTitleFromURL(sourceURL.String()),
		markdown: string(output),
	}, nil
}
