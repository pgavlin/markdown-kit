package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// testFuzzyPicker creates a fuzzyPicker with a pre-populated memFS for testing.
func testFuzzyPicker() (fuzzyPicker, *memFS) {
	fs := newMemFS()
	fs.wd = "/testdir"
	fs.files["/testdir/readme.md"] = []byte("# Hello")
	fs.files["/testdir/notes.md"] = []byte("# Notes")
	fs.files["/testdir/image.png"] = []byte("PNG")
	fs.files["/testdir/subdir/nested.md"] = []byte("# Nested")
	fs.files["/testdir/another.txt"] = []byte("text")

	fp := newFuzzyPicker("/testdir", []string{".md"}, fs)
	return fp, fs
}

// applyReadDir simulates Init by running the readDir command and feeding the result.
func applyReadDir(fp fuzzyPicker) fuzzyPicker {
	cmd := fp.readDir()
	msg := cmd()
	fp, _ = fp.Update(msg)
	return fp
}

func TestFuzzyPicker_ReadDir(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	if len(fp.allEntries) == 0 {
		t.Fatal("expected entries after readDir")
	}

	// ".." should be first, then directories, then files.
	if fp.allEntries[0].Name() != ".." {
		t.Errorf("expected first entry to be '..', got %q", fp.allEntries[0].Name())
	}
	if fp.allEntries[1].Name() != "subdir" {
		t.Errorf("expected second entry to be 'subdir', got %q", fp.allEntries[1].Name())
	}
}

func TestFuzzyPicker_FilterBasic(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Type "md" to filter.
	fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: "r"})
	fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: "e"})

	// Should match "readme.md" (contains 'r' and 'e' as subsequence).
	found := false
	for _, e := range fp.filtered {
		if e.Name() == "readme.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'readme.md' in filtered results after typing 're'")
	}
}

func TestFuzzyPicker_FilterSubsequence(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Type "nmd" — should match "notes.md" (n...m...d as subsequence).
	for _, ch := range "nmd" {
		fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	found := false
	for _, e := range fp.filtered {
		if e.Name() == "notes.md" {
			found = true
		}
	}
	if !found {
		names := make([]string, len(fp.filtered))
		for i, e := range fp.filtered {
			names[i] = e.Name()
		}
		t.Errorf("expected 'notes.md' in filtered results, got %v", names)
	}
}

func TestFuzzyPicker_EmptyFilterShowsAll(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	total := len(fp.filtered)
	if total != len(fp.allEntries) {
		t.Errorf("empty filter: filtered=%d, allEntries=%d", len(fp.filtered), len(fp.allEntries))
	}
}

func TestFuzzyPicker_CursorNavigation(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	if fp.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", fp.cursor)
	}

	// Move down.
	fp, _ = fp.Update(keyMsg("down"))
	if fp.cursor != 1 {
		t.Errorf("expected cursor=1 after down, got %d", fp.cursor)
	}

	// Move up.
	fp, _ = fp.Update(keyMsg("up"))
	if fp.cursor != 0 {
		t.Errorf("expected cursor=0 after up, got %d", fp.cursor)
	}

	// Up at top should stay at 0.
	fp, _ = fp.Update(keyMsg("up"))
	if fp.cursor != 0 {
		t.Errorf("expected cursor=0 at top boundary, got %d", fp.cursor)
	}
}

