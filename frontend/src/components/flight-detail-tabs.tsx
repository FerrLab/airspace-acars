import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { FlightLogService } from "../../bindings/airspace-acars";
import type { FlightDetail, FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { availableTabs, type DetailTab } from "@/lib/flight-detail-sections";
import { FlightDetailLanding } from "@/components/flight-detail-landing";
import { FlightDetailTakeoff } from "@/components/flight-detail-takeoff";
import { FlightDetailTelemetry } from "@/components/flight-detail-telemetry";
import { FlightDetailScore } from "@/components/flight-detail-score";

interface FlightDetailTabsProps {
  flight: FlightLog;
  /** The Overview tab's content: the map and the logbook facts. */
  overview: ReactNode;
}

/**
 * The detail view's tabs. Overview renders at once from the logbook entry;
 * the other tabs appear once the flight's detail arrives, and only for the
 * sections the platform recorded. A detail that cannot be loaded leaves the
 * view at Overview, which is all the logbook entry can show. Inactive tabs
 * are unmounted, so the Telemetry track is fetched only when it is opened.
 */
export function FlightDetailTabs({ flight, overview }: FlightDetailTabsProps) {
  const { t } = useTranslation();
  const [detail, setDetail] = useState<FlightDetail | null>(null);
  const [tab, setTab] = useState<DetailTab>("overview");

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setTab("overview");
    FlightLogService.GetFlightDetail(flight.id)
      .then((result) => {
        if (cancelled) return;
        setDetail(result?.status === "ok" && result.data ? result.data : null);
      })
      .catch(() => {
        // The view still has its Overview from the logbook entry; the
        // backend has already logged why the detail failed.
        if (!cancelled) setDetail(null);
      });
    return () => {
      cancelled = true;
    };
  }, [flight.id]);

  const tabs = availableTabs(detail);

  return (
    <Tabs value={tab} onValueChange={(value) => setTab(value as DetailTab)} className="flex-1 min-h-0">
      {tabs.length > 1 && (
        <TabsList>
          {tabs.map((key) => (
            <TabsTrigger key={key} value={key} className="text-xs">
              {t(`myFlights.tabs.${key}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      )}

      <div className="flex-1 overflow-y-auto pr-0.5 pb-4">
        <TabsContent value="overview" className="space-y-4">
          {overview}
        </TabsContent>
        {detail?.landing && (
          <TabsContent value="landing">
            <FlightDetailLanding detail={detail} landingRateFpm={flight.landing_rate_fpm} arrivalIcao={flight.arrival_airport?.icao} />
          </TabsContent>
        )}
        {detail?.takeoff && (
          <TabsContent value="takeoff">
            <FlightDetailTakeoff takeoff={detail.takeoff} departureIcao={flight.departure_airport?.icao} />
          </TabsContent>
        )}
        {detail && (
          <TabsContent value="telemetry">
            <FlightDetailTelemetry flightId={flight.id} />
          </TabsContent>
        )}
        {detail && tabs.includes("score") && (
          <TabsContent value="score">
            <FlightDetailScore detail={detail} />
          </TabsContent>
        )}
      </div>
    </Tabs>
  );
}
