package xplane

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"airspace-acars/internal/domain"
)

var xplaneDatarefs = []string{
	"sim/flightmodel/position/latitude",
	"sim/flightmodel/position/longitude",
	"sim/flightmodel/position/elevation",
	"sim/flightmodel/position/y_agl",
	"sim/flightmodel/position/theta",
	"sim/flightmodel/position/phi",
	"sim/flightmodel/position/psi",
	"sim/flightmodel/position/mag_psi",
	"sim/flightmodel/position/vh_ind_fpm",
	"sim/flightmodel/position/indicated_airspeed",
	"sim/flightmodel/position/true_airspeed",
	"sim/flightmodel/position/groundspeed",
	"sim/flightmodel/engine/ENGN_running[0]",
	"sim/flightmodel/engine/ENGN_N1_[0]",
	"sim/flightmodel/engine/ENGN_N2_[0]",
	"sim/cockpit2/engine/actuators/throttle_ratio[0]",
	"sim/cockpit2/engine/actuators/mixture_ratio[0]",
	"sim/cockpit2/engine/actuators/prop_ratio[0]",
	"sim/flightmodel/failures/onground_any",
	"sim/cockpit2/annunciators/stall_warning",
	"sim/cockpit2/annunciators/overspeed",
	"sim/time/sim_speed",
	"sim/cockpit/radios/com1_freq_hz",
	"sim/cockpit/radios/com2_freq_hz",
	"sim/cockpit/radios/nav1_freq_hz",
	"sim/cockpit/radios/nav2_freq_hz",
	"sim/cockpit/radios/nav1_obs_degm",
	"sim/cockpit/radios/nav2_obs_degm",
	"sim/cockpit/radios/transponder_code",
	"sim/cockpit/radios/transponder_mode",
	"sim/cockpit/autopilot/autopilot_mode",
	"sim/cockpit/autopilot/heading_mag",
	"sim/cockpit/autopilot/altitude",
	"sim/cockpit/autopilot/vertical_velocity",
	"sim/cockpit/autopilot/airspeed",
	"sim/cockpit2/autopilot/approach_status",
	"sim/cockpit2/autopilot/nav_status",
	"sim/cockpit/misc/barometer_setting",
	"sim/cockpit/electrical/beacon_lights_on",
	"sim/cockpit/electrical/strobe_lights_on",
	"sim/cockpit/electrical/landing_lights_on",
	"sim/cockpit2/controls/yoke_pitch_ratio",
	"sim/cockpit2/controls/yoke_roll_ratio",
	"sim/cockpit2/controls/yoke_heading_ratio",
	"sim/cockpit2/controls/flap_ratio",
	"sim/cockpit2/controls/speedbrake_ratio",
	"sim/cockpit/switches/gear_handle_status",
	"sim/time/zulu_time_sec",
	"sim/time/local_date_days",
	"sim/time/local_date_days",
	"sim/time/local_date_days",
	"sim/time/local_time_sec",
	"sim/flightmodel/position/g_nrml",
	"sim/flightmodel/engine/ENGN_running[1]",
	"sim/flightmodel/engine/ENGN_N1_[1]",
	"sim/flightmodel/engine/ENGN_N2_[1]",
	"sim/cockpit2/engine/actuators/throttle_ratio[1]",
	"sim/cockpit2/engine/actuators/mixture_ratio[1]",
	"sim/cockpit2/engine/actuators/prop_ratio[1]",
	"sim/flightmodel/engine/ENGN_running[2]",
	"sim/flightmodel/engine/ENGN_N1_[2]",
	"sim/flightmodel/engine/ENGN_N2_[2]",
	"sim/cockpit2/engine/actuators/throttle_ratio[2]",
	"sim/cockpit2/engine/actuators/mixture_ratio[2]",
	"sim/cockpit2/engine/actuators/prop_ratio[2]",
	"sim/flightmodel/engine/ENGN_running[3]",
	"sim/flightmodel/engine/ENGN_N1_[3]",
	"sim/flightmodel/engine/ENGN_N2_[3]",
	"sim/cockpit2/engine/actuators/throttle_ratio[3]",
	"sim/cockpit2/engine/actuators/mixture_ratio[3]",
	"sim/cockpit2/engine/actuators/prop_ratio[3]",
	"sim/flightmodel/weight/m_total",
	"sim/flightmodel/weight/m_fuel_total",
	"sim/aircraft/engine/acf_num_engines",
	"sim/weather/wind_direction_degt[0]",
	"sim/weather/wind_speed_kt[0]",
	"sim/weather/barometer_sealevel_inhg",
	"sim/time/paused",
	"sim/flightmodel2/misc/has_crashed",
}

