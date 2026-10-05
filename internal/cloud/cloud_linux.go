//go:build linux

package cloud

import (
	"fmt"
	"syscall"
)

// Filesystem magic numbers from <linux/magic.h>.
const (
	magicCIFS    = 0xFF534D42
	magicSMB2    = 0xFE534D42
	magicNFS     = 0x6969
	magicFUSE    = 0x65735546
	magicCODA    = 0x73757245
	magicAFS     = 0x5346414F
	magicCEPH    = 0x00C36400
	magic9P      = 0x01021997
	magicOCFS2   = 0x7461636f
	magicGFS2    = 0x01161970
	magicSSHFS   = magicFUSE // sshfs/rclone/davfs2 are FUSE
	magicOverlay = 0x794C7630
)

func platformStatus(info *Info) {
	if !info.Exists {
		return
	}
	var fs syscall.Statfs_t
	if err := syscall.Statfs(info.Path, &fs); err != nil {
		return
	}
	switch uint32(fs.Type) {
	case magicCIFS, magicSMB2:
		info.IsNetwork = true
		info.Detail = "CIFS/SMB mount"
	case magicNFS:
		info.IsNetwork = true
		info.Detail = "NFS mount"
	case magicFUSE:
		// rclone, sshfs, davfs2, gvfs: treat as network when it looks like a cloud folder.
		info.IsNetwork = info.Provider != ""
		info.Detail = "FUSE filesystem"
	case magicCODA, magicAFS, magicCEPH, magic9P, magicOCFS2, magicGFS2:
		info.IsNetwork = true
		info.Detail = fmt.Sprintf("network filesystem (0x%X)", uint32(fs.Type))
	}
}
