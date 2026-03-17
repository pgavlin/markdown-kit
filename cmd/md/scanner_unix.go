//go:build !windows

package main

import (
	"os"
	"syscall"
)

func signalProcess(p *os.Process) error {
	return p.Signal(syscall.Signal(0))
}
