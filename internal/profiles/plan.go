package profiles

import (
	"sort"
	"strings"

	"airspace-acars/internal/domain"
)

// Var is one simulator variable an adapter must collect for a plan.
type Var struct {
	Key  string     `json:"key"`
	Kind SourceKind `json:"kind"`
	Name string     `json:"name"`
	Unit string     `json:"unit,omitempty"`
}

// ResolvedSource is one variable feeding a resolved data point.
type ResolvedSource struct {
	Kind      SourceKind `json:"kind"`
	Name      string     `json:"name,omitempty"`
	Unit      string     `json:"unit,omitempty"`
	Key       string     `json:"key"`
	Value     *Literal   `json:"value,omitempty"`
	Transform []Step     `json:"-"`
}

// ResolvedBinding is how a data point is collected once the profiles that
// match have been merged.
type ResolvedBinding struct {
	Point     string           `json:"point"`
	ProfileID string           `json:"profileId"`
	Reduce    string           `json:"reduce,omitempty"`
	Sources   []ResolvedSource `json:"sources"`
}

// AppliedProfile identifies a profile that contributed to a plan.
type AppliedProfile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Origin   string `json:"origin"`
}

// Plan is the resolved instruction set for one aircraft on one simulator: the
// profiles that matched, the data points they mash, and the variables the
// adapter has to subscribe to in order to feed them.
type Plan struct {
	Simulator string            `json:"simulator"`
	Aircraft  Context           `json:"aircraft"`
	Profiles  []AppliedProfile  `json:"profiles"`
	Bindings  []ResolvedBinding `json:"bindings"`
	// Skipped lists bindings that were dropped because no candidate was
	// supported by the active adapter, so the reason is visible to the user.
	Skipped []string `json:"skipped,omitempty"`

	vars []Var
}

// Empty reports whether the plan changes nothing.
func (p *Plan) Empty() bool { return p == nil || len(p.Bindings) == 0 }

// Vars returns the deduplicated, non-constant variables to subscribe to.
func (p *Plan) Vars() []Var {
	if p == nil {
		return nil
	}
	return p.vars
}

// ProfileIDs returns the IDs of the profiles that contributed, in apply order.
func (p *Plan) ProfileIDs() []string {
	if p == nil {
		return nil
	}
	ids := make([]string, 0, len(p.Profiles))
	for _, prof := range p.Profiles {
		ids = append(ids, prof.ID)
	}
	return ids
}

// Describe renders the plan for logs: "Fenix A320, FBW A32NX (7 points)".
func (p *Plan) Describe() string {
	if p == nil || len(p.Profiles) == 0 {
		return "none"
	}
	names := make([]string, 0, len(p.Profiles))
	for _, prof := range p.Profiles {
		names = append(names, prof.Name)
	}
	return strings.Join(names, ", ")
}

// Apply rewrites the data points the plan mashes. values holds the readings the
// adapter collected, keyed by Var.Key; a data point whose variable has not been
// received yet keeps the value the adapter computed by its default route.
func (p *Plan) Apply(fd *domain.FlightData, values map[string]float64) {
	if p == nil || fd == nil {
		return
	}
	for _, b := range p.Bindings {
		point, ok := Lookup(b.Point)
		if !ok {
			continue
		}

		readings := make([]float64, 0, len(b.Sources))
		for _, src := range b.Sources {
			if src.Kind == SourceConst {
				if src.Value == nil {
					continue
				}
				if src.Value.IsString {
					point.Set(fd, Value{Str: src.Value.Str, IsString: true})
					readings = nil
					break
				}
				readings = append(readings, applySteps(src.Transform, src.Value.Num))
				continue
			}
			raw, ok := values[src.Key]
			if !ok {
				continue
			}
			readings = append(readings, applySteps(src.Transform, raw))
			if b.Reduce == ReduceFirst {
				break
			}
		}
		if len(readings) == 0 {
			continue
		}
		point.Set(fd, Value{Num: reduce(b.Reduce, readings)})
	}
}

