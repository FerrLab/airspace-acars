package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// speechAPI answers the speech routes from a table and records what was
// asked. Anything not in the table is a 404 without a message: a route the
// server does not have.
type speechAPI struct {
	stubAPI
	mu      sync.Mutex
	routes  map[string]reply
	asked   []string
	started map[string]interface{}
}

type reply struct {
	status int
	body   string
}

func newSpeechAPI(routes map[string]reply) *speechAPI {
	return &speechAPI{routes: routes}
}

func (s *speechAPI) DoRequest(method, p string, body interface{}) ([]byte, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := method + " " + p
	s.asked = append(s.asked, key)
	if key == "POST /api/v2/acars/start" {
		s.started, _ = body.(map[string]interface{})
	}
	if method == "POST" {
		return []byte(`{}`), 200, nil
	}
	r, ok := s.routes[key]
	if !ok {
		return []byte(`{"message":"Not Found"}`), 404, nil
	}
	return []byte(r.body), r.status, nil
}

func (s *speechAPI) set(key string, r reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[key] = r
}

func (s *speechAPI) wasAsked(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.asked {
		if k == key {
			return true
		}
	}
	return false
}

// harness runs an attendant against a speechAPI, a clock the test moves and
// a download that names each file after its URL.
type harness struct {
	t      *testing.T
	e      *attendant
	api    *speechAPI
	events chan sentEvent

	mu  sync.Mutex
	now time.Time
}

func newHarness(t *testing.T, routes map[string]reply) *harness {
	t.Helper()
	h := &harness{t: t, api: newSpeechAPI(routes), events: make(chan sentEvent, 1024), now: flightStart}
	h.e = newAttendant(attendantDeps{
		api:      h.api,
		download: func(u string) (string, error) { return path.Base(u), nil },
		emit: func(name string, data interface{}) {
			select {
			case h.events <- sentEvent{name: name, data: data}:
			default:
			}
		},
		now: func() time.Time {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.now
		},
	})
	t.Cleanup(h.e.Stop)
	return h
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.now = h.now.Add(d)
	h.mu.Unlock()
}

// waitFor returns the first event called name that ok accepts.
func (h *harness) waitFor(name string, ok func(data interface{}) bool) interface{} {
	h.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-h.events:
			if ev.name == name && (ok == nil || ok(ev.data)) {
				return ev.data
			}
		case <-deadline:
			h.t.Fatalf("no %s event", name)
			return nil
		}
	}
}

func (h *harness) waitMode(mode string) {
	h.t.Helper()
	h.waitFor(attendantStateEvent, func(d interface{}) bool {
		return d.(*domain.AttendantSnapshot).Mode == mode
	})
}

// take returns the next clip, waiting for one to be ready if need be.
func (h *harness) take() *domain.AttendantClip {
	h.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if c := h.e.TakeClip(); c != nil {
			return c
		}
		select {
		case ev := <-h.events:
			_ = ev
		case <-deadline:
			h.t.Fatal("no clip became ready")
			return nil
		}
	}
}

func atTheGate() *domain.FlightData {
	fd := &domain.FlightData{}
	fd.Sensors.OnGround = true
	return fd
}

func packRoutes() map[string]reply {
	return map[string]reply{
		"GET " + speechPackPath: {200, testPack},
		"GET " + speechLinePath + "5": {200, `{"data":{"id":5,"phase":"boarding_complete","type":"text","language":"en","voice":"Lily",
			"text":"Boarding completed at 18:07","playable":true,"dynamic":true,"audio_url":"https://t.example/a/5-1807.mp3"}}`},
	}
}

