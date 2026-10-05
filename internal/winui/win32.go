//go:build windows

// Package winui is PlayAnything's native Windows user interface: a Win32
// window (title bar, menu, keyboard, drag & drop, single instance,
// fullscreen) with the playback engine rendering into an embedded child
// window and a custom-drawn control bar underneath. No cgo, no toolkit.
package winui

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pGetWindowRect        = user32.NewProc("GetWindowRect")
	pMoveWindow           = user32.NewProc("MoveWindow")
	pSetWindowPos         = user32.NewProc("SetWindowPos")
	pGetWindowLongPtrW    = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW    = user32.NewProc("SetWindowLongPtrW")
	pGetWindowPlacement   = user32.NewProc("GetWindowPlacement")
	pSetWindowPlacement   = user32.NewProc("SetWindowPlacement")
	pMonitorFromWindow    = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW      = user32.NewProc("GetMonitorInfoW")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pLoadIconW            = user32.NewProc("LoadIconW")
	pLoadImageW           = user32.NewProc("LoadImageW")
	pSetTimer             = user32.NewProc("SetTimer")
	pKillTimer            = user32.NewProc("KillTimer")
	pInvalidateRect       = user32.NewProc("InvalidateRect")
	pBeginPaint           = user32.NewProc("BeginPaint")
	pEndPaint             = user32.NewProc("EndPaint")
	pFillRect             = user32.NewProc("FillRect")
	pDrawTextW            = user32.NewProc("DrawTextW")
	pCreateMenu           = user32.NewProc("CreateMenu")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pDeleteMenu           = user32.NewProc("DeleteMenu")
	pGetMenuItemCount     = user32.NewProc("GetMenuItemCount")
	pSetMenu              = user32.NewProc("SetMenu")
	pGetMenu              = user32.NewProc("GetMenu")
	pDrawMenuBar          = user32.NewProc("DrawMenuBar")
	pCheckMenuItem        = user32.NewProc("CheckMenuItem")
	pTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pSetFocus             = user32.NewProc("SetFocus")
	pSetCapture           = user32.NewProc("SetCapture")
	pReleaseCapture       = user32.NewProc("ReleaseCapture")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pScreenToClient       = user32.NewProc("ScreenToClient")
	pClientToScreen       = user32.NewProc("ClientToScreen")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pFindWindowW          = user32.NewProc("FindWindowW")
	pIsIconic             = user32.NewProc("IsIconic")
	pGetDpiForWindow      = user32.NewProc("GetDpiForWindow")
	pGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	pSetProcessDpiAwareCt = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetKeyState          = user32.NewProc("GetKeyState")
	pSetCursor            = user32.NewProc("SetCursor")
	pSetWindowRgn         = user32.NewProc("SetWindowRgn")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW     = kernel32.NewProc("CreateMutexW")
	pGetLastError     = kernel32.NewProc("GetLastError")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalFree       = kernel32.NewProc("GlobalFree")

	pCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	pCreatePen             = gdi32.NewProc("CreatePen")
	pCreateFontW           = gdi32.NewProc("CreateFontW")
	pSelectObject          = gdi32.NewProc("SelectObject")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pSetBkMode             = gdi32.NewProc("SetBkMode")
	pSetTextColor          = gdi32.NewProc("SetTextColor")
	pRectangle             = gdi32.NewProc("Rectangle")
	pRoundRect             = gdi32.NewProc("RoundRect")
	pEllipse               = gdi32.NewProc("Ellipse")
	pPolygon               = gdi32.NewProc("Polygon")
	pMoveToEx              = gdi32.NewProc("MoveToEx")
	pLineTo                = gdi32.NewProc("LineTo")
	pCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmp = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt                = gdi32.NewProc("BitBlt")
	pDeleteDC              = gdi32.NewProc("DeleteDC")
	pGetStockObject        = gdi32.NewProc("GetStockObject")

	pDragAcceptFiles = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW  = shell32.NewProc("DragQueryFileW")
	pDragFinish      = shell32.NewProc("DragFinish")
	pShellExecuteW   = shell32.NewProc("ShellExecuteW")

	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
)