func TestFuzzyPicker_CursorClampOnFilter(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Move cursor to end.
	for i := 0; i < len(fp.filtered); i++ {
		fp, _ = fp.Update(keyMsg("down"))
	}
	lastCursor := fp.cursor

	// Now filter to fewer items.
	for _, ch := range "readme" {
		fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	if fp.cursor > len(fp.filtered)-1 {
		t.Errorf("cursor=%d exceeds filtered length=%d (was %d)", fp.cursor, len(fp.filtered), lastCursor)
	}
}

func TestFuzzyPicker_EnterDirectory(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// First entry is "..", second should be "subdir" (directory).
	if fp.filtered[0].Name() != ".." {
		t.Fatalf("expected first entry to be '..', got %q", fp.filtered[0].Name())
	}
	// Move cursor to "subdir".
	fp, _ = fp.Update(keyMsg("down"))
	if fp.filtered[fp.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", fp.filtered[fp.cursor].Name())
	}

	// Press enter to navigate into subdir.
	fp, cmd := fp.Update(keyMsg("enter"))
	if fp.dir != "/testdir/subdir" {
		t.Errorf("expected dir='/testdir/subdir', got %q", fp.dir)
	}
	if len(fp.dirStack) != 1 {
		t.Errorf("expected dirStack length=1, got %d", len(fp.dirStack))
	}
	if fp.input.Value() != "" {
		t.Error("expected input cleared after entering directory")
	}

	// Execute the readDir command.
	if cmd != nil {
		msg := cmd()
		fp, _ = fp.Update(msg)
	}

	// Should now see nested.md.
	found := false
	for _, e := range fp.filtered {
		if e.Name() == "nested.md" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'nested.md' in subdir listing")
	}
}

func TestFuzzyPicker_NavigateBack(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Navigate into subdir (skip ".." first).
	fp, _ = fp.Update(keyMsg("down"))
	fp, cmd := fp.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		fp, _ = fp.Update(msg)
	}
	if fp.dir != "/testdir/subdir" {
		t.Fatalf("expected dir='/testdir/subdir', got %q", fp.dir)
	}

	// Navigate back by selecting ".." (cursor starts at 0 which is "..").
	fp, cmd = fp.Update(keyMsg("enter"))
	if fp.dir != "/testdir" {
		t.Errorf("expected dir='/testdir' after selecting '..', got %q", fp.dir)
	}
	if len(fp.dirStack) != 0 {
		t.Errorf("expected empty dirStack after back, got length %d", len(fp.dirStack))
	}

	// Execute the readDir command.
	if cmd != nil {
		msg := cmd()
		fp, _ = fp.Update(msg)
	}

	// Should see the original entries.
	if len(fp.filtered) == 0 {
		t.Error("expected entries after navigating back")
	}
}

func TestFuzzyPicker_SelectFile(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Move to "readme.md" (skip past the directory).
	for i := 0; i < len(fp.filtered); i++ {
		if fp.filtered[fp.cursor].Name() == "readme.md" {
			break
		}
		fp, _ = fp.Update(keyMsg("down"))
	}

	if fp.filtered[fp.cursor].Name() != "readme.md" {
		t.Fatalf("expected cursor on 'readme.md', got %q", fp.filtered[fp.cursor].Name())
	}

	fp, _ = fp.Update(keyMsg("enter"))
	didSelect, path := fp.DidSelect()
	if !didSelect {
		t.Error("expected selection after enter on allowed file")
	}
	if path != "/testdir/readme.md" {
		t.Errorf("expected path='/testdir/readme.md', got %q", path)
	}
}

func TestFuzzyPicker_DisabledFileNotSelected(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Find "image.png" (not an allowed type).
	for i := 0; i < len(fp.filtered); i++ {
		if fp.filtered[fp.cursor].Name() == "image.png" {
			break
		}
		fp, _ = fp.Update(keyMsg("down"))
	}

	if fp.filtered[fp.cursor].Name() != "image.png" {
		t.Skip("image.png not found in listing")
	}

	fp, _ = fp.Update(keyMsg("enter"))
	didSelect, _ := fp.DidSelect()
	if didSelect {
		t.Error("expected no selection on disabled file type")
	}
}

func TestFuzzyPicker_FilterClearsOnDirEnter(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Type "sub" to filter — matches "subdir" (and hides "..").
	for _, ch := range "sub" {
		fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if fp.filtered[fp.cursor].Name() != "subdir" {
		t.Fatalf("expected cursor on 'subdir', got %q", fp.filtered[fp.cursor].Name())
	}

	// Enter subdir.
	fp, _ = fp.Update(keyMsg("enter"))
	if fp.input.Value() != "" {
		t.Error("expected filter cleared after entering directory")
	}
}

func TestFuzzyPicker_FilterClearsOnNavigateBack(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Navigate into subdir (skip "..").
	fp, _ = fp.Update(keyMsg("down"))
	fp, cmd := fp.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		fp, _ = fp.Update(msg)
	}

	// Type ".." to filter to just the parent entry.
	fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: "."})
	fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: "."})
	if fp.filtered[0].Name() != ".." {
		t.Fatalf("expected '..' as first filtered entry, got %q", fp.filtered[0].Name())
	}

	// Navigate back by selecting "..".
	fp, cmd = fp.Update(keyMsg("enter"))

	if fp.input.Value() != "" {
		t.Error("expected filter cleared after navigating back via '..'")
	}
}

