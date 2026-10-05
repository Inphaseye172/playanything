//go:build !windows

package mpv

import (
	"io"
	"net"
	"time"
)

func dial(path string) (io.ReadWriteCloser, error) {
	return net.DialTimeout("unix", path, time.Second)
}
