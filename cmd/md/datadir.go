package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// dataDir returns the directory for md data files, following XDG conventions.
//
// Resolution order:
//  1. $XDG_DATA_HOME/md/
//  2. Platform default:
//     - macOS: ~/Library/Application Support/md/
//     - Windows: %LOCALAPPDATA%\md\data\
//     - Linux/other: ~/.local/share/md/
func dataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "md"), nil
	}

	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "md"), nil
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			local = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(local, "md", "data"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "md"), nil
	}
}
