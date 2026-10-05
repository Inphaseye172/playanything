package rawpreview

import (
	"encoding/binary"
)

// TIFF tags used while hunting for previews.
const (
	tagNewSubfileType   = 0x00FE
	tagCompression      = 0x0103
	tagStripOffsets     = 0x0111
	tagOrientation      = 0x0112
	tagStripByteCounts  = 0x0117
	tagSubIFDs          = 0x014A
	tagJPEGFormat       = 0x0201 // JPEGInterchangeFormat
	tagJPEGFormatLength = 0x0202 // JPEGInterchangeFormatLength
	tagExifIFD          = 0x8769
	tagMakerNote        = 0x927C
	tagNikonPreviewIFD  = 0x0011 // inside a Nikon maker note
	tagPanasonicJPG     = 0x002E // JpgFromRaw: whole JPEG inline in RW2 IFD0
)

var typeSize = map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8, 13: 4}

// tiff is a minimal TIFF/EXIF structure reader. base is the position of the
// TIFF header inside data; all offsets in the structure are relative to it
// (this is how maker notes embed their own TIFF).
type tiff struct {
	data  []byte
	bo    binary.ByteOrder
	base  int
	nikon bool // inside a Nikon maker note: tag 0x0011 is a preview IFD pointer
}

type entry struct {
	tag, typ uint16
	count    uint32
	dataOff  int // absolute offset of the value data in t.data
}

func (e entry) size() int { return typeSize[e.typ] * int(e.count) }

// newTIFF validates a TIFF header at data[0] and accepts the byte-order
// variants used by Olympus (IIRO/MMOR/IIRS) and Panasonic (IIU\0) RAW files.
func newTIFF(data []byte) (*tiff, bool) {
	return newTIFFAt(data, 0)
}

func newTIFFAt(data []byte, base int) (*tiff, bool) {
	if base < 0 || base+8 > len(data) {
		return nil, false
	}
	t := &tiff{data: data, base: base}
	switch string(data[base : base+2]) {
	case "II":
		t.bo = binary.LittleEndian
	case "MM":
		t.bo = binary.BigEndian
	default:
		return nil, false
	}
	return t, true
}

func (t *tiff) firstIFD() int {
	if t.base+8 > len(t.data) {
		return 0
	}
	return int(t.bo.Uint32(t.data[t.base+4:]))
}

// readIFD parses the IFD at (base-relative) off. Returns entries and the
// base-relative offset of the next IFD (0 when none).
func (t *tiff) readIFD(off int) ([]entry, int, bool) {
	p := t.base + off
	if off <= 0 || p+2 > len(t.data) {
		return nil, 0, false
	}
	n := int(t.bo.Uint16(t.data[p:]))
	if n == 0 || n > 4096 || p+2+n*12+4 > len(t.data) {
		return nil, 0, false
	}
	p += 2
	entries := make([]entry, 0, n)
	for i := 0; i < n; i++ {
		e := entry{
			tag:   t.bo.Uint16(t.data[p:]),
			typ:   t.bo.Uint16(t.data[p+2:]),
			count: t.bo.Uint32(t.data[p+4:]),
		}
		if typeSize[e.typ] == 0 { // unknown type: skip but keep walking
			p += 12
			continue
		}
		if e.size() <= 4 {
			e.dataOff = p + 8
		} else {
			e.dataOff = t.base + int(t.bo.Uint32(t.data[p+8:]))
		}
		if e.dataOff >= 0 && e.dataOff+e.size() <= len(t.data) {
			entries = append(entries, e)
		}
		p += 12
	}
	next := int(t.bo.Uint32(t.data[p:]))
	return entries, next, true
}

// uints decodes SHORT/LONG/IFD values.
func (t *tiff) uints(e entry) []uint32 {
	out := make([]uint32, 0, e.count)
	for i := 0; i < int(e.count); i++ {
		switch e.typ {
		case 3, 8:
			out = append(out, uint32(t.bo.Uint16(t.data[e.dataOff+i*2:])))
		case 4, 9, 13:
			out = append(out, t.bo.Uint32(t.data[e.dataOff+i*4:]))
		default:
			return nil
		}
	}
	return out
}

func (t *tiff) bytes(e entry) []byte { return t.data[e.dataOff : e.dataOff+e.size()] }

