// Package red handles REDCODE RAW (.R3D) and Blackmagic RAW (.braw).
//
// Neither format can be decoded by FFmpeg: both need the manufacturer's
// proprietary SDK. PlayAnything therefore does the next best one-click thing:
//
//   - R3D:  if RED's free REDline command-line tool (ships with REDCINE-X PRO)
//     is installed, render a proxy once into the cache and play that with mpv;
//     otherwise hand the clip to REDCINE-X PRO itself when installed.
//   - BRAW: open with Blackmagic RAW Player when installed.
//
// When nothing suitable is installed, the caller shows a clear message with
// download links instead of a cryptic "unrecognized format".
package red

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tools lists what was found on this machine.
type Tools struct {
	REDline  string // path to REDline executable, "" when absent
	REDCINEX string // path to REDCINE-X PRO executable / .app
	BRAWApp  string // path to Blackmagic RAW Player executable / .app
}

// Detect looks for the RED and Blackmagic tools. override is config.redline_path.
func Detect(override string) Tools {
	var t Tools
	if override != "" {
		if _, err := os.Stat(override); err == nil {
			t.REDline = override
		}
	}
	if t.REDline == "" {
		t.REDline = firstExisting(redlineCandidates(), "REDline")
	}
	t.REDCINEX = firstExisting(redcinexCandidates(), "")
	t.BRAWApp = firstExisting(brawCandidates(), "")
	return t
}

func firstExisting(paths []string, lookPath string) string {
	if lookPath != "" {
		if p, err := exec.LookPath(lookPath); err == nil {
			return p
		}
	}
	for _, pat := range paths {
		matches, _ := filepath.Glob(pat)
		for _, m := range matches {
			if _, err := os.Stat(m); err == nil {
				return m
			}
		}
	}
	return ""
}

func redlineCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		pf := env("ProgramFiles", `C:\Program Files`)
		return []string{
			filepath.Join(pf, "REDCINE-X PRO 64-bit", "REDline.exe"),
			filepath.Join(pf, "REDCINE-X PRO*", "REDline.exe"),
			filepath.Join(pf, "RED", "REDCINE-X PRO*", "REDline.exe"),
			filepath.Join(pf, "REDline*", "REDline.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/REDCINE-X Professional/REDCINE-X PRO.app/Contents/MacOS/REDline",
			"/Applications/REDCINE-X PRO.app/Contents/MacOS/REDline",
			"/Applications/RED*/REDCINE-X PRO.app/Contents/MacOS/REDline",
			"/usr/local/bin/REDline",
		}
	default:
		return []string{"/opt/REDline*/REDline", "/opt/red/REDline", "/usr/local/bin/REDline"}
	}
}

func redcinexCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		pf := env("ProgramFiles", `C:\Program Files`)
		return []string{
			filepath.Join(pf, "REDCINE-X PRO 64-bit", "REDCINE-X PRO.exe"),
			filepath.Join(pf, "REDCINE-X PRO*", "REDCINE-X*.exe"),
			filepath.Join(pf, "RED", "REDCINE-X PRO*", "REDCINE-X*.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/REDCINE-X Professional/REDCINE-X PRO.app",
			"/Applications/REDCINE-X PRO.app",
			"/Applications/RED*/REDCINE-X PRO.app",
		}
	}
	return nil
}

func brawCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		pf := env("ProgramFiles", `C:\Program Files`)
		return []string{
			filepath.Join(pf, "Blackmagic Design", "Blackmagic RAW", "Blackmagic RAW Player", "Blackmagic RAW Player.exe"),
			filepath.Join(pf, "Blackmagic Design", "Blackmagic RAW*", "*Player*", "*.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/Blackmagic RAW/Blackmagic RAW Player.app",
			"/Applications/Blackmagic RAW*/Blackmagic RAW Player.app",
		}
	default:
		return []string{"/opt/BlackmagicRAW*/BlackmagicRAWPlayer*/BlackmagicRawPlayer", "/usr/bin/BlackmagicRawPlayer"}
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ProxyPath returns where the proxy for input lives in cacheDir (it may not exist yet).
// The name encodes path+size+mtime so re-rendered clips get fresh proxies.
func ProxyPath(cacheDir, input string) (string, error) {
	fi, err := os.Stat(input)
	if err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(input)
	h := sha1.New()
	fmt.Fprintf(h, "%s|%d|%d", abs, fi.Size(), fi.ModTime().UnixNano())
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return filepath.Join(cacheDir, fmt.Sprintf("%s-%s", base, hex.EncodeToString(h.Sum(nil))[:12])), nil
}

// FindProxy returns an existing rendered proxy (any extension) for input, or "".
func FindProxy(cacheDir, input string) string {
	base, err := ProxyPath(cacheDir, input)
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(base + ".*")
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && fi.Size() > 0 && !strings.HasSuffix(m, ".tmp") && !strings.HasSuffix(m, ".log") {
			return m
		}
	}
	return ""
}

// ExpandArgs fills the REDline template. Placeholders: {input} {outbase} {outdir}.
func ExpandArgs(template, input, outbase, outdir string) []string {
	fields := strings.Fields(template)
	for i, f := range fields {
		f = strings.ReplaceAll(f, "{input}", input)
		f = strings.ReplaceAll(f, "{outbase}", outbase)
		f = strings.ReplaceAll(f, "{outdir}", outdir)
		fields[i] = f
	}
	return fields
}

