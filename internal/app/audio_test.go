package app

import (
	"errors"
	"testing"

	"airspace-acars/internal/domain"
)

// The sound poll runs on a timer whether or not the pilot has taken a
// booking, and the server answers 404 "No active booking" as the normal
// negative - not a fault. noActiveBooking is what FetchSoundInstructions
// uses to route that status to span.Expected instead of span.Fail.
func TestNoActiveBookingMatchesTheSoundEndpointsExpected404(t *testing.T) {
	err := domain.NewStatusError("GET", "/api/v2/acars/sound", 404, []byte(`{"message":"No active booking"}`))
	if !noActiveBooking(err) {
		t.Errorf("noActiveBooking(%v) = false, want true", err)
	}
}

func TestNoActiveBookingRejectsOtherStatuses(t *testing.T) {
	for _, status := range []int{401, 403, 500, 502} {
		err := domain.NewStatusError("GET", "/api/v2/acars/sound", status, nil)
		if noActiveBooking(err) {
			t.Errorf("noActiveBooking(%v) = true for status %d, want false", err, status)
		}
	}
}

func TestNoActiveBookingRejectsNonStatusErrors(t *testing.T) {
	if noActiveBooking(errors.New("boom")) {
		t.Error("noActiveBooking(plain error) = true, want false")
	}
	if noActiveBooking(nil) {
		t.Error("noActiveBooking(nil) = true, want false")
	}
}

// FetchSoundInstructions still returns the 404 to the caller - the frontend
// poll already discards the error - it just must not be reported as a fault.
func TestFetchSoundInstructionsReturnsErrorOnNoActiveBooking(t *testing.T) {
	api := &stubAPI{status: 404, body: []byte(`{"message":"No active booking"}`)}
	a := &App{Airspace: api}

	instructions, err := a.FetchSoundInstructions()
	if err == nil {
		t.Fatal("FetchSoundInstructions returned no error for a 404")
	}
	if instructions != nil {
		t.Errorf("instructions = %v, want nil", instructions)
	}

	var se *domain.StatusError
	if !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("error does not name the status: %v", err)
	}
}
