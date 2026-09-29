package xplane

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeAircraftName(t *testing.T) {
	cases := []struct {
		name                    string
		author, uiName, descrip string
		want                    string
	}{
		{"the author goes in front of the UI name", "Rotate", "MD-11F", "McDonnell Douglas MD-11 Freighter", "Rotate MD-11F"},
		{"not when the name already carries it", "ToLiss", "ToLiss A321", "A321 with high fidelity system modelling", "ToLiss A321"},
		{"the author's first word is enough", "FlightFactor Aero", "FlightFactor 767-300ER", "", "FlightFactor 767-300ER"},
		{"X-Plane 11 has no UI name, so the description stands in", "ToLiss", "", "A320 with high fidelity system modelling", "ToLiss A320 with high fidelity system modelling"},
		{"no author", "", "Boeing 767-300ER", "Boeing 767-Freighter", "Boeing 767-300ER"},
		{"nothing but the author", "Rotate", "", "", "Rotate"},
		{"nothing at all", "", "", "", ""},
		{"padding is dropped", " ToLiss ", " A321 ", "", "ToLiss A321"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, composeAircraftName(tc.author, tc.uiName, tc.descrip))
		})
	}
}

func TestAircraftNameIsReassembledFromTheAuthorAndTheUIName(t *testing.T) {
	x := &Adapter{}
	writeString(x, descripIndexBase, "A320 with high fidelity system modelling")
	assert.Equal(t, "A320 with high fidelity system modelling", x.RawIdentity().AircraftName,
		"with neither a UI name nor an author, the description alone")

	writeString(x, authorIndexBase, "ToLiss")
	writeString(x, uiNameIndexBase, "A320neo")
	assert.Equal(t, "ToLiss A320neo", x.RawIdentity().AircraftName)

	x.mu.Lock()
	defer x.mu.Unlock()
	assert.Equal(t, "ToLiss A320neo", x.data.AircraftName, "the name reported to the network is the one profiles match")
}

// The name goes out as each position's aircraft name, which the server keeps
// in a 100-character column.
func TestAircraftNameFitsTheServerColumn(t *testing.T) {
	x := &Adapter{}
	writeString(x, authorIndexBase, strings.Repeat("a", authorChars))
	writeString(x, uiNameIndexBase, strings.Repeat("n", uiNameChars))
	writeString(x, descripIndexBase, strings.Repeat("d", descripChars))
	assert.LessOrEqual(t, len(x.RawIdentity().AircraftName), 100)
}

// Every identity character is its own RREF index, so the ranges must neither
// overlap each other nor reach the built-in table below them or the profile
// datarefs above them.
func TestIdentityRangesDoNotOverlap(t *testing.T) {
	ranges := append([]identityString(nil), identityStrings...)
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].base < ranges[j].base })

	require.GreaterOrEqual(t, ranges[0].base, len(xplaneDatarefs), "%s overlaps the built-in table", ranges[0].dataref)
	for i := 1; i < len(ranges); i++ {
		assert.LessOrEqual(t, ranges[i-1].base+ranges[i-1].chars, ranges[i].base,
			"%s overlaps %s", ranges[i-1].dataref, ranges[i].dataref)
	}
	last := ranges[len(ranges)-1]
	assert.LessOrEqual(t, last.base+last.chars, extraIndexBase, "%s reaches the profile datarefs", last.dataref)
}
