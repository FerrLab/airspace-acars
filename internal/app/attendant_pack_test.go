package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pack with every kind of line the engine treats differently.
const testPack = `{"data":{"version":1,"revision":"r1","booking_id":123,
 "flight":{"departure":"LPPT","arrival":"SBGR","is_alternate":false,"original_arrival":null,
   "planned_block_minutes":590,"time_of_day":"night",
   "meal_services":[{"n":1,"meal":"dinner","timing":"after_cruise","minutes":30},
                    {"n":2,"meal":"breakfast","timing":"before_descent","minutes":"90"}]},
 "settings":{"seatbelt":{"on_cooldown_minutes":2,"off_cooldown_minutes":3},
   "landing":{"smooth_max_fpm":180,"smooth_max_g":1.2,"hard_min_fpm":400,"hard_min_g":1.5},
   "night":{"from":"22:00","to":"06:00"}},
 "phases":[
  {"phase":"gate_departure","label":"Boarding","lines":[
    {"id":2,"phase":"gate_departure","sort_order":1,"type":"text","language":"en","voice":"Lily",
     "conditions":{"time_of_day":"any","repeat":true,"repeat_every_minutes":5},"service":null,
     "text":"Please keep the aisle clear","playable":true,"skip_reason":null,"dynamic":false,
     "audio_url":null,"segments":[{"type":"play","url":"https://t.example/a/2a.mp3"},{"type":"pause","duration_ms":300},{"type":"play","url":"https://t.example/a/2b.mp3"}],
     "fetch_url":"https://t.example/api/v2/acars/speech/lines/2","duration_ms":null},
    {"id":"1","phase":"gate_departure","sort_order":0,"type":"text","language":"en","voice":"Lily",
     "conditions":{"repeat":false},"text":"Welcome aboard, flight to São Paulo","playable":true,
     "dynamic":false,"audio_url":"https://t.example/a/1.mp3","segments":[]},
    {"id":3,"phase":"gate_departure","sort_order":2,"type":"audio","language":null,"voice":null,
     "conditions":{"repeat":true,"repeat_every_minutes":5},"text":null,"playable":true,
     "dynamic":false,"audio_url":"https://t.example/a/music.mp3","segments":[]},
    {"id":4,"phase":"gate_departure","sort_order":3,"type":"text","language":"pt","voice":"Ana",
     "conditions":{},"text":"Boa noite","playable":false,"skip_reason":"time_of_day","audio_url":null}]},
  {"phase":"boarding_complete","label":"Boarding complete","lines":[
    {"id":5,"sort_order":0,"type":"text","language":"en","voice":"Lily","conditions":{},
     "text":"Boarding is complete","playable":true,"dynamic":true,"audio_url":null,"segments":[]}]},
  {"phase":"pushback","label":"Pushback","lines":[]},
  {"phase":"cruise","label":"Cruise","lines":[
    {"id":9,"sort_order":0,"type":"text","language":"en","voice":"Lily","conditions":{"cruise_at":"cruise"},
     "text":"We have reached our cruising altitude","playable":true,"audio_url":"https://t.example/a/9.mp3"},
    {"id":10,"sort_order":1,"type":"text","language":"en","voice":"Lily","conditions":{"cruise_at":50},
     "text":"We are halfway","playable":true,"audio_url":"https://t.example/a/10.mp3"}]},
  {"phase":"meal_service","label":"Meal service","lines":[
    {"id":11,"type":"text","language":"en","voice":"Lily","conditions":{"meal":"dinner"},"service":1,
     "text":"Dinner is served","playable":true,"audio_url":"https://t.example/a/11-1.mp3"},
    {"id":11,"type":"text","language":"en","voice":"Lily","conditions":{"meal":"any"},"service":2,
     "text":"Breakfast is served","playable":true,"audio_url":"https://t.example/a/11-2.mp3"}]},
  {"phase":"after_landing","label":"After landing","lines":[
    {"id":6,"type":"text","language":"en","voice":"Lily","conditions":{"landing":"smooth"},
     "text":"What a landing","playable":true,"audio_url":"https://t.example/a/6.mp3"},
    {"id":7,"type":"text","language":"en","voice":"Lily","conditions":{"landing":"any"},
     "text":"Welcome to São Paulo","playable":true,"audio_url":"https://t.example/a/7.mp3"}]},
  {"phase":"alternate_declared","label":"Diversion","lines":[
    {"id":8,"type":"text","language":"en","voice":"Lily","conditions":{},
     "text":"We are diverting","playable":true,"audio_url":"https://t.example/a/8.mp3"}]}
 ]}}`

