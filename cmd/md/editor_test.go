package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envFromMap(m map[string]string) func(string) string {
	return func(name string) string { return m[name] }
}

func TestEditorCommand_DefaultsToVi(t *testing.T) {
	args, err := editorCommand(envFromMap(nil), "/tmp/doc.md", 12)
	require.NoError(t, err)
	assert.Equal(t, []string{"vi", "+12", "/tmp/doc.md"}, args)
}

func TestEditorCommand_VisualBeatsEditor(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{
		"VISUAL": "nvim",
		"EDITOR": "nano",
	}), "/tmp/doc.md", 7)
	require.NoError(t, err)
	assert.Equal(t, []string{"nvim", "+7", "/tmp/doc.md"}, args)
}

func TestEditorCommand_PlusNFamily(t *testing.T) {
	for _, ed := range []string{"vi", "vim", "nvim", "nano", "emacs", "jed", "joe", "kak", "kakoune"} {
		args, err := editorCommand(envFromMap(map[string]string{"EDITOR": ed}), "/x.md", 3)
		require.NoError(t, err, ed)
		assert.Equal(t, []string{ed, "+3", "/x.md"}, args, ed)
	}
}

func TestEditorCommand_VSCodeFamily(t *testing.T) {
	for _, ed := range []string{"code", "code-insiders", "cursor", "vscodium", "codium"} {
		args, err := editorCommand(envFromMap(map[string]string{"EDITOR": ed}), "/x.md", 3)
		require.NoError(t, err, ed)
		assert.Equal(t, []string{ed, "--wait", "-g", "/x.md:3"}, args, ed)
	}
}

func TestEditorCommand_FileColonLineFamily(t *testing.T) {
	cases := map[string][]string{
		"subl":  {"subl", "--wait", "/x.md:3"},
		"zed":   {"zed", "--wait", "/x.md:3"},
		"helix": {"helix", "/x.md:3"},
		"hx":    {"hx", "/x.md:3"},
		"micro": {"micro", "/x.md:3"},
	}
	for ed, want := range cases {
		args, err := editorCommand(envFromMap(map[string]string{"EDITOR": ed}), "/x.md", 3)
		require.NoError(t, err, ed)
		assert.Equal(t, want, args, ed)
	}
}

func TestEditorCommand_JetBrainsFamily(t *testing.T) {
	for _, ed := range []string{"idea", "goland", "pycharm", "webstorm", "rubymine", "clion", "phpstorm", "rider", "datagrip", "fleet"} {
		args, err := editorCommand(envFromMap(map[string]string{"EDITOR": ed}), "/x.md", 3)
		require.NoError(t, err, ed)
		assert.Equal(t, []string{ed, "--line", "3", "/x.md"}, args, ed)
	}
}

func TestEditorCommand_PreservesUserFlags(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{
		"EDITOR": "emacs -nw",
	}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"emacs", "-nw", "+3", "/x.md"}, args)
}

func TestEditorCommand_DoesNotInjectWaitWhenAlreadyPresent(t *testing.T) {
	for _, flag := range []string{"-w", "--wait", "-n"} {
		args, err := editorCommand(envFromMap(map[string]string{
			"EDITOR": "code " + flag,
		}), "/x.md", 3)
		require.NoError(t, err, flag)
		assert.Equal(t, []string{"code", flag, "-g", "/x.md:3"}, args, flag)
	}
}

func TestEditorCommand_QuotedFlags(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{
		"EDITOR": `vim "+set ft=markdown"`,
	}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"vim", "+set ft=markdown", "+3", "/x.md"}, args)
}

func TestEditorCommand_ExpandsEnvVars(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{
		"EDITOR":    "$MY_EDITOR",
		"MY_EDITOR": "nvim",
	}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"nvim", "+3", "/x.md"}, args)
}

func TestEditorCommand_UnknownEditorUsesPlusN(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{"EDITOR": "myedit"}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"myedit", "+3", "/x.md"}, args)
}

func TestEditorCommand_EmptyEditorAfterParse(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{"EDITOR": "   "}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"vi", "+3", "/x.md"}, args)
}

func TestEditorCommand_BasenameDetection(t *testing.T) {
	args, err := editorCommand(envFromMap(map[string]string{"EDITOR": "/usr/local/bin/code"}), "/x.md", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"/usr/local/bin/code", "--wait", "-g", "/x.md:3"}, args)
}

func TestFindSourceLine_FirstOccurrence(t *testing.T) {
	src := "# Top\n\nfirst line\n\nrepeat\n\nrepeat\n"
	assert.Equal(t, 3, findSourceLine([]byte(src), "first line", 0))
}

func TestFindSourceLine_NthOccurrence(t *testing.T) {
	src := "# Top\n\nrepeat\n\nrepeat\n"
	assert.Equal(t, 5, findSourceLine([]byte(src), "repeat", 1))
}

func TestFindSourceLine_OvershotOccurrenceFallsBackToFirst(t *testing.T) {
	src := "# Top\n\nrepeat\n"
	assert.Equal(t, 3, findSourceLine([]byte(src), "repeat", 5))
}

func TestFindSourceLine_NotFoundReturnsOne(t *testing.T) {
	src := "# Top\n\nbody\n"
	assert.Equal(t, 1, findSourceLine([]byte(src), "missing", 0))
}

func TestFindSourceLine_EmptySnippetReturnsOne(t *testing.T) {
	src := "# Top\n"
	assert.Equal(t, 1, findSourceLine([]byte(src), "", 0))
}

func TestFindSourceLine_TrimsWhitespaceForComparison(t *testing.T) {
	src := "# Top\n\n   indented body   \n"
	assert.Equal(t, 3, findSourceLine([]byte(src), "indented body", 0))
}

func TestCanEditSource(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   bool
	}{
		{"empty", "", false},
		{"http", "http://example.com/x.md", false},
		{"https", "https://example.com/x.md", false},
		{"markdown ext", "/tmp/doc.md", true},
		{"markdown alt ext", "/tmp/doc.markdown", true},
		{"convertible docx", "/tmp/doc.docx", false},
		{"unknown ext", "/tmp/doc.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canEditSource(tc.source))
		})
	}
}
