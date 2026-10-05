package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/inphaseye172/playanything/internal/daemon"
	"github.com/inphaseye172/playanything/internal/mpv"
)

// session is a live mpv we can talk to: either the background player or a
// throw-away idle instance started for this one launch (used while a cloud
// file hydrates or an R3D proxy renders, so the window appears immediately).
type session struct {
	client  *mpv.Client
	shared  bool // the background daemon; do not quit it
	ipcPath string
}

// connectDaemon returns the background player if policy allows and it answers.
func (a *App) connectDaemon(noDaemon bool) *session {
	if noDaemon || a.Cfg.Daemon == "never" {
		return nil
	}
	if c, err := daemon.Connect(a.Paths.IPCPath()); err == nil {
		return &session{client: c, shared: true, ipcPath: a.Paths.IPCPath()}
	}
	if a.Cfg.Daemon == "always" {
		if err := daemon.Spawn(a.Self); err == nil {
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				if c, err := daemon.Connect(a.Paths.IPCPath()); err == nil {
					return &session{client: c, shared: true, ipcPath: a.Paths.IPCPath()}
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	return nil
}

// startIdle launches a fresh idle mpv with a window and an IPC endpoint.
func (a *App) startIdle(ctx context.Context, scriptOpts map[string]string, extra []string) (*session, error) {
	var ipc string
	if runtime.GOOS == "windows" {
		ipc = fmt.Sprintf(`\\.\pipe\playanything-%d`, os.Getpid())
	} else {
		if err := os.MkdirAll(a.Paths.Runtime, 0o755); err != nil {
			return nil, err
		}
		ipc = filepath.Join(a.Paths.Runtime, fmt.Sprintf("mpv-%d.sock", os.Getpid()))
	}
	args := mpv.Args(mpv.LaunchOptions{
		ConfigDir:       a.Paths.MPV,
		UseSystemConfig: a.Cfg.UseSystemMPVConfig,
		IPCPath:         ipc,
		Idle:            true,
		ForceWindow:     "yes",
		Fullscreen:      a.Cfg.Fullscreen,
		NoTerminal:      a.GUI,
		ScriptOpts:      scriptOpts,
		Extra:           append(append([]string{}, a.Cfg.ExtraMPVArgs...), extra...),
	})
	cmd := a.MPV.Command(args...)
	if !a.GUI {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mpv: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	c, err := mpv.Dial(ipc, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("mpv started but its IPC endpoint never appeared: %w", err)
	}
	return &session{client: c, ipcPath: ipc}, nil
}

// load hands files to the session. The first file replaces (or appends with
// append=true) and the rest queue up.
func (s *session) load(files []string, append bool, perFileOpts map[string][]string) error {
	_ = s.client.SetProperty("force-window", "yes")
	for i, f := range files {
		mode := "append"
		if i == 0 {
			mode = "replace"
			if append {
				mode = "append-play"
			}
		}
		if err := s.client.LoadFile(f, mode, perFileOpts[f]...); err != nil {
			return err
		}
	}
	s.client.Raise()
	return nil
}

func (s *session) osd(text string, ms int) { _ = s.client.ShowText(text, ms) }

func (s *session) close() { s.client.Close() }
