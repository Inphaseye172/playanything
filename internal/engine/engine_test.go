package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"plain":                `plain`,
		"with space":           `"with space"`,
		`C:\Users\me\a b.mkv`:  `"C:\\Users\\me\\a b.mkv"`,
		`say "hi"`:             `"say \"hi\""`,
		"":                     `""`,
		"av://lavfi:testsrc=1": `av://lavfi:testsrc=1`,
	}
	for in, want := range cases {
		if got := quote(in); got != want {
			t.Errorf("quote(%q)=%s want %s", in, got, want)
		}
	}
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	if _, err := Load(); err != nil {
		t.Skip("libmpv not available:", err)
	}
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"vo": "null", "ao": "null", "idle": "yes", "terminal": "no", "msg-level": "all=no"} {
		if err := e.SetOption(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e
}

func waitEvent(t *testing.T, e *Engine, id EventID, timeout time.Duration) Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-e.Events():
			if !ok {
				t.Fatalf("events closed while waiting for %s", id)
			}
			if ev.ID == id {
				return ev
			}
		case <-deadline:
			t.Fatalf("timeout waiting for %s", id)
		}
	}
}

func TestLoadPlayAndProperties(t *testing.T) {
	e := newTestEngine(t)
	t.Logf("libmpv %s at %s", e.Library().VersionString(), e.Library().Path)

	if v, err := e.GetString("mpv-version"); err != nil || v == "" {
		t.Fatalf("mpv-version: %q %v", v, err)
	}
	if err := e.SetProperty("volume", 42.0); err != nil {
		t.Fatal(err)
	}
	if v, err := e.GetFloat("volume"); err != nil || v != 42 {
		t.Fatalf("volume=%v err=%v", v, err)
	}
	if err := e.SetProperty("pause", true); err != nil {
		t.Fatal(err)
	}
	if p, err := e.GetBool("pause"); err != nil || !p {
		t.Fatalf("pause=%v err=%v", p, err)
	}
	if err := e.Observe(7, "duration", FormatDouble); err != nil {
		t.Fatal(err)
	}
	if err := e.Command("loadfile", "av://lavfi:testsrc=duration=2:size=64x64:rate=10", "replace"); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, e, EventFileLoaded, 15*time.Second)
	if n, err := e.GetInt("track-list/count"); err != nil || n != 1 {
		t.Fatalf("track-list/count=%d err=%v", n, err)
	}
	if typ, _ := e.GetString("track-list/0/type"); typ != "video" {
		t.Fatalf("track type %q", typ)
	}
	// duration can arrive a moment after file-loaded for generated sources.
	var d float64
	for i := 0; i < 100; i++ {
		var err error
		if d, err = e.GetFloat("duration"); err == nil && d > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if d <= 0 || d > 2.5 {
		t.Fatalf("duration=%v", d)
	}
	// The observed property must have been delivered with a double value.
	ev := waitEvent(t, e, EventPropertyChange, 5*time.Second)
	if ev.Name != "duration" || ev.ReplyUserdata != 7 || ev.Format != FormatDouble || ev.Double <= 0 {
		t.Fatalf("property event: %+v", ev)
	}
	if err := e.Command("set", "video-rotate", "90"); err != nil {
		t.Fatal(err)
	}
	if r, _ := e.GetInt("video-rotate"); r != 90 {
		t.Fatalf("video-rotate=%d", r)
	}
	// Per-file options through loadfile's option string.
	if err := e.Command("loadfile", "av://lavfi:testsrc=duration=1:size=32x32:rate=10", "replace", "video-rotate=180"); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, e, EventFileLoaded, 15*time.Second)
	if r, _ := e.GetInt("video-rotate"); r != 180 {
		t.Fatalf("per-file video-rotate=%d", r)
	}
	if err := e.Command("no-such-command"); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

func TestOnLoadHookRedirectsFile(t *testing.T) {
	e := newTestEngine(t)
	if err := e.HookAdd(99, "on_load", 50); err != nil {
		t.Fatal(err)
	}
	if err := e.RequestLog("warn"); err != nil {
		t.Fatal(err)
	}
	// Ask for a file that does not exist; the hook swaps it for a source that does.
	fake := filepath.Join(t.TempDir(), "DSC00001.ARW")
	if err := e.Command("loadfile", fake, "replace"); err != nil {
		t.Fatal(err)
	}
	hook := waitEvent(t, e, EventHook, 10*time.Second)
	if hook.Name != "on_load" || hook.ReplyUserdata != 99 || hook.HookID == 0 {
		t.Fatalf("hook event: %+v", hook)
	}
	got, _ := e.GetString("stream-open-filename")
	if got != fake {
		t.Fatalf("stream-open-filename=%q", got)
	}
	if err := e.SetProperty("stream-open-filename", "av://lavfi:testsrc=duration=1:size=48x24:rate=10"); err != nil {
		t.Fatal(err)
	}
	if err := e.SetProperty("file-local-options/video-rotate", int64(270)); err != nil {
		t.Fatal(err)
	}
	if err := e.HookContinue(hook.HookID); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, e, EventFileLoaded, 15*time.Second)
	if w, _ := e.GetInt("video-params/w"); w != 48 {
		t.Fatalf("video width=%d (hook redirect failed)", w)
	}
	if r, _ := e.GetInt("video-rotate"); r != 270 {
		t.Fatalf("file-local video-rotate=%d", r)
	}
}

func TestEndFileErrorIsReported(t *testing.T) {
	e := newTestEngine(t)
	junk := filepath.Join(t.TempDir(), "junk.mkv")
	os.WriteFile(junk, []byte("definitely not a video"), 0o644)
	if err := e.Command("loadfile", junk, "replace"); err != nil {
		t.Fatal(err)
	}
	ev := waitEvent(t, e, EventEndFile, 15*time.Second)
	if ev.EndReason != EndError || ev.EndError >= 0 {
		t.Fatalf("end-file: %+v", ev)
	}
	if msg := e.ErrorText(ev.EndError); msg == "" {
		t.Fatal("empty error text")
	}
}
