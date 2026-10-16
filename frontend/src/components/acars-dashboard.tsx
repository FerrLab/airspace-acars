import { useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  Activity,
  AlertTriangle,
  Check,
  CheckCircle2,
  Circle,
  Headphones,
  Loader2,
  Plane,
  Plug,
  RefreshCw,
  Square,
  Unplug,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Slider } from "@/components/ui/slider";
import { cn } from "@/lib/utils";
import type { FlightData } from "@/hooks/use-flight-data";
import { FlightProgressBar } from "@/components/flight-progress-bar";
import type { Coordinates } from "@/lib/flight-progress";

interface Props {
  localMode: boolean;
  connectedAdapter: string;
  connecting: boolean;
  flightState: "idle" | "active" | "finishing";
  booking: any;
  activeFlightInfo: {
    callsign?: string;
    departure?: string;
    arrival?: string;
    startTimeMs?: string;
    departureLat?: string;
    departureLon?: string;
    arrivalLat?: string;
    arrivalLon?: string;
  } | null;
  departureCoords?: Coordinates | null;
  arrivalCoords?: Coordinates | null;
  flightData: FlightData | null;
  onGround: boolean;
  groundSpeed: number;
  starting: boolean;
  ending: boolean;
  finishCooldown: number;
  finishPending: number | null;
  volume: number;
  refreshing: boolean;
  bookingError: boolean;
  error: string | null;
  onDismissError: () => void;
  onConnect: () => void;
  onDisconnect: () => void;
  onRefresh: () => void;
  onStart: () => void;
  onStop: () => void;
  onFinish: () => void;
  onCancelFinish: () => void;
  onVolumeChange: (volume: number) => void;
}

