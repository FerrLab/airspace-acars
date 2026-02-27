package main

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"

	sim "github.com/lian/msfs2020-go/simconnect"
)

type SimConnectAdapter struct {
	mu         sync.RWMutex
	sc         *sim.SimConnect
	report     *simReport
	latestData *FlightData
	stopCh     chan struct{}
	stopped    chan struct{}
}

type simReport struct {
	sim.RecvSimobjectDataByType

	// Position
	Latitude    float64 `name:"PLANE LATITUDE" unit:"degrees"`
	Longitude   float64 `name:"PLANE LONGITUDE" unit:"degrees"`
	Altitude    float64 `name:"INDICATED ALTITUDE" unit:"feet"`
	AltitudeAGL float64 `name:"PLANE ALT ABOVE GROUND" unit:"feet"`

	// Attitude
	Pitch       float64 `name:"PLANE PITCH DEGREES" unit:"degrees"`
	Roll        float64 `name:"PLANE BANK DEGREES" unit:"degrees"`
	HeadingTrue float64 `name:"PLANE HEADING DEGREES TRUE" unit:"degrees"`
	HeadingMag  float64 `name:"PLANE HEADING DEGREES MAGNETIC" unit:"degrees"`
	VS          float64 `name:"VERTICAL SPEED" unit:"feet per second"`
	IAS         float64 `name:"AIRSPEED INDICATED" unit:"knots"`
	TAS         float64 `name:"AIRSPEED TRUE" unit:"knots"`
	GS          float64 `name:"GROUND VELOCITY" unit:"knots"`

	// Engine 1
	Eng1Running     float64 `name:"GENERAL ENG COMBUSTION:1" unit:"Bool"`
	Eng1N1          float64 `name:"TURB ENG N1:1" unit:"Percent"`
	Eng1N2          float64 `name:"TURB ENG N2:1" unit:"Percent"`
	Eng1Throttle    float64 `name:"GENERAL ENG THROTTLE LEVER POSITION:1" unit:"Percent"`
	Eng1Mixture     float64 `name:"GENERAL ENG MIXTURE LEVER POSITION:1" unit:"Percent"`
	Eng1Prop        float64 `name:"GENERAL ENG PROPELLER LEVER POSITION:1" unit:"Percent"`

	// Engine 2
	Eng2Running     float64 `name:"GENERAL ENG COMBUSTION:2" unit:"Bool"`
	Eng2N1          float64 `name:"TURB ENG N1:2" unit:"Percent"`
	Eng2N2          float64 `name:"TURB ENG N2:2" unit:"Percent"`
	Eng2Throttle    float64 `name:"GENERAL ENG THROTTLE LEVER POSITION:2" unit:"Percent"`
	Eng2Mixture     float64 `name:"GENERAL ENG MIXTURE LEVER POSITION:2" unit:"Percent"`
	Eng2Prop        float64 `name:"GENERAL ENG PROPELLER LEVER POSITION:2" unit:"Percent"`

	// Engine 3
	Eng3Running     float64 `name:"GENERAL ENG COMBUSTION:3" unit:"Bool"`
	Eng3N1          float64 `name:"TURB ENG N1:3" unit:"Percent"`
	Eng3N2          float64 `name:"TURB ENG N2:3" unit:"Percent"`
	Eng3Throttle    float64 `name:"GENERAL ENG THROTTLE LEVER POSITION:3" unit:"Percent"`
	Eng3Mixture     float64 `name:"GENERAL ENG MIXTURE LEVER POSITION:3" unit:"Percent"`
	Eng3Prop        float64 `name:"GENERAL ENG PROPELLER LEVER POSITION:3" unit:"Percent"`

	// Engine 4
	Eng4Running     float64 `name:"GENERAL ENG COMBUSTION:4" unit:"Bool"`
	Eng4N1          float64 `name:"TURB ENG N1:4" unit:"Percent"`
	Eng4N2          float64 `name:"TURB ENG N2:4" unit:"Percent"`
	Eng4Throttle    float64 `name:"GENERAL ENG THROTTLE LEVER POSITION:4" unit:"Percent"`
	Eng4Mixture     float64 `name:"GENERAL ENG MIXTURE LEVER POSITION:4" unit:"Percent"`
	Eng4Prop        float64 `name:"GENERAL ENG PROPELLER LEVER POSITION:4" unit:"Percent"`

	// Sensors
	OnGround         float64 `name:"SIM ON GROUND" unit:"Bool"`
	StallWarning     float64 `name:"STALL WARNING" unit:"Bool"`
	OverspeedWarning float64 `name:"OVERSPEED WARNING" unit:"Bool"`
	SimulationRate   float64 `name:"SIMULATION RATE" unit:"number"`

	// Radios
	Com1      float64 `name:"COM ACTIVE FREQUENCY:1" unit:"MHz"`
	Com2      float64 `name:"COM ACTIVE FREQUENCY:2" unit:"MHz"`
	Nav1      float64 `name:"NAV ACTIVE FREQUENCY:1" unit:"MHz"`
	Nav2      float64 `name:"NAV ACTIVE FREQUENCY:2" unit:"MHz"`
	Nav1OBS   float64 `name:"NAV OBS:1" unit:"degrees"`
	Nav2OBS   float64 `name:"NAV OBS:2" unit:"degrees"`
	XpdrCode  float64 `name:"TRANSPONDER CODE:1" unit:"number"`
	XpdrState float64 `name:"TRANSPONDER STATE:1" unit:"number"`

	// Autopilot
	APMaster       float64 `name:"AUTOPILOT MASTER" unit:"Bool"`
	APHeading      float64 `name:"AUTOPILOT HEADING LOCK DIR" unit:"degrees"`
	APAltitude     float64 `name:"AUTOPILOT ALTITUDE LOCK VAR" unit:"feet"`
	APVS           float64 `name:"AUTOPILOT VERTICAL HOLD VAR" unit:"feet/minute"`
	APSpeed        float64 `name:"AUTOPILOT AIRSPEED HOLD VAR" unit:"knots"`
	APApproachHold float64 `name:"AUTOPILOT APPROACH HOLD" unit:"Bool"`
	APNavLock      float64 `name:"AUTOPILOT NAV1 LOCK" unit:"Bool"`

	// Altimeter
	AltimeterInHg float64 `name:"KOHLSMAN SETTING HG" unit:"inHg"`

	// Lights
	LightBeacon  float64 `name:"LIGHT BEACON" unit:"Bool"`
	LightStrobe  float64 `name:"LIGHT STROBE" unit:"Bool"`
	LightLanding float64 `name:"LIGHT LANDING" unit:"Bool"`

	// Controls
	Elevator float64 `name:"ELEVATOR POSITION" unit:"Position"`
	Aileron  float64 `name:"AILERON POSITION" unit:"Position"`
	Rudder   float64 `name:"RUDDER POSITION" unit:"Position"`
	Flaps    float64 `name:"FLAPS HANDLE PERCENT" unit:"Percent Over 100"`
	Spoilers float64 `name:"SPOILERS HANDLE POSITION" unit:"Percent Over 100"`
	GearDown float64 `name:"GEAR HANDLE POSITION" unit:"Bool"`

	// SimTime
	ZuluTime  float64 `name:"ZULU TIME" unit:"seconds"`
	ZuluDay   float64 `name:"ZULU DAY OF MONTH" unit:"number"`
	ZuluMonth float64 `name:"ZULU MONTH OF YEAR" unit:"number"`
	ZuluYear  float64 `name:"ZULU YEAR" unit:"number"`
	LocalTime float64 `name:"LOCAL TIME" unit:"seconds"`

	// APU
	APUSwitch    float64 `name:"APU SWITCH" unit:"Bool"`
	APURPMPct    float64 `name:"APU PCT RPM" unit:"Percent"`
	APUGenSwitch float64 `name:"APU GENERATOR SWITCH" unit:"Bool"`
	APUGenActive float64 `name:"APU GENERATOR ACTIVE" unit:"Bool"`

	// Doors
	Door0Open float64 `name:"EXIT OPEN:0" unit:"Percent Over 100"`
	Door1Open float64 `name:"EXIT OPEN:1" unit:"Percent Over 100"`
	Door2Open float64 `name:"EXIT OPEN:2" unit:"Percent Over 100"`
	Door3Open float64 `name:"EXIT OPEN:3" unit:"Percent Over 100"`
	Door4Open float64 `name:"EXIT OPEN:4" unit:"Percent Over 100"`

	// G-Force
	GForce float64 `name:"G FORCE" unit:"GForce"`

	// Weight
	TotalWeight float64 `name:"TOTAL WEIGHT" unit:"pounds"`
	FuelWeight  float64 `name:"FUEL TOTAL QUANTITY WEIGHT" unit:"pounds"`

	// Engine count
	NumberOfEngines float64 `name:"NUMBER OF ENGINES" unit:"number"`

	// Aircraft Title — must be last (256-byte array misaligns subsequent float64s)
	AircraftTitle [256]byte `name:"TITLE" unit:""`
}

