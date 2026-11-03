package xplane

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// levelRecorder keeps the level of every record it is handed.
type levelRecorder struct {
	mu     sync.Mutex
	levels []slog.Level
}

func (r *levelRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *levelRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levels = append(r.levels, rec.Level)
	return nil
}
func (r *levelRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *levelRecorder) WithGroup(string) slog.Handler      { return r }

// A UDP port already held by another process is the pilot's machine, not a
// fault in the ACARS: it must be logged below Error (which becomes a Sentry
// report) and only once while the failure lasts.
func TestPortInUseIsWarnedOnceNotReportedAsError(t *testing.T) {
	rec := &levelRecorder{}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	listenFailing.Store(false)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		listenFailing.Store(false)
	})

	reportListenFailure(syscall.EADDRINUSE)
	reportListenFailure(errors.New("listen udp 127.0.0.1:58633: bind: address already in use"))

	assert.Equal(t, []slog.Level{slog.LevelWarn}, rec.levels,
		"want one Warn for a port in use; got %v", rec.levels)
}
