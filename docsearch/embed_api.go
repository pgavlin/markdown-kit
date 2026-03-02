package docsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIEmbedder generates embeddings using an OpenAI-compatible embeddings API.
type APIEmbedder struct {
	URL        string // e.g. "https://api.openai.com/v1/embeddings"
	Model      string // e.g. "text-embedding-3-small"
	APIKey     string
	dimensions int
}

// NewAPIEmbedder creates a new APIEmbedder.
func NewAPIEmbedder(url, model, apiKey string, dimensions int) *APIEmbedder {
	return &APIEmbedder{
		URL:        url,
		Model:      model,
		APIKey:     apiKey,
		dimensions: dimensions,
	}
}

func (e *APIEmbedder) Dimensions() int { return e.dimensions }

func (e *APIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body := map[string]any{
		"input": text,
		"model": e.Model,
	}
	if e.dimensions > 0 {
		body["dimensions"] = e.dimensions
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embeddings in response")
	}

	return result.Data[0].Embedding, nil
}