func TestParseSpeechPack(t *testing.T) {
	p, err := parseSpeechPack([]byte(testPack))
	require.NoError(t, err)

	assert.Equal(t, "r1", p.Revision)
	assert.Equal(t, 590.0, float64(p.Flight.PlannedBlockMinutes))
	require.Len(t, p.Flight.MealServices, 2)
	assert.Equal(t, 90.0, float64(p.Flight.MealServices[1].Minutes), "minutes sent as a string")
	assert.Equal(t, 180.0, p.Settings.Landing.SmoothMaxFPM)

	boarding := p.playable(domain.PhaseBoarding)
	require.Len(t, boarding, 3, "the unplayable line is left out")
	assert.Equal(t, 1, boarding[0].id(), "lines are in sort_order, and a string id is read")
	assert.Equal(t, 2, boarding[1].id())
	assert.True(t, boarding[2].isMusic())
	assert.True(t, boarding[1].Conditions.Repeat)

	cruise := p.playable(domain.PhaseCruise)
	assert.Equal(t, 0, cruise[0].cruiseAt())
	assert.Equal(t, 50, cruise[1].cruiseAt(), "a numeric cruise_at is read")
	assert.Empty(t, p.playable(domain.PhasePushback))
}

func TestParseSpeechPackAcceptsAFlatReply(t *testing.T) {
	flat := strings.TrimSuffix(strings.TrimPrefix(testPack, `{"data":`), "}")
	p, err := parseSpeechPack([]byte(flat))
	require.NoError(t, err)
	assert.Equal(t, "r1", p.Revision)
}

func TestParseSpeechPackRejectsGarbage(t *testing.T) {
	for _, body := range []string{"<html>502</html>", `{"data":{"phases":[]}}`, `{"data":"nope"}`} {
		_, err := parseSpeechPack([]byte(body))
		assert.Error(t, err, body)
	}
}

// An old server and a pilot without a booking both answer 404; only the
// second says so. Every other failure is an error to retry, never a mode.
func TestFetchSpeechPackTellsTheAnswersApart(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"no booking", 404, `{"message":"No active booking"}`, errNoBooking},
		{"route missing", 404, `{"message":"The route api/v2/acars/speech/pack could not be found."}`, errSpeechUnsupported},
		{"route missing, html", 404, `<html>Not Found</html>`, errSpeechUnsupported},
		{"method not allowed", 405, `{}`, errSpeechUnsupported},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := fetchSpeechPack(&stubAPI{status: c.status, body: []byte(c.body)}, "")
			assert.ErrorIs(t, err, c.want)
		})
	}

	_, err := fetchSpeechPack(&stubAPI{status: 503, body: []byte("error code: 503")}, "")
	var se *domain.StatusError
	require.True(t, errors.As(err, &se))
	assert.Equal(t, 503, se.Status)

	p, err := fetchSpeechPack(&stubAPI{status: 200, body: []byte(testPack)}, "")
	require.NoError(t, err)
	assert.Equal(t, "r1", p.Revision)
}

func TestFetchSpeechLineMapsTheRefusals(t *testing.T) {
	_, err := fetchSpeechLine(&stubAPI{status: 422, body: []byte(`{"message":"No voice","reason":"no_voice","data":{}}`)}, 5, lineQuery{})
	assert.ErrorIs(t, err, errLineUnvoiced)
	_, err = fetchSpeechLine(&stubAPI{status: 404, body: []byte(`{"message":"This line is not part of the active booking's speech pack"}`)}, 5, lineQuery{})
	assert.ErrorIs(t, err, errLineGone)

	l, err := fetchSpeechLine(&stubAPI{status: 200, body: []byte(`{"data":{"id":5,"text":"Now","audio_url":"https://t.example/a/5.mp3"}}`)}, 5, lineQuery{})
	require.NoError(t, err)
	assert.Equal(t, "Now", l.Text)
	assert.True(t, l.hasAudio())
}

