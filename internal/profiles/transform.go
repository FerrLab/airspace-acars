package profiles

import (
	"fmt"
	"math"
	"strconv"
)

// Transform operators. A binding's transform is a pipeline: each step takes
// the number produced by the previous one and returns a new number. Comparison
// operators return 1 or 0 so they can feed boolean data points.
const (
	OpScale  = "scale"  // multiply by value
	OpOffset = "offset" // add value
	OpDivide = "divide" // divide by value
	OpRound  = "round"  // round to the nearest integer, or to value decimals
	OpFloor  = "floor"
	OpCeil   = "ceil"
	OpAbs    = "abs"
	OpClamp  = "clamp" // constrain to [min, max]
	OpBool   = "bool"  // 1 when non-zero (or when >= value), else 0
	OpNot    = "not"   // logical negation
	OpEq     = "eq"    // 1 when equal to value
	OpNe     = "ne"
	OpGt     = "gt"
	OpGte    = "gte"
	OpLt     = "lt"
	OpLte    = "lte"
	OpMap    = "map" // table lookup, with an optional default
)

// Step is one operation in a binding's transform pipeline.
type Step struct {
	Op      string             `json:"op"`
	Value   *float64           `json:"value,omitempty"`
	Min     *float64           `json:"min,omitempty"`
	Max     *float64           `json:"max,omitempty"`
	Map     map[string]float64 `json:"map,omitempty"`
	Default *float64           `json:"default,omitempty"`
}

func (s Step) validate() error {
	switch s.Op {
	case OpScale, OpOffset, OpDivide:
		if s.Value == nil {
			return fmt.Errorf("%q requires a value", s.Op)
		}
		if s.Op == OpDivide && *s.Value == 0 {
			return fmt.Errorf("%q by zero", s.Op)
		}
	case OpEq, OpNe, OpGt, OpGte, OpLt, OpLte:
		if s.Value == nil {
			return fmt.Errorf("%q requires a value", s.Op)
		}
	case OpClamp:
		if s.Min == nil && s.Max == nil {
			return fmt.Errorf("%q requires min and/or max", s.Op)
		}
		if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
			return fmt.Errorf("%q min is greater than max", s.Op)
		}
	case OpMap:
		if len(s.Map) == 0 {
			return fmt.Errorf("%q requires a non-empty map", s.Op)
		}
		for k := range s.Map {
			if _, err := strconv.ParseFloat(k, 64); err != nil {
				return fmt.Errorf("%q key %q is not a number", s.Op, k)
			}
		}
	case OpRound, OpFloor, OpCeil, OpAbs, OpBool, OpNot:
		// no required arguments
	default:
		return fmt.Errorf("unknown transform op %q", s.Op)
	}
	return nil
}

func (s Step) apply(in float64) float64 {
	switch s.Op {
	case OpScale:
		return in * *s.Value
	case OpOffset:
		return in + *s.Value
	case OpDivide:
		return in / *s.Value
	case OpRound:
		if s.Value != nil {
			factor := math.Pow(10, *s.Value)
			return math.Round(in*factor) / factor
		}
		return math.Round(in)
	case OpFloor:
		return math.Floor(in)
	case OpCeil:
		return math.Ceil(in)
	case OpAbs:
		return math.Abs(in)
	case OpClamp:
		if s.Min != nil && in < *s.Min {
			in = *s.Min
		}
		if s.Max != nil && in > *s.Max {
			in = *s.Max
		}
		return in
	case OpBool:
		if s.Value != nil {
			return boolValue(in >= *s.Value)
		}
		return boolValue(in != 0)
	case OpNot:
		return boolValue(in == 0)
	case OpEq:
		return boolValue(in == *s.Value)
	case OpNe:
		return boolValue(in != *s.Value)
	case OpGt:
		return boolValue(in > *s.Value)
	case OpGte:
		return boolValue(in >= *s.Value)
	case OpLt:
		return boolValue(in < *s.Value)
	case OpLte:
		return boolValue(in <= *s.Value)
	case OpMap:
		if out, ok := s.Map[strconv.FormatFloat(in, 'f', -1, 64)]; ok {
			return out
		}
		if s.Default != nil {
			return *s.Default
		}
		return in
	default:
		return in
	}
}

// applySteps runs the pipeline over a raw reading.
func applySteps(steps []Step, in float64) float64 {
	for _, s := range steps {
		in = s.apply(in)
	}
	return in
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
