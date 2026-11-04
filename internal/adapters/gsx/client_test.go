package gsx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"airspace-acars/internal/domain"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quickReconnect shortens the reconnect pause for one test.
func quickReconnect(t *testing.T) {
	t.Helper()
	prev := reconnectEvery
	reconnectEvery = 10 * time.Millisecond
	t.Cleanup(func() { reconnectEvery = prev })
}

// fakeGSX speaks the server's side of the handshake and then sends patches.
// The first `drop` connections are closed straight after hello.
func fakeGSX(t *testing.T, drop int32, patches ...string) *httptest.Server {
	t.Helper()
	var conns atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		write := func(s string) { _ = c.Write(ctx, websocket.MessageText, []byte(s)) }
		expect := func(want string) bool {
			_, data, err := c.Read(ctx)
			if err != nil {
				return false
			}
			var msg map[string]any
			_ = json.Unmarshal(data, &msg)
			return msg["type"] == want
		}

		write(`{"v":1,"type":"hello","protocol":1,"engine":"gsx","gsxRunning":true,"authRequired":false,"capabilities":["state"]}`)
		if conns.Add(1) <= drop {
			c.Close(websocket.StatusGoingAway, "bye")
			return
		}
		if !expect("subscribe") {
			return
		}
		write(`{"v":1,"type":"result","id":"subscribe","ok":true}`)
		if !expect("command") {
			return
		}
		for _, p := range patches {
			write(p)
		}
		// Hold the connection until the client goes.
		_, _, _ = c.Read(ctx)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string { return "ws" + strings.TrimPrefix(srv.URL, "http") }

// waitFor returns the first state the client reports that satisfies ok.
func waitFor(t *testing.T, states <-chan domain.GSXState, ok func(domain.GSXState) bool) domain.GSXState {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-states:
			if ok(s) {
				return s
			}
		case <-deadline:
			t.Fatal("the client never reported the expected GSX state")
		}
	}
}

func TestClientFollowsBoardingFromTheServer(t *testing.T) {
	srv := fakeGSX(t, 0, `{"type":"patch","path":"/services","value":[{"id":"Boarding","stateRaw":5,"progressText":"12/180"}]}`)
	states := make(chan domain.GSXState, 16)
	c := NewClient(wsURL(srv))
	c.OnChange = func(s domain.GSXState) { states <- s }
	c.Start()
	defer c.Stop()

	s := waitFor(t, states, func(s domain.GSXState) bool { return s.Boarding.State == domain.GSXActive })
	assert.True(t, s.Connected)
	assert.True(t, s.Running)
	assert.Equal(t, "12/180", c.State().Boarding.Progress)
}

// GSX quitting is ordinary: the client reports itself disconnected and is
// back as soon as the server answers again.
func TestClientReconnectsAfterTheServerGoes(t *testing.T) {
	quickReconnect(t)
	srv := fakeGSX(t, 1, `{"type":"patch","path":"/services","value":[{"id":"Departure","stateRaw":5}]}`)
	states := make(chan domain.GSXState, 32)
	c := NewClient(wsURL(srv))
	c.OnChange = func(s domain.GSXState) { states <- s }
	c.Start()
	defer c.Stop()

	waitFor(t, states, func(s domain.GSXState) bool { return s.Connected })
	waitFor(t, states, func(s domain.GSXState) bool { return !s.Connected })
	s := waitFor(t, states, func(s domain.GSXState) bool { return s.Departure.State == domain.GSXActive })
	assert.True(t, s.Connected)
}

// A real GSX snapshot is over a megabyte, nearly all of it handlerData: one
// captured at SBBR with boarding under way was 1,217,565 bytes. The client
// must read it; a smaller cap ends every session on its first snapshot and
// the pilot sees GSX as not running for the whole flight.
func TestClientReadsASnapshotTheSizeOfARealOne(t *testing.T) {
	handlerData := strings.Repeat("x", 1_200_000)
	snapshot := `{"v":1,"type":"snapshot","state":0,"handlerData":"` + handlerData +
		`","services":[{"id":"Boarding","stateRaw":5,"progressText":"66/156"}]}`
	srv := fakeGSX(t, 0, snapshot)
	states := make(chan domain.GSXState, 16)
	c := NewClient(wsURL(srv))
	c.OnChange = func(s domain.GSXState) { states <- s }
	c.Start()
	defer c.Stop()

	s := waitFor(t, states, func(s domain.GSXState) bool { return s.Boarding.State == domain.GSXActive })
	assert.True(t, s.Connected, "a %d-byte snapshot dropped the connection", len(snapshot))
	assert.Equal(t, "66/156", s.Boarding.Progress)
}

// Nothing listening is the normal case on most machines: the client stays
// disconnected and Stop still returns promptly.
func TestClientWithoutGSXStaysDisconnected(t *testing.T) {
	quickReconnect(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := wsURL(srv)
	srv.Close()

	c := NewClient(url)
	c.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { c.Stop(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Stop did not return")
	}
	require.False(t, c.State().Connected)
}
