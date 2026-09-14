package profiles

import (
	"fmt"
	"sort"

	"airspace-acars/internal/domain"
)

// PointKind is the value type a data point accepts.
type PointKind int

const (
	// KindFloat stores the transformed number as-is.
	KindFloat PointKind = iota
	// KindBool stores true when the transformed number is non-zero.
	KindBool
	// KindString stores text and can only be fed by a const source.
	KindString
)

func (k PointKind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindString:
		return "string"
	default:
		return "float"
	}
}

// Value is a transformed reading on its way into a FlightData field.
type Value struct {
	Num      float64
	Str      string
	IsString bool
}

// Bool reports the value as a boolean (non-zero is true).
func (v Value) Bool() bool { return v.Num != 0 }

// Point is one addressable field of domain.FlightData.
type Point struct {
	ID   string
	Kind PointKind
	Desc string
	Set  func(fd *domain.FlightData, v Value)
}

var catalog = map[string]Point{}

func register(id string, kind PointKind, desc string, set func(fd *domain.FlightData, v Value)) {
	if _, dup := catalog[id]; dup {
		panic(fmt.Sprintf("profiles: duplicate data point %q", id))
	}
	catalog[id] = Point{ID: id, Kind: kind, Desc: desc, Set: set}
}

func registerFloat(id, desc string, set func(fd *domain.FlightData, n float64)) {
	register(id, KindFloat, desc, func(fd *domain.FlightData, v Value) { set(fd, v.Num) })
}

func registerBool(id, desc string, set func(fd *domain.FlightData, b bool)) {
	register(id, KindBool, desc, func(fd *domain.FlightData, v Value) { set(fd, v.Bool()) })
}

func registerString(id, desc string, set func(fd *domain.FlightData, s string)) {
	register(id, KindString, desc, func(fd *domain.FlightData, v Value) { set(fd, v.Str) })
}

// Lookup returns the catalog entry for a data point ID.
func Lookup(id string) (Point, bool) {
	p, ok := catalog[id]
	return p, ok
}

