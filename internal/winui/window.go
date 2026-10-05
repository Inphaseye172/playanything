//go:build windows

package winui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/inphaseye172/playanything/internal/config"
	"github.com/inphaseye172/playanything/internal/engine"
	"github.com/inphaseye172/playanything/internal/player"
)

// Options configures the native window.
type Options struct {
	Paths   config.Paths
	Cfg     *config.Config
	Inputs  []string
	Append  bool
	Version string
	Log     func(format string, args ...any)
}

const (
	mainClass  = "PlayAnythingMain"
	videoClass = "PlayAnythingVideo"
	mutexName  = "Local\\PlayAnything.SingleInstance"
	timerID    = 1
	appTitle   = "PlayAnything"
)

type ui struct {
	opts  Options
	hinst uintptr
	hwnd  uintptr
	video uintptr
	p     *player.Player
	bar   bar
	menus menus

	mu      sync.Mutex
	pending []player.Event
	state   player.State
	tracks  []player.Track
	nAudio  int
	nSubs   int

	fullscreen bool
	savedStyle uintptr
	savedPlace windowPlacement
	barHidden  bool // user choice (View > Hide controls)
	barShown   bool // currently laid out
	onTop      bool
	lastMouse  time.Time
	statusTill time.Time
	title      string

	ctx    context.Context
	cancel context.CancelFunc
}

var current *ui

// GWL_STYLE is -16; the Win32 call takes it as a sign-extended machine word.
var gwlStyleIndex = func() uintptr { i := gwlStyle; return uintptr(i) }()

// ptr reinterprets a message parameter that carries a Windows-owned pointer.
// (Written this way because vet rejects a direct uintptr -> unsafe.Pointer
// conversion; the memory is never Go-managed.)
func ptr(v uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&v)) }

// Run opens the player window, plays the inputs and blocks until the window
// closes. When another PlayAnything window is already open, the inputs are
// handed to it instead and Run returns immediately.
func Run(o Options) error {
	runtime.LockOSThread()
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	if handOff(o.Inputs, o.Append) {
		return nil
	}
	if pSetProcessDpiAwareCt.Find() == nil {
		pSetProcessDpiAwareCt.Call(dpiAwarenessPerMonitorV2)
	}
	u := &ui{opts: o}
	current = u
	u.ctx, u.cancel = context.WithCancel(context.Background())
	defer u.cancel()

	u.hinst, _, _ = pGetModuleHandleW.Call(0)
	if err := u.registerClasses(); err != nil {
		return err
	}
	if err := u.createWindows(); err != nil {
		return err
	}
	p, err := player.New(player.Options{Paths: o.Paths, Cfg: o.Cfg, WindowID: u.video, Log: o.Log})
	if err != nil {
		pDestroyWindow.Call(u.hwnd)
		return err
	}
	u.p = p
	u.state = p.Snapshot()
	p.OnEvent(u.push)

	u.menus.build()
	pSetMenu.Call(u.hwnd, u.menus.bar)
	u.setIcon()
	u.setTitle("")
	u.layout()
	pShowWindow.Call(u.hwnd, swShow)
	pUpdateWindow.Call(u.hwnd)
	pSetTimer.Call(u.hwnd, timerID, 200, 0)
	pDragAcceptFiles.Call(u.hwnd, 1)

	if len(o.Inputs) > 0 {
		go u.open(o.Inputs, o.Append)
	}

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	u.cancel()
	p.Close()
	return nil
}

