//go:build windows

package native

import (
	"syscall"
	"unsafe"
)

const dWMWA_USE_IMMERSIVE_DARK_MODE = 20

func SetWindowAppearance(hwnd unsafe.Pointer, mode string) {
	if hwnd == nil {
		return
	}

	var useDark bool
	switch mode {
	case "dark":
		useDark = true
	case "light":
		useDark = false
	default:
		return
	}

	dwmapi := syscall.NewLazyDLL("dwmapi.dll")
	proc := dwmapi.NewProc("DwmSetWindowAttribute")
	proc.Call(
		uintptr(hwnd),
		uintptr(dWMWA_USE_IMMERSIVE_DARK_MODE),
		uintptr(unsafe.Pointer(&useDark)),
		unsafe.Sizeof(useDark),
	)
}