// Points returns every data point a profile may mash, sorted by ID.
func Points() []Point {
	out := make([]Point, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func init() {
	registerFloat("position.latitude", "Latitude in degrees", func(fd *domain.FlightData, n float64) { fd.Position.Latitude = n })
	registerFloat("position.longitude", "Longitude in degrees", func(fd *domain.FlightData, n float64) { fd.Position.Longitude = n })
	registerFloat("position.altitude", "Indicated altitude in feet", func(fd *domain.FlightData, n float64) { fd.Position.Altitude = n })
	registerFloat("position.altitudeAGL", "Height above ground in feet", func(fd *domain.FlightData, n float64) { fd.Position.AltitudeAGL = n })

	registerFloat("attitude.pitch", "Pitch in degrees", func(fd *domain.FlightData, n float64) { fd.Attitude.Pitch = n })
	registerFloat("attitude.roll", "Bank in degrees", func(fd *domain.FlightData, n float64) { fd.Attitude.Roll = n })
	registerFloat("attitude.headingTrue", "True heading in degrees", func(fd *domain.FlightData, n float64) { fd.Attitude.HeadingTrue = n })
	registerFloat("attitude.headingMag", "Magnetic heading in degrees", func(fd *domain.FlightData, n float64) { fd.Attitude.HeadingMag = n })
	registerFloat("attitude.vs", "Vertical speed in feet per minute", func(fd *domain.FlightData, n float64) { fd.Attitude.VS = n })
	registerFloat("attitude.ias", "Indicated airspeed in knots", func(fd *domain.FlightData, n float64) { fd.Attitude.IAS = n })
	registerFloat("attitude.tas", "True airspeed in knots", func(fd *domain.FlightData, n float64) { fd.Attitude.TAS = n })
	registerFloat("attitude.gs", "Ground speed in knots", func(fd *domain.FlightData, n float64) { fd.Attitude.GS = n })
	registerFloat("attitude.gForce", "Normal load factor in G", func(fd *domain.FlightData, n float64) { fd.Attitude.GForce = n })

	for i := 0; i < len(domain.FlightData{}.Engines); i++ {
		idx := i
		num := i + 1
		registerBool(fmt.Sprintf("engines.%d.exists", num), fmt.Sprintf("Engine %d is fitted", num),
			func(fd *domain.FlightData, b bool) { fd.Engines[idx].Exists = b })
		registerBool(fmt.Sprintf("engines.%d.running", num), fmt.Sprintf("Engine %d combustion", num),
			func(fd *domain.FlightData, b bool) { fd.Engines[idx].Running = b })
		registerFloat(fmt.Sprintf("engines.%d.n1", num), fmt.Sprintf("Engine %d N1 percent", num),
			func(fd *domain.FlightData, n float64) { fd.Engines[idx].N1 = n })
		registerFloat(fmt.Sprintf("engines.%d.n2", num), fmt.Sprintf("Engine %d N2 percent", num),
			func(fd *domain.FlightData, n float64) { fd.Engines[idx].N2 = n })
		registerFloat(fmt.Sprintf("engines.%d.throttle", num), fmt.Sprintf("Engine %d throttle lever percent", num),
			func(fd *domain.FlightData, n float64) { fd.Engines[idx].ThrottlePos = n })
		registerFloat(fmt.Sprintf("engines.%d.mixture", num), fmt.Sprintf("Engine %d mixture lever percent", num),
			func(fd *domain.FlightData, n float64) { fd.Engines[idx].MixturePos = n })
		registerFloat(fmt.Sprintf("engines.%d.prop", num), fmt.Sprintf("Engine %d propeller lever percent", num),
			func(fd *domain.FlightData, n float64) { fd.Engines[idx].PropPos = n })
	}
	registerFloat("engines.count", "Number of engines fitted (sets every engine's exists flag)",
		func(fd *domain.FlightData, n float64) {
			count := int(n)
			for i := range fd.Engines {
				fd.Engines[i].Exists = count >= i+1
			}
		})

	registerBool("sensors.onGround", "Aircraft is on the ground", func(fd *domain.FlightData, b bool) { fd.Sensors.OnGround = b })
	registerBool("sensors.stallWarning", "Stall warning active", func(fd *domain.FlightData, b bool) { fd.Sensors.StallWarning = b })
	registerBool("sensors.overspeedWarning", "Overspeed warning active", func(fd *domain.FlightData, b bool) { fd.Sensors.OverspeedWarning = b })
	registerBool("sensors.paused", "Simulation paused", func(fd *domain.FlightData, b bool) { fd.Sensors.Paused = b })
	registerBool("sensors.slew", "Slew mode active", func(fd *domain.FlightData, b bool) { fd.Sensors.Slew = b })
	registerBool("sensors.crashed", "Crash flag set", func(fd *domain.FlightData, b bool) { fd.Sensors.Crashed = b })
	registerFloat("sensors.simulationRate", "Simulation rate multiplier", func(fd *domain.FlightData, n float64) { fd.Sensors.SimulationRate = n })

	registerFloat("radios.com1", "COM1 active frequency in MHz", func(fd *domain.FlightData, n float64) { fd.Radios.Com1 = n })
	registerFloat("radios.com2", "COM2 active frequency in MHz", func(fd *domain.FlightData, n float64) { fd.Radios.Com2 = n })
	registerFloat("radios.nav1", "NAV1 active frequency in MHz", func(fd *domain.FlightData, n float64) { fd.Radios.Nav1 = n })
	registerFloat("radios.nav2", "NAV2 active frequency in MHz", func(fd *domain.FlightData, n float64) { fd.Radios.Nav2 = n })
	registerFloat("radios.nav1OBS", "NAV1 OBS course in degrees", func(fd *domain.FlightData, n float64) { fd.Radios.Nav1OBS = n })
	registerFloat("radios.nav2OBS", "NAV2 OBS course in degrees", func(fd *domain.FlightData, n float64) { fd.Radios.Nav2OBS = n })
	registerFloat("radios.xpdrCode", "Transponder code", func(fd *domain.FlightData, n float64) { fd.Radios.XpdrCode = n })
	registerFloat("radios.xpdrState", "Transponder mode (0 off, 1 standby, 2+ active)",
		func(fd *domain.FlightData, n float64) { fd.Radios.XpdrState = domain.TransponderStateString(n) })

	registerBool("autopilot.master", "Autopilot engaged", func(fd *domain.FlightData, b bool) { fd.Autopilot.Master = b })
	registerFloat("autopilot.heading", "Selected heading in degrees", func(fd *domain.FlightData, n float64) { fd.Autopilot.Heading = n })
	registerFloat("autopilot.altitude", "Selected altitude in feet", func(fd *domain.FlightData, n float64) { fd.Autopilot.Altitude = n })
	registerFloat("autopilot.vs", "Selected vertical speed in feet per minute", func(fd *domain.FlightData, n float64) { fd.Autopilot.VS = n })
	registerFloat("autopilot.speed", "Selected airspeed in knots", func(fd *domain.FlightData, n float64) { fd.Autopilot.Speed = n })
	registerBool("autopilot.approachHold", "Approach mode armed or engaged", func(fd *domain.FlightData, b bool) { fd.Autopilot.ApproachHold = b })
	registerBool("autopilot.navLock", "Lateral navigation engaged", func(fd *domain.FlightData, b bool) { fd.Autopilot.NavLock = b })

	registerFloat("altimeter", "Altimeter setting in inHg", func(fd *domain.FlightData, n float64) { fd.Altimeter = n })
	registerFloat("qnh", "Sea level pressure in millibars", func(fd *domain.FlightData, n float64) { fd.QNH = n })
	registerFloat("wind.direction", "Ambient wind direction in degrees", func(fd *domain.FlightData, n float64) { fd.WindDirection = n })
	registerFloat("wind.speed", "Ambient wind speed in knots", func(fd *domain.FlightData, n float64) { fd.WindSpeed = n })

	registerBool("lights.beacon", "Beacon light on", func(fd *domain.FlightData, b bool) { fd.Lights.Beacon = b })
	registerBool("lights.strobe", "Strobe lights on", func(fd *domain.FlightData, b bool) { fd.Lights.Strobe = b })
	registerBool("lights.landing", "Landing lights on", func(fd *domain.FlightData, b bool) { fd.Lights.Landing = b })

	registerFloat("controls.elevator", "Elevator position", func(fd *domain.FlightData, n float64) { fd.Controls.Elevator = n })
	registerFloat("controls.aileron", "Aileron position", func(fd *domain.FlightData, n float64) { fd.Controls.Aileron = n })
	registerFloat("controls.rudder", "Rudder position", func(fd *domain.FlightData, n float64) { fd.Controls.Rudder = n })
	registerFloat("controls.flaps", "Flap handle percent", func(fd *domain.FlightData, n float64) { fd.Controls.Flaps = n })
	registerFloat("controls.spoilers", "Spoiler handle percent", func(fd *domain.FlightData, n float64) { fd.Controls.Spoilers = n })
	registerBool("controls.gearDown", "Gear handle down", func(fd *domain.FlightData, b bool) { fd.Controls.GearDown = b })

	registerFloat("simTime.zuluTime", "Zulu time in seconds", func(fd *domain.FlightData, n float64) { fd.SimTime.ZuluTime = n })
	registerFloat("simTime.zuluDay", "Zulu day of month", func(fd *domain.FlightData, n float64) { fd.SimTime.ZuluDay = n })
	registerFloat("simTime.zuluMonth", "Zulu month of year", func(fd *domain.FlightData, n float64) { fd.SimTime.ZuluMonth = n })
	registerFloat("simTime.zuluYear", "Zulu year", func(fd *domain.FlightData, n float64) { fd.SimTime.ZuluYear = n })
	registerFloat("simTime.localTime", "Local time in seconds", func(fd *domain.FlightData, n float64) { fd.SimTime.LocalTime = n })

	registerBool("apu.switchOn", "APU master switch on", func(fd *domain.FlightData, b bool) { fd.APU.SwitchOn = b })
	registerFloat("apu.rpmPercent", "APU RPM percent", func(fd *domain.FlightData, n float64) { fd.APU.RPMPercent = n })
	registerBool("apu.genSwitch", "APU generator switch on", func(fd *domain.FlightData, b bool) { fd.APU.GenSwitch = b })
	registerBool("apu.genActive", "APU generator supplying power", func(fd *domain.FlightData, b bool) { fd.APU.GenActive = b })

	for i := 0; i < len(domain.FlightData{}.Doors); i++ {
		idx := i
		registerFloat(fmt.Sprintf("doors.%d.openRatio", i+1), fmt.Sprintf("Door %d open ratio (0-1)", i+1),
			func(fd *domain.FlightData, n float64) { fd.Doors[idx].OpenRatio = n })
	}

	registerFloat("weight.total", "Total weight in pounds", func(fd *domain.FlightData, n float64) { fd.Weight.TotalWeight = n })
	registerFloat("weight.fuel", "Fuel weight in pounds", func(fd *domain.FlightData, n float64) { fd.Weight.FuelWeight = n })

	registerString("aircraft.name", "Aircraft title reported to the network", func(fd *domain.FlightData, s string) { fd.AircraftName = s })
	registerString("aircraft.type", "ICAO type reported to the network", func(fd *domain.FlightData, s string) { fd.AircraftType = s })
}