// handOff passes the inputs to an already running window through WM_COPYDATA.
func handOff(inputs []string, add bool) bool {
	h, _, _ := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(utf16(mutexName))))
	if h == 0 {
		return false
	}
	if e, _, _ := pGetLastError.Call(); e != errAlreadyExist {
		return false // we are the first instance; keep the mutex for our lifetime
	}
	var hwnd uintptr
	for i := 0; i < 40; i++ { // the other instance may still be starting
		hwnd, _, _ = pFindWindowW.Call(uintptr(unsafe.Pointer(utf16(mainClass))), 0)
		if hwnd != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if hwnd == 0 {
		return false
	}
	mode := "replace"
	if add {
		mode = "append"
	}
	payload := syscall.StringToUTF16(mode + "\n" + strings.Join(inputs, "\n"))
	cds := copyDataStruct{Data: 1, CbData: uint32(len(payload) * 2), LpData: uintptr(unsafe.Pointer(&payload[0]))}
	pSendMessageW.Call(hwnd, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)))
	if r, _, _ := pIsIconic.Call(hwnd); r != 0 {
		pShowWindow.Call(hwnd, swRestore)
	}
	pSetForegroundWindow.Call(hwnd)
	return true
}

func (u *ui) registerClasses() error {
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	icon, _, _ := pLoadIconW.Call(u.hinst, idiApp)
	main := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		Style:     csHRedraw | csVRedraw,
		WndProc:   syscall.NewCallback(mainProc),
		Instance:  u.hinst,
		Icon:      icon,
		Cursor:    cursor,
		ClassName: utf16(mainClass),
		IconSm:    icon,
	}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&main))); r == 0 {
		return fmt.Errorf("RegisterClassEx(main): %v", err)
	}
	black, _, _ := pGetStockObject.Call(blackBrush)
	vid := wndClassExW{
		Size:       uint32(unsafe.Sizeof(wndClassExW{})),
		Style:      csDblClks,
		WndProc:    syscall.NewCallback(videoProc),
		Instance:   u.hinst,
		Cursor:     cursor,
		Background: black,
		ClassName:  utf16(videoClass),
	}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&vid))); r == 0 {
		return fmt.Errorf("RegisterClassEx(video): %v", err)
	}
	return nil
}

func (u *ui) createWindows() error {
	w, h := uintptr(1280), uintptr(760)
	hwnd, _, err := pCreateWindowExW.Call(wsExAcceptFiles|wsExAppWindow,
		uintptr(unsafe.Pointer(utf16(mainClass))), uintptr(unsafe.Pointer(utf16(appTitle))),
		wsOverlappedWindow|wsClipChildren, cwUseDefault, cwUseDefault, w, h, 0, 0, u.hinst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx(main): %v", err)
	}
	u.hwnd = hwnd
	u.bar.setDPI(dpiFor(hwnd))
	video, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16(videoClass))), 0,
		wsChild|wsVisible|wsClipChildren|wsClipSiblings, 0, 0, 10, 10, hwnd, 0, u.hinst, 0)
	if video == 0 {
		return fmt.Errorf("CreateWindowEx(video): %v", err)
	}
	u.video = video
	return nil
}

func (u *ui) setIcon() {
	cx, _, _ := pGetSystemMetrics.Call(smCxIcon)
	cy, _, _ := pGetSystemMetrics.Call(smCyIcon)
	sx, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	sy, _, _ := pGetSystemMetrics.Call(smCySmIcon)
	big, _, _ := pLoadImageW.Call(u.hinst, idiApp, imageIcon, cx, cy, lrShared)
	small, _, _ := pLoadImageW.Call(u.hinst, idiApp, imageIcon, sx, sy, lrShared)
	if big != 0 {
		pSendMessageW.Call(u.hwnd, wmSetIcon, iconBig, big)
	}
	if small != 0 {
		pSendMessageW.Call(u.hwnd, wmSetIcon, iconSmall, small)
	}
}

func (u *ui) setTitle(media string) {
	t := appTitle
	if media != "" {
		t = media + " — " + appTitle
	}
	if t == u.title {
		return
	}
	u.title = t
	pSetWindowTextW.Call(u.hwnd, uintptr(unsafe.Pointer(utf16(t))))
}

// layout places the video surface and the control bar.
func (u *ui) layout() {
	c := getClientRect(u.hwnd)
	showBar := !u.barHidden && (!u.fullscreen || u.barShown)
	barH := u.bar.height()
	if !showBar {
		barH = 0
	}
	vh := c.Bottom - barH
	if vh < 1 {
		vh = 1
	}
	pMoveWindow.Call(u.video, 0, 0, uintptr(c.Right), uintptr(vh), 1)
	u.fitEngineWindow()
	u.bar.layout(rect{0, vh, c.Right, c.Bottom})
	pInvalidateRect.Call(u.hwnd, 0, 1)
}

