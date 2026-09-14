package simconnect

import (
	"strings"
	"testing"
)

// The live report read "SimConnect_Open: -2147467259 A operação foi concluída
// com êxito" — a failure paired with "the operation completed successfully",
// because SimConnect signals failure through the HRESULT and leaves the
// thread's last-error slot alone.
func TestHresultErrorExplainsTheSimulatorBeingClosed(t *testing.T) {
	err := hresultError("SimConnect_Open", uintptr(uint32(0x80004005)))
	msg := err.Error()

	if !strings.Contains(msg, "0x80004005") || !strings.Contains(msg, "E_FAIL") {
		t.Fatalf("HRESULT should be legible, got %q", msg)
	}
	if !strings.Contains(msg, "not running") {
		t.Fatalf("E_FAIL on open should say what it usually means, got %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "success") {
		t.Fatalf("a failure must not claim success, got %q", msg)
	}
}

func TestHresultErrorFallsBackToTheRawCode(t *testing.T) {
	msg := hresultError("SimConnect_Close", uintptr(uint32(0xDEADBEEF))).Error()
	if !strings.Contains(msg, "0xDEADBEEF") {
		t.Fatalf("unknown codes should still be readable, got %q", msg)
	}
}
