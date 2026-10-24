import { useTranslation } from "react-i18next";
import { ArrowLeft, MapPin, Plane } from "lucide-react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { FlightsMap } from "@/components/flights-map";
import { getFlightStatusBadge } from "@/components/flight-status-badge";
import { getFlightKPIs, getFlightFacts } from "@/lib/flight-detail";
import { formatUtcDate } from "@/lib/flight-format";

interface FlightDetailViewProps {
  flight: FlightLog;
  company: string;
  onBack: () => void;
}

const noopSelectFlight = () => {};

export function FlightDetailView({
  flight,
  company,
  onBack,
}: FlightDetailViewProps) {
  const { t, i18n } = useTranslation();

  const kpis = getFlightKPIs(flight, i18n.language);
  const facts = getFlightFacts(flight);

  const depCity = flight.departure_airport?.city || flight.departure_airport?.name || "";
  const arrCity = flight.arrival_airport?.city || flight.arrival_airport?.name || "";

  return (
    <div className="flex h-full flex-col space-y-4 animate-in fade-in duration-200">
      {/* Top Navigation & Flight Header */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 pb-3">
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={onBack}
            className="h-8 gap-1.5 text-xs border-border/80 shadow-xs"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            <span>{t("myFlights.backToFlights")}</span>
          </Button>

          <Badge
            variant="outline"
            className="border-primary/40 bg-primary/10 text-primary font-mono font-bold text-xs px-2.5 py-0.5"
          >
            {flight.callsign}
          </Badge>

          <div>
            <h2 className="text-base font-bold text-foreground flex items-center gap-2">
              <span>
                {flight.departure_airport?.icao}
                {depCity ? ` (${depCity})` : ""}
              </span>
              <span className="text-primary">➔</span>
              <span>
                {flight.arrival_airport?.icao}
                {arrCity ? ` (${arrCity})` : ""}
              </span>
            </h2>
            <p className="text-xs text-muted-foreground">
              {company || "Airspace"} •{" "}
              {flight.flight_number
                ? `${t("myFlights.flightWord")} ${flight.flight_number}`
                : flight.callsign}{" "}
              • {formatUtcDate(flight.departure_time || flight.created_at)}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {getFlightStatusBadge(flight.status, t)}
          <span className="text-xs font-mono text-muted-foreground">
            {Math.round(flight.distance_nm).toLocaleString(i18n.language)} NM
          </span>
        </div>
      </div>

      {/* Hero Strip: Route and 5 Key KPIs */}
      <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3 pb-2 border-b border-border/40">
          <div className="flex items-center gap-3">
            <div className="h-9 w-9 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
              <Plane className="h-4 w-4" />
            </div>
            <div>
              <span className="text-xs font-bold text-foreground">
                {depCity || flight.departure_airport?.icao} ➔{" "}
                {arrCity || flight.arrival_airport?.icao}
              </span>
              <span className="text-[11px] text-muted-foreground block font-mono">
                {flight.aircraft?.registration || "—"} ·{" "}
                {flight.aircraft?.icao_code || "—"}{" "}
                {flight.aircraft?.name ? `(${flight.aircraft.name})` : ""}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Badge
              variant="outline"
              className="text-xs font-mono border-border/60 text-muted-foreground"
            >
              <MapPin className="h-3 w-3 mr-1 text-primary" />
              {flight.departure_airport?.icao} ➔ {flight.arrival_airport?.icao}
            </Badge>
          </div>
        </div>

        {/* Real Metric KPI Row */}
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2 pt-1">
          {kpis.map((k, idx) => (
            <div
              key={idx}
              className="rounded-lg bg-muted/10 p-2.5 border border-border/40 space-y-1"
            >
              <span className="text-[10px] font-semibold uppercase tracking-[0.15em] text-muted-foreground block truncate">
                {t(k.labelKey, k.defaultLabel)}
              </span>
              <p
                className={`text-base font-bold font-mono ${k.colorClass} truncate`}
              >
                {k.value}
                {k.unit ? ` ${k.unit}` : ""}
              </p>
              {(k.subKey || k.sub) && (
                <span className="text-[10px] text-muted-foreground block truncate">
                  {k.subKey ? t(k.subKey) : k.sub}
                </span>
              )}
            </div>
          ))}
        </div>
      </Card>

      {/* Main Content: Map and Flight Facts */}
      <div className="flex-1 overflow-y-auto pr-0.5 pb-4 space-y-4">
        {/* Interactive Route Map */}
        <div className="relative w-full h-[360px] rounded-xl overflow-hidden border border-border/70 bg-card shadow-xs">
          <FlightsMap
            flights={[flight]}
            selectedFlightId={flight.id}
            onSelectFlight={noopSelectFlight}
            className="h-full w-full"
          />
        </div>

        {/* Flight Facts Card */}
        <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
          <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
            {t("myFlights.flightSummary")}
          </span>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3 text-xs">
            {facts.map((fact, idx) => (
              <div key={idx} className="space-y-0.5">
                <span className="text-muted-foreground text-[11px] block">
                  {t(fact.labelKey, fact.defaultLabel)}
                </span>
                <span className="font-mono font-medium text-foreground block truncate">
                  {fact.value}
                </span>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