// fitEngineWindow keeps the engine's own child window the size of our video surface.
func (u *ui) fitEngineWindow() {
	getWindow := user32.NewProc("GetWindow")
	child, _, _ := getWindow.Call(u.video, 5) // GW_CHILD
	if child == 0 {
		return
	}
	c := getClientRect(u.video)
	pSetWindowPos.Call(child, 0, 0, 0, uintptr(c.Right), uintptr(c.Bottom), swpNoZOrder|swpNoActivate)
}

// push is called on the player goroutine; events are marshalled to the UI thread.
func (u *ui) push(ev player.Event) {
	u.mu.Lock()
	u.pending = append(u.pending, ev)
	n := len(u.pending)
	u.mu.Unlock()
	if n == 1 {
		pPostMessageW.Call(u.hwnd, wmAppEvent, 0, 0)
	}
}

func (u *ui) drain() {
	u.mu.Lock()
	evs := u.pending
	u.pending = nil
	u.mu.Unlock()
	for _, ev := range evs {
		u.handle(ev)
	}
}

func (u *ui) handle(ev player.Event) {
	prev := u.state
	u.state = ev.State
	switch ev.Kind {
	case player.EvState:
		if u.state.Fullscreen != u.fullscreen {
			u.applyFullscreen(u.state.Fullscreen)
		}
		if u.state.MediaTitle != prev.MediaTitle || u.state.Idle != prev.Idle {
			if u.state.Idle {
				u.setTitle("")
			} else {
				u.setTitle(u.state.MediaTitle)
			}
		}
		if u.state.AudioID != prev.AudioID || u.state.SubID != prev.SubID {
			u.menus.rebuildTracks(u.tracks, u.state.AudioID, u.state.SubID)
		}
		u.invalidateBar()
	case player.EvFileLoaded:
		u.tracks = u.p.Tracks()
		u.nAudio, u.nSubs = 0, 0
		for _, t := range u.tracks {
			switch t.Type {
			case "audio":
				u.nAudio++
			case "sub":
				u.nSubs++
			}
		}
		u.menus.rebuildTracks(u.tracks, u.state.AudioID, u.state.SubID)
		u.bar.status = ""
		u.setTitle(u.state.MediaTitle)
		u.invalidateBar()
	case player.EvEndFile:
		if ev.Error {
			u.setStatus("Could not open: "+ev.Text, 8*time.Second)
			if u.state.PlaylistLen <= 1 {
				messageBox(u.hwnd, "Could not open this file.\n\n"+ev.Text, appTitle, mbOK|mbIconError)
			}
		}
	case player.EvMessage:
		u.setStatus(ev.Text, 6*time.Second)
	case player.EvShutdown:
		pDestroyWindow.Call(u.hwnd)
	}
}

func (u *ui) setStatus(s string, d time.Duration) {
	u.bar.status = s
	u.statusTill = time.Now().Add(d)
	u.invalidateBar()
}

func (u *ui) invalidateBar() {
	r := u.bar.rect
	pInvalidateRect.Call(u.hwnd, uintptr(unsafe.Pointer(&r)), 0)
}

// open runs on a goroutine (R3D rendering can take a while).
func (u *ui) open(inputs []string, add bool) {
	if err := u.p.Open(u.ctx, inputs, add); err != nil {
		u.push(player.Event{Kind: player.EvMessage, Text: err.Error(), Error: true})
		go func() { messageBox(u.hwnd, err.Error(), appTitle, mbOK|mbIconError) }()
	}
}

