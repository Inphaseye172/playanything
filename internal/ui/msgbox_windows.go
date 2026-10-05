//go:build windows

package ui

import (
	"syscall"
	"unsafe"
)

var (
	user32         = syscall.NewLazyDLL("user32.dll")
	procMessageBox = user32.NewProc("MessageBoxW")
)

const (
	mbOK              = 0x0
	mbIconError       = 0x10
	mbIconInformation = 0x40
	mbSetForeground   = 0x10000
	mbTopmost         = 0x40000
)

func messageBox(msg string, isError bool) {
	flags := uintptr(mbOK | mbSetForeground | mbTopmost)
	if isError {
		flags |= mbIconError
	} else {
		flags |= mbIconInformation
	}
	text, _ := syscall.UTF16PtrFromString(msg)
	title, _ := syscall.UTF16PtrFromString(Title)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), flags)
}
