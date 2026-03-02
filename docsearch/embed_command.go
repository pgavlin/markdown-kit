package docsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CommandEmbedder generates embeddings by shelling out to an external command.
// The document text is passed on stdin; the command must write a JSON array of
// floats to stdout.
type CommandEmbedder struct {
	Command    string // shell command to run
	dimensions int
}

// NewCommandEmbedder creates a new CommandEmbedder.
func NewCommandEmbedder(command string, dimensions int) *CommandEmbedder {
	return &CommandEmbedder{
		Command:    command,
		dimensions: dimensions,
	}
}

func (e *CommandEmbedder) Dimensions() int { return e.dimensions }

func (e *CommandEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Split the command for exec. Use shell for complex commands.
	cmd := exec.CommandContext(ctx, "sh", "-c", e.Command)
	cmd.Stdin = strings.NewReader(text)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running command %q: %w (stderr: %s)", e.Command, err, stderr.String())
	}

	var embedding []float32
	if err := json.Unmarshal(stdout.Bytes(), &embedding); err != nil {
		return nil, fmt.Errorf("decoding command output: %w", err)
	}

	return embedding, nil
}
