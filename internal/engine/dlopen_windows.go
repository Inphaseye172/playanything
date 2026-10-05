//go:build windows

package engine

import "syscall"

// dlopen loads a DLL. purego on Windows expects handles from LoadLibrary.
func dlopen(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}
