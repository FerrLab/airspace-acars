import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
import { FlightLogService } from "../../bindings/airspace-acars";
import type { TrackPoint } from "../../bindings/airspace-acars/internal/domain/models";
import { buildTelemetryChart, sampleAt, seriesPoints } from "@/lib/flight-telemetry";
import { DetailCard } from "@/components/flight-detail-runway";

const WIDTH = 1000;
const HEIGHT = 220;

type TrackState =
  | { kind: "loading" }
  | { kind: "failed"; messageKey: string }
  | { kind: "loaded"; points: TrackPoint[] };

/**
 * The Telemetry tab. The track is the heaviest reply the detail view makes,
 * so it is fetched only when this tab is first opened, once per flight.
 */
export function FlightDetailTelemetry({ flightId }: { flightId: string }) {
  const { t } = useTranslation();
  const [track, setTrack] = useState<TrackState>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    setTrack({ kind: "loading" });
    FlightLogService.GetFlightTrack(flightId)
      .then((result) => {
        if (cancelled) return;
        if (!result || result.status !== "ok") {
          setTrack({ kind: "failed", messageKey: "myFlights.telemetry.unavailable" });
          return;
        }
        setTrack({ kind: "loaded", points: result.points ?? [] });
      })
      .catch(() => {
        if (!cancelled) setTrack({ kind: "failed", messageKey: "myFlights.telemetry.loadError" });
      });
    return () => {
      cancelled = true;
    };
  }, [flightId]);

  if (track.kind === "loading") {
    return (
      <div className="flex items-center gap-2 p-6 text-xs text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        {t("myFlights.telemetry.loading")}
      </div>
    );
  }
  if (track.kind === "failed") {
    return <p className="p-6 text-xs text-muted-foreground">{t(track.messageKey)}</p>;
  }
  return <TelemetryChartCard points={track.points} />;
}

function TelemetryChartCard({ points }: { points: TrackPoint[] }) {
  const { t, i18n } = useTranslation();
  const chart = useMemo(() => buildTelemetryChart(points), [points]);
  const [hover, setHover] = useState<number | null>(null);

  if (!chart) {
    return <p className="p-6 text-xs text-muted-foreground">{t("myFlights.telemetry.none")}</p>;
  }

  const altitudeLine = seriesPoints(chart, (s) => s.altitude, chart.maxAltitude, WIDTH, HEIGHT);
  const speedLine = seriesPoints(chart, (s) => s.speed, chart.maxSpeed, WIDTH, HEIGHT);
  const hovered = hover != null ? sampleAt(chart, hover) : null;
  const time = (ms: number) => new Date(ms).toISOString().slice(11, 16) + "Z";

  return (
    <DetailCard title={t("myFlights.telemetry.title")}>
      <div className="flex flex-wrap gap-x-4 text-[11px] text-muted-foreground">
        <span className="flex items-center gap-1.5"><span className="h-0.5 w-4 bg-sky-400" />{t("myFlights.telemetry.altitude", { value: Math.round(chart.maxAltitude).toLocaleString(i18n.language) })}</span>
        <span className="flex items-center gap-1.5"><span className="h-0.5 w-4 bg-amber-400" />{t("myFlights.telemetry.speed", { value: Math.round(chart.maxSpeed) })}</span>
        {hovered && (
          <span className="ml-auto font-mono text-foreground">
            {time(hovered.timeMs)} · {Math.round(hovered.altitude).toLocaleString(i18n.language)} ft · {Math.round(hovered.speed)} kt
          </span>
        )}
      </div>

      <svg
        role="img"
        aria-label={t("myFlights.telemetry.chartLabel")}
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        preserveAspectRatio="none"
        className="h-56 w-full rounded-md bg-muted/10"
        onMouseMove={(e) => {
          const box = e.currentTarget.getBoundingClientRect();
          if (box.width > 0) setHover((e.clientX - box.left) / box.width);
        }}
        onMouseLeave={() => setHover(null)}
      >
        <polyline points={altitudeLine} fill="none" className="stroke-sky-400" strokeWidth={2} vectorEffect="non-scaling-stroke" />
        <polyline points={speedLine} fill="none" className="stroke-amber-400" strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
        {hover != null && (
          <line x1={hover * WIDTH} x2={hover * WIDTH} y1={0} y2={HEIGHT} className="stroke-foreground/40" vectorEffect="non-scaling-stroke" />
        )}
      </svg>

      {chart.phases.length > 0 && (
        <div className="relative h-5 rounded-sm bg-muted/20 overflow-hidden" aria-label={t("myFlights.telemetry.phases")}>
          {chart.phases.map((segment, i) => (
            <div
              key={i}
              title={segment.phase}
              className="absolute inset-y-0 flex items-center justify-center border-r border-background/60 bg-primary/15 text-[9px] font-semibold uppercase text-muted-foreground overflow-hidden"
              style={{ left: `${segment.startPct}%`, width: `${segment.widthPct}%` }}
            >
              {segment.widthPct > 7 ? segment.phase : ""}
            </div>
          ))}
        </div>
      )}

      <div className="flex justify-between font-mono text-[10px] text-muted-foreground">
        <span>{time(chart.startMs)}</span>
        <span>{time(chart.endMs)}</span>
      </div>
    </DetailCard>
  );
}
