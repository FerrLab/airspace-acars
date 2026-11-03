package domain

import (
	"math"
	"time"
)

// Speech trigger phases, as Airspace names them (SpeechTriggerPhase).
// PhaseBoarding keeps the id gate_departure for compatibility; the editor
// labels it Boarding.
const (
	PhaseBoarding         = "gate_departure"
	PhaseBoardingComplete = "boarding_complete"
	PhasePushback         = "pushback"
	PhaseTaxi             = "taxi"
	PhaseClimbOut         = "climb_out"
	PhasePassing10000     = "passing_10000"
	PhaseCruise           = "cruise"
	PhaseMealService      = "meal_service"
	PhaseSeatbeltOn       = "seatbelt_on"
	PhaseSeatbeltOff      = "seatbelt_off"
	PhaseDescend          = "descend"
	PhaseDescending10000  = "descending_10000"
	PhaseFinalApproach    = "final_approach"
	PhaseAfterLanding     = "after_landing"
	PhaseGateArrival      = "gate_arrival"
	PhaseAlternate        = "alternate_declared"
)

// Flight attendant modes.
const (
	// AttendantIdle: no flight is under way.
	AttendantIdle = "idle"
	// AttendantLoading: the flight has started and the speech pack is being
	// read, or retried after a failure.
	AttendantLoading = "loading"
	// AttendantClient: the ACARS detects phases and plays the pack itself.
	AttendantClient = "client"
	// AttendantLegacy: the airline's server predates client playback, so the
	// webview polls GET /sound as before.
	AttendantLegacy = "legacy"
	// AttendantNoBooking: the flight has no booking, so there is no pack.
	AttendantNoBooking = "no_booking"
)

// Step statuses for the phase map.
const (
	StepPending = "pending"
	StepFired   = "fired"
	StepSkipped = "skipped"
)

// What became of a clip, for the history.
const (
	ClipPlayed      = "played"
	ClipFailed      = "failed"
	ClipSkipped     = "skipped"
	ClipStopped     = "stopped"
	ClipInterrupted = "interrupted"
	ClipMuted       = "muted" // the phase fired while the attendant was off
)

// Reason codes say why a clip plays. The webview words them, filling in the
// numbers of AttendantReason.
const (
	ReasonFlightStarted     = "flight_started"
	ReasonGSXBoarding       = "gsx_boarding_started"
	ReasonGSXBoardingDone   = "gsx_boarding_complete"
	ReasonBeaconOn          = "beacon_on"
	ReasonBoardingRepeat    = "boarding_repeat"
	ReasonBoardingMusic     = "boarding_music"
	ReasonGSXPushback       = "gsx_pushback"
	ReasonMovingEnginesOff  = "moving_engines_off"
	ReasonLeftStand         = "left_stand"
	ReasonEngineStart       = "engine_start"
	ReasonLeavingGate       = "leaving_gate"
	ReasonTaxi              = "taxi"
	ReasonClimbOut          = "climb_out"
	ReasonPassing10000      = "passing_10000"
	ReasonCruise            = "cruise"
	ReasonCruiseAt          = "cruise_at"
	ReasonMealAfterCruise   = "meal_after_cruise"
	ReasonMealBeforeDescent = "meal_before_descent"
	ReasonSeatbeltOn        = "seatbelt_on"
	ReasonSeatbeltOff       = "seatbelt_off"
	ReasonDescend           = "descend"
	ReasonDescending10000   = "descending_10000"
	ReasonFinalApproach     = "final_approach"
	ReasonTouchdown         = "touchdown"
	ReasonParked            = "parked"
	ReasonGSXDeboarding     = "gsx_deboarding"
	ReasonDiversion         = "diversion"
	ReasonManual            = "manual"
	ReasonReplay            = "replay"
)

// AttendantReason is why something played or was skipped: a code and the
// numbers that go with it. Only the fields the code uses are set.
type AttendantReason struct {
	Code     string  `json:"code"`
	Altitude int     `json:"altitude,omitempty"` // ft, MSL or AGL as the code says
	VS       int     `json:"vs,omitempty"`       // fpm
	GS       int     `json:"gs,omitempty"`       // kt
	G        float64 `json:"g,omitempty"`
	Seconds  int     `json:"seconds,omitempty"`
	Minutes  int     `json:"minutes,omitempty"`
	Percent  int     `json:"percent,omitempty"`
	Service  int     `json:"service,omitempty"`
	Landing  string  `json:"landing,omitempty"` // smooth | normal | hard
	Airport  string  `json:"airport,omitempty"`
}

