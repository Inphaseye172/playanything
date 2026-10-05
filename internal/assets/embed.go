// Package assets embeds PlayAnything's tuned mpv configuration and Lua scripts
// and installs them into the managed mpv config directory.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed mpv/mpv.conf mpv/input.conf mpv/user.conf mpv/scripts/*.lua
var files embed.FS

// Revision is bumped whenever the embedded files change in a way that should
// overwrite what is on disk. The installer stores it in .pa-assets-rev.
const Revision = "5"

const stampName = ".pa-assets-rev"

// managed lists files that PlayAnything owns and rewrites on upgrade.
var managed = []string{"mpv.conf", "input.conf", "scripts/pa-rawhook.lua", "scripts/pa-autoload.lua", "scripts/pa-osd.lua"}

// userOnce lists files written only when absent (user-owned).
var userOnce = []string{"user.conf"}

// Install writes the mpv config tree into dir. It is idempotent and cheap: when
// the stamp matches Revision and all managed files exist, nothing is written.
// It returns true when files were (re)written.
func Install(dir string, force bool) (bool, error) {
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		return false, err
	}
	stamp, _ := os.ReadFile(filepath.Join(dir, stampName))
	upToDate := strings.TrimSpace(string(stamp)) == Revision
	if upToDate && !force {
		for _, name := range managed {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				upToDate = false
				break
			}
		}
	}
	wrote := false
	if !upToDate || force {
		for _, name := range managed {
			if err := write(dir, name); err != nil {
				return wrote, err
			}
			wrote = true
		}
		if err := os.WriteFile(filepath.Join(dir, stampName), []byte(Revision+"\n"), 0o644); err != nil {
			return wrote, err
		}
	}
	for _, name := range userOnce {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			if err := write(dir, name); err != nil {
				return wrote, err
			}
			wrote = true
		}
	}
	return wrote, nil
}

func write(dir, name string) error {
	data, err := fs.ReadFile(files, "mpv/"+name)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

// Read returns an embedded file (for `playanything setup --print`).
func Read(name string) ([]byte, error) { return fs.ReadFile(files, "mpv/"+name) }

// Files lists the embedded file names relative to the mpv dir.
func Files() []string { return append(append([]string{}, managed...), userOnce...) }
