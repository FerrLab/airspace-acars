import { useState, useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { useFlightData } from "@/hooks/use-flight-data";
import { Events } from "@wailsio/runtime";
import { FlightDataService } from "../../bindings/airspace-acars";

function BoolBadge({ value }: { value: boolean }) {
  return (
    <Badge variant={value ? "default" : "secondary"} className="text-[10px] px-1.5 py-0">
      {value ? "ON" : "OFF"}
    </Badge>
  );
}

function DataTable({ rows }: { rows: { label: string; value: string | React.ReactNode; unit?: string }[] }) {
  return (
    <div className="rounded-md border border-border">
      <table className="w-full text-sm">
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className="border-b border-border/50 last:border-0">
              <td className="px-3 py-1 font-mono text-xs text-muted-foreground w-[140px]">{r.label}</td>
              <td className="px-3 py-1 text-right font-mono text-xs tabular-nums">{r.value}</td>
              {r.unit && <td className="px-2 py-1 text-xs text-muted-foreground w-[50px]">{r.unit}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function fmt(v: number, d: number = 2): string {
  return v.toFixed(d);
}

export function DebugTab() {
  const { flightData } = useFlightData();
  const [isConnected, setIsConnected] = useState(false);
  const [updateCount, setUpdateCount] = useState(0);

  useEffect(() => {
    FlightDataService.IsConnected().then(setIsConnected).catch(() => {});

    const cancel = Events.On("connection-state", (event: any) => {
      setIsConnected(event.data);
    });

    return () => { cancel(); };
  }, []);

  useEffect(() => {
    if (!flightData) return;
    setUpdateCount((c) => c + 1);
  }, [flightData]);

  const d = flightData;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Debug</h2>
          <p className="text-sm text-muted-foreground">
            Raw simulator data in real time
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant={isConnected ? "default" : "secondary"}>
            {isConnected ? "Connected" : "Disconnected"}
          </Badge>
          <span className="text-xs text-muted-foreground tabular-nums">
            {updateCount} updates
          </span>
        </div>
      </div>

      <Separator />

      {!d ? (
        <p className="text-sm text-muted-foreground">Waiting for data...</p>
      ) : (
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Position</h3>
            <DataTable rows={[
              { label: "Latitude", value: fmt(d.position.latitude, 6), unit: "deg" },
              { label: "Longitude", value: fmt(d.position.longitude, 6), unit: "deg" },
              { label: "Altitude", value: fmt(d.position.altitude, 0), unit: "ft" },
              { label: "AGL", value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Attitude & Speed</h3>
            <DataTable rows={[
              { label: "Pitch", value: fmt(d.attitude.pitch), unit: "deg" },
              { label: "Roll", value: fmt(d.attitude.roll), unit: "deg" },
              { label: "Heading True", value: fmt(d.attitude.headingTrue, 1), unit: "deg" },
              { label: "Heading Mag", value: fmt(d.attitude.headingMag, 1), unit: "deg" },
              { label: "VS", value: fmt(d.attitude.vs, 0), unit: "fpm" },
              { label: "IAS", value: fmt(d.attitude.ias, 1), unit: "kts" },
              { label: "TAS", value: fmt(d.attitude.tas, 1), unit: "kts" },
              { label: "GS", value: fmt(d.attitude.gs, 1), unit: "kts" },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Sensors</h3>
            <DataTable rows={[
              { label: "On Ground", value: <BoolBadge value={d.sensors.onGround} /> },
              { label: "Stall Warning", value: <BoolBadge value={d.sensors.stallWarning} /> },
              { label: "Overspeed", value: <BoolBadge value={d.sensors.overspeedWarning} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Lights</h3>
            <DataTable rows={[
              { label: "Beacon", value: <BoolBadge value={d.lights.beacon} /> },
              { label: "Strobe", value: <BoolBadge value={d.lights.strobe} /> },
              { label: "Landing", value: <BoolBadge value={d.lights.landing} /> },
            ]} />
          </div>

          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Engines</h3>
            <div className="rounded-md border border-border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border bg-muted/50">
                    <th className="px-2 py-1 text-left text-[10px] font-medium text-muted-foreground">Eng</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">Run</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">N1%</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">N2%</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">Thr%</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">Mix%</th>
                    <th className="px-2 py-1 text-right text-[10px] font-medium text-muted-foreground">Prop%</th>
                  </tr>
                </thead>
                <tbody>
                  {d.engines.map((eng, i) => (
                    <tr key={i} className="border-b border-border/50 last:border-0">
                      <td className="px-2 py-1 font-mono text-xs">{i + 1}</td>
                      <td className="px-2 py-1 text-right"><BoolBadge value={eng.running} /></td>
                      <td className="px-2 py-1 text-right font-mono text-xs tabular-nums">{fmt(eng.n1, 1)}</td>
                      <td className="px-2 py-1 text-right font-mono text-xs tabular-nums">{fmt(eng.n2, 1)}</td>
                      <td className="px-2 py-1 text-right font-mono text-xs tabular-nums">{fmt(eng.throttlePos, 0)}</td>
                      <td className="px-2 py-1 text-right font-mono text-xs tabular-nums">{fmt(eng.mixturePos, 0)}</td>
                      <td className="px-2 py-1 text-right font-mono text-xs tabular-nums">{fmt(eng.propPos, 0)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Radios</h3>
            <DataTable rows={[
              { label: "COM1", value: fmt(d.radios.com1, 3), unit: "MHz" },
              { label: "COM2", value: fmt(d.radios.com2, 3), unit: "MHz" },
              { label: "NAV1", value: fmt(d.radios.nav1, 2), unit: "MHz" },
              { label: "NAV2", value: fmt(d.radios.nav2, 2), unit: "MHz" },
              { label: "NAV1 OBS", value: fmt(d.radios.nav1OBS, 0), unit: "deg" },
              { label: "NAV2 OBS", value: fmt(d.radios.nav2OBS, 0), unit: "deg" },
              { label: "XPDR Code", value: fmt(d.radios.xpdrCode, 0) },
              { label: "XPDR State", value: d.radios.xpdrState || "—" },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Autopilot</h3>
            <DataTable rows={[
              { label: "Master", value: <BoolBadge value={d.autopilot.master} /> },
              { label: "Heading", value: fmt(d.autopilot.heading, 0), unit: "deg" },
              { label: "Altitude", value: fmt(d.autopilot.altitude, 0), unit: "ft" },
              { label: "VS", value: fmt(d.autopilot.vs, 0), unit: "fpm" },
              { label: "Speed", value: fmt(d.autopilot.speed, 0), unit: "kts" },
              { label: "Approach", value: <BoolBadge value={d.autopilot.approachHold} /> },
              { label: "NAV Lock", value: <BoolBadge value={d.autopilot.navLock} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Controls</h3>
            <DataTable rows={[
              { label: "Elevator", value: fmt(d.controls.elevator, 3) },
              { label: "Aileron", value: fmt(d.controls.aileron, 3) },
              { label: "Rudder", value: fmt(d.controls.rudder, 3) },
              { label: "Flaps", value: fmt(d.controls.flaps, 0), unit: "%" },
              { label: "Spoilers", value: fmt(d.controls.spoilers, 0), unit: "%" },
              { label: "Gear Down", value: <BoolBadge value={d.controls.gearDown} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Misc</h3>
            <DataTable rows={[
              { label: "Altimeter", value: fmt(d.altimeterInHg, 2), unit: "inHg" },
              { label: "Zulu Time", value: fmt(d.simTime.zuluTime, 0), unit: "sec" },
              { label: "Local Time", value: fmt(d.simTime.localTime, 0), unit: "sec" },
            ]} />
          </div>
        </div>
      )}

    </div>
  );
}
