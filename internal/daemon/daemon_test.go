package daemon

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/inphaseye172/playanything/internal/mpv"
)

func TestRunningWithMissingPidFile(t *testing.T) {
	if pid, alive := Running(filepath.Join(t.TempDir(), "nope.pid")); alive || pid != 0 {
		t.Fatal("missing pid file should not be running")
	}
}

func TestDaemonLifecycleWithRealMPV(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a desktop session")
	}
	path, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "daemon.pid")
	sock := filepath.Join(dir, "mpv.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Binary:  mpv.Binary{Path: path},
			IPCPath: sock,
			PIDFile: pidFile,
			Extra:   []string{"--no-config", "--vo=null", "--ao=null", "--really-quiet"},
			Log:     t.Logf,
		})
	}()
	var c *mpv.Client
	deadline := time.Now().Add(10 * time.Second)
	for {
		c, err = Connect(sock)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("daemon never came up: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, alive := Running(pidFile); !alive {
		t.Fatal("pid file does not report a live daemon")
	}
	// Hand it a file like the launcher would.
	if err := c.LoadFile("av://lavfi:testsrc=duration=0.5:size=64x64:rate=10", "replace"); err != nil {
		t.Fatal(err)
	}
	// Quitting mpv (what the user does with q) must bring a fresh idle instance back.
	_, _ = c.Command("quit")
	c.Close()
	time.Sleep(300 * time.Millisecond)
	deadline = time.Now().Add(10 * time.Second)
	for {
		c2, err := Connect(sock)
		if err == nil {
			c2.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mpv was not restarted after quit")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Second daemon must refuse to start.
	if err := Run(context.Background(), Options{Binary: mpv.Binary{Path: path}, IPCPath: sock, PIDFile: pidFile}); err == nil {
		t.Fatal("second daemon should refuse")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not stop")
	}
	if _, alive := Running(pidFile); alive {
		t.Fatal("pid file left behind")
	}
}
