package main

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"

	"charm.land/bubbles/v2/key"
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
		"Up":            fmtKey(km.Up),
		"Down":          fmtKey(km.Down),
		"PageUp":        fmtKey(km.PageUp),
		"PageDown":      fmtKey(km.PageDown),
		"GotoTop":       fmtKey(km.GotoTop),
		"GotoEnd":       fmtKey(km.GotoEnd),
		"Home":          fmtKey(km.Home),
		"End":           fmtKey(km.End),
		"Left":          fmtKey(km.Left),
		"Right":         fmtKey(km.Right),
		"NextLink":      fmtKey(km.NextLink),
		"PrevLink":      fmtKey(km.PrevLink),
		"NextCodeBlock": fmtKey(km.NextCodeBlock),
		"PrevCodeBlock": fmtKey(km.PrevCodeBlock),
		"NextHeading":   fmtKey(km.NextHeading),
		"PrevHeading":   fmtKey(km.PrevHeading),
		"DecreaseWidth": fmtKey(km.DecreaseWidth),
		"IncreaseWidth": fmtKey(km.IncreaseWidth),
		"FollowLink":    fmtKey(km.FollowLink),
		"GoBack":        fmtKey(km.GoBack),
		"CopySelection": fmtKey(km.CopySelection),
		"Search":        fmtKey(km.Search),
		"NextMatch":     fmtKey(km.NextMatch),
		"PrevMatch":     fmtKey(km.PrevMatch),
		"ClearSearch":   fmtKey(km.ClearSearch),
		// Reader keys
		"ToggleRaw":       fmtKey(km.ToggleRaw),
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
