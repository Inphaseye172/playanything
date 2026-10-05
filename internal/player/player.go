// Package player is PlayAnything's platform-independent core: it owns an
// embedded engine (internal/engine) and implements the behaviour that makes
// the app what it is - folder playlists, camera-RAW previews swapped in at
// load time, cloud-placeholder awareness, R3D proxy rendering, track handling
// and a small typed event stream for the native user interfaces.
package player

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inphaseye172/playanything/internal/assets"
	"github.com/inphaseye172/playanything/internal/cloud"
	"github.com/inphaseye172/playanything/internal/config"
	"github.com/inphaseye172/playanything/internal/engine"
	"github.com/inphaseye172/playanything/internal/media"
	"github.com/inphaseye172/playanything/internal/rawpreview"
	"github.com/inphaseye172/playanything/internal/red"
)

// Options configures a Player.
type Options struct {
	Paths    config.Paths
	Cfg      *config.Config
	WindowID uintptr // native window to render into (HWND on Windows); 0 = engine-owned window
	Headless bool    // vo=null/ao=null for tests and `info`
	OSC      bool    // keep the engine's on-screen controller (no native control bar)
	Log      func(format string, args ...any)
}

// Track describes one audio/video/subtitle track.
type Track struct {
	ID       int64
	Type     string // "audio", "video", "sub"
	Title    string
	Lang     string
	Codec    string
	Channels int64
	Selected bool
	Default  bool
	External bool
	Image    bool
}

// Label renders a track for menus and the OSD.
func (t Track) Label() string {
	var parts []string
	if t.Lang != "" {
		parts = append(parts, t.Lang)
	}
	if t.Title != "" {
		parts = append(parts, t.Title)
	}
	var tech []string
	if t.Codec != "" {
		tech = append(tech, t.Codec)
	}
	if t.Channels > 0 {
		tech = append(tech, fmt.Sprintf("%dch", t.Channels))
	}
	if len(tech) > 0 {
		parts = append(parts, "("+strings.Join(tech, " ")+")")
	}
	if t.External {
		parts = append(parts, "[external]")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s %d", t.Type, t.ID)
	}
	return strings.Join(parts, " ")
}

// State is the cached playback state the UI paints from.
type State struct {
	Path        string
	MediaTitle  string
	Paused      bool
	Idle        bool
	TimePos     float64
	Duration    float64
	Volume      float64
	Muted       bool
	Fullscreen  bool // the engine's fullscreen property (input.conf toggles it; the UI applies it)
	Width       int64
	Height      int64
	HWDec       string
	PlaylistPos int64
	PlaylistLen int64
	AudioID     int64
	SubID       int64
	Speed       float64
	IsImage     bool
}

// EventKind classifies player events.
type EventKind int

const (
	EvState      EventKind = iota // State changed (any field)
	EvFileLoaded                  // a file started playing
	EvEndFile                     // a file ended; Error set when it failed
	EvMessage                     // informational text for the UI (status bar / OSD)
	EvShutdown                    // the engine quit (user pressed q)
)

// Event is delivered to listeners from the player goroutine.
type Event struct {
	Kind  EventKind
	State State
	Text  string // EvMessage / EvEndFile error text
	Error bool
}

const (
	udHookLoad = 1
	udProps    = 2
)

// Player owns an engine and the PlayAnything behaviour on top of it.
type Player struct {
	E    *engine.Engine
	opts Options

	mu        sync.RWMutex
	state     State
	listeners []func(Event)
	done      chan struct{}
	closeOnce sync.Once
}

var observed = []struct {
	name   string
	format engine.Format
}{
	{"path", engine.FormatString}, {"media-title", engine.FormatString}, {"pause", engine.FormatFlag},
	{"idle-active", engine.FormatFlag}, {"time-pos", engine.FormatDouble}, {"duration", engine.FormatDouble},
	{"volume", engine.FormatDouble}, {"mute", engine.FormatFlag}, {"fullscreen", engine.FormatFlag},
	{"video-params/w", engine.FormatInt64}, {"video-params/h", engine.FormatInt64}, {"hwdec-current", engine.FormatString},
	{"playlist-pos", engine.FormatInt64}, {"playlist-count", engine.FormatInt64}, {"aid", engine.FormatString},
	{"sid", engine.FormatString}, {"speed", engine.FormatDouble}, {"current-tracks/video/image", engine.FormatFlag},
}

