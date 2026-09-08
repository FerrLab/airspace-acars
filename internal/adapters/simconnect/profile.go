//go:build windows

package simconnect

import (
	"log/slog"
	"unsafe"

	"airspace-acars/internal/profiles"
	sim "airspace-acars/internal/simconnect"
)

// requestIDBase is the first request ID used for profile data. Request 0 is
// the adapter's built-in report.
const requestIDBase = 100

// SupportedSources reports the binding kinds this adapter can read. Since Sim
// Update 12, SimConnect resolves local panel variables itself when the datum
// name is prefixed with "L:", so no module inside the simulator is needed.
func (s *Adapter) SupportedSources() []profiles.SourceKind {
	return []profiles.SourceKind{profiles.SourceSimVar, profiles.SourceLVar}
}

// datumName is the name to register with SimConnect for a plan variable.
// Simulation variables go in as they are; local variables carry the "L:"
// prefix that tells SimConnect to resolve them against the panel system.
func datumName(v profiles.Var) string {
	if v.Kind == profiles.SourceLVar {
		return "L:" + v.Name
	}
	return v.Name
}

// RawIdentity reports the aircraft as SimConnect describes it, before any
// profile has had a chance to rewrite it.
func (s *Adapter) RawIdentity() profiles.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identity
}

// ApplyProfile installs a resolved plan. The SimConnect data definition it
// needs is built on the adapter's own thread, so the plan is only staged here
// and picked up by the next pass of the dispatch loop.
func (s *Adapter) ApplyProfile(plan *profiles.Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingPlan = plan
	s.planPending = true
	return nil
}

// installPendingPlan rebuilds the extra data definition when a new plan has
// been staged. It must run on the thread that owns the SimConnect session.
func (s *Adapter) installPendingPlan(sc *sim.SimConnect) {
	s.mu.Lock()
	if !s.planPending {
		s.mu.Unlock()
		return
	}
	plan := s.pendingPlan
	s.planPending = false
	s.pendingPlan = nil
	oldDefineID, hadDefine := s.extraDefineID, s.extraDefined
	s.mu.Unlock()

	if hadDefine {
		if err := sc.ClearDataDefinition(oldDefineID); err != nil {
			slog.Warn("failed to clear profile data definition", "error", err)
		}
	}

	vars := make([]profiles.Var, 0)
	for _, v := range plan.Vars() {
		if v.Kind == profiles.SourceSimVar || v.Kind == profiles.SourceLVar {
			vars = append(vars, v)
		}
	}

	if len(vars) == 0 {
		s.mu.Lock()
		s.plan = plan
		s.extraDefined = false
		s.extraCount = 0
		s.extraKeys = nil
		s.unproven = nil
		s.extraValues = map[string]float64{}
		s.mu.Unlock()
		slog.Info("aircraft profile applied",
			"adapter", s.Name(),
			"profiles", plan.ProfileIDs(),
			"simvars", 0,
			"points", len(plan.Bindings))
		return
	}

	defineID := sc.AllocDefineID()
	keys := make([]string, 0, len(vars))
	unproven := map[string]bool{}
	lvars := 0
	for _, v := range vars {
		if err := sc.AddToDataDefinition(defineID, datumName(v), v.Unit, sim.DATATYPE_FLOAT64); err != nil {
			slog.Warn("profile variable rejected by SimConnect", "name", datumName(v), "unit", v.Unit, "error", err)
			continue
		}
		keys = append(keys, v.Key)
		if v.Kind == profiles.SourceLVar {
			unproven[v.Key] = true
			lvars++
		}
	}

	s.mu.Lock()
	s.plan = plan
	s.extraDefined = true
	s.extraDefineID = defineID
	s.extraRequestID = s.nextRequestID()
	s.extraKeys = keys
	s.extraCount = len(keys)
	s.unproven = unproven
	s.extraValues = map[string]float64{}
	s.mu.Unlock()

	slog.Info("aircraft profile applied",
		"adapter", s.Name(),
		"profiles", plan.ProfileIDs(),
		"simvars", len(keys)-lvars,
		"lvars", lvars,
		"points", len(plan.Bindings))
}

// nextRequestID hands out a fresh request ID for each definition so replies to
// a superseded plan are ignored. Callers hold s.mu.
func (s *Adapter) nextRequestID() sim.DWORD {
	s.requestSeq++
	return requestIDBase + sim.DWORD(s.requestSeq)
}

// readExtras stores the readings of a profile data dispatch.
func (s *Adapter) readExtras(ppData unsafe.Pointer, hdr *sim.RecvSimobjectDataByType) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.extraCount == 0 || hdr.RequestID != s.extraRequestID {
		return
	}
	count := int(hdr.DefineCount)
	if count > s.extraCount {
		count = s.extraCount
	}
	if count <= 0 {
		return
	}

	base := unsafe.Add(ppData, unsafe.Sizeof(sim.RecvSimobjectDataByType{}))
	readings := unsafe.Slice((*float64)(base), count)
	if s.extraValues == nil {
		s.extraValues = map[string]float64{}
	}
	for i, key := range s.extraKeys[:count] {
		// SimConnect creates a local variable that does not exist rather than
		// rejecting it, so a name that is wrong — a typo, or a variable from a
		// different version of the add-on — reads a permanent zero instead of
		// failing. A local variable is therefore withheld until it has been
		// seen non-zero once: until it proves it is real the data point keeps
		// whatever the adapter read for it by its own route.
		if s.unproven[key] {
			if readings[i] == 0 {
				continue
			}
			delete(s.unproven, key)
			slog.Debug("profile local variable is live", "key", key)
		}
		s.extraValues[key] = readings[i]
	}
}
