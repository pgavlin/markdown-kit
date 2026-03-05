package view

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	goldmark_ast "github.com/pgavlin/goldmark/ast"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectLinkDests presses ] repeatedly and returns all distinct link destinations found.
func collectLinkDests(t *testing.T, m Model, maxPresses int) (Model, []string) {
	t.Helper()
	var links []string
	for i := 0; i < maxPresses; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		sel := m.Selection()
		if sel == nil {
			break
		}
		if sel.Node.Kind() != goldmark_ast.KindLink {
			continue
		}
		link := sel.Node.(*goldmark_ast.Link)
		dest := string(link.Destination)
		if len(links) == 0 || links[len(links)-1] != dest {
			links = append(links, dest)
		} else {
			break
		}
	}
	return m, links
}

// extractHighlightedText finds text wrapped in reverse video (\033[7m...\033[27m)
// in the view output and returns it with ANSI sequences stripped.
func extractHighlightedText(view string) string {
	const revOn = "\033[7m"
	const revOff = "\033[27m"

	start := strings.Index(view, revOn)
	if start == -1 {
		return ""
	}
	start += len(revOn)
	rest := view[start:]
	end := strings.Index(rest, revOff)
	if end == -1 {
		return ""
	}
	return ansi.Strip(rest[:end])
}

func TestLinkNav_AnchorLinksInNestedLists(t *testing.T) {
	md := `# **Engineering Process**

This document is a succinct summary of our shared core engineering processes.

* ## Quarters

  * OKRs established by end of first week of quarter
  * Quarterly planning doc shared at least every other quarter (beginning of CY and FY)
  * Hackathon ~once per quarter

* ## Milestones

  * 4 3-week milestones per quarter
  * Standard [Iteration Planning & Execution Process](#iteration-planning--execution) used to track iteration progress in GitHub
  * First two days of milestone for closing out the previous milestone and planning for next

* ## Epics

  * Epics for large collections of work (user facing feature, engineering systems investments)
  * Follow the [Epic Transparency](#epic-transparency) template and process where possible
  * More details in [Epic Owner's Guide](#epic-owners-guide)

* ## Issues

  * ### Opening

    * All work captured in an issue with details of problem (not just solution)
    * Customer feedback captured in issues and linked to customer accounts via the [Customer Requests Management dashboard in Metabase](https://metabase.corp.pulumi.com/dashboard/377-customer-requests-management), which is backed by our Snowflake database.
`

	m := NewModel(WithTheme(styles.GlamourDark), WithContentWidth(160))
	m.SetText("test.md", md)
	m.SetSize(183, 113)

	_, links := collectLinkDests(t, m, 20)

	assert.Contains(t, links, "#iteration-planning--execution", "should find Iteration Planning link")
	assert.Contains(t, links, "#epic-transparency", "should find Epic Transparency link")
	assert.Contains(t, links, "#epic-owners-guide", "should find Epic Owner's Guide link")
	assert.Contains(t, links, "https://metabase.corp.pulumi.com/dashboard/377-customer-requests-management", "should find Metabase link")
	require.Len(t, links, 4, "should find exactly 4 links")
}

func TestLinkNav_WithStripDataURIs(t *testing.T) {
	md := "# Title\n\n" +
		"* ## Milestones\n\n" +
		"  * Standard [Iteration Planning & Execution Process](#iteration-planning--execution) used to track iteration progress in GitHub  \n\n" +
		"* ## Epics\n\n" +
		"  * Follow the [Epic Transparency](#epic-transparency) template  \n" +
		"  * More details in [Epic Owner's Guide](#epic-owners-guide)\n\n" +
		"## Images\n\n" +
		"![][image1]\n\n" +
		"Some [link](https://example.com/last) here.\n\n" +
		"[image1]: <data:image/png;base64,iVBORw0KGgo=>\n"

	m := NewModel(
		WithTheme(styles.GlamourDark),
		WithContentWidth(160),
		WithDocumentTransformer(StripDataURIs),
	)
	m.SetText("test.md", md)
	m.SetSize(183, 113)

	_, links := collectLinkDests(t, m, 20)

	assert.Contains(t, links, "#iteration-planning--execution")
	assert.Contains(t, links, "#epic-transparency")
	assert.Contains(t, links, "#epic-owners-guide")
	assert.Contains(t, links, "https://example.com/last")
}