// reduce combines the readings of a data point fed by several variables.
func reduce(op string, readings []float64) float64 {
	switch op {
	case ReduceOr:
		for _, r := range readings {
			if r != 0 {
				return 1
			}
		}
		return 0
	case ReduceAnd:
		for _, r := range readings {
			if r == 0 {
				return 0
			}
		}
		return 1
	case ReduceMax:
		out := readings[0]
		for _, r := range readings[1:] {
			if r > out {
				out = r
			}
		}
		return out
	case ReduceMin:
		out := readings[0]
		for _, r := range readings[1:] {
			if r < out {
				out = r
			}
		}
		return out
	case ReduceSum:
		out := 0.0
		for _, r := range readings {
			out += r
		}
		return out
	default:
		return readings[0]
	}
}

// Resolve merges the profiles that match ctx into a single plan.
//
// Profiles are applied from the lowest priority to the highest, so a
// higher-priority profile overrides a lower one on the data points they share;
// ties are broken in favour of the profile with the more specific selector, and
// finally by ID so the outcome is deterministic. supported lists the source
// kinds the active adapter can read: the first candidate binding whose
// simulator scope and source kind are both supported wins.
func Resolve(all []*Profile, ctx Context, supported []SourceKind) *Plan {
	plan := &Plan{Simulator: ctx.Simulator, Aircraft: ctx}

	matched := make([]*Profile, 0, len(all))
	for _, prof := range all {
		if prof == nil || prof.Disabled {
			continue
		}
		if prof.Match.Eval(ctx) {
			matched = append(matched, prof)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].Priority != matched[j].Priority {
			return matched[i].Priority < matched[j].Priority
		}
		ci, cj := matched[i].Match.Conditions(), matched[j].Match.Conditions()
		if ci != cj {
			return ci < cj
		}
		return matched[i].ID < matched[j].ID
	})

	supports := make(map[SourceKind]bool, len(supported))
	for _, kind := range supported {
		supports[kind] = true
	}
	supports[SourceConst] = true

	winners := map[string]ResolvedBinding{}
	skipped := map[string]string{}
	for _, prof := range matched {
		plan.Profiles = append(plan.Profiles, AppliedProfile{
			ID: prof.ID, Name: prof.Name, Priority: prof.Priority, Origin: prof.Origin,
		})
		for point, entry := range prof.Mash {
			sources := pick(entry, ctx.Simulator, supports)
			if len(sources) == 0 {
				if _, already := winners[point]; !already {
					skipped[point] = prof.ID
				}
				continue
			}
			winners[point] = ResolvedBinding{
				Point:     point,
				ProfileID: prof.ID,
				Reduce:    entry.Reduce,
				Sources:   sources,
			}
			delete(skipped, point)
		}
	}

	points := make([]string, 0, len(winners))
	for point := range winners {
		points = append(points, point)
	}
	sort.Strings(points)

	seen := map[string]bool{}
	for _, point := range points {
		binding := winners[point]
		plan.Bindings = append(plan.Bindings, binding)
		for _, src := range binding.Sources {
			if src.Kind == SourceConst || seen[src.Key] {
				continue
			}
			seen[src.Key] = true
			plan.vars = append(plan.vars, Var{
				Key:  src.Key,
				Kind: src.Kind,
				Name: src.Name,
				Unit: src.Unit,
			})
		}
	}

	for point := range skipped {
		plan.Skipped = append(plan.Skipped, point)
	}
	sort.Strings(plan.Skipped)

	return plan
}

// pick returns the bindings usable on this simulator: the first one when the
// entry has no reduce operator, every one of them when it has.
func pick(entry MashEntry, simulator string, supports map[SourceKind]bool) []ResolvedSource {
	var out []ResolvedSource
	for _, b := range entry.Bindings {
		if b.Sim != SimAny && !strings.EqualFold(b.Sim, simulator) {
			continue
		}
		if !supports[b.Source.Kind] {
			continue
		}
		out = append(out, ResolvedSource{
			Kind:      b.Source.Kind,
			Name:      b.Source.Name,
			Unit:      b.Source.Unit,
			Key:       b.Source.Key(),
			Value:     b.Source.Value,
			Transform: b.Transform,
		})
		if entry.Reduce == ReduceFirst {
			break
		}
	}
	return out
}
