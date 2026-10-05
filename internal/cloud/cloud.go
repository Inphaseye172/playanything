// Package cloud detects files that are not really local: cloud-sync
// placeholders (Synology Drive On-demand Sync, OneDrive Files On-Demand,
// Dropbox online-only, Google Drive streaming, iCloud "optimize storage") and
// files on network shares (SMB/NFS/UNC paths).
//
// Explorer and Finder refuse to generate previews for such files because the
// bytes are not on disk yet. PlayAnything instead opens them immediately and
// lets the sync client fetch the bytes as mpv reads them, optionally
// pre-downloading ("hydrating") the whole file first with a progress readout.
package cloud

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Info describes how "local" a path really is.
type Info struct {
	Path          string
	Exists        bool
	IsPlaceholder bool   // cloud placeholder: contents are fetched on first read
	IsOffline     bool   // placeholder whose bytes are not on disk right now
	IsNetwork     bool   // on an SMB/NFS/UNC/WebDAV mount
	Provider      string // "Synology Drive", "OneDrive", ... (best guess from the path), or ""
	Size          int64
	Detail        string // platform-specific note for `playanything info`
}

// Remote reports whether reading the file will involve the network: dataless
// placeholders and network shares. A placeholder whose bytes are cached or
// pinned locally reads at disk speed.
func (i Info) Remote() bool { return i.IsOffline || i.IsNetwork }

// Describe returns a short human sentence for OSD / terminal use.
func (i Info) Describe() string {
	who := i.Provider
	switch {
	case i.IsOffline && who != "":
		return fmt.Sprintf("cloud file, not stored locally yet (%s)", who)
	case i.IsOffline:
		return "cloud placeholder, not stored locally yet"
	case i.IsPlaceholder && who != "":
		return fmt.Sprintf("%s file (cached locally)", who)
	case i.IsPlaceholder:
		return "cloud placeholder (cached locally)"
	case i.IsNetwork:
		return "network share"
	}
	return "local file"
}

// Status inspects path. It never fails on unsupported platforms; it degrades to
// path-based heuristics.
func Status(path string) Info {
	info := Info{Path: path}
	fi, err := os.Stat(path)
	if err == nil {
		info.Exists = true
		info.Size = fi.Size()
	}
	info.Provider = guessProvider(path)
	platformStatus(&info)
	if info.Provider != "" && !info.IsPlaceholder && !info.IsOffline && info.Exists {
		// Inside a sync folder but fully present: nothing special to do.
		info.Detail = strings.TrimSpace(info.Detail + " synced copy present")
	}
	return info
}

// guessProvider infers the sync service from folder names in the path.
func guessProvider(path string) string {
	p := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(p, "synologydrive") || strings.Contains(p, "synology drive") || strings.Contains(p, "/drive/") && strings.Contains(p, "synology"):
		return "Synology Drive"
	case strings.Contains(p, "onedrive"):
		return "OneDrive"
	case strings.Contains(p, "dropbox"):
		return "Dropbox"
	case strings.Contains(p, "google drive") || strings.Contains(p, "googledrive") || strings.Contains(p, "drivefs"):
		return "Google Drive"
	case strings.Contains(p, "icloud") || strings.Contains(p, "mobile documents"):
		return "iCloud Drive"
	case strings.Contains(p, "nextcloud"):
		return "Nextcloud"
	case strings.Contains(p, "/cloudstorage/"):
		return "File Provider"
	}
	return ""
}

// Progress is called during Hydrate with bytes read so far and the total size
// (-1 when unknown).
type Progress func(done, total int64)

// Hydrate forces the sync client to download the whole file by reading it from
// start to end. The bytes are discarded; the side effect is that the file
// becomes fully local, so seeking in mpv is instant afterwards.
func Hydrate(path string, progress Progress) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	total := int64(-1)
	if fi, err := f.Stat(); err == nil {
		total = fi.Size()
	}
	buf := make([]byte, 4<<20)
	var done int64
	for {
		n, err := f.Read(buf)
		done += int64(n)
		if progress != nil && n > 0 {
			progress(done, total)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
