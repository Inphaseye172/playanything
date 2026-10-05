package rawpreview

import "bytes"

// cr3Orientation finds the Canon CR3 "CMT1" box (IFD0 of the EXIF data stored
// as a plain TIFF) and returns its Orientation tag. The box lives inside the
// Canon uuid box near the start of the file, so only the head is searched.
func cr3Orientation(data []byte) int {
	if len(data) < 16 || string(data[4:12]) != "ftypcrx " {
		return 0
	}
	limit := len(data)
	if limit > 1<<20 {
		limit = 1 << 20
	}
	idx := bytes.Index(data[:limit], []byte("CMT1"))
	if idx < 0 || idx+4+8 > len(data) {
		return 0
	}
	t, ok := newTIFFAt(data, idx+4)
	if !ok {
		return 0
	}
	return t.orientationIFD0()
}
