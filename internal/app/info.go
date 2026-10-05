package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/inphaseye172/playanything/internal/assets"
	"github.com/inphaseye172/playanything/internal/cloud"
	"github.com/inphaseye172/playanything/internal/daemon"
	"github.com/inphaseye172/playanything/internal/media"
	"github.com/inphaseye172/playanything/internal/mpv"
	"github.com/inphaseye172/playanything/internal/rawpreview"
	"github.com/inphaseye172/playanything/internal/red"
	"github.com/inphaseye172/playanything/internal/version"
)

// Info explains what PlayAnything knows about a path and what Play would do.
func (a *App) Info(w io.Writer, path string) error {
	kind := media.Detect(path)
	fmt.Fprintf(w, "path:      %s\n", path)
	fmt.Fprintf(w, "kind:      %s", kind)
	if e := media.Ext(path); e != "" && media.ByExtension(path) == media.Unknown && kind != media.Unknown && kind != media.Stream && kind != media.Directory {
		fmt.Fprintf(w, "  (by content sniffing; extension .%s is not in the tables)", e)
	}
	fmt.Fprintln(w)
	if kind == media.Stream {
		fmt.Fprintln(w, "action:    stream with mpv")
		return nil
	}
	if kind == media.Directory {
		list, err := media.ExpandDirectory(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "contains:  %d media files\n", len(list))
		for i, f := range list {
			if i == 12 {
				fmt.Fprintf(w, "           … and %d more\n", len(list)-12)
				break
			}
			fmt.Fprintf(w, "           %s (%s)\n", filepath.Base(f), media.ByExtension(f))
		}
		return nil
	}
	st := cloud.Status(path)
	if !st.Exists {
		fmt.Fprintln(w, "exists:    no")
		return nil
	}
	fmt.Fprintf(w, "size:      %s\n", humanSize(st.Size))
	fmt.Fprintf(w, "locality:  %s", st.Describe())
	if st.Detail != "" {
		fmt.Fprintf(w, "  [%s]", st.Detail)
	}
	fmt.Fprintln(w)
	switch kind {
	case media.RawImage:
		start := time.Now()
		res, err := rawpreview.ExtractFile(path)
		if err != nil {
			fmt.Fprintf(w, "preview:   none found (%v)\n", err)
			if dev, ok := FindDeveloper(a.Cfg.RawDecoder); ok {
				fmt.Fprintf(w, "action:    develop with %s (%s)\n", dev.Name, dev.Path)
			} else {
				fmt.Fprintln(w, "action:    cannot display; install darktable-cli / rawtherapee-cli for full decode")
			}
			return nil
		}
		fmt.Fprintf(w, "preview:   embedded JPEG %dx%d, %s, orientation %d (rotate %d°), found via %s among %d candidates in %s\n",
			res.Width, res.Height, humanSize(int64(len(res.JPEG))), res.Orientation, res.Rotate(), res.Source, res.Candidates, time.Since(start).Round(time.Millisecond))
		if a.Cfg.RawFullDecode {
			fmt.Fprintln(w, "action:    full RAW development (raw_full_decode=true)")
		} else {
			fmt.Fprintln(w, "action:    show embedded preview in mpv (add --raw-full for a true demosaic)")
		}
	case media.R3D:
		tools := red.Detect(a.Cfg.REDlinePath)
		if p := red.FindProxy(filepath.Join(a.Paths.Cache, "r3d-proxies"), path); p != "" {
			fmt.Fprintf(w, "proxy:     cached at %s\n", p)
			fmt.Fprintln(w, "action:    play cached proxy")
		} else if tools.REDline != "" {
			fmt.Fprintf(w, "action:    render proxy with REDline (%s), then play\n", tools.REDline)
			fmt.Fprintf(w, "           args: %s\n", a.Cfg.REDlineArgs)
		} else if tools.REDCINEX != "" {
			fmt.Fprintf(w, "action:    open in REDCINE-X PRO (%s)\n", tools.REDCINEX)
		} else {
			fmt.Fprintln(w, "action:    cannot decode R3D here; install REDCINE-X PRO / REDline (see docs/RED-R3D.md)")
		}
	case media.BRAW:
		tools := red.Detect("")
		if tools.BRAWApp != "" {
			fmt.Fprintf(w, "action:    open in Blackmagic RAW Player (%s)\n", tools.BRAWApp)
		} else {
			fmt.Fprintln(w, "action:    cannot decode BRAW here; install Blackmagic RAW Player")
		}
	default:
		fmt.Fprintln(w, "action:    play with mpv (GPU decode when available)")
		if fp, err := exec.LookPath("ffprobe"); err == nil {
			out, err := exec.Command(fp, "-v", "error", "-show_entries", "stream=index,codec_type,codec_name,width,height,pix_fmt,channels,sample_rate:stream_tags=language,title", "-of", "compact=p=0:nk=0", path).Output()
			if err == nil && len(out) > 0 {
				fmt.Fprintln(w, "streams:")
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					fmt.Fprintf(w, "           %s\n", line)
				}
			}
		}
	}
	return nil
}

