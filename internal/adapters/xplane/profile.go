package xplane

import (
	"fmt"
	"log/slog"
	"strings"

	"airspace-acars/internal/profiles"
)

// RREF index ranges. The built-in dataref table owns 0..len(xplaneDatarefs);
// the ranges below sit well above it so the groups never collide.
const (
	icaoIndexBase    = 500 // sim/aircraft/view/acf_ICAO, one index per character
	descripIndexBase = 520 // sim/aircraft/view/acf_descrip, one index per character
	uiNameIndexBase  = 600 // sim/aircraft/view/acf_ui_name, one index per character
	authorIndexBase  = 700 // sim/aircraft/view/acf_author, one index per character
	extraIndexBase   = 1000

	icaoChars    = 8
	descripChars = 48
	// The name is sent as each position's aircraft name, which the server
	// keeps in a 100-character column: the author, a space and the UI name
	// together stay inside it.
	uiNameChars = 64
	authorChars = 32

	identityFreq = 1  // Hz — the loaded aircraft rarely changes
	extraFreq    = 15 // Hz — profile overrides feed 1 Hz position reports
)

// identityString is one string dataref the aircraft identity is read from.
type identityString struct {
	dataref string
	base    int
	chars   int
}

var identityStrings = []identityString{
	{"sim/aircraft/view/acf_ICAO", icaoIndexBase, icaoChars},
	{"sim/aircraft/view/acf_descrip", descripIndexBase, descripChars},
	{"sim/aircraft/view/acf_ui_name", uiNameIndexBase, uiNameChars},
	{"sim/aircraft/view/acf_author", authorIndexBase, authorChars},
}

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
		AircraftName: x.aircraftName(),
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
	x.unproven = map[string]bool{}

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
		if !strings.HasPrefix(v.Name, "sim/") {
			x.unproven[v.Key] = true
		}
		next++
	}

	slog.Info("aircraft profile applied",
		"adapter", x.Name(),
		"profiles", plan.ProfileIDs(),
		"datarefs", len(x.extraByIdx),
		"points", plan.PointCount())
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

// subscribeIdentity requests the aircraft's ICAO type, description, UI name and
// author. X-Plane exposes them as byte arrays, which the RREF protocol can only
// deliver one element at a time, so each character gets its own low-rate
// subscription. If a build of X-Plane refuses one — X-Plane 11 has no UI name —
// that field simply stays empty.
func (x *Adapter) subscribeIdentity() {
	for _, s := range identityStrings {
		for i := 0; i < s.chars; i++ {
			x.subscribeRREF(s.base+i, identityFreq, fmt.Sprintf("%s[%d]", s.dataref, i))
		}
	}
}

// unsubscribeIdentity cancels the identity subscriptions. Callers hold x.mu.
func (x *Adapter) unsubscribeIdentity() {
	for _, s := range identityStrings {
		for i := 0; i < s.chars; i++ {
			x.subscribeRREF(s.base+i, 0, fmt.Sprintf("%s[%d]", s.dataref, i))
		}
	}
}

// aircraftName is the name the loaded aircraft is reported and matched under.
// Callers hold x.mu.
func (x *Adapter) aircraftName() string {
	return composeAircraftName(
		trimIdentity(x.authorChars[:]),
		trimIdentity(x.uiNameChars[:]),
		trimIdentity(x.descripChars[:]))
}

// composeAircraftName builds X-Plane's counterpart of MSFS's TITLE, which names
// the add-on as well as the aircraft ("Fenix A320 IAE Lufthansa") and is what
// profile selectors match. X-Plane 12 lists each aircraft under a UI name;
// X-Plane 11 has only the author's one-line description, such as "A320 with
// high fidelity system modelling". Neither reliably names the developer, so
// the author goes in front unless the name already carries it.
func composeAircraftName(author, uiName, descrip string) string {
	author = strings.TrimSpace(author)
	name := strings.TrimSpace(uiName)
	if name == "" {
		name = strings.TrimSpace(descrip)
	}

	words := strings.Fields(author)
	switch {
	case len(words) == 0:
		return name
	case name == "":
		return author
	case strings.Contains(strings.ToLower(name), strings.ToLower(words[0])):
		return name
	default:
		return author + " " + name
	}
}

// handleReading routes one RREF reading to the identity buffers, to a profile
// override, or to the built-in dataref table. Callers hold x.mu.
func (x *Adapter) handleReading(idx int, val float64) {
	switch {
	case idx >= extraIndexBase:
		key, ok := x.extraByIdx[idx]
		if !ok {
			return
		}
		// A dataref X-Plane cannot resolve does not fail its subscription: it
		// reports nothing, or a permanent zero. So, as the SimConnect adapter
		// does for local variables, an aircraft's own dataref is withheld until
		// it has been seen non-zero once: until it proves it is real, the data
		// point keeps what the adapter read by its own route. Stock datarefs
		// always exist, and their zeros count from the start.
		if x.unproven[key] {
			if val == 0 {
				return
			}
			delete(x.unproven, key)
			slog.Debug("profile dataref is live", "key", key)
		}
		if x.extraValues == nil {
			x.extraValues = map[string]float64{}
		}
		x.extraValues[key] = val
	case idx >= authorIndexBase && idx < authorIndexBase+authorChars:
		x.authorChars[idx-authorIndexBase] = byteFromReading(val)
		x.data.AircraftName = x.aircraftName()
	case idx >= uiNameIndexBase && idx < uiNameIndexBase+uiNameChars:
		x.uiNameChars[idx-uiNameIndexBase] = byteFromReading(val)
		x.data.AircraftName = x.aircraftName()
	case idx >= descripIndexBase && idx < descripIndexBase+descripChars:
		x.descripChars[idx-descripIndexBase] = byteFromReading(val)
		x.data.AircraftName = x.aircraftName()
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
