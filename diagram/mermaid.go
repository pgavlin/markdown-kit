package diagram

import (
	"fmt"

	mermaidDiagram "github.com/pgavlin/mermaid-ascii/pkg/diagram"
	"github.com/pgavlin/mermaid-ascii/pkg/render"
	"github.com/pgavlin/markdown-kit/renderer"
)

// MermaidRenderer returns a DiagramRenderer that converts mermaid code blocks
// into Unicode box-drawing art using the mermaid-ascii library.
func MermaidRenderer() renderer.DiagramRenderer {
	return func(language string, source []byte) (string, error) {
		if language != "mermaid" {
			return "", fmt.Errorf("unsupported diagram language: %s", language)
		}
		config := mermaidDiagram.DefaultConfig()
		return render.Render(string(source), config)
	}
}
