package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"
)

// converter converts raw content (typically HTML) into markdown.
type converter interface {
	convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error)
}

// convertResult holds the output of a content conversion.
type convertResult struct {
	name     string
	markdown string
}

// fallbackConverter tries multiple converters in order, returning the first
// successful result.
type fallbackConverter struct {
	converters []converter
}

func (f *fallbackConverter) convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error) {
	var lastErr error
	for _, c := range f.converters {
		result, err := c.convert(content, sourceURL, logger)
		if err == nil {
			return result, nil
		}
		lastErr = err
		logger.Info("converter_fallback", "error", err)
	}
	return convertResult{}, lastErr
}

// externalConverter runs an external command via the system shell to convert
// content to markdown. The command receives the input via the MD_INPUT env var
// (path to a temp file) and writes output to the path in MD_OUTPUT.
type externalConverter struct {
	command string
	shell   shellRunner
}

func (c *externalConverter) convert(content []byte, sourceURL *url.URL, logger *slog.Logger) (convertResult, error) {
	logger.Info("external_converter_start", "command", c.command)
	start := time.Now()

	output, err := c.shell.Run(c.command, content, "MD_INPUT", "MD_OUTPUT", os.Stderr)
	if err != nil {
		logger.Error("external_converter_error", "command", c.command, "error", err)
		return convertResult{}, fmt.Errorf("running converter command: %w", err)
	}

	logger.Info("external_converter_done", "command", c.command, "duration", time.Since(start))

	var name string
	if sourceURL != nil {
		name = pageTitleFromURL(sourceURL.String())
	}

	return convertResult{
		name:     name,
		markdown: string(output),
	}, nil
}
