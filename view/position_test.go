package view

import (
	"strings"
	"testing"

	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPositionModel(t *testing.T, md string) *Model {
	t.Helper()
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", md)
	return &m
}

func TestPosition_RoundTripIdenticalContent(t *testing.T) {
	md := "# Top\n\n## Setup\n\nUnique line for setup section.\n\n## Usage\n\nBody.\n"
	m := newPositionModel(t, md)
	// Use cursor mode so position is preserved on a small doc where
	// scroll would be clamped to 0.
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "Unique line") {
			m.cursorLine = i
			break
		}
	}
	require.Greater(t, m.cursorLine, 0)

	pre := m.Position()
	require.NotEmpty(t, pre.HeadingAnchor, "should capture an anchor under Setup")

	// Re-set the same content; AST nodes are fresh but anchors and
	// snippet text are identical.
	m.SetText("doc.md", md)
	m.RestorePosition(pre)

	post := m.Position()
	assert.Equal(t, pre.HeadingAnchor, post.HeadingAnchor,
		"identical-content reload preserves heading anchor")
	assert.Equal(t, pre.LineSnippet, post.LineSnippet,
		"identical-content reload preserves line snippet")
}

func TestPosition_HeadingPreservedAcrossEdit(t *testing.T) {
	original := "# Top\n\n## Setup\n\nSetup body.\n\n## Usage\n\nUsage body.\n"
	edited := "# Top\n\n## Background\n\nNew section.\n\n## Setup\n\nSetup body.\n\n## Usage\n\nUsage body.\n"

	m := newPositionModel(t, original)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "Setup body") {
			m.cursorLine = i
			break
		}
	}
	require.Greater(t, m.cursorLine, 0)
	pre := m.Position()
	require.Equal(t, "setup", pre.HeadingAnchor)

	// Reload edited content (which inserted a Background section).
	m.SetText("doc.md", edited)
	m.RestorePosition(pre)

	// The cursor should be inside the new "Setup" section, even though
	// its absolute line index moved.
	require.True(t, m.cursorPositioned)
	heading, _ := m.enclosingHeading(m.cursorLine)
	require.NotNil(t, heading)
	assert.Equal(t, "Setup", string(heading.Text(m.markdown)))
}

func TestPosition_HeadingDeletedFallsBackGracefully(t *testing.T) {
	original := "# Top\n\n## Setup\n\nA unique-string-only-here line.\n\n## Usage\n\nBody.\n"
	m := newPositionModel(t, original)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "unique-string-only-here") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	require.Equal(t, "setup", pre.HeadingAnchor)

	// Reload with Setup removed entirely. The unique line is gone too.
	m.SetText("doc.md", "# Top\n\n## Usage\n\nBody.\n")
	m.RestorePosition(pre)

	// Anchor missing AND snippet missing → cursor at top, lineOffset 0.
	assert.Equal(t, 0, m.lineOffset,
		"deleted heading + missing snippet falls back to top of doc")
}

func TestPosition_SnippetRefinementWithinSection(t *testing.T) {
	original := "# Top\n\n## Section\n\nfirst line\n\nsecond unique line\n\nthird line\n"
	edited := "# Top\n\n## Section\n\nadded first\n\nfirst line\n\nsecond unique line\n\nthird line\n"

	m := newPositionModel(t, original)
	// Enable cursor mode so RestorePosition restores cursorLine
	// independent of viewport-clamp behavior on short documents.
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "second unique line") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	require.NotEmpty(t, pre.LineSnippet)

	m.SetText("doc.md", edited)
	m.RestorePosition(pre)

	// Cursor lands on the same content line, not on the added line.
	require.True(t, m.cursorPositioned)
	require.Less(t, m.cursorLine, len(m.lines))
	assert.Contains(t, m.lines[m.cursorLine].content, "second unique line",
		"snippet refinement should land on the original line, not the added one")
}

func TestPosition_DuplicateAnchorsDisambiguated(t *testing.T) {
	md := "# Top\n\n## Notes\n\nfirst notes.\n\n## Other\n\nbody.\n\n## Notes\n\nsecond notes.\n"
	m := newPositionModel(t, md)
	m.cursorMode = true
	m.cursorPositioned = true

	// Position under the SECOND "Notes" section.
	var second int
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "second notes") {
			second = i
			break
		}
	}
	require.Greater(t, second, 0)
	m.cursorLine = second
	pre := m.Position()
	require.Equal(t, "notes", pre.HeadingAnchor)
	require.Equal(t, 1, pre.HeadingIndex,
		"second 'Notes' should be HeadingIndex 1 within the same-anchor sections")

	// Reload identical content; restore should pick the second Notes.
	m.SetText("doc.md", md)
	m.RestorePosition(pre)
	require.True(t, m.cursorPositioned)
	heading, _ := m.enclosingHeading(m.cursorLine)
	require.NotNil(t, heading)
	assert.Equal(t, "Notes", string(heading.Text(m.markdown)))
	// And it should be the second one — verify by checking we landed
	// near the "second notes" line.
	assert.Contains(t, m.lines[m.cursorLine].content, "second notes",
		"duplicate-anchor disambiguation picks the right occurrence")
}

