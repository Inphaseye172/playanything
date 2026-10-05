package config

import (
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	t.Setenv("PLAYANYTHING_HOME", t.TempDir())
	p := ResolvePaths()
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Daemon != "auto" || c.REDlineArgs != DefaultREDlineArgs || c.RawDecoder != "auto" {
		t.Fatalf("defaults: %+v", c)
	}
	c.MPVPath = "/opt/mpv/bin/mpv"
	c.HydrateFirst = true
	c.Daemon = ""
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c2.MPVPath != c.MPVPath || !c2.HydrateFirst || c2.Daemon != "auto" {
		t.Fatalf("round trip: %+v", c2)
	}
	if filepath.Dir(p.MPV) != p.Config || filepath.Dir(p.PIDFile()) != p.Runtime {
		t.Fatalf("paths: %+v", p)
	}
}