// Progress receives REDline's output lines as they arrive.
type Progress func(line string)

// FallbackTemplates are tried in order after the configured template fails.
// REDline's output formats differ between versions and platforms (QuickTime
// wrapping needs QuickTime, ProRes direct export arrived later, DNx is MXF),
// so the first one that produces a file wins and is cached.
var FallbackTemplates = []string{
	"--i {input} --o {outbase} --outDir {outdir} --format 201 --PRcodec 1 --res 4", // Apple ProRes direct (newer REDline)
	"--i {input} --o {outbase} --outDir {outdir} --format 201 --res 4",
	"--i {input} --o {outbase} --outDir {outdir} --format 12 --res 4",             // Avid DNxHD/HR (MXF)
	"--i {input} --o {outbase} --outDir {outdir} --format 11 --QTcodec 2 --res 4", // QuickTime wrapper (needs QT)
	"--i {input} --o {outbase} --outDir {outdir} --format 11 --res 4",
}

// RenderProxy runs REDline to create a proxy for input in cacheDir and returns
// the rendered file. The configured template (redline_args) is tried first,
// then FallbackTemplates. All attempts are appended to one log next to the
// proxy; the error carries REDline's last lines so the user sees why.
func RenderProxy(ctx context.Context, redline, template, cacheDir, input string, progress Progress) (string, error) {
	tried := map[string]bool{}
	var attempts []string
	if strings.TrimSpace(template) != "" {
		attempts = append(attempts, template)
	}
	for _, t := range FallbackTemplates {
		attempts = append(attempts, t)
	}
	var lastErr error
	var lastTail string
	n := 0
	for _, t := range attempts {
		if tried[t] {
			continue
		}
		tried[t] = true
		n++
		if progress != nil {
			progress(fmt.Sprintf("trying: REDline %s", strings.ReplaceAll(t, "{input}", filepath.Base(input))))
		}
		out, tail, err := renderOnce(ctx, redline, t, cacheDir, input, progress, n > 1)
		if err == nil {
			return out, nil
		}
		lastErr, lastTail = err, tail
		if ctx.Err() != nil {
			break
		}
	}
	logPath := ""
	if base, err := ProxyPath(cacheDir, input); err == nil {
		logPath = base + ".log"
	}
	msg := fmt.Sprintf("REDline could not render a proxy (%d command variants tried; last: %v).", n, lastErr)
	if lastTail != "" {
		msg += "\n\nREDline said:\n" + lastTail
	}
	msg += "\n\nFull log: " + logPath +
		"\nRun `REDline --help` to see the formats your version supports, then set your own command with:" +
		"\n  playanything config redline_args \"--i {input} --o {outbase} --outDir {outdir} --format <n> --res 4\""
	return "", errors.New(msg)
}

// renderOnce runs one REDline command line; tail returns its last output lines.
func renderOnce(ctx context.Context, redline, template, cacheDir, input string, progress Progress, appendLog bool) (out, tail string, err error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	outbasePath, err := ProxyPath(cacheDir, input)
	if err != nil {
		return "", "", err
	}
	outbase := filepath.Base(outbasePath)
	args := ExpandArgs(template, input, outbase, cacheDir)
	cmd := exec.CommandContext(ctx, redline, args...)
	logPath := outbasePath + ".log"
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if appendLog {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	logf, _ := os.OpenFile(logPath, flags, 0o644)
	if logf != nil {
		defer logf.Close()
		fmt.Fprintf(logf, "\n$ %s %s\n", redline, strings.Join(args, " "))
	}
	var lines []string
	keep := func(l string) {
		lines = append(lines, l)
		if len(lines) > 12 {
			lines = lines[1:]
		}
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return "", "", fmt.Errorf("start REDline: %w", err)
	}
	pw.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				if logf != nil {
					logf.Write(chunk)
				}
				for _, b := range chunk {
					if b == '\n' || b == '\r' {
						if acc.Len() > 0 {
							keep(acc.String())
							if progress != nil {
								progress(acc.String())
							}
						}
						acc.Reset()
					} else {
						acc.WriteByte(b)
					}
				}
			}
			if err != nil {
				if acc.Len() > 0 {
					keep(acc.String())
					if progress != nil {
						progress(acc.String())
					}
				}
				return
			}
		}
	}()
	waitErr := cmd.Wait()
	<-done
	tail = strings.Join(lines, "\n")
	out = FindProxy(cacheDir, input)
	if waitErr != nil && out == "" {
		return "", tail, fmt.Errorf("exit status: %v", waitErr)
	}
	if out == "" {
		return "", tail, errors.New("finished without producing a file")
	}
	return out, tail, nil
}

// OpenWith launches a GUI application (REDCINE-X PRO, Blackmagic RAW Player)
// on the given file, handling .app bundles on macOS.
func OpenWith(app, file string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" && strings.HasSuffix(app, ".app") {
		cmd = exec.Command("open", "-a", app, file)
	} else {
		cmd = exec.Command(app, file)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(time.Second)
		_ = cmd.Process.Release()
	}()
	return nil
}

// Links for the user when a required tool is missing.
const (
	REDCINEXURL = "https://www.red.com/download/redcine-x-pro-win" // Windows; macOS: .../redcine-x-pro-mac
	BRAWURL     = "https://www.blackmagicdesign.com/support/family/blackmagic-raw"
)