func TestLinkNav_HighlightApplied(t *testing.T) {
	md := "Some text with a [link here](#anchor) in it.\n"

	m := NewModel(WithTheme(styles.GlamourDark))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	require.NotNil(t, m.Selection())
	assert.Equal(t, goldmark_ast.KindLink, m.Selection().Node.Kind())

	// Selection span must be non-empty.
	assert.Greater(t, m.selectionEnd, m.selectionStart)

	// Reverse video must wrap exactly the link text.
	highlighted := extractHighlightedText(m.View())
	assert.Equal(t, "link here", highlighted)
}

func TestLinkNav_EachLinkHighlightsInNestedLists(t *testing.T) {
	md := `# **Engineering Process**

This document is a succinct summary of our shared core engineering processes.

* ## Quarters

  * OKRs established by end of first week of quarter
  * Quarterly planning doc shared at least every other quarter (beginning of CY and FY)
  * Hackathon ~once per quarter

* ## Milestones

  * 4 3-week milestones per quarter
  * Standard [Iteration Planning & Execution Process](#iteration-planning--execution) used to track iteration progress in GitHub
  * First two days of milestone for closing out the previous milestone and planning for next

* ## Epics

  * Epics for large collections of work (user facing feature, engineering systems investments)
  * Follow the [Epic Transparency](#epic-transparency) template and process where possible
  * More details in [Epic Owner's Guide](#epic-owners-guide)

* ## Issues

  * ### Opening

    * All work captured in an issue with details of problem (not just solution)
    * Customer feedback captured in issues and linked to customer accounts via the [Customer Requests Management dashboard in Metabase](https://metabase.corp.pulumi.com/dashboard/377-customer-requests-management), which is backed by our Snowflake database.
`

	m := NewModel(WithTheme(styles.GlamourDark), WithContentWidth(160), WithGutter(true))
	m.SetText("test.md", md)
	m.SetSize(183, 113)

	expectedLinks := []struct {
		dest string
		text string
	}{
		{"#iteration-planning--execution", "Iteration Planning & Execution Process"},
		{"#epic-transparency", "Epic Transparency"},
		{"#epic-owners-guide", "Epic Owner's Guide"},
		{"https://metabase.corp.pulumi.com/dashboard/377-customer-requests-management", "Customer Requests Management dashboard in Metabase"},
	}

	for _, want := range expectedLinks {
		m, _ = m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
		sel := m.Selection()
		require.NotNil(t, sel, "should have selection for %s", want.dest)
		require.Equal(t, goldmark_ast.KindLink, sel.Node.Kind(), "selection should be a Link")

		link := sel.Node.(*goldmark_ast.Link)
		assert.Equal(t, want.dest, string(link.Destination), "link destination")

		// Selection byte range must be non-empty.
		assert.Greater(t, m.selectionEnd, m.selectionStart,
			"selection span must be non-empty for %s", want.dest)

		// Reverse video must wrap exactly the link text.
		highlighted := extractHighlightedText(m.View())
		assert.Equal(t, want.text, highlighted,
			"highlighted text should match link text for %s", want.dest)
	}
}

func TestLinkNav_HyperlinkSequences(t *testing.T) {
	md := "Before [anchor link](#target) after\n"

	m := NewModel(WithTheme(styles.GlamourDark))
	m.SetText("test.md", md)
	m.SetSize(80, 24)

	view := m.View()

	// Links should render with OSC 8 hyperlink sequences.
	assert.True(t, strings.Contains(view, ansi.SetHyperlink("#target")),
		"should contain OSC 8 set for anchor link")
	assert.True(t, strings.Contains(view, ansi.ResetHyperlink()),
		"should contain OSC 8 reset")

	// Link text should be underlined.
	assert.Contains(t, view, "\033[4m", "should contain underline on sequence")
}
