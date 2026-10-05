package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/inphaseye172/playanything/internal/rawpreview"
)

// RawDeveloper is an external RAW converter PlayAnything can drive when the
// user asks for a true demosaic (--raw-full) instead of the embedded preview.
type RawDeveloper struct {
	Name string
	Path string
	// args builds the command line: input RAW -> output file.
	args func(in, out string) []string
	ext  string // output extension
}

// developers in preference order. All are free/open source:
//   - darktable-cli   (https://www.darktable.org)
//   - rawtherapee-cli (https://rawtherapee.com)
//   - dcraw_emu       (LibRaw samples, https://www.libraw.org)
//   - dcraw           (Dave Coffin's classic)
var developers = []RawDeveloper{
	{Name: "darktable-cli", ext: ".jpg", args: func(in, out string) []string {
		return []string{in, out, "--core", "--conf", "plugins/imageio/format/jpeg/quality=92"}
	}},
	{Name: "rawtherapee-cli", ext: ".jpg", args: func(in, out string) []string {
		return []string{"-o", out, "-j92", "-Y", "-c", in}
	}},
	{Name: "dcraw_emu", ext: ".tiff", args: func(in, out string) []string {
		return []string{"-w", "-T", "-Z", out, in}
	}},
	{Name: "dcraw", ext: ".tiff", args: func(in, out string) []string {
		// dcraw cannot name its output; the caller redirects stdout (-c).
		return []string{"-w", "-T", "-c", in}
	}},
}

// FindDeveloper returns the requested (or first available) RAW developer.
func FindDeveloper(preferred string) (RawDeveloper, bool) {
	for _, d := range developers {
		if preferred != "" && preferred != "auto" && d.Name != preferred {
			continue
		}
		if p, err := exec.LookPath(d.Name); err == nil {
			d.Path = p
			return d, true
		}
		if p := wellKnownDeveloper(d.Name); p != "" {
			d.Path = p
			return d, true
		}
	}
	return RawDeveloper{}, false
}

func wellKnownDeveloper(name string) string {
	var cands []string
	switch name {
	case "darktable-cli":
		cands = []string{
			`C:\Program Files\darktable\bin\darktable-cli.exe`,
			"/Applications/darktable.app/Contents/MacOS/darktable-cli",
			"/opt/homebrew/bin/darktable-cli", "/usr/local/bin/darktable-cli",
		}
	case "rawtherapee-cli":
		cands = []string{
			`C:\Program Files\RawTherapee\*\rawtherapee-cli.exe`,
			"/Applications/RawTherapee.app/Contents/MacOS/rawtherapee-cli",
			"/opt/homebrew/bin/rawtherapee-cli", "/usr/local/bin/rawtherapee-cli",
		}
	}
	for _, c := range cands {
		if m, _ := filepath.Glob(c); len(m) > 0 {
			return m[0]
		}
	}
	return ""
}

// Develop converts a RAW file to a viewable image in cacheDir using dev.
// Results are cached by path+size+mtime like previews.
func Develop(ctx context.Context, dev RawDeveloper, cacheDir, in string) (string, error) {
	key, err := rawpreview.CacheKey(in)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(cacheDir, key+".dev"+dev.ext)
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	tmp := out + ".tmp" + dev.ext
	cmd := exec.CommandContext(ctx, dev.Path, dev.args(in, tmp)...)
	var stdoutFile *os.File
	if dev.Name == "dcraw" {
		stdoutFile, err = os.Create(tmp)
		if err != nil {
			return "", err
		}
		cmd.Stdout = stdoutFile
	}
	var outBytes []byte
	var runErr error
	if stdoutFile != nil {
		// dcraw writes the image to stdout (-c); only stderr carries messages.
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		runErr = cmd.Run()
		stdoutFile.Close()
		outBytes = errBuf.Bytes()
	} else {
		outBytes, runErr = cmd.CombinedOutput()
	}
	if runErr != nil {
		os.Remove(tmp)
		msg := strings.TrimSpace(string(outBytes))
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return "", fmt.Errorf("%s failed: %v\n%s", dev.Name, runErr, msg)
	}
	if fi, err := os.Stat(tmp); err != nil || fi.Size() == 0 {
		os.Remove(tmp)
		return "", errors.New(dev.Name + " produced no output")
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}
