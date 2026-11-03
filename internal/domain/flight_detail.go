package domain

// FlightDetailResponse is one of the pilot's flights with its analysis, as
// the My Flights detail view shows it. Status uses the same vocabulary as the
// other pilot resources ("ok", "accessDenied", "unavailable", ...); Data is
// set only when Status is "ok".
type FlightDetailResponse struct {
	Status string        `json:"status"`
	Data   *FlightDetail `json:"data"`
}

// FlightDetail is the part of a flight's record the logbook list does not
// carry. Every section is recorded by the platform; a nil section had
// nothing recorded, so the view hides it rather than showing zeros.
type FlightDetail struct {
	ID              string         `json:"id"`
	Route           string         `json:"route,omitempty"`
	DepartureRunway string         `json:"departure_runway,omitempty"`
	ArrivalRunway   string         `json:"arrival_runway,omitempty"`
	ApproachType    string         `json:"approach_type,omitempty"`
	LandingGForce   *float64       `json:"landing_g_force"`
	Landing         *LandingReport `json:"landing"`
	Takeoff         *TakeoffReport `json:"takeoff"`
	ScoreBreakdown  ScoreBreakdown `json:"score_breakdown"`
	Events          []FlightEvent  `json:"events"`
	Points          FlightPoints   `json:"points"`
}

// LandingReport is the touchdown analysis. Distances are metres, speeds
// knots; offsets are metres right (+) or left (-) of the centreline.
type LandingReport struct {
	LandingRateBand            string                  `json:"landing_rate_band,omitempty"`
	GForceBand                 string                  `json:"g_force_band,omitempty"`
	TouchdownDistanceM         *float64                `json:"touchdown_distance_m"`
	TouchdownOffsetM           *float64                `json:"touchdown_offset_m"`
	TouchdownGroundSpeed       *float64                `json:"touchdown_ground_speed"`
	TouchdownIndicatedAirspeed *float64                `json:"touchdown_indicated_airspeed"`
	RunwayUsedM                *float64                `json:"runway_used_m"`
	RunwayUsedPercent          *float64                `json:"runway_used_percent"`
	Runway                     *RunwayInfo             `json:"runway"`
	Wind                       *RunwayWind             `json:"wind"`
	StabilizedApproach         *StabilizedApproachGate `json:"stabilized_approach"`
}

// TakeoffReport is the liftoff analysis, in the units of LandingReport.
type TakeoffReport struct {
	LiftoffDistanceM         *float64    `json:"liftoff_distance_m"`
	TakeoffRollM             *float64    `json:"takeoff_roll_m"`
	LiftoffOffsetM           *float64    `json:"liftoff_offset_m"`
	LiftoffGroundSpeed       *float64    `json:"liftoff_ground_speed"`
	LiftoffIndicatedAirspeed *float64    `json:"liftoff_indicated_airspeed"`
	RunwayUsedPercent        *float64    `json:"runway_used_percent"`
	Runway                   *RunwayInfo `json:"runway"`
	Wind                     *RunwayWind `json:"wind"`
}

// RunwayInfo is the runway from the airport database. LengthM is the
// declared landing or takeoff length, else the full length.
type RunwayInfo struct {
	Designator      string   `json:"designator"`
	MagneticHeading *float64 `json:"magnetic_heading"`
	LengthM         *float64 `json:"length_m"`
	WidthM          *float64 `json:"width_m"`
}

// RunwayWind is the wind on the touchdown or liftoff sample against the
// runway. HeadwindKnots is negative for a tailwind; CrosswindKnots is
// positive from the right.
type RunwayWind struct {
	DirectionDegrees float64 `json:"direction_degrees"`
	SpeedKnots       float64 `json:"speed_knots"`
	HeadwindKnots    float64 `json:"headwind_knots"`
	CrosswindKnots   float64 `json:"crosswind_knots"`
}

