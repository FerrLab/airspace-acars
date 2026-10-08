import { useTranslation } from "react-i18next";
import type { FlightData } from "@/hooks/use-flight-data";
import { isUnsetTime, secondsSince, type DebugSnapshot } from "@/lib/debug-snapshot";
import { BoolBadge, DataTable, SectionTitle, fmt } from "./debug-table";

interface Props {
  snapshot: DebugSnapshot | null;
  flightData: FlightData | null;
  now: number;
}

/** Where the ACARS places the aircraft, and the few readings that decide it. */
export function DebugOverview({ snapshot, flightData: d, now }: Props) {
  const { t } = useTranslation();
  const g = snapshot?.ground;

  // "No airport" is only an answer once a lookup has given it; before the
  // first one, or after a failure, nothing is known about airports here.
  const checked = !!g && !isUnsetTime(g.checkedAt);
  let lookup = t("debug.ground.notLoaded");
  if (g?.lastError) lookup = t("debug.ground.error", { error: g.lastError });
  else if (g?.airport) lookup = t("debug.ground.loadedAgo", { age: secondsSince(g.loadedAt, now) });
  else if (checked) lookup = t("debug.ground.noneFoundAgo", { age: secondsSince(g?.checkedAt, now) });

  let airport = "—";
  if (g?.airport) airport = `${g.airport} · ${t("debug.ground.counts", { runways: g.runways, stands: g.stands })}`;
  else if (checked && !g?.lastError) airport = t("debug.ground.noAirport");

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <section className="space-y-2">
        <SectionTitle>{t("debug.ground.title")}</SectionTitle>
        <DataTable
          rows={[
            {
              label: t("debug.ground.airport"),
              value: airport,
            },
            { label: t("debug.ground.runway"), value: g?.runway || "—" },
            { label: t("debug.ground.stand"), value: g?.stand || "—" },
            { label: t("debug.ground.lookup"), value: lookup },
          ]}
        />
      </section>
      {d && (
        <section className="space-y-2">
          <SectionTitle>{t("debug.position")}</SectionTitle>
          <DataTable
            rows={[
              { label: t("debug.row.latitude"), value: fmt(d.position.latitude, 6), unit: "deg" },
              { label: t("debug.row.longitude"), value: fmt(d.position.longitude, 6), unit: "deg" },
              { label: t("debug.row.agl"), value: fmt(d.position.altitudeAGL, 0), unit: "ft" },
              { label: t("debug.row.headingTrue"), value: fmt(d.attitude.headingTrue, 1), unit: "deg" },
              { label: t("debug.row.gs"), value: fmt(d.attitude.gs, 1), unit: "kts" },
              { label: t("debug.row.onGround"), value: <BoolBadge value={d.sensors.onGround} /> },
            ]}
          />
        </section>
      )}
    </div>
  );
}