// AttendantStep is one marker on the phase map: a phase, or one meal service.
type AttendantStep struct {
	// Key is the phase id, or "meal_service:N" for meal service N.
	Key     string          `json:"key"`
	Phase   string          `json:"phase"`
	Service int             `json:"service,omitempty"`
	Status  string          `json:"status"` // one of the Step* values
	Lines   int             `json:"lines"`  // playable lines in the pack
	FiredAt time.Time       `json:"firedAt"`
	Count   int             `json:"count"` // times fired; seatbelt phases repeat
	Reason  AttendantReason `json:"reason"`
}

// AttendantClip is one line handed to the webview to play.
type AttendantClip struct {
	ID       string          `json:"id"` // unique per hand-over
	LineID   int             `json:"lineId"`
	Phase    string          `json:"phase"`
	Service  int             `json:"service,omitempty"`
	Text     string          `json:"text"`
	Language string          `json:"language"`
	Voice    string          `json:"voice"`
	Music    bool            `json:"music"` // boarding music, looped between announcements
	Reason   AttendantReason `json:"reason"`
	// Segments are the cached files and pauses that make up the line, in
	// order: one file when the server combined the line, several otherwise.
	Segments []SoundInstruction `json:"segments"`
}

// AttendantHistoryEntry is a clip that has finished, one way or another.
type AttendantHistoryEntry struct {
	At       time.Time       `json:"at"`
	Phase    string          `json:"phase"`
	Service  int             `json:"service,omitempty"`
	Text     string          `json:"text"`
	Language string          `json:"language"`
	Voice    string          `json:"voice"`
	Music    bool            `json:"music"`
	Outcome  string          `json:"outcome"` // one of the Clip* values
	Reason   AttendantReason `json:"reason"`
}

// SeatbeltCounts is how often each seatbelt announcement fired this flight.
type SeatbeltCounts struct {
	On  int `json:"on"`
	Off int `json:"off"`
}

// AttendantSnapshot is everything the Flight Attendant tab shows.
type AttendantSnapshot struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // one of the Attendant* modes
	// Current is the key of the last phase fired, where the aircraft icon
	// sits on the phase map; "" before the first.
	Current    string                  `json:"current"`
	Arrival    string                  `json:"arrival"`
	Steps      []AttendantStep         `json:"steps"`
	NowPlaying *AttendantClip          `json:"nowPlaying"`
	Queue      []AttendantClip         `json:"queue"`
	History    []AttendantHistoryEntry `json:"history"`
	GSX        GSXState                `json:"gsx"`
	Seatbelt   CabinData               `json:"seatbelt"`
	Seatbelts  SeatbeltCounts          `json:"seatbelts"`
	// Error is the last reason the pack could not be read, "" when it was.
	Error string `json:"error"`
}

// LandingThresholds classify a touchdown. A landing is smooth when both its
// rate and its load are at or under the smooth limits, hard when either
// reaches a hard limit, and normal otherwise.
type LandingThresholds struct {
	SmoothMaxFPM float64 `json:"smooth_max_fpm"`
	SmoothMaxG   float64 `json:"smooth_max_g"`
	HardMinFPM   float64 `json:"hard_min_fpm"`
	HardMinG     float64 `json:"hard_min_g"`
}

// DefaultLandingThresholds are Airspace's defaults, used for any limit the
// pack leaves out.
var DefaultLandingThresholds = LandingThresholds{SmoothMaxFPM: 200, SmoothMaxG: 1.2, HardMinFPM: 400, HardMinG: 1.5}

// Landing classes.
const (
	LandingSmooth = "smooth"
	LandingNormal = "normal"
	LandingHard   = "hard"
)

// ClassifyLanding sorts a touchdown at fpm (sign ignored) and g into smooth,
// normal or hard. Hard is checked first, so limits that overlap err towards
// the passengers having noticed.
func ClassifyLanding(fpm, g float64, t LandingThresholds) string {
	d := DefaultLandingThresholds
	if t.SmoothMaxFPM <= 0 {
		t.SmoothMaxFPM = d.SmoothMaxFPM
	}
	if t.SmoothMaxG <= 0 {
		t.SmoothMaxG = d.SmoothMaxG
	}
	if t.HardMinFPM <= 0 {
		t.HardMinFPM = d.HardMinFPM
	}
	if t.HardMinG <= 0 {
		t.HardMinG = d.HardMinG
	}
	rate := math.Abs(fpm)
	switch {
	case rate >= t.HardMinFPM || g >= t.HardMinG:
		return LandingHard
	case rate <= t.SmoothMaxFPM && g <= t.SmoothMaxG:
		return LandingSmooth
	default:
		return LandingNormal
	}
}