// What the server answers to GET /speech/pack decides how the flight's
// announcements are played.
func TestTheServerDecidesTheMode(t *testing.T) {
	cases := []struct {
		name string
		r    reply
		want string
	}{
		{"a pack", reply{200, testPack}, domain.AttendantClient},
		{"an old server", reply{404, `{"message":"The route could not be found."}`}, domain.AttendantLegacy},
		{"an old server refusing the method", reply{405, ``}, domain.AttendantLegacy},
		{"no booking", reply{404, `{"message":"No active booking"}`}, domain.AttendantNoBooking},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, map[string]reply{"GET " + speechPackPath: c.r})
			h.e.Start("123", flightStart)
			h.waitMode(c.want)
		})
	}
}

// A server in trouble is neither an old server nor no booking: the pack is
// read again later, and nothing plays meanwhile.
func TestAFailedPackIsRetried(t *testing.T) {
	prev := packRetryEvery
	packRetryEvery = 10 * time.Millisecond
	t.Cleanup(func() { packRetryEvery = prev })

	h := newHarness(t, map[string]reply{"GET " + speechPackPath: {503, "error code: 503"}})
	h.e.Start("123", flightStart)
	h.waitFor(attendantStateEvent, func(d interface{}) bool {
		s := d.(*domain.AttendantSnapshot)
		return s.Mode == domain.AttendantLoading && strings.Contains(s.Error, "503")
	})
	h.api.set("GET "+speechPackPath, reply{200, testPack})
	h.waitMode(domain.AttendantClient)
}

// Boarding plays its lines, then loops the boarding music; a repeat line
// comes back repeat_every_minutes after it last finished, cutting the music
// short; boarding complete (here, without GSX, the aircraft leaving the
// stand) ends the loop and plays a line rendered now.
func TestBoardingRepeatsWithMusicBetween(t *testing.T) {
	h := newHarness(t, packRoutes())
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	h.e.Observe(atTheGate())
	welcome := h.take()
	assert.Equal(t, "Welcome aboard, flight to São Paulo", welcome.Text)
	assert.Equal(t, domain.ReasonFlightStarted, welcome.Reason.Code)
	assert.Equal(t, []domain.SoundInstruction{{Type: "play", LocalFile: "1.mp3"}}, welcome.Segments)
	h.e.ClipEnded(welcome.ID, true)

	aisle := h.take()
	assert.Equal(t, "Please keep the aisle clear", aisle.Text)
	assert.Equal(t, []domain.SoundInstruction{
		{Type: "play", LocalFile: "2a.mp3"}, {Type: "pause", DurationMs: 300}, {Type: "play", LocalFile: "2b.mp3"},
	}, aisle.Segments, "a line without a combined file plays its segments")
	h.advance(30 * time.Second)
	h.e.ClipEnded(aisle.ID, true)

	music := h.take()
	assert.True(t, music.Music)
	assert.Equal(t, domain.ReasonBoardingMusic, music.Reason.Code)

	h.advance(4 * time.Minute)
	h.e.Observe(atTheGate())
	assert.Empty(t, h.e.Snapshot().Queue, "the repeat is not due yet")

	h.advance(time.Minute)
	h.e.Observe(atTheGate())
	cut := h.waitFor(attendantInterruptEvent, nil)
	require.NotNil(t, cut)
	again := h.take()
	assert.Equal(t, "Please keep the aisle clear", again.Text)
	assert.Equal(t, domain.ReasonBoardingRepeat, again.Reason.Code)
	assert.Equal(t, 5, again.Reason.Minutes)
	h.e.ClipEnded(again.ID, true)

	ready := atTheGate()
	ready.Lights.Beacon = true
	h.e.Observe(ready)
	still := h.take()
	assert.True(t, still.Music, "without GSX the beacon does not end boarding: the music plays on")
	h.e.ClipEnded(still.ID, true)

	ready.Attitude.GS = 2
	h.e.Observe(ready)
	done := h.take()
	assert.Equal(t, "Boarding completed at 18:07", done.Text, "a dynamic line is rendered when it plays")
	assert.Equal(t, domain.ReasonLeavingGate, done.Reason.Code)
	assert.True(t, h.api.wasAsked("GET "+speechLinePath+"5"))
	h.e.ClipEnded(done.ID, true)

	h.advance(10 * time.Minute)
	h.e.Observe(ready)
	assert.Nil(t, h.e.TakeClip(), "no music and no repeats once boarding is complete")
}

