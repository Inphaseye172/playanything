//go:build !windows

package engine

import "github.com/ebitengine/purego"

func dlopen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}
