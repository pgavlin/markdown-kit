package docsearch

import (
	"regexp"
	"strings"
)

var (
	// Heading prefixes: lines starting with 1-6 '#' chars followed by a space.
	reHeadingPrefix = regexp.MustCompile(`^#{1,6}\s+`)

	// Setext heading underlines: lines of only '=' or '-' (at least 3).
	reSetextUnderline = regexp.MustCompile(`^(?:={3,}|-{3,})\s*$`)

	// Code fence lines: ``` or ~~~ optionally followed by a language tag.
	reCodeFence = regexp.MustCompile("^(?:```|~~~)")

	// Links: [text](url) — capture the text.
	reLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

	// Images: ![alt](url) — capture the alt text.
	reImage = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)

	// HTML tags.
	reHTMLTag = regexp.MustCompile(`<[^>]+>`)

	// Table alignment rows: lines like |---|---|, | :---: | --- |, etc.
	reTableAlignRow = regexp.MustCompile(`^\|?[\s:]*-{3,}[\s:]*(\|[\s:]*-{3,}[\s:]*)*\|?\s*$`)

	// Bold/italic markers: **, __, *, _, ~~
	reEmphasis = regexp.MustCompile(`(\*{1,2}|_{1,2}|~~)`)
)

// stripMarkdown removes markdown formatting from the given text, returning
// plain text suitable for embedding. It operates line-by-line without
// requiring a full markdown parser.
func stripMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var result []string
	inCodeBlock := false
	lastBlank := false

	for _, line := range lines {
		// Handle code fences.
		if reCodeFence.MatchString(strings.TrimSpace(line)) {
			inCodeBlock = !inCodeBlock
			continue
		}

		// Inside code blocks, keep the content as-is.
		if inCodeBlock {
			result = append(result, line)
			lastBlank = false
			continue
		}

		// Remove setext underlines.
		if reSetextUnderline.MatchString(line) {
			continue
		}

		// Remove table alignment rows.
		if reTableAlignRow.MatchString(line) {
			continue
		}

		// Strip heading prefixes.
		line = reHeadingPrefix.ReplaceAllString(line, "")

		// Strip images before links (images contain links).
		line = reImage.ReplaceAllString(line, "$1")

		// Strip links.
		line = reLink.ReplaceAllString(line, "$1")

		// Strip inline code backticks.
		line = strings.ReplaceAll(line, "`", "")

		// Strip emphasis markers.
		line = reEmphasis.ReplaceAllString(line, "")

		// Strip HTML tags.
		line = reHTMLTag.ReplaceAllString(line, "")

		// Strip table pipes.
		if strings.Contains(line, "|") {
			line = strings.ReplaceAll(line, "|", " ")
			// Collapse multiple spaces from pipe removal.
			for strings.Contains(line, "  ") {
				line = strings.ReplaceAll(line, "  ", " ")
			}
		}

		line = strings.TrimSpace(line)

		// Collapse multiple blank lines.
		if line == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
		} else {
			lastBlank = false
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}
