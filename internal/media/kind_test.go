package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestByExtension(t *testing.T) {
	cases := map[string]Kind{
		"a.MKV": Video, "b.mp4": Video, "c.mov": Video, "d.mxf": Video, "e.m2ts": Video,
		"f.flac": Audio, "g.mp3": Audio, "h.dsf": Audio, "i.wav": Audio,
		"j.jpg": Image, "k.heic": Image, "l.exr": Image, "m.png": Image,
		"n.ARW": RawImage, "o.cr3": RawImage, "p.NEF": RawImage, "q.dng": RawImage, "r.raf": RawImage,
		"s.R3D": R3D, "t.braw": BRAW, "u.m3u8": Playlist, "v.txt": Unknown, "noext": Unknown,
	}
	for p, want := range cases {
		if got := ByExtension(p); got != want {
			t.Errorf("ByExtension(%q)=%v want %v", p, got, want)
		}
	}
}

func TestIsURL(t *testing.T) {
	for _, u := range []string{"http://x", "https://x/y.mkv", "rtsp://cam", "srt://1.2.3.4:9000", "smb://nas/share"} {
		if !IsURL(u) {
			t.Errorf("IsURL(%q)=false", u)
		}
	}
	for _, u := range []string{`C:\Videos\a.mkv`, "C:/x", "/home/u/a.mkv", "a.mkv", `\\nas\share\a.mkv`} {
		if IsURL(u) {
			t.Errorf("IsURL(%q)=true", u)
		}
	}
}

func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Kind
	}{
		{"mkv", []byte{0x1a, 0x45, 0xdf, 0xa3, 0x93, 0x42, 0x82}, Video},
		{"mp4", append([]byte{0, 0, 0, 0x20}, []byte("ftypisom")...), Video},
		{"m4a", append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A ")...), Audio},
		{"heic", append([]byte{0, 0, 0, 0x18}, []byte("ftypheic")...), Image},
		{"cr3", append([]byte{0, 0, 0, 0x18}, []byte("ftypcrx ")...), RawImage},
		{"r3d", append([]byte{0, 0, 0, 0x18}, []byte("RED2")...), R3D},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE1}, Image},
		{"flac", []byte("fLaC....."), Audio},
		{"wav", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WAVEfmt ")...), Audio},
		{"avi", append([]byte("RIFF\x00\x00\x00\x00"), []byte("AVI LIST")...), Video},
		{"webp", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WEBPVP8 ")...), Image},
		{"raf", []byte("FUJIFILMCCD-RAW 0201"), RawImage},
		{"orf", []byte("IIRO\x08\x00\x00\x00"), RawImage},
		{"rw2", []byte("IIU\x00\x18\x00\x00\x00"), RawImage},
		{"tiff", []byte("II*\x00\x08\x00\x00\x00"), Image},
		{"m3u", []byte("#EXTM3U\n"), Playlist},
		{"text", []byte("hello world"), Unknown},
	}
	for _, c := range cases {
		if got := Sniff(c.data); got != c.want {
			t.Errorf("Sniff(%s)=%v want %v", c.name, got, c.want)
		}
	}
}

func TestDetectSniffsUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "camera_dump.bin")
	if err := os.WriteFile(p, []byte{0x1a, 0x45, 0xdf, 0xa3, 0x93, 0x42, 0x82, 0x88}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Detect(p); got != Video {
		t.Errorf("Detect=%v want Video", got)
	}
	if got := Detect(dir); got != Directory {
		t.Errorf("Detect(dir)=%v want Directory", got)
	}
	if got := Detect("https://example.com/a.mkv"); got != Stream {
		t.Errorf("Detect(url)=%v want Stream", got)
	}
}

func TestNaturalLessAndExpand(t *testing.T) {
	if !NaturalLess("clip2.mp4", "clip10.mp4") || NaturalLess("clip10.mp4", "clip2.mp4") {
		t.Error("natural order wrong")
	}
	if !NaturalLess("A001_C001.R3D", "A001_C002.R3D") {
		t.Error("natural order wrong for RED names")
	}
	dir := t.TempDir()
	for _, n := range []string{"DSC10.ARW", "DSC2.ARW", "notes.txt", "clip.mkv", ".hidden.mp4"} {
		os.WriteFile(filepath.Join(dir, n), []byte{0}, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	got, err := ExpandDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"clip.mkv", "DSC2.ARW", "DSC10.ARW"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if filepath.Base(got[i]) != want[i] {
			t.Errorf("order: got %v want %v", got, want)
			break
		}
	}
}

func TestMIMETablesAreConsistent(t *testing.T) {
	for e := range mimeByExt {
		if ByExtension("x."+e) == Unknown {
			t.Errorf("mime table has %q which is not a known extension", e)
		}
	}
	m := MIMETypes()
	if len(m) < 100 {
		t.Fatalf("expected a rich MIME list, got %d", len(m))
	}
	for i := 1; i < len(m); i++ {
		if m[i-1] >= m[i] {
			t.Fatal("MIME list not sorted/unique")
		}
	}
	if MIMEFor("cr3")[0] != "image/x-canon-cr3" {
		t.Fatal("cr3 mime")
	}
}
