// Package config resolves PlayAnything's directories and user settings.
//
// Layout (per user, never needs admin):
//
//	<config>/config.json     user settings (this file)
//	<config>/mpv/            mpv.conf, input.conf, scripts/ (managed), user.conf (yours)
//	<cache>/raw-previews/    extracted camera RAW previews
//	<cache>/r3d-proxies/     REDline proxy renders
//	<runtime>/daemon.pid     background player pid
//	<runtime>/mpv.sock       mpv JSON IPC socket (named pipe on Windows)
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config is persisted as config.json. Zero values mean "auto".
type Config struct {
	// Path to the mpv executable. Empty = auto-detect.
	MPVPath string `json:"mpv_path,omitempty"`
	// Use the user's own ~/.config/mpv instead of PlayAnything's tuned profile.
	UseSystemMPVConfig bool `json:"use_system_mpv_config,omitempty"`
	// "auto" (use the background player when it is running), "always"
	// (start it on demand), or "never" (one mpv process per file).
	Daemon string `json:"daemon,omitempty"`
	// Download cloud placeholders completely before playing (progress shown).
	HydrateFirst bool `json:"hydrate_first,omitempty"`
	// Develop camera RAW files with an external decoder instead of showing the
	// embedded preview. Slow; needs darktable-cli, rawtherapee-cli or dcraw_emu.
	RawFullDecode bool `json:"raw_full_decode,omitempty"`
	// Which RAW developer to use when RawFullDecode is on: "auto", "darktable-cli", "rawtherapee-cli", "dcraw_emu".
	RawDecoder string `json:"raw_decoder,omitempty"`
	// Path to RED's REDline executable. Empty = auto-detect.
	REDlinePath string `json:"redline_path,omitempty"`
	// REDline argument template. Placeholders: {input} {outbase} {outdir}.
	REDlineArgs string `json:"redline_args,omitempty"`
	// Extra arguments appended to every mpv launch.
	ExtraMPVArgs []string `json:"extra_mpv_args,omitempty"`
	// Start playback in fullscreen.
	Fullscreen bool `json:"fullscreen,omitempty"`
}

// DefaultREDlineArgs renders a quarter-resolution ProRes proxy, which plays
// smoothly on any machine. See docs/RED-R3D.md for other codecs.
const DefaultREDlineArgs = "--i {input} --o {outbase} --outDir {outdir} --format 11 --QTcodec 2 --res 4"

// Paths groups the resolved directories.
type Paths struct {
	Config  string
	MPV     string // mpv config dir (managed)
	Cache   string
	Runtime string
}

// AppName is used for directory names.
const AppName = "PlayAnything"

// ResolvePaths computes platform-appropriate directories. PLAYANYTHING_HOME
// overrides the config root (portable installs, tests).
func ResolvePaths() Paths {
	var cfg, cache, rt string
	if h := os.Getenv("PLAYANYTHING_HOME"); h != "" {
		cfg = h
		cache = filepath.Join(h, "cache")
		rt = filepath.Join(h, "run")
	} else {
		switch runtime.GOOS {
		case "windows":
			local := os.Getenv("LOCALAPPDATA")
			if local == "" {
				local = filepath.Join(home(), "AppData", "Local")
			}
			cfg = filepath.Join(local, AppName)
			cache = filepath.Join(cfg, "cache")
			rt = filepath.Join(cfg, "run")
		case "darwin":
			cfg = filepath.Join(home(), "Library", "Application Support", AppName)
			cache = filepath.Join(home(), "Library", "Caches", AppName)
			rt = filepath.Join(cfg, "run")
		default:
			xdg := os.Getenv("XDG_CONFIG_HOME")
			if xdg == "" {
				xdg = filepath.Join(home(), ".config")
			}
			cfg = filepath.Join(xdg, "playanything")
			xc := os.Getenv("XDG_CACHE_HOME")
			if xc == "" {
				xc = filepath.Join(home(), ".cache")
			}
			cache = filepath.Join(xc, "playanything")
			if xr := os.Getenv("XDG_RUNTIME_DIR"); xr != "" {
				rt = filepath.Join(xr, "playanything")
			} else {
				rt = filepath.Join(cfg, "run")
			}
		}
	}
	return Paths{Config: cfg, MPV: filepath.Join(cfg, "mpv"), Cache: cache, Runtime: rt}
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// File returns the config.json path.
func (p Paths) File() string { return filepath.Join(p.Config, "config.json") }

// PIDFile is the daemon pid file.
func (p Paths) PIDFile() string { return filepath.Join(p.Runtime, "daemon.pid") }

// IPCPath is the mpv JSON IPC endpoint for the background player.
func (p Paths) IPCPath() string {
	if runtime.GOOS == "windows" {
		user := os.Getenv("USERNAME")
		if user == "" {
			user = "default"
		}
		return `\\.\pipe\playanything-mpv-` + sanitize(user)
	}
	return filepath.Join(p.Runtime, "mpv.sock")
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// Load reads config.json; a missing file yields defaults.
func Load(p Paths) (*Config, error) {
	c := &Config{}
	b, err := os.ReadFile(p.File())
	if errors.Is(err, os.ErrNotExist) {
		c.applyDefaults()
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	return c, nil
}

func (c *Config) applyDefaults() {
	if c.Daemon == "" {
		c.Daemon = "auto"
	}
	if c.RawDecoder == "" {
		c.RawDecoder = "auto"
	}
	if c.REDlineArgs == "" {
		c.REDlineArgs = DefaultREDlineArgs
	}
}

// Save writes config.json (pretty-printed, creating the directory).
func Save(p Paths, c *Config) error {
	if err := os.MkdirAll(p.Config, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.File(), append(b, '\n'), 0o644)
}
