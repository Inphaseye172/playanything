//go:build windows

package winui

import (
	"fmt"
	"unsafe"

	"github.com/inphaseye172/playanything/internal/player"
)

// Colours (COLORREF is 0x00BBGGRR).
var (
	colBar     = rgb(0x15, 0x18, 0x1e)
	colTrack   = rgb(0x3a, 0x40, 0x4a)
	colAccent  = rgb(0x3d, 0xdc, 0x97)
	colText    = rgb(0xe6, 0xe9, 0xee)
	colDim     = rgb(0x9a, 0xa3, 0xaf)
	colHover   = rgb(0x24, 0x29, 0x32)
	colBadgeBg = rgb(0x3d, 0xdc, 0x97)
)

// bar is the custom-drawn control strip under the video.
type bar struct {
	rect     rect // in main-window client coordinates
	dpi      int32
	font     uintptr
	fontBold uintptr

	// element rects (client coordinates)
	play, time, seek, volIcon, vol, audio, subs, fs rect

	dragSeek bool
	dragVol  bool
	hover    int // element under the mouse (elem* constants)
	lastSeek int64
	status   string // transient message (errors, download notes)
}

const (
	elemNone = iota
	elemPlay
	elemSeek
	elemVolIcon
	elemVol
	elemAudio
	elemSubs
	elemFS
)

func (b *bar) height() int32 { return 48 * b.dpi / 96 }

func (b *bar) scale(px int32) int32 { return px * b.dpi / 96 }

func (b *bar) setDPI(dpi int32) {
	if b.font != 0 {
		pDeleteObject.Call(b.font)
		pDeleteObject.Call(b.fontBold)
	}
	b.dpi = dpi
	h := -(13 * dpi / 96)
	face := utf16("Segoe UI")
	f, _, _ := pCreateFontW.Call(uintptr(h), 0, 0, 0, fwNormal, 0, 0, 0, defaultCharset, 0, 0, cleartypeQual, 0, uintptr(unsafe.Pointer(face)))
	fb, _, _ := pCreateFontW.Call(uintptr(h), 0, 0, 0, fwSemibold, 0, 0, 0, defaultCharset, 0, 0, cleartypeQual, 0, uintptr(unsafe.Pointer(face)))
	b.font, b.fontBold = f, fb
}

// layout positions the elements inside r.
func (b *bar) layout(r rect) {
	b.rect = r
	s := b.scale
	x := r.Left + s(8)
	cy0, cy1 := r.Top+s(8), r.Bottom-s(8)
	b.play = rect{x, cy0, x + s(36), cy1}
	x += s(44)
	b.time = rect{x, cy0, x + s(118), cy1}
	x += s(122)
	// right side, laid out from the right edge
	rx := r.Right - s(8)
	b.fs = rect{rx - s(36), cy0, rx, cy1}
	rx -= s(44)
	b.subs = rect{rx - s(34), cy0, rx, cy1}
	rx -= s(40)
	b.audio = rect{rx - s(34), cy0, rx, cy1}
	rx -= s(46)
	b.vol = rect{rx - s(80), cy0, rx, cy1}
	rx -= s(84)
	b.volIcon = rect{rx - s(28), cy0, rx, cy1}
	rx -= s(40)
	if rx-x < s(60) {
		rx = x + s(60)
	}
	b.seek = rect{x, cy0, rx, cy1}
}

func (b *bar) hit(x, y int32) int {
	switch {
	case b.play.contains(x, y):
		return elemPlay
	case b.seek.contains(x, y):
		return elemSeek
	case b.volIcon.contains(x, y):
		return elemVolIcon
	case b.vol.contains(x, y):
		return elemVol
	case b.audio.contains(x, y):
		return elemAudio
	case b.subs.contains(x, y):
		return elemSubs
	case b.fs.contains(x, y):
		return elemFS
	}
	return elemNone
}

func fraction(r rect, x int32) float64 {
	if r.width() <= 0 {
		return 0
	}
	f := float64(x-r.Left) / float64(r.width())
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return f
}

