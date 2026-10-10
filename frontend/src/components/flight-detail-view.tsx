import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  ArrowLeft,
  Award,
  History,
  Info,
  LayoutGrid,
  MapPin,
  Plane,
  TrendingDown,
  TrendingUp,
} from "lucide-react";
import type { FlightLog } from "../../bindings/airspace-acars/internal/domain/models";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { FlightsMap } from "@/components/flights-map";
import {
  getFlightKPIs,
  getFlightFacts,
  getAuditLogs,
  type FlightDetailTab,
} from "@/lib/flight-detail";
import { FlightDetailRunway } from "@/components/flight-detail-runway";
import { FlightDetailTelemetry } from "@/components/flight-detail-telemetry";
import { FlightDetailScore } from "@/components/flight-detail-score";

interface FlightDetailViewProps {
  flight: FlightLog;
  company: string;
  onBack: () => void;
}

const noopSelectFlight = () => {};

export function FlightDetailView({ flight, company, onBack }: FlightDetailViewProps) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<FlightDetailTab>("overview");

  const kpis = getFlightKPIs(flight);
  const facts = getFlightFacts(flight);
  const auditLogs = getAuditLogs(flight);

  const tabs: { id: FlightDetailTab; labelKey: string; defaultLabel: string; icon: React.ComponentType<{ className?: string }>; count?: number }[] = [
    { id: "overview", labelKey: "myFlights.tabOverview", defaultLabel: "Visão Geral", icon: LayoutGrid },
    { id: "landing", labelKey: "myFlights.tabLanding", defaultLabel: "Pouso", icon: TrendingDown },
    { id: "takeoff", labelKey: "myFlights.tabTakeoff", defaultLabel: "Decolagem", icon: TrendingUp },
    { id: "telemetry", labelKey: "myFlights.tabTelemetry", defaultLabel: "Telemetria", icon: Activity },
    { id: "score", labelKey: "myFlights.tabScore", defaultLabel: "Pontuação", icon: Award },
    { id: "log", labelKey: "myFlights.tabLog", defaultLabel: "Histórico", icon: History, count: auditLogs.length },
  ];

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
            <span>{t("myFlights.backToFlights", "Voltar aos voos")}</span>
          </Button>

          <Badge
            variant="outline"
            className="border-primary/40 bg-primary/10 text-primary font-mono font-bold text-xs px-2.5 py-0.5"
          >
            {flight.callsign}
          </Badge>

          <div>
            <h2 className="text-base font-bold text-foreground flex items-center gap-2">
              <span>{flight.departure_airport?.icao} ({flight.departure_airport?.city})</span>
              <span className="text-primary">➔</span>
              <span>{flight.arrival_airport?.icao} ({flight.arrival_airport?.city})</span>
            </h2>
            <p className="text-xs text-muted-foreground">
              {company || "Airspace"} • {flight.flight_number ? `Voo ${flight.flight_number}` : flight.callsign}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <Badge variant="outline" className="text-xs px-2.5 py-0.5 border-emerald-500/30 text-emerald-400 bg-emerald-500/10 font-medium">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 mr-1.5" />
            {flight.status || "Approved"}
          </Badge>
          <span className="text-xs font-mono text-muted-foreground">
            {Math.round(flight.distance_nm)} NM
          </span>
        </div>
      </div>

      {/* Hero Strip: Route and 6 Key KPIs */}
      <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3 pb-2 border-b border-border/40">
          <div className="flex items-center gap-3">
            <div className="h-9 w-9 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
              <Plane className="h-4 w-4" />
            </div>
            <div>
              <span className="text-xs font-bold text-foreground">
                {flight.departure_airport?.city || flight.departure_airport?.icao} ➔ {flight.arrival_airport?.city || flight.arrival_airport?.icao}
              </span>
              <span className="text-[11px] text-muted-foreground block font-mono">
                {flight.aircraft?.registration || "PR-SBG"} · {flight.aircraft?.icao_code || "A320"} {flight.aircraft?.name ? `(${flight.aircraft.name})` : ""}
              </span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-xs font-mono border-border/60 text-muted-foreground">
              <MapPin className="h-3 w-3 mr-1 text-primary" />
              {flight.departure_airport?.icao} ➔ {flight.arrival_airport?.icao}
            </Badge>
          </div>
        </div>

        {/* 6 Metric KPI Row */}
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-2 pt-1">
          {kpis.map((k, idx) => (
            <div key={idx} className="rounded-lg bg-muted/10 p-2.5 border border-border/40 space-y-1">
              <span className="text-[10px] font-semibold uppercase tracking-[0.15em] text-muted-foreground block truncate">
                {t(k.labelKey, k.defaultLabel)}
              </span>
              <p className={`text-base font-bold font-mono ${k.colorClass} truncate`}>
                {k.value}{k.unit ? ` ${k.unit}` : ""}
              </p>
              <span className="text-[10px] text-muted-foreground block truncate">{k.sub}</span>
            </div>
          ))}
        </div>
      </Card>

      {/* Flux-style Navigation Tabs Bar */}
      <div className="flex items-center gap-1 border-b border-border/60 overflow-x-auto pb-0.5">
        {tabs.map((tab) => {
          const IconComponent = tab.icon;
          const isActive = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-3.5 py-2 text-xs font-semibold rounded-t-lg transition-all border-b-2 -mb-[1px] whitespace-nowrap ${
                isActive
                  ? "border-primary text-foreground bg-card/60 shadow-xs"
                  : "border-transparent text-muted-foreground hover:text-foreground hover:bg-muted/10"
              }`}
            >
              <IconComponent className={`h-3.5 w-3.5 ${isActive ? "text-primary" : "text-muted-foreground"}`} />
              <span>{t(tab.labelKey, tab.defaultLabel)}</span>
              {tab.count != null && (
                <Badge variant="outline" className="text-[10px] px-1.5 py-0 border-border/60 text-muted-foreground">
                  {tab.count}
                </Badge>
              )}
            </button>
          );
        })}
      </div>

      {/* Tab Panels */}
      <div className="flex-1 overflow-y-auto pr-0.5 pb-4">
        {activeTab === "overview" && (
          <div className="space-y-4">
            {/* Interactive Route Map */}
            <div className="relative w-full h-[360px] rounded-xl overflow-hidden border border-border/70 bg-card shadow-xs">
              <FlightsMap
                flights={[flight]}
                selectedFlightId={flight.id}
                onSelectFlight={noopSelectFlight}
                className="h-full w-full"
              />
            </div>

            {/* Flight Facts Grid & Score Summary */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              {/* Flight Facts Card */}
              <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
                <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
                  {t("myFlights.flightSummary", "Resumo do Voo")}
                </span>
                <div className="grid grid-cols-2 gap-3 text-xs">
                  {facts.map((fact, idx) => (
                    <div key={idx} className="space-y-0.5">
                      <span className="text-muted-foreground text-[11px] block">{t(fact.labelKey, fact.defaultLabel)}</span>
                      <span className="font-mono font-medium text-foreground block truncate">{fact.value}</span>
                    </div>
                  ))}
                </div>
              </Card>

              {/* FDM Events Card */}
              <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                    {t("myFlights.events", "Eventos")}
                  </span>
                  <Badge variant="outline" className="text-[10px] px-2 py-0 border-sky-500/30 text-sky-400 bg-sky-500/10">
                    3 info · 0 warnings
                  </Badge>
                </div>
                <div className="divide-y divide-border/40 text-xs">
                  <div className="flex items-start gap-2.5 py-2">
                    <Info className="h-4 w-4 text-sky-400 shrink-0 mt-0.5" />
                    <div className="flex-1">
                      <p className="text-foreground">Duração de voo (80 min) dentro do envelope programado (60-120 min)</p>
                      <span className="font-mono text-[10px] text-muted-foreground">14:36Z · Decolagem</span>
                    </div>
                  </div>
                  <div className="flex items-start gap-2.5 py-2">
                    <Info className="h-4 w-4 text-emerald-400 shrink-0 mt-0.5" />
                    <div className="flex-1">
                      <p className="text-foreground">Aproximação estabilizada a 500 ft: flaps FULL, trem travado, velocidade nominal</p>
                      <span className="font-mono text-[10px] text-muted-foreground">16:05Z · Final RWY 14</span>
                    </div>
                  </div>
                  <div className="flex items-start gap-2.5 py-2">
                    <Info className="h-4 w-4 text-sky-400 shrink-0 mt-0.5" />
                    <div className="flex-1">
                      <p className="text-foreground">G-Force de toque (1.16 G) dentro do padrão suave (0 até 1.2 G)</p>
                      <span className="font-mono text-[10px] text-muted-foreground">16:10Z · Toque na Pista</span>
                    </div>
                  </div>
                </div>
              </Card>
            </div>
          </div>
        )}

        {activeTab === "landing" && <FlightDetailRunway flight={flight} mode="landing" />}

        {activeTab === "takeoff" && <FlightDetailRunway flight={flight} mode="takeoff" />}

        {activeTab === "telemetry" && <FlightDetailTelemetry flight={flight} />}

        {activeTab === "score" && <FlightDetailScore flight={flight} />}

        {activeTab === "log" && (
          <Card className="border-border/70 bg-card p-4 shadow-xs space-y-3">
            <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground block">
              {t("myFlights.activityLog", "Registro de Atividade")}
            </span>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-border/60 text-muted-foreground text-[10px] uppercase font-semibold">
                    <th className="py-2 pr-3">Horário</th>
                    <th className="py-2 pr-3">Evento</th>
                    <th className="py-2">Registrado por</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/40">
                  {auditLogs.map((log, idx) => (
                    <tr key={idx}>
                      <td className="py-2.5 pr-3 font-mono text-muted-foreground">{log.timeAgo}</td>
                      <td className="py-2.5 pr-3 text-foreground font-medium">{t(log.eventKey, log.defaultEvent)}</td>
                      <td className="py-2.5 text-muted-foreground">{log.by}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </Card>
        )}
      </div>
    </div>
  );
}
