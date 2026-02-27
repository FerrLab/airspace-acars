import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { useFlightData } from "@/hooks/use-flight-data";
import { Events } from "@wailsio/runtime";
import { FlightDataService } from "../../bindings/airspace-acars";

function BoolBadge({ value }: { value: boolean }) {
  const { t } = useTranslation();
  return (
    <Badge variant={value ? "default" : "secondary"} className="text-[10px] px-1.5 py-0">
      {value ? t("debug.on") : t("debug.off")}
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
  const { t } = useTranslation();
  const { flightData } = useFlightData();
  const [isConnected, setIsConnected] = useState(false);
  const [updateCount, setUpdateCount] = useState(0);

  useEffect(() => {
    FlightDataService.IsConnected().then(setIsConnected).catch(() => {});

    const cancel = Events.On("connection-state", (event: any) => {
      setIsConnected(!!event.data);
    });

    return () => { cancel(); };
  }, []);

  useEffect(() => {
    if (!flightData) return;
    setUpdateCount((c) => c + 1);
  }, [flightData]);

  const [copied, setCopied] = useState(false);

  const d = flightData;

  const payloadJson = useMemo(() => {
    if (!d) return null;
    const m = (value: number | string, unit: string) => ({ value, unit });
    const zuluSec = Math.floor(d.simTime.zuluTime);
    const payload = {
      callsign: "—",
      departure: "—",
      arrival: "—",
      timestamp: new Date().toISOString(),
      elapsedTime: m(0, "s"),
      position: {
        latitude: m(d.position.latitude, "deg"),
        longitude: m(d.position.longitude, "deg"),
        altitude: m(d.position.altitude, "ft"),
        altitudeAgl: m(d.position.altitudeAGL, "ft"),
      },
      attitude: {
        pitch: m(d.attitude.pitch, "deg"),
        roll: m(d.attitude.roll, "deg"),
        headingTrue: m(d.attitude.headingTrue, "deg"),
        headingMag: m(d.attitude.headingMag, "deg"),
        vs: m(d.attitude.vs, "fpm"),
        ias: m(d.attitude.ias, "kts"),
        tas: m(d.attitude.tas, "kts"),
        gs: m(d.attitude.gs, "kts"),
        gForce: m(d.attitude.gForce, "G"),
      },
      engines: d.engines.map((e) => ({
        running: e.running,
        n1: m(e.n1, "%"),
        n2: m(e.n2, "%"),
        throttle: m(e.throttlePos, "%"),
        mixture: m(e.mixturePos, "%"),
        propeller: m(e.propPos, "%"),
      })),
      sensors: {
        onGround: d.sensors.onGround,
        stallWarning: d.sensors.stallWarning,
        overspeedWarning: d.sensors.overspeedWarning,
        simulationRate: m(d.sensors.simulationRate, "x"),
      },
      radios: {
        com1: m(d.radios.com1, "MHz"),
        com2: m(d.radios.com2, "MHz"),
        nav1: m(d.radios.nav1, "MHz"),
        nav2: m(d.radios.nav2, "MHz"),
        nav1Obs: m(d.radios.nav1OBS, "deg"),
        nav2Obs: m(d.radios.nav2OBS, "deg"),
        transponderCode: m(d.radios.xpdrCode, ""),
        transponderState: d.radios.xpdrState,
      },
      autopilot: {
        master: d.autopilot.master,
        heading: m(d.autopilot.heading, "deg"),
        altitude: m(d.autopilot.altitude, "ft"),
        vs: m(d.autopilot.vs, "fpm"),
        speed: m(d.autopilot.speed, "kts"),
        approachHold: d.autopilot.approachHold,
        navLock: d.autopilot.navLock,
      },
      altimeter: m(d.altimeterInHg, "inHg"),
      lights: {
        beacon: d.lights.beacon,
        strobe: d.lights.strobe,
        landing: d.lights.landing,
      },
      controls: {
        elevator: m(d.controls.elevator, "position"),
        aileron: m(d.controls.aileron, "position"),
        rudder: m(d.controls.rudder, "position"),
        flaps: m(d.controls.flaps, "%"),
        spoilers: m(d.controls.spoilers, "%"),
        gearDown: d.controls.gearDown,
      },
      apu: {
        switchOn: d.apu.switchOn,
        rpm: m(d.apu.rpmPercent, "%"),
        genSwitch: d.apu.genSwitch,
        genActive: d.apu.genActive,
      },
      doors: d.doors.map((door) => ({
        open: m(door.openRatio, "ratio"),
      })),
      simTime: {
        zuluHour: m(Math.floor(zuluSec / 3600), "h"),
        zuluMin: m(Math.floor((zuluSec % 3600) / 60), "min"),
        zuluSec: m(zuluSec % 60, "s"),
        zuluDay: m(d.simTime.zuluDay, ""),
        zuluMonth: m(d.simTime.zuluMonth, ""),
        zuluYear: m(d.simTime.zuluYear, ""),
        localTime: m(d.simTime.localTime, "s"),
      },
      aircraftName: d.aircraftName || "",
      weight: {
        total: m(d.weight?.totalWeight ?? 0, "lbs"),
        fuel: m(d.weight?.fuelWeight ?? 0, "lbs"),
      },
    };
    return JSON.stringify(payload, null, 2);
  }, [d]);

  function handleCopy() {
    if (!payloadJson) return;
    navigator.clipboard.writeText(payloadJson).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{t("debug.title")}</h2>
          <p className="text-sm text-muted-foreground">
            {t("debug.subtitle")}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant={isConnected ? "default" : "secondary"}>
            {isConnected ? t("debug.connected") : t("debug.disconnected")}
          </Badge>
          <span className="text-xs text-muted-foreground tabular-nums">
            {t("debug.updates", { count: updateCount })}
          </span>
        </div>
      </div>

      <Separator />

      {!d ? (
        <p className="text-sm text-muted-foreground">{t("debug.waitingForData")}</p>
      ) : (
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.position")}</h3>
            <DataTable rows={[
              { label: "Latitude", value: fmt(d.position.latitude, 6), unit: "deg" },
              { label: "Longitude", value: fmt(d.position.longitude, 6), unit: "deg" },
              { label: "Altitude", value: fmt(d.position.altitude, 0), unit: "ft" },
              { label: "AGL", value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.attitudeSpeed")}</h3>
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

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.sensors")}</h3>
            <DataTable rows={[
              { label: "On Ground", value: <BoolBadge value={d.sensors.onGround} /> },
              { label: "Stall Warning", value: <BoolBadge value={d.sensors.stallWarning} /> },
              { label: "Overspeed", value: <BoolBadge value={d.sensors.overspeedWarning} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.lights")}</h3>
            <DataTable rows={[
              { label: "Beacon", value: <BoolBadge value={d.lights.beacon} /> },
              { label: "Strobe", value: <BoolBadge value={d.lights.strobe} /> },
              { label: "Landing", value: <BoolBadge value={d.lights.landing} /> },
            ]} />
          </div>

          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.engines")}</h3>
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

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.radios")}</h3>
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

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.autopilot")}</h3>
            <DataTable rows={[
              { label: "Master", value: <BoolBadge value={d.autopilot.master} /> },
              { label: "Heading", value: fmt(d.autopilot.heading, 0), unit: "deg" },
              { label: "Altitude", value: fmt(d.autopilot.altitude, 0), unit: "ft" },
              { label: "VS", value: fmt(d.autopilot.vs, 0), unit: "fpm" },
              { label: "Speed", value: fmt(d.autopilot.speed, 0), unit: "kts" },
              { label: "Approach", value: <BoolBadge value={d.autopilot.approachHold} /> },
              { label: "NAV Lock", value: <BoolBadge value={d.autopilot.navLock} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.controls")}</h3>
            <DataTable rows={[
              { label: "Elevator", value: fmt(d.controls.elevator, 3) },
              { label: "Aileron", value: fmt(d.controls.aileron, 3) },
              { label: "Rudder", value: fmt(d.controls.rudder, 3) },
              { label: "Flaps", value: fmt(d.controls.flaps, 0), unit: "%" },
              { label: "Spoilers", value: fmt(d.controls.spoilers, 0), unit: "%" },
              { label: "Gear Down", value: <BoolBadge value={d.controls.gearDown} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.apu")}</h3>
            <DataTable rows={[
              { label: "Switch", value: <BoolBadge value={d.apu.switchOn} /> },
              { label: "RPM", value: fmt(d.apu.rpmPercent, 1), unit: "%" },
              { label: "Gen Switch", value: <BoolBadge value={d.apu.genSwitch} /> },
              { label: "Gen Active", value: <BoolBadge value={d.apu.genActive} /> },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.doors")}</h3>
            <DataTable rows={d.doors.map((door, i) => ({
              label: `Door ${i}`,
              value: fmt(door.openRatio * 100, 0),
              unit: "%",
            }))} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.weight")}</h3>
            <DataTable rows={[
              { label: "Total", value: fmt(d.weight?.totalWeight ?? 0, 0), unit: "lbs" },
              { label: "Fuel", value: fmt(d.weight?.fuelWeight ?? 0, 0), unit: "lbs" },
            ]} />

            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t("debug.misc")}</h3>
            <DataTable rows={[
              { label: "Aircraft", value: d.aircraftName || "—" },
              { label: "Altimeter", value: fmt(d.altimeterInHg, 2), unit: "inHg" },
              { label: "Zulu Time", value: fmt(d.simTime.zuluTime, 0), unit: "sec" },
              { label: "Local Time", value: fmt(d.simTime.localTime, 0), unit: "sec" },
            ]} />
          </div>
        </div>
      )}

      {payloadJson && (
        <>
          <Separator />
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                {t("debug.apiPayload")}
              </h3>
              <Button variant="outline" size="sm" className="h-7 text-xs" onClick={handleCopy}>
                {copied ? t("debug.copied") : t("debug.copyJson")}
              </Button>
            </div>
            <pre className="rounded-md border border-border bg-muted/50 p-3 text-[11px] font-mono leading-relaxed overflow-auto max-h-[400px]">
              {payloadJson}
            </pre>
          </div>
        </>
      )}
    </div>
  );
}
