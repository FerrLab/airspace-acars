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

// SupportedSources reports the binding kinds this adapter can read. Local
// panel variables ("L:" vars) need a WASM bridge inside the simulator, which
// is not wired up yet, so a profile binding that asks for one falls through to
// its next candidate — usually the stock simulation variable.
func (s *Adapter) SupportedSources() []profiles.SourceKind {
	return []profiles.SourceKind{profiles.SourceSimVar}
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
		if v.Kind == profiles.SourceSimVar {
			vars = append(vars, v)
		}
	}

	if len(vars) == 0 {
		s.mu.Lock()
		s.plan = plan
		s.extraDefined = false
		s.extraCount = 0
		s.extraKeys = nil
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
	for _, v := range vars {
		if err := sc.AddToDataDefinition(defineID, v.Name, v.Unit, sim.DATATYPE_FLOAT64); err != nil {
			slog.Warn("profile variable rejected by SimConnect", "name", v.Name, "unit", v.Unit, "error", err)
			continue
		}
		keys = append(keys, v.Key)
	}

	s.mu.Lock()
	s.plan = plan
	s.extraDefined = true
	s.extraDefineID = defineID
	s.extraRequestID = s.nextRequestID()
	s.extraKeys = keys
	s.extraCount = len(keys)
	s.extraValues = map[string]float64{}
	s.mu.Unlock()

	slog.Info("aircraft profile applied",
		"adapter", s.Name(),
		"profiles", plan.ProfileIDs(),
		"simvars", len(keys),
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
		s.extraValues[key] = readings[i]
	}
}