// Doctor checks the installation and prints a report. It returns an error when
// something essential (mpv) is missing.
func (a *App) Doctor(w io.Writer) error {
	ok := func(b bool) string {
		if b {
			return "✔"
		}
		return "✘"
	}
	fmt.Fprintf(w, "PlayAnything %s on %s/%s\n\n", version.String(), runtime.GOOS, runtime.GOARCH)

	fmt.Fprintln(w, "Engine")
	b, err := mpv.Find(a.Cfg.MPVPath, filepath.Join(a.Paths.Config, "mpv-bin"))
	if err != nil {
		fmt.Fprintf(w, "  %s mpv: not found. %s\n", ok(false), installHint())
		return err
	}
	a.MPV = b
	ver, _ := mpv.Version(b)
	fmt.Fprintf(w, "  %s mpv: %s (%s)\n      %s\n", ok(true), b.Path, b.Origin, ver)
	hw, _ := mpv.HWDecoders(b)
	fmt.Fprintf(w, "  %s GPU decoders compiled in: %s\n", ok(len(hw) > 0), strings.Join(hw, ", "))
	fmt.Fprintf(w, "  %s gpu-next renderer: %v\n", ok(mpv.HasVO(b, "gpu-next")), mpv.HasVO(b, "gpu-next"))
	fmt.Fprintf(w, "  · GPU: %s\n", gpuName())

	fmt.Fprintln(w, "\nConfiguration")
	fmt.Fprintf(w, "  · config.json:  %s\n", a.Paths.File())
	stamp, _ := os.ReadFile(filepath.Join(a.Paths.MPV, ".pa-assets-rev"))
	cur := strings.TrimSpace(string(stamp)) == assets.Revision
	fmt.Fprintf(w, "  %s mpv profile:  %s (assets rev %s, embedded %s)\n", ok(cur || a.Cfg.UseSystemMPVConfig), a.Paths.MPV, strings.TrimSpace(string(stamp)), assets.Revision)
	fmt.Fprintf(w, "  · cache:        %s\n", a.Paths.Cache)
	fmt.Fprintf(w, "  · daemon mode:  %s\n", a.Cfg.Daemon)
	if pid, alive := daemon.Running(a.Paths.PIDFile()); alive {
		fmt.Fprintf(w, "  %s background player: running (pid %d, ipc %s)\n", ok(true), pid, a.Paths.IPCPath())
	} else {
		fmt.Fprintf(w, "  · background player: not running\n")
	}

	fmt.Fprintln(w, "\nOptional tools")
	tools := red.Detect(a.Cfg.REDlinePath)
	fmt.Fprintf(w, "  %s REDline (R3D proxies):      %s\n", ok(tools.REDline != ""), orDefault(tools.REDline, "not found — install REDCINE-X PRO"))
	fmt.Fprintf(w, "  %s REDCINE-X PRO:              %s\n", ok(tools.REDCINEX != ""), orDefault(tools.REDCINEX, "not found"))
	fmt.Fprintf(w, "  %s Blackmagic RAW Player:      %s\n", ok(tools.BRAWApp != ""), orDefault(tools.BRAWApp, "not found"))
	if dev, found := FindDeveloper(a.Cfg.RawDecoder); found {
		fmt.Fprintf(w, "  %s RAW developer (--raw-full): %s (%s)\n", ok(true), dev.Name, dev.Path)
	} else {
		fmt.Fprintf(w, "  · RAW developer (--raw-full): none (embedded previews still work)\n")
	}
	for _, t := range []string{"ffprobe", "exiftool"} {
		p, err := exec.LookPath(t)
		fmt.Fprintf(w, "  %s %-27s %s\n", ok(err == nil), t+":", orDefault(p, "not found (optional, used by `info`)"))
	}
	fmt.Fprintf(w, "\nFormats: %d extensions registered (video %d, audio %d, image %d, camera RAW %d, cinema RAW 2)\n",
		len(media.AllExtensions()), len(media.VideoExts), len(media.AudioExts), len(media.ImageExts), len(media.RawImageExts))
	return nil
}

// Setup writes the managed mpv config and a default config.json.
func (a *App) Setup(w io.Writer, force bool) error {
	for _, d := range []string{a.Paths.Config, a.Paths.MPV, a.Paths.Cache, a.Paths.Runtime} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	wrote, err := assets.Install(a.Paths.MPV, force)
	if err != nil {
		return err
	}
	if _, err := os.Stat(a.Paths.File()); os.IsNotExist(err) {
		if err := saveDefault(a); err != nil {
			return err
		}
		fmt.Fprintf(w, "wrote %s\n", a.Paths.File())
	}
	if wrote {
		fmt.Fprintf(w, "wrote mpv profile to %s\n", a.Paths.MPV)
	} else {
		fmt.Fprintf(w, "mpv profile up to date in %s\n", a.Paths.MPV)
	}
	fmt.Fprintf(w, "cache: %s\n", a.Paths.Cache)
	return nil
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func installHint() string {
	switch runtime.GOOS {
	case "windows":
		return "Run: winget install -e --id shinchiro.mpv   (or re-run install.ps1)"
	case "darwin":
		return "Run: brew install mpv"
	default:
		return "Run: sudo apt install mpv   (or dnf/pacman/zypper/flatpak)"
	}
}

// gpuName makes a best effort to name the GPU for the doctor report.
func gpuName() string {
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		done := make(chan []byte, 1)
		go func() {
			out, _ := cmd.Output()
			done <- out
		}()
		select {
		case out := <-done:
			return strings.TrimSpace(string(out))
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			return ""
		}
	}
	var s string
	switch runtime.GOOS {
	case "windows":
		s = run("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name) -join ', '")
	case "darwin":
		out := run("system_profiler", "SPDisplaysDataType")
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "Chipset Model:") {
				s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Chipset Model:"))
				break
			}
		}
	default:
		if out := run("nvidia-smi", "--query-gpu=name", "--format=csv,noheader"); out != "" {
			s = out + " (NVIDIA driver present)"
		} else {
			out := run("sh", "-c", "lspci 2>/dev/null | grep -iE 'vga|3d|display' | head -2 | sed 's/^[^:]*: //'")
			s = strings.ReplaceAll(out, "\n", "; ")
		}
	}
	if s == "" {
		return "unknown (could not query)"
	}
	return s
}
