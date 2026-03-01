package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	humanize "github.com/dustin/go-humanize"
)

// fuzzyReadDirMsg is sent when a directory read completes.
type fuzzyReadDirMsg struct {
	entries []os.DirEntry
}

// fuzzyPathReadDirMsg is sent when a path-mode directory read completes.
type fuzzyPathReadDirMsg struct {
	dir     string
	entries []os.DirEntry
}

// fuzzyPicker is a file picker with fuzzy text filtering.
type fuzzyPicker struct {
	input        textinput.Model
	fsys         fileSystem
	wd           string          // program working directory (for resolving relative paths in path mode)
	dir          string          // current browsing directory
	allEntries   []os.DirEntry   // full dir listing (dirs first, sorted)
	filtered     []os.DirEntry   // fuzzy-matched subset
	cursor       int             // index into filtered
	minIdx       int             // first visible index
	maxIdx       int             // last visible index
	height       int
	width        int
	allowedTypes []string
	showHidden   bool
	selected     string // set when enter pressed on a file

	// Path mode state (ephemeral — does not affect dir or nav stacks).
	pathDir     string        // resolved directory for current path input
	pathEntries []os.DirEntry // entries read from pathDir

	// Navigation stacks for directory enter/back.
	dirStack    []string
	cursorStack []int
	minStack    []int
	maxStack    []int
}

// parentDirEntry is a synthetic os.DirEntry for the ".." parent directory.
type parentDirEntry struct{}

func (parentDirEntry) Name() string             { return ".." }
func (parentDirEntry) IsDir() bool              { return true }
func (parentDirEntry) Type() os.FileMode        { return os.ModeDir }
func (parentDirEntry) Info() (os.FileInfo, error) { return parentFileInfo{}, nil }

type parentFileInfo struct{}

func (parentFileInfo) Name() string        { return ".." }
func (parentFileInfo) Size() int64         { return 0 }
func (parentFileInfo) Mode() os.FileMode   { return os.ModeDir | 0o755 }
func (parentFileInfo) ModTime() time.Time  { return time.Time{} }
func (parentFileInfo) IsDir() bool         { return true }
func (parentFileInfo) Sys() any            { return nil }

// Style constants matching filepicker's defaults.
var (
	fpCursorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	fpSelectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	fpDirectoryStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	fpFileStyle        = lipgloss.NewStyle()
	fpDisabledStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	fpDisabledCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	fpDisabledSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	fpPermissionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	fpFileSizeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(7).Align(lipgloss.Right)
	fpEmptyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func newFuzzyPicker(dir string, allowedTypes []string, fsys fileSystem) fuzzyPicker {
	ti := textinput.New()
	ti.Prompt = "  Filter: "
	ti.Placeholder = "type to filter..."
	cmd := ti.Focus()
	_ = cmd // cursor blink command, will be batched in Init

	return fuzzyPicker{
		input:        ti,
		fsys:         fsys,
		wd:           dir,
		dir:          dir,
		height:       20,
		allowedTypes: allowedTypes,
	}
}

func (fp fuzzyPicker) Init() tea.Cmd {
	return tea.Batch(fp.input.Focus(), fp.readDir())
}

func (fp fuzzyPicker) readDir() tea.Cmd {
	dir := fp.dir
	fsys := fp.fsys
	showHidden := fp.showHidden
	return func() tea.Msg {
		entries := readAndSortDir(fsys, dir, showHidden)
		return fuzzyReadDirMsg{entries: entries}
	}
}

func (fp fuzzyPicker) pathReadDir(dir string) tea.Cmd {
	fsys := fp.fsys
	showHidden := fp.showHidden
	return func() tea.Msg {
		entries := readAndSortDir(fsys, dir, showHidden)
		return fuzzyPathReadDirMsg{dir: dir, entries: entries}
	}
}

// readAndSortDir reads a directory, sorts dirs first then alphabetically,
// and optionally filters hidden files.
func readAndSortDir(fsys fileSystem, dir string, showHidden bool) []os.DirEntry {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() == entries[j].IsDir() {
			return entries[i].Name() < entries[j].Name()
		}
		return entries[i].IsDir()
	})

	if showHidden {
		return entries
	}

	var visible []os.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		visible = append(visible, e)
	}
	return visible
}

