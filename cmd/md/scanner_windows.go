//go:build windows

package main

import "os"

// On Windows, os.FindProcess succeeds only for existing PIDs.
func signalProcess(p *os.Process) error {
	return p.Release()
}
