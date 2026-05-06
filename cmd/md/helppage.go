package main

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

//go:embed help.md
var helpPageMarkdown string

// fmtKey formats a key.Binding as a backtick-wrapped list of its keys,
// e.g. `q` / `ctrl+c`.
func fmtKey(b key.Binding) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return "(unbound)"
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = "`" + k + "`"
	}
	return strings.Join(parts, " / ")
}

// renderHelpPage executes the help.md template with the actual configured
// keybindings and returns the rendered Markdown.
func renderHelpPage(km readerKeyMap) string {
	data := map[string]string{
		// View keys
		"Up":             fmtKey(km.Up),
		"Down":           fmtKey(km.Down),
		"PageUp":         fmtKey(km.PageUp),
		"PageDown":       fmtKey(km.PageDown),
		"GotoTop":        fmtKey(km.GotoTop),
		"GotoEnd":        fmtKey(km.GotoEnd),
		"Home":           fmtKey(km.Home),
		"End":            fmtKey(km.End),
		"Left":           fmtKey(km.Left),
		"Right":          fmtKey(km.Right),
		"NextItem":       fmtKey(km.NextItem),
		"PrevItem":       fmtKey(km.PrevItem),
		"NextHeading":    fmtKey(km.NextHeading),
		"PrevHeading":    fmtKey(km.PrevHeading),
		"DecreaseWidth":  fmtKey(km.DecreaseWidth),
		"IncreaseWidth":  fmtKey(km.IncreaseWidth),
		"FollowLink":     fmtKey(km.FollowLink),
		"GoBack":         fmtKey(km.GoBack),
		"CopySelection":  fmtKey(km.CopySelection),
		"CopySource":     fmtKey(km.CopySource),
		"Search":         fmtKey(km.Search),
		"NextMatch":      fmtKey(km.NextMatch),
		"PrevMatch":      fmtKey(km.PrevMatch),
		"ClearSearch":    fmtKey(km.ClearSearch),
		"ToggleTOC":      fmtKey(km.ToggleTOC),
		"ToggleMetadata": fmtKey(km.ToggleMetadata),
		// Reader keys
		"ToggleSource":    fmtKey(km.ToggleSource),
		"OpenFile":        fmtKey(km.OpenFile),
		"OpenBrowser":     fmtKey(km.OpenBrowser),
		"OpenFileNewTab":  fmtKey(km.OpenFileNewTab),
		"OpenURL":         fmtKey(km.OpenURL),
		"NextTab":         fmtKey(km.NextTab),
		"PrevTab":         fmtKey(km.PrevTab),
		"CloseTab":        fmtKey(km.CloseTab),
		"CloseAllTabs":    fmtKey(km.CloseAllTabs),
		"NewTab":          fmtKey(km.NewTab),
		"Reload":          fmtKey(km.Reload),
		"History":         fmtKey(km.History),
		"SearchDocuments": fmtKey(km.SearchDocuments),
		"FindSimilar":     fmtKey(km.FindSimilar),
		"UserGuide":       fmtKey(km.UserGuide),
		"BugReport":       fmtKey(km.BugReport),
		"Edit":            fmtKey(km.Edit),
		"ExportGist":      fmtKey(km.ExportGist),
		"Help":            fmtKey(km.Help),
		"Quit":            fmtKey(km.Quit),
	}

	tmpl, err := template.New("help").Parse(helpPageMarkdown)
	if err != nil {
		return helpPageMarkdown
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return helpPageMarkdown
	}
	return buf.String()
}

// shortKey formats a key.Binding as a compact string for the help overlay,
// e.g. "j / down".
func shortKey(b key.Binding) string {
	keys := b.Keys()
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, " / ")
}

