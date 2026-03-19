package ports

// SimEventsPort receives data from simulator adapters and feeds it into
// the application layer. In the current implementation, the App manages
// the data stream loop directly — this port exists as the architectural
// boundary point for future extensions (e.g. event detection, takeoff/landing).
//
// The sim data streaming is started automatically when ConnectSim succeeds.
// No additional setup is required from the ports layer.
