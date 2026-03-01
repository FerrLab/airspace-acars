package main

import "time"

// FlightData holds real-time telemetry from the flight simulator.
type FlightData struct {
	Position  PositionData      `json:"position"`
	Attitude  AttitudeData      `json:"attitude"`
	Engines   [4]EngineData     `json:"engines"`
	Sensors   SensorData        `json:"sensors"`
	Radios    RadioData         `json:"radios"`
	Autopilot AutopilotData     `json:"autopilot"`
	Altimeter float64           `json:"altimeterInHg"`
	Lights    LightData         `json:"lights"`
	Controls  FlightControlData `json:"controls"`
	SimTime      SimTimeData       `json:"simTime"`
	APU          APUData           `json:"apu"`
	Doors        [5]DoorData       `json:"doors"`
	AircraftName string            `json:"aircraftName"`
	Weight       WeightData        `json:"weight"`
}

type PositionData struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Altitude    float64 `json:"altitude"`
	AltitudeAGL float64 `json:"altitudeAGL"`
}

type AttitudeData struct {
	Pitch      float64 `json:"pitch"`
	Roll       float64 `json:"roll"`
	HeadingTrue float64 `json:"headingTrue"`
	HeadingMag float64 `json:"headingMag"`
	VS         float64 `json:"vs"`
	IAS        float64 `json:"ias"`
	TAS        float64 `json:"tas"`
	GS         float64 `json:"gs"`
	GForce     float64 `json:"gForce"`
}

type EngineData struct {
	Exists      bool    `json:"exists"`
	Running     bool    `json:"running"`
	N1          float64 `json:"n1"`
	N2          float64 `json:"n2"`
	ThrottlePos float64 `json:"throttlePos"`
	MixturePos  float64 `json:"mixturePos"`
	PropPos     float64 `json:"propPos"`
}

type SensorData struct {
	OnGround         bool    `json:"onGround"`
	StallWarning     bool    `json:"stallWarning"`
	OverspeedWarning bool    `json:"overspeedWarning"`
	SimulationRate   float64 `json:"simulationRate"`
}

type RadioData struct {
	Com1      float64 `json:"com1"`
	Com2      float64 `json:"com2"`
	Nav1      float64 `json:"nav1"`
	Nav2      float64 `json:"nav2"`
	Nav1OBS   float64 `json:"nav1OBS"`
	Nav2OBS   float64 `json:"nav2OBS"`
	XpdrCode  float64 `json:"xpdrCode"`
	XpdrState string  `json:"xpdrState"`
}

type AutopilotData struct {
	Master       bool    `json:"master"`
	Heading      float64 `json:"heading"`
	Altitude     float64 `json:"altitude"`
	VS           float64 `json:"vs"`
	Speed        float64 `json:"speed"`
	ApproachHold bool    `json:"approachHold"`
	NavLock      bool    `json:"navLock"`
}

type LightData struct {
	Beacon  bool `json:"beacon"`
	Strobe  bool `json:"strobe"`
	Landing bool `json:"landing"`
}

type FlightControlData struct {
	Elevator float64 `json:"elevator"`
	Aileron  float64 `json:"aileron"`
	Rudder   float64 `json:"rudder"`
	Flaps    float64 `json:"flaps"`
	Spoilers float64 `json:"spoilers"`
	GearDown bool    `json:"gearDown"`
}

type SimTimeData struct {
	ZuluTime  float64 `json:"zuluTime"`
	ZuluDay   float64 `json:"zuluDay"`
	ZuluMonth float64 `json:"zuluMonth"`
	ZuluYear  float64 `json:"zuluYear"`
	LocalTime float64 `json:"localTime"`
}

type APUData struct {
	SwitchOn   bool    `json:"switchOn"`
	RPMPercent float64 `json:"rpmPercent"`
	GenSwitch  bool    `json:"genSwitch"`
	GenActive  bool    `json:"genActive"`
}

type DoorData struct {
	OpenRatio float64 `json:"openRatio"` // 0.0=closed, 1.0=open
}

type WeightData struct {
	TotalWeight float64 `json:"totalWeight"` // lbs
	FuelWeight  float64 `json:"fuelWeight"`  // lbs
}

// TransponderStateString maps a numeric transponder mode to a human-readable string.
// SimConnect: 0=Off, 1=Standby, ≥2=Active
// X-Plane:   0=Off, 1=Standby, ≥2=Active
func TransponderStateString(val float64) string {
	switch int(val) {
	case 0:
		return "off"
	case 1:
		return "stand-by"
	default:
		return "active"
	}
}

// SimConnector abstracts simulator connections (SimConnect, X-Plane UDP).
type SimConnector interface {
	Connect() error
	Disconnect() error
	GetFlightData() (*FlightData, error)
	Name() string
	LastReceived() time.Time
}