// New creates the engine, applies PlayAnything's profile and starts the
// event loop. The returned player is ready for Open.
func New(opts Options) (*Player, error) {
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	if opts.Cfg == nil {
		opts.Cfg = &config.Config{}
	}
	e, err := engine.New()
	if err != nil {
		return nil, err
	}
	p := &Player{E: e, opts: opts, done: make(chan struct{})}

	set := func(k, v string) error {
		if err := e.SetOption(k, v); err != nil {
			opts.Log("engine option %s=%s: %v", k, v, err)
			return fmt.Errorf("engine option %s=%s: %w", k, v, err)
		}
		return nil
	}
	// PlayAnything's tuned profile (mpv.conf, input.conf, Lua helpers). The
	// config file is applied during Init and overrides options set before it,
	// so headless mode (tests, `info`) skips the file and loads only the
	// scripts it needs.
	profileDir := ""
	if !opts.Cfg.UseSystemMPVConfig && opts.Paths.MPV != "" {
		if _, err := assets.Install(opts.Paths.MPV, false); err != nil {
			opts.Log("could not write engine profile: %v", err)
		} else {
			profileDir = opts.Paths.MPV
		}
	}
	if opts.Headless {
		_ = set("config", "no")
		_ = set("vo", "null")
		_ = set("ao", "null")
		_ = set("force-window", "no")
		_ = set("msg-level", "all=no")
		if profileDir != "" {
			var scripts []string
			for _, sc := range []string{"pa-autoload.lua", "pa-osd.lua"} {
				scripts = append(scripts, filepath.Join(profileDir, "scripts", sc))
			}
			_ = set("scripts", strings.Join(scripts, string(filepath.ListSeparator)))
		}
	} else if profileDir != "" {
		_ = set("config-dir", profileDir)
		_ = set("config", "yes")
	}
	_ = set("script-opts-append", "pa-native=yes") // tells the Lua helpers the RAW hook is handled in-process
	_ = set("terminal", "no")
	_ = set("input-default-bindings", "yes")
	_ = set("input-vo-keyboard", "yes")
	_ = set("osd-level", "1")
	if opts.WindowID != 0 {
		if err := set("wid", strconv.FormatUint(uint64(opts.WindowID), 10)); err != nil {
			return nil, err
		}
	}
	for _, a := range opts.Cfg.ExtraMPVArgs {
		if k, v, ok := strings.Cut(strings.TrimPrefix(a, "--"), "="); ok {
			_ = set(k, v)
		} else {
			_ = set(strings.TrimPrefix(a, "--"), "yes")
		}
	}
	if err := e.Init(); err != nil {
		e.Close()
		return nil, fmt.Errorf("playback engine: %w", err)
	}
	// Settings that must win over the config file are applied after init.
	if !opts.OSC {
		_ = e.SetProperty("osc", false)
	}
	if !opts.Headless {
		_ = e.SetProperty("force-window", "yes")
	}
	_ = e.SetProperty("idle", "yes")
	_ = e.SetProperty("keep-open", "yes")
	if opts.Cfg.Fullscreen {
		_ = e.SetProperty("fullscreen", true)
	}
	if err := e.HookAdd(udHookLoad, "on_load", 50); err != nil {
		opts.Log("RAW hook unavailable: %v", err)
	}
	for _, o := range observed {
		_ = e.Observe(udProps, o.name, o.format)
	}
	_ = e.RequestLog("warn")
	p.state.Volume = 100
	p.state.Speed = 1
	go p.loop()
	return p, nil
}

// OnEvent registers a listener; it is called on the player goroutine, so UIs
// must marshal to their own thread.
func (p *Player) OnEvent(fn func(Event)) {
	p.mu.Lock()
	p.listeners = append(p.listeners, fn)
	p.mu.Unlock()
}

func (p *Player) emit(ev Event) {
	p.mu.RLock()
	ls := append([]func(Event){}, p.listeners...)
	ev.State = p.state
	p.mu.RUnlock()
	for _, l := range ls {
		l(ev)
	}
}

