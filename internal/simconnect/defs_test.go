//go:build windows

package simconnect

import "testing"

// The dispatch IDs are the simulator's numbers, not ours to choose. The enum
// once carried RECV_ID_PICK, an FSX message the MSFS SDK no longer has, which
// pushed everything after it up by one: MSFS 2024 sent facility rows as 28 and
// their end as 29, the adapter waited for 29 and 30, and every airport lookup
// timed out with no runway and no stand. The values below are what the
// simulator sent at SBRF.
func TestRecvIDsAreTheSimulatorsNumbers(t *testing.T) {
	for name, c := range map[string]struct{ got, want DWORD }{
		"OPEN":                  {RECV_ID_OPEN, 2},
		"SIMOBJECT_DATA_BYTYPE": {RECV_ID_SIMOBJECT_DATA_BYTYPE, 9},
		"AIRPORT_LIST":          {RECV_ID_AIRPORT_LIST, 18},
		"FACILITY_DATA":         {RECV_ID_FACILITY_DATA, 28},
		"FACILITY_DATA_END":     {RECV_ID_FACILITY_DATA_END, 29},
	} {
		if c.got != c.want {
			t.Errorf("RECV_ID_%s = %d, the simulator sends %d", name, c.got, c.want)
		}
	}
}
