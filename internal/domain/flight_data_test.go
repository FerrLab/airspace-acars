package domain

import "testing"

// A three-position seatbelt switch in AUTO lights the sign only below
// 10,000 ft, so the flight attendant announces it on the way up and down
// but not when the switch is merely left in AUTO in the cruise.
func TestSeatbeltSwitchAutoIsLitOnlyBelowTenThousandFeet(t *testing.T) {
	cases := []struct {
		name     string
		position float64
		altitude float64
		want     bool
	}{
		{"off at the gate", 0, 300, false},
		{"auto at the gate", 1, 300, true},
		{"auto in the cruise", 1, 35_000, false},
		{"auto just below the line", 1, 9_999, true},
		{"on in the cruise", 2, 35_000, true},
		{"a reading between auto and on rounds up", 1.6, 35_000, true},
		{"a reading just off zero is still off", 0.2, 300, false},
	}
	for _, c := range cases {
		if got := SeatbeltFromSwitch(c.position, c.altitude); got != c.want {
			t.Errorf("%s: sign lit = %v, want %v", c.name, got, c.want)
		}
	}
}
