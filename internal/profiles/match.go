package profiles

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Match fields available to a profile's selector.
const (
	FieldAircraftName = "aircraftName" // simulator title, e.g. "Fenix A320 IAE"
	FieldAircraftType = "aircraftType" // ICAO type, e.g. "A20N"
	FieldSimulator    = "simulator"    // "simconnect" or "xplane"
	FieldEngineCount  = "engineCount"
)

// Match operators.
const (
	OpEquals      = "equals"
	OpNotEquals   = "notEquals"
	OpContains    = "contains"
	OpNotContains = "notContains"
	OpStartsWith  = "startsWith"
	OpEndsWith    = "endsWith"
	OpMatches     = "matches" // regular expression
	OpIn          = "in"
	OpNotIn       = "notIn"
	OpGreaterThan = "greaterThan"
	OpLessThan    = "lessThan"
	OpExists      = "exists" // field is present and non-empty
)

// Context is the aircraft identity a profile is matched against.
type Context struct {
	AircraftName string `json:"aircraftName"`
	AircraftType string `json:"aircraftType"`
	Simulator    string `json:"simulator"`
	EngineCount  int    `json:"engineCount"`
}

// MatchNode is either a group (all/any/not) or a leaf condition. Groups may be
// nested freely, so selectors like "(title contains Fenix OR title contains
// A320) AND sim is simconnect" are expressible.
type MatchNode struct {
	All []MatchNode `json:"all,omitempty"`
	Any []MatchNode `json:"any,omitempty"`
	Not *MatchNode  `json:"not,omitempty"`

	Field         string     `json:"field,omitempty"`
	Op            string     `json:"op,omitempty"`
	Value         *ValueList `json:"value,omitempty"`
	CaseSensitive bool       `json:"caseSensitive,omitempty"`
}

// ValueList holds a condition's operand. JSON accepts a scalar or an array;
// operators other than "in"/"notIn" use the first entry.
type ValueList struct {
	Items []Literal
}

// UnmarshalJSON accepts a scalar or an array of scalars.
func (v *ValueList) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var items []Literal
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		v.Items = items
		return nil
	}
	var single Literal
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	v.Items = []Literal{single}
	return nil
}

// MarshalJSON writes a single operand back as a scalar, several as an array.
func (v ValueList) MarshalJSON() ([]byte, error) {
	if len(v.Items) == 1 {
		return json.Marshal(v.Items[0])
	}
	return json.Marshal(v.Items)
}

func (v *ValueList) first() Literal {
	if v == nil || len(v.Items) == 0 {
		return Literal{}
	}
	return v.Items[0]
}

// isGroup reports whether the node combines other nodes rather than testing a field.
func (n *MatchNode) isGroup() bool {
	return len(n.All) > 0 || len(n.Any) > 0 || n.Not != nil
}

// Validate checks fields, operators and group shape.
func (n *MatchNode) Validate() error {
	if n == nil {
		return nil
	}
	if n.isGroup() {
		if n.Field != "" || n.Op != "" {
			return fmt.Errorf("a group node cannot also carry a field condition")
		}
		for _, child := range n.All {
			if err := child.Validate(); err != nil {
				return err
			}
		}
		for _, child := range n.Any {
			if err := child.Validate(); err != nil {
				return err
			}
		}
		return n.Not.Validate()
	}

	switch n.Field {
	case FieldAircraftName, FieldAircraftType, FieldSimulator, FieldEngineCount:
	case "":
		return fmt.Errorf("condition is missing a field")
	default:
		return fmt.Errorf("unknown match field %q", n.Field)
	}

	switch n.Op {
	case OpEquals, OpNotEquals, OpContains, OpNotContains, OpStartsWith, OpEndsWith,
		OpIn, OpNotIn, OpGreaterThan, OpLessThan:
		if n.Value == nil || len(n.Value.Items) == 0 {
			return fmt.Errorf("operator %q requires a value", n.Op)
		}
	case OpMatches:
		if n.Value == nil || len(n.Value.Items) == 0 {
			return fmt.Errorf("operator %q requires a value", n.Op)
		}
		if _, err := regexp.Compile(n.Value.first().String()); err != nil {
			return fmt.Errorf("invalid regular expression: %w", err)
		}
	case OpExists:
	case "":
		return fmt.Errorf("condition on %q is missing an operator", n.Field)
	default:
		return fmt.Errorf("unknown match operator %q", n.Op)
	}
	return nil
}

