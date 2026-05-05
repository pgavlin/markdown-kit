package main

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/shell"
)

type editorFamily int

const (
	familyPlusN     editorFamily = iota // <ed> +<N> <file>
	familyVSCode                        // <ed> -g <file>:<N>
	familyFileColon                     // <ed> <file>:<N>
	familyJetBrains                     // <ed> --line <N> <file>
)

// editorFamilies maps known editor binaries (basenames) to their CLI
// convention for jumping to a specific line on launch. Anything not
// listed falls back to familyPlusN.
var editorFamilies = map[string]editorFamily{
	"vi":            familyPlusN,
	"vim":           familyPlusN,
	"nvim":          familyPlusN,
	"nano":          familyPlusN,
	"emacs":         familyPlusN,
	"jed":           familyPlusN,
	"joe":           familyPlusN,
	"kak":           familyPlusN,
	"kakoune":       familyPlusN,
	"code":          familyVSCode,
	"code-insiders": familyVSCode,
	"cursor":        familyVSCode,
	"vscodium":      familyVSCode,
	"codium":        familyVSCode,
	"subl":          familyFileColon,
	"zed":           familyFileColon,
	"helix":         familyFileColon,
	"hx":            familyFileColon,
	"micro":         familyFileColon,
	"idea":          familyJetBrains,
	"goland":        familyJetBrains,
	"pycharm":       familyJetBrains,
	"webstorm":      familyJetBrains,
	"rubymine":      familyJetBrains,
	"clion":         familyJetBrains,
	"phpstorm":      familyJetBrains,
	"rider":         familyJetBrains,
	"datagrip":      familyJetBrains,
	"fleet":         familyJetBrains,
}

// editorsThatFork lists binaries that exit immediately after launch
// unless told to wait. We auto-inject --wait so the suspend/reload
// flow doesn't return before the user has finished editing.
var editorsThatFork = map[string]bool{
	"code":          true,
	"code-insiders": true,
	"cursor":        true,
	"vscodium":      true,
	"codium":        true,
	"subl":          true,
	"zed":           true,
}

// hasWaitFlag reports whether any of -w, --wait, or -n already
// appears in flags. -n is Sublime's "no fork" flag; treating it as a
// wait signal lets users opt out of the auto-injection by passing -n.
func hasWaitFlag(flags []string) bool {
	for _, f := range flags {
		if f == "-w" || f == "--wait" || f == "-n" {
			return true
		}
	}
	return false
}

// editorCommand resolves $VISUAL/$EDITOR via the env callback and
// returns the argv to invoke for opening file at the given 1-based
// line. The first element is the binary; remaining elements are
// flags and positional args ready to hand to exec.Command.
func editorCommand(env func(name string) string, file string, line int) ([]string, error) {
	raw := env("VISUAL")
	if strings.TrimSpace(raw) == "" {
		raw = env("EDITOR")
	}
	if strings.TrimSpace(raw) == "" {
		raw = "vi"
	}

	fields, err := shell.Fields(raw, env)
	if err != nil {
		return nil, fmt.Errorf("parsing editor command %q: %w", raw, err)
	}
	if len(fields) == 0 {
		fields = []string{"vi"}
	}

	binary := fields[0]
	userFlags := fields[1:]
	base := filepath.Base(binary)
	family := editorFamilies[base]

	if editorsThatFork[base] && !hasWaitFlag(userFlags) {
		userFlags = append([]string{"--wait"}, userFlags...)
	}

	out := []string{binary}
	out = append(out, userFlags...)
	switch family {
	case familyVSCode:
		out = append(out, "-g", fmt.Sprintf("%s:%d", file, line))
	case familyFileColon:
		out = append(out, fmt.Sprintf("%s:%d", file, line))
	case familyJetBrains:
		out = append(out, "--line", fmt.Sprintf("%d", line), file)
	default: // familyPlusN
		out = append(out, fmt.Sprintf("+%d", line), file)
	}
	return out, nil
}

// findSourceLine returns the 1-based line in source whose trimmed
// content equals snippet, taking the occurrence-th match. If the
// requested occurrence is past the last match, falls back to the
// first match. If snippet is empty or never matches, returns 1.
func findSourceLine(source []byte, snippet string, occurrence int) int {
	if snippet == "" {
		return 1
	}
	scanner := bufio.NewScanner(bytes.NewReader(source))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var matches []int
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if strings.TrimSpace(scanner.Text()) == snippet {
			matches = append(matches, lineNum)
		}
	}
	if len(matches) == 0 {
		return 1
	}
	if occurrence >= 0 && occurrence < len(matches) {
		return matches[occurrence]
	}
	return matches[0]
}

// canEditSource reports whether the given source path can be edited
// via $EDITOR. True only for non-empty, non-URL local markdown files.
// Convertible non-markdown files (.docx, .html, ...) are excluded
// because their on-disk form doesn't round-trip through markdown.
func canEditSource(source string) bool {
	if source == "" {
		return false
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return false
	}
	return isMarkdownFile(source)
}
