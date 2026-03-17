//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr configures the command to run as a detached daemon.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