// isPathMode returns true when the input contains a path separator.
func (fp *fuzzyPicker) isPathMode() bool {
	return strings.ContainsRune(fp.input.Value(), filepath.Separator)
}

// splitPathInput splits the input into a resolved directory and a filter query.
// The directory part is everything up to and including the last separator.
// Relative paths are resolved against the program's working directory.
func (fp *fuzzyPicker) splitPathInput() (resolvedDir, query string) {
	value := fp.input.Value()
	lastSep := strings.LastIndexByte(value, filepath.Separator)
	if lastSep < 0 {
		return fp.dir, value
	}
	dirPart := value[:lastSep]
	query = value[lastSep+1:]
	if dirPart == "" {
		dirPart = string(filepath.Separator)
	}
	if filepath.IsAbs(dirPart) {
		resolvedDir = filepath.Clean(dirPart)
	} else {
		resolvedDir = filepath.Clean(filepath.Join(fp.wd, dirPart))
	}
	return
}

// rawDirPart returns the directory portion of the input (up to and including
// the last separator), preserving the user's original text.
func (fp *fuzzyPicker) rawDirPart() string {
	value := fp.input.Value()
	lastSep := strings.LastIndexByte(value, filepath.Separator)
	if lastSep < 0 {
		return ""
	}
	return value[:lastSep+1]
}

// filter applies case-insensitive subsequence matching.
// In path mode it filters pathEntries; otherwise it filters allEntries.
func (fp *fuzzyPicker) filter() {
	var source []os.DirEntry
	var query string

	if fp.isPathMode() {
		var resolvedDir string
		resolvedDir, query = fp.splitPathInput()
		// Use pathEntries if they match the current resolved dir.
		if fp.pathDir == resolvedDir {
			source = fp.pathEntries
		}
		// Prepend ".." if not at root.
		if filepath.Dir(resolvedDir) != resolvedDir {
			source = append([]os.DirEntry{parentDirEntry{}}, source...)
		}
	} else {
		source = fp.allEntries
		query = fp.input.Value()
		// Clear stale path state.
		fp.pathDir = ""
		fp.pathEntries = nil
	}

	queryLower := strings.ToLower(query)
	fp.filtered = nil
	for _, e := range source {
		if queryLower == "" || subsequenceMatch(strings.ToLower(e.Name()), queryLower) {
			fp.filtered = append(fp.filtered, e)
		}
	}

	// Clamp cursor.
	if fp.cursor < 0 {
		fp.cursor = 0
	}
	if fp.cursor >= len(fp.filtered) {
		fp.cursor = max(0, len(fp.filtered)-1)
	}
	// Reset viewport to include cursor.
	fp.minIdx = 0
	fp.maxIdx = fp.height - 1
	if fp.cursor > fp.maxIdx {
		fp.minIdx = fp.cursor - fp.height + 1
		fp.maxIdx = fp.cursor
	}
}

// handleInputChange re-filters and, if in path mode with a new directory,
// triggers an async directory read.
func (fp *fuzzyPicker) handleInputChange() tea.Cmd {
	fp.filter()
	if fp.isPathMode() {
		resolvedDir, _ := fp.splitPathInput()
		if resolvedDir != fp.pathDir {
			return fp.pathReadDir(resolvedDir)
		}
	}
	return nil
}

