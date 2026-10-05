//go:build windows

package cloud

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Windows file attribute bits set by the Cloud Files API (cldflt.sys), which
// Synology Drive On-demand Sync, OneDrive Files On-Demand, Dropbox, Google
// Drive and others all use on Windows 10 1809+.
const (
	attrReparsePoint       = 0x00000400
	attrOffline            = 0x00001000
	attrRecallOnOpen       = 0x00040000
	attrPinned             = 0x00080000
	attrUnpinned           = 0x00100000
	attrRecallOnDataAccess = 0x00400000

	driveRemote = 4
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGetDriveType = kernel32.NewProc("GetDriveTypeW")
)

func platformStatus(info *Info) {
	p := info.Path
	if strings.HasPrefix(p, `\\`) {
		info.IsNetwork = true
		info.Detail = "UNC path"
	} else if vol := filepath.VolumeName(p); len(vol) == 2 {
		root, _ := syscall.UTF16PtrFromString(vol + `\`)
		if r, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(root))); r == driveRemote {
			info.IsNetwork = true
			info.Detail = "mapped network drive"
		}
	}
	if !info.Exists {
		return
	}
	wp, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return
	}
	attrs, err := syscall.GetFileAttributes(wp)
	if err != nil {
		return
	}
	var flags []string
	if attrs&attrRecallOnDataAccess != 0 {
		info.IsPlaceholder, info.IsOffline = true, true
		flags = append(flags, "RECALL_ON_DATA_ACCESS")
	}
	if attrs&attrRecallOnOpen != 0 {
		info.IsPlaceholder, info.IsOffline = true, true
		flags = append(flags, "RECALL_ON_OPEN")
	}
	if attrs&attrOffline != 0 {
		info.IsPlaceholder, info.IsOffline = true, true
		flags = append(flags, "OFFLINE")
	}
	if attrs&attrUnpinned != 0 {
		info.IsPlaceholder = true
		flags = append(flags, "UNPINNED")
	}
	if attrs&attrPinned != 0 {
		info.IsPlaceholder = true
		flags = append(flags, "PINNED")
	}
	if attrs&attrReparsePoint != 0 && info.Provider != "" {
		info.IsPlaceholder = true
		flags = append(flags, "REPARSE_POINT")
	}
	if len(flags) > 0 {
		d := fmt.Sprintf("attributes 0x%X: %s", attrs, strings.Join(flags, "|"))
		if info.Detail != "" {
			d = info.Detail + "; " + d
		}
		info.Detail = d
	}
}
