import { useEffect, useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertTriangle,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Clock,
  Eye,
  Fuel,
  Globe,
  Map,
  Plane,
  PlaneTakeoff,
  RefreshCw,
  Search,
  ShieldCheck,
  TrendingDown,
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
import { FlightsMap } from "@/components/flights-map";

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

function formatUtcDate(val?: string): string {
  if (!val) return "—";
  const d = new Date(val);
  if (Number.isNaN(d.getTime())) return "—";
  const day = String(d.getUTCDate()).padStart(2, "0");
  const month = String(d.getUTCMonth() + 1).padStart(2, "0");
  const hours = String(d.getUTCHours()).padStart(2, "0");
  const mins = String(d.getUTCMinutes()).padStart(2, "0");
  return `${day}/${month} ${hours}:${mins}z`;
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
        className="text-[10px] py-0 px-1.5 border-emerald-500/30 text-emerald-400 bg-emerald-500/10 font-mono"
      >
        {Math.round(rate)} ft/min
      </Badge>
    );
  }
  if (abs <= 300) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-sky-500/30 text-sky-400 bg-sky-500/10 font-mono"
      >
        {Math.round(rate)} ft/min
      </Badge>
    );
  }
  if (abs <= 450) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-1.5 border-amber-500/30 text-amber-400 bg-amber-500/10 font-mono"
      >
        {Math.round(rate)} ft/min
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="text-[10px] py-0 px-1.5 border-red-500/30 text-red-400 bg-red-500/10 font-mono"
    >
      {Math.round(rate)} ft/min
    </Badge>
  );
}

export function normalizeFlightStatus(status?: string): string {
  if (!status || !status.trim()) return "unknown";
  return status.trim().toLowerCase();
}

