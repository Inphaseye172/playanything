//go:build !windows

package main

import "os"

// platformInit reports whether we were started without a terminal (e.g. from a
// .desktop file or Finder), in which case errors are shown as dialogs.
func platformInit() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