export function AcarsDashboard(p: Props) {
  const { t } = useTranslation();
  const lastVolume = useRef(25);
  if (p.volume > 0) lastVolume.current = p.volume;
  const connected = !!p.connectedAdapter;
  const active = !p.localMode && p.flightState !== "idle";
  const finishing = active && p.flightState === "finishing";
  const booking = !p.localMode ? p.booking : null;
  const route = active ? p.activeFlightInfo : {
    callsign: booking?.callsign ?? booking?.flight_number,
    departure: booking?.departure_airport?.icao,
    arrival: booking?.alternate_airport?.icao ?? booking?.arrival_airport?.icao,
  };
  const hasRoute = active || !!booking;

  const depCoords: Coordinates | null =
    p.activeFlightInfo?.departureLat && p.activeFlightInfo?.departureLon
      ? { lat: Number(p.activeFlightInfo.departureLat), lon: Number(p.activeFlightInfo.departureLon) }
      : p.departureCoords ?? null;

  const arrCoords: Coordinates | null =
    p.activeFlightInfo?.arrivalLat && p.activeFlightInfo?.arrivalLon
      ? { lat: Number(p.activeFlightInfo.arrivalLat), lon: Number(p.activeFlightInfo.arrivalLon) }
      : p.arrivalCoords ?? null;

  const data = connected ? p.flightData : null;
  const status = p.localMode
    ? "acars.localMode"
    : finishing
      ? "acars.finishing"
      : active
        ? "acars.flightActive"
        : booking
          ? "acars.activeBooking"
          : "acars.dashboard.standby";
  const checklist = [
    { label: t("acars.dashboard.simulator"), done: connected },
    { label: t("acars.activeBooking"), done: !!booking },
    { label: t("acars.dashboard.onGround"), done: connected && p.onGround },
    { label: t("acars.dashboard.stationary"), done: connected && p.groundSpeed < 1 },
  ];
  const ready = checklist.every((item) => item.done);

  return (
    <div className="acars-dashboard space-y-5">
      {/* Header section with page title and connection badge/button */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground">{t("acars.dashboard.title")}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t("acars.dashboard.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Badge
            variant="outline"
            className={cn(
              "px-3 py-1 font-medium transition-colors",
              connected
                ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/30"
                : "bg-muted text-muted-foreground border-border/80"
            )}
          >
            <span
              className={cn(
                "mr-2 h-2 w-2 rounded-full",
                connected ? "bg-emerald-500" : "bg-muted-foreground/60"
              )}
            />
            {connected
              ? t("acars.dashboard.connected", { adapter: p.connectedAdapter })
              : t("acars.dashboard.disconnected")}
          </Badge>
          <Button
            size="sm"
            variant={connected ? "outline" : "default"}
            onClick={connected ? p.onDisconnect : p.onConnect}
            disabled={p.connecting}
            className="shadow-xs"
          >
            {p.connecting ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : connected ? (
              <Unplug className="h-4 w-4" />
            ) : (
              <Plug className="h-4 w-4" />
            )}
            {p.connecting ? t("acars.connecting") : connected ? t("acars.disconnect") : t("acars.connect")}
          </Button>
        </div>
      </div>

      {p.error && (
        <div
          role="alert"
          aria-live="assertive"
          className="flex items-start justify-between gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-destructive shadow-xs"
        >
          <div className="flex items-start gap-3">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" aria-hidden="true" />
            <p className="text-xs font-medium leading-relaxed">{p.error}</p>
          </div>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t("common.dismissAlert")}
            onClick={p.onDismissError}
            className="text-destructive/80 hover:bg-destructive/15 hover:text-destructive"
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      )}

      {/* Main Grid: Active Flight & Route + Preflight Checklist */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Active Flight Card */}
        <Card className="gap-0 overflow-hidden border-border/80 bg-card shadow-xs lg:col-span-2">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/70 bg-muted/30 px-5 py-3.5">
            <Badge
              variant="outline"
              className={cn(
                "gap-1.5 px-2.5 py-1 text-xs font-medium transition-colors",
                active
                  ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/30"
                  : booking
                    ? "bg-sky-500/10 text-sky-500 border-sky-500/30"
                    : "bg-muted text-muted-foreground border-border/80"
              )}
            >
              {finishing ? (
                <Loader2 className="h-3 w-3 animate-spin motion-reduce:animate-none" />
              ) : (
                <span
                  className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    active ? "bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.5)]" : "bg-sky-500"
                  )}
                />
              )}
              {t(status)}
            </Badge>
            <span className="font-mono text-xs font-semibold tracking-wider px-2 py-0.5 rounded-md bg-muted/60 border border-border/60 text-foreground">
              {route?.callsign || "—"}
            </span>
          </div>

          <div className="px-5 py-6 sm:px-6">
            <div className="grid grid-cols-[auto_1fr_auto] items-center gap-4 sm:gap-6">
              <div className="min-w-20">
                <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{t("acars.departure")}</p>
                <p className="mt-1 font-mono text-3xl font-bold tracking-tight text-foreground sm:text-4xl">{route?.departure || "— — —"}</p>
                {!active && booking?.departure_airport?.city && (
                  <p className="mt-1 truncate text-xs font-medium text-muted-foreground">{booking.departure_airport.city}</p>
                )}
              </div>
              <div className="flex-1 px-1 sm:px-3">
                <FlightProgressBar
                  flightState={p.flightState}
                  departureCoords={depCoords}
                  arrivalCoords={arrCoords}
                  currentLat={data?.position?.latitude}
                  currentLon={data?.position?.longitude}
                  onGround={p.onGround}
                  groundSpeed={p.groundSpeed}
                  flightStartTime={p.activeFlightInfo?.startTimeMs}
                />
              </div>
              <div className="min-w-20 text-right">
                <p className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">{t("acars.arrival")}</p>
                <p className="mt-1 font-mono text-3xl font-bold tracking-tight text-foreground sm:text-4xl">{route?.arrival || "— — —"}</p>
                {!active && booking?.alternate_airport ? (
                  <p className="mt-1 text-xs font-medium text-amber-500">{t("acars.dashboard.diverted", { airport: booking.arrival_airport?.icao ?? "—" })}</p>
                ) : !active && booking?.arrival_airport?.city ? (
                  <p className="mt-1 truncate text-xs font-medium text-muted-foreground">{booking.arrival_airport.city}</p>
                ) : null}
              </div>
            </div>
            <div className="mt-5 flex items-center gap-2 text-xs text-muted-foreground">
              <Plane className="h-3.5 w-3.5 shrink-0 text-muted-foreground/70" aria-hidden="true" />
              <span className="truncate font-medium">{data?.aircraftName || t("acars.dashboard.awaitingAircraft")}</span>
            </div>
          </div>

          <div className="border-t border-border/70 bg-muted/25 px-5 py-4">
            {p.localMode ? (
              <p className="text-sm text-muted-foreground">{t("acars.localModeDesc")}</p>
            ) : finishing ? (
              <div className="space-y-3">
                <p role="status" className="text-sm font-medium text-foreground">
                  {p.finishPending !== null && p.finishPending > 0 ? t("acars.finishingDrain", { count: p.finishPending }) : t("acars.finishing")}
                </p>
                <Button size="sm" variant="outline" onClick={p.onCancelFinish} className="shadow-xs">
                  {t("acars.cancelFinish")}
                </Button>
              </div>
            ) : active ? (
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  onClick={p.onFinish}
                  disabled={p.ending || p.finishCooldown > 0}
                  className="shadow-xs"
                >
                  <CheckCircle2 className="h-4 w-4" />
                  {p.ending ? t("acars.finishing") : p.finishCooldown > 0 ? t("acars.finishCooldown", { seconds: p.finishCooldown }) : t("acars.finishFlight")}
                </Button>
                <Button
                  variant="ghost"
                  onClick={p.onStop}
                  disabled={p.ending}
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                >
                  <Square className="h-4 w-4" />
                  {t("acars.cancel")}
                </Button>
              </div>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="max-w-sm text-xs leading-relaxed text-muted-foreground">
                  {p.bookingError ? t("acars.dashboard.bookingError") : !connected ? t("acars.dashboard.connectHint") : !hasRoute ? t("acars.noBooking") : ready ? t("acars.dashboard.readyHint") : t("acars.groundRequired")}
                </p>
                <Button
                  onClick={p.onStart}
                  disabled={!ready || p.starting}
                  className="shadow-xs"
                >
                  {p.starting ? <Loader2 className="animate-spin motion-reduce:animate-none" /> : <Plane className="h-4 w-4" />}
                  {p.starting ? t("acars.starting") : t("acars.startFlight")}
                </Button>
              </div>
            )}
          </div>
        </Card>

        {/* Preflight Checklist Card */}
        <Card className="gap-0 p-5 border-border/80 bg-card shadow-xs" aria-label={t(active ? "acars.dashboard.tracking" : "acars.dashboard.preflight")}>
          <div className="mb-4 flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold tracking-tight text-foreground">{t(active ? "acars.dashboard.tracking" : "acars.dashboard.preflight")}</h3>
            {!active && !p.localMode && (
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t("acars.dashboard.refreshBooking")}
                onClick={p.onRefresh}
                disabled={!connected || p.refreshing}
                className="text-muted-foreground hover:text-foreground"
              >
                <RefreshCw className={cn("h-3.5 w-3.5", p.refreshing && "animate-spin motion-reduce:animate-none")} />
              </Button>
            )}
          </div>
          {p.localMode ? (
            <p className="text-sm leading-relaxed text-muted-foreground">{t("acars.dashboard.localHint")}</p>
          ) : active ? (
            <div className="space-y-4">
              <div
                className={cn(
                  "inline-flex rounded-xl p-3 border",
                  connected ? "bg-emerald-500/10 text-emerald-500 border-emerald-500/20" : "bg-amber-500/10 text-amber-500 border-amber-500/20"
                )}
              >
                <Activity className="h-5 w-5" />
              </div>
              <p className="text-sm font-semibold text-foreground">{t(finishing ? "acars.finishing" : connected ? "acars.dashboard.trackingActive" : "acars.dashboard.connectionLost")}</p>
              <p className="text-xs leading-relaxed text-muted-foreground">{t(finishing ? "acars.dashboard.finishHint" : connected ? "acars.dashboard.trackingHint" : "acars.dashboard.reconnectHint")}</p>
            </div>
          ) : (
            <ul className="space-y-3">
              {checklist.map(({ label, done }) => (
                <li key={label} className="flex items-center gap-2.5 text-xs">
                  <span
                    className={cn(
                      "flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors",
                      done ? "bg-emerald-500/15 text-emerald-500 border-emerald-500/30" : "bg-muted/40 text-muted-foreground/40 border-border/80"
                    )}
                  >
                    {done ? <Check className="h-3 w-3 stroke-[2.5]" /> : <Circle className="h-1.5 w-1.5 fill-current" />}
                  </span>
                  <span className={cn("font-medium transition-colors", done ? "text-foreground" : "text-muted-foreground")}>{label}</span>
                  <span className="sr-only">{t(done ? "acars.dashboard.complete" : "acars.dashboard.pending")}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Cabin Audio Card */}
      <Card
        className="gap-0 flex-row flex-wrap items-center justify-between gap-4 py-3.5 px-5 border-border/80 bg-card shadow-xs"
        aria-label={t("acars.cabinAudio")}
      >
        <div className="flex items-center gap-3">
          <div className="rounded-lg bg-muted/60 p-2 text-muted-foreground border border-border/60">
            <Headphones className="h-4 w-4" />
          </div>
          <div>
            <h4 className="text-sm font-semibold tracking-tight text-foreground">{t("acars.cabinAudio")}</h4>
            <p className="text-xs text-muted-foreground">{t("acars.dashboard.volume")}</p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t(p.volume === 0 ? "acars.dashboard.unmute" : "acars.dashboard.mute")}
            onClick={() => p.onVolumeChange(p.volume === 0 ? lastVolume.current : 0)}
            className="text-muted-foreground hover:text-foreground"
          >
            {p.volume === 0 ? <VolumeX className="h-4 w-4" /> : <Volume2 className="h-4 w-4" />}
          </Button>

          <Slider
            aria-label={t("acars.cabinAudio")}
            value={[p.volume]}
            max={100}
            step={1}
            onValueChange={([val]) => p.onVolumeChange(val)}
            className="w-24 sm:w-28"
          />

          <span className="font-mono text-xs font-semibold text-foreground w-8 text-right tabular-nums">
            {p.volume}%
          </span>
        </div>
      </Card>
    </div>
  );
}
