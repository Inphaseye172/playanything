//go:build windows

package winui

import "fmt"

// Virtual-key codes we translate to mpv key names. Printable characters
// arrive through WM_CHAR and are forwarded as-is, so input.conf bindings
// (a, A, <, >, [, ], r, i, …) keep working exactly as documented.
var vkNames = map[uint32]string{
	0x08: "BS", 0x09: "TAB", 0x0D: "ENTER", 0x1B: "ESC", 0x20: "SPACE",
	0x21: "PGUP", 0x22: "PGDWN", 0x23: "END", 0x24: "HOME",
	0x25: "LEFT", 0x26: "UP", 0x27: "RIGHT", 0x28: "DOWN",
	0x2D: "INS", 0x2E: "DEL",
	0x70: "F1", 0x71: "F2", 0x72: "F3", 0x73: "F4", 0x74: "F5", 0x75: "F6",
	0x76: "F7", 0x77: "F8", 0x78: "F9", 0x79: "F10", 0x7A: "F11", 0x7B: "F12",
	0xB0: "NEXT", 0xB1: "PREV", 0xB2: "STOP", 0xB3: "PLAYPAUSE", // media keys
	0xAD: "MUTE", 0xAE: "VOLUME_DOWN", 0xAF: "VOLUME_UP",
}

const (
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12 // Alt
)

func keyDown(vk uint32) bool {
	r, _, _ := pGetKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

// mpvKeyName maps a WM_KEYDOWN to an mpv key name, or "" when the key should
// instead be delivered by WM_CHAR (plain printable characters).
func mpvKeyName(vk uint32) string {
	ctrl, alt, shift := keyDown(vkControl), keyDown(vkMenu), keyDown(vkShift)
	name, special := vkNames[vk]
	if !special {
		// Letters/digits with Ctrl or Alt: WM_CHAR will not give a usable
		// character, so build the chord here. Plain ones go through WM_CHAR.
		if !ctrl && !alt {
			return ""
		}
		switch {
		case vk >= 'A' && vk <= 'Z':
			if shift {
				name = string(rune(vk))
			} else {
				name = string(rune(vk + 32))
			}
		case vk >= '0' && vk <= '9':
			name = string(rune(vk))
		default:
			return ""
		}
	}
	prefix := ""
	if ctrl {
		prefix += "Ctrl+"
	}
	if alt {
		prefix += "Alt+"
	}
	if shift && special {
		prefix += "Shift+"
	}
	return prefix + name
}

// mpvCharName maps a WM_CHAR code point to an mpv key name.
func mpvCharName(ch uint32) string {
	if ch <= 0x20 || ch == 0x7F { // control characters and Space are handled by WM_KEYDOWN
		return ""
	}
	if ch == '#' {
		return "SHARP"
	}
	return fmt.Sprintf("%c", rune(ch))
}
