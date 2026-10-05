// Package app wires everything together: it decides how a given input should
// be opened (mpv directly, the background player, a RAW preview, an R3D proxy,
// an external player) and does it.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/inphaseye172/playanything/internal/assets"
	"github.com/inphaseye172/playanything/internal/cloud"
	"github.com/inphaseye172/playanything/internal/config"
	"github.com/inphaseye172/playanything/internal/media"
	"github.com/inphaseye172/playanything/internal/mpv"
	"github.com/inphaseye172/playanything/internal/rawpreview"
	"github.com/inphaseye172/playanything/internal/red"
	"github.com/inphaseye172/playanything/internal/ui"
)

// App holds resolved configuration for one invocation.
type App struct {
	Paths config.Paths
	Cfg   *config.Config
	Self  string     // path of this executable (passed to the Lua RAW hook)
	MPV   mpv.Binary // resolved lazily by EnsureMPV
	GUI   bool       // no terminal attached: report errors via dialogs
	Wait  bool       // block until mpv exits (terminal use)
	Log   func(format string, args ...any)
}

// New loads config and resolves paths. It does not require mpv yet.
func New() (*App, error) {
	paths := config.ResolvePaths()
	cfg, err := config.Load(paths)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", paths.File(), err)
	}
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return &App{Paths: paths, Cfg: cfg, Self: self, Log: func(string, ...any) {}}, nil
}

// EnsureMPV locates mpv and installs the managed config files.
func (a *App) EnsureMPV() error {
	b, err := mpv.Find(a.Cfg.MPVPath, filepath.Join(a.Paths.Config, "mpv-bin"))
	if err != nil {
		return err
	}
	a.MPV = b
	if !a.Cfg.UseSystemMPVConfig {
		if _, err := assets.Install(a.Paths.MPV, false); err != nil {
			return fmt.Errorf("writing mpv config to %s: %w", a.Paths.MPV, err)
		}
	}
	return nil
}

// PlayOptions are the per-invocation switches.
type PlayOptions struct {
	Append       bool // add to the background player's playlist instead of replacing
	HydrateFirst bool // download cloud placeholders completely before playing
	RawFull      bool // develop camera RAW with an external decoder
	NoDaemon     bool // ignore the background player
	Fullscreen   bool
	MPVArgs      []string // raw mpv options (after --)
}

// Play opens inputs (files, folders, URLs). With no inputs it opens an empty
// player window that accepts drag & drop.
func (a *App) Play(ctx context.Context, inputs []string, opt PlayOptions) error {
	if err := a.EnsureMPV(); err != nil {
		return err
	}
	files, err := a.expand(inputs)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return a.openEmpty(opt)
	}

	scriptOpts := map[string]string{"pa-exe": a.Self}
	perFile := map[string][]string{}

	// Cloud / network awareness for the first file (that is the one the user clicked).
	first := files[0]
	if !media.IsURL(first) {
		st := cloud.Status(first)
		if st.IsOffline {
			prov := st.Provider
			if prov == "" {
				prov = "cloud storage"
			}
			scriptOpts["pa-cloud"] = prov
			a.Log("%s: %s", filepath.Base(first), st.Describe())
		}
		if (opt.HydrateFirst || a.Cfg.HydrateFirst) && st.Remote() {
			s, err := a.hydrateWithFeedback(ctx, first, st, opt, scriptOpts)
			if err != nil {
				return err
			}
			if s != nil {
				// A window is already up showing the download; play in it.
				defer s.close()
				if err := s.load(files, opt.Append, perFile); err != nil {
					return err
				}
				return nil
			}
		}
	}

	// Camera RAW: either the Lua hook shows the embedded preview (default), or
	// we develop the files first (--raw-full).
	if opt.RawFull || a.Cfg.RawFullDecode {
		files, err = a.developAll(ctx, files, perFile)
		if err != nil {
			return err
		}
	}

	// Cinema RAW that FFmpeg cannot decode.
	var normal []string
	var special []string
	for _, f := range files {
		switch media.ByExtension(f) {
		case media.R3D, media.BRAW:
			special = append(special, f)
		default:
			normal = append(normal, f)
		}
	}
	if len(special) > 0 {
		resolved, err := a.resolveCinemaRaw(ctx, special, opt, scriptOpts)
		if err != nil {
			return err
		}
		if len(resolved) == 0 && len(normal) == 0 {
			return nil // handed off to an external player
		}
		normal = append(resolved, normal...)
		files = normal
	}

	// Prefer the background player when it is around.
	if s := a.connectDaemon(opt.NoDaemon); s != nil {
		defer s.close()
		if cloudNote := scriptOpts["pa-cloud"]; cloudNote != "" {
			s.osd(fmt.Sprintf("Opening %s\n⬇ Downloading from %s — not stored locally yet", filepath.Base(first), cloudNote), 60000)
		}
		if err := s.load(files, opt.Append, perFile); err != nil {
			return fmt.Errorf("background player: %w", err)
		}
		return nil
	}
	return a.launchDirect(files, perFile, scriptOpts, opt)
}

