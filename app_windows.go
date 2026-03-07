package main

import (
	"syscall"
	"unsafe"
)

func init() {
	// Set the Application User Model ID so Windows associates
	// WebView2 audio sessions with this app instead of showing
	// "Edge WebView" in the Volume Mixer.
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	appID, _ := syscall.UTF16PtrFromString("FerrLab.AirspaceACARS")
	proc.Call(uintptr(unsafe.Pointer(appID)))
}
