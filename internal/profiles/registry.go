package profiles

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed builtin/*.json
var builtinFS embed.FS

// OriginBuiltin marks profiles that ship with the application.
const OriginBuiltin = "builtin"

// Aware is implemented by simulator adapters that can collect the extra
// variables a plan asks for. Adapters that do not implement it simply ignore
// profiles, so the feature degrades to the default data collection.
type Aware interface {
	// ApplyProfile installs a plan. A nil plan clears any active one.
	ApplyProfile(plan *Plan) error
	// RawIdentity reports the aircraft as the simulator sees it, before any
	// profile rewrites it. Matching uses these values so that a profile which
	// mashes aircraft.type cannot change what it matches against.
	RawIdentity() Context
	// SupportedSources lists the source kinds the adapter can read.
	SupportedSources() []SourceKind
}

// Info is a profile summary for the UI and for logs.
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Priority    int    `json:"priority"`
	Disabled    bool   `json:"disabled"`
	Origin      string `json:"origin"`
	Points      int    `json:"points"`
	Generated   bool   `json:"generated"`
}

// Registry holds the loaded profiles: the built-in set plus anything the user
// dropped into the profiles directory. A user file that reuses a built-in ID
// replaces it, which is how a pilot overrides a shipped profile.
type Registry struct {
	mu       sync.RWMutex
	profiles map[string]*Profile
	dir      string
	loadErrs []string
}

// NewRegistry loads the built-in profiles.
func NewRegistry() *Registry {
	r := &Registry{profiles: map[string]*Profile{}}
	if err := r.loadBuiltin(); err != nil {
		// Built-in profiles are validated by a test, so this only fires if the
		// embedded files are corrupt in a build.
		slog.Error("failed to load built-in aircraft profiles", "error", err)
	}
	return r
}

func (r *Registry) loadBuiltin() error {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := builtinFS.ReadFile(path("builtin", entry.Name()))
		if err != nil {
			return err
		}
		prof, err := Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
		prof.Origin = OriginBuiltin
		r.profiles[prof.ID] = prof
	}
	return nil
}

// path joins embed.FS elements, which always use forward slashes.
func path(parts ...string) string { return strings.Join(parts, "/") }

// LoadDir reads every *.json profile in dir, replacing built-ins that share an
// ID. A file that fails to parse or validate is skipped and reported; the rest
// still load, so one bad profile cannot stop the ACARS from starting.
func (r *Registry) LoadDir(dir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.dir = dir
	r.loadErrs = nil

	// Start from a clean built-in set so reloading drops removed user files.
	r.profiles = map[string]*Profile{}
	if err := r.loadBuiltin(); err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read profiles dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			r.loadErrs = append(r.loadErrs, fmt.Sprintf("%s: %v", entry.Name(), err))
			slog.Warn("skipping aircraft profile", "file", full, "error", err)
			continue
		}
		prof, err := Parse(data)
		if err != nil {
			r.loadErrs = append(r.loadErrs, fmt.Sprintf("%s: %v", entry.Name(), err))
			slog.Warn("skipping invalid aircraft profile", "file", full, "error", err)
			continue
		}
		prof.Origin = full
		r.profiles[prof.ID] = prof
		slog.Info("loaded aircraft profile", "id", prof.ID, "name", prof.Name, "file", full)
	}
	return nil
}

// Parse decodes and validates a single profile document.
func Parse(data []byte) (*Profile, error) {
	var prof Profile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&prof); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := prof.Validate(); err != nil {
		return nil, err
	}
	return &prof, nil
}

// LoadErrors returns the messages for profiles skipped by the last LoadDir.
func (r *Registry) LoadErrors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.loadErrs...)
}

// Dir returns the user profile directory the registry last read.
func (r *Registry) Dir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dir
}

// All returns every loaded profile, sorted by ID.
func (r *Registry) All() []*Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Profile, 0, len(r.profiles))
	for _, prof := range r.profiles {
		out = append(out, prof)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// List summarises the loaded profiles for the UI.
func (r *Registry) List() []Info {
	profs := r.All()
	out := make([]Info, 0, len(profs))
	for _, prof := range profs {
		out = append(out, Info{
			ID:          prof.ID,
			Name:        prof.Name,
			Description: prof.Description,
			Notes:       prof.Notes,
			Priority:    prof.Priority,
			Disabled:    prof.Disabled,
			Origin:      prof.Origin,
			Points:      len(prof.Mash),
			Generated:   prof.Generated,
		})
	}
	return out
}

// Get returns a profile by ID.
func (r *Registry) Get(id string) (*Profile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prof, ok := r.profiles[id]
	return prof, ok
}

// Resolve merges every profile matching ctx into a plan.
func (r *Registry) Resolve(ctx Context, supported []SourceKind) *Plan {
	return Resolve(r.All(), ctx, supported)
}

// ResolveForced builds a plan from one specific profile, ignoring its selector.
// It backs the manual profile override in settings, which is how a pilot pins a
// profile for an aircraft the ACARS cannot identify on its own.
func (r *Registry) ResolveForced(id string, ctx Context, supported []SourceKind) (*Plan, error) {
	prof, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("unknown aircraft profile %q", id)
	}
	forced := *prof
	forced.Match = nil
	forced.Disabled = false
	return Resolve([]*Profile{&forced}, ctx, supported), nil
}

// Signature identifies the aircraft/simulator combination a plan was resolved
// for. The data stream compares it every tick and only re-resolves on a change.
func (c Context) Signature() string {
	return fmt.Sprintf("%s|%s|%s|%d", c.Simulator, c.AircraftName, c.AircraftType, c.EngineCount)
}
