//go:build windows

package winui

import (
	"fmt"
	"unsafe"

	"github.com/inphaseye172/playanything/internal/player"
)

// Menu command ids.
const (
	cmdOpen = 1000 + iota
	cmdOpenAppend
	cmdExit
	cmdPlayPause
	cmdStop
	cmdPrev
	cmdNext
	cmdFrameBack
	cmdFrameStep
	cmdSpeedHalf
	cmdSpeedNormal
	cmdSpeedFast
	cmdSpeedDouble
	cmdScreenshot
	cmdAudioExternal
	cmdSubOff
	cmdSubExternal
	cmdRotate
	cmdZoomReset
	cmdHWDec
	cmdStats
	cmdFullscreen
	cmdOnTop
	cmdHideBar
	cmdShortcuts
	cmdDiagnostics
	cmdConfigFolder
	cmdAbout
	cmdAudioBase = 2000 // + track id
	cmdSubBase   = 3000 // + track id
)

type menus struct {
	bar, audio, subs, view uintptr
}

func appendItem(menu uintptr, id int, label string) {
	pAppendMenuW.Call(menu, mfString, uintptr(id), uintptr(unsafe.Pointer(utf16(label))))
}

func appendSep(menu uintptr) { pAppendMenuW.Call(menu, mfSeparator, 0, 0) }

func appendPopup(bar uintptr, sub uintptr, label string) {
	pAppendMenuW.Call(bar, mfPopup, sub, uintptr(unsafe.Pointer(utf16(label))))
}

func newPopup() uintptr {
	h, _, _ := pCreatePopupMenu.Call()
	return h
}

func (m *menus) build() {
	m.bar, _, _ = pCreateMenu.Call()

	file := newPopup()
	appendItem(file, cmdOpen, "&Open…\tCtrl+O")
	appendItem(file, cmdOpenAppend, "&Add to playlist…\tCtrl+Shift+O")
	appendSep(file)
	appendItem(file, cmdExit, "E&xit\tQ")
	appendPopup(m.bar, file, "&File")

	play := newPopup()
	appendItem(play, cmdPlayPause, "&Play / Pause\tSpace")
	appendItem(play, cmdStop, "&Stop")
	appendSep(play)
	appendItem(play, cmdPrev, "P&revious file\tPgUp")
	appendItem(play, cmdNext, "&Next file\tPgDn")
	appendSep(play)
	appendItem(play, cmdFrameBack, "Frame &back\t,")
	appendItem(play, cmdFrameStep, "Frame &forward\t.")
	appendSep(play)
	appendItem(play, cmdSpeedHalf, "Speed 0.5×")
	appendItem(play, cmdSpeedNormal, "Speed 1×\tBackspace")
	appendItem(play, cmdSpeedFast, "Speed 1.5×")
	appendItem(play, cmdSpeedDouble, "Speed 2×")
	appendSep(play)
	appendItem(play, cmdScreenshot, "S&creenshot to Desktop\tCtrl+S")
	appendPopup(m.bar, play, "&Playback")

	m.audio = newPopup()
	appendPopup(m.bar, m.audio, "&Audio")
	m.subs = newPopup()
	appendPopup(m.bar, m.subs, "&Subtitles")
	m.rebuildTracks(nil, 0, 0)

	video := newPopup()
	appendItem(video, cmdRotate, "&Rotate 90°\tR")
	appendItem(video, cmdZoomReset, "Reset &zoom\t0")
	appendSep(video)
	appendItem(video, cmdHWDec, "Toggle &GPU decoding\tCtrl+H")
	appendItem(video, cmdStats, "&Statistics overlay\tI")
	appendPopup(m.bar, video, "&Video")

	m.view = newPopup()
	appendItem(m.view, cmdFullscreen, "&Fullscreen\tF")
	appendItem(m.view, cmdOnTop, "Always on &top")
	appendItem(m.view, cmdHideBar, "Hide &controls\tCtrl+B")
	appendPopup(m.bar, m.view, "Vie&w")

	help := newPopup()
	appendItem(help, cmdShortcuts, "&Keyboard shortcuts")
	appendItem(help, cmdDiagnostics, "&Diagnostics")
	appendItem(help, cmdConfigFolder, "Open &settings folder")
	appendSep(help)
	appendItem(help, cmdAbout, "&About PlayAnything")
	appendPopup(m.bar, help, "&Help")
}

