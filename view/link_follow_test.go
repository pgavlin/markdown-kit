package view

import (
	"testing"

	"github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// selectFirstLink steps selection forward until it lands on an AutoLink or
// regular Link. Returns the underlying URL string for assertions.
func selectFirstLink(t *testing.T, m *Model) string {
	t.Helper()
	// Use SelectNext directly instead of driving key input so the test
	// isn't sensitive to keymap changes.
	matched := m.SelectNext(func(n ast.Node) (bool, bool) {
		switch n.(type) {
		case *ast.Link, *ast.AutoLink:
			return true, true
		}
		return false, false
	})
	require.True(t, matched, "document should contain at least one link")
	return m.FocusedLinkDestination()
}

func TestFocusedLinkDestination_NoSelection(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Title\n\nNo links here yet.\n")
	assert.Equal(t, "", m.FocusedLinkDestination(),
		"should return empty string when nothing is selected")
}

func TestFocusedLinkDestination_SelectionNotALink(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Title\n\nParagraph.\n")
	// Select the heading (a non-link node).
	matched := m.SelectNext(func(n ast.Node) (bool, bool) {
		if n.Kind() == ast.KindHeading {
			return true, true
		}
		return false, false
	})
	require.True(t, matched)
	assert.Equal(t, "", m.FocusedLinkDestination(),
		"non-link selection should return empty destination")
}

func TestFocusedLinkDestination_Link(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Title\n\nSee [the docs](https://example.com/docs).\n")
	url := selectFirstLink(t, &m)
	assert.Equal(t, "https://example.com/docs", url)
}

func TestFocusedLinkDestination_AutoLink(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Title\n\nVisit <https://example.com/auto>.\n")
	url := selectFirstLink(t, &m)
	assert.Equal(t, "https://example.com/auto", url)
}

func TestFollowLink_InternalAnchor(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n\nJump to [Section](#section-a).\n\n## Section A\n\nContent.\n")

	// Select the first link.
	url := selectFirstLink(t, &m)
	require.Equal(t, "#section-a", url)
	priorSelection := m.selection
	require.NotNil(t, priorSelection)

	// Follow it.
	ok := m.FollowLink()
	require.True(t, ok, "FollowLink should succeed for internal anchor")

	// Selection should have moved to the Section A heading.
	require.NotNil(t, m.selection)
	heading, isHeading := m.selection.Node.(*ast.Heading)
	require.True(t, isHeading, "selection should land on a heading, got %T", m.selection.Node)
	assert.Equal(t, "Section A", string(heading.Text(m.markdown)))

	// Prior selection was pushed onto the backstack.
	require.Len(t, m.backstack, 1)
	assert.Same(t, priorSelection, m.backstack[0])
}

func TestFollowLink_QualifiedInternalAnchor(t *testing.T) {
	// An anchor prefixed with the document name ("doc.md#foo") should still
	// be recognized as internal.
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n\nSee [Section](doc.md#section-a).\n\n## Section A\n\nText.\n")

	url := selectFirstLink(t, &m)
	require.Equal(t, "doc.md#section-a", url)

	ok := m.FollowLink()
	require.True(t, ok, "FollowLink should accept name-qualified anchors")

	heading, isHeading := m.selection.Node.(*ast.Heading)
	require.True(t, isHeading)
	assert.Equal(t, "Section A", string(heading.Text(m.markdown)))
}

func TestFollowLink_ExternalLinkNoOp(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n\nSee [example](https://example.com).\n")

	selectFirstLink(t, &m)
	priorSelection := m.selection
	require.NotNil(t, priorSelection)

	ok := m.FollowLink()
	assert.False(t, ok, "FollowLink on external URL should return false")
	// Selection unchanged; backstack empty.
	assert.Same(t, priorSelection, m.selection)
	assert.Empty(t, m.backstack)
}

func TestFollowLink_NoSelection(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n\n[link](#foo)\n")
	require.Nil(t, m.selection)
	ok := m.FollowLink()
	assert.False(t, ok)
	assert.Empty(t, m.backstack)
}

func TestGoBack_PopsBackstack(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n\n[link](#section-a)\n\n## Section A\n\nText.\n")

	// Select link, follow, then go back.
	selectFirstLink(t, &m)
	priorSelection := m.selection
	require.True(t, m.FollowLink())

	// Selection is now on the heading; backstack has the prior link.
	require.Len(t, m.backstack, 1)
	afterFollow := m.selection
	require.NotNil(t, afterFollow)

	ok := m.GoBack()
	require.True(t, ok)
	// Backstack popped.
	assert.Empty(t, m.backstack)
	// Selection restored to what it was before FollowLink.
	assert.Same(t, priorSelection, m.selection)
}

func TestGoBack_EmptyBackstack(t *testing.T) {
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", "# Top\n")
	assert.False(t, m.GoBack(), "GoBack with empty backstack returns false")
}

func TestSelectAnchor_HTMLAnchor(t *testing.T) {
	// <a id="target"> is an HTML anchor; SelectAnchor must use LookupNode.
	md := "# Heading\n\n<a id=\"target\">\n</a>\n\nThis paragraph follows the anchor.\n"
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", md)

	ok := m.SelectAnchor("target")
	require.True(t, ok, "SelectAnchor should succeed for HTML anchor")
	require.NotNil(t, m.selection)
	// Selection should be on the HTML block containing the <a id="target">.
	assert.Equal(t, ast.KindHTMLBlock, m.selection.Node.Kind())
}

func TestSelectAnchor_MultipleOccurrencesCycleForward(t *testing.T) {
	// Two headings with the same anchor; each SelectAnchor call advances
	// to the next occurrence.
	md := "# Top\n\n## Foo\n\nA.\n\n## Foo\n\nB.\n"
	m := NewModel(WithTheme(styles.Pulumi), WithWidth(80), WithHeight(24))
	m.SetText("doc.md", md)

	require.True(t, m.SelectAnchor("foo"))
	firstStart := m.selection.Start

	require.True(t, m.SelectAnchor("foo"))
	secondStart := m.selection.Start
	assert.Greater(t, secondStart, firstStart,
		"second SelectAnchor call should advance to the later occurrence")
}
