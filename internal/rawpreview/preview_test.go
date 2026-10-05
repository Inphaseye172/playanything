package rawpreview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// makeJPEG renders a real baseline JPEG of the given size.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// noise returns pseudo-random "sensor data" sprinkled with fake SOI markers.
func noise(n int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	b := make([]byte, n)
	r.Read(b)
	for i := 100; i+4 < n; i += 4096 {
		b[i], b[i+1], b[i+2], b[i+3] = 0xFF, 0xD8, 0xFF, 0xE1 // looks like a JPEG, is not
	}
	return b
}

// tiffBuilder assembles little-endian TIFF structures for tests.
type tiffBuilder struct {
	buf []byte
}

type tEntry struct {
	tag, typ uint16
	vals     []uint32 // for SHORT/LONG
	raw      []byte   // for UNDEFINED/BYTE (typ 7/1)
}

func newBuilder() *tiffBuilder {
	b := &tiffBuilder{}
	b.buf = append(b.buf, 'I', 'I', 0x2A, 0x00, 0, 0, 0, 0) // IFD0 offset patched later
	return b
}

func (b *tiffBuilder) put(data []byte) int {
	off := len(b.buf)
	b.buf = append(b.buf, data...)
	for len(b.buf)%2 != 0 {
		b.buf = append(b.buf, 0)
	}
	return off
}

func (b *tiffBuilder) le16(v uint16) []byte {
	x := make([]byte, 2)
	binary.LittleEndian.PutUint16(x, v)
	return x
}
func (b *tiffBuilder) le32(v uint32) []byte {
	x := make([]byte, 4)
	binary.LittleEndian.PutUint32(x, v)
	return x
}

// ifd writes an IFD and returns its offset. nextOff is patched via the returned pointer position.
func (b *tiffBuilder) ifd(entries []tEntry, next uint32) int {
	// first write out-of-line data
	type pending struct {
		e    tEntry
		data []byte
		off  int
	}
	var ps []pending
	for _, e := range entries {
		var data []byte
		switch e.typ {
		case 3:
			for _, v := range e.vals {
				data = append(data, b.le16(uint16(v))...)
			}
		case 4, 13:
			for _, v := range e.vals {
				data = append(data, b.le32(v)...)
			}
		case 7, 1:
			data = e.raw
		}
		p := pending{e: e, data: data}
		if len(data) > 4 {
			p.off = b.put(data)
		}
		ps = append(ps, p)
	}
	off := len(b.buf)
	b.buf = append(b.buf, b.le16(uint16(len(entries)))...)
	for _, p := range ps {
		count := uint32(len(p.e.vals))
		if p.e.typ == 7 || p.e.typ == 1 {
			count = uint32(len(p.e.raw))
		}
		b.buf = append(b.buf, b.le16(p.e.tag)...)
		b.buf = append(b.buf, b.le16(p.e.typ)...)
		b.buf = append(b.buf, b.le32(count)...)
		if len(p.data) > 4 {
			b.buf = append(b.buf, b.le32(uint32(p.off))...)
		} else {
			v := make([]byte, 4)
			copy(v, p.data)
			b.buf = append(b.buf, v...)
		}
	}
	b.buf = append(b.buf, b.le32(next)...)
	return off
}

func (b *tiffBuilder) setIFD0(off int) { binary.LittleEndian.PutUint32(b.buf[4:], uint32(off)) }

func TestParseJPEG(t *testing.T) {
	j := makeJPEG(t, 320, 200)
	info, ok := parseJPEG(j)
	if !ok || info.Width != 320 || info.Height != 200 || info.Length != len(j) || !info.displayable() {
		t.Fatalf("parse: ok=%v info=%+v len=%d", ok, info, len(j))
	}
	// trailing garbage is ignored
	info2, ok := parseJPEG(append(append([]byte{}, j...), 1, 2, 3))
	if !ok || info2.Length != len(j) {
		t.Fatalf("trailing garbage: %+v", info2)
	}
	// truncated stream is rejected
	if _, ok := parseJPEG(j[:len(j)-10]); ok {
		t.Fatal("truncated JPEG accepted")
	}
	// lossless SOF3 (DNG/CR2 sensor data) is parsed but not displayable
	ll := append([]byte{}, j...)
	for i := 2; i+1 < len(ll); i++ {
		if ll[i] == 0xFF && ll[i+1] == 0xC0 {
			ll[i+1] = 0xC3
			break
		}
	}
	info3, ok := parseJPEG(ll)
	if !ok || !info3.Lossless || info3.displayable() {
		t.Fatalf("lossless: ok=%v %+v", ok, info3)
	}
	if _, ok := parseJPEG(noise(5000, 1)); ok {
		t.Fatal("noise accepted as JPEG")
	}
}

