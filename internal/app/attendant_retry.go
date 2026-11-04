package app

import (
	"errors"
	"log/slog"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// lineRetry is a line that could not be prepared and is to be asked for
// again: on 2026-11-04 a Samba boarding welcome was lost because the server's
// text-to-speech provider timed out once, and nothing asked again.
type lineRetry struct {
	line speechLine
	fire phaseFire
	// fresh says the line was rendered for this fire, as after_landing's
	// are, so only its audio is fetched again.
	fresh bool
	// during is the flight's step when the line's phase fired. Once the
	// flight is at another, the line is given up: a welcome aboard heard
	// during the pushback is worse than none.
	during string
	// failures counts the tries that have failed, 1 after the first.
	failures int
	due      time.Time
}

// lineRetryAfter decides whether a line that could not be prepared is asked
// for again, and how long to wait first. failures is how many tries have
// failed so far: 1 after the first.
//
// The errors that reach it: errLineNotReady (the server could not produce
// the audio yet), errLineUnvoiced (it never will: no voice, empty, disabled),
// errLineGone (the pack changed; lineFailed reads it again), errNoBooking,
// and whatever the network or a 5xx gave, which observability.Transient
// recognises.
//
// Audio the server could not produce yet and a network or server failure
// are worth another try; a refusal the server means (errLineUnvoiced), a
// changed pack and no booking are not.
func lineRetryAfter(err error, failures int) (wait time.Duration, retry bool) {
	if !errors.Is(err, errLineNotReady) && !observability.Transient(err) {
		return 0, false
	}
	if failures < 1 || failures > len(lineRetryWaits) {
		return 0, false
	}
	return lineRetryWaits[failures-1], true
}

// lineRetryWaits are the pauses before a line is asked for again, one per
// failed try; when they run out the line is given up. The server waits 30 s
// on its text-to-speech provider for each sentence, so asking sooner only
// stacks requests on a provider already in trouble, and each try may be
// billed again. Three and a half minutes in all rides out a provider's bad
// moment and still lands within boarding or a cruise; a phase the flight
// leaves meanwhile drops the line anyway.
var lineRetryWaits = []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute}

// lineNotReady handles a line that could not be prepared: it waits to be
// asked for again when lineRetryAfter says so, and is recorded as failed
// otherwise.
func (e *attendant) lineNotReady(gen int, r lineRetry, err error) {
	r.failures++
	wait, retry := lineRetryAfter(err, r.failures)
	if !retry {
		e.lineFailed(gen, r.line, r.fire, err)
		return
	}
	observability.Count("attendant.line_retry")
	slog.Debug("attendant: line not ready; will ask again", "attendant.line", r.line.id(),
		"attendant.failures", r.failures, "attendant.wait", wait, "error", err)

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gen != gen || !e.enabled {
		return
	}
	r.due = e.deps.now().Add(wait)
	e.retries = append(e.retries, r)
}

// retriesLocked hands the worker the lines whose wait is over, and gives up
// those whose step the flight has left. Callers hold e.mu.
func (e *attendant) retriesLocked(now time.Time) {
	if len(e.retries) == 0 {
		return
	}
	kept := e.retries[:0]
	for _, r := range e.retries {
		switch {
		case r.during != e.current:
			slog.Info("attendant: line given up; the flight has moved on", "attendant.line", r.line.id(),
				"attendant.phase", r.fire.stepKey(), "attendant.failures", r.failures)
			e.history = appendHistory(e.history, failedEntry(now, r.line, r.fire))
		case now.Before(r.due):
			kept = append(kept, r)
		default:
			e.sendLocked(attendantJob{gen: e.gen, kind: jobRetry, retry: r})
		}
	}
	e.retries = kept
}

// retryLine asks for a line again and queues it for the phase that fired it.
func (e *attendant) retryLine(gen int, r lineRetry) {
	e.mu.Lock()
	arrival := e.packArrival
	e.mu.Unlock()

	rendered, segs, err := e.prepare(r.line, arrival, r.fresh)
	if err != nil {
		e.lineNotReady(gen, r, err)
		// A line given up shows as failed on the tab.
		e.notify()
		return
	}
	e.enqueue(gen, []queuedClip{clipFor(rendered, r.fire, segs)}, false)
}

// failedEntry is the history's record of a line that never played.
func failedEntry(at time.Time, l speechLine, f phaseFire) domain.AttendantHistoryEntry {
	return domain.AttendantHistoryEntry{
		At: at, Phase: f.phase, Service: l.service(), Text: l.Text,
		Language: l.Language, Voice: l.Voice, Outcome: domain.ClipFailed, Reason: f.reason}
}
