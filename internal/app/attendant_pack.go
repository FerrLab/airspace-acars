package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"airspace-acars/internal/domain"
)

// The speech pack routes. The client builds every path itself rather than
// following the fetch_url the server puts in each line: the pilot token goes
// only to /api/v2/acars/ on the tenant (AGENTS.md section 7), never to a URL
// a reply named.
const (
	speechPackPath   = "/api/v2/acars/speech/pack"
	speechLinePath   = "/api/v2/acars/speech/lines/"
	speechPhasePath  = "/api/v2/acars/speech/phases/"
	noBookingMessage = "no active booking"
)

// errSpeechUnsupported means the airline's server predates client-side
// playback: GET /speech/pack is not a route there.
var errSpeechUnsupported = errors.New("server does not serve speech packs")

// errLineGone means a line id is no longer in the booking's resolved pack,
// which happens when the pack changed under the flight.
var errLineGone = errors.New("line is not part of the active speech pack")

// errLineUnvoiced means the server cannot voice a line (no voice for its
// language, or empty text).
var errLineUnvoiced = errors.New("line cannot be voiced")

// speechPack is GET /speech/pack, decoded.
type speechPack struct {
	Revision string       `json:"revision"`
	Flight   packFlight   `json:"flight"`
	Settings packSettings `json:"settings"`
	Phases   []packPhase  `json:"phases"`
	lines    map[string][]speechLine
}

type packFlight struct {
	Arrival             string        `json:"arrival"`
	IsAlternate         bool          `json:"is_alternate"`
	PlannedBlockMinutes flexFloat     `json:"planned_block_minutes"`
	MealServices        []mealService `json:"meal_services"`
}

type mealService struct {
	N       int       `json:"n"`
	Meal    string    `json:"meal"`
	Timing  string    `json:"timing"` // after_cruise | before_descent
	Minutes flexFloat `json:"minutes"`
}

type packSettings struct {
	Seatbelt struct {
		OnCooldownMinutes  flexFloat `json:"on_cooldown_minutes"`
		OffCooldownMinutes flexFloat `json:"off_cooldown_minutes"`
	} `json:"seatbelt"`
	Landing domain.LandingThresholds `json:"landing"`
}

type packPhase struct {
	Phase string       `json:"phase"`
	Lines []speechLine `json:"lines"`
}

// speechLine is one LINE of the contract.
type speechLine struct {
	ID         flexFloat      `json:"id"`
	Phase      string         `json:"phase"`
	SortOrder  flexFloat      `json:"sort_order"`
	Type       string         `json:"type"` // text | audio
	Language   string         `json:"language"`
	Voice      string         `json:"voice"`
	Conditions lineConditions `json:"conditions"`
	Service    flexFloat      `json:"service"`
	Text       string         `json:"text"`
	Playable   bool           `json:"playable"`
	Dynamic    bool           `json:"dynamic"`
	AudioURL   string         `json:"audio_url"`
	Segments   []lineSegment  `json:"segments"`
}

type lineConditions struct {
	Landing            string     `json:"landing"`
	CruiseAt           flexString `json:"cruise_at"`
	Repeat             bool       `json:"repeat"`
	RepeatEveryMinutes flexFloat  `json:"repeat_every_minutes"`
}

// lineSegment is one play or pause step, the shape of the legacy /sound
// instructions.
type lineSegment struct {
	Type       string    `json:"type"`
	URL        string    `json:"url"`
	DurationMs flexFloat `json:"duration_ms"`
}

// flexFloat decodes a number, a numeric string or null; anything else is 0.
type flexFloat float64

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexFloat(n)
	return nil
}

// flexString decodes a string or a number as text; null is "".
type flexString string

// UnmarshalJSON implements json.Unmarshaler.
func (s *flexString) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "null" {
		*s = ""
		return nil
	}
	var str string
	if json.Unmarshal(b, &str) == nil {
		*s = flexString(str)
		return nil
	}
	*s = flexString(raw)
	return nil
}

func (l speechLine) id() int      { return int(l.ID) }
func (l speechLine) service() int { return int(l.Service) }
func (l speechLine) isMusic() bool {
	return l.Type == "audio" && l.Conditions.Repeat
}

// hasAudio reports whether the line can be played as it is, without asking
// the server to render it.
func (l speechLine) hasAudio() bool {
	if l.AudioURL != "" {
		return true
	}
	for _, s := range l.Segments {
		if s.Type == "play" && s.URL != "" {
			return true
		}
	}
	return false
}

// cruiseAt is the percentage of the planned block time a cruise line plays
// at, or 0 for the line that plays on reaching cruise.
func (l speechLine) cruiseAt() int {
	n, err := strconv.Atoi(strings.TrimSpace(string(l.Conditions.CruiseAt)))
	if err != nil {
		return 0
	}
	return n
}

