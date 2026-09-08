// Package profiles implements aircraft-specific ACARS profiles.
//
// A profile is a JSON document with two halves:
//
//   - match — a boolean expression (AND/OR/NOT groups over leaf conditions)
//     that decides whether the profile applies to the aircraft currently
//     loaded in the simulator;
//   - mash — a set of overrides that change *how* individual data points are
//     collected: which simulator variable feeds a field of domain.FlightData
//     and how the raw number is converted before it lands there.
//
// Profiles are resolved into a Plan (see plan.go), which the simulator
// adapters use to subscribe to the extra variables and to rewrite the data
// points they feed.
package profiles

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Simulator identifiers used by match conditions and by binding scopes.
const (
	SimAny        = ""
	SimSimConnect = "simconnect"
	SimXPlane     = "xplane"
)

// SourceKind identifies the transport a binding reads its value from.
type SourceKind string

const (
	// SourceSimVar is an MSFS SimConnect simulation variable ("A:" var).
	SourceSimVar SourceKind = "simvar"
	// SourceLVar is an MSFS local (panel) variable ("L:" var). Reading these
	// requires a WASM bridge in the simulator; adapters that cannot read them
	// report the kind as unsupported and the next candidate binding is used.
	SourceLVar SourceKind = "lvar"
	// SourceDataRef is an X-Plane dataref.
	SourceDataRef SourceKind = "dataref"
	// SourceConst is a fixed value carried by the profile itself.
	SourceConst SourceKind = "const"
)

