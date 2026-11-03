// Package gsx follows GSX Pro's ground services through its remote control
// server, a websocket GSX opens on the pilot's machine (GSX 4.0.6 or later,
// with "Remote control server" enabled, which is the default).
package gsx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/coder/websocket"
)

// DefaultURL is where GSX's remote control server listens. It takes no path.
const DefaultURL = "ws://127.0.0.1:8744"

// maxMessageBytes caps one frame from the server. A full state snapshot is a
// few kilobytes; anything far past that is not GSX.
const maxMessageBytes = 1 << 20

// How the client reaches GSX. Variables so a test can run the reconnect path
// without waiting out the real interval.
var (
	// reconnectEvery is the pause between attempts while GSX is not
	// answering. Most pilots start GSX minutes after the ACARS, and boarding
	// is called minutes after that, so five seconds loses nothing and costs
	// one refused local connection per attempt.
	reconnectEvery = 5 * time.Second
	// dialTimeout bounds one attempt. The server is on the same machine, so
	// a dial that takes longer is a server that is not going to answer.
	dialTimeout = 3 * time.Second
)

// errDisconnected wraps the end of a session that had connected: GSX closing,
// or the simulator going away. It is logged when it happens and is not a
// failure to reach the server.
var errDisconnected = errors.New("gsx connection ended")

// Client keeps a connection to GSX's remote control server and the state of
// the ground services it reports.
type Client struct {
	url string

	// OnChange, when set before Start, is told every new state on the
	// client's goroutine. It must not block.
	OnChange func(domain.GSXState)

	// mu guards state, failures and warned.
	mu       sync.Mutex
	state    domain.GSXState
	failures int
	// warned is set once an unusable server has been reported at Warn, and
	// cleared by a session that subscribes, so a server that keeps refusing
	// is reported once and not every reconnectEvery.
	warned bool

	cancel  context.CancelFunc
	stopped chan struct{}
}

// NewClient returns a client for the server at url. Nothing connects until
// Start.
func NewClient(url string) *Client {
	return &Client{url: url}
}

// Start connects in the background and keeps reconnecting until Stop.
func (c *Client) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.stopped = make(chan struct{})
	go c.run(ctx)
}

// Stop closes the connection and waits for the client to finish.
func (c *Client) Stop() {
	if c.cancel == nil {
		return
	}
	c.cancel()
	<-c.stopped
}

// State returns what GSX last reported. Connected is false while the server
// does not answer.
func (c *Client) State() domain.GSXState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) run(ctx context.Context) {
	defer close(c.stopped)
	defer observability.Recover()

	for {
		err := c.session(ctx)
		c.setState(domain.GSXState{})
		if ctx.Err() != nil {
			return
		}
		c.noteFailure(err)

		t := time.NewTimer(reconnectEvery)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// session is one connection: dial, follow messages until the connection
// ends, and return why it ended.
func (c *Client) session(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	conn, resp, err := websocket.Dial(dialCtx, c.url, nil)
	cancel()
	if resp != nil && resp.Body != nil {
		// Only a failed upgrade leaves a body; nothing in it is needed.
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("dial gsx: %w", err)
	}
	defer func() {
		// The connection is done with either way; a failed close only
		// means the server went first.
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(maxMessageBytes)

	c.mu.Lock()
	missed := c.failures
	c.failures = 0
	c.mu.Unlock()
	slog.Info("gsx: remote control server connected", "gsx.after_failed_attempts", missed)
	observability.Count("gsx.connected")

	t := newTracker()
	c.setState(t.state())
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Info("gsx: remote control server disconnected", "error", err)
			}
			return fmt.Errorf("%w: %v", errDisconnected, err)
		}
		replies, changed, err := t.handle(data)
		if err != nil {
			observability.Count("gsx.message_unreadable")
			slog.Debug("gsx: unreadable message", "error", err)
			continue
		}
		if t.rejected {
			// Authentication is the only known reason; the ACARS has no
			// credentials to offer, so the pilot has to turn it off in GSX.
			return errors.New("gsx refused the state subscription; is authentication required in the GSX remote control settings?")
		}
		for _, r := range replies {
			if err := c.send(ctx, conn, r); err != nil {
				return err
			}
		}
		if t.subscribed {
			c.mu.Lock()
			c.warned = false
			c.mu.Unlock()
		}
		if changed {
			c.setState(t.state())
		}
	}
}

func (c *Client) send(ctx context.Context, conn *websocket.Conn, msg any) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal gsx request: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		return fmt.Errorf("write gsx: %w", err)
	}
	return nil
}

func (c *Client) setState(s domain.GSXState) {
	c.mu.Lock()
	same := c.state == s
	c.state = s
	c.mu.Unlock()
	if !same && c.OnChange != nil {
		c.OnChange(s)
	}
}

// noteFailure logs an attempt that ended. GSX not running is the normal
// state of most machines most of the time, so a refused connection is Debug,
// always: it must never reach the error tracker. Anything else is said once
// at Warn, and the rest of the streak at Debug, since it repeats every
// reconnectEvery.
func (c *Client) noteFailure(err error) {
	c.mu.Lock()
	c.failures++
	n := c.failures
	warn := !c.warned && !errors.Is(err, errDisconnected) && !observability.Transient(err)
	if warn {
		c.warned = true
	}
	c.mu.Unlock()

	if warn {
		slog.Warn("gsx: remote control server unusable; will keep trying", "error", err)
		return
	}
	slog.Debug("gsx: remote control server not answering", "error", err, "attempts", n)
}
