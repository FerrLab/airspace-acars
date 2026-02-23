import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Plug, Unplug } from "lucide-react";
import { FlightDataCard } from "@/components/flight-data-card";
import { RecordingControls } from "@/components/recording-controls";
import { useFlightData } from "@/hooks/use-flight-data";
import { FlightDataService } from "../../bindings/airspace-acars";

export function AcarsTab() {
  const { flightData, isRecording } = useFlightData();
  const [isConnected, setIsConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);

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

      <RecordingControls isRecording={isRecording} isConnected={isConnected} />

      <div className="grid grid-cols-4 gap-3">
        <FlightDataCard
          label="Altitude"
          value={flightData?.altitude ?? null}
          unit="ft"
          decimals={0}
        />
        <FlightDataCard
          label="Heading"
          value={flightData?.heading ?? null}
          unit="deg"
          decimals={1}
        />
        <FlightDataCard
          label="Airspeed"
          value={flightData?.airspeed ?? null}
          unit="kts"
          decimals={0}
        />
        <FlightDataCard
          label="Ground Speed"
          value={flightData?.groundSpeed ?? null}
          unit="kts"
          decimals={0}
        />
        <FlightDataCard
          label="Vertical Speed"
          value={flightData?.verticalSpeed ?? null}
          unit="fpm"
          decimals={0}
        />
        <FlightDataCard
          label="Pitch"
          value={flightData?.pitch ?? null}
          unit="deg"
          decimals={1}
        />
        <FlightDataCard
          label="Roll"
          value={flightData?.roll ?? null}
          unit="deg"
          decimals={1}
        />
      </div>
    </div>
  );
}
