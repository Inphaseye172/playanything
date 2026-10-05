// Package mpv locates, launches and talks to mpv - the playback engine
// PlayAnything is built on. mpv (https://mpv.io) wraps FFmpeg, so it plays
// every container and codec FFmpeg knows, with GPU decoding (NVDEC, D3D11VA,
// VAAPI, VideoToolbox), 10/12-bit and HDR output, and a JSON IPC interface.
package mpv

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Binary is how to invoke mpv: an executable plus fixed leading arguments
// (needed for Flatpak, where mpv is `flatpak run io.mpv.Mpv`).
type Binary struct {
	Path   string
	Prefix []string
	Origin string // where it was found, for `doctor`
}

// Command builds an exec.Cmd for this binary with args appended.
func (b Binary) Command(args ...string) *exec.Cmd {
	all := append(append([]string{}, b.Prefix...), args...)
	return exec.Command(b.Path, all...)
}

// ErrNotFound is returned when no mpv could be located.
var ErrNotFound = errors.New("mpv not found: install it (see README) or set mpv_path in config.json")

// Find locates mpv. Order: explicit override, $PLAYANYTHING_MPV, PlayAnything's
// managed copy, PATH, well-known install locations for winget/scoop/choco/
// Homebrew/Flatpak/Snap.
func Find(override, managedDir string) (Binary, error) {
	exe := "mpv"
	if runtime.GOOS == "windows" {
		exe = "mpv.exe"
	}
	try := func(p, origin string) (Binary, bool) {
		if p == "" {
			return Binary{}, false
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return Binary{Path: p, Origin: origin}, true
		}
		return Binary{}, false
	}
	if override != "" {
		if b, ok := try(override, "config"); ok {
			return b, nil
		}
		if p, err := exec.LookPath(override); err == nil {
			return Binary{Path: p, Origin: "config"}, nil
		}
		return Binary{}, errors.New("configured mpv_path does not exist: " + override)
	}
	if env := os.Getenv("PLAYANYTHING_MPV"); env != "" {
		if b, ok := try(env, "PLAYANYTHING_MPV"); ok {
			return b, nil
		}
	}
	if managedDir != "" {
		if b, ok := try(filepath.Join(managedDir, exe), "managed"); ok {
			return b, nil
		}
	}
	// Look up the exact file name: on Windows a bare "mpv" would resolve the
	// console helper mpv.com first and flash a console window.
	if p, err := exec.LookPath(exe); err == nil {
		return Binary{Path: p, Origin: "PATH"}, nil
	}
	for _, c := range candidates() {
		matches, _ := filepath.Glob(c)
		for _, m := range matches {
			if b, ok := try(m, "well-known location"); ok {
				return b, nil
			}
		}
	}
	if runtime.GOOS == "linux" {
		if fp, err := exec.LookPath("flatpak"); err == nil {
			if out, err := exec.Command(fp, "info", "io.mpv.Mpv").Output(); err == nil && len(out) > 0 {
				return Binary{Path: fp, Prefix: []string{"run", "--file-forwarding", "io.mpv.Mpv"}, Origin: "flatpak"}, nil
			}
		}
	}
	return Binary{}, ErrNotFound
}

func candidates() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		pf := env("ProgramFiles", `C:\Program Files`)
		pf86 := env("ProgramFiles(x86)", `C:\Program Files (x86)`)
		local := env("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
		pd := env("ProgramData", `C:\ProgramData`)
		return []string{
			filepath.Join(pf, "mpv", "mpv.exe"), // winget shinchiro.mpv / manual
			filepath.Join(pf86, "mpv", "mpv.exe"),
			filepath.Join(local, "Programs", "mpv", "mpv.exe"),
			filepath.Join(local, "Microsoft", "WinGet", "Packages", "*mpv*", "mpv.exe"),
			filepath.Join(local, "Microsoft", "WinGet", "Packages", "*mpv*", "*", "mpv.exe"),
			filepath.Join(home, "scoop", "apps", "mpv", "current", "mpv.exe"),
			filepath.Join(home, "scoop", "shims", "mpv.exe"),
			filepath.Join(pd, "chocolatey", "bin", "mpv.exe"),
			filepath.Join(pd, "chocolatey", "lib", "mpv", "tools", "mpv.exe"),
			filepath.Join(pd, "chocolatey", "lib", "mpv", "tools", "mpv-*", "mpv.exe"),
			`C:\mpv\mpv.exe`,
			`D:\mpv\mpv.exe`,
		}
	case "darwin":
		return []string{
			"/opt/homebrew/bin/mpv",
			"/usr/local/bin/mpv",
			"/opt/local/bin/mpv", // MacPorts
			"/Applications/mpv.app/Contents/MacOS/mpv",
			filepath.Join(home, "Applications", "mpv.app", "Contents", "MacOS", "mpv"),
		}
	default:
		return []string{
			"/usr/bin/mpv", "/usr/local/bin/mpv", "/snap/bin/mpv",
			filepath.Join(home, ".local", "bin", "mpv"),
			filepath.Join(home, ".nix-profile", "bin", "mpv"),
			"/var/lib/flatpak/exports/bin/io.mpv.Mpv",
			filepath.Join(home, ".local", "share", "flatpak", "exports", "bin", "io.mpv.Mpv"),
		}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Version runs `mpv --version` and returns the first line.
func Version(b Binary) (string, error) {
	out, err := b.Command("--version").Output()
	if err != nil {
		return "", err
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return line, nil
}

// HWDecoders returns the hardware decoders compiled into this mpv
// (nvdec, d3d11va, vaapi, videotoolbox, ...), de-duplicated.
func HWDecoders(b Binary) ([]string, error) {
	out, err := b.Command("--hwdec=help").CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	seen := map[string]bool{}
	var list []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Valid values") {
			continue
		}
		name := strings.Fields(line)[0]
		if strings.HasSuffix(name, "-copy") || name == "auto" || name == "auto-safe" || name == "auto-copy" || name == "no" || name == "yes" {
			continue
		}
		if !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}
	return list, nil
}

// HasVO reports whether a video output (e.g. gpu-next) is available.
func HasVO(b Binary, vo string) bool {
	out, _ := b.Command("--vo=help").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == vo {
			return true
		}
	}
	return false
}