// Eval reports whether the node matches the context. A nil node matches
// everything, so a profile without a selector is a catch-all.
func (n *MatchNode) Eval(ctx Context) bool {
	if n == nil {
		return true
	}
	if n.isGroup() {
		for _, child := range n.All {
			if !child.Eval(ctx) {
				return false
			}
		}
		if len(n.Any) > 0 {
			matched := false
			for _, child := range n.Any {
				if child.Eval(ctx) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		if n.Not != nil && n.Not.Eval(ctx) {
			return false
		}
		return true
	}
	return n.evalLeaf(ctx)
}

// Conditions counts the leaf conditions below the node. It is used to break
// priority ties in favour of the more specific profile.
func (n *MatchNode) Conditions() int {
	if n == nil {
		return 0
	}
	if !n.isGroup() {
		return 1
	}
	total := 0
	for _, child := range n.All {
		total += child.Conditions()
	}
	for _, child := range n.Any {
		total += child.Conditions()
	}
	total += n.Not.Conditions()
	return total
}

func (n *MatchNode) evalLeaf(ctx Context) bool {
	text, number, isNumeric := fieldValue(ctx, n.Field)

	switch n.Op {
	case OpExists:
		if isNumeric {
			return number != 0
		}
		return text != ""
	case OpGreaterThan, OpLessThan:
		operand, ok := numericOperand(n.Value.first())
		if !ok {
			return false
		}
		value := number
		if !isNumeric {
			parsed, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return false
			}
			value = parsed
		}
		if n.Op == OpGreaterThan {
			return value > operand
		}
		return value < operand
	case OpIn, OpNotIn:
		found := false
		for _, item := range n.Value.Items {
			if leafEquals(text, number, isNumeric, item, n.CaseSensitive) {
				found = true
				break
			}
		}
		return found == (n.Op == OpIn)
	case OpEquals, OpNotEquals:
		eq := leafEquals(text, number, isNumeric, n.Value.first(), n.CaseSensitive)
		return eq == (n.Op == OpEquals)
	}

	// Remaining operators are textual.
	haystack := text
	if isNumeric {
		haystack = strconv.FormatFloat(number, 'f', -1, 64)
	}
	needle := n.Value.first().String()
	if !n.CaseSensitive {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	switch n.Op {
	case OpContains:
		return strings.Contains(haystack, needle)
	case OpNotContains:
		return !strings.Contains(haystack, needle)
	case OpStartsWith:
		return strings.HasPrefix(haystack, needle)
	case OpEndsWith:
		return strings.HasSuffix(haystack, needle)
	case OpMatches:
		pattern := n.Value.first().String()
		if !n.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(text)
	}
	return false
}

// fieldValue reads a context field as text or as a number.
func fieldValue(ctx Context, field string) (text string, number float64, isNumeric bool) {
	switch field {
	case FieldAircraftName:
		return ctx.AircraftName, 0, false
	case FieldAircraftType:
		return ctx.AircraftType, 0, false
	case FieldSimulator:
		return ctx.Simulator, 0, false
	case FieldEngineCount:
		return "", float64(ctx.EngineCount), true
	}
	return "", 0, false
}

func leafEquals(text string, number float64, isNumeric bool, operand Literal, caseSensitive bool) bool {
	if isNumeric {
		value, ok := numericOperand(operand)
		return ok && number == value
	}
	if caseSensitive {
		return text == operand.String()
	}
	return strings.EqualFold(text, operand.String())
}

func numericOperand(operand Literal) (float64, bool) {
	if !operand.IsString {
		return operand.Num, true
	}
	parsed, err := strconv.ParseFloat(operand.Str, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
