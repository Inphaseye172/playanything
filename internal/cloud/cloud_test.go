package cloud

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuessProvider(t *testing.T) {
	cases := map[string]string{
		`C:\Users\me\SynologyDrive\Footage\a.mkv`:                "Synology Drive",
		`D:\Synology Drive\team\clip.mov`:                        "Synology Drive",
		`C:\Users\me\OneDrive - Studio\x.mp4`:                    "OneDrive",
		"/Users/me/Library/CloudStorage/SynologyDrive-NAS/a.ARW": "Synology Drive",
		"/Users/me/Library/CloudStorage/Box-Box/a.ARW":           "File Provider",
		"/home/me/Dropbox/a.flac":                                "Dropbox",
		"/home/me/Videos/a.mkv":                                  "",
	}
	for p, want := range cases {
		if got := guessProvider(p); got != want {
			t.Errorf("guessProvider(%q)=%q want %q", p, got, want)
		}
	}
}

func TestStatusAndHydrateLocal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.bin")
	data := make([]byte, 9<<20) // > one 4 MiB read buffer
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	st := Status(p)
	if !st.Exists || st.Size != int64(len(data)) || st.IsOffline {
		t.Fatalf("status: %+v", st)
	}
	if st.Describe() != "local file" && !st.IsNetwork {
		t.Fatalf("describe: %q", st.Describe())
	}
	var last, calls int64
	if err := Hydrate(p, func(done, total int64) { last = done; calls++; _ = total }); err != nil {
		t.Fatal(err)
	}
	if last != int64(len(data)) || calls < 3 {
		t.Fatalf("hydrate progress: last=%d calls=%d", last, calls)
	}
	missing := Status(filepath.Join(dir, "nope.mkv"))
	if missing.Exists {
		t.Fatal("missing file reported as existing")
	}
}
