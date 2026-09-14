package winaudio

import (
	"context"
	"testing"
	"time"
)

// Start runs for the life of the process, so a cancelled context has to end
// it promptly rather than at the next poll.
func TestStartStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		Start(ctx, "Airspace ACARS")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after its context was cancelled")
	}
}

func TestIconPathNamesAResource(t *testing.T) {
	// Windows wants "file,index"; an empty result is the documented
	// fallback when the executable cannot be located.
	if got := iconPath(); got != "" && !hasSuffix(got, ",0") {
		t.Errorf("iconPath() = %q, want it to end in an icon index", got)
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