// Adapter is the X-Plane UDP adapter using the RREF protocol.
type Adapter struct {
	host string
	port int

	mu           sync.Mutex
	conn         *net.UDPConn
	data         domain.FlightData
	lastReceived time.Time
	stop         chan struct{}
}

// NewAdapter creates a new X-Plane adapter.
func NewAdapter(host string, port int) domain.SimConnector {
	return &Adapter{
		host: host,
		port: port,
	}
}

func (x *Adapter) Name() string { return "X-Plane" }

func (x *Adapter) Connect() error {
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

func (x *Adapter) Disconnect() error {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.stop != nil {
		close(x.stop)
		x.stop = nil
	}

	if x.conn != nil {
		for i, dref := range xplaneDatarefs {
			x.subscribeRREF(i, 0, dref)
		}
		x.conn.Close()
		x.conn = nil
	}
	return nil
}

func (x *Adapter) GetFlightData() (*domain.FlightData, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.conn == nil {
		return nil, fmt.Errorf("not connected")
	}
	if x.lastReceived.IsZero() || time.Since(x.lastReceived) > 3*time.Second {
		return nil, fmt.Errorf("no data from simulator")
	}

	data := x.data
	return &data, nil
}

func (x *Adapter) LastReceived() time.Time {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.lastReceived
}

func (x *Adapter) subscribeRREF(index, freq int, dataref string) error {
	buf := make([]byte, 413)
	copy(buf[0:4], "RREF")
	buf[4] = 0
	binary.LittleEndian.PutUint32(buf[5:9], uint32(freq))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(index))
	copy(buf[13:], dataref)
	_, err := x.conn.Write(buf)
	return err
}

func (x *Adapter) listenLoop() {
	buf := make([]byte, 4096)

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

		offset := 5
		for offset+8 <= n {
			idx := int(binary.LittleEndian.Uint32(buf[offset : offset+4]))
			val := math.Float32frombits(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))
			offset += 8

			x.mu.Lock()
			switch idx {
			case 0:
				x.data.Position.Latitude = float64(val)
			case 1:
				x.data.Position.Longitude = float64(val)
			case 2:
				x.data.Position.Altitude = float64(val) * 3.28084
			case 3:
				x.data.Position.AltitudeAGL = float64(val) * 3.28084
			case 4:
				x.data.Attitude.Pitch = float64(val)
			case 5:
				x.data.Attitude.Roll = float64(val)
			case 6:
				x.data.Attitude.HeadingTrue = float64(val)
			case 7:
				x.data.Attitude.HeadingMag = float64(val)
			case 8:
				x.data.Attitude.VS = float64(val)
			case 9:
				x.data.Attitude.IAS = float64(val)
			case 10:
				x.data.Attitude.TAS = float64(val) * 1.94384
			case 11:
				x.data.Attitude.GS = float64(val) * 1.94384
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
			case 18:
				x.data.Sensors.OnGround = val != 0
			case 19:
				x.data.Sensors.StallWarning = val != 0
			case 20:
				x.data.Sensors.OverspeedWarning = val != 0
			case 21:
				x.data.Sensors.SimulationRate = float64(val)
			case 22:
				x.data.Radios.Com1 = float64(val) / 100
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
				x.data.Radios.XpdrState = domain.TransponderStateString(float64(val))
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
			case 37:
				x.data.Altimeter = float64(val)
			case 38:
				x.data.Lights.Beacon = val != 0
			case 39:
				x.data.Lights.Strobe = val != 0
			case 40:
				x.data.Lights.Landing = val != 0
			case 41:
				x.data.Controls.Elevator = float64(val)
			case 42:
				x.data.Controls.Aileron = float64(val)
			case 43:
				x.data.Controls.Rudder = float64(val)
			case 44:
				x.data.Controls.Flaps = float64(val) * 100
			case 45:
				x.data.Controls.Spoilers = float64(val) * 100
			case 46:
				x.data.Controls.GearDown = val != 0
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
			case 52:
				x.data.Attitude.GForce = float64(val)
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
			case 71:
				x.data.Weight.TotalWeight = float64(val) * 2.20462
			case 72:
				x.data.Weight.FuelWeight = float64(val) * 2.20462
			case 73:
				n := int(val)
				x.data.Engines[0].Exists = n >= 1
				x.data.Engines[1].Exists = n >= 2
				x.data.Engines[2].Exists = n >= 3
				x.data.Engines[3].Exists = n >= 4
			case 74:
				x.data.WindDirection = float64(val)
			case 75:
				x.data.WindSpeed = float64(val)
			case 76:
				x.data.QNH = float64(val) * 33.8639
			case 77:
				x.data.Sensors.Paused = val != 0
			case 78:
				x.data.Sensors.Crashed = val != 0
			}
			x.mu.Unlock()
		}

		x.mu.Lock()
		x.lastReceived = time.Now()
		x.mu.Unlock()
	}
}
