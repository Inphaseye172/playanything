//go:build darwin

package cloud

import (
	"strings"
	"syscall"
)

// SF_DATALESS marks a File Provider / iCloud placeholder whose contents are
// not on disk (macOS 10.15+). Synology Drive 3.x on macOS uses File Provider.
const sfDataless = 0x40000000

func platformStatus(info *Info) {
	if !info.Exists {
		return
	}
	var st syscall.Stat_t
	if err := syscall.Lstat(info.Path, &st); err == nil {
		if st.Flags&sfDataless != 0 {
			info.IsPlaceholder, info.IsOffline = true, true
			info.Detail = "SF_DATALESS (File Provider placeholder)"
		}
	}
	if strings.Contains(info.Path, "/Library/CloudStorage/") || strings.Contains(info.Path, "/Mobile Documents/") {
		info.IsPlaceholder = true
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(info.Path, &fs); err == nil {
		name := cstr(fs.Fstypename[:])
		switch name {
		case "smbfs", "nfs", "afpfs", "webdav", "cifs":
			info.IsNetwork = true
			if info.Detail != "" {
				info.Detail += "; "
			}
			info.Detail += name + " mount"
		}
	}
}

func cstr(b []int8) string {
	var sb strings.Builder
	for _, c := range b {
		if c == 0 {
			break
		}
		sb.WriteByte(byte(c))
	}
	return sb.String()
}
