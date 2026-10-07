//go:build windows

package aptdat

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runningXPlaneRoot returns the folder of the running X-Plane, which is
// where its executable lives. X-Plane is a single install folder with
// everything under it, wherever Steam or the installer put it, so the
// process is a more reliable guide than any registry key or default path.
func runningXPlaneRoot() (string, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !isXPlaneExe(windows.UTF16ToString(e.ExeFile[:])) {
			continue
		}
		if path, ok := processImagePath(e.ProcessID); ok {
			return filepath.Dir(path), true
		}
	}
	return "", false
}

func processImagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

// isXPlaneExe matches the simulator's executable, "X-Plane.exe", and not
// the installer or the tools that ship beside it.
func isXPlaneExe(name string) bool {
	return strings.EqualFold(name, "X-Plane.exe")
}
