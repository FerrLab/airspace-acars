package app

import (
	"strings"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The welcome line of testPack, made one the server voices when it plays, as
// the Samba boarding line that failed on 2026-11-04 was.
const welcomeText = "Welcome aboard, flight to São Paulo"

var (
	welcomeRoute = "GET " + speechLinePath + "1"
	notVoiced    = reply{422, `{"message":"This line cannot be played","reason":"not_generated","data":{"id":1}}`}
	neverVoiced  = reply{422, `{"message":"This line cannot be played","reason":"no_voice","data":{"id":1}}`}
	voiced       = reply{200, `{"data":{"id":1,"phase":"gate_departure","type":"text","language":"en","voice":"Lily",
		"text":"` + welcomeText + `","playable":true,"dynamic":true,"audio_url":"https://t.example/a/1-now.mp3"}}`}
)

// retryStep is how far the tests move the clock between readings: past any
// wait the retry policy may choose.
const retryStep = time.Hour

func welcomeRoutes(first reply) map[string]reply {
	routes := packRoutes()
	routes["GET "+speechPackPath] = reply{200, strings.Replace(testPack,
		`"dynamic":false,"audio_url":"https://t.example/a/1.mp3"`, `"dynamic":true,"audio_url":null`, 1)}
	routes[welcomeRoute] = first
	return routes
}

// boardingWithWelcome starts a flight at the gate and returns the clip that
// played while the welcome line was refused: the boarding line that was
// ready.
func boardingWithWelcome(t *testing.T, first reply) (*harness, *domain.AttendantClip) {
	t.Helper()
	h := newHarness(t, welcomeRoutes(first))
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)
	h.e.Observe(atTheGate())
	aisle := h.take()
	require.Equal(t, "Please keep the aisle clear", aisle.Text, "the boarding line that was ready plays meanwhile")
	return h, aisle
}

func failedWelcome(s domain.AttendantSnapshot) bool {
	for _, e := range s.History {
		if e.Text == welcomeText && e.Outcome == domain.ClipFailed {
			return true
		}
	}
	return false
}

// A line the server could not voice yet (its TTS provider timed out) is asked
// for again, and plays for the phase that fired it, instead of being lost.
func TestALineTheServerHasNotVoicedYetPlaysOnceItIs(t *testing.T) {
	h, aisle := boardingWithWelcome(t, notVoiced)
	assert.False(t, failedWelcome(h.e.Snapshot()), "a line still being tried is not shown as failed")

	h.api.set(welcomeRoute, voiced)
	h.advance(retryStep)
	h.e.Observe(atTheGate())
	h.e.ClipEnded(aisle.ID, true)

	welcome := h.take()
	assert.Equal(t, welcomeText, welcome.Text, "the welcome was dropped instead of tried again")
	assert.Equal(t, domain.ReasonFlightStarted, welcome.Reason.Code, "it plays for the phase that fired it")
	assert.Equal(t, []domain.SoundInstruction{{Type: "play", LocalFile: "1-now.mp3"}}, welcome.Segments)
	assert.Equal(t, 2, h.api.timesAsked(welcomeRoute))
}

// The server in trouble (a 503 from it or the CDN in front) is not the line's
// fault: it is asked for again like audio not generated yet.
func TestALineLostToAServerErrorIsTriedAgain(t *testing.T) {
	h, aisle := boardingWithWelcome(t, reply{503, "error code: 503"})

	h.api.set(welcomeRoute, voiced)
	h.advance(retryStep)
	h.e.Observe(atTheGate())
	h.e.ClipEnded(aisle.ID, true)

	assert.Equal(t, welcomeText, h.take().Text, "a 503 lost the welcome")
}

// A line that can never play (no voice for its language) fails at once, as
// before: asking again would only fail again.
func TestALineRefusedForGoodIsNotTriedAgain(t *testing.T) {
	h, _ := boardingWithWelcome(t, neverVoiced)
	assert.True(t, failedWelcome(h.e.Snapshot()), "a final refusal is shown as failed straight away")

	h.advance(retryStep)
	h.e.Observe(atTheGate())
	assert.Equal(t, 1, h.api.timesAsked(welcomeRoute), "a line that cannot be voiced was asked for again")
}

// An announcement is only worth playing while the flight is still where it
// fired: a welcome aboard heard during the pushback is worse than none.
func TestARetryIsDroppedOnceTheFlightMovesOn(t *testing.T) {
	h, aisle := boardingWithWelcome(t, notVoiced)
	h.e.ClipEnded(aisle.ID, true)

	leaving := atTheGate()
	leaving.Attitude.GS = 2
	h.e.Observe(leaving)
	assert.True(t, failedWelcome(h.e.Snapshot()), "the welcome is given up once boarding is over")

	h.api.set(welcomeRoute, voiced)
	h.advance(retryStep)
	h.e.Observe(leaving)
	for c := h.e.TakeClip(); c != nil; c = h.e.TakeClip() {
		assert.NotEqual(t, welcomeText, c.Text, "the welcome played after the aircraft left the gate")
		h.e.ClipEnded(c.ID, true)
	}
	assert.Equal(t, 1, h.api.timesAsked(welcomeRoute))
}

// A server that never voices the line is not asked forever: the line is given
// up and shown as failed.
func TestALineThatNeverBecomesReadyIsGivenUp(t *testing.T) {
	h, _ := boardingWithWelcome(t, notVoiced)

	deadline := time.After(5 * time.Second)
	for !failedWelcome(h.e.Snapshot()) {
		h.advance(retryStep)
		h.e.Observe(atTheGate())
		select {
		case <-h.events:
		case <-deadline:
			t.Fatalf("the welcome was still being retried after %d requests", h.api.timesAsked(welcomeRoute))
		}
	}
	assert.Greater(t, h.api.timesAsked(welcomeRoute), 1, "the line was given up without a second try")
}
