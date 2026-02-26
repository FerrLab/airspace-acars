package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/creativeprojects/go-selfupdate"
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
}

func (s *UpdateService) GetCurrentVersion() string {
	return Version
}

func (s *UpdateService) isStableRelease() bool {
	return Version != "dev" && !strings.Contains(Version, "-beta")
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

	latest, found, err := updater.DetectLatest(context.Background(), selfupdate.ParseSlug("FerrLab/airspace-acars"))
	if err != nil {
		return nil, fmt.Errorf("failed to detect latest version: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion:  Version,
		UpdateAvailable: false,
	}

	if found {
		info.LatestVersion = latest.Version()
		info.ReleaseURL = latest.ReleaseNotes
		if latest.GreaterThan(s.comparableVersion()) {
			info.UpdateAvailable = true
			s.latest = latest
		}
	}

	slog.Info("update check complete", "current", Version, "latest", info.LatestVersion, "available", info.UpdateAvailable)
	return info, nil
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
