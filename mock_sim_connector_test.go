package main

import "fmt"

// MockSimConnector implements SimConnector for use in tests.
type MockSimConnector struct {
	data *FlightData
	err  error
	name string
}

func (m *MockSimConnector) Connect() error        { return nil }
func (m *MockSimConnector) Disconnect() error      { return nil }
func (m *MockSimConnector) Name() string           { return m.name }
func (m *MockSimConnector) GetFlightData() (*FlightData, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.data == nil {
		return nil, fmt.Errorf("no data")
	}
	return m.data, nil
}

// sampleFlightData returns a FlightData with realistic default values.
func sampleFlightData() *FlightData {
	return &FlightData{
		Position: PositionData{
			Latitude:    51.4775,
			Longitude:   -0.4614,
			Altitude:    83.0,
			AltitudeAGL: 0.0,
		},
		Attitude: AttitudeData{
			Pitch:       -1.2,
			Roll:        0.3,
			HeadingTrue: 270.0,
			HeadingMag:  269.0,
			VS:          0,
			IAS:         0,
			TAS:         0,
			GS:          0,
			GForce:      1.0,
		},
		Engines: [4]EngineData{
			{Exists: true, Running: true, N1: 22.5, N2: 60.0, ThrottlePos: 0, MixturePos: 100, PropPos: 0},
			{Exists: true, Running: true, N1: 22.5, N2: 60.0, ThrottlePos: 0, MixturePos: 100, PropPos: 0},
		},
		Sensors: SensorData{
			OnGround:       true,
			SimulationRate: 1.0,
		},
		Radios: RadioData{
			Com1:      118.3,
			Com2:      121.5,
			Nav1:      110.1,
			Nav2:      113.0,
			XpdrCode:  1200,
			XpdrState: "stand-by",
		},
		Autopilot: AutopilotData{
			Heading:  270,
			Altitude: 5000,
		},
		Altimeter:    29.92,
		AircraftName: "Boeing 737-800",
		Weight: WeightData{
			TotalWeight: 130000,
			FuelWeight:  40000,
		},
		SimTime: SimTimeData{
			ZuluTime:  43200, // 12:00:00
			ZuluDay:   15,
			ZuluMonth: 6,
			ZuluYear:  2025,
			LocalTime: 46800,
		},
	}
}
