// Command playanything is a one-click launcher that plays any media file:
// video, audio, images and camera RAW photos, local or on cloud/network
// storage, with GPU acceleration. It is a thin, fast front end for mpv.
//
// Usage:
//
//	playanything [flags] <file|folder|url>...   play (default command)
//	playanything daemon                         run the background player (service mode)
//	playanything stop                           stop the background player
//	playanything info <file>                    what is this file and what would happen
//	playanything doctor                         check mpv, GPU decoders, tools, config
//	playanything setup [--force]                (re)write the managed mpv profile
//	playanything raw-preview <file>             extract a camera RAW's embedded JPEG
//	playanything version
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/inphaseye172/playanything/internal/app"
	"github.com/inphaseye172/playanything/internal/config"
	"github.com/inphaseye172/playanything/internal/daemon"
	"github.com/inphaseye172/playanything/internal/media"
	"github.com/inphaseye172/playanything/internal/mpv"
	"github.com/inphaseye172/playanything/internal/ui"
	"github.com/inphaseye172/playanything/internal/version"
)

func main() {
	gui := platformInit() // attaches to a parent console on Windows; true when none exists
	code := run(os.Args[1:], gui)
	os.Exit(code)
}

func usage(w *os.File) {
	fmt.Fprint(w, `PlayAnything — one click, any media.

Usage:
  playanything [flags] <file|folder|url>...   play (default)
  playanything daemon                         background player / service mode
  playanything stop                           stop the background player
  playanything info <file>                    explain what this file is and how it will be opened
  playanything doctor                         check mpv, GPU decoders, optional tools
  playanything setup [--force]                write the managed mpv profile
  playanything raw-preview <file>             print the path of a RAW photo's embedded preview
  playanything config [<key> [<value>]]       show or change settings (mpv_path, daemon, hydrate_first, ...)
  playanything extensions | mimetypes         list what gets registered as openable
  playanything version

Play flags:
  --append          add to the background player's playlist instead of replacing
  --hydrate-first   download a cloud placeholder completely before playing
  --raw-full        develop camera RAW with darktable-cli/rawtherapee-cli instead of the preview
  --no-daemon       always start a fresh player window
  --fullscreen, -f  start fullscreen
  --                everything after this goes to mpv unchanged (e.g. -- --volume=50)

Config: see 'playanything doctor' for the config.json location.
`)
}

func run(args []string, gui bool) int {
	a, err := app.New()
	if err != nil {
		ui.Error(gui, err.Error())
		return 1
	}
	a.GUI = gui
	a.Wait = !gui && isTerminal()
	if os.Getenv("PLAYANYTHING_DEBUG") != "" {
		a.Log = func(f string, v ...any) { fmt.Fprintf(os.Stderr, "[playanything] "+f+"\n", v...) }
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "play", "daemon", "stop", "info", "doctor", "setup", "raw-preview", "version", "help", "extensions", "mimetypes", "config":
			cmd, args = args[0], args[1:]
		}
	}
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		cmd = "help"
	}

	switch cmd {
	case "help":
		usage(os.Stdout)
		return 0
	case "version":
		fmt.Println("playanything " + version.String())
		return 0
	case "daemon":
		return cmdDaemon(ctx, a, args)
	case "stop":
		if err := daemon.Stop(a.Paths.PIDFile(), a.Paths.IPCPath()); err != nil {
			ui.Error(gui, err.Error())
			return 1
		}
		fmt.Println("background player stopped")
		return 0
	case "info":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: playanything info <file>")
			return 2
		}
		if err := a.Info(os.Stdout, args[0]); err != nil {
			fmt.Fprintln(os.Stderr, "playanything:", err)
			return 1
		}
		return 0
	case "doctor":
		if err := a.Doctor(os.Stdout); err != nil {
			return 1
		}
		return 0
	case "setup":
		fs := flag.NewFlagSet("setup", flag.ContinueOnError)
		force := fs.Bool("force", false, "overwrite managed files even when up to date")
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if err := a.Setup(os.Stdout, *force); err != nil {
			fmt.Fprintln(os.Stderr, "playanything:", err)
			return 1
		}
		return 0
	case "extensions":
		// Used by the installers to register file associations (single source of truth).
		fmt.Println(strings.Join(media.AllExtensions(), " "))
		return 0
	case "mimetypes":
		fmt.Println(strings.Join(media.MIMETypes(), " "))
		return 0
	case "config":
		return cmdConfig(a, args)
	case "raw-preview":
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: playanything raw-preview <file>")
			return 2
		}
		p, rot, err := a.RawPreview(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "playanything:", err)
			return 1
		}
		// Machine-readable: line 1 path, line 2 clockwise rotation (read by scripts/pa-rawhook.lua).
		fmt.Printf("%s\n%d\n", p, rot)
		return 0
	}
	return cmdPlay(ctx, a, args, gui)
}

