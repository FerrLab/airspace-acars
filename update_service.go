package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var Version = "dev"

type UpdateInfo struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseURL"`
}

type UpdateService struct {
	latest *selfupdate.Release
	app    *application.App
}

func (s *UpdateService) setApp(app *application.App) {
	s.app = app
}

func (s *UpdateService) GetCurrentVersion() string {
	return Version
}

func (s *UpdateService) isBeta() bool {
	return strings.Contains(Version, "-beta")
}

func (s *UpdateService) isStableRelease() bool {
	return Version != "dev" && !s.isBeta()
}

func (s *UpdateService) comparableVersion() string {
	if Version == "dev" {
		return "0.0.0"
	}
	return Version
}

func (s *UpdateService) newUpdater() (*selfupdate.Updater, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create github source: %w", err)
	}

	cfg := selfupdate.Config{
		Source:  source,
		Filters: []string{"airspace-acars-windows-amd64.exe$"},
	}
	// Only stable releases skip pre-releases; dev and beta builds see everything
	if !s.isStableRelease() {
		cfg.Prerelease = true
	}

	updater, err := selfupdate.NewUpdater(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}
	return updater, nil
}

func (s *UpdateService) CheckForUpdate() (*UpdateInfo, error) {
	updater, err := s.newUpdater()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	slug := selfupdate.ParseSlug("FerrLab/airspace-acars")

	info := &UpdateInfo{
		CurrentVersion:  Version,
		UpdateAvailable: false,
	}

	if s.isBeta() {
		// For beta builds, DetectLatest returns the stable release (higher major)
		// which we'd skip. Instead, find the latest beta version explicitly.
		betaVersion, err := s.findLatestBetaVersion(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to find latest beta: %w", err)
		}
		if betaVersion == "" {
			slog.Info("no beta releases found")
			return info, nil
		}

		latest, found, err := updater.DetectVersion(ctx, slug, betaVersion)
		if err != nil || !found {
			return info, err
		}

		info.LatestVersion = latest.Version()
		info.ReleaseURL = latest.ReleaseNotes
		if latest.GreaterThan(s.comparableVersion()) {
			info.UpdateAvailable = true
			s.latest = latest
		}
	} else {
		// Stable builds: use DetectLatest normally (pre-releases excluded)
		latest, found, err := updater.DetectLatest(ctx, slug)
		if err != nil {
			return nil, fmt.Errorf("failed to detect latest version: %w", err)
		}
		if found {
			info.LatestVersion = latest.Version()
			info.ReleaseURL = latest.ReleaseNotes
			if latest.GreaterThan(s.comparableVersion()) {
				info.UpdateAvailable = true
				s.latest = latest
			}
		}
	}

	slog.Info("update check complete", "current", Version, "latest", info.LatestVersion, "available", info.UpdateAvailable)
	return info, nil
}

// findLatestBetaVersion lists all GitHub releases and returns the version string
// of the newest pre-release tagged with "-beta".
func (s *UpdateService) findLatestBetaVersion(ctx context.Context) (string, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return "", err
	}

	rels, err := source.ListReleases(ctx, selfupdate.ParseSlug("FerrLab/airspace-acars"))
	if err != nil {
		return "", err
	}

	var bestVersion *semver.Version
	var bestTag string

	for _, rel := range rels {
		if !rel.GetPrerelease() {
			continue
		}
		tag := rel.GetTagName()
		v := strings.TrimPrefix(tag, "v")
		if !strings.Contains(v, "-beta") {
			continue
		}

		// Check that the release has a matching asset
		hasAsset := false
		for _, asset := range rel.GetAssets() {
			if strings.HasSuffix(asset.GetName(), "airspace-acars-windows-amd64.exe") {
				hasAsset = true
				break
			}
		}
		if !hasAsset {
			continue
		}

		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		if bestVersion == nil || sv.GreaterThan(bestVersion) {
			bestVersion = sv
			bestTag = v
		}
	}

	return bestTag, nil
}

func (s *UpdateService) ApplyUpdate() error {
	if s.latest == nil {
		return fmt.Errorf("no update available — run CheckForUpdate first")
	}

	updater, err := s.newUpdater()
	if err != nil {
		return err
	}

	if err := updater.UpdateTo(context.Background(), s.latest, ""); err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	slog.Info("update applied", "version", s.latest.Version())
	return nil
}

func (s *UpdateService) AutoUpdate() {
	if Version == "dev" {
		slog.Info("skipping auto-update in dev mode")
		return
	}
	info, err := s.CheckForUpdate()
	if err != nil {
		slog.Warn("auto-update check failed", "error", err)
		return
	}
	if !info.UpdateAvailable {
		slog.Info("no update available")
		return
	}

	slog.Info("update available, applying", "version", info.LatestVersion)
	if err := s.ApplyUpdate(); err != nil {
		slog.Error("auto-update apply failed", "error", err)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "error", err)
		return
	}
	if err := exec.Command(exe).Start(); err != nil {
		slog.Error("failed to restart after update", "error", err)
		return
	}
	s.app.Quit()
}
