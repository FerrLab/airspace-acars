package app

import "airspace-acars/internal/domain"

// observeGround places the aircraft once per data stream tick and, outside a
// flight, keeps the report that would be sent. During a flight the uploader
// keeps the real one, which a preview must not replace.
func (a *App) observeGround(locator *groundLocator, fd *domain.FlightData) {
	fix := locator.Observe(fd)
	if a.flightIdle() {
		a.reports.record([]map[string]interface{}{a.buildPositionReport(fd, fix)}, domain.ReportPreview)
	}
}

// flightIdle reports whether no flight is under way.
func (a *App) flightIdle() bool {
	a.flightMu.Lock()
	defer a.flightMu.Unlock()
	return a.state == "" || a.state == "idle"
}

// DebugSnapshot is what the debug screen polls for: where the aircraft is on
// the airport, the last position report and its fate, and the active
// aircraft profiles.
func (a *App) DebugSnapshot() domain.DebugSnapshot {
	snap := domain.DebugSnapshot{
		Ground: a.currentLocator().Status(),
		Report: a.reports.snapshot(),
		// [] rather than null: the generated bindings type it as an array.
		Profiles: []string{},
	}
	if plan := a.GetActiveProfile(); plan != nil {
		if ids := plan.ProfileIDs(); ids != nil {
			snap.Profiles = ids
		}
	}
	return snap
}
