import { useState, useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { FlightDataService } from "../../bindings/airspace-acars";

export interface PositionData {
  latitude: number;
  longitude: number;
  altitude: number;
  altitudeAGL: number;
}

export interface AttitudeData {
  pitch: number;
  roll: number;
  headingTrue: number;
  headingMag: number;
  vs: number;
  ias: number;
  tas: number;
  gs: number;
  gForce: number;
}

export interface EngineData {
  running: boolean;
  n1: number;
  n2: number;
  throttlePos: number;
  mixturePos: number;
  propPos: number;
}

export interface SensorData {
  onGround: boolean;
  stallWarning: boolean;
  overspeedWarning: boolean;
  simulationRate: number;
}

export interface RadioData {
  com1: number;
  com2: number;
  nav1: number;
  nav2: number;
  nav1OBS: number;
  nav2OBS: number;
  xpdrCode: number;
  xpdrState: string;
}

export interface AutopilotData {
  master: boolean;
  heading: number;
  altitude: number;
  vs: number;
  speed: number;
  approachHold: boolean;
  navLock: boolean;
}

export interface LightData {
  beacon: boolean;
  strobe: boolean;
  landing: boolean;
}

export interface FlightControlData {
  elevator: number;
  aileron: number;
  rudder: number;
  flaps: number;
  spoilers: number;
  gearDown: boolean;
}

export interface SimTimeData {
  zuluTime: number;
  zuluDay: number;
  zuluMonth: number;
  zuluYear: number;
  localTime: number;
}

export interface APUData {
  switchOn: boolean;
  rpmPercent: number;
  genSwitch: boolean;
  genActive: boolean;
}

export interface DoorData {
  openRatio: number;
}

export interface WeightData {
  totalWeight: number;
  fuelWeight: number;
}

export interface FlightData {
  position: PositionData;
  attitude: AttitudeData;
  engines: [EngineData, EngineData, EngineData, EngineData];
  sensors: SensorData;
  radios: RadioData;
  autopilot: AutopilotData;
  altimeterInHg: number;
  lights: LightData;
  controls: FlightControlData;
  simTime: SimTimeData;
  apu: APUData;
  doors: DoorData[];
  aircraftName: string;
  weight: WeightData;
}

export function useFlightData() {
  const [flightData, setFlightData] = useState<FlightData | null>(null);
  const [isRecording, setIsRecording] = useState(false);

  useEffect(() => {
    FlightDataService.IsRecording().then(setIsRecording).catch(() => {});

    const cancelFlight = Events.On("flight-data", (event: any) => {
      setFlightData(event.data);
    });

    const cancelRecording = Events.On("recording-state", (event: any) => {
      setIsRecording(event.data);
    });

    return () => {
      cancelFlight();
      cancelRecording();
    };
  }, []);

  return { flightData, isRecording };
}
