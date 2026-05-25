//go:build linux

package native

import "unsafe"

func SetWindowAppearance(_ unsafe.Pointer, _ string) {}
