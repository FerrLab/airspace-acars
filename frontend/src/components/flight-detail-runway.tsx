import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { RunwayInfo, RunwayWind } from "../../bindings/airspace-acars/internal/domain/models";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { windComponents, type Phrase, type RunwayStrip } from "@/lib/flight-detail-sections";

/** A translated phrase, or null when there is nothing to say. */
export function usePhrase() {
  const { t } = useTranslation();
  return (phrase: Phrase | null) => (phrase ? t(phrase.key, phrase.values) : null);
}

export function DetailCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
      <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">{title}</span>
      {children}
    </Card>
  );
}

/** One label/value line. Renders nothing when the value was not recorded. */
export function StatRow({ label, value, tone }: { label: string; value: ReactNode | null | undefined; tone?: string }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex items-baseline justify-between gap-3 border-b border-border/40 py-1.5 last:border-0 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className={cn("font-mono font-medium text-foreground text-right", tone)}>{value}</span>
    </div>
  );
}

interface RunwayStripViewProps {
  strip: RunwayStrip;
  runway: RunwayInfo;
  icao?: string;
  /** Translation key of the marked point (touchdown or liftoff). */
  markKey: string;
  /** Translation key of the coloured segment (rollout or takeoff roll). */
  segmentKey: string;
}

/**
 * The runway drawn threshold-left, with the coloured segment and the marked
 * point the platform measured. Only recorded positions are drawn.
 */
export function RunwayStripView({ strip, runway, icao, markKey, segmentKey }: RunwayStripViewProps) {
  const { t, i18n } = useTranslation();
  const metres = (m: number) => t("myFlights.units.metres", { value: Math.round(m).toLocaleString(i18n.language) });

  return (
    <DetailCard title={t("myFlights.runway.title", { icao: icao ?? "", designator: runway.designator })}>
      <div
        role="img"
        aria-label={t("myFlights.runway.diagramLabel", { designator: runway.designator })}
        className="relative h-10 rounded-md bg-zinc-900 border border-border/60 overflow-hidden"
      >
        <div className="absolute inset-y-0 left-0 border-r-2 border-dashed border-white/50" style={{ width: `${strip.runwayEndPct}%` }} />
        <div className="absolute top-1/2 h-px -translate-y-1/2 bg-white/40 left-0" style={{ width: `${strip.runwayEndPct}%` }} />
        <div
          data-testid="runway-segment"
          className="absolute inset-y-2 rounded-sm bg-amber-400/70"
          style={{ left: `${strip.fromPct}%`, width: `${Math.max(0.5, strip.toPct - strip.fromPct)}%` }}
        />
        <div
          data-testid="runway-mark"
          className="absolute inset-y-0 w-1 -translate-x-1/2 bg-emerald-400"
          style={{ left: `${strip.markPct}%` }}
        />
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
        <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-emerald-400" />{t(markKey)}</span>
        <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-sm bg-amber-400/70" />{t(segmentKey)}</span>
        <span className="ml-auto font-mono">
          {t("myFlights.runway.length", { value: metres(strip.lengthM) })}
          {runway.width_m != null ? ` · ${t("myFlights.runway.width", { value: metres(runway.width_m) })}` : ""}
        </span>
      </div>
    </DetailCard>
  );
}

/** The wind on the touchdown or liftoff sample against the runway. */
export function WindCard({ title, wind }: { title: string; wind: RunwayWind | null | undefined }) {
  const { t } = useTranslation();
  const phrase = usePhrase();
  const components = windComponents(wind);
  if (!wind || !components) return null;
  return (
    <DetailCard title={title}>
      <StatRow
        label={t("myFlights.wind.reported")}
        value={t("myFlights.wind.value", { direction: Math.round(wind.direction_degrees), speed: Math.round(wind.speed_knots) })}
      />
      <StatRow label={t("myFlights.wind.along")} value={phrase(components.along)} />
      <StatRow label={t("myFlights.wind.across")} value={phrase(components.across)} />
    </DetailCard>
  );
}
