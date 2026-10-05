// Package daemon runs PlayAnything's optional background player: one idle mpv
// process kept alive with a JSON IPC socket. Opening a file then means sending
// a `loadfile` command to the running instance instead of starting a process,
// which gives a single always-reused window, instant opens, and lets you queue
// files with --append. It is a per-user background process (a systemd user
// service, launchd agent or Windows Run entry), because a real session-0
// service could not show a window.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/inphaseye172/playanything/internal/mpv"
)

// Options configures Run.
type Options struct {
	Binary     mpv.Binary
	ConfigDir  string
	IPCPath    string
	PIDFile    string
	Extra      []string
	ScriptOpts map[string]string
	Log        func(format string, args ...any)
}

// Run keeps an idle mpv alive until ctx is cancelled. It writes a pid file so
// `playanything stop` and `status` can find it.
func Run(ctx context.Context, o Options) error {
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	if pid, alive := Running(o.PIDFile); alive {
		return fmt.Errorf("background player already running (pid %d)", pid)
	}
	if err := os.MkdirAll(filepath.Dir(o.PIDFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(o.PIDFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	defer os.Remove(o.PIDFile)

	backoff := 500 * time.Millisecond
	for {
		args := mpv.Args(mpv.LaunchOptions{
			ConfigDir:   o.ConfigDir,
			IPCPath:     o.IPCPath,
			Idle:        true,
			ForceWindow: "no",
			NoTerminal:  true,
			ScriptOpts:  o.ScriptOpts,
			Extra:       o.Extra,
		})
		cmd := o.Binary.Command(args...)
		// mpv runs with --terminal=no; when the daemon itself was spawned
		// detached (Windows) our own stdio handles may be invalid, so pass none.
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
		start := time.Now()
		o.Log("starting mpv: %s %s", o.Binary.Path, strings.Join(args, " "))
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start mpv: %w", err)
		}
		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			o.Log("stopping")
			if c, err := mpv.Dial(o.IPCPath, time.Second); err == nil {
				_, _ = c.Command("quit")
				c.Close()
			}
			select {
			case <-waitErr:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
			}
			return nil
		case err := <-waitErr:
			// mpv exits when the user presses q; that is normal - start a fresh idle instance.
			o.Log("mpv exited (%v), restarting", err)
			if time.Since(start) < 2*time.Second {
				backoff *= 2
				if backoff > 15*time.Second {
					backoff = 15 * time.Second
				}
			} else {
				backoff = 500 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
		}
	}
}

// Running reports the daemon pid from pidFile and whether that process is alive.
func Running(pidFile string) (int, bool) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, processAlive(pid)
}

// Connect returns an IPC client to the background player when it is up.
func Connect(ipcPath string) (*mpv.Client, error) {
	c, err := mpv.Dial(ipcPath, 300*time.Millisecond)
	if err != nil {
		return nil, err
	}
	if !c.Ping() {
		c.Close()
		return nil, errors.New("background player did not answer")
	}
	return c, nil
}

// Stop terminates the daemon (and its mpv). Safe to call when not running.
func Stop(pidFile, ipcPath string) error {
	pid, alive := Running(pidFile)
	if !alive {
		os.Remove(pidFile)
		// a stray mpv may still own the socket
		if c, err := mpv.Dial(ipcPath, 200*time.Millisecond); err == nil {
			_, _ = c.Command("quit")
			c.Close()
		}
		removeSocketFile(ipcPath)
		return nil
	}
	if err := terminate(pid); err != nil {
		return err
	}
	// Give the daemon a moment to shut mpv down itself, then make sure.
	time.Sleep(500 * time.Millisecond)
	if c, err := mpv.Dial(ipcPath, 200*time.Millisecond); err == nil {
		_, _ = c.Command("quit")
		c.Close()
	}
	os.Remove(pidFile)
	removeSocketFile(ipcPath)
	return nil
}

// removeSocketFile deletes a stale unix socket; named pipes vanish by themselves.
func removeSocketFile(ipcPath string) {
	if strings.HasPrefix(ipcPath, `\\.\pipe\`) {
		return
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := mpv.Dial(ipcPath, 100*time.Millisecond); err != nil {
		os.Remove(ipcPath)
	}
}

// Spawn starts `playanything daemon` detached, for config daemon="always".
func Spawn(self string, extraArgs ...string) error {
	cmd := exec.Command(self, append([]string{"daemon"}, extraArgs...)...)
	// Tell the child not to attach to our console: otherwise closing the
	// terminal that spawned it would take the background player down.
	cmd.Env = append(os.Environ(), "PLAYANYTHING_DETACHED=1")
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