func (u *ui) applyFullscreen(on bool) {
	if on == u.fullscreen {
		return
	}
	u.fullscreen = on
	u.menus.setChecked(cmdFullscreen, on)
	if on {
		u.savedStyle, _, _ = pGetWindowLongPtrW.Call(u.hwnd, gwlStyleIndex)
		u.savedPlace.Length = uint32(unsafe.Sizeof(windowPlacement{}))
		pGetWindowPlacement.Call(u.hwnd, uintptr(unsafe.Pointer(&u.savedPlace)))
		mon, _, _ := pMonitorFromWindow.Call(u.hwnd, monitorDefaultToNearest)
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
		pSetMenu.Call(u.hwnd, 0)
		pSetWindowLongPtrW.Call(u.hwnd, gwlStyleIndex, wsPopup|wsVisible|wsClipChildren)
		pSetWindowPos.Call(u.hwnd, ^uintptr(0) /* HWND_TOPMOST */, uintptr(mi.Monitor.Left), uintptr(mi.Monitor.Top),
			uintptr(mi.Monitor.width()), uintptr(mi.Monitor.height()), swpFrameChanged)
		u.barShown = false
	} else {
		pSetWindowLongPtrW.Call(u.hwnd, gwlStyleIndex, u.savedStyle)
		pSetMenu.Call(u.hwnd, u.menus.bar)
		pSetWindowPlacement.Call(u.hwnd, uintptr(unsafe.Pointer(&u.savedPlace)))
		top := ^uintptr(1) // HWND_NOTOPMOST
		if u.onTop {
			top = ^uintptr(0)
		}
		pSetWindowPos.Call(u.hwnd, top, 0, 0, 0, 0, swpFrameChanged|0x0001|0x0002) // SWP_NOSIZE|SWP_NOMOVE
	}
	u.layout()
}

func (u *ui) toggleOnTop() {
	u.onTop = !u.onTop
	u.menus.setChecked(cmdOnTop, u.onTop)
	top := ^uintptr(1)
	if u.onTop {
		top = ^uintptr(0)
	}
	pSetWindowPos.Call(u.hwnd, top, 0, 0, 0, 0, 0x0001|0x0002)
}

func (u *ui) command(id int) {
	p := u.p
	switch {
	case id == cmdOpen || id == cmdOpenAppend:
		files := openFilesDialog(u.hwnd, "Open media")
		if len(files) > 0 {
			go u.open(files, id == cmdOpenAppend)
		}
	case id == cmdExit:
		pDestroyWindow.Call(u.hwnd)
	case id == cmdPlayPause:
		_ = p.TogglePause()
	case id == cmdStop:
		_ = p.Stop()
	case id == cmdPrev:
		_ = p.Prev()
	case id == cmdNext:
		_ = p.Next()
	case id == cmdFrameBack:
		_ = p.FrameBack()
	case id == cmdFrameStep:
		_ = p.FrameStep()
	case id == cmdSpeedHalf:
		_ = p.SetSpeed(0.5)
	case id == cmdSpeedNormal:
		_ = p.SetSpeed(1)
	case id == cmdSpeedFast:
		_ = p.SetSpeed(1.5)
	case id == cmdSpeedDouble:
		_ = p.SetSpeed(2)
	case id == cmdScreenshot:
		_ = p.Screenshot()
	case id == cmdAudioExternal:
		if f := openFilesDialog(u.hwnd, "Load external audio"); len(f) > 0 {
			_ = p.E.Command("audio-add", f[0], "select")
		}
	case id == cmdSubExternal:
		if f := openFilesDialog(u.hwnd, "Load subtitle file"); len(f) > 0 {
			_ = p.E.Command("sub-add", f[0], "select")
		}
	case id == cmdSubOff:
		_ = p.SelectTrack("sub", 0)
	case id == cmdRotate:
		_ = p.Rotate()
	case id == cmdZoomReset:
		_ = p.E.Command("set", "video-zoom", "0")
		_ = p.E.Command("set", "video-pan-x", "0")
		_ = p.E.Command("set", "video-pan-y", "0")
	case id == cmdHWDec:
		_ = p.ToggleHWDec()
	case id == cmdStats:
		_ = p.ToggleStats()
	case id == cmdFullscreen:
		_ = p.SetFullscreen(!u.fullscreen)
	case id == cmdOnTop:
		u.toggleOnTop()
	case id == cmdHideBar:
		u.barHidden = !u.barHidden
		u.menus.setChecked(cmdHideBar, u.barHidden)
		u.layout()
	case id == cmdShortcuts:
		messageBox(u.hwnd, shortcutsText, "Keyboard shortcuts", mbOK|mbIconInfo)
	case id == cmdDiagnostics:
		messageBox(u.hwnd, u.diagnostics(), "Diagnostics", mbOK|mbIconInfo)
	case id == cmdConfigFolder:
		shellOpen(u.opts.Paths.Config)
	case id == cmdAbout:
		lib := u.p.E.Library()
		messageBox(u.hwnd, fmt.Sprintf("PlayAnything %s\nby Anchor Point Studio\n\nOne click, any media: video, audio, photos, camera RAW — local, NAS or cloud, GPU accelerated.\n\nPlayback engine: libmpv %s (FFmpeg, libplacebo)\n%s\n\nMIT licensed. https://github.com/inphaseye172/playanything",
			u.opts.Version, lib.VersionString(), lib.Path), "About PlayAnything", mbOK|mbIconInfo)
	case id >= cmdAudioBase && id < cmdAudioBase+1000:
		_ = p.SelectTrack("audio", int64(id-cmdAudioBase))
	case id >= cmdSubBase && id < cmdSubBase+1000:
		_ = p.SelectTrack("sub", int64(id-cmdSubBase))
	}
}

