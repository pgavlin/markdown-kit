//go:build windows

package main

import "os"

// openTTY opens the controlling terminal for keyboard input when stdin is piped.
func openTTY() (*os.File, error) {
	return os.Open("CONIN$")
}
