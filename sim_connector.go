package main

// FlightData holds real-time telemetry from the flight simulator.
type FlightData struct {
	Altitude      float64 `json:"altitude"`
	Heading       float64 `json:"heading"`
	Pitch         float64 `json:"pitch"`
	Roll          float64 `json:"roll"`
	Airspeed      float64 `json:"airspeed"`
	GroundSpeed   float64 `json:"groundSpeed"`
	VerticalSpeed float64 `json:"verticalSpeed"`
}

// SimConnector abstracts simulator connections (SimConnect, X-Plane UDP).
type SimConnector interface {
	Connect() error
	Disconnect() error
	GetFlightData() (*FlightData, error)
	Name() string
}
