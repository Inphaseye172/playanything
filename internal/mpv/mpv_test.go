package mpv

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestArgs(t *testing.T) {
	got := Args(LaunchOptions{
		ConfigDir:   "/cfg/mpv",
		IPCPath:     "/run/mpv.sock",
		Idle:        true,
		ForceWindow: "immediate",
		Rotate:      90,
		ScriptOpts:  map[string]string{"pa-exe": "/bin/playanything", "pa-cloud": "Synology Drive"},
		Extra:       []string{"--volume=50"},
		Files:       []string{"-weird.mkv", "b.mp4"},
	})
	want := []string{
		"--config-dir=/cfg/mpv", "--input-ipc-server=/run/mpv.sock", "--idle=yes", "--force-window=immediate",
		"--video-rotate=90", "--script-opts-append=pa-cloud=Synology Drive", "--script-opts-append=pa-exe=/bin/playanything",
		"--volume=50", "--", "-weird.mkv", "b.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if a := Args(LaunchOptions{ConfigDir: "/x", UseSystemConfig: true}); len(a) != 0 {
		t.Fatalf("system config should drop --config-dir: %q", a)
	}
}

// TestIPCAgainstRealMPV drives a headless mpv when one is installed.
func TestIPCAgainstRealMPV(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipe test needs a desktop session")
	}
	path, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	sock := filepath.Join(t.TempDir(), "s")
	cmd := exec.Command(path, "--no-config", "--idle=yes", "--vo=null", "--ao=null", "--input-ipc-server="+sock, "--really-quiet")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()

	c, err := Dial(sock, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !c.Ping() {
		t.Fatal("ping failed")
	}
	var idle bool
	if err := c.GetProperty("idle-active", &idle); err != nil || !idle {
		t.Fatalf("idle-active=%v err=%v", idle, err)
	}
	if err := c.SetProperty("volume", 42); err != nil {
		t.Fatal(err)
	}
	var vol float64
	if err := c.GetProperty("volume", &vol); err != nil || vol != 42 {
		t.Fatalf("volume=%v err=%v", vol, err)
	}
	if err := c.ShowText("hello", 100); err != nil {
		t.Fatal(err)
	}
	// Load a synthetic clip with a per-file option and check it took effect.
	if err := c.LoadFile("av://lavfi:testsrc=duration=1:size=64x64:rate=10", "replace", "video-rotate=90"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var rot int
		var p string
		_ = c.GetProperty("path", &p)
		if err := c.GetProperty("video-rotate", &rot); err == nil && rot == 90 && p != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("file did not load with rotation: path=%q rot=%d", p, rot)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := c.Command("no-such-command"); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

func TestFindFallsBackToPath(t *testing.T) {
	b, err := Find("", t.TempDir())
	if _, lookErr := exec.LookPath("mpv"); lookErr != nil {
		if err == nil {
			t.Fatalf("expected not found, got %+v", b)
		}
		return
	}
	if err != nil || b.Path == "" {
		t.Fatalf("Find: %v %+v", err, b)
	}
	if _, err := Find("/definitely/not/here/mpv", ""); err == nil {
		t.Fatal("bad override should error")
	}
}