// rebuildTracks refreshes the Audio and Subtitles menus from the track list.
func (m *menus) rebuildTracks(tracks []player.Track, aid, sid int64) {
	clear := func(menu uintptr) {
		n, _, _ := pGetMenuItemCount.Call(menu)
		for i := int(n) - 1; i >= 0; i-- {
			pDeleteMenu.Call(menu, uintptr(i), 0x0400) // MF_BYPOSITION
		}
	}
	clear(m.audio)
	clear(m.subs)
	var na, ns int
	for _, t := range tracks {
		switch t.Type {
		case "audio":
			na++
			appendItem(m.audio, cmdAudioBase+int(t.ID), fmt.Sprintf("&%d: %s", na, t.Label()))
			if t.ID == aid {
				pCheckMenuItem.Call(m.audio, uintptr(cmdAudioBase+int(t.ID)), mfByCommand|mfChecked)
			}
		case "sub":
			ns++
			appendItem(m.subs, cmdSubBase+int(t.ID), fmt.Sprintf("&%d: %s", ns, t.Label()))
			if t.ID == sid {
				pCheckMenuItem.Call(m.subs, uintptr(cmdSubBase+int(t.ID)), mfByCommand|mfChecked)
			}
		}
	}
	if na == 0 {
		pAppendMenuW.Call(m.audio, mfString|mfGrayed, 0, uintptr(unsafe.Pointer(utf16("(no audio tracks)"))))
	}
	appendSep(m.audio)
	appendItem(m.audio, cmdAudioExternal, "Load &external audio file…")
	appendItem(m.subs, cmdSubOff, "&Off")
	if sid == 0 {
		pCheckMenuItem.Call(m.subs, cmdSubOff, mfByCommand|mfChecked)
	}
	appendSep(m.subs)
	appendItem(m.subs, cmdSubExternal, "Load &subtitle file…")
}

// trackPopup shows a popup of one track type at screen position (x,y) and
// returns the chosen command id (0 when dismissed).
func (m *menus) trackPopup(hwnd uintptr, typ string, x, y int32) int {
	src := m.audio
	if typ == "sub" {
		src = m.subs
	}
	r, _, _ := pTrackPopupMenu.Call(src, tpmLeftAlign|tpmBottomAlign|tpmReturnCmd, uintptr(x), uintptr(y), 0, hwnd, 0)
	return int(r)
}

func (m *menus) setChecked(id int, on bool) {
	flags := uintptr(mfByCommand)
	if on {
		flags |= mfChecked
	}
	pCheckMenuItem.Call(m.view, uintptr(id), flags)
}

const shortcutsText = `Playback
  Space  play / pause          ←  →  seek 5 s        ↑  ↓  seek 1 min
  ,  .   frame back / forward  [  ]  slower / faster   Backspace  normal speed
  PgUp / PgDn   previous / next file in folder        Q  quit

Tracks
  A / Shift+A   next / previous audio track      Ctrl+A  list tracks on screen
  S  next subtitle     V  toggle subtitles       #  cycle audio (mpv default)

Picture
  R  rotate 90°        Ctrl+wheel  zoom          0  reset zoom
  F / Enter / double-click  fullscreen            Esc  leave fullscreen
  Ctrl+S  screenshot (Desktop)     I  statistics / GPU decoder in use
  Ctrl+H  toggle GPU decoding      D  deinterlace

Files
  Ctrl+O  open    Ctrl+Shift+O  add to playlist    drag & drop onto the window
  Shift + drop  add instead of replace`