func (u *ui) diagnostics() string {
	st := u.state
	lib := u.p.E.Library()
	var sb strings.Builder
	fmt.Fprintf(&sb, "PlayAnything %s\n\n", u.opts.Version)
	fmt.Fprintf(&sb, "Engine: libmpv client API %s\n%s\n\n", lib.VersionString(), lib.Path)
	if v, err := u.p.E.GetString("mpv-version"); err == nil {
		fmt.Fprintf(&sb, "%s\n", v)
	}
	if v, err := u.p.E.GetString("ffmpeg-version"); err == nil {
		fmt.Fprintf(&sb, "FFmpeg %s\n", v)
	}
	fmt.Fprintf(&sb, "\nGPU decoding: %s\n", orNone(st.HWDec))
	if v, err := u.p.E.GetString("video-codec"); err == nil {
		fmt.Fprintf(&sb, "Video codec: %s (%dx%d)\n", v, st.Width, st.Height)
	}
	if v, err := u.p.E.GetString("video-params/pixelformat"); err == nil {
		fmt.Fprintf(&sb, "Pixel format: %s\n", v)
	}
	if v, err := u.p.E.GetString("audio-codec-name"); err == nil {
		fmt.Fprintf(&sb, "Audio codec: %s\n", v)
	}
	if v, err := u.p.E.GetString("current-vo"); err == nil {
		fmt.Fprintf(&sb, "Renderer: %s\n", v)
	}
	if v, err := u.p.E.GetString("gpu-api"); err == nil {
		fmt.Fprintf(&sb, "GPU API: %s\n", orNone(v))
	}
	fmt.Fprintf(&sb, "\nSettings: %s\nCache: %s\n", u.opts.Paths.Config, u.opts.Paths.Cache)
	return sb.String()
}

func orNone(s string) string {
	if s == "" || s == "no" {
		return "no (software)"
	}
	return s
}

// --- window procedures ---------------------------------------------------

