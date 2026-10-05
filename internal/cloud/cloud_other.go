//go:build !windows && !darwin && !linux

package cloud

func platformStatus(info *Info) {}
