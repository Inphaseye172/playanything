package rawpreview

import "bytes"

var soi = []byte{0xFF, 0xD8, 0xFF}

// plausibleAfterSOI reports whether the marker following SOI is one a real
// camera JPEG starts with. This rejects most random FF D8 FF triplets inside
// sensor data before the (slower) structural parse runs.
func plausibleAfterSOI(m byte) bool {
	switch {
	case m >= 0xE0 && m <= 0xEF: // APPn (JFIF, Exif, Adobe, ...)
		return true
	case m == 0xDB, m == 0xC4, m == 0xDD, m == 0xFE: // DQT, DHT, DRI, COM
		return true
	case isSOF(m):
		return true
	}
	return false
}

// scanJPEGs brute-force scans data for complete, displayable JPEG streams and
// adds them to the collector. This is what makes CR3, RAF, ORF, RW2, PEF, X3F
// and every other maker-specific layout work without format-specific code: a
// camera's embedded preview is always a complete baseline/progressive JPEG.
func scanJPEGs(data []byte, c *collector) {
	pos := 0
	for pos < len(data) {
		idx := bytes.Index(data[pos:], soi)
		if idx < 0 {
			return
		}
		start := pos + idx
		if start+3 < len(data) && plausibleAfterSOI(data[start+3]) {
			if info, ok := parseJPEG(data[start:]); ok {
				if info.displayable() {
					c.add(start, "scan")
				}
				pos = start + info.Length
				continue
			}
		}
		pos = start + 1
	}
}
