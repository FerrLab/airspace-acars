package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"airspace-acars/internal/domain"
	"airspace-acars/internal/profiles"
)

// profileAuto and profileOff are the reserved values of Settings.AircraftProfile.
const (
	profileAuto = "auto"
	profileOff  = "off"
)

// InitProfiles loads the built-in aircraft profiles and any the user dropped
// into the profiles directory next to settings.json.
func (a *App) InitProfiles() {
	configDir, _ := os.UserConfigDir()
	a.profilesDir = filepath.Join(configDir, "airspace-acars", "profiles")

	registry := profiles.NewRegistry()
	if err := registry.LoadDir(a.profilesDir); err != nil {
		slog.Warn("failed to read user aircraft profiles", "dir", a.profilesDir, "error", err)
	}

	a.profileMu.Lock()
	a.profileRegistry = registry
	a.profileMu.Unlock()

	slog.Info("aircraft profiles loaded", "count", len(registry.All()), "dir", a.profilesDir)
}

// ProfilesDir returns the directory user profiles are read from.
func (a *App) ProfilesDir() string { return a.profilesDir }

// ListProfiles returns every loaded profile.
func (a *App) ListProfiles() []profiles.Info {
	a.profileMu.RLock()
	registry := a.profileRegistry
	a.profileMu.RUnlock()
	if registry == nil {
		return nil
	}
	return registry.List()
}

// ReloadProfiles re-reads the user profile directory. The active plan is
// dropped so the next tick of the data stream resolves against the new set.
func (a *App) ReloadProfiles() error {
	a.profileMu.RLock()
	registry := a.profileRegistry
	a.profileMu.RUnlock()
	if registry == nil {
		return fmt.Errorf("aircraft profiles are not initialised")
	}

	if err := registry.LoadDir(a.profilesDir); err != nil {
		return err
	}

	a.profileMu.Lock()
	a.activePlan = nil
	a.activeProfileSig = ""
	a.profileMu.Unlock()

	if errs := registry.LoadErrors(); len(errs) > 0 {
		return fmt.Errorf("%d profile(s) skipped: %v", len(errs), errs)
	}
	return nil
}

// GetActiveProfile returns the plan in force for the aircraft currently loaded,
// or nil when no profile matches.
func (a *App) GetActiveProfile() *profiles.Plan {
	a.profileMu.RLock()
	defer a.profileMu.RUnlock()
	return a.activePlan
}

// refreshAircraftProfile re-resolves the aircraft profile when the aircraft,
// the simulator or the profile setting has changed, and hands the resulting
// plan to the adapter. Called once per tick from dataStreamLoop.
func (a *App) refreshAircraftProfile(connector domain.SimConnector) {
	aware, ok := connector.(profiles.Aware)
	if !ok {
		return
	}

	a.profileMu.RLock()
	registry := a.profileRegistry
	previous := a.activeProfileSig
	a.profileMu.RUnlock()
	if registry == nil {
		return
	}

	ctx := aware.RawIdentity()
	setting := a.GetSettings().AircraftProfile
	if setting == "" {
		setting = profileAuto
	}
	signature := setting + "@" + ctx.Signature()
	if signature == previous {
		return
	}

	var plan *profiles.Plan
	switch setting {
	case profileOff:
		plan = nil
	case profileAuto:
		plan = registry.Resolve(ctx, aware.SupportedSources())
		if plan.Empty() {
			plan = nil
		}
	default:
		forced, err := registry.ResolveForced(setting, ctx, aware.SupportedSources())
		if err != nil {
			slog.Warn("aircraft profile override not applied", "profile", setting, "error", err)
			plan = nil
		} else {
			plan = forced
		}
	}

	if err := aware.ApplyProfile(plan); err != nil {
		slog.Warn("failed to apply aircraft profile", "error", err)
	}

	a.profileMu.Lock()
	a.activePlan = plan
	a.activeProfileSig = signature
	a.profileMu.Unlock()

	if plan == nil {
		slog.Info("no aircraft profile matched",
			"aircraft", ctx.AircraftName, "type", ctx.AircraftType, "sim", ctx.Simulator)
	} else {
		slog.Info("aircraft profile selected",
			"aircraft", ctx.AircraftName,
			"type", ctx.AircraftType,
			"sim", ctx.Simulator,
			"profiles", plan.ProfileIDs(),
			"points", len(plan.Bindings),
			"skipped", plan.Skipped)
	}
	a.UI.EmitEvent("aircraft-profile", plan)
}

// clearAircraftProfile forgets the active plan, so the next connection
// resolves from scratch.
func (a *App) clearAircraftProfile() {
	a.profileMu.Lock()
	a.activePlan = nil
	a.activeProfileSig = ""
	a.profileMu.Unlock()
}
