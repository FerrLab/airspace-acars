package gsx

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"airspace-acars/internal/domain"
)

// The message shapes below come from the third-party client Fragtality/Any2GSX;
// FSDreamTeam's own guide to the remote control server ships only inside the
// GSX installer. Everything is decoded leniently: unknown fields and message
// types are ignored, and numbers may arrive as strings.

// errNotJSON means a frame from the server was not a JSON object.
var errNotJSON = errors.New("gsx message is not a JSON object")

// Service ids as GSX names them in state.services[].id.
const (
	serviceBoarding   = "boarding"
	serviceDeboarding = "deboarding"
	serviceDeparture  = "departure"
)

// envelope is the part every server message shares.
type envelope struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	OK         *bool           `json:"ok"`
	GSXRunning *bool           `json:"gsxRunning"`
	Path       string          `json:"path"`
	Value      json.RawMessage `json:"value"`
	Error      json.RawMessage `json:"error"`
}

// serviceEntry is one element of state.services.
type serviceEntry struct {
	ID           string     `json:"id"`
	StateRaw     flexNumber `json:"stateRaw"`
	ProgressText string     `json:"progressText"`
}

// flexNumber decodes a JSON number or a numeric string; anything else reads 0.
type flexNumber float64

// UnmarshalJSON implements json.Unmarshaler.
func (n *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*n = 0
		return nil
	}
	*n = flexNumber(f)
	return nil
}

// subscribeRequest asks the server for the state channel.
var subscribeRequest = map[string]any{"type": "subscribe", "id": "subscribe", "channels": []string{"state"}}

// snapshotRequest asks the server for a fresh state snapshot.
var snapshotRequest = map[string]any{"type": "command", "id": "state.get.1", "verb": "state.get", "args": map[string]any{}}

// tracker follows one connection's messages and keeps the state they add up
// to. It is not safe for concurrent use; the client owns it on its read loop.
type tracker struct {
	running  bool
	services map[string]domain.GSXService
	// rejected is set when the server refused the subscription, most likely
	// because it wants authentication the ACARS cannot give.
	rejected bool
	// subscribed is set once the server has accepted the subscription.
	subscribed bool
}

func newTracker() *tracker {
	return &tracker{services: map[string]domain.GSXService{}}
}

// handle applies one server message. It returns the requests to send back,
// whether the state the ACARS reports changed, and an error for a frame it
// could not read at all.
func (t *tracker) handle(raw []byte) (replies []any, changed bool, err error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false, fmt.Errorf("%w: %v", errNotJSON, err)
	}

	switch env.Type {
	case "hello":
		if env.GSXRunning != nil && *env.GSXRunning != t.running {
			t.running = *env.GSXRunning
			changed = true
		}
		return []any{subscribeRequest}, changed, nil
	case "result":
		if env.ID == "subscribe" {
			if env.OK != nil && !*env.OK {
				t.rejected = true
				return nil, false, nil
			}
			t.subscribed = true
			// The server pushes a snapshot after subscribing; asking again
			// costs one frame and covers a build that waits to be asked.
			return []any{snapshotRequest}, false, nil
		}
		return nil, false, nil
	case "snapshot":
		var top map[string]json.RawMessage
		if err := json.Unmarshal(raw, &top); err != nil {
			return nil, false, fmt.Errorf("%w: %v", errNotJSON, err)
		}
		// Some builds nest the state under "state" or "data".
		for _, key := range []string{"state", "data"} {
			if inner, ok := top[key]; ok {
				var nested map[string]json.RawMessage
				if json.Unmarshal(inner, &nested) == nil {
					top = nested
				}
			}
		}
		for key, value := range top {
			if t.applyKey(key, value) {
				changed = true
			}
		}
		return nil, changed, nil
	case "patch":
		key := strings.Trim(env.Path, "/")
		if i := strings.Index(key, "/"); i >= 0 {
			// A deeper path than a top-level key is not part of the
			// protocol as known; re-read the whole state instead of guessing.
			return []any{snapshotRequest}, false, nil
		}
		return nil, t.applyKey(key, env.Value), nil
	}
	return nil, false, nil
}

// applyKey replaces one top-level state key and reports whether that changed
// what the ACARS reports.
func (t *tracker) applyKey(key string, value json.RawMessage) bool {
	switch key {
	case "gsxRunning":
		var running bool
		if json.Unmarshal(value, &running) != nil || running == t.running {
			return false
		}
		t.running = running
		return true
	case "services":
		var entries []serviceEntry
		if json.Unmarshal(value, &entries) != nil {
			return false
		}
		changed := false
		for _, e := range entries {
			id := strings.ToLower(strings.TrimSpace(e.ID))
			if id != serviceBoarding && id != serviceDeboarding && id != serviceDeparture {
				continue
			}
			next := advance(t.services[id], domain.GSXServiceState(int(e.StateRaw)), e.ProgressText)
			if next != t.services[id] {
				t.services[id] = next
				changed = true
			}
		}
		return changed
	}
	return false
}

// advance moves a service to its next reported state, latching completion:
// completed is reported, or the service drops from in progress back to
// callable or below. Calling the service again clears the latch.
func advance(prev domain.GSXService, state domain.GSXServiceState, progress string) domain.GSXService {
	next := domain.GSXService{State: state, Completed: prev.Completed, Progress: progress}
	switch {
	case state == domain.GSXCompleted:
		next.Completed = true
	case state == domain.GSXRequested || state == domain.GSXActive:
		next.Completed = false
	case prev.State.InProgress() && state <= domain.GSXBypassed:
		next.Completed = true
	}
	return next
}

// state is what the tracker has seen so far, for a connected server.
func (t *tracker) state() domain.GSXState {
	return domain.GSXState{
		Connected:  true,
		Running:    t.running,
		Boarding:   t.services[serviceBoarding],
		Deboarding: t.services[serviceDeboarding],
		Departure:  t.services[serviceDeparture],
	}
}