export function getFlightStatusBadge(status: string | undefined, t: (key: string, fallback: string) => string) {
  const norm = normalizeFlightStatus(status);
  if (norm === "unknown") {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-border text-muted-foreground bg-muted/20 font-medium"
      >
        —
      </Badge>
    );
  }
  if (norm.includes("accept") || norm.includes("approved") || norm.includes("closed")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-emerald-500/30 text-emerald-400 bg-emerald-500/10 font-medium"
      >
        {t("myFlights.statusApproved", "Aprovado")}
      </Badge>
    );
  }
  if (norm.includes("pending") || norm.includes("review")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-amber-500/30 text-amber-400 bg-amber-500/10 font-medium"
      >
        {t("myFlights.statusPending", "Pendente")}
      </Badge>
    );
  }
  if (norm.includes("reject") || norm.includes("denied")) {
    return (
      <Badge
        variant="outline"
        className="text-[10px] py-0 px-2 border-red-500/30 text-red-400 bg-red-500/10 font-medium"
      >
        {t("myFlights.statusRejected", "Rejeitado")}
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="text-[10px] py-0 px-2 border-red-500/30 text-red-400 bg-red-500/10 font-medium"
    >
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
  const { t } = useTranslation();
  const [flights, setFlights] = useState<FlightLog[]>([]);
  const [pilotStats, setPilotStats] = useState<PilotSummaryStats | null>(null);
  const [statusFilter, setStatusFilter] = useState<"all" | "accepted" | "pending">("all");
  const [errorStatus, setErrorStatus] = useState<string | null>(null);

  const [page, setPage] = useState(1);
  const [lastPage, setLastPage] = useState(1);
  const [totalFlights, setTotalFlights] = useState(0);

  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [inspectFlight, setInspectFlight] = useState<FlightLog | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(!localMode && ready);

    if (localMode || !ready) {
      setFlights([]);
      setPilotStats(null);
      setErrorStatus(null);
      setLoading(false);
      setLastPage(1);
      setTotalFlights(0);
      return;
    }

    FlightLogService.GetMyFlights(page, 50)
      .then((res: any) => {
        if (cancelled) return;
        if (res?.status && res.status !== "ok") {
          if (res.status === "localMode" || res.status === "noSession") {
            setErrorStatus(null);
            setFlights([]);
            setPilotStats(null);
            setLastPage(1);
            setTotalFlights(0);
            return;
          }
          setErrorStatus(res.status);
          setFlights([]);
          setPilotStats(null);
          setLastPage(1);
          setTotalFlights(0);
          return;
        }
        setErrorStatus(null);
        if (res?.flights) {
          setFlights(res.flights);
        } else {
          setFlights([]);
        }
        if (res?.pilot) {
          setPilotStats(res.pilot);
        } else {
          setPilotStats(null);
        }
        const lp = res?.last_page || 1;
        setLastPage(lp);
        setTotalFlights(res?.total ?? res?.pilot?.total_flights ?? (res?.flights?.length || 0));
      })
      .catch(() => {
        if (!cancelled) {
          setErrorStatus("loadError");
          setFlights([]);
          setPilotStats(null);
          setLastPage(1);
          setTotalFlights(0);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [localMode, ready, refresh, page]);

  // Filter flights by status and search query
  const filteredFlights = useMemo(() => {
    let list = flights;

    if (statusFilter === "accepted") {
      list = list.filter((f) => {
        const s = normalizeFlightStatus(f.status);
        return s.includes("accept") || s.includes("approved") || s.includes("closed");
      });
    } else if (statusFilter === "pending") {
      list = list.filter((f) => {
        const s = normalizeFlightStatus(f.status);
        return s.includes("pending") || s.includes("review");
      });
    }

    if (!search.trim()) return list;
    const term = search.toLowerCase().trim();
    return list.filter((f) => {
      const callsign = (f.callsign || "").toLowerCase();
      const fltNum = (f.flight_number || "").toLowerCase();
      const depIcao = (f.departure_airport?.icao || "").toLowerCase();
      const depCity = (f.departure_airport?.city || "").toLowerCase();
      const arrIcao = (f.arrival_airport?.icao || "").toLowerCase();
      const arrCity = (f.arrival_airport?.city || "").toLowerCase();
      const acCode = (f.aircraft?.icao_code || "").toLowerCase();
      const acReg = (f.aircraft?.registration || "").toLowerCase();

      return (
        callsign.includes(term) ||
        fltNum.includes(term) ||
        depIcao.includes(term) ||
        depCity.includes(term) ||
        arrIcao.includes(term) ||
        arrCity.includes(term) ||
        acCode.includes(term) ||
        acReg.includes(term)
      );
    });
  }, [flights, statusFilter, search]);

  // Calculate live average score from flights
  const avgScore = useMemo(() => {
    const scored = flights.filter((f) => f.score != null && f.score > 0);
    if (scored.length === 0) return null;
    const total = scored.reduce((acc, f) => acc + f.score, 0);
    return (total / scored.length).toFixed(1);
  }, [flights]);

  // If a specific flight is selected for inspection, show the full-screen view inside the tab
  if (inspectFlight) {
    return (
      <FlightDetailView
        flight={inspectFlight}
        company={company}
        onBack={() => setInspectFlight(null)}
      />
    );
  }

  return (
    <div className="flex h-full flex-col space-y-4">
      {/* Error Alert Banner */}
      {errorStatus && (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-400">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 shrink-0 text-red-400" />
            <span>
              {errorStatus === "accessDenied"
                ? t(
                    "myFlights.accessDenied",
                    "Sua companhia aérea não autorizou o acesso ao seu histórico de voos."
                  )
                : errorStatus === "rateLimited"
                ? t(
                    "myFlights.rateLimited",
                    "Muitas solicitações. Aguarde um momento antes de atualizar."
                  )
                : errorStatus === "unavailable"
                ? t(
                    "myFlights.unavailable",
                    "O histórico de voos não está disponível na sua companhia no momento."
                  )
                : t(
                    "myFlights.loadError",
                    "Não foi possível carregar os dados de voos. Verifique sua conexão."
                  )}
            </span>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((r) => r + 1)}
            disabled={loading}
            className="h-7 text-xs border-red-500/30 text-red-300 hover:bg-red-500/20"
          >
            {t("myFlights.retry", "Tentar novamente")}
          </Button>
        </div>
      )}

      {/* Header bar strictly following the ACARS Flux UI design system */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-xl font-semibold tracking-tight text-foreground">
              {t("myFlights.title", "Meus Voos")}
            </h2>
            {company && (
              <Badge variant="outline" className="text-xs border-border bg-card/60">
                {company}
              </Badge>
            )}
            <Badge variant="outline" className="text-[10px] text-muted-foreground border-border/60">
              {filteredFlights.length} {filteredFlights.length === 1 ? t("myFlights.flightSingular", "voo") : t("myFlights.flightPlural", "voos")}
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(
              "myFlights.subtitle",
              "Histórico de voos realizados, estatísticas e auditoria operacional."
            )}
          </p>
        </div>

        {/* Action Controls & Filters */}
        <div className="flex flex-wrap items-center gap-2">
          {/* Status Filter Segmented Buttons */}
          <div className="flex items-center gap-1 rounded-lg border border-border/60 bg-muted/30 p-0.5">
            <Button
              variant={statusFilter === "all" ? "default" : "ghost"}
              size="sm"
              onClick={() => setStatusFilter("all")}
              className="h-7 text-xs px-2.5"
            >
              {t("myFlights.all", "Todos")} ({flights.length})
            </Button>
            <Button
              variant={statusFilter === "accepted" ? "default" : "ghost"}
              size="sm"
              onClick={() => setStatusFilter("accepted")}
              className="h-7 text-xs px-2.5"
            >
              {t("myFlights.accepted", "Aprovados")}
            </Button>
            <Button
              variant={statusFilter === "pending" ? "default" : "ghost"}
              size="sm"
              onClick={() => setStatusFilter("pending")}
              className="h-7 text-xs px-2.5"
            >
              {t("myFlights.pending", "Pendentes")}
            </Button>
          </div>



          {/* Search Box */}
          <div className="relative w-44 sm:w-56">
            <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              type="text"
              placeholder={t("myFlights.searchPlaceholder", "Buscar por voo, aeroporto, aeronave...")}
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

          {/* Refresh Button */}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((r) => r + 1)}
            disabled={loading}
            className="h-8 gap-1.5 text-xs border-border/80 shadow-xs"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            <span className="hidden sm:inline">{t("myFlights.refresh", "Atualizar")}</span>
          </Button>
        </div>
      </div>

      {/* Top Pilot KPI Metrics Cards in Flux UI style */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        <Card className="border-border/70 bg-card p-3 shadow-xs">
          <span className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <PlaneTakeoff className="h-3.5 w-3.5 text-primary" />
            {t("myFlights.totalFlights", "Total de Voos")}
          </span>
          <p className="mt-1 text-xl font-bold font-mono text-foreground">
            {pilotStats?.total_flights ?? flights.length}
          </p>
        </Card>

        <Card className="border-border/70 bg-card p-3 shadow-xs">
          <span className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <Clock className="h-3.5 w-3.5 text-sky-400" />
            {t("myFlights.totalHours", "Horas de Voo")}
          </span>
          <p className="mt-1 text-xl font-bold font-mono text-foreground">
            {pilotStats?.total_hours != null && pilotStats.total_hours > 0
              ? `${pilotStats.total_hours.toFixed(1)}h`
              : (flights.length > 0 ? "0h" : "—")}
          </p>
        </Card>

        <Card className="border-border/70 bg-card p-3 shadow-xs">
          <span className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <TrendingDown className="h-3.5 w-3.5 text-emerald-400" />
            {t("myFlights.avgLanding", "Média de Pouso")}
          </span>
          <p className="mt-1 text-xl font-bold font-mono text-emerald-400">
            {pilotStats?.avg_landing_rate ? (
              <>
                {Math.round(pilotStats.avg_landing_rate)}{" "}
                <span className="text-xs font-normal text-muted-foreground font-sans">ft/min</span>
              </>
            ) : (
              "—"
            )}
          </p>
        </Card>

        <Card className="border-border/70 bg-card p-3 shadow-xs">
          <span className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <Globe className="h-3.5 w-3.5 text-indigo-400" />
            {pilotStats?.has_global_distance || (pilotStats?.total_flights ?? flights.length) <= flights.length
              ? t("myFlights.totalDistance", "Distância Total")
              : t("myFlights.pageDistance", "Distância da Página")}
          </span>
          <p className="mt-1 text-xl font-bold font-mono text-foreground">
            {pilotStats?.total_distance_nm ? (
              <>
                {pilotStats.total_distance_nm.toLocaleString()}{" "}
                <span className="text-xs font-normal text-muted-foreground font-sans">NM</span>
              </>
            ) : (
              flights.length > 0 ? "0 NM" : "—"
            )}
          </p>
        </Card>

        <Card className="border-border/70 bg-card p-3 shadow-xs col-span-2 sm:col-span-1">
          <span className="text-[11px] font-medium text-muted-foreground flex items-center gap-1.5">
            <ShieldCheck className="h-3.5 w-3.5 text-emerald-400" />
            {t("myFlights.avgScore", "Score Médio")}
          </span>
          <p className="mt-1 text-xl font-bold font-mono text-emerald-400">
            {avgScore ? (
              <>
                {avgScore}{" "}
                <span className="text-xs font-normal text-muted-foreground font-sans">pts</span>
              </>
            ) : (
              "—"
            )}
          </p>
        </Card>
      </div>

      {/* Flights Logbook Table */}
      <Card className="flex-1 overflow-hidden border-border/70 bg-card p-0 shadow-xs">
          <div className="overflow-x-auto max-h-[calc(100vh-275px)] overflow-y-auto">
            <table className="w-full text-left text-xs border-collapse">
              <thead className="sticky top-0 z-10 border-b border-border/60 bg-muted/60 text-muted-foreground backdrop-blur-xs font-semibold">
                <tr>
                  <th className="py-2.5 px-3.5">{t("myFlights.colFlight", "Voo")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colOrigin", "Origem")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colDestination", "Destino")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colDeparture", "Partida (UTC)")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colAircraft", "Aeronave")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colDuration", "Duração")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colDistance", "Distância")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colLanding", "Toque")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colScore", "Score")}</th>
                  <th className="py-2.5 px-3">{t("myFlights.colStatus", "Status")}</th>
                  <th className="py-2.5 px-3 text-right">{t("myFlights.colAction", "Ação")}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/40 text-foreground">
                {filteredFlights.length === 0 ? (
                  <tr>
                    <td colSpan={11} className="py-12 text-center text-muted-foreground">
                      <p className="text-sm font-medium">{t("myFlights.noFlightsFound", "Nenhum voo encontrado")}</p>
                      <p className="text-xs text-muted-foreground/75 mt-0.5">
                        {localMode
                          ? t("myFlights.localMode", "O Airspace ACARS está em Modo Local. Conecte-se com sua companhia aérea para sincronizar seu histórico de voos.")
                          : !ready
                          ? t("myFlights.noSession", "Faça login com sua companhia aérea para acessar seu histórico de voos.")
                          : search
                          ? t("myFlights.noSearchResults", "Tente buscar com outros termos de pesquisa.")
                          : t("myFlights.noFilterResults", "Nenhum registro para o filtro selecionado.")}
                      </p>
                    </td>
                  </tr>
                ) : (
                  filteredFlights.map((flight) => {
                    return (
                      <tr
                        key={flight.id}
                        className="hover:bg-accent/40 transition-colors group cursor-pointer"
                        onClick={() => setInspectFlight(flight)}
                      >
                        {/* Flight Callsign */}
                        <td className="py-2.5 px-3.5 whitespace-nowrap">
                          <span className="font-mono font-bold text-foreground tracking-tight">
                            {flight.callsign}
                          </span>
                        </td>

                        {/* Origin Airport */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          <div className="font-mono font-bold text-foreground">
                            {flight.departure_airport?.icao}
                          </div>
                          <div className="text-[10px] text-muted-foreground truncate max-w-[130px]">
                            {flight.departure_airport?.city || flight.departure_airport?.name}
                          </div>
                        </td>

                        {/* Destination Airport */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          <div className="font-mono font-bold text-foreground">
                            {flight.arrival_airport?.icao}
                          </div>
                          <div className="text-[10px] text-muted-foreground truncate max-w-[130px]">
                            {flight.arrival_airport?.city || flight.arrival_airport?.name}
                          </div>
                        </td>

                        {/* Departure Time in UTC */}
                        <td className="py-2.5 px-3 whitespace-nowrap font-mono text-muted-foreground text-[11px]">
                          {formatUtcDate(flight.departure_time || flight.created_at)}
                        </td>

                        {/* Aircraft */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          <div className="font-medium text-foreground text-[11px]">
                            {flight.aircraft?.registration || "—"}
                          </div>
                          <div className="font-mono text-[10px] text-muted-foreground">
                            {flight.aircraft?.icao_code || "A/C"}
                          </div>
                        </td>

                        {/* Duration */}
                        <td className="py-2.5 px-3 whitespace-nowrap font-medium text-foreground">
                          {formatDuration(flight.flight_time_minutes)}
                        </td>

                        {/* Distance */}
                        <td className="py-2.5 px-3 whitespace-nowrap font-mono text-muted-foreground">
                          {Math.round(flight.distance_nm)} NM
                        </td>

                        {/* Touchdown Landing Rate */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          {getLandingRateBadge(flight.landing_rate_fpm)}
                        </td>

                        {/* Operational Score */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          <Badge variant="outline" className="text-[10px] font-mono border-border text-foreground">
                            {flight.score != null && flight.score > 0 ? `${flight.score} pts` : "—"}
                          </Badge>
                        </td>

                        {/* Status */}
                        <td className="py-2.5 px-3 whitespace-nowrap">
                          {getFlightStatusBadge(flight.status, t)}
                        </td>

                        {/* Action - Eye Button */}
                        <td className="py-2.5 px-3 text-right whitespace-nowrap">
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            title={t("myFlights.viewFlightDetails", "Ver status e estatísticas do voo")}
                            onClick={(e) => {
                              e.stopPropagation();
                              setInspectFlight(flight);
                            }}
                            className="h-7 w-7 text-muted-foreground hover:text-foreground hover:bg-accent border border-border/50"
                          >
                            <Eye className="h-3.5 w-3.5" />
                          </Button>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination Controls */}
          {(lastPage > 1 || totalFlights > 50) && (
            <div className="flex items-center justify-between border-t border-border/60 bg-muted/20 px-3.5 py-2 text-xs">
              <span className="text-muted-foreground">
                {t("myFlights.pageInfo", {
                  page,
                  lastPage,
                  defaultValue: `Página ${page} de ${lastPage} (50 por página)`,
                })}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1 || loading}
                  className="h-7 px-2.5 text-xs gap-1"
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                  <span>{t("myFlights.paginationPrevious", "Anterior")}</span>
                </Button>
                <span className="font-mono text-xs text-muted-foreground px-1">
                  {t("myFlights.paginationPageOf", {
                    page,
                    lastPage,
                    defaultValue: `Página ${page} de ${lastPage}`,
                  })}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.min(lastPage, p + 1))}
                  disabled={page >= lastPage || loading}
                  className="h-7 px-2.5 text-xs gap-1"
                >
                  <span>{t("myFlights.paginationNext", "Próximo")}</span>
                  <ChevronRight className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          )}
      </Card>
    </div>
  );
}

const noopSelectFlight = () => {};

/**
 * Dedicated Full-Page View for a specific flight, covering the entire tab area.
 * Displays the route map, telemetry KPI metrics, and operational audit report.
 */
function FlightDetailView({
  flight,
  company,
  onBack,
}: {
  flight: FlightLog;
  company: string;
  onBack: () => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex h-full flex-col space-y-3 animate-in fade-in duration-200">
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
              <span>
                {flight.departure_airport?.icao} ({flight.departure_airport?.city})
              </span>
              <span className="text-primary">➔</span>
              <span>
                {flight.arrival_airport?.icao} ({flight.arrival_airport?.city})
              </span>
            </h2>
            <p className="text-xs text-muted-foreground">
              {company || "Airspace"} • {t("myFlights.departureLabel", "Partida")}: {formatUtcDate(flight.departure_time || flight.created_at)}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {getFlightStatusBadge(flight.status, t)}
          <span className="text-xs font-mono text-muted-foreground">
            {Math.round(flight.distance_nm)} NM
          </span>
        </div>
      </div>

      {/* Main Expansive Layout: Big Map on Left, Metrics & Audit on Right */}
      <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1.35fr)_400px] gap-3 flex-1 min-h-0">
        {/* Left Column: Big, Tall, Immersive Interactive Map filling 100% height and top area */}
        <div className="relative flex-1 w-full min-h-[480px] lg:min-h-0 h-full rounded-xl overflow-hidden border border-border/70 bg-card shadow-xs">
          <FlightsMap
            flights={[flight]}
            selectedFlightId={flight.id}
            onSelectFlight={noopSelectFlight}
            className="h-full w-full"
            overlayContent={
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold text-foreground flex items-center gap-1.5">
                  <Map className="h-3.5 w-3.5 text-primary shrink-0" />
                  {t("myFlights.routeLabel", "Rota")}: {flight.departure_airport?.icao} ({flight.departure_airport?.city}) ➔ {flight.arrival_airport?.icao} ({flight.arrival_airport?.city})
                </span>
                <span className="font-mono text-xs text-muted-foreground">
                  • {Math.round(flight.distance_nm)} NM • {flight.aircraft?.icao_code} ({flight.aircraft?.registration})
                </span>
              </div>
            }
          />
        </div>

        {/* Right Column: Telemetry Cards & Audit Report */}
        <div className="flex flex-col space-y-3 overflow-y-auto pr-0.5">
          {/* 6 Key Telemetry KPIs in 2x3 Grid */}
          <div className="grid grid-cols-2 gap-2.5">
            {/* Flight Duration */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <Clock className="h-3.5 w-3.5 text-sky-400" />
                {t("myFlights.flightDuration", "Duração de Voo")}
              </span>
              <p className="text-lg font-bold font-mono text-foreground mt-1">
                {formatDuration(flight.flight_time_minutes)}
              </p>
              <span className="text-[10px] text-muted-foreground">
                {flight.flight_time_minutes} {t("myFlights.minutesFlown", "min voados")}
              </span>
            </Card>

            {/* Touchdown Landing Rate */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <TrendingDown className="h-3.5 w-3.5 text-emerald-400" />
                {t("myFlights.touchdownRate", "Toque na Pista")}
              </span>
              <p className="text-lg font-bold font-mono text-emerald-400 mt-1">
                {flight.landing_rate_fpm && flight.landing_rate_fpm !== 0
                  ? `${Math.round(flight.landing_rate_fpm)} ft/min`
                  : "—"}
              </p>
              <span className="text-[10px] text-muted-foreground">
                {flight.landing_rate_fpm && flight.landing_rate_fpm !== 0 ? (
                  Math.abs(flight.landing_rate_fpm) <= 180
                    ? t("myFlights.butterLanding", "Toque Suave (Butter)")
                    : t("myFlights.normalLanding", "Toque Operacional")
                ) : (
                  "—"
                )}
              </span>
            </Card>

            {/* Distance Flown */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <Globe className="h-3.5 w-3.5 text-indigo-400" />
                {t("myFlights.totalFlownDistance", "Distância Total")}
              </span>
              <p className="text-lg font-bold font-mono text-foreground mt-1">
                {flight.distance_nm && flight.distance_nm > 0
                  ? `${Math.round(flight.distance_nm)} NM`
                  : "—"}
              </p>
              <span className="text-[10px] text-muted-foreground">
                {flight.distance_nm && flight.distance_nm > 0
                  ? `~${Math.round(flight.distance_nm * 1.852)} km`
                  : "—"}
              </span>
            </Card>

            {/* Fuel Consumed */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <Fuel className="h-3.5 w-3.5 text-amber-400" />
                {t("myFlights.fuelBurned", "Combustível")}
              </span>
              <p className="text-lg font-bold font-mono text-foreground mt-1">
                {flight.fuel_used_kg != null && flight.fuel_used_kg > 0
                  ? `${Math.round(flight.fuel_used_kg).toLocaleString()} kg`
                  : "—"}
              </p>
              <span className="text-[10px] text-muted-foreground">{t("myFlights.tripFuel", "Trip Fuel")}</span>
            </Card>

            {/* Aircraft Used */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <Plane className="h-3.5 w-3.5 text-purple-400" />
                {t("myFlights.aircraftUsed", "Aeronave")}
              </span>
              <p className="text-base font-bold font-mono text-foreground mt-1 truncate">
                {flight.aircraft?.registration || "—"}
              </p>
              <span className="text-[10px] text-muted-foreground font-mono truncate block">
                {flight.aircraft?.icao_code} {flight.aircraft?.name ? `(${flight.aircraft.name})` : ""}
              </span>
            </Card>

            {/* Operational Score */}
            <Card className="border-border/70 bg-card p-3 shadow-xs">
              <span className="text-[11px] text-muted-foreground font-medium flex items-center gap-1.5">
                <ShieldCheck className="h-3.5 w-3.5 text-emerald-400" />
                {t("myFlights.operationalScore", "Score Operacional")}
              </span>
              <p className="text-lg font-bold font-mono text-emerald-400 mt-1">
                {flight.score != null && flight.score > 0 ? `${flight.score} / 100` : "—"}
              </p>
              <span className="text-[10px] text-muted-foreground">
                {flight.score != null && flight.score > 0
                  ? t("myFlights.scoreRecorded", "Score Registrado")
                  : t("myFlights.noScore", "Sem pontuação")}
              </span>
            </Card>
          </div>

          {/* Flight Summary */}
          <Card className="border-border/70 bg-card p-4 space-y-2.5 shadow-xs flex-1">
            <h4 className="text-xs font-semibold text-foreground flex items-center gap-2">
              <Plane className="h-4 w-4 text-primary" />
              {t("myFlights.flightSummary", "Resumo do Voo")}
            </h4>
            <div className="space-y-1.5 text-xs text-muted-foreground leading-relaxed">
              <p>
                {t("myFlights.departureLabel", "Partida")}:{" "}
                <span className="text-foreground font-semibold font-mono">
                  {flight.departure_airport?.icao}
                </span>{" "}
                ({flight.departure_airport?.city || flight.departure_airport?.name || "—"}) —{" "}
                {formatUtcDate(flight.departure_time || flight.created_at)}
              </p>
              <p>
                {t("myFlights.destinationLabel", "Destino")}:{" "}
                <span className="text-foreground font-semibold font-mono">
                  {flight.arrival_airport?.icao}
                </span>{" "}
                ({flight.arrival_airport?.city || flight.arrival_airport?.name || "—"}) —{" "}
                {formatUtcDate(flight.arrival_time)}
              </p>
              <p>
                {t("myFlights.flightDuration", "Duração de Voo")}:{" "}
                <span className="text-foreground font-medium">
                  {formatDuration(flight.flight_time_minutes)}
                </span>{" "}
                ({Math.round(flight.distance_nm)} NM)
              </p>
              {flight.landing_rate_fpm && flight.landing_rate_fpm !== 0 ? (
                <p>
                  {t("myFlights.touchdownRate", "Toque na Pista")}:{" "}
                  <span className="text-emerald-400 font-semibold font-mono">
                    {Math.round(flight.landing_rate_fpm)} ft/min
                  </span>
                </p>
              ) : null}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