// Snapshot returns the current cached state.
func (p *Player) Snapshot() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *Player) loop() {
	defer close(p.done)
	for ev := range p.E.Events() {
		switch ev.ID {
		case engine.EventHook:
			if ev.ReplyUserdata == udHookLoad {
				p.onLoadHook()
				_ = p.E.HookContinue(ev.HookID)
			}
		case engine.EventPropertyChange:
			if p.applyProperty(ev) {
				p.emit(Event{Kind: EvState})
			}
		case engine.EventFileLoaded:
			p.refreshTrackState()
			p.emit(Event{Kind: EvFileLoaded})
		case engine.EventEndFile:
			if ev.EndReason == engine.EndError {
				msg := p.E.ErrorText(ev.EndError)
				p.emit(Event{Kind: EvEndFile, Error: true, Text: msg})
			} else {
				p.emit(Event{Kind: EvEndFile})
			}
		case engine.EventLogMessage:
			p.opts.Log("[%s] %s: %s", ev.LogLevel, ev.LogPrefix, ev.LogText)
		case engine.EventShutdown:
			p.emit(Event{Kind: EvShutdown})
			return
		}
	}
}

func (p *Player) applyProperty(ev engine.Event) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := &p.state
	switch ev.Name {
	case "path":
		s.Path = ev.Str
	case "media-title":
		s.MediaTitle = ev.Str
	case "pause":
		s.Paused = ev.Flag
	case "idle-active":
		s.Idle = ev.Flag
	case "time-pos":
		if ev.Format == engine.FormatNone {
			s.TimePos = 0
		} else {
			s.TimePos = ev.Double
		}
	case "duration":
		if ev.Format == engine.FormatNone {
			s.Duration = 0
		} else {
			s.Duration = ev.Double
		}
	case "volume":
		s.Volume = ev.Double
	case "mute":
		s.Muted = ev.Flag
	case "fullscreen":
		s.Fullscreen = ev.Flag
	case "video-params/w":
		s.Width = ev.Int
	case "video-params/h":
		s.Height = ev.Int
	case "hwdec-current":
		s.HWDec = ev.Str
	case "playlist-pos":
		s.PlaylistPos = ev.Int
	case "playlist-count":
		s.PlaylistLen = ev.Int
	case "aid":
		s.AudioID, _ = strconv.ParseInt(ev.Str, 10, 64)
	case "sid":
		s.SubID, _ = strconv.ParseInt(ev.Str, 10, 64)
	case "speed":
		s.Speed = ev.Double
	case "current-tracks/video/image":
		s.IsImage = ev.Format != engine.FormatNone && ev.Flag
	default:
		return false
	}
	return true
}

func (p *Player) refreshTrackState() {
	if v, err := p.E.GetInt("video-params/w"); err == nil {
		p.mu.Lock()
		p.state.Width = v
		p.mu.Unlock()
	}
	if v, err := p.E.GetInt("video-params/h"); err == nil {
		p.mu.Lock()
		p.state.Height = v
		p.mu.Unlock()
	}
}

// onLoadHook runs inside the engine's load hook: camera RAW files are swapped
// for their embedded JPEG preview with the EXIF rotation applied.
func (p *Player) onLoadHook() {
	path, err := p.E.GetString("stream-open-filename")
	if err != nil || path == "" || media.IsURL(path) {
		return
	}
	if media.ByExtension(path) != media.RawImage {
		return
	}
	jpg, rot, err := rawpreview.Cached(filepath.Join(p.opts.Paths.Cache, "raw-previews"), path)
	if err != nil {
		p.opts.Log("raw preview %s: %v", path, err)
		_ = p.E.Command("show-text", "No embedded preview in "+filepath.Base(path), "6000")
		return
	}
	_ = p.E.SetProperty("stream-open-filename", jpg)
	if rot != 0 {
		_ = p.E.SetProperty("file-local-options/video-rotate", int64(rot))
	}
	_ = p.E.SetProperty("file-local-options/force-media-title", filepath.Base(path))
	p.opts.Log("RAW preview: %s -> %s (rotate %d)", path, jpg, rot)
}