// Window styles, messages and constants (WinUser.h).
const (
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsClipChildren     = 0x02000000
	wsClipSiblings     = 0x04000000
	wsPopup            = 0x80000000
	wsCaption          = 0x00C00000
	wsThickFrame       = 0x00040000
	wsExAcceptFiles    = 0x00000010
	wsExAppWindow      = 0x00040000

	cwUseDefault = 0x80000000

	swShow          = 5
	swHide          = 0
	swShowMaximized = 3
	swRestore       = 9

	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmSetFocus      = 0x0007
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmSetCursor     = 0x0020
	wmGetMinMaxInfo = 0x0024
	wmSetIcon       = 0x0080
	wmNCHitTest     = 0x0084
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmChar          = 0x0102
	wmSysKeyDown    = 0x0104
	wmSysChar       = 0x0106
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmMouseMove     = 0x0200
	wmLButtonDown   = 0x0201
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmMouseWheel    = 0x020A
	wmDropFiles     = 0x0233
	wmCopyData      = 0x004A
	wmDpiChanged    = 0x02E0
	wmApp           = 0x8000

	wmAppEvent = wmApp + 1 // player event marshalled to the UI thread

	gwlStyle   = -16
	gwlExStyle = -20

	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	swpNoOwnerZ     = 0x0200
	hwndTop         = 0

	monitorDefaultToNearest = 2

	idcArrow = 32512
	idiApp   = 1 // resource id of the embedded icon (rsrc -ico)

	imageIcon       = 1
	lrDefaultSize   = 0x0040
	lrShared        = 0x8000
	iconSmall       = 0
	iconBig         = 1
	smCxSmIcon      = 49
	smCySmIcon      = 50
	smCxIcon        = 11
	smCyIcon        = 12
	csHRedraw       = 0x0002
	csVRedraw       = 0x0001
	csDblClks       = 0x0008
	colorWindow     = 5
	blackBrush      = 4
	dcBrush         = 18
	dcPen           = 19
	nullPen         = 8
	transparent     = 1
	dtCenter        = 0x0001
	dtVCenter       = 0x0004
	dtSingleLine    = 0x0020
	dtLeft          = 0x0000
	dtRight         = 0x0002
	dtEndEllipsis   = 0x8000
	dtNoPrefix      = 0x0800
	srcCopy         = 0x00CC0020
	psSolid         = 0
	fwNormal        = 400
	fwSemibold      = 600
	defaultCharset  = 1
	cleartypeQual   = 5
	mfString        = 0x0000
	mfPopup         = 0x0010
	mfSeparator     = 0x0800
	mfChecked       = 0x0008
	mfGrayed        = 0x0001
	mfByCommand     = 0x0000
	tpmLeftAlign    = 0x0000
	tpmReturnCmd    = 0x0100
	tpmBottomAlign  = 0x0020
	mbOK            = 0x0000
	mbIconError     = 0x0010
	mbIconInfo      = 0x0040
	mbYesNo         = 0x0004
	idYes           = 6
	ofnAllowMulti   = 0x00000200
	ofnExplorer     = 0x00080000
	ofnFileMustExst = 0x00001000
	ofnPathMustExst = 0x00000800
	ofnHideReadOnly = 0x00000004
	errAlreadyExist = 183
	vkLButton       = 0x01
	wheelDelta      = 120
	gmemMoveable    = 0x0002

	dpiAwarenessPerMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) width() int32  { return r.Right - r.Left }
func (r rect) height() int32 { return r.Bottom - r.Top }
func (r rect) contains(x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	_       uint32
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	HDC         uintptr
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    point
	MaxPosition    point
	NormalPosition rect
	RcDevice       rect
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type copyDataStruct struct {
	Data   uintptr
	CbData uint32
	LpData uintptr
}

type openFileNameW struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) int32 { return int32(int16(v & 0xFFFF)) }
func hiword(v uintptr) int32 { return int32(int16((v >> 16) & 0xFFFF)) }

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func getClientRect(hwnd uintptr) rect {
	var r rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func getWindowRect(hwnd uintptr) rect {
	var r rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func dpiFor(hwnd uintptr) int32 {
	if pGetDpiForWindow.Find() != nil {
		return 96
	}
	d, _, _ := pGetDpiForWindow.Call(hwnd)
	if d == 0 {
		return 96
	}
	return int32(d)
}

func messageBox(hwnd uintptr, text, title string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(title))), flags)
	return int(r)
}

func shellOpen(url string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16("open"))), uintptr(unsafe.Pointer(utf16(url))), 0, 0, swShow)
}

func utf16Slice(p *uint16, max int) string {
	if p == nil {
		return ""
	}
	buf := unsafe.Slice(p, max)
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return syscall.UTF16ToString(buf[:n])
}