func TestPosition_DocWithoutHeadings(t *testing.T) {
	md := "first line\n\nsecond line\n\nthird unique line\n\nfourth line\n"
	m := newPositionModel(t, md)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "third unique line") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	assert.Empty(t, pre.HeadingAnchor)
	assert.NotEmpty(t, pre.LineSnippet)

	m.SetText("doc.md", md)
	m.RestorePosition(pre)
	require.True(t, m.cursorPositioned)
	assert.Contains(t, m.lines[m.cursorLine].content, "third unique line",
		"no-heading documents fall back to whole-doc snippet match")
}

func TestPosition_EmptyReloadIsSafe(t *testing.T) {
	m := newPositionModel(t, "# Body\n\nfiller\n")
	m.lineOffset = 1
	pre := m.Position()
	m.SetText("doc.md", "")
	assert.NotPanics(t, func() { m.RestorePosition(pre) })
}

func TestPosition_IsZero(t *testing.T) {
	var z Position
	assert.True(t, z.IsZero())
	assert.False(t, Position{HeadingAnchor: "x"}.IsZero())
	assert.False(t, Position{LineSnippet: "x"}.IsZero())
	assert.False(t, Position{CursorMode: true}.IsZero())
}

func TestPosition_CursorPreservedAcrossReload(t *testing.T) {
	md := "# Top\n\n## Setup\n\nfirst\n\nsecond\n\nthird\n"
	m := newPositionModel(t, md)
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "second") {
			m.cursorLine = i
			break
		}
	}
	m.cursorMode = true
	m.cursorPositioned = true
	m.cursorCol = 3

	pre := m.Position()
	require.True(t, pre.CursorMode)
	require.True(t, pre.CursorPositioned)
	require.Equal(t, 3, pre.CursorCol)

	m.SetText("doc.md", md)
	m.RestorePosition(pre)
	assert.True(t, m.cursorMode)
	assert.True(t, m.cursorPositioned)
	assert.Equal(t, 3, m.cursorCol)
	assert.Contains(t, m.lines[m.cursorLine].content, "second",
		"cursor line should re-resolve to the same content")
}

// Anchor matches but the snippet is gone — the user's specific line was
// deleted but the section still exists. RestorePosition should leave
// the cursor at the heading's first line of the matched section.
func TestPosition_AnchorFoundSnippetMissing(t *testing.T) {
	original := "# Top\n\n## Setup\n\nuniquely-named-line\n\n## Usage\n\nbody\n"
	edited := "# Top\n\n## Setup\n\ndifferent content here\n\n## Usage\n\nbody\n"

	m := newPositionModel(t, original)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "uniquely-named-line") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	require.Equal(t, "setup", pre.HeadingAnchor)
	require.Equal(t, "uniquely-named-line", pre.LineSnippet)

	m.SetText("doc.md", edited)
	m.RestorePosition(pre)

	// Anchor "setup" exists in the edited doc → cursor lands inside
	// the Setup section even though the snippet line is gone.
	require.True(t, m.cursorPositioned)
	heading, _ := m.enclosingHeading(m.cursorLine)
	require.NotNil(t, heading)
	assert.Equal(t, "Setup", string(heading.Text(m.markdown)),
		"cursor should land inside Setup section despite missing snippet")
}

// Anchor missing but snippet present elsewhere in the document — the
// section was deleted but the line moved into another section.
func TestPosition_AnchorMissingSnippetMatchesElsewhere(t *testing.T) {
	original := "# Top\n\n## Old Section\n\nThe-portable-line is here.\n\n## Other\n\n"
	// Old Section is gone, but the snippet line was moved under Other.
	edited := "# Top\n\n## Other\n\nThe-portable-line is here.\n\nMore body.\n"

	m := newPositionModel(t, original)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "The-portable-line") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	require.Equal(t, "old-section", pre.HeadingAnchor)
	require.Equal(t, "The-portable-line is here.", pre.LineSnippet)

	m.SetText("doc.md", edited)
	m.RestorePosition(pre)

	// Old Section was deleted; whole-doc snippet sweep finds the line
	// under "Other".
	require.True(t, m.cursorPositioned)
	assert.Contains(t, m.lines[m.cursorLine].content, "The-portable-line",
		"whole-doc snippet sweep should pick up the moved line")
}

// HeadingIndex out of range after edits (e.g., one of two duplicate
// sections is removed). Restore should clamp to a valid index instead
// of panicking or falling through to top of doc.
func TestPosition_HeadingIndexOutOfRange(t *testing.T) {
	original := "# Top\n\n## Notes\n\nfirst notes.\n\n## Notes\n\nsecond notes.\n"
	edited := "# Top\n\n## Notes\n\nfirst notes.\n" // second Notes section removed

	m := newPositionModel(t, original)
	m.cursorMode = true
	m.cursorPositioned = true
	for i, ln := range m.lines {
		if strings.Contains(ln.content, "second notes") {
			m.cursorLine = i
			break
		}
	}
	pre := m.Position()
	require.Equal(t, 1, pre.HeadingIndex,
		"capture should record HeadingIndex=1 for the second Notes section")

	m.SetText("doc.md", edited)
	require.NotPanics(t, func() { m.RestorePosition(pre) })

	// HeadingIndex was 1 but only 1 Notes section now (index 0). It
	// should clamp to 0 — cursor lands inside the remaining Notes.
	require.True(t, m.cursorPositioned)
	heading, _ := m.enclosingHeading(m.cursorLine)
	require.NotNil(t, heading)
	assert.Equal(t, "Notes", string(heading.Text(m.markdown)))
}
