package domain

// AirportInfo contains geographic and identification data for an airport.
type AirportInfo struct {
	ICAO      string  `json:"icao"`
	IATA      string  `json:"iata,omitempty"`
	Name      string  `json:"name,omitempty"`
	City      string  `json:"city,omitempty"`
	Country   string  `json:"country,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// AircraftInfo contains aircraft registration and type identification.
type AircraftInfo struct {
	Registration string `json:"registration"`
	Name         string `json:"name,omitempty"`
	ICAOCode     string `json:"icao_code,omitempty"`
}

// FlightLog represents a single pilot flight record (PIREP).
type FlightLog struct {
	ID                string        `json:"id"`
	Callsign          string        `json:"callsign"`
	FlightNumber      string        `json:"flight_number,omitempty"`
	Status            string        `json:"status"` // accepted, pending, rejected, etc.
	DepartureAirport  AirportInfo   `json:"departure_airport"`
	ArrivalAirport    AirportInfo   `json:"arrival_airport"`
	Aircraft          *AircraftInfo `json:"aircraft,omitempty"`
	FlightTimeMinutes int           `json:"flight_time_minutes"`
	DistanceNM        float64       `json:"distance_nm"`
	LandingRateFPM    float64       `json:"landing_rate_fpm"`
	FuelUsedKG        float64       `json:"fuel_used_kg"`
	Score             int           `json:"score"`
	DepartureTime     string        `json:"departure_time,omitempty"`
	ArrivalTime       string        `json:"arrival_time,omitempty"`
	CreatedAt         string        `json:"created_at,omitempty"`
}

// PilotSummaryStats holds aggregated statistics for the pilot.
type PilotSummaryStats struct {
	PilotID         string  `json:"pilot_id"`
	Name            string  `json:"name"`
	Callsign        string  `json:"callsign"`
	Rank            string  `json:"rank"`
	RankImageURL    string  `json:"rank_image_url,omitempty"`
	TotalFlights    int     `json:"total_flights"`
	TotalHours      float64 `json:"total_hours"`
	AvgLandingRate  float64 `json:"avg_landing_rate"`
	TotalDistanceNM      float64 `json:"total_distance_nm"`
	Points               int     `json:"points"`
	HasGlobalDistance    bool    `json:"has_global_distance,omitempty"`
	HasGlobalHours       bool    `json:"has_global_hours,omitempty"`
	HasGlobalLandingRate bool    `json:"has_global_landing_rate,omitempty"`
}

// MyFlightsResponse wraps pilot flight history and summary statistics.
type MyFlightsResponse struct {
	Status      string            `json:"status"` // "ok", "localMode", "noSession", "accessDenied", "unavailable", "rateLimited"
	Pilot       PilotSummaryStats `json:"pilot"`
	Flights     []FlightLog       `json:"flights"`
	CurrentPage int               `json:"current_page"`
	LastPage    int               `json:"last_page"`
	Total       int               `json:"total"`
}
