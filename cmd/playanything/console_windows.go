//go:build windows

package main

import (
	"os"
	"syscall"
)

// The Windows binary is linked as a GUI application (-H windowsgui) so that a
// double-click in Explorer never flashes a console window. When run from a
// terminal we attach to that terminal's console so `doctor`, `info` and
// error messages still print. Returns true when no console exists (GUI launch).
func platformInit() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // (DWORD)-1
	r, _, _ := attach.Call(attachParentProcess)
	if r == 0 {
		return true
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = out
		os.Stderr = out
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = in
	}
	return false
}
