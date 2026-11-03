import { useTranslation } from "react-i18next";
import { CheckCircle2, XCircle } from "lucide-react";
import type { FlightDetail, StabilizedApproachGate } from "../../bindings/airspace-acars/internal/domain/models";
import { Badge } from "@/components/ui/badge";
import { centrelineOffset, landingStrip, touchdownDistance } from "@/lib/flight-detail-sections";
import { landingRateBand } from "@/lib/flight-detail";
import { DetailCard, RunwayStripView, StatRow, WindCard, usePhrase } from "@/components/flight-detail-runway";

interface FlightDetailLandingProps {
  detail: FlightDetail;
  landingRateFpm: number;
  arrivalIcao?: string;
}

/** The Landing tab: what the platform measured at touchdown. */
export function FlightDetailLanding({ detail, landingRateFpm, arrivalIcao }: FlightDetailLandingProps) {
  const { t } = useTranslation();
  const phrase = usePhrase();
  const landing = detail.landing;
  if (!landing) return null;

  const band = landingRateBand(landingRateFpm);
  const strip = landingStrip(landing);
  const knots = (v: number | null | undefined) => (v == null ? null : t("myFlights.units.knots", { value: Math.round(v) }));

  return (
    <div className="space-y-4">
      {strip && landing.runway && (
        <RunwayStripView
          strip={strip}
          runway={landing.runway}
          icao={arrivalIcao}
          markKey="myFlights.landing.touchdown"
          segmentKey="myFlights.landing.rollout"
        />
      )}

      <div className="grid gap-4 md:grid-cols-2">
        <DetailCard title={t("myFlights.landing.title")}>
          <StatRow label={t("myFlights.landingRate")} value={`${Math.round(landingRateFpm)} fpm`} tone={band.colorClass} />
          <StatRow
            label={t("myFlights.landing.gForce")}
            value={detail.landing_g_force != null ? `${detail.landing_g_force.toFixed(2)} G` : null}
          />
          <StatRow label={t("myFlights.landing.touchdownDistance")} value={phrase(touchdownDistance(landing.touchdown_distance_m))} />
          <StatRow label={t("myFlights.landing.touchdownOffset")} value={phrase(centrelineOffset(landing.touchdown_offset_m))} />
          <StatRow label={t("myFlights.landing.groundSpeed")} value={knots(landing.touchdown_ground_speed)} />
          <StatRow label={t("myFlights.landing.indicatedAirspeed")} value={knots(landing.touchdown_indicated_airspeed)} />
          <StatRow
            label={t("myFlights.landing.runwayUsed")}
            value={
              landing.runway_used_m != null
                ? t("myFlights.landing.runwayUsedValue", {
                    value: landing.runway_used_m,
                    percent: landing.runway_used_percent != null ? landing.runway_used_percent.toFixed(1) : "—",
                  })
                : null
            }
          />
          <StatRow
            label={t("myFlights.landing.approach")}
            value={detail.approach_type ? t(`myFlights.approach.${detail.approach_type}`, detail.approach_type) : null}
          />
        </DetailCard>

        <WindCard title={t("myFlights.wind.atTouchdown")} wind={landing.wind} />
      </div>

      {landing.stabilized_approach && <StabilizedGateCard gate={landing.stabilized_approach} />}
    </div>
  );
}

function GateVerdict({ passed }: { passed: boolean }) {
  const { t } = useTranslation();
  return passed ? (
    <span className="inline-flex items-center gap-1 text-emerald-400"><CheckCircle2 className="h-3.5 w-3.5" />{t("myFlights.gate.pass")}</span>
  ) : (
    <span className="inline-flex items-center gap-1 text-red-400"><XCircle className="h-3.5 w-3.5" />{t("myFlights.gate.fail")}</span>
  );
}

/**
 * The airline's stabilized-approach rule as the platform judged it. The
 * per-criterion rows exist only on flights evaluated since the platform kept
 * the snapshot; older flights show the verdict and the stored violations.
 */
function StabilizedGateCard({ gate }: { gate: StabilizedApproachGate }) {
  const { t } = useTranslation();
  const criteria = gate.criteria;

  return (
    <DetailCard title={t("myFlights.gate.title", { height: Math.round(gate.gate_height_ft) })}>
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">{t("myFlights.gate.verdict")}</span>
        <Badge variant="outline" className={gate.passed ? "border-emerald-500/40" : "border-red-500/40"}>
          <GateVerdict passed={gate.passed} />
        </Badge>
      </div>
      {criteria && (
        <div>
          <StatRow
            label={t("myFlights.gate.flaps", { required: Math.round(criteria.flaps.required_pct) })}
            value={
              <span className="inline-flex items-center gap-2">
                {criteria.flaps.measured_pct != null ? `${Math.round(criteria.flaps.measured_pct)}%` : "—"}
                <GateVerdict passed={criteria.flaps.passed} />
              </span>
            }
          />
          <StatRow
            label={t("myFlights.gate.gear")}
            value={
              <span className="inline-flex items-center gap-2">
                {t(criteria.gear.down ? "myFlights.gate.gearDown" : "myFlights.gate.gearUp")}
                <GateVerdict passed={criteria.gear.passed} />
              </span>
            }
          />
          <StatRow
            label={t("myFlights.gate.speed", { limit: Math.round(criteria.speed.limit_kt) })}
            value={
              <span className="inline-flex items-center gap-2">
                {t("myFlights.units.knots", { value: Math.round(criteria.speed.measured_kt) })}
                <GateVerdict passed={criteria.speed.passed} />
              </span>
            }
          />
        </div>
      )}
      {gate.failures.length > 0 && (
        <ul className="space-y-1 text-xs text-red-300">
          {gate.failures.map((failure, i) => (
            <li key={i}>{failure}</li>
          ))}
        </ul>
      )}
    </DetailCard>
  );
}
