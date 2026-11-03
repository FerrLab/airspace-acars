import { useTranslation } from "react-i18next";
import type { FlightDetail, ScoreCategory } from "../../bindings/airspace-acars/internal/domain/models";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { eventTone, eventTypeKey, scoreTone } from "@/lib/flight-detail-sections";
import { formatUtcDate } from "@/lib/flight-format";
import { DetailCard } from "@/components/flight-detail-runway";

/**
 * The Score tab: the platform's live per-category score, the FDM events that
 * fed it, and the points awarded. Formulas and input labels arrive already
 * translated into the pilot's language.
 */
export function FlightDetailScore({ detail }: { detail: FlightDetail }) {
  const { t } = useTranslation();
  const breakdown = detail.score_breakdown;

  return (
    <div className="space-y-4">
      <DetailCard title={t("myFlights.score.title")}>
        <div className="flex items-baseline gap-2">
          <span className={cn("text-3xl font-bold font-mono", scoreTone(breakdown.score))}>
            {breakdown.score != null ? breakdown.score.toFixed(2) : "—"}
          </span>
          <span className="text-xs text-muted-foreground">/ 10</span>
        </div>
        <div className="space-y-3">
          {breakdown.categories.map((category) => (
            <CategoryRow key={category.key} category={category} />
          ))}
        </div>
      </DetailCard>

      <DetailCard title={t("myFlights.events.title")}>
        {detail.events.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("myFlights.events.none")}</p>
        ) : (
          <ul className="space-y-1.5 text-xs">
            {detail.events.map((event) => (
              <li key={event.id} className="flex items-baseline gap-2">
                <span className={cn("font-semibold uppercase text-[10px] w-20 shrink-0", eventTone(event.type))}>
                  {t(eventTypeKey(event.type))}
                </span>
                <span className="flex-1 text-foreground">
                  {event.event}
                  {event.duration_seconds ? ` (${t("myFlights.events.duration", { value: event.duration_seconds })})` : ""}
                </span>
                {event.happened_at && (
                  <span className="font-mono text-muted-foreground">{formatUtcDate(event.happened_at)}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </DetailCard>

      <DetailCard title={t("myFlights.points.title")}>
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{t("myFlights.points.total")}</span>
          <span className={cn("font-mono font-bold", detail.points.total < 0 ? "text-red-400" : "text-emerald-400")}>
            {signed(detail.points.total)}
          </span>
        </div>
        {detail.points.entries.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("myFlights.points.none")}</p>
        ) : (
          <ul className="space-y-1.5 text-xs">
            {detail.points.entries.map((entry) => (
              <li key={entry.id} className="flex items-baseline gap-2">
                <Badge variant="outline" className="text-[10px] shrink-0">
                  {t(entry.source === "fdm" ? "myFlights.points.fdm" : "myFlights.points.staff")}
                </Badge>
                <span className="flex-1">
                  {entry.reason}
                  {entry.remarks ? <span className="text-muted-foreground"> · {entry.remarks}</span> : null}
                </span>
                <span className={cn("font-mono", entry.points < 0 ? "text-red-400" : "text-emerald-400")}>{signed(entry.points)}</span>
              </li>
            ))}
          </ul>
        )}
      </DetailCard>
    </div>
  );
}

function signed(points: number): string {
  return points > 0 ? `+${points}` : String(points);
}

function CategoryRow({ category }: { category: ScoreCategory }) {
  const { t } = useTranslation();
  const scored = category.available && category.score != null;
  const label = t(`myFlights.score.category.${category.key}`, category.key);

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">{label}</span>
        <span className={cn("font-mono", scoreTone(scored ? category.score : null))}>
          {scored ? category.score!.toFixed(1) : t("myFlights.score.notAvailable")}
        </span>
      </div>
      <div
        role="meter"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={10}
        aria-valuenow={scored ? category.score! : 0}
        className="h-1.5 rounded-full bg-muted overflow-hidden"
      >
        <div className="h-full rounded-full bg-primary" style={{ width: `${scored ? Math.min(100, category.score! * 10) : 0}%` }} />
      </div>
      {!category.available && category.unavailable_reason && (
        <p className="text-[11px] text-muted-foreground">{category.unavailable_reason}</p>
      )}
      {category.formula && <p className="text-[11px] text-muted-foreground">{category.formula}</p>}
      {category.inputs.length > 0 && (
        <dl className="flex flex-wrap gap-x-4 gap-y-0.5 text-[11px]">
          {category.inputs.map((input, i) => (
            <div key={i} className="flex gap-1">
              <dt className="text-muted-foreground">{input.label}:</dt>
              <dd className="font-mono text-foreground">{input.value}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}
