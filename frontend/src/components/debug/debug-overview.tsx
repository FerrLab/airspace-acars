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

  let lookup = t("debug.ground.notLoaded");
  if (g?.lastError) lookup = t("debug.ground.error", { error: g.lastError });
  else if (g && !isUnsetTime(g.loadedAt)) lookup = t("debug.ground.loadedAgo", { age: secondsSince(g.loadedAt, now) });

  return (
    <div className="grid gap-4 md:grid-cols-2">
      <section className="space-y-2">
        <SectionTitle>{t("debug.ground.title")}</SectionTitle>
        <DataTable
          rows={[
            {
              label: t("debug.ground.airport"),
              value: g?.airport
                ? `${g.airport} · ${t("debug.ground.counts", { runways: g.runways, stands: g.stands })}`
                : t("debug.ground.noAirport"),
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
