//go:build !windows

package main

import "github.com/inphaseye172/playanything/internal/app"

// The native window exists for Windows today; macOS and Linux use the
// launcher path (external mpv) until their native ports land.
func runNative(a *app.App, inputs []string, opt app.PlayOptions) (bool, error) {
	return false, nil
}
