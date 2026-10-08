import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { FlightData } from "@/hooks/use-flight-data";
import { BoolBadge, DataTable, SectionTitle, fmt } from "./debug-table";

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 rounded-md border border-border p-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

/** Every reading the simulator gives, grouped the way a pilot thinks of them. */
export function DebugTelemetry({ flightData: d }: { flightData: FlightData }) {
  const { t } = useTranslation();
  const th = "px-2 py-1 text-right text-[10px] font-medium text-muted-foreground";
  const td = "px-2 py-1 text-right font-mono text-xs tabular-nums";

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Group title={t("debug.group.flight")}>
        <SectionTitle>{t("debug.position")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.latitude"), value: fmt(d.position.latitude, 6), unit: "deg" },
          { label: t("debug.row.longitude"), value: fmt(d.position.longitude, 6), unit: "deg" },
          { label: t("debug.row.altitude"), value: fmt(d.position.altitude, 0), unit: "ft" },
          { label: t("debug.row.agl"), value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
        ]} />
        <SectionTitle>{t("debug.attitudeSpeed")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.pitch"), value: fmt(d.attitude.pitch), unit: "deg" },
          { label: t("debug.row.roll"), value: fmt(d.attitude.roll), unit: "deg" },
          { label: t("debug.row.headingTrue"), value: fmt(d.attitude.headingTrue, 1), unit: "deg" },
          { label: t("debug.row.headingMag"), value: fmt(d.attitude.headingMag, 1), unit: "deg" },
          { label: t("debug.row.vs"), value: fmt(d.attitude.vs, 0), unit: "fpm" },
          { label: t("debug.row.ias"), value: fmt(d.attitude.ias, 1), unit: "kts" },
          { label: t("debug.row.tas"), value: fmt(d.attitude.tas, 1), unit: "kts" },
          { label: t("debug.row.gs"), value: fmt(d.attitude.gs, 1), unit: "kts" },
        ]} />
        <SectionTitle>{t("debug.sensors")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.onGround"), value: <BoolBadge value={d.sensors.onGround} /> },
          { label: t("debug.row.stall"), value: <BoolBadge value={d.sensors.stallWarning} /> },
          { label: t("debug.row.overspeed"), value: <BoolBadge value={d.sensors.overspeedWarning} /> },
        ]} />
      </Group>

      <Group title={t("debug.group.systems")}>
        <SectionTitle>{t("debug.engines")}</SectionTitle>
        <div className="rounded-md border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-2 py-1 text-left text-[10px] font-medium text-muted-foreground">{t("debug.row.engine")}</th>
                <th className={th}>{t("debug.row.running")}</th>
                <th className={th}>{t("debug.row.n1")}</th>
                <th className={th}>{t("debug.row.n2")}</th>
                <th className={th}>{t("debug.row.throttle")}</th>
                <th className={th}>{t("debug.row.mixture")}</th>
                <th className={th}>{t("debug.row.prop")}</th>
              </tr>
            </thead>
            <tbody>
              {d.engines.map((eng, i) => (
                <tr key={i} className="border-b border-border/50 last:border-0">
                  <td className="px-2 py-1 font-mono text-xs">{i + 1}</td>
                  <td className="px-2 py-1 text-right"><BoolBadge value={eng.running} /></td>
                  <td className={td}>{fmt(eng.n1, 1)}</td>
                  <td className={td}>{fmt(eng.n2, 1)}</td>
                  <td className={td}>{fmt(eng.throttlePos, 0)}</td>
                  <td className={td}>{fmt(eng.mixturePos, 0)}</td>
                  <td className={td}>{fmt(eng.propPos, 0)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <SectionTitle>{t("debug.apu")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.apuSwitch"), value: <BoolBadge value={d.apu.switchOn} /> },
          { label: t("debug.row.apuRpm"), value: fmt(d.apu.rpmPercent, 1), unit: "%" },
          { label: t("debug.row.genSwitch"), value: <BoolBadge value={d.apu.genSwitch} /> },
          { label: t("debug.row.genActive"), value: <BoolBadge value={d.apu.genActive} /> },
        ]} />
        <SectionTitle>{t("debug.doors")}</SectionTitle>
        <DataTable rows={d.doors.map((door, i) => ({
          label: t("debug.row.door", { n: i + 1 }),
          value: fmt(door.openRatio * 100, 0),
          unit: "%",
        }))} />
        <SectionTitle>{t("debug.weight")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.total"), value: fmt(d.weight?.totalWeight ?? 0, 0), unit: "lbs" },
          { label: t("debug.row.fuel"), value: fmt(d.weight?.fuelWeight ?? 0, 0), unit: "lbs" },
        ]} />
      </Group>

      <Group title={t("debug.group.radios")}>
        <SectionTitle>{t("debug.radios")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.com1"), value: fmt(d.radios.com1, 3), unit: "MHz" },
          { label: t("debug.row.com2"), value: fmt(d.radios.com2, 3), unit: "MHz" },
          { label: t("debug.row.nav1"), value: fmt(d.radios.nav1, 2), unit: "MHz" },
          { label: t("debug.row.nav2"), value: fmt(d.radios.nav2, 2), unit: "MHz" },
          { label: t("debug.row.nav1Obs"), value: fmt(d.radios.nav1OBS, 0), unit: "deg" },
          { label: t("debug.row.nav2Obs"), value: fmt(d.radios.nav2OBS, 0), unit: "deg" },
          { label: t("debug.row.xpdrCode"), value: fmt(d.radios.xpdrCode, 0) },
          { label: t("debug.row.xpdrState"), value: d.radios.xpdrState || "—" },
        ]} />
        <SectionTitle>{t("debug.autopilot")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.apMaster"), value: <BoolBadge value={d.autopilot.master} /> },
          { label: t("debug.row.heading"), value: fmt(d.autopilot.heading, 0), unit: "deg" },
          { label: t("debug.row.altitude"), value: fmt(d.autopilot.altitude, 0), unit: "ft" },
          { label: t("debug.row.vs"), value: fmt(d.autopilot.vs, 0), unit: "fpm" },
          { label: t("debug.row.speed"), value: fmt(d.autopilot.speed, 0), unit: "kts" },
          { label: t("debug.row.approach"), value: <BoolBadge value={d.autopilot.approachHold} /> },
          { label: t("debug.row.navLock"), value: <BoolBadge value={d.autopilot.navLock} /> },
        ]} />
      </Group>

      <Group title={t("debug.group.lightsControls")}>
        <SectionTitle>{t("debug.lights")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.beacon"), value: <BoolBadge value={d.lights.beacon} /> },
          { label: t("debug.row.strobe"), value: <BoolBadge value={d.lights.strobe} /> },
          { label: t("debug.row.landing"), value: <BoolBadge value={d.lights.landing} /> },
        ]} />
        <SectionTitle>{t("debug.controls")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.elevator"), value: fmt(d.controls.elevator, 3) },
          { label: t("debug.row.aileron"), value: fmt(d.controls.aileron, 3) },
          { label: t("debug.row.rudder"), value: fmt(d.controls.rudder, 3) },
          { label: t("debug.row.flaps"), value: fmt(d.controls.flaps, 0), unit: "%" },
          { label: t("debug.row.spoilers"), value: fmt(d.controls.spoilers, 0), unit: "%" },
          { label: t("debug.row.gearDown"), value: <BoolBadge value={d.controls.gearDown} /> },
        ]} />
        <SectionTitle>{t("debug.misc")}</SectionTitle>
        <DataTable rows={[
          { label: t("debug.row.aircraft"), value: d.aircraftName || "—" },
          { label: t("debug.row.altimeter"), value: fmt(d.altimeterInHg, 2), unit: "inHg" },
          { label: t("debug.row.zulu"), value: fmt(d.simTime.zuluTime, 0), unit: "sec" },
          { label: t("debug.row.local"), value: fmt(d.simTime.localTime, 0), unit: "sec" },
        ]} />
      </Group>
    </div>
  );
}