// Profile is a single aircraft profile as stored on disk.
type Profile struct {
	// Schema is an optional "$schema" key so editors can offer completion.
	Schema      string `json:"$schema,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	// Generated marks a profile drafted by cmd/hubhop from the community
	// variable database rather than confirmed against the aircraft.
	Generated bool       `json:"x-generated,omitempty"`
	Match     *MatchNode `json:"match,omitempty"`
	Mash      MashMap    `json:"mash,omitempty"`

	// Origin records where the profile was loaded from ("builtin" or a path).
	Origin string `json:"-"`
}

// MashMap maps a data point ID (see points.go) to the way it is collected.
type MashMap map[string]MashEntry

// MashEntry describes how one data point is collected. By default its bindings
// are candidates and the first one usable on the active simulator wins, which
// is how a profile states "read this local variable, and fall back to the stock
// simulation variable". With a Reduce set, every usable binding is read and
// their values are combined instead — "autopilot engaged is AP1 or AP2".
//
// JSON accepts three shapes:
//
//	"autopilot.master": {"source": …}                       one binding
//	"autopilot.master": [{"source": …}, {"source": …}]       candidates
//	"autopilot.master": {"reduce": "or", "bindings": [ … ]}  combined
type MashEntry struct {
	Reduce   string     `json:"reduce,omitempty"`
	Bindings BindingSet `json:"bindings"`
}

// BindingSet is an ordered list of bindings for one data point.
type BindingSet []Binding

// UnmarshalJSON accepts a single binding, an array of bindings, or the full
// object form with a reduce operator.
func (m *MashEntry) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var list []Binding
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		m.Bindings = list
		return nil
	}

	// Distinguish the full object form from a bare binding by its keys.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if _, full := probe["bindings"]; full {
		type entry MashEntry
		var e entry
		if err := json.Unmarshal(data, &e); err != nil {
			return err
		}
		*m = MashEntry(e)
		return nil
	}

	var single Binding
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	m.Bindings = BindingSet{single}
	return nil
}

// Binding describes one way of collecting a data point.
type Binding struct {
	// Sim restricts the binding to a simulator ("simconnect", "xplane").
	// Empty means it applies to any simulator.
	Sim string `json:"sim,omitempty"`
	// Source is where the raw value comes from.
	Source Source `json:"source"`
	// Transform is applied to the raw value, in order, before it is stored.
	Transform []Step `json:"transform,omitempty"`
}

// Source identifies the variable that feeds a data point.
type Source struct {
	Kind  SourceKind `json:"kind"`
	Name  string     `json:"name,omitempty"`
	Unit  string     `json:"unit,omitempty"`
	Value *Literal   `json:"value,omitempty"`
}

// Key returns a stable identifier for the underlying variable. Adapters use it
// to deduplicate subscriptions and to index the values they collect.
func (s Source) Key() string {
	if s.Kind == SourceConst {
		return "const:" + s.Value.String()
	}
	return string(s.Kind) + ":" + s.Name + ":" + s.Unit
}

// Literal is a constant JSON scalar (number, string or boolean).
type Literal struct {
	Num      float64
	Str      string
	IsString bool
}

// UnmarshalJSON decodes a JSON number, string or boolean into a Literal.
func (l *Literal) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case float64:
		l.Num, l.Str, l.IsString = t, "", false
	case string:
		l.Num, l.Str, l.IsString = 0, t, true
	case bool:
		l.IsString = false
		l.Str = ""
		if t {
			l.Num = 1
		} else {
			l.Num = 0
		}
	default:
		return fmt.Errorf("value must be a number, string or boolean, got %T", v)
	}
	return nil
}

// MarshalJSON writes the literal back in its original scalar form.
func (l Literal) MarshalJSON() ([]byte, error) {
	if l.IsString {
		return json.Marshal(l.Str)
	}
	return json.Marshal(l.Num)
}

// String renders the literal for keys and log messages.
func (l Literal) String() string {
	if l.IsString {
		return l.Str
	}
	return fmt.Sprintf("%g", l.Num)
}

// Validate checks that a profile is structurally sound: it has an ID, every
// mashed data point exists in the catalog, every source and transform step is
// understood, and the match expression uses known fields and operators.
func (p *Profile) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("profile id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile %q: name is required", p.ID)
	}
	if p.Match != nil {
		if err := p.Match.Validate(); err != nil {
			return fmt.Errorf("profile %q: match: %w", p.ID, err)
		}
	}
	for point, entry := range p.Mash {
		pt, ok := Lookup(point)
		if !ok {
			return fmt.Errorf("profile %q: unknown data point %q", p.ID, point)
		}
		if len(entry.Bindings) == 0 {
			return fmt.Errorf("profile %q: data point %q has no bindings", p.ID, point)
		}
		if err := validateReduce(entry.Reduce, pt); err != nil {
			return fmt.Errorf("profile %q: data point %q: %w", p.ID, point, err)
		}
		for i, b := range entry.Bindings {
			if err := b.validate(pt); err != nil {
				return fmt.Errorf("profile %q: data point %q binding %d: %w", p.ID, point, i, err)
			}
		}
	}
	return nil
}

func (b Binding) validate(pt Point) error {
	switch b.Sim {
	case SimAny, SimSimConnect, SimXPlane:
	default:
		return fmt.Errorf("unknown simulator %q", b.Sim)
	}
	switch b.Source.Kind {
	case SourceSimVar, SourceLVar, SourceDataRef:
		if strings.TrimSpace(b.Source.Name) == "" {
			return fmt.Errorf("source of kind %q requires a name", b.Source.Kind)
		}
	case SourceConst:
		if b.Source.Value == nil {
			return fmt.Errorf("source of kind %q requires a value", b.Source.Kind)
		}
		if b.Source.Value.IsString && pt.Kind != KindString {
			return fmt.Errorf("string constant cannot feed the numeric data point %q", pt.ID)
		}
	default:
		return fmt.Errorf("unknown source kind %q", b.Source.Kind)
	}
	if pt.Kind == KindString && b.Source.Kind != SourceConst {
		return fmt.Errorf("data point %q is textual and can only be fed by a const source", pt.ID)
	}
	for i, s := range b.Transform {
		if err := s.validate(); err != nil {
			return fmt.Errorf("transform step %d: %w", i, err)
		}
	}
	return nil
}

// Reduce operators combine every usable binding of a data point.
const (
	ReduceFirst = ""    // no combination: the first usable binding wins
	ReduceOr    = "or"  // 1 when any reading is non-zero
	ReduceAnd   = "and" // 1 when every reading is non-zero
	ReduceMax   = "max"
	ReduceMin   = "min"
	ReduceSum   = "sum"
)

func validateReduce(reduce string, pt Point) error {
	switch reduce {
	case ReduceFirst:
		return nil
	case ReduceOr, ReduceAnd, ReduceMax, ReduceMin, ReduceSum:
		if pt.Kind == KindString {
			return fmt.Errorf("reduce %q cannot be used on the textual data point %q", reduce, pt.ID)
		}
		return nil
	default:
		return fmt.Errorf("unknown reduce operator %q", reduce)
	}
}
