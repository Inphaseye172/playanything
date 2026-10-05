package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallIdempotentAndPreservesUserConf(t *testing.T) {
	dir := t.TempDir()
	wrote, err := Install(dir, false)
	if err != nil || !wrote {
		t.Fatalf("first install: wrote=%v err=%v", wrote, err)
	}
	for _, f := range Files() {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	// user.conf is the user's: never overwritten
	uc := filepath.Join(dir, "user.conf")
	os.WriteFile(uc, []byte("fullscreen=yes\n"), 0o644)
	// managed file edited by hand gets restored only on revision change / force
	os.WriteFile(filepath.Join(dir, "input.conf"), []byte("# edited\n"), 0o644)
	wrote, err = Install(dir, false)
	if err != nil || wrote {
		t.Fatalf("second install should be a no-op: wrote=%v err=%v", wrote, err)
	}
	wrote, err = Install(dir, true)
	if err != nil || !wrote {
		t.Fatalf("forced install: wrote=%v err=%v", wrote, err)
	}
	b, _ := os.ReadFile(uc)
	if string(b) != "fullscreen=yes\n" {
		t.Fatal("user.conf was overwritten")
	}
	b, _ = os.ReadFile(filepath.Join(dir, "input.conf"))
	if string(b) == "# edited\n" {
		t.Fatal("managed input.conf was not restored on force")
	}
	// stamp mismatch triggers rewrite
	os.WriteFile(filepath.Join(dir, stampName), []byte("0\n"), 0o644)
	wrote, _ = Install(dir, false)
	if !wrote {
		t.Fatal("stale stamp should trigger rewrite")
	}
}
