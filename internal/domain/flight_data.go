package domain

import "time"

// FlightData holds real-time telemetry from the flight simulator.
type FlightData struct {
	Position      PositionData      `json:"position"`
	Attitude      AttitudeData      `json:"attitude"`
	Engines       [4]EngineData     `json:"engines"`
	Sensors       SensorData        `json:"sensors"`
	Radios        RadioData         `json:"radios"`
	Autopilot     AutopilotData     `json:"autopilot"`
	Altimeter     float64           `json:"altimeterInHg"`
	Lights        LightData         `json:"lights"`
	Controls      FlightControlData `json:"controls"`
	SimTime       SimTimeData       `json:"simTime"`
	APU           APUData           `json:"apu"`
	Doors         [5]DoorData       `json:"doors"`
	AircraftName  string            `json:"aircraftName"`
	AircraftType  string            `json:"aircraftType"`
	Weight        WeightData        `json:"weight"`
	WindDirection float64           `json:"windDirection"`
	WindSpeed     float64           `json:"windSpeed"`
	QNH           float64           `json:"qnh"`
	Cabin         CabinData         `json:"cabin"`
}

type PositionData struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Altitude    float64 `json:"altitude"`
	AltitudeAGL float64 `json:"altitudeAGL"`
}

type AttitudeData struct {
	Pitch       float64 `json:"pitch"`
	Roll        float64 `json:"roll"`
	HeadingTrue float64 `json:"headingTrue"`
	HeadingMag  float64 `json:"headingMag"`
	VS          float64 `json:"vs"`
	IAS         float64 `json:"ias"`
	TAS         float64 `json:"tas"`
	GS          float64 `json:"gs"`
	GForce      float64 `json:"gForce"`
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
	Paused           bool    `json:"paused"`
	Slew             bool    `json:"slew"`
	Crashed          bool    `json:"crashed"`
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
	OpenRatio float64 `json:"openRatio"`
}

// SeatbeltAutoBelowFt is the altitude under which a seatbelt switch in AUTO
// lights the sign. Boeing's AUTO logic keys on flaps, gear and cabin
// altitude, none of which every aircraft exposes; 10,000 ft MSL is the rule
// the SLCSeatbelts plugin uses for X-Plane and lands on the same sign in an
// ordinary climb and descent.
const SeatbeltAutoBelowFt = 10_000.0

// CabinData is what the cabin shows the passengers.
type CabinData struct {
	// SeatbeltSign is the fasten seatbelt sign as the aircraft drives it.
	SeatbeltSign bool `json:"seatbeltSign"`
	// SeatbeltKnown is set once anything has read the sign: an aircraft
	// whose sign nobody reads leaves SeatbeltSign false without meaning
	// "off", and the flight attendant must not announce that.
	SeatbeltKnown bool `json:"seatbeltKnown"`
}

// SeatbeltFromSwitch resolves a three-position seatbelt switch (0 off,
// 1 auto, 2 on) to whether the sign is lit at altitudeFt MSL. AUTO is lit
// below SeatbeltAutoBelowFt. A value between positions rounds to the
// nearest one.
func SeatbeltFromSwitch(position, altitudeFt float64) bool {
	switch {
	case position >= 1.5:
		return true
	case position >= 0.5:
		return altitudeFt < SeatbeltAutoBelowFt
	default:
		return false
	}
}

type WeightData struct {
	TotalWeight float64 `json:"totalWeight"`
	FuelWeight  float64 `json:"fuelWeight"`
}

// TransponderStateString maps a numeric transponder mode to a human-readable string.
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
