package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/pgavlin/readability-go"
)

// converter converts raw content (typically HTML) into markdown.
type converter interface {
	convert(content []byte, sourceURL *url.URL) (convertResult, error)
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

func (c *builtinConverter) convert(content []byte, sourceURL *url.URL) (convertResult, error) {
	article, err := readability.ParseReader(strings.NewReader(string(content)), sourceURL, nil)
	if err != nil {
		return convertResult{}, fmt.Errorf("failed to parse page: %w", err)
	}
	if article == nil {
		return convertResult{}, fmt.Errorf("could not extract content from page")
	}

	name := article.Title
	if name == "" {
		name = pageTitleFromURL(sourceURL.String())
	}

	return convertResult{
		name:            name,
		markdown:        article.Markdown(),
		originalHTML:    string(content),
		readabilityHTML: article.Content,
	}, nil
}

// externalConverter runs an external command to convert content to markdown.
// The command receives the input via the MD_INPUT env var (path to a temp file)
// and writes output to the path in MD_OUTPUT.
type externalConverter struct {
	command []string
}

func (c *externalConverter) convert(content []byte, sourceURL *url.URL) (convertResult, error) {
	// Write input to a temp file.
	inputFile, err := os.CreateTemp("", "md-input-*")
	if err != nil {
		return convertResult{}, fmt.Errorf("creating temp input file: %w", err)
	}
	defer os.Remove(inputFile.Name())

	if _, err := inputFile.Write(content); err != nil {
		inputFile.Close()
		return convertResult{}, fmt.Errorf("writing temp input file: %w", err)
	}
	inputFile.Close()

	// Create a temp file path for output.
	outputFile, err := os.CreateTemp("", "md-output-*")
	if err != nil {
		return convertResult{}, fmt.Errorf("creating temp output file: %w", err)
	}
	outputFile.Close()
	defer os.Remove(outputFile.Name())

	// Run the external command.
	cmd := exec.Command(c.command[0], c.command[1:]...)
	cmd.Env = append(os.Environ(),
		"MD_INPUT="+inputFile.Name(),
		"MD_OUTPUT="+outputFile.Name(),
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return convertResult{}, fmt.Errorf("running converter command: %w", err)
	}

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
