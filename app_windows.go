package main

import (
	"syscall"
	"unsafe"
)

func init() {
	// Identify this process to the shell, which groups taskbar buttons and
	// attributes notifications by it.
	//
	// This does not name the volume mixer entry: the audio session belongs
	// to the webview child process, not to this one. That is handled
	// separately, by the winaudio package.
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	appID, _ := syscall.UTF16PtrFromString("FerrLab.AirspaceACARS")
	proc.Call(uintptr(unsafe.Pointer(appID)))
}
