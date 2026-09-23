package app

import (
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

const (
	positionPath = "/api/v2/acars/position"

	// uploadQueueDepth is how many batches may be waiting on the uploader
	// before the sampler stops handing them over and writes them straight to
	// the outbox instead. A disk write costs microseconds; a round trip costs
	// as long as the network says. The sampler may pay the first and must
	// never pay the second.
	uploadQueueDepth = 64

	outboxDrainEvery  = time.Second
	uploaderStopGrace = 5 * time.Second
)

// How hard a failed upload is retried before it goes to the outbox. These are
// variables so a test can run the failure path without waiting out the real
// backoff.
var (
	uploadAttempts = 4
	uploadBackoff  = 500 * time.Millisecond
)

// positionUploader owns every position request, so that no sampler ever waits
// on one.
//
// The flare is sampled at roughly 30 Hz by a loop that also used to send. A
// ticker channel holds a single tick and drops the rest, so all the time the
// loop spent inside a blocking POST was time it did not sample — and a sample
// never taken cannot be retried, queued or recovered. A slow but entirely
// successful upload therefore punched a hole in the touchdown data with
// nothing logged and nothing reported.
type positionUploader struct {
	app       *App
	bookingID string
	batches   chan []map[string]interface{}
	stopped   chan struct{}
}

func (a *App) startPositionUploader(bookingID string) *positionUploader {
	u := &positionUploader{
		app:       a,
		bookingID: bookingID,
		batches:   make(chan []map[string]interface{}, uploadQueueDepth),
		stopped:   make(chan struct{}),
	}
	go u.run()
	return u
}

// Submit hands reports over for delivery. It never blocks on the network: if
// the uploader is behind, the reports go to the outbox, which is durable and
// drained later. It does not block for long on anything, because its caller is
// the sampler.
func (u *positionUploader) Submit(reports []map[string]interface{}) {
	if u == nil || len(reports) == 0 {
		return
	}
	for start := 0; start < len(reports); start += maxBatchSize {
		end := start + maxBatchSize
		if end > len(reports) {
			end = len(reports)
		}
		batch := reports[start:end]
		select {
		case u.batches <- batch:
		default:
			// The queue is full. Persisting is the whole point of having an
			// outbox, and is still far cheaper than waiting for a send.
			observability.Count("position.upload_queue_full")
			u.persist(batch, "upload-queue-full")
		}
	}
}

// Stop ends the uploader and makes sure nothing still in hand is lost. What
// has not been sent is written to the outbox rather than sent, so shutdown
// does not wait on a network that may be why we are stopping.
func (u *positionUploader) Stop() {
	if u == nil {
		return
	}
	close(u.batches)
	select {
	case <-u.stopped:
	case <-time.After(uploaderStopGrace):
		slog.Warn("position uploader did not stop in time; its batch is in the outbox")
	}
}

func (u *positionUploader) run() {
	defer close(u.stopped)
	defer observability.Recover()

	drain := time.NewTicker(outboxDrainEvery)
	defer drain.Stop()

	for {
		select {
		case batch, ok := <-u.batches:
			if !ok {
				// Closed: whatever is still queued is persisted rather than
				// sent, so stopping is bounded.
				for rest := range u.batches {
					u.persist(rest, "uploader-stopped")
				}
				return
			}
			u.send(batch)
		case <-drain.C:
			u.drainOutbox()
		}
	}
}

// send delivers one batch, retrying a failure that may pass. A batch that
// still will not go is written to the outbox — never dropped.
func (u *positionUploader) send(reports []map[string]interface{}) {
	backoff := uploadBackoff
	for attempt := 1; attempt <= uploadAttempts; attempt++ {
		_, status, err := u.app.Airspace.DoRequest("POST", positionPath, reports)
		if err == nil {
			err = domain.NewStatusError("POST", positionPath, status, nil)
		}
		if err == nil {
			observability.Add("position.reports_sent", int64(len(reports)))
			return
		}
		if attempt == uploadAttempts {
			slog.Warn("position upload failed; queuing to the outbox",
				"count", len(reports), "attempts", attempt, "error", err)
			break
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	observability.Add("position.reports_failed", int64(len(reports)))
	u.persist(reports, "upload-failed")
}

func (u *positionUploader) drainOutbox() {
	if u.bookingID == "" {
		return
	}
	sent, remaining, err := u.app.drainOutbox(u.bookingID, 1)
	if err != nil {
		slog.Debug("outbox drain failed", "error", err, "remaining", remaining)
		return
	}
	if sent > 0 {
		observability.Add("position.reports_sent", int64(sent))
		slog.Info("outbox drained", "sent", sent, "remaining", remaining)
	}
}

// persist writes reports to the outbox. It is the last line before data is
// lost, so it must not be able to fail on the shape of a report: every value
// is sanitised first, which is why encoding here cannot hit a NaN.
func (u *positionUploader) persist(reports []map[string]interface{}, reason string) {
	if u.bookingID == "" {
		// Without a booking there is nowhere durable to put these. Say so
		// rather than discarding them silently.
		slog.Warn("position reports dropped: no booking to file them under",
			"count", len(reports), "reason", reason)
		observability.Add("position.reports_dropped", int64(len(reports)))
		return
	}
	for _, report := range reports {
		raw, err := json.Marshal(report)
		if err != nil {
			slog.Warn("position: could not encode a report for the outbox",
				"error", err, "reason", reason)
			observability.Count("position.reports_dropped")
			continue
		}
		if err := u.app.DB.EnqueuePosition(u.bookingID, raw); err != nil {
			slog.Warn("position: could not queue a report", "error", err, "reason", reason)
			observability.Count("position.reports_dropped")
			continue
		}
		observability.Count("position.reports_queued")
		observability.Count("position.outbox_enqueued")
	}
}

// MarshalJSON writes a reading, turning one that is not a number into null.
//
// Every number in a position report goes through measurement, so this is the
// one place that has to care. It matters because json.Marshal refuses NaN and
// ±Inf, and it refuses the whole document: a single bad reading from the
// simulator would otherwise fail a batch of 250 samples, and fail again when
// the same batch was written to the outbox. A null loses one reading; the
// alternative lost the flare.
func (mv measurement) MarshalJSON() ([]byte, error) {
	value := mv.Value
	switch f := value.(type) {
	case float64:
		if math.IsNaN(f) || math.IsInf(f, 0) {
			value = nil
		}
	case float32:
		if g := float64(f); math.IsNaN(g) || math.IsInf(g, 0) {
			value = nil
		}
	}
	return json.Marshal(struct {
		Value interface{} `json:"value"`
		Unit  string      `json:"unit"`
	}{value, mv.Unit})
}
