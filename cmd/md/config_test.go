package main

import (
	"testing"

	"github.com/pgavlin/markdown-kit/styles"
)

func TestConverterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     converterConfig
		wantErr bool
	}{
		{"builtin_explicit", converterConfig{Method: "builtin"}, false},
		{"empty_method", converterConfig{}, false},
		{"external_with_cmd", converterConfig{Method: "external", Command: "pandoc"}, false},
		{"external_no_cmd", converterConfig{Method: "external"}, true},
		{"unknown", converterConfig{Method: "magic"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Theme(t *testing.T) {
	tests := []struct {
		name      string
		theme     string
		wantStyle string
	}{
		{"empty_returns_glamour_dark", "", styles.GlamourDark.Name},
		{"known_theme", "monokai", "monokai"},
		{"unknown_returns_glamour_dark", "nonexistent-theme-xyz", styles.GlamourDark.Name},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{Theme: tt.theme}
			got := cfg.theme()
			if got.Name != tt.wantStyle {
				t.Errorf("theme().Name = %q, want %q", got.Name, tt.wantStyle)
			}
		})
	}
}

func TestConfig_ApplyKeys(t *testing.T) {
	t.Run("override", func(t *testing.T) {
		km := defaultReaderKeyMap()
		cfg := config{
			Keys: map[string]any{
				"quit": "x",
			},
		}
		cfg.applyKeys(&km)
		h := km.Quit.Help()
		if h.Key != "x" {
			t.Errorf("expected quit help key = %q, got %q", "x", h.Key)
		}
	})

	t.Run("array", func(t *testing.T) {
		km := defaultReaderKeyMap()
		cfg := config{
			Keys: map[string]any{
				"quit": []any{"x", "ctrl+q"},
			},
		}
		cfg.applyKeys(&km)
		h := km.Quit.Help()
		if h.Key != "x/ctrl+q" {
			t.Errorf("expected quit help key = %q, got %q", "x/ctrl+q", h.Key)
		}
	})

	t.Run("unknown_ignored", func(t *testing.T) {
		km := defaultReaderKeyMap()
		origHelp := km.Help.Help()
		cfg := config{
			Keys: map[string]any{
				"nonexistent_key": "z",
			},
		}
		cfg.applyKeys(&km)
		// Verify no change to existing bindings.
		if km.Help.Help() != origHelp {
			t.Error("unknown key should not affect existing bindings")
		}
	})
}

func TestLoadConfig_ExistingFile(t *testing.T) {
	fs := newMemFS()
	fs.files["/config.toml"] = []byte(`theme = "monokai"
[converter]
method = "builtin"
`)
	cfg, err := loadConfig("/config.toml", fs, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Theme != "monokai" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "monokai")
	}
	if cfg.Converter.Method != "builtin" {
		t.Errorf("Converter.Method = %q, want %q", cfg.Converter.Method, "builtin")
	}
}

func TestLoadConfig_MissingCreatesDefault(t *testing.T) {
	fs := newMemFS()
	cfg, err := loadConfig("/config/md/config.toml", fs, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty config.
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want empty", cfg.Theme)
	}

	// Default config should have been written.
	data, ok := fs.files["/config/md/config.toml"]
	if !ok {
		t.Fatal("expected default config file to be created")
	}
	if len(data) == 0 {
		t.Error("expected non-empty default config")
	}
}

func TestLoadConfig_MalformedTOML(t *testing.T) {
	fs := newMemFS()
	fs.files["/config.toml"] = []byte(`[[[invalid toml`)

	_, err := loadConfig("/config.toml", fs, discardLogger())
	if err == nil {
		t.Fatal("expected error for malformed TOML")
	}
}

func TestLoadConfig_PartialConfig(t *testing.T) {
	fs := newMemFS()
	fs.files["/config.toml"] = []byte(`theme = "dracula"`)

	cfg, err := loadConfig("/config.toml", fs, discardLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "dracula")
	}
	if cfg.Converter.Method != "" {
		t.Errorf("Converter.Method = %q, want empty", cfg.Converter.Method)
	}
}
