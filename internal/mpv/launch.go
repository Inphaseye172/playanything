package mpv

import (
	"strconv"
	"strings"
)

// LaunchOptions describes one mpv invocation.
type LaunchOptions struct {
	ConfigDir       string // PlayAnything's managed mpv config dir ("" = mpv's own default)
	IPCPath         string // --input-ipc-server endpoint ("" = none)
	Idle            bool   // start with no file and keep running (daemon)
	ForceWindow     string // "", "yes", "no", "immediate"
	Fullscreen      bool
	ScriptOpts      map[string]string // passed as --script-opts-append=key=value (read by the Lua scripts)
	Rotate          int               // --video-rotate for RAW previews
	Title           string
	Extra           []string // user-supplied, appended before files
	Files           []string
	Append          bool // not used for direct launches; informational
	NoTerminal      bool // --terminal=no when launched from a GUI with no console
	UseSystemConfig bool
}

// Args renders the command line. Files always come last after "--" so that
// names starting with '-' are safe.
func Args(o LaunchOptions) []string {
	var a []string
	if o.ConfigDir != "" && !o.UseSystemConfig {
		a = append(a, "--config-dir="+o.ConfigDir)
	}
	if o.IPCPath != "" {
		a = append(a, "--input-ipc-server="+o.IPCPath)
	}
	if o.Idle {
		a = append(a, "--idle=yes")
	}
	if o.ForceWindow != "" {
		a = append(a, "--force-window="+o.ForceWindow)
	}
	if o.Fullscreen {
		a = append(a, "--fullscreen")
	}
	if o.NoTerminal {
		a = append(a, "--terminal=no")
	}
	if o.Rotate != 0 {
		a = append(a, "--video-rotate="+strconv.Itoa(o.Rotate))
	}
	if o.Title != "" {
		a = append(a, "--title="+o.Title)
	}
	for _, k := range sortedKeys(o.ScriptOpts) {
		a = append(a, "--script-opts-append="+k+"="+o.ScriptOpts[k])
	}
	a = append(a, o.Extra...)
	if len(o.Files) > 0 {
		a = append(a, "--")
		a = append(a, o.Files...)
	}
	return a
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// insertion sort: tiny maps
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && strings.Compare(keys[j-1], keys[j]) > 0; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
