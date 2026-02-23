import { useState, useEffect } from "react";
import { Events } from "@wailsio/runtime";

export interface FlightData {
  altitude: number;
  heading: number;
  pitch: number;
  roll: number;
  airspeed: number;
  groundSpeed: number;
  verticalSpeed: number;
}

export function useFlightData() {
  const [flightData, setFlightData] = useState<FlightData | null>(null);
  const [isRecording, setIsRecording] = useState(false);

  useEffect(() => {
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
