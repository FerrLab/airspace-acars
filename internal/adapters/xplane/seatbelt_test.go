package xplane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeatbeltDatarefIsAppendedAtTheEnd(t *testing.T) {
	require.Equal(t, seatbeltSwitchIndex, len(xplaneDatarefs)-1,
		"fasten_seat_belts must be the last entry; new datarefs go after it")
	assert.Equal(t, "sim/cockpit2/switches/fasten_seat_belts", xplaneDatarefs[seatbeltSwitchIndex])
}

// The stock switch has an AUTO position, which lights the sign only on the
// way up and down; the sign is unknown until the dataref has reported.
func TestSeatbeltSignFollowsTheStockSwitch(t *testing.T) {
	x := &Adapter{}
	withConn(t, x)
	read := func() (lit, known bool) {
		x.mu.Lock()
		x.lastReceived = time.Now()
		x.mu.Unlock()
		fd, err := x.GetFlightData()
		require.NoError(t, err)
		return fd.Cabin.SeatbeltSign, fd.Cabin.SeatbeltKnown
	}

	_, known := read()
	assert.False(t, known, "a sign nobody has read is not 'off'")

	feed(x, map[int]float64{2: 3000 / 3.28084, seatbeltSwitchIndex: 1})
	lit, known := read()
	assert.True(t, known)
	assert.True(t, lit, "AUTO at 3,000 ft is lit")

	feed(x, map[int]float64{2: 35000 / 3.28084})
	lit, _ = read()
	assert.False(t, lit, "AUTO in the cruise is off")

	feed(x, map[int]float64{seatbeltSwitchIndex: 2})
	lit, _ = read()
	assert.True(t, lit, "ON is lit at any altitude")
}
