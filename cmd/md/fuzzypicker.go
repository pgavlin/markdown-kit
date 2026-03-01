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

// fuzzyPicker is a file picker with fuzzy text filtering.
type fuzzyPicker struct {
	input        textinput.Model
	fsys         fileSystem
	dir          string
	allEntries   []os.DirEntry // full dir listing (dirs first, sorted)
	filtered     []os.DirEntry // fuzzy-matched subset
	cursor       int           // index into filtered
	minIdx       int           // first visible index
	maxIdx       int           // last visible index
	height       int
	width        int
	allowedTypes []string
	showHidden   bool
	selected     string // set when enter pressed on a file

	// Navigation stacks for directory enter/back.
	dirStack    []string
	cursorStack []int
	minStack    []int
	maxStack    []int
}

// parentDirEntry is a synthetic os.DirEntry for the ".." parent directory.
type parentDirEntry struct{}

func (parentDirEntry) Name() string               { return ".." }
func (parentDirEntry) IsDir() bool                 { return true }
func (parentDirEntry) Type() os.FileMode           { return os.ModeDir }
func (parentDirEntry) Info() (os.FileInfo, error)   { return parentFileInfo{}, nil }

type parentFileInfo struct{}

func (parentFileInfo) Name() string      { return ".." }
func (parentFileInfo) Size() int64       { return 0 }
func (parentFileInfo) Mode() os.FileMode { return os.ModeDir | 0o755 }
func (parentFileInfo) ModTime() time.Time { return time.Time{} }
func (parentFileInfo) IsDir() bool       { return true }
func (parentFileInfo) Sys() any          { return nil }

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
		entries, err := fsys.ReadDir(dir)
		if err != nil {
			return fuzzyReadDirMsg{entries: nil}
		}

		// Sort dirs first, then alphabetically within each group.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() == entries[j].IsDir() {
				return entries[i].Name() < entries[j].Name()
			}
			return entries[i].IsDir()
		})

		if showHidden {
			return fuzzyReadDirMsg{entries: entries}
		}

		// Filter hidden files.
		var visible []os.DirEntry
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			visible = append(visible, e)
		}
		return fuzzyReadDirMsg{entries: visible}
	}
}

// filter applies case-insensitive subsequence matching to allEntries.
func (fp *fuzzyPicker) filter() {
	query := strings.ToLower(fp.input.Value())
	// Always allocate a new slice to avoid corrupting allEntries
	// through shared backing arrays.
	fp.filtered = nil
	for _, e := range fp.allEntries {
		if query == "" || subsequenceMatch(strings.ToLower(e.Name()), query) {
			fp.filtered = append(fp.filtered, e)
		}
	}

	// Clamp cursor.
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
			if len(fp.filtered) == 0 {
				return fp, nil
			}
			entry := fp.filtered[fp.cursor]
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
				fp.filter()
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
