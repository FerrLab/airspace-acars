package xplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const idxAutopilotMode = 30 // sim/cockpit/autopilot/autopilot_mode

func TestAutopilotModeDatarefIsWhereTheSwitchExpectsIt(t *testing.T) {
	require.Equal(t, "sim/cockpit/autopilot/autopilot_mode", xplaneDatarefs[idxAutopilotMode])
}

// autopilot_mode is 0 off, 1 flight director only, 2 autopilot on. A pilot
// hand-flying with the flight director on is flying manually, and the server
// counts manual flying time from this field.
func TestFlightDirectorAloneIsNotTheAutopilot(t *testing.T) {
	cases := map[float64]bool{
		0: false, // off
		1: false, // flight director only
		2: true,  // autopilot on
	}
	for mode, want := range cases {
		x := &Adapter{}
		feed(x, map[int]float64{idxAutopilotMode: mode})
		assert.Equal(t, want, x.data.Autopilot.Master, "autopilot_mode %v", mode)
	}
}
