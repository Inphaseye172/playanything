package rawpreview

import "encoding/binary"

// rafPreview handles Fujifilm RAF: a fixed header whose bytes 84..91 hold the
// big-endian offset and length of the embedded full-size JPEG.
func rafPreview(data []byte, c *collector) bool {
	if len(data) < 92 || string(data[:15]) != "FUJIFILMCCD-RAW" {
		return false
	}
	off := int(binary.BigEndian.Uint32(data[84:]))
	if off > 0 && off < len(data) {
		c.add(off, "RAF-header")
	}
	return true
}