// The after-landing lines are asked for the touchdown that happened.
func TestAfterLandingAsksForTheTouchdownsLines(t *testing.T) {
	routes := packRoutes()
	routes["GET "+speechPhasePath+"after_landing?landing=smooth"] = reply{200,
		`{"data":{"phase":"after_landing","lines":[{"id":6,"type":"text","text":"What a landing","playable":true,"audio_url":"https://t.example/a/6.mp3"}]}}`}
	h := newHarness(t, routes)
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	h.e.mu.Lock()
	h.e.firedLocked(phaseFire{phase: domain.PhaseAfterLanding, landing: domain.LandingSmooth,
		reason: domain.AttendantReason{Code: domain.ReasonTouchdown, VS: -142, G: 1.08, Landing: domain.LandingSmooth}}, flightStart)
	h.e.mu.Unlock()

	c := h.take()
	assert.Equal(t, "What a landing", c.Text)
	assert.Equal(t, -142, c.Reason.VS)
	assert.Equal(t, domain.PhaseAfterLanding, h.e.Snapshot().Current)
}

// A diversion reads the pack again for the alternate, so the lines name it,
// and announces it.
func TestADiversionRereadsThePackForTheAlternate(t *testing.T) {
	routes := packRoutes()
	alternate := strings.NewReplacer(`"arrival":"SBGR"`, `"arrival":"SBKP"`, `"revision":"r1"`, `"revision":"r2"`,
		"We are diverting", "We are diverting to Campinas", "a/8.mp3", "a/8-sbkp.mp3").Replace(testPack)
	routes["GET "+speechPackPath+"?arrival=SBKP"] = reply{200, alternate}
	h := newHarness(t, routes)
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	h.e.ArrivalChanged("SBKP")
	c := h.take()
	assert.Equal(t, "We are diverting to Campinas", c.Text)
	assert.Equal(t, []domain.SoundInstruction{{Type: "play", LocalFile: "8-sbkp.mp3"}}, c.Segments)
	assert.Equal(t, domain.AttendantReason{Code: domain.ReasonDiversion, Airport: "SBKP"}, c.Reason)

	snap := h.e.Snapshot()
	assert.Equal(t, "SBKP", snap.Arrival)
	for _, s := range snap.Steps {
		if s.Key == domain.PhaseAlternate {
			assert.Equal(t, domain.StepFired, s.Status)
		}
	}
}

// Turned off, the attendant follows the flight but plays nothing.
func TestADisabledAttendantPlaysNothing(t *testing.T) {
	h := newHarness(t, packRoutes())
	h.e.SetEnabled(false)
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	h.e.Observe(atTheGate())
	assert.Nil(t, h.e.TakeClip())
	snap := h.e.Snapshot()
	require.NotEmpty(t, snap.History)
	assert.Equal(t, domain.ClipMuted, snap.History[0].Outcome)
	assert.Equal(t, domain.PhaseBoarding, snap.Current)
}

func TestSkipStopAndReplay(t *testing.T) {
	h := newHarness(t, packRoutes())
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)
	h.e.Observe(atTheGate())

	first := h.take()
	h.e.ClipEnded(first.ID, true)
	second := h.take()
	h.e.Skip()
	assert.Equal(t, second.ID, h.waitFor(attendantInterruptEvent, nil))

	h.e.Replay()
	replayed := h.take()
	assert.Equal(t, first.Text, replayed.Text, "the last line heard to the end")
	assert.Equal(t, domain.ReasonReplay, replayed.Reason.Code)

	h.e.StopAll()
	assert.Equal(t, replayed.ID, h.waitFor(attendantInterruptEvent, nil))
	assert.Nil(t, h.e.TakeClip(), "stop all ends the boarding music too")

	outcomes := map[string]bool{}
	for _, e := range h.e.Snapshot().History {
		outcomes[e.Outcome] = true
	}
	assert.True(t, outcomes[domain.ClipSkipped])
	assert.True(t, outcomes[domain.ClipStopped])
}

