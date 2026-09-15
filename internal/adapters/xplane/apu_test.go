package xplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apuIndex names the RREF index each APU dataref is subscribed under. The
// index IS the position in xplaneDatarefs, and applyDefaultRef switches on it,
// so inserting a dataref anywhere above these would silently reassign them to
// the wrong fields. This pins them.
const (
	idxAPUStarterSwitch = 77
	idxAPUN1Percent     = 78
	idxAPUGeneratorOn   = 79
	idxAPUGeneratorAmps = 80
)

func TestAPUDatarefsAreWhereTheSwitchExpectsThem(t *testing.T) {
	want := map[int]string{
		idxAPUStarterSwitch: "sim/cockpit2/electrical/APU_starter_switch",
		idxAPUN1Percent:     "sim/cockpit2/electrical/APU_N1_percent",
		idxAPUGeneratorOn:   "sim/cockpit2/electrical/APU_generator_on",
		idxAPUGeneratorAmps: "sim/cockpit2/electrical/APU_generator_amps",
	}
	for idx, dataref := range want {
		require.Less(t, idx, len(xplaneDatarefs), "index %d is past the end of the table", idx)
		assert.Equal(t, dataref, xplaneDatarefs[idx],
			"index %d no longer subscribes %s — a dataref was inserted above it, "+
				"which reassigns every applyDefaultRef case after the insertion", idx, dataref)
	}
}

func TestAPUReadingsReachFlightData(t *testing.T) {
	// A running APU with its generator on the bus.
	x := &Adapter{}
	feed(x, map[int]float64{
		idxAPUStarterSwitch: 1,
		idxAPUN1Percent:     99.4,
		idxAPUGeneratorOn:   1,
		idxAPUGeneratorAmps: 48,
	})

	assert.True(t, x.data.APU.SwitchOn)
	assert.InDelta(t, 99.4, x.data.APU.RPMPercent, 0.01)
	assert.True(t, x.data.APU.GenSwitch)
	assert.True(t, x.data.APU.GenActive, "amperage on the bus means the generator is supplying it")
}

func TestAPUStartPositionCountsAsOn(t *testing.T) {
	// The switch is an enum, not a bool: 0 off, 1 on, 2 start. Treating it as
	// a bool naively would still work for 1, but 2 must not read as off.
	for _, position := range []float64{1, 2} {
		x := &Adapter{}
		feed(x, map[int]float64{idxAPUStarterSwitch: position})
		assert.True(t, x.data.APU.SwitchOn, "switch position %v should read as on", position)
	}

	x := &Adapter{}
	feed(x, map[int]float64{idxAPUStarterSwitch: 0})
	assert.False(t, x.data.APU.SwitchOn)
}

// The generator switch can be on while the generator supplies nothing — an APU
// that has not spun up yet, or one that has failed. Collapsing the two into a
// single flag would report power that is not there.
func TestGeneratorSwitchOnWithoutOutputIsNotActive(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		idxAPUGeneratorOn:   1,
		idxAPUGeneratorAmps: 0,
	})

	assert.True(t, x.data.APU.GenSwitch)
	assert.False(t, x.data.APU.GenActive, "no amperage means nothing is reaching the bus")
}

// A cold APU must read cold, not merely default-zero: before this was wired up
// every X-Plane pilot reported an all-false APU block whether or not it was
// running, which is indistinguishable from a correct cold reading.
func TestColdAPUReadsCold(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		idxAPUStarterSwitch: 0,
		idxAPUN1Percent:     0,
		idxAPUGeneratorOn:   0,
		idxAPUGeneratorAmps: 0,
	})

	assert.False(t, x.data.APU.SwitchOn)
	assert.Zero(t, x.data.APU.RPMPercent)
	assert.False(t, x.data.APU.GenSwitch)
	assert.False(t, x.data.APU.GenActive)
}