func TestExtractSonyLikeARW(t *testing.T) {
	full := makeJPEG(t, 640, 426) // 0x0201 preview in IFD0
	thumb := makeJPEG(t, 160, 120)
	b := newBuilder()
	rawOff := b.put(noise(200_000, 2))
	fullOff := b.put(full)
	thumbOff := b.put(thumb)
	ifd1 := b.ifd([]tEntry{
		{tag: tagJPEGFormat, typ: 4, vals: []uint32{uint32(thumbOff)}},
		{tag: tagJPEGFormatLength, typ: 4, vals: []uint32{uint32(len(thumb))}},
	}, 0)
	ifd0 := b.ifd([]tEntry{
		{tag: tagNewSubfileType, typ: 4, vals: []uint32{0}},
		{tag: tagCompression, typ: 3, vals: []uint32{32767}},
		{tag: tagOrientation, typ: 3, vals: []uint32{6}},
		{tag: tagStripOffsets, typ: 4, vals: []uint32{uint32(rawOff)}},
		{tag: tagStripByteCounts, typ: 4, vals: []uint32{200_000}},
		{tag: tagJPEGFormat, typ: 4, vals: []uint32{uint32(fullOff)}},
		{tag: tagJPEGFormatLength, typ: 4, vals: []uint32{uint32(len(full))}},
	}, uint32(ifd1))
	b.setIFD0(ifd0)

	res, err := Extract(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 640 || res.Height != 426 || !bytes.Equal(res.JPEG, full) {
		t.Fatalf("wrong preview: %dx%d source=%s", res.Width, res.Height, res.Source)
	}
	if res.Orientation != 6 || res.Rotate() != 90 {
		t.Fatalf("orientation=%d rotate=%d", res.Orientation, res.Rotate())
	}
	if res.Candidates < 2 {
		t.Fatalf("expected thumb and preview candidates, got %d", res.Candidates)
	}
}

func TestExtractNikonLikeNEF(t *testing.T) {
	full := makeJPEG(t, 800, 532)   // SubIFD0 single-strip JPEG
	mnPrev := makeJPEG(t, 570, 375) // maker-note PreviewIFD JPEG
	thumb := makeJPEG(t, 160, 120)
	b := newBuilder()
	fullOff := b.put(full)
	rawOff := b.put(noise(300_000, 3))
	thumbOff := b.put(thumb)

	// Nikon maker note: "Nikon\0" + version(4) + embedded TIFF whose offsets are relative to it.
	mn := newBuilder()
	mnPrevOff := mn.put(mnPrev)
	prevIFD := mn.ifd([]tEntry{
		{tag: tagJPEGFormat, typ: 4, vals: []uint32{uint32(mnPrevOff)}},
		{tag: tagJPEGFormatLength, typ: 4, vals: []uint32{uint32(len(mnPrev))}},
	}, 0)
	mnIFD0 := mn.ifd([]tEntry{
		{tag: 0x0001, typ: 7, raw: []byte("0211")},
		{tag: tagNikonPreviewIFD, typ: 4, vals: []uint32{uint32(prevIFD)}},
	}, 0)
	mn.setIFD0(mnIFD0)
	makerNote := append([]byte("Nikon\x00\x02\x11\x00\x00"), mn.buf...)

	exifIFD := b.ifd([]tEntry{
		{tag: 0x829A, typ: 3, vals: []uint32{1, 250}},
		{tag: tagMakerNote, typ: 7, raw: makerNote},
	}, 0)
	sub0 := b.ifd([]tEntry{
		{tag: tagNewSubfileType, typ: 4, vals: []uint32{1}},
		{tag: tagCompression, typ: 3, vals: []uint32{6}},
		{tag: tagStripOffsets, typ: 4, vals: []uint32{uint32(fullOff)}},
		{tag: tagStripByteCounts, typ: 4, vals: []uint32{uint32(len(full))}},
	}, 0)
	sub1 := b.ifd([]tEntry{
		{tag: tagNewSubfileType, typ: 4, vals: []uint32{0}},
		{tag: tagCompression, typ: 3, vals: []uint32{34713}},
		{tag: tagStripOffsets, typ: 4, vals: []uint32{uint32(rawOff)}},
		{tag: tagStripByteCounts, typ: 4, vals: []uint32{300_000}},
	}, 0)
	ifd0 := b.ifd([]tEntry{
		{tag: tagNewSubfileType, typ: 4, vals: []uint32{1}},
		{tag: tagOrientation, typ: 3, vals: []uint32{8}},
		{tag: tagStripOffsets, typ: 4, vals: []uint32{uint32(thumbOff)}},
		{tag: tagStripByteCounts, typ: 4, vals: []uint32{uint32(len(thumb))}},
		{tag: tagSubIFDs, typ: 4, vals: []uint32{uint32(sub0), uint32(sub1)}},
		{tag: tagExifIFD, typ: 4, vals: []uint32{uint32(exifIFD)}},
	}, 0)
	b.setIFD0(ifd0)

	res, err := Extract(b.buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 800 || !bytes.Equal(res.JPEG, full) {
		t.Fatalf("wrong preview %dx%d from %s", res.Width, res.Height, res.Source)
	}
	if res.Rotate() != 270 {
		t.Fatalf("rotate=%d", res.Rotate())
	}
	if res.Candidates != 3 {
		t.Fatalf("candidates=%d want 3 (thumb, maker-note preview, full)", res.Candidates)
	}

	// Structured-only check: the maker-note preview must be reachable without the scan.
	c := &collector{seen: map[int]bool{}}
	tf, _ := newTIFF(b.buf)
	tf.walk(tf.firstIFD(), 0, map[int]bool{}, c)
	foundMN := false
	for _, cand := range c.cands {
		if info, ok := parseJPEG(b.buf[cand.off:]); ok && info.Width == 570 {
			foundMN = true
		}
	}
	if !foundMN {
		t.Fatal("Nikon maker-note preview not found by the structured walk")
	}
}

func TestExtractCanonLikeCR2AndInlineRW2(t *testing.T) {
	full := makeJPEG(t, 720, 480)
	b := newBuilder()
	fullOff := b.put(full)
	rawOff := b.put(noise(100_000, 4))
	ifd0 := b.ifd([]tEntry{
		{tag: tagCompression, typ: 3, vals: []uint32{6}},
		{tag: tagStripOffsets, typ: 4, vals: []uint32{uint32(fullOff)}},
		{tag: tagStripByteCounts, typ: 4, vals: []uint32{uint32(len(full))}},
	}, 0)
	_ = rawOff
	b.setIFD0(ifd0)
	res, err := Extract(b.buf)
	if err != nil || res.Width != 720 {
		t.Fatalf("CR2-like: %v %+v", err, res)
	}

	// Panasonic-style: JPEG stored inline as the value of an UNDEFINED tag.
	inline := makeJPEG(t, 400, 300)
	b2 := newBuilder()
	b2.buf = append(b2.buf[:2], 'U', 0x00, 0, 0, 0, 0) // IIU\0 magic
	b2.put(noise(50_000, 5))
	ifd := b2.ifd([]tEntry{{tag: tagPanasonicJPG, typ: 7, raw: inline}}, 0)
	b2.setIFD0(ifd)
	res2, err := Extract(b2.buf)
	if err != nil || res2.Width != 400 || res2.Source != "inline-tag" {
		t.Fatalf("RW2-like: %v %+v", err, res2)
	}
}

func TestExtractRAFAndCR3(t *testing.T) {
	full := makeJPEG(t, 600, 400)
	raf := make([]byte, 160)
	copy(raf, "FUJIFILMCCD-RAW 0201FF393103FinePix X100   ")
	off := len(raf) + 1000
	binary.BigEndian.PutUint32(raf[84:], uint32(off))
	binary.BigEndian.PutUint32(raf[88:], uint32(len(full)))
	raf = append(raf, noise(1000, 6)...)
	raf = append(raf, full...)
	raf = append(raf, noise(20_000, 7)...)
	res, err := Extract(raf)
	if err != nil || res.Width != 600 {
		t.Fatalf("RAF: %v %+v", err, res)
	}

	// CR3: ISO-BMFF with a CMT1 TIFF carrying Orientation, JPEG later in mdat.
	cmt := newBuilder()
	cmt.setIFD0(cmt.ifd([]tEntry{{tag: tagOrientation, typ: 3, vals: []uint32{3}}}, 0))
	var cr3 []byte
	cr3 = append(cr3, 0, 0, 0, 0x18)
	cr3 = append(cr3, []byte("ftypcrx ")...)
	cr3 = append(cr3, make([]byte, 12)...)
	box := append([]byte{0, 0, 0, 0}, []byte("CMT1")...)
	box = append(box, cmt.buf...)
	binary.BigEndian.PutUint32(box, uint32(len(box)))
	cr3 = append(cr3, box...)
	cr3 = append(cr3, noise(30_000, 8)...)
	cr3 = append(cr3, makeJPEG(t, 160, 120)...) // THMB
	cr3 = append(cr3, noise(3_000, 9)...)
	cr3 = append(cr3, full...) // full-size JPEG track
	cr3 = append(cr3, noise(50_000, 10)...)
	res, err = Extract(cr3)
	if err != nil || res.Width != 600 || res.Orientation != 3 || res.Rotate() != 180 {
		t.Fatalf("CR3: %v %+v", err, res)
	}
}

func TestOwnExifOrientationIsUsedAndStripped(t *testing.T) {
	j := makeJPEG(t, 300, 200)
	// Build APP1 Exif with Orientation=6.
	ex := newBuilder()
	ex.setIFD0(ex.ifd([]tEntry{{tag: tagOrientation, typ: 3, vals: []uint32{6}}}, 0))
	payload := append([]byte("Exif\x00\x00"), ex.buf...)
	app1 := append([]byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}, payload...)
	withExif := append([]byte{0xFF, 0xD8}, app1...)
	withExif = append(withExif, j[2:]...)
	if got := exifOrientation(withExif); got != 6 {
		t.Fatalf("exifOrientation=%d", got)
	}
	data := append(noise(10_000, 11), withExif...)
	res, err := Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Orientation != 6 {
		t.Fatalf("orientation=%d", res.Orientation)
	}
	if exifOrientation(res.JPEG) != 0 || !bytes.Equal(res.JPEG, j) {
		t.Fatal("EXIF not stripped from output")
	}
}

func TestNoPreview(t *testing.T) {
	if _, err := Extract(noise(100_000, 12)); err != ErrNoPreview {
		t.Fatalf("err=%v", err)
	}
	if _, err := Extract([]byte{}); err != ErrNoPreview {
		t.Fatalf("empty: err=%v", err)
	}
}

func TestCached(t *testing.T) {
	dir := t.TempDir()
	full := makeJPEG(t, 500, 300)
	b := newBuilder()
	fullOff := b.put(full)
	b.setIFD0(b.ifd([]tEntry{
		{tag: tagOrientation, typ: 3, vals: []uint32{8}},
		{tag: tagJPEGFormat, typ: 4, vals: []uint32{uint32(fullOff)}},
		{tag: tagJPEGFormatLength, typ: 4, vals: []uint32{uint32(len(full))}},
	}, 0))
	raw := filepath.Join(dir, "DSC00001.ARW")
	if err := os.WriteFile(raw, b.buf, 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	p1, rot, err := Cached(cache, raw)
	if err != nil || rot != 270 {
		t.Fatalf("Cached: %v rot=%d", err, rot)
	}
	got, _ := os.ReadFile(p1)
	if !bytes.Equal(got, full) {
		t.Fatal("cached JPEG differs")
	}
	// Second call hits the cache (same path, same rotation) even if the RAW is now unreadable.
	os.Chmod(raw, 0o000)
	defer os.Chmod(raw, 0o644)
	p2, rot2, err := Cached(cache, raw)
	if err != nil || p2 != p1 || rot2 != 270 {
		t.Fatalf("cache miss: %v %s %d", err, p2, rot2)
	}
}

func BenchmarkExtract40MB(b *testing.B) {
	full := makeJPEGb(b, 1600, 1066)
	data := noise(40<<20, 99)
	copy(data[20<<20:], full)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Extract(data); err != nil {
			b.Fatal(err)
		}
	}
}

func makeJPEGb(b *testing.B, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}
