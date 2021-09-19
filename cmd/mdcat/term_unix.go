//go:build !windows
// +build !windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func terminalGeometry() (cols, rows, width, height int, ok bool) {
	var winsize struct {
		row, col       uint16
		xpixel, ypixel uint16
	}
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(os.Stdout.Fd()), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&winsize)))
	if err != 0 {
		return 0, 0, 0, 0, false
	}
	return int(winsize.col), int(winsize.row), int(winsize.xpixel), int(winsize.ypixel), true
}

func canDisplayImages() bool {
	return os.Getenv("TERM") == "xterm-kitty"
}