func NewSimConnectAdapter() SimConnector {
	return &SimConnectAdapter{}
}

func (s *SimConnectAdapter) Name() string {
	return "SimConnect"
}

func (s *SimConnectAdapter) Connect() error {
	s.stopCh = make(chan struct{})
	s.stopped = make(chan struct{})
	errCh := make(chan error, 1)

	go s.run(errCh)

	return <-errCh
}

func (s *SimConnectAdapter) Disconnect() error {
	s.mu.RLock()
	sc := s.sc
	s.mu.RUnlock()

	if sc != nil {
		close(s.stopCh)
		<-s.stopped
	}
	return nil
}

// run performs ALL SimConnect operations on a single locked OS thread.
func (s *SimConnectAdapter) run(errCh chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.stopped)

	sc, err := sim.New("Airspace ACARS")
	if err != nil {
		errCh <- fmt.Errorf("simconnect open: %w", err)
		return
	}

	report := &simReport{}
	if err := sc.RegisterDataDefinition(report); err != nil {
		sc.Close()
		errCh <- fmt.Errorf("register data definition: %w", err)
		return
	}

	s.mu.Lock()
	s.sc = sc
	s.report = report
	s.mu.Unlock()

	slog.Info("SimConnect connected")
	errCh <- nil // signal success to Connect()

	defineID := sc.GetDefineID(report)

	requestTicker := time.NewTicker(time.Second)
	defer requestTicker.Stop()

	// Initial data request
	sc.RequestDataOnSimObjectType(0, defineID, 0, sim.SIMOBJECT_TYPE_USER)

	defer func() {
		sc.Close()
		s.mu.Lock()
		s.sc = nil
		s.latestData = nil
		s.mu.Unlock()
	}()

	for {
		select {
		case <-s.stopCh:
			return
		case <-requestTicker.C:
			sc.RequestDataOnSimObjectType(0, defineID, 0, sim.SIMOBJECT_TYPE_USER)
		default:
			ppData, r1, _ := sc.GetNextDispatch()
			if r1 < 0 {
				time.Sleep(5 * time.Millisecond)
				continue
			}

			recvInfo := *(*sim.Recv)(ppData)

			switch recvInfo.ID {
			case sim.RECV_ID_SIMOBJECT_DATA_BYTYPE:
				r := (*simReport)(ppData)
				fd := &FlightData{
					Position: PositionData{
						Latitude:    r.Latitude,
						Longitude:   r.Longitude,
						Altitude:    r.Altitude,
						AltitudeAGL: r.AltitudeAGL,
					},
					Attitude: AttitudeData{
						Pitch:       r.Pitch,
						Roll:        r.Roll,
						HeadingTrue: r.HeadingTrue,
						HeadingMag:  r.HeadingMag,
						VS:          r.VS * 60, // fps to fpm
						IAS:         r.IAS,
						TAS:         r.TAS,
						GS:          r.GS,
						GForce:      r.GForce,
					},
					Engines: [4]EngineData{
						{
							Exists:      int(r.NumberOfEngines) >= 1,
							Running:     r.Eng1Running != 0,
							N1:          r.Eng1N1,
							N2:          r.Eng1N2,
							ThrottlePos: r.Eng1Throttle,
							MixturePos:  r.Eng1Mixture,
							PropPos:     r.Eng1Prop,
						},
						{
							Exists:      int(r.NumberOfEngines) >= 2,
							Running:     r.Eng2Running != 0,
							N1:          r.Eng2N1,
							N2:          r.Eng2N2,
							ThrottlePos: r.Eng2Throttle,
							MixturePos:  r.Eng2Mixture,
							PropPos:     r.Eng2Prop,
						},
						{
							Exists:      int(r.NumberOfEngines) >= 3,
							Running:     r.Eng3Running != 0,
							N1:          r.Eng3N1,
							N2:          r.Eng3N2,
							ThrottlePos: r.Eng3Throttle,
							MixturePos:  r.Eng3Mixture,
							PropPos:     r.Eng3Prop,
						},
						{
							Exists:      int(r.NumberOfEngines) >= 4,
							Running:     r.Eng4Running != 0,
							N1:          r.Eng4N1,
							N2:          r.Eng4N2,
							ThrottlePos: r.Eng4Throttle,
							MixturePos:  r.Eng4Mixture,
							PropPos:     r.Eng4Prop,
						},
					},
					Sensors: SensorData{
						OnGround:         r.OnGround != 0,
						StallWarning:     r.StallWarning != 0,
						OverspeedWarning: r.OverspeedWarning != 0,
						SimulationRate:   r.SimulationRate,
					},
					Radios: RadioData{
						Com1:      r.Com1,
						Com2:      r.Com2,
						Nav1:      r.Nav1,
						Nav2:      r.Nav2,
						Nav1OBS:   r.Nav1OBS,
						Nav2OBS:   r.Nav2OBS,
						XpdrCode:  r.XpdrCode,
						XpdrState: TransponderStateString(r.XpdrState),
					},
					Autopilot: AutopilotData{
						Master:       r.APMaster != 0,
						Heading:      r.APHeading,
						Altitude:     r.APAltitude,
						VS:           r.APVS,
						Speed:        r.APSpeed,
						ApproachHold: r.APApproachHold != 0,
						NavLock:      r.APNavLock != 0,
					},
					Altimeter: r.AltimeterInHg,
					Lights: LightData{
						Beacon:  r.LightBeacon != 0,
						Strobe:  r.LightStrobe != 0,
						Landing: r.LightLanding != 0,
					},
					Controls: FlightControlData{
						Elevator: r.Elevator,
						Aileron:  r.Aileron,
						Rudder:   r.Rudder,
						Flaps:    r.Flaps * 100,    // Percent Over 100 → percent
						Spoilers: r.Spoilers * 100, // Percent Over 100 → percent
						GearDown: r.GearDown != 0,
					},
					SimTime: SimTimeData{
						ZuluTime:  r.ZuluTime,
						ZuluDay:   r.ZuluDay,
						ZuluMonth: r.ZuluMonth,
						ZuluYear:  r.ZuluYear,
						LocalTime: r.LocalTime,
					},
					APU: APUData{
						SwitchOn:   r.APUSwitch != 0,
						RPMPercent: r.APURPMPct,
						GenSwitch:  r.APUGenSwitch != 0,
						GenActive:  r.APUGenActive != 0,
					},
					Doors: [5]DoorData{
						{OpenRatio: r.Door0Open},
						{OpenRatio: r.Door1Open},
						{OpenRatio: r.Door2Open},
						{OpenRatio: r.Door3Open},
						{OpenRatio: r.Door4Open},
					},
					AircraftName: trimNullBytes(r.AircraftTitle[:]),
					Weight: WeightData{
						TotalWeight: r.TotalWeight,
						FuelWeight:  r.FuelWeight,
					},
				}
				s.mu.Lock()
				s.latestData = fd
				s.mu.Unlock()
			case sim.RECV_ID_EXCEPTION:
				slog.Warn("SimConnect exception received")
			}
		}
	}
}

// trimNullBytes returns a string from a null-padded byte slice.
func trimNullBytes(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// GetFlightData returns the most recently cached flight data.
func (s *SimConnectAdapter) GetFlightData() (*FlightData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.latestData == nil {
		return nil, fmt.Errorf("waiting for sim data")
	}

	data := *s.latestData
	_ = unsafe.Pointer(nil)
	return &data, nil
}
