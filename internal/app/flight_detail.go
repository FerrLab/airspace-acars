package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// flightDetailPath is the pilot-token route for one of the pilot's flights.
// It answers 404 for another pilot's flight, so a stale id from a previous
// company reads as "unavailable", never as someone else's data.
const flightDetailPath = "/api/v2/acars/flights/"

// errInvalidFlightID rejects an id that is not a single path segment, so the
// webview can never steer the request to another route.
var errInvalidFlightID = errors.New("invalid flight id")

// errInvalidFlightDetail reports a reply that is not a flight detail, so it
// is shown as a load error rather than as a flight with nothing recorded.
var errInvalidFlightDetail = errors.New("invalid flight detail response")

// errInvalidFlightTrack is errInvalidFlightDetail for the track reply.
var errInvalidFlightTrack = errors.New("invalid flight track response")

// flightDetailWire decodes the detail reply. The server sends numeric ids;
// the outer ID fields shadow the domain ones so string ids work too.
type flightDetailWire struct {
	domain.FlightDetail
	ID     json.RawMessage   `json:"id"`
	Events []flightEventWire `json:"events"`
	Points struct {
		Total   float64          `json:"total"`
		Entries []pointEntryWire `json:"entries"`
	} `json:"points"`
}

type flightEventWire struct {
	domain.FlightEvent
	ID json.RawMessage `json:"id"`
}

type pointEntryWire struct {
	domain.PointEntry
	ID json.RawMessage `json:"id"`
}

func (w flightDetailWire) toDomain() (domain.FlightDetail, error) {
	detail := w.FlightDetail
	id, err := parseRawID(w.ID)
	if err != nil || id == "" {
		return domain.FlightDetail{}, errInvalidFlightDetail
	}
	detail.ID = id

	detail.Events = make([]domain.FlightEvent, 0, len(w.Events))
	for _, event := range w.Events {
		eventID, err := parseRawID(event.ID)
		if err != nil {
			return domain.FlightDetail{}, errInvalidFlightDetail
		}
		event.FlightEvent.ID = eventID
		detail.Events = append(detail.Events, event.FlightEvent)
	}

	detail.Points = domain.FlightPoints{Total: w.Points.Total, Entries: make([]domain.PointEntry, 0, len(w.Points.Entries))}
	for _, entry := range w.Points.Entries {
		entryID, err := parseRawID(entry.ID)
		if err != nil {
			return domain.FlightDetail{}, errInvalidFlightDetail
		}
		entry.PointEntry.ID = entryID
		detail.Points.Entries = append(detail.Points.Entries, entry.PointEntry)
	}

	if detail.ScoreBreakdown.Categories == nil {
		detail.ScoreBreakdown.Categories = []domain.ScoreCategory{}
	}
	if detail.ScoreBreakdown.Contributing == nil {
		detail.ScoreBreakdown.Contributing = []string{}
	}
	return detail, nil
}

// unwrapData returns the "data" member of a JSON object, or the body itself
// when the reply is flat. It errors on a body that is not JSON at all.
func unwrapData(body []byte) (json.RawMessage, error) {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		return envelope.Data, nil
	}
	return body, nil
}

// validFlightID reports whether id is a single, plain path segment.
func validFlightID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, "/\\?#%")
}

// GetFlightDetail fetches one of the pilot's flights with its landing and
// takeoff analysis, score breakdown, FDM events and points. A status other
// than "ok" (no session, the airline lacking the route, access denied) is
// returned in the response, not as an error.
func (a *App) GetFlightDetail(id string) (*domain.FlightDetailResponse, error) {
	if !validFlightID(id) {
		return nil, errInvalidFlightID
	}
	_, span := observability.Start(context.Background(), "flights.get_detail")
	defer span.Finish()

	body, status, err := executeCandidates([]string{flightDetailPath + url.PathEscape(id)}, a.executeRequest)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	result := &domain.FlightDetailResponse{Status: status}
	if status != "ok" {
		span.Set("http.result", status)
		return result, nil
	}

	data, err := unwrapData(body)
	if err != nil {
		span.Fail(errInvalidFlightDetail)
		return nil, errInvalidFlightDetail
	}
	var wire flightDetailWire
	if err := json.Unmarshal(data, &wire); err != nil {
		span.Fail(errInvalidFlightDetail)
		return nil, errInvalidFlightDetail
	}
	detail, err := wire.toDomain()
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	result.Data = &detail
	return result, nil
}

// GetFlightTrack fetches the flight's recorded positions in time order.
// The server downsamples to at most 5000 samples, which the chart and map
// cannot meaningfully exceed.
func (a *App) GetFlightTrack(id string) (*domain.FlightTrackResponse, error) {
	if !validFlightID(id) {
		return nil, errInvalidFlightID
	}
	_, span := observability.Start(context.Background(), "flights.get_track")
	defer span.Finish()

	body, status, err := executeCandidates([]string{flightDetailPath + url.PathEscape(id) + "/track"}, a.executeRequest)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	result := &domain.FlightTrackResponse{Status: status, Points: []domain.TrackPoint{}}
	if status != "ok" {
		span.Set("http.result", status)
		return result, nil
	}

	var reply struct {
		Data *[]domain.TrackPoint `json:"data"`
	}
	if err := json.Unmarshal(body, &reply); err != nil || reply.Data == nil {
		span.Fail(errInvalidFlightTrack)
		return nil, errInvalidFlightTrack
	}
	result.Points = *reply.Data
	span.Set("track.points", len(result.Points))
	return result, nil
}
