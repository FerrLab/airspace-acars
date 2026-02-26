import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Plug, Unplug, Plane, Square, CheckCircle2 } from "lucide-react";
import { RecordingControls } from "@/components/recording-controls";
import { useFlightData } from "@/hooks/use-flight-data";
import { useSoundPlayer } from "@/hooks/use-sound-player";
import { FlightDataService, FlightService } from "../../bindings/airspace-acars";
import { Events } from "@wailsio/runtime";

interface AcarsTabProps {
  localMode?: boolean;
}

export function AcarsTab({ localMode = false }: AcarsTabProps) {
  const { isRecording } = useFlightData();
  const [isConnected, setIsConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [flightState, setFlightState] = useState<"idle" | "active">("idle");
  const [booking, setBooking] = useState<any>(null);
  const [startingFlight, setStartingFlight] = useState(false);
  const [endingFlight, setEndingFlight] = useState(false);
  const [volume, setVolume] = useState(() => {
    const stored = localStorage.getItem("acars_volume");
    return stored ? parseInt(stored, 10) : 50;
  });

  // Sound player: active when flight is active and not in local mode
  useSoundPlayer(volume, flightState === "active" && !localMode);

  useEffect(() => {
    FlightDataService.IsConnected().then(setIsConnected).catch(() => {});
    if (!localMode) {
      FlightService.GetFlightState().then((s) => setFlightState(s as any)).catch(() => {});
    }

    const cancelConn = Events.On("connection-state", (event: any) => {
      setIsConnected(event.data);
    });
    const cancelFlight = localMode ? () => {} : Events.On("flight-state", (event: any) => {
      setFlightState(event.data);
    });

    return () => {
      cancelConn();
      cancelFlight();
    };
  }, [localMode]);

  const fetchBooking = useCallback(async () => {
    try {
      const result = await FlightService.GetBooking();
      setBooking(result);
    } catch {
      setBooking(null);
    }
  }, []);

  // Poll booking every 10s when idle and connected (skip in local mode)
  useEffect(() => {
    if (localMode || !isConnected || flightState === "active") return;
    fetchBooking();
    const interval = setInterval(fetchBooking, 10_000);
    return () => clearInterval(interval);
  }, [localMode, isConnected, flightState, fetchBooking]);

  const handleConnect = async () => {
    setConnecting(true);
    try {
      await FlightDataService.ConnectSim("auto");
      setIsConnected(true);
    } catch (e: any) {
      console.error("Failed to connect:", e);
      alert("Failed to connect: " + e);
    } finally {
      setConnecting(false);
    }
  };

  const handleDisconnect = async () => {
    try {
      FlightDataService.DisconnectSim();
      setIsConnected(false);
    } catch (e: any) {
      console.error("Failed to disconnect:", e);
    }
  };

  const handleStartFlight = async () => {
    if (!booking) return;
    setStartingFlight(true);
    try {
      const callsign = booking.callsign ?? booking.flight_number ?? "";
      const departure = booking.departure ?? booking.dep ?? "";
      const arrival = booking.arrival ?? booking.arr ?? "";
      await FlightService.StartFlight(callsign, departure, arrival);
    } catch (e: any) {
      alert("Failed to start flight: " + e);
    } finally {
      setStartingFlight(false);
    }
  };

  const handleStopFlight = async () => {
    setEndingFlight(true);
    try {
      await FlightService.StopFlight();
    } catch (e: any) {
      alert("Failed to stop flight: " + e);
    } finally {
      setEndingFlight(false);
    }
  };

  const handleFinishFlight = async () => {
    setEndingFlight(true);
    try {
      await FlightService.FinishFlight();
    } catch (e: any) {
      alert("Failed to finish flight: " + e);
    } finally {
      setEndingFlight(false);
    }
  };

  const handleVolumeChange = (v: number) => {
    setVolume(v);
    localStorage.setItem("acars_volume", String(v));
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Flight Data</h2>
          <p className="text-sm text-muted-foreground">
            Connect to your simulator to view live telemetry
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant={isConnected ? "default" : "secondary"}>
            {isConnected ? "Connected" : "Disconnected"}
          </Badge>
          {!isConnected ? (
            <Button size="sm" onClick={handleConnect} disabled={connecting} className="gap-2">
              <Plug className="h-3 w-3" />
              {connecting ? "Connecting..." : "Connect"}
            </Button>
          ) : (
            <Button size="sm" variant="outline" onClick={handleDisconnect} className="gap-2">
              <Unplug className="h-3 w-3" />
              Disconnect
            </Button>
          )}
        </div>
      </div>

      <Separator />

      {/* Local Mode indicator */}
      {localMode && (
        <div className="rounded-lg border border-dashed border-yellow-500/50 bg-yellow-500/5 p-4 text-center">
          <Badge variant="outline" className="mb-2 border-yellow-500/50 text-yellow-500">
            Local Mode
          </Badge>
          <p className="text-sm text-muted-foreground">
            Flights, booking, and cabin audio are disabled. Simulator connection and recording are still available.
          </p>
        </div>
      )}

      {/* Flight Controls */}
      {!localMode && isConnected && (
        <div className="space-y-4">
          {flightState === "idle" && booking && (
            <div className="rounded-lg border border-border p-4 space-y-3">
              <div className="flex items-center gap-2">
                <Plane className="h-4 w-4 text-primary" />
                <span className="text-sm font-medium">Active Booking</span>
              </div>
              <div className="grid grid-cols-3 gap-4 text-sm">
                <div>
                  <span className="text-xs text-muted-foreground block">Callsign</span>
                  <span className="font-mono font-medium">
                    {booking.callsign ?? booking.flight_number ?? "---"}
                  </span>
                </div>
                <div>
                  <span className="text-xs text-muted-foreground block">Departure</span>
                  <span className="font-mono font-medium">
                    {booking.departure ?? booking.dep ?? "---"}
                  </span>
                </div>
                <div>
                  <span className="text-xs text-muted-foreground block">Arrival</span>
                  <span className="font-mono font-medium">
                    {booking.arrival ?? booking.arr ?? "---"}
                  </span>
                </div>
              </div>
              <Button
                size="sm"
                onClick={handleStartFlight}
                disabled={startingFlight}
                className="gap-2"
              >
                <Plane className="h-3 w-3" />
                {startingFlight ? "Starting..." : "Start Flight"}
              </Button>
            </div>
          )}

          {flightState === "idle" && !booking && (
            <div className="rounded-lg border border-dashed border-border p-4 text-center">
              <p className="text-sm text-muted-foreground">
                No active booking. Create a booking on the VA website to start a flight.
              </p>
            </div>
          )}

          {flightState === "active" && (
            <div className="rounded-lg border border-primary/30 bg-primary/5 p-4 space-y-3">
              <div className="flex items-center gap-2">
                <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />
                <span className="text-sm font-medium">Flight Active</span>
                <Badge variant="outline" className="ml-auto text-xs">
                  Position reporting
                </Badge>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="default"
                  onClick={handleFinishFlight}
                  disabled={endingFlight}
                  className="gap-2"
                >
                  <CheckCircle2 className="h-3 w-3" />
                  {endingFlight ? "Finishing..." : "Finish Flight"}
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={handleStopFlight}
                  disabled={endingFlight}
                  className="gap-2"
                >
                  <Square className="h-3 w-3" />
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      <Separator />

      {/* Volume Control */}
      <div className="flex items-center gap-3">
        <span className="text-sm text-muted-foreground w-28">
          Cabin Audio
        </span>
        <input
          type="range"
          min={0}
          max={100}
          value={volume}
          onChange={(e) => handleVolumeChange(Number(e.target.value))}
          className="flex-1 accent-primary cursor-pointer"
        />
        <span className="text-xs text-muted-foreground tabular-nums w-10 text-right">
          {volume}%
        </span>
      </div>

      <Separator />

      <RecordingControls isRecording={isRecording} isConnected={isConnected} />
    </div>
  );
}
