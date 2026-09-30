package xplane

import (
	"net"
	"testing"

	"airspace-acars/internal/profiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withConn gives the adapter a socket to send its subscriptions into, so a
// plan can be installed the way ApplyProfile installs it. Nothing answers.
func withConn(t *testing.T, x *Adapter) {
	t.Helper()
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	conn, err := net.DialUDP("udp", nil, listener.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	x.conn = conn
}

// A dataref X-Plane cannot resolve reports nothing, or zero, so a zero from an
// aircraft's own dataref cannot be told apart from a name that is wrong until
// the dataref has once read something else.
func TestAircraftDatarefsAreWithheldUntilTheyReadNonZero(t *testing.T) {
	prof, err := profiles.Parse([]byte(`{
		"id": "t", "name": "T",
		"mash": {
			"controls.flaps": {
				"reduce": "max",
				"bindings": [
					{"sim": "xplane", "source": {"kind": "dataref", "name": "Vendor/aircraft/flap_handle"}},
					{"sim": "xplane", "source": {"kind": "dataref", "name": "sim/cockpit2/controls/flap_ratio"}}
				]
			}
		}
	}`))
	require.NoError(t, err)

	x := &Adapter{}
	withConn(t, x)
	plan := profiles.Resolve([]*profiles.Profile{prof}, x.RawIdentity(), x.SupportedSources())
	require.NoError(t, x.ApplyProfile(plan))

	idx := map[string]int{}
	for _, ref := range x.extraRefs {
		idx[ref.dataref] = ref.index
	}
	aircraft, stock := idx["Vendor/aircraft/flap_handle"], idx["sim/cockpit2/controls/flap_ratio"]
	aircraftKey, stockKey := x.extraByIdx[aircraft], x.extraByIdx[stock]
	require.NotEmpty(t, aircraftKey)
	require.NotEmpty(t, stockKey)

	feed(x, map[int]float64{aircraft: 0, stock: 0})
	assert.NotContains(t, x.extraValues, aircraftKey, "an aircraft dataref's zero may be a name X-Plane could not resolve")
	assert.Contains(t, x.extraValues, stockKey, "a stock dataref's zero is a reading")

	feed(x, map[int]float64{aircraft: 0.5})
	feed(x, map[int]float64{aircraft: 0})
	assert.Equal(t, 0.0, x.extraValues[aircraftKey], "once it has read non-zero, its zeros are readings too")

	// A plan installed afresh has to prove its datarefs again.
	require.NoError(t, x.ApplyProfile(plan))
	feed(x, map[int]float64{aircraft: 0})
	assert.NotContains(t, x.extraValues, aircraftKey)
}