func TestFuzzyPicker_NoDuplicatesAfterRefilter(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)

	// Type a filter that matches one file (".." does not match "readme").
	for _, ch := range "readme" {
		fp, _ = fp.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}
	if len(fp.filtered) != 1 {
		t.Fatalf("expected 1 match for 'readme', got %d", len(fp.filtered))
	}

	// Delete the filter back to just "r" which matches more entries.
	for i := 0; i < 5; i++ { // delete "eadme"
		fp, _ = fp.Update(keyMsg("backspace"))
	}

	// Check for duplicates in filtered results.
	seen := map[string]int{}
	for _, e := range fp.filtered {
		seen[e.Name()]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("entry %q appears %d times in filtered results (expected 1)", name, count)
		}
	}
}

func TestFuzzyPicker_SetHeight(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp.SetHeight(5)
	if fp.height != 5 {
		t.Errorf("expected height=5, got %d", fp.height)
	}
}

func TestFuzzyPicker_SetWidth(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp.SetWidth(80)
	if fp.width != 80 {
		t.Errorf("expected width=80, got %d", fp.width)
	}
}

func TestFuzzyPicker_View(t *testing.T) {
	fp, _ := testFuzzyPicker()
	fp = applyReadDir(fp)
	fp.SetHeight(20)
	fp.SetWidth(80)

	view := fp.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	// Should contain the filter prompt.
	if len(view) < 10 {
		t.Error("expected view to have meaningful content")
	}
}

func TestFuzzyPicker_EmptyDirectory(t *testing.T) {
	fs := newMemFS()
	fs.wd = "/empty"
	fp := newFuzzyPicker("/empty", []string{".md"}, fs)
	fp = applyReadDir(fp)

	// Only ".." should be present (not at root).
	if len(fp.filtered) != 1 || fp.filtered[0].Name() != ".." {
		t.Errorf("expected only '..' entry, got %d entries", len(fp.filtered))
	}

	view := fp.View()
	if view == "" {
		t.Error("expected non-empty view even for empty directory")
	}
}

func TestFuzzyPicker_RootHasNoParentEntry(t *testing.T) {
	fs := newMemFS()
	fs.wd = "/"
	fs.files["/file.md"] = []byte("# Root file")
	fp := newFuzzyPicker("/", []string{".md"}, fs)
	fp = applyReadDir(fp)

	for _, e := range fp.filtered {
		if e.Name() == ".." {
			t.Error("root directory should not have '..' entry")
		}
	}
}

func TestSubsequenceMatch(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{"readme.md", "rmd", true},
		{"readme.md", "readme", true},
		{"readme.md", "xyz", false},
		{"notes.md", "nmd", true},
		{"notes.md", "nm", true},
		{"image.png", "img", true},
		{"image.png", "ipg", true},
		{"image.png", "ipa", false}, // 'a' comes before 'p' but after 'g'
		{"", "a", false},
		{"abc", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got := subsequenceMatch(tt.haystack, tt.needle)
		if got != tt.want {
			t.Errorf("subsequenceMatch(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
		}
	}
}

func TestFuzzyPicker_PageDown(t *testing.T) {
	fs := newMemFS()
	fs.wd = "/bigdir"
	// Create many files.
	for i := 0; i < 30; i++ {
		name := "/bigdir/" + string(rune('a'+i%26)) + string(rune('0'+i/26)) + ".md"
		fs.files[name] = []byte("content")
	}

	fp := newFuzzyPicker("/bigdir", []string{".md"}, fs)
	fp.SetHeight(10)
	fp = applyReadDir(fp)

	if len(fp.filtered) < 20 {
		t.Fatalf("expected at least 20 entries, got %d", len(fp.filtered))
	}

	// Page down.
	fp, _ = fp.Update(keyMsg("pgdown"))
	if fp.cursor < 5 {
		t.Errorf("expected cursor to advance on pgdown, got %d", fp.cursor)
	}
}
