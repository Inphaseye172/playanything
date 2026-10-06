package red

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestExpandArgs(t *testing.T) {
	got := ExpandArgs("--i {input} --o {outbase} --outDir {outdir} --format 11 --QTcodec 2 --res 4",
		"/v/A001_C001.R3D", "A001_C001-abc", "/cache/r3d")
	want := []string{"--i", "/v/A001_C001.R3D", "--o", "A001_C001-abc", "--outDir", "/cache/r3d", "--format", "11", "--QTcodec", "2", "--res", "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestProxyPathAndFind(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "A001_C001_0101XX_001.R3D")
	os.WriteFile(in, []byte("RED2"), 0o644)
	cache := filepath.Join(dir, "cache")
	os.MkdirAll(cache, 0o755)
	base, err := ProxyPath(cache, in)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(base) != cache || filepath.Base(base)[:21] != "A001_C001_0101XX_001-" {
		t.Fatalf("base=%s", base)
	}
	if FindProxy(cache, in) != "" {
		t.Fatal("proxy should not exist yet")
	}
	os.WriteFile(base+".log", []byte("x"), 0o644)
	os.WriteFile(base+".mov", []byte("moov"), 0o644)
	if got := FindProxy(cache, in); got != base+".mov" {
		t.Fatalf("FindProxy=%q", got)
	}
}

// A fake REDline (shell script) exercises RenderProxy end to end.
func TestRenderProxyWithFakeREDline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.R3D")
	os.WriteFile(in, []byte("RED2"), 0o644)
	cache := filepath.Join(dir, "cache")
	fake := filepath.Join(dir, "REDline")
	script := "#!/bin/sh\n# args: --i IN --o BASE --outDir DIR ...\nwhile [ $# -gt 0 ]; do case \"$1\" in --o) BASE=$2; shift;; --outDir) DIR=$2; shift;; esac; shift; done\necho 'Decoding 50%'\necho 'Decoding 100%'\nprintf moov > \"$DIR/$BASE.mov\"\n"
	os.WriteFile(fake, []byte(script), 0o755)
	var lines []string
	out, err := RenderProxy(context.Background(), fake, "--i {input} --o {outbase} --outDir {outdir} --format 11", cache, in, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(out) != ".mov" || len(lines) != 3 || lines[2] != "Decoding 100%" {
		t.Fatalf("out=%s lines=%q", out, lines)
	}
	if FindProxy(cache, in) != out {
		t.Fatal("rendered proxy not found afterwards")
	}
}

func TestDetectDoesNotPanic(t *testing.T) {
	tools := Detect("")
	_ = tools
	if Detect("/nope/REDline").REDline != "" {
		t.Fatal("bad override accepted")
	}
}

func TestRenderProxyFallbackAndErrorTail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.R3D")
	os.WriteFile(in, []byte("RED2"), 0o644)
	cache := filepath.Join(dir, "cache")
	// Fails for --format 11, succeeds for --format 12 (second-level fallback).
	fake := filepath.Join(dir, "REDline")
	script := "#!/bin/sh\nfmt=; while [ $# -gt 0 ]; do case \"$1\" in --format) fmt=$2; shift;; --o) BASE=$2; shift;; --outDir) DIR=$2; shift;; esac; shift; done\nif [ \"$fmt\" != 12 ]; then echo \"Error: unsupported output format $fmt\"; exit 1; fi\nprintf moov > \"$DIR/$BASE.mxf\"\n"
	os.WriteFile(fake, []byte(script), 0o755)
	out, err := RenderProxy(context.Background(), fake, "--i {input} --o {outbase} --outDir {outdir} --format 11", cache, in, nil)
	if err != nil || filepath.Ext(out) != ".mxf" {
		t.Fatalf("fallback: out=%s err=%v", out, err)
	}
	// Always failing: the error must carry REDline's words and the log path.
	always := filepath.Join(dir, "REDline2")
	os.WriteFile(always, []byte("#!/bin/sh\necho 'License check failed: no RED SDK license'\nexit 1\n"), 0o755)
	in2 := filepath.Join(dir, "clip2.R3D")
	os.WriteFile(in2, []byte("RED2"), 0o644)
	_, err = RenderProxy(context.Background(), always, "", cache, in2, nil)
	if err == nil || !strings.Contains(err.Error(), "License check failed") || !strings.Contains(err.Error(), ".log") {
		t.Fatalf("error should quote REDline and the log: %v", err)
	}
}
