// Command mkicon builds the Windows icon for PlayAnything.
//
//	go run ./tools/mkicon -in assets/icon/source.png -out assets/icon/playanything.ico
//
// It reads a square PNG (the Anchor Point Studio icon logo goes in
// assets/icon/source.png) and writes a multi-size .ico with PNG-compressed
// entries (Vista+), which is what Explorer, the taskbar and the Alt-Tab switcher
// expect. With -placeholder it draws a stand-in icon instead of reading a file.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

var sizes = []int{256, 128, 64, 48, 32, 24, 16}

func main() {
	in := flag.String("in", "", "source PNG (square, ideally 512px or larger)")
	out := flag.String("out", "assets/icon/playanything.ico", "output .ico")
	pngOut := flag.String("png", "", "also write a 256px PNG here (for docs/.desktop/macOS)")
	placeholder := flag.Bool("placeholder", false, "draw a placeholder icon instead of reading -in")
	flag.Parse()

	var src image.Image
	if *placeholder || *in == "" {
		src = drawPlaceholder(512)
	} else {
		f, err := os.Open(*in)
		if err != nil {
			fatal(err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			fatal(fmt.Errorf("%s: %w (PNG expected)", *in, err))
		}
		src = squareWithPadding(img)
	}
	ico, err := encodeICO(src)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, ico, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes, %d sizes)\n", *out, len(ico), len(sizes))
	if *pngOut != "" {
		var buf bytes.Buffer
		if err := png.Encode(&buf, resize(src, 256)); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*pngOut, buf.Bytes(), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s\n", *pngOut)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mkicon:", err)
	os.Exit(1)
}

// encodeICO writes an ICONDIR with one PNG-compressed entry per size.
func encodeICO(src image.Image) ([]byte, error) {
	type entry struct {
		size int
		data []byte
	}
	var entries []entry
	for _, s := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, resize(src, s)); err != nil {
			return nil, err
		}
		entries = append(entries, entry{s, buf.Bytes()})
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&out, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		dim := uint8(e.size)
		if e.size >= 256 {
			dim = 0 // 0 means 256
		}
		out.WriteByte(dim)
		out.WriteByte(dim)
		out.WriteByte(0)                                    // palette
		out.WriteByte(0)                                    // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&out, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	return out.Bytes(), nil
}

// squareWithPadding centres a non-square image on a transparent square.
func squareWithPadding(img image.Image) image.Image {
	b := img.Bounds()
	if b.Dx() == b.Dy() {
		return img
	}
	side := b.Dx()
	if b.Dy() > side {
		side = b.Dy()
	}
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	off := image.Pt((side-b.Dx())/2, (side-b.Dy())/2)
	draw.Draw(dst, b.Sub(b.Min).Add(off), img, b.Min, draw.Over)
	return dst
}

// resize does a box-filter downscale (or bilinear upscale) to size×size.
func resize(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sw, sh := float64(b.Dx()), float64(b.Dy())
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			x0 := float64(x) * sw / float64(size)
			x1 := float64(x+1) * sw / float64(size)
			y0 := float64(y) * sh / float64(size)
			y1 := float64(y+1) * sh / float64(size)
			var r, g, bl, a, n float64
			for sy := int(y0); sy < int(math.Ceil(y1)) && sy < b.Dy(); sy++ {
				for sx := int(x0); sx < int(math.Ceil(x1)) && sx < b.Dx(); sx++ {
					c := color.NRGBAModel.Convert(src.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					w := float64(c.A) / 255
					r += float64(c.R) * w
					g += float64(c.G) * w
					bl += float64(c.B) * w
					a += float64(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			if a > 0 {
				dst.SetNRGBA(x, y, color.NRGBA{uint8(r / (a / 255)), uint8(g / (a / 255)), uint8(bl / (a / 255)), uint8(a / n)})
			}
		}
	}
	return dst
}

// drawPlaceholder renders the interim PlayAnything mark: rounded dark tile,
// green ring, white play triangle.
func drawPlaceholder(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	cx, cy := s/2, s/2
	radius := s * 0.205 // corner radius of the tile
	inset := s * 0.0625
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !insideRoundedRect(fx, fy, inset, inset, s-inset, s-inset, radius) {
				continue
			}
			t := (fx + fy) / (2 * s)
			c := color.NRGBA{uint8(30 - 19*t), uint8(42 - 24*t), uint8(58 - 26*t), 255}
			d := math.Hypot(fx-cx, fy-cy)
			ring := s * 0.32
			if math.Abs(d-ring) < s*0.02 {
				c = color.NRGBA{0x3d, 0xdc, 0x97, 255}
			}
			// play triangle
			ax, ay := cx-s*0.085, cy-s*0.165
			bx, by := cx+s*0.165, cy
			ex, ey := cx-s*0.085, cy+s*0.165
			if pointInTriangle(fx, fy, ax, ay, bx, by, ex, ey) {
				c = color.NRGBA{255, 255, 255, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func insideRoundedRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return math.Hypot(x-cx, y-cy) <= r
}

func pointInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := (px-bx)*(ay-by) - (ax-bx)*(py-by)
	d2 := (px-cx)*(by-cy) - (bx-cx)*(py-cy)
	d3 := (px-ax)*(cy-ay) - (cx-ax)*(py-ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}
