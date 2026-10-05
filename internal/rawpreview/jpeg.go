package rawpreview

import (
	"bytes"
	"encoding/binary"
)

// jpegInfo describes a JPEG stream found inside a RAW file.
type jpegInfo struct {
	Width, Height int
	Precision     int  // bits per sample (8 for displayable previews)
	Components    int  // 1 = grey, 3 = YCbCr
	Lossless      bool // SOF3/7/11/15 - raw sensor data in DNG/CR2, never a preview
	Length        int  // bytes from SOI through EOI
}

// displayable reports whether a decoded JPEG is something mpv/FFmpeg can show
// as a picture (as opposed to lossless CFA sensor data or 12/16-bit tiles).
func (j jpegInfo) displayable() bool {
	return j.Width > 0 && j.Height > 0 && !j.Lossless && j.Precision == 8 && (j.Components == 3 || j.Components == 1)
}

func (j jpegInfo) pixels() int { return j.Width * j.Height }

func isSOF(m byte) bool {
	switch m {
	case 0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
		return true
	}
	return false
}

// parseJPEG walks the marker structure of a JPEG starting at b[0] (which must
// be SOI) and returns its dimensions and total length. It is tolerant of
// trailing garbage (it stops at EOI) but strict about structure, so that a
// random FF D8 FF inside sensor data is rejected quickly.
func parseJPEG(b []byte) (jpegInfo, bool) {
	var info jpegInfo
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 || b[2] != 0xFF {
		return info, false
	}
	pos := 2
	sawSOF := false
	for {
		if pos >= len(b) || b[pos] != 0xFF {
			return info, false
		}
		for pos < len(b) && b[pos] == 0xFF { // fill bytes
			pos++
		}
		if pos >= len(b) {
			return info, false
		}
		m := b[pos]
		pos++
		switch {
		case m == 0xD9: // EOI
			info.Length = pos
			return info, sawSOF
		case m == 0x01 || (m >= 0xD0 && m <= 0xD7): // TEM / RSTn: standalone
			continue
		case m == 0xD8 || m == 0x00: // nested SOI or stuffed byte where a marker should be
			return info, false
		}
		if pos+2 > len(b) {
			return info, false
		}
		segLen := int(binary.BigEndian.Uint16(b[pos:]))
		if segLen < 2 || pos+segLen > len(b) {
			return info, false
		}
		seg := b[pos+2 : pos+segLen]
		if isSOF(m) {
			if len(seg) < 6 {
				return info, false
			}
			info.Precision = int(seg[0])
			info.Height = int(binary.BigEndian.Uint16(seg[1:]))
			info.Width = int(binary.BigEndian.Uint16(seg[3:]))
			info.Components = int(seg[5])
			info.Lossless = m == 0xC3 || m == 0xC7 || m == 0xCB || m == 0xCF
			if info.Width == 0 || info.Height == 0 { // DNL-defined height: not a preview
				return info, false
			}
			sawSOF = true
		}
		pos += segLen
		if m == 0xDA { // SOS: skip entropy-coded data up to the next real marker
			if !sawSOF {
				return info, false
			}
			for {
				idx := bytes.IndexByte(b[pos:], 0xFF)
				if idx < 0 {
					return info, false
				}
				pos += idx
				if pos+1 >= len(b) {
					return info, false
				}
				nx := b[pos+1]
				if nx == 0x00 || (nx >= 0xD0 && nx <= 0xD7) {
					pos += 2
					continue
				}
				if nx == 0xFF {
					pos++
					continue
				}
				break
			}
		}
	}
}

// exifOrientation returns the EXIF Orientation (1-8) stored in a JPEG's APP1
// segment, or 0 when absent.
func exifOrientation(jpg []byte) int {
	if len(jpg) < 4 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		return 0
	}
	pos := 2
	for pos+4 <= len(jpg) && jpg[pos] == 0xFF {
		m := jpg[pos+1]
		if m == 0xDA || m == 0xD9 {
			return 0
		}
		if m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			pos += 2
			continue
		}
		segLen := int(binary.BigEndian.Uint16(jpg[pos+2:]))
		if segLen < 2 || pos+2+segLen > len(jpg) {
			return 0
		}
		if m == 0xE1 && segLen > 8 && bytes.HasPrefix(jpg[pos+4:], []byte("Exif\x00\x00")) {
			t, ok := newTIFF(jpg[pos+10 : pos+2+segLen])
			if !ok {
				return 0
			}
			return t.orientationIFD0()
		}
		pos += 2 + segLen
	}
	return 0
}

// stripExif returns a copy of jpg without its APP1 Exif segment, so that the
// viewer does not apply a second rotation on top of the one PlayAnything
// derives from the RAW container.
func stripExif(jpg []byte) []byte {
	if len(jpg) < 4 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		return jpg
	}
	pos := 2
	for pos+4 <= len(jpg) && jpg[pos] == 0xFF {
		m := jpg[pos+1]
		if m == 0xDA || m == 0xD9 {
			return jpg
		}
		if m == 0xD8 || m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			pos += 2
			continue
		}
		segLen := int(binary.BigEndian.Uint16(jpg[pos+2:]))
		if segLen < 2 || pos+2+segLen > len(jpg) {
			return jpg
		}
		if m == 0xE1 && bytes.HasPrefix(jpg[pos+4:], []byte("Exif\x00\x00")) {
			out := make([]byte, 0, len(jpg)-(2+segLen))
			out = append(out, jpg[:pos]...)
			out = append(out, jpg[pos+2+segLen:]...)
			return out
		}
		pos += 2 + segLen
	}
	return jpg
}