// StabilizedApproachGate is the verdict of the airline's stabilized-approach
// rule. Criteria is nil on flights evaluated before the platform kept the
// per-criterion snapshot; Failures are the violation texts it stored.
type StabilizedApproachGate struct {
	GateHeightFt float64             `json:"gate_height_ft"`
	Passed       bool                `json:"passed"`
	Criteria     *StabilizedCriteria `json:"criteria"`
	Failures     []string            `json:"failures"`
}

// StabilizedCriteria is the aircraft's state at the gate.
type StabilizedCriteria struct {
	Flaps GateFlaps `json:"flaps"`
	Gear  GateGear  `json:"gear"`
	Speed GateSpeed `json:"speed"`
}

// GateFlaps is the flap setting at the gate against the rule's minimum.
type GateFlaps struct {
	Passed      bool     `json:"passed"`
	MeasuredPct *float64 `json:"measured_pct"`
	RequiredPct float64  `json:"required_pct"`
}

// GateGear is the landing gear state at the gate.
type GateGear struct {
	Passed bool `json:"passed"`
	Down   bool `json:"down"`
}

// GateSpeed is the airspeed at the gate against the rule's limit (the
// landing speed plus MarginKt).
type GateSpeed struct {
	Passed     bool    `json:"passed"`
	MeasuredKt float64 `json:"measured_kt"`
	LimitKt    float64 `json:"limit_kt"`
	MarginKt   float64 `json:"margin_kt"`
}

// ScoreBreakdown is the platform's live per-category flight score, on its
// 0–10 scale. Score is nil until a category can be scored.
type ScoreBreakdown struct {
	Score        *float64        `json:"score"`
	Contributing []string        `json:"contributing"`
	Categories   []ScoreCategory `json:"categories"`
}

// ScoreCategory is one scored category (otp, eco, manual, comfort, safety).
// Formula and the input labels arrive translated into the pilot's language.
type ScoreCategory struct {
	Key               string       `json:"key"`
	Score             *float64     `json:"score"`
	Weight            float64      `json:"weight"`
	Available         bool         `json:"available"`
	UnavailableReason string       `json:"unavailable_reason,omitempty"`
	Formula           string       `json:"formula,omitempty"`
	Inputs            []ScoreInput `json:"inputs"`
}

// ScoreInput is one value a category score was computed from.
type ScoreInput struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// FlightEvent is an FDM event recorded during the flight. Type is
// "information", "warning" or "violation".
type FlightEvent struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Event           string   `json:"event"`
	HappenedAt      string   `json:"happened_at,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds"`
}

// FlightPoints is the points awarded for the flight.
type FlightPoints struct {
	Total   float64      `json:"total"`
	Entries []PointEntry `json:"entries"`
}

// PointEntry is one award. Source is "fdm" (from an event) or "staff".
type PointEntry struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Reason    string  `json:"reason"`
	Points    float64 `json:"points"`
	Remarks   string  `json:"remarks,omitempty"`
	CreatedAt string  `json:"created_at,omitempty"`
}

// FlightTrackResponse is the flight's recorded position track. Status uses
// the FlightDetailResponse vocabulary; Points is empty unless it is "ok".
type FlightTrackResponse struct {
	Status string       `json:"status"`
	Points []TrackPoint `json:"points"`
}

// TrackPoint is one recorded position. Lng is unwrapped from the departure
// airport, so a track across the antimeridian is continuous and may pass
// ±180. Altitudes are feet, speeds knots, VerticalSpeed feet per minute.
type TrackPoint struct {
	SentAt            string   `json:"sent_at"`
	Phase             string   `json:"phase,omitempty"`
	Lat               float64  `json:"lat"`
	Lng               float64  `json:"lng"`
	Altitude          float64  `json:"altitude"`
	AltitudeAGL       *float64 `json:"altitude_agl"`
	IndicatedAirspeed float64  `json:"indicated_airspeed"`
	GroundSpeed       float64  `json:"ground_speed"`
	VerticalSpeed     float64  `json:"vertical_speed"`
	Heading           *float64 `json:"heading"`
}
