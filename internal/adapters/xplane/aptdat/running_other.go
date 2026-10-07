//go:build !windows

package aptdat

import (
	"os"
	"path/filepath"
	"strings"
)

// runningXPlaneRoot returns the folder of the running X-Plane, found
// through /proc on Linux. Elsewhere (macOS has no /proc) it finds nothing,
// and the xplanePath setting is the way to point at the installation.
func runningXPlaneRoot() (string, bool) {
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return "", false
	}
	for _, p := range procs {
		exe, err := os.Readlink(filepath.Join("/proc", p.Name(), "exe"))
		if err != nil {
			continue
		}
		if strings.HasPrefix(filepath.Base(exe), "X-Plane") {
			return filepath.Dir(exe), true
		}
	}
	return "", false
}
