package app

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"airspace-acars/internal/domain"
)

// reportTap keeps the last position report the uploader handled, and what
// became of it, for the debug screen; before a flight, the report that would
// be sent. It holds the report map itself and encodes it only when asked, so
// a screen nobody is looking at costs nothing.
type reportTap struct {
	mu        sync.Mutex // guards every field below
	report    map[string]interface{}
	at        time.Time
	outcome   string
	batchSize int
}

// record keeps the last report of a batch and its outcome, one of the
// domain.Report* values. A nil tap records nothing.
func (t *reportTap) record(batch []map[string]interface{}, outcome string) {
	if t == nil || len(batch) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.report = batch[len(batch)-1]
	t.at = time.Now()
	t.outcome = outcome
	t.batchSize = len(batch)
}

// snapshot encodes the kept report with the encoder the upload uses. A report
// map is not changed after it is built, so it is encoded outside the lock.
func (t *reportTap) snapshot() domain.PositionReportSnapshot {
	if t == nil {
		return domain.PositionReportSnapshot{}
	}
	t.mu.Lock()
	report, at, outcome, n := t.report, t.at, t.outcome, t.batchSize
	t.mu.Unlock()
	if report == nil {
		return domain.PositionReportSnapshot{}
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		// measurement already turns NaN into null; anything else that will
		// not encode is shown as such rather than hidden.
		raw = []byte(fmt.Sprintf("encode report: %v", err))
	}
	return domain.PositionReportSnapshot{JSON: string(raw), At: at, Outcome: outcome, BatchSize: n}
}