// expand turns the inputs into absolute file paths / URLs, expanding folders.
func (a *App) expand(inputs []string) ([]string, error) {
	var out []string
	for _, in := range inputs {
		in = strings.TrimSpace(in)
		if in == "" {
			continue
		}
		if media.IsURL(in) {
			out = append(out, in)
			continue
		}
		abs, err := filepath.Abs(in)
		if err != nil {
			abs = in
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("cannot open %s: %w", in, err)
		}
		if fi.IsDir() {
			list, err := media.ExpandDirectory(abs)
			if err != nil {
				return nil, err
			}
			if len(list) == 0 {
				return nil, fmt.Errorf("no media files in %s", abs)
			}
			out = append(out, list...)
			continue
		}
		out = append(out, abs)
	}
	return out, nil
}

func (a *App) openEmpty(opt PlayOptions) error {
	if s := a.connectDaemon(opt.NoDaemon); s != nil {
		defer s.close()
		_ = s.client.SetProperty("force-window", "yes")
		s.client.Raise()
		return nil
	}
	args := mpv.Args(mpv.LaunchOptions{
		ConfigDir:       a.Paths.MPV,
		UseSystemConfig: a.Cfg.UseSystemMPVConfig,
		Idle:            true,
		ForceWindow:     "yes",
		ScriptOpts:      map[string]string{"pa-exe": a.Self},
		Extra:           append(append([]string{}, a.Cfg.ExtraMPVArgs...), opt.MPVArgs...),
	})
	return a.run(args)
}

// launchDirect starts one mpv process for these files.
func (a *App) launchDirect(files []string, perFile map[string][]string, scriptOpts map[string]string, opt PlayOptions) error {
	extra := append([]string{}, a.Cfg.ExtraMPVArgs...)
	extra = append(extra, opt.MPVArgs...)
	// Per-file options for a direct launch are only needed for the first file
	// (RAW previews handled by the Lua hook set their own rotation).
	rotate := 0
	for _, o := range perFile[files[0]] {
		if strings.HasPrefix(o, "video-rotate=") {
			fmt.Sscanf(strings.TrimPrefix(o, "video-rotate="), "%d", &rotate)
		}
	}
	args := mpv.Args(mpv.LaunchOptions{
		ConfigDir:       a.Paths.MPV,
		UseSystemConfig: a.Cfg.UseSystemMPVConfig,
		Fullscreen:      opt.Fullscreen || a.Cfg.Fullscreen,
		ScriptOpts:      scriptOpts,
		Rotate:          rotate,
		Extra:           extra,
		Files:           files,
	})
	return a.run(args)
}

// run executes mpv. In terminal mode it waits and relays the exit status. In
// GUI mode it watches the first seconds so a failure becomes a dialog instead
// of a window that silently never appears.
func (a *App) run(args []string) error {
	cmd := a.MPV.Command(args...)
	a.Log("exec: %s %s", a.MPV.Path, strings.Join(args, " "))
	var errBuf bytes.Buffer
	if a.Wait && !a.GUI {
		cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mpv exited: %w", err)
		}
		return nil
	}
	cmd.Stderr = &errBuf
	cmd.Stdout = &errBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mpv (%s): %w", a.MPV.Path, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if len(msg) > 600 {
				msg = "…" + msg[len(msg)-600:]
			}
			return fmt.Errorf("mpv could not play this:\n%s", msg)
		}
		return nil
	case <-time.After(2500 * time.Millisecond):
		// Still running: playback started. Let it live on without us.
		go func() { <-done }()
		_ = cmd.Process.Release()
		return nil
	}
}