// subsequenceMatch returns true if all characters in needle appear in haystack
// in order (case-insensitive matching should be done by caller).
func subsequenceMatch(haystack, needle string) bool {
	hi := 0
	for _, c := range needle {
		found := false
		for hi < len(haystack) {
			r := rune(haystack[hi])
			hi++
			if r == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// canSelect checks if a filename has an allowed extension.
func (fp *fuzzyPicker) canSelect(name string) bool {
	if len(fp.allowedTypes) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, allowed := range fp.allowedTypes {
		if ext == allowed {
			return true
		}
	}
	return false
}

func (fp fuzzyPicker) Update(msg tea.Msg) (fuzzyPicker, tea.Cmd) {
	switch msg := msg.(type) {
	case fuzzyReadDirMsg:
		// Prepend ".." entry when not at filesystem root.
		if filepath.Dir(fp.dir) != fp.dir {
			fp.allEntries = append([]os.DirEntry{parentDirEntry{}}, msg.entries...)
		} else {
			fp.allEntries = msg.entries
		}
		fp.filter()
		return fp, nil

	case fuzzyPathReadDirMsg:
		fp.pathDir = msg.dir
		fp.pathEntries = msg.entries
		fp.filter()
		return fp, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "ctrl+p":
			fp.cursor--
			if fp.cursor < 0 {
				fp.cursor = 0
			}
			if fp.cursor < fp.minIdx {
				fp.minIdx = fp.cursor
				fp.maxIdx = fp.minIdx + fp.height - 1
			}
			return fp, nil

		case "down", "ctrl+n":
			fp.cursor++
			if fp.cursor >= len(fp.filtered) {
				fp.cursor = len(fp.filtered) - 1
			}
			if fp.cursor < 0 {
				fp.cursor = 0
			}
			if fp.cursor > fp.maxIdx {
				fp.maxIdx = fp.cursor
				fp.minIdx = fp.maxIdx - fp.height + 1
			}
			return fp, nil

		case "pgup":
			fp.cursor -= fp.height
			if fp.cursor < 0 {
				fp.cursor = 0
			}
			fp.minIdx -= fp.height
			if fp.minIdx < 0 {
				fp.minIdx = 0
			}
			fp.maxIdx = fp.minIdx + fp.height - 1
			return fp, nil

		case "pgdown":
			fp.cursor += fp.height
			if fp.cursor >= len(fp.filtered) {
				fp.cursor = max(0, len(fp.filtered)-1)
			}
			fp.maxIdx += fp.height
			if fp.maxIdx >= len(fp.filtered) {
				fp.maxIdx = max(0, len(fp.filtered)-1)
			}
			fp.minIdx = fp.maxIdx - fp.height + 1
			if fp.minIdx < 0 {
				fp.minIdx = 0
			}
			return fp, nil

		case "enter":
			if len(fp.filtered) == 0 || fp.cursor < 0 {
				return fp, nil
			}
			entry := fp.filtered[fp.cursor]

			if fp.isPathMode() {
				resolvedDir, _ := fp.splitPathInput()
				if entry.Name() == ".." || entry.IsDir() {
					// Update the input text to navigate into the directory.
					raw := fp.rawDirPart()
					if entry.Name() == ".." {
						// Remove the last path element: "subdir/" → "", "a/b/" → "a/".
						trimmed := strings.TrimSuffix(raw, string(filepath.Separator))
						if i := strings.LastIndexByte(trimmed, filepath.Separator); i >= 0 {
							fp.input.SetValue(trimmed[:i+1])
						} else {
							fp.input.SetValue("")
						}
					} else {
						fp.input.SetValue(raw + entry.Name() + string(filepath.Separator))
					}
					return fp, fp.handleInputChange()
				}
				// File selection in path mode.
				fp.selected = filepath.Join(resolvedDir, entry.Name())
				return fp, nil
			}

			// Normal mode.
			if entry.Name() == ".." {
				return fp, fp.navigateBack()
			}
			if entry.IsDir() {
				// Push current state.
				fp.dirStack = append(fp.dirStack, fp.dir)
				fp.cursorStack = append(fp.cursorStack, fp.cursor)
				fp.minStack = append(fp.minStack, fp.minIdx)
				fp.maxStack = append(fp.maxStack, fp.maxIdx)
				// Navigate into directory.
				fp.dir = filepath.Join(fp.dir, entry.Name())
				fp.cursor = 0
				fp.minIdx = 0
				fp.maxIdx = fp.height - 1
				fp.input.SetValue("")
				return fp, fp.readDir()
			}
			// File: check if allowed.
			if fp.canSelect(entry.Name()) {
				fp.selected = filepath.Join(fp.dir, entry.Name())
			}
			return fp, nil

		default:
			// Forward to textinput.
			prevValue := fp.input.Value()
			var cmd tea.Cmd
			fp.input, cmd = fp.input.Update(msg)
			if fp.input.Value() != prevValue {
				if pathCmd := fp.handleInputChange(); pathCmd != nil {
					cmd = tea.Batch(cmd, pathCmd)
				}
			}
			return fp, cmd
		}
	}

	// Forward other messages (e.g. cursor blink) to textinput.
	var cmd tea.Cmd
	fp.input, cmd = fp.input.Update(msg)
	return fp, cmd
}

// navigateBack goes to the parent directory, restoring saved state.
func (fp *fuzzyPicker) navigateBack() tea.Cmd {
	parent := filepath.Dir(fp.dir)
	if parent == fp.dir {
		return nil // already at root
	}
	fp.dir = parent
	if len(fp.dirStack) > 0 {
		n := len(fp.dirStack) - 1
		fp.dir = fp.dirStack[n]
		fp.dirStack = fp.dirStack[:n]
		fp.cursor = fp.cursorStack[n]
		fp.cursorStack = fp.cursorStack[:n]
		fp.minIdx = fp.minStack[n]
		fp.minStack = fp.minStack[:n]
		fp.maxIdx = fp.maxStack[n]
		fp.maxStack = fp.maxStack[:n]
	} else {
		fp.cursor = 0
		fp.minIdx = 0
		fp.maxIdx = fp.height - 1
	}
	fp.input.SetValue("")
	return fp.readDir()
}

func (fp fuzzyPicker) View() string {
	var s strings.Builder

	// Text input.
	s.WriteString(fp.input.View())
	s.WriteRune('\n')

	if len(fp.filtered) == 0 {
		s.WriteString(fpEmptyStyle.Render("  No matching files."))
		s.WriteRune('\n')
	} else {
		for i, entry := range fp.filtered {
			if i < fp.minIdx || i > fp.maxIdx {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			name := entry.Name()
			isDir := entry.IsDir()
			size := ""
			if !isDir {
				size = strings.Replace(humanize.Bytes(uint64(info.Size())), " ", "", 1) //nolint:gosec
			}
			disabled := !fp.canSelect(name) && !isDir

			sizeCol := fpFileSizeStyle.GetWidth()
			if i == fp.cursor {
				selected := " " + info.Mode().String()
				if isDir {
					selected += fmt.Sprintf("%"+strconv.Itoa(sizeCol)+"s", "")
				} else {
					selected += fmt.Sprintf("%"+strconv.Itoa(sizeCol)+"s", size)
				}
				selected += " " + name

				if disabled {
					s.WriteString(fpDisabledCursor.Render(">") + fpDisabledSelected.Render(selected))
				} else {
					s.WriteString(fpCursorStyle.Render(">") + fpSelectedStyle.Render(selected))
				}
			} else {
				style := fpFileStyle
				if isDir {
					style = fpDirectoryStyle
				} else if disabled {
					style = fpDisabledStyle
				}

				s.WriteString(fpCursorStyle.Render(" "))
				s.WriteString(" " + fpPermissionStyle.Render(info.Mode().String()))
				if isDir {
					s.WriteString(fpFileSizeStyle.Render(""))
				} else {
					s.WriteString(fpFileSizeStyle.Render(size))
				}
				s.WriteString(" " + style.Render(name))
			}
			s.WriteRune('\n')
		}
	}

	// Pad remaining height.
	rendered := lipgloss.Height(s.String())
	for i := rendered; i <= fp.height+1; i++ { // +1 for the input line
		s.WriteRune('\n')
	}

	return s.String()
}

// DidSelect returns true and the selected path if the user selected a file.
func (fp fuzzyPicker) DidSelect() (bool, string) {
	if fp.selected != "" {
		return true, fp.selected
	}
	return false, ""
}

// SetHeight sets the visible file list height (not counting the input line).
func (fp *fuzzyPicker) SetHeight(h int) {
	fp.height = h
	if fp.maxIdx > fp.minIdx+fp.height-1 {
		fp.maxIdx = fp.minIdx + fp.height - 1
	}
}

// SetWidth sets the width of the picker.
func (fp *fuzzyPicker) SetWidth(w int) {
	fp.width = w
	fp.input.SetWidth(w - lipgloss.Width(fp.input.Prompt) - 1)
}
