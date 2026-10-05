// Package ui shows errors to a person who double-clicked a file and has no
// terminal to read: a native message box on Windows, an AppleScript dialog on
// macOS, zenity/kdialog/notify-send on Linux. Everything is also written to
// stderr so terminal users see it too.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Title is the dialog title.
const Title = "PlayAnything"

// Error reports msg. When gui is true a dialog is shown as well.
func Error(gui bool, msg string) {
	fmt.Fprintln(os.Stderr, "playanything: "+msg)
	if gui {
		dialog(msg, true)
	}
}

// Info reports an informational message the same way.
func Info(gui bool, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if gui {
		dialog(msg, false)
	}
}

func dialog(msg string, isError bool) {
	switch runtime.GOOS {
	case "windows":
		messageBox(msg, isError)
	case "darwin":
		icon := "note"
		if isError {
			icon = "stop"
		}
		script := fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button "OK" with icon %s`, appleQuote(msg), appleQuote(Title), icon)
		_ = exec.Command("osascript", "-e", script).Run()
	default:
		if p, err := exec.LookPath("zenity"); err == nil {
			kind := "--info"
			if isError {
				kind = "--error"
			}
			_ = exec.Command(p, kind, "--no-wrap", "--title="+Title, "--text="+msg).Run()
			return
		}
		if p, err := exec.LookPath("kdialog"); err == nil {
			kind := "--msgbox"
			if isError {
				kind = "--error"
			}
			_ = exec.Command(p, "--title", Title, kind, msg).Run()
			return
		}
		if p, err := exec.LookPath("notify-send"); err == nil {
			urgency := "normal"
			if isError {
				urgency = "critical"
			}
			_ = exec.Command(p, "-u", urgency, "-a", Title, Title, msg).Run()
			return
		}
		if p, err := exec.LookPath("xmessage"); err == nil {
			_ = exec.Command(p, "-center", Title+": "+msg).Run()
		}
	}
}

func appleQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