// paint draws the bar into hdc (already double-buffered by the caller).
func (b *bar) paint(hdc uintptr, st player.State, audioCount, subCount int) {
	fill(hdc, b.rect, colBar)
	s := b.scale

	// play / pause
	b.button(hdc, b.play, elemPlay)
	if st.Paused || st.Idle {
		pcx, pcy := center(b.play)
		triangle(hdc, pcx, pcy, s(7), colText)
	} else {
		cx, cy := center(b.play)
		fill(hdc, rect{cx - s(6), cy - s(7), cx - s(2), cy + s(7)}, colText)
		fill(hdc, rect{cx + s(2), cy - s(7), cx + s(6), cy + s(7)}, colText)
	}

	// time
	var label string
	if st.Idle {
		label = "No file"
	} else if st.IsImage {
		label = fmt.Sprintf("%dx%d", st.Width, st.Height)
	} else if st.Duration > 0 {
		label = player.FormatTime(st.TimePos) + " / " + player.FormatTime(st.Duration)
	} else {
		label = player.FormatTime(st.TimePos)
	}
	if b.status != "" {
		label = b.status
	}
	text(hdc, b.time, label, b.font, colText, dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis|dtNoPrefix)

	// seek bar
	cy := (b.seek.Top + b.seek.Bottom) / 2
	fill(hdc, rect{b.seek.Left, cy - s(2), b.seek.Right, cy + s(2)}, colTrack)
	if st.Duration > 0 && !st.IsImage {
		f := st.TimePos / st.Duration
		if f > 1 {
			f = 1
		}
		px := b.seek.Left + int32(f*float64(b.seek.width()))
		fill(hdc, rect{b.seek.Left, cy - s(2), px, cy + s(2)}, colAccent)
		knob := s(6)
		if b.hover == elemSeek || b.dragSeek {
			knob = s(8)
		}
		ellipse(hdc, px-knob, cy-knob, px+knob, cy+knob, colAccent)
	} else if st.PlaylistLen > 1 {
		text(hdc, b.seek, fmt.Sprintf("%d / %d", st.PlaylistPos+1, st.PlaylistLen), b.font, colDim, dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	}

	// volume
	b.button(hdc, b.volIcon, elemVolIcon)
	vcxI, vcyI := center(b.volIcon)
	speaker(hdc, vcxI, vcyI, s(6), colText, st.Muted)
	vcy := (b.vol.Top + b.vol.Bottom) / 2
	fill(hdc, rect{b.vol.Left, vcy - s(2), b.vol.Right, vcy + s(2)}, colTrack)
	vf := st.Volume / 150
	if vf > 1 {
		vf = 1
	}
	vx := b.vol.Left + int32(vf*float64(b.vol.width()))
	col := colAccent
	if st.Muted {
		col = colDim
	}
	fill(hdc, rect{b.vol.Left, vcy - s(2), vx, vcy + s(2)}, col)
	ellipse(hdc, vx-s(5), vcy-s(5), vx+s(5), vcy+s(5), col)

	// tracks
	b.button(hdc, b.audio, elemAudio)
	text(hdc, b.audio, "A", b.fontBold, colText, dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	if audioCount > 1 {
		badge(hdc, b.audio, audioCount, b.font, s)
	}
	b.button(hdc, b.subs, elemSubs)
	text(hdc, b.subs, "CC", b.fontBold, colText, dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
	if subCount > 0 {
		badge(hdc, b.subs, subCount, b.font, s)
	}

	// fullscreen
	b.button(hdc, b.fs, elemFS)
	corners(hdc, b.fs, s(5), s(2), colText)
}

func (b *bar) button(hdc uintptr, r rect, elem int) {
	if b.hover == elem {
		roundFill(hdc, r, b.scale(6), colHover)
	}
}

func center(r rect) (int32, int32) { return (r.Left + r.Right) / 2, (r.Top + r.Bottom) / 2 }

func fill(hdc uintptr, r rect, col uintptr) {
	br, _, _ := pCreateSolidBrush.Call(col)
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), br)
	pDeleteObject.Call(br)
}

func roundFill(hdc uintptr, r rect, radius int32, col uintptr) {
	br, _, _ := pCreateSolidBrush.Call(col)
	pen, _, _ := pGetStockObject.Call(nullPen)
	ob, _, _ := pSelectObject.Call(hdc, br)
	op, _, _ := pSelectObject.Call(hdc, pen)
	pRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1), uintptr(radius*2), uintptr(radius*2))
	pSelectObject.Call(hdc, ob)
	pSelectObject.Call(hdc, op)
	pDeleteObject.Call(br)
}

