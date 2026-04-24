package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestRegistry(extensions []string) *converterRegistry {
	return newConverterRegistry([]formatConverterConfig{{
		Command:    "cat $MD_INPUT",
		Extensions: extensions,
	}}, nil)
}

func TestIsConvertibleFile_NilRegistry(t *testing.T) {
	assert.False(t, isConvertibleFile("doc.rst", nil),
		"nil registry → no conversions; always false")
}

func TestIsConvertibleFile_MatchesRegisteredExtension(t *testing.T) {
	reg := newTestRegistry([]string{".rst", ".org"})
	assert.True(t, isConvertibleFile("doc.rst", reg))
	assert.True(t, isConvertibleFile("doc.org", reg))
}

func TestIsConvertibleFile_IgnoresUnregisteredExtension(t *testing.T) {
	reg := newTestRegistry([]string{".rst"})
	assert.False(t, isConvertibleFile("doc.md", reg))
	assert.False(t, isConvertibleFile("doc.txt", reg))
	assert.False(t, isConvertibleFile("noext", reg))
}

func TestIsConvertibleFile_CaseInsensitive(t *testing.T) {
	reg := newTestRegistry([]string{".rst"})
	assert.True(t, isConvertibleFile("DOC.RST", reg))
	assert.True(t, isConvertibleFile("Doc.Rst", reg))
}

func TestIsConvertibleFile_StripsFragment(t *testing.T) {
	reg := newTestRegistry([]string{".rst"})
	assert.True(t, isConvertibleFile("doc.rst#section", reg))
	// Fragment-only URL with no extension is not convertible.
	assert.False(t, isConvertibleFile("#foo", reg))
}

func TestIsConvertibleFile_PathWithDirectory(t *testing.T) {
	reg := newTestRegistry([]string{".rst"})
	assert.True(t, isConvertibleFile("/path/to/doc.rst", reg))
	assert.True(t, isConvertibleFile("./sub/DOC.RST", reg))
}

func TestStripDataURIs_DefaultTrue(t *testing.T) {
	// Unset (nil pointer) means enabled.
	c := config{}
	assert.True(t, c.stripDataURIs(),
		"stripDataURIs must default to enabled when unset")
}

func TestStripDataURIs_ExplicitTrue(t *testing.T) {
	v := true
	c := config{StripDataURIs: &v}
	assert.True(t, c.stripDataURIs())
}

func TestStripDataURIs_ExplicitFalse(t *testing.T) {
	v := false
	c := config{StripDataURIs: &v}
	assert.False(t, c.stripDataURIs(),
		"explicit false should disable data URI stripping")
}

func TestConfigPath_ReturnsPathContainingMd(t *testing.T) {
	// configPath uses os.UserConfigDir(). The result is OS-specific but
	// must always contain "md" as a directory segment and end in config.toml.
	p, err := configPath()
	if err != nil {
		// Some sandboxed environments reject UserConfigDir; skip in that case.
		t.Skipf("UserConfigDir unavailable: %v", err)
	}
	assert.True(t, strings.HasSuffix(p, "config.toml"),
		"configPath should end in config.toml, got %q", p)
	assert.Contains(t, p, "md", "configPath should include the md namespace, got %q", p)
}
