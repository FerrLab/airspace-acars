package xplane

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The index IS the position in xplaneDatarefs, and applyDefaultRef switches on
// it, so inserting a dataref above these would silently reassign them.
const (
	idxBeaconLightsOn  = 38
	idxStrobeLightsOn  = 39
	idxLandingLightsOn = 40
)

func TestLightDatarefsAreWhereTheSwitchExpectsThem(t *testing.T) {
	want := map[int]string{
		idxBeaconLightsOn:  "sim/cockpit/electrical/beacon_lights_on",
		idxStrobeLightsOn:  "sim/cockpit/electrical/strobe_lights_on",
		idxLandingLightsOn: "sim/cockpit/electrical/landing_lights_on",
	}
	for i := 0; i < landingLightCount; i++ {
		want[landingSwitchIndexBase+i] = fmt.Sprintf("sim/cockpit2/switches/landing_lights_switch[%d]", i)
	}
	for idx, dataref := range want {
		require.Less(t, idx, len(xplaneDatarefs), "index %d is past the end of the table", idx)
		assert.Equal(t, dataref, xplaneDatarefs[idx],
			"index %d no longer subscribes %s — a dataref was inserted above it, "+
				"which reassigns every applyDefaultRef case after the insertion", idx, dataref)
	}
	assert.Less(t, landingSwitchIndexBase+landingLightCount-1, extraIndexBase,
		"the landing light switches must stay below the profile dataref indices")
}

func TestBeaconAndStrobeReachFlightData(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{idxBeaconLightsOn: 1, idxStrobeLightsOn: 1})
	assert.True(t, x.data.Lights.Beacon)
	assert.True(t, x.data.Lights.Strobe)

	feed(x, map[int]float64{idxBeaconLightsOn: 0, idxStrobeLightsOn: 0})
	assert.False(t, x.data.Lights.Beacon)
	assert.False(t, x.data.Lights.Strobe)
}

// The legacy master switch is what the stock light aircraft drive.
func TestLegacyMasterSwitchTurnsLandingLightsOn(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{idxLandingLightsOn: 1})
	assert.True(t, x.data.Lights.Landing)

	feed(x, map[int]float64{idxLandingLightsOn: 0})
	assert.False(t, x.data.Lights.Landing)
}

// An airliner with a switch per landing light never touches the legacy master
// switch. Before the per-light switches were read, such an aircraft reported
// its landing lights off with every one of them on.
func TestAnyPerLightSwitchTurnsLandingLightsOn(t *testing.T) {
	for i := 0; i < landingLightCount; i++ {
		x := &Adapter{}
		feed(x, map[int]float64{idxLandingLightsOn: 0, landingSwitchIndexBase + i: 1})
		assert.True(t, x.data.Lights.Landing, "switch %d alone should read as on", i)
	}
}

// The per-light switch is a ratio, not a bool, and aircraft do not keep it to
// 0 or 1: the Zibo 737 writes 2, and a dimmed light reads below 1.
func TestPerLightSwitchCountsAnyPositiveValueAsOn(t *testing.T) {
	for _, v := range []float64{2, 0.5, 1} {
		x := &Adapter{}
		feed(x, map[int]float64{landingSwitchIndexBase: v})
		assert.True(t, x.data.Lights.Landing, "switch value %v should read as on", v)
	}
}

func TestLandingLightsOffOnlyWhenEverySwitchIsOff(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		landingSwitchIndexBase:     1,
		landingSwitchIndexBase + 1: 1,
	})
	require.True(t, x.data.Lights.Landing)

	// One of two lights off keeps the landing lights on.
	feed(x, map[int]float64{landingSwitchIndexBase: 0})
	assert.True(t, x.data.Lights.Landing, "the other light is still on")

	feed(x, map[int]float64{landingSwitchIndexBase + 1: 0})
	assert.False(t, x.data.Lights.Landing)
}

// A cold aircraft must read off, not merely default-zero.
func TestAllLandingSwitchesOffReadsOff(t *testing.T) {
	x := &Adapter{}
	readings := map[int]float64{idxLandingLightsOn: 0}
	for i := 0; i < landingLightCount; i++ {
		readings[landingSwitchIndexBase+i] = 0
	}
	feed(x, readings)
	assert.False(t, x.data.Lights.Landing)
}
