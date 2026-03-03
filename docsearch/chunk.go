package docsearch

import (
	"regexp"
	"strings"
)

// chunk represents a section of a document for embedding.
type chunk struct {
	heading string // heading path context, e.g. "Section > Subsection"
	text    string // plain text content to embed
}

var reHeadingLine = regexp.MustCompile(`^(#{1,6})\s+(.+)`)

// chunkMarkdown splits markdown into heading-based sections, strips formatting,
// and returns chunks suitable for embedding. Each chunk includes the heading
// hierarchy as context. Chunks exceeding maxLen are split into overlapping windows.
func chunkMarkdown(markdown string, maxLen int) []chunk {
	if maxLen <= 0 {
		maxLen = 2000
	}

	lines := strings.Split(markdown, "\n")

	type section struct {
		level   int
		heading string
		lines   []string
	}

	var sections []section
	current := section{level: 0, heading: ""}

	for _, line := range lines {
		if m := reHeadingLine.FindStringSubmatch(line); m != nil {
			// Save previous section.
			sections = append(sections, current)

			level := len(m[1])
			heading := stripMarkdown(strings.TrimSpace(m[2]))
			current = section{level: level, heading: heading}
			continue
		}
		current.lines = append(current.lines, line)
	}
	// Save the last section.
	sections = append(sections, current)

	// If there are no headings (only section 0 with no heading), treat the
	// entire document as one chunk.
	if len(sections) == 1 && sections[0].heading == "" {
		text := stripMarkdown(markdown)
		if text == "" {
			return nil
		}
		return splitChunk(chunk{text: text}, maxLen)
	}

	// Build heading hierarchy and create chunks.
	// headingStack tracks the current heading path by level.
	headingStack := make([]string, 7) // levels 1-6, index 0 unused

	var chunks []chunk
	for _, sec := range sections {
		if sec.heading == "" && len(sec.lines) == 0 {
			continue
		}

		// Update heading stack: a heading of level N clears all deeper levels.
		if sec.level > 0 {
			headingStack[sec.level] = sec.heading
			for i := sec.level + 1; i <= 6; i++ {
				headingStack[i] = ""
			}
		}

		// Build the heading path.
		var pathParts []string
		for i := 1; i <= 6; i++ {
			if headingStack[i] != "" {
				pathParts = append(pathParts, headingStack[i])
			}
			if i == sec.level {
				break
			}
		}
		headingPath := strings.Join(pathParts, " > ")

		// Strip markdown from the section body.
		body := stripMarkdown(strings.Join(sec.lines, "\n"))
		if body == "" {
			continue
		}

		// Prepend heading path for context.
		var text string
		if headingPath != "" {
			text = headingPath + ":\n" + body
		} else {
			text = body
		}

		chunks = append(chunks, splitChunk(chunk{heading: headingPath, text: text}, maxLen)...)
	}

	if len(chunks) == 0 {
		// All sections were empty after stripping; fall back to whole-document.
		text := stripMarkdown(markdown)
		if text == "" {
			return nil
		}
		return splitChunk(chunk{text: text}, maxLen)
	}

	return chunks
}

// splitChunk splits a chunk into overlapping windows if it exceeds maxLen.
func splitChunk(c chunk, maxLen int) []chunk {
	if len(c.text) <= maxLen {
		return []chunk{c}
	}

	overlap := maxLen / 5 // ~20% overlap
	var chunks []chunk
	text := c.text

	for len(text) > 0 {
		end := maxLen
		if end > len(text) {
			end = len(text)
		}

		// Try to break at a newline or space boundary.
		if end < len(text) {
			if idx := strings.LastIndex(text[:end], "\n"); idx > maxLen/2 {
				end = idx + 1
			} else if idx := strings.LastIndex(text[:end], " "); idx > maxLen/2 {
				end = idx + 1
			}
		}

		chunks = append(chunks, chunk{heading: c.heading, text: text[:end]})
		advance := end - overlap
		if advance <= 0 {
			advance = end
		}
		if advance >= len(text) {
			break
		}
		text = text[advance:]
	}

	return chunks
}
