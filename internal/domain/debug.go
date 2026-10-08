package domain

import "time"

// GroundStatus is where the ACARS places the aircraft on an airport, for the
// debug screen: the airport whose layout it holds and the runway or stand the
// aircraft was last found on.
type GroundStatus struct {
	Airport   string    `json:"airport"`   // ICAO, "" when no layout is held
	Runways   int       `json:"runways"`   // in the held layout
	Stands    int       `json:"stands"`    // in the held layout
	Runway    string    `json:"runway"`    // "36/18", "" when not on one
	Stand     string    `json:"stand"`     // "7", "" when not on one
	LoadedAt  time.Time `json:"loadedAt"`  // zero when no layout is held
	LastError string    `json:"lastError"` // the last lookup's failure, "" after a success
}

// What became of a position report the debug screen shows.
const (
	ReportSent    = "sent"    // the API accepted it
	ReportOutbox  = "outbox"  // it failed and waits in the outbox
	ReportDropped = "dropped" // it had no booking to be filed under
	ReportPreview = "preview" // no flight: the report that would be sent
)

// PositionReportSnapshot is a position report as the uploader handled it:
// the report itself, encoded the way it was sent, and its fate.
type PositionReportSnapshot struct {
	JSON      string    `json:"json"`      // indented report, "" before any
	At        time.Time `json:"at"`        // when it was handled
	Outcome   string    `json:"outcome"`   // one of the Report* values
	BatchSize int       `json:"batchSize"` // reports in the request it came from
}

// DebugSnapshot is everything the debug screen polls for.
type DebugSnapshot struct {
	Ground   GroundStatus           `json:"ground"`
	Report   PositionReportSnapshot `json:"report"`
	Profiles []string               `json:"profiles"` // active aircraft profile IDs
}
