package main

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"
)

type XPlaneAdapter struct {
	host string
	port int

	mu   sync.Mutex
	conn *net.UDPConn
	data FlightData
	stop chan struct{}
}

// RREF dataref paths
var xplaneDatarefs = []string{
	"sim/flightmodel/position/elevation",        // altitude (meters)
	"sim/flightmodel/position/psi",              // heading (degrees true)
	"sim/flightmodel/position/theta",            // pitch (degrees)
	"sim/flightmodel/position/phi",              // roll (degrees)
	"sim/flightmodel/position/indicated_airspeed", // airspeed (kias)
	"sim/flightmodel/position/groundspeed",      // ground speed (m/s)
	"sim/flightmodel/position/vh_ind_fpm",       // vertical speed (fpm)
}

func NewXPlaneAdapter(host string, port int) SimConnector {
	return &XPlaneAdapter{
		host: host,
		port: port,
	}
}

func (x *XPlaneAdapter) Name() string {
	return "X-Plane"
}

func (x *XPlaneAdapter) Connect() error {
	x.mu.Lock()
	defer x.mu.Unlock()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", x.host, x.port))
	if err != nil {
		return fmt.Errorf("resolve addr: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("dial udp: %w", err)
	}
	x.conn = conn

	// Subscribe to datarefs using RREF protocol
	for i, dref := range xplaneDatarefs {
		if err := x.subscribeRREF(i, 1, dref); err != nil {
			conn.Close()
			x.conn = nil
			return fmt.Errorf("subscribe %s: %w", dref, err)
		}
	}

	x.stop = make(chan struct{})
	go x.listenLoop()

	slog.Info("X-Plane UDP connected", "addr", addr.String())
	return nil
}

func (x *XPlaneAdapter) Disconnect() error {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.stop != nil {
		close(x.stop)
		x.stop = nil
	}

	if x.conn != nil {
		// Unsubscribe by sending frequency 0
		for i, dref := range xplaneDatarefs {
			x.subscribeRREF(i, 0, dref)
		}
		x.conn.Close()
		x.conn = nil
	}
	return nil
}

func (x *XPlaneAdapter) GetFlightData() (*FlightData, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	return &FlightData{
		Altitude:      x.data.Altitude,
		Heading:       x.data.Heading,
		Pitch:         x.data.Pitch,
		Roll:          x.data.Roll,
		Airspeed:      x.data.Airspeed,
		GroundSpeed:   x.data.GroundSpeed,
		VerticalSpeed: x.data.VerticalSpeed,
	}, nil
}

func (x *XPlaneAdapter) subscribeRREF(index, freq int, dataref string) error {
	// RREF packet: "RREF\0" + freq(4 bytes) + index(4 bytes) + dataref(400 bytes null-padded)
	buf := make([]byte, 413)
	copy(buf[0:4], "RREF")
	buf[4] = 0
	binary.LittleEndian.PutUint32(buf[5:9], uint32(freq))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(index))
	copy(buf[13:], dataref)

	_, err := x.conn.Write(buf)
	return err
}

func (x *XPlaneAdapter) listenLoop() {
	buf := make([]byte, 4096)

	// Create a local UDP listener to receive responses
	localAddr := x.conn.LocalAddr().(*net.UDPAddr)
	listener, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		slog.Error("failed to listen for X-Plane responses", "error", err)
		return
	}
	defer listener.Close()

	listener.SetReadDeadline(time.Now().Add(2 * time.Second))

	for {
		select {
		case <-x.stop:
			return
		default:
		}

		listener.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, err := listener.Read(buf)
		if err != nil {
			continue
		}

		if n < 5 || string(buf[0:4]) != "RREF" {
			continue
		}

		// Parse RREF response: header(5) + entries of (index:4 + value:4)
		offset := 5
		for offset+8 <= n {
			idx := int(binary.LittleEndian.Uint32(buf[offset : offset+4]))
			val := math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))
			offset += 8

			x.mu.Lock()
			switch idx {
			case 0:
				x.data.Altitude = float64(val) * 3.28084 // meters to feet
			case 1:
				x.data.Heading = float64(val)
			case 2:
				x.data.Pitch = float64(val)
			case 3:
				x.data.Roll = float64(val)
			case 4:
				x.data.Airspeed = float64(val)
			case 5:
				x.data.GroundSpeed = float64(val) * 1.94384 // m/s to knots
			case 6:
				x.data.VerticalSpeed = float64(val)
			}
			x.mu.Unlock()
		}
	}
}