// Open plays inputs (files, folders, URLs). With queue=true they are added
// after the current playlist instead of replacing it.
func (p *Player) Open(ctx context.Context, inputs []string, queue bool) error {
	files, err := expand(inputs)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	first := files[0]
	if !media.IsURL(first) {
		st := cloud.Status(first)
		if st.IsOffline {
			prov := st.Provider
			if prov == "" {
				prov = "cloud storage"
			}
			p.ShowText(fmt.Sprintf("Opening %s\n⬇ Downloading from %s — not stored locally yet", filepath.Base(first), prov), 60000)
			p.emit(Event{Kind: EvMessage, Text: "Downloading from " + prov + "…"})
		}
	}
	var playable []string
	for _, f := range files {
		switch media.ByExtension(f) {
		case media.R3D, media.BRAW:
			sub, err := p.cinemaRaw(ctx, f)
			if err != nil {
				return err
			}
			if sub != "" {
				playable = append(playable, sub)
			}
		default:
			playable = append(playable, f)
		}
	}
	if len(playable) == 0 {
		return nil // handed to an external viewer
	}
	for i, f := range playable {
		mode := "append"
		if i == 0 {
			mode = "replace"
			if queue {
				mode = "append-play"
			}
		}
		if err := p.E.Command("loadfile", f, mode); err != nil {
			return err
		}
	}
	return nil
}

// cinemaRaw handles R3D/BRAW: cached proxy, REDline render, vendor player, or
// an explanatory error. Returns the playable substitute ("" when handed off).
func (p *Player) cinemaRaw(ctx context.Context, f string) (string, error) {
	tools := red.Detect(p.opts.Cfg.REDlinePath)
	name := filepath.Base(f)
	switch media.ByExtension(f) {
	case media.R3D:
		cache := filepath.Join(p.opts.Paths.Cache, "r3d-proxies")
		if proxy := red.FindProxy(cache, f); proxy != "" {
			return proxy, nil
		}
		if tools.REDline != "" {
			p.ShowText(fmt.Sprintf("Rendering a proxy of %s with REDline…\nOnce per clip; the result is cached.", name), 600000)
			args := p.opts.Cfg.REDlineArgs
			if args == "" {
				args = config.DefaultREDlineArgs
			}
			proxy, err := red.RenderProxy(ctx, tools.REDline, args, cache, f, func(line string) {
				p.ShowText("REDline: "+name+"\n"+line, 5000)
			})
			if err != nil {
				p.ShowText("REDline failed: "+err.Error(), 10000)
				return "", err
			}
			return proxy, nil
		}
		if tools.REDCINEX != "" {
			return "", red.OpenWith(tools.REDCINEX, f)
		}
		return "", fmt.Errorf("%s is REDCODE RAW (R3D). FFmpeg cannot decode R3D; install the free REDCINE-X PRO (includes REDline) and PlayAnything will render a proxy automatically: %s", name, redURL())
	case media.BRAW:
		if tools.BRAWApp != "" {
			return "", red.OpenWith(tools.BRAWApp, f)
		}
		return "", fmt.Errorf("%s is Blackmagic RAW. Install the free Blackmagic RAW Player and PlayAnything will open .braw files with it: %s", name, red.BRAWURL)
	}
	return f, nil
}

func redURL() string {
	if runtime.GOOS == "darwin" {
		return "https://www.red.com/download/redcine-x-pro-mac"
	}
	return red.REDCINEXURL
}

