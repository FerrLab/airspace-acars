package xplane

import (
	"testing"

	"airspace-acars/internal/profiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feed pushes readings through the adapter the way listenLoop does.
func feed(x *Adapter, readings map[int]float64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for idx, val := range readings {
		x.handleReading(idx, val)
	}
}

func writeString(x *Adapter, base int, text string) {
	readings := map[int]float64{}
	for i, c := range []byte(text) {
		readings[base+i] = float64(c)
	}
	feed(x, readings)
}

func TestIdentityIsReassembledFromTheStringDatarefs(t *testing.T) {
	x := &Adapter{}
	writeString(x, icaoIndexBase, "B738")
	writeString(x, descripIndexBase, "Boeing 737-800X")

	id := x.RawIdentity()
	assert.Equal(t, "B738", id.AircraftType)
	assert.Equal(t, "Boeing 737-800X", id.AircraftName)
	assert.Equal(t, profiles.SimXPlane, id.Simulator)
}

func TestIdentityIgnoresNonPrintableCharacters(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		icaoIndexBase:     float64('A'),
		icaoIndexBase + 1: float64('3'),
		icaoIndexBase + 2: 7, // control character X-Plane may pad with
	})
	assert.Equal(t, "A3", x.RawIdentity().AircraftType)
}

func TestRawIdentityCountsFittedEngines(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{71: 3}) // sim/aircraft/engine/acf_num_engines
	assert.Equal(t, 3, x.RawIdentity().EngineCount)
}

func TestDefaultDatarefsStillReachTheirFields(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{
		0:  51.47, // latitude
		2:  1000,  // elevation, metres
		18: 1,     // on ground
		22: 11825, // com1, 10 kHz units
	})

	x.mu.Lock()
	defer x.mu.Unlock()
	assert.InDelta(t, 51.47, x.data.Position.Latitude, 1e-6)
	assert.InDelta(t, 3280.84, x.data.Position.Altitude, 1e-2)
	assert.True(t, x.data.Sensors.OnGround)
	assert.InDelta(t, 118.25, x.data.Radios.Com1, 1e-9)
}

func TestProfileOverridesTheDefaultReading(t *testing.T) {
	prof, err := profiles.Parse([]byte(`{
		"id": "test-833", "name": "Test 8.33",
		"match": {"field": "simulator", "op": "equals", "value": "xplane"},
		"mash": {
			"radios.com1": {
				"sim": "xplane",
				"source": {"kind": "dataref", "name": "sim/cockpit2/radios/actuators/com1_frequency_hz_833"},
				"transform": [{"op": "divide", "value": 1000}, {"op": "round", "value": 3}]
			}
		}
	}`))
	require.NoError(t, err)

	x := &Adapter{}
	plan := profiles.Resolve([]*profiles.Profile{prof}, x.RawIdentity(), x.SupportedSources())
	require.Len(t, plan.Vars(), 1)

	// Install the plan by hand: subscribing needs a live UDP socket, but the
	// index bookkeeping and the override itself do not.
	x.mu.Lock()
	x.plan = plan
	x.extraByIdx = map[int]string{extraIndexBase: plan.Vars()[0].Key}
	x.extraValues = map[string]float64{}
	x.mu.Unlock()

	feed(x, map[int]float64{
		22:             11825,  // the 25 kHz dataref rounds the channel down
		extraIndexBase: 121905, // the 8.33 kHz dataref carries it in full
	})

	x.mu.Lock()
	data := x.data
	x.plan.Apply(&data, x.extraValues)
	x.mu.Unlock()

	assert.InDelta(t, 118.25, x.data.Radios.Com1, 1e-9, "the default reading is left untouched")
	assert.InDelta(t, 121.905, data.Radios.Com1, 1e-9, "the snapshot handed to callers carries the override")
}

func TestUnknownExtraIndexIsIgnored(t *testing.T) {
	x := &Adapter{}
	feed(x, map[int]float64{extraIndexBase + 7: 42})
	x.mu.Lock()
	defer x.mu.Unlock()
	assert.Empty(t, x.extraValues)
}
