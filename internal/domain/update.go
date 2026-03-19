package domain

// Version is set at build time via -ldflags.
var Version = "dev"

type UpdateInfo struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseURL"`
}
