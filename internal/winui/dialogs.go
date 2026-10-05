//go:build windows

package winui

import (
	"strings"
	"syscall"
	"unsafe"

	"github.com/inphaseye172/playanything/internal/media"
)

func utf16Ptr(s string) (*uint16, error) { return syscall.UTF16PtrFromString(s) }

// openFilesDialog shows the standard multi-select Open dialog and returns the
// chosen paths.
func openFilesDialog(owner uintptr, title string) []string {
	// Filter: "Media files\0*.mkv;*.mp4;...\0All files\0*.*\0\0"
	var pats []string
	for _, e := range media.AllExtensions() {
		pats = append(pats, "*."+e)
	}
	filter := "All media\x00" + strings.Join(pats, ";") + "\x00All files\x00*.*\x00\x00"
	filterU := syscall.StringToUTF16(filter)
	// StringToUTF16 adds one terminator; the double terminator is already in filter.

	buf := make([]uint16, 64*1024)
	ofn := openFileNameW{
		StructSize: uint32(unsafe.Sizeof(openFileNameW{})),
		Owner:      owner,
		Filter:     &filterU[0],
		File:       &buf[0],
		MaxFile:    uint32(len(buf)),
		Title:      utf16(title),
		Flags:      ofnAllowMulti | ofnExplorer | ofnFileMustExst | ofnPathMustExst | ofnHideReadOnly,
	}
	r, _, _ := pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return nil
	}
	// Multi-select: "dir\0name1\0name2\0\0"; single: "full\path\0\0".
	var parts []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			parts = append(parts, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts
	}
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, n := range parts[1:] {
		out = append(out, strings.TrimRight(dir, `\`)+`\`+n)
	}
	return out
}
