import { useTranslation } from "react-i18next";
import type { TakeoffReport } from "../../bindings/airspace-acars/internal/domain/models";
import { centrelineOffset, takeoffStrip } from "@/lib/flight-detail-sections";
import { DetailCard, RunwayStripView, StatRow, WindCard, usePhrase } from "@/components/flight-detail-runway";

interface FlightDetailTakeoffProps {
  takeoff: TakeoffReport;
  departureIcao?: string;
}

/** The Takeoff tab: what the platform measured at liftoff. */
export function FlightDetailTakeoff({ takeoff, departureIcao }: FlightDetailTakeoffProps) {
  const { t } = useTranslation();
  const phrase = usePhrase();
  const strip = takeoffStrip(takeoff);
  const knots = (v: number | null | undefined) => (v == null ? null : t("myFlights.units.knots", { value: Math.round(v) }));
  const metres = (v: number | null | undefined) => (v == null ? null : t("myFlights.units.metres", { value: Math.round(v) }));

  return (
    <div className="space-y-4">
      {strip && takeoff.runway && (
        <RunwayStripView
          strip={strip}
          runway={takeoff.runway}
          icao={departureIcao}
          markKey="myFlights.takeoff.liftoff"
          segmentKey="myFlights.takeoff.roll"
        />
      )}

      <div className="grid gap-4 md:grid-cols-2">
        <DetailCard title={t("myFlights.takeoff.title")}>
          <StatRow
            label={t("myFlights.takeoff.liftoffDistance")}
            value={
              takeoff.liftoff_distance_m != null && takeoff.liftoff_distance_m < 0
                ? t("myFlights.takeoff.beforeThreshold", { value: Math.abs(takeoff.liftoff_distance_m) })
                : metres(takeoff.liftoff_distance_m)
            }
          />
          <StatRow label={t("myFlights.takeoff.rollLength")} value={metres(takeoff.takeoff_roll_m)} />
          <StatRow label={t("myFlights.takeoff.liftoffOffset")} value={phrase(centrelineOffset(takeoff.liftoff_offset_m))} />
          <StatRow label={t("myFlights.takeoff.groundSpeed")} value={knots(takeoff.liftoff_ground_speed)} />
          <StatRow label={t("myFlights.takeoff.indicatedAirspeed")} value={knots(takeoff.liftoff_indicated_airspeed)} />
          <StatRow
            label={t("myFlights.takeoff.runwayUsed")}
            value={takeoff.runway_used_percent != null ? `${takeoff.runway_used_percent.toFixed(1)}%` : null}
          />
        </DetailCard>

        <WindCard title={t("myFlights.wind.atLiftoff")} wind={takeoff.wind} />
      </div>
    </div>
  );
}
