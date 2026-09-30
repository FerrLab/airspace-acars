package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"

	"airspace-acars/internal/domain"
)

// Known airport coordinates dictionary for instant coordinate resolution.
var knownAirportCoords = map[string][2]float64{
	// Brazil
	"SBGR": {-23.435556, -46.473056},
	"SBRJ": {-22.910278, -43.163056},
	"SBSP": {-23.626111, -46.656389},
	"SBKP": {-23.0075, -47.134444},
	"SBBR": {-15.869167, -47.920833},
	"SBGL": {-22.809999, -43.250556},
	"SBCF": {-19.624444, -43.971944},
	"SBPA": {-29.993889, -51.171111},
	"SBFZ": {-3.776111, -38.5325},
	"SBSV": {-12.908611, -38.3225},
	"SBRF": {-8.126389, -34.923056},
	"SBCT": {-25.528333, -49.175833},
	"SBEG": {-3.035833, -60.049722},
	"SBGO": {-16.632222, -49.220556},
	"SBNF": {-26.879444, -48.651389},
	"SBFL": {-27.670278, -48.5525},
	"SBMQ": {0.050833, -51.072222},
	"SBBH": {-19.851944, -43.950556},
	"SBVT": {-20.258056, -40.286389},
	"SBPS": {-16.438889, -39.080833},
	"SBCY": {-15.652778, -56.116667},
	"SBCG": {-20.468611, -54.6725},
	"SBEI": {-23.080278, -47.134444},
	"SBMO": {-9.510833, -35.791667},
	"SBNT": {-5.911389, -35.247778},
	"SBJP": {-7.148333, -34.950278},
	"SBTE": {-5.059722, -42.823611},
	"SBSL": {-2.585556, -44.234167},
	"SBCX": {-29.196944, -51.187222},
	"SBMG": {-23.479444, -52.012222},
	"SBCA": {-24.953889, -53.500833},
	"SBLN": {-12.483611, -55.686111},
	"SBMA": {-22.345, -49.068333},
	// International
	"SAEZ": {-34.822222, -58.535833},
	"SABE": {-34.559167, -58.415556},
	"SCEL": {-33.393001, -70.785797},
	"SPJC": {-12.021944, -77.114444},
	"SKBO": {4.701667, -74.146944},
	"SEQM": {-0.129167, -78.3575},
	"SUMU": {-34.838333, -56.030833},
	"SGAS": {-25.239722, -57.518889},
	"EGLL": {51.4706, -0.461941},
	"EGKK": {51.148056, -0.190278},
	"LFPG": {49.009722, 2.547778},
	"EHAM": {52.308613, 4.763889},
	"EDDF": {50.033333, 8.570556},
	"LEMD": {40.471926, -3.56264},
	"LPPT": {38.774167, -9.134167},
	"KJFK": {40.639751, -73.778925},
	"KMIA": {25.79325, -80.290556},
	"KLAX": {33.9425, -118.408056},
	"KORD": {41.974162, -87.907321},
	"KATL": {33.6407, -84.4277},
}

// resolveCoordinates checks airport coords and falls back to dictionary.
func resolveCoordinates(icao string, lat, lon float64) (float64, float64) {
	if lat != 0 || lon != 0 {
		return lat, lon
	}
	if coords, ok := knownAirportCoords[strings.ToUpper(strings.TrimSpace(icao))]; ok {
		return coords[0], coords[1]
	}
	return 0, 0
}

// calculateGreatCircleDistanceNM computes distance in nautical miles between coordinates.
func calculateGreatCircleDistanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	if (lat1 == 0 && lon1 == 0) || (lat2 == 0 && lon2 == 0) {
		return 0
	}
	rad := math.Pi / 180.0
	phi1 := lat1 * rad
	phi2 := lat2 * rad
	deltaPhi := (lat2 - lat1) * rad
	deltaLambda := (lon2 - lon1) * rad

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	const earthRadiusNM = 3440.065
	return math.Round(earthRadiusNM * c)
}

type flightWireAirport struct {
	ID        json.RawMessage `json:"id"`
	ICAO      string          `json:"icao"`
	IATA      string          `json:"iata"`
	Name      string          `json:"name"`
	City      string          `json:"city"`
	Country   string          `json:"country"`
	Latitude  interface{}     `json:"latitude"`
	Longitude interface{}     `json:"longitude"`
	Lat       interface{}     `json:"lat"`
	Lon       interface{}     `json:"lon"`
}

func parseFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0
		}
		return finiteOrZero(f)
	default:
		return 0
	}
}

// finiteOrZero drops NaN and ±Inf. A tenant can put "NaN" or "1e999" in a
// numeric field; encoding/json refuses to marshal those, which would make the
// whole response fail and take the logbook down for that pilot.
func finiteOrZero(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// clampInt converts a float to int without the undefined wrap-around that
// int(f) gives for values outside the int range.
func clampInt(f float64) int {
	f = finiteOrZero(f)
	if f > math.MaxInt32 {
		return math.MaxInt32
	}
	if f < math.MinInt32 {
		return math.MinInt32
	}
	return int(f)
}

func (a flightWireAirport) toDomain() domain.AirportInfo {
	lat := parseFloat(a.Latitude)
	if lat == 0 {
		lat = parseFloat(a.Lat)
	}
	lon := parseFloat(a.Longitude)
	if lon == 0 {
		lon = parseFloat(a.Lon)
	}

	lat, lon = resolveCoordinates(a.ICAO, lat, lon)

	return domain.AirportInfo{
		ICAO:      strings.ToUpper(strings.TrimSpace(a.ICAO)),
		IATA:      strings.ToUpper(strings.TrimSpace(a.IATA)),
		Name:      a.Name,
		City:      a.City,
		Country:   a.Country,
		Latitude:  lat,
		Longitude: lon,
	}
}

type flightWireAircraft struct {
	ID           json.RawMessage `json:"id"`
	Registration string          `json:"registration"`
	Name         string          `json:"name"`
	ICAOCode     string          `json:"icao_code"`
	Icao         string          `json:"icao"`
}

func (ac *flightWireAircraft) toDomain() *domain.AircraftInfo {
	if ac == nil {
		return nil
	}
	icao := ac.ICAOCode
	if icao == "" {
		icao = ac.Icao
	}
	return &domain.AircraftInfo{
		Registration: ac.Registration,
		Name:         ac.Name,
		ICAOCode:     icao,
	}
}

type flightWire struct {
	ID               json.RawMessage     `json:"id"`
	Callsign         string              `json:"callsign"`
	FlightNumber     string              `json:"flight_number"`
	Status           string              `json:"status"`
	DepartureAirport flightWireAirport   `json:"departure_airport"`
	ArrivalAirport   flightWireAirport   `json:"arrival_airport"`
	DepartureICAO    string              `json:"departure_airport_icao"`
	ArrivalICAO      string              `json:"arrival_airport_icao"`
	Aircraft         *flightWireAircraft `json:"aircraft"`
	FlightTime       interface{}         `json:"flight_time"`
	DurationMinutes  interface{}         `json:"duration_minutes"`
	Duration         interface{}         `json:"duration"`
	Distance         interface{}         `json:"distance"`
	LandingRate      interface{}         `json:"landing_rate"`
	FuelUsed         interface{}         `json:"fuel_used"`
	Score            interface{}         `json:"score"`
	DepartureTime    string              `json:"departure_time"`
	ArrivalTime      string              `json:"arrival_time"`
	CreatedAt        string              `json:"created_at"`
}

func (f flightWire) toDomain() (domain.FlightLog, error) {
	id, err := parseRawID(f.ID)
	if err != nil || id == "" {
		return domain.FlightLog{}, fmt.Errorf("missing flight id")
	}

	callsign := f.Callsign
	if callsign == "" {
		callsign = f.FlightNumber
	}

	depAirport := f.DepartureAirport.toDomain()
	if depAirport.ICAO == "" && f.DepartureICAO != "" {
		depAirport.ICAO = strings.ToUpper(strings.TrimSpace(f.DepartureICAO))
		depAirport.Latitude, depAirport.Longitude = resolveCoordinates(depAirport.ICAO, 0, 0)
	}

	arrAirport := f.ArrivalAirport.toDomain()
	if arrAirport.ICAO == "" && f.ArrivalICAO != "" {
		arrAirport.ICAO = strings.ToUpper(strings.TrimSpace(f.ArrivalICAO))
		arrAirport.Latitude, arrAirport.Longitude = resolveCoordinates(arrAirport.ICAO, 0, 0)
	}

	flightTime := clampInt(parseFloat(f.FlightTime))
	if flightTime == 0 {
		flightTime = clampInt(parseFloat(f.DurationMinutes))
	}
	if flightTime == 0 {
		flightTime = clampInt(parseFloat(f.Duration))
	}

	dist := parseFloat(f.Distance)
	if dist == 0 {
		dist = calculateGreatCircleDistanceNM(depAirport.Latitude, depAirport.Longitude, arrAirport.Latitude, arrAirport.Longitude)
	}

	landingRate := parseFloat(f.LandingRate)
	fuelUsed := parseFloat(f.FuelUsed)
	score := clampInt(parseFloat(f.Score))

	status := f.Status

	return domain.FlightLog{
		ID:                id,
		Callsign:          callsign,
		FlightNumber:      f.FlightNumber,
		Status:            status,
		DepartureAirport:  depAirport,
		ArrivalAirport:    arrAirport,
		Aircraft:          f.Aircraft.toDomain(),
		FlightTimeMinutes: flightTime,
		DistanceNM:        dist,
		LandingRateFPM:    landingRate,
		FuelUsedKG:        fuelUsed,
		Score:             score,
		DepartureTime:     f.DepartureTime,
		ArrivalTime:       f.ArrivalTime,
		CreatedAt:         f.CreatedAt,
	}, nil
}

func extractPilotUserID(pilot map[string]interface{}) string {
	for _, key := range []string{"id", "user_id", "pilot_id"} {
		switch v := pilot[key].(type) {
		case float64:
			if v > 0 {
				return strconv.FormatInt(int64(v), 10)
			}
		case int:
			if v > 0 {
				return strconv.Itoa(v)
			}
		case string:
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// GetMyFlights loads pilot past flights and calculates summary metrics.
func (a *App) GetMyFlights(page int, limit int) (*domain.MyFlightsResponse, error) {
	if a.GetSettings().LocalMode {
		return &domain.MyFlightsResponse{
			Status:  "localMode",
			Flights: []domain.FlightLog{},
		}, nil
	}
	if a.Airspace.Token() == "" || a.Airspace.BaseURL() == "" {
		return &domain.MyFlightsResponse{
			Status:  "noSession",
			Flights: []domain.FlightLog{},
		}, nil
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	// Fetch pilot profile for overview statistics and user filter
	pilotProfile, err := a.GetPilot()
	if err != nil {
		var se *domain.StatusError
		if errors.As(err, &se) {
			switch se.Status {
			case 401, 403:
				return &domain.MyFlightsResponse{
					Status:  "accessDenied",
					Flights: []domain.FlightLog{},
				}, nil
			case 429:
				return &domain.MyFlightsResponse{
					Status:  "rateLimited",
					Flights: []domain.FlightLog{},
				}, nil
			case 404, 405:
				return &domain.MyFlightsResponse{
					Status:  "unavailable",
					Flights: []domain.FlightLog{},
				}, nil
			}
		}
		return nil, err
	}
	if pilotProfile == nil {
		return &domain.MyFlightsResponse{
			Status:  "unavailable",
			Flights: []domain.FlightLog{},
		}, nil
	}

	pilotUserID := extractPilotUserID(pilotProfile)
	if pilotUserID == "" {
		return &domain.MyFlightsResponse{
			Status:  "unavailable",
			Flights: []domain.FlightLog{},
		}, fmt.Errorf("could not determine pilot user id")
	}

	summary := domain.PilotSummaryStats{
		PilotID: pilotUserID,
	}
	if name, ok := pilotProfile["name"].(string); ok {
		summary.Name = name
	}
	if callsign, ok := pilotProfile["callsign"].(string); ok {
		summary.Callsign = callsign
	}
	if points, ok := pilotProfile["points"].(float64); ok {
		summary.Points = int(points)
	}
	if flightsCount, ok := pilotProfile["flights_count"].(float64); ok {
		summary.TotalFlights = int(flightsCount)
	}
	if flightHours, ok := pilotProfile["flight_hours"].(float64); ok {
		summary.TotalHours = flightHours
		summary.HasGlobalHours = true
	} else if hours, ok := pilotProfile["hours"].(float64); ok {
		summary.TotalHours = hours
		summary.HasGlobalHours = true
	}
	if landingAvg, ok := pilotProfile["landing_rate_avg"].(float64); ok {
		summary.AvgLandingRate = landingAvg
		summary.HasGlobalLandingRate = true
	} else if lRate, ok := pilotProfile["avg_landing_rate"].(float64); ok {
		summary.AvgLandingRate = lRate
		summary.HasGlobalLandingRate = true
	}
	if dist, ok := pilotProfile["total_distance"].(float64); ok {
		summary.TotalDistanceNM = dist
		summary.HasGlobalDistance = true
	} else if dist, ok := pilotProfile["distance"].(float64); ok {
		summary.TotalDistanceNM = dist
		summary.HasGlobalDistance = true
	}
	if rankObj, ok := pilotProfile["rank"].(map[string]interface{}); ok {
		if rName, ok := rankObj["name"].(string); ok {
			summary.Rank = rName
		}
		if rImg, ok := rankObj["image_url"].(string); ok {
			summary.RankImageURL = rImg
		}
	} else if rName, ok := pilotProfile["rank"].(string); ok {
		summary.Rank = rName
	}

	query := url.Values{
		"page":            {strconv.Itoa(page)},
		"per_page":        {strconv.Itoa(limit)},
		"sort":            {"created_at"},
		"sort_dir":        {"desc"},
		"filter[user_id]": {pilotUserID},
	}

	// Only the ACARS pilot route: it takes the pilot token and scopes to it
	// server-side. The private v1 API is for integration keys, not pilots.
	candidates := []string{
		"/api/v2/acars/pilot/flights?" + query.Encode(),
	}

	body, status, err := executeCandidates(candidates, a.executeRequest)
	if err != nil {
		return nil, err
	}

	response := &domain.MyFlightsResponse{
		Status:      status,
		Pilot:       summary,
		Flights:     []domain.FlightLog{},
		CurrentPage: page,
		LastPage:    page,
		Total:       0,
	}

	if status != "ok" {
		return response, nil
	}

	var envelope struct {
		Data        *[]flightWire `json:"data"`
		CurrentPage int           `json:"current_page"`
		LastPage    int           `json:"last_page"`
		Total       int           `json:"total"`
		Meta        struct {
			CurrentPage int `json:"current_page"`
			LastPage    int `json:"last_page"`
			Total       int `json:"total"`
		} `json:"meta"`
	}

	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Data == nil {
		// The body carries pilot names and callsigns, so log its size, not its contents.
		slog.Debug("could not parse flight collection envelope", "bytes", len(body), "err", err)
		return response, nil
	}

	var totalFlightMinutes int
	var totalDistance float64
	var totalLandingRate float64
	var validLandingCount int

	for _, item := range *envelope.Data {
		logItem, err := item.toDomain()
		if err != nil {
			continue
		}
		response.Flights = append(response.Flights, logItem)
		totalFlightMinutes += logItem.FlightTimeMinutes
		totalDistance += logItem.DistanceNM
		if logItem.LandingRateFPM != 0 {
			totalLandingRate += logItem.LandingRateFPM
			validLandingCount++
		}
	}

	if len(response.Flights) > 0 {
		if response.Pilot.TotalFlights == 0 {
			if envelope.Meta.Total > 0 {
				response.Pilot.TotalFlights = envelope.Meta.Total
			} else if envelope.Total > 0 {
				response.Pilot.TotalFlights = envelope.Total
			} else {
				response.Pilot.TotalFlights = len(response.Flights)
			}
		}

		// Only compute aggregate fallback stats from this page if all of the pilot's flights fit on this page.
		isCompleteHistory := response.Pilot.TotalFlights <= len(response.Flights)

		if response.Pilot.TotalHours == 0 && totalFlightMinutes > 0 && isCompleteHistory {
			response.Pilot.TotalHours = math.Round((float64(totalFlightMinutes)/60.0)*10) / 10
		}
		if response.Pilot.AvgLandingRate == 0 && validLandingCount > 0 && isCompleteHistory {
			response.Pilot.AvgLandingRate = math.Round(totalLandingRate / float64(validLandingCount))
		}
		if response.Pilot.TotalDistanceNM == 0 && totalDistance > 0 && isCompleteHistory {
			response.Pilot.TotalDistanceNM = math.Round(totalDistance)
		}
	}

	if envelope.Meta.CurrentPage > 0 {
		response.CurrentPage = envelope.Meta.CurrentPage
	} else if envelope.CurrentPage > 0 {
		response.CurrentPage = envelope.CurrentPage
	}
	if envelope.Meta.LastPage > 0 {
		response.LastPage = envelope.Meta.LastPage
	} else if envelope.LastPage > 0 {
		response.LastPage = envelope.LastPage
	}
	if envelope.Meta.Total > 0 {
		response.Total = envelope.Meta.Total
	} else if envelope.Total > 0 {
		response.Total = envelope.Total
	} else {
		response.Total = len(response.Flights)
	}

	return response, nil
}