func ellipse(hdc uintptr, l, t, r, bt int32, col uintptr) {
	br, _, _ := pCreateSolidBrush.Call(col)
	pen, _, _ := pGetStockObject.Call(nullPen)
	ob, _, _ := pSelectObject.Call(hdc, br)
	op, _, _ := pSelectObject.Call(hdc, pen)
	pEllipse.Call(hdc, uintptr(l), uintptr(t), uintptr(r+1), uintptr(bt+1))
	pSelectObject.Call(hdc, ob)
	pSelectObject.Call(hdc, op)
	pDeleteObject.Call(br)
}

func polygon(hdc uintptr, pts []point, col uintptr) {
	br, _, _ := pCreateSolidBrush.Call(col)
	pen, _, _ := pGetStockObject.Call(nullPen)
	ob, _, _ := pSelectObject.Call(hdc, br)
	op, _, _ := pSelectObject.Call(hdc, pen)
	pPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	pSelectObject.Call(hdc, ob)
	pSelectObject.Call(hdc, op)
	pDeleteObject.Call(br)
}

func triangle(hdc uintptr, cx, cy int32, r int32, col uintptr) {
	polygon(hdc, []point{{cx - r + 1, cy - r}, {cx + r, cy}, {cx - r + 1, cy + r}}, col)
}

func speaker(hdc uintptr, cx, cy int32, r int32, col uintptr, muted bool) {
	polygon(hdc, []point{{cx - r, cy - r/2}, {cx - r/3, cy - r/2}, {cx + r/3, cy - r}, {cx + r/3, cy + r}, {cx - r/3, cy + r/2}, {cx - r, cy + r/2}}, col)
	pen, _, _ := pCreatePen.Call(psSolid, 2, col)
	op, _, _ := pSelectObject.Call(hdc, pen)
	if muted {
		pMoveToEx.Call(hdc, uintptr(cx+r/2), uintptr(cy-r/2), 0)
		pLineTo.Call(hdc, uintptr(cx+r+r/2), uintptr(cy+r/2))
		pMoveToEx.Call(hdc, uintptr(cx+r+r/2), uintptr(cy-r/2), 0)
		pLineTo.Call(hdc, uintptr(cx+r/2), uintptr(cy+r/2))
	} else {
		pMoveToEx.Call(hdc, uintptr(cx+r/2+2), uintptr(cy-r/2), 0)
		pLineTo.Call(hdc, uintptr(cx+r+2), uintptr(cy))
		pLineTo.Call(hdc, uintptr(cx+r/2+2), uintptr(cy+r/2))
	}
	pSelectObject.Call(hdc, op)
	pDeleteObject.Call(pen)
}

func corners(hdc uintptr, r rect, size, thick int32, col uintptr) {
	cx, cy := center(r)
	half := size + 2
	pen, _, _ := pCreatePen.Call(psSolid, uintptr(thick), col)
	op, _, _ := pSelectObject.Call(hdc, pen)
	l, t, rr, b := cx-half, cy-half, cx+half, cy+half
	seg := size
	for _, c := range [][4]int32{{l, t, 1, 1}, {rr, t, -1, 1}, {l, b, 1, -1}, {rr, b, -1, -1}} {
		pMoveToEx.Call(hdc, uintptr(c[0]), uintptr(c[1]+c[3]*seg), 0)
		pLineTo.Call(hdc, uintptr(c[0]), uintptr(c[1]))
		pLineTo.Call(hdc, uintptr(c[0]+c[2]*seg), uintptr(c[1]))
	}
	pSelectObject.Call(hdc, op)
	pDeleteObject.Call(pen)
}

func badge(hdc uintptr, r rect, n int, font uintptr, s func(int32) int32) {
	br := rect{r.Right - s(14), r.Top - s(2), r.Right + s(2), r.Top + s(12)}
	roundFill(hdc, br, s(6), colBadgeBg)
	text(hdc, br, fmt.Sprint(n), font, colBar, dtCenter|dtVCenter|dtSingleLine|dtNoPrefix)
}

func text(hdc uintptr, r rect, s string, font uintptr, col uintptr, flags uintptr) {
	of, _, _ := pSelectObject.Call(hdc, font)
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, col)
	u, _ := utf16Ptr(s)
	pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(u)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	pSelectObject.Call(hdc, of)
}
