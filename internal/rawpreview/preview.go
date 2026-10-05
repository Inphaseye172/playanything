// Package rawpreview extracts the camera-rendered JPEG preview embedded in
// camera RAW files (Sony ARW, Canon CR2/CR3/CRW, Nikon NEF/NRW, Adobe DNG,
// Fujifilm RAF, Olympus ORF, Panasonic RW2, Pentax PEF, Hasselblad 3FR,
// Phase One IIQ, Sigma X3F and others).
//
// Every RAW camera stores at least one JPEG rendition of the shot inside the
// file; most store a full-resolution one. Showing that JPEG is how Finder,
// Explorer, Lightroom's import grid and the camera's own LCD "preview" RAW
// files, and it takes milliseconds instead of the seconds a full demosaic
// takes. PlayAnything uses it for one-click viewing and leaves true RAW
// development to the optional external decoders (see package app).
//
// Strategy: parse the TIFF/EXIF structure (fast, exact) and in addition
// brute-force scan the file for complete JPEG streams (format-agnostic),
// then pick the largest displayable image.
package rawpreview

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// MaxFileSize bounds how much of a RAW file is read into memory.
// Medium-format RAWs are ~100-250 MB; anything bigger is not a photo.
const MaxFileSize = 1 << 30

// Result is an extracted preview.
type Result struct {
	JPEG        []byte
	Width       int
	Height      int
	Orientation int    // EXIF orientation 1-8 (0 = unknown)
	Source      string // where the preview was found (debugging / `info`)
	Candidates  int    // how many JPEGs were considered
}

// Rotate returns the clockwise rotation in degrees mpv must apply
// (--video-rotate) so the preview displays upright.
func (r Result) Rotate() int { return RotationFor(r.Orientation) }

// RotationFor maps an EXIF orientation value to clockwise degrees.
func RotationFor(orientation int) int {
	switch orientation {
	case 3, 4:
		return 180
	case 6, 7:
		return 90
	case 5, 8:
		return 270
	}
	return 0
}

var ErrNoPreview = errors.New("no embedded JPEG preview found")

// ExtractFile reads path and extracts its best preview.
func ExtractFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() > MaxFileSize {
		return nil, fmt.Errorf("file is %d MB, larger than the %d MB limit for RAW photos", fi.Size()>>20, MaxFileSize>>20)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return Extract(data)
}

// Extract finds the largest displayable JPEG inside a RAW file held in memory.
func Extract(data []byte) (*Result, error) {
	c := &collector{seen: map[int]bool{}}

	// 1. Structured: TIFF-based RAW (ARW, NEF, CR2, DNG, ORF, RW2, PEF, SRW, 3FR, IIQ, MEF, MOS, ERF, KDC, DCR).
	if t, ok := newTIFF(data); ok {
		t.walk(t.firstIFD(), 0, map[int]bool{}, c)
	}
	// 2. Fujifilm RAF header.
	rafPreview(data, c)
	// 3. Canon CR3 orientation (the JPEG itself is found by the scan below).
	if c.orientation == 0 {
		c.orientation = cr3Orientation(data)
	}
	// 4. Format-agnostic scan: catches CR3, CRW, X3F, MRW, maker-note previews and anything the parsers missed.
	scanJPEGs(data, c)

	type scored struct {
		candidate
		info jpegInfo
	}
	var found []scored
	for _, cand := range c.cands {
		if cand.off >= len(data) {
			continue
		}
		info, ok := parseJPEG(data[cand.off:])
		if !ok || !info.displayable() {
			continue
		}
		found = append(found, scored{cand, info})
	}
	if len(found) == 0 {
		return nil, ErrNoPreview
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].info.pixels() != found[j].info.pixels() {
			return found[i].info.pixels() > found[j].info.pixels()
		}
		return found[i].info.Length > found[j].info.Length
	})
	best := found[0]
	jpg := data[best.off : best.off+best.info.Length]
	orientation := c.orientation
	if own := exifOrientation(jpg); own != 0 {
		// The preview carries its own EXIF orientation. Trust it when the
		// container had none, then strip it so the viewer does not rotate twice.
		if orientation == 0 {
			orientation = own
		}
		jpg = stripExif(jpg)
	}
	out := make([]byte, len(jpg))
	copy(out, jpg)
	return &Result{
		JPEG:        out,
		Width:       best.info.Width,
		Height:      best.info.Height,
		Orientation: orientation,
		Source:      best.source,
		Candidates:  len(found),
	}, nil
}
