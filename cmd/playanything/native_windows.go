//go:build windows

package main

import (
	"github.com/inphaseye172/playanything/internal/app"
	"github.com/inphaseye172/playanything/internal/version"
	"github.com/inphaseye172/playanything/internal/winui"
)

// runNative opens PlayAnything's own player window. It returns handled=false
// when the embedded engine is unavailable so the caller can fall back to the
// legacy external-mpv launcher.
func runNative(a *app.App, inputs []string, opt app.PlayOptions) (handled bool, err error) {
	if a.Cfg.Engine == "external" {
		return false, nil
	}
	if err := winui.LibraryAvailable(); err != nil {
		a.Log("embedded engine unavailable, using external mpv: %v", err)
		return false, nil
	}
	cfg := *a.Cfg
	if opt.Fullscreen {
		cfg.Fullscreen = true
	}
	cfg.ExtraMPVArgs = append(append([]string{}, cfg.ExtraMPVArgs...), opt.MPVArgs...)
	err = winui.Run(winui.Options{
		Paths:   a.Paths,
		Cfg:     &cfg,
		Inputs:  inputs,
		Append:  opt.Append,
		Version: version.String(),
		Log:     a.Log,
	})
	return true, err
}
