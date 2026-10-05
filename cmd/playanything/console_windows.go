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
//
// Standard handles that are already valid (PowerShell capturing our output
// through a pipe, mpv's Lua hook reading `raw-preview`, redirection to a file)
// are left untouched: only missing handles are pointed at the console.
func platformInit() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // (DWORD)-1

	hadIn := validStdHandle(syscall.STD_INPUT_HANDLE)
	hadOut := validStdHandle(syscall.STD_OUTPUT_HANDLE)
	hadErr := validStdHandle(syscall.STD_ERROR_HANDLE)

	r, _, _ := attach.Call(attachParentProcess)
	if r == 0 {
		// No parent console: Explorer double-click, "Open with", a scheduled
		// task... but if stdout is a pipe (mpv's subprocess) it still works.
		return !hadOut
	}
	if !hadOut || !hadErr {
		if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			if !hadOut {
				os.Stdout = out
			}
			if !hadErr {
				os.Stderr = out
			}
		}
	}
	if !hadIn {
		if in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
			os.Stdin = in
		}
	}
	return false
}

func validStdHandle(which int) bool {
	h, err := syscall.GetStdHandle(which)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return false
	}
	t, err := syscall.GetFileType(h)
	return err == nil && t != syscall.FILE_TYPE_UNKNOWN
}
