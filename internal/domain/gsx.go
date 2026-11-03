package domain

// GSXServiceState is a GSX service's state as GSX numbers it. The remote
// control server's stateRaw is assumed to carry the same values as the
// documented L:FSDT_GSX_<SERVICE>_STATE variables; that is unverified, since
// the server's protocol guide ships only inside the GSX installer.
type GSXServiceState int

// The values of L:FSDT_GSX_<SERVICE>_STATE.
const (
	GSXUnknown      GSXServiceState = 0
	GSXCallable     GSXServiceState = 1
	GSXNotAvailable GSXServiceState = 2
	GSXBypassed     GSXServiceState = 3
	GSXRequested    GSXServiceState = 4
	GSXActive       GSXServiceState = 5
	GSXCompleted    GSXServiceState = 6
	// GSXCompleting is reported by some GSX builds between active and
	// completed.
	GSXCompleting GSXServiceState = 7
)

// InProgress reports whether the service has been called and not finished:
// requested, active or completing.
func (s GSXServiceState) InProgress() bool {
	return s == GSXRequested || s == GSXActive || s == GSXCompleting
}

// GSXService is one GSX ground service as the ACARS follows it.
type GSXService struct {
	State GSXServiceState `json:"state"`
	// Completed is latched when the service reports completed, or when it
	// drops from in progress back to callable or below: GSX resets a
	// finished service to callable without always passing through
	// completed. It clears when the service is called again.
	Completed bool `json:"completed"`
	// Progress is GSX's own progress text ("Boarding 45/180"), shown as is.
	Progress string `json:"progress"`
}

// GSXState is what the ACARS knows of GSX: whether its remote control server
// answers, and the services the flight attendant follows.
type GSXState struct {
	Connected  bool       `json:"connected"`
	Running    bool       `json:"running"` // GSX itself is running in the simulator
	Boarding   GSXService `json:"boarding"`
	Deboarding GSXService `json:"deboarding"`
	Departure  GSXService `json:"departure"` // pushback
}
