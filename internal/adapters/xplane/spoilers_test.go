package xplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const idxSpeedbrakeRatio = 45 // sim/cockpit2/controls/speedbrake_ratio

func TestSpeedbrakeDatarefIsWhereTheSwitchExpectsIt(t *testing.T) {
	require.Equal(t, "sim/cockpit2/controls/speedbrake_ratio", xplaneDatarefs[idxSpeedbrakeRatio])
}

// X-Plane reports armed speedbrakes as a handle ratio of -0.5. The server
// multiplies anything at or below 1 by 100, taking it for a ratio, so the -50
// the adapter used to send was stored as -5000.
func TestArmedSpeedbrakesReadAsStowed(t *testing.T) {
	cases := map[float64]float64{
		-0.5: 0,   // armed
		0:    0,   // stowed
		0.5:  50,  // halfway
		1:    100, // fully deployed
	}
	for ratio, want := range cases {
		x := &Adapter{}
		feed(x, map[int]float64{idxSpeedbrakeRatio: ratio})
		assert.Equal(t, want, x.data.Controls.Spoilers, "speedbrake ratio %v", ratio)
	}
}
