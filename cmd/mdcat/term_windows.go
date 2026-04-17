//go:build windows
// +build windows

package main

import (
	"os"

	"golang.org/x/term"
)

func terminalGeometry() (cols, rows, width, height int, ok bool) {
	c, r, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, 0, 0, false
	}
	return c, r, 0, 0, true
}

func canDisplayImages() bool {
	return false
}