func find(entries []entry, tag uint16) (entry, bool) {
	for _, e := range entries {
		if e.tag == tag {
			return e, true
		}
	}
	return entry{}, false
}

// orientationIFD0 returns the Orientation tag of IFD0, or 0.
func (t *tiff) orientationIFD0() int {
	entries, _, ok := t.readIFD(t.firstIFD())
	if !ok {
		return 0
	}
	if e, ok := find(entries, tagOrientation); ok {
		if v := t.uints(e); len(v) == 1 && v[0] >= 1 && v[0] <= 8 {
			return int(v[0])
		}
	}
	return 0
}

// collector gathers preview candidates found while walking the structure.
type collector struct {
	cands       []candidate
	seen        map[int]bool
	orientation int
}

type candidate struct {
	off    int // absolute offset of SOI in the file
	source string
}

func (c *collector) add(off int, source string) {
	if off <= 0 || c.seen[off] {
		return
	}
	c.seen[off] = true
	c.cands = append(c.cands, candidate{off: off, source: source})
}

// walk visits the IFD chain starting at base-relative off, recursing into
// SubIFDs, the EXIF IFD and Nikon maker notes.
func (t *tiff) walk(off int, depth int, visited map[int]bool, c *collector) {
	for off != 0 {
		abs := t.base + off
		if depth > 8 || visited[abs] || abs <= 0 || abs >= len(t.data) {
			return
		}
		visited[abs] = true
		entries, next, ok := t.readIFD(off)
		if !ok {
			return
		}
		t.collect(entries, depth, c)
		if e, ok := find(entries, tagSubIFDs); ok {
			for _, sub := range t.uints(e) {
				t.walk(int(sub), depth+1, visited, c)
			}
		}
		if e, ok := find(entries, tagExifIFD); ok {
			if v := t.uints(e); len(v) == 1 {
				t.walk(int(v[0]), depth+1, visited, c)
			}
		}
		if t.nikon {
			if e, ok := find(entries, tagNikonPreviewIFD); ok {
				if v := t.uints(e); len(v) == 1 {
					t.walk(int(v[0]), depth+1, visited, c)
				}
			}
		}
		if e, ok := find(entries, tagMakerNote); ok && !t.nikon {
			t.makerNote(e, depth+1, visited, c)
		}
		off = next
	}
}

func (t *tiff) collect(entries []entry, depth int, c *collector) {
	if c.orientation == 0 && depth == 0 {
		if e, ok := find(entries, tagOrientation); ok {
			if v := t.uints(e); len(v) == 1 && v[0] >= 1 && v[0] <= 8 {
				c.orientation = int(v[0])
			}
		}
	}
	// JPEGInterchangeFormat: classic EXIF thumbnail/preview pointer (Sony ARW, IFD1 thumbs, Nikon preview IFD).
	if e, ok := find(entries, tagJPEGFormat); ok {
		if v := t.uints(e); len(v) == 1 {
			c.add(t.base+int(v[0]), "JPEGInterchangeFormat")
		}
	}
	// Single-strip JPEG image IFDs: Canon CR2 IFD0, Nikon NEF SubIFD0, DNG preview IFDs.
	if so, ok := find(entries, tagStripOffsets); ok {
		if offs := t.uints(so); len(offs) == 1 {
			c.add(t.base+int(offs[0]), "StripOffsets")
		}
	}
	// Whole JPEGs stored inline as UNDEFINED/BYTE data (Panasonic RW2 JpgFromRaw and friends).
	for _, e := range entries {
		if (e.typ == 7 || e.typ == 1) && e.count > 1024 && e.tag != tagMakerNote {
			b := t.bytes(e)
			if len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
				c.add(e.dataOff, "inline-tag")
			}
		}
	}
}

// makerNote recognises the Nikon maker note, which embeds its own TIFF with a
// PreviewIFD pointing at a large JPEG. Other makers are covered by the
// brute-force scan in scan.go.
func (t *tiff) makerNote(e entry, depth int, visited map[int]bool, c *collector) {
	b := t.bytes(e)
	if len(b) > 18 && string(b[:6]) == "Nikon\x00" {
		sub, ok := newTIFFAt(t.data, e.dataOff+10)
		if !ok {
			return
		}
		sub.nikon = true
		sub.walk(sub.firstIFD(), depth, visited, c)
	}
}