// renderHelpOverlay renders a styled, compact key binding overlay.
func renderHelpOverlay(km readerKeyMap) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorTabActiveBg)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Width(16)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(colorMuted).MarginTop(1)

	binding := func(b key.Binding, desc string) string {
		k := shortKey(b)
		if k == "" {
			return ""
		}
		return keyStyle.Render(k) + descStyle.Render(desc)
	}

	var lines []string
	lines = append(lines, titleStyle.Render("md -- Key Bindings"))
	lines = append(lines, "")

	lines = append(lines, sectionStyle.Render("Movement"))
	lines = append(lines, binding(km.Up, "Scroll up"))
	lines = append(lines, binding(km.Down, "Scroll down"))
	lines = append(lines, binding(km.PageUp, "Page up"))
	lines = append(lines, binding(km.PageDown, "Page down"))
	lines = append(lines, binding(km.GotoTop, "Go to top"))
	lines = append(lines, binding(km.GotoEnd, "Go to end"))
	lines = append(lines, binding(km.Left, "Scroll left"))
	lines = append(lines, binding(km.Right, "Scroll right"))

	lines = append(lines, sectionStyle.Render("Navigation"))
	lines = append(lines, binding(km.NextItem, "Next item"))
	lines = append(lines, binding(km.PrevItem, "Previous item"))
	lines = append(lines, binding(km.NextHeading, "Next heading"))
	lines = append(lines, binding(km.PrevHeading, "Previous heading"))
	lines = append(lines, binding(km.FollowLink, "Follow link"))
	lines = append(lines, binding(km.GoBack, "Go back"))
	lines = append(lines, binding(km.History, "Page history"))
	lines = append(lines, binding(km.ToggleTOC, "Table of contents"))
	lines = append(lines, binding(km.ToggleMetadata, "Document metadata"))

	lines = append(lines, sectionStyle.Render("Search"))
	lines = append(lines, binding(km.Search, "Search in page"))
	lines = append(lines, binding(km.NextMatch, "Next match"))
	lines = append(lines, binding(km.PrevMatch, "Previous match"))
	lines = append(lines, binding(km.ClearSearch, "Clear search"))
	lines = append(lines, binding(km.SearchDocuments, "Search documents"))
	lines = append(lines, binding(km.FindSimilar, "Find similar"))

	lines = append(lines, sectionStyle.Render("Tabs & Files"))
	lines = append(lines, binding(km.NextTab, "Next tab"))
	lines = append(lines, binding(km.PrevTab, "Previous tab"))
	lines = append(lines, binding(km.CloseTab, "Close tab"))
	lines = append(lines, binding(km.OpenFile, "Open file"))
	lines = append(lines, binding(km.OpenFileNewTab, "Open in new tab"))
	lines = append(lines, binding(km.OpenURL, "Open URL"))
	lines = append(lines, binding(km.NewTab, "Link in new tab"))

	lines = append(lines, sectionStyle.Render("Actions"))
	lines = append(lines, binding(km.CopySelection, "Copy selection"))
	lines = append(lines, binding(km.CopySource, "Copy source"))
	lines = append(lines, binding(km.OpenBrowser, "Open in browser"))
	lines = append(lines, binding(km.ToggleSource, "View source"))
	lines = append(lines, binding(km.DecreaseWidth, "Decrease width"))
	lines = append(lines, binding(km.IncreaseWidth, "Increase width"))
	lines = append(lines, binding(km.Reload, "Reload page"))
	lines = append(lines, binding(km.Edit, "Edit"))
	lines = append(lines, binding(km.ExportGist, "Export as gist"))

	lines = append(lines, sectionStyle.Render("General"))
	lines = append(lines, binding(km.Help, "Toggle this help"))
	lines = append(lines, binding(km.UserGuide, "Open user guide"))
	lines = append(lines, binding(km.BugReport, "Copy bug report"))
	lines = append(lines, binding(km.Quit, "Quit"))

	// Filter out empty lines from unbound keys.
	filtered := lines[:0]
	for _, l := range lines {
		if l != "" {
			filtered = append(filtered, l)
		}
	}

	return strings.Join(filtered, "\n")
}
