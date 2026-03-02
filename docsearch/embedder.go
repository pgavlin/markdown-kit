package docsearch

import "context"

// Embedder generates vector embeddings for text.
type Embedder interface {
	// Embed returns the vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the number of dimensions in the embedding vectors.
	Dimensions() int
}