// hydrateWithFeedback downloads a placeholder fully, showing progress in the
// terminal and, when possible, on an mpv window that opens immediately. The
// returned session (nil in plain terminal mode) is where the caller should
// then load the files, so the user sees one window, not two.
func (a *App) hydrateWithFeedback(ctx context.Context, path string, st cloud.Info, opt PlayOptions, scriptOpts map[string]string) (*session, error) {
	name := filepath.Base(path)
	s := a.connectDaemon(opt.NoDaemon)
	if s == nil && a.GUI {
		var err error
		s, err = a.startIdle(ctx, scriptOpts, opt.MPVArgs)
		if err != nil {
			s = nil
		}
	}
	last := time.Now()
	progress := func(done, total int64) {
		if time.Since(last) < 300*time.Millisecond && done != total {
			return
		}
		last = time.Now()
		pct := 0.0
		if total > 0 {
			pct = float64(done) * 100 / float64(total)
		}
		line := fmt.Sprintf("Downloading %s from %s: %.0f%% (%d / %d MB)", name, orDefault(st.Provider, "network"), pct, done>>20, total>>20)
		if !a.GUI {
			fmt.Fprintf(os.Stderr, "\r%-100s", line)
		}
		if s != nil {
			s.osd(line, 2000)
		}
	}
	err := cloud.Hydrate(path, progress)
	if !a.GUI {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		if s != nil {
			s.close()
		}
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	if s != nil {
		s.osd("Download complete", 1000)
	}
	return s, nil
}

// developAll runs the external RAW developer over RAW inputs.
func (a *App) developAll(ctx context.Context, files []string, perFile map[string][]string) ([]string, error) {
	dev, ok := FindDeveloper(a.Cfg.RawDecoder)
	if !ok {
		return nil, errors.New("--raw-full needs darktable-cli, rawtherapee-cli, dcraw_emu or dcraw on PATH (the embedded preview is used otherwise)")
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		if media.ByExtension(f) != media.RawImage {
			out = append(out, f)
			continue
		}
		if !a.GUI {
			fmt.Fprintf(os.Stderr, "developing %s with %s…\n", filepath.Base(f), dev.Name)
		}
		img, err := Develop(ctx, dev, filepath.Join(a.Paths.Cache, "raw-developed"), f)
		if err != nil {
			return nil, err
		}
		out = append(out, img)
	}
	return out, nil
}

// RawPreview extracts (or reuses) the embedded preview for a RAW photo and
// returns its path and rotation. Used by the Lua hook and `raw-preview`.
func (a *App) RawPreview(path string) (string, int, error) {
	return rawpreview.Cached(filepath.Join(a.Paths.Cache, "raw-previews"), path)
}

// resolveCinemaRaw maps R3D/BRAW inputs to something mpv can play, or opens
// them in the vendor's player. Returns the playable substitutes.
func (a *App) resolveCinemaRaw(ctx context.Context, files []string, opt PlayOptions, scriptOpts map[string]string) ([]string, error) {
	tools := red.Detect(a.Cfg.REDlinePath)
	cache := filepath.Join(a.Paths.Cache, "r3d-proxies")
	var playable []string
	var s *session // window used for rendering feedback; proxies are loaded into it at the end
	defer func() {
		if s != nil {
			s.close() // drops the IPC connection only; the window stays
		}
	}()
	for _, f := range files {
		kind := media.ByExtension(f)
		name := filepath.Base(f)
		switch kind {
		case media.R3D:
			if p := red.FindProxy(cache, f); p != "" {
				playable = append(playable, p)
				continue
			}
			if tools.REDline != "" {
				if s == nil {
					s = a.connectDaemon(opt.NoDaemon)
					if s == nil {
						var err error
						if s, err = a.startIdle(ctx, scriptOpts, opt.MPVArgs); err != nil {
							return nil, err
						}
					}
				}
				s.osd(fmt.Sprintf("Rendering a proxy of %s with REDline…\nThis happens once per clip; the result is cached.", name), 600000)
				if !a.GUI {
					fmt.Fprintf(os.Stderr, "rendering R3D proxy for %s with REDline (once per clip)…\n", name)
				}
				p, err := red.RenderProxy(ctx, tools.REDline, a.Cfg.REDlineArgs, cache, f, func(line string) {
					s.osd(fmt.Sprintf("REDline: %s\n%s", name, line), 5000)
					if !a.GUI {
						fmt.Fprintf(os.Stderr, "\r%-100s", line)
					}
				})
				if !a.GUI {
					fmt.Fprintln(os.Stderr)
				}
				if err != nil {
					s.osd("REDline failed: "+err.Error(), 10000)
					return nil, err
				}
				playable = append(playable, p)
				continue
			}
			if tools.REDCINEX != "" {
				a.Log("opening %s in REDCINE-X PRO", name)
				if err := red.OpenWith(tools.REDCINEX, f); err != nil {
					return nil, err
				}
				continue
			}
			ui.Error(a.GUI, fmt.Sprintf("%s is REDCODE RAW (R3D).\n\nFFmpeg cannot decode R3D; it needs RED's SDK. Install the free REDCINE-X PRO (which includes the REDline tool) and PlayAnything will render a proxy automatically next time:\n%s\n\nSee docs/RED-R3D.md for details.", name, redDownloadURL()))
			return nil, nil
		case media.BRAW:
			if tools.BRAWApp != "" {
				a.Log("opening %s in Blackmagic RAW Player", name)
				if err := red.OpenWith(tools.BRAWApp, f); err != nil {
					return nil, err
				}
				continue
			}
			ui.Error(a.GUI, fmt.Sprintf("%s is Blackmagic RAW.\n\nFFmpeg cannot decode BRAW; it needs Blackmagic's SDK. Install the free Blackmagic RAW Player and PlayAnything will open .braw files with it:\n%s", name, red.BRAWURL))
			return nil, nil
		}
	}
	if s != nil && len(playable) > 0 {
		// We already have a window up (rendering feedback). Load the proxies into it.
		if err := s.load(playable, opt.Append, nil); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return playable, nil
}

func redDownloadURL() string {
	if runtime.GOOS == "darwin" {
		return "https://www.red.com/download/redcine-x-pro-mac"
	}
	return red.REDCINEXURL
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
