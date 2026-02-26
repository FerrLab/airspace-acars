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

// RREF dataref paths — indices match the switch cases in listenLoop.
var xplaneDatarefs = []string{
	// Position (0-3)
	"sim/flightmodel/position/latitude",          // 0: degrees
	"sim/flightmodel/position/longitude",         // 1: degrees
	"sim/flightmodel/position/elevation",         // 2: altitude (meters → feet)
	"sim/flightmodel/position/y_agl",             // 3: AGL (meters → feet)

	// Attitude (4-11)
	"sim/flightmodel/position/theta",             // 4: pitch (degrees)
	"sim/flightmodel/position/phi",               // 5: roll (degrees)
	"sim/flightmodel/position/psi",               // 6: heading true (degrees)
	"sim/flightmodel/position/mag_psi",           // 7: heading magnetic (degrees)
	"sim/flightmodel/position/vh_ind_fpm",        // 8: vertical speed (fpm)
	"sim/flightmodel/position/indicated_airspeed", // 9: IAS (kias)
	"sim/flightmodel/position/true_airspeed",     // 10: TAS (m/s → knots)
	"sim/flightmodel/position/groundspeed",       // 11: GS (m/s → knots)

	// Engine 1 (12-17)
	"sim/flightmodel/engine/ENGN_running[0]",              // 12: engine 1 running
	"sim/flightmodel/engine/ENGN_N1_[0]",                  // 13: engine 1 N1
	"sim/flightmodel/engine/ENGN_N2_[0]",                  // 14: engine 1 N2
	"sim/cockpit2/engine/actuators/throttle_ratio[0]",     // 15: engine 1 throttle
	"sim/cockpit2/engine/actuators/mixture_ratio[0]",      // 16: engine 1 mixture
	"sim/cockpit2/engine/actuators/prop_ratio[0]",         // 17: engine 1 prop

	// Sensors (18-21)
	"sim/flightmodel/failures/onground_any",      // 18: on ground
	"sim/cockpit2/annunciators/stall_warning",    // 19: stall warning
	"sim/cockpit2/annunciators/overspeed",        // 20: overspeed warning
	"sim/time/sim_speed",                         // 21: simulation rate

	// Radios (22-29)
	"sim/cockpit/radios/com1_freq_hz",            // 22: COM1
	"sim/cockpit/radios/com2_freq_hz",            // 23: COM2
	"sim/cockpit/radios/nav1_freq_hz",            // 24: NAV1
	"sim/cockpit/radios/nav2_freq_hz",            // 25: NAV2
	"sim/cockpit/radios/nav1_obs_degm",           // 26: NAV1 OBS
	"sim/cockpit/radios/nav2_obs_degm",           // 27: NAV2 OBS
	"sim/cockpit/radios/transponder_code",        // 28: transponder code
	"sim/cockpit/radios/transponder_mode",        // 29: transponder state

	// Autopilot (30-36)
	"sim/cockpit/autopilot/autopilot_mode",       // 30: AP master
	"sim/cockpit/autopilot/heading_mag",          // 31: AP heading
	"sim/cockpit/autopilot/altitude",             // 32: AP altitude
	"sim/cockpit/autopilot/vertical_velocity",    // 33: AP VS
	"sim/cockpit/autopilot/airspeed",             // 34: AP speed
	"sim/cockpit2/autopilot/approach_status",     // 35: AP approach hold
	"sim/cockpit2/autopilot/nav_status",          // 36: AP nav lock

	// Altimeter (37)
	"sim/cockpit/misc/barometer_setting",         // 37: altimeter inHg

	// Lights (38-40)
	"sim/cockpit/electrical/beacon_lights_on",    // 38: beacon
	"sim/cockpit/electrical/strobe_lights_on",    // 39: strobe
	"sim/cockpit/electrical/landing_lights_on",   // 40: landing

	// Controls (41-46)
	"sim/cockpit2/controls/yoke_pitch_ratio",     // 41: elevator (-1 to 1)
	"sim/cockpit2/controls/yoke_roll_ratio",      // 42: aileron (-1 to 1)
	"sim/cockpit2/controls/yoke_heading_ratio",   // 43: rudder (-1 to 1)
	"sim/cockpit2/controls/flap_ratio",           // 44: flaps (0-1 → percent)
	"sim/cockpit2/controls/speedbrake_ratio",     // 45: spoilers (0-1 → percent)
	"sim/cockpit/switches/gear_handle_status",    // 46: gear down

	// SimTime (47-51)
	"sim/time/zulu_time_sec",                     // 47: zulu time (seconds)
	"sim/time/local_date_days",                   // 48: zulu day (proxy)
	"sim/time/local_date_days",                   // 49: zulu month (proxy)
	"sim/time/local_date_days",                   // 50: zulu year (proxy)
	"sim/time/local_time_sec",                    // 51: local time (seconds)

	// G-Force (52)
	"sim/flightmodel/position/g_nrml",            // 52: normal G-force

	// Engine 2 (53-58)
	"sim/flightmodel/engine/ENGN_running[1]",              // 53: engine 2 running
	"sim/flightmodel/engine/ENGN_N1_[1]",                  // 54: engine 2 N1
	"sim/flightmodel/engine/ENGN_N2_[1]",                  // 55: engine 2 N2
	"sim/cockpit2/engine/actuators/throttle_ratio[1]",     // 56: engine 2 throttle
	"sim/cockpit2/engine/actuators/mixture_ratio[1]",      // 57: engine 2 mixture
	"sim/cockpit2/engine/actuators/prop_ratio[1]",         // 58: engine 2 prop

	// Engine 3 (59-64)
	"sim/flightmodel/engine/ENGN_running[2]",              // 59: engine 3 running
	"sim/flightmodel/engine/ENGN_N1_[2]",                  // 60: engine 3 N1
	"sim/flightmodel/engine/ENGN_N2_[2]",                  // 61: engine 3 N2
	"sim/cockpit2/engine/actuators/throttle_ratio[2]",     // 62: engine 3 throttle
	"sim/cockpit2/engine/actuators/mixture_ratio[2]",      // 63: engine 3 mixture
	"sim/cockpit2/engine/actuators/prop_ratio[2]",         // 64: engine 3 prop

	// Engine 4 (65-70)
	"sim/flightmodel/engine/ENGN_running[3]",              // 65: engine 4 running
	"sim/flightmodel/engine/ENGN_N1_[3]",                  // 66: engine 4 N1
	"sim/flightmodel/engine/ENGN_N2_[3]",                  // 67: engine 4 N2
	"sim/cockpit2/engine/actuators/throttle_ratio[3]",     // 68: engine 4 throttle
	"sim/cockpit2/engine/actuators/mixture_ratio[3]",      // 69: engine 4 mixture
	"sim/cockpit2/engine/actuators/prop_ratio[3]",         // 70: engine 4 prop

	// Weight (71-72)
	"sim/flightmodel/weight/m_total",                      // 71: total weight (kg → lbs)
	"sim/flightmodel/weight/m_fuel_total",                 // 72: fuel weight (kg → lbs)
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

	data := x.data
	return &data, nil
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
			// Position
			case 0:
				x.data.Position.Latitude = float64(val)
			case 1:
				x.data.Position.Longitude = float64(val)
			case 2:
				x.data.Position.Altitude = float64(val) * 3.28084 // meters to feet
			case 3:
				x.data.Position.AltitudeAGL = float64(val) * 3.28084

			// Attitude
			case 4:
				x.data.Attitude.Pitch = float64(val)
			case 5:
				x.data.Attitude.Roll = float64(val)
			case 6:
				x.data.Attitude.HeadingTrue = float64(val)
			case 7:
				x.data.Attitude.HeadingMag = float64(val)
			case 8:
				x.data.Attitude.VS = float64(val) // already fpm
			case 9:
				x.data.Attitude.IAS = float64(val)
			case 10:
				x.data.Attitude.TAS = float64(val) * 1.94384 // m/s to knots
			case 11:
				x.data.Attitude.GS = float64(val) * 1.94384

			// Engine 1
			case 12:
				x.data.Engines[0].Running = val != 0
			case 13:
				x.data.Engines[0].N1 = float64(val)
			case 14:
				x.data.Engines[0].N2 = float64(val)
			case 15:
				x.data.Engines[0].ThrottlePos = float64(val) * 100
			case 16:
				x.data.Engines[0].MixturePos = float64(val) * 100
			case 17:
				x.data.Engines[0].PropPos = float64(val) * 100

			// Sensors
			case 18:
				x.data.Sensors.OnGround = val != 0
			case 19:
				x.data.Sensors.StallWarning = val != 0
			case 20:
				x.data.Sensors.OverspeedWarning = val != 0
			case 21:
				x.data.Sensors.SimulationRate = float64(val)

			// Radios
			case 22:
				x.data.Radios.Com1 = float64(val) / 100 // hz to MHz
			case 23:
				x.data.Radios.Com2 = float64(val) / 100
			case 24:
				x.data.Radios.Nav1 = float64(val) / 100
			case 25:
				x.data.Radios.Nav2 = float64(val) / 100
			case 26:
				x.data.Radios.Nav1OBS = float64(val)
			case 27:
				x.data.Radios.Nav2OBS = float64(val)
			case 28:
				x.data.Radios.XpdrCode = float64(val)
			case 29:
				x.data.Radios.XpdrState = TransponderStateString(float64(val))

			// Autopilot
			case 30:
				x.data.Autopilot.Master = val != 0
			case 31:
				x.data.Autopilot.Heading = float64(val)
			case 32:
				x.data.Autopilot.Altitude = float64(val)
			case 33:
				x.data.Autopilot.VS = float64(val)
			case 34:
				x.data.Autopilot.Speed = float64(val)
			case 35:
				x.data.Autopilot.ApproachHold = val != 0
			case 36:
				x.data.Autopilot.NavLock = val != 0

			// Altimeter
			case 37:
				x.data.Altimeter = float64(val)

			// Lights
			case 38:
				x.data.Lights.Beacon = val != 0
			case 39:
				x.data.Lights.Strobe = val != 0
			case 40:
				x.data.Lights.Landing = val != 0

			// Controls
			case 41:
				x.data.Controls.Elevator = float64(val)
			case 42:
				x.data.Controls.Aileron = float64(val)
			case 43:
				x.data.Controls.Rudder = float64(val)
			case 44:
				x.data.Controls.Flaps = float64(val) * 100 // 0-1 → percent
			case 45:
				x.data.Controls.Spoilers = float64(val) * 100
			case 46:
				x.data.Controls.GearDown = val != 0

			// SimTime
			case 47:
				x.data.SimTime.ZuluTime = float64(val)
			case 48:
				x.data.SimTime.ZuluDay = float64(val)
			case 49:
				x.data.SimTime.ZuluMonth = float64(val)
			case 50:
				x.data.SimTime.ZuluYear = float64(val)
			case 51:
				x.data.SimTime.LocalTime = float64(val)

			// G-Force
			case 52:
				x.data.Attitude.GForce = float64(val)

			// Engine 2
			case 53:
				x.data.Engines[1].Running = val != 0
			case 54:
				x.data.Engines[1].N1 = float64(val)
			case 55:
				x.data.Engines[1].N2 = float64(val)
			case 56:
				x.data.Engines[1].ThrottlePos = float64(val) * 100
			case 57:
				x.data.Engines[1].MixturePos = float64(val) * 100
			case 58:
				x.data.Engines[1].PropPos = float64(val) * 100

			// Engine 3
			case 59:
				x.data.Engines[2].Running = val != 0
			case 60:
				x.data.Engines[2].N1 = float64(val)
			case 61:
				x.data.Engines[2].N2 = float64(val)
			case 62:
				x.data.Engines[2].ThrottlePos = float64(val) * 100
			case 63:
				x.data.Engines[2].MixturePos = float64(val) * 100
			case 64:
				x.data.Engines[2].PropPos = float64(val) * 100

			// Engine 4
			case 65:
				x.data.Engines[3].Running = val != 0
			case 66:
				x.data.Engines[3].N1 = float64(val)
			case 67:
				x.data.Engines[3].N2 = float64(val)
			case 68:
				x.data.Engines[3].ThrottlePos = float64(val) * 100
			case 69:
				x.data.Engines[3].MixturePos = float64(val) * 100
			case 70:
				x.data.Engines[3].PropPos = float64(val) * 100

			// Weight
			case 71:
				x.data.Weight.TotalWeight = float64(val) * 2.20462 // kg to lbs
			case 72:
				x.data.Weight.FuelWeight = float64(val) * 2.20462
			}
			x.mu.Unlock()
		}
	}
}
