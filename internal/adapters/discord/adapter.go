package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"airspace-acars/observability"

	"go.opentelemetry.io/otel/codes"
)

var discordTracer = observability.Tracer("discord")

const clientID = "1471884432234381494"

// Adapter handles Discord Rich Presence IPC communication.
type Adapter struct {
	mu        sync.Mutex
	pipe      *os.File
	connected bool
}

// NewAdapter creates a new Discord presence adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Connect opens the Discord IPC pipe and performs the handshake.
func (d *Adapter) Connect() error {
	_, span := discordTracer.Start(context.Background(), "discord.connect")
	defer span.End()

	d.mu.Lock()
	defer d.mu.Unlock()

	for i := 0; i < 10; i++ {
		path := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}

		hs, _ := json.Marshal(map[string]interface{}{
			"v":         1,
			"client_id": clientID,
		})
		if err := writeFrame(f, 0, hs); err != nil {
			f.Close()
			continue
		}
		if _, err := readFrame(f); err != nil {
			f.Close()
			continue
		}

		d.pipe = f
		d.connected = true
		slog.Info("discord: connected", "pipe", i)
		return nil
	}
	err := fmt.Errorf("no discord pipe found")
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return err
}

// Disconnect closes the Discord IPC pipe.
func (d *Adapter) Disconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pipe != nil {
		d.pipe.Close()
		d.pipe = nil
	}
	d.connected = false
}

// IsConnected returns whether the Discord pipe is connected.
func (d *Adapter) IsConnected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connected
}

// SetActivity sends a SET_ACTIVITY command to Discord.
func (d *Adapter) SetActivity(activity map[string]interface{}) error {
	_, span := discordTracer.Start(context.Background(), "discord.set_activity")
	defer span.End()

	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.connected || d.pipe == nil {
		return fmt.Errorf("not connected")
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"cmd":   "SET_ACTIVITY",
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
		"args": map[string]interface{}{
			"pid":      os.Getpid(),
			"activity": activity,
		},
	})
	if err := writeFrame(d.pipe, 1, payload); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err := readFrame(d.pipe)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func writeFrame(pipe *os.File, opcode uint32, payload []byte) error {
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[0:4], opcode)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	if _, err := pipe.Write(hdr); err != nil {
		return err
	}
	_, err := pipe.Write(payload)
	return err
}

func readFrame(pipe *os.File) (json.RawMessage, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(pipe, hdr); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(hdr[4:8])
	buf := make([]byte, length)
	if _, err := io.ReadFull(pipe, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
