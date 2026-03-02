package docsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaEmbedder generates embeddings using the Ollama API.
type OllamaEmbedder struct {
	URL        string // e.g. "http://localhost:11434"
	Model      string // e.g. "nomic-embed-text"
	dimensions int
}

// NewOllamaEmbedder creates a new OllamaEmbedder.
func NewOllamaEmbedder(url, model string, dimensions int) *OllamaEmbedder {
	return &OllamaEmbedder{
		URL:        url,
		Model:      model,
		dimensions: dimensions,
	}
}

func (e *OllamaEmbedder) Dimensions() int { return e.dimensions }

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body := map[string]any{
		"model":  e.Model,
		"prompt": text,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := e.URL + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("no embedding in response")
	}

	return result.Embedding, nil
}