// expand resolves folders to their media files and makes paths absolute.
func expand(inputs []string) ([]string, error) {
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

// --- commands used by the user interfaces -------------------------------

// Key forwards a key press in mpv's input.conf naming ("SPACE", "LEFT", "a", "Ctrl+a").
func (p *Player) Key(name string) error { return p.E.Command("keypress", name) }

// TogglePause flips pause.
func (p *Player) TogglePause() error { return p.E.Command("cycle", "pause") }

// SeekRelative seeks by seconds.
func (p *Player) SeekRelative(secs float64) error {
	return p.E.Command("seek", strconv.FormatFloat(secs, 'f', 2, 64), "relative")
}

// SeekPercent seeks to a position in percent; exact=false uses keyframes (fast scrubbing).
func (p *Player) SeekPercent(pct float64, exact bool) error {
	mode := "absolute-percent+keyframes"
	if exact {
		mode = "absolute-percent+exact"
	}
	return p.E.Command("seek", strconv.FormatFloat(pct, 'f', 3, 64), mode)
}

// SetVolume sets the volume (0-150).
func (p *Player) SetVolume(v float64) error {
	if v < 0 {
		v = 0
	}
	if v > 150 {
		v = 150
	}
	return p.E.SetProperty("volume", v)
}

// ToggleMute flips mute.
func (p *Player) ToggleMute() error { return p.E.Command("cycle", "mute") }

// Next / Prev move in the playlist (folder).
func (p *Player) Next() error { return p.E.Command("playlist-next", "weak") }
func (p *Player) Prev() error { return p.E.Command("playlist-prev", "weak") }

// FrameStep / FrameBack step one frame.
func (p *Player) FrameStep() error { return p.E.Command("frame-step") }
func (p *Player) FrameBack() error { return p.E.Command("frame-back-step") }

// Stop clears playback.
func (p *Player) Stop() error { return p.E.Command("stop") }

// SetFullscreen sets the engine property (the UI observes it and resizes).
func (p *Player) SetFullscreen(on bool) error { return p.E.SetProperty("fullscreen", on) }

// SetSpeed sets playback speed.
func (p *Player) SetSpeed(s float64) error { return p.E.SetProperty("speed", s) }

// Rotate rotates the picture by 90° clockwise.
func (p *Player) Rotate() error {
	return p.E.Command("cycle-values", "video-rotate", "90", "180", "270", "0")
}

// ToggleHWDec toggles GPU decoding (troubleshooting).
func (p *Player) ToggleHWDec() error { return p.E.Command("cycle-values", "hwdec", "auto-safe", "no") }

// ToggleStats shows the engine statistics overlay.
func (p *Player) ToggleStats() error {
	return p.E.Command("script-binding", "stats/display-stats-toggle")
}

// Screenshot saves a frame (per mpv.conf: PNG on the Desktop).
func (p *Player) Screenshot() error { return p.E.Command("screenshot") }

// ShowText shows an OSD message.
func (p *Player) ShowText(text string, ms int) { _ = p.E.Command("show-text", text, strconv.Itoa(ms)) }

// Quit asks the engine to quit; EvShutdown follows.
func (p *Player) Quit() { _ = p.E.Command("quit") }

// Tracks lists the current file's tracks.
func (p *Player) Tracks() []Track {
	n, err := p.E.GetInt("track-list/count")
	if err != nil {
		return nil
	}
	var out []Track
	for i := int64(0); i < n; i++ {
		pre := fmt.Sprintf("track-list/%d/", i)
		var t Track
		t.ID, _ = p.E.GetInt(pre + "id")
		t.Type, _ = p.E.GetString(pre + "type")
		t.Title, _ = p.E.GetString(pre + "title")
		t.Lang, _ = p.E.GetString(pre + "lang")
		t.Codec, _ = p.E.GetString(pre + "codec")
		t.Channels, _ = p.E.GetInt(pre + "demux-channel-count")
		t.Selected, _ = p.E.GetBool(pre + "selected")
		t.Default, _ = p.E.GetBool(pre + "default")
		t.External, _ = p.E.GetBool(pre + "external")
		t.Image, _ = p.E.GetBool(pre + "image")
		out = append(out, t)
	}
	return out
}

// SelectTrack selects a track by type ("audio"/"sub"/"video") and id; id 0 disables.
func (p *Player) SelectTrack(typ string, id int64) error {
	prop := map[string]string{"audio": "aid", "sub": "sid", "video": "vid"}[typ]
	if prop == "" {
		return fmt.Errorf("unknown track type %q", typ)
	}
	if id == 0 {
		return p.E.SetProperty(prop, "no")
	}
	return p.E.SetProperty(prop, strconv.FormatInt(id, 10))
}

// Close shuts the engine down.
func (p *Player) Close() {
	p.closeOnce.Do(func() {
		p.E.Close()
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
		}
	})
}

// FormatTime renders seconds as h:mm:ss or m:ss.
func FormatTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	s := int(sec + 0.5)
	h, m, ss := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, ss)
	}
	return fmt.Sprintf("%d:%02d", m, ss)
}
