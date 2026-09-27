import { useEffect, useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertCircle,
  Award,
  Calendar,
  Clock,
  Compass,
  ExternalLink,
  Filter,
  Globe,
  Loader2,
  MapPin,
  Navigation,
  Plane,
  PlaneTakeoff,
  RefreshCw,
  Search,
  TrendingDown,
  User,
  X,
} from "lucide-react";
import { FlightLogService } from "../../bindings/airspace-acars";
import type {
  FlightLog,
  PilotSummaryStats,
} from "../../bindings/airspace-acars/internal/domain/models";
import { useAuth } from "@/context/auth-context";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { FlightsMap } from "@/components/flights-map";

const knownStatuses = ["accessDenied", "unavailable", "rateLimited", "noSession", "localMode"];
function statusKey(status: string) {
  return `myFlights.${knownStatuses.includes(status) ? status : "loadError"}`;
}

export function MyFlightsTab({ localMode = false }: { localMode?: boolean }) {
  const { tenant, tokenSynced } = useAuth();
  return (
    <MyFlightsDashboard
      key={`${tenant?.id}:${tenant?.domain}`}
      company={tenant?.name ?? ""}
      localMode={localMode}
      ready={tokenSynced && !!tenant}
    />
  );
}

function formatDuration(minutes: number): string {
  if (!minutes || minutes <= 0) return "—";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m}m`;
  return `${h}h ${m}m`;
}

function getLandingRateBadge(rate: number) {
  if (!rate || rate === 0) {
    return (
      <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-border text-muted-foreground">
        —
      </Badge>
    );
  }
  const abs = Math.abs(rate);
  if (abs <= 180) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-emerald-500/40 text-emerald-400 bg-emerald-500/10 font-mono"
      >
        {Math.round(rate)} fpm
      </Badge>
    );
  }
  if (abs <= 300) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-sky-500/40 text-sky-400 bg-sky-500/10 font-mono"
      >
        {Math.round(rate)} fpm
      </Badge>
    );
  }
  if (abs <= 450) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-amber-500/40 text-amber-400 bg-amber-500/10 font-mono"
      >
        {Math.round(rate)} fpm
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="text-[10px] py-0 px-1.5 border-red-500/40 text-red-400 bg-red-500/10 font-mono"
    >
      {Math.round(rate)} fpm
    </Badge>
  );
}

function getFlightStatusBadge(status: string) {
  const norm = (status || "accepted").toLowerCase();
  if (norm.includes("accept") || norm.includes("approved")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-emerald-500/40 text-emerald-400 bg-emerald-500/10"
      >
        Aprovado
      </Badge>
    );
  }
  if (norm.includes("pending") || norm.includes("review")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-amber-500/40 text-amber-400 bg-amber-500/10"
      >
        Pendente
      </Badge>
    );
  }
  if (norm.includes("reject")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-red-500/40 text-red-400 bg-red-500/10"
      >
        Rejeitado
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="text-[10px] py-0 px-1.5 border-border text-muted-foreground">
      {status}
    </Badge>
  );
}

function MyFlightsDashboard({
  company,
  localMode,
  ready,
}: {
  company: string;
  localMode: boolean;
  ready: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [pilot, setPilot] = useState<PilotSummaryStats | null>(null);
  const [flights, setFlights] = useState<FlightLog[]>([]);
  const [selectedFlightId, setSelectedFlightId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(!localMode);
    setError("");

    if (localMode) {
      setLoading(false);
      return;
    }
    if (!ready) {
      setLoading(false);
      return;
    }

    FlightLogService.GetMyFlights(1, 50)
      .then((res) => {
        if (cancelled) return;
        if (!res || res.status !== "ok") {
          setError(statusKey(res?.status ?? ""));
          return;
        }
        setPilot(res.pilot ?? null);
        const flightList = (res.flights ?? []) as FlightLog[];
        setFlights(flightList);
        if (flightList.length > 0 && !selectedFlightId) {
          setSelectedFlightId(flightList[0].id);
        }
      })
      .catch(() => {
        if (!cancelled) setError("myFlights.loadError");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [localMode, ready, refresh]);

  // Filter flights by search query
  const filteredFlights = useMemo(() => {
    if (!search.trim()) return flights;
    const term = search.toLowerCase().trim();
    return flights.filter((f) => {
      const callsign = (f.callsign || "").toLowerCase();
      const fltNum = (f.flight_number || "").toLowerCase();
      const depICAO = (f.departure_airport?.icao || "").toLowerCase();
      const depName = (f.departure_airport?.name || "").toLowerCase();
      const arrICAO = (f.arrival_airport?.icao || "").toLowerCase();
      const arrName = (f.arrival_airport?.name || "").toLowerCase();
      const reg = (f.aircraft?.registration || "").toLowerCase();
      const acType = (f.aircraft?.icao_code || "").toLowerCase();

      return (
        callsign.includes(term) ||
        fltNum.includes(term) ||
        depICAO.includes(term) ||
        depName.includes(term) ||
        arrICAO.includes(term) ||
        arrName.includes(term) ||
        reg.includes(term) ||
        acType.includes(term)
      );
    });
  }, [flights, search]);

  const selectedFlight = useMemo(() => {
    return flights.find((f) => f.id === selectedFlightId) || null;
  }, [flights, selectedFlightId]);

  const formatDate = (val?: string) => {
    if (!val) return "—";
    const d = new Date(val);
    return Number.isNaN(d.getTime())
      ? "—"
      : d.toLocaleDateString(i18n.language, {
          day: "2-digit",
          month: "short",
          year: "numeric",
        });
  };

  return (
    <div className="flex h-full flex-col space-y-4">
      {/* Header bar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-bold tracking-tight text-foreground">
              {t("myFlights.title", "My Flights Dashboard")}
            </h2>
            {company && (
              <Badge variant="outline" className="text-xs border-border bg-card/60">
                {company}
              </Badge>
            )}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(
              "myFlights.subtitle",
              "Acompanhe suas estatísticas de voo, histórico de rotas e mapa de operações."
            )}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative w-48 sm:w-64">
            <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              type="text"
              placeholder={t("myFlights.searchPlaceholder", "Buscar voo, ICAO, aeronave...")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 pl-8 text-xs bg-card/60 border-border/80"
            />
            {search && (
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setSearch("")}
                className="absolute right-1 top-1.5 h-5 w-5 text-muted-foreground hover:text-foreground"
              >
                <X className="h-3 w-3" />
              </Button>
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((r) => r + 1)}
            disabled={loading || localMode}
            className="h-8 gap-1.5 text-xs border-border/80"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            <span className="hidden sm:inline">{t("myFlights.refresh", "Atualizar")}</span>
          </Button>
        </div>
      </div>

      {/* Local Mode / Error Notice */}
      {localMode && (
        <Card className="flex items-center gap-3 border-yellow-500/30 bg-yellow-500/10 p-3 text-xs text-yellow-300">
          <AlertCircle className="h-4 w-4 shrink-0 text-yellow-400" />
          <span>
            {t(
              "myFlights.localMode",
              "O Airspace ACARS está em Modo Local. Conecte-se com sua companhia aérea para sincronizar seu histórico de voos."
            )}
          </span>
        </Card>
      )}

      {error && !localMode && (
        <Card className="flex items-center justify-between gap-3 border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
          <div className="flex items-center gap-2">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{t(error, "Não foi possível carregar os dados de voos.")}</span>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((r) => r + 1)}
            className="h-7 text-xs border-destructive/30 text-destructive hover:bg-destructive/10"
          >
            {t("myFlights.retry", "Tentar novamente")}
          </Button>
        </Card>
      )}

      {/* Pilot Statistics KPI Cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        {/* Pilot Profile Card */}
        <Card className="relative overflow-hidden border-border/70 bg-card p-3 shadow-xs">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 border border-primary/20 text-primary">
              {pilot?.rank_image_url ? (
                <img
                  src={pilot.rank_image_url}
                  alt={pilot.rank}
                  className="h-7 w-7 object-contain"
                />
              ) : (
                <User className="h-5 w-5" />
              )}
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-[11px] font-medium text-muted-foreground truncate">
                {pilot?.rank || t("myFlights.pilot", "Piloto")}
              </p>
              <h3 className="text-sm font-bold text-foreground truncate">
                {pilot?.callsign || pilot?.name || "—"}
              </h3>
              {pilot?.points ? (
                <span className="text-[10px] text-primary font-medium">{pilot.points} pts</span>
              ) : null}
            </div>
          </div>
        </Card>

        {/* Total Flights Card */}
        <Card className="relative overflow-hidden border-border/70 bg-card p-3 shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">
              {t("myFlights.totalFlights", "Total de Voos")}
            </span>
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-sky-500/10 text-sky-400">
              <PlaneTakeoff className="h-3.5 w-3.5" />
            </div>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-xl font-extrabold tracking-tight text-foreground">
              {pilot?.total_flights ?? flights.length}
            </span>
            <span className="text-[10px] text-muted-foreground">voos</span>
          </div>
        </Card>

        {/* Total Hours Card */}
        <Card className="relative overflow-hidden border-border/70 bg-card p-3 shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">
              {t("myFlights.totalHours", "Horas de Voo")}
            </span>
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-indigo-500/10 text-indigo-400">
              <Clock className="h-3.5 w-3.5" />
            </div>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-xl font-extrabold tracking-tight text-foreground">
              {pilot?.total_hours != null ? `${pilot.total_hours.toFixed(1)}` : "—"}
            </span>
            <span className="text-[10px] text-muted-foreground">h</span>
          </div>
        </Card>

        {/* Average Landing Rate Card */}
        <Card className="relative overflow-hidden border-border/70 bg-card p-3 shadow-xs">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">
              {t("myFlights.avgLanding", "Média de Pouso")}
            </span>
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-400">
              <TrendingDown className="h-3.5 w-3.5" />
            </div>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-xl font-extrabold tracking-tight text-foreground font-mono">
              {pilot?.avg_landing_rate ? `${Math.round(pilot.avg_landing_rate)}` : "—"}
            </span>
            <span className="text-[10px] text-muted-foreground">fpm</span>
          </div>
        </Card>

        {/* Total Distance Card */}
        <Card className="relative overflow-hidden border-border/70 bg-card p-3 shadow-xs col-span-2 sm:col-span-1">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-medium text-muted-foreground">
              {t("myFlights.totalDistance", "Distância Total")}
            </span>
            <div className="flex h-6 w-6 items-center justify-center rounded-md bg-amber-500/10 text-amber-400">
              <Globe className="h-3.5 w-3.5" />
            </div>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-xl font-extrabold tracking-tight text-foreground">
              {pilot?.total_distance_nm
                ? Math.round(pilot.total_distance_nm).toLocaleString()
                : "—"}
            </span>
            <span className="text-[10px] text-muted-foreground">NM</span>
          </div>
        </Card>
      </div>

      {/* Interactive Map Panel */}
      <div className="relative h-80 sm:h-96 w-full shrink-0">
        <FlightsMap
          flights={filteredFlights}
          selectedFlightId={selectedFlightId}
          onSelectFlight={setSelectedFlightId}
        />
      </div>

      {/* Selected Flight Quick Summary Bar */}
      {selectedFlight && (
        <Card className="flex flex-wrap items-center justify-between gap-3 border-primary/20 bg-primary/5 p-3 shadow-xs">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary/15 text-primary">
              <Plane className="h-4 w-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-foreground text-sm">
                  {selectedFlight.callsign}
                </span>
                {selectedFlight.flight_number && (
                  <span className="text-xs text-muted-foreground font-mono">
                    (Voo {selectedFlight.flight_number})
                  </span>
                )}
                {getFlightStatusBadge(selectedFlight.status)}
              </div>
              <p className="text-xs text-muted-foreground">
                <span className="font-semibold text-foreground">
                  {selectedFlight.departure_airport?.icao}
                </span>{" "}
                ({selectedFlight.departure_airport?.city || selectedFlight.departure_airport?.name}){" "}
                ➔{" "}
                <span className="font-semibold text-foreground">
                  {selectedFlight.arrival_airport?.icao}
                </span>{" "}
                ({selectedFlight.arrival_airport?.city || selectedFlight.arrival_airport?.name})
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-4 text-xs">
            {selectedFlight.aircraft && (
              <div>
                <span className="text-muted-foreground">Aeronave: </span>
                <span className="font-medium text-foreground">
                  {selectedFlight.aircraft.registration}{" "}
                  {selectedFlight.aircraft.icao_code ? `(${selectedFlight.aircraft.icao_code})` : ""}
                </span>
              </div>
            )}
            <div>
              <span className="text-muted-foreground">Tempo: </span>
              <span className="font-medium text-foreground">
                {formatDuration(selectedFlight.flight_time_minutes)}
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Distância: </span>
              <span className="font-medium text-foreground">
                {Math.round(selectedFlight.distance_nm)} NM
              </span>
            </div>
            <div>
              <span className="text-muted-foreground">Pouso: </span>
              {getLandingRateBadge(selectedFlight.landing_rate_fpm)}
            </div>
          </div>
        </Card>
      )}

      {/* Flight History Logbook Table */}
      <div className="flex-1 overflow-hidden rounded-xl border border-border/70 bg-card shadow-xs">
        <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold text-foreground">
              {t("myFlights.historyTitle", "Histórico de Voos")}
            </h3>
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              {filteredFlights.length} {filteredFlights.length === 1 ? "registro" : "registros"}
            </Badge>
          </div>
          {search && (
            <span className="text-xs text-muted-foreground">
              Filtrado por: <span className="font-medium text-foreground">"{search}"</span>
            </span>
          )}
        </div>

        <div className="overflow-x-auto max-h-96">
          <table className="w-full text-left text-xs">
            <thead className="sticky top-0 z-10 border-b border-border/60 bg-muted/60 text-muted-foreground backdrop-blur-xs">
              <tr>
                <th className="py-2.5 px-3 font-semibold">Callsign</th>
                <th className="py-2.5 px-3 font-semibold">Rota</th>
                <th className="py-2.5 px-3 font-semibold">Aeronave</th>
                <th className="py-2.5 px-3 font-semibold">Duração</th>
                <th className="py-2.5 px-3 font-semibold">Distância</th>
                <th className="py-2.5 px-3 font-semibold">Pouso</th>
                <th className="py-2.5 px-3 font-semibold">Status</th>
                <th className="py-2.5 px-3 font-semibold">Data</th>
                <th className="py-2.5 px-3 text-right font-semibold">Ação</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/40">
              {loading ? (
                <tr>
                  <td colSpan={9} className="py-12 text-center text-muted-foreground">
                    <Loader2 className="mx-auto h-6 w-6 animate-spin text-primary" />
                    <p className="mt-2 text-xs">Carregando histórico de voos...</p>
                  </td>
                </tr>
              ) : filteredFlights.length === 0 ? (
                <tr>
                  <td colSpan={9} className="py-10 text-center text-muted-foreground">
                    <p className="text-sm font-medium">Nenhum voo encontrado</p>
                    <p className="text-xs text-muted-foreground/75 mt-0.5">
                      {search
                        ? "Tente buscar com outros termos de pesquisa."
                        : "Você ainda não possui voos registrados nesta companhia."}
                    </p>
                  </td>
                </tr>
              ) : (
                filteredFlights.map((flight) => {
                  const isSelected = flight.id === selectedFlightId;
                  return (
                    <tr
                      key={flight.id}
                      onClick={() => setSelectedFlightId(flight.id)}
                      className={`cursor-pointer transition-colors ${
                        isSelected
                          ? "bg-accent/80 font-medium"
                          : "hover:bg-accent/40 text-foreground"
                      }`}
                    >
                      <td className="py-2.5 px-3 whitespace-nowrap">
                        <div className="flex items-center gap-1.5">
                          <span className="font-bold text-sky-400">{flight.callsign}</span>
                          {flight.flight_number && (
                            <span className="text-[10px] text-muted-foreground font-mono">
                              #{flight.flight_number}
                            </span>
                          )}
                        </div>
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap">
                        <div className="flex items-center gap-1 font-mono text-[11px]">
                          <span className="font-bold text-foreground">
                            {flight.departure_airport?.icao}
                          </span>
                          <span className="text-muted-foreground">➔</span>
                          <span className="font-bold text-foreground">
                            {flight.arrival_airport?.icao}
                          </span>
                        </div>
                        <div className="text-[10px] text-muted-foreground truncate max-w-[140px]">
                          {flight.departure_airport?.city || flight.departure_airport?.name}
                        </div>
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap">
                        <div className="text-[11px] font-medium text-foreground">
                          {flight.aircraft?.registration || "—"}
                        </div>
                        <div className="text-[10px] text-muted-foreground font-mono">
                          {flight.aircraft?.icao_code || "A/C"}
                        </div>
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap font-medium text-foreground">
                        {formatDuration(flight.flight_time_minutes)}
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap text-muted-foreground font-mono">
                        {Math.round(flight.distance_nm)} NM
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap">
                        {getLandingRateBadge(flight.landing_rate_fpm)}
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap">
                        {getFlightStatusBadge(flight.status)}
                      </td>

                      <td className="py-2.5 px-3 whitespace-nowrap text-muted-foreground text-[11px]">
                        {formatDate(flight.created_at || flight.departure_time)}
                      </td>

                      <td className="py-2.5 px-3 text-right whitespace-nowrap">
                        <Button
                          variant={isSelected ? "default" : "outline"}
                          size="icon-xs"
                          title="Focar no mapa"
                          onClick={(e) => {
                            e.stopPropagation();
                            setSelectedFlightId(flight.id);
                          }}
                          className="h-6 w-6"
                        >
                          <Navigation className="h-3 w-3" />
                        </Button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