// The server refuses a line with 422 for two different things: audio it could
// not produce yet (the TTS provider timed out, or is still working), which is
// worth asking for again, and a line that can never play (no voice, empty,
// disabled). The reason says which; a refusal without one is final, as
// before the server sent it.
func TestFetchSpeechLineTellsAWaitFromARefusal(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		notReady bool
		inError  string
	}{
		{"audio not generated yet", `{"message":"This line cannot be played","reason":"not_generated","data":{"id":5}}`, true, ""},
		{"no voice for the language", `{"message":"This line cannot be played","reason":"no_voice","data":{"id":5}}`, false, "no_voice"},
		{"an older server without a reason", `{"message":"This line cannot be played"}`, false, ""},
		{"garbage", `<html>422</html>`, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := fetchSpeechLine(&stubAPI{status: 422, body: []byte(c.body)}, 5, lineQuery{})
			require.Error(t, err)
			assert.Equal(t, c.notReady, errors.Is(err, errLineNotReady), "is %q worth asking again?", err)
			assert.Equal(t, !c.notReady, errors.Is(err, errLineUnvoiced), "is %q final?", err)
			assert.Contains(t, err.Error(), c.inError, "the reason is kept for the log")
		})
	}
}

func TestLineQueryEncodesOnlyWhatIsSet(t *testing.T) {
	assert.Equal(t, "", lineQuery{}.encode())
	assert.Equal(t, "?arrival=SBKP&landing=smooth&service=2",
		lineQuery{arrival: "SBKP", service: 2, landing: "smooth"}.encode())
}

// A 202 for the pack means the server is still voicing it: the ACARS asks
// again after the pause the server names, and shows how far it has got.
func TestAPackBeingPreparedSaysWhenToAskAgain(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wait       time.Duration
		ready, all int
	}{
		{"the server names the pause",
			`{"message":"The speech pack is being prepared","data":{"status":"preparing","lines_ready":12,"lines_total":28,"retry_after":10}}`,
			10 * time.Second, 12, 28},
		{"the pause as a string", `{"data":{"status":"preparing","lines_ready":"3","lines_total":"9","retry_after":"15"}}`,
			15 * time.Second, 3, 9},
		{"no pause given", `{"data":{"status":"preparing","lines_ready":0,"lines_total":5}}`, packPreparingDefault, 0, 5},
		{"a body that is not JSON", `accepted`, packPreparingDefault, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := newSpeechAPI(map[string]reply{"GET " + speechPackPath: {202, c.body}})
			pack, err := fetchSpeechPack(api, "")
			assert.Nil(t, pack)
			var p *packPreparing
			require.True(t, errors.As(err, &p), "a 202 is read as a failure (%v); the pilot sees an error while the pack is voiced", err)
			assert.Equal(t, c.wait, p.retryAfter)
			assert.Equal(t, c.ready, p.linesReady)
			assert.Equal(t, c.all, p.linesTotal)
		})
	}
}

// The pause the server asks for is bounded, so a bad number neither spins
// the worker nor leaves the cabin silent past the server's own two minutes.
func TestThePreparingPauseIsBounded(t *testing.T) {
	cases := []struct {
		asked, want time.Duration
	}{
		{0, packPreparingDefault},
		{-5 * time.Second, packPreparingDefault},
		{time.Millisecond, packPreparingMin},
		{packPreparingMin, packPreparingMin},
		{10 * time.Second, 10 * time.Second},
		{packPreparingMax, packPreparingMax},
		{time.Hour, packPreparingMax},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, clampPreparing(c.asked), "asked to wait %v", c.asked)
	}
}