func mainProc(hwnd uintptr, m uint32, wParam, lParam uintptr) uintptr {
	u := current
	if u == nil || (u.hwnd != 0 && hwnd != u.hwnd) {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(m), wParam, lParam)
		return r
	}
	switch m {
	case wmAppEvent:
		u.drain()
		return 0
	case wmSize:
		u.layout()
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		u.paint()
		return 0
	case wmDpiChanged:
		u.bar.setDPI(int32(wParam & 0xFFFF))
		r := (*rect)(ptr(lParam))
		pSetWindowPos.Call(hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), swpNoZOrder|swpNoActivate)
		u.layout()
		return 0
	case wmCommand:
		if lParam == 0 { // menu / accelerator
			u.command(int(wParam & 0xFFFF))
			return 0
		}
	case wmKeyDown, wmSysKeyDown:
		vk := uint32(wParam)
		if vk == 0x1B && u.fullscreen { // Esc leaves fullscreen
			_ = u.p.SetFullscreen(false)
			return 0
		}
		if keyDown(vkControl) && !keyDown(vkMenu) {
			switch vk {
			case 'O':
				if keyDown(vkShift) {
					u.command(cmdOpenAppend)
				} else {
					u.command(cmdOpen)
				}
				return 0
			case 'B':
				u.command(cmdHideBar)
				return 0
			}
		}
		if name := mpvKeyName(vk); name != "" {
			_ = u.p.Key(name)
			return 0
		}
		if m == wmSysKeyDown && vk == 0x0D { // Alt+Enter fullscreen
			_ = u.p.SetFullscreen(!u.fullscreen)
			return 0
		}
	case wmChar:
		if name := mpvCharName(uint32(wParam)); name != "" {
			_ = u.p.Key(name)
			return 0
		}
	case wmSysChar:
		return 0 // no beep on Alt+key
	case wmLButtonDown:
		u.mouseDown(loword(lParam), hiword(lParam))
		return 0
	case wmLButtonUp:
		u.mouseUp(loword(lParam), hiword(lParam))
		return 0
	case wmMouseMove:
		u.mouseMove(loword(lParam), hiword(lParam))
		return 0
	case wmMouseWheel:
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		if u.bar.rect.contains(pt.X, pt.Y) {
			delta := float64(hiword(wParam)) / wheelDelta
			_ = u.p.SetVolume(u.state.Volume + 5*delta)
			return 0
		}
	case wmDropFiles:
		u.dropFiles(wParam)
		return 0
	case wmCopyData:
		cds := (*copyDataStruct)(ptr(lParam))
		if cds != nil && cds.CbData >= 2 {
			s := utf16Slice((*uint16)(ptr(cds.LpData)), int(cds.CbData/2))
			lines := strings.Split(s, "\n")
			if len(lines) > 1 {
				files := lines[1:]
				go u.open(files, lines[0] == "append")
			}
		}
		return 1
	case wmTimer:
		u.tick()
		return 0
	case wmSetCursor:
		if u.fullscreen && !u.barShown && (lParam&0xFFFF) == 1 { // HTCLIENT
			return 0
		}
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pKillTimer.Call(hwnd, timerID)
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(m), wParam, lParam)
	return r
}

func videoProc(hwnd uintptr, m uint32, wParam, lParam uintptr) uintptr {
	switch m {
	case wmEraseBkgnd:
		return 1
	case wmSize:
		if u := current; u != nil {
			u.fitEngineWindow()
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(m), wParam, lParam)
	return r
}

func (u *ui) paint() {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(u.hwnd, uintptr(unsafe.Pointer(&ps)))
	r := u.bar.rect
	if r.height() <= 0 {
		return
	}
	// double buffer the bar
	mem, _, _ := pCreateCompatibleDC.Call(hdc)
	bmp, _, _ := pCreateCompatibleBitmp.Call(hdc, uintptr(r.width()), uintptr(r.height()))
	old, _, _ := pSelectObject.Call(mem, bmp)
	saved := u.bar.rect
	u.bar.rect = rect{0, 0, r.width(), r.height()}
	u.bar.layout(u.bar.rect)
	u.bar.paint(mem, u.state, u.nAudio, u.nSubs)
	u.bar.rect = saved
	u.bar.layout(saved)
	pBitBlt.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.width()), uintptr(r.height()), mem, 0, 0, srcCopy)
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
}

