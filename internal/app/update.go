package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var updateTracer = observability.Tracer("update")

// GetCurrentVersion returns the app version.
func (a *App) GetCurrentVersion() string {
	return domain.Version
}

func (a *App) isBeta() bool {
	return strings.Contains(domain.Version, "-beta")
}

func (a *App) isStableRelease() bool {
	return domain.Version != "dev" && !a.isBeta()
}

func (a *App) comparableVersion() string {
	if domain.Version == "dev" {
		return "0.0.0"
	}
	return domain.Version
}

func assetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	// macOS ships as a universal binary (arm64 + amd64)
	if os == "darwin" {
		arch = "universal"
	}
	name := fmt.Sprintf("airspace-acars-%s-%s", os, arch)
	if os == "windows" {
		name += ".exe"
	}
	return name
}

func assetFilter() string {
	return assetName() + "$"
}

func (a *App) newUpdater() (*selfupdate.Updater, error) {
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create github source: %w", err)
	}

	cfg := selfupdate.Config{
		Source:  source,
		Filters: []string{assetFilter()},
	}
	if !a.isStableRelease() {
		cfg.Prerelease = true
	}

	updater, err := selfupdate.NewUpdater(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create updater: %w", err)
	}
	return updater, nil
}

// CheckForUpdate checks GitHub for a newer version.
func (a *App) CheckForUpdate() (*domain.UpdateInfo, error) {
	_, span := updateTracer.Start(context.Background(), "update.check",
		trace.WithAttributes(attribute.String("update.current_version", domain.Version)))
	defer span.End()

	updater, err := a.newUpdater()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	ctx := context.Background()
	slug := selfupdate.ParseSlug("FerrLab/airspace-acars")

	info := &domain.UpdateInfo{
		CurrentVersion:  domain.Version,
		UpdateAvailable: false,
	}

	if a.isBeta() {
		betaVersion, err := a.findLatestBetaVersion(ctx)
		if err != nil {
			slog.Warn("update check skipped (could not list beta releases)", "error", err)
			span.SetAttributes(attribute.Bool("update.skipped", true))
			return info, nil
		}
		if betaVersion == "" {
			slog.Info("no beta releases found")
			span.SetAttributes(attribute.Bool("update.available", info.UpdateAvailable))
			return info, nil
		}

		latest, found, err := updater.DetectVersion(ctx, slug, betaVersion)
		if err != nil || !found {
			if err != nil {
				slog.Warn("update check skipped (could not detect beta version)", "error", err)
				span.SetAttributes(attribute.Bool("update.skipped", true))
			}
			return info, nil
		}

		info.LatestVersion = latest.Version()
		info.ReleaseURL = latest.ReleaseNotes
		if latest.GreaterThan(a.comparableVersion()) {
			info.UpdateAvailable = true
			a.updateLatest = latest
		}
	} else {
		latest, found, err := updater.DetectLatest(ctx, slug)
		if err != nil {
			slog.Warn("update check skipped (could not detect latest version)", "error", err)
			span.SetAttributes(attribute.Bool("update.skipped", true))
			return info, nil
		}
		if found {
			info.LatestVersion = latest.Version()
			info.ReleaseURL = latest.ReleaseNotes
			if latest.GreaterThan(a.comparableVersion()) {
				info.UpdateAvailable = true
				a.updateLatest = latest
			}
		}
	}

	slog.Info("update check complete", "current", domain.Version, "latest", info.LatestVersion, "available", info.UpdateAvailable)
	span.SetAttributes(attribute.Bool("update.available", info.UpdateAvailable))
	return info, nil
}

func (a *App) findLatestBetaVersion(ctx context.Context) (string, error) {
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

		hasAsset := false
		expected := assetName()
		for _, asset := range rel.GetAssets() {
			if asset.GetName() == expected {
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
			bestTag = tag
		}
	}

	return bestTag, nil
}

// ApplyUpdate downloads and applies the latest update.
func (a *App) ApplyUpdate() error {
	if a.updateLatest == nil {
		return fmt.Errorf("no update available — run CheckForUpdate first")
	}

	updater, err := a.newUpdater()
	if err != nil {
		return err
	}

	if err := updater.UpdateTo(context.Background(), a.updateLatest, ""); err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	slog.Info("update applied", "version", a.updateLatest.Version())
	return nil
}

// AutoUpdate checks for and applies updates on startup.
func (a *App) AutoUpdate() {
	if domain.Version == "dev" {
		slog.Info("skipping auto-update in dev mode")
		return
	}
	info, err := a.CheckForUpdate()
	if err != nil {
		slog.Warn("auto-update check failed", "error", err)
		return
	}
	if !info.UpdateAvailable {
		slog.Info("no update available")
		return
	}

	slog.Info("update available, applying", "version", info.LatestVersion)
	if err := a.ApplyUpdate(); err != nil {
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
	if a.QuitFunc != nil {
		a.QuitFunc()
	}
}
