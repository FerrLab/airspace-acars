package xplane

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idxTheta = 4 // sim/flightmodel/position/theta
	idxPhi   = 5 // sim/flightmodel/position/phi
)

func TestAttitudeDatarefsAreWhereTheSwitchExpectsThem(t *testing.T) {
	require.Equal(t, "sim/flightmodel/position/theta", xplaneDatarefs[idxTheta])
	require.Equal(t, "sim/flightmodel/position/phi", xplaneDatarefs[idxPhi])
}

// The server stores pitch and bank in SimConnect's sign and negates every
// flight for display, so X-Plane has to report the same sign as MSFS: a climb
// is negative pitch, a right turn is negative bank.
func TestAttitudeIsReportedInSimConnectsSign(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		idxTheta: 8.5,  // X-Plane: nose up
		idxPhi:   25.0, // X-Plane: right wing down
	})
	assert.Equal(t, -8.5, x.data.Attitude.Pitch, "nose up is negative pitch")
	assert.Equal(t, -25.0, x.data.Attitude.Roll, "right wing down is negative bank")

	feed(x, map[int]float64{idxTheta: -3, idxPhi: -10})
	assert.Equal(t, 3.0, x.data.Attitude.Pitch, "nose down is positive pitch")
	assert.Equal(t, 10.0, x.data.Attitude.Roll, "left wing down is positive bank")
}

func TestLevelAttitudeIsNotNegativeZero(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{idxTheta: 0, idxPhi: math.Copysign(0, -1)})
	assert.False(t, math.Signbit(x.data.Attitude.Pitch), "a level pitch must not read as -0")
	assert.False(t, math.Signbit(x.data.Attitude.Roll), "a level bank must not read as -0")
}
