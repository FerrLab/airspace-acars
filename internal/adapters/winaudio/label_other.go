//go:build !windows

package winaudio

// Label does nothing off Windows: the volume mixer this works around is a
// Windows feature.
func Label(displayName, iconPath string) (int, error) { return 0, nil }