func TestPlayPhaseNowNeedsThePack(t *testing.T) {
	h := newHarness(t, map[string]reply{"GET " + speechPackPath: {405, ""}})
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantLegacy)
	assert.ErrorIs(t, h.e.PlayPhaseNow(domain.PhaseCruise, 0), errPackNotLoaded)

	h2 := newHarness(t, packRoutes())
	h2.e.Start("123", flightStart)
	h2.waitMode(domain.AttendantClient)
	require.NoError(t, h2.e.PlayPhaseNow(domain.PhaseMealService, 2))
	c := h2.take()
	assert.Equal(t, "Breakfast is served", c.Text)
	assert.Equal(t, domain.ReasonManual, c.Reason.Code)
	assert.Empty(t, h2.e.Snapshot().Current, "a manual play is not the phase firing")
}

// While the attendant plays the pack, the server queues nothing for the old
// poll, so the poll does not ask.
func TestTheSoundPollIsSilentInClientMode(t *testing.T) {
	h := newHarness(t, packRoutes())
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	a := &App{Airspace: h.api, attendant: h.e}
	got, err := a.FetchSoundInstructions()
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.False(t, h.api.wasAsked("GET /api/v2/acars/sound"))
}

// The server stops queuing /sound audio for a booking whose ACARS says it
// plays the announcements itself.
func TestStartFlightSaysTheACARSPlaysTheAnnouncements(t *testing.T) {
	api := newSpeechAPI(packRoutes())
	a, _, sim := newAutoApp(t, &routeAPI{})
	a.Airspace = api
	sim.set(atTheGate())

	require.NoError(t, a.StartFlight("AZU4512", "SBGR", "SBRJ", "42"))
	assert.Equal(t, "client", api.started["speech"])
}

func TestTheSnapshotListsThePhasesInFlightOrder(t *testing.T) {
	h := newHarness(t, packRoutes())
	h.e.Start("123", flightStart)
	h.waitMode(domain.AttendantClient)

	var keys []string
	lines := map[string]int{}
	for _, s := range h.e.Snapshot().Steps {
		keys = append(keys, s.Key)
		lines[s.Key] = s.Lines
	}
	assert.Equal(t, []string{"gate_departure", "boarding_complete", "pushback", "taxi", "climb_out",
		"passing_10000", "cruise", "meal_service:1", "meal_service:2", "seatbelt_off", "seatbelt_on",
		"descend", "descending_10000", "final_approach", "after_landing", "gate_arrival", "alternate_declared"}, keys)
	assert.Equal(t, 3, lines["gate_departure"])
	assert.Equal(t, 0, lines["pushback"], "a phase without lines is silent")
	assert.Equal(t, 1, lines["meal_service:2"])

	raw, err := json.Marshal(h.e.Snapshot())
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"nowPlaying":null`)
}

// An error page is not audio: cached under the URL, it would be played and
// fail for as long as the cache lives.
func TestAudioDownloadRefusesAnErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "missing.mp3") {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3 audio"))
	}))
	defer srv.Close()

	a := &App{audioCacheDir: t.TempDir(), audioClient: srv.Client()}
	_, err := a.downloadAndCacheAudio(srv.URL + "/missing.mp3")
	assert.Error(t, err)
	entries, _ := os.ReadDir(a.audioCacheDir)
	assert.Empty(t, entries, "nothing is cached for an error page")

	name, err := a.downloadAndCacheAudio(srv.URL + "/ok.mp3")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(name, ".mp3"))
}