// parseSpeechPack decodes a pack, data-wrapped or flat, and indexes its
// lines by phase in play order.
func parseSpeechPack(body []byte) (*speechPack, error) {
	raw, err := unwrapData(body)
	if err != nil {
		return nil, fmt.Errorf("parse speech pack: %w", err)
	}
	var p speechPack
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse speech pack: %w", err)
	}
	if len(p.Phases) == 0 {
		return nil, errors.New("parse speech pack: no phases")
	}
	p.lines = map[string][]speechLine{}
	for _, ph := range p.Phases {
		lines := append([]speechLine(nil), ph.Lines...)
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].SortOrder < lines[j].SortOrder })
		for i := range lines {
			if lines[i].Phase == "" {
				lines[i].Phase = ph.Phase
			}
		}
		p.lines[ph.Phase] = append(p.lines[ph.Phase], lines...)
	}
	return &p, nil
}

// playable returns the phase's lines that can play, in order.
func (p *speechPack) playable(phase string) []speechLine {
	if p == nil {
		return nil
	}
	var out []speechLine
	for _, l := range p.lines[phase] {
		if l.Playable {
			out = append(out, l)
		}
	}
	return out
}

// messageOf returns the "message" of a JSON error body, lowercased.
func messageOf(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e) // a body that is not JSON has no message
	return strings.ToLower(strings.TrimSpace(e.Message))
}

// fetchSpeechPack reads the booking's pack, at another arrival airport when
// arrival is set. It tells an old server (errSpeechUnsupported) from a pilot
// without a booking (errNoBooking): both answer 404, but only the second says
// so in its message.
func fetchSpeechPack(api AirspaceAPI, arrival string) (*speechPack, error) {
	path := speechPackPath
	if arrival != "" {
		path += "?" + url.Values{"arrival": {arrival}}.Encode()
	}
	body, status, err := api.DoRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusNotFound && strings.Contains(messageOf(body), noBookingMessage):
		return nil, errNoBooking
	case status == http.StatusNotFound || status == http.StatusMethodNotAllowed:
		return nil, errSpeechUnsupported
	}
	if err := domain.NewStatusError(http.MethodGet, speechPackPath, status, body); err != nil {
		return nil, err
	}
	return parseSpeechPack(body)
}

// lineQuery is what the server needs to render a line for the situation.
type lineQuery struct {
	arrival  string
	service  int
	landing  string
	cruiseAt int
}

func (q lineQuery) encode() string {
	v := url.Values{}
	if q.arrival != "" {
		v.Set("arrival", q.arrival)
	}
	if q.service > 0 {
		v.Set("service", strconv.Itoa(q.service))
	}
	if q.landing != "" {
		v.Set("landing", q.landing)
	}
	if q.cruiseAt > 0 {
		v.Set("cruise_at", strconv.Itoa(q.cruiseAt))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// fetchSpeechLine renders one line now, with its audio generated.
func fetchSpeechLine(api AirspaceAPI, id int, q lineQuery) (speechLine, error) {
	path := speechLinePath + strconv.Itoa(id)
	body, status, err := api.DoRequest(http.MethodGet, path+lineQuery{arrival: q.arrival, service: q.service}.encode(), nil)
	if err != nil {
		return speechLine{}, err
	}
	switch status {
	case http.StatusNotFound:
		if strings.Contains(messageOf(body), noBookingMessage) {
			return speechLine{}, errNoBooking
		}
		return speechLine{}, errLineGone
	case http.StatusUnprocessableEntity:
		return speechLine{}, errLineUnvoiced
	}
	if err := domain.NewStatusError(http.MethodGet, path, status, body); err != nil {
		return speechLine{}, err
	}
	raw, err := unwrapData(body)
	if err != nil {
		return speechLine{}, fmt.Errorf("parse speech line: %w", err)
	}
	var l speechLine
	if err := json.Unmarshal(raw, &l); err != nil {
		return speechLine{}, fmt.Errorf("parse speech line: %w", err)
	}
	return l, nil
}

// fetchSpeechPhase reads the playable lines of one phase for the situation,
// each with its audio: how after_landing gets only the lines for the
// touchdown that happened.
func fetchSpeechPhase(api AirspaceAPI, phase string, q lineQuery) ([]speechLine, error) {
	path := speechPhasePath + url.PathEscape(phase)
	body, status, err := api.DoRequest(http.MethodGet, path+q.encode(), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound && strings.Contains(messageOf(body), noBookingMessage) {
		return nil, errNoBooking
	}
	if err := domain.NewStatusError(http.MethodGet, path, status, body); err != nil {
		return nil, err
	}
	raw, err := unwrapData(body)
	if err != nil {
		return nil, fmt.Errorf("parse speech phase: %w", err)
	}
	var reply struct {
		Lines []speechLine `json:"lines"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("parse speech phase: %w", err)
	}
	sort.SliceStable(reply.Lines, func(i, j int) bool { return reply.Lines[i].SortOrder < reply.Lines[j].SortOrder })
	var out []speechLine
	for _, l := range reply.Lines {
		if l.Playable || l.hasAudio() {
			if l.Phase == "" {
				l.Phase = phase
			}
			out = append(out, l)
		}
	}
	return out, nil
}