func cmdPlay(ctx context.Context, a *app.App, args []string, gui bool) int {
	var opt app.PlayOptions
	var inputs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--":
			opt.MPVArgs = append(opt.MPVArgs, args[i+1:]...)
			i = len(args)
		case "--append", "-a":
			opt.Append = true
		case "--hydrate-first":
			opt.HydrateFirst = true
		case "--raw-full":
			opt.RawFull = true
		case "--no-daemon":
			opt.NoDaemon = true
		case "--fullscreen", "-f", "--fs":
			opt.Fullscreen = true
		case "-h", "--help":
			usage(os.Stdout)
			return 0
		default:
			if strings.HasPrefix(arg, "--") && len(arg) > 2 {
				// Unknown long option: pass through to mpv (power users: --volume=50, --start=10).
				opt.MPVArgs = append(opt.MPVArgs, arg)
			} else {
				inputs = append(inputs, arg)
			}
		}
	}
	if err := a.Play(ctx, inputs, opt); err != nil {
		msg := err.Error()
		if err == mpv.ErrNotFound || strings.Contains(msg, "mpv not found") {
			msg += "\n\n" + mpvInstallHint()
		}
		ui.Error(gui, msg)
		return 1
	}
	return 0
}

func cmdDaemon(ctx context.Context, a *app.App, args []string) int {
	if err := a.EnsureMPV(); err != nil {
		ui.Error(a.GUI, err.Error()+"\n\n"+mpvInstallHint())
		return 1
	}
	logf := func(f string, v ...any) { fmt.Fprintf(os.Stderr, "[playanything daemon] "+f+"\n", v...) }
	err := daemon.Run(ctx, daemon.Options{
		Binary:     a.MPV,
		ConfigDir:  configDirOrEmpty(a),
		IPCPath:    a.Paths.IPCPath(),
		PIDFile:    a.Paths.PIDFile(),
		Extra:      append(append([]string{}, a.Cfg.ExtraMPVArgs...), args...),
		ScriptOpts: map[string]string{"pa-exe": a.Self},
		Log:        logf,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "playanything daemon:", err)
		return 1
	}
	return 0
}

func configDirOrEmpty(a *app.App) string {
	if a.Cfg.UseSystemMPVConfig {
		return ""
	}
	return a.Paths.MPV
}

func mpvInstallHint() string {
	return "PlayAnything plays everything through mpv. Install it with the one-line installer from the README, or:\n" +
		"  Windows: winget install -e --id shinchiro.mpv\n" +
		"  macOS:   brew install mpv\n" +
		"  Linux:   sudo apt install mpv  /  sudo dnf install mpv  /  sudo pacman -S mpv"
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// cmdConfig shows or sets a config.json key: `config`, `config daemon`, `config daemon always`.
func cmdConfig(a *app.App, args []string) int {
	keys := []string{"mpv_path", "use_system_mpv_config", "daemon", "hydrate_first", "raw_full_decode", "raw_decoder", "redline_path", "redline_args", "fullscreen"}
	get := func(k string) string {
		c := a.Cfg
		switch k {
		case "mpv_path":
			return c.MPVPath
		case "use_system_mpv_config":
			return fmt.Sprint(c.UseSystemMPVConfig)
		case "daemon":
			return c.Daemon
		case "hydrate_first":
			return fmt.Sprint(c.HydrateFirst)
		case "raw_full_decode":
			return fmt.Sprint(c.RawFullDecode)
		case "raw_decoder":
			return c.RawDecoder
		case "redline_path":
			return c.REDlinePath
		case "redline_args":
			return c.REDlineArgs
		case "fullscreen":
			return fmt.Sprint(c.Fullscreen)
		}
		return ""
	}
	if len(args) == 0 {
		fmt.Printf("# %s\n", a.Paths.File())
		for _, k := range keys {
			fmt.Printf("%-22s %s\n", k, get(k))
		}
		return 0
	}
	k := args[0]
	valid := false
	for _, kk := range keys {
		if kk == k {
			valid = true
		}
	}
	if !valid {
		fmt.Fprintf(os.Stderr, "unknown key %q; keys: %s\n", k, strings.Join(keys, ", "))
		return 2
	}
	if len(args) == 1 {
		fmt.Println(get(k))
		return 0
	}
	v := strings.Join(args[1:], " ")
	b := strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	c := a.Cfg
	switch k {
	case "mpv_path":
		c.MPVPath = v
	case "use_system_mpv_config":
		c.UseSystemMPVConfig = b
	case "daemon":
		if v != "auto" && v != "always" && v != "never" {
			fmt.Fprintln(os.Stderr, "daemon must be auto, always or never")
			return 2
		}
		c.Daemon = v
	case "hydrate_first":
		c.HydrateFirst = b
	case "raw_full_decode":
		c.RawFullDecode = b
	case "raw_decoder":
		c.RawDecoder = v
	case "redline_path":
		c.REDlinePath = v
	case "redline_args":
		c.REDlineArgs = v
	case "fullscreen":
		c.Fullscreen = b
	}
	if err := config.Save(a.Paths, c); err != nil {
		fmt.Fprintln(os.Stderr, "playanything:", err)
		return 1
	}
	fmt.Printf("%s = %s\n", k, get(k))
	return 0
}
