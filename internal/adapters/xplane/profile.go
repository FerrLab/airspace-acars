package xplane

import (
	"fmt"
	"log/slog"

	"airspace-acars/internal/profiles"
)

// RREF index ranges. The built-in dataref table owns 0..len(xplaneDatarefs);
// the ranges below sit well above it so the three groups never collide.
const (
	icaoIndexBase    = 500 // sim/aircraft/view/acf_ICAO, one index per character
	descripIndexBase = 520 // sim/aircraft/view/acf_descrip, one index per character
	extraIndexBase   = 1000

	icaoChars    = 8
	descripChars = 48

	identityFreq = 1  // Hz — the loaded aircraft rarely changes
	extraFreq    = 15 // Hz — profile overrides feed 1 Hz position reports
)

// subscribedRef records an extra dataref subscription so it can be cancelled
// when the plan changes.
type subscribedRef struct {
	index   int
	dataref string
}

// SupportedSources reports the binding kinds this adapter can read.
func (x *Adapter) SupportedSources() []profiles.SourceKind {
	return []profiles.SourceKind{profiles.SourceDataRef}
}

// RawIdentity reports the aircraft as X-Plane describes it, before any profile
// has had a chance to rewrite it.
func (x *Adapter) RawIdentity() profiles.Context {
	x.mu.Lock()
	defer x.mu.Unlock()

	engines := 0
	for _, e := range x.data.Engines {
		if e.Exists {
			engines++
		}
	}
	return profiles.Context{
		AircraftName: trimIdentity(x.descripChars[:]),
		AircraftType: trimIdentity(x.icaoChars[:]),
		Simulator:    profiles.SimXPlane,
		EngineCount:  engines,
	}
}

// ApplyProfile installs a resolved plan: the datarefs it needs are subscribed
// to and the previous plan's subscriptions are dropped. A nil plan clears the
// override and returns the adapter to its default data collection.
func (x *Adapter) ApplyProfile(plan *profiles.Plan) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.subscribeExtras(plan)
}

// subscribeExtras swaps the extra dataref subscriptions over to plan.
// Callers hold x.mu.
func (x *Adapter) subscribeExtras(plan *profiles.Plan) error {
	x.unsubscribeExtras()

	x.plan = plan
	x.extraByIdx = map[int]string{}
	x.extraValues = map[string]float64{}
	x.extraRefs = nil

	if plan == nil || x.conn == nil {
		return nil
	}

	var firstErr error
	next := extraIndexBase
	for _, v := range plan.Vars() {
		if v.Kind != profiles.SourceDataRef {
			continue
		}
		if err := x.subscribeRREF(next, extraFreq, v.Name); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("subscribe %s: %w", v.Name, err)
			}
			continue
		}
		x.extraByIdx[next] = v.Key
		x.extraRefs = append(x.extraRefs, subscribedRef{index: next, dataref: v.Name})
		next++
	}

	slog.Info("aircraft profile applied",
		"adapter", x.Name(),
		"profiles", plan.ProfileIDs(),
		"datarefs", len(x.extraByIdx),
		"points", len(plan.Bindings))
	return firstErr
}

// unsubscribeExtras cancels the previous plan's dataref subscriptions.
// Callers hold x.mu.
func (x *Adapter) unsubscribeExtras() {
	if x.conn == nil {
		return
	}
	for _, ref := range x.extraRefs {
		x.subscribeRREF(ref.index, 0, ref.dataref)
	}
}

// subscribeIdentity requests the aircraft ICAO type and description. X-Plane
// exposes both as byte arrays, which the RREF protocol can only deliver one
// element at a time, so each character gets its own low-rate subscription.
// If a build of X-Plane refuses them the fields simply stay empty.
func (x *Adapter) subscribeIdentity() {
	for i := 0; i < icaoChars; i++ {
		x.subscribeRREF(icaoIndexBase+i, identityFreq, fmt.Sprintf("sim/aircraft/view/acf_ICAO[%d]", i))
	}
	for i := 0; i < descripChars; i++ {
		x.subscribeRREF(descripIndexBase+i, identityFreq, fmt.Sprintf("sim/aircraft/view/acf_descrip[%d]", i))
	}
}

// unsubscribeIdentity cancels the identity subscriptions. Callers hold x.mu.
func (x *Adapter) unsubscribeIdentity() {
	for i := 0; i < icaoChars; i++ {
		x.subscribeRREF(icaoIndexBase+i, 0, fmt.Sprintf("sim/aircraft/view/acf_ICAO[%d]", i))
	}
	for i := 0; i < descripChars; i++ {
		x.subscribeRREF(descripIndexBase+i, 0, fmt.Sprintf("sim/aircraft/view/acf_descrip[%d]", i))
	}
}

// handleReading routes one RREF reading to the identity buffers, to a profile
// override, or to the built-in dataref table. Callers hold x.mu.
func (x *Adapter) handleReading(idx int, val float64) {
	switch {
	case idx >= extraIndexBase:
		if key, ok := x.extraByIdx[idx]; ok {
			if x.extraValues == nil {
				x.extraValues = map[string]float64{}
			}
			x.extraValues[key] = val
		}
	case idx >= descripIndexBase && idx < descripIndexBase+descripChars:
		x.descripChars[idx-descripIndexBase] = byteFromReading(val)
		x.data.AircraftName = trimIdentity(x.descripChars[:])
	case idx >= icaoIndexBase && idx < icaoIndexBase+icaoChars:
		x.icaoChars[idx-icaoIndexBase] = byteFromReading(val)
		x.data.AircraftType = trimIdentity(x.icaoChars[:])
	default:
		x.applyDefaultRef(idx, val)
	}
}

// byteFromReading converts one character of a string dataref into a byte,
// dropping anything outside printable ASCII.
func byteFromReading(val float64) byte {
	c := int(val)
	if c < 32 || c > 126 {
		return 0
	}
	return byte(c)
}

// trimIdentity assembles the characters received so far into a string, stopping
// at the first character that has not arrived yet.
func trimIdentity(chars []byte) string {
	for i, c := range chars {
		if c == 0 {
			return string(chars[:i])
		}
	}
	return string(chars)
}
