//go:build darwin

package native

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -framework Cocoa
#include "appearance.h"
*/
import "C"
import "unsafe"

func SetWindowAppearance(_ unsafe.Pointer, mode string) {
	var cMode C.int
	switch mode {
	case "dark":
		cMode = 1
	case "light":
		cMode = 0
	default:
		cMode = 2
	}
	C.setWindowAppearanceMode(cMode)
}
