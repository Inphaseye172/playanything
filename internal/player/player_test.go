package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/inphaseye172/playanything/internal/config"
	"github.com/inphaseye172/playanything/internal/engine"
)

// synthARW builds a minimal Sony-style ARW: TIFF with Orientation=6 and a
// JPEGInterchangeFormat preview of the given size.
func synthARW(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, img, nil); err != nil {
		t.Fatal(err)
	}
	jpg := jb.Bytes()
	buf := []byte("II*\x00\x00\x00\x00\x00")
	put := func(d []byte) int {
		off := len(buf)
		buf = append(buf, d...)
		if len(buf)%2 != 0 {
			buf = append(buf, 0)
		}
		return off
	}
	raw := put(make([]byte, 50_000))
	jo := put(jpg)
	le16 := func(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
	le32 := func(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }
	ifd := len(buf)
	entries := [][]byte{
		append(append(le16(0x0103), le16(3)...), append(le32(1), le16(32767)...)...),
		append(append(le16(0x0112), le16(3)...), append(le32(1), le16(6)...)...),
		append(append(le16(0x0111), le16(4)...), append(le32(1), le32(uint32(raw))...)...),
		append(append(le16(0x0117), le16(4)...), append(le32(1), le32(50_000)...)...),
		append(append(le16(0x0201), le16(4)...), append(le32(1), le32(uint32(jo))...)...),
		append(append(le16(0x0202), le16(4)...), append(le32(1), le32(uint32(len(jpg)))...)...),
	}
	buf = append(buf, le16(uint16(len(entries)))...)
	for _, e := range entries {
		b := make([]byte, 12)
		copy(b, e)
		buf = append(buf, b...)
	}
	buf = append(buf, le32(0)...)
	binary.LittleEndian.PutUint32(buf[4:], uint32(ifd))
	return buf
}

func newTestPlayer(t *testing.T) *Player {
	t.Helper()
	if _, err := engine.Load(); err != nil {
		t.Skip("libmpv not available:", err)
	}
	home := t.TempDir()
	t.Setenv("PLAYANYTHING_HOME", home)
	paths := config.ResolvePaths()
	p, err := New(Options{Paths: paths, Cfg: &config.Config{}, Headless: true, Log: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func waitFor(t *testing.T, p *Player, kind EventKind, timeout time.Duration) Event {
	t.Helper()
	ch := make(chan Event, 64)
	p.OnEvent(func(e Event) {
		select {
		case ch <- e:
		default:
		}
	})
	deadline := time.After(timeout)
	for {
		select {
		case e := <-ch:
			if e.Kind == kind {
				return e
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event %d", kind)
		}
	}
}

func TestRawPhotoOpensAsRotatedPreview(t *testing.T) {
	p := newTestPlayer(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "DSC00001.ARW")
	b := filepath.Join(dir, "DSC00002.ARW")
	os.WriteFile(a, synthARW(t, 640, 426), 0o644)
	os.WriteFile(b, synthARW(t, 320, 213), 0o644)
	ch := make(chan Event, 64)
	p.OnEvent(func(e Event) {
		select {
		case ch <- e:
		default:
		}
	})
	if err := p.Open(context.Background(), []string{a}, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(20 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Kind == EvFileLoaded {
				goto loaded
			}
			if e.Kind == EvEndFile && e.Error {
				t.Fatalf("load failed: %s", e.Text)
			}
		case <-deadline:
			t.Fatal("timeout")
		}
	}
loaded:
	st := p.Snapshot()
	if st.Width != 640 || st.Height != 426 {
		t.Fatalf("preview size %dx%d", st.Width, st.Height)
	}
	if r, _ := p.E.GetInt("video-rotate"); r != 90 {
		t.Fatalf("rotation %d", r)
	}
	if title, _ := p.E.GetString("media-title"); title != "DSC00001.ARW" {
		t.Fatalf("media-title %q", title)
	}
	if !st.IsImage {
		t.Fatal("expected image state")
	}
	// The folder sibling was added by the autoload script (loaded from the managed config dir).
	time.Sleep(500 * time.Millisecond)
	if n, _ := p.E.GetInt("playlist-count"); n != 2 {
		t.Fatalf("playlist-count=%d (autoload)", n)
	}
	if _, err := os.Stat(filepath.Join(config.ResolvePaths().Cache, "raw-previews")); err != nil {
		t.Fatal("preview cache dir missing")
	}
}

func TestTracksAndControls(t *testing.T) {
	p := newTestPlayer(t)
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	clip := filepath.Join(t.TempDir(), "two-audio.mkv")
	cmd := exec.Command(ff, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration=3:size=64x64:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-f", "lavfi", "-i", "sine=frequency=880:duration=3",
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-metadata:s:a:0", "language=eng", "-metadata:s:a:0", "title=Game",
		"-metadata:s:a:1", "language=deu", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v %s", err, out)
	}
	if err := p.Open(context.Background(), []string{clip}, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, p, EvFileLoaded, 20*time.Second)
	tracks := p.Tracks()
	var audio []Track
	for _, tr := range tracks {
		if tr.Type == "audio" {
			audio = append(audio, tr)
		}
	}
	if len(audio) != 2 || audio[0].Lang != "eng" || audio[0].Title != "Game" || audio[0].Codec != "aac" {
		t.Fatalf("tracks: %+v", tracks)
	}
	if !audio[0].Selected || audio[1].Selected {
		t.Fatalf("initial selection: %+v", audio)
	}
	if err := p.SelectTrack("audio", audio[1].ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if aid, _ := p.E.GetString("aid"); aid != "2" {
		t.Fatalf("aid=%s", aid)
	}
	if err := p.TogglePause(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if !p.Snapshot().Paused {
		t.Fatal("pause not reflected in state")
	}
	if err := p.SetVolume(55); err != nil {
		t.Fatal(err)
	}
	if err := p.SeekPercent(50, true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	st := p.Snapshot()
	if st.Volume != 55 || st.TimePos < 1.0 || st.Duration < 2.5 {
		t.Fatalf("state after seek: %+v", st)
	}
	if err := p.Key("SPACE"); err != nil { // input.conf: toggle pause
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if p.Snapshot().Paused {
		t.Fatal("keypress SPACE did not resume")
	}
	if lbl := audio[0].Label(); lbl != "eng Game (aac 1ch)" {
		t.Fatalf("label %q", lbl)
	}
	if FormatTime(3725) != "1:02:05" || FormatTime(65) != "1:05" {
		t.Fatal("FormatTime")
	}
}

func TestOpenErrors(t *testing.T) {
	p := newTestPlayer(t)
	if err := p.Open(context.Background(), []string{"/definitely/missing.mkv"}, false); err == nil {
		t.Fatal("expected error for missing file")
	}
	r3d := filepath.Join(t.TempDir(), "A001_C001.R3D")
	os.WriteFile(r3d, []byte("\x00\x00\x00\x18RED2"), 0o644)
	err := p.Open(context.Background(), []string{r3d}, false)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("REDCINE-X")) {
		t.Fatalf("R3D without tools should explain: %v", err)
	}
	junk := filepath.Join(t.TempDir(), "junk.mp4")
	os.WriteFile(junk, []byte("not a video at all"), 0o644)
	if err := p.Open(context.Background(), []string{junk}, false); err != nil {
		t.Fatal(err)
	}
	ev := waitFor(t, p, EvEndFile, 20*time.Second)
	if !ev.Error || ev.Text == "" {
		t.Fatalf("expected an error event: %+v", ev)
	}
}
