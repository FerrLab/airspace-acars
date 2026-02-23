package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unsafe"

	sim "github.com/lian/msfs2020-go/simconnect"
)

type SimConnectAdapter struct {
	mu     sync.Mutex
	sc     *sim.SimConnect
	report *simReport
}

type simReport struct {
	sim.RecvSimobjectDataByType
	Altitude      float64 `name:"PLANE ALTITUDE" unit:"feet"`
	Heading       float64 `name:"PLANE HEADING DEGREES TRUE" unit:"degrees"`
	Pitch         float64 `name:"PLANE PITCH DEGREES" unit:"degrees"`
	Roll          float64 `name:"PLANE BANK DEGREES" unit:"degrees"`
	Airspeed      float64 `name:"AIRSPEED INDICATED" unit:"knots"`
	GroundSpeed   float64 `name:"GROUND VELOCITY" unit:"knots"`
	VerticalSpeed float64 `name:"VERTICAL SPEED" unit:"feet per second"`
}

func NewSimConnectAdapter() SimConnector {
	return &SimConnectAdapter{}
}

func (s *SimConnectAdapter) Name() string {
	return "SimConnect"
}

func (s *SimConnectAdapter) Connect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, err := sim.New("Airspace ACARS")
	if err != nil {
		return fmt.Errorf("simconnect open: %w", err)
	}
	s.sc = sc

	s.report = &simReport{}
	err = s.sc.RegisterDataDefinition(s.report)
	if err != nil {
		s.sc.Close()
		s.sc = nil
		return fmt.Errorf("register data definition: %w", err)
	}

	slog.Info("SimConnect connected")
	return nil
}

func (s *SimConnectAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sc != nil {
		s.sc.Close()
		s.sc = nil
	}
	return nil
}

func (s *SimConnectAdapter) GetFlightData() (*FlightData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sc == nil {
		return nil, fmt.Errorf("not connected")
	}

	defineID := s.sc.GetDefineID(s.report)
	s.sc.RequestDataOnSimObjectType(0, defineID, 0, sim.SIMOBJECT_TYPE_USER)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		ppData, r1, err := s.sc.GetNextDispatch()
		if r1 < 0 || err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		recvInfo := *(*sim.Recv)(ppData)
		switch recvInfo.ID {
		case sim.RECV_ID_SIMOBJECT_DATA_BYTYPE:
			report := (*simReport)(ppData)
			fd := &FlightData{
				Altitude:      report.Altitude,
				Heading:       report.Heading,
				Pitch:         report.Pitch,
				Roll:          report.Roll,
				Airspeed:      report.Airspeed,
				GroundSpeed:   report.GroundSpeed,
				VerticalSpeed: report.VerticalSpeed * 60, // fps to fpm
			}
			return fd, nil
		}
	}

	_ = unsafe.Pointer(nil)
	return nil, fmt.Errorf("timeout waiting for SimConnect data")
}
