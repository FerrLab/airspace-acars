package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// levelRecorder remembers the highest level logged through it.
type levelRecorder struct{ max slog.Level }

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	if rec.Level > r.max {
		r.max = rec.Level
	}
	return nil
}
func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler      { return r }

// A new binary the machine refuses to launch (App Control policy) must not
// raise a Sentry error: the update is installed and the old copy keeps running.
func TestBlockedRelaunchIsAWarningNotAnError(t *testing.T) {
	rec := &levelRecorder{max: slog.LevelDebug}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(prev) })

	blocked := errors.New("fork/exec app.exe: blocked by policy")
	if relaunch("app.exe", func(string) error { return blocked }) {
		t.Fatal("relaunch reported success although the process could not start")
	}
	if rec.max >= slog.LevelError {
		t.Errorf("blocked relaunch logged at %v; it would be reported to Sentry", rec.max)
	}
	if rec.max < slog.LevelWarn {
		t.Errorf("blocked relaunch logged at %v; the failure went unrecorded", rec.max)
	}
}

func TestSuccessfulRelaunchReportsSuccess(t *testing.T) {
	if !relaunch("app.exe", func(string) error { return nil }) {
		t.Fatal("relaunch reported failure although the process started")
	}
}
