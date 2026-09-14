// Package winaudio names this application's audio session on Windows so the
// volume mixer shows the ACARS rather than the webview hosting its audio.
package winaudio

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// pollInterval is how often the session is re-named.
//
// A session is only created once something has played, and it is destroyed
// again after the sound stops, so there is no single moment at which this can
// be done once. Polling is the only option; the work is a COM enumeration of
// the sessions on one endpoint, which is cheap enough at this rate.
const pollInterval = 15 * time.Second

// Start keeps this process tree's audio sessions named until ctx is done. It
// is a no-op off Windows.
//
// Everything here is cosmetic: a failure costs a mislabelled volume slider and
// must never affect the flight, so errors are logged once and the loop keeps
// going.
func Start(ctx context.Context, displayName string) {
	icon := iconPath()

	var lastErr string
	for {
		if _, err := Label(displayName, icon); err != nil && err.Error() != lastErr {
			// Only the first of a repeating error is logged: this runs
			// every few seconds for the life of the process.
			lastErr = err.Error()
			slog.Debug("could not name the audio session", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// iconPath returns the icon the volume mixer should show, in the
// "file,index" form Windows expects for an icon resource. An empty string
// leaves whatever icon the session already has.
func iconPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe + ",0"
}