func (u *ui) mouseDown(x, y int32) {
	pSetFocus.Call(u.hwnd)
	switch u.bar.hit(x, y) {
	case elemPlay:
		_ = u.p.TogglePause()
	case elemSeek:
		if u.state.Duration > 0 {
			u.bar.dragSeek = true
			pSetCapture.Call(u.hwnd)
			u.seekTo(x, false)
		}
	case elemVolIcon:
		_ = u.p.ToggleMute()
	case elemVol:
		u.bar.dragVol = true
		pSetCapture.Call(u.hwnd)
		_ = u.p.SetVolume(fraction(u.bar.vol, x) * 150)
	case elemAudio, elemSubs:
		typ := "audio"
		r := u.bar.audio
		if u.bar.hit(x, y) == elemSubs {
			typ, r = "sub", u.bar.subs
		}
		pt := point{r.Left, r.Top}
		pClientToScreen.Call(u.hwnd, uintptr(unsafe.Pointer(&pt)))
		if id := u.menus.trackPopup(u.hwnd, typ, pt.X, pt.Y); id != 0 {
			u.command(id)
		}
	case elemFS:
		_ = u.p.SetFullscreen(!u.fullscreen)
	}
}

func (u *ui) mouseUp(x, y int32) {
	if u.bar.dragSeek {
		u.bar.dragSeek = false
		pReleaseCapture.Call()
		u.seekTo(x, true)
	}
	if u.bar.dragVol {
		u.bar.dragVol = false
		pReleaseCapture.Call()
	}
}

func (u *ui) mouseMove(x, y int32) {
	u.lastMouse = time.Now()
	if u.bar.dragSeek {
		u.seekTo(x, false)
		return
	}
	if u.bar.dragVol {
		_ = u.p.SetVolume(fraction(u.bar.vol, x) * 150)
		return
	}
	if h := u.bar.hit(x, y); h != u.bar.hover {
		u.bar.hover = h
		u.invalidateBar()
	}
}

func (u *ui) seekTo(x int32, exact bool) {
	now := time.Now().UnixMilli()
	if !exact && now-u.bar.lastSeek < 60 {
		return
	}
	u.bar.lastSeek = now
	_ = u.p.SeekPercent(fraction(u.bar.seek, x)*100, exact)
}

func (u *ui) dropFiles(hDrop uintptr) {
	n, _, _ := pDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	var files []string
	for i := uintptr(0); i < n; i++ {
		size, _, _ := pDragQueryFileW.Call(hDrop, i, 0, 0)
		buf := make([]uint16, size+1)
		pDragQueryFileW.Call(hDrop, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
		files = append(files, syscall.UTF16ToString(buf))
	}
	pDragFinish.Call(hDrop)
	if len(files) > 0 {
		go u.open(files, keyDown(vkShift))
	}
}

// tick runs 5×/s: fullscreen bar auto-show, status expiry, focus upkeep.
func (u *ui) tick() {
	if u.bar.status != "" && !u.statusTill.IsZero() && time.Now().After(u.statusTill) {
		u.bar.status = ""
		u.invalidateBar()
	}
	if u.fullscreen && !u.barHidden {
		var pt point
		pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		pScreenToClient.Call(u.hwnd, uintptr(unsafe.Pointer(&pt)))
		c := getClientRect(u.hwnd)
		near := pt.Y > c.Bottom-u.bar.height()*2 && pt.Y <= c.Bottom && pt.X >= 0 && pt.X <= c.Right
		if near != u.barShown {
			u.barShown = near
			u.layout()
		}
	}
	// Keep keyboard focus with us so menus and shortcuts work even after a
	// click on the video (the engine's window would otherwise take it).
	getFocus := user32.NewProc("GetFocus")
	getForeground := user32.NewProc("GetForegroundWindow")
	if f, _, _ := getFocus.Call(); f != u.hwnd {
		if fg, _, _ := getForeground.Call(); fg == u.hwnd {
			pSetFocus.Call(u.hwnd)
		}
	}
}

// LibraryAvailable reports whether the embedded engine can be loaded.
func LibraryAvailable() error {
	_, err := engine.Load()
	return err
}

// defaultWindowTitle is used by the entry point for logging.
func defaultWindowTitle() string {
	exe, _ := os.Executable()
	return filepath.Base(exe)
}
