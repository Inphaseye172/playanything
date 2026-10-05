//go:build windows

package mpv

import (
	"io"
	"os"
)

// mpv on Windows exposes the IPC as a byte-mode duplex named pipe which can be
// opened like a file.
func dial(path string) (io.ReadWriteCloser, error) {
	return os.OpenFile(path, os.O_RDWR, 0)
}
