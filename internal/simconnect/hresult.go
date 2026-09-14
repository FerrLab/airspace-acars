package simconnect

import "fmt"

// hresultError renders a failed SimConnect call.
//
// SimConnect reports failure through the HRESULT it returns, not through
// SetLastError, so the error value syscall hands back is whatever happened to
// be in the thread's last-error slot — usually ERROR_SUCCESS. Formatting it
// produced the contradiction "SimConnect_Open: -2147467259 The operation
// completed successfully", which reads as a bug in the ACARS rather than as
// the simulator not running. The last-error value is dropped for that reason.
func hresultError(call string, r1 uintptr) error {
	hr := uint32(int32(r1))
	switch hr {
	case 0x80004005: // E_FAIL
		return fmt.Errorf("%s: 0x%08X (E_FAIL) — the simulator is probably not running", call, hr)
	case 0x80004003: // E_POINTER
		return fmt.Errorf("%s: 0x%08X (E_POINTER)", call, hr)
	case 0x80070005: // E_ACCESSDENIED
		return fmt.Errorf("%s: 0x%08X (E_ACCESSDENIED) — the simulator may be running elevated", call, hr)
	case 0x8007000E: // E_OUTOFMEMORY
		return fmt.Errorf("%s: 0x%08X (E_OUTOFMEMORY)", call, hr)
	default:
		return fmt.Errorf("%s: 0x%08X", call, hr)
	}
}
