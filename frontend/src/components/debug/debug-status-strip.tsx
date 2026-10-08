import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { FlightData } from "@/hooks/use-flight-data";
import { outcomeKey, secondsSince, type DebugSnapshot } from "@/lib/debug-snapshot";

interface Props {
  connected: boolean;
  adapter: string;
  flightData: FlightData | null;
  snapshot: DebugSnapshot | null;
  now: number;
}

/** One glance at the link, the aircraft and the last report, above every tab. */
export function DebugStatusStrip({ connected, adapter, flightData, snapshot, now }: Props) {
  const { t } = useTranslation();
  const profiles = snapshot?.profiles ?? [];
  const ground = snapshot?.ground;
  const report = snapshot?.report;
  const age = secondsSince(report?.at, now);
  const place = [ground?.airport, ground?.runway, ground?.stand && `${t("debug.ground.stand")} ${ground.stand}`]
    .filter(Boolean)
    .join(" · ");

  return (
    <div role="status" className="space-y-1 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant={connected ? "default" : "secondary"}>
          {connected ? t("debug.connected") : t("debug.disconnected")}
        </Badge>
        {adapter && <span>{adapter}</span>}
        <span>·</span>
        <span>{flightData?.aircraftName || t("debug.strip.noAircraft")}</span>
        <span>·</span>
        <span>{profiles.length ? t("debug.strip.profile", { name: profiles.join(", ") }) : t("debug.strip.noProfile")}</span>
      </div>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
        {flightData && <span>{flightData.sensors.onGround ? t("debug.strip.onGround") : t("debug.strip.airborne")}</span>}
        {place && <span>{place}</span>}
        <span>
          {age === null || !report?.json
            ? t("debug.strip.noReport")
            : t("debug.strip.report", { age, outcome: t(outcomeKey(report.outcome)) })}
        </span>
      </div>
    </div>
  );
}
